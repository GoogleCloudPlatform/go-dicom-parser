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
	"io"
	"testing"
)

type parseInput struct {
	file   string
	syntax transferSyntax
	opts   []ParseOption
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   parseInput
		want *DataSet
	}{
		{
			"Parse Explicit VR Little Endian",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, ExplicitVRLittleEndianUID),
		},
		{
			"Parse Explicit VR Little Endian with undefined lengths",
			parseInput{
				"ExplicitVRLittleEndianUndefLen.dcm",
				explicitVRLittleEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, ExplicitVRLittleEndianUID),
		},
		{
			"when given an option that transforms BulkDataIterators, that transformation is respected",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{ReferenceBulkData(DefaultBulkDataDefinition)},
			},
			createExpectedDataSet(referencedPixelDataElement(428, 4), ExplicitVRLittleEndianUID),
		},
		{
			"when no options transform BulkDataIterators, we still buffer BulkDataIterators",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{excludeTagRange(10, 10)},
			},
			createExpectedDataSet(bufferedPixelData, ExplicitVRLittleEndianUID),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compareDataSets(parse(tc.in.file, t, tc.in.opts...), tc.want, tc.in.syntax.ByteOrder, t)
		})
	}
}

func TestParse_filteringOption(t *testing.T) {
	tests := []struct {
		name     string
		in       parseInput
		filtered uint32
	}{
		{
			"simple exclude filter test",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{excludeTagRange(PixelDataTag, PixelDataTag)},
			},
			PixelDataTag,
		},
		{
			"test sequence items can be filtered",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{excludeTagRange(0x00081150, 0x00081150)},
			},
			0x00081150,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := parse(tc.in.file, t, tc.in.opts...)
			if _, ok := ds.Elements[tc.filtered]; ok {
				t.Fatalf("filter did not work. Did not expect %v to be in the dataset", tc.filtered)
			}
		})
	}
}

func TestParse_filteringNestedSeq(t *testing.T) {
	seqTag := uint32(0x00081110)
	nestedSeqTag := uint32(0x00081150)
	ds := parse("ExplicitVRLittleEndian.dcm", t, excludeTagRange(nestedSeqTag, nestedSeqTag))
	seqElement, ok := ds.Elements[seqTag]
	if !ok {
		t.Fatalf("could not find top level sequence")
	}
	seq, ok := seqElement.ValueField.(*Sequence)
	if !ok {
		t.Fatalf("expected sequence type for top level sequence. Got %T want *Sequence", seqElement.ValueField)
	}
	if len(seq.Items) != 1 {
		t.Fatalf("wrong length for sequence. Got %v, want 1", len(seq.Items))
	}
	if _, ok := seq.Items[0].Elements[nestedSeqTag]; ok {
		t.Fatalf("expected nested sequence to be filtered")
	}
}

