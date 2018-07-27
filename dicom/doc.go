// Package dicom provides functions and data structures for manipulating the DICOM file format.
// The package provides a high level and low level API for parsing and writing the DICOM format.
// The high level API consists of functions such as Parse and Construct which by default
// operate on DICOM Data Elements buffered into memory as a DataSet. The low level API
// consists of streaming interfaces like the DataElementIterator and the DataElementWriter which
// do not require buffering and can operate on DataElements one at a time.
//
// The Parse function and the DataElementIterator represent the ValueField of DataElements
// differently. The Parse function by default buffers VRs of potentially enormous size
// (SQ, OX, UN, UT, UR, UC) into memory. In contrast, the DataElementIterator does not buffer these
// VRs and instead represents them as streaming interfaces. This is particularly useful for heavy
// image processing.
package dicom
