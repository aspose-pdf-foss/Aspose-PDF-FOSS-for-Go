// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A CMap's own entries override those of the CMap it builds on. Merged into one
// sorted list, a search could land on the parent's range instead.
func TestCMapOwnEntriesOverrideParent(t *testing.T) {
	for _, tc := range []struct {
		cmap string
		code []byte
		cid  uint16
	}{
		{"90msp-RKSJ-V", []byte{0x81, 0x41}, 7887}, // vertical ideographic comma
		{"90msp-RKSJ-H", []byte{0x81, 0x41}, 634},
		{"90msp-RKSJ-V", []byte{0x88, 0x9f}, 1125}, // only the parent has this one
		{"UniJIS-UCS2-HW-H", []byte{0x00, 0x41}, 264},
		{"UniJIS-UCS2-H", []byte{0x00, 0x41}, 34},
	} {
		cm := predefinedCMap(tc.cmap)
		if cm == nil {
			t.Fatalf("%s not bundled", tc.cmap)
		}
		if _, cid, _ := cm.next(tc.code); cid != tc.cid {
			t.Errorf("%s %x: CID %d, want %d", tc.cmap, tc.code, cid, tc.cid)
		}
	}
	if cm := predefinedCMap("90msp-RKSJ-V"); cm.wmode != 1 || cm.verticalBases()[7887] != 634 {
		t.Errorf("wmode %d, base %d", cm.wmode, cm.verticalBases()[7887])
	}
	if predefinedCMap("90msp-RKSJ-H").verticalBases() != nil {
		t.Error("a CMap with no parent has no vertical forms")
	}
}

// Every name ISO 32000-1 Table 118 predefines is bundled.
func TestPredefinedCMapsAreBundled(t *testing.T) {
	for _, base := range []string{
		"GB-EUC", "GBpc-EUC", "GBK-EUC", "GBKp-EUC", "GBK2K", "UniGB-UCS2", "UniGB-UTF16",
		"B5pc", "HKscs-B5", "ETen-B5", "ETenms-B5", "CNS-EUC", "UniCNS-UCS2", "UniCNS-UTF16",
		"90ms-RKSJ", "90msp-RKSJ", "Add-RKSJ", "EUC", "Ext-RKSJ", "UniJIS-UCS2", "UniJIS-UCS2-HW", "UniJIS-UTF16",
		"KSC-EUC", "KSCms-UHC", "KSCms-UHC-HW", "UniKS-UCS2", "UniKS-UTF16",
	} {
		for _, suffix := range []string{"-H", "-V"} {
			if predefinedCMap(base+suffix) == nil {
				t.Errorf("%s%s missing", base, suffix)
			}
		}
	}
	for _, name := range []string{"H", "V", "83pv-RKSJ-H", "90pv-RKSJ-H", "KSCpc-EUC-H"} {
		if predefinedCMap(name) == nil {
			t.Errorf("%s missing", name)
		}
	}
	if v := predefinedCMap("V"); v == nil || v.wmode != 1 {
		t.Error("/V is vertical")
	}
}

func verticalFont(cid pdfDict, encoding pdfValue) fontInfo {
	d := NewDocument(100, 100)
	id := d.addObject(cid)
	return resolveFont(d.objects, pdfDict{"/Subtype": pdfName("/Type0"), "/Encoding": encoding, "/DescendantFonts": pdfArray{pdfRef{Num: id}}})
}

func TestVerticalMetrics(t *testing.T) {
	cid := pdfDict{"/Subtype": pdfName("/CIDFontType2"), "/DW": 800, "/W": pdfArray{5, pdfArray{600}}}
	fi := verticalFont(cid, pdfName("/Identity-V"))
	if !fi.vertical {
		t.Fatal("Identity-V is vertical")
	}
	// The defaults: 880 up, one em down, centred on the glyph's own width.
	check := func(fi fontInfo, c uint16, wantW, wantX, wantY float64) {
		t.Helper()
		if w, x, y := fi.verticalMetrics(c); w != wantW || x != wantX || y != wantY {
			t.Errorf("CID %d: %v %v %v, want %v %v %v", c, w, x, y, wantW, wantX, wantY)
		}
	}
	check(fi, 5, -1000, 300, 880)
	check(fi, 6, -1000, 400, 880)
	cid["/DW2"] = pdfArray{700, -900}
	cid["/W2"] = pdfArray{7, pdfArray{-500, 11, 22, -600, 33, 44}, 20, 22, -300, 55, 66}
	fi = verticalFont(cid, pdfName("/Identity-V"))
	check(fi, 5, -900, 300, 700)
	check(fi, 7, -500, 11, 22)
	check(fi, 8, -600, 33, 44)
	check(fi, 21, -300, 55, 66)
	check(fi, 23, -900, 400, 700)
	// Not vertical.
	if verticalFont(cid, pdfName("/Identity-H")).vertical {
		t.Error("Identity-H is horizontal")
	}
	st := buildFlateStream([]byte("x"))
	st.Dict["/WMode"] = 1
	if !verticalFont(cid, st).vertical {
		t.Error("an embedded CMap's dictionary sets the mode too")
	}
}

