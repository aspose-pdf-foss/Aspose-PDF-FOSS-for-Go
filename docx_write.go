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
		dropFurniture: true,
		dropRotated:   true,
		collectLinks:  true,
		images:        !opt.NoImages,
	})
	if err != nil {
		return err
	}

	blocks := buildDocxBlocks(doc, !opt.NoPageBreaks)
	dw := &docxWriter{bodySize: doc.bodySize, margins: docxContentMargins(doc, pages, sel)}
	if len(sel) > 0 {
		if size, err := pages[sel[0]-1].Size(); err == nil {
			dw.pageWPt = size.Width
		}
	}
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
	parts := []docxPart{
		{"[Content_Types].xml", []byte(docxContentTypes)},
		{"_rels/.rels", []byte(docxRootRels)},
		{"word/document.xml", body},
		{"word/_rels/document.xml.rels", []byte(docxDocumentRels(dw.rels))},
		{"word/styles.xml", []byte(docxStyles(bodyHalf, spacingAfter))},
		{"word/numbering.xml", []byte(docxNumbering(dw.numInstances))},
	}
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
	kind      docxBlockKind
	level     int  // heading level (1..6) or list nesting (0-based ilvl)
	ordered   bool // list kind
	listID    int  // 1-based numbering instance (w:numId)
	runs      []docRun
	lines     [][]docRun // code blocks: one run list per visual line
	imgs      []*Image   // image row: side-by-side images share one paragraph
	pageNo    int        // source page (for media naming)
	brkBefore bool       // start a new Word page before this block
}

// buildDocxBlocks classifies every page's segments into Word blocks,
// tracking list instances (a fresh w:num per list so ordered lists restart)
// and merging split multi-line headings. With pageBreaks, the first block of
// every source page after the first carries a page-break-before mark, so the
// output pagination mirrors the original (an empty source page becomes an
// empty paragraph to keep the page count aligned).
func buildDocxBlocks(doc *flowDoc, pageBreaks bool) *docxBlockList {
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
		for _, blk := range fp.blocks {
			if blk.img != nil {
				// Images that sat side by side on the source page (their
				// vertical ranges overlap) share one paragraph, so a row of
				// thumbnails stays a row instead of a page-bursting stack.
				if last := bl.last(); last != nil && !bl.breakNext &&
					last.kind == docxImageBlock && last.pageNo == fp.number &&
					imagesOverlapVert(last.imgs[len(last.imgs)-1], blk.img) {
					last.imgs = append(last.imgs, blk.img)
					continue
				}
				bl.add(docxBlock{kind: docxImageBlock, imgs: []*Image{blk.img}, pageNo: fp.number})
				endList()
				continue
			}
			for _, seg := range segmentParagraph(blk.para) {
				if doc.furniture.dropSegment(seg, fp.pageH) || len(seg.lines) == 0 {
					continue
				}
				if seg.mono {
					var lines [][]docRun
					for _, line := range seg.lines {
						lineSeg := docSeg{lines: []TextLine{line}, size: seg.size, mono: true}
						lines = append(lines, segmentRuns(lineSeg, fp.links))
					}
					bl.add(docxBlock{kind: docxCodeBlock, lines: lines})
					endList()
					continue
				}
				runs := segmentRuns(seg, fp.links)
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
					bl.add(docxBlock{kind: docxListItemBlock, level: depth,
						ordered: seg.marker == "1", listID: st.listID, runs: runs})
					continue
				}
				text := runsPlainText(runs)
				if level := headingLevel(seg.size, doc.bodySize, len(text)); level > 0 {
					// Merge a heading the extractor split across segments —
					// but never across a pending page break.
					if last := bl.last(); last != nil && !bl.breakNext && last.kind == docxHeadingBlock && last.level == level {
						last.runs = append(append(last.runs, docRun{text: " "}), runs...)
						continue
					}
					bl.add(docxBlock{kind: docxHeadingBlock, level: level, runs: runs})
					endList()
					continue
				}
				bl.add(docxBlock{kind: docxParaBlock, runs: runs})
				endList()
			}
		}
	}
	return bl
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
// first exported page's geometry (points → twips).
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
	fmt.Fprintf(b, `<w:sectPr><w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr>`,
		wTw, hTw, orient, dw.margins[0], dw.margins[1], dw.margins[2], dw.margins[3])
}

// brk returns the page-break-before element when the block starts a new
// page. Its slot in the CT_PPr sequence is after pStyle/keepNext and before
// numPr/shd.
func brk(blk *docxBlock) string {
	if blk.brkBefore {
		return `<w:pageBreakBefore/>`
	}
	return ""
}

func (dw *docxWriter) writeBlock(b *strings.Builder, blk *docxBlock) error {
	switch blk.kind {
	case docxImageBlock:
		return dw.writeImagePara(b, blk)
	case docxCodeBlock:
		dw.writeCodePara(b, blk)
	case docxHeadingBlock:
		fmt.Fprintf(b, `<w:p><w:pPr><w:pStyle w:val="Heading%d"/>%s</w:pPr>`, blk.level, brk(blk))
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	case docxListItemBlock:
		fmt.Fprintf(b, `<w:p><w:pPr>%s<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr></w:pPr>`,
			brk(blk), blk.level, blk.listID)
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	default:
		if blk.brkBefore {
			b.WriteString(`<w:p><w:pPr><w:pageBreakBefore/></w:pPr>`)
		} else {
			b.WriteString(`<w:p>`)
		}
		dw.writeRuns(b, blk.runs, false)
		b.WriteString(`</w:p>`)
	}
	return nil
}

// writeCodePara emits a monospace segment as one shaded paragraph, one
// visual line per w:br-separated stretch.
func (dw *docxWriter) writeCodePara(b *strings.Builder, blk *docxBlock) {
	fmt.Fprintf(b, `<w:p><w:pPr>%s<w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/></w:pPr>`, brk(blk))
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
func docxSameStyle(a, b docRun) bool {
	return a.sameLook(b)
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
		if blk.brkBefore {
			b.WriteString(`<w:p><w:pPr><w:pageBreakBefore/></w:pPr></w:p>`)
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

	if blk.brkBefore {
		b.WriteString(`<w:p><w:pPr><w:pageBreakBefore/></w:pPr>`)
	} else {
		b.WriteString(`<w:p>`)
	}
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
