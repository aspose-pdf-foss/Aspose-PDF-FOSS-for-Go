// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// WordWrapMode says how a TextParagraph breaks its lines. Mirrors Aspose.PDF
// for .NET's TextFormattingOptions.WordWrapMode.
type WordWrapMode int

const (
	// WrapByWords breaks lines at spaces; a word wider than the paragraph
	// overflows (and is clipped). The zero value.
	WrapByWords WordWrapMode = iota
	// WrapNoWrap keeps every line on one row, clipped at the right edge.
	WrapNoWrap
	// WrapByWordsBreakLong breaks at spaces and also inside a word wider than
	// the paragraph.
	WrapByWordsBreakLong
)

// TextFormattingOptions holds how a TextParagraph lays out its text. Mirrors
// Aspose.PDF for .NET's TextFormattingOptions.
type TextFormattingOptions struct {
	WrapMode WordWrapMode
}

// TextParagraph is a block of text laid out inside a rectangle: wrapped,
// aligned, optionally justified and indented. Lines are appended with
// AppendLine and drawn by TextBuilder.AppendParagraph. Mirrors Aspose.PDF for
// .NET's TextParagraph. (The paragraphs TextParagraphAbsorber finds are
// AbsorbedTextParagraph.)
type TextParagraph struct {
	// Rectangle is where the paragraph is drawn; text is clipped to it less
	// Margin.
	Rectangle Rectangle
	// HorizontalAlignment places each line between the margins; the zero
	// value (None) is the same as Left.
	HorizontalAlignment HorizontalAlignment
	// VerticalAlignment places the block of lines in the rectangle.
	VerticalAlignment VAlign
	// Justify spreads every line but the last of each appended line across the
	// width.
	Justify bool
	// FirstLineIndent narrows the first line of the paragraph from the left.
	FirstLineIndent float64
	// SubsequentLinesIndent narrows every other line from the left.
	SubsequentLinesIndent float64
	// Margin is the space kept free inside Rectangle.
	Margin MarginInfo
	// FormattingOptions selects the wrapping.
	FormattingOptions TextFormattingOptions

	lines []*TextFragment
}

// NewTextParagraph returns an empty paragraph.
func NewTextParagraph() *TextParagraph { return &TextParagraph{} }

// AppendLine adds f as the next line of the paragraph. A line longer than the
// paragraph is wrapped, as the paragraph's FormattingOptions say; CRLF, CR,
// LF, NEL, U+2028 and U+2029 in a segment's text start a new line.
func (p *TextParagraph) AppendLine(f *TextFragment) {
	if f != nil {
		p.lines = append(p.lines, f)
	}
}

// paraRun is one segment of a paragraph line, resolved.
type paraRun struct {
	seg       *TextSegment
	st        TextState
	style     TextStyle
	asc, desc float64
	recorded  bool
}

// paraLine is one laid-out line of the paragraph.
type paraLine struct {
	frag   int
	ln     runLine
	last   bool    // last line of its appended fragment, or of a line ended by a forced break
	indent float64 // from the paragraph's left edge
	avail  float64 // width available to the line
}

// paraPiece is a part of a line drawn on its own: a segment, or one word.
type paraPiece struct {
	run     int
	text    string
	x, w    float64 // relative to the line start
	gapsBef int
}

