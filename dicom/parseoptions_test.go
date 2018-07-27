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
	"os"
	"path"
	"strings"
	"testing"

	
)

func TestSplitUncompressedPixelDataFrames(t *testing.T) {
	tests := []struct {
		name string
		in   *DataElement
		want *DataElement
	}{
		{
			"a DataElement of zero length does not cause an error",
			&DataElement{BitsAllocatedTag, USVR, nil, 0},
			&DataElement{BitsAllocatedTag, USVR, nil, 0},
		},
		{
			"a Data Element with zero length as an empty slice does not cause an error",
			&DataElement{BitsAllocatedTag, USVR, []uint16{}, 0},
			&DataElement{BitsAllocatedTag, USVR, []uint16{}, 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt := SplitUncompressedPixelDataFrames()
			got, err := opt.transform(tc.in)
			if err != nil {
				t.Fatalf("SplitUncompressedPixelDataFrames.transform(_) => %v", err)
			}
			compareDataElements(got, tc.want, binary.LittleEndian, t)
		})
	}
}

func TestUTF8Text(t *testing.T) {
	order := binary.LittleEndian // TODO cleanup dependency on byte order for element comparison

	shiftJISTerm := "ISO 2022 IR 13"
	shiftJISBytes := []byte{0xA6, 0xC2}
	shiftJISText := string(shiftJISBytes)
	utf8Text := "ｦﾂ"

	tests := []struct {
		name       string
		in         *DataElement
		codingTerm string
		want       *DataElement
	}{
		{
			"when a specific character set is encountered, buffered text is decoded to UTF-8",
			&DataElement{ViewNameTag, SHVR, []string{shiftJISText}, uint32(len(shiftJISText))},
			shiftJISTerm,
			&DataElement{ViewNameTag, SHVR, []string{utf8Text}, uint32(len(shiftJISText))},
		},
		{
			"when a specific character set is encountered, streamed text is decoded to UTF-8",
			&DataElement{LocalNamespaceEntityIDTag, UTVR, createBulkDataIterator(shiftJISBytes), uint32(len(shiftJISText))},
			shiftJISTerm,
			&DataElement{LocalNamespaceEntityIDTag, UTVR, []string{utf8Text}, uint32(len(shiftJISText))},
		},
		{
			"when a specific character set is encountered, non-text buffers are not modified",
			&DataElement{PixelDataTag, OWVR, NewBulkDataBuffer(shiftJISBytes), uint32(len(shiftJISText))},
			shiftJISTerm,
			&DataElement{PixelDataTag, OWVR, NewBulkDataBuffer(shiftJISBytes), uint32(len(shiftJISText))},
		},
		{
			"when a specific character set is encountered, non-text streams are not modified",
			&DataElement{PixelDataTag, OWVR, createBulkDataIterator(shiftJISBytes), uint32(len(shiftJISText))},
			shiftJISTerm,
			&DataElement{PixelDataTag, OWVR, createBulkDataIterator(shiftJISBytes), uint32(len(shiftJISText))},
		},
		{
			"when a specific character set is encountered, some VRs are still interpreted as the default ISO 2022 IR 6",
			&DataElement{ViewNameTag, AEVR, []string{shiftJISText}, uint32(len(shiftJISText))},
			shiftJISTerm,
			&DataElement{ViewNameTag, AEVR, []string{shiftJISText}, uint32(len(shiftJISText))},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			utf8 := UTF8TextOption()

			utf8.transform(createCharacterSetElement(tc.codingTerm))
			utf8.transform(tc.in)

			compareDataElements(tc.in, tc.want, order, t)
		})
	}
}

func TestUTF8TextOption_NoCharacterSetSpecified(t *testing.T) {
	asciiText := "abcd"
	tests := []struct {
		name string
		in   *DataElement
		want *DataElement
	}{
		{
			"when no specific character set is specified, text is not modified",
			&DataElement{ViewNameTag, SHVR, []string{asciiText}, uint32(len(asciiText))},
			&DataElement{ViewNameTag, SHVR, []string{asciiText}, uint32(len(asciiText))},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UTF8TextOption().transform(tc.in)
			if err != nil {
				t.Fatalf("utf8 option transform: %v", err)
			}
			// TODO cleanup dependency on byte order for element comparison
			compareDataElements(got, tc.want, binary.LittleEndian, t)
		})
	}
}

