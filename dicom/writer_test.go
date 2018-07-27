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
	"reflect"
	"testing"
)

func TestDataElementWriter_options(t *testing.T) {
	opts := ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if !element.Tag.IsMetaElement() {
			element.ValueField = []string{"HI"}
		}
		return element, nil
	})

	got := bytes.NewBuffer([]byte{})
	writer := mustNewDataElementWriterWithSyntax(t, got, ExplicitVRLittleEndianUID, opts)

	if err := writer.WriteElement(&DataElement{Tag: SpecificCharacterSetTag, VR: CSVR, ValueField: []string{}}); err != nil {
		t.Fatalf("writer.Next: %v", err)
	}

	gotDataSet, err := Parse(got)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	gotElement := gotDataSet.Elements[SpecificCharacterSetTag]
	want := &DataElement{Tag: SpecificCharacterSetTag, VR: CSVR, ValueLength: 2, ValueField: []string{"HI"}}

	if !reflect.DeepEqual(gotElement, want) {
		t.Fatalf("got %v, want %v", gotElement, want)
	}
}

func TestDataElementWriter_calculatesMetaHeaderAfterOptions(t *testing.T) {
	changeHeaderLen := ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if element.Tag == TransferSyntaxUIDTag {
			element.ValueField = []string{JPEGBaselineUID}
		}
		return element, nil
	})
	file := bytes.NewBuffer([]byte{})
	mustNewDataElementWriterWithSyntax(t, file, ImplicitVRLittleEndianUID, changeHeaderLen)

	got, err := Parse(file)
	if err != nil {
		t.Fatalf("parsing written DICOM: %v", err)
	}

	want := explicitVRLittleEndian.elementSize(TransferSyntaxUIDTag.DictionaryVR(), uint32(len(JPEGBaselineUID)))
	v, err := got.Elements[FileMetaInformationGroupLengthTag].IntValue()
	if err != nil {
		t.Fatalf("getting meta group length: %v", err)
	}
	if v != int64(want) {
		t.Fatalf("got %v, want %v", v, want)
	}
}

func mustNewDataElementWriterWithSyntax(t *testing.T, w io.Writer, syntaxUID string, opts ...ConstructOption) DataElementWriter {
	ret, err := NewDataElementWriter(w, &DataSet{
		Elements: map[DataElementTag]*DataElement{
			TransferSyntaxUIDTag: {Tag: TransferSyntaxUIDTag, ValueField: []string{syntaxUID}},
		},
	}, opts...)
	if err != nil {
		t.Fatalf("NewDataElementWriter: %v", err)
	}
	return ret
}
