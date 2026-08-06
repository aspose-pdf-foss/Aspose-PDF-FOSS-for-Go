// SPDX-License-Identifier: MIT

package asposepdf

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
)

// PDF → EPUB export (epic pdf-go-7qiu): the document is reconstructed as a
// reflowable EPUB 3 book over the shared flow core (flow_doc.go) and the
// same block model the DOCX writer uses — headings, styled runs with
// hyperlinks, lists, code paragraphs, images, vector-graphics patches and
// inline icons all carry over. Chapters split at level-1 headings (or every
// few source pages when the document has none), and the level-1/2 headings
// form the navigation document. Mirrors Aspose.PDF for .NET's
// Document.Save(path, SaveFormat.Epub) with EpubSaveOptions (the reflowable
// ContentRecognitionMode.Flow; PdfFlow/Fixed are out of scope).

// EpubSaveOptions configures SaveEpub / WriteEpub. The zero value exports
// all pages with images, titled from the document's /Info.
type EpubSaveOptions struct {
	// Pages is a 1-based subset (in the given order); nil = all pages.
	Pages []int
	// Title sets dc:title; empty falls back to the Info title, then "Document".
	Title string
	// NoImages skips images (raster, vector patches and inline icons alike).
	NoImages bool
}

// SaveEpub writes the document as an EPUB file.
func (d *Document) SaveEpub(path string, opts ...EpubSaveOptions) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save epub: %w", err)
	}
	werr := d.WriteEpub(f, opts...)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return fmt.Errorf("save epub: %w", cerr)
	}
	return nil
}

// WriteEpub writes the document as an EPUB 3 package to w.
func (d *Document) WriteEpub(w io.Writer, opts ...EpubSaveOptions) error {
	var opt EpubSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
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
				return fmt.Errorf("WriteEpub: page %d out of range 1..%d", n, len(pages))
			}
		}
	}

	doc, err := buildFlowDoc(pages, sel, flowDocOptions{
		dropFurniture:  true,
		dropRotated:    true,
		collectLinks:   true,
		images:         !opt.NoImages,
		vectorGraphics: !opt.NoImages,
		detectTables:   true,
	})
	if err != nil {
		return err
	}
	// pageBreaks=true only to mark each source page's first block (chapter
	// fallback splitting); the spacing side is ignored by this serializer.
	blocks := buildDocxBlocks(doc, true, [4]int{1440, 1440, 1440, 1440})

	title := opt.Title
	if title == "" {
		if info, err := d.Info(); err == nil && strings.TrimSpace(info.Title) != "" {
			title = strings.TrimSpace(info.Title)
		}
	}
	if title == "" {
		title = "Document"
	}
	lang := "en"
	if lv, ok := d.catalog["/Lang"]; ok {
		if s, ok := resolveRef(d.objects, lv).(string); ok && strings.TrimSpace(s) != "" {
			lang = strings.TrimSpace(s)
		}
	}
	modified := "2000-01-01T00:00:00Z"
	if info, err := d.Info(); err == nil {
		if t, ok := parsePDFDate(info.ModDate); ok {
			modified = t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}

	ew := &epubWriter{title: title, lang: lang, modified: modified}
	ew.build(blocks.blocks)
	return ew.writeZip(w)
}

// --- chapter building -------------------------------------------------------------

type epubChapter struct {
	file  string
	body  strings.Builder
	empty bool
}

type epubTOCEntry struct {
	title string
	file  string
	id    string
	level int
}

type epubWriter struct {
	title    string
	lang     string
	modified string

	chapters []*epubChapter
	toc      []epubTOCEntry
	images   []docxPart // OEBPS-relative image files
	imgByKey map[[32]byte]string
	imgSeq   int
	headSeq  int

	listOpen  int  // 0 closed, else the open list's listID
	listOrder bool // open list ordered?
}

// build serializes the block stream into chapters.
func (ew *epubWriter) build(blocks []docxBlock) {
	hasH1 := false
	for i := range blocks {
		if blocks[i].kind == docxHeadingBlock && blocks[i].level == 1 {
			hasH1 = true
			break
		}
	}
	pageStarts := 0
	const pagesPerChapter = 8
	for i := range blocks {
		blk := &blocks[i]
		startChapter := len(ew.chapters) == 0
		if blk.kind == docxHeadingBlock && blk.level == 1 {
			startChapter = true
		}
		if !hasH1 && blk.brkBefore {
			pageStarts++
			if pageStarts%pagesPerChapter == 0 {
				startChapter = true
			}
		}
		if startChapter {
			ew.closeList()
			ew.chapters = append(ew.chapters, &epubChapter{
				file:  fmt.Sprintf("text/chap%03d.xhtml", len(ew.chapters)+1),
				empty: true,
			})
		}
		ew.writeBlock(blk)
	}
	ew.closeList()
	if len(ew.chapters) == 0 {
		ew.chapters = append(ew.chapters, &epubChapter{file: "text/chap001.xhtml", empty: true})
	}
}

