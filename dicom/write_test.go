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
	tests := []struct {
		name   string
		in     *DataElement
		syntax transferSyntax
		want   []byte
	}{
		{
			"writing element with empty []string",
			&DataElement{Tag: ImplementationVersionNameTag, ValueField: []string{}},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0, 0},
		},
		{
			"writing element with empty []int",
			&DataElement{Tag: SimpleFrameListTag, ValueField: []uint32{}},
			explicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 'U', 'L', 0, 0},
		},
		{
			"writing odd length []int in the explicit syntax",
			&DataElement{Tag: SimpleFrameListTag, ValueField: []uint32{7}},
			explicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 'U', 'L', 0x04, 0x00, 0x07, 0x00, 0x00, 0x00},
		},
		{
			"writing odd length []int in the implicit syntax",
			&DataElement{Tag: SimpleFrameListTag, ValueField: []uint32{7}},
			implicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 0x04, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00},
		},
		{
			"writing odd length []string in the explicit syntax",
			&DataElement{Tag: ImplementationVersionNameTag, ValueField: []string{"abc"}},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x04, 0x00, 'a', 'b', 'c', ' '},
		},
		{
			"writing []string with multiple values",
			&DataElement{Tag: ImplementationVersionNameTag, ValueField: []string{"abc", "de"}},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x06, 0x00, 'a', 'b', 'c', '\\', 'd', 'e'},
		},
		{
			"writing []string with multiple values that requires padding",
			&DataElement{Tag: ImplementationVersionNameTag, ValueField: []string{"AB", "DE"}},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x13, 0x00, 'S', 'H', 0x06, 0x00, 'A', 'B', '\\', 'D', 'E', ' '},
		},
		{
			"writing []int with multiple values in the implicit syntax",
			&DataElement{Tag: SimpleFrameListTag, ValueField: []uint32{0x1234ABCD, 0xABCD1234}},
			implicitVRLittleEndian,
			[]byte{0x08, 0x00, 0x61, 0x11, 0x08, 0x00, 0x00, 0x00, 0xCD, 0xAB, 0x34, 0x12, 0x34, 0x12, 0xCD, 0xAB},
		},
		{
			"writing UI element is padded with correct characters",
			&DataElement{Tag: MediaStorageSOPClassUIDTag, ValueField: []string{"1.2"}},
			explicitVRLittleEndian,
			[]byte{0x02, 0x00, 0x02, 0x00, 'U', 'I', 0x04, 0x00, '1', '.', '2', 0x00},
		},
		{
			"writing odd length []int in the big endian syntax",
			&DataElement{Tag: SimpleFrameListTag, ValueField: []uint32{0x1234ABCD, 0xABCD1234}},
			explicitVRBigEndian,
			[]byte{0x00, 0x08, 0x11, 0x61, 'U', 'L', 0x00, 0x08, 0x12, 0x34, 0xAB, 0xCD, 0xAB, 0xCD, 0x12, 0x34},
		},
		{
			"writing []uint16 in the big endian syntax",
			&DataElement{Tag: IdentifyingPrivateElementsTag, ValueField: []uint16{0x1234, 0xABCD}},
			explicitVRBigEndian,
			[]byte{0x00, 0x08, 0x03, 0x06, 'U', 'S', 0x00, 0x04, 0x12, 0x34, 0xAB, 0xCD},
		},
		{
			"writing private tag with []int16 in the explicit syntax",
			&DataElement{Tag: DataElementTag(0x00000001), ValueField: []int16{0x12}},
			explicitVRLittleEndian,
			[]byte{0x00, 0x00, 0x01, 0x00, 'U', 'N', 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x12, 0x00},
		},
		{
			"writing private tag with []uint32 in the implicit syntax",
			&DataElement{Tag: DataElementTag(0x00000001), ValueField: []int32{0x12}},
			implicitVRLittleEndian,
			[]byte{0x00, 0x00, 0x01, 0x00, 0x04, 0x00, 0x00, 0x00, 0x12, 0x00, 0x00, 0x00},
		},
		{
			"sequences of explicit length are currently written as undefined length",
			&DataElement{Tag: ReferencedCurveSequenceTag, ValueField: &Sequence{Items: []*DataSet{}}},
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
			"writing encapsulated format iterator without offset table",
			&DataElement{
				Tag:        PixelDataTag,
				VR:         OBVR,
				ValueField: encapsulatedFormatIterFromFragments(t, false, []byte{0x12, 0x23}, []byte{0x45, 0x67}),
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
				0x02, 0x00, 0x00, 0x00, // Item Length
				0x12, 0x23, // Fragment Item Bytes
				0xFE, 0xFF, 0x00, 0xE0, // Item Tag
				0x02, 0x00, 0x00, 0x00, // Item Length
				0x45, 0x67, // Fragment Item Bytes
				0xFE, 0xFF, 0xDD, 0xE0, // Sequence delimitation Tag
				0x00, 0x00, 0x00, 0x00, // Item Length
			},
		},
		{
			"writing encapsulated format iterator with offset table",
			&DataElement{
				Tag:        PixelDataTag,
				VR:         OBVR,
				ValueField: encapsulatedFormatIterFromFragments(t, true, []byte{0x12, 0x23}, []byte{0x45, 0x67}),
			},
			explicitVRLittleEndian,
			[]byte{
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

func TestWriteDataElement_unsupported(t *testing.T) {
	tests := []struct {
		name string
		in   *DataElement
	}{
		{
			"calculating lengths of one shot iterator not supported",
			&DataElement{
				Tag:        PixelDataTag,
				ValueField: oneShotIteratorFromBytes([]byte{1}),
			},
		},
		{
			"calculating lengths of native multi-frame not supported",
			&DataElement{
				Tag:        PixelDataTag,
				ValueField: mustCreateNativeMultiFrame(t),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &dcmWriter{bytes.NewBuffer([]byte{})}
			if err := writeDataElement(w, explicitVRLittleEndian, tc.in); err == nil {
				t.Fatalf("expected error to be returned")
			}
		})
	}
}

func mustCreateNativeMultiFrame(t *testing.T) BulkDataIterator {
	fragment := oneShotIteratorFromBytes([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x00, 0x00})
	nativeMultiFrame, err := newNativeMultiFrame(fragment, 2, 3)
	if err != nil {
		t.Fatalf("newNativeMultiFrame: %v", err)
	}
	return nativeMultiFrame
}
