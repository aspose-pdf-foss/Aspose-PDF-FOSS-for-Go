// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// Styled-runs paragraph layout (epic pdf-go-fh4l.3): a paragraph as a
// sequence of differently-styled text runs (bold/italic/code/link spans from
// Markdown — and, later, HTML) laid out with a shared baseline per line and
// greedy word wrapping ACROSS run borders. The flow integration (fkRuns)
// paginates line by line like plain paragraphs; runs carrying a link
// destination get a real LinkAnnotation over their drawn extent.

// textRun is one uniformly-styled fragment of a runs paragraph.
type textRun struct {
	text     string
	style    TextStyle
	linkDest string // non-empty → wrap the drawn segments in a link annotation
	brk      bool   // hard line break marker (text ignored)
}

// addRuns queues a styled-runs paragraph. st selects the structure-element
// type used when the flow is tagged (StructP, StructH1…).
func (f *Flow) addRuns(runs []textRun, st StructType) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkRuns, runs: runs, st: st})
	return f
}

// runMetrics caches per-run font measurements.
type runMetrics struct {
	width      widthFn
	ascent     float64 // ascent factor (of em)
	size       float64
	lineHeight float64
}

func metricsFor(style TextStyle) (runMetrics, error) {
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	size := style.Size
	if size == 0 {
		size = 12
	}
	w, asc, err := fontWidthAndAscent(font, size)
	if err != nil {
		return runMetrics{}, err
	}
	return runMetrics{width: w, ascent: asc, size: size, lineHeight: size * lineSpacingOf(style)}, nil
}

// runSeg is one drawn segment: a run's contiguous text on one line, at x
// (relative to the line start) and width w.
type runSeg struct {
	run  int
	text string
	x, w float64
}

type runLine struct {
	segs   []runSeg
	h      float64 // line height
	ascent float64 // baseline offset from the line top
}

// runToken is a word, a space, or a hard break, attributed to its run.
type runToken struct {
	run   int
	text  string
	w     float64
	space bool
	brk   bool
}

func tokenizeRuns(runs []textRun, ms []runMetrics) []runToken {
	var tokens []runToken
	for ri, r := range runs {
		if r.brk {
			tokens = append(tokens, runToken{run: ri, brk: true})
			continue
		}
		text := r.text
		for text != "" {
			if text[0] == ' ' {
				j := 1
				for j < len(text) && text[j] == ' ' {
					j++
				}
				tokens = append(tokens, runToken{run: ri, text: text[:j], w: measureString(text[:j], ms[ri].width), space: true})
				text = text[j:]
				continue
			}
			j := strings.IndexByte(text, ' ')
			if j < 0 {
				j = len(text)
			}
			tokens = append(tokens, runToken{run: ri, text: text[:j], w: measureString(text[:j], ms[ri].width)})
			text = text[j:]
		}
	}
	return tokens
}

