// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// outlinedDocBytes saves a one-page document with three bookmarks whose
// titles carry a searchable marker.
func outlinedDocBytes(t *testing.T) []byte {
	t.Helper()
	doc := NewDocument(300, 300)
	for i := 0; i < 3; i++ {
		item := NewOutlineItemCollection(doc)
		item.SetTitle(fmt.Sprintf("OLDTITLE-%d", i))
		if err := doc.Outlines().Add(item); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reopen(t *testing.T, data []byte) *Document {
	t.Helper()
	doc, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The catalog's /Pages names an object number that is free on a reopened
// file (the page tree is rebuilt on save), and the next new object takes it.
// Following /Pages from the catalog kept that object alive forever.
func TestRemoveUnusedObjectsIgnoresStalePagesNumber(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	stale, ok := doc.catalog["/Pages"].(pdfRef)
	if !ok {
		t.Fatal("the reopened catalog has no /Pages reference")
	}
	// An orphan created now lands on the old /Pages number when the numbers
	// line up — force it so the test does not depend on the layout.
	doc.objects[stale.Num] = &pdfObject{Num: stale.Num, Value: pdfDict{"/Orphan": true}}
	doc.RemoveUnusedObjects()
	if _, kept := doc.objects[stale.Num]; kept {
		t.Error("an unreferenced object was kept because it reused the old /Pages number")
	}
}

// Once bookmarks are loaded and edited, the writer rebuilds the tree; the
// parsed one must not survive RemoveUnusedObjects as dead weight.
func TestRemoveUnusedObjectsDropsReplacedOutlineTree(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	for _, item := range doc.Outlines().All() {
		item.SetTitle("renamed")
	}
	doc.RemoveUnusedObjects()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("OLDTITLE")) {
		t.Error("the replaced outline tree is still in the output")
	}
	back := reopen(t, buf.Bytes())
	if n := back.Outlines().Count(); n != 3 {
		t.Fatalf("%d bookmarks after the round trip, want 3", n)
	}
	if got := back.Outlines().At(0).Title(); got != "renamed" {
		t.Errorf("bookmark title = %q, want renamed", got)
	}
}

// Untouched bookmarks are only reachable from the catalog; they must survive.
func TestRemoveUnusedObjectsKeepsUntouchedOutlines(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	doc.RemoveUnusedObjects()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if n := reopen(t, buf.Bytes()).Outlines().Count(); n != 3 {
		t.Errorf("%d bookmarks after RemoveUnusedObjects, want 3", n)
	}
}

// TestRemoveUnusedObjectsKeepsPDFAOutputIntent pins GitHub issue #10's first
// case: an /OutputIntents entry (added by ConvertToPDFA, referenced only from
// the catalog) must survive RemoveUnusedObjects. TestRemoveUnusedObjectsKeeps
// CatalogOnlyContent already covers /Catalog/Metadata the same way; this adds
// the exact PDF/A-conformance scenario the issue reported, run through
// WriteToLinearized as in the report.
func TestRemoveUnusedObjectsKeepsPDFAOutputIntent(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Hello PDF/A", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if rep, err := doc.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	} else if !rep.Conformant {
		t.Fatalf("setup: not conformant before Optimize: %+v", rep.Issues)
	}
	oiArr, ok := doc.catalog["/OutputIntents"].(pdfArray)
	if !ok || len(oiArr) != 1 {
		t.Fatalf("setup: /Catalog/OutputIntents = %#v, want a one-element array", doc.catalog["/OutputIntents"])
	}
	oiRef, ok := oiArr[0].(pdfRef)
	if !ok {
		t.Fatal("setup: /Catalog/OutputIntents[0] is not a plain reference")
	}

	if _, err := doc.Optimize(OptimizationOptions{RemoveUnusedObjects: true}); err != nil {
		t.Fatal(err)
	}
	if _, exists := doc.objects[oiRef.Num]; !exists {
		t.Error("/Catalog/OutputIntents object was removed even though the catalog still references it")
	}

	var buf bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("/Metadata null")) || bytes.Contains(buf.Bytes(), []byte("/OutputIntents [null]")) {
		t.Errorf("output has a dangling catalog reference:\n%s", buf.Bytes())
	}
	reopened := reopen(t, buf.Bytes())
	if rep := reopened.ValidatePDFA(PDFA2B); !rep.Conformant {
		t.Errorf("not PDF/A-conformant after Optimize+WriteToLinearized+reopen: %+v", rep.Issues)
	}
}

