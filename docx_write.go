// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
)

// PDF → DOCX export (epic pdf-go-7qiu): the document is reconstructed as an
// editable Word file over the shared flow core (flow_doc.go) — paragraphs
// with styled runs, named heading styles (Heading1-6, so Word's navigation
// pane and TOC fields see the structure), hyperlinks, bullet/numbered lists,
// inline images carrying the PDF's own bytes, and the page geometry of the
// source. Mirrors Aspose.PDF for .NET's Document.Save(path, SaveFormat.DocX)
// with DocSaveOptions.
//
// v1 limitations (documented): repeating headers/footers are suppressed (not
// converted to w:hdr/w:ftr), underline/strikethrough are not recovered (they
// are vector rules in PDF, invisible to text extraction), fonts are
// referenced by family name only (no font embedding), and tables flow as
// paragraphs until the table-detection epic lands (DocEnhancedFlow).

// DocRecognitionMode selects the reconstruction algorithm — mirrors
// Aspose.PDF for .NET's DocSaveOptions.RecognitionMode.
type DocRecognitionMode int

const (
	// DocFlow performs full structure recognition (headings, lists, runs)
	// and produces a maximally editable document; the layout may differ
	// from the original.
	DocFlow DocRecognitionMode = iota
)

// DocSaveOptions configures SaveDocx / WriteDocx. The zero value exports all
// pages in Flow mode with images, preserving the source pagination.
type DocSaveOptions struct {
	// Pages is a 1-based subset (in the given order); nil = all pages.
	Pages []int
	// Mode is the reconstruction algorithm (DocFlow is the default and the
	// only mode implemented so far).
	Mode DocRecognitionMode
	// NoImages skips images.
	NoImages bool
	// NoPageBreaks lets content flow continuously instead of starting a new
	// Word page where each source PDF page started (the default inserts a
	// page break, so the output pagination mirrors the original; exact
	// fit still depends on Word's own line layout).
	NoPageBreaks bool
}

// SaveDocx writes the document as a Word (.docx) file.
func (d *Document) SaveDocx(path string, opts ...DocSaveOptions) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save docx: %w", err)
	}
	werr := d.WriteDocx(f, opts...)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return fmt.Errorf("save docx: %w", cerr)
	}
	return nil
}

// WriteDocx writes the document as a Word (.docx) package to w.
func (d *Document) WriteDocx(w io.Writer, opts ...DocSaveOptions) error {
	var opt DocSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Mode != DocFlow {
		return fmt.Errorf("WriteDocx: unsupported recognition mode %d", opt.Mode)
	}
	pages := d.Pages()
	sel := opt.Pages
	if len(sel) == 0 {
		sel = make([]int, len(pages))
		for i := range pages {
			sel[i] = i + 1
		}
	} else {
		for _, n := range sel {
			if n < 1 || n > len(pages) {
				return fmt.Errorf("WriteDocx: page %d out of range 1..%d", n, len(pages))
			}
		}
	}

	doc, err := buildFlowDoc(pages, sel, flowDocOptions{
		dropFurniture:  true,
		dropRotated:    true,
		collectLinks:   true,
		images:         !opt.NoImages,
		vectorGraphics: !opt.NoImages,
	})
	if err != nil {
		return err
	}

	margins := docxContentMargins(doc, pages, sel)
	blocks := buildDocxBlocks(doc, !opt.NoPageBreaks, margins)
	dw := &docxWriter{bodySize: doc.bodySize, margins: margins}
	if len(sel) > 0 {
		if size, err := pages[sel[0]-1].Size(); err == nil {
			dw.pageWPt = size.Width
		}
	}
	dw.footer = doc.furniture.footerExemplar()
	dw.totalPages = len(sel)
	body, err := dw.serialize(blocks, d, pages, sel)
	if err != nil {
		return err
	}

	bodyHalf := int(doc.bodySize*2 + 0.5)
	if bodyHalf < 2 {
		bodyHalf = 22
	}
	// When the source pagination is preserved, tighter paragraph spacing
	// keeps a PDF page's content within one Word page (PDF encodes no
	// inter-paragraph spacing of its own).
	spacingAfter := 120
	if !opt.NoPageBreaks {
		spacingAfter = 60
	}
	contentTypes := docxContentTypes
	parts := []docxPart{
		{"[Content_Types].xml", nil}, // filled below (footer needs an Override)
		{"_rels/.rels", []byte(docxRootRels)},
		{"word/document.xml", body},
		{"word/_rels/document.xml.rels", []byte(docxDocumentRels(dw.rels))},
		{"word/styles.xml", []byte(docxStyles(bodyHalf, spacingAfter))},
		{"word/numbering.xml", []byte(docxNumbering(dw.numInstances))},
	}
	if dw.footerXML != "" {
		parts = append(parts, docxPart{"word/footer1.xml", []byte(dw.footerXML)})
		contentTypes = strings.Replace(contentTypes,
			"</Types>",
			`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/></Types>`, 1)
	}
	parts[0].data = []byte(contentTypes)
	parts = append(parts, dw.media...)
	return writeDocxZip(w, parts)
}

