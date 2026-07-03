// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestNamedDestinations_EmptyDoc(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	nd := doc.NamedDestinations()
	if nd == nil {
		t.Fatal("NamedDestinations() returned nil")
	}
	if nd.Count() != 0 {
		t.Errorf("Count = %d, want 0", nd.Count())
	}
	if nd.Document() != doc {
		t.Error("Document() != original doc")
	}
}

func TestNamedDestinations_RootStable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	first, second := doc.NamedDestinations(), doc.NamedDestinations()
	if first != second {
		t.Error("repeated calls should return same instance")
	}
}

func TestNamedDestinations_AddGet(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	dest := pdf.NewDestinationXYZ(page, 100, 800, 1)
	if err := nd.Add("intro", dest); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if nd.Count() != 1 {
		t.Errorf("Count = %d", nd.Count())
	}
	if got := nd.Get("intro"); got != dest {
		t.Errorf("Get returned %v, want %v", got, dest)
	}
	if !nd.Has("intro") {
		t.Error("Has should report true")
	}
}

func TestNamedDestinations_AddNilError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.NamedDestinations().Add("x", nil); err == nil {
		t.Error("Add(nil) should error")
	}
}

func TestNamedDestinations_AddEmptyNameError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("", pdf.NewDestinationFit(page)); err == nil {
		t.Error("Add with empty name should error")
	}
}

func TestNamedDestinations_AddNamedDestValueError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	nd := doc.NamedDestinations()
	inner := pdf.NewNamedDestination(doc, "x")
	if err := nd.Add("y", inner); err == nil {
		t.Error("Add(NamedDestination value) should error (would loop)")
	}
}

func TestNamedDestinations_AddOverwrites(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	d1 := pdf.NewDestinationFit(page)
	d2 := pdf.NewDestinationXYZ(page, 0, 0, 0)
	nd.Add("x", d1)
	if err := nd.Add("x", d2); err != nil {
		t.Fatalf("overwrite Add: %v", err)
	}
	if nd.Count() != 1 {
		t.Errorf("Count after overwrite = %d", nd.Count())
	}
	if nd.Get("x") != d2 {
		t.Error("overwrite should replace value")
	}
}

func TestNamedDestinations_Remove(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	nd.Add("x", pdf.NewDestinationFit(page))
	if !nd.Remove("x") {
		t.Error("Remove on present should return true")
	}
	if nd.Count() != 0 {
		t.Errorf("Count after Remove = %d", nd.Count())
	}
	if nd.Remove("x") {
		t.Error("Remove on absent should return false")
	}
}

func TestNamedDestinations_NamesSorted(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	for _, n := range []string{"zebra", "apple", "mango"} {
		nd.Add(n, pdf.NewDestinationFit(page))
	}
	names := nd.Names()
	if len(names) != 3 || names[0] != "apple" || names[1] != "mango" || names[2] != "zebra" {
		t.Errorf("Names() = %v, want sorted [apple mango zebra]", names)
	}
}

func TestNamedDestinations_AllSnapshot(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	nd.Add("x", pdf.NewDestinationFit(page))
	snap := nd.All()
	if len(snap) != 1 {
		t.Errorf("All() len = %d", len(snap))
	}
	// Mutate snapshot → collection should be unchanged.
	delete(snap, "x")
	if nd.Count() != 1 {
		t.Error("All() should return a snapshot, not the live map")
	}
}

func TestNamedDestinations_Clear(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	nd.Add("a", pdf.NewDestinationFit(page))
	nd.Add("b", pdf.NewDestinationFit(page))
	nd.Clear()
	if nd.Count() != 0 {
		t.Error("Clear should empty the collection")
	}
}

func TestNamedDestinations_WriterEmitsNamesDests(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	doc.NamedDestinations().Add("intro", pdf.NewDestinationFit(page))

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "/Names") {
		t.Error("output missing /Catalog/Names entry")
	}
	if !strings.Contains(s, "/Dests") {
		t.Error("output missing /Dests inside name tree")
	}
	if !strings.Contains(s, "/Limits") {
		t.Error("output missing /Limits in tree root")
	}
	if !strings.Contains(s, "intro") {
		t.Error("output missing the registered name")
	}
}

