// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
	"os"
	"strings"
)

// Flow is a document generator that lays content out top-to-bottom and
// paginates automatically — the "flow" counterpart to the Rectangle-based
// drawing API. Add paragraphs, headings, images, tables, lists and spacers, then
// Render lays them into the document, appending pages as needed and (optionally)
// tagging each element for accessibility. Mirrors the intent of Aspose.PDF for
// .NET's generator / Paragraphs flow model. Obtain one with (*Document).NewFlow.
type Flow struct {
	doc            *Document
	w, h           float64
	mL, mR, mT, mB float64
	paraGap        float64
	cols           int
	colGap         float64
	tc             *TaggedContent
	elems          []flowElem
	mdOpts         *MarkdownOptions // theme/options for AddMarkdown
}

// FlowOptions configures a Flow. Zero values pick sensible defaults.
type FlowOptions struct {
	Format                                           PageFormat     // zero → A4
	MarginLeft, MarginRight, MarginTop, MarginBottom float64        // zero → 54pt
	ParagraphSpacing                                 float64        // gap after each block; zero → 6pt
	Columns                                          int            // ≤1 → single column; >1 → multi-column flow
	ColumnGap                                        float64        // gap between columns; zero → 18pt
	Tagged                                           *TaggedContent // non-nil → auto-tag elements (Tagged PDF / PDF/UA)
}

type flowKind int

const (
	fkParagraph flowKind = iota
	fkHeading
	fkImage
	fkTable
	fkList
	fkSpacer
	fkBox
	fkFloat
	fkColBreak
	fkRuns   // styled-runs paragraph (flow_runs.go)
	fkCustom // caller-supplied placement callback (markdown renderer)
)

type flowElem struct {
	kind       flowKind
	text       string
	style      TextStyle
	level      int
	imgPath    string
	imgW, imgH float64
	alt        string
	table      *Table
	items      []string
	ordered    bool
	height     float64
	box        *FloatingBox
	floatSide  FloatSide
	floatW     float64
	runs       []textRun              // fkRuns
	st         StructType             // fkRuns tagging type (StructP, StructH1…)
	custom     func(*flowState) error // fkCustom
}

// NewFlow creates a flow that renders into the document d.
func (d *Document) NewFlow(opts FlowOptions) *Flow {
	format := opts.Format
	if format.Width <= 0 || format.Height <= 0 {
		format = PageFormatA4
	}
	def := func(v, dflt float64) float64 {
		if v <= 0 {
			return dflt
		}
		return v
	}
	cols := opts.Columns
	if cols < 1 {
		cols = 1
	}
	return &Flow{
		doc:     d,
		w:       format.Width,
		h:       format.Height,
		mL:      def(opts.MarginLeft, 54),
		mR:      def(opts.MarginRight, 54),
		mT:      def(opts.MarginTop, 54),
		mB:      def(opts.MarginBottom, 54),
		paraGap: def(opts.ParagraphSpacing, 6),
		cols:    cols,
		colGap:  def(opts.ColumnGap, 18),
		tc:      opts.Tagged,
	}
}

// AddParagraph appends a paragraph of flowing text (wraps and splits across pages).
func (f *Flow) AddParagraph(text string, style TextStyle) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkParagraph, text: text, style: style})
	return f
}

// AddHeading appends a heading of the given level (1–6). Empty style fields get
// level-appropriate bold defaults.
func (f *Flow) AddHeading(level int, text string, style TextStyle) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkHeading, text: text, style: style, level: level})
	return f
}

// AddImage appends an image scaled to width×height points. If height <= 0 the
// image's aspect ratio is preserved.
func (f *Flow) AddImage(path string, width, height float64) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkImage, imgPath: path, imgW: width, imgH: height})
	return f
}

// AddImageAlt is AddImage with alternate text (used when the flow is tagged).
func (f *Flow) AddImageAlt(path string, width, height float64, alt string) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkImage, imgPath: path, imgW: width, imgH: height, alt: alt})
	return f
}

// AddTable appends a table (paginated when taller than a page).
func (f *Flow) AddTable(t *Table) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkTable, table: t})
	return f
}

// AddList appends a bulleted (ordered=false) or numbered list.
func (f *Flow) AddList(items []string, ordered bool, style TextStyle) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkList, items: items, ordered: ordered, style: style})
	return f
}

// AddSpacer appends vertical space.
func (f *Flow) AddSpacer(height float64) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkSpacer, height: height})
	return f
}

// AddColumnBreak forces the following content to start in the next column (or,
// from the last column, on the next page). A no-op in a single-column flow that
// is already at the top of a page.
func (f *Flow) AddColumnBreak() *Flow {
	f.elems = append(f.elems, flowElem{kind: fkColBreak})
	return f
}

// AddFloatingBox appends a floating box, which takes its measured height in the
// flow (moving to a new page if it does not fit).
func (f *Flow) AddFloatingBox(box *FloatingBox) *Flow {
	f.elems = append(f.elems, flowElem{kind: fkBox, box: box})
	return f
}

