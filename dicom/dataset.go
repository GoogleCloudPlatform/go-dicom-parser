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

// Package dicom is the core package of the dicomparser library. It provides functions and data
// structures for manipulating the DICOM file format as specified in
// [http://dicom.nema.org/medical/dicom/current/output/pdf/part05.pdf].
//
// The package is divided into two levels of abstraction for manipulating the DICOM file format.
// The low level API consists of streaming interfaces like DataElementIterator, BulkDataIterator,
// and BulkDataReader. The high level API consists of helper functions like Parse, which internally
// call the low level API and transform the streaming interfaces into more convenient, non-streaming
// interfaces. For example, Parse will transform DataElementIterator into a collection of
// DataElements, known as a DataSet. During this transformation, Parse will also transform any
// streaming interfaces within a DataElement into the non-streaming interfaces. For example
// Parse can transform BulkDataIterator into []BulkDataReference.
package dicom

// DataElementTag is a unique identifier for a Data Element composed of an unordered pair
// of numbers called the group number and the element number as specified in
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_3.10.
//
// The least significant 16 bits is the element number. The most significant 16 bits is the group
// number.
type DataElementTag uint32

// GroupNumber returns the group number component of the DataElementTag
func (t DataElementTag) GroupNumber() uint16 {
	return uint16(t >> 16)
}

// ElementNumber returns the element number component of the DataElementTag
func (t DataElementTag) ElementNumber() uint16 {
	return uint16(t & 0xFFFF)
}

// IsMetadataElement is true if and only if the Data Element is a meta data element
func (t DataElementTag) IsMetadataElement() bool {
	return t.GroupNumber() == uint16(0x0002)
}

// DataElement models a DICOM Data Element as defined in
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_3.10
type DataElement struct {
	Tag DataElementTag

	// Value Representation
	VR *VR

	// ValueField represents the field within a Data Element that contains its value(s)
	// Can be any of of the following types:
	// []string,
	// []byte
	// []int16,
	// []uint16,
	// []int32,
	// []uint32,
	// []float32,
	// []float64
	// []BulkDataReference
	// BulkDataIterator
	// *Sequence
	ValueField interface{}

	// ValueLength is equal to the length of the ValueField in bytes.
	// Can be equal to 0xFFFFFFFF to represent an undefined length:
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.1
	ValueLength uint32
}

// DataSet models a DICOM Data Set as defined
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_3.10
type DataSet struct {
	// Elements is a map of DataElement tags to *DataElement
	Elements map[uint32]*DataElement
}