// AppendParagraph draws the paragraph: each appended line is wrapped to the
// rectangle's width less margins and indents, placed from the top, middle or
// bottom, aligned and, when asked, justified, and clipped to the rectangle
// less margins. Every fragment passed to AppendLine is bound to the page
// afterwards, like AppendText's, with all its wrapped pieces. Rotated text is
// not supported in a paragraph.
func (b *TextBuilder) AppendParagraph(tp *TextParagraph) error {
	if b == nil || b.page == nil {
		return errors.New("text builder: nil page")
	}
	if tp == nil {
		return errors.New("text builder: nil paragraph")
	}
	if len(tp.lines) == 0 {
		return nil
	}
	pg := b.page
	r := orderedRect(tp.Rectangle)
	inner := Rectangle{LLX: r.LLX + tp.Margin.Left, LLY: r.LLY + tp.Margin.Bottom, URX: r.URX - tp.Margin.Right, URY: r.URY - tp.Margin.Top}
	innerW := inner.URX - inner.LLX
	if !(innerW > 0) || !(inner.URY-inner.LLY > 0) || math.IsInf(innerW, 0) {
		return errors.New("text builder: paragraph rectangle leaves no room")
	}
	if tp.FirstLineIndent < 0 || tp.SubsequentLinesIndent < 0 || nonFinite(tp.FirstLineIndent) || nonFinite(tp.SubsequentLinesIndent) {
		return errors.New("text builder: paragraph indents must be finite and not negative")
	}
	restW := innerW - tp.SubsequentLinesIndent
	firstW := innerW - tp.FirstLineIndent
	if !(restW > 0) || !(firstW > 0) {
		return errors.New("text builder: paragraph indents leave no room")
	}
	mode := tp.FormattingOptions.WrapMode
	if mode < WrapByWords || mode > WrapByWordsBreakLong {
		return fmt.Errorf("text builder: unknown wrap mode %d", mode)
	}

	// Lay every line out first, so nothing is drawn on an error.
	frags := make([][]paraRun, len(tp.lines))
	mss := make([][]runMetrics, len(tp.lines))
	var lines []paraLine
	for fi, f := range tp.lines {
		if f == nil {
			return errors.New("text builder: nil line")
		}
		segs := f.segmentsToDraw()
		runs := make([]textRun, len(segs))
		ms := make([]runMetrics, len(segs))
		rs := make([]paraRun, len(segs))
		for i, s := range segs {
			if s.TextState.Rotation != 0 {
				return errors.New("text builder: rotated text is not supported in a paragraph")
			}
			st := s.TextState.resolved()
			if err := checkFontSize("text builder", st.FontSize); err != nil {
				return err
			}
			style := st.style()
			if err := checkTextStyleValues("text builder", style); err != nil {
				return err
			}
			style.Background = nil // drawn per piece below
			asc, desc, err := textBox(style.Font, st.FontSize)
			if err != nil {
				return fmt.Errorf("text builder: %w", err)
			}
			rs[i] = paraRun{seg: s, st: st, style: style, asc: asc, desc: desc}
			runs[i] = textRun{text: s.Text, style: style}
			m, err := metricsFor(style, s.Text)
			if err != nil {
				return fmt.Errorf("text builder: %w", err)
			}
			if tc, tw, tz := st.CharacterSpacing, st.WordSpacing, st.HorizontalScaling; tc != 0 || tw != 0 || (tz != 0 && tz != 100) {
				font, rtl := style.Font, style.RTL
				m.adjust = func(text string, w float64) float64 {
					return spacedWidth(w, glyphSpacingText(font, text, rtl), tc, tw, tz)
				}
			}
			ms[i] = m
		}
		w1, w2 := restW, restW
		if fi == 0 {
			w1 = firstW
		}
		if mode == WrapNoWrap {
			w1, w2 = math.MaxFloat64/4, math.MaxFloat64/4
		}
		laid := layoutRunsOpt(runs, ms, w2, w1, mode == WrapByWordsBreakLong)
		if len(laid) == 0 {
			laid = []runLine{{h: ms[0].lineHeight, ascent: ms[0].ascent * ms[0].size}}
		}
		for li, ln := range laid {
			indent, avail := tp.SubsequentLinesIndent, restW
			if fi == 0 && li == 0 {
				indent, avail = tp.FirstLineIndent, firstW
			}
			lines = append(lines, paraLine{frag: fi, ln: ln, last: li == len(laid)-1 || ln.hard, indent: indent, avail: avail})
		}
		frags[fi], mss[fi] = rs, ms
	}

	total := 0.0
	for _, l := range lines {
		total += l.ln.h
	}
	top := inner.URY
	switch tp.VerticalAlignment {
	case VAlignMiddle:
		top = (inner.URY+inner.LLY)/2 + total/2
	case VAlignBottom:
		top = inner.LLY + total
	}

	justifyAlign := tp.Justify || tp.HorizontalAlignment == HorizontalAlignmentJustify
	justifyAll := tp.HorizontalAlignment == HorizontalAlignmentFullJustify

	clip := fmt.Sprintf("q\n%s %s %s %s re W n\n", formatFloat(inner.LLX), formatFloat(inner.LLY), formatFloat(innerW), formatFloat(inner.URY-inner.LLY))
	if err := pg.appendToContentStream([]byte(clip)); err != nil {
		return fmt.Errorf("text builder: %w", err)
	}
	drawn := make([][]QuadPoint, len(tp.lines))
	first := make([]*Position, len(tp.lines))
	err := func() error {
		y := top
		for _, l := range lines {
			lineTop := y
			baseline := y - l.ln.ascent
			y -= l.ln.h
			if lineTop <= inner.LLY || y >= inner.URY {
				continue // wholly outside the clip: not drawn, not bound
			}
			rs, ms := frags[l.frag], mss[l.frag]
			f := tp.lines[l.frag]
			lineW := 0.0
			if n := len(l.ln.segs); n > 0 {
				lineW = l.ln.segs[n-1].x + l.ln.segs[n-1].w
			}
			overflow := lineW > l.avail+0.01
			justify := !overflow && (justifyAll || (justifyAlign && !l.last))
			pieces, gaps := linePieces(l.ln, ms, justify || overflow)
			xoff, extra := 0.0, 0.0
			switch {
			case justify && gaps > 0:
				extra = (l.avail - lineW) / float64(gaps)
			case overflow:
			case tp.HorizontalAlignment == HorizontalAlignmentRight:
				xoff = l.avail - lineW
			case tp.HorizontalAlignment == HorizontalAlignmentCenter:
				xoff = (l.avail - lineW) / 2
			}
			for _, pc := range pieces {
				x := inner.LLX + l.indent + xoff + pc.x + extra*float64(pc.gapsBef)
				if overflow && x >= inner.URX {
					continue // wholly outside the clip: not drawn
				}
				run := &rs[pc.run]
				box, quad, _, err := drawTextPiece(pg, run.st, run.style, run.seg.Hyperlink, pc.text, x, baseline, pc.w, run.asc, run.desc)
				if err != nil {
					return err
				}
				if first[l.frag] == nil {
					first[l.frag] = &Position{XIndent: x, YIndent: baseline}
				}
				if !run.recorded {
					run.recorded = true
					if pc.run < len(f.Segments) {
						f.Segments[pc.run].Position = Position{XIndent: x, YIndent: baseline}
						f.Segments[pc.run].Rectangle = box
					}
				}
				drawn[l.frag] = append(drawn[l.frag], quad)
			}
		}
		return nil
	}()
	if qerr := pg.appendToContentStream([]byte("Q\n")); err == nil && qerr != nil {
		err = qerr
	}
	if err != nil {
		return fmt.Errorf("text builder: %w", err)
	}

	for fi, f := range tp.lines {
		var all strings.Builder
		for _, run := range frags[fi] {
			all.WriteString(run.seg.Text)
		}
		f.Text = all.String()
		if first[fi] != nil {
			f.Position = *first[fi]
		}
		f.bind = &fragmentBinding{page: pg, drawn: drawn[fi], noEdit: "text laid out in a paragraph (draw the paragraph again with AppendParagraph)"}
		f.bind.snap = snapshotOf(f)
		pg.doc.bindFragment(f)
	}
	return nil
}

// linePieces splits a laid-out line into the parts drawn separately: its
// segments, or with words each word of them. gaps counts the places a
// justified line may stretch: every run of spaces, and every positional gap
// between segments; each piece records how many precede it.
func linePieces(ln runLine, ms []runMetrics, words bool) (pieces []paraPiece, gaps int) {
	prevEnd := 0.0
	for si, sg := range ln.segs {
		if si > 0 && sg.x-prevEnd > 0.01 {
			gaps++
		}
		prevEnd = sg.x + sg.w
		if !words {
			pieces = append(pieces, paraPiece{run: sg.run, text: sg.text, x: sg.x, w: sg.w})
			continue
		}
		off := 0.0
		text := sg.text
		for text != "" {
			j := 1
			sp := text[0] == ' '
			for j < len(text) && (text[j] == ' ') == sp {
				j++
			}
			tok := text[:j]
			text = text[j:]
			w := tokenWidth(ms, sg.run, tok)
			if sp {
				if off > 0 {
					gaps++
				}
			} else {
				pieces = append(pieces, paraPiece{run: sg.run, text: tok, x: sg.x + off, w: w, gapsBef: gaps})
			}
			off += w
		}
	}
	return pieces, gaps
}
