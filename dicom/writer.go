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
	"fmt"
	"io"
	"math"
)

// DataElementWriter writes DataElements one at a time
type DataElementWriter interface {
	WriteElement(element *DataElement) error
}

var errExpectedMetaHeader = fmt.Errorf("expected header to only contain file meta elements, " +
	"use DataSet.MetaElements to filter DataSet")

// NewDataElementWriter writes the DICOM preamble, signature, and meta header to w and returns a
// DataElementWriter that writes DataElements in the transfer syntax specified by the header.
// The options are applied in the order given to all DataElements including File Meta Elements
// before being written to w.
func NewDataElementWriter(w io.Writer, header *DataSet, opts ...ConstructOption) (DataElementWriter, error) {
	if !header.isMetaHeader() {
		return nil, errExpectedMetaHeader
	}

	syntax, err := header.transferSyntax()
	if err != nil {
		return nil, fmt.Errorf("getting transfer syntax from header: %v", err)
	}
	if syntax == deflatedExplicitVRLittleEndian {
		return nil, fmt.Errorf("writing in the deflated syntax is not supported yet")
	}

	dw := &dcmWriter{w}
	if err := writeDicomSignature(dw); err != nil {
		return nil, err
	}

	// Process meta header elements before re-calculating the FileMetaInformationGroupLength in case
	// an option modifies the length of a DataElement.
	for tag, element := range header.Elements {
		element, err := processElementForConstruct(element, explicitVRLittleEndian, opts...)
		if err != nil {
			return nil, fmt.Errorf("processing element: %v", err)
		}
		header.Elements[tag] = element
	}

	// The FileMetaInformationGroupLength element is a critical component of the Meta Header. It
	// stores how long the meta header is. Thus, we need to re-calculate it properly.
	metaGroupLengthElement, err := createMetaGroupLengthElement(header)
	if err != nil {
		return nil, fmt.Errorf("creating meta group length element: %v", err)
	}
	header.Elements[FileMetaInformationGroupLengthTag] = metaGroupLengthElement

	// Meta elements are always written in the Explicit VR Little Endian syntax in ascending order.
	for _, element := range header.SortedElements() {
		if err := writeDataElement(dw, explicitVRLittleEndian, element); err != nil {
			return nil, fmt.Errorf("writing data element: %v", err)
		}
	}

	return &dataElementWriter{dw, syntax, opts}, nil
}

type dataElementWriter struct {
	dw     *dcmWriter
	syntax transferSyntax
	opts   []ConstructOption
}

func (dew *dataElementWriter) WriteElement(element *DataElement) error {
	element, err := processElementForConstruct(element, dew.syntax, dew.opts...)
	if err != nil {
		return err
	}
	return writeDataElement(dew.dw, dew.syntax, element)
}

func writeDicomSignature(dw *dcmWriter) error {
	if err := dw.Bytes(make([]byte, 128)); err != nil {
		return fmt.Errorf("writing DICOM preamble: %v", err)
	}

	if err := dw.String("DICM"); err != nil {
		return fmt.Errorf("writing DICOM signature: %v", err)
	}

	return nil
}

func createMetaGroupLengthElement(header *DataSet) (*DataElement, error) {
	// Please refer to the DICOM Standard Part 10 for information on the File Meta Information Group
	// Length. http://dicom.nema.org/medical/dicom/current/output/html/part10.html#sect_7.1

	size := uint32(0)
	for _, element := range header.Elements {
		if element.Tag == FileMetaInformationGroupLengthTag {
			// The FileMetaGroupLength byte count excludes itself from the calculation.
			continue
		}
		size += explicitVRLittleEndian.elementSize(element.VR, element.ValueLength)
	}

	return &DataElement{
		Tag:         FileMetaInformationGroupLengthTag,
		VR:          FileMetaInformationGroupLengthTag.DictionaryVR(),
		ValueField:  []uint32{size},
		ValueLength: 4, // 4bytes = sizeof uint32
	}, nil
}

func processElementForConstruct(element *DataElement, syntax transferSyntax, opts ...ConstructOption) (*DataElement, error) {
	element, err := applyConstructOptions(element, syntax, opts...)
	if err != nil {
		return nil, fmt.Errorf("applying construct options: %v", err)
	}

	if seq, ok := element.ValueField.(*Sequence); ok {
		processedSeq, err := processSequenceForConstruct(seq, syntax, opts...)
		if err != nil {
			return nil, fmt.Errorf("processing sequence: %v", err)
		}
		element.ValueField = processedSeq
	}

	return element, nil
}

