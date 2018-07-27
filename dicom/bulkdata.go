// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dicom

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
)

// BulkDataReference describes the location of a contiguous sequence of bytes in a file
type BulkDataReference struct {
	Reference ByteRegion
}

// ByteRegion is a contiguous sequence of bytes in a file described by an Offset and a length
type ByteRegion struct {
	Offset int64
	Length int64
}

// BulkDataBuffer represents a contiguous sequence of bytes buffered into memory from a file
type BulkDataBuffer interface {
	DataElementValue

	// Data returns a reference to the underlying data in the BulkDataBuffer. Implementations shall
	// guarantee O(1) complexity.
	Data() [][]byte

	// Length returns the total length of this bulk data.
	// Will *not* add a padding byte.
	// May return UndefinedLength for encapsulated data. Always returns >= 0 with
	// 0 indicating an empty buffer.
	Length() int64

	write(w io.Writer, syntax transferSyntax) error
}

// NewBulkDataBuffer returns a DataElementValue representing a raw sequence of bytes.
func NewBulkDataBuffer(b ...[]byte) BulkDataBuffer {
	return bytesValue(b)
}

// NewEncapsulatedFormatBuffer returns a DataElementValue representing the encapsulated image
// format. The offset table is assumed to be the basic offset table fragment and fragments is
// assumed to be the remaining image fragments (excluding the basic offset table fragment). To
// specify no offsetTable, offsetTable can be set to an empty slice.
func NewEncapsulatedFormatBuffer(offsetTable []byte, fragments ...[]byte) BulkDataBuffer {
	buff := [][]byte{offsetTable}

	for _, fragment := range fragments {
		buff = append(buff, fragment)
	}

	return encapsulatedFormatBuffer(buff)
}

type bytesValue [][]byte

func (b bytesValue) write(w io.Writer, syntax transferSyntax) error {
	totalLength := b.Length()

	idx := 0
	return writeByteFragments(w, func() (io.Reader, error) {
		if idx >= len(b) {
			return nil, io.EOF
		}

		r := bytes.NewReader(b[idx])
		if idx == len(b)-1 && totalLength%2 != 0 {
			// To achieve even length, append trailing null byte to the last fragment.
			r = bytes.NewReader(append(b[idx], 0x00))
		}

		idx++
		return r, nil
	})
}

func (b bytesValue) Data() [][]byte {
	return b
}

func (b bytesValue) Length() int64 {
	totalLength := 0
	for _, fragment := range b {
		totalLength += len(fragment)
	}
	return int64(totalLength)
}

type encapsulatedFormatBuffer [][]byte

func (b encapsulatedFormatBuffer) write(w io.Writer, syntax transferSyntax) error {
	idx := 0
	return writeEncapsulatedFormat(w, syntax.byteOrder(), func() (io.Reader, error) {
		if idx >= len(b) {
			return nil, io.EOF
		}

		r := bytes.NewReader(b[idx])
		if len(b[idx])%2 != 0 {
			r = bytes.NewReader(append(b[idx], 0x00))
		}

		idx++
		return r, nil
	})
}

func (b encapsulatedFormatBuffer) Data() [][]byte {
	return b
}

func (b encapsulatedFormatBuffer) Length() int64 {
	return UndefinedLength
}

// BulkDataReader represents a streamable contiguous sequence of bytes within a file
type BulkDataReader struct {
	io.Reader

	// Offset is the number of bytes in the file preceding the bulk data described
	// by the BulkDataReader
	Offset int64
}

// Close discards all bytes in the reader
func (r *BulkDataReader) Close() error {
	_, err := io.Copy(ioutil.Discard, r)
	return err
}

