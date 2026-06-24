// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// TagArtifact brackets a block of page drawing as an /Artifact marked-content
// sequence, so it is treated as decoration and excluded from the logical
// structure tree. Use it for running headers/footers, page numbers, backgrounds
// and rules in a Tagged PDF — PDF/UA requires every content item to be either
// tagged as structure or marked as an artifact. Requires Document.TaggedContent.
func (p *Page) TagArtifact(draw func() error) error {
	if p.doc == nil || p.doc.tagged == nil {
		return fmt.Errorf("TagArtifact: call Document.TaggedContent() first")
	}
	return p.artifact(draw)
}

// AddTaggedList draws a bulleted (ordered=false) or numbered (ordered=true) list
// of items inside rect and builds its accessible structure: an /L element under
// parent (nil = the document root), with an /LI per item containing an /Lbl (the
// bullet/number) and an /LBody (the item text). Each item's text wraps within
// the body column; items are stacked top-down and any that would fall below rect
// are dropped (no auto-pagination in this version). Returns the /L element.
// Requires Document.TaggedContent.
func (p *Page) AddTaggedList(tc *TaggedContent, parent *StructElement, items []string, style TextStyle, rect Rectangle, ordered bool) (*StructElement, error) {
	if tc == nil || p.doc == nil || p.doc.tagged == nil {
		return nil, fmt.Errorf("AddTaggedList: call Document.TaggedContent() first")
	}
	if parent == nil {
		parent = tc.root
	}
	size := style.Size
	if size <= 0 {
		size = 12
	}
	labelW := size * 1.9 // room for "99." and a gap
	gap := size * 0.4
	bodyW := rect.URX - (rect.LLX + labelW)
	if bodyW <= 0 {
		return nil, fmt.Errorf("AddTaggedList: rect too narrow for the list")
	}

	list := parent.AddChild(StructList)
	y := rect.URY
	for i, item := range items {
		lines, lineH, err := measureText(item, style, bodyW)
		if err != nil {
			return nil, err
		}
		if lines < 1 {
			lines = 1
		}
		itemH := float64(lines) * lineH
		if y-itemH < rect.LLY {
			break // would overflow the rectangle
		}
		label := "•" // bullet
		if ordered {
			label = fmt.Sprintf("%d.", i+1)
		}
		labelRect := Rectangle{LLX: rect.LLX, LLY: y - lineH, URX: rect.LLX + labelW - gap, URY: y}
		bodyRect := Rectangle{LLX: rect.LLX + labelW, LLY: y - itemH, URX: rect.URX, URY: y}

		li := list.AddChild(StructListItem)
		if _, err := p.TagContent(li, StructLabel, func() error {
			return p.AddText(label, style, labelRect)
		}); err != nil {
			return nil, err
		}
		if _, err := p.TagContent(li, StructListBody, func() error {
			return p.AddText(item, style, bodyRect)
		}); err != nil {
			return nil, err
		}
		y -= itemH + gap
	}
	return list, nil
}