func TestUTF8Text_encodings(t *testing.T) {
	// Please refer to the section the DICOM standard linked below for useful explanation of the
	// character sets
	// http://dicom.nema.org/medical/dicom/current/output/html/part03.html#table_C.12-2
	table := []byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xAA, 0xBB, 0XCC, 0xDD, 0xEE, 0xFF}

	tests := []struct {
		characterSetTerm string
		encoded          []byte
		utf8             string // go source code is always utf-8
	}{
		{
			"ISO_IR 100",
			table,
			"\"3DUfwª»ÌÝîÿ",
		},
		{
			"ISO_IR 101",
			table,
			"\"3DUfwŞťĚÝî˙",
		},
		{
			"ISO_IR 109",
			table,
			"\"3DUfwŞğÌŬî˙",
		},
		{
			"ISO_IR 110",
			table,
			"\"3DUfwĒģĖŨî˙",
		},
		{
			"ISO_IR 144",
			table,
			"\"3DUfwЊЛЬнюџ",
		},
		{
			"ISO_IR 127",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xBB, 0xCC},
			"\"3DUfw؛ج",
		},
		{
			"ISO_IR 126",
			table,
			"\"3DUfwͺ»Μέξ�",
		},
		{
			"ISO_IR 138",
			table,
			"\"3DUfw×»��מ�",
		},
		{
			"ISO_IR 148",
			table,
			"\"3DUfwª»Ìİîÿ",
		},
		{
			"ISO_IR 13",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xAA, 0xBB, 0XCC, 0xDD},
			"\"3DUfwｪｻﾌﾝ",
		},
		{
			"ISO_IR 166",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xAA, 0xBB, 0xCC},
			"\"3DUfwชปฬ",
		},
		{
			"ISO_IR 192",
			[]byte{0xF0, 0x9F, 0x95, 0xB6, 0xF0, 0x9F, 0x92, 0xA9, 0xF0, 0x9F, 0x87, 0xA8, 0xF0, 0x9F, 0x87, 0xA6},
			"🕶💩🇨🇦",
		},
		{
			"GB18030",
			[]byte{0xFE, 0x55, 0x81, 0x30, 0x8A, 0x30},
			"㑳ã",
		},
		{
			"GBK",
			[]byte{0x57, 0x61, 0x6e, 0x67, 0x5e, 0x58, 0x69, 0x61, 0x6f, 0x44, 0x6f, 0x6e, 0x67, 0x3d, 0xcd, 0xf5, 0x5e, 0xd0, 0xa1, 0xb6, 0xab, 0x3d},
			"Wang^XiaoDong=王^小东=",
		},
		{
			"ISO 2022 IR 6",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77},
			"\"3DUfw",
		},
		{
			"ISO 2022 IR 100",
			table,
			"\"3DUfwª»ÌÝîÿ",
		},
		{
			"ISO 2022 IR 101",
			table,
			"\"3DUfwŞťĚÝî˙",
		},
		{
			"ISO 2022 IR 109",
			table,
			"\"3DUfwŞğÌŬî˙",
		},
		{
			"ISO 2022 IR 110",
			table,
			"\"3DUfwĒģĖŨî˙",
		},
		{
			"ISO 2022 IR 144",
			table,
			"\"3DUfwЊЛЬнюџ",
		},
		{
			"ISO 2022 IR 127",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xBB, 0xCC},
			"\"3DUfw؛ج",
		},
		{
			"ISO 2022 IR 126",
			table,
			"\"3DUfwͺ»Μέξ�",
		},
		{
			"ISO 2022 IR 138",
			table,
			"\"3DUfw×»��מ�",
		},
		{
			"ISO 2022 IR 148",
			table,
			"\"3DUfwª»Ìİîÿ",
		},
		{
			"ISO 2022 IR 13\\ISO 2022 IR 87",
			[]byte("\324\317\300\336^\300\333\263=\033$B;3ED\033(J^\033$BB@O:\033(J=\033$B$d$^$@\033(J^\033$B$?$m$&\033(J"),
			"ﾔﾏﾀﾞ^ﾀﾛｳ=山田^太郎=やまだ^たろう",
		},
		{
			"ISO 2022 IR 166",
			[]byte{0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0xAA, 0xBB, 0xCC},
			"\"3DUfwชปฬ",
		},
		{
			"\\ISO 2022 IR 87",
			[]byte("Yamada^Tarou=\033$B;3ED\033(B^\033$BB@O:\033(B=\033$B$d$^$@\033(B^\033$B$?$m$&\033(B"),
			"Yamada^Tarou=山田^太郎=やまだ^たろう",
		},
		{
			"\\ISO 2022 IR 149",
			[]byte("Hong^Gildong=\033$)C\373\363^\033$)C\321\316\324\327=\033$)C\310\253^\033$)C\261\346\265\277"),
			"Hong^Gildong=洪^吉洞=홍^길동",
		},
		{
			"ISO 2022 IR 6\\ISO 2022 IR 6\\ISO 2022 IR 6\\ISO 2022 IR 6",
			[]byte("Joe^Smith"),
			"Joe^Smith",
		},
		{
			"ISO 2022 IR 6",
			[]byte("Joe^Smith=Joe^Smith=Joe^Smith=Joe^Smith"),
			"Joe^Smith=Joe^Smith=Joe^Smith=Joe^Smith",
		},
	}

	for _, tc := range tests {
		t.Run(tc.characterSetTerm, func(t *testing.T) {
			opt := UTF8TextOption()
			characterSetElement := createCharacterSetElement(tc.characterSetTerm)
			opt.transform(characterSetElement)

			in := &DataElement{ViewNameTag, PNVR, []string{string(tc.encoded)}, uint32(len(tc.encoded))}
			want := &DataElement{ViewNameTag, PNVR, []string{tc.utf8}, uint32(len(tc.encoded))}
			got, err := opt.transform(in)
			if err != nil {
				t.Fatalf("transform(_) => %v", err)
			}

			compareDataElements(got, want, binary.LittleEndian, t)
		})
	}
}

