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
	"errors"
	"fmt"
	"io"
	"strings"
)

func parseDataElement(dr *dcmReader, metaData dicomMetaData) (*DataElement, error) {
	if metaData.syntax.Implicit {
		return nil, errors.New("implicit VR not supported yet")
	}

	tag, err := dr.Tag(metaData.syntax.ByteOrder)
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("getting tag: %v", err)
	}

	if tag == ItemDelimitationItemTag {
		// handles the case when we are parsing a nested data set within a sequence with undefined
		// length. This code should never run for the top level data set
		length, err := dr.UInt32(metaData.syntax.ByteOrder)
		if err != nil {
			return nil, fmt.Errorf("reading 32 bit length of item delimitation: %v", err)
		}
		if length != 0 {
			return nil, fmt.Errorf("wrong length for item delimiter. got %v, want %v", length, 0)
		}
		return nil, io.EOF
	}

	vr, err := parseVR(dr)
	if err != nil {
		return nil, fmt.Errorf("getting vr %v", err)
	}

	length, err := parseValueLength(dr, vr, metaData)
	if err != nil {
		return nil, fmt.Errorf("getting length: %v", err)
	}

	value, err := parseValue(tag, dr, vr, length, metaData)
	if err != nil {
		return nil, fmt.Errorf("parsing value %v", err)
	}

	return &DataElement{tag, vr, value, length}, nil
}

func parseValueLength(dr *dcmReader, vr *VR, metaData dicomMetaData) (uint32, error) {
	// For explicit VR, lengths can be stored in a 32 bit field or a 16 bit field
	// depending on the VR type. The 2 cases are defined at the link:
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.2

	switch vr {
	case OBVR, ODVR, OFVR, OLVR, OWVR, SQVR, UCVR, URVR, UTVR, UNVR:
		// case 1: 32-bit length
		if _, err := dr.Int16(metaData.syntax.ByteOrder); err != nil {
			return 0, fmt.Errorf("reading reserved field %v", err)
		}

		length, err := dr.UInt32(metaData.syntax.ByteOrder)
		if err != nil {
			return 0, fmt.Errorf("reading 32 bit length: %v", err)
		}

		return length, nil
	}

	// case 2: read 16-bit length
	length, err := dr.UInt16(metaData.syntax.ByteOrder)
	if err != nil {
		return 0, fmt.Errorf("reading 16 bit length: %v", err)
	}

	return uint32(length), nil
}

func parseValue(tag DataElementTag, dr *dcmReader, vr *VR, length uint32, metaData dicomMetaData) (interface{}, error) {
	spacePadding := byte(0x20)
	nullPadding := byte(0x00)

	switch vr.kind {
	case textVR:
		return parseText(dr, length, spacePadding)
	case numberBinaryVR:
		return parseNumberBinary(dr, length, vr, metaData)
	case bulkDataVR:
		return parseBulkData(dr, tag, length)
	case uniqueIdentifierVR:
		return parseText(dr, length, nullPadding)
	case sequenceVR:
		return parseSequence(dr, length, metaData)
	case tagVR:
		return parseTag(dr, metaData, length)
	default:
		return nil, fmt.Errorf("unknown vr type found: %v", vr.kind)
	}
}

func parseTag(dr *dcmReader, metaData dicomMetaData, length uint32) ([]uint32, error) {
	ret := make([]uint32, length/4) // 4 bytes per tag

	for i := range ret {
		t, err := dr.Tag(metaData.syntax.ByteOrder)
		if err != nil {
			return nil, err
		}
		ret[i] = uint32(t)
	}
	return ret, nil
}

func parseText(dr *dcmReader, length uint32, paddingByte byte) ([]string, error) {
	if length <= 0 {
		return nil, nil
	}

	valueField, err := dr.String(int64(length))
	if err != nil {
		return nil, fmt.Errorf("reading text field value: %v", err)
	}

	// deal with value multiplicity
	strs := strings.Split(valueField, "\\")
	for i, s := range strs {
		strs[i] = strings.TrimRight(s, string(paddingByte)) // TODO this is wrong for UT
	}
	return strs, nil
}

func parseNumberBinary(dr *dcmReader, length uint32, vr *VR, metaData dicomMetaData) (interface{}, error) {
	var data interface{}

	switch vr {
	case SSVR:
		data = make([]int16, length/2)
	case USVR:
		data = make([]uint16, length/2)
	case SLVR:
		data = make([]int32, length/4)
	case ULVR:
		data = make([]uint32, length/4)
	case FLVR:
		data = make([]float32, length/4)
	case FDVR:
		data = make([]float64, length/8)
	default:
		return nil, fmt.Errorf("unknown vr: %v", vr)
	}

	if err := binary.Read(dr.cr, metaData.syntax.ByteOrder, data); err != nil {
		return nil, fmt.Errorf("binary.Read(_, _, _) => %v", err)
	}

	return data, nil
}

func parseBulkData(dr *dcmReader, tag DataElementTag, length uint32) (BulkDataIterator, error) {
	if length == UndefinedLength {
		if tag == PixelDataTag {
			// Specified in http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
			// (7FE0,0010) and undefined length means pixel data in encapsulated (compressed) format
			return newEncapsulatedFormatIterator(dr)
		}

		return nil, errors.New("syntax with undefined length in non-pixel data not supported")
	}

	// for native (uncompressed) formats, return regular bulk data stream
	return newOneShotIterator(limitCountReader(dr.cr, int64(length))), nil
}

func parseSequence(dr *dcmReader, length uint32, metaData dicomMetaData) (SequenceIterator, error) {
	return newSequenceIterator(dr, length, metaData)
}

func parseVR(dr *dcmReader) (*VR, error) {
	vrString, err := dr.String(2)
	if err != nil {
		return nil, fmt.Errorf("getting vr %v", vrString)
	}

	return lookupVRByName(vrString)
}
