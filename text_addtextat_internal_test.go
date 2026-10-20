// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// TestAddTextAtDoesNotCollapseLeadingOrTrailingWhitespace is the
// internal-package companion to TestAddTextAtPreservesWhitespace
// (text_addtextat_test.go): ExtractText right-trims every line and
// TrimSpaces its final result regardless of how the text was drawn, so it
// cannot prove leading/trailing whitespace survived AddTextAt — only a
// direct look at the content-stream bytes can. AddText's own wrapText
// would drop leading/trailing whitespace entirely via strings.Fields;
// AddTextAt must not.
func TestAddTextAtDoesNotCollapseLeadingOrTrailingWhitespace(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const text = "  a  " // two leading + two trailing spaces around a single letter
	if err := p.AddTextAt(text, TextStyle{Size: 12}, 100, 200); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}

	content, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Standard-14 fonts encode via encodeFn's "(...)" literal-string form
	// (resolveFontForPage), so the drawn text appears byte-for-byte as a
	// parenthesized PDF string operand.
	want := "(" + text + ")"
	if !strings.Contains(string(content), want) {
		t.Errorf("content stream does not contain %q — leading/trailing whitespace was not preserved.\ncontent:\n%s", want, content)
	}
}

// TestAddTextAtEmitsNoClipOperator confirms AddTextAt never writes a
// clipping path ("W n", ISO 32000-1 §8.5.4) — unlike renderTextInBuilder's
// shared machinery (used by AddText), which always clips to its rect.
// AddTextAt takes no rectangle at all, so there is nothing to clip to.
func TestAddTextAtEmitsNoClipOperator(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Unclipped", TextStyle{Size: 12}, 100, 200); err != nil {
		t.Fatal(err)
	}
	content, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "W n") {
		t.Errorf("content stream contains a clip operator (\"W n\"), want none:\n%s", content)
	}
}

// TestAddTextAtUnderlineAndStrikethrough confirms TextStyle.Underline and
// Strikethrough each draw exactly one filled decoration rectangle (the
// same mechanism renderTextInBuilder uses for AddText), and that neither
// appears when the corresponding flag is false.
func TestAddTextAtUnderlineAndStrikethrough(t *testing.T) {
	countREf := func(content []byte) int {
		return strings.Count(string(content), "re\nf\n") + strings.Count(string(content), " re f\n")
	}

	run := func(style TextStyle) (int, error) {
		doc := NewDocumentFromFormat(PageFormatA4)
		p, err := doc.Page(1)
		if err != nil {
			return 0, err
		}
		if err := p.AddTextAt("Line", style, 100, 200); err != nil {
			return 0, err
		}
		content, err := p.contentStreams()
		if err != nil {
			return 0, err
		}
		return countREf(content), nil
	}

	plain, err := run(TextStyle{Size: 12})
	if err != nil {
		t.Fatal(err)
	}
	if plain != 0 {
		t.Errorf("plain style: %d filled rects, want 0", plain)
	}

	underlined, err := run(TextStyle{Size: 12, Underline: true})
	if err != nil {
		t.Fatal(err)
	}
	if underlined != plain+1 {
		t.Errorf("Underline: %d filled rects, want %d", underlined, plain+1)
	}

	struck, err := run(TextStyle{Size: 12, Strikethrough: true})
	if err != nil {
		t.Fatal(err)
	}
	if struck != plain+1 {
		t.Errorf("Strikethrough: %d filled rects, want %d", struck, plain+1)
	}

	both, err := run(TextStyle{Size: 12, Underline: true, Strikethrough: true})
	if err != nil {
		t.Fatal(err)
	}
	if both != plain+2 {
		t.Errorf("Underline+Strikethrough: %d filled rects, want %d", both, plain+2)
	}
}

// TestAddTextAtInvisibleSetsRenderMode3 confirms Invisible emits text
// rendering mode 3 (ISO 32000-1 §9.3.6) and suppresses Underline, matching
// renderTextInBuilder's AddText behavior.
func TestAddTextAtInvisibleSetsRenderMode3(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Hidden", TextStyle{Size: 12, Invisible: true, Underline: true}, 100, 200); err != nil {
		t.Fatal(err)
	}
	content, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "3 Tr") {
		t.Error("Invisible: content stream missing \"3 Tr\"")
	}
	if strings.Contains(s, "re\nf\n") || strings.Contains(s, " re f\n") {
		t.Error("Invisible: underline decoration must be suppressed, but a filled rect was found")
	}
}

// TestAddTextAtBehindPrepends confirms TextStyle.Behind prepends this
// call's content before whatever the page already has, the same way
// AddText's Behind does.
func TestAddTextAtBehindPrepends(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10},
		ShapeStyle{FillColor: &Color{A: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Behind", TextStyle{Size: 12, Behind: true}, 100, 200); err != nil {
		t.Fatal(err)
	}
	content, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	rectIdx := strings.Index(s, "re f")
	textIdx := strings.Index(s, "Behind")
	if rectIdx == -1 || textIdx == -1 {
		t.Fatalf("expected both the rectangle fill and the text operand in the content stream, got:\n%s", s)
	}
	if textIdx >= rectIdx {
		t.Errorf("Behind text (at byte %d) was not prepended before the rectangle (at byte %d)", textIdx, rectIdx)
	}
}
