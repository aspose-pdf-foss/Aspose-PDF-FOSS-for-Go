package asposepdf

import (
	"testing"
)

func TestCollectReachableIDs(t *testing.T) {
	// Page references object 1 (image) via /XObject dict.
	// Object 10 is orphaned (not referenced from page).
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Image"),
			"/Width":   10,
			"/Height":  10,
		},
		Data:    []byte{0xFF, 0xD8, 0xFF, 0xD9},
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	// Orphaned object — not referenced from the page.
	orphanStream := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Image"),
			"/Width":   5,
			"/Height":  5,
		},
		Data:    []byte{0xFF, 0xD8, 0xFF, 0xD9},
		Decoded: false,
	}
	orphanObj := &pdfObject{Num: 10, Value: orphanStream}

	objects := map[int]*pdfObject{
		1: imgObj, 2: contentObj, 3: pageObj, 10: orphanObj,
	}

	reachable := collectReachableIDs(objects, []*pdfObject{pageObj})

	// Page (3), image (1), content (2) should be reachable.
	if !reachable[3] {
		t.Error("page object should be reachable")
	}
	if !reachable[1] {
		t.Error("image object should be reachable")
	}
	if !reachable[2] {
		t.Error("content object should be reachable")
	}
	// Orphan (10) should NOT be reachable.
	if reachable[10] {
		t.Error("orphaned object should not be reachable")
	}
}

func TestCollectReachableIDsCyclic(t *testing.T) {
	// Two objects referencing each other, but not reachable from any page.
	dictA := pdfDict{"/Ref": pdfRef{Num: 2}}
	objA := &pdfObject{Num: 1, Value: dictA}

	dictB := pdfDict{"/Ref": pdfRef{Num: 1}}
	objB := &pdfObject{Num: 2, Value: dictB}

	// A simple page with no references to objA or objB.
	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 100.0, 100.0},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	objects := map[int]*pdfObject{1: objA, 2: objB, 3: pageObj}

	reachable := collectReachableIDs(objects, []*pdfObject{pageObj})

	if reachable[1] {
		t.Error("cyclic orphan A should not be reachable")
	}
	if reachable[2] {
		t.Error("cyclic orphan B should not be reachable")
	}
	if !reachable[3] {
		t.Error("page should be reachable")
	}
}