// --- intermediate block model -----------------------------------------------------

type docxBlockKind int

const (
	docxParaBlock docxBlockKind = iota
	docxHeadingBlock
	docxListItemBlock
	docxCodeBlock
	docxImageBlock
)

// docxBlock is one Word block element to emit, produced by classifying the
// flow segments (so multi-line headings merge before serialization instead
// of by rewinding emitted text).
type docxBlock struct {
	kind        docxBlockKind
	level       int  // heading level (1..6) or list nesting (0-based ilvl)
	ordered     bool // list kind
	listID      int  // 1-based numbering instance (w:numId)
	runs        []docRun
	lines       [][]docRun // code blocks: one run list per visual line
	imgs        []*Image   // image row: side-by-side images share one paragraph
	pageNo      int        // source page (for media naming)
	brkBefore   bool       // start a new Word page before this block
	align       int8       // 0 left, 1 center, 2 right (w:jc)
	spaceBefore int        // extra vertical gap above, twips (w:spacing w:before)
	lastFilled  bool       // last visual line filled its column (wrap continues)
	yTop, yBot  float64    // source vertical extent (spacing reconstruction)
	xMin, xMax  float64    // image rows: horizontal extent (centering)
}

// Alignment inference: a line is centered when its side gaps within the
// column are near-equal (and real); right-aligned when it hugs the right
// edge while leaving a substantial left gap.
func lineAlign(llx, urx float64, col Rectangle) int8 {
	colW := col.URX - col.LLX
	if colW <= 0 {
		return 0
	}
	left, right := llx-col.LLX, col.URX-urx
	tol := maxf(8, 0.08*colW)
	// The side gaps must be clearly larger than an ordinary text margin —
	// relative to the reference width, so both tight section rects and
	// full-page frames work.
	minSide := maxf(12, 0.15*colW)
	if left > minSide && right > minSide && absf(left-right) <= tol {
		return 1
	}
	if right <= maxf(6, 0.02*colW) && left > 0.25*colW {
		return 2
	}
	return 0
}

// lineFills reports whether a visual line reaches (close to) the column's
// right edge — i.e. it wrapped naturally rather than ending deliberately.
func lineFills(urx float64, col Rectangle) bool {
	colW := col.URX - col.LLX
	return colW > 0 && urx >= col.URX-0.15*colW
}

// segGeometry summarizes a segment's alignment/fill/vertical extent.
func segGeometry(seg docSeg, col Rectangle) (align int8, lastFilled bool, yTop, yBot float64) {
	centered, right := 0, 0
	for li, line := range seg.lines {
		llx, urx := lineExtent(line)
		switch lineAlign(llx, urx, col) {
		case 1:
			centered++
		case 2:
			right++
		}
		size := 12.0
		if len(line.Fragments) > 0 && line.Fragments[0].FontSize > 0 {
			size = line.Fragments[0].FontSize
		}
		top, bot := line.Y+0.8*size, line.Y-0.25*size
		if li == 0 || top > yTop {
			yTop = top
		}
		if li == 0 || bot < yBot {
			yBot = bot
		}
		if li == len(seg.lines)-1 {
			lastFilled = lineFills(urx, col)
		}
	}
	n := len(seg.lines)
	switch {
	case n > 0 && centered*2 > n:
		align = 1
	case n > 0 && right*2 > n:
		align = 2
	}
	return
}

