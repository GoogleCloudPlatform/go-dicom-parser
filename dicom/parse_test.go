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
	"io"
	"io/ioutil"
	"math"
	"reflect"
	"testing"
)

func TestParseDataElement(t *testing.T) {
	// see http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.2 for byte
	// structure
	testCases := []struct {
		name     string
		bytes    []byte
		syntax   transferSyntax
		expected *DataElement
		err      error
	}{
		{
			"unsigned long ExplicitVRLittleEndian",
			[]byte{0x02, 0x00, 0x00, 0x00, 'U', 'L', 0x04, 0x00, 0xCA, 0x00, 0x00, 0x00},
			explicitVRLittleEndian,
			&DataElement{
				0x00020000,
				ULVR, []uint32{202}, 4},
			nil,
		},
		{
			"Item Delimination Item",
			[]byte{0xFE, 0xFF, 0x0D, 0xE0, 0x00, 0x00, 0x00, 0x00},
			explicitVRLittleEndian,
			nil,
			io.EOF,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			element, err := parseDataElement(dcmReaderFromBytes(tc.bytes), metaDataWithSyntax(tc.syntax))
			if err != tc.err {
				t.Fatalf("parseDataElement(_, _) => (%v, %v), want (%v, %v)",
					element, err, tc.expected, tc.err)
			}

			if tc.expected != nil && !reflect.DeepEqual(*element, *tc.expected) {
				t.Fatalf("parseDataElement(_, _) => (%v, %v) want (%v, %v)",
					*element, err, *tc.expected, tc.err)
			}
		})
	}
}

func TestGetValueLength(t *testing.T) {
	// testing format outlined in Table 7.1-1 and 7.1-2 is respected
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_6.2
	testCases := []struct {
		name     string
		bytes    []byte
		vr       *VR
		syntax   transferSyntax
		expected uint32
	}{
		{
			"Sequence explicitVRLittleEndian",
			[]byte{0x00, 0x00, 0x11, 0x22, 0x33, 0x44},
			SQVR,
			explicitVRLittleEndian,
			0x44332211,
		},
		{
			"Sequence explicitVRBigEndian",
			[]byte{0x00, 0x00, 0x11, 0x22, 0x33, 0x44},
			SQVR,
			explicitVRBigEndian,
			0x11223344,
		},
		{
			"unsigned short explicitVRLittleEndian",
			[]byte{0x11, 0x22},
			USVR,
			explicitVRLittleEndian,
			0x2211,
		},
		{
			"unsigned short explicitVRBigEndian",
			[]byte{0x11, 0x22},
			USVR,
			explicitVRBigEndian,
			0x1122,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			length, err := parseValueLength(
				dcmReaderFromBytes(tc.bytes), tc.vr, metaDataWithSyntax(tc.syntax))

			if err != nil {
				t.Fatalf("parseValueLength(_, _, _) => %v", err)
			}
			if length != tc.expected {
				t.Fatalf("got %v, want %v", length, 0x78563412)
			}
		})
	}
}

