// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Markdown → PDF (epic pdf-go-fh4l.4): render CommonMark + GFM documents
// through the flow layout with one default typographic theme. Mirrors
// Aspose.PDF for .NET's Markdown import (`new Document(md, MdLoadOptions())`)
// in this library's exported-func idiom (like ImageToDocument).
//
// Dialect: full CommonMark 0.31 (the parser passes the official spec test
// suite 652/652) plus the GFM core extensions — tables, strikethrough, task
// lists, autolinks. Raw HTML is skipped (inline <br> is honored). Images:
// local paths (resolved against BasePath) and data: URLs; remote URLs render
// their alt text as a placeholder — the library performs no network I/O.

// MarkdownOptions configures Markdown rendering. The zero value gives A4,
// 54pt margins, Helvetica/Courier at 11pt.
type MarkdownOptions struct {
	Format PageFormat // zero → A4
	Margin float64    // page margin; zero → 54pt

	// Fonts. BaseFont nil → Helvetica (Standard-14, Latin-1 text only —
	// load a Unicode face with Document.LoadFont for Cyrillic, Greek, …).
	// The style variants default to the matching Standard-14 faces when
	// BaseFont is nil, and to BaseFont itself otherwise.
	BaseFont, BoldFont, ItalicFont, BoldItalicFont Font
	CodeFont                                       Font    // nil → Courier
	BaseSize                                       float64 // body size; zero → 11pt (headings scale from it)

	// BasePath resolves relative image paths. Empty → the .md file's
	// directory (MarkdownToDocument) or the current directory.
	BasePath string

	// Tagged builds a logical structure tree while rendering (headings,
	// paragraphs, lists, tables, figures) for accessible (PDF/UA) output.
	Tagged bool
	// Title and Language feed the tagged document's metadata (PDF/UA
	// requires both). Title empty → derived from the first heading;
	// Language empty → "en". Ignored when Tagged is false.
	Title    string
	Language string
}

// mdTheme is the resolved look derived from MarkdownOptions.
type mdTheme struct {
	base, bold, italic, boldItalic, code Font
	size                                 float64
	linkColor                            Color
	codeBg                               Color
	ruleColor                            Color
}

var mdHeadingScale = [6]float64{1.9, 1.5, 1.25, 1.1, 1.0, 0.9}

func (o MarkdownOptions) theme() mdTheme {
	th := mdTheme{
		base: o.BaseFont, bold: o.BoldFont, italic: o.ItalicFont,
		boldItalic: o.BoldItalicFont, code: o.CodeFont,
		size:      o.BaseSize,
		linkColor: Color{R: 0.05, G: 0.35, B: 0.7, A: 1},
		codeBg:    Color{R: 0.94, G: 0.94, B: 0.94, A: 1},
		ruleColor: Color{R: 0.7, G: 0.7, B: 0.7, A: 1},
	}
	if th.size == 0 {
		th.size = 11
	}
	if th.base == nil {
		th.base = FontHelvetica
		if th.bold == nil {
			th.bold = FontHelveticaBold
		}
		if th.italic == nil {
			th.italic = FontHelveticaOblique
		}
		if th.boldItalic == nil {
			th.boldItalic = FontHelveticaBoldOblique
		}
	}
	if th.bold == nil {
		th.bold = th.base
	}
	if th.italic == nil {
		th.italic = th.base
	}
	if th.boldItalic == nil {
		th.boldItalic = th.bold
	}
	if th.code == nil {
		th.code = FontCourier
	}
	return th
}

// MarkdownToDocument renders a Markdown file as a new PDF document.
func MarkdownToDocument(path string, opts ...MarkdownOptions) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("markdown: %w", err)
	}
	o := firstMarkdownOptions(opts)
	if o.BasePath == "" {
		o.BasePath = filepath.Dir(path)
	}
	return markdownDocument(string(data), o)
}

