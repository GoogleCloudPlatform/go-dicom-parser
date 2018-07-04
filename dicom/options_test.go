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
	"testing"
)

type arithmeticSeq struct {
	start uint32
	end   uint32
	inc   uint32
}

func TestIsBulkData(t *testing.T) {
	tests := []struct {
		name string
		in   arithmeticSeq
		want bool
	}{
		{
			"Curve Data (50xx,3000) is bulk data",
			arithmeticSeq{0x50003000, CurveDataTag, 0x00010000},
			true,
		},
		{
			"Pixel data is bulk data (7FE0,0010) is bulk data",
			arithmeticSeq{PixelDataTag, PixelDataTag, 1},
			true,
		},
		{
			"Source Image IDs (0x0020,31xx) is not bulk data",
			arithmeticSeq{0x00203100, 0x002031FF, 1},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for tag := tc.in.start; tag <= tc.in.end; tag += tc.in.inc {
				got := DefaultBulkDataDefinition(elementWithTag(tag))
				if got != tc.want {
					t.Fatalf("IsPixelData(0x%08X) => %v, want %v", tag, got, true)
				}
			}
		})
	}
}

func TestReferenceBulkData(t *testing.T) {
	length := uint32(len(sampleBytes))
	refs := []BulkDataReference{{ByteRegion{0, int64(length)}}}
	tests := []struct {
		name  string
		in    *DataElement
		order binary.ByteOrder
		want  *DataElement
	}{
		{
			"when not bulk data, ValueField is of a buffered type",
			createDataElement(FileMetaInformationVersionTag, OBVR, createBulkDataIterator(sampleBytes), length),
			binary.LittleEndian,
			createDataElement(FileMetaInformationVersionTag, OBVR, [][]byte{sampleBytes}, length),
		},
		{
			"when bulk data, ValueField is of type []ByteFragmentReference",
			createDataElement(PixelDataTag, OBVR, createBulkDataIterator(sampleBytes), length),
			binary.LittleEndian,
			createDataElement(PixelDataTag, OBVR, refs, length),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReferenceBulkData(DefaultBulkDataDefinition).transform(tc.in)
			if err != nil {
				t.Fatalf("ReferenceBulkData.Apply(_) => (_, %v)", err)
			}
			compareDataElements(got, tc.want, tc.order, t)
		})
	}
}

func TestDropGroupLengths(t *testing.T) {
	tests := []struct {
		name string
		in *DataElement
		want *DataElement
	} {
		{
			"a group length element is filtered",
			createDataElement(0x00020000, OBVR, []byte{}, 0),
			nil,
		},
		{
			"non-group length elements are not filtered",
			createDataElement(0x00020001, ULVR, []uint32{}, 0),
			createDataElement(0x00020001, ULVR, []uint32{}, 0),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DropGroupLengths.transform(tc.in)
			if err != nil {
				t.Fatalf("DropGroupLengths.transform(%v) => %v", tc.in, err)
			}
			compareDataElements(got, tc.want, binary.LittleEndian, t)
		})
	}
}

func TestDropBasicOffsetTable(t *testing.T) {
	tests := []struct {
		name string
		in *DataElement
		want *DataElement
	} {
		{
			"the offset table is dropped for the encapsulated format",
			createDataElement(PixelDataTag, OBVR, encapsulatedFormatIterFromFragments(t, true, sampleBytes), UndefinedLength),
			createDataElement(PixelDataTag, OBVR, oneShotIteratorFromBytes(sampleBytes), UndefinedLength),
		},
		{
			"pixel data of non-encapsulated formats are not modified",
			createDataElement(PixelDataTag, OBVR, oneShotIteratorFromBytes(sampleBytes), UndefinedLength),
			createDataElement(PixelDataTag, OBVR, oneShotIteratorFromBytes(sampleBytes), UndefinedLength),
		},
	}
	for _, tc := range tests {
		got, err := DropBasicOffsetTable.transform(tc.in)
		if err != nil {
			t.Fatalf("DropBasicOffsetTable.Transform(_) => %v", err)
		}
		compareDataElements(got, tc.want, binary.LittleEndian, t)
	}
}

func elementWithTag(tag uint32) *DataElement {
	return &DataElement{Tag: DataElementTag(tag)}
}

func createBulkDataIterator(b []byte) BulkDataIterator {
	return newOneShotIterator(countReaderFromBytes(b))
}
