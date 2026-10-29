// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------------

func atRenderAll(t *testing.T, d *Document) [][]byte {
	t.Helper()
	var out [][]byte
	for i := 1; i <= d.PageCount(); i++ {
		img, err := d.RenderImage(i, RenderOptions{DPI: 72})
		if err != nil {
			t.Fatal(err)
		}
		rgba, ok := img.(*image.RGBA)
		if !ok {
			t.Fatalf("page %d: render is %T", i, img)
		}
		out = append(out, append([]byte(nil), rgba.Pix...))
	}
	return out
}

func atTexts(t *testing.T, d *Document) []string {
	t.Helper()
	txt, err := d.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	return txt
}

// atStructTypes lists the structure tree below the document root as
// "Type(child,child)" strings, page order as written.
func atStructTypes(d *Document) []string {
	root, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	if !ok {
		return nil
	}
	var describe func(v pdfValue) string
	describe = func(v pdfValue) string {
		e, ok := resolveRefToDict(d.objects, v)
		if !ok {
			return ""
		}
		s := strings.TrimPrefix(dictGetName(e, "/S"), "/")
		var kids []string
		for _, k := range atKids(e["/K"]) {
			if _, isRef := k.(pdfRef); isRef {
				if c := describe(k); c != "" {
					kids = append(kids, c)
				}
			}
		}
		if len(kids) > 0 {
			s += "(" + strings.Join(kids, ",") + ")"
		}
		return s
	}
	var out []string
	for _, docElem := range atKids(root["/K"]) {
		de, _ := resolveRefToDict(d.objects, docElem)
		for _, k := range atKids(de["/K"]) {
			if _, isRef := k.(pdfRef); isRef {
				out = append(out, describe(k))
			}
		}
	}
	return out
}

func atKids(v pdfValue) []pdfValue {
	if arr, ok := v.(pdfArray); ok {
		return arr
	}
	if v == nil {
		return nil
	}
	return []pdfValue{v}
}

func atFixture(t *testing.T) *Document {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	for i := 0; i < 2; i++ {
		if err := doc.AddBlankPageFromFormat(PageFormatA4); err != nil {
			t.Fatal(err)
		}
	}
	body := TextStyle{Font: FontHelvetica, Size: 11}
	add := func(p *Page, s string, st TextStyle, r Rectangle) {
		t.Helper()
		if err := p.AddText(s, st, r); err != nil {
			t.Fatal(err)
		}
	}
	p1, _ := doc.Page(1)
	add(p1, "Annual Report", TextStyle{Font: FontHelveticaBold, Size: 28}, Rectangle{LLX: 72, LLY: 740, URX: 520, URY: 780})
	add(p1, "This report describes the year in some detail. It has more than one line of text so that the paragraph wraps inside its rectangle and spans several lines.",
		body, Rectangle{LLX: 72, LLY: 660, URX: 400, URY: 720})
	add(p1, "• First item", body, Rectangle{LLX: 72, LLY: 620, URX: 400, URY: 636})
	add(p1, "• Second item", body, Rectangle{LLX: 72, LLY: 606, URX: 400, URY: 622})
	tbl := NewTable().SetColumnWidths([]float64{120, 120}).
		SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 1}).
		SetDefaultCellStyle(body)
	tbl.AddRows([][]string{{"Name", "Amount"}, {"Alpha", "10"}, {"Beta", "20"}})
	if _, err := p1.AddTable(tbl, Rectangle{LLX: 72, LLY: 420, URX: 312, URY: 580}); err != nil {
		t.Fatal(err)
	}
	if err := p1.AddImage("testdata/aspose-logo.png", Rectangle{LLX: 360, LLY: 300, URX: 520, URY: 400}); err != nil {
		t.Fatal(err)
	}
	p2, _ := doc.Page(2)
	add(p2, "Second section", TextStyle{Font: FontHelveticaBold, Size: 18}, Rectangle{LLX: 72, LLY: 750, URX: 520, URY: 780})
	add(p2, "More text on the second page, written as an ordinary paragraph.", body, Rectangle{LLX: 72, LLY: 700, URX: 520, URY: 740})
	p3, _ := doc.Page(3)
	add(p3, "The last page has one paragraph only.", body, Rectangle{LLX: 72, LLY: 700, URX: 520, URY: 740})
	for i := 1; i <= 3; i++ {
		p, _ := doc.Page(i)
		add(p, fmt.Sprintf("Page %d of 3", i), TextStyle{Font: FontHelvetica, Size: 8}, Rectangle{LLX: 260, LLY: 30, URX: 400, URY: 45})
	}
	return doc
}

