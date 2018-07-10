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
	"io/ioutil"

	"golang.org/x/text/encoding"
)

// ParseOption configures the behavior of the Parse function.
type ParseOption struct {
	transform func(*DataElement) (*DataElement, error)
}

// WithTransform returns a ParseOption that applies the given transformation to each DataElement in
// the DICOM file in the order encountered. For DataElements that contain a sequence, the transform
// is applied to nested DataElements first (i.e. transform is called on DataElements in post-order).
// If the transform returns an error, Parse will stop parsing and return an error.
// If no error is returned and a non-nil DataElement is returned, this DataElement will be added to
// the returned DataSet of Parse. If a nil DataElement is returned, this DataElement will be
// excluded from the DataSet returned from Parse.
func WithTransform(t func(*DataElement) (*DataElement, error)) ParseOption {
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
// formats please see http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4.
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
	return uint32(elem.Tag) == PixelDataTag
}

// SplitUncompressedPixelDataFrames returns an option that ensures Data Elements with
// uncompressed pixel data (7FE0,0010) respect the image pixel module tags if present.
// i.e. If a DataElement corresponds to pixel data (7FE0,0010) each element of the ValueField slice
// (type [][]byte or []BulkDataReference) will represent an image frame. If ValueField is a
// BulkDataIterator, each BulkDataReader within the iterator will be an image frame. If image module
// tags are encountered that do not conform to the Image Pixel Module IOD linked below, the pixel
// data will be excluded from the returned DataSet.
// http://dicom.nema.org/medical/dicom/current/output/chtml/part03/sect_C.7.6.3.html
//
// Note this option should be applied before all other options that modify pixel data.
func SplitUncompressedPixelDataFrames() ParseOption {
	metadata := map[DataElementTag]int64{
		RowsTag:            0,
		ColumnsTag:         0,
		SamplesPerPixelTag: 0,
		BitsAllocatedTag:   0,
		NumberOfFramesTag:  0,
	}

	return WithTransform(func(element *DataElement) (*DataElement, error) {
		if _, ok := metadata[element.Tag]; ok {
			v, err := element.IntValue()
			if err != nil {
				return nil, fmt.Errorf("tag %v can't be converted to int value", element.Tag)
			}
			metadata[element.Tag] = v
		}

		if element.Tag == PixelDataTag {
			return toMultiFrame(element, metadata)
		}

		return element, nil
	})
}

func toMultiFrame(element *DataElement, metadata map[DataElementTag]int64) (*DataElement, error) {
	if _, ok := element.ValueField.(*EncapsulatedFormatIterator); ok {
		// as specified, the SplitUncompressedPixelDataFrames does not do anything to compressed images
		return element, nil
	}

	if metadata[BitsAllocatedTag]%8 != 0 {
		// BitsAllocated must be a multiple of 8 or 1 as specified in PS3.5
		// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_8.1.1
		// TODO support BitsAllocated=1
		return nil, nil
	}

	frameLength := (metadata[RowsTag] * metadata[ColumnsTag] * metadata[SamplesPerPixelTag] * metadata[BitsAllocatedTag]) / 8
	if frameLength <= 0 {
		return nil, nil
	}

	numberOfFrames := metadata[NumberOfFramesTag]
	if numberOfFrames <= 0 {
		numberOfFrames = 1
	}

	if bulkData, ok := element.ValueField.(BulkDataIterator); ok {
		multiFrame, err := newNativeMultiFrame(bulkData, frameLength, numberOfFrames)
		if err != nil {
			return nil, err
		}
		element.ValueField = multiFrame
	}
	return element, nil
}

type nativeMultiFrame struct {
	underlyingFragment *BulkDataReader
	frameLength        int64
	numberOfFrames     int64
	framesRead         int64
	currentFrame       *BulkDataReader
}

func newNativeMultiFrame(iter BulkDataIterator, frameLength, numberOfFrames int64) (BulkDataIterator, error) {
	if frameLength <= 0 {
		return nil, fmt.Errorf("invalid frame length: %v", frameLength)
	}

	r, err := iter.Next()
	if err != nil {
		return nil, fmt.Errorf("retreiving image fragment: %v", err)
	}
	if _, err := iter.Next(); err != io.EOF {
		return nil, fmt.Errorf("internal error: cannot convert multiple fragments to native multi-frame")
	}

	return &nativeMultiFrame{r, frameLength, numberOfFrames, 0, nil}, nil
}

