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
// Built on the flow-mode structural analysis shared with the HTML exporter
// (Paragraphs() + heading inference against the length-weighted body font
// size + image interleaving by vertical position), plus richer inline
// fidelity: **bold**/*italic* runs merged from text fragments, `code` spans
// by monospace font, [links](…) recovered from link annotations by rectangle
// intersection, list items by bullet-glyph/numbering detection, and fenced
// code blocks for monospace paragraphs with indentation reconstructed from X
// offsets. Tables are not reconstructed (no table reader yet) — their cell
// text flows as paragraphs.

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

	// Pass 1: structure + body-size median (shared with the HTML flow mode).
	type mdPage struct {
		blocks []flowBlock
		links  []mdLinkArea
		pageH  float64
	}
	mps := make([]mdPage, 0, len(sel))
	var sizes []struct {
		size   float64
		weight int
	}
	// Header/footer suppression: paragraphs near the page's top/bottom edge
	// whose digit-masked text repeats across pages are page furniture.
	furniture := map[string]int{} // masked text → number of pages it appears on
	pageHs := make([]float64, len(sel))
	for i, n := range sel {
		p := pages[n-1]
		if size, err := p.Size(); err == nil {
			pageHs[i] = size.Height
		}
		pm, err := p.Paragraphs()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for si := range pm.Sections {
			for pi := range pm.Sections[si].Paragraphs {
				para := &pm.Sections[si].Paragraphs[pi]
				if key := mdFurnitureKey(para, pageHs[i]); key != "" && !seen[key] {
					seen[key] = true
					furniture[key]++
				}
			}
		}
	}
	minRepeats := len(sel)/2 + 1
	if minRepeats < 3 {
		minRepeats = 3
	}

	for selIdx, n := range sel {
		p := pages[n-1]
		pm, err := p.Paragraphs()
		if err != nil {
			return err
		}
		var blocks []flowBlock
		for si := range pm.Sections {
			for pi := range pm.Sections[si].Paragraphs {
				para := &pm.Sections[si].Paragraphs[pi]
				if strings.TrimSpace(para.Text) == "" {
					continue
				}
				if mdIsRotatedDecoration(para) {
					continue // diagonal watermarks and other rotated overlays
				}
				if key := mdFurnitureKey(para, pageHs[selIdx]); key != "" && furniture[key] >= minRepeats {
					continue // repeating header/footer line
				}
				blocks = append(blocks, flowBlock{para: para, top: para.Rectangle.URY})
				for _, line := range para.Lines {
					for _, fr := range line.Fragments {
						if fr.FontSize > 0 {
							sizes = append(sizes, struct {
								size   float64
								weight int
							}{fr.FontSize, len([]rune(fr.Text))})
						}
					}
				}
			}
		}
		if !opt.NoImages && sink != nil {
			if imgs, err := p.ExtractImages(); err == nil {
				for i := range imgs {
					img := &imgs[i]
					if len(img.Data) == 0 {
						continue
					}
					insertFlowImage(&blocks, flowBlock{img: img, top: img.Y + img.PageHeight})
				}
			}
		}
		mps = append(mps, mdPage{blocks: blocks, links: pageLinkAreas(p), pageH: pageHs[selIdx]})
	}
	body := weightedMedianSize(sizes)

	// Pass 2: serialize.
	var b strings.Builder
	st := &mdEmitState{}
	imgSeq := 0
	for i, mp := range mps {
		for _, blk := range mp.blocks {
			if blk.img != nil {
				imgSeq++
				url, err := mdImageURL(sink, sel[i], imgSeq, blk.img)
				if err != nil {
					return err
				}
				mdBlankLine(&b)
				fmt.Fprintf(&b, "![](%s)\n", url)
				st.reset()
				continue
			}
			mdWriteParagraph(&b, blk.para, body, mp.links, st, &mdFurniture{keys: furniture, min: minRepeats, pageH: mp.pageH})
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
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

var mdDigitsRe = regexp.MustCompile(`\d+`)

// mdFurnitureKey returns a page-invariant key for a paragraph that sits in
// the header/footer band (top or bottom ~12% of the page), with digits masked
// so "Page 1 / 15" matches "Page 2 / 15"; "" when the paragraph is content.
func mdFurnitureKey(para *MarkupParagraph, pageH float64) string {
	if pageH <= 0 {
		return ""
	}
	if para.Rectangle.LLY < 0.88*pageH && para.Rectangle.URY > 0.12*pageH {
		return "" // not in the edge bands
	}
	text := collapseWS(para.Text)
	if text == "" || len([]rune(text)) > 120 {
		return ""
	}
	return mdDigitsRe.ReplaceAllString(text, "#")
}

// mdIsRotatedDecoration reports whether every fragment of the paragraph is
// rotated — diagonal watermarks, stamps and axis labels are decoration, not
// document flow.
func mdIsRotatedDecoration(para *MarkupParagraph) bool {
	any := false
	for _, line := range para.Lines {
		for _, fr := range line.Fragments {
			any = true
			if fr.Rotation == 0 {
				return false
			}
		}
	}
	return any
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

// --- links ------------------------------------------------------------------------

// mdLinkArea is a clickable region with its external destination.
type mdLinkArea struct {
	rect Rectangle
	uri  string
}

// pageLinkAreas collects the page's URI link annotations.
func pageLinkAreas(p *Page) []mdLinkArea {
	var out []mdLinkArea
	for _, a := range p.Annotations().All() {
		link, ok := a.(*LinkAnnotation)
		if !ok {
			continue
		}
		if act, ok := link.Action().(*GoToURIAction); ok && act.URI() != "" {
			out = append(out, mdLinkArea{rect: link.Rect(), uri: act.URI()})
		}
	}
	return out
}

// linkURIAt returns the URI whose area contains the point, or "".
func linkURIAt(links []mdLinkArea, x, y float64) string {
	for _, l := range links {
		if x >= l.rect.LLX && x <= l.rect.URX && y >= l.rect.LLY && y <= l.rect.URY {
			return l.uri
		}
	}
	return ""
}

// --- paragraph serialization ------------------------------------------------------

var (
	mdBulletRunRe  = regexp.MustCompile(`^\s*[•◦▪‣∙·–—*-]\s+`)
	mdOrderedRunRe = regexp.MustCompile(`^\s*(\d{1,3})[.)]\s+`)
)

// mdWriteParagraph splits one extracted paragraph into homogeneous segments
// (Paragraphs() can merge a heading with its body, or a whole list plus a
// code block into one paragraph when the line spacing is uniform), classifies
// each, and emits it. Returns the list kind ("-", "1") when the LAST emitted
// segment was a list item, else "".
func mdWriteParagraph(b *strings.Builder, para *MarkupParagraph, bodySize float64, links []mdLinkArea, st *mdEmitState, fk *mdFurniture) {
	for _, seg := range mdSegments(para) {
		if fk.dropSegment(seg) {
			continue // a header/footer line merged into a content paragraph
		}
		mdWriteSegment(b, seg, bodySize, links, st)
	}
}

// mdFurniture filters repeating header/footer lines at segment level (the
// paragraph-level filter misses furniture the extractor merged into content).
type mdFurniture struct {
	keys  map[string]int
	min   int
	pageH float64
}

func (fk *mdFurniture) dropSegment(seg mdSeg) bool {
	if fk == nil || fk.pageH <= 0 || len(seg.lines) == 0 {
		return false
	}
	y := seg.lines[0].Y
	if y < 0.88*fk.pageH && y > 0.12*fk.pageH {
		return false
	}
	var texts []string
	for _, l := range seg.lines {
		texts = append(texts, mdLineText(l))
	}
	key := mdDigitsRe.ReplaceAllString(collapseWS(strings.Join(texts, " ")), "#")
	return fk.keys[key] >= fk.min
}

// mdSeg is a run of visually-homogeneous lines within one extracted
// paragraph: same dominant font size, same mono-ness, one list item.
type mdSeg struct {
	lines   []TextLine
	size    float64 // dominant font size
	mono    bool    // every line is monospace-dominant
	marker  string  // "-", "1" when the segment opens with a list marker; else ""
	ordNum  string  // the ordinal ("2") for ordered items
	indentX float64 // X of the first line's first fragment (nesting signal)
}

// mdSegments splits a paragraph's lines on list-marker starts, dominant-size
// jumps, monospace flips, and enlarged vertical gaps (a paragraph break the
// structural extractor merged away).
func mdSegments(para *MarkupParagraph) []mdSeg {
	var segs []mdSeg
	var cur *mdSeg
	prevY := 0.0
	for li, line := range para.Lines {
		size, mono := mdLineDominant(line)
		marker, ord := mdLineMarker(line)
		sizeJump := cur != nil && cur.size > 0 && size > 0 &&
			(size/cur.size > 1.12 || cur.size/size > 1.12)
		gapJump := cur != nil && li > 0 && size > 0 && (prevY-line.Y) > 1.6*size
		if cur == nil || marker != "" || mono != cur.mono || sizeJump || gapJump {
			segs = append(segs, mdSeg{size: size, mono: mono, marker: marker, ordNum: ord, indentX: lineStartX(line)})
			cur = &segs[len(segs)-1]
		}
		cur.lines = append(cur.lines, line)
		if size > cur.size {
			cur.size = size
		}
		prevY = line.Y
	}
	return segs
}

// lineStartX is the X of the line's first fragment (segment indent signal).
func lineStartX(line TextLine) float64 {
	if len(line.Fragments) == 0 {
		return 0
	}
	return line.Fragments[0].X
}

// mdLineDominant returns a line's length-weighted dominant font size and
// whether the line is monospace-dominant.
func mdLineDominant(line TextLine) (float64, bool) {
	sizeW := map[int]int{}
	monoW, totalW := 0, 0
	for _, fr := range line.Fragments {
		w := len([]rune(fr.Text))
		sizeW[int(fr.FontSize*10+0.5)] += w
		totalW += w
		if fontFamilyClass(fr.FontName) == "mono" {
			monoW += w
		}
	}
	best, bestW := 120, -1
	for s, w := range sizeW {
		if w > bestW {
			best, bestW = s, w
		}
	}
	return float64(best) / 10, totalW > 0 && monoW*2 > totalW
}

// mdLineMarker reports whether the line opens with a list marker: "-" for a
// bullet glyph, "1" (plus the ordinal) for "N."/"N)" numbering.
func mdLineMarker(line TextLine) (kind, ord string) {
	text := mdLineText(line)
	if m := mdOrderedRunRe.FindStringSubmatch(text); m != nil {
		return "1", m[1]
	}
	if mdBulletRunRe.MatchString(text) {
		return "-", ""
	}
	return "", ""
}

// mdLineText joins a line's fragments, synthesizing the spaces that live only
// as horizontal gaps between fragments (styled-run boundaries carry no space
// glyph).
func mdLineText(line TextLine) string {
	var b strings.Builder
	prevEnd := 0.0
	for i, fr := range line.Fragments {
		if i > 0 && mdGapIsSpace(prevEnd, fr) {
			b.WriteString(" ")
		}
		b.WriteString(fr.Text)
		prevEnd = fr.X + fr.Width
	}
	return b.String()
}

// mdGapIsSpace reports whether the horizontal gap before fr reads as a space.
func mdGapIsSpace(prevEnd float64, fr TextFragment) bool {
	threshold := fr.FontSize * 0.15
	if threshold <= 0 {
		threshold = 1.5
	}
	return fr.X-prevEnd > threshold
}

// mdWriteSegment classifies and emits one homogeneous segment, updating the
// serializer state (active list, trailing heading).
func mdWriteSegment(b *strings.Builder, seg mdSeg, bodySize float64, links []mdLinkArea, st *mdEmitState) {
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

	runs := mdSegRuns(seg, links)

	// List item (nested when indented past the list's base items).
	if seg.marker != "" {
		nested := st.listKind != "" && seg.indentX > st.listIndentX+6
		if st.listKind == "" || (st.listKind != seg.marker && !nested) {
			mdBlankLine(b)
			st.listKind = seg.marker
			st.listIndentX = seg.indentX
		}
		if nested {
			b.WriteString("    ")
		}
		if seg.marker == "1" {
			stripRunsPrefix(&runs, mdOrderedRunRe)
			fmt.Fprintf(b, "%s. ", seg.ordNum)
		} else {
			stripRunsPrefix(&runs, mdBulletRunRe)
			b.WriteString("- ")
		}
		mdEmitRuns(b, runs)
		b.WriteString("\n")
		st.headingLevel = 0
		return
	}

	// Heading by size ratio (same thresholds as the HTML flow mode).
	text := runsPlainText(runs)
	ratio := seg.size / bodySize
	level := 0
	switch {
	case ratio >= 1.7 && len(text) < 200:
		level = 1
	case ratio >= 1.35 && len(text) < 200:
		level = 2
	case ratio >= 1.14 && len(text) < 200:
		level = 3
	}
	if level > 0 {
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

func runsPlainText(runs []mdRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.text)
	}
	return b.String()
}

// mdWriteCodeBlock emits a monospace segment as a fenced code block, one
// output line per visual line, indentation reconstructed from X offsets
// (≈0.6 em per character for monospace faces).
func mdWriteCodeBlock(b *strings.Builder, seg mdSeg) {
	minX := 0.0
	for i, line := range seg.lines {
		if len(line.Fragments) == 0 {
			continue
		}
		if x := line.Fragments[0].X; i == 0 || x < minX {
			minX = x
		}
	}
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
			if i > 0 && mdGapIsSpace(prevEnd, fr) {
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

// --- inline runs ------------------------------------------------------------------

// mdRun is a maximal same-style stretch of paragraph text.
type mdRun struct {
	text         string
	bold, italic bool
	code         bool
	link         string
}

func (r mdRun) sameStyle(o mdRun) bool {
	return r.bold == o.bold && r.italic == o.italic && r.code == o.code && r.link == o.link
}

// mdSegRuns flattens a segment's fragments into styled runs: visual lines
// join with a space, adjacent same-style fragments merge, and inter-fragment
// gaps that read as spaces are synthesized back.
func mdSegRuns(seg mdSeg, links []mdLinkArea) []mdRun {
	var runs []mdRun
	appendRun := func(r mdRun) {
		if n := len(runs); n > 0 && runs[n-1].sameStyle(r) {
			runs[n-1].text += r.text
			return
		}
		runs = append(runs, r)
	}
	for li, line := range seg.lines {
		if li > 0 && len(runs) > 0 {
			runs[len(runs)-1].text += " "
		}
		prevEnd := 0.0
		for i, fr := range line.Fragments {
			if fr.Text == "" {
				continue
			}
			// Synthesize the space that exists only as a horizontal gap
			// between differently-styled fragments.
			if i > 0 && mdGapIsSpace(prevEnd, fr) && len(runs) > 0 {
				runs[len(runs)-1].text += " "
			}
			midX := fr.X + fr.Width/2
			midY := line.Y + fr.FontSize*0.35
			appendRun(mdRun{
				text:   fr.Text,
				bold:   fr.Bold,
				italic: fr.Italic,
				code:   fontFamilyClass(fr.FontName) == "mono",
				link:   linkURIAt(links, midX, midY),
			})
			prevEnd = fr.X + fr.Width
		}
	}
	return runs
}

// stripRunsPrefix removes the list-marker prefix (already matched against the
// paragraph text) from the front of the run list.
func stripRunsPrefix(runs *[]mdRun, re *regexp.Regexp) {
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
func mdEmitRuns(b *strings.Builder, runs []mdRun) {
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
func mdStyledText(core string, r mdRun) string {
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

// collapseWS collapses runs of whitespace to single spaces.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
