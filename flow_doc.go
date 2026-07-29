// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"sort"
	"strings"
)

// Format-neutral document-reconstruction core (epic pdf-go-7qiu, phase 1) —
// the shared analysis layer under every flow-mode exporter (Markdown, HTML
// flow, and the upcoming DOCX/EPUB writers). It turns rendered pages back
// into logical structure: ordered blocks (paragraphs + images) per page, the
// document's body font size, repeating header/footer detection, paragraph
// re-segmentation (headings/lists/code the structural extractor merged), and
// styled runs carrying the full fragment look (bold/italic/mono, font, size,
// colour, sub/superscript, hyperlink). Serializers stay per-format: they
// consume []docSeg / []docRun and emit their own syntax.

// flowBlock is one emitted unit — a paragraph or an image — ordered by its
// visual top within the page.
type flowBlock struct {
	para *MarkupParagraph
	img  *Image
	top  float64
	col  Rectangle // the column (MarkupSection) the paragraph belongs to
}

// sizeSample is one text run's vote for the document body font size.
type sizeSample struct {
	size   float64
	weight int
}

// flowDocOptions configures buildFlowDoc.
type flowDocOptions struct {
	dropFurniture bool // suppress repeating header/footer paragraphs
	dropRotated   bool // suppress fully-rotated paragraphs (watermarks)
	collectLinks  bool // gather URI link areas per page
	images        bool // interleave extracted images by vertical position
	// vectorGraphics rasterizes vector-drawing clusters (logos, charts) into
	// image blocks via the built-in renderer (see flow_vector.go).
	vectorGraphics bool
	// onParagraph is called for every kept paragraph (HTML flow registers
	// embedded-font usage here). May be nil.
	onParagraph func(p *Page, para *MarkupParagraph)
}

// flowDocPage is one source page's reconstructed content.
type flowDocPage struct {
	number int // 1-based source page number
	blocks []flowBlock
	links  []linkArea
	pageH  float64
	pageW  float64
}

// flowDoc is the reconstructed document: pages of ordered blocks plus the
// document-level analysis results.
type flowDoc struct {
	pages     []flowDocPage
	bodySize  float64
	furniture *furnitureFilter // non-nil when dropFurniture was set
}