func (it *nativeMultiFrame) Next() (*BulkDataReader, error) {
	if it.framesRead >= it.numberOfFrames {
		// This handles the case when there are trailing nulls remaining after all image frames.
		io.Copy(ioutil.Discard, it.underlyingFragment)
		return nil, io.EOF
	}
	if it.currentFrame != nil {
		if err := it.currentFrame.Close(); err != nil {
			return nil, fmt.Errorf("discarding previous frame: %v", err)
		}
	}

	frameBytes := io.LimitReader(it.underlyingFragment, it.frameLength)
	frameOffset := it.underlyingFragment.Offset + (it.framesRead * it.frameLength)

	it.currentFrame = &BulkDataReader{frameBytes, frameOffset}

	it.framesRead++

	return it.currentFrame, nil
}

func (it *nativeMultiFrame) Close() error {
	for _, err := it.Next(); err != io.EOF; _, err = it.Next() {
		if err != nil {
			return fmt.Errorf("discarding frame: %v", err)
		}
	}
	return nil
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

// UTF8TextOption returns an option that ensures all textual VRs are decoded into UTF-8.
func UTF8TextOption() ParseOption {
	dataSetEncoding := defaultCharacterRepertoire

	return WithTransform(func(element *DataElement) (*DataElement, error) {
		if element.Tag == SpecificCharacterSetTag {
			coding, err := findEncodingFromElement(element)
			if err != nil {
				return nil, fmt.Errorf("finding encoding from element: %v", err)
			}
			dataSetEncoding = coding
		}

		return toUTF8(element, dataSetEncoding)
	})
}

func findEncodingFromElement(element *DataElement) (encoding.Encoding, error) {
	s, ok := element.ValueField.([]string)
	if !ok {
		return nil, fmt.Errorf("unexpected character set type %T (expected string array)", element.ValueField)
	}
	if len(s) <= 0 {
		// As specified above table C.12-4, if the value of (0008,0005) is empty, it is assumed that
		// value 1 is ISO 2022 IR 6.
		// http://dicom.nema.org/medical/dicom/current/output/html/part03.html#table_C.12-4
		return defaultCharacterRepertoire, nil
	}

	return lookupEncoding(s[0])
}

func toUTF8(element *DataElement, coding encoding.Encoding) (*DataElement, error) {
	replaceableCharacterRepertoires := map[*VR]bool{
		SHVR: true,
		LOVR: true,
		STVR: true,
		LTVR: true,
		PNVR: true,
		UCVR: true,
		UTVR: true,
	}
	if !replaceableCharacterRepertoires[element.VR] {
		// Some VRs cannot have their character repertoires replaced.
		// If the character repertoire is not replaceable, return the element unmodified. Refer to
		// part5 of the DICOM standard for more information on character repertoire replacements:
		// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#chapter_6.1.2.3
		return element, nil
	}

	decoder := coding.NewDecoder()

	if s, ok := element.ValueField.([]string); ok {
		// Small text fields are buffered into memory by default and need to be decoded.
		for i := range s {
			decoded, err := decoder.String(s[i])
			if err != nil {
				continue // If decoding fails for any reason, just leave the string unmodified.
			}
			s[i] = decoded
		}
	}

	if bulkData, ok := element.ValueField.(BulkDataIterator); ok {
		// Large text fields are streamed by default and need to be decoded.
		bulkDataReader, err := bulkData.Next()
		if err != nil {
			return nil, fmt.Errorf("converting text fragment to UTF-8: %v", err)
		}
		if _, err := bulkData.Next(); err != io.EOF {
			return nil, fmt.Errorf("converting multi-fragment text to UTF-8 not supported")
		}

		utf8Reader := &countReader{decoder.Reader(bulkDataReader), bulkDataReader.Offset}

		element.ValueField = newOneShotIterator(utf8Reader)
	}

	return element, nil
}
