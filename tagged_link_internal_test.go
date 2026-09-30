// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

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
	if err := elem.AddObjectReference(link); err == nil {
		t.Error("AddObjectReference on an unattached annotation = nil error, want an error")
	}
}

// TestAddObjectReferenceRejectsDoubleTagging: calling AddObjectReference
// twice for the same annotation must error rather than silently overwrite
// /StructParent and orphan the first index's /ParentTree entry, or append a
// duplicate OBJR sibling into /K.
func TestAddObjectReferenceRejectsDoubleTagging(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	link := NewLinkAnnotation(p, Rectangle{LLX: 50, LLY: 700, URX: 150, URY: 720})
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	elem1 := tc.Root().AddChild(StructLink)
	if err := elem1.AddObjectReference(link); err != nil {
		t.Fatal(err)
	}
	firstIdx := link.annotationBaseRef().dict["/StructParent"]

	elem2 := tc.Root().AddChild(StructLink)
	if err := elem2.AddObjectReference(link); err == nil {
		t.Error("AddObjectReference a second time on the same annotation = nil error, want an error")
	}
	if got := link.annotationBaseRef().dict["/StructParent"]; got != firstIdx {
		t.Errorf("/StructParent changed to %v after the rejected second call, want it to stay %v", got, firstIdx)
	}
	if len(tc.objRefs) != 1 {
		t.Errorf("tc.objRefs has %d entries after a rejected second call, want 1 (no orphaned entry)", len(tc.objRefs))
	}
	if k, ok := elem2.dict["/K"].(pdfArray); !ok || len(k) != 0 {
		t.Errorf("elem2's /K = %#v, want it untouched by the rejected call", elem2.dict["/K"])
	}
}

// TestAddObjectReferenceOBJRShape verifies the /K value becomes an OBJR dict
// pointing at the annotation's own object and the page it is actually
// attached to (not a page the caller merely claims), and that the
// annotation gains a /StructParent entry.
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
	if err := elem.AddObjectReference(link); err != nil {
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
	// /Pg must come from the annotation's actual attachment
	// (annotationBase.attachedPage), not a page argument the caller could
	// get wrong — there is no such argument any more, precisely so this
	// can't diverge.
	if ref, ok := objr["/Pg"].(pdfRef); !ok || ref.Num != p.pageObj().Num {
		t.Errorf("/K/Pg = %#v, want a ref to the page the annotation is attached to", objr["/Pg"])
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
// regardless of the (randomized) map iteration order they were built from —
// and each entry's VALUE must keep the right shape for its kind (array for a
// page, direct ref for an object reference), not just the right key order.
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

	// Call order fixes the index assignment: page 1 (idx 0), the link's
	// object reference (idx 1), page 2 (idx 2) — nextSP is shared and
	// strictly incrementing, so this is deterministic regardless of map
	// iteration order elsewhere.
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
	if err := linkElem.AddObjectReference(link); err != nil {
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
		switch idx {
		case 0, 2: // the two pages' /StructParents entries
			if _, ok := nums[i+1].(pdfArray); !ok {
				t.Errorf("/Nums[%d] (page entry, key %d) = %#v (%T), want a kid array", i+1, idx, nums[i+1], nums[i+1])
			}
		case 1: // the link's /StructParent entry
			ref, ok := nums[i+1].(pdfRef)
			if !ok || ref.Num != linkElem.objID {
				t.Errorf("/Nums[%d] (object-reference entry, key %d) = %#v, want a direct ref to object %d", i+1, idx, nums[i+1], linkElem.objID)
			}
		default:
			t.Errorf("unexpected /ParentTree key %d", idx)
		}
	}
}

// TestAddObjectReferenceSurvivesRoundTrip is the internal counterpart to
// tagged_link_test.go's TestTaggedLinkObjectReferenceRoundTrip: that
// external test can only check that the bytes "/OBJR"/"/StructParent" appear
// somewhere in the saved file, since pdfDict/pdfRef aren't visible outside
// the package — a check that would pass even if the OBJR pointed at the
// wrong object or the /ParentTree entry were orphaned. This one reopens the
// file and walks the raw reparsed objects to confirm the whole chain still
// resolves correctly: the /Link element's OBJR still points at the actual
// annotation, and /ParentTree's entry for that annotation's /StructParent
// still resolves directly back to the same /Link element.
func TestAddObjectReferenceSurvivesRoundTrip(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	link := NewLinkAnnotation(p, Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(NewGoToURIAction("https://example.com"))
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	linkElem, err := p.TagContent(tc.Root(), StructLink, func() error {
		return p.AddText("Visit our site", TextStyle{Font: FontHelvetica, Size: 12}, link.Rect())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := linkElem.AddObjectReference(link); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// The document was not authored via out.TaggedContent() on reopen — that
	// would build a fresh, unrelated tree (reading an existing one back
	// isn't implemented yet; see pdf-go-u8ol) — so raw object inspection is
	// the only way to reach the reparsed structure tree here.
	var linkElemNum int
	var linkDict pdfDict
	for num, obj := range out.objects {
		d, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		if d["/Type"] == pdfName("/StructElem") && d["/S"] == pdfName(string(StructLink)) {
			linkElemNum, linkDict = num, d
			break
		}
	}
	if linkDict == nil {
		t.Fatal("no /Link structure element found after round-trip")
	}

	kArr, ok := resolveRefToArray(out.objects, linkDict["/K"])
	if !ok {
		t.Fatalf("/Link element's /K = %#v, want a resolvable array", linkDict["/K"])
	}
	var objrDict pdfDict
	for _, item := range kArr {
		if d, ok := resolveRefToDict(out.objects, item); ok && d["/Type"] == pdfName("/OBJR") {
			objrDict = d
			break
		}
	}
	if objrDict == nil {
		t.Fatalf("/K = %#v, no OBJR entry found among its items after round-trip", kArr)
	}
	annotDict, ok := resolveRefToDict(out.objects, objrDict["/Obj"])
	if !ok {
		t.Fatalf("OBJR's /Obj = %#v, does not resolve to a dict", objrDict["/Obj"])
	}
	if annotDict["/Subtype"] != pdfName("/Link") {
		t.Errorf("OBJR's /Obj resolves to %#v, want a /Link annotation", annotDict)
	}
	sp, ok := annotDict["/StructParent"].(int)
	if !ok {
		t.Fatalf("annotation /StructParent = %#v after round-trip, want an int", annotDict["/StructParent"])
	}

	structTreeRoot, ok := resolveRefToDict(out.objects, out.catalog["/StructTreeRoot"])
	if !ok {
		t.Fatal("no /StructTreeRoot after round-trip")
	}
	parentTree, ok := resolveRefToDict(out.objects, structTreeRoot["/ParentTree"])
	if !ok {
		t.Fatal("no /ParentTree after round-trip")
	}
	nums, ok := resolveRefToArray(out.objects, parentTree["/Nums"])
	if !ok {
		t.Fatal("/ParentTree /Nums does not resolve to an array after round-trip")
	}
	found := false
	for i := 0; i+1 < len(nums); i += 2 {
		if nums[i] == sp {
			ref, ok := nums[i+1].(pdfRef)
			if !ok || ref.Num != linkElemNum {
				t.Errorf("/ParentTree /Nums[%d] (key %d) = %#v, want a direct ref to the /Link element (object %d)", i+1, sp, nums[i+1], linkElemNum)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("/ParentTree /Nums has no entry for /StructParent %d after round-trip", sp)
	}
}