func (ew *epubWriter) cur() *epubChapter {
	return ew.chapters[len(ew.chapters)-1]
}

func (ew *epubWriter) closeList() {
	if ew.listOpen == 0 || len(ew.chapters) == 0 {
		return
	}
	if ew.listOrder {
		ew.cur().body.WriteString("</ol>\n")
	} else {
		ew.cur().body.WriteString("</ul>\n")
	}
	ew.listOpen = 0
}

// epubSameStyle is this serializer's run equivalence: everything XHTML can
// express inline.
func epubSameStyle(a, b docRun) bool {
	return !a.br && !b.br && a.icon == nil && b.icon == nil &&
		a.bold == b.bold && a.italic == b.italic && a.code == b.code &&
		a.link == b.link && a.color == b.color && a.sub == b.sub && a.super == b.super
}

func (ew *epubWriter) writeBlock(blk *docxBlock) {
	ch := ew.cur()
	if blk.kind != docxListItemBlock {
		ew.closeList()
	}
	switch blk.kind {
	case docxTableBlock:
		ew.writeTable(&ch.body, blk.table)
		ch.empty = false
	case docxHeadingBlock:
		ew.headSeq++
		id := fmt.Sprintf("h%d", ew.headSeq)
		level := blk.level
		if level > 3 {
			level = 3
		}
		text := collapseWS(runsPlainText(blk.runs))
		if text == "" {
			return
		}
		fmt.Fprintf(&ch.body, `<h%d id="%s"%s>`, level, id, epubAlignAttr(blk.align))
		ew.writeRuns(&ch.body, blk.runs)
		fmt.Fprintf(&ch.body, "</h%d>\n", level)
		if blk.level <= 2 {
			ew.toc = append(ew.toc, epubTOCEntry{title: text, file: ch.file, id: id, level: blk.level})
		}
		ch.empty = false
	case docxListItemBlock:
		if ew.listOpen != blk.listID {
			ew.closeList()
			if blk.ordered {
				ch.body.WriteString("<ol>\n")
			} else {
				ch.body.WriteString("<ul>\n")
			}
			ew.listOpen = blk.listID
			ew.listOrder = blk.ordered
		}
		ch.body.WriteString("<li>")
		ew.writeRuns(&ch.body, blk.runs)
		ch.body.WriteString("</li>\n")
		ch.empty = false
	case docxCodeBlock:
		ch.body.WriteString("<pre>")
		for i, line := range blk.lines {
			if i > 0 {
				ch.body.WriteString("\n")
			}
			for _, r := range line {
				ch.body.WriteString(xmlEscape(r.text))
			}
		}
		ch.body.WriteString("</pre>\n")
		ch.empty = false
	case docxImageBlock:
		var imgs []string
		for _, img := range blk.imgs {
			if img.PageWidth < 4 || img.PageHeight < 4 {
				continue // degenerate hairline rasters (border artifacts)
			}
			if src := ew.imageFile(img); src != "" {
				style := ""
				if img.PageWidth > 0 {
					style = fmt.Sprintf(` style="width:%.0fpt"`, minf(img.PageWidth, 480))
				}
				imgs = append(imgs, fmt.Sprintf(`<img src="../%s" alt=""%s/>`, src, style))
			}
		}
		if len(imgs) > 0 {
			fmt.Fprintf(&ch.body, `<p class="img"%s>%s</p>`+"\n", epubAlignAttr(blk.align), strings.Join(imgs, " "))
			ch.empty = false
		}
	default:
		if runsPlainText(blk.runs) == "" {
			return
		}
		fmt.Fprintf(&ch.body, `<p%s>`, epubAlignAttr(blk.align))
		ew.writeRuns(&ch.body, blk.runs)
		ch.body.WriteString("</p>\n")
		ch.empty = false
	}
}

