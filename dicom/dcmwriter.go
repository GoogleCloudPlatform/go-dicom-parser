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
)

type dcmWriter struct {
	io.Writer
}

func (dw *dcmWriter) Tag(order binary.ByteOrder, tag DataElementTag) error {
	if err := dw.UInt16(order, tag.GroupNumber()); err != nil {
		return err
	}
	return dw.UInt16(order, tag.ElementNumber())
}

func (dw *dcmWriter) Delimiter(order binary.ByteOrder, tag DataElementTag) error {
	if err := dw.Tag(order, tag); err != nil {
		return fmt.Errorf("writing delimiter tag: %v", err)
	}
	if err := dw.UInt32(order, 0); err != nil {
		return fmt.Errorf("writing item length of delimiter: %v", err)
	}
	return nil
}

func (dw *dcmWriter) UInt16(order binary.ByteOrder, v uint16) error {
	buf := make([]byte, 2)
	order.PutUint16(buf, v)
	return dw.Bytes(buf)
}

func (dw *dcmWriter) UInt32(order binary.ByteOrder, v uint32) error {
	buf := make([]byte, 4)
	order.PutUint32(buf, v)
	return dw.Bytes(buf)
}

func (dw *dcmWriter) String(s string) error {
	_, err := dw.Write([]byte(s))
	return err
}

func (dw *dcmWriter) Bytes(b []byte) error {
	_, err := dw.Write(b)
	return err
}