// flowState carries the cursor during Render.
type flowState struct {
	f                     *Flow
	page                  *Page
	y                     float64
	contentW, top, bottom float64
	pages                 int
	parent                *StructElement // tagging parent (nil = untagged)
	boxed                 bool           // true = single rectangle, no pagination
	col, cols             int            // current column / column count
	colGap                float64        // gap between columns
	floats                []activeFloat  // floated boxes the text currently wraps around
}

// errBoxFull stops layout inside a floating box when its rectangle is full.
var errBoxFull = fmt.Errorf("flow: box full")

// Render lays out the queued elements into the document and returns the number
// of pages the flow occupies.
func (f *Flow) Render() (int, error) {
	cols := f.cols
	if cols < 1 {
		cols = 1
	}
	totalW := f.w - f.mL - f.mR
	colW := (totalW - float64(cols-1)*f.colGap) / float64(cols)
	s := &flowState{
		f:        f,
		contentW: colW,
		top:      f.h - f.mT,
		bottom:   f.mB,
		cols:     cols,
		colGap:   f.colGap,
	}
	if colW <= 0 || s.top <= s.bottom {
		return 0, fmt.Errorf("flow: margins/columns leave no content area")
	}
	if f.tc != nil {
		s.parent = f.tc.Root()
	}
	if err := s.startPage(); err != nil {
		return 0, err
	}
	for _, el := range f.elems {
		if err := s.place(el); err != nil {
			return s.pages, err
		}
	}
	return s.pages, nil
}

// startPage reuses the document's single blank page if there is one, else
// appends a fresh page.
func (s *flowState) startPage() error {
	if s.f.doc.PageCount() == 1 {
		if p, err := s.f.doc.Page(1); err == nil {
			if data, _ := p.contentStreams(); len(strings.TrimSpace(string(data))) == 0 {
				if err := p.SetPageSize(s.f.w, s.f.h); err == nil {
					s.page = p
					s.y = s.top
					s.pages = 1
					return nil
				}
			}
		}
	}
	return s.newPage()
}

// advance moves to the next column, or the next page once the last column on the
// current page is full. Inside a box it stops layout instead (errBoxFull).
func (s *flowState) advance() error {
	if s.boxed {
		return errBoxFull
	}
	if s.col < s.cols-1 {
		s.col++
		s.y = s.top
		s.floats = nil
		return nil
	}
	return s.newPage()
}

// colLeft is the left X of the current column.
func (s *flowState) colLeft() float64 {
	return s.f.mL + float64(s.col)*(s.contentW+s.colGap)
}

func (s *flowState) newPage() error {
	if err := s.f.doc.AddBlankPage(s.f.w, s.f.h); err != nil {
		return err
	}
	p, err := s.f.doc.Page(s.f.doc.PageCount())
	if err != nil {
		return err
	}
	s.page = p
	s.col = 0
	s.y = s.top
	s.floats = nil
	s.pages++
	return nil
}

func (s *flowState) place(el flowElem) error {
	switch el.kind {
	case fkSpacer:
		s.y -= el.height
		if s.y < s.bottom {
			return s.advance()
		}
		return nil
	case fkParagraph:
		return s.flowText(el.text, paragraphStyle(el.style), StructP)
	case fkHeading:
		return s.flowText(el.text, headingStyle(el.style, el.level), headingType(el.level))
	case fkImage:
		return s.flowImage(el)
	case fkTable:
		return s.flowTable(el.table)
	case fkList:
		return s.flowList(el)
	case fkBox:
		return s.flowBox(el.box)
	case fkFloat:
		return s.placeFloat(el)
	case fkColBreak:
		// Already at the top of a fresh column with nothing drawn: stay put.
		if s.y >= s.top {
			return nil
		}
		return s.advance()
	case fkRuns:
		return s.flowRuns(el.runs, el.st)
	case fkCustom:
		return el.custom(s)
	}
	return nil
}

