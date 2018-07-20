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
	"sort"
	"strconv"
	"strings"
)

// DataElementValue represents the value field of a Data Element
type DataElementValue interface {}

// DataElementTag is a unique identifier for a Data Element composed of an unordered pair
// of numbers called the group number and the element number as specified in
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_3.10.
//
// The least significant 16 bits is the element number. The most significant 16 bits is the group
// number.
type DataElementTag uint32

func (t DataElementTag) String() string {
	return fmt.Sprintf("(%04X,%04X)", t.GroupNumber(), t.ElementNumber())
}

// GroupNumber returns the group number component of the DataElementTag
func (t DataElementTag) GroupNumber() uint16 {
	return uint16(t >> 16)
}

// ElementNumber returns the element number component of the DataElementTag
func (t DataElementTag) ElementNumber() uint16 {
	return uint16(t & 0xFFFF)
}

// IsPrivate returns true if the DataElementTag is a private data element as defined in
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.8.1
func (t DataElementTag) IsPrivate() bool {
	return t.GroupNumber()%2 != 0
}

// IsPrivateCreator returns true if the DataElementTag is a private creator element as specified in
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.8.1
func (t DataElementTag) IsPrivateCreator() bool {
	if !t.IsPrivate() {
		return false
	}

	// Private Creator Elements are of the form (gggg,0010-00F0) where gggg is odd
	return 0x0010 <= t.ElementNumber() && t.ElementNumber() <= 0x00FF
}

// IsMetaElement returns true if the DataElementTag is a file meta element as defined in
// http://dicom.nema.org/medical/dicom/current/output/html/part10.html#sect_7.1
func (t DataElementTag) IsMetaElement() bool {
	return t.GroupNumber() == 0x0002
}

// DictionaryVR returns the VR of this DataElementTag as defined in the DICOM data dictionary
// http://dicom.nema.org/medical/dicom/current/output/html/part06.html. When the dictionary
// specifies multiple VRs, the last one in VR row is chosen. If the tag cannot be found in the data
// dictionary, UNVR is returned.
func (t DataElementTag) DictionaryVR() *VR {
	if t.IsPrivateCreator() {
		// private creator elements have LO VR as specified in
		// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.8.1
		return LOVR
	}

	if t.ElementNumber() == 0 {
		// support group length elements of the form (gggg,0000)
		return ULVR
	}

	vr, err := t.dictionaryVR()
	if err != nil {
		return UNVR
	}
	return vr
}

func (t DataElementTag) dictionaryVR() (*VR, error) {
	// see comment above declaration of vrWildcardMasks
	for _, m := range vrWildcardMasks {
		normalizedTag := DataElementTag(uint32(t) &^ m)

		if vrName, ok := singleValueTagVRMap[normalizedTag]; ok {
			return lookupVRByName(vrName)
		}

		if vrName, ok := wildcardTagVRMap[normalizedTag]; ok {
			return lookupVRByName(vrName)
		}
	}

	return nil, fmt.Errorf("VR for tag %v not found", t)
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
	// [][]byte
	// []int16,
	// []uint16,
	// []int32,
	// []uint32,
	// []float32,
	// []float64
	// []BulkDataReference
	// BulkDataIterator
	// *Sequence
	// SequenceIterator
	ValueField DataElementValue

	// ValueLength is equal to the length of the ValueField in bytes.
	// Can be equal to 0xFFFFFFFF to represent an undefined length:
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.1
	ValueLength uint32
}

func (e *DataElement) String() string {
	return e.string(0)
}

func (e *DataElement) string(indentLvl int) string {
	val := ""
	indent := strings.Repeat(">", indentLvl)

	switch field := e.ValueField.(type) {
	case *Sequence:
		val += field.string(indentLvl)
	default:
		val += fmt.Sprintf("%v", field)
	}

	return fmt.Sprintf("%v%v %v #%v %v", indent, e.Tag, e.VR.Name, e.ValueLength, val)
}

// IntValue returns the first value of ValueField as an int64 if it can safely do so. If it cannot
// safely do so, an error is returned.
//
// Note since DICOM does not support binary integers larger than 32-bits. This is safe as long as
// there is no integer string that overflows an int64.
func (e *DataElement) IntValue() (int64, error) {
	switch v := e.ValueField.(type) {
	case []string:
		if len(v) > 0 {
			return strconv.ParseInt(v[0], 10, 64)
		}
	case []int8:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	case []uint8:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	case []int16:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	case []uint16:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	case []int32:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	case []uint32:
		if len(v) > 0 {
			return int64(v[0]), nil
		}
	}

	return 0, fmt.Errorf("unexpected type %T (expected integer array or integer string)", e.ValueField)
}

// StringValue value returns the first element of ValueField as a string if ValueField is a string
// slice with at least 1 value. If this is not the case, an error is returned.
func (e *DataElement) StringValue() (string, error) {
	slice, ok := e.ValueField.([]string)
	if !ok {
		return "", fmt.Errorf("unexpected type %T (expected string array)", e.ValueField)
	}
	if len(slice) > 0 {
		return slice[0], nil
	}

	return "", fmt.Errorf("expected non-empty string array")
}

// DataSet models a DICOM Data Set as defined
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_3.10
type DataSet struct {
	// Elements is a map of DataElement tags to *DataElement
	Elements map[DataElementTag]*DataElement

	// Length returns the number of bytes used to store the DataSet in the DICOM file. Can be equal
	// to UndefinedLength (or equivalently 0xFFFFFFFF) to represent undefined length
	Length uint32
}

func (d *DataSet) String() string {
	return d.string(0)
}

func (d *DataSet) string(indentLvl int) string {
	lines := make([]string, 0)
	for _, tag := range d.SortedTags() {
		elem := d.Elements[tag]
		lines = append(lines, elem.string(indentLvl))
	}
	return strings.Join(lines, "\n")
}

// SortedTags returns a copy of the DataElementTags in the DataSet in ascending sorted order
func (d *DataSet) SortedTags() []DataElementTag {
	tags := make([]DataElementTag, 0)
	for tag := range d.Elements {
		tags = append(tags, DataElementTag(tag))
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i] < tags[j]
	})

	return tags
}
