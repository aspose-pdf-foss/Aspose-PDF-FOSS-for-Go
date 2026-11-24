// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

// GHSA-gc9q-3vx8-phx2 point 2: the size SetText shrinks a paragraph to is
// found in a bounded number of steps, whatever font size the file states.
func TestParagraphSetTextHugeFontSize(t *testing.T) {
	content := "BT /F0 1 Tf 10000000 0 0 10000000 10 100 Tm (Hello world) Tj ET"
	objs := pageWith("", "", "<< /Font << /F0 5 0 R >> >>", content)
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	d := openHardening(t, hardeningPDF(objs...))
	pm, err := mustPage(t, d).Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	var mp *MarkupParagraph
	for _, s := range pm.Sections {
		for i := range s.Paragraphs {
			mp = &s.Paragraphs[i]
		}
	}
	if mp == nil {
		t.Fatal("no paragraph found")
	}
	withinDeadline(t, hardeningDeadline, func() {
		if err := mp.SetText(strings.Repeat("word ", 50)); err != nil {
			t.Error(err)
		}
	})
}

// manyParagraphs is a page of n two-line paragraphs set far apart.
func manyParagraphs(n int) []byte {
	var content strings.Builder
	content.WriteString("BT /F0 10 Tf\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&content, "1 0 0 1 10 %d Tm (Paragraph %d) Tj 1 0 0 1 10 %d Tm (goes on) Tj\n", 32+40*i, i, 20+40*i)
	}
	content.WriteString("ET")
	objs := pageWith("", "", "<< /Font << /F0 5 0 R >> >>", content.String())
	objs[2] = strings.Replace(objs[2], "[0 0 200 200]", fmt.Sprintf("[0 0 200 %d]", 40*n+40), 1)
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	return hardeningPDF(objs...)
}

// GHSA-gc9q-3vx8-phx2 point 3: TextParagraphAbsorber extracts the page once,
// not once per paragraph.
func TestTextParagraphAbsorberManyParagraphsStaysLinear(t *testing.T) {
	const small, large = 300, 1200
	files := map[int][]byte{small: manyParagraphs(small), large: manyParagraphs(large)}
	assertLinearGrowth(t, "paragraph absorber", small, large, func(n int) {
		d, err := OpenStream(bytes.NewReader(files[n]))
		if err != nil {
			t.Fatal(err)
		}
		a := NewTextParagraphAbsorber()
		if err := a.Visit(mustPage(t, d)); err != nil {
			t.Fatal(err)
		}
		if got := len(a.TextParagraphs()); got != n {
			t.Fatalf("%d paragraphs, want %d", got, n)
		}
	})
}