// MarkdownToDocumentFromStream renders Markdown read from r as a new PDF
// document.
func MarkdownToDocumentFromStream(r io.Reader, opts ...MarkdownOptions) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("markdown: %w", err)
	}
	return markdownDocument(string(data), firstMarkdownOptions(opts))
}

func firstMarkdownOptions(opts []MarkdownOptions) MarkdownOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return MarkdownOptions{}
}

func markdownDocument(md string, o MarkdownOptions) (*Document, error) {
	format := o.Format
	if format.Width <= 0 || format.Height <= 0 {
		format = PageFormatA4
	}
	doc := NewDocumentFromFormat(format)
	fo := FlowOptions{Format: format, MarginLeft: o.Margin, MarginRight: o.Margin, MarginTop: o.Margin, MarginBottom: o.Margin}
	if o.Tagged {
		tc := doc.TaggedContent()
		title := o.Title
		if title == "" {
			title = firstHeadingText(md)
		}
		if title == "" {
			title = "Document"
		}
		tc.SetTitle(title)
		lang := o.Language
		if lang == "" {
			lang = "en"
		}
		tc.SetLanguage(lang)
		fo.Tagged = tc
	}
	flow := doc.NewFlow(fo)
	flow.SetMarkdownOptions(o)
	flow.AddMarkdown(md)
	if _, err := flow.Render(); err != nil {
		return nil, err
	}
	return doc, nil
}

// firstHeadingText returns the plain text of the document's first heading
// (the default tagged-PDF title).
func firstHeadingText(md string) string {
	doc := parseMarkdown(md)
	var find func(*mdBlock) string
	find = func(b *mdBlock) string {
		if b.kind == mdHeading {
			return strings.TrimSpace(mdInlinesPlain(b.inlines))
		}
		for _, c := range b.children {
			if t := find(c); t != "" {
				return t
			}
		}
		return ""
	}
	return find(doc)
}

// SetMarkdownOptions sets the theme/options used by subsequent AddMarkdown
// calls on this flow (fonts, base size, image base path).
func (f *Flow) SetMarkdownOptions(o MarkdownOptions) *Flow {
	f.mdOpts = &o
	return f
}

// AddMarkdown appends a Markdown fragment to the flow: its blocks become
// flow elements (headings, paragraphs, lists, tables, images, …) rendered
// with the flow's Markdown options.
func (f *Flow) AddMarkdown(md string) *Flow {
	o := MarkdownOptions{}
	if f.mdOpts != nil {
		o = *f.mdOpts
	}
	r := &mdRender{th: o.theme(), basePath: o.BasePath}
	blocks := parseMarkdown(md)
	f.elems = append(f.elems, flowElem{kind: fkCustom, custom: func(s *flowState) error {
		return r.renderBlocks(s, blocks.children, 0)
	}})
	return f
}

// AddMarkdown renders a Markdown fragment inside the rectangle. Content that
// does not fit is clipped (same convention as FloatingBox).
func (p *Page) AddMarkdown(md string, rect Rectangle, opts ...MarkdownOptions) error {
	if err := rect.validate(); err != nil {
		return fmt.Errorf("add markdown: %w", err)
	}
	o := firstMarkdownOptions(opts)
	f := &Flow{doc: p.doc, w: rect.URX, h: rect.URY, mL: rect.LLX, paraGap: 6}
	r := &mdRender{th: o.theme(), basePath: o.BasePath}
	blocks := parseMarkdown(md)
	s := &flowState{
		f: f, page: p, y: rect.URY, top: rect.URY, bottom: rect.LLY,
		contentW: rect.URX - rect.LLX, boxed: true, cols: 1, pages: 1,
	}
	err := r.renderBlocks(s, blocks.children, 0)
	if err == errBoxFull {
		return nil // clipped, like FloatingBox
	}
	return err
}

// --- renderer -------------------------------------------------------------------

type mdRender struct {
	th       mdTheme
	basePath string
	refmap   map[string]mdLinkRef
	tmpSeq   int
}

