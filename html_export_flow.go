// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"sort"
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

// flowBlock is one emitted unit — a paragraph or an image — ordered by its
// visual top within the page.
type flowBlock struct {
	para *MarkupParagraph
	img  *Image
	top  float64
}

// writeHTMLFlow renders the selected pages as one reflowable document.
func (d *Document) writeHTMLFlow(w io.Writer, pages []*Page, sel []int, title string, opt HTMLSaveOptions) error {
	var fonts *htmlFontSet
	if !opt.NoFontEmbedding {
		fonts = newHTMLFontSet(d)
	}

	// Pass 1: extract structure (and images) per page, register font usage,
	// and gather the length-weighted font sizes for the body median.
	type flowPage struct{ blocks []flowBlock }
	fps := make([]flowPage, 0, len(sel))
	var sizes []struct {
		size   float64
		weight int
	}
	for _, n := range sel {
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
				blocks = append(blocks, flowBlock{para: para, top: para.Rectangle.URY})
				if fonts != nil {
					fonts.markUsed(p, para.Lines)
				}
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
		if imgs, err := p.ExtractImages(); err == nil {
			for i := range imgs {
				img := &imgs[i]
				if len(img.Data) == 0 {
					continue
				}
				insertFlowImage(&blocks, flowBlock{img: img, top: img.Y + img.PageHeight})
			}
		}
		fps = append(fps, flowPage{blocks: blocks})
	}
	body := weightedMedianSize(sizes)

	fontCSS := ""
	if fonts != nil {
		fontCSS = fonts.finish()
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
`)
	b.WriteString(fontCSS)
	b.WriteString("</style>\n</head>\n<body>\n<div class=\"fl\">\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}

	for i, fp := range fps {
		var pb strings.Builder
		for _, blk := range fp.blocks {
			if blk.img != nil {
				writeFlowImage(&pb, blk.img)
				continue
			}
			writeFlowParagraph(&pb, pages[sel[i]-1], blk.para, body, fonts)
		}
		if _, err := io.WriteString(w, pb.String()); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</div>\n</body>\n</html>\n")
	return err
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
func weightedMedianSize(sizes []struct {
	size   float64
	weight int
}) float64 {
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
	best, bestW := key{size10: 120}, -1
	for _, line := range para.Lines {
		for _, fr := range line.Fragments {
			k := key{int(fr.FontSize*10 + 0.5), fr.Bold, fr.Italic, fr.Color, fr.FontName}
			weights[k] += len([]rune(fr.Text))
			if weights[k] > bestW {
				best, bestW = k, weights[k]
			}
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
	switch {
	case ratio >= 1.7 && len(text) < 200:
		tag = "h1"
	case ratio >= 1.35 && len(text) < 200:
		tag = "h2"
	case ratio >= 1.14 && len(text) < 200:
		tag = "h3"
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

// writeFlowImage emits one image as a responsive <img> with the PDF's own
// bytes (JPEG passes through verbatim).
func writeFlowImage(b *strings.Builder, img *Image) {
	mime := "image/png"
	if img.Format == ImageFormatJPEG {
		mime = "image/jpeg"
	}
	fmt.Fprintf(b, "<img src=\"data:%s;base64,%s\" alt=\"\" loading=\"lazy\">\n",
		mime, base64.StdEncoding.EncodeToString(img.Data))
}
