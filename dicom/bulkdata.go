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

	write(w io.Writer, syntax transferSyntax) error
}

// oneShotIterator is a BulkDataIterator that contains exactly one BulkDataReader
type oneShotIterator struct {
	cr    *countReader
	empty bool
}

func newOneShotIterator(r *countReader) BulkDataIterator {
	return &oneShotIterator{r, false}
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

func newEncapsulatedFormatIterator(dr *dcmReader) (BulkDataIterator, error) {
	return &encapsulatedFormatIterator{dr, nil, false}, nil
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

func (it *encapsulatedFormatIterator) write(w io.Writer, syntax transferSyntax) error {
	return writeEncapsulatedFormat(w, syntax.ByteOrder, func() (io.Reader, error) {
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
