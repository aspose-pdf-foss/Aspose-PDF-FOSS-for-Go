// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// applyDocBytes builds a page with "A Figure here" (Helvetica 12, blue) at
// (100, 600) and "second line" at (100, 500), and returns it saved.
func applyDocBytes(t *testing.T) []byte {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("A Figure here")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontHelvetica, FontSize: 12, ForegroundColor: &Color{0, 0, 1, 1}}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	g := NewTextFragment("second line")
	g.Position = Position{XIndent: 100, YIndent: 500}
	if err := NewTextBuilder(p).AppendText(g); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func openBytes(t *testing.T, b []byte) *Document {
	t.Helper()
	d, err := OpenStream(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeBytes(t *testing.T, d *Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func absorbOne(t *testing.T, d *Document, phrase string) *TextFragment {
	t.Helper()
	a := NewTextFragmentAbsorber(phrase)
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	if n := len(a.TextFragments()); n != 1 {
		t.Fatalf("%q found %d times", phrase, n)
	}
	return a.TextFragments()[0]
}

func absorbCount(t *testing.T, d *Document, phrase string) int {
	t.Helper()
	a := NewTextFragmentAbsorber(phrase)
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	return len(a.TextFragments())
}

func nearPt(a, b float64) bool { return math.Abs(a-b) < 0.5 }

func TestApplyUntouchedIsNoop(t *testing.T) {
	src := applyDocBytes(t)
	plain := writeBytes(t, openBytes(t, src))

	d := openBytes(t, src)
	a := NewTextFragmentAbsorber("")
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	if len(a.TextFragments()) == 0 {
		t.Fatal("nothing found")
	}
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(writeBytes(t, d), plain) {
		t.Fatal("an untouched absorbed document saves differently")
	}
}

func TestApplyTwice(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	f.Text = "Picture"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if !f.bind.snap.equal(snapshotOf(f)) {
		t.Fatal("snapshot not refreshed")
	}
	n := len(d.objects)
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if len(d.objects) != n {
		t.Fatal("second apply changed objects")
	}
	a, b := writeBytes(t, d), writeBytes(t, d)
	if !bytes.Equal(a, b) {
		t.Fatal("two saves differ")
	}
}

func TestApplyText(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	oldY, oldX := f.Position.YIndent, f.Position.XIndent
	f.Text = "Picture"
	f.TextState.FontSize = 8
	f.TextState.ForegroundColor = &Color{0, 1, 0, 1}

	r := openBytes(t, writeBytes(t, d))
	if absorbCount(t, r, "Figure") != 0 {
		t.Fatal("old text still found")
	}
	g := absorbOne(t, r, "Picture")
	if !nearPt(g.Position.YIndent, oldY) || !nearPt(g.Position.XIndent, oldX) {
		t.Fatalf("moved: %+v, want (%v, %v)", g.Position, oldX, oldY)
	}
	if len(g.Segments) != 1 || g.TextState.FontSize != 8 {
		t.Fatalf("size %v segments %d", g.TextState.FontSize, len(g.Segments))
	}
	if c := g.TextState.ForegroundColor; c == nil || c.G < 0.99 || c.B > 0.01 {
		t.Fatalf("colour %+v", c)
	}
	if !strings.HasPrefix(g.TextState.Font.BaseFont(), "Helvetica") {
		t.Fatalf("font %q", g.TextState.Font.BaseFont())
	}
	// The neighbours on the line stay.
	if absorbCount(t, r, "A ") != 1 || absorbCount(t, r, " here") != 1 || absorbCount(t, r, "second line") != 1 {
		t.Fatal("neighbouring text lost")
	}
	// Forensic: the removed text is in no stream of the saved file.
	if ok, _ := documentContainsBytes(r, "Figure"); ok {
		t.Fatal("old content stream with \"Figure\" kept in the file")
	}
}

func TestApplyAfterAppend(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("first ")
	f.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.Segments = append(f.Segments, NewTextSegment("second"))
	r := reopenDoc(t, d)
	g := absorbOne(t, r, "first second")
	if len(g.Segments) != 1 && len(g.Segments) != 2 {
		t.Fatalf("segments %d", len(g.Segments))
	}
	for _, s := range g.Segments {
		if s.TextState.Font.BaseFont() != "Helvetica" {
			t.Fatalf("font %q", s.TextState.Font.BaseFont())
		}
	}
	if absorbCount(t, r, "first") != 1 {
		t.Fatal("first segment drawn twice")
	}
	if f.Text != "first second" {
		t.Fatalf("text %q", f.Text)
	}
}

func TestApplyMove(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "second line")
	f.Position = Position{XIndent: 300, YIndent: 200}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "second line")
	if !nearPt(g.Position.XIndent, 300) || !nearPt(g.Position.YIndent, 200) {
		t.Fatalf("at %+v", g.Position)
	}
	if absorbCount(t, r, "Figure") != 1 {
		t.Fatal("other line touched")
	}
}

func TestApplyOnIncremental(t *testing.T) {
	src := applyDocBytes(t)
	d := openBytes(t, src)
	f := absorbOne(t, d, "Figure")
	f.Text = "Picture"
	var buf bytes.Buffer
	if _, err := d.WriteIncremental(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), src) {
		t.Fatal("not an incremental update")
	}
	r := openBytes(t, buf.Bytes())
	if absorbCount(t, r, "Picture") != 1 || absorbCount(t, r, "Figure") != 0 {
		t.Fatal("edit not in the revision")
	}
}

