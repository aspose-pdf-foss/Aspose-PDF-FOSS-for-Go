// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// GitHub issue #14: AddText wraps shaped text by unshaped widths.
//
// wrapText (and the other callers that measure text before handing it to
// AddText) used to measure a candidate line by summing each rune's
// unshaped advance, even for a font that OpenType-shapes (GSUB/GPOS).
// Kerning/ligatures then make the actual DRAWN line narrower or wider than
// the width used to decide the wrap, causing either an early wrap (a line
// that would fit once shaped gets split anyway) or an overflow (a line
// that fits by the unshaped sum draws wider than its rectangle once
// shaped, clipping the end). See lineMeasurer in text_add.go for the fix.
//
// dejaVuFont loads testdata/DejaVuSans.ttf, which has real GSUB/GPOS
// kerning (fontHasShaping true), into a fresh document.
func dejaVuFont(t *testing.T) (*Document, Font) {
	t.Helper()
	doc := NewDocument(595, 842)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("load DejaVuSans.ttf: %v", err)
	}
	if !fontHasShaping(f.(*embeddedFont).ttf) {
		t.Fatal("testdata/DejaVuSans.ttf unexpectedly has no GSUB/GPOS tables")
	}
	return doc, f
}

// TestAddTextShapedFontFitsOneLine is the issue's own reproduction: "AV" at
// 40pt measures 54.73pt by unshaped per-rune advances but only 52.17pt once
// kerned/shaped. In a rectangle just wide enough for the SHAPED width, the
// text must draw on one line, not wrap to "A\nV".
func TestAddTextShapedFontFitsOneLine(t *testing.T) {
	doc, f := dejaVuFont(t)
	page, _ := doc.Page(1)
	style := TextStyle{Font: f, Size: 40}
	rect := Rectangle{LLX: 50, LLY: 600, URX: 50 + 52.18, URY: 700}
	if err := page.AddText("AV", style, rect); err != nil {
		t.Fatalf("add text: %v", err)
	}
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if got != "AV" {
		t.Errorf("ExtractText() = %q, want %q (text wrapped when it should fit on one line)", got, "AV")
	}
}

// TestAddTextShapedFontOverflowForcesSplit is the issue's overflow
// direction: two glyphs that kern WIDER than their unshaped advances sum
// to (DejaVu Sans's "AA": 54.73pt unshaped, 55.84pt shaped) must still be
// split so the drawn result never exceeds the rectangle — even though "AA"
// is a single word with no space to break at.
func TestAddTextShapedFontOverflowForcesSplit(t *testing.T) {
	doc, f := dejaVuFont(t)
	page, _ := doc.Page(1)
	style := TextStyle{Font: f, Size: 40}
	// Just enough for the UNSHAPED sum of "AA" (54.727pt) — the old bug
	// would accept this as one line and draw it 1.1pt wider than the box.
	rect := Rectangle{LLX: 50, LLY: 600, URX: 50 + 54.73, URY: 700}
	if err := page.AddText("AA", style, rect); err != nil {
		t.Fatalf("add text: %v", err)
	}
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if got != "A\nA" {
		t.Errorf("ExtractText() = %q, want %q (single AA line would overflow the rect once shaped)", got, "A\nA")
	}
}

// TestWrapTextShapedWordFastEstimateIsShaped checks the lineMeasurer
// primitive directly: a shaped font's per-word fast estimate must be the
// font's actual shaped advance, not the unshaped per-rune sum (the root
// cause of the bug).
func TestWrapTextShapedWordFastEstimateIsShaped(t *testing.T) {
	_, f := dejaVuFont(t)
	ttf := f.(*embeddedFont).ttf
	unshapedWidth := func(r rune) float64 {
		gid := ttf.glyphID(r)
		if int(gid) >= len(ttf.glyphWidths) {
			return 0
		}
		return float64(ttf.glyphWidths[gid]) / float64(ttf.unitsPerEm) * 40
	}
	m := newShapedLineMeasurer(ttf, 40, 0, false, unshapedWidth)
	gotAV := m.word("AV")
	wantAV := shapeLine(ttf, "AV", 0, false).width(40)
	if gotAV != wantAV {
		t.Errorf("m.word(%q) = %v, want the shaped width %v", "AV", gotAV, wantAV)
	}
	if unshaped := measureString("AV", unshapedWidth); gotAV == unshaped {
		t.Errorf("m.word(%q) = %v equals the unshaped sum %v; the fast estimate must use shaping", "AV", gotAV, unshaped)
	}
}