// BulkDataIterator represents a sequence of BulkDataReaders.
type BulkDataIterator interface {
	// Next returns the next BulkDataReader in the iterator and discards all bytes from all previous
	// BulkDataReaders returned from Next. If there are no remaining BulkDataReader in the iterator,
	// the error io.EOF is returned
	Next() (*BulkDataReader, error)

	// Close discards all remaining BulkDataReaders in the iterator. Any previously returned
	// BulkDataReaders from calls to Next are also emptied.
	Close() error

	// ToBuffer converts the BulkDataIterator into its equivalent BulkDataBuffer. Behaviour after
	// Next has been called is undefined. This will close the iterator.
	ToBuffer() (BulkDataBuffer, error)

	// Length returns the total length of this BulkData.
	// Will *not* add a padding byte.
	// May return -1 if length is unknown or UndefinedLength for encapsulate
	// data. Required to be >= 0 when Constructing a DataSet with a
	// BulkDataIterator. See specific implementations for how to specify
	// explicit length.
	Length() int64

	write(w io.Writer, syntax transferSyntax) error
}

// NewEncapsulatedFormatIterator returns an iterator over byte fragments. r must read the ValueField
// of a DataElement in the encapsulated format as described in the DICOM standard part5 linked
// below. offset is the number of bytes preceding the ValueField in the DICOM file.
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
func NewEncapsulatedFormatIterator(r io.Reader, offset int64) BulkDataIterator {
	dr := &dcmReader{cr: &countReader{r: r, bytesRead: offset}}
	return &encapsulatedFormatIterator{dr, nil, false}
}

// NewBulkDataIterator returns a BulkDataIterator with a single BulkDataReader
// described by r and offset. Offset can safely be set to 0 with the
// understanding that the BulkDataReaders won't have the proper offset set.
func NewBulkDataIterator(r io.Reader, offset int64) BulkDataIterator {
	cr := &countReader{r: r, bytesRead: offset}
	return &oneShotIterator{cr: cr, empty: false, length: -1}
}

// NewBulkDataIteratorWithLength returns a BulkDataIterator with an explicit
// length. A length is required when Constructing a DataSet and native bulk
// data must write out an explicit length before the bulk data.
func NewBulkDataIteratorWithLength(r io.Reader, offset, length int64) BulkDataIterator {
	cr := &countReader{r: r, bytesRead: offset}
	return &oneShotIterator{cr: cr, empty: false, length: length}
}

// oneShotIterator is a BulkDataIterator that contains exactly one BulkDataReader
type oneShotIterator struct {
	cr    *countReader
	empty bool

	// The length of the underlying data. Might not be available after parsing
	// but needs to be present to be able to Construct a DataSet without buffering
	// the whole reader into memory.
	length int64
}

func (it *oneShotIterator) Next() (*BulkDataReader, error) {
	if it.empty {
		return nil, io.EOF
	}

	it.empty = true

	return &BulkDataReader{it.cr, it.cr.bytesRead}, nil
}

func (it *oneShotIterator) Close() error {
	if _, err := io.Copy(ioutil.Discard, it.cr); err != nil {
		return fmt.Errorf("closing bulk data: %v", err)
	}

	it.empty = true

	return nil
}

func (it *oneShotIterator) ToBuffer() (BulkDataBuffer, error) {
	b, err := ioutil.ReadAll(it.cr)
	if err != nil {
		return nil, fmt.Errorf("collecting fragments into memory: %v", err)
	}
	return NewBulkDataBuffer(b), nil
}

func (it *oneShotIterator) Length() int64 {
	return it.length
}

func (it *oneShotIterator) write(w io.Writer, syntax transferSyntax) error {
	return writeByteFragments(w, func() (io.Reader, error) {
		return it.Next()
	})
}

// encapsulatedFormatIterator represents image pixel data (7FE0,0010) in encapsulated format as
// described in http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4.
type encapsulatedFormatIterator struct {
	dr            *dcmReader
	currentReader *BulkDataReader
	empty         bool
}

