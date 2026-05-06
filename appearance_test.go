package asposepdf

import (
	"testing"
)

func TestMakeFormXObject(t *testing.T) {
	stream := makeFormXObject([]byte("Q\n"), Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	if stream == nil {
		t.Fatal("makeFormXObject returned nil")
	}
	if got := stream.Dict["/Type"]; got != pdfName("/XObject") {
		t.Errorf("/Type = %v, want /XObject", got)
	}
	if got := stream.Dict["/Subtype"]; got != pdfName("/Form") {
		t.Errorf("/Subtype = %v, want /Form", got)
	}
	bbox, ok := stream.Dict["/BBox"].(pdfArray)
	if !ok || len(bbox) != 4 {
		t.Fatalf("/BBox = %v, want 4-elem pdfArray", stream.Dict["/BBox"])
	}
	if !stream.Decoded {
		t.Error("stream.Decoded should be true (writer flate-compresses)")
	}
}

func TestSetAppearanceNAllocatesNew(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	// A bare annotationBase (not via constructor — for unit-test isolation).
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{0.0, 0.0, 10.0, 10.0},
	}
	base := &annotationBase{dict: dict, doc: doc, page: page}

	stream := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream)

	apDict, ok := dict["/AP"].(pdfDict)
	if !ok {
		t.Fatal("/AP missing after setAppearanceN")
	}
	ref, ok := apDict["/N"].(pdfRef)
	if !ok {
		t.Fatal("/AP/N missing or not a pdfRef")
	}
	if _, ok := doc.objects[ref.Num]; !ok {
		t.Errorf("/AP/N target object %d not in doc.objects", ref.Num)
	}
}

func TestSetAppearanceNMutatesInPlace(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{0.0, 0.0, 10.0, 10.0},
	}
	base := &annotationBase{dict: dict, doc: doc, page: page}

	stream1 := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream1)

	apDict := dict["/AP"].(pdfDict)
	firstObjID := apDict["/N"].(pdfRef).Num

	// Second call: must reuse the same object ID, only mutate the bytes.
	stream2 := makeFormXObject([]byte("f\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream2)

	apDict = dict["/AP"].(pdfDict)
	if got := apDict["/N"].(pdfRef).Num; got != firstObjID {
		t.Errorf("setAppearanceN allocated new objID %d on second call (was %d)", got, firstObjID)
	}
	// And the underlying object must hold the new bytes.
	obj := doc.objects[firstObjID]
	if got := string(obj.Value.(*pdfStream).Data); got != "f\n" {
		t.Errorf("object data = %q, want %q", got, "f\n")
	}
}

func TestSetAppearanceNUnboundDoc(t *testing.T) {
	// Does nothing when base.doc is nil (annotation not yet linked).
	dict := pdfDict{}
	base := &annotationBase{dict: dict, doc: nil}
	stream := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream)
	if _, ok := dict["/AP"]; ok {
		t.Error("/AP must not be set when doc is nil")
	}
}