// TestWrapTextUnshapedFontByteIdentical pins that an unshaped font (no
// GSUB/GPOS — Standard-14 here) keeps exactly the pre-fix wrapping: the
// measurer's word/space/exact must reduce to the plain per-rune sum, with
// no exact-verify/backoff ever firing (there is nothing to disagree with).
func TestWrapTextUnshapedFontByteIdentical(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	width := func(r rune) float64 {
		code, ok := winAnsiEncodeRune(r)
		if !ok {
			code = byte('?')
		}
		return widths[code] / 1000.0 * 12
	}
	m := newLineMeasurer(width)
	if m.shaper != nil {
		t.Fatal("newLineMeasurer must never set a shaper")
	}
	got := wrapText("The quick brown fox jumps over the lazy dog", m, 90)
	want := []string{"The quick brown", "fox jumps over", "the lazy dog"}
	if len(got) != len(want) {
		t.Fatalf("wrapText = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestWrapTextShapedNeverExceedsRectWidth is the fix's correctness
// invariant, checked broadly rather than against one specific kerning
// pair: for a variety of maxWidth values, every line wrapText returns for
// a shaped font must have a real (re-shaped) drawn width that does not
// exceed maxWidth — the guarantee the GitHub issue asked for ("no line is
// drawn wider than its rectangle").
func TestWrapTextShapedNeverExceedsRectWidth(t *testing.T) {
	_, f := dejaVuFont(t)
	ttf := f.(*embeddedFont).ttf
	const size = 18.0
	unshapedWidth := func(r rune) float64 {
		gid := ttf.glyphID(r)
		if int(gid) >= len(ttf.glyphWidths) {
			return 0
		}
		return float64(ttf.glyphWidths[gid]) / float64(ttf.unitsPerEm) * size
	}
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 8) +
		"AVATAR Wally Toto AWAY Victory Kerning Overflow Tiny WAWA VAVA."

	for _, maxWidth := range []float64{40, 60, 80, 100, 150, 220, 400} {
		m := newShapedLineMeasurer(ttf, size, 0, false, unshapedWidth)
		lines := wrapText(text, m, maxWidth)
		for i, line := range lines {
			if line == "" {
				continue
			}
			w := shapeLine(ttf, line, 0, false).width(size)
			if w > maxWidth+1e-6 {
				t.Errorf("maxWidth=%v line %d %q: shaped width %v exceeds maxWidth", maxWidth, i, line, w)
			}
		}
		// Every word of the input must still be present, in order (no
		// silent content loss from the exact-verify backoff). Skipped at
		// the narrowest widths, where some words are forcibly split
		// (breakWordMeasured) — intended behavior, just not
		// word-for-word reassembly.
		if maxWidth >= 150 {
			if strings.Join(strings.Fields(strings.Join(lines, " ")), " ") != strings.Join(strings.Fields(text), " ") {
				t.Errorf("maxWidth=%v: wrapped words do not reassemble the input text", maxWidth)
			}
		}
	}
}

// TestMeasureTextShapedMatchesAddText checks that measureText (used by
// table row-height sizing, Flow boxes and tagged lists) predicts the same
// line count AddText actually draws for a shaped font — the two must
// agree, or a caller that sized a box from measureText would clip the
// extra/fewer lines AddText draws.
func TestMeasureTextShapedMatchesAddText(t *testing.T) {
	doc, f := dejaVuFont(t)
	page, _ := doc.Page(1)
	style := TextStyle{Font: f, Size: 40}
	width := 52.18

	lines, _, err := measureText("AV", style, width)
	if err != nil {
		t.Fatalf("measureText: %v", err)
	}
	if lines != 1 {
		t.Errorf("measureText(%q) = %d lines, want 1 (matching AddText, which fits it on one shaped line)", "AV", lines)
	}

	rect := Rectangle{LLX: 50, LLY: 600, URX: 50 + width, URY: 700}
	if err := page.AddText("AV", style, rect); err != nil {
		t.Fatalf("add text: %v", err)
	}
	layout, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatalf("extract layout: %v", err)
	}
	if len(layout) != lines {
		t.Errorf("AddText drew %d lines, measureText predicted %d", len(layout), lines)
	}
}

// TestFlowParagraphShapedFontMatchesAddTextWrapPoints exercises the Flow
// layer (flow.go's flowText), which pre-wraps a paragraph ITSELF (to decide
// pagination) and then hands each already-wrapped chunk to AddText, which
// re-wraps that chunk's "\n"-joined lines internally too. If Flow's own
// pre-wrap measurement disagreed with what AddText draws for a shaped
// font, AddText could split a chunk line Flow counted as one into two,
// silently overflowing the fixed-height band Flow reserved for it — not
// detectable via ExtractText's flattened word stream (the extra line's
// glyphs are still emitted, just clipped/overlapping visually), so this
// checks the actual LINE COUNT AddText drew via ExtractTextWithLayout
// against the line count an independently shaped-aware wrapText predicts
// for the same text at the same content width.
//
// "AV" repeated: DejaVu Sans kerns it from 54.73pt (unshaped) down to
// 52.17pt shaped, so at a tuned column width more instances legitimately
// fit per line when measured correctly (8 lines) than the old unshaped
// sum would allow (10 lines) — see TestProbeFlowAV-style reasoning in the
// bug investigation.
func TestFlowParagraphShapedFontMatchesAddTextWrapPoints(t *testing.T) {
	doc, f := dejaVuFont(t)
	ef := f.(*embeddedFont)
	rawWidth := func(r rune) float64 {
		gid := ef.ttf.glyphID(r)
		if int(gid) >= len(ef.ttf.glyphWidths) {
			return 0
		}
		return float64(ef.ttf.glyphWidths[gid]) / float64(ef.ttf.unitsPerEm) * 40
	}
	const contentW = 250.0
	text := strings.TrimSpace(strings.Repeat("AV ", 30))

	wantLines := len(wrapText(text, newShapedLineMeasurer(ef.ttf, 40, 0, false, rawWidth), contentW))

	flow := doc.NewFlow(FlowOptions{
		Format:     PageFormat{Width: contentW + 20, Height: 2000}, // tall enough to stay on one page
		MarginLeft: 10, MarginRight: 10, MarginTop: 20, MarginBottom: 20,
	})
	flow.AddParagraph(text, TextStyle{Font: f, Size: 40})
	if _, err := flow.Render(); err != nil {
		t.Fatalf("flow render: %v", err)
	}

	layout, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	gotLines, err := layout.ExtractTextWithLayout()
	if err != nil {
		t.Fatalf("extract layout: %v", err)
	}
	if len(gotLines) != wantLines {
		t.Errorf("AddText drew %d lines via Flow, want %d (matching wrapText's own shaped-width prediction for the same content width)", len(gotLines), wantLines)
	}
}

func TestLineMeasurerForFontPlainFontHasNoShaper(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	width := func(r rune) float64 {
		code, ok := winAnsiEncodeRune(r)
		if !ok {
			code = byte('?')
		}
		return widths[code] / 1000.0 * 12
	}
	m := lineMeasurerForFont(FontHelvetica, 12, "hello world", false, width)
	if m.shaper != nil {
		t.Error("lineMeasurerForFont must not set a shaper for a Standard-14 font")
	}
}

func TestLineMeasurerForFontShapedFontSetsShaper(t *testing.T) {
	_, f := dejaVuFont(t)
	ttf := f.(*embeddedFont).ttf
	width := func(r rune) float64 { return float64(ttf.glyphAdvance(ttf.glyphID(r))) / float64(ttf.unitsPerEm) * 12 }
	m := lineMeasurerForFont(f, 12, "hello world", false, width)
	if m.shaper == nil {
		t.Error("lineMeasurerForFont must set a shaper for a font with GSUB/GPOS tables")
	}
}

// lipsumWords is a vocabulary of ~90 distinct Latin words, used to build a
// long paragraph that isn't trivially cache-friendly (unlike one repeated
// sentence) — closer to a real document, and to the ~1650-word paragraph
// the GitHub issue's own benchmark used.
var lipsumWords = strings.Fields(`Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod
tempor incididunt ut labore et dolore magna aliqua Ut enim ad minim veniam
quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
consequat Duis aute irure dolor in reprehenderit in voluptate velit esse
cillum dolore eu fugiat nulla pariatur Excepteur sint occaecat cupidatat
non proident sunt in culpa qui officia deserunt mollit anim id est laborum
Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium
doloremque laudantium totam rem aperiam eaque ipsa quae ab illo inventore
veritatis et quasi architecto beatae vitae dicta sunt explicabo`)

func buildLipsumParagraph(words int) string {
	var b strings.Builder
	for i := 0; i < words; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(lipsumWords[i%len(lipsumWords)])
	}
	return b.String()
}

