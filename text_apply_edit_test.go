// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// showOps returns the text-showing operators of page 1's content, each as
// "<operator> <operand bytes>", in order.
func showOps(t *testing.T, d *Document) []string {
	t.Helper()
	p := mustPage(t, d)
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, op := range ops {
		switch op.Operator {
		case "Tj", "TJ", "'", `"`:
			if op.Operator == "TJ" && advanceOnly(op) {
				continue // what the removal leaves: an advance, no glyph
			}
			out = append(out, op.Operator+" "+string(serializeContentOps([]contentOp{{Operands: op.Operands}})))
		}
	}
	return out
}

func advanceOnly(op contentOp) bool {
	if len(op.Operands) != 1 {
		return false
	}
	arr, ok := op.Operands[0].(pdfArray)
	if !ok {
		return false
	}
	for _, v := range arr {
		switch v.(type) {
		case int, float64:
		default:
			return false
		}
	}
	return true
}

// pageOps counts page 1's content operators named op.
func pageOps(t *testing.T, d *Document, op string) int {
	t.Helper()
	data, err := mustPage(t, d).contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, o := range ops {
		if o.Operator == op {
			n++
		}
	}
	return n
}

// dejaVuDoc draws text in an embedded DejaVu Sans subset at (100, 600) and
// returns the document saved and reopened.
func dejaVuDoc(t *testing.T, text string) []byte {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := NewTextFragment(text)
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: font, FontSize: 14}
	if err := NewTextBuilder(mustPage(t, d)).AppendText(f); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SubsetFonts(); err != nil {
		t.Fatal(err)
	}
	return writeBytes(t, d)
}

// A font change on one segment redraws that segment alone, at its old start;
// the other segments' show operators stay as they were.
func TestApplySegmentFont(t *testing.T) {
	d, _ := absorberDoc(t)
	before := showOps(t, d)
	f := absorbOne(t, d, "plain bold tail")
	start := f.Segments[1].Position
	f.Segments[1].TextState.Font = FontCourier
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	after := showOps(t, d)
	if len(before) != 3 || len(after) != 3 {
		t.Fatalf("show ops %q -> %q", before, after)
	}
	// Segments 0 and 2 keep their operators byte for byte; the redrawn
	// segment is drawn last.
	if after[0] != before[0] || after[1] != before[2] {
		t.Fatalf("other segments changed: %q -> %q", before, after)
	}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "plain bold tail")
	// The segment stays bold: Courier in its bold face.
	if len(g.Segments) != 3 || g.Segments[1].TextState.Font.BaseFont() != "Courier-Bold" {
		t.Fatalf("segments %d font %q", len(g.Segments), g.Segments[1].TextState.Font.BaseFont())
	}
	if !nearPt(g.Segments[1].Position.XIndent, start.XIndent) || !nearPt(g.Segments[1].Position.YIndent, start.YIndent) {
		t.Fatalf("moved %+v -> %+v", start, g.Segments[1].Position)
	}
}

// New text with a character neither the embedded subset nor the Standard-14
// face has is drawn in the original family, found by name; with a character
// that family lacks too, the call fails naming it and the page is unchanged.
func TestApplyFontFallback(t *testing.T) {
	AddFontFile("testdata/DejaVuSans.ttf")
	defer ClearFontSources()
	src := dejaVuDoc(t, "Hello world")

	d := openBytes(t, src)
	f := absorbOne(t, d, "Hello")
	f.Text = "Привет"
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "Привет")
	if !strings.Contains(g.FontName, "DejaVu") {
		t.Fatalf("drawn in %q", g.FontName)
	}
	if absorbCount(t, r, "Hello") != 0 || absorbCount(t, r, "world") != 1 {
		t.Fatal("old text left or neighbour lost")
	}

	d = openBytes(t, src)
	before := writeBytes(t, openBytes(t, src))
	f = absorbOne(t, d, "Hello")
	fontsBefore := len(d.embeddedFonts)
	f.Text = "Hello 中"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "中") || !strings.Contains(err.Error(), "page 1") ||
		!strings.Contains(err.Error(), `"Hello"`) {
		t.Fatalf("err %v", err)
	}
	if len(d.embeddedFonts) != fontsBefore {
		t.Fatal("fallback font loaded while planning stays registered")
	}
	f.Text = "Hello"
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("refused edit changed the page")
	}
}

