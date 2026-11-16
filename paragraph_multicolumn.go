// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Paragraphs that continue across columns and pages (pdf-go-6u85), mirroring
// Aspose.PDF for .NET's PageMarkup.IsMulticolumnParagraphsAllowed and
// ParagraphAbsorber.IsMulticolumnParagraphsAllowed.

// ParagraphOptions tunes Page.Paragraphs and Document.Paragraphs.
type ParagraphOptions struct {
	// MulticolumnParagraphs joins a paragraph cut off by the end of a
	// column with its continuation at the top of the next one — on the same
	// page, or with Document.Paragraphs on the next page. The continuation
	// leaves its section (a section left with no paragraph is dropped) and
	// is recorded in the paragraph's Continuations. Mirrors Aspose.PDF for
	// .NET's IsMulticolumnParagraphsAllowed.
	MulticolumnParagraphs bool
}

// ParagraphContinuation is the part of a paragraph found in a later column or
// page: where it is, and its lines. Mirrors the SecondaryPoints and
// ContinuationPageNumbers of an Aspose.PDF for .NET MarkupParagraph.
type ParagraphContinuation struct {
	PageNumber int
	Rectangle  Rectangle
	Lines      []TextLine
}

func paragraphOption(opts []ParagraphOptions) ParagraphOptions {
	if len(opts) == 0 {
		return ParagraphOptions{}
	}
	return opts[len(opts)-1]
}

// IsMulticolumnParagraphsAllowed reports whether the markup's paragraphs are
// joined across its columns.
func (m *PageMarkup) IsMulticolumnParagraphsAllowed() bool { return m.multicolumn }

// SetMulticolumnParagraphsAllowed re-groups the markup's paragraphs: with v,
// a paragraph cut off by the end of a column is joined with its continuation
// in the next column of the page; without, the paragraphs are as each column
// holds them. Mirrors setting Aspose.PDF for .NET's
// PageMarkup.IsMulticolumnParagraphsAllowed.
func (m *PageMarkup) SetMulticolumnParagraphsAllowed(v bool) {
	if v == m.multicolumn {
		return
	}
	if m.plain == nil {
		m.plain = copySections(m.Sections)
	}
	if v {
		joined := joinContinuations([]PageMarkup{{PageNumber: m.PageNumber, Sections: copySections(m.plain), pageBox: m.pageBox}})
		m.Sections = joined[0].Sections
	} else {
		m.Sections = copySections(m.plain)
	}
	m.multicolumn = v
}

func copySections(ss []MarkupSection) []MarkupSection {
	out := make([]MarkupSection, len(ss))
	for i, s := range ss {
		out[i] = MarkupSection{Rectangle: s.Rectangle, Paragraphs: append([]MarkupParagraph(nil), s.Paragraphs...)}
	}
	return out
}

// joinContinuations joins, across the given pages in order, each column's
// last paragraph with the next column's first when it continues there.
func joinContinuations(pages []PageMarkup) []PageMarkup {
	// carrier: where the paragraph that may continue lives.
	type ref struct{ page, sec, par int }
	var carrier *ref
	for pi := range pages {
		pm := &pages[pi]
		for si := range pm.Sections {
			sec := &pm.Sections[si]
			if len(sec.Paragraphs) == 0 {
				continue
			}
			// Furniture in the top or bottom band of the page (a running
			// head, a footer, a page number) is neither where a paragraph
			// continues nor one that continues: it is stepped over.
			first := 0
			for first < len(sec.Paragraphs) && pageEdgeLine(&sec.Paragraphs[first], pm.pageBox) {
				first++
			}
			if first == len(sec.Paragraphs) {
				continue
			}
			if carrier != nil {
				a := &pages[carrier.page].Sections[carrier.sec].Paragraphs[carrier.par]
				b := sec.Paragraphs[first]
				if continuesInto(a, &b) {
					appendContinuation(a, b, pm.PageNumber)
					sec.Paragraphs = append(sec.Paragraphs[:first:first], sec.Paragraphs[first+1:]...)
				}
			}
			last := len(sec.Paragraphs) - 1
			for last >= first && pageEdgeLine(&sec.Paragraphs[last], pm.pageBox) {
				last--
			}
			if last < first {
				continue // nothing left but furniture: the carrier goes on
			}
			carrier = &ref{pi, si, last}
		}
	}
	for pi := range pages {
		kept := pages[pi].Sections[:0]
		for _, s := range pages[pi].Sections {
			if len(s.Paragraphs) > 0 {
				s.Rectangle = paragraphsBBox(s.Paragraphs)
				kept = append(kept, s)
			}
		}
		pages[pi].Sections = kept
	}
	return pages
}

