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
	"io"
	"testing"
)

func TestSequenceIterator_Termination(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		length uint32
		syntax transferSyntax
		err    error
	}{
		{
			"ExplicitSequenceLength, ExplicitVRLittleEndian, EOF in input causes EOF",
			nil,
			0,
			explicitVRLittleEndian,
			io.EOF,
		},
		{
			"ExplicitSequenceLength, ExplicitVRBigEndian, EOF in input causes EOF",
			nil,
			0,
			explicitVRBigEndian,
			io.EOF,
		},
		{
			"UndefinedSequenceLength, ExplicitVRLittleEndian, SequenceDelimiter causes EOF",
			[]byte{0xFE, 0xFF, 0xDD, 0xE0, 0, 0, 0, 0},
			UndefinedLength,
			explicitVRLittleEndian,
			io.EOF,
		},
		{
			"UndefinedSequenceLength, ExplicitVRBigEndian, SequenceDelimiter causes EOF",
			[]byte{0xFF, 0xFE, 0xE0, 0xDD, 0, 0, 0, 0},
			UndefinedLength,
			explicitVRBigEndian,
			io.EOF,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iter := createSeqIter(tc.data, tc.length, tc.syntax, t)
			if _, err := iter.Next(); err != tc.err {
				t.Fatalf("got error %v, want %v", err, tc.err)
			}
		})
	}
}

func createSeqIter(b []byte, length uint32, syntax transferSyntax, t *testing.T) SequenceIterator {
	iter, err := newSequenceIterator(dcmReaderFromBytes(b), length, syntax)
	if err != nil {
		t.Fatalf("unexpected error creating SequenceIterator: %v", err)
	}
	return iter
}
