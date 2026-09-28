// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// addSimpleEmbeddedFont builds a bare-bones embedded simple (TrueType) font
// object — /BaseFont, /Widths (a full 256-entry table so unembedWidthsMatch
// has something to compare) and a /FontDescriptor carrying a /FontFile2
// stream — and returns its object number. The stream content is irrelevant
// to unembedFonts (it never parses the font program, only checks for the
// key's presence), so a placeholder is enough; this mirrors what a real
// Word/LibreOffice-produced ArialMT/TimesNewRomanPSMT/CourierNewPSMT font
// dict looks like, which this library has no public constructor for (LoadFont
// always produces a composite Type0/CIDFontType2 font, never a simple one).
func addSimpleEmbeddedFont(d *Document, baseFont string, widths [256]float64) int {
	arr := make(pdfArray, 256)
	for i, w := range widths {
		arr[i] = int(w)
	}
	fontFileID := d.addObject(&pdfStream{
		Dict: pdfDict{"/Length": 4},
		Data: []byte{0, 0, 0, 0},
	})
	descID := d.addObject(pdfDict{
		"/Type":      pdfName("/FontDescriptor"),
		"/FontFile2": pdfRef{Num: fontFileID},
	})
	return d.addObject(pdfDict{
		"/Type":           pdfName("/Font"),
		"/Subtype":        pdfName("/TrueType"),
		"/BaseFont":       pdfName(baseFont),
		"/FirstChar":      0,
		"/LastChar":       255,
		"/Widths":         arr,
		"/Encoding":       pdfName("/WinAnsiEncoding"),
		"/FontDescriptor": pdfRef{Num: descID},
	})
}

// attachFontToPage registers fontID in the page's /Resources/Font so it
// counts as reachable — RemoveUnusedObjects would otherwise sweep an
// unattached test font along with its FontFile2, making a test that also
// exercises RemoveUnusedObjects ambiguous about which one actually happened.
func attachFontToPage(p *Page, fontID int) {
	pd := p.pageDict()
	res, _ := pd["/Resources"].(pdfDict)
	if res == nil {
		res = pdfDict{}
		pd["/Resources"] = res
	}
	fonts, _ := res["/Font"].(pdfDict)
	if fonts == nil {
		fonts = pdfDict{}
		res["/Font"] = fonts
	}
	fonts["/F1"] = pdfRef{Num: fontID}
}

func fontHasFontFile2(d *Document, fontID int) bool {
	dict := d.objects[fontID].Value.(pdfDict)
	fd, ok := resolveRefToDict(d.objects, dict["/FontDescriptor"])
	if !ok {
		return false
	}
	_, ok = fd["/FontFile2"]
	return ok
}

// TestUnembedFontsGenuineArial: a font named and metriced exactly like a real
// embedded Arial is unembedded — the core positive case.
func TestUnembedFontsGenuineArial(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	id := addSimpleEmbeddedFont(d, "/ArialMT", helveticaWidths)
	if n := d.unembedFonts(); n != 1 {
		t.Fatalf("unembedFonts() = %d, want 1", n)
	}
	if fontHasFontFile2(d, id) {
		t.Error("/FontFile2 survived unembedding a genuine Arial")
	}
	dict := d.objects[id].Value.(pdfDict)
	if dictGetName(dict, "/BaseFont") != "/ArialMT" {
		t.Error("/BaseFont changed — unembedding must not rewrite the name")
	}
	if _, ok := dict["/Widths"]; !ok {
		t.Error("/Widths removed — layout must stay intact after unembedding")
	}
}

// TestUnembedFontsLiteralStandard14Name: a font embedded under one of the 14
// standard names outright (not an alias like "Arial") is also eligible.
func TestUnembedFontsLiteralStandard14Name(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	id := addSimpleEmbeddedFont(d, "/Times-Bold", timesBoldWidths)
	if n := d.unembedFonts(); n != 1 {
		t.Fatalf("unembedFonts() = %d, want 1", n)
	}
	if fontHasFontFile2(d, id) {
		t.Error("/FontFile2 survived unembedding a literal /Times-Bold")
	}
}

// TestUnembedFontsExoticNameKept: a font with no recognized Standard-14
// alias is left alone — there is no substitute to fall back to.
func TestUnembedFontsExoticNameKept(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	id := addSimpleEmbeddedFont(d, "/ABCDEF+BrushScriptMT", helveticaWidths)
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 for an exotic face", n)
	}
	if !fontHasFontFile2(d, id) {
		t.Error("/FontFile2 removed from a font with no recognized substitute")
	}
}

// TestUnembedFontsSymbolKept: Symbol and ZapfDingbats are Standard-14 names
// too, but have no Latin substitute (render_std14.go) — must never be
// unembedded even though the literal-name switch would otherwise match them.
func TestUnembedFontsSymbolKept(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	id := addSimpleEmbeddedFont(d, "/Symbol", symbolFontWidths)
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 for /Symbol", n)
	}
	if !fontHasFontFile2(d, id) {
		t.Error("/FontFile2 removed from /Symbol — it has no renderable substitute")
	}

	d2 := NewDocumentFromFormat(PageFormatA4)
	id2 := addSimpleEmbeddedFont(d2, "/ZapfDingbats", zapfDingbatsFontWidths)
	if n := d2.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 for /ZapfDingbats", n)
	}
	if !fontHasFontFile2(d2, id2) {
		t.Error("/FontFile2 removed from /ZapfDingbats — it has no renderable substitute")
	}
}

