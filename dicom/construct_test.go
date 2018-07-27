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
	"fmt"
	"io"
	"io/ioutil"
	"testing"
)

func TestConstruct(t *testing.T) {
	var setElementLengthsToZero = ParseOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		element.ValueLength = 0
		return element, nil
	})
	var noMetaGroupLength = ParseOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if element.Tag == FileMetaInformationGroupLengthTag {
			return nil, nil
		}
		return element, nil
	})

	tests := []struct {
		name string
		file string
		opts []ParseOption
	}{
		{
			"explicit VR little endian syntax with undefined seq & item lengths",
			"ExplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{},
		},
		{
			"explicit VR little endian syntax with explicit lengths",
			"ExplicitVRLittleEndian.dcm",
			[]ParseOption{},
		},
		{
			"explicit VR little endian syntax with no element lengths set",
			"ExplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{setElementLengthsToZero},
		},
		{
			"implicit VR little endian syntax with undefined seq & item lengths",
			"ImplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{},
		},
		{
			"implicit VR little endian syntax with explicit lengths",
			"ImplicitVRLittleEndian.dcm",
			[]ParseOption{},
		},
		{
			"implicit VR little endian syntax with no element lengths set",
			"ImplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{setElementLengthsToZero},
		},
		{
			"explicit VR big endian with undef seq & item lengths",
			"ExplicitVRBigEndianUndefLen.dcm",
			[]ParseOption{},
		},
		{
			"no meta group length in the implicit syntax",
			"ImplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{noMetaGroupLength},
		},
		{
			"no meta group length in the explicit syntax",
			"ExplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{noMetaGroupLength},
		},
		{
			"no meta group length and no element lengths given",
			"ExplicitVRLittleEndianUndefLen.dcm",
			[]ParseOption{noMetaGroupLength, setElementLengthsToZero},
		},
		{
			"writing compressed format",
			"MultiFrameCompressed.dcm",
			[]ParseOption{},
		},
		{
			"writing uncompressed format",
			"MultiFrameUncompressed.dcm",
			[]ParseOption{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := bytes.NewBuffer([]byte{})
			dataSet := parse(tc.file, t, tc.opts...)
			if err := Construct(w, dataSet); err != nil {
				t.Fatalf("Construct: %v", err)
			}

			f, err := openFile(tc.file)
			if err != nil {
				t.Fatalf("opening test file: %v", err)
			}

			compareFiles(t, w, f)
		})
	}
}

func TestConstruct_NoVR(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{
			"explicit syntax with no VRs set",
			"ExplicitVRLittleEndianUndefLen.dcm",
		},
		{
			"implicit syntax with no VRs set",
			"ImplicitVRLittleEndianUndefLen.dcm",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := bytes.NewBuffer([]byte{})
			dataSet := parse(tc.file, t)
			removeVRsFromDataSet(dataSet)
			if err := Construct(w, dataSet); err != nil {
				t.Fatalf("Construct: %v", err)
			}

			f, err := openFile(tc.file)
			if err != nil {
				t.Fatalf("opening test file: %v", err)
			}

			compareFiles(t, w, f)
		})
	}
}

