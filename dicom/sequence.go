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
	"strings"
)

// Sequence models a DICOM sequence
type Sequence struct {
	Items []*DataSet
}

func (seq *Sequence) String() string {
	return seq.string(0)
}

func (seq *Sequence) string(indentLvl int) string {
	lines := make([]string, 0)
	for _, obj := range seq.Items {
		lines = append(lines, obj.string(indentLvl+1))
	}
	return "\n" + strings.Join(lines, "\n")
}

func (seq *Sequence) append(dataSet *DataSet) {
	seq.Items = append(seq.Items, dataSet)
}

// SequenceIterator is an iterator over a DICOM Sequence of Items in the order in which they appear
// in the DICOM file.
type SequenceIterator interface {
	// Next returns the next item in the DICOM Sequence of Items. If there is no next item, the error
	// io.EOF is returned. In addition, any previously returned iterators from Next are emptied.
	Next() (DataElementIterator, error)

	// Close discards all remaining items in the iterator. In addition, any previously returned
	// iterators from calls to Next are emptied.
	Close() error
}

func newSequenceIterator(dr *dcmReader, length uint32, syntax transferSyntax) (SequenceIterator, error) {
	if length < UndefinedLength {
		return &explicitLengthSequenceIterator{dr.Limit(int64(length)), syntax, nil}, nil
	}
	return &undefinedLengthSequenceIterator{dr, syntax, nil, false}, nil
}

type explicitLengthSequenceIterator struct {
	dr             *dcmReader
	syntax         transferSyntax
	currentSeqItem DataElementIterator
}

func (it *explicitLengthSequenceIterator) Next() (DataElementIterator, error) {
	if it.currentSeqItem != nil {
		if err := it.currentSeqItem.Close(); err != nil {
			return nil, err
		}
	}

	tag, err := processItemTag(it.dr, it.syntax.ByteOrder)
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	if tag == SequenceDelimitationItemTag {
		return nil, fmt.Errorf("unexpected sequence delimitation item tag in explicit length sequence")
	}

	it.currentSeqItem, err = newSeqItem(it.dr, it.syntax)

	return it.currentSeqItem, err
}

func (it *explicitLengthSequenceIterator) Close() error {
	return closeSeq(it)
}

type undefinedLengthSequenceIterator struct {
	dr             *dcmReader
	syntax         transferSyntax
	currentSeqItem DataElementIterator
	empty          bool
}

func (it *undefinedLengthSequenceIterator) Next() (DataElementIterator, error) {
	if it.empty {
		return nil, io.EOF
	}
	if it.currentSeqItem != nil {
		if err := it.currentSeqItem.Close(); err != nil {
			return nil, err
		}
	}

	tag, err := processItemTag(it.dr, it.syntax.ByteOrder)
	if err == io.EOF {
		return nil, fmt.Errorf("unexpected EOF in undefined sequence")
	}
	if err != nil {
		return nil, err
	}
	if tag == SequenceDelimitationItemTag {
		return nil, it.terminate()
	}

	it.currentSeqItem, err = newSeqItem(it.dr, it.syntax)

	return it.currentSeqItem, err
}

func (it *undefinedLengthSequenceIterator) terminate() error {
	itemLength, err := it.dr.UInt32(it.syntax.ByteOrder)
	if err != nil {
		return fmt.Errorf("reading 32 bit length of sequence delimitation item: %v", err)
	}
	if itemLength != 0 {
		return fmt.Errorf("expected 0 length on sequence delimiter length")
	}
	// this empty flag is needed for sequences of undefined sequence lengths to prevent the iterator
	// from advancing the input stream past the bytes of the sequence when Next() is called. This is
	// not used for sequences of explicit length because the input stream is wrapped in a
	// io.LimitedReader.
	it.empty = true
	return io.EOF
}

func (it *undefinedLengthSequenceIterator) Close() error {
	return closeSeq(it)
}

func processItemTag(dr *dcmReader, order binary.ByteOrder) (DataElementTag, error) {
	tag, err := dr.Tag(order)
	if err == io.EOF {
		return tag, io.EOF
	}
	if err != nil {
		return tag, fmt.Errorf("unexpected error reading item tag: %v", err)
	}
	if tag != ItemTag && tag != SequenceDelimitationItemTag {
		return tag, fmt.Errorf("invalid item tag in sequnce iterator, got %08X want %08X or %08X",
			tag, ItemTag, SequenceDelimitationItemTag)
	}

	return tag, nil
}

func newSeqItem(dr *dcmReader, syntax transferSyntax) (DataElementIterator, error) {
	itemLength, err := dr.UInt32(syntax.ByteOrder)
	if err != nil {
		return nil, fmt.Errorf("reading sequence item length: %v", err)
	}

	if itemLength >= UndefinedLength {
		return newDataElementIterator(dr, syntax, itemLength), nil
	}

	return newDataElementIterator(dr.Limit(int64(itemLength)), syntax, itemLength), nil
}

func closeSeq(iter SequenceIterator) error {
	for _, err := iter.Next(); err != io.EOF; _, err = iter.Next() {
		if err != nil {
			return err
		}
	}
	return nil
}