// segRunsWithBreaks joins a segment's per-line runs, preserving deliberate
// line breaks. In centered/right-aligned segments (title blocks, covers) a
// short line is unambiguously intentional. In left-aligned text the rule is
// stricter — geometry alone cannot tell a deliberate line list from ragged
// wrapped prose (hard breaks + the substitute fonts' wider metrics would
// double every prose line), so a break additionally requires the previous
// line to end like a sentence and the next to start like one (font-sample
// sheets, definition lists); mid-sentence wraps keep their space-joins.
func segRunsWithBreaks(seg docSeg, links []linkArea, col Rectangle, align int8) []docRun {
	lineRuns := segmentLineRuns(seg, links)
	var runs []docRun
	for li, lr := range lineRuns {
		if len(lr) == 0 {
			continue
		}
		if len(runs) > 0 {
			_, prevURX := lineExtent(seg.lines[li-1])
			brk := false
			if !lineFills(prevURX, col) {
				if align != 0 {
					brk = true
				} else {
					brk = endsSentence(lineJoinedText(seg.lines[li-1])) &&
						startsSentence(lineJoinedText(seg.lines[li]))
				}
			}
			if brk {
				runs = append(runs, docRun{br: true})
			} else {
				runs[len(runs)-1].text += " "
			}
		}
		runs = append(runs, lr...)
	}
	return runs
}

// endsSentence reports whether a visual line ends with terminal punctuation.
func endsSentence(s string) bool {
	s = strings.TrimRight(s, "  ")
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?', ':', ';':
		return true
	}
	return false
}

// startsSentence reports whether a visual line opens like a new statement
// (uppercase letter, digit, or a bullet-ish glyph).
func startsSentence(s string) bool {
	for _, r := range s {
		if r == ' ' || r == ' ' {
			continue
		}
		return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r >= 0x80
	}
	return false
}

