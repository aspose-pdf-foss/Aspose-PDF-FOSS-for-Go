// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// drawRTLAndReopen draws one line of text with the bundled DejaVu Sans, saves
// the document and opens it again, so what is checked is what a reader of the
// finished file sees.
func drawRTLAndReopen(t *testing.T, text string) *Document {
	t.Helper()
	doc := NewDocument(500, 120)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	page, _ := doc.Page(1)
	if err := page.AddText(text, TextStyle{Font: font, Size: 20},
		Rectangle{LLX: 20, LLY: 30, URX: 480, URY: 90}); err != nil {
		t.Fatalf("add text: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

// Right-to-left text is drawn in visual order, so extraction has to put it
// back the way it was typed — otherwise every Arabic and Hebrew word reads
// backwards.
func TestExtractTextRestoresLogicalOrder(t *testing.T) {
	for _, text := range []string{
		"\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064a\u0643\u0645",       // as-salamu alaykum
		"\u0645\u0631\u062d\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645", // marhaba bil-alam
		"\u05e9\u05dc\u05d5\u05dd \u05e2\u05d5\u05dc\u05dd",                         // shalom olam
		"\u0627\u0644\u0639\u062f\u062f 1234",                                       // an RTL run with Western digits
		// Mixed direction: the line is mostly Arabic, so it reads right to
		// left and its Latin tail belongs at the end.
		"العدد 1234 (مثال) و ABC",
		// The other way round: mostly Latin, with an Arabic word at the end.
		"Invoice 2026 مرحبا",
	} {
		doc := drawRTLAndReopen(t, text)
		page, _ := doc.Page(1)
		got, err := page.ExtractText()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(got) != text {
			t.Errorf("extracted %q, want %q", got, text)
		}
	}
}

// Left-to-right text must come back untouched — the reordering only applies to
// lines that actually carry right-to-left characters.
func TestExtractTextLeavesLTRAlone(t *testing.T) {
	const text = "Invoice 2026 (paid)"
	doc := drawRTLAndReopen(t, text)
	page, _ := doc.Page(1)
	got, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != text {
		t.Errorf("extracted %q, want %q", got, text)
	}
}

// Searching for a right-to-left word finds it as typed, and the match's
// rectangle lands on the drawn glyphs.
func TestSearchTextFindsRTLWord(t *testing.T) {
	const text = "\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064a\u0643\u0645"
	const word = "\u0639\u0644\u064a\u0643\u0645" // alaykum — the second word
	doc := drawRTLAndReopen(t, text)
	page, _ := doc.Page(1)
	matches, err := page.SearchText(word)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d matches for %q, want 1", len(matches), word)
	}
	r := matches[0].Rect
	if r.LLX < 20 || r.URX > 480 || r.URY <= r.LLY || r.URX <= r.LLX {
		t.Errorf("match rectangle %+v is not on the drawn line", r)
	}
}
