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
	"io"
	"os"
	"path"
	"reflect"
	"testing"

	"encoding/binary"

	
)

var (
	nestedDataSetElement = createDataElement(0x00081155, UIVR,
		[]string{"1.2.840.10008.5.1.4.1.1.4"}, 26)
	nestedSeq         = createSingletonSequence(nestedDataSetElement)
	seq               = createSingletonSequence(createDataElement(0x00081150, SQVR, &nestedSeq, 42))
	bufferedPixelData = createDataElement(PixelDataTag, OWVR, [][]byte{{0x11, 0x11, 0x22, 0x22}}, 4)
	expectedElements  = []*DataElement{
		createDataElement(0x00020000, ULVR, []uint32{198}, 4),
		createDataElement(0x00020001, OBVR, [][]byte{{0, 1}}, 2),
		createDataElement(0x00020002, UIVR, []string{"1.2.840.10008.5.1.4.1.1.4"}, 26),
		createDataElement(0x00020003, UIVR,
			[]string{"1.2.840.113619.2.176.3596.3364818.7271.1259708501.876"}, 54),
		createDataElement(0x00020010, UIVR, []string{"1.2.840.10008.1.2.1"}, 20),
		createDataElement(0x00020012, UIVR, []string{"1.2.276.0.7230010.3.0.3.5.4"}, 28),
		createDataElement(0x00020013, SHVR, []string{"OFFIS_DCMTK_354"}, 16),
		createDataElement(0x00081110, SQVR, &seq, 62),
		bufferedPixelData,
	}
)

func TestIterator_NextElement(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		syntax transferSyntax
		want   *DataSet
	}{
		{
			"Explicit Lengths, Explicit VR, Little Endian",
			"ExplicitVRLittleEndian.dcm",
			explicitVRLittleEndian,
			createExpectedDataSet(bufferedPixelData, ExplicitVRLittleEndianUID),
		},
		{
			"Undefined Sequence & Item lengths, Explicit VR, Little Endian",
			"ExplicitVRLittleEndianUndefLen.dcm",
			explicitVRLittleEndian,
			createExpectedDataSet(bufferedPixelData, ExplicitVRLittleEndianUID),
		},
		{
			"Explicit Length, Explicit VR, Big Endian",
			"ExplicitVRBigEndian.dcm",
			explicitVRBigEndian,
			createExpectedDataSet(bufferedPixelData, ExplicitVRBigEndianUID),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iter, err := createIteratorFromFile(tc.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for elem, err := iter.NextElement(); err != io.EOF; elem, err = iter.NextElement() {
				if err != nil {
					t.Fatalf("NextElement() => %v", err)
				}
				compareDataElements(elem, tc.want.Elements[uint32(elem.Tag)], tc.syntax.ByteOrder, t)
			}
		})
	}
}

func TestIterator_Close(t *testing.T) {
	iter, err := createIteratorFromFile("ExplicitVRLittleEndian.dcm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := iter.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := iter.NextElement(); err != io.EOF {
		t.Fatalf("got %v, want %v", err, io.EOF)
	}
}

func TestIterator_atEndOfInput(t *testing.T) {
	iter, err := newDataElementIterator(dcmReaderFromBytes(nil), defaultEncoding)
	if err != nil {
		t.Fatalf("unexepcted error: %v", err)
	}
	_, err = iter.NextElement()
	if err != io.EOF {
		t.Fatalf("expected iterator to return EOF if at end of input: got %v, want %v", err, io.EOF)
	}
}

func TestEmptyIterator(t *testing.T) {
	iter := &dataElementIterator{empty: true, metaHeader: emptyElementIterator{}}
	_, err := iter.NextElement()
	if err != io.EOF {
		t.Fatalf("expected empty iterator to return io.EOF: got %v, want %v", err, io.EOF)
	}
}

func compareDataElements(e1 *DataElement, e2 *DataElement, order binary.ByteOrder, t *testing.T) {
	if e1 == nil || e2 == nil {
		if e1 != e2 {
			t.Fatalf("expected both elements to be nil: got %v, want %v", e1, e2)
		}
		return
	}
	if e1.VR != e2.VR {
		t.Fatalf("expected VRs to be equal: got %v, want %v", e1.VR, e2.VR)
	}
	if e1.Tag != e2.Tag {
		t.Fatalf("expected tags to be equal: got %v, want %v", e1.Tag, e2.Tag)
	}

	var err error
	e1, err = processElement(e1, order)
	if err != nil {
		t.Fatalf("unexpected error unstreaming data element: %v", err)
	}
	e2, err = processElement(e2, order)
	if err != nil {
		t.Fatalf("unexpected error unstreaming data element: %v", err)
	}

	if e1.VR != SQVR {
		if !reflect.DeepEqual(e1.ValueField, e2.ValueField) {
			t.Fatalf("expected ValueFields to be equal: got %v, want %v",
				e1.ValueField, e2.ValueField)
		}
	} else {
		compareSequences(e1.ValueField.(*Sequence), e2.ValueField.(*Sequence), order, t)
	}
}

func compareSequences(s1 *Sequence, s2 *Sequence, order binary.ByteOrder, t *testing.T) {
	if len(s1.Items) != len(s2.Items) {
		t.Fatalf("expected sequences to have same length: got %v, want %v",
			len(s1.Items), len(s2.Items))
	}

	for i := range s1.Items {
		compareDataSets(s1.Items[i], s2.Items[i], order, t)
	}
}

func compareDataSets(d1 *DataSet, d2 *DataSet, order binary.ByteOrder, t *testing.T) {
	k1, k2 := getKeys(d1.Elements), getKeys(d2.Elements)

	if !reflect.DeepEqual(k1, k2) {
		t.Fatalf("expected datasets to have same keys: got %v, want %v", k1, k2)
	}

	for k := range k1 {
		compareDataElements(d1.Elements[k], d2.Elements[k], order, t)
	}
}

func getKeys(m map[uint32]*DataElement) map[uint32]bool {
	ret := make(map[uint32]bool)
	for k := range m {
		ret[k] = true
	}
	return ret
}

func createIteratorFromFile(file string) (DataElementIterator, error) {
	r, err := openFile(file)
	if err != nil {
		return nil, err
	}

	return NewDataElementIterator(r)
}

func openFile(name string) (io.Reader, error) {
	var p = path.Join("../",
		"testdata/+name)

	return os.Open(p)
}

var sampleBytes = []byte{1, 2, 3, 4}

func countReaderFromBytes(data []byte) *countReader {
	return &countReader{
		bytes.NewBuffer(data),
		0,
	}
}

func dcmReaderFromBytes(data []byte) *dcmReader {
	return newDcmReader(bytes.NewBuffer(data))
}

func createDataElement(tag uint32, vr *VR, value interface{}, length uint32) *DataElement {
	return &DataElement{DataElementTag(tag), vr, value, length}
}

func createSingletonSequence(elements ...*DataElement) Sequence {
	ds := DataSet{map[uint32]*DataElement{}}
	for _, elem := range elements {
		ds.Elements[uint32(elem.Tag)] = elem
	}
	return Sequence{[]*DataSet{&ds}}
}