func TestNamedDestinations_WriterSkipsEmptyCollection(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "/Dests") {
		t.Error("empty collection should not produce /Dests in output")
	}
}

// TestNamedDestinations_WriterPreservesDirectDictNamesSibling exercises the
// direct-dict /Names branch of the writer merge step. ISO 32000-1 §7.7.4
// allows /Catalog/Names to be encoded as either a direct dict or an indirect
// ref. The crafted PDF below uses the direct-dict form with a /JavaScript
// sibling alongside /Dests; the test then adds a new named destination and
// verifies on the second roundtrip that the /JavaScript sibling survived the
// /Dests rewrite.
func TestNamedDestinations_WriterPreservesDirectDictNamesSibling(t *testing.T) {
	// Assemble a minimal PDF where /Catalog/Names is a direct dict containing
	// a /JavaScript sibling. Object 1 = catalog, 2 = pages, 3 = page,
	// 4 = content stream, 5 = a JavaScript name tree referenced from the
	// direct-dict /Names. Build the xref with computed offsets so byte
	// positions stay correct regardless of whitespace shifts above.
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /Names << /JavaScript 5 0 R >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << >> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
		"<< /Names [] >>",
	}
	var pdfBuf bytes.Buffer
	pdfBuf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1) // 1-based; index 0 unused
	for i, body := range objs {
		offsets[i+1] = pdfBuf.Len()
		pdfBuf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, body))
	}
	xrefOff := pdfBuf.Len()
	pdfBuf.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objs)+1))
	pdfBuf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		pdfBuf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	pdfBuf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefOff))

	doc, err := pdf.OpenStream(bytes.NewReader(pdfBuf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream of crafted PDF: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	if err := doc.NamedDestinations().Add("intro", pdf.NewDestinationFit(page)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "/Dests") {
		t.Error("output missing new /Dests entry after Add")
	}
	if !strings.Contains(out, "/JavaScript") {
		t.Error("direct-dict /Names sibling /JavaScript was dropped by writer merge")
	}
	if !strings.Contains(out, "intro") {
		t.Error("output missing registered destination name")
	}

	// Roundtrip-parseability sanity check. Task 7 wires up parsing of
	// /Names/Dests into the NamedDestinations collection; until then we
	// just confirm the reopened doc opens cleanly and the catalog still
	// references both /Dests and /JavaScript.
	if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

func TestNamedDestinations_OutlineEmitsNameString(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	doc.NamedDestinations().Add("chapter1", pdf.NewDestinationFit(page))

	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Chapter 1")
	oic.SetDestination(pdf.NewNamedDestination(doc, "chapter1"))
	doc.Outlines().Add(oic)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// Outline /Dest should contain the name "chapter1" as a PDF string
	// literal, not an explicit-dest array. The name will also appear in
	// the /Names/Dests tree (registered above), so a plain substring
	// check on "chapter1" passes spuriously. Instead, assert the exact
	// "/Dest (chapter1)" form the writer produces for a Go string value.
	if !strings.Contains(s, "/Dest (chapter1)") {
		t.Error("output missing /Dest (chapter1) string literal in outline item")
	}
}

func TestNamedDestinations_OutlineParsesNamedRef(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page))
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Chapter")
	oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
	doc.Outlines().Add(oic)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	root := doc2.Outlines()
	if root.Count() != 1 {
		t.Fatal("outline lost")
	}
	dest := root.At(0).Destination()
	nd, ok := dest.(*pdf.NamedDestination)
	if !ok {
		t.Fatalf("Destination type = %T, want *NamedDestination", dest)
	}
	if nd.Name() != "ch1" {
		t.Errorf("Name = %q, want ch1", nd.Name())
	}
	if nd.Resolve() == nil {
		t.Error("Resolve should return the registered destination")
	}
}