// --- end to end ------------------------------------------------------------

func TestAutoTagFixture(t *testing.T) {
	doc := atFixture(t)
	beforeText := atTexts(t, doc)
	beforePix := atRenderAll(t, doc)

	res, err := doc.AutoTag(DefaultAutoTaggingSettings())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("result: %+v", *res)
	t.Logf("structure: %v", atStructTypes(doc))

	if res.Pages != 3 || res.Headings < 2 || res.Paragraphs < 3 || res.Lists != 1 || res.Tables != 1 || res.Figures != 1 {
		t.Errorf("unexpected counts %+v", *res)
	}

	types := atStructTypes(doc)
	want := []string{"H1", "P", "L(LI(Lbl,LBody),LI(Lbl,LBody))", "Table(TR(TH,TH),TR(TD,TD),TR(TD,TD))", "Figure", "H", "P", "P"}
	got := strings.Join(types, " ")
	for _, w := range want {
		if w == "H" {
			continue
		}
		if !strings.Contains(got, w) {
			t.Errorf("structure %q lacks %q", got, w)
		}
	}
	if len(types) == 0 || types[0] != "H1" {
		t.Errorf("first element = %v, want H1", types)
	}

	for i, b := range atTexts(t, doc) {
		if b != beforeText[i] {
			t.Errorf("page %d text changed:\nbefore %q\nafter  %q", i+1, beforeText[i], b)
		}
	}
	for i, pix := range atRenderAll(t, doc) {
		if !bytes.Equal(pix, beforePix[i]) {
			t.Errorf("page %d renders differently after tagging", i+1)
		}
	}

	rep := doc.ValidatePDFUA()
	for _, is := range rep.Issues {
		if is.Rule != "UA_FIGURE_NO_ALT" {
			t.Errorf("PDF/UA issue: %s %s", is.Rule, is.Message)
		}
	}

	// Footer text is an artifact on every page — on pages 2 and 3 the
	// extractor merges it into the page's last paragraph.
	for pn := 1; pn <= 3; pn++ {
		pg, _ := doc.Page(pn)
		data, err := pg.contentStreams()
		if err != nil {
			t.Fatal(err)
		}
		ops, err := parseContentStream(data)
		if err != nil {
			t.Fatal(err)
		}
		var stack []string
		seen := false
		for _, op := range ops {
			switch op.Operator {
			case "BDC", "BMC":
				stack = append(stack, operandName(op.Operands[0]))
			case "EMC":
				stack = stack[:len(stack)-1]
			case "Tj":
				if s, _ := op.Operands[0].(string); strings.Contains(s, "of 3") {
					seen = true
					if len(stack) == 0 || stack[len(stack)-1] != "/Artifact" {
						t.Errorf("page %d: footer drawn inside %v, want /Artifact", pn, stack)
					}
				}
			}
		}
		if !seen {
			t.Errorf("page %d: footer not found in the content", pn)
		}
	}

	// Survives a round trip.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(atStructTypes(re), " "); got != strings.Join(types, " ") {
		t.Errorf("structure after reopen %q, want %q", got, strings.Join(types, " "))
	}
	for _, is := range re.ValidatePDFUA().Issues {
		if is.Rule != "UA_FIGURE_NO_ALT" {
			t.Errorf("after reopen, PDF/UA issue: %s %s", is.Rule, is.Message)
		}
	}
	for i, b := range atTexts(t, re) {
		if b != beforeText[i] {
			t.Errorf("after reopen, page %d text changed", i+1)
		}
	}
}