// Edited right-to-left text keeps its right edge.
func TestApplyRTL(t *testing.T) {
	AddFontFile("testdata/DejaVuSans.ttf")
	defer ClearFontSources()
	d := openBytes(t, dejaVuDoc(t, "שלום עולם"))
	f := absorbOne(t, d, "שלום עולם")
	right := f.Segments[len(f.Segments)-1].Rectangle.URX
	for _, s := range f.Segments {
		if s.Rectangle.URX > right {
			right = s.Rectangle.URX
		}
	}
	left := f.Segments[0].Rectangle.LLX
	f.Text = "שלום"
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "שלום")
	box := g.Segments[0].Rectangle
	if !nearPt(box.URX, right) {
		t.Fatalf("right edge %v, want %v", box.URX, right)
	}
	if box.LLX < left+10 {
		t.Fatalf("left edge %v did not move in from %v", box.LLX, left)
	}
}

// Text "" removes the fragment and draws nothing in its place.
func TestApplyEmptyText(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	before := showOps(t, d)
	f := absorbOne(t, d, "second line")
	f.Text = ""
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	after := showOps(t, d)
	if len(after) != len(before)-1 {
		t.Fatalf("show ops %q -> %q", before, after)
	}
	for _, op := range after {
		if strings.Contains(op, "second") || strings.Contains(op, "()") {
			t.Fatalf("left %q", op)
		}
	}
	if len(f.bind.drawn) != 0 {
		t.Fatalf("drawn %v", f.bind.drawn)
	}
}

// A fragment on a 45-degree baseline is found but refuses edits, naming its
// page and text, before any page changes.
func TestApplyRotatedRefused(t *testing.T) {
	src := rotatedDocBytes(t)
	d := openBytes(t, src)
	before := writeBytes(t, openBytes(t, src))
	tilt := absorbOne(t, d, "tilted")
	tilt.TextState.FontSize = 20
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "page 1") || !strings.Contains(err.Error(), `"tilted"`) ||
		!strings.Contains(err.Error(), "rotated") {
		t.Fatalf("err %v", err)
	}
	tilt.TextState = copyState(tilt.bind.snap.state)
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("refused edit changed the page")
	}
}

// nestedFormDoc places a form with text inside a nested form on each of pages
// (1-based page count), returned saved.
func nestedFormDoc(t *testing.T, pages int) []byte {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	for i := 1; i < pages; i++ {
		if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
			t.Fatal(err)
		}
	}
	inner := d.CreateForm(200, 40)
	if err := inner.Canvas().AddText("Shared caption", TextStyle{Font: FontHelvetica, Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 40}); err != nil {
		t.Fatal(err)
	}
	outer := d.CreateForm(200, 40)
	if err := outer.Canvas().AddForm(inner, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 40}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= pages; i++ {
		p, _ := d.Page(i)
		if err := p.AddForm(outer, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 640}); err != nil {
			t.Fatal(err)
		}
		g := NewTextFragment("page text")
		g.Position = Position{XIndent: 100, YIndent: 400}
		if err := NewTextBuilder(p).AppendText(g); err != nil {
			t.Fatal(err)
		}
	}
	return writeBytes(t, d)
}

// reachableContainsText reports whether a decoded stream reachable from the
// page holds text.
func reachableContainsText(d *Document, page int, text string) bool {
	for id := range collectReachableIDs(d.objects, []*pdfObject{d.pages[page]}) {
		if o, ok := d.objects[id]; ok {
			if s, ok := o.Value.(*pdfStream); ok && bytes.Contains(decodedStreamData(s), []byte(text)) {
				return true
			}
		}
	}
	return false
}

// Text drawn in a (nested) form placed on two pages, edited on one: the other
// keeps the original; the edited page's graph holds the old text nowhere.
func TestApplyFormXObject(t *testing.T) {
	d := openBytes(t, nestedFormDoc(t, 2))
	p1 := mustPage(t, d)
	a := NewTextFragmentAbsorber("Shared")
	if err := a.Visit(p1); err != nil {
		t.Fatal(err)
	}
	if len(a.TextFragments()) != 1 {
		t.Fatalf("found %d", len(a.TextFragments()))
	}
	a.TextFragments()[0].Text = "Edited"
	r := openBytes(t, writeBytes(t, d))
	p1, _ = r.Page(1)
	p2, _ := r.Page(2)
	t1, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	t2, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(t1, "Shared") || !strings.Contains(t1, "Edited caption") {
		t.Fatalf("page 1 %q", t1)
	}
	if !strings.Contains(t2, "Shared caption") || strings.Contains(t2, "Edited") {
		t.Fatalf("page 2 %q", t2)
	}
	if reachableContainsText(r, 0, "Shared") {
		t.Fatal("old text reachable from the edited page")
	}
	if !reachableContainsText(r, 1, "Shared") {
		t.Fatal("other page lost the form")
	}
}

