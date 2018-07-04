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
			DeflatedExplicitVRLittleEndian,
			explicitVRLittleEndian,
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