// pageEdgeLine reports a one-line paragraph of at most three words in the top
// or bottom eighth of the page: a running head, a footer, a page number.
func pageEdgeLine(p *MarkupParagraph, box Rectangle) bool {
	if len(p.Lines) != 1 || len(strings.Fields(p.Text)) > 3 || box.URY <= box.LLY {
		return false
	}
	band := (box.URY - box.LLY) / 8
	return p.Rectangle.LLY >= box.URY-band || p.Rectangle.URY <= box.LLY+band
}

// continuesInto reports whether b, at the top of the next column or page, is
// the rest of a, the last paragraph of the previous one: same size of type,
// a's last part filled its column to the end (it was cut, not finished), b's
// first line is not indented, and either b opens in lower case or a does not
// end a sentence.
func continuesInto(a, b *MarkupParagraph) bool {
	aLines := a.Lines
	if n := len(a.Continuations); n > 0 {
		aLines = a.Continuations[n-1].Lines
	}
	if len(aLines) == 0 || len(b.Lines) == 0 {
		return false
	}
	sa, sb := paragraphFontSize(aLines), paragraphFontSize(b.Lines)
	if sa <= 0 || sb <= 0 || sb > sa*1.15 || sa > sb*1.15 {
		return false
	}
	// a's last line runs to (within two ems of) its widest line.
	right := 0.0
	for i := range aLines {
		right = maxf(right, lineRight(aLines[i]))
	}
	if lineRight(aLines[len(aLines)-1]) < right-2*sa {
		return false
	}
	// b's first line starts where its other lines do.
	if len(b.Lines) > 1 && lineLeft(&b.Lines[0]) > lineLeft(&b.Lines[1])+0.5*sb {
		return false
	}
	tail := strings.TrimRightFunc(a.Text, unicode.IsSpace)
	head := strings.TrimLeftFunc(b.Text, unicode.IsSpace)
	if tail == "" || head == "" {
		return false
	}
	if r, _ := utf8.DecodeRuneInString(head); unicode.IsLower(r) {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(tail)
	return !strings.ContainsRune(".!?:;…\"”’)»", r)
}

func paragraphFontSize(lines []TextLine) float64 {
	best := 0.0
	for _, l := range lines {
		for _, f := range l.Fragments {
			if strings.TrimSpace(f.Text) != "" {
				best = maxf(best, f.FontSize)
			}
		}
	}
	return best
}

// lineLeft is the x of a line's first fragment that is not blank.
func lineLeft(l *TextLine) float64 {
	for _, f := range l.Fragments {
		if strings.TrimSpace(f.Text) != "" {
			return f.X
		}
	}
	return 0
}

// appendContinuation adds b, found on page, to a: its text after a space (no
// space after a hyphen), its lines, and its place.
func appendContinuation(a *MarkupParagraph, b MarkupParagraph, page int) {
	sep := " "
	if strings.HasSuffix(strings.TrimRightFunc(a.Text, unicode.IsSpace), "-") {
		sep = ""
	}
	a.Text = strings.TrimRightFunc(a.Text, unicode.IsSpace) + sep + strings.TrimLeftFunc(b.Text, unicode.IsSpace)
	a.Lines = append(append([]TextLine(nil), a.Lines...), b.Lines...)
	a.Continuations = append(append([]ParagraphContinuation(nil), a.Continuations...),
		ParagraphContinuation{PageNumber: page, Rectangle: b.Rectangle, Lines: b.Lines})
}
