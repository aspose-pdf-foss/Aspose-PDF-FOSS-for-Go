// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PDF → Markdown export (epic pdf-go-ho25) — the reverse of the Markdown
// renderer: the document's content is re-assembled as GFM-flavoured Markdown.
// Built on the shared flow-reconstruction core (flow_doc.go): buildFlowDoc
// supplies blocks/body size/furniture suppression, segmentParagraph recovers
// headings/lists/code the structural extractor merged, and segmentRuns
// rebuilds styled runs with gap-synthesized spaces. This file is the
// Markdown serializer over that model: **bold**/*italic* markers, `code`
// spans, [links](…), list markers with X-indent nesting, and fenced code
// blocks with indentation reconstructed from X offsets. Ruled tables (via
// the TableAbsorber) become GFM pipe tables — spans flatten, since GFM has
// no colspan/rowspan.

// MarkdownSaveOptions configures SaveMarkdown / WriteMarkdown. The zero
// value exports all pages; SaveMarkdown writes images into "<stem>_files"
// next to the output.
type MarkdownSaveOptions struct {
	// Pages is a 1-based subset (in the given order); nil = all pages.
	Pages []int
	// ImageDir (SaveMarkdown only) is the directory, relative to the output
	// file, that images are written into. Empty → "<stem>_files".
	ImageDir string
	// ImageWriter externalizes images anywhere (S3, CDN, …): it receives a
	// unique name + bytes and returns the URL to reference. Byte-identical
	// images are written once (SHA-256 dedup).
	ImageWriter func(name string, data []byte) (url string, err error)
	// EmbedImages inlines images as data: URLs instead of files. Note that
	// some renderers (e.g. GitHub) do not display data: images.
	EmbedImages bool
	// NoImages skips images entirely.
	NoImages bool
}

// SaveMarkdown writes the document as a Markdown file. Images go to
// opts.ImageDir (default "<stem>_files") unless EmbedImages/NoImages or an
// ImageWriter says otherwise. Mirrors the shape of SaveHTML.
func (d *Document) SaveMarkdown(path string, opts ...MarkdownSaveOptions) error {
	var opt MarkdownSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.ImageWriter == nil && !opt.EmbedImages && !opt.NoImages {
		dir := opt.ImageDir
		if dir == "" {
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			dir = stem + "_files"
		}
		opt.ImageWriter = dirResourceWriter(filepath.Dir(path), dir)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save markdown: %w", err)
	}
	werr := d.WriteMarkdown(f, opt)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return fmt.Errorf("save markdown: %w", cerr)
	}
	return nil
}

// WriteMarkdown writes the document as Markdown to w. Without an ImageWriter
// and with EmbedImages unset, images are skipped (a stream has no natural
// place for files).
func (d *Document) WriteMarkdown(w io.Writer, opts ...MarkdownSaveOptions) error {
	var opt MarkdownSaveOptions
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
				return fmt.Errorf("WriteMarkdown: page %d out of range 1..%d", n, len(pages))
			}
		}
	}

	sink := d.mdImageSink(opt)

	// Pass 1: the shared reconstruction core.
	doc, err := buildFlowDoc(pages, sel, flowDocOptions{
		dropFurniture: true,
		dropRotated:   true,
		collectLinks:  true,
		images:        !opt.NoImages && sink != nil,
		detectTables:  true,
	})
	if err != nil {
		return err
	}

	// Pass 2: serialize.
	var b strings.Builder
	st := &mdEmitState{}
	imgSeq := 0
	for _, fp := range doc.pages {
		for _, blk := range fp.blocks {
			if blk.table != nil {
				mdWriteTable(&b, blk.table)
				st.reset()
				continue
			}
			if blk.img != nil {
				imgSeq++
				url, err := mdImageURL(sink, fp.number, imgSeq, blk.img)
				if err != nil {
					return err
				}
				mdBlankLine(&b)
				fmt.Fprintf(&b, "![](%s)\n", url)
				st.reset()
				continue
			}
			for _, seg := range segmentParagraph(blk.para) {
				if doc.furniture.dropSegment(seg, fp.pageH) {
					continue // a header/footer line merged into a content paragraph
				}
				mdWriteSegment(&b, seg, doc.bodySize, fp.links, st)
			}
		}
	}
	_, err = io.WriteString(w, b.String())
	return err
}

