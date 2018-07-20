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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

func writeDataElement(dw *dcmWriter, syntax transferSyntax, element *DataElement) error {
	element, err := processedElement(element)
	if err != nil {
		return fmt.Errorf("processing element: %v", err)
	}

	if err := dw.Tag(syntax.ByteOrder, element.Tag); err != nil {
		return fmt.Errorf("writing tag: %v", err)
	}
	if err := writeVR(dw, syntax, element.VR); err != nil {
		return fmt.Errorf("writing VR: %v", err)
	}
	if err := writeValueLength(dw, syntax, element.VR, element.ValueLength); err != nil {
		return fmt.Errorf("writing length: %v", err)
	}
	if err := writeValue(dw, syntax, element.VR, element.ValueLength, element.ValueField); err != nil {
		return fmt.Errorf("writing value: %v", err)
	}

	return nil
}

func processedElement(element *DataElement) (*DataElement, error) {
	vr := element.VR
	if element.VR == nil {
		vr = element.Tag.DictionaryVR()
	}

	length, err := calculateValueLength(element)
	if err != nil {
		return nil, fmt.Errorf("calculating value length: %v", err)
	}

	return &DataElement{element.Tag, vr, element.ValueField, length}, nil
}

func writeVR(dw *dcmWriter, syntax transferSyntax, vr *VR) error {
	if syntax.Implicit {
		// implicit VR syntax does not include VR in the DICOM file
		return nil
	}
	return dw.String(vr.Name)
}

func calculateValueLength(element *DataElement) (uint32, error) {
	if element.ValueLength == UndefinedLength {
		return UndefinedLength, nil
	}

	numBytes := int64(0)

	switch v := element.ValueField.(type) {
	case []string:
		for _, s := range v {
			numBytes += int64(len(s))
		}
		if len(v) > 0 { // requires "/" delimiter
			numBytes += int64(len(v)) - 1
		}
	case [][]byte:
		for _, fragment := range v {
			numBytes += int64(len(fragment))
		}
	case []int16:
		numBytes = int64(len(v)) * 2
	case []uint16:
		numBytes = int64(len(v)) * 2
	case []int32:
		numBytes = int64(len(v)) * 4
	case []uint32:
		numBytes = int64(len(v)) * 4
	case []float32:
		numBytes = int64(len(v)) * 4
	case []float64:
		numBytes = int64(len(v)) * 8
	case *Sequence, SequenceIterator:
		numBytes = UndefinedLength // TODO support explicit length sequence construction
	case *encapsulatedFormatIterator:
		numBytes = UndefinedLength
	case *oneShotIterator: // TODO support calculating length from one shot iterator
		return 0, fmt.Errorf("calculating lengths of one shot iterator not supported yet")
	case *nativeMultiFrame: // TODO support calculating length from native multi-frame
		return 0, fmt.Errorf("calculating lengths of native multiframe not supported yet")
	default:
		return 0, fmt.Errorf("unexpected ValueField type %T", element.ValueField)
	}

	if numBytes >= math.MaxUint32 {
		return UndefinedLength, nil
	}

	if numBytes%2 != 0 {
		numBytes++
	}

	return uint32(numBytes), nil
}

func writeValueLength(dw *dcmWriter, syntax transferSyntax, vr *VR, length uint32) error {
	if syntax.Implicit {
		return dw.UInt32(syntax.ByteOrder, length)
	}

	// For explicit VR, lengths can be stored in a 32 bit field or a 16 bit field
	// depending on the VR type. The 2 cases are defined at the link:
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_7.1.2

	switch vr {
	case OBVR, ODVR, OFVR, OLVR, OWVR, SQVR, UCVR, URVR, UTVR, UNVR:
		// case 1: 32-bit length
		if err := dw.UInt16(syntax.ByteOrder, 0); err != nil {
			return fmt.Errorf("writing reserved field")
		}
		if err := dw.UInt32(syntax.ByteOrder, length); err != nil {
			return fmt.Errorf("writing 32 bit length: %v", err)
		}
	default:
		// case 2: 16-bit length
		if length > math.MaxUint16 {
			return fmt.Errorf("data element value length exceeds unsigned 16-bit length")
		}
		if err := dw.UInt16(syntax.ByteOrder, uint16(length)); err != nil {
			return fmt.Errorf("writing 16 bit length: %v", err)
		}
	}

	return nil
}

