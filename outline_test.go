package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestOutlines_EmptyDocReturnsRoot(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	if root == nil {
		t.Fatal("Outlines() returned nil; want non-nil empty root")
	}
	if root.Count() != 0 {
		t.Errorf("empty doc root Count = %d, want 0", root.Count())
	}
	if root.Document() != doc {
		t.Error("Document() should return original doc")
	}
	if root.Parent() != nil {
		t.Error("root.Parent() should be nil")
	}
}

func TestOutlines_RootIsStable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	r1 := doc.Outlines()
	r2 := doc.Outlines()
	if r1 != r2 {
		t.Error("Outlines() should return the same instance on repeated calls")
	}
}

func TestNewOutlineItemCollection_Standalone(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic == nil {
		t.Fatal("constructor returned nil")
	}
	if oic.Document() != doc {
		t.Error("Document() should bind to provided doc")
	}
	if oic.Parent() != nil {
		t.Error("unattached item should have nil parent")
	}
	if oic.Count() != 0 {
		t.Errorf("fresh item Count = %d, want 0", oic.Count())
	}
}

func TestOutlines_TitleGetSet(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic.Title() != "" {
		t.Errorf("default Title = %q, want \"\"", oic.Title())
	}
	oic.SetTitle("Chapter 1")
	if oic.Title() != "Chapter 1" {
		t.Errorf("Title = %q", oic.Title())
	}
}

func TestOutlines_BoldItalic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic.Bold() || oic.Italic() {
		t.Error("default Bold/Italic should be false")
	}
	oic.SetBold(true)
	oic.SetItalic(true)
	if !oic.Bold() || !oic.Italic() {
		t.Error("Set* should flip")
	}
	oic.SetBold(false)
	if oic.Bold() || !oic.Italic() {
		t.Errorf("after SetBold(false): Bold=%v Italic=%v", oic.Bold(), oic.Italic())
	}
}

func TestOutlines_Color(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic.Color() != nil {
		t.Errorf("default Color should be nil")
	}
	red := &pdf.Color{R: 1, G: 0, B: 0, A: 1}
	oic.SetColor(red)
	got := oic.Color()
	if got == nil || got.R != 1 {
		t.Errorf("Color = %+v", got)
	}
	oic.SetColor(nil)
	if oic.Color() != nil {
		t.Error("SetColor(nil) should clear")
	}
}

func TestOutlines_IsExpandedDefaultsTrue(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if !oic.IsExpanded() {
		t.Error("default IsExpanded should be true (matches Aspose .NET)")
	}
	oic.SetIsExpanded(false)
	if oic.IsExpanded() {
		t.Error("after SetIsExpanded(false), should be false")
	}
}

func TestOutlines_DestinationGetSet(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic.Destination() != nil {
		t.Error("default Destination should be nil")
	}
	d := pdf.NewDestinationXYZ(page, 100, 800, 1)
	oic.SetDestination(d)
	if oic.Destination() != d {
		t.Error("Destination should round-trip via pointer identity")
	}
	oic.SetDestination(nil)
	if oic.Destination() != nil {
		t.Error("SetDestination(nil) should clear")
	}
}

func TestOutlines_ActionGetSet(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	if oic.Action() != nil {
		t.Error("default Action should be nil")
	}
	a := pdf.NewGoToURIAction("https://example.com")
	oic.SetAction(a)
	if oic.Action() != a {
		t.Error("Action should round-trip via pointer identity")
	}
}

func TestOutlines_DestinationAndActionCoexist(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetDestination(pdf.NewDestinationFit(page))
	oic.SetAction(pdf.NewGoToURIAction("https://example.com"))
	if oic.Destination() == nil {
		t.Error("Destination should remain after SetAction")
	}
	if oic.Action() == nil {
		t.Error("Action should remain after SetDestination")
	}
}

func TestOutlines_Add(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	child := pdf.NewOutlineItemCollection(doc)
	child.SetTitle("Chapter 1")
	if err := root.Add(child); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if root.Count() != 1 {
		t.Errorf("Count after Add = %d, want 1", root.Count())
	}
	if root.At(0) != child {
		t.Error("At(0) should return added child")
	}
	if child.Parent() != root {
		t.Error("child.Parent() should be root after Add")
	}
}