// buildFlowDoc runs the shared pass 1 over the selected pages (1-based
// indices into pages): structural extraction, furniture detection, body-size
// median, image interleaving and link collection.
func buildFlowDoc(pages []*Page, sel []int, opt flowDocOptions) (*flowDoc, error) {
	// Extract once per page; the furniture pass needs all pages before the
	// block pass can filter.
	pms := make([]PageMarkup, len(sel))
	pageHs := make([]float64, len(sel))
	pageWs := make([]float64, len(sel))
	for i, n := range sel {
		p := pages[n-1]
		if size, err := p.Size(); err == nil {
			pageHs[i] = size.Height
			pageWs[i] = size.Width
		}
		pm, err := p.Paragraphs()
		if err != nil {
			return nil, err
		}
		pms[i] = pm
	}

	// Header/footer suppression: paragraphs near the page's top/bottom edge
	// whose digit-masked text repeats across pages are page furniture.
	var fk *furnitureFilter
	if opt.dropFurniture {
		keys := map[string]int{}
		for i := range sel {
			seen := map[string]bool{}
			for si := range pms[i].Sections {
				for pi := range pms[i].Sections[si].Paragraphs {
					para := &pms[i].Sections[si].Paragraphs[pi]
					if key := furnitureKey(para, pageHs[i]); key != "" && !seen[key] {
						seen[key] = true
						keys[key]++
					}
				}
			}
		}
		minRepeats := len(sel)/2 + 1
		if minRepeats < 3 {
			minRepeats = 3
		}
		fk = &furnitureFilter{keys: keys, min: minRepeats}
	}

	doc := &flowDoc{furniture: fk}
	var sizes []sizeSample
	for i, n := range sel {
		p := pages[n-1]

		// Raster images: their blocks and their footprint (vector geometry
		// drawn over an image — frames — belongs to the image, not to a
		// standalone graphics cluster).
		var imageBlocks []flowBlock
		var imageRects []Rectangle
		if opt.images {
			if imgs, err := p.ExtractImages(); err == nil {
				for j := range imgs {
					img := &imgs[j]
					if len(img.Data) == 0 {
						continue
					}
					imageRects = append(imageRects, Rectangle{
						LLX: img.X, LLY: img.Y,
						URX: img.X + img.PageWidth, URY: img.Y + img.PageHeight,
					})
					imageBlocks = append(imageBlocks, flowBlock{img: img, top: img.Y + img.PageHeight})
				}
			}
		}

		// Vector-graphics clusters (logos, charts): computed before the
		// paragraph pass so paragraphs living inside a cluster (chart
		// labels) can leave the flow — their text is in the patch.
		var vecBlocks []flowBlock
		var vecClusters []Rectangle
		if opt.vectorGraphics {
			var textRects []Rectangle
			for si := range pms[i].Sections {
				for pi := range pms[i].Sections[si].Paragraphs {
					para := &pms[i].Sections[si].Paragraphs[pi]
					for _, line := range para.Lines {
						for _, fr := range line.Fragments {
							textRects = append(textRects, Rectangle{
								LLX: fr.X, LLY: fr.Y,
								URX: fr.X + fr.Width, URY: fr.Y + fr.Height,
							})
						}
					}
				}
			}
			vecBlocks, vecClusters = vectorGraphicBlocks(p, imageRects, textRects)
		}

		var blocks []flowBlock
		for si := range pms[i].Sections {
			for pi := range pms[i].Sections[si].Paragraphs {
				para := &pms[i].Sections[si].Paragraphs[pi]
				if strings.TrimSpace(para.Text) == "" {
					continue
				}
				if opt.dropRotated && isRotatedDecoration(para) {
					continue // diagonal watermarks and other rotated overlays
				}
				if fk != nil && fk.dropParagraph(para, pageHs[i]) {
					continue // repeating header/footer line
				}
				if rectMostlyInside(para.Rectangle, vecClusters) {
					continue // lives inside a rasterized graphics cluster
				}
				col := pms[i].Sections[si].Rectangle
				if len(pms[i].Sections) == 1 && pageWs[i] > 0 {
					// A single-section page's section rect is just the text
					// bbox (degenerate on sparse cover-like pages) — the
					// page frame is the honest alignment/wrap reference.
					col = Rectangle{LLX: 0, LLY: 0, URX: pageWs[i], URY: pageHs[i]}
				}
				blocks = append(blocks, flowBlock{para: para, top: para.Rectangle.URY, col: col})
				if opt.onParagraph != nil {
					opt.onParagraph(p, para)
				}
				for _, line := range para.Lines {
					for _, fr := range line.Fragments {
						if fr.FontSize > 0 {
							sizes = append(sizes, sizeSample{fr.FontSize, len([]rune(fr.Text))})
						}
					}
				}
			}
		}
		for _, ib := range imageBlocks {
			r := Rectangle{LLX: ib.img.X, LLY: ib.img.Y,
				URX: ib.img.X + ib.img.PageWidth, URY: ib.img.Y + ib.img.PageHeight}
			if rectMostlyInside(r, vecClusters) {
				continue // already rendered inside a graphics patch
			}
			insertFlowImage(&blocks, ib)
		}
		for _, vb := range vecBlocks {
			insertFlowImage(&blocks, vb)
		}
		fp := flowDocPage{number: n, blocks: blocks, pageH: pageHs[i], pageW: pageWs[i]}
		if opt.collectLinks {
			fp.links = pageLinkAreas(p)
		}
		doc.pages = append(doc.pages, fp)
	}
	doc.bodySize = weightedMedianSize(sizes)
	return doc, nil
}

// rectMostlyInside reports whether >= 90% of r's area lies within one of the
// given rectangles.
func rectMostlyInside(r Rectangle, rects []Rectangle) bool {
	area := (r.URX - r.LLX) * (r.URY - r.LLY)
	if area <= 0 {
		return false
	}
	for _, c := range rects {
		w := minf(r.URX, c.URX) - maxf(r.LLX, c.LLX)
		h := minf(r.URY, c.URY) - maxf(r.LLY, c.LLY)
		if w > 0 && h > 0 && w*h >= 0.9*area {
			return true
		}
	}
	return false
}

