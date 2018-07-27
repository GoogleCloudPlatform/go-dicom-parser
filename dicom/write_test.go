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
	"testing"
)

func TestWriteDateElement(t *testing.T) {
	fragments := [][]byte{{0x12, 0x23}, {0x45, 0x67}}
	offsetTable := []byte{
		0x00, 0x00, 0x00, 0x00, // Offset Table Item 1
		0x0A, 0x00, 0x00, 0x00, // Offset Table item 2
	}
	encapsulatedFormatWithoutOffsetTable := []byte{
		0xE0, 0x7F, 0x10, 0x00, // Tag
		'O', 'B', // VR
		0x00, 0x00, // Reserved Bytes
		0xFF, 0xFF, 0xFF, 0xFF, // Undefined Length
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x00, 0x00, 0x00, 0x00, // Item Length
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x02, 0x00, 0x00, 0x00, // Item Length
		0x12, 0x23, // Fragment Item Bytes
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x02, 0x00, 0x00, 0x00, // Item Length
		0x45, 0x67, // Fragment Item Bytes
		0xFE, 0xFF, 0xDD, 0xE0, // Sequence delimitation Tag
		0x00, 0x00, 0x00, 0x00, // Item Length
	}
	encapsulatedFormatWithOffsetTable := []byte{
		0xE0, 0x7F, 0x10, 0x00, // Tag
		'O', 'B', // VR
		0x00, 0x00, // Reserved Bytes
		0xFF, 0xFF, 0xFF, 0xFF, // Undefined Length
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x08, 0x00, 0x00, 0x00, // Item Length
		0x00, 0x00, 0x00, 0x00, // Offset Table Item 1
		0x0A, 0x00, 0x00, 0x00, // Offset Table Item 2
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x02, 0x00, 0x00, 0x00, // Item Length
		0x12, 0x23, // Fragment Item Bytes
		0xFE, 0xFF, 0x00, 0xE0, // Item Tag
		0x02, 0x00, 0x00, 0x00, // Item Length
		0x45, 0x67, // Fragment Item Bytes
		0xFE, 0xFF, 0xDD, 0xE0, // Sequence delimitation Tag
		0x00, 0x00, 0x00, 0x00, // Item Length
	}

	tests := []struct {
		name   string
		in     *DataElement
		syntax transferSyntax
		want   []byte
	}{
		{
			"writing element with empty []string",
			&DataElement{Tag: ImplementationVersionNameTag, VR: SHVR, ValueField: []string{}, ValueLength: 0},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0, 0},
		},
		{
			"writing element with empty []int",
			&DataElement{Tag: SimpleFrameListTag, VR: ULVR, ValueField: []uint32{}, ValueLength: 0},
			explicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 'U', 'L', 0, 0},
		},
		{
			"writing odd length []int in the explicit syntax",
			&DataElement{Tag: SimpleFrameListTag, VR: ULVR, ValueField: []uint32{7}, ValueLength: 4},
			explicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 'U', 'L', 0x04, 0x00, 0x07, 0x00, 0x00, 0x00},
		},
		{
			"writing odd length []string in the explicit syntax",
			&DataElement{Tag: ImplementationVersionNameTag, VR: SHVR, ValueField: []string{"abc"}, ValueLength: 4},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x04, 0x00, 'a', 'b', 'c', ' '},
		},
		{
			"writing []string with multiple values",
			&DataElement{Tag: ImplementationVersionNameTag, VR: SHVR, ValueField: []string{"abc", "de"}, ValueLength: 6},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x06, 0x00, 'a', 'b', 'c', '\\', 'd', 'e'},
		},
		{
			"writing []string with multiple values that requires padding",
			&DataElement{Tag: ImplementationVersionNameTag, VR: SHVR, ValueField: []string{"AB", "DE"}, ValueLength: 6},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x06, 0x00, 'A', 'B', '\\', 'D', 'E', ' '},
		},
		{
			"writing UI element is padded with correct characters",
			&DataElement{Tag: MediaStorageSOPClassUIDTag, VR: UIVR, ValueField: []string{"1.2"}, ValueLength: 4},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x02, 0x00, 'U', 'I', 0x04, 0x00, '1', '.', '2', 0x00},
		},
		{
			"writing odd length []int in the big endian syntax",
			&DataElement{Tag: SimpleFrameListTag, VR: ULVR, ValueField: []uint32{0x1234ABCD, 0xABCD1234}, ValueLength: 8},
			explicitVRBigEndian,
			[]byte{0x00, 0x08, 0x11, 0x61, 'U', 'L', 0x00, 0x08, 0x12, 0x34, 0xAB, 0xCD, 0xAB, 0xCD, 0x12, 0x34},
		},
		{
			"writing []uint16 in the big endian syntax",
			&DataElement{Tag: IdentifyingPrivateElementsTag, VR: USVR, ValueField: []uint16{0x1234, 0xABCD}, ValueLength: 4},
			explicitVRBigEndian,
			[]byte{0x00, 0x08, 0x03, 0x06, 'U', 'S', 0x00, 0x04, 0x12, 0x34, 0xAB, 0xCD},
		},
		{
			"writing AT VR in little endian",
			&DataElement{
				Tag: FrameIncrementPointerTag,
				VR:  ATVR, ValueField: []uint32{0x12345678, 0x12345678},
				ValueLength: 8,
			},
			explicitVRLittleEndian,
			[]byte{0x28, 0x00, 0x09, 0x00, 'A', 'T', 0x08, 0x00, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12, 0x78, 0x56},
		},
		{
			"writing AT VR in big endian",
			&DataElement{
				Tag:         FrameIncrementPointerTag,
				VR:          ATVR,
				ValueField:  []uint32{0x12345678, 0x12345678},
				ValueLength: 8,
			},
			explicitVRBigEndian,
			[]byte{0x00, 0x28, 0x00, 0x09, 'A', 'T', 0x00, 0x08, 0x12, 0x34, 0x56, 0x78, 0x12, 0x34, 0x56, 0x78},
		},
		{
			"sequences of undefined length are currently written as undefined length",
			&DataElement{Tag: ReferencedCurveSequenceTag, VR: SQVR, ValueField: &Sequence{Items: []*DataSet{}}, ValueLength: UndefinedLength},
			explicitVRLittleEndian,
			[]byte{
				0x08, 0x00, 0x45, 0x11, // Tag
				'S', 'Q', // VR
				0x00, 0x00, // Reserved bytes
				0xFF, 0xFF, 0xFF, 0xFF, // Undefined Length
				0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00, // Sequence delimiter
			},
		},
		{
			"sequences of explicit length are currently written as explicit length",
			&DataElement{Tag: ReferencedCurveSequenceTag, VR: SQVR, ValueField: &Sequence{Items: []*DataSet{}}, ValueLength: 0},
			explicitVRLittleEndian,
			[]byte{
				0x08, 0x00, 0x45, 0x11, // Tag
				'S', 'Q', // VR
				0x00, 0x00, // Reserved bytes
				0x00, 0x00, 0x00, 0x00, // Zero Length
			},
		},
		{
			"writing EncapsulatedFormatIterator without offset table",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  encapsulatedFormatIterFromFragments(false, fragments...),
				ValueLength: UndefinedLength,
			},
			explicitVRLittleEndian,
			encapsulatedFormatWithoutOffsetTable,
		},
		{
			"writing EncapsulatedFormatIterator with offset table",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  encapsulatedFormatIterFromFragments(true, fragments...),
				ValueLength: UndefinedLength,
			},
			explicitVRLittleEndian,
			encapsulatedFormatWithOffsetTable,
		},
		{
			"writing NewEncapsulatedFormatBuffer without offset table",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  NewEncapsulatedFormatBuffer([]byte{}, fragments...),
				ValueLength: UndefinedLength,
			},
			explicitVRLittleEndian,
			encapsulatedFormatWithoutOffsetTable,
		},
		{
			"writing NewEncapsulatedFormatBuffer with offset table",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  NewEncapsulatedFormatBuffer(offsetTable, fragments...),
				ValueLength: UndefinedLength,
			},
			explicitVRLittleEndian,
			encapsulatedFormatWithOffsetTable,
		},
		{
			"writing NewEncapsulatedFormatBuffer with odd length fragments",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  NewEncapsulatedFormatBuffer([]byte{}, []byte{0x01, 0x02, 0x03}, []byte{0x04, 0x05, 0x06}),
				ValueLength: UndefinedLength,
			},
			explicitVRLittleEndian,
			[]byte{
				0xE0, 0x7F, 0x10, 0x00, // Tag
				'O', 'B', // VR
				0x00, 0x00, // Reserved Bytes
				0xFF, 0xFF, 0xFF, 0xFF, // Undefined Length
				0xFE, 0xFF, 0x00, 0xE0, // Item Tag
				0x00, 0x00, 0x00, 0x00, // Item Length
				0xFE, 0xFF, 0x00, 0xE0, // Item Tag
				0x04, 0x00, 0x00, 0x00, // Item Length
				0x01, 0x02, 0x03, 0x00, // Fragment Item Bytes
				0xFE, 0xFF, 0x00, 0xE0, // Item Tag
				0x04, 0x00, 0x00, 0x00, // Item Length
				0x04, 0x05, 0x06, 0x00, // Fragment Item Bytes
				0xFE, 0xFF, 0xDD, 0xE0, // Sequence delimitation Tag
				0x00, 0x00, 0x00, 0x00, // Item Length
			},
		},
		{
			"writing NewBulkDataBuffer of odd length adds padding",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OWVR,
				ValueField:  NewBulkDataBuffer([]byte{0x01, 0x02, 0x03}),
				ValueLength: 4,
			},
			explicitVRLittleEndian,
			[]byte{
				0xE0, 0x7F,
				0x10, 0x00,
				'O', 'W',
				0x00, 0x00,
				0x04, 0x00, 0x00, 0x00,
				0x01, 0x02, 0x03, 0x00,
			},
		},
		{
			"writing NewBulkDataBuffer of even length does not add padding",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OWVR,
				ValueField:  NewBulkDataBuffer([]byte{0x01, 0x02}),
				ValueLength: 2,
			},
			explicitVRLittleEndian,
			[]byte{
				0xE0, 0x7F,
				0x10, 0x00,
				'O', 'W',
				0x00, 0x00,
				0x02, 0x00, 0x00, 0x00,
				0x01, 0x02,
			},
		},
		{
			"writing NewBulkDataBuffer with multiple frames",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  NewBulkDataBuffer([]byte{0x01, 0x02}, []byte{0x03, 0x04}),
				ValueLength: 4,
			},
			explicitVRLittleEndian,
			[]byte{
				0xE0, 0x7F,
				0x10, 0x00,
				'O', 'B',
				0x00, 0x00,
				0x04, 0x00, 0x00, 0x00,
				0x01, 0x02, 0x03, 0x04,
			},
		},
		{
			"writing NewBulkDataBuffer with multiple frames and odd overall length",
			&DataElement{
				Tag:         PixelDataTag,
				VR:          OBVR,
				ValueField:  NewBulkDataBuffer([]byte{0x01, 0x02, 0x03}, []byte{0x04, 0x05}),
				ValueLength: 6,
			},
			explicitVRLittleEndian,
			[]byte{
				0xE0, 0x7F,
				0x10, 0x00,
				'O', 'B',
				0x00, 0x00,
				0x06, 0x00, 0x00, 0x00,
				0x01, 0x02, 0x03, 0x04, 0x05, 0x00,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buff := bytes.NewBuffer([]byte{})
			w := &dcmWriter{buff}

			if err := writeDataElement(w, tc.syntax, tc.in); err != nil {
				t.Fatalf("writeDataElement: %v", err)
			}
			got := buff.Bytes()
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewDataElementWriter_invalidMetaHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header *DataSet
	}{
		{
			"deflated syntax is not supported",
			&DataSet{Elements: map[DataElementTag]*DataElement{
				TransferSyntaxUIDTag: &DataElement{
					Tag:        TransferSyntaxUIDTag,
					ValueField: []string{DeflatedExplicitVRLittleEndianUID},
				},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDataElementWriter(bytes.NewBuffer([]byte{}), tc.header); err == nil {
				t.Fatalf("expected an error to be returned")
			}
		})
	}
}
