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
	"fmt"
	"io"
	"os"
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
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID),
		},
		{
			"Parse Explicit VR Big Endian",
			parseInput{
				"ExplicitVRBigEndian.dcm",
				explicitVRBigEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRBigEndianUID),
		},
		{
			"Parse Explicit VR Big Endian with undefined lengths",
			parseInput{
				"ExplicitVRBigEndianUndefLen.dcm",
				explicitVRBigEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRBigEndianUID),
		},
		{
			"Parse Explicit VR Little Endian with undefined lengths",
			parseInput{
				"ExplicitVRLittleEndianUndefLen.dcm",
				explicitVRLittleEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID),
		},
		{
			"Parse Implicit VR Little Endian",
			parseInput{
				"ImplicitVRLittleEndian.dcm",
				implicitVRLittleEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, 196, ImplicitVRLittleEndianUID),
		},
		{
			"Parse DeflatedExplicitVRLittleEndian",
			parseInput{
				"DeflatedExplicitVRLittleEndian.dcm",
				deflatedExplicitVRLittleEndian,
				nil,
			},
			createExpectedDataSet(bufferedPixelData, 200, DeflatedExplicitVRLittleEndianUID),
		},
		{
			"when given an option that transforms BulkDataIterators, that transformation is respected",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{ReferenceBulkData(DefaultBulkDataDefinition)},
			},
			createExpectedDataSet(referencedPixelDataElement(462, 4), 198, ExplicitVRLittleEndianUID),
		},
		{
			"when no options transform BulkDataIterators, we still buffer BulkDataIterators",
			parseInput{
				"ExplicitVRLittleEndian.dcm",
				explicitVRLittleEndian,
				[]ParseOption{excludeTagRange(10, 10)},
			},
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compareDataSets(parse(tc.in.file, t, tc.in.opts...), tc.want, tc.in.syntax.byteOrder(), t)
		})
	}
}

func TestParse_dataSetLengths(t *testing.T) {
	// values validated using dcmtk dump tool
	tests := []struct {
		name                      string
		file                      string
		wantedSeqItemLength       uint32
		wantedNestedSeqItemLength uint32
	}{
		{
			"sequence item lengths are respected in the ExplicitVRLittleEndian format",
			"ExplicitVRLittleEndian.dcm",
			88,
			68,
		},
		{
			"sequence item lengths are respected in ImplicitVRLittleEndian format",
			"ImplicitVRLittleEndian.dcm",
			84,
			68,
		},
		{
			"sequence item lengths are respected in ExplicitVRLittleEndian format with undefined lengths",
			"ExplicitVRLittleEndianUndefLen.dcm",
			UndefinedLength,
			UndefinedLength,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := parse(tc.file, t)

			gotSeqItem := mustGetFirstSeqItem(ds.Elements[ReferencedStudySequenceTag], t)
			gotNestedSeqItem := mustGetFirstSeqItem(gotSeqItem.Elements[ReferencedImageSequenceTag], t)

			if gotSeqItem.Length != tc.wantedSeqItemLength {
				t.Fatalf("wrong seq item length. got %v, want %v", gotSeqItem.Length, tc.wantedSeqItemLength)
			}
			if gotNestedSeqItem.Length != tc.wantedNestedSeqItemLength {
				t.Fatalf("wrong nested seq item length. got %v, want %v", gotNestedSeqItem.Length, tc.wantedNestedSeqItemLength)
			}
		})
	}
}

func mustGetFirstSeqItem(element *DataElement, t *testing.T) *DataSet {
	if element == nil {
		t.Fatalf("unexpected nil *DataElement")
	}
	seq, ok := element.ValueField.(*Sequence)
	if !ok {
		t.Fatalf("unexpected type %T for element.ValueField (expected *Sequence)", element.ValueField)
	}
	if len(seq.Items) == 0 {
		t.Fatalf("got empty sequence, expeced sequence with at least 1 item")
	}
	return seq.Items[0]
}