// mdWriteTable emits a detected table as a GFM pipe table: the first row is
// the header, spans flatten (anchor text at its position, covered cells
// empty), multi-line cells join with spaces, and a data column whose cells
// are mostly right-aligned gets the ---: alignment marker.
func mdWriteTable(b *strings.Builder, t *AbsorbedTable) {
	rows := t.RowList()
	if len(rows) == 0 {
		return
	}
	cols := len(rows[0].CellList())
	cellText := func(cell *AbsorbedCell) string {
		if cell.Covered {
			return ""
		}
		txt := collapseWS(strings.ReplaceAll(cell.Text(), "\n", " "))
		return strings.ReplaceAll(mdEscapeText(txt), "|", "\\|")
	}
	// Column alignment vote over the data rows.
	rightVotes := make([]int, cols)
	dataRows := 0
	for _, row := range rows[1:] {
		dataRows++
		for c, cell := range row.CellList() {
			if c >= cols || cell.Covered || len(cell.TextFragments()) == 0 {
				continue
			}
			frs := cell.TextFragments()
			llx, urx := frs[0].X, frs[0].X+frs[0].Width
			for _, fr := range frs {
				llx = minf(llx, fr.X)
				urx = maxf(urx, fr.X+fr.Width)
			}
			colW := cell.Rect.URX - cell.Rect.LLX
			if colW > 0 && cell.Rect.URX-urx <= maxf(8, 0.08*colW) &&
				llx-cell.Rect.LLX > (cell.Rect.URX-urx)+maxf(6, 0.1*colW) {
				rightVotes[c]++
			}
		}
	}

	mdBlankLine(b)
	writeRow := func(row *AbsorbedRow) {
		b.WriteString("|")
		for c, cell := range row.CellList() {
			if c >= cols {
				break
			}
			b.WriteString(" " + cellText(cell) + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|")
	for c := 0; c < cols; c++ {
		if dataRows > 0 && rightVotes[c]*2 > dataRows {
			b.WriteString(" ---: |")
		} else {
			b.WriteString(" --- |")
		}
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		writeRow(row)
	}
}

// mdEmitState carries the serializer's cross-block context: the active list
// (kind + its base indent, for nesting) and the just-emitted heading (so a
// heading the extractor split across lines can be merged back).
type mdEmitState struct {
	listKind     string  // "-" / "1" while inside a list
	listIndentX  float64 // base indent of the list's top-level items
	headingLevel int     // level of the immediately-preceding heading block
}

func (s *mdEmitState) reset() {
	s.listKind = ""
	s.headingLevel = 0
}

// mdBlankLine ensures exactly one blank line before the next block.
func mdBlankLine(b *strings.Builder) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
}

// --- image sinks ------------------------------------------------------------------

// mdSink turns an image into a referencable URL; nil = skip images.
type mdSink func(name, mime string, data []byte) (string, error)

func (d *Document) mdImageSink(opt MarkdownSaveOptions) mdSink {
	switch {
	case opt.NoImages:
		return nil
	case opt.EmbedImages:
		return func(_, mime string, data []byte) (string, error) {
			return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
		}
	case opt.ImageWriter != nil:
		cache := map[[32]byte]string{}
		return func(name, _ string, data []byte) (string, error) {
			key := sha256.Sum256(data)
			if url, ok := cache[key]; ok {
				return url, nil
			}
			url, err := opt.ImageWriter(name, data)
			if err != nil {
				return "", err
			}
			cache[key] = url
			return url, nil
		}
	default:
		return nil // WriteMarkdown without a writer: skip
	}
}

func mdImageURL(sink mdSink, page, seq int, img *Image) (string, error) {
	mime, ext := "image/png", "png"
	if img.Format == ImageFormatJPEG {
		mime, ext = "image/jpeg", "jpg"
	}
	return sink(fmt.Sprintf("p%d_img%d.%s", page, seq, ext), mime, img.Data)
}

// --- segment serialization --------------------------------------------------------

// mdSameStyle is the Markdown view of run equivalence: only the attributes
// Markdown can express matter (font/size/colour coarsen away).
func mdSameStyle(a, b docRun) bool {
	return a.bold == b.bold && a.italic == b.italic && a.code == b.code && a.link == b.link
}

// mdWriteSegment classifies and emits one homogeneous segment, updating the
// serializer state (active list, trailing heading).
func mdWriteSegment(b *strings.Builder, seg docSeg, bodySize float64, links []linkArea, st *mdEmitState) {
	if len(seg.lines) == 0 {
		return
	}

	// Fenced code block: monospace-dominant segment.
	if seg.mono {
		mdBlankLine(b)
		mdWriteCodeBlock(b, seg)
		st.reset()
		return
	}

	runs := mergeRuns(segmentRuns(seg, links), mdSameStyle)

	// List item (nested when indented past the list's base items). Markdown
	// caps nesting at one level: deeper X-indents are usually layout artifacts
	// (table-cell lists, hanging wraps), and 8+ leading spaces would turn the
	// item into an indented code block anyway. DOCX's <w:ilvl> can use the
	// full listNestDepth once parent levels are tracked.
	if seg.marker != "" {
		depth := 0
		if st.listKind != "" && listNestDepth(seg.indentX, st.listIndentX) > 0 {
			depth = 1
		}
		if st.listKind == "" || (st.listKind != seg.marker && depth == 0) {
			mdBlankLine(b)
			st.listKind = seg.marker
			st.listIndentX = seg.indentX
		}
		if depth > 0 {
			b.WriteString(strings.Repeat("    ", depth))
		}
		if seg.marker == "1" {
			stripRunsPrefix(&runs, orderedRunRe)
			fmt.Fprintf(b, "%s. ", seg.ordNum)
		} else {
			stripRunsPrefix(&runs, bulletRunRe)
			b.WriteString("- ")
		}
		mdEmitRuns(b, runs)
		b.WriteString("\n")
		st.headingLevel = 0
		return
	}

	// Heading by size ratio (shared thresholds with the HTML flow mode).
	text := runsPlainText(runs)
	if level := headingLevel(seg.size, bodySize, len(text)); level > 0 {
		escaped := mdEscapeText(collapseWS(text))
		if level == st.headingLevel {
			// Continuation of a multi-line heading the extractor split:
			// append to the just-emitted heading line.
			head := b.String()
			b.Reset()
			b.WriteString(strings.TrimRight(head, "\n") + " " + escaped + "\n")
			return
		}
		mdBlankLine(b)
		fmt.Fprintf(b, "%s %s\n", strings.Repeat("#", level), escaped)
		st.reset()
		st.headingLevel = level
		return
	}

	mdBlankLine(b)
	start := b.Len()
	mdEmitRuns(b, runs)
	mdEscapeBlockStart(b, start)
	b.WriteString("\n")
	st.reset()
}

// mdWriteCodeBlock emits a monospace segment as a fenced code block, one
// output line per visual line, indentation reconstructed from X offsets
// (≈0.6 em per character for monospace faces).
func mdWriteCodeBlock(b *strings.Builder, seg docSeg) {
	minX := segMinStartX(seg)
	charW := 0.6 * seg.size
	if charW <= 0 {
		charW = 6
	}
	var lines []string
	hasBackticks := false
	for _, line := range seg.lines {
		var lb strings.Builder
		if len(line.Fragments) > 0 {
			if pad := int((line.Fragments[0].X-minX)/charW + 0.5); pad > 0 {
				lb.WriteString(strings.Repeat(" ", pad))
			}
		}
		prevEnd := 0.0
		for i, fr := range line.Fragments {
			if i > 0 && gapIsSpace(prevEnd, fr) {
				lb.WriteString(" ")
			}
			lb.WriteString(fr.Text)
			prevEnd = fr.X + fr.Width
		}
		s := strings.TrimRight(lb.String(), " ")
		if strings.Contains(s, "```") {
			hasBackticks = true
		}
		lines = append(lines, s)
	}
	fence := "```"
	if hasBackticks {
		fence = "````"
	}
	b.WriteString(fence + "\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n" + fence + "\n")
}

// stripRunsPrefix removes the list-marker prefix (already matched against the
// paragraph text) from the front of the run list.
func stripRunsPrefix(runs *[]docRun, re *regexp.Regexp) {
	joined := ""
	for _, r := range *runs {
		joined += r.text
	}
	m := re.FindString(joined)
	remain := len(m)
	out := (*runs)[:0]
	for _, r := range *runs {
		if remain >= len(r.text) {
			remain -= len(r.text)
			continue
		}
		if remain > 0 {
			r.text = r.text[remain:]
			remain = 0
		}
		out = append(out, r)
	}
	*runs = out
}

// mdEmitRuns writes styled runs as Markdown, keeping emphasis markers tight
// against non-space text (spaces migrate outside the markers).
func mdEmitRuns(b *strings.Builder, runs []docRun) {
	for _, r := range runs {
		lead := r.text[:len(r.text)-len(strings.TrimLeft(r.text, " "))]
		trail := r.text[len(strings.TrimRight(r.text, " ")):]
		core := strings.TrimSpace(r.text)
		b.WriteString(lead)
		if core != "" {
			b.WriteString(mdStyledText(core, r))
		}
		b.WriteString(trail)
	}
}

// mdStyledText wraps escaped text in the run's markers.
func mdStyledText(core string, r docRun) string {
	var s string
	if r.code {
		s = mdBacktickSpan(core)
	} else {
		s = mdEscapeText(core)
		switch {
		case r.bold && r.italic:
			s = "***" + s + "***"
		case r.bold:
			s = "**" + s + "**"
		case r.italic:
			s = "*" + s + "*"
		}
	}
	if r.link != "" {
		s = "[" + s + "](" + mdEscapeLinkDest(r.link) + ")"
	}
	return s
}

// mdBacktickSpan wraps core in backticks, lengthening the fence when the text
// itself contains backticks.
func mdBacktickSpan(core string) string {
	fence := "`"
	for strings.Contains(core, fence) {
		fence += "`"
	}
	pad := ""
	if strings.HasPrefix(core, "`") || strings.HasSuffix(core, "`") {
		pad = " "
	}
	return fence + pad + core + pad + fence
}

var mdEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"[", `\[`,
	"]", `\]`,
	"<", `\<`,
)

// mdEscapeText escapes Markdown-significant characters in plain text.
func mdEscapeText(s string) string {
	return mdEscaper.Replace(s)
}

// mdEscapeLinkDest escapes characters that would terminate a (…) link
// destination.
func mdEscapeLinkDest(s string) string {
	s = strings.ReplaceAll(s, "(", "%28")
	s = strings.ReplaceAll(s, ")", "%29")
	return strings.ReplaceAll(s, " ", "%20")
}

var mdBlockStartRe = regexp.MustCompile(`^(\d{1,9})([.)])( |$)`)

// mdEscapeBlockStart neutralizes constructs that would change the block's
// meaning at line start (#, >, -, +, "N."), operating on what was just
// written from offset start.
func mdEscapeBlockStart(b *strings.Builder, start int) {
	s := b.String()[start:]
	if s == "" {
		return
	}
	esc := ""
	switch {
	case strings.HasPrefix(s, "# "), strings.HasPrefix(s, "> "),
		strings.HasPrefix(s, "- "), strings.HasPrefix(s, "+ "),
		s[0] == '#' && strings.TrimLeft(s, "#") != s && strings.HasPrefix(strings.TrimLeft(s, "#"), " "):
		esc = `\` + s
	default:
		if m := mdBlockStartRe.FindStringSubmatch(s); m != nil {
			esc = m[1] + `\` + s[len(m[1]):]
		}
	}
	if esc != "" {
		head := b.String()[:start]
		b.Reset()
		b.WriteString(head)
		b.WriteString(esc)
	}
}