func TestOutlines_AddNilError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.Outlines().Add(nil); err == nil {
		t.Error("Add(nil) should error")
	}
}

func TestOutlines_AddCrossDocumentError(t *testing.T) {
	docA := pdf.NewDocument(595, 842)
	docB := pdf.NewDocument(595, 842)
	foreign := pdf.NewOutlineItemCollection(docB)
	if err := docA.Outlines().Add(foreign); err == nil {
		t.Error("Add cross-document should error")
	}
}

func TestOutlines_AddSelfError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	if err := root.Add(root); err == nil {
		t.Error("Add(self) should error (cycle)")
	}
}

func TestOutlines_AddAlreadyAttachedError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc)
	b := pdf.NewOutlineItemCollection(doc)
	root.Add(a)
	if err := b.Add(a); err == nil {
		t.Error("Add of already-attached child should error")
	}
}

func TestOutlines_AddAncestorCycleError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc)
	b := pdf.NewOutlineItemCollection(doc)
	root.Add(a)
	a.Add(b)
	if err := b.Add(a); err == nil {
		t.Error("ancestor cycle should error")
	}
}

func TestOutlines_Insert(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc); a.SetTitle("A")
	c := pdf.NewOutlineItemCollection(doc); c.SetTitle("C")
	root.Add(a)
	root.Add(c)
	b := pdf.NewOutlineItemCollection(doc); b.SetTitle("B")
	if err := root.Insert(1, b); err != nil {
		t.Fatal(err)
	}
	if root.Count() != 3 || root.At(0) != a || root.At(1) != b || root.At(2) != c {
		t.Errorf("after Insert: order wrong")
	}
}

func TestOutlines_InsertOutOfRange(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	item := pdf.NewOutlineItemCollection(doc)
	if err := root.Insert(5, item); err == nil {
		t.Error("Insert at out-of-range should error")
	}
	if err := root.Insert(-1, item); err == nil {
		t.Error("Insert at negative should error")
	}
}

func TestOutlines_Remove(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc)
	root.Add(a)
	if !root.Remove(a) {
		t.Error("Remove should return true on hit")
	}
	if root.Count() != 0 {
		t.Errorf("Count after Remove = %d", root.Count())
	}
	if a.Parent() != nil {
		t.Error("removed item should have nil Parent")
	}
	if root.Remove(a) {
		t.Error("Remove on missing should return false")
	}
}

func TestOutlines_RemoveAt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc); a.SetTitle("A")
	b := pdf.NewOutlineItemCollection(doc); b.SetTitle("B")
	c := pdf.NewOutlineItemCollection(doc); c.SetTitle("C")
	root.Add(a)
	root.Add(b)
	root.Add(c)
	if err := root.RemoveAt(1); err != nil {
		t.Fatal(err)
	}
	if root.Count() != 2 || root.At(0) != a || root.At(1) != c {
		t.Errorf("after RemoveAt: wrong remaining")
	}
	if err := root.RemoveAt(99); err == nil {
		t.Error("RemoveAt out-of-range should error")
	}
}

func TestOutlines_AllSnapshot(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	root := doc.Outlines()
	a := pdf.NewOutlineItemCollection(doc)
	b := pdf.NewOutlineItemCollection(doc)
	root.Add(a)
	root.Add(b)
	snap := root.All()
	if len(snap) != 2 || snap[0] != a || snap[1] != b {
		t.Error("All() snapshot wrong")
	}
	// Verify it's a copy.
	snap = append(snap, pdf.NewOutlineItemCollection(doc))
	if root.Count() != 2 {
		t.Error("All() should return a snapshot, not the live slice")
	}
}

func TestOutlines_WriterEmitsOutlinesEntry(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Chapter 1")
	doc.Outlines().Add(item)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "/Outlines") {
		t.Errorf("output missing /Outlines entry; first 200 bytes: %q", s[:200])
	}
	if !strings.Contains(s, "/Type /Outlines") {
		t.Error("output missing /Type /Outlines on root dict")
	}
	if !strings.Contains(s, "Chapter 1") {
		t.Error("output missing the bookmark Title")
	}
}

func TestOutlines_WriterSkipsEmptyTree(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	// Don't add any outline items.
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	if strings.Contains(s, "/Outlines") {
		t.Error("empty outline tree should not produce /Outlines in catalog")
	}
}