// The original forms, nested ones included, no longer referenced once the
// only placement was edited are not left in the saved file.
func TestApplyFormXObjectDropsOriginals(t *testing.T) {
	d := openBytes(t, nestedFormDoc(t, 1))
	absorbOne(t, d, "Shared").Text = "Edited"
	r := openBytes(t, writeBytes(t, d))
	if ok, n := documentContainsBytes(r, "Shared"); ok {
		t.Fatalf("old text in %d objects of the saved file", n)
	}
	if absorbCount(t, r, "Edited caption") != 1 {
		t.Fatal("edit lost")
	}
}

// A redrawn authored fragment takes its old background box and link with it;
// an identical box drawn by the caller stays.
func TestApplyRemovesOwnBackgroundAndLink(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("linked")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontHelvetica, FontSize: 12, BackgroundColor: &Color{1, 1, 0, 1}}
	f.Segments[0].Hyperlink = NewWebHyperlink("https://example.com/")
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	box := f.Segments[0].Rectangle
	// The caller's own box, exactly where the background is.
	if err := p.DrawRectangle(box, ShapeStyle{FillColor: &Color{1, 1, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	if n := pageOps(t, d, "re"); n != 2 {
		t.Fatalf("rects %d", n)
	}
	f.Text = "a longer linked text"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if n := pageOps(t, d, "re"); n != 2 {
		t.Fatalf("rects after %d", n)
	}
	r := openBytes(t, writeBytes(t, d))
	rp := mustPage(t, r)
	if n := rp.Annotations().Count(); n != 1 {
		t.Fatalf("annotations %d", n)
	}
	link, ok := rp.Annotations().At(0).(*LinkAnnotation)
	if !ok || link.Rect().URX < box.URX+20 {
		t.Fatalf("link %+v", rp.Annotations().At(0))
	}
	// The caller's box is still there, with its old size.
	data, _ := rp.contentStreams()
	ops, _ := parseContentStream(data)
	var widths []float64
	for _, op := range ops {
		if op.Operator == "re" && len(op.Operands) == 4 {
			if w, err := toFloat(op.Operands[2]); err == nil {
				widths = append(widths, w)
			}
		}
	}
	if len(widths) != 2 || !(nearPt(widths[0], box.URX-box.LLX) || nearPt(widths[1], box.URX-box.LLX)) {
		t.Fatalf("rect widths %v, old box width %v", widths, box.URX-box.LLX)
	}
	// A second edit removes the box and link drawn by the first.
	f.Text = "short"
	r = openBytes(t, writeBytes(t, d))
	if n := mustPage(t, r).Annotations().Count(); n != 1 {
		t.Fatalf("annotations after second edit %d", n)
	}
	if n := pageOps(t, r, "re"); n != 2 {
		t.Fatalf("rects after second edit %d", n)
	}
}

// A segment redrawn alone takes only its own background and link.
func TestApplySegmentRemovesOwnBackgroundAndLink(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := &TextFragment{Position: Position{XIndent: 100, YIndent: 600},
		TextState: TextState{Font: FontHelvetica, FontSize: 12, BackgroundColor: &Color{0.8, 0.8, 0.8, 1}}}
	for _, s := range []string{"one ", "two ", "three"} {
		f.Segments = append(f.Segments, &TextSegment{Text: s, Hyperlink: NewWebHyperlink("https://example.com/" + strings.TrimSpace(s))})
	}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.Segments[1].TextState.ForegroundColor = &Color{1, 0, 0, 1}
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if n := pageOps(t, d, "re"); n != 3 {
		t.Fatalf("rects %d", n)
	}
	if n := p.Annotations().Count(); n != 3 {
		t.Fatalf("annotations %d", n)
	}
	uris := map[string]bool{}
	for _, a := range p.Annotations().All() {
		if l, ok := a.(*LinkAnnotation); ok {
			if u, ok := l.Action().(*GoToURIAction); ok {
				uris[u.URI()] = true
			}
		}
	}
	if len(uris) != 3 {
		t.Fatalf("links %v", uris)
	}
}

// A found fragment's background and links belong to the page: an edit keeps
// them.
func TestApplyFoundKeepsPageGraphics(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("boxed")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontHelvetica, FontSize: 12, BackgroundColor: &Color{1, 1, 0, 1}}
	f.Segments[0].Hyperlink = NewWebHyperlink("https://example.com/")
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	absorbOne(t, r, "boxed").Text = "boxes"
	if err := r.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if n := pageOps(t, r, "re"); n != 1 {
		t.Fatalf("rects %d", n)
	}
	if n := mustPage(t, r).Annotations().Count(); n != 1 {
		t.Fatalf("annotations %d", n)
	}
}

// After an edit the found fragment's value fields describe what was drawn.
func TestApplyRefreshesValueFields(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	f.Text = "Picture"
	f.TextState.FontSize = 18
	f.TextState.ForegroundColor = &Color{1, 0, 0, 1}
	f.Position = Position{XIndent: 150, YIndent: 650}
	f.TextState.Font = FontCourier
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	g := absorbOne(t, d, "Picture")
	if f.Text != "Picture" || !nearPt(f.X, g.X) || !nearPt(f.Y, g.Y) || !nearPt(f.Width, g.Width) ||
		f.FontName != g.FontName || f.FontSize != g.FontSize || f.Color != g.Color {
		t.Fatalf("fields\n got  %q x%v y%v w%v %q %v %+v\n want %q x%v y%v w%v %q %v %+v",
			f.Text, f.X, f.Y, f.Width, f.FontName, f.FontSize, f.Color,
			g.Text, g.X, g.Y, g.Width, g.FontName, g.FontSize, g.Color)
	}
}

// Edits pending on a document carrying a revision appended in memory
// (AddValidationInfo), which is written back as it is, are refused rather
// than dropped.
func TestApplyRefusedAfterAppendedRevision(t *testing.T) {
	doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
	ff := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: "good"}
	if err := doc.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline, Fetcher: ff}); err != nil {
		t.Fatal(err)
	}
	f := absorbOne(t, doc, "Signed")
	// Untouched, nothing stands in the way.
	if err := doc.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	f.Text = "Changed"
	err := doc.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), `"Signed"`) || !strings.Contains(err.Error(), "AddValidationInfo") {
		t.Fatalf("err %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err == nil {
		t.Fatal("WriteTo dropped the pending edit")
	}
	if _, err := buildDocumentPDF(doc); err == nil {
		t.Fatal("the writer dropped the pending edit")
	}
}