// The index hands clipFragments every fragment that can have a glyph inside
// the rectangle: the result is the same as clipping all of them.
func TestFragmentIndexMatchesClipFragments(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var frags []textFragment
	for i := 0; i < 300; i++ {
		x, y := rng.Float64()*500, rng.Float64()*700
		f := textFragment{x: x, y: y}
		k := 1 + rng.IntN(6)
		if i%7 == 0 {
			k = 1 // a lone glyph of no width: its box is a point
		}
		// Some fragments run along a slanted baseline.
		dx, dy := 6.0, 0.0
		if i%5 == 0 {
			dx, dy = 4, 4
		}
		for j := 0; j < k; j++ {
			f.text.WriteRune(rune('a' + j))
			f.runeX = append(f.runeX, x+dx*float64(j))
			f.runeY = append(f.runeY, y+dy*float64(j))
		}
		f.endX, f.endY = x+dx*float64(k), y+dy*float64(k)
		if i%7 == 0 {
			f.endX, f.endY = x, y
		}
		frags = append(frags, f)
	}
	ix := newFragmentIndex(frags)
	var rects []Rectangle
	for i := 0; i < 500; i++ {
		x, y := rng.Float64()*520-10, rng.Float64()*720-10
		rects = append(rects, Rectangle{LLX: x, LLY: y, URX: x + rng.Float64()*200, URY: y + rng.Float64()*100})
	}
	// Rectangles that begin or end just past a fragment's box, within the
	// tolerance clipFragments allows.
	const near = 0.005
	for i := range frags {
		b := ix.box[i]
		rects = append(rects,
			Rectangle{LLX: b.URX + near, LLY: b.LLY - 1, URX: b.URX + 50, URY: b.URY + 1},
			Rectangle{LLX: b.LLX - 50, LLY: b.LLY - 1, URX: b.LLX - near, URY: b.URY + 1},
			Rectangle{LLX: b.LLX - 1, LLY: b.URY + near, URX: b.URX + 1, URY: b.URY + 50},
			Rectangle{LLX: b.LLX - 1, LLY: b.LLY - 50, URX: b.URX + 1, URY: b.LLY - near})
	}
	for _, r := range rects {
		want, got := clipFragments(frags, r), ix.clip(r)
		if len(want) != len(got) {
			t.Fatalf("rect %+v: %d fragments, want %d", r, len(got), len(want))
		}
		for k := range want {
			if want[k].text.String() != got[k].text.String() || want[k].x != got[k].x || want[k].y != got[k].y {
				t.Fatalf("rect %+v: fragment %d is %q at (%v,%v), want %q at (%v,%v)", r, k,
					got[k].text.String(), got[k].x, got[k].y, want[k].text.String(), want[k].x, want[k].y)
			}
		}
	}
	// A rectangle over one fragment's box reaches only the fragments whose
	// boxes meet it, not the whole page.
	for i := range frags {
		var meet int
		b := ix.box[i]
		for j := range frags {
			o := ix.box[j]
			if o.URX >= b.LLX-0.01 && o.LLX <= b.URX+0.01 && o.URY >= b.LLY-0.01 && o.LLY <= b.URY+0.01 {
				meet++
			}
		}
		if got := len(ix.candidates(b)); got != meet {
			t.Fatalf("fragment %d: %d candidates, want the %d boxes meeting its own", i, got, meet)
		}
	}
}

// GHSA-gc9q-3vx8-phx2 point 4: breaking a long word costs time in proportion
// to its length, with zero-width characters or many hyphens in it.
// zwsp is ZERO WIDTH SPACE, a character that draws nothing.
var zwsp = string(rune(0x200b))