// layoutRuns breaks the runs into lines of at most maxW points. Words that
// span run borders (no space between tokens of adjacent runs) are treated as
// one unbreakable cluster. An oversized cluster gets its own overflowing line
// (no mid-word breaking, matching browsers' default).
func layoutRuns(runs []textRun, ms []runMetrics, maxW float64) []runLine {
	tokens := tokenizeRuns(runs, ms)

	var lines []runLine
	var cur []runSeg
	curW := 0.0
	var pend []runToken // pending inter-word spaces
	pendW := 0.0

	lineOf := func(segs []runSeg) runLine {
		ln := runLine{segs: segs}
		for _, sg := range segs {
			m := ms[sg.run]
			if m.lineHeight > ln.h {
				ln.h = m.lineHeight
			}
			if a := m.ascent * m.size; a > ln.ascent {
				ln.ascent = a
			}
		}
		if len(segs) == 0 {
			ln.h = ms[0].lineHeight
			ln.ascent = ms[0].ascent * ms[0].size
		}
		return ln
	}
	appendSeg := func(run int, text string, w float64) {
		if n := len(cur); n > 0 && cur[n-1].run == run {
			cur[n-1].text += text
			cur[n-1].w += w
		} else {
			cur = append(cur, runSeg{run: run, text: text, x: curW, w: w})
		}
		curW += w
	}
	flush := func() {
		lines = append(lines, lineOf(cur))
		cur = nil
		curW = 0
		pend = nil
		pendW = 0
	}

	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if t.brk {
			flush()
			i++
			continue
		}
		if t.space {
			if curW > 0 { // leading spaces on a line are dropped
				pend = append(pend, t)
				pendW += t.w
			}
			i++
			continue
		}
		// Cluster: consecutive non-space tokens form one unbreakable word.
		j := i
		clusterW := 0.0
		for j < len(tokens) && !tokens[j].space && !tokens[j].brk {
			clusterW += tokens[j].w
			j++
		}
		if curW > 0 && curW+pendW+clusterW > maxW {
			flush()
		}
		for _, sp := range pend {
			appendSeg(sp.run, sp.text, sp.w)
		}
		pend = nil
		pendW = 0
		for ; i < j; i++ {
			appendSeg(tokens[i].run, tokens[i].text, tokens[i].w)
		}
		if curW > maxW {
			flush() // oversized single cluster overflows on its own line
		}
	}
	if len(cur) > 0 {
		flush()
	}
	if len(lines) == 0 && len(ms) > 0 {
		return nil
	}
	return lines
}

// flowRuns paginates and draws a styled-runs paragraph.
func (s *flowState) flowRuns(runs []textRun, st StructType) error {
	if len(runs) == 0 {
		return nil
	}
	s.dropBelowFloats()
	ms := make([]runMetrics, len(runs))
	for i, r := range runs {
		m, err := metricsFor(r.style)
		if err != nil {
			return fmt.Errorf("flow runs: %w", err)
		}
		ms[i] = m
	}
	lines := layoutRuns(runs, ms, s.contentW)

	start := 0
	for start < len(lines) {
		avail := s.y - s.bottom
		n, chunkH := 0, 0.0
		for start+n < len(lines) && chunkH+lines[start+n].h <= avail {
			chunkH += lines[start+n].h
			n++
		}
		if n == 0 {
			if err := s.advance(); err != nil {
				return err
			}
			avail = s.y - s.bottom
			for start+n < len(lines) && chunkH+lines[start+n].h <= avail {
				chunkH += lines[start+n].h
				n++
			}
			if n == 0 { // a single line taller than the content area
				n = 1
				chunkH = lines[start].h
			}
		}
		chunk := lines[start : start+n]
		if err := s.draw(st, func() error { return s.drawRunLines(chunk, runs, ms) }); err != nil {
			return err
		}
		s.y -= chunkH
		start += n
	}
	s.y -= s.f.paraGap
	return nil
}

// drawRunLines draws the given lines starting at the current cursor, all
// segments of a line sharing one baseline, and attaches link annotations.
func (s *flowState) drawRunLines(lines []runLine, runs []textRun, ms []runMetrics) error {
	y := s.y
	left := s.colLeft()
	for _, ln := range lines {
		baseline := y - ln.ascent
		for _, sg := range ln.segs {
			r := runs[sg.run]
			m := ms[sg.run]
			// AddText places the first baseline at rect.URY − ascent·size
			// (VAlignTop), so URY = baseline + ascent·size pins the shared
			// baseline; the slack below keeps descenders out of the clip.
			rect := Rectangle{
				LLX: left + sg.x,
				LLY: baseline - 0.35*m.size,
				URX: left + sg.x + sg.w + 1.5,
				URY: baseline + m.ascent*m.size,
			}
			if err := s.page.AddText(sg.text, r.style, rect); err != nil {
				return err
			}
			if r.linkDest != "" {
				linkRect := Rectangle{
					LLX: left + sg.x,
					LLY: baseline - 0.2*m.size,
					URX: left + sg.x + sg.w,
					URY: baseline + 0.8*m.size,
				}
				link := NewLinkAnnotation(s.page, linkRect)
				link.SetAction(NewGoToURIAction(r.linkDest))
				if err := s.page.Annotations().Add(link); err != nil {
					return err
				}
			}
		}
		y -= ln.h
	}
	return nil
}
