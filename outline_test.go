// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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
	a := pdf.NewOutlineItemCollection(doc)
	a.SetTitle("A")
	c := pdf.NewOutlineItemCollection(doc)
	c.SetTitle("C")
	root.Add(a)
	root.Add(c)
	b := pdf.NewOutlineItemCollection(doc)
	b.SetTitle("B")
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
	a := pdf.NewOutlineItemCollection(doc)
	a.SetTitle("A")
	b := pdf.NewOutlineItemCollection(doc)
	b.SetTitle("B")
	c := pdf.NewOutlineItemCollection(doc)
	c.SetTitle("C")
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
	_ = snap // appending to the snapshot must not affect root (checked below)
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

func TestOutlines_Roundtrip_Single(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Chapter 1")
	item.SetDestination(pdf.NewDestinationXYZ(page, 100, 800, 1.5))
	doc.Outlines().Add(item)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	root2 := doc2.Outlines()
	if root2.Count() != 1 {
		t.Fatalf("after roundtrip Count = %d", root2.Count())
	}
	item2 := root2.At(0)
	if item2.Title() != "Chapter 1" {
		t.Errorf("Title = %q", item2.Title())
	}
	dest := item2.Destination()
	xyz, ok := dest.(*pdf.DestinationXYZ)
	if !ok {
		t.Fatalf("Destination type = %T", dest)
	}
	if xyz.Left() != 100 || xyz.Top() != 800 || xyz.Zoom() != 1.5 {
		t.Errorf("coords: %v %v %v", xyz.Left(), xyz.Top(), xyz.Zoom())
	}
}

func TestOutlines_NestedHierarchyRoundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetTitle("Ch1")
	chapter.SetDestination(pdf.NewDestinationFit(page))
	section := pdf.NewOutlineItemCollection(doc)
	section.SetTitle("Sec1.1")
	chapter.Add(section)
	doc.Outlines().Add(chapter)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	r2 := doc2.Outlines()
	if r2.Count() != 1 {
		t.Fatalf("top Count = %d", r2.Count())
	}
	ch := r2.At(0)
	if ch.Title() != "Ch1" || ch.Count() != 1 {
		t.Errorf("Chapter: title=%q count=%d", ch.Title(), ch.Count())
	}
	sec := ch.At(0)
	if sec.Title() != "Sec1.1" {
		t.Errorf("Section title = %q", sec.Title())
	}
	if sec.Parent() != ch {
		t.Error("parent linkage broken")
	}
}

func TestOutlines_StylePropertiesRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Styled")
	item.SetBold(true)
	item.SetItalic(true)
	item.SetColor(&pdf.Color{R: 1, G: 0, B: 0.5, A: 1})
	doc.Outlines().Add(item)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Outlines().At(0)
	if !got.Bold() || !got.Italic() {
		t.Errorf("Bold=%v Italic=%v", got.Bold(), got.Italic())
	}
	c := got.Color()
	if c == nil || c.R != 1 || c.G != 0 || c.B != 0.5 {
		t.Errorf("Color = %+v", c)
	}
}

func TestOutlines_IsExpandedRoundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	parent := pdf.NewOutlineItemCollection(doc)
	parent.SetTitle("P")
	parent.SetIsExpanded(false)
	child := pdf.NewOutlineItemCollection(doc)
	child.SetTitle("C")
	parent.Add(child)
	doc.Outlines().Add(parent)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	p2 := doc2.Outlines().At(0)
	if p2.IsExpanded() {
		t.Error("collapsed state lost in roundtrip")
	}
}

