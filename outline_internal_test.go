package asposepdf

import (
	"testing"
)

func TestOutlineFlags_Encoding(t *testing.T) {
	cases := []struct {
		bold, italic bool
		want         int
	}{
		{false, false, 0},
		{true, false, 2},
		{false, true, 1},
		{true, true, 3},
	}
	for _, tc := range cases {
		got := outlineFlags(tc.bold, tc.italic)
		if got != tc.want {
			t.Errorf("flags(bold=%v italic=%v) = %d, want %d", tc.bold, tc.italic, got, tc.want)
		}
	}
}

func TestVisibleDescendantCount_Flat(t *testing.T) {
	doc := NewDocument(595, 842)
	root := doc.Outlines()
	for i := 0; i < 3; i++ {
		root.Add(NewOutlineItemCollection(doc))
	}
	if got := visibleDescendantCount(root); got != 3 {
		t.Errorf("flat-3 count = %d, want 3", got)
	}
}

func TestVisibleDescendantCount_NestedExpanded(t *testing.T) {
	doc := NewDocument(595, 842)
	root := doc.Outlines()
	parent := NewOutlineItemCollection(doc)
	parent.SetIsExpanded(true)
	parent.Add(NewOutlineItemCollection(doc))
	parent.Add(NewOutlineItemCollection(doc))
	root.Add(parent)
	// parent (1) + 2 grandchildren = 3
	if got := visibleDescendantCount(root); got != 3 {
		t.Errorf("nested-expanded count = %d, want 3", got)
	}
}

func TestVisibleDescendantCount_NestedCollapsed(t *testing.T) {
	doc := NewDocument(595, 842)
	root := doc.Outlines()
	parent := NewOutlineItemCollection(doc)
	parent.SetIsExpanded(false)
	parent.Add(NewOutlineItemCollection(doc))
	parent.Add(NewOutlineItemCollection(doc))
	root.Add(parent)
	// root sees just parent (1) because parent is collapsed
	if got := visibleDescendantCount(root); got != 1 {
		t.Errorf("nested-collapsed (root view) count = %d, want 1", got)
	}
}

func TestEncodeDestinationXYZ_AllExplicit(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	d := NewDestinationXYZ(page, 100, 800, 1.5)
	arr := encodeDestination(d)
	if len(arr) != 5 {
		t.Fatalf("XYZ array len = %d, want 5", len(arr))
	}
	if name, _ := arr[1].(pdfName); name != "/XYZ" {
		t.Errorf("arr[1] = %v", arr[1])
	}
	if l, _ := arr[2].(float64); l != 100 {
		t.Errorf("arr[2] (left) = %v", arr[2])
	}
}

func TestEncodeDestinationXYZ_UnchangedFields(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	d := NewDestinationXYZUnchanged(page, 0, false, 800, true, 0, false)
	arr := encodeDestination(d)
	if _, ok := arr[2].(pdfNull); !ok {
		t.Errorf("arr[2] should be pdfNull, got %T", arr[2])
	}
	if l, _ := arr[3].(float64); l != 800 {
		t.Errorf("arr[3] (top) = %v", arr[3])
	}
	if _, ok := arr[4].(pdfNull); !ok {
		t.Errorf("arr[4] should be pdfNull, got %T", arr[4])
	}
}

func TestEncodeDestinationFit(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := encodeDestination(NewDestinationFit(page))
	if len(arr) != 2 {
		t.Fatalf("Fit array len = %d", len(arr))
	}
	if name, _ := arr[1].(pdfName); name != "/Fit" {
		t.Errorf("arr[1] = %v", arr[1])
	}
}

func TestEncodeDestinationFitR(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := encodeDestination(NewDestinationFitR(page, 10, 20, 100, 200))
	if len(arr) != 6 {
		t.Fatalf("FitR array len = %d", len(arr))
	}
	if name, _ := arr[1].(pdfName); name != "/FitR" {
		t.Errorf("arr[1] = %v", arr[1])
	}
}

func TestBuildOutlineObjects_Empty(t *testing.T) {
	doc := NewDocument(595, 842)
	ref, objs := buildOutlineObjects(doc)
	if ref.Num != 0 || len(objs) != 0 {
		t.Errorf("empty doc: ref=%v objCount=%d, want zero", ref, len(objs))
	}
}

func TestBuildOutlineObjects_Flat(t *testing.T) {
	doc := NewDocument(595, 842)
	root := doc.Outlines()
	a := NewOutlineItemCollection(doc)
	a.SetTitle("A")
	root.Add(a)
	ref, objs := buildOutlineObjects(doc)
	if ref.Num == 0 {
		t.Fatal("root ref should be non-zero")
	}
	// Expect 2 objects: root dict + 1 item dict
	if len(objs) != 2 {
		t.Errorf("obj count = %d, want 2", len(objs))
	}
	// Find the root dict (its /Type is /Outlines).
	var rootDict pdfDict
	for _, o := range objs {
		if d, ok := o.Value.(pdfDict); ok {
			if t, _ := d["/Type"].(pdfName); t == "/Outlines" {
				rootDict = d
			}
		}
	}
	if rootDict == nil {
		t.Fatal("no /Outlines root dict found")
	}
	if _, ok := rootDict["/First"]; !ok {
		t.Error("/Outlines root should have /First")
	}
	if _, ok := rootDict["/Last"]; !ok {
		t.Error("/Outlines root should have /Last")
	}
}