func TestApplyOnLinearized(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	f.Text = "Picture"
	var buf bytes.Buffer
	if _, err := d.WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, buf.Bytes())
	if absorbCount(t, r, "Picture") != 1 || absorbCount(t, r, "Figure") != 0 {
		t.Fatal("edit not in the linearized file")
	}
}

func TestApplyOnExporters(t *testing.T) {
	type exp struct {
		name  string
		write func(d *Document, w *bytes.Buffer) error
	}
	for _, e := range []exp{
		{"markdown", func(d *Document, w *bytes.Buffer) error { return d.WriteMarkdown(w) }},
		{"html", func(d *Document, w *bytes.Buffer) error { return d.WriteHTML(w, HTMLSaveOptions{Mode: HTMLModeFlow}) }},
		{"svg", func(d *Document, w *bytes.Buffer) error { return mustPage(t, d).WriteSVG(w) }},
		{"docx", func(d *Document, w *bytes.Buffer) error { return d.WriteDocx(w) }},
		{"epub", func(d *Document, w *bytes.Buffer) error { return d.WriteEpub(w) }},
		{"xlsx", func(d *Document, w *bytes.Buffer) error { return d.WriteXlsx(w, XlsxSaveOptions{Mode: XlsxFullPage}) }},
		{"render", func(d *Document, w *bytes.Buffer) error { return mustPage(t, d).RenderPNG(w, RenderOptions{DPI: 36}) }},
	} {
		t.Run(e.name, func(t *testing.T) {
			d := openBytes(t, applyDocBytes(t))
			f := absorbOne(t, d, "Figure")
			f.Text = "Picture"
			var buf bytes.Buffer
			if err := e.write(d, &buf); err != nil {
				t.Fatal(err)
			}
			if f.bind.snap.text != "Picture" {
				t.Fatal("exporter did not apply the edit first")
			}
			if absorbCount(t, d, "Picture") != 1 {
				t.Fatal("edit not on the page")
			}
		})
	}
}

// A changed segment is redrawn alone, at its place; the others keep their
// glyphs and positions.
func TestApplySegmentOnly(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	before0, before2 := f.Segments[0].Rectangle, f.Segments[2].Rectangle
	f.Segments[1].TextState.ForegroundColor = &Color{1, 0, 0, 1}
	pl, err := d.planTextEdit(f, newReplaceFonts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if pl == nil || pl.whole || len(pl.segs) != 1 || pl.segs[0] != 1 {
		t.Fatalf("plan %+v", pl)
	}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "plain bold tail")
	if len(g.Segments) != 3 {
		t.Fatalf("segments %d", len(g.Segments))
	}
	if c := g.Segments[1].TextState.ForegroundColor; c == nil || c.R < 0.99 {
		t.Fatalf("colour %+v", c)
	}
	if g.Segments[1].TextState.Font.BaseFont() != "Helvetica-Bold" {
		t.Fatalf("font %q", g.Segments[1].TextState.Font.BaseFont())
	}
	if !nearPt(g.Segments[0].Rectangle.LLX, before0.LLX) || !nearPt(g.Segments[2].Rectangle.LLX, before2.LLX) {
		t.Fatal("unchanged segments moved")
	}
	if g.Segments[2].TextState.Font.BaseFont() != "Times-Roman" {
		t.Fatalf("tail font %q", g.Segments[2].TextState.Font.BaseFont())
	}
}

// A fragment-level change applies field by field: the regular run after a
// bold one stays regular.
func TestApplyFragmentStateFieldByField(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.TextState.FontSize = 20
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "plain bold tail")
	if len(g.Segments) != 3 {
		t.Fatalf("segments %d", len(g.Segments))
	}
	want := []string{"Helvetica", "Helvetica-Bold", "Times-Roman"}
	for i, s := range g.Segments {
		if s.TextState.FontSize != 20 {
			t.Fatalf("segment %d size %v", i, s.TextState.FontSize)
		}
		if s.TextState.Font.BaseFont() != want[i] {
			t.Fatalf("segment %d font %q", i, s.TextState.Font.BaseFont())
		}
	}
}

func rotatedDocBytes(t *testing.T) []byte {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("tilted")
	f.Position = Position{XIndent: 200, YIndent: 400}
	f.TextState = TextState{Rotation: 45}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	g := NewTextFragment("level")
	g.Position = Position{XIndent: 100, YIndent: 700}
	if err := NewTextBuilder(p).AppendText(g); err != nil {
		t.Fatal(err)
	}
	return writeBytes(t, d)
}

