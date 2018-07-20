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
	"io"
)

// Construct writes the given *DataSet as a DICOM file to the given io.Writer. The desired output
// transfer syntax is specified as a required TransferSyntax DataElement (0002,0010). By default,
// there is no validation against the DICOM standard of any form.
//
// If a *DataElement in the *DataSet is missing VR it will be filled in from the
// DICOM Data Dictionary. The ValueLength of DataElements are ignored and re-calculated.
func Construct(w io.Writer, dataSet *DataSet) error {
	dw := &dcmWriter{w}

	if err := writeDicomSignature(dw); err != nil {
		return err
	}

	dataSetSyntax, err := findSyntaxFromDataSet(dataSet)
	if err != nil {
		return fmt.Errorf("getting transfer syntax from data set: %v", err)
	}

	// The FileMetaInformationGroupLength element is a critical component of the Meta Header. It
	// stores how long the meta header is. Thus, we need to re-calculate it properly.
	metaGroupLengthElement, err := createMetaGroupLengthElement(dataSet)
	if err != nil {
		return fmt.Errorf("creating meta group length element: %v", err)
	}
	dataSet.Elements[FileMetaInformationGroupLengthTag] = metaGroupLengthElement

	for _, tag := range dataSet.SortedTags() {
		element := dataSet.Elements[tag]

		syntax := dataSetSyntax
		if tag.IsMetaElement() {
			// File meta elements are always in explicit VR little endian as specified in the standard
			// http://dicom.nema.org/medical/dicom/current/output/html/part10.html#sect_7.1
			syntax = explicitVRLittleEndian
		}
		if err := writeDataElement(dw, syntax, element); err != nil {
			return fmt.Errorf("writing data element: %v", err)
		}
	}

	return nil
}

func createMetaGroupLengthElement(dataSet *DataSet) (*DataElement, error) {
	// Please refer to the DICOM Standard Part 10 for information on the File Meta Information Group
	// Length. http://dicom.nema.org/medical/dicom/current/output/html/part10.html#sect_7.1

	size := uint32(0)
	for _, tag := range dataSet.SortedTags() {
		if tag == FileMetaInformationGroupLengthTag {
			// The Group Length stores the size of the meta elements following this tag.
			continue
		}
		if !tag.IsMetaElement() {
			break
		}
		element, err := processedElement(dataSet.Elements[tag])
		if err != nil {
			return nil, fmt.Errorf("processing element: %v", err)
		}
		switch element.VR {
		case OBVR, ODVR, OFVR, OLVR, OWVR, SQVR, UCVR, URVR, UTVR, UNVR:
			size += 4 /*tag*/ + 2 /*vr*/ + 2 /*reserved*/ + 4 /*32-bit length*/ + element.ValueLength
		default:
			size += 4 /*tag*/ + 2 /*vr*/ + 2 /*16-bit length*/ + element.ValueLength
		}
	}

	return &DataElement{
		Tag:         FileMetaInformationGroupLengthTag,
		VR:          FileMetaInformationGroupLengthTag.DictionaryVR(),
		ValueField:  []uint32{size},
		ValueLength: 4, // 4bytes = sizeof uint32
	}, nil
}

func findSyntaxFromDataSet(dataSet *DataSet) (transferSyntax, error) {
	syntaxElement, ok := dataSet.Elements[TransferSyntaxUIDTag]
	if !ok {
		return transferSyntax{}, fmt.Errorf("transfer syntax element is missing from data set")
	}

	syntaxUID, err := syntaxElement.StringValue()
	if err != nil {
		return transferSyntax{}, fmt.Errorf("transfer syntax element cannot be converted to string: %v", err)
	}

	return lookupTransferSyntax(syntaxUID), nil
}

func writeDicomSignature(dw *dcmWriter) error {
	if err := dw.Bytes(make([]byte, 128)); err != nil {
		return fmt.Errorf("writing DICOM preamble: %v", err)
	}

	if err := dw.String("DICM"); err != nil {
		return fmt.Errorf("writing DICOM signature: %v", err)
	}

	return nil
}