// runCtx carries the inline style context while flattening the inline tree.
type runCtx struct {
	bold, italic, strike bool
	size                 float64
	link                 string
}

func (r *mdRender) styleFor(ctx runCtx) TextStyle {
	st := TextStyle{Size: ctx.size, Strikethrough: ctx.strike}
	switch {
	case ctx.bold && ctx.italic:
		st.Font = r.th.boldItalic
	case ctx.bold:
		st.Font = r.th.bold
	case ctx.italic:
		st.Font = r.th.italic
	default:
		st.Font = r.th.base
	}
	if ctx.link != "" {
		c := r.th.linkColor
		st.Color = &c
		st.Underline = true
	}
	return st
}

// runsFor flattens an inline tree into styled text runs.
func (r *mdRender) runsFor(nodes []*mdInline, ctx runCtx) []textRun {
	var runs []textRun
	for _, n := range nodes {
		switch n.kind {
		case mdText:
			runs = append(runs, textRun{text: n.text, style: r.styleFor(ctx), linkDest: linkDestOf(ctx)})
		case mdSoftBreak:
			runs = append(runs, textRun{text: " ", style: r.styleFor(ctx)})
		case mdHardBreak:
			runs = append(runs, textRun{brk: true})
		case mdCodeSpan:
			st := r.styleFor(ctx)
			st.Font = r.th.code
			st.Size = ctx.size * 0.9
			bg := r.th.codeBg
			st.Background = &bg
			runs = append(runs, textRun{text: n.text, style: st, linkDest: linkDestOf(ctx)})
		case mdEmph:
			c := ctx
			c.italic = true
			runs = append(runs, r.runsFor(n.children, c)...)
		case mdStrong:
			c := ctx
			c.bold = true
			runs = append(runs, r.runsFor(n.children, c)...)
		case mdStrike:
			c := ctx
			c.strike = true
			runs = append(runs, r.runsFor(n.children, c)...)
		case mdLink:
			c := ctx
			c.link = n.dest
			runs = append(runs, r.runsFor(n.children, c)...)
		case mdImage:
			// Inline images render their alt text.
			runs = append(runs, r.runsFor(n.children, ctx)...)
		case mdHTMLInline:
			if strings.HasPrefix(n.text, "<br") {
				runs = append(runs, textRun{brk: true})
			}
		}
	}
	return runs
}

// linkDestOf filters destinations to those a LinkAnnotation can carry (v1:
// external URIs; internal #fragments are rendered as styled text only).
func linkDestOf(ctx runCtx) string {
	if strings.HasPrefix(ctx.link, "#") {
		return ""
	}
	return ctx.link
}

func mdInlinesPlain(nodes []*mdInline) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.kind {
		case mdText, mdCodeSpan:
			b.WriteString(n.text)
		case mdSoftBreak, mdHardBreak:
			b.WriteByte(' ')
		default:
			b.WriteString(mdInlinesPlain(n.children))
		}
	}
	return b.String()
}

func (r *mdRender) renderBlocks(s *flowState, blocks []*mdBlock, indent float64) error {
	for _, b := range blocks {
		if err := r.renderBlock(s, b, indent); err != nil {
			return err
		}
	}
	return nil
}

func (r *mdRender) renderBlock(s *flowState, b *mdBlock, indent float64) error {
	switch b.kind {
	case mdParagraph:
		if img, ok := blockImage(b); ok {
			return r.renderImage(s, img, indent)
		}
		return s.flowRunsIndent(r.runsFor(b.inlines, runCtx{size: r.th.size}), StructP, indent)
	case mdHeading:
		lvl := b.level
		if lvl < 1 {
			lvl = 1
		}
		if lvl > 6 {
			lvl = 6
		}
		ctx := runCtx{bold: true, size: r.th.size * mdHeadingScale[lvl-1]}
		return s.flowRunsIndent(r.runsFor(b.inlines, ctx), headingType(lvl), indent)
	case mdCodeBlock:
		return r.renderCodeBlock(s, b.literal, indent)
	case mdBlockQuote:
		return r.renderQuote(s, b, indent)
	case mdList:
		return r.renderList(s, b, indent)
	case mdThematicBreak:
		return r.renderRule(s, indent)
	case mdTable:
		t := r.buildTable(b)
		if n := len(b.headerCells); n > 0 {
			widths := make([]float64, n)
			for i := range widths {
				widths[i] = (s.contentW - indent) / float64(n)
			}
			t.SetColumnWidths(widths)
		}
		return s.flowTable(t)
	case mdHTMLBlock:
		return nil // raw HTML blocks are skipped by design
	}
	return nil
}