// buildDocxBlocks classifies every page's segments into Word blocks,
// tracking list instances (a fresh w:num per list so ordered lists restart)
// and merging split multi-line headings. With pageBreaks, the first block of
// every source page after the first carries a page-break-before mark, so the
// output pagination mirrors the original (an empty source page becomes an
// empty paragraph to keep the page count aligned).
func buildDocxBlocks(doc *flowDoc, pageBreaks bool, margins [4]int) *docxBlockList {
	bl := &docxBlockList{}
	st := struct {
		listKind    string
		listIndentX float64
		listID      int
	}{}
	endList := func() { st.listKind = "" }
	for pageIdx, fp := range doc.pages {
		if pageBreaks && pageIdx > 0 {
			bl.breakNext = true
			if len(fp.blocks) == 0 {
				// Blank source page: an empty paragraph keeps the slot.
				bl.add(docxBlock{kind: docxParaBlock})
			}
		}
		// Vertical-placement reconstruction, first block of the page only:
		// the gap between the content-area top and the page's first content
		// (a cover's mid-page title, a chapter opener) becomes w:spacing
		// w:before. Inter-block gaps are NOT reconstructed — Word's line
		// metrics run taller than the PDF's, and on dense pages any added
		// spacing spills onto an extra page. Scaled by 0.8 for the same
		// reason.
		contentTop := fp.pageH - float64(margins[0])/20
		firstOnPage := pageBreaks
		spacingFor := func(yTop float64) int {
			if !firstOnPage || yTop <= 0 {
				return 0
			}
			gapPt := (contentTop - yTop) * 0.8
			if gapPt < 24 {
				return 0
			}
			if gapPt > 300 {
				gapPt = 300
			}
			return int(gapPt * 20)
		}
		addBlock := func(b docxBlock) {
			b.spaceBefore = spacingFor(b.yTop)
			bl.add(b)
			firstOnPage = false
		}
		for _, blk := range fp.blocks {
			if blk.img != nil {
				// Images that sat side by side on the source page (their
				// vertical ranges overlap) share one paragraph, so a row of
				// thumbnails stays a row instead of a page-bursting stack.
				if last := bl.last(); last != nil && !bl.breakNext &&
					last.kind == docxImageBlock && last.pageNo == fp.number &&
					imagesOverlapVert(last.imgs[len(last.imgs)-1], blk.img) {
					last.imgs = append(last.imgs, blk.img)
					last.yTop = maxf(last.yTop, blk.img.Y+blk.img.PageHeight)
					if blk.img.Y < last.yBot {
						last.yBot = blk.img.Y
					}
					last.xMin = minf(last.xMin, blk.img.X)
					last.xMax = maxf(last.xMax, blk.img.X+blk.img.PageWidth)
					last.align = imageRowAlign(last.xMin, last.xMax, fp.pageW)
					continue
				}
				b := docxBlock{kind: docxImageBlock, imgs: []*Image{blk.img}, pageNo: fp.number,
					yTop: blk.img.Y + blk.img.PageHeight, yBot: blk.img.Y,
					xMin: blk.img.X, xMax: blk.img.X + blk.img.PageWidth}
				b.align = imageRowAlign(b.xMin, b.xMax, fp.pageW)
				addBlock(b)
				endList()
				continue
			}
			for _, seg := range segmentParagraph(blk.para) {
				if doc.furniture.dropSegment(seg, fp.pageH) || len(seg.lines) == 0 {
					continue
				}
				align, lastFilled, yTop, yBot := segGeometry(seg, blk.col)
				if seg.mono {
					var lines [][]docRun
					for _, line := range seg.lines {
						lineSeg := docSeg{lines: []TextLine{line}, size: seg.size, mono: true}
						lines = append(lines, segmentRuns(lineSeg, fp.links))
					}
					addBlock(docxBlock{kind: docxCodeBlock, lines: lines, yTop: yTop, yBot: yBot})
					endList()
					continue
				}
				runs := segRunsWithBreaks(seg, fp.links, blk.col, align)
				if seg.marker != "" {
					depth := 0
					if st.listKind != "" {
						depth = listNestDepth(seg.indentX, st.listIndentX)
						if depth > 8 {
							depth = 8
						}
					}
					if st.listKind == "" || (st.listKind != seg.marker && depth == 0) {
						st.listKind = seg.marker
						st.listIndentX = seg.indentX
						bl.numInstances = append(bl.numInstances, seg.marker == "1")
						st.listID = len(bl.numInstances)
					}
					re := bulletRunRe
					if seg.marker == "1" {
						re = orderedRunRe
					}
					stripRunsPrefix(&runs, re)
					addBlock(docxBlock{kind: docxListItemBlock, level: depth,
						ordered: seg.marker == "1", listID: st.listID, runs: runs,
						yTop: yTop, yBot: yBot})
					continue
				}
				text := runsPlainText(runs)
				if level := headingLevel(seg.size, doc.bodySize, len(text)); level > 0 {
					// Merge a heading the extractor split across segments —
					// only when the previous heading's last line wrapped
					// (filled its column), and never across a page break.
					if last := bl.last(); last != nil && !bl.breakNext &&
						last.kind == docxHeadingBlock && last.level == level && last.lastFilled {
						last.runs = append(append(last.runs, docRun{text: " "}), runs...)
						last.lastFilled = lastFilled
						if yBot > 0 && yBot < last.yBot {
							last.yBot = yBot
						}
						continue
					}
					addBlock(docxBlock{kind: docxHeadingBlock, level: level, runs: runs,
						align: align, lastFilled: lastFilled, yTop: yTop, yBot: yBot})
					endList()
					continue
				}
				addBlock(docxBlock{kind: docxParaBlock, runs: runs, align: align,
					lastFilled: lastFilled, yTop: yTop, yBot: yBot})
				endList()
			}
		}
	}
	return bl
}

// imageRowAlign centers an image row that sits around the page's middle.
func imageRowAlign(xMin, xMax, pageW float64) int8 {
	if pageW <= 0 {
		return 0
	}
	center := (xMin + xMax) / 2
	if absf(center-pageW/2) <= 0.08*pageW && xMin > 0.05*pageW {
		return 1
	}
	return 0
}

// imagesOverlapVert reports whether two images' vertical ranges intersect —
// the side-by-side test for grouping them into one image row.
func imagesOverlapVert(a, b *Image) bool {
	return b.Y < a.Y+a.PageHeight && b.Y+b.PageHeight > a.Y
}

type docxBlockList struct {
	blocks       []docxBlock
	numInstances []bool // per list instance: ordered?
	breakNext    bool   // pending page break for the next added block
}

