// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"html"
	"io"
	"strings"
)

// Flow (reflowable) HTML export (pdf-go-ibai) — the counterpart of Aspose
// .PDF for .NET's FixedLayout=false. Instead of absolutely-positioned page
// replicas, the document's text is re-assembled into a responsive, flowing
// article: the Paragraphs() structural extractor supplies columns and
// paragraphs in reading order, each paragraph becomes a real <p> (or
// <h1>-<h3>, inferred from its dominant font size relative to the
// document's body median), styled with the paragraph's dominant look
// (bold/italic/colour/family — the WOFF faces of the embedded fonts when
// available), and raster images are placed between paragraphs by vertical
// position as responsive <img> elements carrying the PDF's own bytes.
//
// Fixed-layout concepts do not apply here: no page divs, no raster
// backgrounds, no link overlays and no interactive form controls (all are
// position-based); DPI is unused. Tables and vector graphics are not
// reconstructed — their text flows as paragraphs, their look is dropped
// (this is the trade-off of a reflowable representation).

// writeHTMLFlow renders the selected pages as one reflowable document.
func (d *Document) writeHTMLFlow(w io.Writer, pages []*Page, sel []int, title string, opt HTMLSaveOptions) error {
	var fonts *htmlFontSet
	if !opt.NoFontEmbedding {
		fonts = newHTMLFontSet(d)
	}

	// Pass 1: the shared reconstruction core (with repeating header/footer
	// and rotated-watermark suppression); embedded-font usage registers via
	// the paragraph hook.
	doc, err := buildFlowDoc(pages, sel, flowDocOptions{
		dropFurniture: true,
		dropRotated:   true,
		images:        true,
		detectTables:  true,
		onParagraph: func(p *Page, para *MarkupParagraph) {
			if fonts != nil {
				fonts.markUsed(p, para.Lines)
			}
		},
	})
	if err != nil {
		return err
	}
	body := doc.bodySize

	sink := htmlResourceSink(opt.ResourceWriter)
	if sink != nil {
		if opt.dedupCache == nil {
			opt.dedupCache = map[[32]byte]string{}
		}
		sink = dedupResourceSink(sink, opt.dedupCache)
	}
	fontCSS := ""
	if fonts != nil {
		css, err := fonts.finish(sink)
		if err != nil {
			return err
		}
		fontCSS = css
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString(`<style>
body { margin: 0; background: #fff; }
.fl { max-width: 46em; margin: 0 auto; padding: 2.5em 1.5em;
      font-family: Arial, Helvetica, sans-serif; font-size: 12pt; line-height: 1.5;
      overflow-wrap: break-word; }
.fl p, .fl h1, .fl h2, .fl h3 { margin: 0 0 0.9em; }
.fl h1 { font-size: 1.8em; } .fl h2 { font-size: 1.45em; } .fl h3 { font-size: 1.2em; }
.fl img { max-width: 100%; height: auto; display: block; margin: 1.2em auto; }
.fl .f-serif { font-family: 'Times New Roman', Times, serif; }
.fl .f-mono  { font-family: 'Courier New', Courier, monospace; }
.fl table { border-collapse: collapse; margin: 0 0 0.9em; }
.fl td { border: 1px solid #999; padding: 0.25em 0.5em; }
`)
	b.WriteString(fontCSS)
	b.WriteString("</style>\n</head>\n<body>\n<div class=\"fl\">\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}

	for _, fp := range doc.pages {
		var pb strings.Builder
		imgSeq := 0
		for _, blk := range fp.blocks {
			if blk.table != nil {
				writeFlowTable(&pb, blk.table)
				continue
			}
			if blk.img != nil {
				imgSeq++
				if err := writeFlowImage(&pb, blk.img, sink, fp.number, imgSeq); err != nil {
					return err
				}
				continue
			}
			writeFlowParagraph(&pb, pages[fp.number-1], blk.para, body, fonts)
		}
		if _, err := io.WriteString(w, pb.String()); err != nil {
			return err
		}
	}
	_, err = io.WriteString(w, "</div>\n</body>\n</html>\n")
	return err
}

// dominantFlowStyle picks the paragraph's dominant look, weighted by text
// length: font size, bold/italic, colour, family class and font name.
func dominantFlowStyle(para *MarkupParagraph) (size float64, bold, italic bool, col Color, fontName string) {
	type key struct {
		size10 int
		bold   bool
		italic bool
		col    Color
		name   string
	}
	weights := map[key]int{}
	for _, line := range para.Lines {
		for _, fr := range line.Fragments {
			k := key{int(fr.FontSize*10 + 0.5), fr.Bold, fr.Italic, fr.Color, fr.FontName}
			weights[k] += len([]rune(fr.Text))
		}
	}
	// Accumulate first, then pick with a deterministic tie-break (a total
	// order on the key) — updating the winner mid-accumulation, or breaking
	// ties by map iteration order, flips the result between runs.
	keyLess := func(a, b key) bool {
		if a.size10 != b.size10 {
			return a.size10 < b.size10
		}
		if a.name != b.name {
			return a.name < b.name
		}
		if a.bold != b.bold {
			return b.bold
		}
		if a.italic != b.italic {
			return b.italic
		}
		ac := [4]float64{a.col.R, a.col.G, a.col.B, a.col.A}
		bc := [4]float64{b.col.R, b.col.G, b.col.B, b.col.A}
		for i := range ac {
			if ac[i] != bc[i] {
				return ac[i] < bc[i]
			}
		}
		return false
	}
	best, bestW := key{size10: 120}, -1
	for k, w := range weights {
		if w > bestW || (w == bestW && keyLess(k, best)) {
			best, bestW = k, w
		}
	}
	return float64(best.size10) / 10, best.bold, best.italic, best.col, best.name
}

// writeFlowParagraph emits one paragraph as <p> or an inferred heading.
func writeFlowParagraph(b *strings.Builder, p *Page, para *MarkupParagraph, bodySize float64, fonts *htmlFontSet) {
	size, bold, italic, col, fontName := dominantFlowStyle(para)
	ratio := size / bodySize

	tag := "p"
	text := para.Text
	if level := headingLevel(size, bodySize, len(text)); level > 0 {
		tag = fmt.Sprintf("h%d", level)
	}

	class := ""
	if fonts != nil {
		if ef := fonts.resolve(p, fontName); ef != nil {
			class = ef.id
		}
	}
	if class == "" {
		switch fontFamilyClass(fontName) {
		case "serif":
			class = "f-serif"
		case "mono":
			class = "f-mono"
		}
	}

	style := ""
	if tag == "p" {
		// Keep notable size deviations relative to the body (small print,
		// slightly enlarged lead-ins); the base size stays responsive.
		if ratio <= 0.85 || (ratio >= 1.05 && ratio < 1.14) {
			style += fmt.Sprintf(";font-size:%.2fem", ratio)
		}
		if bold {
			style += ";font-weight:bold"
		}
	}
	if italic {
		style += ";font-style:italic"
	}
	if c := htmlColor(col); c != "#000000" {
		style += ";color:" + c
	}

	attrs := ""
	if class != "" {
		attrs += ` class="` + class + `"`
	}
	if style != "" {
		attrs += ` style="` + style[1:] + `"`
	}
	fmt.Fprintf(b, "<%s%s>%s</%s>\n", tag, attrs, html.EscapeString(text), tag)
}

// writeFlowTable emits a detected table as a real <table> with
// colspan/rowspan, cell shading and styled cell runs.
func writeFlowTable(b *strings.Builder, t *AbsorbedTable) {
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
			for _, r := range mergeRuns(absorbedCellRuns(cell), epubSameStyle) {
				switch {
				case r.br:
					b.WriteString("<br/>")
				case r.text != "":
					open, close := "", ""
					if r.code {
						open, close = "<code>", "</code>"
					}
					if r.bold {
						open, close = open+"<b>", "</b>"+close
					}
					if r.italic {
						open, close = open+"<i>", "</i>"+close
					}
					b.WriteString(open + html.EscapeString(r.text) + close)
				}
			}
			b.WriteString("</td>")
			c += cell.ColSpan - 1
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}

// writeFlowImage emits one image as a responsive <img> with the PDF's own
// bytes (JPEG passes through verbatim), inlined or externalized per the
// resource sink.
func writeFlowImage(b *strings.Builder, img *Image, sink htmlResourceSink, page, seq int) error {
	mime, ext := "image/png", "png"
	if img.Format == ImageFormatJPEG {
		mime, ext = "image/jpeg", "jpg"
	}
	url, err := htmlResource(sink, fmt.Sprintf("p%d_img%d.%s", page, seq, ext), mime, img.Data)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "<img src=\"%s\" alt=\"\" loading=\"lazy\">\n", html.EscapeString(url))
	return nil
}