// blockImage detects a paragraph consisting solely of one image.
func blockImage(b *mdBlock) (*mdInline, bool) {
	if len(b.inlines) == 1 && b.inlines[0].kind == mdImage {
		return b.inlines[0], true
	}
	return nil, false
}

func (r *mdRender) renderImage(s *flowState, img *mdInline, indent float64) error {
	alt := mdInlinesPlain(img.children)
	path, err := r.resolveImagePath(img.dest)
	if err != nil || path == "" {
		// Remote/unresolvable image: alt-text placeholder.
		st := TextStyle{Font: r.th.italic, Size: r.th.size, Color: &Color{R: 0.4, G: 0.4, B: 0.4, A: 1}}
		text := alt
		if text == "" {
			text = img.dest
		}
		return s.flowRunsIndent([]textRun{{text: "[" + text + "]", style: st}}, StructP, indent)
	}
	pxw, _, err := imageAspect(path)
	if err != nil {
		return fmt.Errorf("markdown: image %s: %w", path, err)
	}
	w := float64(pxw) * 0.75 // 96 dpi px → pt
	if max := s.contentW - indent; w > max {
		w = max
	}
	return s.flowImage(flowElem{imgPath: path, imgW: w, imgH: 0, alt: alt})
}

// resolveImagePath maps a Markdown image destination to a local file path:
// relative/absolute paths (against basePath) and data: URLs (decoded to a
// temp file); remote URLs return "".
func (r *mdRender) resolveImagePath(dest string) (string, error) {
	low := strings.ToLower(dest)
	switch {
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return "", nil
	case strings.HasPrefix(low, "data:"):
		comma := strings.IndexByte(dest, ',')
		if comma < 0 || !strings.Contains(dest[:comma], "base64") {
			return "", nil
		}
		raw, err := base64.StdEncoding.DecodeString(dest[comma+1:])
		if err != nil {
			return "", nil
		}
		f, err := os.CreateTemp("", "mdimg*.bin")
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.Write(raw); err != nil {
			return "", err
		}
		return f.Name(), nil
	}
	if filepath.IsAbs(dest) {
		return dest, nil
	}
	return filepath.Join(r.basePath, filepath.FromSlash(dest)), nil
}

func (r *mdRender) renderCodeBlock(s *flowState, literal string, indent float64) error {
	style := TextStyle{Font: r.th.code, Size: r.th.size * 0.9}
	m, err := metricsFor(style)
	if err != nil {
		return err
	}
	const pad = 6.0
	// Preserve leading indentation: AddText's word wrap strips leading
	// spaces, so each code line is drawn separately at its own x offset.
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(literal, "\t", "    "), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	spaceW := m.width(' ')
	for len(lines) > 0 {
		if s.y-(m.lineHeight+2*pad) < s.bottom {
			if err := s.advance(); err != nil {
				return err
			}
		}
		fit := int((s.y - s.bottom - 2*pad) / m.lineHeight)
		if fit < 1 {
			fit = 1
		}
		if fit > len(lines) {
			fit = len(lines)
		}
		chunk := lines[:fit]
		lines = lines[fit:]
		h := float64(fit)*m.lineHeight + 2*pad
		box := Rectangle{LLX: s.colLeft() + indent, LLY: s.y - h, URX: s.colLeft() + s.contentW, URY: s.y}
		bg := r.th.codeBg
		frame := func() error {
			return s.page.DrawRectangle(box, ShapeStyle{FillColor: &bg})
		}
		if err := s.artifactDraw(frame); err != nil {
			return err
		}
		text := func() error {
			y := box.URY - pad
			for _, line := range chunk {
				lead := 0
				for lead < len(line) && line[lead] == ' ' {
					lead++
				}
				if trimmed := line[lead:]; trimmed != "" {
					rect := Rectangle{
						LLX: box.LLX + pad + float64(lead)*spaceW,
						LLY: y - m.lineHeight,
						// Generous right edge: a long code line overflows
						// (clipped at the page) rather than wrapping over
						// the next line.
						URX: box.LLX + pad + float64(lead)*spaceW + measureString(trimmed, m.width) + 4,
						URY: y,
					}
					if err := s.page.AddText(trimmed, style, rect); err != nil {
						return err
					}
				}
				y -= m.lineHeight
			}
			return nil
		}
		if err := s.draw(StructP, text); err != nil {
			return err
		}
		s.y -= h + 2
	}
	s.y -= s.f.paraGap
	return nil
}