func (bl *docxBlockList) add(b docxBlock) {
	if bl.breakNext {
		b.brkBefore = true
		bl.breakNext = false
	}
	bl.blocks = append(bl.blocks, b)
}
func (bl *docxBlockList) last() *docxBlock {
	if len(bl.blocks) == 0 {
		return nil
	}
	return &bl.blocks[len(bl.blocks)-1]
}

// --- document.xml serialization ---------------------------------------------------

type docxWriter struct {
	bodySize     float64
	margins      [4]int  // twips: top, right, bottom, left
	pageWPt      float64 // first exported page width in points
	footer       *furnitureExemplar
	footerXML    string
	totalPages   int
	rels         []docxRel
	numInstances []bool
	media        []docxPart
	relByURI     map[string]string
	relByImage   map[[32]byte]string
	imageSeq     int
	drawingID    int
}

// docxContentMargins derives the section margins from the content bounding
// box across the exported pages (clamped to [0.5", 1"]), so a source page's
// content has a chance to fit one Word page when pagination is preserved.
// Falls back to 1" margins when there is nothing to measure.
func docxContentMargins(doc *flowDoc, pages []*Page, sel []int) [4]int {
	const defTw = 1440
	m := [4]int{defTw, defTw, defTw, defTw}
	if len(sel) == 0 {
		return m
	}
	size, err := pages[sel[0]-1].Size()
	if err != nil || size.Width <= 0 || size.Height <= 0 {
		return m
	}
	minX, minY := size.Width, size.Height
	maxX, maxY := 0.0, 0.0
	seen := false
	for _, fp := range doc.pages {
		for _, blk := range fp.blocks {
			var r Rectangle
			switch {
			case blk.para != nil:
				r = blk.para.Rectangle
			case blk.img != nil:
				r = Rectangle{LLX: blk.img.X, LLY: blk.img.Y,
					URX: blk.img.X + blk.img.PageWidth, URY: blk.img.Y + blk.img.PageHeight}
			default:
				continue
			}
			if r.URX <= r.LLX || r.URY <= r.LLY {
				continue
			}
			seen = true
			minX, minY = minf(minX, r.LLX), minf(minY, r.LLY)
			maxX, maxY = maxf(maxX, r.URX), maxf(maxY, r.URY)
		}
	}
	if !seen {
		return m
	}
	clamp := func(pt float64) int {
		tw := int(pt*20 + 0.5)
		if tw < 720 {
			return 720
		}
		if tw > 1440 {
			return 1440
		}
		return tw
	}
	m[0] = clamp(size.Height - maxY) // top
	m[1] = clamp(size.Width - maxX)  // right
	m[2] = clamp(minY)               // bottom
	m[3] = clamp(minX)               // left
	return m
}

const (
	relTypeHyperlink = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relTypeImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	relTypeStyles    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relTypeNumbering = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering"
)

func (dw *docxWriter) serialize(bl *docxBlockList, d *Document, pages []*Page, sel []int) ([]byte, error) {
	dw.numInstances = bl.numInstances
	dw.relByURI = map[string]string{}
	dw.relByImage = map[[32]byte]string{}
	// styles/numbering are implicit relationships: referenced by type, but
	// they still must be declared in document.xml.rels.
	dw.rels = []docxRel{
		{id: "rId1", relType: relTypeStyles, target: "styles.xml"},
		{id: "rId2", relType: relTypeNumbering, target: "numbering.xml"},
	}

	var b strings.Builder
	b.WriteString(docxXMLHeader)
	fmt.Fprintf(&b, `<w:document xmlns:w="%s" xmlns:r="%s"><w:body>`, docxNSw, docxNSr)
	for i := range bl.blocks {
		if err := dw.writeBlock(&b, &bl.blocks[i]); err != nil {
			return nil, err
		}
	}
	dw.writeSectPr(&b, pages, sel)
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String()), nil
}