func writeValue(dw *dcmWriter, syntax transferSyntax, vr *VR, length uint32, valueField interface{}) error {
	spacePadding := byte(0x20)
	nullPadding := byte(0x00)

	switch vr.kind {
	case textVR:
		return writeText(dw, spacePadding, valueField)
	case numberBinaryVR:
		return writeNumberBinary(dw, syntax, valueField)
	case bulkDataVR:
		return writeBulkData(dw, syntax, length, valueField)
	case uniqueIdentifierVR:
		return writeText(dw, nullPadding, valueField)
	case sequenceVR:
		return writeSequence(dw, syntax, valueField)
	case tagVR:
		return writeTag(dw, syntax.ByteOrder, valueField)
	default:
		return fmt.Errorf("unknown vr kind found: %v", vr.kind)
	}
}

func writeText(dw *dcmWriter, paddingByte byte, v interface{}) error {
	strs, ok := v.([]string)
	if !ok {
		return fmt.Errorf("expected type []string got %v", v)
	}

	b := strings.Join(strs, "\\")
	if len(b)%2 != 0 {
		b += string(paddingByte)
	}

	return dw.String(b)
}

func writeNumberBinary(dw *dcmWriter, syntax transferSyntax, v interface{}) error {
	switch field := v.(type) {
	case []int16, []uint16, []int32, []uint32, []float32, []float64:
		return binary.Write(dw, syntax.ByteOrder, v)
	default:
		return fmt.Errorf("unsupported binary number type: %T", field)
	}
}

func writeBulkData(dw *dcmWriter, syntax transferSyntax, length uint32, v interface{}) error {
	switch field := v.(type) {
	case BulkDataIterator:
		return field.write(dw, syntax)
	case [][]byte:
		idx := 0
		fragmentProvider := func() (io.Reader, error) {
			if idx >= len(field) {
				return nil, io.EOF
			}
			r := bytes.NewReader(field[idx])
			idx++
			return r, nil
		}
		if length == UndefinedLength {
			// UndefinedLength is always the encapsulated format.
			return writeEncapsulatedFormat(dw, syntax.ByteOrder, fragmentProvider)
		}
		return writeByteFragments(dw, fragmentProvider)
	case []int16, []uint16, []int32, []uint32, []float32, []float64:
		return binary.Write(dw, syntax.ByteOrder, field)
	case []string:
		return writeText(dw, ' ', v)
	default:
		return fmt.Errorf("unknown bulk data type: %T", v)
	}
}

func writeSequence(dw *dcmWriter, syntax transferSyntax, v interface{}) error {
	switch seq := v.(type) {
	case SequenceIterator:
		return errors.New("writing sequence from SequenceIterator not supported yet")
	case *Sequence:
		for _, item := range seq.Items {
			if err := dw.Tag(syntax.ByteOrder, DataElementTag(ItemTag)); err != nil {
				return fmt.Errorf("writing item tag: %v", err)
			}
			if err := dw.UInt32(syntax.ByteOrder, UndefinedLength); err != nil {
				return fmt.Errorf("writing item length: %v", err)
			}

			if err := writeDataSet(dw, syntax, item); err != nil {
				return fmt.Errorf("writing sequence item: %v", err)
			}

			if err := dw.Tag(syntax.ByteOrder, DataElementTag(ItemDelimitationItemTag)); err != nil {
				return fmt.Errorf("writing item delimitation item tag: %v", err)
			}
			if err := dw.UInt32(syntax.ByteOrder, 0); err != nil {
				return fmt.Errorf("writing length of item delimitation item tag: %v", err)
			}
		}
		// write sequence delimitation item
		if err := dw.Tag(syntax.ByteOrder, DataElementTag(SequenceDelimitationItemTag)); err != nil {
			return fmt.Errorf("writing tag of sequence delimitation item: %v", err)
		}
		if err := dw.UInt32(syntax.ByteOrder, 0); err != nil {
			return fmt.Errorf("writing item length of sequence delimitation item: %v", err)
		}
	default:
		return fmt.Errorf("unknown sequence type found: %v (expected *Sequence or SequenceIterator)", seq)
	}
	return nil
}

func writeTag(dr *dcmWriter, order binary.ByteOrder, valueField interface{}) error {
	tag, ok := valueField.([]uint32)
	if !ok {
		return fmt.Errorf("unexpected type for tag VR: %v (expected []uint32)", valueField)
	}
	if len(tag) != 1 {
		return fmt.Errorf("wrong value length for tag VR: %v (expected exactly 1)", len(tag))
	}
	return dr.Tag(order, DataElementTag(tag[0]))
}

func writeDataSet(dw *dcmWriter, syntax transferSyntax, ds *DataSet) error {
	for _, tag := range ds.SortedTags() {
		element := ds.Elements[tag]
		if err := writeDataElement(dw, syntax, element); err != nil {
			return fmt.Errorf("writing data element: %v", err)
		}
	}
	return nil
}