// Next returns the next fragment of the pixel data. The first return from Next will be the
// Basic Offset Table if present or an empty BulkDataReader otherwise. When Next is called,
// any previously returned BulkDataReaders from previous calls to Next will be emptied. When there
// are no remaining fragments in the iterator, the error io.EOF is returned.
func (it *encapsulatedFormatIterator) Next() (*BulkDataReader, error) {
	if it.empty {
		return nil, io.EOF
	}

	if it.currentReader != nil {
		if err := it.currentReader.Close(); err != nil {
			return nil, err
		}
	}

	tag, err := processItemTag(it.dr, binary.LittleEndian)
	if err != nil {
		return nil, fmt.Errorf("reading tag in encapsulated format fragment: %v", err)
	}
	if tag == SequenceDelimitationItemTag {
		return nil, it.terminate()
	}

	length, err := it.dr.UInt32(binary.LittleEndian)
	if err != nil {
		return nil, err
	}
	if length >= UndefinedLength {
		return nil, fmt.Errorf("expected fragment to be of explicit length")
	}

	currentReaderBytes := limitCountReader(it.dr.cr, int64(length))
	it.currentReader = &BulkDataReader{currentReaderBytes, currentReaderBytes.bytesRead}

	return it.currentReader, nil
}

// Close discards all fragments in the iterator
func (it *encapsulatedFormatIterator) Close() error {
	for r, err := it.Next(); err != io.EOF; r, err = it.Next() {
		if err != nil {
			return fmt.Errorf("reading next reader: %v", err)
		}
		if err := r.Close(); err != nil {
			return fmt.Errorf("discarding reader on Close: %v", err)
		}
	}

	return nil
}

func (it *encapsulatedFormatIterator) ToBuffer() (BulkDataBuffer, error) {
	fragments, err := CollectFragments(it)
	if err != nil {
		return nil, fmt.Errorf("collecting fragments of encapsulated format: %v", err)
	}
	return encapsulatedFormatBuffer(fragments), nil
}

func (it *encapsulatedFormatIterator) Length() int64 {
	return UndefinedLength
}

func (it *encapsulatedFormatIterator) write(w io.Writer, syntax transferSyntax) error {
	return writeEncapsulatedFormat(w, syntax.byteOrder(), func() (io.Reader, error) {
		return it.Next()
	})
}

func (it *encapsulatedFormatIterator) terminate() error {
	_, err := it.dr.UInt32(binary.LittleEndian)
	if err != nil {
		return fmt.Errorf("reading 32 bit length of sequence delimitation item: %v", err)
	}
	it.empty = true
	return io.EOF
}

// writeByteFragments writes the concatenated byte fragments in the fragmentProvider to w
func writeByteFragments(w io.Writer, fragmentProvider func() (io.Reader, error)) error {
	for fragment, err := fragmentProvider(); err != io.EOF; fragment, err = fragmentProvider() {
		if err != nil {
			return fmt.Errorf("retrieving next fragment: %v", err)
		}
		if _, err := io.Copy(w, fragment); err != nil {
			return fmt.Errorf("writing fragment: %v", err)
		}
	}
	return nil
}

// writeEncapsulatedFormat writes the byte fragments in the BulkDataIterator in the encapsulated
// format. The first fragment provided by fragmentProvider is assumed to be the basic offset table.
func writeEncapsulatedFormat(w io.Writer, order binary.ByteOrder, fragmentProvider func() (io.Reader, error)) error {
	dw := &dcmWriter{w}

	for fragment, err := fragmentProvider(); err != io.EOF; fragment, err = fragmentProvider() {
		if err != nil {
			return err
		}
		if err := dw.Tag(order, ItemTag); err != nil {
			return fmt.Errorf("writing fragment tag: %v", err)
		}

		// TODO provide way of stream writing the fragments without buffering
		buff, err := ioutil.ReadAll(fragment)
		if err != nil {
			return fmt.Errorf("buffering fragment: %v", err)
		}

		if err := dw.UInt32(order, uint32(len(buff))); err != nil {
			return fmt.Errorf("writing fragment length: %v", err)
		}
		if err := dw.Bytes(buff); err != nil {
			return fmt.Errorf("writing fragment: %v", err)
		}
	}

	if err := dw.Tag(order, SequenceDelimitationItemTag); err != nil {
		return fmt.Errorf("writing fragment delimitation tag: %v", err)
	}
	if err := dw.UInt32(order, 0); err != nil {
		return fmt.Errorf("writing delimiter length: %v", err)
	}

	return nil
}
