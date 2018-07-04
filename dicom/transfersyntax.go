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

import "encoding/binary"

// list of transfer syntaxes obtained from
// http://dicom.nema.org/medical/dicom/current/output/html/part06.html#chapter_A
const (
	// ImplicitVRLittleEndianUID is the Implicit VR Little Endian UID
	ImplicitVRLittleEndianUID = "1.2.840.10008.1.2"
	// ExplicitVRLittleEndianUID is the Explicit VR Little Endian UID
	ExplicitVRLittleEndianUID = "1.2.840.10008.1.2.1"
	// ExplicitVRBigEndianUID is the Explicit VR Big Endian UID
	ExplicitVRBigEndianUID = "1.2.840.10008.1.2.2"
	// DeflatedExplicitVRLittleEndian is the Deflated Explicit VR Little Endian UID
	DeflatedExplicitVRLittleEndian = "1.2.840.10008.1.2.1.99"
	// JPEGBaselineUID is the JPEG Baseline (Process 1) transfer syntax UID
	JPEGBaselineUID = "1.2.840.10008.1.2.4.50"
)

func lookupTransferSyntax(uid string) transferSyntax {
	if uid == ExplicitVRLittleEndianUID {
		return explicitVRLittleEndian
	}
	if uid == ImplicitVRLittleEndianUID {
		return implicitVRLittleEndian
	}
	if uid == ExplicitVRBigEndianUID {
		return explicitVRBigEndian
	}

	// any other syntax should be explicit VR little endian according to PS3.5 A.4
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
	return explicitVRLittleEndian
}

type transferSyntax struct {
	Implicit  bool
	ByteOrder binary.ByteOrder
}

var (
	explicitVRLittleEndian = transferSyntax{false, binary.LittleEndian}
	implicitVRLittleEndian = transferSyntax{true, binary.LittleEndian}
	explicitVRBigEndian    = transferSyntax{false, binary.BigEndian}
)
