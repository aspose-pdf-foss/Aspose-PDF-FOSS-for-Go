// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestReplaceTextNonLatinWithFont: a replacement outside the Standard-14
// face's encoding is drawn in ReplaceOptions.Font, not written as '?'
// (pdf-go-o4py).
func TestReplaceTextNonLatinWithFont(t *testing.T) {
	doc := makeTextDoc(t, "Hello world")
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	if n, err := p.ReplaceText("world", "мир", pdf.ReplaceOptions{Font: font}); err != nil || n != 1 {
		t.Fatalf("ReplaceText = %d, %v", n, err)
	}
	got := extractFirst(t, doc)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "мир") || strings.ContainsAny(got, "?") {
		t.Errorf("text %q", got)
	}
	// Drawn in the font given, not one found by name.
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range lines {
		for _, f := range l.Fragments {
			if strings.Contains(f.Text, "мир") {
				found = true
				if !strings.Contains(f.FontName, "DejaVu") {
					t.Errorf("replacement drawn in %q, want DejaVu Sans", f.FontName)
				}
			}
		}
	}
	if !found {
		t.Error("no fragment holds the replacement")
	}
}

// TestReplaceTextRightToLeftEndsAtMatch: a right-to-left replacement is
// shaped and ends where the match ended.
func TestReplaceTextRightToLeftEndsAtMatch(t *testing.T) {
	doc := makeTextDoc(t, "Hello world")
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	before, err := p.SearchText("world")
	if err != nil || len(before) != 1 {
		t.Fatalf("SearchText: %d, %v", len(before), err)
	}
	if _, err := p.ReplaceText("world", "سلام", pdf.ReplaceOptions{Font: font}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	after, err := rp.SearchText("سلام")
	if err != nil || len(after) != 1 {
		t.Fatalf("Arabic replacement not found: %d, %v; text %q", len(after), err, extractFirst(t, doc))
	}
	if d := math.Abs(after[0].Rect.URX - before[0].Rect.URX); d > 1 {
		t.Errorf("replacement ends at %.2f, match ended at %.2f", after[0].Rect.URX, before[0].Rect.URX)
	}
}

// TestReplaceTextNoFontCovers: with no font able to show a character the
// call fails naming it, and the page is left as it was.
func TestReplaceTextNoFontCovers(t *testing.T) {
	doc := makeTextDoc(t, "Hello world")
	p, _ := doc.Page(1)
	_, err := p.ReplaceText("world", "x\U0010FFFD")
	if err == nil || !strings.Contains(err.Error(), "U+10FFFD") {
		t.Fatalf("err = %v, want one naming U+10FFFD", err)
	}
	if got := extractFirst(t, doc); !strings.Contains(got, "Hello world") {
		t.Errorf("page changed: %q", got)
	}
}

// TestReplaceTextKeepsFontLoadedForLater: a font loaded before a replacement
// that does not use it stays usable for the next one; ReplaceText's sweep of
// the replaced content deleted it, and text drawn with it afterwards pointed
// at nothing (pdf-go-2tdw).
func TestReplaceTextKeepsFontLoadedForLater(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}
	if err := p.AddTextAt("first word", style, 72, 700); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("second word", style, 72, 680); err != nil {
		t.Fatal(err)
	}
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := doc.ReplaceText("first", "one"); err != nil || n != 1 {
		t.Fatalf("first replacement: %d, %v", n, err)
	}
	if n, err := doc.ReplaceText("second", "второе", pdf.ReplaceOptions{Font: font}); err != nil || n != 1 {
		t.Fatalf("second replacement: %d, %v", n, err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	text, _ := rp.ExtractText()
	if !strings.Contains(text, "второе word") || !strings.Contains(text, "one word") {
		t.Errorf("text after save: %q", text)
	}
}