// A vertical CID extracts as the character it stands for; drawing wants its form.
func TestVerticalFormsAndBase(t *testing.T) {
	cid := pdfDict{"/Subtype": pdfName("/CIDFontType0"), "/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Japan1", "/Supplement": 2}}
	fi := verticalFont(cid, pdfName("/90msp-RKSJ-V"))
	if !fi.vertical || fi.cidToUni[7899] != '（' {
		t.Errorf("vertical %v, CID 7899 = %q", fi.vertical, fi.cidToUni[7899])
	}
	if fi.vertForms[7899] != '︵' {
		t.Errorf("form %q", fi.vertForms[7899])
	}
	// The shared table stays as it was for every other font.
	if cidToUnicodeForOrdering("Japan1")[7899] != '︵' {
		t.Error("the ordering's table was changed")
	}
}

func TestVerticalHelpers(t *testing.T) {
	v, h := &fontInfo{vertical: true}, &fontInfo{}
	if m := textAdvanceMatrix(v, -5); m[4] != 0 || m[5] != -5 {
		t.Errorf("vertical: %v", m)
	}
	if m := textAdvanceMatrix(h, 5); m[4] != 5 || m[5] != 0 {
		t.Errorf("horizontal: %v", m)
	}
	id := [6]float64{1, 0, 0, 1, 0, 0}
	if x, y := glyphCentre(v, id, -5, 1); x != 0 || y != -4 {
		t.Errorf("vertical: %v %v", x, y)
	}
	if x, y := glyphCentre(h, id, 5, 1); x != 5 || y != 1 {
		t.Errorf("horizontal: %v %v", x, y)
	}
	if filterHorizScale(v, 0.5) != 1 || filterHorizScale(h, 0.5) != 0.5 {
		t.Error("Tz applies across a vertical line, not along it")
	}
	if w := glyphAdvanceCID(&fontInfo{vertical: true, dw2: [2]float64{880, -1000}}, 1, 2, 10, 1, 5, 0.5); w != -9 {
		t.Errorf("advance %v", w)
	}
	if w := glyphAdvanceCID(&fontInfo{vertical: true, dw2: [2]float64{880, -1000}}, 32, 1, 10, 0, 5, 1); w != -5 {
		t.Errorf("word space, advance %v", w)
	}
}

// verticalDoc draws text down the page through an embedded Identity-V font.
func verticalDoc(t *testing.T, x, y float64, text string) (*Document, *Page) {
	t.Helper()
	d, _, _ := glyphDoc(t)
	p, _ := d.Page(1)
	gf := d.glyphFontsInUse()[0]
	gf.dict["/Encoding"] = pdfName("/Identity-V")
	var b strings.Builder
	for _, r := range text {
		gid := gf.rf.prog.glyphID(r)
		b.WriteByte(byte(gid >> 8))
		b.WriteByte(byte(gid))
	}
	res := p.pageResources()
	fonts, _ := resolveRefToDict(d.objects, res["/Font"])
	var name string
	for k := range fonts {
		name = k
	}
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName(name), 20}},
		{Operator: "Td", Operands: []pdfValue{x, y}},
		{Operator: "Tj", Operands: []pdfValue{b.String()}},
		{Operator: "ET"},
	}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream(serializeContentOps(ops)))}
	return d, p
}

func TestVerticalExtractionPositions(t *testing.T) {
	_, p := verticalDoc(t, 100, 150, "AB")
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var a, b *TextFragment
	for i := range lines {
		for j := range lines[i].Fragments {
			f := &lines[i].Fragments[j]
			switch f.Text {
			case "A":
				a = f
			case "B":
				b = f
			}
		}
	}
	if a == nil || b == nil {
		t.Fatalf("fragments: %+v", lines)
	}
	if a.Y-b.Y < 19 || a.Y-b.Y > 21 {
		t.Errorf("B should be one em below A: %v, %v", a.Y, b.Y)
	}
	// Hung from its position vector: left of the line by half a width, 0.88 em down.
	if a.X > 100 || a.X < 85 || a.Y > 150-17 || a.Y < 150-19 {
		t.Errorf("A at %v, %v", a.X, a.Y)
	}
}