// writeTable emits a detected table as XHTML: covered grid positions are
// omitted (the HTML span model), cell text keeps its styled runs.
func (ew *epubWriter) writeTable(b *strings.Builder, t *AbsorbedTable) {
	b.WriteString("<table>\n")
	for _, row := range t.RowList() {
		b.WriteString("<tr>")
		cells := row.CellList()
		for c := 0; c < len(cells); c++ {
			cell := cells[c]
			if cell.Covered {
				continue
			}
			b.WriteString("<td")
			if cell.ColSpan > 1 {
				fmt.Fprintf(b, ` colspan="%d"`, cell.ColSpan)
			}
			if cell.RowSpan > 1 {
				fmt.Fprintf(b, ` rowspan="%d"`, cell.RowSpan)
			}
			if cell.Shading != nil {
				fmt.Fprintf(b, ` style="background-color:#%s"`, docxColor(*cell.Shading))
			}
			b.WriteString(">")
			ew.writeCellRuns(b, cell)
			b.WriteString("</td>")
			c += cell.ColSpan - 1
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}

// writeCellRuns emits the cell's styled runs (lines joined with <br/>).
func (ew *epubWriter) writeCellRuns(b *strings.Builder, cell *AbsorbedCell) {
	ew.writeRuns(b, absorbedCellRuns(cell))
}

func epubAlignAttr(align int8) string {
	switch align {
	case 1:
		return ` style="text-align:center"`
	case 2:
		return ` style="text-align:right"`
	}
	return ""
}

func (ew *epubWriter) writeRuns(b *strings.Builder, runs []docRun) {
	runs = mergeRuns(runs, epubSameStyle)
	for _, r := range runs {
		switch {
		case r.br:
			b.WriteString("<br/>")
		case r.icon != nil:
			if src := ew.imageFile(r.icon); src != "" {
				h := r.icon.PageHeight
				if h <= 0 || h > 24 {
					h = 12
				}
				fmt.Fprintf(b, `<img class="ic" src="../%s" alt="" style="height:%.0fpt"/>`, src, h)
			}
		default:
			ew.writeStyledRun(b, r)
		}
	}
}

func (ew *epubWriter) writeStyledRun(b *strings.Builder, r docRun) {
	if r.text == "" {
		return
	}
	open, close := "", ""
	if r.link != "" {
		open += `<a href="` + xmlEscape(r.link) + `">`
		close = "</a>" + close
	}
	if r.code {
		open += "<code>"
		close = "</code>" + close
	}
	if r.bold {
		open += "<b>"
		close = "</b>" + close
	}
	if r.italic {
		open += "<i>"
		close = "</i>" + close
	}
	if r.super {
		open += "<sup>"
		close = "</sup>" + close
	} else if r.sub {
		open += "<sub>"
		close = "</sub>" + close
	}
	if c := docxColor(r.color); c != "000000" && r.link == "" {
		open += `<span style="color:#` + c + `">`
		close = "</span>" + close
	}
	b.WriteString(open)
	b.WriteString(xmlEscape(r.text))
	b.WriteString(close)
}

// imageFile stores the image bytes as an OEBPS part (SHA-256 deduped) and
// returns its OEBPS-relative path.
func (ew *epubWriter) imageFile(img *Image) string {
	if len(img.Data) == 0 {
		return ""
	}
	if ew.imgByKey == nil {
		ew.imgByKey = map[[32]byte]string{}
	}
	key := sha256.Sum256(img.Data)
	if src, ok := ew.imgByKey[key]; ok {
		return src
	}
	ew.imgSeq++
	ext := "png"
	if img.Format == ImageFormatJPEG {
		ext = "jpg"
	}
	src := fmt.Sprintf("images/img%03d.%s", ew.imgSeq, ext)
	ew.images = append(ew.images, docxPart{name: src, data: img.Data})
	ew.imgByKey[key] = src
	return src
}

// --- package assembly -------------------------------------------------------------

const epubCSS = `body { font-family: sans-serif; line-height: 1.5; }
h1, h2, h3 { line-height: 1.25; }
pre { background: #f2f2f2; padding: 0.6em; overflow-x: auto; font-size: 0.85em; }
code { background: #f2f2f2; }
img { max-width: 100%; }
img.ic { vertical-align: text-bottom; }
p.img { text-align: center; }
table { border-collapse: collapse; margin: 0.8em 0; }
td { border: 1px solid #999; padding: 0.25em 0.5em; }
`

func (ew *epubWriter) chapterXHTML(ch *epubChapter) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + "\n")
	fmt.Fprintf(&b, "<head><title>%s</title><link rel=\"stylesheet\" type=\"text/css\" href=\"../styles.css\"/></head>\n", xmlEscape(ew.title))
	b.WriteString("<body>\n")
	b.WriteString(ch.body.String())
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func (ew *epubWriter) navXHTML() string {
	// Normalize the flat heading list into a two-level tree: a level-2
	// entry appearing before any level-1 becomes a top-level entry itself.
	type navNode struct {
		epubTOCEntry
		kids []epubTOCEntry
	}
	var tops []navNode
	for _, e := range ew.toc {
		if e.level <= 1 || len(tops) == 0 {
			tops = append(tops, navNode{epubTOCEntry: e})
			continue
		}
		last := &tops[len(tops)-1]
		last.kids = append(last.kids, e)
	}
	if len(tops) == 0 {
		tops = []navNode{{epubTOCEntry: epubTOCEntry{title: ew.title, file: ew.chapters[0].file}}}
	}

	href := func(e epubTOCEntry) string {
		if e.id != "" {
			return e.file + "#" + e.id
		}
		return e.file
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + "\n")
	fmt.Fprintf(&b, "<head><title>%s</title></head>\n<body>\n", xmlEscape(ew.title))
	b.WriteString(`<nav epub:type="toc"><ol>` + "\n")
	for _, top := range tops {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a>`, xmlEscape(href(top.epubTOCEntry)), xmlEscape(top.title))
		if len(top.kids) > 0 {
			b.WriteString("<ol>")
			for _, k := range top.kids {
				fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, xmlEscape(href(k)), xmlEscape(k.title))
			}
			b.WriteString("</ol>")
		}
		b.WriteString("</li>\n")
	}
	b.WriteString("</ol></nav>\n</body>\n</html>\n")
	return b.String()
}

func (ew *epubWriter) contentOPF() string {
	// A deterministic identifier keeps builds reproducible.
	sum := sha256.Sum256([]byte(ew.title + fmt.Sprint(len(ew.chapters), len(ew.images))))
	uid := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">` + "\n")
	b.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	fmt.Fprintf(&b, "<dc:identifier id=\"uid\">%s</dc:identifier>\n", uid)
	fmt.Fprintf(&b, "<dc:title>%s</dc:title>\n", xmlEscape(ew.title))
	fmt.Fprintf(&b, "<dc:language>%s</dc:language>\n", xmlEscape(ew.lang))
	fmt.Fprintf(&b, "<meta property=\"dcterms:modified\">%s</meta>\n", ew.modified)
	b.WriteString("</metadata>\n<manifest>\n")
	b.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	b.WriteString(`<item id="css" href="styles.css" media-type="text/css"/>` + "\n")
	for i, ch := range ew.chapters {
		fmt.Fprintf(&b, "<item id=\"c%d\" href=\"%s\" media-type=\"application/xhtml+xml\"/>\n", i+1, ch.file)
	}
	for i, img := range ew.images {
		mt := "image/png"
		if strings.HasSuffix(img.name, ".jpg") {
			mt = "image/jpeg"
		}
		fmt.Fprintf(&b, "<item id=\"i%d\" href=\"%s\" media-type=\"%s\"/>\n", i+1, img.name, mt)
	}
	b.WriteString("</manifest>\n<spine>\n")
	for i := range ew.chapters {
		fmt.Fprintf(&b, "<itemref idref=\"c%d\"/>\n", i+1)
	}
	b.WriteString("</spine>\n</package>\n")
	return b.String()
}

const epubContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>
`

// writeZip assembles the EPUB container: the mimetype entry must be the
// FIRST zip entry and STORED (uncompressed), per the OCF spec.
func (ew *epubWriter) writeZip(w io.Writer) error {
	zw := zip.NewWriter(w)
	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mf, err := zw.CreateHeader(mh)
	if err != nil {
		return fmt.Errorf("epub: mimetype: %w", err)
	}
	if _, err := mf.Write([]byte("application/epub+zip")); err != nil {
		return err
	}
	add := func(name, data string) error {
		f, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("epub: create %s: %w", name, err)
		}
		_, err = f.Write([]byte(data))
		return err
	}
	if err := add("META-INF/container.xml", epubContainerXML); err != nil {
		return err
	}
	if err := add("OEBPS/content.opf", ew.contentOPF()); err != nil {
		return err
	}
	if err := add("OEBPS/nav.xhtml", ew.navXHTML()); err != nil {
		return err
	}
	if err := add("OEBPS/styles.css", epubCSS); err != nil {
		return err
	}
	for _, ch := range ew.chapters {
		if err := add("OEBPS/"+ch.file, ew.chapterXHTML(ch)); err != nil {
			return err
		}
	}
	for _, img := range ew.images {
		f, err := zw.Create("OEBPS/" + img.name)
		if err != nil {
			return fmt.Errorf("epub: create %s: %w", img.name, err)
		}
		if _, err := f.Write(img.data); err != nil {
			return err
		}
	}
	return zw.Close()
}
