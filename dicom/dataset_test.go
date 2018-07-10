package dicom

import (
	"math"
	"strconv"
	"testing"
)

type arithmeticSeq struct {
	start uint32
	end   uint32
	inc   uint32
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