func TestVerticalTJAndRendering(t *testing.T) {
	d, p := verticalDoc(t, 100, 150, "A")
	gf := d.glyphFontsInUse()[0]
	gid := gf.rf.prog.glyphID('A')
	code := string([]byte{byte(gid >> 8), byte(gid)})
	res := p.pageResources()
	fonts, _ := resolveRefToDict(d.objects, res["/Font"])
	var name string
	for k := range fonts {
		name = k
	}
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName(name), 20}},
		{Operator: "Td", Operands: []pdfValue{100, 180}},
		{Operator: "TJ", Operands: []pdfValue{pdfArray{code, 1000, code}}}, // one more em down
		{Operator: "ET"},
	}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream(serializeContentOps(ops)))}
	lines, _ := p.ExtractTextWithLayout()
	var ys []float64
	for _, l := range lines {
		for _, f := range l.Fragments {
			ys = append(ys, f.Y)
		}
	}
	if len(ys) != 2 || ys[0]-ys[1] < 39 || ys[0]-ys[1] > 41 {
		t.Errorf("baselines %v, want two ems apart", ys)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	// Ink sits around x=100, going down the page: the first glyph below y=180 in
	// PDF space, the second two ems further down.
	bounds := img.Bounds()
	minX, maxX, minY, maxY := bounds.Max.X, 0, bounds.Max.Y, 0
	for y := 0; y < bounds.Max.Y; y++ {
		for x := 0; x < bounds.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 128 {
				minX, maxX, minY, maxY = min(minX, x), max(maxX, x), min(minY, y), max(maxY, y)
			}
		}
	}
	// Centred on the line by the position vector, and hung below the text position.
	if minX > 97 || maxX < 103 || maxX > 115 {
		t.Errorf("ink spans x %d..%d, want one column centred on 100", minX, maxX)
	}
	if minY < 15 {
		t.Errorf("ink starts at y %d, above where the position vector hangs it", minY)
	}
	if h := maxY - minY; h < 40 || h > 70 {
		t.Errorf("ink spans y %d..%d, want two glyphs two ems apart", minY, maxY)
	}
}

// A wrong position here means a glyph stays that should have gone.
func TestRedactVerticalText(t *testing.T) {
	d, p := verticalDoc(t, 100, 180, "ABC")
	matches, err := p.SearchText("B")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches %+v", matches)
	}
	lines, _ := p.ExtractTextWithLayout()
	yOf := func(lines []TextLine, s string) float64 {
		for _, l := range lines {
			for _, f := range l.Fragments {
				if f.Text == s {
					return f.Y
				}
			}
		}
		return -1
	}
	cBefore := yOf(lines, "C")
	red := NewRedactAnnotation(p, matches[0].Rect)
	if err := p.Annotations().Add(red); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}
	text, _ := p.ExtractText()
	if strings.Contains(text, "B") || !strings.Contains(text, "A") || !strings.Contains(text, "C") {
		t.Errorf("text %q", text)
	}
	// What follows stays where it was.
	after, _ := p.ExtractTextWithLayout()
	if got := yOf(after, "C"); got < cBefore-0.5 || got > cBefore+0.5 {
		t.Errorf("C moved from %v to %v", cBefore, got)
	}
}

func TestAutoTagVerticalText(t *testing.T) {
	d, p := verticalDoc(t, 100, 180, "ABC")
	if _, err := d.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	a, _ := p.ExtractText()
	b, _ := rp.ExtractText()
	if strings.TrimSpace(a) == "" || a != b {
		t.Errorf("text %q -> %q", a, b)
	}
}

// A cidchar override counts as a vertical form as much as a cidrange one.
func TestVerticalBasesReadSingles(t *testing.T) {
	parent := parseCIDCMap([]byte("1 begincodespacerange <00> <FF> endcodespacerange 1 begincidchar <41> 10 endcidchar 1 begincidrange <50> <51> 20 endcidrange"), nil)
	child := parseCIDCMap([]byte("/P usecmap 1 begincidchar <41> 99 endcidchar 1 begincidrange <50> <50> 98 endcidrange"), func(string) *cidCMap { return parent })
	got := child.verticalBases()
	if got[99] != 10 || got[98] != 20 {
		t.Errorf("%v", got)
	}
}

