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
	"strings"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding"
)

var defaultCharacterRepertoire = &namedEncoding{charmap.Windows1252, "windows-1252"}

// lookupLabelByTerm is a mapping of specific character set defined terms to golang charset labels.
// See link below for list of character set defined terms.
// http://dicom.nema.org/medical/dicom/current/output/chtml/part02/sect_D.6.2.html
var lookupLabelByTerm = map[string]string{
	"ISO_IR 100": "iso-ir-100",
	"ISO_IR 101": "iso-ir-101",
	"ISO_IR 109": "iso-ir-109",
	"ISO_IR 110": "iso-ir-110",
	"ISO_IR 144": "iso-ir-144",
	"ISO_IR 127": "iso-ir-127",
	"ISO_IR 126": "iso-ir-126",
	"ISO_IR 138": "iso-ir-138",
	"ISO_IR 148": "iso-ir-148",
	"ISO_IR 13":  "shift-jis",
	"ISO_IR 166": "tis-620",
	"ISO_IR 192": "utf-8",
	"GB18030":    "gb18030",
	"GBK":        "gbk",
	// TODO properly support ISO 2022 extensions)
	"ISO 2022 IR 6":   "us-ascii",
	"":                "us-ascii", // empty value maps to default character repertoire in DICOM standard
	"ISO 2022 IR 100": "iso-ir-100",
	"ISO 2022 IR 101": "iso-ir-101",
	"ISO 2022 IR 109": "iso-ir-109",
	"ISO 2022 IR 110": "iso-ir-110",
	"ISO 2022 IR 144": "iso-ir-144",
	"ISO 2022 IR 127": "iso-ir-127",
	"ISO 2022 IR 126": "iso-ir-126",
	"ISO 2022 IR 138": "iso-ir-138",
	"ISO 2022 IR 148": "iso-ir-148",
	"ISO 2022 IR 13":  "shift-jis",
	"ISO 2022 IR 166": "tis-620",
	"ISO 2022 IR 87":  "iso-2022-jp",
	"ISO 2022 IR 159": "iso-2022-jp",
	"ISO 2022 IR 149": "euc-kr",
}

type namedEncoding struct {
	encoding.Encoding
	canonicalName string
}

func lookupEncoding(term string) (*namedEncoding, error) {
	label, ok := lookupLabelByTerm[term]
	if !ok {
		return nil, fmt.Errorf("specific character set defined term not found: %v", term)
	}

	coding, canonicalName := charset.Lookup(label)
	if coding == nil {
		return nil, fmt.Errorf("missing encoding for label %q", label)
	}
	return &namedEncoding{Encoding: coding, canonicalName: canonicalName}, nil
}

// encodingSystem is a utility for decoding textual data elements to UTF-8. The encoding of data
// elements is defined by the potentially multi-valued specific character set (0008,0005) element.
// For more information on the specific character set please refer to the part3 of the standard:
// http://dicom.nema.org/medical/dicom/current/output/html/part03.html#sect_C.12.1.1.2
// The person name (PN) VR is the only consumer of the multiple encodings:
// http://dicom.nema.org/medical/dicom/current/output/html/part05.html#sect_6.2.1.2
type encodingSystem struct {
	// encodings loosely represents the values of the specific character set (0008,0005) with the
	// first, second, and third value of the slice representing the alphabetic, ideographic, and
	// phonetic specific character sets as defined in person name (PN) section linked above.
	// It is assumed all elements in the slice are non-nil since a constructor newEncodingSystem is
	// provided.
	encodings [3]*namedEncoding
}

func defaultEncodingSystem() *encodingSystem {
	return &encodingSystem{
		encodings: [3]*namedEncoding{
			defaultCharacterRepertoire,
			defaultCharacterRepertoire,
			defaultCharacterRepertoire,
		},
	}
}

func newEncodingSystem(characterSet *DataElement) (*encodingSystem, error) {
	codingSystem := defaultEncodingSystem()

	if characterSet.Tag != SpecificCharacterSetTag {
		return nil, fmt.Errorf("wrong specific character set tag: %v (expected %v)", characterSet.Tag, SpecificCharacterSetTag)
	}

	encodingTerms, ok := characterSet.ValueField.([]string)
	if !ok {
		// As specified above table C.12-4, if the value of (0008,0005) is empty, it is assumed that
		// value 1 is ISO 2022 IR 6.
		// http://dicom.nema.org/medical/dicom/current/output/html/part03.html#table_C.12-4
		return codingSystem, nil
	}

	for i, term := range encodingTerms {
		coding, err := lookupEncoding(term)
		if err != nil {
			return nil, err
		}
		if i >= len(codingSystem.encodings) {
			break
		}
		codingSystem.encodings[i] = coding
	}

	// re-arrange slice to make decode code simpler
	if len(encodingTerms) == 1 {
		codingSystem.encodings[1] = codingSystem.encodings[0]
		codingSystem.encodings[2] = codingSystem.encodings[0]
	} else if len(encodingTerms) == 2 {
		codingSystem.encodings[2] = codingSystem.encodings[1]
	}

	return codingSystem, nil
}

func (c *encodingSystem) decode(element *DataElement) (*DataElement, error) {
	switch element.VR {
	case PNVR:
		personName, ok := element.ValueField.([]string)
		if !ok {
			return nil, fmt.Errorf("wrong value field type %T for person name (expected string array)", element.ValueField)
		}
		if len(personName) <= 0 {
			return element, nil
		}

		componentGroups := strings.Split(personName[0], "=")
		for i, group := range componentGroups {
			if i >= len(c.encodings) {
				break
			}
			componentGroups[i] = decodeString(group, c.encodings[i])
		}

		element.ValueField = []string{strings.Join(componentGroups, "=")}
	case SHVR, LOVR, STVR, LTVR, UCVR, UTVR:
		coding := c.encodings[0]

		if s, ok := element.ValueField.([]string); ok {
			// Small text fields are buffered into memory by default and need to be decoded.
			for i := range s {
				s[i] = decodeString(s[i], coding)
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

			utf8Reader := &countReader{coding.NewDecoder().Reader(bulkDataReader), bulkDataReader.Offset}

			// TODO ParseOptions in should not depend on unexported methods/types)
			element.ValueField = newOneShotIterator(utf8Reader)
		}
	}

	return element, nil
}

func decodeString(s string, coding *namedEncoding) string {
	decoded, err := coding.NewDecoder().String(s)
	if err != nil {
		// If decoding fails for some reason fallback to non-decode string instead of failing to Parse.
		// TODO find a good way of reporting warnings/errors
		return s
	}

	if coding.canonicalName == "euc-kr" {
		// Unfortunately, the go charset library does not support the ISO 2022 escape sequence to the
		// GR version of KS X 1001. We handle this by decoding the string as EUC-KR and then removing
		// the ISO 2022 escape sequence. This approach is similar to pydicom.
		decoded = strings.Replace(decoded, "\x1B\x24\x29\x43", "", -1)
	}

	return decoded
}