func TestAutoTagRefusesTaggedDocument(t *testing.T) {
	doc := atFixture(t)
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); !errors.Is(err, ErrAlreadyTagged) {
		t.Errorf("second AutoTag: err = %v, want ErrAlreadyTagged", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := re.AutoTag(DefaultAutoTaggingSettings()); !errors.Is(err, ErrAlreadyTagged) {
		t.Errorf("AutoTag on reopened tagged file: err = %v, want ErrAlreadyTagged", err)
	}
}

func TestAutoTagHeadingStrategies(t *testing.T) {
	count := func(s AutoTaggingSettings, outline bool) int {
		doc := atFixture(t)
		if outline {
			p2, _ := doc.Page(2)
			it := NewOutlineItemCollection(doc)
			it.SetTitle("Second section")
			it.SetDestination(NewDestinationFit(p2))
			if err := doc.Outlines().Add(it); err != nil {
				t.Fatal(err)
			}
		}
		res, err := doc.AutoTag(s)
		if err != nil {
			t.Fatal(err)
		}
		return res.Headings
	}
	if n := count(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionNone}, false); n != 0 {
		t.Errorf("None: %d headings, want 0", n)
	}
	if n := count(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionHeuristic}, false); n != 2 {
		t.Errorf("Heuristic: %d headings, want 2", n)
	}
	// With an outline, Auto trusts it: only the bookmarked title is a heading.
	if n := count(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionAuto}, true); n != 1 {
		t.Errorf("Auto with outline: %d headings, want 1", n)
	}
	if n := count(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionHeuristic, HeadingLevels: []float64{40}}, false); n != 0 {
		t.Errorf("HeadingLevels above every size: %d headings, want 0", n)
	}
}

// --- operator splitting ----------------------------------------------------

// atForceSplit rewrites a page so every other glyph belongs to a different
// owner — the worst case for splitting — and returns the rewritten ops.
func atForceSplit(t *testing.T, p *Page) []contentOp {
	t.Helper()
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	items, err := atWalk(data, p.doc.objects, p.pageResources())
	if err != nil {
		t.Fatal(err)
	}
	for i := range items {
		it := &items[i]
		switch it.kind {
		case atText:
			n := 0
			for _, tk := range it.toks {
				if tk.glyph {
					it.owners = append(it.owners, n%2)
					n++
				}
			}
		default:
			it.owner = atOwnerArtifact
		}
	}
	next := 0
	ops := atEmit(items, func(int) (int, StructType) { next++; return next - 1, StructP })
	if err := replacePageContents(p, serializeContentOps(ops)); err != nil {
		t.Fatal(err)
	}
	return ops
}

func countOps(ops []contentOp, names ...string) int {
	n := 0
	for _, op := range ops {
		for _, name := range names {
			if op.Operator == name {
				n++
			}
		}
	}
	return n
}

func TestAutoTagSplitIsExact(t *testing.T) {
	doc := NewDocument(400, 300)
	p, _ := doc.Page(1)
	f, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	content := "BT " + f + " 14 Tf 20 250 Td 0.5 Tc 2 Tw (Split me here) Tj 0 -24 Td " +
		"[(Ke) -300 (rn) 120 (ing T) -50 (J)] TJ 18 TL (Quote line) ' 3 1 (Double quote) \" " +
		"150 Tz 2 Ts (Scaled) Tj ET"
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	beforeText, beforePix := atTexts(t, doc), atRenderAll(t, doc)

	ops := atForceSplit(t, p)
	if n := countOps(ops, "Tj", "TJ"); n < 20 {
		t.Fatalf("only %d show operators after a forced split — nothing was split", n)
	}
	if got := atTexts(t, doc); got[0] != beforeText[0] {
		t.Errorf("text changed:\nbefore %q\nafter  %q", beforeText[0], got[0])
	}
	if !bytes.Equal(atRenderAll(t, doc)[0], beforePix[0]) {
		t.Error("split page renders differently")
	}
}

