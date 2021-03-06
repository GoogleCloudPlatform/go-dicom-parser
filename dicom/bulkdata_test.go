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
	"io"
	"io/ioutil"
	"reflect"
	"testing"
)

func TestNewBulkDataBuffer_offsets(t *testing.T) {
	offset := int64(5)
	iter := NewBulkDataIterator(bytes.NewBuffer(sampleBytes), offset)
	r, err := iter.Next()
	if err != nil {
		t.Fatalf("BulkDataIterator.Next => %v", err)
	}
	if got := r.Offset; got != offset {
		t.Fatalf("got %v, want %v", got, offset)
	}
}

func TestNewEncapsulatedFormatIterator_offsets(t *testing.T) {
	offset := int64(10)
	fragments := [][]byte{{2, 3}, {4, 5, 6, 7}}
	valueField := encapsulatedFormatBytes(false, fragments...)
	iter := NewEncapsulatedFormatIterator(bytes.NewReader(valueField), offset)

	wantOffset := int64(offset)

	for fragment, err := iter.Next(); err != io.EOF; fragment, err = iter.Next() {
		if err != nil {
			t.Fatalf("Next on iterator: %v", err)
		}

		wantOffset += 4 /*item tag*/ + 4 /*item length*/

		if got := fragment.Offset; got != wantOffset {
			t.Fatalf("got %v, want %v", got, wantOffset)
		}

		fragmentLength, err := io.Copy(ioutil.Discard, fragment)
		if err != nil {
			t.Fatalf("reading fragment: %v", err)
		}

		wantOffset += fragmentLength
	}
}

func TestOneShotIterator_Next(t *testing.T) {
	iter := oneShotIteratorFromBytes(sampleBytes)
	r, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error getting first bulk data: %v", err)
	}

	data, err := ioutil.ReadAll(r)
	if !bytes.Equal(data, sampleBytes) {
		t.Fatalf("got %v, want %v", data, sampleBytes)
	}
}

func TestOneShotIterator_Next_EOF(t *testing.T) {
	iter := oneShotIteratorFromBytes(sampleBytes)
	_, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error getting first bulk data: %v", err)
	}

	if _, err := iter.Next(); err != io.EOF {
		t.Fatalf("expected iterator to return EOF after first Next call: got %v want %v", err, io.EOF)
	}
}

func TestOneShotIterator_Close(t *testing.T) {
	iter := oneShotIteratorFromBytes(sampleBytes)
	if err := iter.Close(); err != nil {
		t.Fatalf("unexpected error getting first bulk data: %v", err)
	}

	if _, err := iter.Next(); err != io.EOF {
		t.Fatalf("expected Close to empty iterator. got %v, want %v", err, io.EOF)
	}
}

func TestOneShotIterator_CloseAfterNext(t *testing.T) {
	iter := oneShotIteratorFromBytes(sampleBytes)
	r, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error getting first bulk data: %v", err)
	}

	if err := iter.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}

	data, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error reading first bulk data: %v", err)
	}

	if len(data) > 0 {
		t.Fatalf("expected Close to discard bytes in io.Reader returned from call to Next")
	}
}

func TestOneShotIterator_ToBuffer(t *testing.T) {
	iter := oneShotIteratorFromBytes(sampleBytes)
	b, err := iter.ToBuffer()
	if err != nil {
		t.Fatalf("toBuffer: %v", err)
	}
	want := bytesValue([][]byte{sampleBytes})
	if !reflect.DeepEqual(b, want) {
		t.Fatalf("got %v, want %v", b, want)
	}
}

func TestEncapsulatedFormatIterator_OffsetTablePresent(t *testing.T) {
	// test behavior of encapsulated pixel data value field as described in
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4
	iter := encapsulatedFormatIterFromFragments(true, sampleBytes)
	r, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error retreiving offset table: %v", err)
	}

	table, err := ioutil.ReadAll(r)
	want := []byte{0, 0, 0, 0}
	if !bytes.Equal(table, want) {
		t.Fatalf("with 1 fragment expected to have 1 32-bit offset value equal to zero. "+
			"got %v, want %v", table, want)
	}
}

func TestEncapsulatedFormatIterator_OffsetTableNotPresent(t *testing.T) {
	iter := encapsulatedFormatIterFromFragments(false, sampleBytes)
	r, err := iter.Next()
	if err != nil {
		t.Fatalf("error getting empty offset table: %v", err)
	}

	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("reading bytes of empty offset table: %v", err)
	}
	if len(b) != 0 {
		t.Fatalf("expected empty offset table to be returned")
	}
}

func TestEncapsulatedFormatIterator_Next_EndsWithEOF(t *testing.T) {
	iter := encapsulatedFormatIterFromFragments(false, sampleBytes)
	iter.Next() // skip offset table
	_, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error getting first bulk data: %v", err)
	}

	if _, err := iter.Next(); err != io.EOF {
		t.Fatalf("got %v, want %v", err, io.EOF)
	}
}