func TestBuildOutlineObjects_Nested(t *testing.T) {
	doc := NewDocument(595, 842)
	root := doc.Outlines()
	parent := NewOutlineItemCollection(doc)
	parent.SetTitle("P")
	child := NewOutlineItemCollection(doc)
	child.SetTitle("C")
	parent.Add(child)
	root.Add(parent)
	_, objs := buildOutlineObjects(doc)
	// Expect 3 objects: root + parent + child
	if len(objs) != 3 {
		t.Errorf("obj count = %d, want 3", len(objs))
	}
	// Find the parent item (Title == "P") and verify it has /First /Last.
	var parentDict pdfDict
	for _, o := range objs {
		if d, ok := o.Value.(pdfDict); ok {
			if title := decodeFormString(d["/Title"]); title == "P" {
				parentDict = d
			}
		}
	}
	if parentDict == nil {
		t.Fatal("no parent item dict found")
	}
	if _, ok := parentDict["/First"]; !ok {
		t.Error("parent /First missing")
	}
	if _, ok := parentDict["/Count"]; !ok {
		t.Error("parent /Count missing (has 1 child)")
	}
}

func TestParseDestinationArray_XYZ(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	pageNum := page.pageObj().Num
	arr := pdfArray{pdfRef{Num: pageNum}, pdfName("/XYZ"), 100.0, 800.0, 1.5}
	d := parseDestinationArray(doc, arr)
	if d == nil {
		t.Fatal("parseDestinationArray returned nil")
	}
	xyz, ok := d.(*DestinationXYZ)
	if !ok {
		t.Fatalf("type = %T, want *DestinationXYZ", d)
	}
	if xyz.Left() != 100 || xyz.Top() != 800 || xyz.Zoom() != 1.5 {
		t.Errorf("XYZ values: %v %v %v", xyz.Left(), xyz.Top(), xyz.Zoom())
	}
	if !xyz.HasLeft() || !xyz.HasTop() || !xyz.HasZoom() {
		t.Error("all Has* should be true")
	}
	if xyz.Page() != page {
		t.Error("Page resolution failed")
	}
}

func TestParseDestinationArray_XYZWithNulls(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/XYZ"),
		pdfNull{}, 800.0, pdfNull{}}
	d := parseDestinationArray(doc, arr).(*DestinationXYZ)
	if d.HasLeft() || !d.HasTop() || d.HasZoom() {
		t.Errorf("Has*: L=%v T=%v Z=%v", d.HasLeft(), d.HasTop(), d.HasZoom())
	}
}

func TestParseDestinationArray_Fit(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")}
	d := parseDestinationArray(doc, arr)
	if _, ok := d.(*DestinationFit); !ok {
		t.Errorf("type = %T", d)
	}
}

func TestParseDestinationArray_FitR(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/FitR"),
		10.0, 20.0, 100.0, 200.0}
	d := parseDestinationArray(doc, arr).(*DestinationFitR)
	if d.Left() != 10 || d.Bottom() != 20 || d.Right() != 100 || d.Top() != 200 {
		t.Errorf("FitR coords: %v %v %v %v", d.Left(), d.Bottom(), d.Right(), d.Top())
	}
}

func TestParseDestinationArray_UnknownFitName(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/UnknownFit")}
	if d := parseDestinationArray(doc, arr); d != nil {
		t.Errorf("unknown fit name should return nil, got %T", d)
	}
}

func TestParseDestinationArray_BadPageRef(t *testing.T) {
	doc := NewDocument(595, 842)
	arr := pdfArray{pdfRef{Num: 99999}, pdfName("/Fit")}
	if d := parseDestinationArray(doc, arr); d != nil {
		t.Errorf("bad page ref should return nil, got %T", d)
	}
}

func TestOutlines_Roundtrip_Single_Internal(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	item := NewOutlineItemCollection(doc)
	item.SetTitle("Chapter 1")
	item.SetDestination(NewDestinationXYZ(page, 100, 800, 1.5))
	doc.Outlines().Add(item)

	// We can't easily go through OpenStream here since this is an
	// internal test, but we can verify that buildOutlineObjects +
	// parseOutlines roundtrip via the doc.objects map.

	// Build and inject outline objects into d.objects (simulating
	// what the writer does).
	outlinesRef, outlineObjs := buildOutlineObjects(doc)
	for _, obj := range outlineObjs {
		doc.objects[obj.Num] = obj
	}
	// Set catalog /Outlines (simulating writer integration).
	if doc.catalog == nil {
		doc.catalog = pdfDict{}
	}
	doc.catalog["/Outlines"] = outlinesRef

	// Now reset doc.outlinesRoot and re-parse via Outlines().
	doc.outlinesRoot = nil
	root2 := doc.Outlines()
	if root2.Count() != 1 {
		t.Fatalf("after re-parse Count = %d, want 1", root2.Count())
	}
	item2 := root2.At(0)
	if item2.Title() != "Chapter 1" {
		t.Errorf("Title after re-parse = %q", item2.Title())
	}
	dest := item2.Destination()
	xyz, ok := dest.(*DestinationXYZ)
	if !ok {
		t.Fatalf("Destination type = %T", dest)
	}
	if xyz.Left() != 100 || xyz.Top() != 800 || xyz.Zoom() != 1.5 {
		t.Errorf("coords after re-parse: %v %v %v", xyz.Left(), xyz.Top(), xyz.Zoom())
	}
}