// Edits pending when AddValidationInfo appends its revision would be lost:
// refused.
func TestAddValidationInfoRefusesPendingEdits(t *testing.T) {
	doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
	absorbOne(t, doc, "Signed").Text = "Changed"
	ff := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: "good"}
	if err := doc.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline, Fetcher: ff}); err == nil ||
		!strings.Contains(err.Error(), "text changes pending") {
		t.Fatalf("err %v", err)
	}
}

// Text in a font whose glyph positions are unknown cannot be removed: the
// edit is refused while planning, before any page changes.
func TestApplyUnknownPositionsRefusedInPlanning(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	a := NewTextFragment("alpha")
	a.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p).AppendText(a); err != nil {
		t.Fatal(err)
	}
	if err := p.appendToContentStream([]byte("BT /Fmissing 12 Tf 100 300 Td (zzz) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	next := d.nextID
	a.Text = "ALPHA"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "page 1") || !strings.Contains(err.Error(), `"alpha"`) ||
		!strings.Contains(err.Error(), "cannot be edited") || !strings.Contains(err.Error(), "position") {
		t.Fatalf("err %v", err)
	}
	if d.nextID != next {
		t.Fatal("objects created before the refusal")
	}
}

// A background drawn in an earlier session keeps its section id: one drawn
// after reopening gets another, so editing the new fragment leaves the old
// box alone.
func TestApplyBackgroundIDsAcrossSessions(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	old := NewTextFragment("earlier")
	old.Position = Position{XIndent: 100, YIndent: 600}
	old.TextState = TextState{BackgroundColor: &Color{1, 1, 0, 1}}
	if err := NewTextBuilder(mustPage(t, d)).AppendText(old); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	f := NewTextFragment("later")
	f.Position = Position{XIndent: 100, YIndent: 500}
	f.TextState = TextState{BackgroundColor: &Color{0, 1, 1, 1}}
	if err := NewTextBuilder(mustPage(t, r)).AppendText(f); err != nil {
		t.Fatal(err)
	}
	if f.bind.owned[0].bg != 2 {
		t.Fatalf("id %d", f.bind.owned[0].bg)
	}
	f.Text = "much later"
	if err := r.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if n := pageOps(t, r, "re"); n != 2 {
		t.Fatalf("rects %d", n)
	}
}

