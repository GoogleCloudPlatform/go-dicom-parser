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

// Construct writes the given *DataSet as a DICOM file to the given io.Writer. The desired transfer
// syntax must be specified as a transfer syntax DataElement (0002,0010) within the *DataSet.
//
// *DataElements that contain nil VRs are filled in from the DICOM Data Dictionary. The ValueLength
// of DataElements are re-calculated to enforce consistency with their ValueFields. The calculation
// will default to explicit length unless a DataElement specifies undefined length.
//
// By default, there is no validation against the DICOM standard of any form.
func Construct(w io.Writer, dataSet *DataSet, opts ...ConstructOption) error {
	writer, err := NewDataElementWriter(w, dataSet.MetaElements(), opts...)
	if err != nil {
		return fmt.Errorf("creating new DataElementWriter: %v", err)
	}

	for _, elem := range dataSet.SortedElements() {
		if elem.Tag.IsMetaElement() {
			continue
		}
		if err := writer.WriteElement(elem); err != nil {
			return fmt.Errorf("writing data element %s: %v", elem.Tag, err)
		}
	}

	return nil
}
