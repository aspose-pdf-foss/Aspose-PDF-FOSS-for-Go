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
	list := parent.AddChild(StructList)
	if _, err := drawList(p, items, style, rect, ordered, list); err != nil {
		return nil, err
	}
	return list, nil
}

// listMetrics returns the label column width and inter-item gap for a list at
// the given font size.
func listMetrics(size float64) (labelW, gap float64) {
	if size <= 0 {
		size = 12
	}
	return size * 1.9, size * 0.4 // room for "99." plus a gap
}

// listHeight returns the total height a list of items occupies within contentW.
func listHeight(items []string, style TextStyle, contentW float64) float64 {
	labelW, gap := listMetrics(style.Size)
	bodyW := contentW - labelW
	if bodyW <= 0 {
		return 0
	}
	var total float64
	for _, item := range items {
		lines, lineH, err := measureText(item, style, bodyW)
		if err != nil || lines < 1 {
			lines = 1
		}
		total += float64(lines)*lineH + gap
	}
	return total
}

// drawList lays out a bulleted/numbered list in rect and returns the height
// consumed. When list is non-nil each item is tagged as /LI → /Lbl+/LBody under
// it (the page must belong to a tagged document); otherwise the items are drawn
// untagged.
func drawList(page *Page, items []string, style TextStyle, rect Rectangle, ordered bool, list *StructElement) (float64, error) {
	labelW, gap := listMetrics(style.Size)
	bodyW := rect.URX - (rect.LLX + labelW)
	if bodyW <= 0 {
		return 0, fmt.Errorf("list: rect too narrow")
	}
	startY := rect.URY
	y := startY
	for i, item := range items {
		lines, lineH, err := measureText(item, style, bodyW)
		if err != nil {
			return startY - y, err
		}
		if lines < 1 {
			lines = 1
		}
		itemH := float64(lines) * lineH
		if y-itemH < rect.LLY {
			break // would overflow the rectangle
		}
		label := "•"
		if ordered {
			label = fmt.Sprintf("%d.", i+1)
		}
		labelRect := Rectangle{LLX: rect.LLX, LLY: y - lineH, URX: rect.LLX + labelW - gap, URY: y}
		bodyRect := Rectangle{LLX: rect.LLX + labelW, LLY: y - itemH, URX: rect.URX, URY: y}
		drawLabel := func() error { return page.AddText(label, style, labelRect) }
		drawBody := func() error { return page.AddText(item, style, bodyRect) }

		if list != nil {
			li := list.AddChild(StructListItem)
			if _, err := page.TagContent(li, StructLabel, drawLabel); err != nil {
				return startY - y, err
			}
			if _, err := page.TagContent(li, StructListBody, drawBody); err != nil {
				return startY - y, err
			}
		} else {
			if err := drawLabel(); err != nil {
				return startY - y, err
			}
			if err := drawBody(); err != nil {
				return startY - y, err
			}
		}
		y -= itemH + gap
	}
	return startY - y, nil
}