// writeSectPr emits the section properties — last child of w:body — with the
// first exported page's geometry (points → twips). A recognized running
// footer becomes a real w:ftr part referenced from the section.
func (dw *docxWriter) writeSectPr(b *strings.Builder, pages []*Page, sel []int) {
	wTw, hTw := 11906, 16838 // A4 default
	if len(sel) > 0 {
		if size, err := pages[sel[0]-1].Size(); err == nil && size.Width > 0 && size.Height > 0 {
			wTw, hTw = int(size.Width*20+0.5), int(size.Height*20+0.5)
		}
	}
	orient := ""
	if wTw > hTw {
		orient = ` w:orient="landscape"`
	}
	footerRef := ""
	if dw.footer != nil {
		relID := fmt.Sprintf("rId%d", len(dw.rels)+1)
		dw.rels = append(dw.rels, docxRel{id: relID,
			relType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer",
			target:  "footer1.xml"})
		dw.footerXML = dw.buildFooterXML(dw.footer)
		footerRef = fmt.Sprintf(`<w:footerReference w:type="default" r:id="%s"/>`, relID)
	}
	fmt.Fprintf(b, `<w:sectPr>%s<w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr>`,
		footerRef, wTw, hTw, orient, dw.margins[0], dw.margins[1], dw.margins[2], dw.margins[3])
}

