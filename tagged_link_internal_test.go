// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestAddObjectReferenceRequiresAttachedAnnotation: an annotation not yet
// added to a page has objID == 0 and must be rejected.
func TestAddObjectReferenceRequiresAttachedAnnotation(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	link := NewLinkAnnotation(p, Rectangle{LLX: 50, LLY: 700, URX: 150, URY: 720})
	// Deliberately not added to p.Annotations() yet.
	elem := tc.Root().AddChild(StructLink)
	if err := elem.AddObjectReference(p, link); err == nil {
		t.Error("AddObjectReference on an unattached annotation = nil error, want an error")
	}
}

// TestAddObjectReferenceOBJRShape verifies the /K value becomes an OBJR dict
// pointing at the annotation's own object, and that the annotation gains a
// /StructParent entry.
func TestAddObjectReferenceOBJRShape(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	link := NewLinkAnnotation(p, Rectangle{LLX: 50, LLY: 700, URX: 150, URY: 720})
	link.SetAction(NewGoToURIAction("https://example.com"))
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}

	elem := tc.Root().AddChild(StructLink)
	if err := elem.AddObjectReference(p, link); err != nil {
		t.Fatal(err)
	}

	// elem was created via AddChild, which always starts /K as an array (so
	// it can hold multiple children); AddObjectReference appends into that
	// array via the same addKidRef every other structure element uses, so
	// the OBJR ends up as the array's one element rather than a bare dict —
	// consistent with how AddChild treats every other single-child case.
	k, ok := elem.dict["/K"].(pdfArray)
	if !ok || len(k) != 1 {
		t.Fatalf("/K = %#v (%T), want a one-element array wrapping the OBJR dict", elem.dict["/K"], elem.dict["/K"])
	}
	objr, ok := k[0].(pdfDict)
	if !ok {
		t.Fatalf("/K[0] = %#v (%T), want an OBJR dict", k[0], k[0])
	}
	if objr["/Type"] != pdfName("/OBJR") {
		t.Errorf("/K/Type = %v, want /OBJR", objr["/Type"])
	}
	annotID := link.annotationBaseRef().objID
	if ref, ok := objr["/Obj"].(pdfRef); !ok || ref.Num != annotID {
		t.Errorf("/K/Obj = %#v, want a ref to object %d", objr["/Obj"], annotID)
	}
	if ref, ok := objr["/Pg"].(pdfRef); !ok || ref.Num != p.pageObj().Num {
		t.Errorf("/K/Pg = %#v, want a ref to the page object", objr["/Pg"])
	}

	sp, ok := link.annotationBaseRef().dict["/StructParent"].(int)
	if !ok {
		t.Fatalf("annotation /StructParent = %#v, want an int", link.annotationBaseRef().dict["/StructParent"])
	}

	// The /ParentTree's /Nums must carry that exact index mapped directly to
	// the /Link structure element (not wrapped in an array, unlike a page's
	// /StructParents entry).
	numsObj, ok := tc.doc.objects[tc.parentNum]
	if !ok {
		t.Fatal("no /ParentTree object")
	}
	nums, ok := numsObj.Value.(pdfDict)["/Nums"].(pdfArray)
	if !ok {
		t.Fatal("/ParentTree /Nums is not an array")
	}
	found := false
	for i := 0; i+1 < len(nums); i += 2 {
		if nums[i] == sp {
			ref, ok := nums[i+1].(pdfRef)
			if !ok || ref.Num != elem.objID {
				t.Errorf("/Nums[%d] value = %#v, want a direct ref to the /Link element (object %d)", i+1, nums[i+1], elem.objID)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("/Nums = %#v, missing an entry for index %d", nums, sp)
	}
}

// TestParentTreeInterleavesPagesAndObjectReferences: page /StructParents
// entries (arrays) and annotation /StructParent entries (direct refs) share
// one index space and must come out of rebuildParentTree in ascending order
// regardless of the (randomized) map iteration order they were built from.
func TestParentTreeInterleavesPagesAndObjectReferences(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p1, _ := doc.Page(1)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	p2, _ := doc.Page(2)

	if _, err := p1.TagContent(tc.Root(), StructP, func() error {
		return p1.AddText("a", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 100, URY: 720})
	}); err != nil {
		t.Fatal(err)
	}
	link := NewLinkAnnotation(p1, Rectangle{LLX: 50, LLY: 600, URX: 150, URY: 620})
	if err := p1.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	linkElem := tc.Root().AddChild(StructLink)
	if err := linkElem.AddObjectReference(p1, link); err != nil {
		t.Fatal(err)
	}
	if _, err := p2.TagContent(tc.Root(), StructP, func() error {
		return p2.AddText("b", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 100, URY: 720})
	}); err != nil {
		t.Fatal(err)
	}

	numsObj := tc.doc.objects[tc.parentNum]
	nums := numsObj.Value.(pdfDict)["/Nums"].(pdfArray)
	if len(nums) != 6 { // 3 entries (2 pages + 1 objRef) * (index, value)
		t.Fatalf("/Nums has %d elements, want 6 (3 entries)", len(nums))
	}
	prev := -1
	for i := 0; i+1 < len(nums); i += 2 {
		idx, ok := nums[i].(int)
		if !ok {
			t.Fatalf("/Nums[%d] = %#v, want an int key", i, nums[i])
		}
		if idx <= prev {
			t.Errorf("/Nums keys not ascending: %d after %d", idx, prev)
		}
		prev = idx
	}
}
