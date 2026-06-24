// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// FloatSide selects which edge a floated box hugs while text wraps around it.
type FloatSide int

const (
	// FloatLeft pins the box to the left of the column; text flows to its right.
	FloatLeft FloatSide = iota
	// FloatRight pins the box to the right of the column; text flows to its left.
	FloatRight
)

// floatTextGap is the horizontal gap between a floated box and the text wrapping
// beside it.
const floatTextGap = 10

// activeFloat is a floated box's reserved horizontal band; the text wraps around
// it while the cursor Y is above bottom.
type activeFloat struct {
	side        FloatSide
	bottom      float64 // band occupies cursor Y > bottom
	left, right float64 // page-space horizontal extent (incl. the box, excl. the gap)
}

// AddFloatBox appends a floated box pinned to the given side at width points;
// the paragraphs that follow wrap around it until the text passes its bottom.
// Designed for single-column flows (in a multi-column flow the box floats within
// the current column).
func (f *Flow) AddFloatBox(box *FloatingBox, side FloatSide, width float64) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkFloat, box: box, floatSide: side, floatW: width})
	return f
}

// placeFloat draws a floated box and registers its band so following text wraps
// around it. The cursor Y is left unchanged (text flows alongside the box).
func (s *flowState) placeFloat(el flowElem) error {
	box, w := el.box, el.floatW
	if box == nil {
		return fmt.Errorf("flow: nil float box")
	}
	if w <= 0 || w > s.contentW {
		return fmt.Errorf("flow: float box width must be within the column")
	}
	innerW := w - box.padding.Left - box.padding.Right
	if innerW <= 0 {
		return fmt.Errorf("flow: float box padding leaves no width")
	}
	boxH := measureFlowElems(box.elems, innerW, box.paraGap) + box.padding.Top + box.padding.Bottom
	if boxH > s.y-s.bottom && boxH <= s.top-s.bottom {
		if err := s.advance(); err != nil {
			return err
		}
	}
	left := s.colLeft()
	if el.floatSide == FloatRight {
		left = s.colLeft() + s.contentW - w
	}
	rect := Rectangle{LLX: left, LLY: s.y - boxH, URX: left + w, URY: s.y}
	if err := drawBoxFrame(s.page, box, rect); err != nil {
		return err
	}
	var parent *StructElement
	if s.parent != nil {
		parent = s.parent.AddChild(StructDiv)
	}
	if _, err := box.layout(s.page, boxContentRect(rect, box.padding), parent); err != nil {
		return err
	}
	s.floats = append(s.floats, activeFloat{side: el.floatSide, bottom: s.y - boxH, left: rect.LLX, right: rect.URX})
	return nil
}

// pruneFloats drops floats the cursor has descended past.
func (s *flowState) pruneFloats() {
	if len(s.floats) == 0 {
		return
	}
	kept := s.floats[:0]
	for _, f := range s.floats {
		if s.y > f.bottom {
			kept = append(kept, f)
		}
	}
	s.floats = kept
}

// lineBounds returns the [left, right] X available for a line at cursor Y after
// subtracting any active float bands, and whether a float constrains it.
func (s *flowState) lineBounds(y float64) (left, right float64, constrained bool) {
	left = s.colLeft()
	right = s.colLeft() + s.contentW
	for _, f := range s.floats {
		if y <= f.bottom {
			continue
		}
		switch f.side {
		case FloatLeft:
			if f.right+floatTextGap > left {
				left = f.right + floatTextGap
				constrained = true
			}
		case FloatRight:
			if f.left-floatTextGap < right {
				right = f.left - floatTextGap
				constrained = true
			}
		}
	}
	return left, right, constrained
}

// nextFloatBottom returns the highest float bottom still below Y (the level the
// cursor can drop to in order to clear the narrowest active float).
func (s *flowState) nextFloatBottom(y float64) float64 {
	nb := s.bottom - 1
	for _, f := range s.floats {
		if y > f.bottom && f.bottom > nb {
			nb = f.bottom
		}
	}
	return nb
}

// dropBelowFloats lowers the cursor below all active floats and clears them, so
// a non-wrapping element (image, table, list, box) starts on a clean line.
func (s *flowState) dropBelowFloats() {
	if len(s.floats) == 0 {
		return
	}
	lowest := s.y
	for _, f := range s.floats {
		if s.y > f.bottom && f.bottom < lowest {
			lowest = f.bottom
		}
	}
	if lowest < s.y {
		s.y = lowest
	}
	s.floats = nil
}

// flowTextAround lays text out line by line, narrowing each line to the width
// left by the active float bands. Lines on the same page are bracketed into one
// tagged paragraph; a page/column break starts a new one.
func (s *flowState) flowTextAround(text string, style TextStyle, st StructType, width widthFn, lh float64) error {
	minLine := 4 * style.Size
	type lineDraw struct {
		text string
		rect Rectangle
	}
	var batch []lineDraw
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		lines := batch
		batch = nil
		return s.draw(st, func() error {
			for _, ld := range lines {
				if err := s.page.AddText(ld.text, style, ld.rect); err != nil {
					return err
				}
			}
			return nil
		})
	}

	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		for len(words) > 0 {
			s.pruneFloats()
			if s.y-lh < s.bottom {
				if err := flush(); err != nil {
					return err
				}
				if err := s.advance(); err != nil {
					return err
				}
				continue
			}
			left, right, constrained := s.lineBounds(s.y)
			if right-left < minLine && constrained {
				// Not enough room beside the float: drop below it.
				nb := s.nextFloatBottom(s.y)
				if nb > s.bottom {
					s.y = nb
				} else {
					if err := flush(); err != nil {
						return err
					}
					if err := s.advance(); err != nil {
						return err
					}
				}
				continue
			}
			line, rest := nextLine(words, width, right-left)
			words = rest
			batch = append(batch, lineDraw{text: line, rect: Rectangle{LLX: left, LLY: s.y - lh, URX: right, URY: s.y}})
			s.y -= lh
		}
	}
	if err := flush(); err != nil {
		return err
	}
	s.y -= s.f.paraGap
	return nil
}

// nextLine greedily takes the words that fit within maxWidth and returns the
// line plus the remaining words. A single word wider than maxWidth is broken.
func nextLine(words []string, width widthFn, maxWidth float64) (string, []string) {
	if len(words) == 0 {
		return "", nil
	}
	if measureString(words[0], width) > maxWidth {
		parts := breakWord(words[0], width, maxWidth)
		rest := append(append([]string{}, parts[1:]...), words[1:]...)
		return parts[0], rest
	}
	line := words[0]
	lineW := measureString(line, width)
	sp := width(' ')
	i := 1
	for ; i < len(words); i++ {
		ww := measureString(words[i], width)
		if lineW+sp+ww > maxWidth {
			break
		}
		line += " " + words[i]
		lineW += sp + ww
	}
	return line, words[i:]
}
