// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// FloatingBox is a positioned content container (Tier 2 of the flow model): a
// box with an optional border, background and padding that lays its own content
// (paragraphs, headings, images, lists) inside its width. Place it absolutely on
// a page with (*Page).AddFloatingBox, or insert it into a flow with
// (*Flow).AddFloatingBox (where it takes its measured height). Mirrors the
// intent of Aspose.PDF for .NET's FloatingBox. Text flow-around and multi-column
// layout are not implemented (Tier 3).
type FloatingBox struct {
	elems      []flowElem
	border     BorderInfo
	background *Color
	padding    MarginInfo
	paraGap    float64
}

// NewFloatingBox creates an empty floating box.
func NewFloatingBox() *FloatingBox { return &FloatingBox{paraGap: 6} }

// AddParagraph appends a paragraph to the box.
func (b *FloatingBox) AddParagraph(text string, style TextStyle) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkParagraph, text: text, style: style})
	return b
}

// AddHeading appends a heading (level 1–6) to the box.
func (b *FloatingBox) AddHeading(level int, text string, style TextStyle) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkHeading, text: text, style: style, level: level})
	return b
}

// AddImage appends an image (height ≤ 0 preserves aspect) to the box.
func (b *FloatingBox) AddImage(path string, width, height float64) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkImage, imgPath: path, imgW: width, imgH: height})
	return b
}

// AddImageAlt appends an image with alternate text to the box.
func (b *FloatingBox) AddImageAlt(path string, width, height float64, alt string) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkImage, imgPath: path, imgW: width, imgH: height, alt: alt})
	return b
}

// AddList appends a bulleted/numbered list to the box.
func (b *FloatingBox) AddList(items []string, ordered bool, style TextStyle) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkList, items: items, ordered: ordered, style: style})
	return b
}

// AddSpacer appends vertical space to the box.
func (b *FloatingBox) AddSpacer(height float64) *FloatingBox {
	b.elems = append(b.elems, flowElem{kind: fkSpacer, height: height})
	return b
}

// SetBorder sets the box border.
func (b *FloatingBox) SetBorder(border BorderInfo) *FloatingBox { b.border = border; return b }

// SetBackground sets the box fill colour (nil = none).
func (b *FloatingBox) SetBackground(c *Color) *FloatingBox { b.background = c; return b }

// SetPadding sets the inner padding between the border and the content.
func (b *FloatingBox) SetPadding(m MarginInfo) *FloatingBox { b.padding = m; return b }

// AddFloatingBox draws box at an absolute position inside rect: it paints the
// background and border, then lays out the box content within rect minus
// padding (content that overflows the box is clipped). When the document is
// tagged the box becomes a /Div structure element and its decoration an artifact.
func (p *Page) AddFloatingBox(box *FloatingBox, rect Rectangle) error {
	if box == nil {
		return fmt.Errorf("AddFloatingBox: nil box")
	}
	if rect.URX <= rect.LLX || rect.URY <= rect.LLY {
		return fmt.Errorf("AddFloatingBox: empty rect")
	}
	if err := drawBoxFrame(p, box, rect); err != nil {
		return err
	}
	var parent *StructElement
	if p.doc != nil && p.doc.tagged != nil {
		parent = p.doc.tagged.Root().AddChild(StructDiv)
	}
	_, err := box.layout(p, boxContentRect(rect, box.padding), parent)
	return err
}

// flowBox places a box in the flow, reserving its measured height (moving it to
// a new page if it does not fit on the current one).
func (s *flowState) flowBox(box *FloatingBox) error {
	if box == nil {
		return nil
	}
	innerW := s.contentW - box.padding.Left - box.padding.Right
	if innerW <= 0 {
		return fmt.Errorf("flow: box padding leaves no width")
	}
	boxH := measureFlowElems(box.elems, innerW, box.paraGap) + box.padding.Top + box.padding.Bottom
	if boxH > s.y-s.bottom && boxH <= s.top-s.bottom {
		if err := s.newPage(); err != nil {
			return err
		}
	}
	rect := Rectangle{LLX: s.f.mL, LLY: s.y - boxH, URX: s.f.mL + s.contentW, URY: s.y}
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
	s.y -= boxH + s.f.paraGap
	return nil
}

