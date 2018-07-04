package dicom

import "golang.org/x/text/encoding"

// dicomMetaData represents information about how objects within the DICOM file are stored
type dicomMetaData struct {
	syntax   transferSyntax
	encoding encoding.Encoding
}

var defaultEncoding = dicomMetaData{explicitVRLittleEndian, isoIR6}