func TestReferenceBulkData(t *testing.T) {
	length := uint32(len(sampleBytes))
	refs := []BulkDataReference{{ByteRegion{0, int64(length)}}}
	tests := []struct {
		name  string
		in    *DataElement
		order binary.ByteOrder
		want  *DataElement
	}{
		{
			"when not bulk data, ValueField is of a buffered type",
			&DataElement{FileMetaInformationVersionTag, OBVR, createBulkDataIterator(sampleBytes), length},
			binary.LittleEndian,
			&DataElement{FileMetaInformationVersionTag, OBVR, NewBulkDataBuffer(sampleBytes), length},
		},
		{
			"when bulk data, ValueField is of type []ByteFragmentReference",
			&DataElement{PixelDataTag, OBVR, createBulkDataIterator(sampleBytes), length},
			binary.LittleEndian,
			&DataElement{PixelDataTag, OBVR, refs, length},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReferenceBulkData(DefaultBulkDataDefinition).transform(tc.in)
			if err != nil {
				t.Fatalf("ReferenceBulkData.Apply(_) => (_, %v)", err)
			}
			compareDataElements(got, tc.want, tc.order, t)
		})
	}
}

func TestDropGroupLengths(t *testing.T) {
	tests := []struct {
		name string
		in   *DataElement
		want *DataElement
	}{
		{
			"a group length element is filtered",
			&DataElement{0x00020000, OBVR, []byte{}, 0},
			nil,
		},
		{
			"non-group length elements are not filtered",
			&DataElement{0x00020001, ULVR, []uint32{}, 0},
			&DataElement{0x00020001, ULVR, []uint32{}, 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DropGroupLengths.transform(tc.in)
			if err != nil {
				t.Fatalf("DropGroupLengths.transform(%v) => %v", tc.in, err)
			}
			compareDataElements(got, tc.want, binary.LittleEndian, t)
		})
	}
}