// layout draws the box's content within contentRect (single rectangle, no
// pagination) and returns the height consumed. parent is the tagging container
// (nil = untagged).
func (b *FloatingBox) layout(page *Page, contentRect Rectangle, parent *StructElement) (float64, error) {
	gap := b.paraGap
	if gap <= 0 {
		gap = 6
	}
	sf := &Flow{doc: page.doc, mL: contentRect.LLX, paraGap: gap, tc: page.docTagged()}
	s := &flowState{
		f:        sf,
		page:     page,
		y:        contentRect.URY,
		contentW: contentRect.URX - contentRect.LLX,
		top:      contentRect.URY,
		bottom:   contentRect.LLY,
		boxed:    true,
		parent:   parent,
	}
	for _, el := range b.elems {
		if err := s.place(el); err != nil {
			if err == errBoxFull {
				break
			}
			return contentRect.URY - s.y, err
		}
	}
	return contentRect.URY - s.y, nil
}

// docTagged returns the document's TaggedContent (or nil).
func (p *Page) docTagged() *TaggedContent {
	if p.doc == nil {
		return nil
	}
	return p.doc.tagged
}

// drawBoxFrame paints the box background and border (as an artifact when tagged).
func drawBoxFrame(page *Page, box *FloatingBox, rect Rectangle) error {
	if box.background == nil && (box.border.Sides == BorderSideNone || box.border.Width <= 0) {
		return nil
	}
	style := ShapeStyle{FillColor: box.background}
	if box.border.Sides != BorderSideNone && box.border.Width > 0 {
		col := box.border.Color
		if col == nil {
			col = &Color{A: 1}
		}
		style.LineStyle = LineStyle{Color: col, Width: box.border.Width}
	}
	draw := func() error { return page.DrawRectangle(rect, style) }
	if page.doc != nil && page.doc.tagged != nil {
		return page.artifact(draw)
	}
	return draw()
}

// boxContentRect returns the rectangle inside rect after applying padding.
func boxContentRect(rect Rectangle, pad MarginInfo) Rectangle {
	return Rectangle{
		LLX: rect.LLX + pad.Left,
		LLY: rect.LLY + pad.Bottom,
		URX: rect.URX - pad.Right,
		URY: rect.URY - pad.Top,
	}
}

// measureFlowElems estimates the total height of a sequence of flow elements at
// the given content width (used to size in-flow floating boxes).
func measureFlowElems(elems []flowElem, width, paraGap float64) float64 {
	var total float64
	for _, el := range elems {
		switch el.kind {
		case fkSpacer:
			total += el.height
		case fkParagraph:
			total += textBlockHeight(el.text, paragraphStyle(el.style), width) + paraGap
		case fkHeading:
			total += textBlockHeight(el.text, headingStyle(el.style, el.level), width) + paraGap
		case fkImage:
			_, h := resolveImageSize(el)
			total += h + paraGap
		case fkList:
			total += listHeight(el.items, paragraphStyle(el.style), width) + paraGap
		}
	}
	return total
}

// textBlockHeight returns the height a wrapped paragraph occupies at width.
func textBlockHeight(text string, style TextStyle, width float64) float64 {
	lines, lh, err := measureText(text, style, width)
	if err != nil || lines < 1 {
		return style.Size * lineSpacingOf(style)
	}
	return float64(lines) * lh
}

// resolveImageSize returns the display width/height for an image element,
// preserving aspect when the height is unset.
func resolveImageSize(el flowElem) (float64, float64) {
	w, h := el.imgW, el.imgH
	if w <= 0 {
		return 0, 0
	}
	if h <= 0 {
		if iw, ih, err := imageAspect(el.imgPath); err == nil && iw > 0 {
			h = w * float64(ih) / float64(iw)
		} else {
			h = w
		}
	}
	return w, h
}