// BenchmarkAddTextLongParagraphShaped guards against a pathological
// slowdown for shaped fonts: naively re-shaping a growing wrap candidate
// from scratch for every word is reported (GitHub issue #14) to cost
// several times an unshaped paragraph of the same length. The per-word
// cache + exact-verify-once-per-line design here keeps shaped-font
// wrapping to a bounded, roughly linear-in-content cost rather than
// quadratic — compare against BenchmarkAddTextLongParagraphUnshaped, which
// this fix must leave untouched.
func BenchmarkAddTextLongParagraphShaped(b *testing.B) {
	doc := NewDocument(595, 842)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		b.Fatal(err)
	}
	text := buildLipsumParagraph(1650)
	style := TextStyle{Font: f, Size: 10}
	rect := Rectangle{LLX: 50, LLY: 50, URX: 545, URY: 790}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, _ := doc.Page(1)
		if err := page.AddText(text, style, rect); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAddTextLongParagraphUnshaped(b *testing.B) {
	doc := NewDocument(595, 842)
	text := buildLipsumParagraph(1650)
	style := TextStyle{Font: FontHelvetica, Size: 10}
	rect := Rectangle{LLX: 50, LLY: 50, URX: 545, URY: 790}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, _ := doc.Page(1)
		if err := page.AddText(text, style, rect); err != nil {
			b.Fatal(err)
		}
	}
}