func TestAutoTagSplitIsExactType0(t *testing.T) {
	doc := NewDocument(400, 200)
	fnt, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddText("Grüße aus Köln — Привет", TextStyle{Font: fnt, Size: 16}, Rectangle{LLX: 20, LLY: 120, URX: 380, URY: 180}); err != nil {
		t.Fatal(err)
	}
	beforeText, beforePix := atTexts(t, doc), atRenderAll(t, doc)
	ops := atForceSplit(t, p)
	if n := countOps(ops, "Tj", "TJ"); n < 10 {
		t.Fatalf("only %d show operators after a forced split", n)
	}
	if got := atTexts(t, doc); got[0] != beforeText[0] {
		t.Errorf("text changed:\nbefore %q\nafter  %q", beforeText[0], got[0])
	}
	if !bytes.Equal(atRenderAll(t, doc)[0], beforePix[0]) {
		t.Error("split Type0 page renders differently")
	}
}

// Marked content must nest with BT/ET, q/Q and existing sections: walk the
// rewritten stream and check that every new section closes before any of
// them.
func TestAutoTagSectionsNestCorrectly(t *testing.T) {
	doc := atFixture(t)
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= doc.PageCount(); i++ {
		atCheckNesting(t, doc, i)
	}

	// A painted path right before q, more paint inside: the section must
	// not carry across the q.
	doc2 := NewDocument(200, 200)
	p, _ := doc2.Page(1)
	if err := replacePageContents(p, []byte("0 0 10 10 re f q 20 20 10 10 re f Q 40 40 10 10 re f")); err != nil {
		t.Fatal(err)
	}
	if _, err := doc2.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	atCheckNesting(t, doc2, 1)
}

// atCheckNesting asserts every marked-content section on page i closes in
// the same text object and at the same q depth it opened in.
func atCheckNesting(t *testing.T, doc *Document, i int) {
	t.Helper()
	p, _ := doc.Page(i)
	data, _ := p.contentStreams()
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatal(err)
	}
	inText, qDepth := false, 0
	type open struct {
		inText bool
		q      int
	}
	var stack []open
	for _, op := range ops {
		switch op.Operator {
		case "BT":
			inText = true
		case "ET":
			inText = false
		case "q":
			qDepth++
		case "Q":
			qDepth--
		case "BDC", "BMC":
			stack = append(stack, open{inText, qDepth})
		case "EMC":
			if len(stack) == 0 {
				t.Fatalf("page %d: unbalanced EMC", i)
			}
			if top := stack[len(stack)-1]; top.q != qDepth || top.inText != inText {
				t.Fatalf("page %d: section opened at q-depth %d (in text %v) closed at %d (in text %v)", i, top.q, top.inText, qDepth, inText)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		t.Errorf("page %d: %d unclosed marked-content sections", i, len(stack))
	}
}

// A scan under its OCR text: the full-page image is an artifact, so the text
// is not announced twice; a small image on the same page stays a figure.
func TestAutoTagScanBackgroundIsArtifact(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	if err := p.AddImage("testdata/Penguins.png", Rectangle{LLX: 0, LLY: 0, URX: 595, URY: 842}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Recognised text of the scan.", TextStyle{Size: 12, Invisible: true}, Rectangle{LLX: 72, LLY: 700, URX: 500, URY: 720}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddImage("testdata/aspose-logo.png", Rectangle{LLX: 72, LLY: 400, URX: 200, URY: 460}); err != nil {
		t.Fatal(err)
	}
	res, err := doc.AutoTag(DefaultAutoTaggingSettings())
	if err != nil {
		t.Fatal(err)
	}
	if res.Figures != 1 || res.Paragraphs != 1 {
		t.Errorf("got %+v, want 1 figure (the logo) and 1 paragraph", *res)
	}
}

// The page's previous content stream is dropped, so the saved file holds
// the text once, not twice.
func TestAutoTagDropsReplacedContent(t *testing.T) {
	doc := atFixture(t)
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(doc, "Annual Report"); !found || n != 1 {
		t.Errorf("heading text found in %d objects, want exactly 1", n)
	}
}

// An untagged file can still carry MCIDs from an earlier, discarded
// structure tree; they are stripped so no MCID nests inside another.
func TestAutoTagStripsStaleMCIDs(t *testing.T) {
	doc := NewDocument(300, 200)
	p, _ := doc.Page(1)
	f, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	content := "/P <</MCID 7>> BDC BT " + f + " 12 Tf 20 150 Td (Stale tagged text) Tj ET EMC " +
		"/Span <</MCID 8 /ActualText (x)>> BDC BT " + f + " 12 Tf 20 120 Td (More) Tj ET EMC"
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	data, _ := p.contentStreams()
	ops, _ := parseContentStream(data)
	// Our MCID sections may sit inside the kept /Span (legal); one MCID
	// section may never sit inside another, and the empty stale /P wrapper
	// must be gone.
	var stack []bool // per open section: does it carry an MCID
	wrappers := 0
	for _, op := range ops {
		switch op.Operator {
		case "BDC":
			has := false
			if d, ok := op.Operands[1].(pdfDict); ok {
				_, has = d["/MCID"]
				if m, _ := d["/MCID"].(int); has && m >= 7 {
					t.Errorf("stale MCID %d survived", m)
				}
			}
			if has {
				for _, outer := range stack {
					if outer {
						t.Errorf("MCID section nested inside another MCID section")
					}
				}
			}
			stack = append(stack, has)
		case "BMC":
			if operandName(op.Operands[0]) == "/P" {
				wrappers++
			}
			stack = append(stack, false)
		case "EMC":
			stack = stack[:len(stack)-1]
		}
	}
	if wrappers != 0 {
		t.Errorf("the emptied stale /P wrapper is still in the stream")
	}
	if len(stack) != 0 {
		t.Errorf("%d unclosed sections", len(stack))
	}
}

// Text drawn under a CTM (the top-left-origin flip Chrome/Skia emit) is
// located through it: both paragraphs get their text.
func TestAutoTagTextUnderCTM(t *testing.T) {
	doc := NewDocument(400, 300)
	p, _ := doc.Page(1)
	f, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	content := "1 0 0 -1 0 300 cm BT " + f + " 12 Tf 1 0 0 -1 30 50 Tm (First paragraph here) Tj ET " +
		"BT " + f + " 12 Tf 1 0 0 -1 30 200 Tm (Second paragraph there) Tj ET"
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	res, err := doc.AutoTag(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionNone})
	if err != nil {
		t.Fatal(err)
	}
	if res.Paragraphs != 2 {
		t.Errorf("%d paragraphs tagged, want 2 (text under a CTM was not located)", res.Paragraphs)
	}
}

