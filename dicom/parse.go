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
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"unicode"
)

// Parse parses a DICOM file represented as an io.Reader, returning the DataSet defined by applying
// options sequentially in the order given to DataElements in the file.
//
// By default, BulkDataIterators are transformed into their appropriate buffered types for the VR:
// BulkDataBuffer for OW, OB, UN
// []uint32 for OL
// []float64 for OD
// []float32 for OF
// []string for UR, UT, UC
// This behaviour can be overridden by supplying a ParseOption that transforms DataElements with
// ValueField of type BulkDataIterator to a ValueField other than BulkDataIterator.
func Parse(r io.Reader, opts ...ParseOption) (*DataSet, error) {
	iter, err := NewDataElementIterator(r)
	if err != nil {
		return nil, fmt.Errorf("creating new data element iterator: %v", err)
	}
	defer iter.Close()

	return CollectDataElements(iter, opts...)
}

// CollectDataElements returns the DataSet defined by the elements in the DataElementIterator.
// The options will be applied in the order given. The DataElementIterator will be closed.
func CollectDataElements(iter DataElementIterator, opts ...ParseOption) (*DataSet, error) {
	ds := &DataSet{map[DataElementTag]*DataElement{}, iter.Length()}

	for elem, err := iter.Next(); err != io.EOF; elem, err = iter.Next() {
		if err != nil {
			return nil, err
		}
		processedElement, err := processElement(elem, iter.syntax().byteOrder(), opts...)
		if err != nil {
			return nil, err
		}
		if processedElement != nil { // nil check to test if ParseOption wants to filter out element
			ds.Elements[elem.Tag] = processedElement
		}
	}
	return ds, nil
}

// CollectSequence returns the Sequence defined by the items in the SequenceIterator.
// The options will be applied in the order given. The SequenceIterator will be closed.
func CollectSequence(iter SequenceIterator, opts ...ParseOption) (*Sequence, error) {
	var seq = &Sequence{[]*DataSet{}}
	for obj, err := iter.Next(); err != io.EOF; obj, err = iter.Next() {
		if err != nil {
			return nil, err
		}
		dataSet, err := CollectDataElements(obj, opts...)
		if err != nil {
			return nil, err
		}
		seq.append(dataSet)
	}
	return seq, nil
}

// CollectFragments returns the sequence of byte slices defined by the sequence of BulkDataReaders
// in the BulkDataIterator. The BulkDataIterator will be closed.
func CollectFragments(iter BulkDataIterator) ([][]byte, error) {
	buff := make([][]byte, 0)
	for r, err := iter.Next(); err != io.EOF; r, err = iter.Next() {
		if err != nil {
			return nil, err
		}
		fragment, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("reading fragment: %v", fragment)
		}
		buff = append(buff, fragment)
	}

	return buff, nil
}

// CollectFragmentReferences returns the sequence of BulkDataReferences defined by the sequence of
// BulkDataReaders in the BulkDataIterator. The given BulkDataIterator will be closed.
func CollectFragmentReferences(iter BulkDataIterator) ([]BulkDataReference, error) {
	refs := make([]BulkDataReference, 0)
	for r, err := iter.Next(); err != io.EOF; r, err = iter.Next() {
		if err != nil {
			return nil, err
		}
		fragmentSize, err := io.Copy(ioutil.Discard, r)
		if err != nil {
			return nil, err
		}

		refs = append(refs, BulkDataReference{ByteRegion{r.Offset, fragmentSize}})
	}

	return refs, nil
}

