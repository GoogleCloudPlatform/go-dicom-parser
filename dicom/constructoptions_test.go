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

func TestLengthOptions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		opts []ConstructOption
		want string
	}{
		{
			"explicit -> undefined length conversion in the explicit syntax",
			"ExplicitVRLittleEndian.dcm",
			[]ConstructOption{UndefinedLengths},
			"ExplicitVRLittleEndianUndefLen.dcm",
		},
		{
			"undefined -> explicit length conversion in the explicit syntax",
			"ExplicitVRLittleEndianUndefLen.dcm",
			[]ConstructOption{ExplicitLengths},
			"ExplicitVRLittleEndian.dcm",
		},
		{
			"explicit -> undefined length conversion in the implicit syntax",
			"ImplicitVRLittleEndian.dcm",
			[]ConstructOption{UndefinedLengths},
			"ImplicitVRLittleEndianUndefLen.dcm",
		},
		{
			"undefined -> explicit length conversion in the implicit syntax",
			"ImplicitVRLittleEndianUndefLen.dcm",
			[]ConstructOption{ExplicitLengths},
			"ImplicitVRLittleEndian.dcm",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataSet := parse(tc.in, t)

			got := bytes.NewBuffer([]byte{})
			if err := Construct(got, dataSet, tc.opts...); err != nil {
				t.Fatalf("Construct: %v", err)
			}

			want, err := openFile(tc.want)
			if err != nil {
				t.Fatalf("opening wanted file: %v", err)
			}
			compareFiles(t, got, want)
		})
	}
}