func (r *mdRender) renderQuote(s *flowState, b *mdBlock, indent float64) error {
	yStart := s.y
	pageStart := s.page
	if err := r.renderBlocks(s, b.children, indent+14); err != nil {
		return err
	}
	// Vertical bar over the quoted extent (on the starting page; when the
	// quote crossed a page/column boundary the bar runs to the bottom).
	yEnd := s.y + s.f.paraGap
	if s.page != pageStart {
		yEnd = s.bottom
	}
	if yEnd < yStart {
		bar := r.th.ruleColor
		line := func() error {
			return pageStart.DrawLine(
				Point{X: s.colLeft() + indent + 4, Y: yStart},
				Point{X: s.colLeft() + indent + 4, Y: yEnd},
				LineStyle{Color: &bar, Width: 2.5},
			)
		}
		if err := s.artifactDraw(line); err != nil {
			return err
		}
	}
	return nil
}

func (r *mdRender) renderRule(s *flowState, indent float64) error {
	if s.y-16 < s.bottom {
		if err := s.advance(); err != nil {
			return err
		}
	}
	c := r.th.ruleColor
	y := s.y - 8
	line := func() error {
		return s.page.DrawLine(Point{X: s.colLeft() + indent, Y: y}, Point{X: s.colLeft() + s.contentW, Y: y}, LineStyle{Color: &c, Width: 0.75})
	}
	if err := s.artifactDraw(line); err != nil {
		return err
	}
	s.y -= 16
	return nil
}

// renderList draws bullet/ordered/task lists with nesting; each item's label
// hangs left of the item content.
func (r *mdRender) renderList(s *flowState, list *mdBlock, indent float64) error {
	var listEl *StructElement
	parentSave := s.parent
	if s.parent != nil {
		listEl = s.parent.AddChild(StructList)
	}
	defer func() { s.parent = parentSave }()

	num := list.list.start
	for _, item := range list.children {
		if s.parent != nil {
			s.parent = listEl.AddChild(StructListItem)
		}
		label := "•"
		depth := int(indent / 18)
		if !list.list.ordered {
			label = [3]string{"•", "◦", "▪"}[depth%3]
			// Standard-14 WinAnsi has no ◦/▪: fall back to bullet.
			label = "•"
		} else {
			label = strconv.Itoa(num) + "."
			num++
		}

		// Task list: a leading [ ] / [x] in the first paragraph becomes a
		// drawn checkbox (detected at parse time, markTaskItems).
		task, checked := item.task, item.taskChecked

		labelStyle := TextStyle{Font: r.th.base, Size: r.th.size}
		lm, err := metricsFor(labelStyle)
		if err != nil {
			return err
		}
		labelW := measureString(label, lm.width)
		contentIndent := indent + labelW + 8
		if contentIndent < indent+16 {
			contentIndent = indent + 16
		}
		if task {
			contentIndent = indent + 16
		}

		// Room for at least the first line.
		if s.y-lm.lineHeight < s.bottom {
			if err := s.advance(); err != nil {
				return err
			}
		}
		yTop := s.y
		drawLabel := func() error {
			if task {
				return r.drawTaskBox(s, indent, yTop, checked)
			}
			rect := Rectangle{LLX: s.colLeft() + indent, LLY: yTop - lm.lineHeight, URX: s.colLeft() + contentIndent, URY: yTop}
			return s.page.AddText(label, labelStyle, rect)
		}
		if s.parent != nil {
			if _, err := s.page.TagContent(s.parent, StructLabel, drawLabel); err != nil {
				return err
			}
		} else if err := drawLabel(); err != nil {
			return err
		}

		if err := r.renderBlocks(s, item.children, contentIndent); err != nil {
			return err
		}
		if item.list.tight {
			s.y += s.f.paraGap * 0.65 // tighter spacing between tight items
		}
	}
	if list.list.tight {
		s.y -= s.f.paraGap * 0.65
	}
	return nil
}