// A multi-segment fragment's refreshed width spans its whole first line.
func TestApplyRefreshesWidthAcrossSegments(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.TextState.FontSize = 20
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	g := absorbOne(t, d, "plain bold tail")
	if !nearPt(f.Width, g.Width) || !nearPt(f.X, g.X) {
		t.Fatalf("x %v w %v, want x %v w %v", f.X, f.Width, g.X, g.Width)
	}
}

// A segment redrawn twice: the second redraw removes what the first drew.
func TestApplySegmentTwiceRemovesFirstRedraw(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := &TextFragment{Position: Position{XIndent: 100, YIndent: 600},
		TextState: TextState{Font: FontHelvetica, FontSize: 12, BackgroundColor: &Color{0.8, 0.8, 0.8, 1}}}
	for _, s := range []string{"one ", "two"} {
		f.Segments = append(f.Segments, &TextSegment{Text: s, Hyperlink: NewWebHyperlink("https://example.com/")})
	}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Color{{1, 0, 0, 1}, {0, 1, 0, 1}} {
		f.Segments[1].TextState.ForegroundColor = c
		if err := d.ApplyTextChanges(); err != nil {
			t.Fatal(err)
		}
	}
	if n := pageOps(t, d, "re"); n != 2 {
		t.Fatalf("rects %d", n)
	}
	if n := p.Annotations().Count(); n != 2 {
		t.Fatalf("annotations %d", n)
	}
}

// Right-to-left text given a new Position is drawn from it, not anchored at
// the old right edge.
func TestApplyRTLMovedStartsAtPosition(t *testing.T) {
	AddFontFile("testdata/DejaVuSans.ttf")
	defer ClearFontSources()
	d := openBytes(t, dejaVuDoc(t, "שלום עולם"))
	f := absorbOne(t, d, "שלום עולם")
	f.Text = "שלום"
	f.Position = Position{XIndent: 300, YIndent: 500}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "שלום")
	if !nearPt(g.Segments[0].Rectangle.LLX, 300) {
		t.Fatalf("left %v", g.Segments[0].Rectangle.LLX)
	}
}

// A background section is removed whole, nested marked content included, and
// nothing after it.
func TestRemoveTextBackgroundsNested(t *testing.T) {
	data := []byte("/AsposeTextBackground <</N 3>> BDC q /Span BMC 0 0 1 1 re f EMC Q EMC 5 5 2 2 re f")
	out, err := removeTextBackgrounds(data, map[int]bool{3: true})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Operator != "re" || ops[1].Operator != "f" {
		t.Fatalf("ops %+v", ops)
	}
}

// A form the caller still holds survives an edit of its only placement, nested
// forms included: placing it again draws its original text.
func TestApplyKeepsLiveForms(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	inner := d.CreateForm(200, 40)
	if err := inner.Canvas().AddText("Shared caption", TextStyle{Font: FontHelvetica, Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 40}); err != nil {
		t.Fatal(err)
	}
	outer := d.CreateForm(200, 40)
	if err := outer.Canvas().AddForm(inner, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 40}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(outer, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 640}); err != nil {
		t.Fatal(err)
	}
	absorbOne(t, d, "Shared").Text = "Edited"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(outer, Rectangle{LLX: 100, LLY: 300, URX: 300, URY: 340}); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	txt, err := mustPage(t, r).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "Edited caption") || !strings.Contains(txt, "Shared caption") {
		t.Fatalf("text %q", txt)
	}
}

