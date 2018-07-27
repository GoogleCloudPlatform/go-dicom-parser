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
	"unicode"
)

func readDataElement(dr *dcmReader, syntax transferSyntax) (*DataElement, error) {
	tag, err := dr.Tag(syntax.byteOrder())
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("getting tag: %v", err)
	}

	if tag == ItemDelimitationItemTag {
		// handles the case when we are parsing a nested data set within a sequence with undefined
		// length. This code should never run for the top level data set
		length, err := dr.UInt32(syntax.byteOrder())
		if err != nil {
			return nil, fmt.Errorf("reading 32 bit length of item delimitation: %v", err)
		}
		if length != 0 {
			return nil, fmt.Errorf("wrong length for item delimiter. got %v, want %v", length, 0)
		}
		return nil, io.EOF
	}

	vr, err := syntax.readVR(dr, tag)
	if err != nil {
		return nil, fmt.Errorf("getting vr %v", err)
	}

	length, err := syntax.readValueLength(dr, vr)
	if err != nil {
		return nil, fmt.Errorf("getting length: %v", err)
	}

	value, err := readValue(tag, dr, vr, length, syntax)
	if err != nil {
		return nil, fmt.Errorf("parsing value %v", err)
	}

	return &DataElement{tag, vr, value, length}, nil
}

func readValue(tag DataElementTag, dr *dcmReader, vr *VR, length uint32, syntax transferSyntax) (interface{}, error) {
	switch vr.kind {
	case textVR:
		return readText(dr, length, vr, unicode.IsSpace)
	case numberBinaryVR:
		return readNumberBinary(dr, length, vr, syntax.byteOrder())
	case bulkDataVR:
		return readBulkData(dr, tag, length)
	case uniqueIdentifierVR:
		return readText(dr, length, vr, func(r rune) bool {
			return r == 0x00 || r == ' '
		})
	case sequenceVR:
		return readSequence(dr, length, syntax)
	case tagVR:
		return readTag(dr, syntax, length)
	default:
		return nil, fmt.Errorf("unknown vr type found: %v", vr.kind)
	}
}

func readTag(dr *dcmReader, syntax transferSyntax, length uint32) ([]uint32, error) {
	ret := make([]uint32, length/4) // 4 bytes per tag

	for i := range ret {
		t, err := dr.Tag(syntax.byteOrder())
		if err != nil {
			return nil, err
		}
		ret[i] = uint32(t)
	}
	return ret, nil
}

func readText(dr *dcmReader, length uint32, vr *VR, isPadding func(rune) bool) ([]string, error) {
	if length <= 0 {
		return []string{}, nil
	}

	valueField, err := dr.String(int64(length))
	if err != nil {
		return nil, fmt.Errorf("reading text field value: %v", err)
	}

	// deal with value multiplicity
	strs := strings.Split(valueField, "\\")
	for i, s := range strs {
		if vr == UTVR || vr == STVR || vr == LTVR {
			strs[i] = strings.TrimRightFunc(s, isPadding)
		} else {
			strs[i] = strings.TrimFunc(s, isPadding)
		}
	}
	return strs, nil
}

func readNumberBinary(dr *dcmReader, length uint32, vr *VR, order binary.ByteOrder) (interface{}, error) {
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

	if err := binary.Read(dr.cr, order, data); err != nil {
		return nil, fmt.Errorf("binary.Read(_, _, _) => %v", err)
	}

	return data, nil
}

func readBulkData(dr *dcmReader, tag DataElementTag, length uint32) (BulkDataIterator, error) {
	if length == UndefinedLength {
		if tag == PixelDataTag {
			// Specified in http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
			// (7FE0,0010) and undefined length means pixel data in encapsulated (compressed) format
			return NewEncapsulatedFormatIterator(dr.cr, dr.cr.bytesRead), nil
		}

		return nil, errors.New("syntax with undefined length in non-pixel data not supported")
	}

	// for native (uncompressed) formats, return regular bulk data stream
	limitedReader := limitCountReader(dr.cr, int64(length))
	return NewBulkDataIterator(limitedReader, dr.cr.bytesRead), nil
}

func readSequence(dr *dcmReader, length uint32, syntax transferSyntax) (SequenceIterator, error) {
	return newSequenceIterator(dr, length, syntax)
}
