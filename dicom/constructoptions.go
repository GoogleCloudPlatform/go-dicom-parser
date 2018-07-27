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

// ConstructOption configures how the Construct function behaves
type ConstructOption struct {
	transform func(element *DataElement) (*DataElement, error)
}

// ConstructOptionWithTransform returns a construct option that applies the given transformation to
// each DataElement before it is written to the DICOM file. For sequence DataElements, the transform
// is applied to the parent DataElement first before being applied to its children
// (i.e. the transform is applied to DataElements in pre-order)
//
// After all the ConstructOptions are applied to a DataElement, the length of the DataElement is
// re-calculated and VRs added from the DICOM data dictionary if the DataElement has a nil VR.
// The length re-calculation will default to explicit lengths unless a DataElement specifies
// undefined length in its ValueLength field.
func ConstructOptionWithTransform(transform func(element *DataElement) (*DataElement, error)) ConstructOption {
	return ConstructOption{transform: transform}
}

// ExplicitLengths ensures all sequences and sequence items are written with explicit length. This
// option should be applied after all other options. This behaviour when used in conjunction with
// UndefinedLengths is undefined.
var ExplicitLengths = ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
	// As specified in ConstructOptionWithTransform, the length of DataElements is re-calculated after
	// all the ConstructOptions are applied. So all we need to do is remove any UndefinedLengths
	// and then everything will be re-calculated as explicit length.
	//
	// We need to fully recurse into the sequence here because DataElements are written in pre-order
	// which causes the length calculation happen on the parent before the children. Any child with
	// undefined length will cause the parent to have undefined length, thus we need to explicitly
	// remove undefined lengths from children through recursion.

	var clearUndefinedLengths func(elem *DataElement)

	clearUndefinedLengths = func(elem *DataElement) {
		if seq, ok := elem.ValueField.(*Sequence); ok {
			elem.ValueLength = 0
			for _, item := range seq.Items {
				item.Length = 0
				for _, itemElem := range item.Elements {
					clearUndefinedLengths(itemElem)
				}
			}
		}
	}

	clearUndefinedLengths(element)

	return element, nil
})

// UndefinedLengths ensures all sequences and sequence items are written with undefined length.
// This option should be applied after all other options. The behaviour when used in conjunction
// with ExplicitLengths is undefined.
var UndefinedLengths = ConstructOptionWithTransform(func(element *DataElement) (*DataElement, error) {
	if seq, ok := element.ValueField.(*Sequence); ok {
		element.ValueLength = UndefinedLength
		for _, item := range seq.Items {
			item.Length = UndefinedLength
		}
	}

	return element, nil
})
