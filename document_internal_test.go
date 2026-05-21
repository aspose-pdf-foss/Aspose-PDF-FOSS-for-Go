// SPDX-License-Identifier: MIT

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

func TestRemoveUnusedObjectsBasic(t *testing.T) {
	// Page references objects 1 and 2. Object 10 is orphaned.
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
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"),
		Decoded: true,
	}
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
	orphanObj := &pdfObject{Num: 10, Value: &pdfStream{
		Dict:    pdfDict{"/Type": pdfName("/XObject")},
		Data:    []byte{0xFF, 0xD8, 0xFF, 0xD9},
		Decoded: false,
	}}

	doc := &Document{
		objects: map[int]*pdfObject{
			1: {Num: 1, Value: imgStream}, 2: {Num: 2, Value: contentStream},
			3: pageObj, 10: orphanObj,
		},
		pages:  []*pdfObject{pageObj},
		nextID: 11,
	}

	removed := doc.RemoveUnusedObjects()
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, exists := doc.objects[10]; exists {
		t.Error("orphaned object 10 should be deleted")
	}
	if _, exists := doc.objects[1]; !exists {
		t.Error("referenced object 1 should still exist")
	}
}

func TestRemoveUnusedObjectsNone(t *testing.T) {
	// All objects are reachable.
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("q Q\n"),
		Decoded: true,
	}
	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 100.0, 100.0},
		"/Contents": pdfRef{Num: 1},
	}
	pageObj := &pdfObject{Num: 2, Value: pageDict}

	doc := &Document{
		objects: map[int]*pdfObject{
			1: {Num: 1, Value: contentStream}, 2: pageObj,
		},
		pages:  []*pdfObject{pageObj},
		nextID: 3,
	}

	origLen := len(doc.objects)
	removed := doc.RemoveUnusedObjects()
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if len(doc.objects) != origLen {
		t.Errorf("objects count changed: %d -> %d", origLen, len(doc.objects))
	}
}

func TestRemoveUnusedObjectsAfterRemoveImage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	// Object 1 is the image XObject.
	if _, exists := doc.objects[1]; !exists {
		t.Fatal("setup: object 1 should exist before removal")
	}

	err := infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Image XObject (1) is now orphaned.
	removed := doc.RemoveUnusedObjects()
	if removed < 1 {
		t.Errorf("expected at least 1 removed, got %d", removed)
	}
	if _, exists := doc.objects[1]; exists {
		t.Error("orphaned image XObject should be deleted after RemoveUnusedObjects")
	}
}

func TestRemoveUnusedObjectsSharedXObject(t *testing.T) {
	// Two pages sharing the same image XObject (object 1).
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

	content1 := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"),
		Decoded: true,
	}
	content1Obj := &pdfObject{Num: 2, Value: content1}

	content2 := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"),
		Decoded: true,
	}
	content2Obj := &pdfObject{Num: 5, Value: content2}

	page1Dict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{"/Im0": pdfRef{Num: 1}},
		},
		"/Contents": pdfRef{Num: 2},
	}
	page1Obj := &pdfObject{Num: 3, Value: page1Dict}

	page2Dict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{"/Im0": pdfRef{Num: 1}},
		},
		"/Contents": pdfRef{Num: 5},
	}
	page2Obj := &pdfObject{Num: 4, Value: page2Dict}

	doc := &Document{
		objects: map[int]*pdfObject{
			1: imgObj, 2: content1Obj, 3: page1Obj, 4: page2Obj, 5: content2Obj,
		},
		pages:  []*pdfObject{page1Obj, page2Obj},
		nextID: 6,
	}

	// Remove image from page 1 only.
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	infos[0].Remove()

	removed := doc.RemoveUnusedObjects()
	// Image is still referenced from page 2 — should NOT be removed.
	if _, exists := doc.objects[1]; !exists {
		t.Error("shared XObject should NOT be removed (still referenced from page 2)")
	}
	// The old content stream (object 2) is now orphaned (page 1 got a new content stream).
	if removed < 1 {
		t.Errorf("expected at least 1 removed (old content stream), got %d", removed)
	}
}

func TestRemoveUnusedObjectsCyclicRefs(t *testing.T) {
	// Two orphaned objects referencing each other.
	dictA := pdfDict{"/Ref": pdfRef{Num: 2}}
	objA := &pdfObject{Num: 1, Value: dictA}
	dictB := pdfDict{"/Ref": pdfRef{Num: 1}}
	objB := &pdfObject{Num: 2, Value: dictB}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 100.0, 100.0},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	doc := &Document{
		objects: map[int]*pdfObject{1: objA, 2: objB, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	removed := doc.RemoveUnusedObjects()
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if _, exists := doc.objects[1]; exists {
		t.Error("cyclic orphan A should be removed")
	}
	if _, exists := doc.objects[2]; exists {
		t.Error("cyclic orphan B should be removed")
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