// A failure after planning (here while drawing) also takes out the fallback
// font planning loaded: the document is as it was.
func TestApplyRollbackDropsPlanningFont(t *testing.T) {
	AddFontFile("testdata/DejaVuSans.ttf")
	defer ClearFontSources()
	src := dejaVuDoc(t, "Hello world")
	d := openBytes(t, src)
	before := writeBytes(t, openBytes(t, src))
	fonts, next := len(d.embeddedFonts), d.nextID
	// Both found before any edit: a visit applies pending edits first.
	a := absorbOne(t, d, "Hello")
	b := absorbOne(t, d, "world")
	a.Text = "Привет" // needs DejaVu found by name
	seg := NewTextSegment("x")
	seg.TextState.Rotation = 30 // rotated multi-segment: drawing fails
	b.Segments = append(b.Segments, seg)
	if err := d.ApplyTextChanges(); err == nil {
		t.Fatal("expected an error")
	}
	if len(d.embeddedFonts) != fonts || d.nextID != next {
		t.Fatalf("fonts %d->%d next %d->%d", fonts, len(d.embeddedFonts), next, d.nextID)
	}
	absorbed := d.boundFragments
	for _, f := range absorbed {
		f.Text = f.bind.snap.text
		f.Segments = f.Segments[:len(f.bind.snap.segments)]
	}
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("failed apply left changes")
	}
}

// A background section with no EMC is an error, never a cut stream.
func TestRemoveTextBackgroundsUnterminated(t *testing.T) {
	data := []byte("/AsposeTextBackground <</N 3>> BDC q 0 0 1 1 re f Q BT /F1 12 Tf (x) Tj ET")
	if _, err := removeTextBackgrounds(data, map[int]bool{3: true}); err == nil {
		t.Fatal("expected an error")
	}
}

// backgroundDoc draws "boxed" over a yellow box: as a text background (marked
// section) or as a plain rectangle.
func backgroundDoc(t *testing.T, marked bool) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("boxed")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontHelvetica, FontSize: 12}
	if marked {
		f.TextState.BackgroundColor = &Color{1, 1, 0, 1}
	}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	if !marked {
		if err := p.prependToContentStream([]byte("q 1 1 0 rg " + rectOps(f.Segments[0].Rectangle) + " f Q\n")); err != nil {
			t.Fatal(err)
		}
	}
	return openBytes(t, writeBytes(t, d))
}

func rectOps(r Rectangle) string {
	return formatFloat(r.LLX) + " " + formatFloat(r.LLY) + " " + formatFloat(r.URX-r.LLX) + " " + formatFloat(r.URY-r.LLY) + " re"
}

// The background marker costs no conformance: the PDF/UA and PDF/A reports
// of a document with it equal those of one with a plain box.
func TestTextBackgroundMarkerConformance(t *testing.T) {
	rules := func(marked bool) (ua, pa []string) {
		d := backgroundDoc(t, marked)
		if !marked {
			if pageOps(t, d, "BDC") != 0 {
				t.Fatal("plain fixture has a section")
			}
		} else if pageOps(t, d, "BDC") != 1 {
			t.Fatal("marked fixture has no section")
		}
		if _, err := d.AutoTag(AutoTaggingSettings{}); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ConvertToPDFUA(); err != nil {
			t.Fatal(err)
		}
		for _, i := range d.ValidatePDFUA().Issues {
			ua = append(ua, i.Rule)
		}
		e := backgroundDoc(t, marked)
		if _, err := e.ConvertToPDFA(PDFA2B); err != nil {
			t.Fatal(err)
		}
		for _, i := range e.ValidatePDFA(PDFA2B).Issues {
			pa = append(pa, i.Rule)
		}
		return ua, pa
	}
	ua0, pa0 := rules(false)
	ua1, pa1 := rules(true)
	if strings.Join(ua0, ",") != strings.Join(ua1, ",") || strings.Join(pa0, ",") != strings.Join(pa1, ",") {
		t.Fatalf("PDF/UA %v vs %v, PDF/A %v vs %v", ua0, ua1, pa0, pa1)
	}
	if len(pa1) != 0 {
		t.Fatalf("PDF/A-2b issues %v", pa1)
	}
	t.Logf("PDF/UA issues (both) %v", ua1)
}

// A form handle from Page.Forms keeps the forms nested in it, which no handle
// names, through an edit of its only placement.
func TestApplyKeepsNestedFormsOfLiveForm(t *testing.T) {
	d := openBytes(t, nestedFormDoc(t, 1))
	p := mustPage(t, d)
	forms := p.Forms()
	if len(forms) != 1 {
		t.Fatalf("forms %d", len(forms))
	}
	absorbOne(t, d, "Shared").Text = "Edited"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(forms[0], Rectangle{LLX: 100, LLY: 200, URX: 300, URY: 240}); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	txt, err := mustPage(t, r).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "Edited caption") || !strings.Contains(txt, "Shared caption") {
		t.Fatalf("text %q", txt)
	}
}