// buildFooterXML rebuilds the recognized running footer as a w:ftr part.
// Digit runs in the exemplar text that equal its source page number become
// the PAGE field and runs equal to the page count become NUMPAGES, so
// "Report · 3 / 15" turns into "Report · {PAGE} / {NUMPAGES}".
func (dw *docxWriter) buildFooterXML(ex *furnitureExemplar) string {
	family := docxFontFamily(ex.fontName, false)
	half := int(ex.size*2 + 0.5)
	if half < 2 {
		half = 18
	}
	var rPr strings.Builder
	fmt.Fprintf(&rPr, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>`, family, family, family)
	if ex.bold {
		rPr.WriteString(`<w:b/>`)
	}
	if ex.italic {
		rPr.WriteString(`<w:i/>`)
	}
	if c := docxColor(ex.color); c != "000000" {
		fmt.Fprintf(&rPr, `<w:color w:val="%s"/>`, c)
	}
	fmt.Fprintf(&rPr, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, half, half)

	run := func(text string) string {
		return `<w:r><w:rPr>` + rPr.String() + `</w:rPr><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r>`
	}
	field := func(instr, current string) string {
		return `<w:fldSimple w:instr="` + instr + `"><w:r><w:rPr>` + rPr.String() +
			`</w:rPr><w:t>` + xmlEscape(current) + `</w:t></w:r></w:fldSimple>`
	}

	var body strings.Builder
	text := ex.text
	for len(text) > 0 {
		i := 0
		for i < len(text) && !(text[i] >= '0' && text[i] <= '9') {
			i++
		}
		if i > 0 {
			body.WriteString(run(text[:i]))
			text = text[i:]
			continue
		}
		j := 0
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		num := text[:j]
		switch num {
		case fmt.Sprint(ex.pageNo):
			body.WriteString(field(" PAGE ", num))
		case fmt.Sprint(dw.totalPages):
			body.WriteString(field(" NUMPAGES ", num))
		default:
			body.WriteString(run(num))
		}
		text = text[j:]
	}

	jc := ""
	if ex.centered {
		jc = `<w:jc w:val="center"/>`
	}
	return docxXMLHeader +
		`<w:ftr xmlns:w="` + docxNSw + `" xmlns:r="` + docxNSr + `">` +
		`<w:p><w:pPr><w:spacing w:after="0"/>` + jc + `</w:pPr>` + body.String() + `</w:p>` +
		`</w:ftr>`
}

// docxPPr assembles a paragraph-properties element in the CT_PPr schema
// sequence: pStyle, pageBreakBefore, numPr, shd, spacing, jc. Returns ""
// when every part is empty.
func docxPPr(blk *docxBlock, pStyle, numPr, shd string) string {
	var b strings.Builder
	b.WriteString(pStyle)
	if blk.brkBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	b.WriteString(numPr)
	b.WriteString(shd)
	if blk.spaceBefore > 0 {
		fmt.Fprintf(&b, `<w:spacing w:before="%d"/>`, blk.spaceBefore)
	}
	switch blk.align {
	case 1:
		b.WriteString(`<w:jc w:val="center"/>`)
	case 2:
		b.WriteString(`<w:jc w:val="right"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<w:pPr>` + b.String() + `</w:pPr>`
}

func (dw *docxWriter) writeBlock(b *strings.Builder, blk *docxBlock) error {
	switch blk.kind {
	case docxImageBlock:
		return dw.writeImagePara(b, blk)
	case docxCodeBlock:
		dw.writeCodePara(b, blk)
	case docxHeadingBlock:
		b.WriteString(`<w:p>`)
		b.WriteString(docxPPr(blk, fmt.Sprintf(`<w:pStyle w:val="Heading%d"/>`, blk.level), "", ""))
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	case docxListItemBlock:
		b.WriteString(`<w:p>`)
		b.WriteString(docxPPr(blk,
			"", fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, blk.level, blk.listID), ""))
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	default:
		b.WriteString(`<w:p>`)
		b.WriteString(docxPPr(blk, "", "", ""))
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	}
	return nil
}

// writeCodePara emits a monospace segment as one shaded paragraph, one
// visual line per w:br-separated stretch.
func (dw *docxWriter) writeCodePara(b *strings.Builder, blk *docxBlock) {
	b.WriteString(`<w:p>`)
	b.WriteString(docxPPr(blk, "", "", `<w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/>`))
	for i, line := range blk.lines {
		if i > 0 {
			b.WriteString(`<w:r><w:br/></w:r>`)
		}
		dw.writeRuns(b, line, true)
	}
	b.WriteString(`</w:p>`)
}

// docxSameStyle is the Word view of run equivalence (everything docRun
// carries is expressible, so only the hyperlink grouping stays outside).
// Break markers never merge — two breaks are a blank line.
func docxSameStyle(a, b docRun) bool {
	return !a.br && !b.br && a.sameLook(b)
}

// writeRuns emits the runs, wrapping stretches that share a hyperlink in
// w:hyperlink elements.
func (dw *docxWriter) writeRuns(b *strings.Builder, runs []docRun, forceMono bool) {
	runs = mergeRuns(runs, docxSameStyle)
	i := 0
	for i < len(runs) {
		if uri := runs[i].link; uri != "" {
			j := i
			for j < len(runs) && runs[j].link == uri {
				j++
			}
			fmt.Fprintf(b, `<w:hyperlink r:id="%s">`, dw.hyperlinkRel(uri))
			for _, r := range runs[i:j] {
				dw.writeRun(b, r, forceMono, true)
			}
			b.WriteString(`</w:hyperlink>`)
			i = j
			continue
		}
		dw.writeRun(b, runs[i], forceMono, false)
		i++
	}
}

// writeRun emits one w:r. rPr children follow the schema sequence: rStyle,
// rFonts, b, i, strike, color, sz, szCs, u, vertAlign.
func (dw *docxWriter) writeRun(b *strings.Builder, r docRun, forceMono, inLink bool) {
	if r.br {
		b.WriteString(`<w:r><w:br/></w:r>`)
		return
	}
	if r.text == "" {
		return
	}
	b.WriteString(`<w:r><w:rPr>`)
	if inLink {
		b.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	}
	family := docxFontFamily(r.fontName, r.code || forceMono)
	fmt.Fprintf(b, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>`, family, family, family)
	if r.bold {
		b.WriteString(`<w:b/>`)
	}
	if r.italic {
		b.WriteString(`<w:i/>`)
	}
	if c := docxColor(r.color); c != "000000" {
		fmt.Fprintf(b, `<w:color w:val="%s"/>`, c)
	}
	if r.fontSize > 0 {
		half := int(r.fontSize*2 + 0.5)
		fmt.Fprintf(b, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, half, half)
	}
	if r.super {
		b.WriteString(`<w:vertAlign w:val="superscript"/>`)
	} else if r.sub {
		b.WriteString(`<w:vertAlign w:val="subscript"/>`)
	}
	b.WriteString(`</w:rPr>`)
	// xml:space="preserve" keeps leading/trailing spaces (Word trims bare
	// w:t whitespace).
	fmt.Fprintf(b, `<w:t xml:space="preserve">%s</w:t></w:r>`, xmlEscape(r.text))
}

// docxFontFamily maps an extracted PDF font name to a metric-compatible
// web-safe family — the same substitution the HTML text mode uses.
func docxFontFamily(name string, mono bool) string {
	if mono {
		return "Courier New"
	}
	switch fontFamilyClass(name) {
	case "serif":
		return "Times New Roman"
	case "mono":
		return "Courier New"
	}
	return "Arial"
}

// docxColor formats a fill colour as RRGGBB (no #).
func docxColor(c Color) string {
	clamp := func(v float64) int {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 255
		}
		return int(v*255 + 0.5)
	}
	return fmt.Sprintf("%02X%02X%02X", clamp(c.R), clamp(c.G), clamp(c.B))
}