// TestRemoveUnusedObjectsKeepsAcroFormDRFont pins GitHub issue #10's second
// case: a font referenced only through /AcroForm/DR (a form field's style),
// never from any page content or annotation appearance directly, must
// survive RemoveUnusedObjects. The widget's own appearance stream has an
// EMPTY /Resources (confirmed by inspection) and its /DA is a literal string
// naming "/DejaVuSans", not a numeric reference — so /AcroForm/DR/Font is
// genuinely the font's only path to a GC root, unlike many other annotation
// scenarios where the appearance stream also carries its own font reference.
func TestRemoveUnusedObjectsKeepsAcroFormDRFont(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	field, err := doc.Form().AddTextField(1, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 725}, "name")
	if err != nil {
		t.Fatal(err)
	}
	if err := field.SetStyle(FieldStyle{TextFont: font, TextSize: 10}); err != nil {
		t.Fatal(err)
	}

	acroForm, ok := resolveRefToDict(doc.objects, doc.catalog["/AcroForm"])
	if !ok {
		t.Fatal("setup: /AcroForm is not a resolvable dict")
	}
	dr, ok := resolveRefToDict(doc.objects, acroForm["/DR"])
	if !ok {
		t.Fatal("setup: /AcroForm/DR is not a resolvable dict")
	}
	drFonts, ok := resolveRefToDict(doc.objects, dr["/Font"])
	if !ok {
		t.Fatal("setup: /AcroForm/DR/Font is not a resolvable dict")
	}
	fontRef, ok := drFonts["/DejaVuSans"].(pdfRef)
	if !ok {
		t.Fatal("setup: /AcroForm/DR/Font/DejaVuSans is not a plain reference")
	}

	if _, err := doc.Optimize(OptimizationOptions{RemoveUnusedObjects: true}); err != nil {
		t.Fatal(err)
	}
	// The direct, decisive check: the font object itself must still be in
	// doc.objects — a bare "the key still exists in the dict" check would
	// pass even for a now-dangling reference, since RemoveUnusedObjects
	// never touches dict keys, only the object table.
	if _, exists := doc.objects[fontRef.Num]; !exists {
		t.Fatal("the DejaVuSans font object was removed even though /AcroForm/DR/Font still references it")
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened := reopen(t, buf.Bytes())
	racroForm, ok := resolveRefToDict(reopened.objects, reopened.catalog["/AcroForm"])
	if !ok {
		t.Fatal("/AcroForm did not survive the round trip")
	}
	rdr, ok := resolveRefToDict(reopened.objects, racroForm["/DR"])
	if !ok {
		t.Fatal("/AcroForm/DR did not survive the round trip")
	}
	rdrFonts, ok := resolveRefToDict(reopened.objects, rdr["/Font"])
	if !ok {
		t.Fatal("/AcroForm/DR/Font did not survive the round trip")
	}
	// Resolve the value, not just the key: a dangling reference to a
	// renumbered, unrelated object (or to no object at all) would still
	// leave the "/DejaVuSans" key present, syntactically unchanged.
	rfontObj, ok := resolveRefToDict(reopened.objects, rdrFonts["/DejaVuSans"])
	if !ok {
		t.Fatal("/AcroForm/DR/Font/DejaVuSans does not resolve to any dict after the round trip")
	}
	if rfontObj["/BaseFont"] == nil {
		t.Errorf("/AcroForm/DR/Font/DejaVuSans resolves to %#v, which is not a font dict", rfontObj)
	}
}

// Removing every bookmark must remove them from the saved file; the catalog
// used to keep pointing at the parsed tree, so they all came back.
func TestRemovingAllOutlinesRemovesThem(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	for doc.Outlines().Count() > 0 {
		if err := doc.Outlines().RemoveAt(0); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if n := reopen(t, buf.Bytes()).Outlines().Count(); n != 0 {
		t.Errorf("%d bookmarks came back after removing all of them", n)
	}
}