func applyConstructOptions(element *DataElement, syntax transferSyntax, opts ...ConstructOption) (*DataElement, error) {
	var err error
	for i, opt := range opts {
		element, err = opt.transform(element)
		if err != nil {
			return nil, fmt.Errorf("applying option %v: %v", i, err)
		}
	}

	// As documented in NewConstructOption, after a transforms are applied, the length is
	// re-calculated and VRs added from the Data Dictionary if nil
	vr := element.VR
	if vr == nil {
		vr = element.Tag.DictionaryVR()
	}

	length, err := calculateElementLength(element, syntax)
	if err != nil {
		return nil, fmt.Errorf("calculating element length: %v", err)
	}

	return &DataElement{element.Tag, vr, element.ValueField, length}, nil
}

func calculateElementLength(element *DataElement, syntax transferSyntax) (uint32, error) {
	if element.ValueLength == UndefinedLength {
		return UndefinedLength, nil
	}

	numBytes := int64(0)

	switch v := element.ValueField.(type) {
	case []string:
		for _, s := range v {
			numBytes += int64(len(s))
		}
		if len(v) > 0 { // requires "/" delimiter
			numBytes += int64(len(v)) - 1
		}
	case []int16:
		numBytes = int64(len(v)) * 2
	case []uint16:
		numBytes = int64(len(v)) * 2
	case []int32:
		numBytes = int64(len(v)) * 4
	case []uint32:
		numBytes = int64(len(v)) * 4
	case []float32:
		numBytes = int64(len(v)) * 4
	case []float64:
		numBytes = int64(len(v)) * 8
	case *Sequence:
		seqLen, err := calculateSequenceLength(v, syntax)
		if err != nil {
			return 0, fmt.Errorf("calculating sequence length: %v", err)
		}
		numBytes = int64(seqLen)
	case SequenceIterator:
		numBytes = UndefinedLength // TODO support explicit length sequence construction
	case BulkDataBuffer:
		numBytes = v.Length()
		if numBytes < 0 {
			return 0, fmt.Errorf("explicit length must be provided to write BulkDataBuffer")
		}
	case BulkDataIterator:
		numBytes = v.Length()
		if numBytes < 0 {
			return 0, fmt.Errorf("explicit length must be provided to write BulkDataIterator")
		}
	default:
		return 0, fmt.Errorf("unexpected ValueField type %T", element.ValueField)
	}

	if numBytes >= math.MaxUint32 {
		return UndefinedLength, nil
	}

	if numBytes%2 != 0 {
		numBytes++
	}

	return uint32(numBytes), nil
}

func calculateSequenceLength(seq *Sequence, syntax transferSyntax) (uint32, error) {
	size := int64(0)
	for _, item := range seq.Items {
		itemLen, err := calculateDataSetLength(item, syntax)
		if err != nil {
			return 0, fmt.Errorf("calculating sequence item length: %v", err)
		}
		item.Length = itemLen
		size += tagSize + 4 /*32 bit length*/ + int64(itemLen)
	}

	if size > math.MaxUint32 {
		return UndefinedLength, nil
	}

	return uint32(size), nil
}

func calculateDataSetLength(item *DataSet, syntax transferSyntax) (uint32, error) {
	if item.Length >= UndefinedLength {
		return UndefinedLength, nil
	}

	size := int64(0)
	for _, elem := range item.Elements {
		elemLength, err := calculateElementLength(elem, syntax)
		if err != nil {
			return 0, fmt.Errorf("calculating data set element length: %v", err)
		}
		size += int64(syntax.elementSize(elem.VR, elemLength))
	}

	if size > math.MaxUint32 {
		return UndefinedLength, nil
	}

	return uint32(size), nil
}

func processSequenceForConstruct(sequence *Sequence, syntax transferSyntax, opts ...ConstructOption) (*Sequence, error) {
	ret := &Sequence{Items: []*DataSet{}}
	for _, item := range sequence.Items {
		processedItem, err := processItemForConstruct(item, syntax, opts...)
		if err != nil {
			return nil, fmt.Errorf("processing sequence item: %v", err)
		}
		ret.append(processedItem)
	}
	return ret, nil
}

func processItemForConstruct(dataSet *DataSet, syntax transferSyntax, opts ...ConstructOption) (*DataSet, error) {
	ret := &DataSet{Elements: map[DataElementTag]*DataElement{}, Length: dataSet.Length}
	for _, element := range dataSet.SortedElements() {
		processedElement, err := processElementForConstruct(element, syntax, opts...)
		if err != nil {
			return nil, fmt.Errorf("processing element %s", element.Tag)
		}
		ret.Elements[processedElement.Tag] = processedElement
	}
	return ret, nil
}