func (dw *docxWriter) hyperlinkRel(uri string) string {
	if id, ok := dw.relByURI[uri]; ok {
		return id
	}
	id := fmt.Sprintf("rId%d", len(dw.rels)+1)
	dw.rels = append(dw.rels, docxRel{id: id, relType: relTypeHyperlink, target: uri, external: true})
	dw.relByURI[uri] = id
	return id
}

// writeImagePara emits one paragraph holding the block's image row: media
// parts (SHA-256 deduped), their relationships, and one minimal wp:inline
// drawing tree per image, separated by spaces. Display sizes come from each
// image's placement on the PDF page (points → EMU); a row wider than the
// printable width is scaled down proportionally as a whole.
func (dw *docxWriter) writeImagePara(b *strings.Builder, blk *docxBlock) error {
	// Printable width cap (points).
	maxWPt := dw.pageWPt - float64(dw.margins[1]+dw.margins[3])/20
	if maxWPt < 72 {
		maxWPt = 522 // degenerate geometry: fall back to 7.25"
	}

	type placed struct {
		relID  string
		cx, cy int64
	}
	var row []placed
	totalW := 0.0
	for _, img := range blk.imgs {
		wPt, hPt := img.PageWidth, img.PageHeight
		if wPt <= 0 || hPt <= 0 {
			// Fall back to pixel dimensions at 96 dpi.
			wPt, hPt = float64(img.Width)*72/96, float64(img.Height)*72/96
		}
		if wPt <= 0 || hPt <= 0 {
			continue // zero extents trigger Word repair; skip
		}
		key := sha256.Sum256(img.Data)
		relID, ok := dw.relByImage[key]
		if !ok {
			dw.imageSeq++
			ext := "png"
			if img.Format == ImageFormatJPEG {
				ext = "jpg"
			}
			name := fmt.Sprintf("media/image%d.%s", dw.imageSeq, ext)
			dw.media = append(dw.media, docxPart{name: "word/" + name, data: img.Data})
			relID = fmt.Sprintf("rId%d", len(dw.rels)+1)
			dw.rels = append(dw.rels, docxRel{id: relID, relType: relTypeImage, target: name})
			dw.relByImage[key] = relID
		}
		row = append(row, placed{relID: relID, cx: int64(wPt * 12700), cy: int64(hPt * 12700)})
		totalW += wPt
	}
	if len(row) == 0 {
		if pPr := docxPPr(blk, "", "", ""); pPr != "" {
			b.WriteString(`<w:p>` + pPr + `</w:p>`)
		}
		return nil
	}
	// Inter-image gaps (~6pt each) count against the printable width too.
	gapPt := 6.0 * float64(len(row)-1)
	if totalW+gapPt > maxWPt {
		scale := (maxWPt - gapPt) / totalW
		if scale <= 0 {
			scale = maxWPt / totalW
		}
		for i := range row {
			row[i].cx = int64(float64(row[i].cx) * scale)
			row[i].cy = int64(float64(row[i].cy) * scale)
		}
	}

	b.WriteString(`<w:p>`)
	b.WriteString(docxPPr(blk, "", "", ""))
	for i, pl := range row {
		if pl.cx <= 0 || pl.cy <= 0 {
			continue
		}
		if i > 0 {
			b.WriteString(`<w:r><w:t xml:space="preserve"> </w:t></w:r>`)
		}
		dw.drawingID++
		id := dw.drawingID
		b.WriteString(`<w:r><w:drawing>`)
		fmt.Fprintf(b, `<wp:inline distT="0" distB="0" distL="0" distR="0" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">`)
		fmt.Fprintf(b, `<wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Picture %d"/>`, pl.cx, pl.cy, id, id)
		b.WriteString(`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`)
		b.WriteString(`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr>`)
		fmt.Fprintf(b, `<pic:cNvPr id="%d" name="Picture %d"/><pic:cNvPicPr/></pic:nvPicPr>`, id, id)
		fmt.Fprintf(b, `<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`, pl.relID)
		fmt.Fprintf(b, `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`, pl.cx, pl.cy)
		b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`)
	}
	b.WriteString(`</w:p>`)
	return nil
}