// insertFlowImage places an image block before the first paragraph whose
// top edge lies below the image's top (keeping paragraph reading order).
func insertFlowImage(blocks *[]flowBlock, img flowBlock) {
	at := len(*blocks)
	for i, blk := range *blocks {
		if blk.para != nil && blk.top < img.top {
			at = i
			break
		}
	}
	*blocks = append(*blocks, flowBlock{})
	copy((*blocks)[at+1:], (*blocks)[at:])
	(*blocks)[at] = img
}

// weightedMedianSize returns the text-length-weighted median font size —
// the document's body size (12 when there is no text).
func weightedMedianSize(sizes []sizeSample) float64 {
	if len(sizes) == 0 {
		return 12
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i].size < sizes[j].size })
	total := 0
	for _, s := range sizes {
		total += s.weight
	}
	acc := 0
	for _, s := range sizes {
		acc += s.weight
		if acc*2 >= total {
			return s.size
		}
	}
	return sizes[len(sizes)-1].size
}

// headingLevel classifies a block as a heading (1..3) by its dominant font
// size relative to the document body size, or 0 for ordinary text. Long
// blocks are never headings.
func headingLevel(size, bodySize float64, textLen int) int {
	if bodySize <= 0 || textLen >= 200 {
		return 0
	}
	switch ratio := size / bodySize; {
	case ratio >= 1.7:
		return 1
	case ratio >= 1.35:
		return 2
	case ratio >= 1.14:
		return 3
	}
	return 0
}

// listNestDepth returns 0 for a top-level list item and n >= 1 for nested
// items — one level per ~18pt of indent past the list's base, with a 6pt
// tolerance for ragged alignment.
func listNestDepth(indentX, baseX float64) int {
	over := indentX - baseX - 6
	if over <= 0 {
		return 0
	}
	return 1 + int(over/18)
}

// --- furniture (repeating headers/footers) ----------------------------------------

var flowDigitsRe = regexp.MustCompile(`\d+`)

// furnitureKey returns a page-invariant key for a paragraph that sits in
// the header/footer band (top or bottom ~12% of the page), with digits masked
// so "Page 1 / 15" matches "Page 2 / 15"; "" when the paragraph is content.
func furnitureKey(para *MarkupParagraph, pageH float64) string {
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
	return flowDigitsRe.ReplaceAllString(text, "#")
}

// furnitureFilter suppresses repeating header/footer text, both as whole
// paragraphs and as segments the extractor merged into content.
type furnitureFilter struct {
	keys map[string]int // masked text → number of pages it appears on
	min  int            // repeats needed to count as furniture
}

// dropParagraph reports whether the paragraph is a repeating header/footer.
func (fk *furnitureFilter) dropParagraph(para *MarkupParagraph, pageH float64) bool {
	if fk == nil {
		return false
	}
	key := furnitureKey(para, pageH)
	return key != "" && fk.keys[key] >= fk.min
}

// dropSegment is the segment-level filter (the paragraph-level one misses
// furniture the extractor merged into a content paragraph).
func (fk *furnitureFilter) dropSegment(seg docSeg, pageH float64) bool {
	if fk == nil || pageH <= 0 || len(seg.lines) == 0 {
		return false
	}
	y := seg.lines[0].Y
	if y < 0.88*pageH && y > 0.12*pageH {
		return false
	}
	var texts []string
	for _, l := range seg.lines {
		texts = append(texts, lineJoinedText(l))
	}
	key := flowDigitsRe.ReplaceAllString(collapseWS(strings.Join(texts, " ")), "#")
	return fk.keys[key] >= fk.min
}