// TestAddTOCShapedFontTitleNotOverTruncated checks toc.go's drawTOCRow fix:
// a TOC title's "does it need truncating" decision must use the entry
// font's SHAPED width when it shapes (GitHub issue #14's "TOC entries"
// scope), not the plain per-rune sum — otherwise a title that actually
// fits once kerned gets an unnecessary "..." ellipsis.
func TestAddTOCShapedFontTitleNotOverTruncated(t *testing.T) {
	doc, f := dejaVuFont(t)
	page, _ := doc.Page(1)
	// "AVAVAVAVAV" (5x "AV") at 40pt: 273.63pt unshaped, 250.60pt shaped
	// (DejaVu Sans kerns A-V pairs narrower). titleMaxW = rectWidth-6 = 260
	// sits between the two: the old unshaped check would truncate, the
	// shaped width fits without truncation.
	title := strings.Repeat("AV", 5)
	entries := []TOCEntry{{Title: title, Level: 0}}
	rect := Rectangle{LLX: 50, LLY: 700, URX: 50 + 266, URY: 780}
	if _, err := page.AddTOC(entries, rect, TOCOptions{EntryStyle: TextStyle{Font: f, Size: 40}}); err != nil {
		t.Fatalf("AddTOC: %v", err)
	}
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if strings.Contains(got, "...") {
		t.Errorf("ExtractText() = %q; title was truncated with an ellipsis even though its shaped width fits the row", got)
	}
	if !strings.Contains(got, title) {
		t.Errorf("ExtractText() = %q; want the full title %q present", got, title)
	}
}
