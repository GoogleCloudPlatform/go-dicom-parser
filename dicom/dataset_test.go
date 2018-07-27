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
	"math"
	"reflect"
	"strconv"
	"testing"
)

type arithmeticSeq struct {
	start DataElementTag
	end   DataElementTag
	inc   DataElementTag
}

func TestDataElementTag_String(t *testing.T) {
	got := ItemTag.String()
	want := "(FFFE,E000)"
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDataElementTag_ElementNumber(t *testing.T) {
	tag := DataElementTag(0xFEDCBA98)
	if tag.ElementNumber() != 0xBA98 {
		t.Fatalf("got %v, want %v", tag.ElementNumber(), 0xBA98)
	}
}

func TestDataElementTag_GroupNumber(t *testing.T) {
	tag := DataElementTag(0xFEDCBA98)
	if tag.GroupNumber() != 0xFEDC {
		t.Fatalf("got %v, want %v", tag.GroupNumber(), 0xFEDC)
	}
}

func TestDataElementTag_IsPrivate(t *testing.T) {
	tests := []struct {
		name string
		tag  DataElementTag
		want bool
	}{
		{
			"when group number is odd, the tag is considered private",
			DataElementTag(0x00010000),
			true,
		},
		{
			"when group number is even, the tag is considered non-private",
			DataElementTag(PixelDataTag),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tag.IsPrivate()
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDataElementTag_DictionaryVR(t *testing.T) {
	tests := []struct {
		name   string
		tagSet arithmeticSeq
		want   *VR
	}{
		{
			"Tags of the form (50xx,2610) have VR US",
			arithmeticSeq{0x50002610, 0x50FF2610, 0x00010000},
			USVR,
		},
		{
			"Tags without wildcard lookup",
			arithmeticSeq{MACParametersSequenceTag, MACParametersSequenceTag, 1},
			SQVR,
		},
		{
			// TODO current parser behaviour relies on choosing the last VR from the standard
			"When the Tag has multiple associated VRs, the last one in the dictionary row is chosen",
			arithmeticSeq{GrayLookupTableDataTag, GrayLookupTableDataTag, 1},
			OWVR,
		},
		{
			"When the data dictionary is ambiguous because there is a collision between " +
				"a wildcard entry and an exact match, the  takes precedence",
			arithmeticSeq{TransformLabelTag, TransformLabelTag, 1},
			LOVR,
		},
		{
			"when lookup fails, UNVR is returned",
			arithmeticSeq{0xABCDEF98, 0xABCDEF98, 1},
			UNVR,
		},
		{
			"when the tag belongs to private creator group (gggg,0010-00FF) where gggg is odd, " +
				"the dictionary VR is LO",
			arithmeticSeq{0x80010010, 0x800100FF, 1},
			LOVR,
		},
		{
			"when the tag is a group length element (gggg,0000) the VR is UL",
			arithmeticSeq{0x00020000, 0x0FFF0000, 0x00010000},
			ULVR,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for tag := tc.tagSet.start; tag <= tc.tagSet.end; tag += tc.tagSet.inc {
				got := DataElementTag(tag).DictionaryVR()
				if got != tc.want {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestDataElement_String(t *testing.T) {
	tests := []struct {
		name string
		in   *DataElement
		want string
	}{
		{
			"non-nested data element",
			&DataElement{FileMetaInformationGroupLengthTag, FileMetaInformationGroupLengthTag.DictionaryVR(), []uint32{198}, 4},
			"(0002,0000) UL #4 [198]",
		},
		{
			"sequence data element",
			&DataElement{ReferencedStudySequenceTag, ReferencedStudySequenceTag.DictionaryVR(), &nestedSeq, 34},
			"(0008,1110) SQ #34 \n" +
				">(0008,1155) UI #26 [1.2.840.10008.5.1.4.1.1.4]\n" +
				">(0018,2042) UI #26 [1.2.840.10008.5.1.4.1.1.5]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.String()
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDataElement_IntValue(t *testing.T) {
	intMaxStr := strconv.FormatInt(math.MaxInt64, 10)
	intMinStr := strconv.FormatInt(math.MinInt64, 10)

	tests := []struct {
		name         string
		fieldValues  []interface{}
		wantedValues []int64
	}{
		{
			"non empty []int16 values are ok",
			[]interface{}{[]int16{1}, []int16{1, 2}},
			[]int64{1, 1},
		},
		{
			"non empty []int32 values are ok",
			[]interface{}{[]int32{10}, []int32{12, 2}},
			[]int64{10, 12},
		},
		{
			"non empty []uint16 values are ok",
			[]interface{}{[]int16{10}, []int16{12, 2}},
			[]int64{10, 12},
		},
		{
			"non empty []uint32 values are ok",
			[]interface{}{[]int32{1}, []int32{12, 2}},
			[]int64{1, 12},
		},
		{
			"integer strings are ok",
			[]interface{}{[]string{intMaxStr}, []string{intMinStr}, []string{intMinStr, intMinStr}},
			[]int64{math.MaxInt64, math.MinInt64, math.MinInt64},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i, fieldValue := range tc.fieldValues {
				elem := &DataElement{ValueField: fieldValue}
				got, err := elem.IntValue()
				if err != nil {
					t.Fatalf("elem.FieldValue: %v", err)
				}
				if got != tc.wantedValues[i] {
					t.Fatalf("got %v, want %v", got, tc.wantedValues[i])
				}
			}
		})
	}
}

func TestDataElement_IntValue_invalidCases(t *testing.T) {
	tests := []struct {
		name       string
		fieldValue interface{}
	}{
		{
			"integer strings larger than int64 max",
			[]string{strconv.FormatUint(math.MaxUint64, 10)},
		},
		{
			"integer strings smaller than int64 min",
			[]string{"-999999999999999999999999999999999999999999999999999999999999999999"},
		},
		{
			"invalid string format",
			[]string{"BINARYSEARCHTREE"},
		},
		{
			"empty []int16",
			[]int16{},
		},
		{
			"empty []uint16",
			[]uint16{},
		},
		{
			"empty []int32",
			[]int32{},
		},
		{
			"empty []uint32{}",
			[]uint32{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			elem := &DataElement{ValueField: tc.fieldValue}
			if _, err := elem.IntValue(); err == nil {
				t.Fatalf("expected error to be returned")
			}
		})
	}
}

func TestDataElement_StringValue(t *testing.T) {
	tests := []struct {
		name       string
		fieldValue interface{}
		want       string
	}{
		{
			"[]string with 1 value",
			[]string{"A"},
			"A",
		},
		{
			"[]string with more than 1 value",
			[]string{"A", "B"},
			"A",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			elem := &DataElement{ValueField: tc.fieldValue}
			got, err := elem.StringValue()
			if err != nil {
				t.Fatalf("StringValue: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDataElement_StringValue_invalidCases(t *testing.T) {
	tests := []struct {
		name       string
		fieldValue interface{}
	}{
		{
			"empty string slice",
			[]string{},
		},
		{
			"empty int slice",
			[]int16{},
		},
		{
			"empty byte slice",
			NewBulkDataBuffer([]byte{}),
		},
		{
			"nil value",
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			elem := &DataElement{ValueField: tc.fieldValue}
			if _, err := elem.StringValue(); err == nil {
				t.Fatalf("expected error to be returned")
			}
		})
	}
}

func TestDataSet_NewDataSet(t *testing.T) {
	expected := &DataSet{
		Elements: map[DataElementTag]*DataElement{
			TransferSyntaxUIDTag: {Tag: TransferSyntaxUIDTag, VR: UIVR, ValueField: []string{ExplicitVRLittleEndianUID}},
		},
		Length: UndefinedLength,
	}

	actual := NewDataSet(map[DataElementTag]interface{}{
		TransferSyntaxUIDTag: []string{ExplicitVRLittleEndianUID},
	})

	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected %s, actual: %s", expected, actual)
	}
}

func TestDataSet_Merge(t *testing.T) {
	expected := &DataSet{
		Elements: map[DataElementTag]*DataElement{
			TransferSyntaxUIDTag:    {Tag: TransferSyntaxUIDTag, VR: UIVR, ValueField: []string{ImplicitVRLittleEndianUID}},
			SpecificCharacterSetTag: {Tag: SpecificCharacterSetTag, VR: CSVR, ValueField: []string{"ISO_IR 192"}},
		},
		Length: UndefinedLength,
	}

	actual := NewDataSet(map[DataElementTag]interface{}{
		TransferSyntaxUIDTag: []string{ExplicitVRLittleEndianUID},
	}).Merge(NewDataSet(map[DataElementTag]interface{}{
		TransferSyntaxUIDTag:    []string{ImplicitVRLittleEndianUID},
		SpecificCharacterSetTag: []string{"ISO_IR 192"},
	}))

	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected %s, actual: %s", expected, actual)
	}
}

func TestDataSet_SortedTags(t *testing.T) {
	tests := []struct {
		name string
		in   *DataSet
		want []DataElementTag
	}{
		{
			"when Elements is nil",
			&DataSet{},
			[]DataElementTag{},
		},
		{
			"when Elements is empty map",
			&DataSet{Elements: map[DataElementTag]*DataElement{}},
			[]DataElementTag{},
		},
		{
			"when Elements contains multiple elements",
			&DataSet{map[DataElementTag]*DataElement{
				PrivateInformationTag:           {},
				PrivateInformationCreatorUIDTag: {},
				SourceApplicationEntityTitleTag: {},
				ImplementationVersionNameTag:    {},
				ImplementationClassUIDTag:       {},
			}, UndefinedLength},
			[]DataElementTag{
				ImplementationClassUIDTag,
				ImplementationVersionNameTag,
				SourceApplicationEntityTitleTag,
				PrivateInformationCreatorUIDTag,
				PrivateInformationTag,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.SortedTags()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDataSet_SortedElements(t *testing.T) {
	tests := []struct {
		name string
		in   *DataSet
		want []*DataElement
	}{
		{
			"when Elements is nil",
			&DataSet{},
			[]*DataElement{},
		},
		{
			"when Elements is empty map",
			&DataSet{Elements: map[DataElementTag]*DataElement{}},
			[]*DataElement{},
		},
		{
			"when Elements contains multiple elements",
			&DataSet{map[DataElementTag]*DataElement{
				PrivateInformationTag:           {Tag: PrivateInformationTag},
				PrivateInformationCreatorUIDTag: {Tag: PrivateInformationCreatorUIDTag},
				SourceApplicationEntityTitleTag: {Tag: SourceApplicationEntityTitleTag},
				ImplementationVersionNameTag:    {Tag: ImplementationVersionNameTag},
				ImplementationClassUIDTag:       {Tag: ImplementationClassUIDTag},
			}, UndefinedLength},
			[]*DataElement{
				{Tag: ImplementationClassUIDTag},
				{Tag: ImplementationVersionNameTag},
				{Tag: SourceApplicationEntityTitleTag},
				{Tag: PrivateInformationCreatorUIDTag},
				{Tag: PrivateInformationTag},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.SortedElements()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("")
			}
		})
	}
}

func TestDataSet_MetaElements(t *testing.T) {
	tests := []struct {
		name string
		in   *DataSet
		want *DataSet
	}{
		{
			"when Elements is nil",
			&DataSet{},
			&DataSet{Elements: map[DataElementTag]*DataElement{}},
		},
		{
			"when Elements is an empty map",
			&DataSet{Elements: map[DataElementTag]*DataElement{}},
			&DataSet{Elements: map[DataElementTag]*DataElement{}},
		},
		{
			"when elements is non empty",
			&DataSet{Elements: map[DataElementTag]*DataElement{
				FileMetaInformationGroupLengthTag: {Tag: FileMetaInformationGroupLengthTag},
				TransferSyntaxUIDTag:              {Tag: TransferSyntaxUIDTag},
				PixelDataTag:                      {Tag: PixelDataTag},
			}},
			&DataSet{Elements: map[DataElementTag]*DataElement{
				FileMetaInformationGroupLengthTag: {Tag: FileMetaInformationGroupLengthTag},
				TransferSyntaxUIDTag:              {Tag: TransferSyntaxUIDTag},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.MetaElements()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
