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
	"fmt"
	"io"
)

// DataElementIterator represents an iterator over a DataSet's DataElements
type DataElementIterator interface {
	// NextElement returns the next DataElement in the DataSet. If there is no next DataElement, the
	// error io.EOF is returned. In Addition, if any previously returned DataElements contained
	// iterable objects like SequenceIterator, BulkDataIterator, these iterators are emptied.
	NextElement() (*DataElement, error)

	// Close discards all remaining DataElements in the iterator
	Close() error

	syntax() transferSyntax
}

// NewDataElementIterator creates a DataElementIterator from a DICOM file. The implementation
// returned will consume input from the io.Reader given as needed.
func NewDataElementIterator(r io.Reader) (DataElementIterator, error) {
	dr := newDcmReader(r)
	if err := readDicomSignature(dr); err != nil {
		return nil, err
	}

	metaHeaderBytes, err := bufferMetadataHeader(dr)
	if err != nil {
		return nil, fmt.Errorf("reading meta header: %v", err)
	}

	syntax, err := findSyntax(metaHeaderBytes)
	if err != nil {
		return nil, fmt.Errorf("finding transfer syntax: %v", err)
	}

	metaIter, err := newDataElementIterator(newDcmReader(bytes.NewBuffer(metaHeaderBytes)), defaultMetaData)
	if err != nil {
		return nil, fmt.Errorf("creating meta element iterator: %v", err)
	}

	metadata := dicomMetaData{syntax, defaultCharacterRepertoire}
	return &dataElementIterator{dr, metadata, nil, false, metaIter}, nil
}

// newDataElementIterator creates a DataElementIterator from a byte stream that excludes header info
// (preamble and metadata elements)
func newDataElementIterator(r *dcmReader, metaData dicomMetaData) (DataElementIterator, error) {
	return &dataElementIterator{r, metaData, nil, false, emptyElementIterator{metaData}}, nil
}

type dataElementIterator struct {
	dr             *dcmReader
	metaData       dicomMetaData
	currentElement *DataElement
	empty          bool
	metaHeader     DataElementIterator
}

func (it *dataElementIterator) NextElement() (*DataElement, error) {
	metaElem, err := it.metaHeader.NextElement()
	if err == io.EOF {
		return it.nextDataSetElement()
	}
	if err != nil {
		return nil, err
	}
	return metaElem, nil
}

func (it *dataElementIterator) syntax() transferSyntax {
	return it.metaData.syntax
}

func (it *dataElementIterator) nextDataSetElement() (*DataElement, error) {
	if it.empty {
		return nil, io.EOF
	}
	if err := it.closeCurrent(); err != nil {
		return nil, fmt.Errorf("closing: %v", err)
	}

	element, err := parseDataElement(it.dr, it.metaData)
	if err == io.EOF {
		it.empty = true
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("parsing element: %v", err)
	}

	it.currentElement = element

	return it.currentElement, nil
}

func (it *dataElementIterator) Close() error {
	// empty the iterator
	for _, err := it.NextElement(); err != io.EOF; _, err = it.NextElement() {
		if err != nil {
			return fmt.Errorf("unexpected error closing iterator: %v", err)
		}
	}
	return nil
}

// closeCurrent ensures the iterator is ready to read the next DataElement. If this iterator
// previously returned a stream of bytes such as a BulkDataIterator, we need to make sure this
// previously returned stream is emptied in order to advance the input to the bytes of the
// next DataElement. This pattern is similar to the implementation of multipart.Reader in the
// go standard library. https://golang.org/src/mime/multipart/multipart.go?s=8400:8697#L303
func (it *dataElementIterator) closeCurrent() error {
	if it.currentElement == nil {
		return nil
	}

	if closer, ok := it.currentElement.ValueField.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

func readDicomSignature(r *dcmReader) error {
	if err := r.Skip(128); err != nil {
		return fmt.Errorf("skipping preamble: %v", err)
	}

	magic, err := r.String(4)
	if err != nil {
		return fmt.Errorf("reading DICOM signature: %v", err)
	}

	if magic != "DICM" {
		return fmt.Errorf("wrong DICOM signature: %v", magic)
	}

	return nil
}

func bufferMetadataHeader(dr *dcmReader) ([]byte, error) {
	firstElemBytes, err := dr.Bytes(4 /*tag*/ + 2 /*vr*/ + 2 /*len*/ + 4 /*UL=4bytes*/)
	if err != nil {
		return nil, fmt.Errorf("buffering bytes of File​Meta​Information​Group​Length: %v", err)
	}
	firstElem, err := parseDataElement(
		newDcmReader(bytes.NewBuffer(firstElemBytes)), defaultMetaData)
	if err != nil {
		return nil, fmt.Errorf("parsing FileMetaInformationGroupLength element: %v", err)
	}
	if metaGroupLength, ok := firstElem.ValueField.([]uint32); ok {
		if len(metaGroupLength) != 1 {
			return nil, fmt.Errorf("expected 1 value for meta group lengths")
		}
		remainderBytes, err := dr.Bytes(int64(metaGroupLength[0]))
		if err != nil {
			return nil, fmt.Errorf("buffering the file meta elements: %v", err)
		}

		return append(firstElemBytes, remainderBytes...), nil
	}

	return nil, fmt.Errorf("wrong type for File​Meta​Information​Group​Length. Got %v, want []uint32", firstElem.ValueField)
}

func findSyntax(metaHeaderBytes []byte) (transferSyntax, error) {
	var syntax transferSyntax
	metaDCMReader := newDcmReader(bytes.NewBuffer(metaHeaderBytes))
	metaIter, err := newDataElementIterator(metaDCMReader, defaultMetaData)
	if err != nil {
		return syntax, fmt.Errorf("creating iterator for file meta elements: %v", err)
	}

	for elem, err := metaIter.NextElement(); err != io.EOF; elem, err = metaIter.NextElement() {
		if err != nil {
			return syntax, fmt.Errorf("reading meta element: %v", err)
		}
		if elem.Tag == TransferSyntaxUIDTag {
			return findSyntaxFromElement(elem)
		}
	}

	return syntax, fmt.Errorf("transfer syntax not found")
}

func findSyntaxFromElement(element *DataElement) (transferSyntax, error) {
	var syntax transferSyntax
	ids, ok := element.ValueField.([]string)
	if !ok {
		return syntax, fmt.Errorf("expected type []string for transfer syntax element")
	}
	if len(ids) != 1 {
		return syntax, fmt.Errorf("expected 1 value length for transfer syntax")
	}

	return lookupTransferSyntax(ids[0]), nil
}

type emptyElementIterator struct {
	metaData dicomMetaData
}

func (it emptyElementIterator) NextElement() (*DataElement, error) {
	return nil, io.EOF
}

func (it emptyElementIterator) syntax() transferSyntax {
	return it.metaData.syntax
}

func (it emptyElementIterator) Close() error {
	return nil
}