func (r *mdRender) drawTaskBox(s *flowState, indent, yTop float64, checked bool) error {
	size := r.th.size * 0.8
	x := s.colLeft() + indent
	top := yTop - r.th.size*0.15
	box := Rectangle{LLX: x, LLY: top - size, URX: x + size, URY: top}
	border := Color{R: 0.35, G: 0.35, B: 0.35, A: 1}
	if err := s.page.DrawRectangle(box, ShapeStyle{LineStyle: LineStyle{Color: &border, Width: 0.9}}); err != nil {
		return err
	}
	if checked {
		green := Color{R: 0.1, G: 0.5, B: 0.15, A: 1}
		ls := LineStyle{Color: &green, Width: 1.4, Cap: LineCapRound}
		if err := s.page.DrawLine(Point{X: x + size*0.2, Y: top - size*0.55}, Point{X: x + size*0.45, Y: top - size*0.8}, ls); err != nil {
			return err
		}
		return s.page.DrawLine(Point{X: x + size*0.45, Y: top - size*0.8}, Point{X: x + size*0.85, Y: top - size*0.2}, ls)
	}
	return nil
}

// buildTable maps a GFM table onto the Table engine: bold repeating header,
// per-column alignment, cells flattened to plain text.
func (r *mdRender) buildTable(b *mdBlock) *Table {
	t := NewTable()
	t.SetDefaultCellStyle(TextStyle{Font: r.th.base, Size: r.th.size * 0.95})
	t.SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 0.5, Color: &Color{R: 0.6, G: 0.6, B: 0.6, A: 1}})
	t.SetDefaultCellMargin(MarginInfo{Top: 3, Right: 5, Bottom: 3, Left: 5})
	t.SetRepeatingRowsCount(1)

	align := func(i int) HAlign {
		switch b.aligns[i] {
		case mdAlignCenter:
			return HAlignCenter
		case mdAlignRight:
			return HAlignRight
		}
		return HAlignLeft
	}
	cellText := func(raw string) string {
		return mdInlinesPlain(parseInlineContent(raw, r.refmap, false))
	}

	head := t.AddRow()
	for i, c := range b.headerCells {
		cell := head.AddCell(cellText(c))
		cell.SetTextStyle(TextStyle{Font: r.th.bold, Size: r.th.size * 0.95}).SetHAlign(align(i))
		bg := Color{R: 0.92, G: 0.92, B: 0.92, A: 1}
		cell.SetBackground(&bg)
	}
	for _, row := range b.rows {
		tr := t.AddRow()
		for i, c := range row {
			tr.AddCell(cellText(c)).SetHAlign(align(i))
		}
	}
	return t
}

// artifactDraw draws pure decoration, tagged as an artifact when the flow is
// tagged.
func (s *flowState) artifactDraw(fn func() error) error {
	if s.parent != nil {
		return s.page.TagArtifact(fn)
	}
	return fn()
}
