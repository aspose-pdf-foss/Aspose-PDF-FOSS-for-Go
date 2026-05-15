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
