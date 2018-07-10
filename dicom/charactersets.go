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

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding"
)

var defaultCharacterRepertoire encoding.Encoding = charmap.Windows1252

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
	"GB18030": "gb18030",
	"GBK": "gbk",
	// TODO properly support ISO 2022 extensions)
	"ISO 2022 IR 6":   "us-ascii",
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
	// TODO properly support multi-byte encodings with code extensions
	"ISO 2022 IR 87":  "iso-2022-jp",
	"ISO 2022 IR 159": "iso-2022-jp",
	"ISO 2022 IR 149": "iso-ir-149",
}

func lookupEncoding(term string) (encoding.Encoding, error) {
	label, ok := lookupLabelByTerm[term]
	if !ok {
		return nil, fmt.Errorf("specific character set defined term not found: %v", term)
	}

	coding, _ := charset.Lookup(label)
	if coding == nil {
		return nil, fmt.Errorf("missing encoding for label %q", label)
	}
	return coding, nil
}
