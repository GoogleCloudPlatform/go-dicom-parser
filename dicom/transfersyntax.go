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
	"math"
)

// list of transfer syntaxes obtained from
// http://dicom.nema.org/medical/dicom/current/output/html/part06.html#chapter_A
const (
	// ImplicitVRLittleEndianUID is the Implicit VR Little Endian UID
	ImplicitVRLittleEndianUID = "1.2.840.10008.1.2"
	// ExplicitVRLittleEndianUID is the Explicit VR Little Endian UID
	ExplicitVRLittleEndianUID = "1.2.840.10008.1.2.1"
	// ExplicitVRBigEndianUID is the Explicit VR Big Endian UID
	ExplicitVRBigEndianUID = "1.2.840.10008.1.2.2"
	// DeflatedExplicitVRLittleEndianUID is the Deflated Explicit VR Little Endian UID
	DeflatedExplicitVRLittleEndianUID = "1.2.840.10008.1.2.1.99"
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
	if uid == DeflatedExplicitVRLittleEndianUID {
		return deflatedExplicitVRLittleEndian
	}

	// any other syntax should be explicit VR little endian according to PS3.5 A.4
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
	return explicitVRLittleEndian
}

const (
	vrSize  = 2
	tagSize = 4
)

type transferSyntax interface {
	byteOrder() binary.ByteOrder
	isDeflated() bool
	elementSize(vr *VR, valueFieldLength uint32) uint32
	readVR(dr *dcmReader, tag DataElementTag) (*VR, error)
	readValueLength(dr *dcmReader, vr *VR) (uint32, error)
	writeVR(dw *dcmWriter, vr *VR) error
	writeValueLength(dw *dcmWriter, vr *VR, valueFieldLength uint32) error
}

type implicitSyntax struct{}

func (implicitSyntax) byteOrder() binary.ByteOrder {
	return binary.LittleEndian
}

func (implicitSyntax) isDeflated() bool {
	return false
}

func (implicitSyntax) elementSize(vr *VR, valueFieldLength uint32) uint32 {
	if valueFieldLength == UndefinedLength {
		return UndefinedLength
	}
	return tagSize + 4 /*length*/ + valueFieldLength
}

func (implicitSyntax) readVR(dr *dcmReader, tag DataElementTag) (*VR, error) {
	return tag.DictionaryVR(), nil
}

func (implicitSyntax) readValueLength(dr *dcmReader, vr *VR) (uint32, error) {
	return dr.UInt32(binary.LittleEndian)
}

func (implicitSyntax) writeValueLength(dw *dcmWriter, vr *VR, valueFieldLength uint32) error {
	return dw.UInt32(binary.LittleEndian, valueFieldLength)
}

func (implicitSyntax) writeVR(dw *dcmWriter, vr *VR) error {
	// This just a no-op since the implicit syntax does not write VRs into the file.
	return nil
}

type explicitSyntax struct {
	order    binary.ByteOrder
	deflated bool
}

func (s explicitSyntax) byteOrder() binary.ByteOrder {
	return s.order
}

func (s explicitSyntax) isDeflated() bool {
	return s.deflated
}

func (s explicitSyntax) elementSize(vr *VR, valueFieldLength uint32) uint32 {
	if valueFieldLength == UndefinedLength {
		return UndefinedLength
	}

	if s.has32BitLength(vr) {
		return tagSize + vrSize + 2 /*reserved*/ + 4 /*32-bit length*/ + valueFieldLength
	}
	return tagSize + vrSize + 2 /*16-bit length*/ + valueFieldLength
}

func (s explicitSyntax) readVR(dr *dcmReader, tag DataElementTag) (*VR, error) {
	vrString, err := dr.String(vrSize)
	if err != nil {
		return nil, fmt.Errorf("getting vr %v", vrString)
	}

	return lookupVRByName(vrString)
}

func (s explicitSyntax) readValueLength(dr *dcmReader, vr *VR) (uint32, error) {
	if s.has32BitLength(vr) {
		if _, err := dr.UInt16(s.order); err != nil {
			return 0, fmt.Errorf("reading reserved field %v", err)
		}

		length, err := dr.UInt32(s.order)
		if err != nil {
			return 0, fmt.Errorf("reading 32 bit length: %v", err)
		}
		return length, nil
	}

	length, err := dr.UInt16(s.order)
	if err != nil {
		return 0, fmt.Errorf("reading 16 bit length: %v", err)
	}
	return uint32(length), nil
}

func (s explicitSyntax) writeValueLength(dw *dcmWriter, vr *VR, valueFieldLength uint32) error {
	if s.has32BitLength(vr) {
		if err := dw.UInt16(s.order, 0); err != nil {
			return fmt.Errorf("writing reserved field")
		}
		if err := dw.UInt32(s.order, valueFieldLength); err != nil {
			return fmt.Errorf("writing 32 bit length: %v", err)
		}
	} else {
		if valueFieldLength > math.MaxUint16 {
			return fmt.Errorf("data element value length exceeds unsigned 16-bit length")
		}
		if err := dw.UInt16(s.order, uint16(valueFieldLength)); err != nil {
			return fmt.Errorf("writing 16 bit length: %v", err)
		}
	}

	return nil
}

func (s explicitSyntax) writeVR(dw *dcmWriter, vr *VR) error {
	return dw.String(vr.Name)
}

func (s explicitSyntax) has32BitLength(vr *VR) bool {
	// For explicit VR, lengths can be stored in a 32 bit field or a 16 bit field
	// depending on the VR type. The 2 cases are defined at the link:
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.2
	switch vr {
	case OBVR, ODVR, OFVR, OLVR, OWVR, SQVR, UCVR, URVR, UTVR, UNVR:
		return true
	default:
		return false
	}
}

var (
	explicitVRLittleEndian         = explicitSyntax{binary.LittleEndian, false}
	deflatedExplicitVRLittleEndian = explicitSyntax{binary.LittleEndian, true}
	implicitVRLittleEndian         = implicitSyntax{}
	explicitVRBigEndian            = explicitSyntax{binary.BigEndian, false}
)
