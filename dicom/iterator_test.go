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
	"io/ioutil"
	"os"
	"testing"
	"testing/iotest"
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
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID),
		},
		{
			"Undefined Sequence & Item lengths, Explicit VR, Little Endian",
			"ExplicitVRLittleEndianUndefLen.dcm",
			explicitVRLittleEndian,
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID),
		},
		{
			"Explicit Length, Explicit VR, Big Endian",
			"ExplicitVRBigEndian.dcm",
			explicitVRBigEndian,
			createExpectedDataSet(bufferedPixelData, 198, ExplicitVRBigEndianUID),
		},
		{
			"Explicit Lengths, Implicit VR Little Endian",
			"ImplicitVRLittleEndian.dcm",
			implicitVRLittleEndian,
			createExpectedDataSet(bufferedPixelData, 196, ImplicitVRLittleEndianUID),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iter, err := createIteratorFromFile(tc.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer iter.Close()

			for elem, err := iter.Next(); err != io.EOF; elem, err = iter.Next() {
				if err != nil {
					t.Fatalf("Next() => %v", err)
				}
				compareDataElements(elem, tc.want.Elements[elem.Tag], tc.syntax.byteOrder(), t)
			}
		})
	}
}

func TestIterator_oneByteReader(t *testing.T) {
	r, err := openFile("ExplicitVRLittleEndian.dcm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	iter, err := NewDataElementIterator(iotest.OneByteReader(r))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := createExpectedDataSet(bufferedPixelData, 198, ExplicitVRLittleEndianUID)
	for elem, err := iter.Next(); err != io.EOF; elem, err = iter.Next() {
		if err != nil {
			t.Fatalf("Next() => %v", err)
		}
		compareDataElements(elem, want.Elements[elem.Tag], binary.LittleEndian, t)
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

	if _, err := iter.Next(); err != io.EOF {
		t.Fatalf("got %v, want %v", err, io.EOF)
	}
}

func TestIterator_atEndOfInput(t *testing.T) {
	iter := newDataElementIterator(dcmReaderFromBytes(nil), explicitVRLittleEndian, UndefinedLength)
	if _, err := iter.Next(); err != io.EOF {
		t.Fatalf("expected iterator to return EOF if at end of input: got %v, want %v", err, io.EOF)
	}
}

func TestEmptyIterator(t *testing.T) {
	iter := &dataElementIterator{empty: true, metaHeader: emptyElementIterator{}}
	_, err := iter.Next()
	if err != io.EOF {
		t.Fatalf("expected empty iterator to return io.EOF: got %v, want %v", err, io.EOF)
	}
}

func ExampleDataElementIterator() {
	r, err := os.Open("multiframe_image.dcm")
	if err != nil {
		fmt.Println(err)
		return
	}

	iter, err := NewDataElementIterator(r)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer iter.Close()

	for element, err := iter.Next(); err != io.EOF; element, err = iter.Next() {
		if element.Tag != PixelDataTag { // skip elements until pixel data is encountered
			continue
		}
		if fragments, ok := element.ValueField.(BulkDataIterator); ok {
			for fragment, err := fragments.Next(); err != io.EOF; fragment, err = fragments.Next() {
				ioutil.ReadAll(fragment) // process image fragment
			}
		}
	}
}

func TestDeflatedIterator_Close(t *testing.T) {
	closer := &stubCloser{}
	iter := &deflatedDataElementIterator{&emptyElementIterator{}, closer}
	iter.Close()

	if !closer.closed {
		t.Fatalf("expected deflated iterator to close the decompressor")
	}
}

type stubCloser struct {
	closed bool
}

func (closer *stubCloser) Close() error {
	closer.closed = true
	return nil
}