func processElement(element *DataElement, order binary.ByteOrder, opts ...ParseOption) (*DataElement, error) {
	if seqIter, ok := element.ValueField.(SequenceIterator); ok {
		// for sequence elements, apply options in post-order. (i.e process sequence items before
		// the sequence element)
		// Processing sequence items first protects options transforming SQ DataElements from the misuse
		// of the SequenceIterator (e.g. not collecting sequence items correctly)
		seq, err := CollectSequence(seqIter, opts...)
		if err != nil {
			return nil, fmt.Errorf("collecting sequence: %v", err)
		}

		processedSeq := &DataElement{element.Tag, element.VR, seq, element.ValueLength}
		return processElement(processedSeq, order, opts...)
	}

	return applyOptions(element, order, opts...)
}

func applyOptions(element *DataElement, order binary.ByteOrder, opts ...ParseOption) (*DataElement, error) {
	var err error
	for i, opt := range opts {
		element, err = opt.transform(element)
		if err != nil {
			return nil, fmt.Errorf("applying option %v: %v", i, err)
		}
		if element == nil { // option wants to filter this element out
			return nil, nil
		}
	}

	if _, ok := element.ValueField.(BulkDataIterator); ok {
		// As documented in Parse, when the options given do not collect data from the
		// BulkDataIterator we must collect the data in the byte stream somehow otherwise the
		// returned DataSet will not be coherent since it would contain a bunch of empty
		// BulkDataIterators.
		element, err = bufferBulkData(element, order)
	}

	return element, err
}

func bufferBulkData(element *DataElement, order binary.ByteOrder) (*DataElement, error) {
	if fragmentIterator, ok := element.ValueField.(BulkDataIterator); ok {
		fragments, err := fragmentIterator.ToBuffer()
		if err != nil {
			return nil, fmt.Errorf("buffering fragments: %v", err)
		}
		buff := fragments.Data()
		var valueField interface{}
		if element.VR == OWVR || element.VR == OBVR || element.VR == UNVR {
			valueField = fragments // preserve potentially multi-fragment types
		} else if len(buff) == 0 {
			valueField, err = emptyFragmentForType(element.VR)
		} else if len(buff) == 1 {
			valueField, err = decodeFragment(fragments.Data()[0], order, element.VR)
		} else {
			return nil, fmt.Errorf("more than 1 fragments found for single fragment type: got %v, want 0 or 1", len(buff))
		}

		return &DataElement{element.Tag, element.VR, valueField, element.ValueLength}, err
	}

	return nil, fmt.Errorf("wrong type for element.ValueField: got %v, want be BulkDataIterator", element.ValueField)
}

func emptyFragmentForType(vr *VR) (interface{}, error) {
	switch vr {
	case UCVR, URVR, UTVR:
		return []string{}, nil
	case OLVR:
		return []uint32{}, nil
	case ODVR:
		return []float64{}, nil
	case OFVR:
		return []float32{}, nil
	}
	return nil, fmt.Errorf("unexpected VR found for bulk data: %v", vr)
}

func decodeFragment(buff []byte, order binary.ByteOrder, vr *VR) (interface{}, error) {
	// Please refer to DICOM PS3.5 Part 5 for details on UC, UR, UT value representations
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_6.1

	var valueField interface{}
	switch vr {
	case UCVR:
		// UC may be padded with trailing spaces and uses the "\" to delimit multiple values
		return strings.Split(string(buff), "\\"), nil
	case URVR, UTVR:
		// UR: Trailing spaces shall be ignored. Backslash is not allowed. Shall be in ISO 2022 IR 6
		// UT: Trailing spaces may be ignored (and are in this implementation). Backslash not allowed.
		return []string{strings.TrimRightFunc(string(buff), unicode.IsSpace)}, nil
	case OLVR:
		valueField = make([]uint32, len(buff)/4)
	case ODVR:
		valueField = make([]float64, len(buff)/8)
	case OFVR:
		valueField = make([]float32, len(buff)/4)
	default:
		return nil, fmt.Errorf("unexpected vr found: %v", vr)
	}

	if err := binary.Read(bytes.NewReader(buff), order, valueField); err != nil {
		return nil, fmt.Errorf("reading to buffer: %v", err)
	}

	return valueField, nil
}
