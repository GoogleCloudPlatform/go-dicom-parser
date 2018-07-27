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
	"testing"
)

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

func TestImplicitSyntax_ReadVR(t *testing.T) {
	tests := []struct {
		name string
		in   DataElementTag
		want *VR
	}{
		{
			"private tags return UN VR",
			DataElementTag(0x12331233),
			UNVR,
		},
		{
			"public tags are looked up from data dictionary",
			FileMetaInformationGroupLengthTag,
			ULVR,
		},
		{
			"when multiple VRs are specified in the data dictionary, the right-most is returned",
			GrayLookupTableDataTag,
			OWVR,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := implicitVRLittleEndian.readVR(dcmReaderFromBytes([]byte{}), tc.in)
			if err != nil {
				t.Fatalf("reading VR from implicit syntax: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExplicitSyntax_invalidVR(t *testing.T) {
	dr := dcmReaderFromBytes([]byte("ZZ"))
	if _, err := explicitVRLittleEndian.readVR(dr, DataElementTag(0)); err == nil {
		t.Fatalf("expected error to be returned")
	}
}

func TestExplicitSyntax_ReadVR(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		tag   DataElementTag
		want  *VR
	}{
		{
			"when in the explicit VR syntax, the data dictionary specified VR is ignored",
			[]byte("UI"),
			GrayLookupTableDataTag,
			UIVR,
		},
	}

	for _, tc := range tests {
		vr, err := explicitVRLittleEndian.readVR(dcmReaderFromBytes(tc.bytes), tc.tag)
		if err != nil {
			t.Fatalf("readVR(_) => %v", err)
		}
		if vr != tc.want {
			t.Fatalf("got %v, want %v", vr, tc.want)
		}
	}
}
