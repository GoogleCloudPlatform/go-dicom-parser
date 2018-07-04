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

// Transform describes a transformation applied to a DataElement
type Transform func(*DataElement) (*DataElement, error)

// ParseOption configures the behavior of the Parse function.
type ParseOption struct {
	transform Transform
}

// WithTransform returns a ParseOption that applies the given transformation to each DataElement in
// the DICOM file in the order encountered. For DataElements that contain a sequence, the transform
// is applied to nested DataElements first (i.e. transform is called on DataElements in post-order).
// If the transform returns an error, Parse will stop parsing and return an error.
// If no error is returned and a non-nil DataElement is returned, this DataElement will be added to
// the returned DataSet of Parse. If a nil DataElement is returned, this DataElement will be
// excluded from the DataSet returned from Parse.
func WithTransform(t Transform) ParseOption {
	return ParseOption{t}
}

// ReferenceBulkData ensures that all DataElements with ValueField of type BulkDataIterator are
// transformed to []BulkDataReference when bulkDataDefinition returns true and their default
// buffered types otherwise
func ReferenceBulkData(bulkDataDefinition func(*DataElement) bool) ParseOption {
	return WithTransform(func(element *DataElement) (*DataElement, error) {
		return referenceBulkData(element, bulkDataDefinition)
	})
}

// DropGroupLengths will exclude all group length elements (gggg,0000) from the returned DataSet
var DropGroupLengths = WithTransform(func(element *DataElement) (*DataElement, error) {
	if element.Tag.ElementNumber() == 0 {
		return nil, nil
	}
	return element, nil
})

// DropBasicOffsetTable will exclude the basic offset table fragment from pixel data encoded using
// the encapsulated (compressed) format. For more information on the offset table and encapsulated
// formats please see http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
var DropBasicOffsetTable = WithTransform(func(element *DataElement) (*DataElement, error) {
	if iter, ok := element.ValueField.(*EncapsulatedFormatIterator); ok && element.Tag == PixelDataTag {
		if _, err := iter.Next(); err != nil {
			return nil, fmt.Errorf("discarding offset table: %v", err)
		}
	}
	return element, nil
})

// DefaultBulkDataDefinition returns true if and only if the tag corresponds to a data element
// contains that contains large non-metadata fields
func DefaultBulkDataDefinition(elem *DataElement) bool {
	// Tags in the DICOM data dictionary have wildcards (e.g. tags like (gggg,eexx), (ggxx,eeee))
	// The tag library stores the value of the tag with the x's set to '0' in hex.
	// For example the Curve Data tag is defined as (50xx,3000). The variable
	// CurveDataTag = 0x50003000. So we can check if a given tag is of the form (50xx,3000) from
	// the condition (tag & 0xFF00FFFF) == CurveDataTag.
	//
	// The following list of masks handles all wildcards in the DICOM data dictionary. The value
	// 0xFFFFFFFF is included in the list of masks for convenience since
	// (tag & 0xFFFFFFFF) == tag
	for _, m := range []uint32{0xFFFFFF00, 0xFFFFFF0F, 0xFFFF000F, 0xFFFF0000, 0xFF00FFFF, 0xFFFFFFFF} {
		// TODO add this mask logic to tag library once implemented
		switch uint32(elem.Tag) & m {
		case PixelDataProviderURLTag, AudioSampleDataTag, CurveDataTag, SpectroscopyDataTag,
			OverlayDataTag, EncapsulatedDocumentTag, FloatPixelDataTag, DoubleFloatPixelDataTag,
			PixelDataTag, WaveformDataTag:
			return true
		}
	}
	return false
}

func referenceBulkData(element *DataElement, isBulkData func(*DataElement) bool) (*DataElement, error) {
	if isBulkData(element) {
		if bulkIter, ok := element.ValueField.(BulkDataIterator); ok {
			refs, err := CollectFragmentReferences(bulkIter)
			if err != nil {
				return nil, fmt.Errorf("collecting fragment references: %v", err)
			}
			element.ValueField = refs
		}
		return element, nil
	}
	return element, nil
}
