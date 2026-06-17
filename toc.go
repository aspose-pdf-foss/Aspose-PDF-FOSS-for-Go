// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strconv"
)

// TOCEntry is one line of a table of contents: a title, an indent level
// (0 = top level), and the page it points at. A nil Page renders the line
// without a page number or link.
type TOCEntry struct {
	Title string
	Level int
	Page  *Page
	// Label overrides the displayed page number. Empty uses the target
	// Page's 1-based number; set it to show a logical label instead (e.g.
	// a /PageLabels label like "iv" or a body number that differs from the
	// physical page index). The link still targets Page.
	Label string
}

// TOCOptions controls how a table of contents is rendered. The zero value
// produces a sensible TOC: black 12pt entries, an 18pt-per-level indent,
// 1.6 line spacing, dotted leaders, page numbers, and clickable links.
// The negative ("No…") booleans turn those defaults off so that the zero
// value stays the full-featured one.
//
// Loosely mirrors Aspose.PDF for .NET's TocInfo (a title plus per-entry
// formatting and dotted leaders).
type TOCOptions struct {
	// Title is the heading drawn above the entries (e.g. "Table of
	// Contents"). Empty draws no heading.
	Title string
	// TitleStyle styles the heading. Zero value = Helvetica-Bold 18, black.
	TitleStyle TextStyle
	// EntryStyle styles every entry line. Zero value = Helvetica 12, black.
	// HAlign/VAlign are managed internally and ignored here.
	EntryStyle TextStyle
	// IndentStep is the horizontal indent added per level, in points.
	// Zero = 18.
	IndentStep float64
	// LineSpacing multiplies the entry font size to get the row height.
	// Zero = 1.6.
	LineSpacing float64
	// Margin is the page margin used for the pages GenerateTOC inserts,
	// in points. Zero = 54. Ignored by Page.AddTOC (which uses its rect).
	Margin float64

	NoPageNumbers bool // zero value draws page numbers
	NoLeader      bool // zero value draws a dotted leader between title and number
	NoLinks       bool // zero value adds a GoTo link over each entry
}

// tocOpts is the resolved (defaulted) form of TOCOptions.
type tocOpts struct {
	title       string
	titleStyle  TextStyle
	entryStyle  TextStyle
	entryFont   Font
	entrySize   float64
	indentStep  float64
	lineSpacing float64
	margin      float64
	numbers     bool
	leader      bool
	links       bool
}

func resolveTOCOptions(opts []TOCOptions) tocOpts {
	var in TOCOptions
	if len(opts) > 0 {
		in = opts[len(opts)-1]
	}
	o := tocOpts{
		title:       in.Title,
		titleStyle:  in.TitleStyle,
		entryStyle:  in.EntryStyle,
		indentStep:  in.IndentStep,
		lineSpacing: in.LineSpacing,
		margin:      in.Margin,
		numbers:     !in.NoPageNumbers,
		leader:      !in.NoLeader,
		links:       !in.NoLinks,
	}
	if o.indentStep == 0 {
		o.indentStep = 18
	}
	if o.lineSpacing == 0 {
		o.lineSpacing = 1.6
	}
	if o.margin == 0 {
		o.margin = 54
	}
	if o.titleStyle.Font == nil {
		o.titleStyle.Font = FontHelveticaBold
	}
	if o.titleStyle.Size == 0 {
		o.titleStyle.Size = 18
	}
	if o.titleStyle.Color == nil {
		o.titleStyle.Color = &Color{A: 1}
	}
	if o.entryStyle.Font == nil {
		o.entryStyle.Font = FontHelvetica
	}
	if o.entryStyle.Size == 0 {
		o.entryStyle.Size = 12
	}
	if o.entryStyle.Color == nil {
		o.entryStyle.Color = &Color{A: 1}
	}
	o.entryFont = o.entryStyle.Font
	o.entrySize = o.entryStyle.Size
	return o
}

// titleHeight is the vertical space reserved for the heading block (0 when
// there is no title).
func (o tocOpts) titleHeight() float64 {
	if o.title == "" {
		return 0
	}
	return o.titleStyle.Size*1.5 + 6
}