func TestConstruct_BulkData(t *testing.T) {
	pixels := []byte("asbcdegh")
	tests := []struct {
		name      string
		pixelData DataElementValue
		expected  BulkDataBuffer
	}{
		{
			"BulkDataIterator with explicit length",
			NewBulkDataIteratorWithLength(bytes.NewReader(pixels), 0 /*offset*/, int64(len(pixels))),
			NewBulkDataBuffer(pixels),
		},
		{
			"BulkDataIterator with 0 length",
			NewBulkDataIteratorWithLength(bytes.NewReader([]byte{}), 0 /*offset*/, int64(0)),
			NewBulkDataBuffer([]byte{}),
		},
		{
			"Native BulkDataBuffer",
			NewBulkDataBuffer(pixels),
			NewBulkDataBuffer(pixels),
		},
		{
			"Native BulkDataBuffer with odd length",
			NewBulkDataBuffer([]byte{'a'}),
			NewBulkDataBuffer([]byte{'a', 0x00}),
		},
		{
			"Encapsulated BulkDataBuffer",
			NewEncapsulatedFormatBuffer(nil, pixels),
			NewEncapsulatedFormatBuffer(nil, pixels),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := minimalDataSet.Merge(NewDataSet(map[DataElementTag]interface{}{
				PixelDataTag: tc.pixelData,
			}))

			buf := &bytes.Buffer{}
			if err := Construct(buf, ds); err != nil {
				t.Fatalf("Construct(_, %v) => %v, expected nil error", ds, err)
			}

			actualDS, err := Parse(buf)
			if err != nil {
				t.Fatalf("Parse => %v", err)
			}

			actualPixelData, ok := actualDS.Elements[PixelDataTag].ValueField.(BulkDataBuffer)
			if !ok {
				t.Fatalf("expected PixelData to be BulkDataBuffer but found %T", actualDS.Elements[PixelDataTag].ValueField)
			}
			if len(tc.expected.Data()) != len(actualPixelData.Data()) {
				t.Fatalf("expected %d fragments but found %d", len(tc.expected.Data()), len(actualPixelData.Data()))
			}
			for i := 0; i < len(tc.expected.Data()); i++ {
				if !bytes.Equal(tc.expected.Data()[i], actualPixelData.Data()[i]) {
					t.Errorf("index %d: expected PixelData = %s but found %s", i, string(tc.expected.Data()[i]), string(actualPixelData.Data()[i]))
				}
			}
		})
	}
}

func TestConstruct_InvalidDataSet(t *testing.T) {
	tests := []struct {
		name string
		in   *DataSet
	}{
		{
			"nil cannot be written",
			NewDataSet(map[DataElementTag]interface{}{
				PixelDataTag: nil,
			}),
		},
		{
			"empty []BulkDataReference cannot be written",
			NewDataSet(map[DataElementTag]interface{}{
				PixelDataTag: []BulkDataReference{},
			}),
		},
		{
			"non-empty []BulkDataReference cannot be written",
			NewDataSet(map[DataElementTag]interface{}{
				PixelDataTag: []BulkDataReference{{ByteRegion{1, 2}}},
			}),
		},
		{
			"BulkDataIterator without explicit length",
			NewDataSet(map[DataElementTag]interface{}{
				PixelDataTag: NewBulkDataIterator(bytes.NewBuffer([]byte("blhblah")), 0),
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Construct(bytes.NewBuffer([]byte{}), tc.in); err == nil {
				t.Fatal("expected an error to be returned")
			}
		})
	}
}

func TestConstructOptions_Recursion(t *testing.T) {
	childFound := false
	var lookForChild = ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if element.Tag == ReferencedImageSequenceTag {
			childFound = true
		}
		return element, nil
	})

	dataSet := parse("ExplicitVRLittleEndian.dcm", t)
	if err := Construct(bytes.NewBuffer([]byte{}), dataSet, lookForChild); err != nil {
		t.Fatalf("Construct: %v", err)
	}

	if !childFound {
		t.Fatalf("expected the nested data element to be visited")
	}
}

func TestConstructOptions_Error(t *testing.T) {
	var errorConstruct = ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		return nil, fmt.Errorf("producing expected error")
	})
	dataSet := parse("ExplicitVRLittleEndian.dcm", t)
	if err := Construct(bytes.NewBuffer([]byte{}), dataSet, errorConstruct); err == nil {
		t.Fatalf("expected error to be returned")
	}
}

func removeVRsFromElement(element *DataElement) {
	element.VR = nil
	if seq, ok := element.ValueField.(*Sequence); ok {
		for _, ds := range seq.Items {
			removeVRsFromDataSet(ds)
		}
	}
}

func removeVRsFromDataSet(dataSet *DataSet) {
	for _, elem := range dataSet.Elements {
		removeVRsFromElement(elem)
	}
}

func compareFiles(t *testing.T, got, want io.Reader) {
	gotBytes, err := ioutil.ReadAll(got)
	if err != nil {
		t.Fatalf("reading result bytes: %v", err)
	}
	wantBytes, err := ioutil.ReadAll(want)
	if err != nil {
		t.Fatalf("reading expected bytes: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("got %v\n, want %v", gotBytes, wantBytes)
	}
}
