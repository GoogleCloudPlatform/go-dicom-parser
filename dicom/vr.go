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
	"fmt"
)

// vrType is to group common encodings together
type vrType int

const (
	// textVR is for value fields that will be interpreted as simple text with space padding
	textVR vrType = iota

	// numberBinaryVR is for value fields that are parsed as binary numbers
	numberBinaryVR

	// bulkDataVR groups sequences of binary numbers
	bulkDataVR

	// uniqueIdentifierVR is for VR: UI. It has null padding
	uniqueIdentifierVR

	// sequenceVR is for VR: SQ
	sequenceVR

	// tagVR is for tags. Distinct from numberBinaryVR due to little endian byte ordering
	tagVR
)

// UndefinedLength as specified
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.1
const UndefinedLength = 0xffffffff

// VR models the DICOM Value representations (VR)
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_6.2
type VR struct {
	// Name represents the 2-character VR Code
	Name string

	kind vrType
}

var vrLookupMap = map[string]*VR{}

func newVR(text string, vrType vrType) *VR {
	vr := &VR{text, vrType}
	vrLookupMap[vr.Name] = vr

	return vr
}

func lookupVRByName(name string) (*VR, error) {
	r, ok := vrLookupMap[name]
	if !ok {
		return nil, fmt.Errorf("unknown vr name: %v", name)
	}
	return r, nil
}

// VR list obtained from
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_6.2
var (
	// textual VRs
	CSVR = newVR("CS", textVR)
	SHVR = newVR("SH", textVR)
	LOVR = newVR("LO", textVR)
	STVR = newVR("ST", textVR)
	LTVR = newVR("LT", textVR)
	ASVR = newVR("AS", textVR)

	// person name
	PNVR = newVR("PN", textVR)

	// application entity
	AEVR = newVR("AE", textVR)

	// dates/time VR
	DAVR = newVR("DA", textVR)
	TMVR = newVR("TM", textVR)
	DTVR = newVR("DT", textVR)

	// textual numbers
	ISVR = newVR("IS", textVR)
	DSVR = newVR("DS", textVR)

	// binary numbers
	SSVR = newVR("SS", numberBinaryVR)
	USVR = newVR("US", numberBinaryVR)
	SLVR = newVR("SL", numberBinaryVR)
	ULVR = newVR("UL", numberBinaryVR)
	FLVR = newVR("FL", numberBinaryVR)
	FDVR = newVR("FD", numberBinaryVR)

	// large binary sequences
	OBVR = newVR("OB", bulkDataVR)
	ODVR = newVR("OD", bulkDataVR)
	OLVR = newVR("OL", bulkDataVR)
	OWVR = newVR("OW", bulkDataVR)
	OFVR = newVR("OF", bulkDataVR)

	// unlimited char
	UCVR = newVR("UC", bulkDataVR)

	// unknown
	UNVR = newVR("UN", bulkDataVR)

	// URL
	URVR = newVR("UR", bulkDataVR)

	// unlimited text
	UTVR = newVR("UT", bulkDataVR)

	// attribute tag
	ATVR = newVR("AT", tagVR)

	// unique identifier
	UIVR = newVR("UI", uniqueIdentifierVR)

	// sequence
	SQVR = newVR("SQ", sequenceVR)
)