// AddTOC renders a table of contents from a supplied list of entries into
// rect on this page. Entries flow top-to-bottom; long titles are
// truncated to fit before the page-number column. When the entries
// overflow rect, continuation pages of the same size are appended to the
// document and the number of pages added is returned (0 if everything fits
// in rect).
//
// Each entry with a non-nil Page is drawn with its page number and a
// clickable GoTo link (unless disabled in TOCOptions). Mirrors building a
// TOC region by hand in Aspose.PDF for .NET.
func (p *Page) AddTOC(entries []TOCEntry, rect Rectangle, opts ...TOCOptions) (int, error) {
	if p == nil {
		return 0, fmt.Errorf("AddTOC: nil page")
	}
	o := resolveTOCOptions(opts)
	ewidth, _, err := fontWidthAndAscent(o.entryFont, o.entrySize)
	if err != nil {
		return 0, err
	}
	sz, err := p.Size()
	if err != nil {
		return 0, err
	}
	lineH := o.entrySize * o.lineSpacing

	cur := p
	y := rect.URY
	if o.title != "" {
		if err := cur.AddText(o.title, o.titleStyle, Rectangle{
			LLX: rect.LLX, LLY: rect.URY - o.titleHeight(), URX: rect.URX, URY: rect.URY,
		}); err != nil {
			return 0, err
		}
		y = rect.URY - o.titleHeight()
	}

	pagesAdded := 0
	for _, e := range entries {
		if y-lineH < rect.LLY {
			if err := p.doc.AddBlankPage(sz.Width, sz.Height); err != nil {
				return pagesAdded, err
			}
			cur, err = p.doc.Page(p.doc.PageCount())
			if err != nil {
				return pagesAdded, err
			}
			pagesAdded++
			y = rect.URY
		}
		num := 0
		top := 0.0
		if e.Page != nil {
			num = e.Page.Number()
			if s, err := e.Page.Size(); err == nil {
				top = s.Height
			}
		}
		numStr := ""
		if o.numbers {
			if e.Label != "" {
				numStr = e.Label
			} else if e.Page != nil {
				numStr = strconv.Itoa(num)
			}
		}
		if err := cur.drawTOCRow(rect, y, e.Title, e.Level, numStr, o, ewidth); err != nil {
			return pagesAdded, err
		}
		if e.Page != nil && o.links {
			if err := cur.addTOCLink(rect, y, lineH, num, top); err != nil {
				return pagesAdded, err
			}
		}
		y -= lineH
	}
	return pagesAdded, nil
}

// GenerateTOC builds a table of contents from the document's outline
// (bookmark) tree and inserts it as new page(s) at the front of the
// document. Outline nesting becomes the indent level. Returns the number
// of pages added (0 when the document has no outline entries).
//
// Page numbers and GoTo links reflect the final page order after the TOC
// pages are inserted. Mirrors Aspose.PDF for .NET's TocInfo-driven TOC
// generation, sourced from the document outline.
func (d *Document) GenerateTOC(opts ...TOCOptions) (int, error) {
	o := resolveTOCOptions(opts)
	items := collectTOCFromOutlines(d)
	if len(items) == 0 {
		return 0, nil
	}

	// Page geometry from the current first page (A4 fallback for an empty
	// document, though GenerateTOC with entries implies pages exist).
	pw, ph := PageFormatA4.Width, PageFormatA4.Height
	if len(d.pages) > 0 {
		if first, err := d.Page(1); err == nil {
			if s, err := first.Size(); err == nil {
				pw, ph = s.Width, s.Height
			}
		}
	}
	rect := Rectangle{LLX: o.margin, LLY: o.margin, URX: pw - o.margin, URY: ph - o.margin}
	lineH := o.entrySize * o.lineSpacing
	firstTop := rect.URY - o.titleHeight()

	k := paginateTOC(len(items), firstTop, rect.URY, rect.LLY, lineH)

	// Insert k blank pages at the front (positions 1..k), contiguous.
	for i := 0; i < k; i++ {
		if err := d.InsertBlankPage(i+1, pw, ph); err != nil {
			return 0, err
		}
	}

	// Resolve each target to its final 1-based page number (the underlying
	// object pointer is stable across the insertion).
	numOf := func(obj *pdfObject) int {
		for i, p := range d.pages {
			if p == obj {
				return i + 1
			}
		}
		return 0
	}

	ewidth, _, err := fontWidthAndAscent(o.entryFont, o.entrySize)
	if err != nil {
		return k, err
	}

	tocPages := make([]*Page, k)
	for i := 0; i < k; i++ {
		tocPages[i], err = d.Page(i + 1)
		if err != nil {
			return k, err
		}
	}

	pageIdx := 0
	y := rect.URY
	if o.title != "" {
		if err := tocPages[0].AddText(o.title, o.titleStyle, Rectangle{
			LLX: rect.LLX, LLY: rect.URY - o.titleHeight(), URX: rect.URX, URY: rect.URY,
		}); err != nil {
			return k, err
		}
		y = firstTop
	}

	for _, it := range items {
		if y-lineH < rect.LLY {
			pageIdx++
			if pageIdx >= k {
				break // measured to fit; guard against rounding
			}
			y = rect.URY
		}
		cur := tocPages[pageIdx]
		num := 0
		if it.hasTarget {
			num = numOf(it.targetObj)
		}
		numStr := ""
		if o.numbers && num > 0 {
			numStr = strconv.Itoa(num)
		}
		if err := cur.drawTOCRow(rect, y, it.title, it.level, numStr, o, ewidth); err != nil {
			return k, err
		}
		if it.hasTarget && o.links && num > 0 {
			if err := cur.addTOCLink(rect, y, lineH, num, it.targetTop); err != nil {
				return k, err
			}
		}
		y -= lineH
	}
	return k, nil
}

// tocItem is an internal TOC line with a stable target reference (the
// underlying page object) so a page number can be resolved after pages are
// inserted ahead of the target.
type tocItem struct {
	title     string
	level     int
	targetObj *pdfObject
	targetTop float64
	hasTarget bool
}