func TestNamedDestinations_OutlineUnregisteredNameStillWraps(t *testing.T) {
	// Outline references "missing" but the name is never added to
	// NamedDestinations. The parser must still return a *NamedDestination
	// wrapper; Resolve() returns nil for the unresolved name.
	doc := pdf.NewDocument(595, 842)
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Orphan")
	oic.SetDestination(pdf.NewNamedDestination(doc, "missing"))
	doc.Outlines().Add(oic)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	dest := doc2.Outlines().At(0).Destination()
	nd, ok := dest.(*pdf.NamedDestination)
	if !ok {
		t.Fatalf("Destination type = %T, want *NamedDestination", dest)
	}
	if nd.Name() != "missing" {
		t.Errorf("Name = %q, want missing", nd.Name())
	}
	if nd.Resolve() != nil {
		t.Error("Resolve should be nil for unregistered name")
	}
}

func TestNamedDestinations_RoundTrip_SingleEntry(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("intro", pdf.NewDestinationXYZ(page, 100, 800, 1.5)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	dest := doc2.NamedDestinations().Get("intro")
	if dest == nil {
		t.Fatal("intro not in NamedDestinations after roundtrip")
	}
	xyz, ok := dest.(*pdf.DestinationXYZ)
	if !ok {
		t.Fatalf("type = %T, want *DestinationXYZ", dest)
	}
	if xyz.Left() != 100 || xyz.Top() != 800 || xyz.Zoom() != 1.5 {
		t.Errorf("coords: %v %v %v", xyz.Left(), xyz.Top(), xyz.Zoom())
	}
}

func TestNamedDestinations_RoundTrip_AllDestTypes(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	cases := map[string]struct {
		d    pdf.Destination
		want pdf.DestinationType
	}{
		"xyz":   {pdf.NewDestinationXYZ(page, 1, 2, 3), pdf.DestinationTypeXYZ},
		"fit":   {pdf.NewDestinationFit(page), pdf.DestinationTypeFit},
		"fith":  {pdf.NewDestinationFitH(page, 100), pdf.DestinationTypeFitH},
		"fitv":  {pdf.NewDestinationFitV(page, 50), pdf.DestinationTypeFitV},
		"fitr":  {pdf.NewDestinationFitR(page, 10, 20, 30, 40), pdf.DestinationTypeFitR},
		"fitb":  {pdf.NewDestinationFitB(page), pdf.DestinationTypeFitB},
		"fitbh": {pdf.NewDestinationFitBH(page, 100), pdf.DestinationTypeFitBH},
		"fitbv": {pdf.NewDestinationFitBV(page, 50), pdf.DestinationTypeFitBV},
	}
	for name, c := range cases {
		if err := doc.NamedDestinations().Add(name, c.d); err != nil {
			t.Fatalf("Add(%q): %v", name, err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range cases {
		got := doc2.NamedDestinations().Get(name)
		if got == nil {
			t.Errorf("[%s] missing after roundtrip", name)
			continue
		}
		if got.DestinationType() != c.want {
			t.Errorf("[%s] type = %v, want %v", name, got.DestinationType(), c.want)
		}
	}
}

func TestNamedDestinations_ForwardReference(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Notes")
	// Reference before registering.
	oic.SetDestination(pdf.NewNamedDestination(doc, "notes"))
	doc.Outlines().Add(oic)
	// Register later.
	if err := doc.NamedDestinations().Add("notes", pdf.NewDestinationFit(page)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	dest := doc2.Outlines().At(0).Destination()
	nd, ok := dest.(*pdf.NamedDestination)
	if !ok {
		t.Fatalf("Destination type = %T, want *NamedDestination", dest)
	}
	resolved := nd.Resolve()
	if resolved == nil {
		t.Fatal("forward reference didn't resolve after roundtrip")
	}
	if _, ok := resolved.(*pdf.DestinationFit); !ok {
		t.Errorf("resolved type = %T, want *DestinationFit", resolved)
	}
}
