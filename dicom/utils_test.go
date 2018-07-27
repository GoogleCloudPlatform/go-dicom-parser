package dicom

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path"
	"reflect"
	"testing"

	
)

var (
	minimalDataSet = NewDataSet(map[DataElementTag]interface{}{
		TransferSyntaxUIDTag: []string{ExplicitVRLittleEndianUID},
	})
	nestedDataSetElement1 = &DataElement{ReferencedSOPInstanceUIDTag, UIVR, []string{"1.2.840.10008.5.1.4.1.1.4"}, 26}
	nestedDataSetElement2 = &DataElement{TargetUIDTag, UIVR, []string{"1.2.840.10008.5.1.4.1.1.5"}, 26}
	nestedSeq             = createSingletonSequence(nestedDataSetElement1, nestedDataSetElement2)
	seq                   = createSingletonSequence(&DataElement{ReferencedImageSequenceTag, SQVR, &nestedSeq, 42})
	bufferedPixelData     = &DataElement{PixelDataTag, OWVR, NewBulkDataBuffer([]byte{0x11, 0x11, 0x22, 0x22}), 4}
	expectedElements      = []*DataElement{
		{FileMetaInformationGroupLengthTag, ULVR, []uint32{198}, 4},
		{FileMetaInformationVersionTag, OBVR, NewBulkDataBuffer([]byte{0, 1}), 2},
		{MediaStorageSOPClassUIDTag, UIVR, []string{"1.2.840.10008.5.1.4.1.1.4"}, 26},
		{MediaStorageSOPInstanceUIDTag, UIVR, []string{"1.2.840.113619.2.176.3596.3364818.7271.1259708501.876"}, 54},
		{TransferSyntaxUIDTag, UIVR, []string{"1.2.840.10008.1.2.1"}, 20},
		{ImplementationClassUIDTag, UIVR, []string{"1.2.276.0.7230010.3.0.3.5.4"}, 28},
		{ImplementationVersionNameTag, SHVR, []string{"OFFIS_DCMTK_354"}, 16},
		{ReferencedStudySequenceTag, SQVR, &seq, 62},
		bufferedPixelData,
	}
)

func compareDataElements(e1 *DataElement, e2 *DataElement, order binary.ByteOrder, t *testing.T) {
	if e1 == nil || e2 == nil {
		if e1 != e2 {
			t.Fatalf("expected both elements to be nil: got %v, want %v", e1, e2)
		}
		return
	}
	if e1.VR != e2.VR {
		t.Fatalf("expected VRs to be equal: got %v, want %v", e1.VR, e2.VR)
	}
	if e1.Tag != e2.Tag {
		t.Fatalf("expected tags to be equal: got %v, want %v", e1.Tag, e2.Tag)
	}

	e1, err := processElement(e1, order)
	if err != nil {
		t.Fatalf("unexpected error unstreaming data element: %v", err)
	}
	e2, err = processElement(e2, order)
	if err != nil {
		t.Fatalf("unexpected error unstreaming data element: %v", err)
	}

	if e1.VR != SQVR {
		if !reflect.DeepEqual(e1.ValueField, e2.ValueField) {
			t.Fatalf("expected ValueFields to be equal: got %v, want %v",
				e1.ValueField, e2.ValueField)
		}
	} else {
		compareSequences(e1.ValueField.(*Sequence), e2.ValueField.(*Sequence), order, t)
	}
}

func compareSequences(s1 *Sequence, s2 *Sequence, order binary.ByteOrder, t *testing.T) {
	if len(s1.Items) != len(s2.Items) {
		t.Fatalf("expected sequences to have same length: got %v, want %v",
			len(s1.Items), len(s2.Items))
	}

	for i := range s1.Items {
		compareDataSets(s1.Items[i], s2.Items[i], order, t)
	}
}

func compareDataSets(d1 *DataSet, d2 *DataSet, order binary.ByteOrder, t *testing.T) {
	k1, k2 := d1.SortedTags(), d2.SortedTags()

	if !reflect.DeepEqual(k1, k2) {
		t.Fatalf("expected datasets to have same keys: got %v, want %v", k1, k2)
	}

	for _, tag := range k1 {
		compareDataElements(d1.Elements[tag], d2.Elements[tag], order, t)
	}
}

func createIteratorFromFile(file string) (DataElementIterator, error) {
	r, err := openFile(file)
	if err != nil {
		return nil, err
	}

	return NewDataElementIterator(r)
}

func openFile(name string) (io.Reader, error) {
	p := path.Join("../", "testdata/"+name)

	return os.Open(p)
}

var sampleBytes = []byte{1, 2, 3, 4}

func dcmReaderFromBytes(data []byte) *dcmReader {
	return newDcmReader(bytes.NewBuffer(data))
}

func createSingletonSequence(elements ...*DataElement) Sequence {
	ds := DataSet{Elements: map[DataElementTag]*DataElement{}}
	for _, elem := range elements {
		ds.Elements[elem.Tag] = elem
	}
	return Sequence{Items: []*DataSet{&ds}}
}
