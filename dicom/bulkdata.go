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

// EncapsulatedFormatIterator represents image pixel data (7FE0,0010) in encapsulated format as
// described in http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4.
type EncapsulatedFormatIterator struct {
	dr            *dcmReader
	currentReader *BulkDataReader
	empty         bool
}

func newEncapsulatedFormatIterator(dr *dcmReader) (BulkDataIterator, error) {
	return &EncapsulatedFormatIterator{dr, nil, false}, nil
}

// Next returns the next fragment of the pixel data. The first return from Next will be the
// Basic Offset Table if present or an empty BulkDataReader otherwise. When Next is called,
// any previously returned BulkDataReaders from previous calls to Next will be emptied. When there
// are no remaining fragments in the iterator, the error io.EOF is returned.
func (it *EncapsulatedFormatIterator) Next() (*BulkDataReader, error) {
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
func (it *EncapsulatedFormatIterator) Close() error {
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

func (it *EncapsulatedFormatIterator) terminate() error {
	_, err := it.dr.UInt32(binary.LittleEndian)
	if err != nil {
		return fmt.Errorf("reading 32 bit length of sequence delimitation item: %v", err)
	}
	it.empty = true
	return io.EOF
}
