// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// paragraphPage lays out a bold heading, a three-line paragraph, a line of
// spaces and a second paragraph, all at one size and one line pitch.
func paragraphPage(t *testing.T) *pdf.Page {
	_, p := paragraphDoc(t)
	return p
}

func paragraphDoc(t *testing.T) (*pdf.Document, *pdf.Page) {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	regular := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 11}
	bold := pdf.TextStyle{Font: pdf.FontTimesBold, Size: 11}
	y := 700.0
	for _, l := range []struct {
		s     string
		style pdf.TextStyle
	}{
		{"2.2 Dynamic Server", bold},
		{"The first paragraph tells of one thing", regular},
		{"and goes on about it for a while", regular},
		{"before it comes to an end.", regular},
		{" ", regular},
		{"The second paragraph is about another.", regular},
	} {
		if err := p.AddTextAt(l.s, l.style, 72, y); err != nil {
			t.Fatal(err)
		}
		y -= 18
	}
	return doc, p
}

func paragraphTextsOf(t *testing.T, p *pdf.Page) (pdf.PageMarkup, []string) {
	t.Helper()
	m, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range m.Sections {
		for _, par := range s.Paragraphs {
			if par.Text != "" {
				out = append(out, par.Text)
			}
		}
	}
	return m, out
}

// TestParagraphsSplitAtBlankLineAndHeading: a line of spaces and a bold line
// followed by regular text end a paragraph, though the line pitch is even
// (pdf-go-tjxp).
func TestParagraphsSplitAtBlankLineAndHeading(t *testing.T) {
	_, got := paragraphTextsOf(t, paragraphPage(t))
	want := []string{
		"2.2 Dynamic Server",
		"The first paragraph tells of one thing and goes on about it for a while before it comes to an end.",
		"The second paragraph is about another.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("paragraphs\n %q\nwant\n %q", got, want)
	}
}

// setParagraph replaces the paragraph opening with opens, saves, reopens
// and returns the reopened page's paragraphs and text.
func setParagraph(t *testing.T, opens, text string) ([]string, string, *pdf.Page) {
	t.Helper()
	doc, p := paragraphDoc(t)
	m, _ := paragraphTextsOf(t, p)
	var target *pdf.MarkupParagraph
	for si := range m.Sections {
		for pi := range m.Sections[si].Paragraphs {
			if strings.HasPrefix(m.Sections[si].Paragraphs[pi].Text, opens) {
				target = &m.Sections[si].Paragraphs[pi]
			}
		}
	}
	if target == nil {
		t.Fatalf("no paragraph opens with %q", opens)
	}
	if err := target.SetText(text); err != nil {
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
	_, got := paragraphTextsOf(t, rp)
	s, _ := rp.ExtractText()
	return got, s, rp
}

// TestMarkupParagraphSetText: the paragraph's text is replaced in place,
// wrapped to its width from its first baseline and left edge — the heading
// above and the paragraph below stay — and is found again as that paragraph
// after a save; a longer text runs on below (pdf-go-tjxp).
func TestMarkupParagraphSetText(t *testing.T) {
	newText := "A replacement that wraps over two lines of the paragraph."
	got, s, rp := setParagraph(t, "The first", newText)
	want := []string{"2.2 Dynamic Server", newText, "The second paragraph is about another."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("paragraphs after SetText %q, want %q", got, want)
	}
	if strings.Contains(s, "first paragraph") {
		t.Errorf("old text left: %q", s)
	}
	// The first line sits where the old one did, the second a line pitch
	// (18pt) below it.
	_, p := paragraphDoc(t)
	old, _ := p.SearchText("The first")
	first, _ := rp.SearchText("A replacement")
	second, _ := rp.SearchText("paragraph.")
	if len(old) != 1 || len(first) != 1 || len(second) != 1 {
		t.Fatalf("lines not found: %d %d %d", len(old), len(first), len(second))
	}
	if d := first[0].Rect.LLY - old[0].Rect.LLY; d < -0.5 || d > 0.5 || first[0].Rect.LLX < 71.5 || first[0].Rect.LLX > 72.5 {
		t.Errorf("first line at %+v, the old one at %+v", first[0].Rect, old[0].Rect)
	}
	if d := first[0].Rect.LLY - second[0].Rect.LLY; d < 17.5 || d > 18.5 {
		t.Errorf("second line %.2fpt below the first, want 18", d)
	}

	long := "The last paragraph replaced by a text long enough to need several lines at the width the old one had."
	got, _, _ = setParagraph(t, "The second", long)
	if n := len(got); n != 3 || got[2] != long {
		t.Errorf("paragraphs after a longer SetText %q", got)
	}
	if err := (&pdf.MarkupParagraph{}).SetText("x"); err == nil {
		t.Error("a paragraph not found by Paragraphs was accepted")
	}
}

// TestParagraphsSplitAtIndent: a paragraph whose last line stops short, then
// a line indented by more than an em that runs to the right edge, is two
// paragraphs; a centred two-line title is one, and so is a short line
// followed by a short indented one (a signature), whose second line does not
// run on as a paragraph's first line does.
func TestParagraphsSplitAtIndent(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 11}
	y := 700.0
	put := func(s string, x float64) {
		if err := p.AddTextAt(s, style, x, y); err != nil {
			t.Fatal(err)
		}
		y -= 18
	}
	put("    A centred title of two lines", 200)
	put("that stays one paragraph", 222)
	y -= 18
	put("The first paragraph opens indented and runs on to the right", 108)
	put("margin, then on its second line it stops short.", 72)
	put("The second paragraph opens indented the same way and runs on", 108)
	put("to the margin too.", 72)
	y -= 18
	put("We close this letter with every good wish for the season ahead,", 72)
	put("signed by the author,", 72)
	put("on the day above", 108)
	_, got := paragraphTextsOf(t, p)
	if len(got) != 4 || got[3] != "We close this letter with every good wish for the season ahead, signed by the author, on the day above" {
		t.Fatalf("paragraphs %q, want the title, two paragraphs and the signature", got)
	}
	if !strings.HasPrefix(got[1], "The first") || !strings.HasPrefix(got[2], "The second") {
		t.Errorf("paragraphs %q", got)
	}
}

// TestParagraphsBlankLineByGap: a line of spaces ends a paragraph when the
// text either side stands more than a line and a half apart, even where wide
// gaps elsewhere raise the page's typical spacing; a line of spaces squeezed
// between the lines of a paragraph at their normal pitch does not.
func TestParagraphsBlankLineByGap(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 11}
	put := func(s string, y float64) {
		if err := p.AddTextAt(s, style, 72, y); err != nil {
			t.Fatal(err)
		}
	}
	put("A running header line", 700)
	put(" ", 688)
	put("The first paragraph starts here and", 676)
	put("ends on this line.", 661)
	put(" ", 637)
	put("The second paragraph starts here and", 614)
	put(" ", 606.5)
	put("goes on past a stray line of spaces.", 599)
	put(" ", 575)
	put("The third paragraph starts here and", 552)
	put("ends on this line.", 537)
	_, got := paragraphTextsOf(t, p)
	want := []string{
		"A running header line",
		"The first paragraph starts here and ends on this line.",
		"The second paragraph starts here and goes on past a stray line of spaces.",
		"The third paragraph starts here and ends on this line.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("paragraphs\n %q\nwant\n %q", got, want)
	}
}