// collectTOCFromOutlines flattens the outline tree into TOC items, using
// nesting depth as the indent level. Each item's target is captured as the
// stable underlying page object.
func collectTOCFromOutlines(d *Document) []tocItem {
	var items []tocItem
	var walk func(nodes []*OutlineItemCollection, level int)
	walk = func(nodes []*OutlineItemCollection, level int) {
		for _, n := range nodes {
			it := tocItem{title: n.Title(), level: level}
			if pg := outlineTargetPage(d, n); pg != nil {
				it.targetObj = pg.pageObj()
				if s, err := pg.Size(); err == nil {
					it.targetTop = s.Height
				}
				it.hasTarget = true
			}
			items = append(items, it)
			walk(n.All(), level+1)
		}
	}
	walk(d.Outlines().All(), 0)
	return items
}

// outlineTargetPage resolves the page an outline item points at, via its
// explicit /Dest first, then a /GoTo action. Returns nil when neither
// resolves to a page in this document.
func outlineTargetPage(d *Document, n *OutlineItemCollection) *Page {
	if dest := n.Destination(); dest != nil {
		if pg := dest.Page(); pg != nil {
			return pg
		}
	}
	if act, ok := n.Action().(*GoToAction); ok {
		if num := act.PageNum(); num >= 1 && num <= len(d.pages) {
			if pg, err := d.Page(num); err == nil {
				return pg
			}
		}
	}
	return nil
}

// paginateTOC returns how many pages a run of n single-line entries needs,
// using the exact placement rule the render loop applies: start at
// firstTop, drop to top on each new page, advancing by lineH per entry.
func paginateTOC(n int, firstTop, top, bottom, lineH float64) int {
	pages := 1
	y := firstTop
	for i := 0; i < n; i++ {
		if y-lineH < bottom {
			pages++
			y = top
		}
		y -= lineH
	}
	return pages
}

// drawTOCRow renders one TOC line at vertical position yTop: an
// (indented, truncated) title on the left, a right-aligned page number
// (numStr — already formatted, may be a logical label or empty), and an
// optional dotted leader between them.
func (p *Page) drawTOCRow(rect Rectangle, yTop float64, title string, level int, numStr string, o tocOpts, ewidth widthFn) error {
	lineH := o.entrySize * o.lineSpacing
	yBot := yTop - lineH
	indentX := rect.LLX + float64(level)*o.indentStep

	numW := measureString(numStr, ewidth)
	numColX := rect.URX - numW

	titleMaxW := numColX - 6 - indentX
	if titleMaxW < 1 {
		titleMaxW = 1
	}
	shown := truncateToWidth(title, ewidth, titleMaxW)

	if err := p.AddText(shown, o.entryStyle, Rectangle{
		LLX: indentX, LLY: yBot, URX: numColX - 4, URY: yTop,
	}); err != nil {
		return err
	}

	if numStr != "" {
		numStyle := o.entryStyle
		numStyle.HAlign = HAlignRight
		if err := p.AddText(numStr, numStyle, Rectangle{
			LLX: indentX, LLY: yBot, URX: rect.URX, URY: yTop,
		}); err != nil {
			return err
		}
	}

	if o.leader && numStr != "" {
		titleW := measureString(shown, ewidth)
		x0 := indentX + titleW + 4
		x1 := numColX - 4
		if x1 > x0 {
			// Align the leader with the glyph baseline. AddText is
			// top-anchored, so text sits near yTop with a baseline roughly
			// 0.8·size below it; place the dots just under that.
			leaderY := yTop - o.entrySize*0.92
			col := o.entryStyle.Color
			if err := p.DrawLine(
				Point{X: x0, Y: leaderY}, Point{X: x1, Y: leaderY},
				LineStyle{Color: col, Width: 0.8, DashPattern: []float64{0.75, 3}, Cap: LineCapRound},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// addTOCLink overlays a borderless GoTo link covering the row, navigating
// to the top of the target page.
func (p *Page) addTOCLink(rect Rectangle, yTop, lineH float64, pageNum int, top float64) error {
	link := NewLinkAnnotation(p, Rectangle{
		LLX: rect.LLX, LLY: yTop - lineH, URX: rect.URX, URY: yTop,
	})
	gt := NewGoToAction(pageNum, top)
	gt.doc = p.doc // page-ref destination on encode
	link.SetAction(gt)
	link.SetBorderWidth(0)
	return p.Annotations().Add(link)
}

// truncateToWidth shortens s to fit maxW under the width function,
// appending "..." when it has to cut. Returns s unchanged when it fits.
func truncateToWidth(s string, width widthFn, maxW float64) string {
	if measureString(s, width) <= maxW {
		return s
	}
	const ell = "..."
	ellW := measureString(ell, width)
	var b []rune
	var w float64
	for _, r := range s {
		cw := width(r)
		if w+cw+ellW > maxW {
			break
		}
		b = append(b, r)
		w += cw
	}
	return string(b) + ell
}