func TestBufferBulkData(t *testing.T) {
	length := uint32(len(sampleBytes))
	tests := []struct {
		name  string
		in    *DataElement
		order binary.ByteOrder
		want  *DataElement
	}{
		{
			"when ValueField has OB VR, empty input produces empty slice",
			createDataElement(1, OBVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, OBVR, [][]byte{}, 0),
		},
		{
			"when ValueField has OB VR",
			createDataElement(FileMetaInformationVersionTag, OBVR, createBulkDataIterator(sampleBytes), length),
			binary.LittleEndian,
			createDataElement(FileMetaInformationVersionTag, OBVR, [][]byte{sampleBytes}, length),
		},
		{
			"when ValueField has OW VR, empty input produces empty slice",
			createDataElement(1, OBVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, OBVR, [][]byte{}, 0),
		},
		{
			"when ValueField has OW VR in little endian",
			createDataElement(PixelDataTag, OBVR, createBulkDataIterator(sampleBytes), length),
			binary.LittleEndian,
			createDataElement(PixelDataTag, OBVR, [][]byte{sampleBytes}, length),
		},
		{
			"when ValueField has OW VR in big endian",
			createDataElement(PixelDataTag, OWVR, createBulkDataIterator(sampleBytes), length),
			binary.BigEndian,
			createDataElement(PixelDataTag, OWVR, [][]byte{sampleBytes}, length),
		},
		{
			"when ValueField has UN VR",
			createDataElement(1, UNVR, createBulkDataIterator(sampleBytes), length),
			binary.LittleEndian,
			createDataElement(1, UNVR, [][]byte{sampleBytes}, length),
		},
		{
			"when ValueField has UN VR empty input produces empty slice",
			createDataElement(1, UNVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, UNVR, [][]byte{}, 0),
		},
		{
			"when ValueField has OF VR, empty input produces empty slice",
			createDataElement(1, OFVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, OFVR, []float32{}, 0),
		},
		{
			"when ValueField has OF VR in big endian",
			createDataElement(1, OFVR, createBulkDataIterator([]byte{0x3F, 0xC0, 0, 0}), 4),
			binary.BigEndian,
			createDataElement(1, OFVR, []float32{1.5}, length),
		},
		{
			"when ValueField has OF VR in big endian with vm > 1",
			createDataElement(1, OFVR, createBulkDataIterator([]byte{0x3F, 0xC0, 0, 0, 0x3F, 0xC0, 0, 0}), 8),
			binary.BigEndian,
			createDataElement(1, OFVR, []float32{1.5, 1.5}, 8),
		},
		{
			"when ValueField has OF VR in little endian",
			createDataElement(1, OFVR, createBulkDataIterator([]byte{0, 0, 0xC0, 0x3F}), 4),
			binary.LittleEndian,
			createDataElement(1, OFVR, []float32{1.5}, length),
		},
		{
			"when ValueField has OD VR, empty input produces empty slice",
			createDataElement(1, ODVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, ODVR, []float64{}, 0),
		},
		{
			"when ValueField has OD VR in big endian",
			createDataElement(1, ODVR, createBulkDataIterator([]byte{0x3F, 0xF8, 0, 0, 0, 0, 0, 0}), 8),
			binary.BigEndian,
			createDataElement(1, ODVR, []float64{1.5}, 8),
		},
		{
			"when ValueField has OD VR in little endian",
			createDataElement(1, ODVR, createBulkDataIterator([]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F}), 8),
			binary.LittleEndian,
			createDataElement(1, ODVR, []float64{1.5}, 8),
		},
		{
			"when ValueField has OD VR in little endian with vm > 1",
			createDataElement(1, ODVR,
				createBulkDataIterator([]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F, 0, 0, 0, 0, 0, 0, 0xF8, 0x3F}), 16),
			binary.LittleEndian,
			createDataElement(1, ODVR, []float64{1.5, 1.5}, 16),
		},
		{
			"when Value Field has UC VR, empty input produces empty slice",
			createDataElement(1, UCVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, UCVR, []string{}, 0),
		},
		{
			"when ValueField has UC VR with vm > 1 with trailing spaces",
			createDataElement(1, UCVR, createBulkDataIterator([]byte("abcd \\gef ")), 10),
			binary.LittleEndian,
			createDataElement(1, UCVR, []string{"abcd ", "gef "}, 10),
		},
		{
			"when ValueField has UR VR, empty input produces empty slice",
			createDataElement(1, URVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, URVR, []string{}, 0),
		},
		{
			"when ValueField has UR VR, trailing spaces are removed",
			createDataElement(1, URVR, createBulkDataIterator([]byte("abcdgef ")), 10),
			binary.LittleEndian,
			createDataElement(1, URVR, []string{"abcdgef"}, 10),
		},
		{
			"when ValueField has UT VR, empty input produce empty slice",
			createDataElement(1, UTVR, emptyBulkDataIterator{}, 0),
			binary.LittleEndian,
			createDataElement(1, UTVR, []string{}, 0),
		},
		{
			"when ValueField has UT VR, trailing spaces are ignored and backslashes are allowed",
			createDataElement(1, UTVR, createBulkDataIterator([]byte("abcd\\\\ ")), 10),
			binary.LittleEndian,
			createDataElement(1, UTVR, []string{"abcd\\\\"}, 10),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bufferBulkData(tc.in, tc.order)
			if err != nil {
				t.Fatalf("ReferenceBulkData.Apply(_) => (_, %v)", err)
			}
			compareDataElements(got, tc.want, tc.order, t)
		})
	}
}

type emptyBulkDataIterator struct{}

func (emptyBulkDataIterator) Next() (*BulkDataReader, error) {
	return nil, io.EOF
}

func (emptyBulkDataIterator) ByteOrder() binary.ByteOrder {
	return binary.LittleEndian
}

func (emptyBulkDataIterator) Close() error {
	return nil
}

func parse(file string, t *testing.T, opts ...ParseOption) *DataSet {
	f, err := openFile(file)
	if err != nil {
		t.Fatalf("opening file: %v", err)
	}
	res, err := Parse(f, opts...)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return res
}

func referencedPixelDataElement(offset, length int) *DataElement {
	refs := []BulkDataReference{{ByteRegion{int64(offset), int64(length)}}}
	return createDataElement(PixelDataTag, OWVR, refs, uint32(length))
}

func createExpectedDataSet(pixelElement *DataElement, transferSyntaxUID string) *DataSet {
	expectedDataSet := &DataSet{map[uint32]*DataElement{}}

	for _, elem := range expectedElements {
		expectedDataSet.Elements[uint32(elem.Tag)] = elem
	}

	expectedDataSet.Elements[PixelDataTag] = pixelElement
	transferSyntaxElement := &DataElement{
		TransferSyntaxUIDTag, UIVR,
		[]string{transferSyntaxUID}, uint32(len(transferSyntaxUID))}
	expectedDataSet.Elements[TransferSyntaxUIDTag] = transferSyntaxElement

	return expectedDataSet
}

func excludeTagRange(start, end uint32) ParseOption {
	return WithTransform(func(element *DataElement) (*DataElement, error) {
		if start <= uint32(element.Tag) && uint32(element.Tag) <= end {
			// in range. exclude it by returning nil
			return nil, nil
		}
		return element, nil
	})
}
