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

import "testing"

func TestLookupTransferSyntax(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want transferSyntax
	}{
		{
			"explicit vr little endian",
			ExplicitVRLittleEndianUID,
			explicitVRLittleEndian,
		},
		{
			"implicit vr little endian",
			ImplicitVRLittleEndianUID,
			implicitVRLittleEndian,
		},
		{
			"explicit vr big endian",
			ExplicitVRBigEndianUID,
			explicitVRBigEndian,
		},
		{
			"jpeg baseline uid",
			JPEGBaselineUID,
			explicitVRLittleEndian,
		},
		{
			"deflated explicit vr little endian",
			DeflatedExplicitVRLittleEndianUID,
			deflatedExplicitVRLittleEndian,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lookupTransferSyntax(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