// flowText draws wrapping text, splitting it across pages line by line.
func (s *flowState) flowText(text string, style TextStyle, st StructType) error {
	if text == "" {
		return nil
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	width, _, err := fontWidthAndAscent(font, style.Size)
	if err != nil {
		return err
	}
	lh := style.Size * lineSpacingOf(style)
	if len(s.floats) > 0 {
		return s.flowTextAround(text, style, st, width, lh)
	}
	lines := wrapText(text, width, s.contentW)
	for len(lines) > 0 {
		fit := int((s.y - s.bottom) / lh)
		if fit < 1 {
			if err := s.advance(); err != nil {
				return err
			}
			fit = int((s.y - s.bottom) / lh)
			if fit < 1 {
				fit = 1 // a single line taller than the content area: draw anyway
			}
		}
		if fit > len(lines) {
			fit = len(lines)
		}
		chunk := strings.Join(lines[:fit], "\n")
		lines = lines[fit:]
		blockH := float64(fit) * lh
		rect := Rectangle{LLX: s.colLeft(), LLY: s.y - blockH, URX: s.colLeft() + s.contentW, URY: s.y}
		if err := s.draw(st, func() error { return s.page.AddText(chunk, style, rect) }); err != nil {
			return err
		}
		s.y -= blockH
	}
	s.y -= s.f.paraGap
	return nil
}

func (s *flowState) flowImage(el flowElem) error {
	s.dropBelowFloats()
	w, h := resolveImageSize(el)
	if w <= 0 {
		return fmt.Errorf("flow: image width must be positive")
	}
	if h > s.top-s.bottom {
		// Scale down to fit a full content area.
		h = s.top - s.bottom
	}
	if s.y-h < s.bottom {
		if err := s.advance(); err != nil {
			return err
		}
	}
	rect := Rectangle{LLX: s.colLeft(), LLY: s.y - h, URX: s.colLeft() + w, URY: s.y}
	st := StructFigure
	draw := func() error { return s.page.AddImage(el.imgPath, rect) }
	if s.parent != nil {
		fig, err := s.page.TagContent(s.parent, st, draw)
		if err != nil {
			return err
		}
		if el.alt != "" {
			fig.SetAlt(el.alt)
		}
	} else if err := draw(); err != nil {
		return err
	}
	s.y -= h + s.f.paraGap
	return nil
}

func (s *flowState) flowTable(t *Table) error {
	if t == nil {
		return nil
	}
	s.dropBelowFloats()
	heights, err := computeRowHeights(t)
	if err != nil {
		return err
	}
	var tableH float64
	for _, rh := range heights {
		tableH += rh
	}
	tableW := s.contentW
	if len(t.columnWidths) > 0 {
		var sum float64
		for _, c := range t.columnWidths {
			sum += c
		}
		if sum > 0 {
			tableW = sum
		}
	}
	// Place on the current page if it fits; otherwise on a fresh page; tables
	// taller than a page are paginated by AddTable.
	if tableH > s.y-s.bottom && tableH <= s.top-s.bottom {
		if err := s.advance(); err != nil {
			return err
		}
	}
	rect := Rectangle{LLX: s.colLeft(), LLY: s.bottom, URX: s.colLeft() + tableW, URY: s.y}
	var pagesAdded int
	if s.parent != nil {
		_, pagesAdded, err = s.page.AddTaggedTable(s.f.tc, s.parent, t, rect)
	} else {
		pagesAdded, err = s.page.AddTable(t, rect)
	}
	if err != nil {
		return err
	}
	if pagesAdded > 0 {
		// Multi-page table: continue after it on a fresh page.
		s.pages += pagesAdded
		p, e := s.f.doc.Page(s.f.doc.PageCount())
		if e != nil {
			return e
		}
		s.page = p
		s.y = s.bottom // force the next element onto a new page
	} else {
		s.y -= tableH + s.f.paraGap
	}
	return nil
}

func (s *flowState) flowList(el flowElem) error {
	s.dropBelowFloats()
	style := paragraphStyle(el.style)
	listH := listHeight(el.items, style, s.contentW)
	if listH > s.y-s.bottom && listH <= s.top-s.bottom {
		if err := s.advance(); err != nil {
			return err
		}
	}
	rect := Rectangle{LLX: s.colLeft(), LLY: s.bottom, URX: s.colLeft() + s.contentW, URY: s.y}
	var list *StructElement
	if s.parent != nil {
		list = s.parent.AddChild(StructList)
	}
	used, err := drawList(s.page, el.items, style, rect, el.ordered, list)
	if err != nil {
		return err
	}
	s.y -= used + s.f.paraGap
	return nil
}

// draw runs the drawing callback, wrapping it in a structure element of type st
// when the flow is tagged.
func (s *flowState) draw(st StructType, fn func() error) error {
	if s.parent != nil {
		_, err := s.page.TagContent(s.parent, st, fn)
		return err
	}
	return fn()
}

func lineSpacingOf(style TextStyle) float64 {
	if style.LineSpacing > 0 {
		return style.LineSpacing
	}
	return 1.2
}

func paragraphStyle(style TextStyle) TextStyle {
	if style.Font == nil {
		style.Font = FontHelvetica
	}
	if style.Size <= 0 {
		style.Size = 12
	}
	return style
}

var headingSizes = map[int]float64{1: 24, 2: 18, 3: 14, 4: 12, 5: 11, 6: 10}

func headingStyle(style TextStyle, level int) TextStyle {
	if style.Font == nil {
		style.Font = FontHelveticaBold
	}
	if style.Size <= 0 {
		sz, ok := headingSizes[level]
		if !ok {
			sz = 13
		}
		style.Size = sz
	}
	return style
}

func headingType(level int) StructType {
	switch level {
	case 1:
		return StructH1
	case 2:
		return StructH2
	case 3:
		return StructH3
	case 4:
		return StructH4
	case 5:
		return StructH5
	case 6:
		return StructH6
	}
	return StructH
}

// imageAspect returns an image file's pixel dimensions without fully decoding it.
func imageAspect(path string) (w, h int, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = file.Close() }()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