func TestParse_filteringOption(t *testing.T) {
	tests := []struct {
		name     string
		in       parseInput
		filtered DataElementTag
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
				[]ParseOption{excludeTagRange(ReferencedImageSequenceTag, ReferencedImageSequenceTag)},
			},
			ReferencedImageSequenceTag,
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
	ds := parse("ExplicitVRLittleEndian.dcm", t, excludeTagRange(ReferencedImageSequenceTag, ReferencedImageSequenceTag))
	seqElement, ok := ds.Elements[ReferencedStudySequenceTag]
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
	if _, ok := seq.Items[0].Elements[ReferencedImageSequenceTag]; ok {
		t.Fatalf("expected nested sequence to be filtered")
	}
}

func TestParse_utf8Encoding(t *testing.T) {
	dataSet := parse("Encoding_ISO_IR_13.dcm", t, UTF8TextOption())
	element, ok := dataSet.Elements[ViewNameTag]
	if !ok {
		t.Fatalf("expected tag %v to be in the returned data set", ViewNameTag)
	}
	want := &DataElement{ViewNameTag, SHVR, []string{"ｦﾂﾐﾑ"}, 4}
	compareDataElements(element, want, binary.LittleEndian, t)
}

func TestParse_multiFrameSupport(t *testing.T) {
	frames := [][]byte{
		[]byte("4\022xV\252\231"),
		[]byte("\356\335\000\377\021\000"),
		[]byte("UDwf\231\210"),
		[]byte("\335\314\377\356\021\000"),
	}
	frameRefsUncompressed := []BulkDataReference{
		{ByteRegion{452, 6}},
		{ByteRegion{458, 6}},
		{ByteRegion{464, 6}},
		{ByteRegion{470, 6}},
	}
	frameRefsCompressed := []BulkDataReference{
		{ByteRegion{422, 0}},
		{ByteRegion{430, 6}},
		{ByteRegion{444, 6}},
		{ByteRegion{458, 6}},
		{ByteRegion{472, 6}},
	}
	referenceOpt := ReferenceBulkData(DefaultBulkDataDefinition)

	tests := []struct {
		name string
		file string
		opts []ParseOption
		want *DataElement
	}{
		{
			"when the file is in encapsulated format, fragments are untouched",
			"MultiFrameCompressed.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			&DataElement{PixelDataTag, OBVR, NewEncapsulatedFormatBuffer([]byte{}, frames...), 24},
		},
		{
			"when the file is encapsulated format and the ReferenceBulkData is used, " +
				"the fragments are untouched",
			"MultiFrameCompressed.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames(), referenceOpt},
			&DataElement{PixelDataTag, OBVR, frameRefsCompressed, 24},
		},
		{
			"when the file is native format, fragments are transformed into frames",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			&DataElement{PixelDataTag, OWVR, NewBulkDataBuffer(frames...), 24},
		},
		{
			"when the file is native format, and given ReferenceBulkData, fragments are " +
				"transformed into frame refs",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames(), referenceOpt},
			&DataElement{PixelDataTag, OWVR, frameRefsUncompressed, 24},
		},
		{
			"when given SplitUncompressedPixelDataFrames and UTF8TextOption, frames are not " +
				"affected by UTF-8",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{UTF8TextOption(), SplitUncompressedPixelDataFrames()},
			&DataElement{PixelDataTag, OWVR, NewBulkDataBuffer(frames...), 24},
		},
		{
			"When given UTF8TextOption, SplitUncompressedPixelDataFrames, ReferenceBulkData, " +
				"UTF-8 streaming does not affect the offsets within references",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{UTF8TextOption(), SplitUncompressedPixelDataFrames(), referenceOpt},
			&DataElement{PixelDataTag, OWVR, frameRefsUncompressed, 24},
		},
		{
			"When given SplitUncompressedPixelDataFrames, UTF8TextOption, ReferenceBulkData, " +
				"UTF-8 streaming does not affect the offsets within references",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames(), UTF8TextOption(), referenceOpt},
			&DataElement{PixelDataTag, OWVR, frameRefsUncompressed, 24},
		},
		{
			"DropBasicOffsetTable option can be used with this option",
			"MultiFrameCompressed.dcm",
			[]ParseOption{DropBasicOffsetTable, SplitUncompressedPixelDataFrames(), referenceOpt},
			&DataElement{PixelDataTag, OBVR, frameRefsCompressed[1:], 24},
		},
		{
			"when bits allocated is 0, pixel data is excluded",
			"MultiFrameUncompressed_BitsAllocatedOne.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			nil,
		},
		{
			"when pixel meta tags are missing, pixel data is excluded",
			"MultiFrameUncompressed_missingPixelTags.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			nil,
		},
		{
			"when pixel meta tags are equal to zero, pixel data is excluded",
			"MultiFrameUncompressed_zeroPixelTags.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			nil,
		},
		{
			"when pixel meta tags are null, pixel data is excluded",
			"MultiFrameUncompressed_nullPixelTags.dcm",
			[]ParseOption{SplitUncompressedPixelDataFrames()},
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataSet := parse(tc.file, t, tc.opts...)
			compareDataElements(dataSet.Elements[PixelDataTag], tc.want, binary.LittleEndian, t)
		})
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
			&DataElement{1, OBVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, OBVR, NewBulkDataBuffer(), 0},
		},
		{
			"when ValueField has OB VR",
			&DataElement{FileMetaInformationVersionTag, OBVR, createBulkDataIterator(sampleBytes), length},
			binary.LittleEndian,
			&DataElement{FileMetaInformationVersionTag, OBVR, NewBulkDataBuffer(sampleBytes), length},
		},
		{
			"when ValueField has OW VR, empty input produces empty slice",
			&DataElement{1, OBVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, OBVR, NewBulkDataBuffer(), 0},
		},
		{
			"when ValueField has OW VR in little endian",
			&DataElement{PixelDataTag, OBVR, createBulkDataIterator(sampleBytes), length},
			binary.LittleEndian,
			&DataElement{PixelDataTag, OBVR, NewBulkDataBuffer(sampleBytes), length},
		},
		{
			"when ValueField has OW VR in big endian",
			&DataElement{PixelDataTag, OWVR, createBulkDataIterator(sampleBytes), length},
			binary.BigEndian,
			&DataElement{PixelDataTag, OWVR, NewBulkDataBuffer(sampleBytes), length},
		},
		{
			"when ValueField has UN VR",
			&DataElement{1, UNVR, createBulkDataIterator(sampleBytes), length},
			binary.LittleEndian,
			&DataElement{1, UNVR, NewBulkDataBuffer(sampleBytes), length},
		},
		{
			"when ValueField has UN VR empty input produces empty slice",
			&DataElement{1, UNVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, UNVR, NewBulkDataBuffer(), 0},
		},
		{
			"when ValueField has OF VR, empty input produces empty slice",
			&DataElement{1, OFVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, OFVR, []float32{}, 0},
		},
		{
			"when ValueField has OF VR in big endian",
			&DataElement{1, OFVR, createBulkDataIterator([]byte{0x3F, 0xC0, 0, 0}), 4},
			binary.BigEndian,
			&DataElement{1, OFVR, []float32{1.5}, length},
		},
		{
			"when ValueField has OF VR in big endian with vm > 1",
			&DataElement{1, OFVR, createBulkDataIterator([]byte{0x3F, 0xC0, 0, 0, 0x3F, 0xC0, 0, 0}), 8},
			binary.BigEndian,
			&DataElement{1, OFVR, []float32{1.5, 1.5}, 8},
		},
		{
			"when ValueField has OF VR in little endian",
			&DataElement{1, OFVR, createBulkDataIterator([]byte{0, 0, 0xC0, 0x3F}), 4},
			binary.LittleEndian,
			&DataElement{1, OFVR, []float32{1.5}, length},
		},
		{
			"when ValueField has OD VR, empty input produces empty slice",
			&DataElement{1, ODVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, ODVR, []float64{}, 0},
		},
		{
			"when ValueField has OD VR in big endian",
			&DataElement{1, ODVR, createBulkDataIterator([]byte{0x3F, 0xF8, 0, 0, 0, 0, 0, 0}), 8},
			binary.BigEndian,
			&DataElement{1, ODVR, []float64{1.5}, 8},
		},
		{
			"when ValueField has OD VR in little endian",
			&DataElement{1, ODVR, createBulkDataIterator([]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F}), 8},
			binary.LittleEndian,
			&DataElement{1, ODVR, []float64{1.5}, 8},
		},
		{
			"when ValueField has OD VR in little endian with vm > 1",
			&DataElement{1, ODVR,
				createBulkDataIterator([]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F, 0, 0, 0, 0, 0, 0, 0xF8, 0x3F}), 16},
			binary.LittleEndian,
			&DataElement{1, ODVR, []float64{1.5, 1.5}, 16},
		},
		{
			"when Value Field has UC VR, empty input produces empty slice",
			&DataElement{1, UCVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, UCVR, []string{}, 0},
		},
		{
			"when ValueField has UC VR with vm > 1 with trailing spaces",
			&DataElement{1, UCVR, createBulkDataIterator([]byte("abcd \\gef ")), 10},
			binary.LittleEndian,
			&DataElement{1, UCVR, []string{"abcd ", "gef "}, 10},
		},
		{
			"when ValueField has UR VR, empty input produces empty slice",
			&DataElement{1, URVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, URVR, []string{}, 0},
		},
		{
			"when ValueField has UR VR, trailing spaces are removed",
			&DataElement{1, URVR, createBulkDataIterator([]byte("abcdgef \r\n")), 12},
			binary.LittleEndian,
			&DataElement{1, URVR, []string{"abcdgef"}, 12},
		},
		{
			"when ValueField has UT VR, empty input produce empty slice",
			&DataElement{1, UTVR, emptyBulkDataIterator{}, 0},
			binary.LittleEndian,
			&DataElement{1, UTVR, []string{}, 0},
		},
		{
			"when ValueField has UT VR, trailing spaces are ignored and backslashes are allowed",
			&DataElement{1, UTVR, createBulkDataIterator([]byte("abcd\\\\ \r\r\n")), 12},
			binary.LittleEndian,
			&DataElement{1, UTVR, []string{"abcd\\\\"}, 12},
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

func ExampleParse() {
	r, err := os.Open("example.dcm")
	if err != nil {
		fmt.Println(err)
		return
	}
	dataSet, err := Parse(r)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, element := range dataSet.Elements {
		if sequence, ok := element.ValueField.(*Sequence); ok {
			for _, item := range sequence.Items {
				for _, element := range item.Elements {
					fmt.Println("sequence item element", element)
				}
			}
		}
		fmt.Println(element)
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

func (emptyBulkDataIterator) ToBuffer() (BulkDataBuffer, error) {
	return NewBulkDataBuffer(), nil
}

func (emptyBulkDataIterator) Length() int64 {
	return 0
}

func (emptyBulkDataIterator) write(w io.Writer, syntax transferSyntax) error {
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
	return &DataElement{PixelDataTag, OWVR, refs, uint32(length)}
}

func createExpectedDataSet(pixelElement *DataElement, metaLength uint32, transferSyntaxUID string) *DataSet {
	expectedDataSet := &DataSet{map[DataElementTag]*DataElement{}, UndefinedLength}

	for _, elem := range expectedElements {
		expectedDataSet.Elements[elem.Tag] = elem
	}

	expectedDataSet.Elements[FileMetaInformationGroupLengthTag] = &DataElement{
		FileMetaInformationGroupLengthTag,
		ULVR,
		[]uint32{metaLength},
		4,
	}
	expectedDataSet.Elements[TransferSyntaxUIDTag] = &DataElement{
		TransferSyntaxUIDTag,
		UIVR,
		[]string{transferSyntaxUID},
		uint32(len(transferSyntaxUID)),
	}
	expectedDataSet.Elements[PixelDataTag] = pixelElement

	return expectedDataSet
}

func excludeTagRange(start, end DataElementTag) ParseOption {
	return ParseOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if start <= element.Tag && element.Tag <= end {
			// in range. exclude it by returning nil
			return nil, nil
		}
		return element, nil
	})
}