func TestLongWordBreakingStaysLinear(t *testing.T) {
	const small, large = 8000, 32000
	zw := func(n int) string { return "a" + strings.Repeat(zwsp, n) + strings.Repeat("W", 1000) }
	assertLinearGrowth(t, "WrapByWordsBreakLong", 4000, 24000, func(n int) {
		d := NewDocument(595, 842)
		f := NewTextFragment(zw(n))
		f.TextState.FontSize = 1
		tp := NewTextParagraph()
		tp.Rectangle = Rectangle{LLX: 10, LLY: 10, URX: 585, URY: 832}
		tp.FormattingOptions.WrapMode = WrapByWordsBreakLong
		tp.AppendLine(f)
		if err := NewTextBuilder(mustPage(t, d)).AppendParagraph(tp); err != nil {
			t.Fatal(err)
		}
	})
	assertLinearGrowth(t, "AddText with hyphens", small, large, func(n int) {
		d := NewDocument(595, 842)
		if err := mustPage(t, d).AddText(strings.Repeat("ab-", n), TextStyle{Size: 1}, Rectangle{LLX: 10, LLY: 10, URX: 20, URY: 832}); err != nil {
			t.Fatal(err)
		}
	})
	data, err := readFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	assertLinearGrowth(t, "AddText, shaped font, zero-width characters", small/4, large/4, func(n int) {
		d := NewDocument(595, 842)
		font, err := d.LoadFontFromStream(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := mustPage(t, d).AddText(zw(n), TextStyle{Font: font, Size: 1}, Rectangle{LLX: 10, LLY: 10, URX: 585, URY: 832}); err != nil {
			t.Fatal(err)
		}
	})
}

// longestFit gives the prefix the rune-by-rune scan gives for widths that
// grow with the prefix, at every limit.
func TestLongestFitMatchesScan(t *testing.T) {
	words := []string{"", "a", "ab", "héllo wörld", zwsp + zwsp + "ab" + zwsp + "cdefgh", strings.Repeat("xé"+zwsp, 40), "a\xffb\xfe"}
	width := func(s string) float64 {
		var w float64
		for _, r := range s {
			if string(r) != zwsp {
				w += float64(1 + r%3)
			}
		}
		return w
	}
	scan := func(word string, maxW float64) int {
		n := 0
		for i := range word {
			_, size := utf8.DecodeRuneInString(word[i:])
			if i > 0 && width(word[:i+size]) > maxW {
				break
			}
			n = i + size
		}
		return n
	}
	for _, w := range words {
		for maxW := -1.0; maxW <= width(w)+2; maxW += 0.5 {
			if got, want := longestFit(w, maxW, width), scan(w, maxW); got != want {
				t.Errorf("%q at %v: %d bytes, want %d", w, maxW, got, want)
			}
		}
	}
}

// GHSA-gc9q-3vx8-phx2 point 5: RemoveAllText leaves no /ActualText of a named
// property list in the saved file, whether the page or a form without
// resources of its own names it, and leaves a page it was not asked to
// change as it was.
func TestRemoveAllTextScrubsNamedActualText(t *testing.T) {
	const secret = "Secret 4711"
	build := func() *Document {
		objs := []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R 9 0 R] /Count 2 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources 8 0 R >>",
			"<< /Length 0 >> stream\n/Span /MC0 BDC BT /F1 12 Tf 50 100 Td (" + secret + ") Tj ET EMC /Fm0 Do\nendstream",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
			"<< /ActualText (" + secret + ") >>",
			"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Length 0 >> stream\n/Span /MC1 BDC BT /F1 12 Tf 50 50 Td (x) Tj ET EMC\nendstream",
			"<< /Font << /F1 5 0 R >> /Properties << /MC0 6 0 R /MC1 11 0 R >> /XObject << /Fm0 7 0 R >> >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 10 0 R /Resources 8 0 R >>",
			"<< /Length 0 >> stream\n/Span /MC0 BDC BT /F1 12 Tf 50 100 Td (z) Tj ET EMC\nendstream",
			"<< /ActualText (" + secret + " too) >>",
		}
		return openHardening(t, hardeningPDF(objs...))
	}
	saved := func(d *Document) *Document {
		var buf bytes.Buffer
		if _, err := d.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		return openHardening(t, buf.Bytes())
	}

	d := build()
	if err := d.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	if found, _ := documentContainsBytes(saved(d), secret); found {
		t.Errorf("%q is still in the saved file after RemoveAllText", secret)
	}

	// Only page 1: page 2 still shows its replacement text, so the property
	// list it names stays, while page 1 no longer reaches it.
	d = build()
	if err := d.Pages()[0].RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	out := saved(d)
	if text, _ := out.Pages()[1].ExtractText(); !strings.Contains(text, secret) {
		t.Errorf("page 2 reads %q, want its /ActualText %q", text, secret)
	}
	if text, _ := out.Pages()[0].ExtractText(); strings.TrimSpace(text) != "" {
		t.Errorf("page 1 still reads %q", text)
	}
	if props, ok := resolveRefToDict(out.objects, out.Pages()[0].pageResources()["/Properties"]); ok {
		for name, v := range props {
			if p, ok := resolveRefToDict(out.objects, v); ok && p["/ActualText"] != nil {
				t.Errorf("page 1's /Properties %s still holds /ActualText", name)
			}
		}
	}
}

// A form with resources of its own that name the property list gets them
// replaced in its clone.
func TestRemoveAllTextScrubsNamedActualTextInForm(t *testing.T) {
	const secret = "Secret 4711"
	objs := pageWith("", "", "<< /XObject << /Fm0 5 0 R >> >>", "/Fm0 Do")
	objs = append(objs,
		"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F1 6 0 R >> /Properties << /MC0 7 0 R >> >> /Length 0 >> stream\n/Span /MC0 BDC BT /F1 12 Tf 50 50 Td (x) Tj ET EMC\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /ActualText ("+secret+") >>")
	d := openHardening(t, hardeningPDF(objs...))
	if err := d.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if found, _ := documentContainsBytes(openHardening(t, buf.Bytes()), secret); found {
		t.Errorf("%q is still in the saved file after RemoveAllText", secret)
	}
}