// TestUnembedFontsWidthMismatchKept: a font named like Arial but with widths
// that don't actually match Helvetica's AFM (a mislabeled or customized
// face) is left embedded — the name alone isn't trusted.
func TestUnembedFontsWidthMismatchKept(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	widths := helveticaWidths
	widths['A'] += 200 // well outside unembedWidthTolerance
	id := addSimpleEmbeddedFont(d, "/Arial", widths)
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 for a width mismatch", n)
	}
	if !fontHasFontFile2(d, id) {
		t.Error("/FontFile2 removed despite a width mismatch with Helvetica's AFM")
	}
}

// TestUnembedFontsNoDescriptorSkipped: a malformed font with no
// /FontDescriptor at all must not panic.
func TestUnembedFontsNoDescriptorSkipped(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	id := d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"),
		"/BaseFont": pdfName("/ArialMT"),
	})
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 (no /FontDescriptor to act on)", n)
	}
	_ = id
}

// TestUnembedFontsIgnoresNonEmbedded: a Standard-14 font that was never
// embedded in the first place (no /FontDescriptor) is correctly a no-op, not
// double-counted.
func TestUnembedFontsIgnoresAlreadyNonEmbedded(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
	})
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 for an already non-embedded font", n)
	}
}

// TestUnembedFontsCompositeSkipped: Type0/CID fonts are out of scope —
// Standard-14 has no CJK/composite equivalent.
func TestUnembedFontsCompositeSkipped(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	fontFileID := d.addObject(&pdfStream{Dict: pdfDict{"/Length": 4}, Data: []byte{0, 0, 0, 0}})
	descID := d.addObject(pdfDict{"/Type": pdfName("/FontDescriptor"), "/FontFile2": pdfRef{Num: fontFileID}})
	cidID := d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"),
		"/BaseFont": pdfName("/ArialMT"), "/FontDescriptor": pdfRef{Num: descID},
	})
	d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"),
		"/BaseFont": pdfName("/ArialMT"), "/Encoding": pdfName("/Identity-H"),
		"/DescendantFonts": pdfArray{pdfRef{Num: cidID}},
	})
	if n := d.unembedFonts(); n != 0 {
		t.Fatalf("unembedFonts() = %d, want 0 (composite fonts out of scope)", n)
	}
	if !fontHasFontFile2(d, cidID) {
		t.Error("/FontFile2 removed from a composite font's descendant")
	}
}

// TestUnembedFontsReclaimsOrphans: the public UnembedFonts wrapper reclaims
// the now-unreferenced FontFile2/FontDescriptor objects.
func TestUnembedFontsReclaimsOrphans(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, _ := d.Page(1)
	id := addSimpleEmbeddedFont(d, "/CourierNewPSMT", courierWidths)
	attachFontToPage(p, id)
	before := len(d.objects)

	n, err := d.UnembedFonts()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("UnembedFonts() = %d, want 1", n)
	}
	// The font itself is still reachable from the page (attachFontToPage),
	// so this decrease can only be the orphaned FontFile2/FontDescriptor —
	// not the whole font getting swept as unused.
	if after := len(d.objects); after >= before {
		t.Errorf("object count %d -> %d, want a decrease (orphaned FontFile2 reclaimed)", before, after)
	}
	if fontHasFontFile2(d, id) {
		t.Error("/FontFile2 still present after UnembedFonts")
	}
	if _, ok := d.objects[id]; !ok {
		t.Fatal("the font object itself was removed — it's still reachable from the page, only its FontFile2 should be gone")
	}
}

// TestOptimizeUnembedFontsWiring covers Optimize(UnembedFonts: true) end to
// end, including the RemoveUnusedObjects interaction (unembedFonts itself
// only detaches — Optimize's own remove-unused-objects step, not a second
// call inside unembedFonts, is what reclaims it during a full Optimize run).
func TestOptimizeUnembedFontsWiring(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, _ := d.Page(1)
	id := addSimpleEmbeddedFont(d, "/ArialMT", helveticaWidths)
	attachFontToPage(p, id)
	before := len(d.objects)

	res, err := d.Optimize(OptimizationOptions{UnembedFonts: true, RemoveUnusedObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.UnembeddedFonts != 1 {
		t.Errorf("UnembeddedFonts = %d, want 1", res.UnembeddedFonts)
	}
	if res.RemovedObjects == 0 {
		t.Error("RemovedObjects = 0, want at least the orphaned FontFile2 stream")
	}
	if after := len(d.objects); after >= before {
		t.Errorf("object count %d -> %d after Optimize, want a decrease", before, after)
	}
	if _, ok := d.objects[id]; !ok {
		t.Fatal("the font object itself was removed — it's reachable from the page, only its FontFile2 should be gone")
	}
	if fontHasFontFile2(d, id) {
		t.Error("/FontFile2 still present after Optimize(UnembedFonts: true)")
	}
}

// TestOptimizeDefaultOptionsLeavesFontsEmbedded: UnembedFonts is opt-in, not
// part of DefaultOptimizationOptions, since unlike the other transforms it
// can change how the document actually looks.
func TestOptimizeDefaultOptionsLeavesFontsEmbedded(t *testing.T) {
	if DefaultOptimizationOptions().UnembedFonts {
		t.Error("DefaultOptimizationOptions().UnembedFonts = true, want false (opt-in)")
	}
	d := NewDocumentFromFormat(PageFormatA4)
	p, _ := d.Page(1)
	id := addSimpleEmbeddedFont(d, "/ArialMT", helveticaWidths)
	attachFontToPage(p, id)
	if _, err := d.Optimize(DefaultOptimizationOptions()); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.objects[id]; !ok {
		t.Fatal("the font object itself was removed by DefaultOptimizationOptions()")
	}
	if !fontHasFontFile2(d, id) {
		t.Error("DefaultOptimizationOptions() unembedded a font — it must be opt-in")
	}
}
