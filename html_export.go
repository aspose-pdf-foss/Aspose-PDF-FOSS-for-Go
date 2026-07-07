// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
)

// PDF → HTML export, phase 1 of epic pdf-go-rfom: the "faithful" fixed-layout
// mode. Each page is rendered fully by the built-in rasterizer (so the visual
// result is pixel-identical to RenderPNG — vectors, images, shadings, fonts and
// transparency all included) and embedded as a base64 PNG; a transparent text
// layer, positioned from ExtractTextWithLayout, sits on top so the HTML is
// selectable, copyable and searchable (Ctrl+F) like a real document. The
// output is one self-contained file with no external assets and no JavaScript.
// Mirrors the intent of Aspose.PDF for .NET's Document.Save(SaveFormat.Html);
// a reflowable/semantic mode is a later phase.

// HTMLSaveOptions configures SaveHTML / WriteHTML. The zero value renders at
// 144 DPI with the document title taken from the Info dictionary.
type HTMLSaveOptions struct {
	// DPI is the raster resolution of the page backgrounds (0 → 144, crisp on
	// high-density screens since pages display at their natural point size).
	DPI float64
	// Title overrides the HTML <title> (default: the document's Info title,
	// else "Document").
	Title string
}

// SaveHTML writes the document as a single self-contained HTML file.
func (d *Document) SaveHTML(outputPath string, opts ...HTMLSaveOptions) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	if err := d.WriteHTML(f, opts...); err != nil {
		_ = f.Close() // best-effort; the write error takes precedence
		return err
	}
	return f.Close()
}

// WriteHTML writes the document as a single self-contained HTML file to w:
// per page, a full raster of the page (base64 PNG) under a transparent,
// selectable text layer.
func (d *Document) WriteHTML(w io.Writer, opts ...HTMLSaveOptions) error {
	opt := HTMLSaveOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	dpi := opt.DPI
	if dpi <= 0 {
		dpi = 144
	}
	title := opt.Title
	if title == "" {
		if info, err := d.Info(); err == nil && info.Title != "" {
			title = info.Title
		} else {
			title = "Document"
		}
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString(`<style>
body { background: #888; margin: 0; padding: 16px 0; }
.page { position: relative; margin: 0 auto 16px; background: #fff;
        box-shadow: 0 1px 4px rgba(0,0,0,.5); overflow: hidden; }
.page > img { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
.tl { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
.tl span { position: absolute; color: transparent; white-space: pre;
           line-height: 1; transform-origin: 0 0;
           font-family: sans-serif; }
.tl span::selection { background: rgba(60,120,255,.35); }
.tl span.f-serif { font-family: serif; }
.tl span.f-mono  { font-family: monospace; }
</style>
</head>
<body>
`)
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}

	for i, p := range d.Pages() {
		if err := writeHTMLPage(w, p, i+1, dpi); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</body>\n</html>\n")
	return err
}

// writeHTMLPage emits one .page div: the rendered background image and the
// transparent text layer.
func writeHTMLPage(w io.Writer, p *Page, num int, dpi float64) error {
	sz, err := p.Size()
	if err != nil {
		return err
	}

	// Background: the full page render, base64-inlined.
	var png strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &png)
	if err := p.RenderPNG(enc, RenderOptions{DPI: dpi}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<div class=\"page\" id=\"page%d\" style=\"width:%spt;height:%spt\">\n",
		num, htmlNum(sz.Width), htmlNum(sz.Height))
	fmt.Fprintf(&b, "<img src=\"data:image/png;base64,%s\" alt=\"page %d\">\n", png.String(), num)
	b.WriteString("<div class=\"tl\">\n")

	lines, err := p.ExtractTextWithLayout()
	if err == nil {
		for _, line := range lines {
			for _, frag := range line.Fragments {
				writeHTMLFragment(&b, frag, sz.Height)
			}
		}
	}
	b.WriteString("</div>\n</div>\n")
	_, err = io.WriteString(w, b.String())
	return err
}

// writeHTMLFragment emits one transparent text span positioned over the
// rendered glyphs. Fragment Y is the baseline from the page bottom; CSS top is
// measured from the page top to the span's top, approximated as baseline minus
// ~80% of the font size (the typical ascent).
func writeHTMLFragment(b *strings.Builder, frag TextFragment, pageH float64) {
	text := strings.TrimRight(frag.Text, " ")
	if text == "" {
		return
	}
	size := frag.FontSize
	if size <= 0 {
		size = 12
	}
	top := pageH - frag.Y - size*0.8
	class := ""
	switch fontFamilyClass(frag.FontName) {
	case "serif":
		class = " class=\"f-serif\""
	case "mono":
		class = " class=\"f-mono\""
	}
	style := fmt.Sprintf("left:%spt;top:%spt;font-size:%spt", htmlNum(frag.X), htmlNum(top), htmlNum(size))
	if frag.Bold {
		style += ";font-weight:bold"
	}
	if frag.Italic {
		style += ";font-style:italic"
	}
	fmt.Fprintf(b, "<span%s style=\"%s\">%s</span>\n", class, style, html.EscapeString(text))
}

// fontFamilyClass maps a PDF base-font name onto a generic CSS family for the
// (invisible) text layer, so selection geometry roughly tracks the glyphs.
func fontFamilyClass(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "courier"), strings.Contains(n, "mono"), strings.Contains(n, "consol"):
		return "mono"
	case strings.Contains(n, "times"), strings.Contains(n, "serif") && !strings.Contains(n, "sans"),
		strings.Contains(n, "georgia"), strings.Contains(n, "garamond"), strings.Contains(n, "book"):
		return "serif"
	}
	return "sans"
}

// htmlNum formats a CSS length number compactly (two decimals, trimmed).
func htmlNum(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}