// isRotatedDecoration reports whether every fragment of the paragraph is
// rotated — diagonal watermarks, stamps and axis labels are decoration, not
// document flow.
func isRotatedDecoration(para *MarkupParagraph) bool {
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

// --- links ------------------------------------------------------------------------

// linkArea is a clickable region with its external destination.
type linkArea struct {
	rect Rectangle
	uri  string
}

// pageLinkAreas collects the page's URI link annotations.
func pageLinkAreas(p *Page) []linkArea {
	var out []linkArea
	for _, a := range p.Annotations().All() {
		link, ok := a.(*LinkAnnotation)
		if !ok {
			continue
		}
		if act, ok := link.Action().(*GoToURIAction); ok && act.URI() != "" {
			out = append(out, linkArea{rect: link.Rect(), uri: act.URI()})
		}
	}
	return out
}

// linkURIAt returns the URI whose area contains the point, or "".
func linkURIAt(links []linkArea, x, y float64) string {
	for _, l := range links {
		if x >= l.rect.LLX && x <= l.rect.URX && y >= l.rect.LLY && y <= l.rect.URY {
			return l.uri
		}
	}
	return ""
}

// --- segmentation -----------------------------------------------------------------

var (
	bulletRunRe  = regexp.MustCompile(`^\s*[•◦▪‣∙·–—*-]\s+`)
	orderedRunRe = regexp.MustCompile(`^\s*(\d{1,3})[.)]\s+`)
)

// docSeg is a run of visually-homogeneous lines within one extracted
// paragraph: same dominant font size, same mono-ness, one list item.
type docSeg struct {
	lines   []TextLine
	size    float64 // dominant font size
	mono    bool    // every line is monospace-dominant
	marker  string  // "-", "1" when the segment opens with a list marker; else ""
	ordNum  string  // the ordinal ("2") for ordered items
	indentX float64 // X of the first line's first fragment (nesting signal)
}

// segmentParagraph splits a paragraph's lines on list-marker starts,
// dominant-size jumps, monospace flips, and enlarged vertical gaps (a
// paragraph break the structural extractor merged away).
func segmentParagraph(para *MarkupParagraph) []docSeg {
	var segs []docSeg
	var cur *docSeg
	prevY := 0.0
	for li, line := range para.Lines {
		size, mono := lineDominant(line)
		marker, ord := lineMarker(line)
		sizeJump := cur != nil && cur.size > 0 && size > 0 &&
			(size/cur.size > 1.12 || cur.size/size > 1.12)
		gapJump := cur != nil && li > 0 && size > 0 && (prevY-line.Y) > 1.6*size
		if cur == nil || marker != "" || mono != cur.mono || sizeJump || gapJump {
			segs = append(segs, docSeg{size: size, mono: mono, marker: marker, ordNum: ord, indentX: lineStartX(line)})
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

// segMinStartX is the smallest first-fragment X across the segment's lines —
// the left edge a code block's indentation is measured from.
func segMinStartX(seg docSeg) float64 {
	minX := 0.0
	for i, line := range seg.lines {
		if len(line.Fragments) == 0 {
			continue
		}
		if x := line.Fragments[0].X; i == 0 || x < minX {
			minX = x
		}
	}
	return minX
}

// lineDominant returns a line's length-weighted dominant font size and
// whether the line is monospace-dominant.
func lineDominant(line TextLine) (float64, bool) {
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
	// Deterministic tie-break (map iteration order is random): the smaller
	// size wins on equal weight, so classification never flips between runs.
	best, bestW := 120, -1
	for s, w := range sizeW {
		if w > bestW || (w == bestW && s < best) {
			best, bestW = s, w
		}
	}
	return float64(best) / 10, totalW > 0 && monoW*2 > totalW
}

// lineMarker reports whether the line opens with a list marker: "-" for a
// bullet glyph, "1" (plus the ordinal) for "N."/"N)" numbering.
func lineMarker(line TextLine) (kind, ord string) {
	text := lineJoinedText(line)
	if m := orderedRunRe.FindStringSubmatch(text); m != nil {
		return "1", m[1]
	}
	if bulletRunRe.MatchString(text) {
		return "-", ""
	}
	return "", ""
}

// lineJoinedText joins a line's fragments, synthesizing the spaces that live
// only as horizontal gaps between fragments (styled-run boundaries carry no
// space glyph).
func lineJoinedText(line TextLine) string {
	var b strings.Builder
	prevEnd := 0.0
	for i, fr := range line.Fragments {
		if i > 0 && gapIsSpace(prevEnd, fr) {
			b.WriteString(" ")
		}
		b.WriteString(fr.Text)
		prevEnd = fr.X + fr.Width
	}
	return b.String()
}

// gapIsSpace reports whether the horizontal gap before fr reads as a space.
func gapIsSpace(prevEnd float64, fr TextFragment) bool {
	threshold := fr.FontSize * 0.15
	if threshold <= 0 {
		threshold = 1.5
	}
	return fr.X-prevEnd > threshold
}

// --- styled runs ------------------------------------------------------------------

// docRun is a maximal same-style stretch of segment text, carrying the full
// fragment look. Serializers merge adjacent runs by whatever style subset
// their format can express (mergeRuns).
type docRun struct {
	text         string
	bold, italic bool
	code         bool // monospace-dominant fragment
	link         string
	fontName     string
	fontSize     float64
	color        Color
	sub, super   bool
	br           bool // explicit line break (no other fields set)
}

// segmentRuns flattens a segment's fragments into styled runs: visual lines
// join with a space, adjacent same-look fragments merge, and inter-fragment
// gaps that read as spaces are synthesized back.
func segmentRuns(seg docSeg, links []linkArea) []docRun {
	var runs []docRun
	for li, lineRuns := range segmentLineRuns(seg, links) {
		if li > 0 && len(runs) > 0 {
			runs[len(runs)-1].text += " "
		}
		for _, r := range lineRuns {
			if n := len(runs); n > 0 && runs[n-1].sameLook(r) {
				runs[n-1].text += r.text
				continue
			}
			runs = append(runs, r)
		}
	}
	return runs
}

// segmentLineRuns builds the styled runs per visual line (serializers that
// preserve line breaks — DOCX — decide themselves how lines join).
func segmentLineRuns(seg docSeg, links []linkArea) [][]docRun {
	out := make([][]docRun, 0, len(seg.lines))
	for _, line := range seg.lines {
		var runs []docRun
		appendRun := func(r docRun) {
			if n := len(runs); n > 0 && runs[n-1].sameLook(r) {
				runs[n-1].text += r.text
				return
			}
			runs = append(runs, r)
		}
		prevEnd := 0.0
		for i, fr := range line.Fragments {
			if fr.Text == "" {
				continue
			}
			// Synthesize the space that exists only as a horizontal gap
			// between differently-styled fragments.
			if i > 0 && gapIsSpace(prevEnd, fr) && len(runs) > 0 {
				runs[len(runs)-1].text += " "
			}
			midX := fr.X + fr.Width/2
			midY := line.Y + fr.FontSize*0.35
			appendRun(docRun{
				text:     fr.Text,
				bold:     fr.Bold,
				italic:   fr.Italic,
				code:     fontFamilyClass(fr.FontName) == "mono",
				link:     linkURIAt(links, midX, midY),
				fontName: fr.FontName,
				fontSize: fr.FontSize,
				color:    fr.Color,
				sub:      fr.IsSubscript,
				super:    fr.IsSuperscript,
			})
			prevEnd = fr.X + fr.Width
		}
		out = append(out, runs)
	}
	return out
}

// lineExtent returns a visual line's horizontal span.
func lineExtent(line TextLine) (llx, urx float64) {
	if len(line.Fragments) == 0 {
		return 0, 0
	}
	llx = line.Fragments[0].X
	urx = llx
	for _, fr := range line.Fragments {
		llx = minf(llx, fr.X)
		urx = maxf(urx, fr.X+fr.Width)
	}
	return llx, urx
}

// sameLook compares the full style surface (used while building runs, so no
// information is lost; serializers coarsen afterwards via mergeRuns).
func (r docRun) sameLook(o docRun) bool {
	return r.bold == o.bold && r.italic == o.italic && r.code == o.code &&
		r.link == o.link && r.fontName == o.fontName && r.fontSize == o.fontSize &&
		r.color == o.color && r.sub == o.sub && r.super == o.super
}

// mergeRuns coalesces adjacent runs the given predicate considers
// equivalent — each serializer's view of "same style".
func mergeRuns(runs []docRun, same func(a, b docRun) bool) []docRun {
	var out []docRun
	for _, r := range runs {
		if n := len(out); n > 0 && same(out[n-1], r) {
			out[n-1].text += r.text
			continue
		}
		out = append(out, r)
	}
	return out
}

// runsPlainText concatenates the runs' text.
func runsPlainText(runs []docRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.text)
	}
	return b.String()
}

// collapseWS collapses runs of whitespace to single spaces.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