func TestOutlines_AllDestinationTypesRoundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	cases := []struct {
		title string
		d     pdf.Destination
		want  pdf.DestinationType
	}{
		{"XYZ", pdf.NewDestinationXYZ(page, 1, 2, 3), pdf.DestinationTypeXYZ},
		{"Fit", pdf.NewDestinationFit(page), pdf.DestinationTypeFit},
		{"FitH", pdf.NewDestinationFitH(page, 100), pdf.DestinationTypeFitH},
		{"FitV", pdf.NewDestinationFitV(page, 50), pdf.DestinationTypeFitV},
		{"FitR", pdf.NewDestinationFitR(page, 10, 20, 30, 40), pdf.DestinationTypeFitR},
		{"FitB", pdf.NewDestinationFitB(page), pdf.DestinationTypeFitB},
		{"FitBH", pdf.NewDestinationFitBH(page, 100), pdf.DestinationTypeFitBH},
		{"FitBV", pdf.NewDestinationFitBV(page, 50), pdf.DestinationTypeFitBV},
	}
	for _, c := range cases {
		oic := pdf.NewOutlineItemCollection(doc)
		oic.SetTitle(c.title)
		oic.SetDestination(c.d)
		doc.Outlines().Add(oic)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	r := doc2.Outlines()
	if r.Count() != len(cases) {
		t.Fatalf("count = %d, want %d", r.Count(), len(cases))
	}
	for i, c := range cases {
		got := r.At(i)
		if got.Title() != c.title {
			t.Errorf("[%d] title = %q", i, got.Title())
		}
		dest := got.Destination()
		if dest == nil {
			t.Fatalf("[%d] dest is nil", i)
		}
		if dest.DestinationType() != c.want {
			t.Errorf("[%d] type = %v, want %v", i, dest.DestinationType(), c.want)
		}
	}
}

func TestOutlines_ActionRoundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Visit")
	item.SetAction(pdf.NewGoToURIAction("https://example.com"))
	doc.Outlines().Add(item)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	a := doc2.Outlines().At(0).Action()
	uri, ok := a.(*pdf.GoToURIAction)
	if !ok {
		t.Fatalf("Action type = %T", a)
	}
	if uri.URI() != "https://example.com" {
		t.Errorf("URI = %q", uri.URI())
	}
}

func TestOutlines_CrossEpicAcroForm(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	form := doc.Form()
	tb, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720}, "Name")
	tb.SetValue("Alice")
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Bookmark")
	item.SetDestination(pdf.NewDestinationFit(page))
	doc.Outlines().Add(item)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if doc2.Outlines().Count() != 1 {
		t.Error("outline lost with AcroForm")
	}
	if doc2.Form().Field("Name").Value() != "Alice" {
		t.Error("AcroForm lost with outline")
	}
}

func TestOutlines_CrossEpicAES128(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Secret Bookmark")
	item.SetDestination(pdf.NewDestinationXYZ(page, 0, 800, 1))
	doc.Outlines().Add(item)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Outlines().At(0).Title() != "Secret Bookmark" {
		t.Error("outline title lost through AES-128 roundtrip")
	}
}

func TestOutlines_CrossEpicAES256(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Secret 256")
	item.SetDestination(pdf.NewDestinationFit(page))
	doc.Outlines().Add(item)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
	})

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Outlines().At(0).Title() != "Secret 256" {
		t.Error("outline title lost through AES-256 roundtrip")
	}
}

func TestOutlines_CrossEpicAnnotation(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	page.Annotations().Add(link)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Bookmark")
	doc.Outlines().Add(item)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if doc2.Outlines().Count() != 1 {
		t.Error("outline lost with annotation")
	}
	page2 := doc2.Pages()[0]
	if page2.Annotations().Count() != 1 {
		t.Error("annotation lost with outline")
	}
}

func TestOutlines_PreservesOnReSaveWithoutCall(t *testing.T) {
	// Create a doc with outlines, save, reopen but NEVER call Outlines()
	// on the reopened doc, re-save → output should still contain the
	// outline (preserved via untouched /Outlines ref in catalog).
	doc := pdf.NewDocument(595, 842)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Preserve")
	doc.Outlines().Add(item)
	var buf1 bytes.Buffer
	doc.WriteTo(&buf1)

	doc2, _ := pdf.OpenStream(bytes.NewReader(buf1.Bytes()))
	// Intentionally skip doc2.Outlines() to avoid triggering parse.
	var buf2 bytes.Buffer
	doc2.WriteTo(&buf2)

	if !strings.Contains(buf2.String(), "/Outlines") {
		t.Error("/Outlines lost after Open + re-Save without explicit access")
	}
	if !strings.Contains(buf2.String(), "Preserve") {
		t.Error("outline Title lost after Open + re-Save")
	}
}