func TestParseTag(t *testing.T) {
	testCases := []struct {
		name   string
		in     []byte
		want   []uint32
		syntax transferSyntax
	}{
		{
			"read tag in big endian",
			[]byte{0x00, 0x02, 0x00, 0x10},
			[]uint32{0x00020010},
			explicitVRBigEndian,
		},
		{
			"read tag in little endian",
			[]byte{0x02, 0x00, 0x10, 0x00},
			[]uint32{0x00020010},
			explicitVRLittleEndian,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTag(dcmReaderFromBytes(tc.in), metaDataWithSyntax(tc.syntax), uint32(len(tc.in)))
			if err != nil {
				t.Fatalf("parseTag(_, _, _) => %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseText(t *testing.T) {
	testCases := []struct {
		name     string
		bytes    []byte
		length   uint32
		padding  byte
		expected []string
	}{
		{
			"trailing space, vm = 1",
			[]byte("ABC "),
			4,
			' ',
			[]string{"ABC"},
		},
		{
			"no trailing space, vm = 1",
			[]byte("ABCD"),
			4,
			' ',
			[]string{"ABCD"},
		},
		{
			"trailing space vm > 1",
			[]byte("ABC\\DEF "),
			8,
			' ',
			[]string{"ABC", "DEF"},
		},
		{
			"trailing null",
			[]byte("1.2.840.10008.1.2\x00"),
			18,
			0,
			[]string{"1.2.840.10008.1.2"},
		},
		{
			"multiple trailing spaces are not significant",
			[]byte("DERIVED \\SECONDARY\\OTHER  "),
			26,
			' ',
			[]string{"DERIVED", "SECONDARY", "OTHER"},
		},
		{
			"test length 0",
			[]byte{},
			0,
			' ',
			nil,
		},
	}

	for _, tc := range testCases {
		result, err := parseText(dcmReaderFromBytes(tc.bytes), tc.length, tc.padding)
		if err != nil {
			t.Fatalf("parseText(_, _, _) => %v", err)
		}
		if !reflect.DeepEqual(result, tc.expected) {
			t.Fatalf("got %v, want %v", result, tc.expected)
		}
	}
}

func TestParseNumberBinary_integers(t *testing.T) {
	testCases := []struct {
		name     string
		bytes    []byte
		length   uint32
		vr       *VR
		endian   binary.ByteOrder
		expected interface{}
	}{
		{
			"unsigned short, little endian, vm > 1",
			[]byte{0xAB, 0xCD, 0x12, 0x34},
			4,
			USVR,
			binary.LittleEndian,
			[]uint16{0xCDAB, 0x3412},
		},
		{
			"unsigned short, big endian, vm > 1",
			[]byte{0xAB, 0xCD, 0x12, 0x34},
			4,
			USVR,
			binary.BigEndian,
			[]uint16{0xABCD, 0x1234},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseNumberBinary(dcmReaderFromBytes(tc.bytes),
				tc.length, tc.vr, metaDataWithSyntax(transferSyntax{false, tc.endian}))
			if err != nil {
				t.Fatalf("parseNumberBinary(_, _, _, _) => %v", err)
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Fatalf("got %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestParseNumberBinary_float(t *testing.T) {
	testCases := []struct {
		name     string
		bytes    []byte
		length   uint32
		vr       *VR
		endian   binary.ByteOrder
		expected []float32
	}{
		{
			"32-bit float, big endian",
			[]byte{0x3F, 0xC0, 0x00, 0x00},
			4,
			FLVR,
			binary.BigEndian,
			[]float32{1.5},
		},
		{
			"32-bit float, little endian",
			[]byte{0x00, 0x00, 0xC0, 0x3F},
			4,
			FLVR,
			binary.LittleEndian,
			[]float32{1.5},
		}, {
			"32-bit float, little endian, vm > 1",
			[]byte{0x00, 0x00, 0xC0, 0x3F, 0x00, 0x00, 0xC0, 0x3F},
			8,
			FLVR,
			binary.LittleEndian,
			[]float32{1.5, 1.5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseNumberBinary(dcmReaderFromBytes(tc.bytes),
				tc.length, tc.vr, metaDataWithSyntax(transferSyntax{false, tc.endian}))
			if err != nil {
				t.Fatalf("parseNumberBinary(_, _, _, _) => %v", err)
			}

			resultSlice, ok := result.([]float32)
			if !ok {
				t.Fatalf("result has wrong type %T expected %T", result, tc.expected)
			}

			if len(resultSlice) != len(tc.expected) {
				t.Fatalf("got %v, want %v", tc.expected, resultSlice)
			}

			if math.Abs(float64(tc.expected[0]-resultSlice[0])) > 0.000000000001 {
				t.Fatalf("got %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestParseByteSequence(t *testing.T) {
	expected := []byte{0x01, 0x02, 0x03, 0x00}
	result, err := parseBulkData(
		dcmReaderFromBytes(expected), 0, 4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader, err := result.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bytes.Compare(data, expected) != 0 {
		t.Fatalf("got %v, want %v", data, expected)
	}
}

func TestParseVR_invalid(t *testing.T) {
	_, err := parseVR(dcmReaderFromBytes([]byte("ZZ")))
	if err == nil {
		t.Fatalf("expected error to be returned")
	}
}

func TestParseVR(t *testing.T) {
	vr, err := parseVR(dcmReaderFromBytes([]byte("US")))
	if err != nil {
		t.Fatalf("parseVR(_) => %v", err)
	}

	if vr != USVR {
		t.Fatalf("got %v, want %v", vr, USVR)
	}
}

func metaDataWithSyntax(syntax transferSyntax) dicomMetaData {
	return dicomMetaData{syntax, isoIR6}
}