func TestApplyNoEditRefused(t *testing.T) {
	src := rotatedDocBytes(t)
	d := openBytes(t, src)
	tilt := absorbOne(t, d, "tilted")
	level := absorbOne(t, d, "level")
	if tilt.bind.noEdit == "" {
		t.Fatal("rotated fragment editable")
	}
	// Untouched, the rotated fragment is no obstacle.
	level.Text = "flat"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}

	d = openBytes(t, src)
	tilt = absorbOne(t, d, "tilted")
	level = absorbOne(t, d, "level")
	before := writeBytes(t, openBytes(t, src))
	level.Text = "flat" // bound after tilt? order does not matter: refused first
	tilt.Text = "upright"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "tilted") || !strings.Contains(err.Error(), "rotated") {
		t.Fatalf("err %v", err)
	}
	level.Text = "level"
	tilt.Text = "tilted"
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("refused apply changed the page")
	}
}

// A failure while drawing (after the page was stripped) leaves every page as
// it was.
func TestApplyAtomicRollback(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	p1 := mustPage(t, d)
	p2, _ := d.Page(2)
	a := NewTextFragment("alpha")
	a.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p1).AppendText(a); err != nil {
		t.Fatal(err)
	}
	b := NewTextFragment("beta")
	b.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p2).AppendText(b); err != nil {
		t.Fatal(err)
	}
	before := writeBytes(t, d)
	a.Text = "ALPHA"
	seg := NewTextSegment("x")
	seg.TextState.Rotation = 30 // rotated multi-segment: drawing fails
	b.Segments = append(b.Segments, seg)
	if err := d.ApplyTextChanges(); err == nil {
		t.Fatal("expected an error")
	}
	a.Text = "alpha"
	b.Segments = b.Segments[:1]
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("failed apply left changes")
	}
}

// Text set to "" deletes the fragment.
func TestApplyEmptyDeletes(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "second line")
	f.Text = ""
	r := openBytes(t, writeBytes(t, d))
	if absorbCount(t, r, "second line") != 0 || absorbCount(t, r, "Figure") != 1 {
		t.Fatal("deletion")
	}
}

// Edits to a fragment drawn by AppendText (not found) inherit as authored.
func TestApplyAuthoredStateChange(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("drawn")
	f.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.TextState.FontSize = 30
	f.TextState.Font = FontCourier
	r := reopenDoc(t, d)
	g := absorbOne(t, r, "drawn")
	if g.TextState.FontSize != 30 || g.TextState.Font.BaseFont() != "Courier" || !nearPt(g.Position.XIndent, 100) {
		t.Fatalf("state %+v at %+v", g.TextState, g.Position)
	}
	if absorbCount(t, r, "drawn") != 1 {
		t.Fatal("drawn twice")
	}
}

// A paragraph line refuses edits; an untouched one is fine.
func TestApplyParagraphRefused(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	tp := NewTextParagraph()
	tp.Rectangle = Rectangle{LLX: 100, LLY: 400, URX: 400, URY: 700}
	line := NewTextFragment("a paragraph line")
	tp.AppendLine(line)
	if err := NewTextBuilder(p).AppendParagraph(tp); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	line.Text = "changed"
	if err := d.ApplyTextChanges(); err == nil || !strings.Contains(err.Error(), "paragraph") {
		t.Fatalf("err %v", err)
	}
}

// Setting Text on a found fragment of several segments draws one segment in
// the first segment's state (Aspose's behaviour).
func TestApplyTextCollapsesSegments(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.Text = "all new words"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	if len(f.Segments) != 1 || f.Segments[0].Text != "all new words" {
		t.Fatalf("segments %d", len(f.Segments))
	}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "all new words")
	if len(g.Segments) != 1 || g.Segments[0].TextState.Font.BaseFont() != "Helvetica" {
		t.Fatalf("segments %d font %q", len(g.Segments), g.Segments[0].TextState.Font.BaseFont())
	}
	if absorbCount(t, r, "bold") != 0 || absorbCount(t, r, "tail") != 0 {
		t.Fatal("old segments kept")
	}
}

// Changing one segment's text (Text left as it was) redraws the fragment.
func TestApplySegmentText(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.Segments[1].Text = "BOLD"
	r := openBytes(t, writeBytes(t, d))
	if absorbCount(t, r, "plain BOLD tail") != 1 || absorbCount(t, r, "bold") != 0 {
		t.Fatal("segment text not applied")
	}
	if f.Text != "plain BOLD tail" {
		t.Fatalf("fragment text %q", f.Text)
	}
}

// Text set together with a change to the first segment's own state: the
// single segment takes the first segment's state, not the fragment's.
func TestApplyTextTakesFirstSegmentState(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.Segments[0].TextState.ForegroundColor = &Color{1, 0, 0, 1}
	f.Text = "red words"
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "red words")
	if c := g.TextState.ForegroundColor; c == nil || c.R < 0.99 {
		t.Fatalf("colour %+v", c)
	}
}

// The snapshot records the fragment's own state.
func TestSnapshotRecordsFragmentState(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.TextState.FontSize = 30
	if f.bind.snap.equal(snapshotOf(f)) {
		t.Fatal("fragment state change not in the snapshot")
	}
}