// What follows a redacted string in the same text object has to be found where it
// is, or it stays when it should go.
func TestRedactVerticalAcrossShowOperators(t *testing.T) {
	d, p := verticalDoc(t, 100, 180, "A")
	gf := d.glyphFontsInUse()[0]
	code := func(r rune) string {
		g := gf.rf.prog.glyphID(r)
		return string([]byte{byte(g >> 8), byte(g)})
	}
	fonts, _ := resolveRefToDict(d.objects, p.pageResources()["/Font"])
	var name string
	for k := range fonts {
		name = k
	}
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName(name), 20}},
		{Operator: "Td", Operands: []pdfValue{100, 180}},
		{Operator: "Tj", Operands: []pdfValue{code('A')}},
		{Operator: "Tj", Operands: []pdfValue{code('B')}},
		{Operator: "ET"},
	}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream(serializeContentOps(ops)))}
	matches, err := p.SearchText("B")
	if err != nil || len(matches) != 1 {
		t.Fatalf("%+v, %v", matches, err)
	}
	if err := p.Annotations().Add(NewRedactAnnotation(p, matches[0].Rect)); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}
	if text, _ := p.ExtractText(); strings.Contains(text, "B") || !strings.Contains(text, "A") {
		t.Errorf("text %q", text)
	}
}

// Glyphs are placed down the page, and ShiftRestOfLine has no baseline to move along.
func TestAutoTagWalkVertical(t *testing.T) {
	d, p := verticalDoc(t, 100, 180, "AB")
	data, _ := p.contentStreams()
	items, err := atWalk(data, d.objects, p.pageResources())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.kind != atText {
			continue
		}
		found = true
		if it.horizontal {
			t.Error("a vertical line reported as horizontal")
		}
		if len(it.toks) != 2 {
			t.Fatalf("tokens %+v", it.toks)
		}
		a, b := it.toks[0], it.toks[1]
		if a.x != 100 || b.x != 100 || a.y >= 180 || a.y-b.y < 19 || a.y-b.y > 21 {
			t.Errorf("tokens at (%v,%v) and (%v,%v)", a.x, a.y, b.x, b.y)
		}
	}
	if !found {
		t.Fatal("no text item")
	}
	// A following string starts below this one.
	var fi fontInfo
	fi.vertical, fi.known, fi.isType0, fi.dw2 = true, true, true, [2]float64{880, -1000}
	gs := &textGState{font: &fi, fontSize: 10, horizScale: 1, ctm: [6]float64{1, 0, 0, 1, 0, 0}}
	tm := [6]float64{1, 0, 0, 1, 50, 50}
	atTextGlyphs("\x00\x01", gs, &tm)
	if tm[4] != 50 || tm[5] != 40 {
		t.Errorf("tm %v", tm)
	}
}

func TestVerticalFormsPickTheGlyph(t *testing.T) {
	data, err := osReadFontForTest()
	if err != nil {
		t.Skip(err)
	}
	prog, err := parseTTF(data)
	if err != nil {
		t.Fatal(err)
	}
	f := &renderFont{isType0: true, prog: prog, cjkUni: map[uint16]rune{5: 'A', 6: 'A'}}
	f.fi.vertForms = map[uint16]rune{5: 'B', 6: 0xE000} // no such glyph in the font
	if f.gid(5) != prog.glyphID('B') {
		t.Error("the vertical form should be drawn")
	}
	if f.gid(6) != prog.glyphID('A') {
		t.Error("a form the font lacks falls back to the character")
	}
	// Embedding takes the same glyph, which its width shows.
	d := NewDocument(100, 100)
	var fi fontInfo
	fi.cidToUni = map[uint16]rune{5: 'A'}
	fi.vertForms = map[uint16]rune{5: 'I'}
	cid, ok := d.buildEmbeddedCompositeFont(pdfDict{}, fi, prog, map[uint16]bool{5: true}, nil)
	if !ok {
		t.Fatal("not built")
	}
	w, _ := cid["/W"].(pdfArray)
	want := int(float64(prog.glyphWidths[prog.glyphID('I')])*1000/float64(prog.unitsPerEm) + 0.5)
	if inner, _ := w[1].(pdfArray); len(inner) != 1 || inner[0] != want {
		t.Errorf("/W = %v, want %d", w, want)
	}
}

func osReadFontForTest() ([]byte, error) { return os.ReadFile(glyphTestFont) }
