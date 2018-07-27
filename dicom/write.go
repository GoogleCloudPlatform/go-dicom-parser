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
	"strings"
)

func writeDataElement(dw *dcmWriter, syntax transferSyntax, element *DataElement) error {
	if err := dw.Tag(syntax.byteOrder(), element.Tag); err != nil {
		return fmt.Errorf("writing tag: %v", err)
	}
	if err := syntax.writeVR(dw, element.VR); err != nil {
		return fmt.Errorf("writing VR: %v", err)
	}
	if err := syntax.writeValueLength(dw, element.VR, element.ValueLength); err != nil {
		return fmt.Errorf("writing length: %v", err)
	}
	if err := writeValue(dw, syntax, element.VR, element.ValueField, element.ValueLength); err != nil {
		return fmt.Errorf("writing value: %v", err)
	}

	return nil
}

func writeValue(dw *dcmWriter, syntax transferSyntax, vr *VR, valueField interface{}, valueLength uint32) error {
	spacePadding := byte(0x20)
	nullPadding := byte(0x00)

	switch vr.kind {
	case textVR:
		return writeText(dw, spacePadding, valueField)
	case numberBinaryVR:
		return writeNumberBinary(dw, syntax, valueField)
	case bulkDataVR:
		return writeBulkData(dw, syntax, valueField)
	case uniqueIdentifierVR:
		return writeText(dw, nullPadding, valueField)
	case sequenceVR:
		return writeSequence(dw, syntax, valueField, valueLength)
	case tagVR:
		return writeTag(dw, syntax.byteOrder(), valueField)
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
		return binary.Write(dw, syntax.byteOrder(), v)
	default:
		return fmt.Errorf("unsupported binary number type: %T", field)
	}
}

func writeBulkData(dw *dcmWriter, syntax transferSyntax, v interface{}) error {
	switch field := v.(type) {
	case BulkDataIterator:
		return field.write(dw, syntax)
	case BulkDataBuffer:
		return field.write(dw, syntax)
	case []int16, []uint16, []int32, []uint32, []float32, []float64:
		return binary.Write(dw, syntax.byteOrder(), field)
	case []string:
		return writeText(dw, ' ', v)
	default:
		return fmt.Errorf("unknown bulk data type: %T", v)
	}
}

func writeSequence(dw *dcmWriter, syntax transferSyntax, v interface{}, seqLength uint32) error {
	switch seq := v.(type) {
	case SequenceIterator:
		return errors.New("writing sequence from SequenceIterator not supported yet")
	case *Sequence:
		for _, item := range seq.Items {
			if err := dw.Tag(syntax.byteOrder(), DataElementTag(ItemTag)); err != nil {
				return fmt.Errorf("writing item tag: %v", err)
			}
			if err := dw.UInt32(syntax.byteOrder(), item.Length); err != nil {
				return fmt.Errorf("writing item length: %v", err)
			}

			if err := writeDataSet(dw, syntax, item); err != nil {
				return fmt.Errorf("writing sequence item: %v", err)
			}

			if item.Length >= UndefinedLength {
				if err := dw.Delimiter(syntax.byteOrder(), ItemDelimitationItemTag); err != nil {
					return fmt.Errorf("writing item delimitation item: %v", err)
				}
			}
		}

		if seqLength >= UndefinedLength {
			if err := dw.Delimiter(syntax.byteOrder(), SequenceDelimitationItemTag); err != nil {
				return fmt.Errorf("writing sequence delimitation item: %v", err)
			}
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
	for _, t := range tag {
		if err := dr.Tag(order, DataElementTag(t)); err != nil {
			return fmt.Errorf("writing tag: %v", err)
		}
	}
	return nil
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