func TestDropBasicOffsetTable(t *testing.T) {
	tests := []struct {
		name string
		in   *DataElement
		want *DataElement
	}{
		{
			"the offset table is dropped for the encapsulated format",
			&DataElement{PixelDataTag, OBVR, encapsulatedFormatIterFromFragments(true, sampleBytes), UndefinedLength},
			&DataElement{PixelDataTag, OBVR, encapsulatedFormatBuffer([][]byte{sampleBytes}), UndefinedLength},
		},
		{
			"pixel data of non-encapsulated formats are not modified",
			&DataElement{PixelDataTag, OBVR, oneShotIteratorFromBytes(sampleBytes), uint32(len(sampleBytes))},
			&DataElement{PixelDataTag, OBVR, oneShotIteratorFromBytes(sampleBytes), uint32(len(sampleBytes))},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DropBasicOffsetTable.transform(tc.in)
			if err != nil {
				t.Fatalf("DropBasicOffsetTable.Transform(_) => %v", err)
			}
			compareDataElements(got, tc.want, binary.LittleEndian, t)
		})
	}
}

func TestNativeMultiFrame_Next_discardsPreviouslyReturnedFragments(t *testing.T) {
	data := []byte{
		1, 2, 3, 4, 5, // frame1
		6, 7, 8, 9, 10, // frame2
		0, 0, // trailing nulls
	}
	iter := createBulkDataIterator(data)
	frames, err := newNativeMultiFrame(iter, 5, 2)
	if err != nil {
		t.Fatalf("newNativeMultiFrame: %v", err)
	}

	frame1, err := frames.Next()
	if err != nil {
		t.Fatalf("getting first frame: %v", err)
	}
	frame1.Read([]byte{1, 2}) // read 2 bytes of 5 byte frame

	frame2, err := frames.Next()
	frame2Buff, err := ioutil.ReadAll(frame2)
	if err != nil {
		t.Fatalf("buffering frame2 :%v", err)
	}

	if !bytes.Equal(frame2Buff, data[5:10]) {
		t.Fatalf("frame 2 is corrupted. Got %v, want %v", frame2Buff, data[5:10])
	}
}

func TestNativeMultiFrame_Close(t *testing.T) {
	frames, err := newNativeMultiFrame(createBulkDataIterator(sampleBytes), int64(len(sampleBytes)), 1)
	if err != nil {
		t.Fatalf("newMultiFrame: %v", err)
	}
	frames.Close()

	if _, err := frames.Next(); err != io.EOF {
		t.Fatalf("expected Close to discard frames in the iterator")
	}
}

func TestNewNativeMultiFrame_frameLengthZeroBanned(t *testing.T) {
	_, err := newNativeMultiFrame(createBulkDataIterator(sampleBytes), 0, int64(len(sampleBytes)))
	if err == nil {
		t.Fatalf("expected error to be returned")
	}
}

func ExampleParseOption() {
	p := path.Join("../", "testdata/"+"ImplicitVRLittleEndian.dcm")
	r, err := os.Open(p)
	if err != nil {
		fmt.Println(err)
		return
	}

	excludeFileMetaElements := ParseOptionWithTransform(func(element *DataElement) (*DataElement, error) {
		if element.Tag.GroupNumber() == 0x0002 {
			return nil, nil // exclude meta element by transforming it to nil
		}
		return element, nil
	})

	dataSet, err := Parse(r, excludeFileMetaElements)

	fileMetaElementCount := 0
	for _, element := range dataSet.Elements {
		if element.Tag.GroupNumber() == 0x0002 {
			fileMetaElementCount++
		}
	}

	fmt.Println("There are", fileMetaElementCount, "file meta elements in the returned data set.")
	// Output: There are 0 file meta elements in the returned data set.
}

func createCharacterSetElement(term string) *DataElement {
	return &DataElement{SpecificCharacterSetTag, SpecificCharacterSetTag.DictionaryVR(), strings.Split(term, "\\"), uint32(len(term))}
}

func createBulkDataIterator(b []byte) BulkDataIterator {
	return NewBulkDataIterator(bytes.NewReader(b), 0)
}
