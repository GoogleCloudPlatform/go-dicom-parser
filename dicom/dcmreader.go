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
	"fmt"
	"io"
	"io/ioutil"
)

// dcmReader is a wrapper around io.Reader, providing convenience methods for
// parsing tags, numbers, strings
type dcmReader struct {
	cr *countReader
}

func newDcmReader(r io.Reader) *dcmReader {
	return &dcmReader{&countReader{r, 0}}
}

func (dr *dcmReader) Tag(order binary.ByteOrder) (DataElementTag, error) {
	group, err := dr.UInt16(order)
	if err != nil {
		return 0, err
	}
	element, err := dr.UInt16(order)
	if err != nil {
		return 0, err
	}

	return DataElementTag(uint32(group)<<16 | uint32(element)), nil
}

// Limit returns a dcmReader that shares the same underlying io.Reader that returns
// EOF after reading n bytes.
func (dr *dcmReader) Limit(n int64) *dcmReader {
	return &dcmReader{limitCountReader(dr.cr, n)}
}

// Skip advances the input stream by n bytes
func (dr *dcmReader) Skip(n int64) error {
	_, err := io.CopyN(ioutil.Discard, dr.cr, int64(n))
	return err
}

// String returns a string of length n from the input stream
func (dr *dcmReader) String(n int64) (string, error) {
	b, err := dr.Bytes(n)
	return string(b), err
}

// Bytes returns a byte array of size n from the input stream
func (dr *dcmReader) Bytes(n int64) ([]byte, error) {
	b := make([]byte, n)
	gotN, err := io.ReadAtLeast(dr.cr, b, int(n))
	if err != nil && gotN != int(n) {
		return nil, fmt.Errorf("internal error: expected ReadAtLeast to return %d bytes but got %d", n, gotN)
	}
	return b, err
}

// UInt32 returns a uint32 from the input stream
func (dr *dcmReader) UInt32(byteOrder binary.ByteOrder) (uint32, error) {
	var b uint32
	err := binary.Read(dr.cr, byteOrder, &b)
	return b, err
}

// UInt16 returns a uint16 from the input stream
func (dr *dcmReader) UInt16(byteOrder binary.ByteOrder) (uint16, error) {
	var b uint16
	err := binary.Read(dr.cr, byteOrder, &b)
	return b, err
}

// countReader is an io.Reader that counts how many bytes read
type countReader struct {
	r         io.Reader
	bytesRead int64 // number of bytes read
}

func (cr *countReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.bytesRead += int64(n)
	return n, err
}

// limitCountReader returns a *countReader that reads from cr and stops with EOF after reading
// n bytes (or cr reaches EOF). The returned *countReader has a starting bytesRead equal to the
// current bytesRead of cr. Since the returned *countReader reads from cr, cr's bytesRead will be
// updated as the returned *countReader reads bytes.
func limitCountReader(cr *countReader, n int64) *countReader {
	return &countReader{io.LimitReader(cr, n), cr.bytesRead}
}