// Content the producer already marked as an artifact is left alone: no
// structure element is opened inside it.
func TestAutoTagKeepsExistingArtifacts(t *testing.T) {
	doc := NewDocument(400, 300)
	p, _ := doc.Page(1)
	f, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	content := "/Artifact BMC BT " + f + " 12 Tf 30 250 Td (Decoration) Tj ET EMC " +
		"BT " + f + " 12 Tf 30 150 Td (Real content) Tj ET"
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	data, _ := p.contentStreams()
	ops, _ := parseContentStream(data)
	var stack []string
	for _, op := range ops {
		switch op.Operator {
		case "BDC", "BMC":
			if len(stack) > 0 && stack[0] == "/Artifact" {
				t.Errorf("section %v opened inside an existing artifact", op.Operands)
			}
			stack = append(stack, operandName(op.Operands[0]))
		case "EMC":
			stack = stack[:len(stack)-1]
		}
	}
}

// A document with no /Title and no heading still gets a title (PDF/UA
// requires one): the start of its first tagged text.
func TestAutoTagTitleFallback(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	long := "Quarterly figures for the northern region show steady growth in every product line, with exports leading the increase over the previous year."
	if err := p.AddText(long, TextStyle{Size: 11}, Rectangle{LLX: 72, LLY: 600, URX: 520, URY: 760}); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionNone}); err != nil {
		t.Fatal(err)
	}
	info, _ := doc.Info()
	if !strings.HasPrefix(info.Title, "Quarterly figures for the northern region") || len([]rune(info.Title)) > 101 {
		t.Errorf("title = %q", info.Title)
	}
	for _, is := range doc.ValidatePDFUA().Issues {
		t.Errorf("PDF/UA issue: %s", is.Rule)
	}
}