func TestEncapsulatedFormatIterator_Next_MultiFragments(t *testing.T) {
	frag1 := []byte{1, 2, 3, 4}
	frag2 := []byte{5, 6, 7, 8, 9, 10}
	fragments := [][]byte{frag1, frag2}
	iter := encapsulatedFormatIterFromFragments(false, frag1, frag2)

	iter.Next() // skip offset table
	for i := 0; i < 2; i++ {
		frag, err := iter.Next()
		if err != nil {
			t.Fatalf("unexpected error retreiving fragment: %v", err)
		}
		fragBytes, err := ioutil.ReadAll(frag)
		if err != nil {
			t.Fatalf("unexpected error reading fragment: %v", err)
		}
		if !bytes.Equal(fragBytes, fragments[i]) {
			t.Fatalf("wrong fragment data: got %v, want %v", fragBytes, fragments[i])
		}
	}
}

func TestEncapsulatedFormatIterator_Next_PreviousFragmentsInvalidated(t *testing.T) {
	iter := encapsulatedFormatIterFromFragments(false, []byte{0, 1}, []byte{2, 3})
	iter.Next() // skip offset table
	previousFragment, err := iter.Next()
	if err != nil {
		t.Fatalf("unexpected error getting first fragment: %v", err)
	}
	if _, err := iter.Next(); err != nil {
		t.Fatalf("unexpected error getting second fragment: %v", err)
	}
	size, err := io.Copy(ioutil.Discard, previousFragment)
	if err != nil {
		t.Fatalf("unexpected error reading previous fragment: %v", err)
	}
	if size != 0 {
		t.Fatalf("expected previously returned fragment to be emptied after another call to Next")
	}
}

func TestEncapsulatedFormatIterator_Close(t *testing.T) {
	pd := encapsulatedFormatIterFromFragments(false, sampleBytes)
	pd.Next() // skip offset table
	if err := pd.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := pd.Next(); err != io.EOF {
		t.Fatalf("expected %v got %v", io.EOF, err)
	}
}

func TestNewEncapsulatedFormat_ToBuffer(t *testing.T) {
	fragments := [][]byte{{1, 2}, {3, 4}}
	offsetTable := []byte{0x00, 0x00, 0x00, 0x00, 0x0A, 0x00, 0x00, 0x00}

	tests := []struct {
		name string
		in   BulkDataIterator
		want BulkDataBuffer
	}{
		{
			"to buffer with offset table",
			encapsulatedFormatIterFromFragments(true, fragments...),
			NewEncapsulatedFormatBuffer(offsetTable, fragments...),
		},
		{
			"to buffer without offset table",
			encapsulatedFormatIterFromFragments(false, fragments...),
			NewEncapsulatedFormatBuffer([]byte{}, fragments...),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.ToBuffer()
			if err != nil {
				t.Fatalf("ToBuffer: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func encapsulatedFormatIterFromFragments(includeOffsetTable bool, fragments ...[]byte) BulkDataIterator {
	data := encapsulatedFormatBytes(includeOffsetTable, fragments...)
	return NewEncapsulatedFormatIterator(bytes.NewReader(data), 0)
}

func encapsulatedFormatBytes(includeOffsetTable bool, fragments ...[]byte) []byte {
	// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_A.4 for documentation
	// of the offset table and encapsulated format

	data := []byte{0xFE, 0xFF, 0x00, 0xE0} // little endian item tag
	if includeOffsetTable {
		// each fragment offset takes up 32-bit len
		offsetTableLength := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetTableLength, uint32(4*len(fragments)))
		data = append(data, offsetTableLength...)

		offset := uint32(0)
		for _, fragment := range fragments {
			fragmentOffset := make([]byte, 4)
			binary.LittleEndian.PutUint32(fragmentOffset, offset)
			data = append(data, fragmentOffset...)

			offset += 4 /*tag*/ + 4 /*item length*/ + uint32(len(fragment))
		}
	} else {
		// if offset table not present, its item length shall be 0
		data = append(data, []byte{0, 0, 0, 0}...)
	}

	for _, fragmentContent := range fragments {
		itemLength := make([]byte, 4)
		binary.LittleEndian.PutUint32(itemLength, uint32(len(fragmentContent)))

		fragment := []byte{0xFE, 0xFF, 0x00, 0xE0} // start with item tag
		fragment = append(fragment, itemLength...)
		fragment = append(fragment, fragmentContent...)

		data = append(data, fragment...)
	}

	delimiter := []byte{0xFE, 0xFF, 0xDD, 0xE0}
	data = append(data, delimiter...)
	data = append(data, []byte{0, 0, 0, 0}...)

	return data
}

func oneShotIteratorFromBytes(data []byte) BulkDataIterator {
	return NewBulkDataIterator(bytes.NewReader(data), 0)
}
