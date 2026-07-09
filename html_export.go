// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"image/png"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// PDF → HTML export, epic pdf-go-rfom. Two fixed-layout modes:
//
// HTMLModeFaithful (phase 1): each page is rendered fully by the built-in
// rasterizer (so the visual result is pixel-identical to RenderPNG — vectors,
// images, shadings, fonts and transparency all included) and embedded as a
// base64 PNG; a transparent text layer, positioned from
// ExtractTextWithLayout, sits on top so the HTML is selectable, copyable and
// searchable (Ctrl+F) like a real document.
//
// HTMLModeText (phase 2): the page background is rendered *without glyphs*
// (renderer.suppressText — graphics, images and text-clip effects only) and
// the text layer is visible: real colour, size, weight and a metric-matched
// generic family, width-fitted to the PDF layout with transform:scaleX plus
// letter-spacing. Text stays crisp at any zoom and is styleable; exact glyph
// shapes await WOFF font embedding (phase 3).
//
// In both modes link annotations become positioned <a> overlays (/URI actions
// → external links, /GoTo → #pageN anchors) and the output is one
// self-contained file with no external assets and no JavaScript. Mirrors the
// intent of Aspose.PDF for .NET's Document.Save(SaveFormat.Html); a
// reflowable/semantic mode is a later phase.

// HTMLMode selects how SaveHTML / WriteHTML represents page text.
type HTMLMode int

const (
	// HTMLModeFaithful (default) renders each page fully as a raster image
	// under a transparent selectable text layer — pixel-identical to
	// RenderPNG by construction.
	HTMLModeFaithful HTMLMode = iota
	// HTMLModeText renders the page background without glyphs and draws the
	// text as visible HTML spans (real colour/size/style, width-fitted to
	// the PDF layout) — crisp at any zoom, accessible, smaller output.
	HTMLModeText
	// HTMLModeNative drops the raster background entirely: page graphics
	// become one inline SVG layer per page (true-curve paths, native
	// strokes, clips, images with JPEG passthrough, blend modes), with
	// per-element raster patches only for content SVG cannot express
	// (shadings, patterns, soft masks, transparency groups). Text is the
	// same visible span layer as HTMLModeText (WOFF fonts included).
	HTMLModeNative
	// HTMLModeFlow produces reflowable HTML instead of positioned page
	// replicas: paragraphs and columns (from the Paragraphs() extractor)
	// become real <p>/<h1>-<h3> elements in reading order in a responsive
	// centered column, images flow between them. Position-based features
	// (link overlays, interactive forms) and DPI do not apply; tables and
	// vector graphics are not reconstructed — their text flows as
	// paragraphs. Mirrors Aspose.PDF for .NET's FixedLayout=false.
	HTMLModeFlow
)

// HTMLSaveOptions configures SaveHTML / WriteHTML. The zero value exports all
// pages in faithful mode at 144 DPI with the document title taken from the
// Info dictionary.
type HTMLSaveOptions struct {
	// DPI is the raster resolution of the page backgrounds (0 → 144, crisp on
	// high-density screens since pages display at their natural point size).
	DPI float64
	// Title overrides the HTML <title> (default: the document's Info title,
	// else "Document").
	Title string
	// Mode selects faithful (default) or visible-text page representation.
	Mode HTMLMode
	// Pages selects which pages to export as 1-based numbers, in the given
	// order (repeats allowed). Empty exports every page. Page anchors keep
	// their source numbers, so cross-page links stay stable in a subset.
	Pages []int
	// NoFontEmbedding disables the WOFF @font-face embedding of the
	// document's fonts in HTMLModeText (spans then always use the metric
	// substitutes + width fitting). No effect in faithful mode.
	NoFontEmbedding bool
	// InteractiveForms converts AcroForm fields into real, fillable HTML
	// controls (inputs, textareas, selects) positioned over the page, and
	// removes their widget appearances from the background render. Text,
	// checkbox, radio, combo and list fields convert; push buttons and
	// signatures keep their static look. The form can be filled in and
	// printed in a browser without JavaScript; writing values back into
	// the PDF is out of scope. HTMLModeText / HTMLModeNative only.
	InteractiveForms bool
	// ResourceWriter externalizes the heavy binary parts — page background
	// rasters, WOFF fonts, images and raster patches: it is called with a
	// unique file name and the content and returns the URL to write into
	// the HTML. nil keeps everything inlined as base64 data: URLs (the
	// default single self-contained file). Mirrors the intent of Aspose
	// .PDF for .NET's CustomResourceSavingStrategy / PartsEmbeddingMode.
	ResourceWriter func(name string, data []byte) (url string, err error)
	// ResourceDir (SaveHTML only) is the convenience form of
	// ResourceWriter: resources are written as files into this directory
	// (created on demand, relative to the output HTML) and referenced by
	// relative URL. Ignored when ResourceWriter is set. Mirrors Aspose's
	// SpecialFolderForAllImages.
	ResourceDir string
	// SplitPages (SaveHTML only) writes one HTML document per exported
	// page as <stem>_p<N>.html instead of one big file; in-document links
	// point across the files. Mirrors Aspose's SplitIntoPages.
	SplitPages bool
	// OutlineNav adds a fixed sidebar built from the document outline
	// (bookmark) tree, linking to the page anchors — collapsible with pure
	// HTML, no JavaScript. No-op for documents without outlines and in
	// HTMLModeFlow (which has no page anchors).
	OutlineNav bool

	// pageHref rewrites #pageN link targets (set internally by the
	// SplitPages save so GoTo links cross file boundaries).
	pageHref func(page int) string
}

// htmlResourceSink is the resolved ResourceWriter (nil = inline base64).
type htmlResourceSink func(name string, data []byte) (string, error)

// htmlResource returns the URL for one binary part: through the sink when
// set, else as a base64 data: URL.
func htmlResource(sink htmlResourceSink, name, mime string, data []byte) (string, error) {
	if sink == nil {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	return sink(name, data)
}

// SaveHTML writes the document as an HTML file — self-contained by
// default; with ResourceDir/ResourceWriter the heavy parts become external
// files, and with SplitPages each page becomes its own <stem>_p<N>.html.
func (d *Document) SaveHTML(outputPath string, opts ...HTMLSaveOptions) error {
	opt := HTMLSaveOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.ResourceWriter == nil && opt.ResourceDir != "" {
		opt.ResourceWriter = dirResourceWriter(filepath.Dir(outputPath), opt.ResourceDir)
	}
	if opt.SplitPages {
		return d.saveHTMLSplit(outputPath, opt)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	if err := d.WriteHTML(f, opt); err != nil {
		_ = f.Close() // best-effort; the write error takes precedence
		return err
	}
	return f.Close()
}

// dirResourceWriter returns a ResourceWriter that writes files into
// htmlDir/resDir (created on first use) and references them by the
// relative URL resDir/name.
func dirResourceWriter(htmlDir, resDir string) func(string, []byte) (string, error) {
	made := false
	target := filepath.Join(htmlDir, resDir)
	return func(name string, data []byte) (string, error) {
		if !made {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			made = true
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
			return "", err
		}
		return path.Join(filepath.ToSlash(resDir), name), nil
	}
}

// saveHTMLSplit writes one HTML file per exported page as <stem>_p<N>.html,
// rewiring in-document GoTo links to point across the files.
func (d *Document) saveHTMLSplit(outputPath string, opt HTMLSaveOptions) error {
	sel := opt.Pages
	if len(sel) == 0 {
		sel = make([]int, d.PageCount())
		for i := range sel {
			sel[i] = i + 1
		}
	}
	ext := filepath.Ext(outputPath)
	if ext == "" {
		ext = ".html"
	}
	stem := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	baseStem := filepath.Base(stem)
	opt.pageHref = func(page int) string {
		return fmt.Sprintf("%s_p%d%s#page%d", baseStem, page, ext, page)
	}
	for _, n := range sel {
		po := opt
		po.Pages = []int{n}
		po.SplitPages = false
		f, err := os.Create(fmt.Sprintf("%s_p%d%s", stem, n, ext))
		if err != nil {
			return err
		}
		if err := d.WriteHTML(f, po); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WriteHTML writes the document as a single self-contained HTML file to w:
// per page, a raster background under a text layer (transparent in
// HTMLModeFaithful, visible in HTMLModeText) and link-annotation overlays.
func (d *Document) WriteHTML(w io.Writer, opts ...HTMLSaveOptions) error {
	opt := HTMLSaveOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.SplitPages {
		// A single stream cannot hold multiple files; mirrors Aspose's
		// stream-save restriction for multi-file HTML output.
		return fmt.Errorf("WriteHTML: SplitPages requires SaveHTML (multi-file output)")
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
				return fmt.Errorf("WriteHTML: page %d out of range 1..%d", n, len(pages))
			}
		}
	}

	if opt.Mode == HTMLModeFlow {
		return d.writeHTMLFlow(w, pages, sel, title, opt)
	}

	// Pass 1: extract each exported page's text layout once — the spans need
	// it, and in text mode the font collector needs the full picture (which
	// fonts, which runes) before the <style> block is written.
	type htmlPage struct {
		page  *Page
		num   int
		lines []TextLine
	}
	hps := make([]htmlPage, 0, len(sel))
	visibleText := opt.Mode == HTMLModeText || opt.Mode == HTMLModeNative
	var fonts *htmlFontSet
	if visibleText && !opt.NoFontEmbedding {
		fonts = newHTMLFontSet(d)
	}
	for _, n := range sel {
		p := pages[n-1]
		lines, _ := p.ExtractTextWithLayout() // best-effort: no text layer on error
		if fonts != nil {
			fonts.markUsed(p, lines)
		}
		hps = append(hps, htmlPage{page: p, num: n, lines: lines})
	}
	fontCSS := ""
	if fonts != nil {
		css, err := fonts.finish(htmlResourceSink(opt.ResourceWriter))
		if err != nil {
			return err
		}
		fontCSS = css
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString(`<style>
body { background: #888; margin: 0; padding: 16px 0; }
.page { position: relative; margin: 0 auto 16px; background: #fff;
        box-shadow: 0 1px 4px rgba(0,0,0,.5); overflow: hidden; }
.page > img { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
.tl, .tv { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
.tl span, .tv span { position: absolute; white-space: pre; line-height: 1;
                     transform-origin: 0 0; }
.tl span { color: transparent; font-family: sans-serif; }
.tl span::selection { background: rgba(60,120,255,.35); }
.tl span.f-serif { font-family: serif; }
.tl span.f-mono  { font-family: monospace; }
.tv span { font-family: Arial, Helvetica, sans-serif; }
.tv span.f-serif { font-family: 'Times New Roman', Times, serif; }
.tv span.f-mono  { font-family: 'Courier New', Courier, monospace; }
.vg { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
a.lnk { position: absolute; }
.fw { position: absolute; box-sizing: border-box; margin: 0;
      font-family: Arial, Helvetica, sans-serif; font-size: 11pt; }
textarea.fw { resize: none; }
#nvt { display: none; }
.nvbtn { position: fixed; left: 12px; top: 12px; z-index: 40; width: 38px; height: 38px;
         display: flex; align-items: center; justify-content: center; cursor: pointer;
         background: #fff; border: 1px solid #e2e5ea; border-radius: 10px;
         box-shadow: 0 2px 8px rgba(15,23,42,.10); color: #475569; font-size: 17px;
         user-select: none; }
.nvbtn:hover { background: #f4f6fa; color: #1d4ed8; }
.nv { position: fixed; left: 0; top: 0; bottom: 0; width: 268px; overflow: auto;
      z-index: 30; background: #fff; border-right: 1px solid #e8eaee;
      box-shadow: 2px 0 14px rgba(15,23,42,.05); padding: 60px 12px 18px;
      box-sizing: border-box;
      font: 13.5px/1.45 system-ui, -apple-system, "Segoe UI", Arial, sans-serif; }
.nv ul { list-style: none; margin: 0; padding-left: 14px; }
.nv > ul { padding-left: 0; }
.nv li { margin: 1px 0; }
.nv a { display: block; padding: 5px 9px; border-radius: 7px; color: #334155;
        text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nv a:hover { background: #eef2ff; color: #1d4ed8; }
.nv summary { cursor: pointer; list-style: none; display: flex; align-items: center; }
.nv summary::-webkit-details-marker { display: none; }
.nv summary::before { content: "\25B8"; flex: 0 0 1em; color: #9aa4b2; font-size: 11px;
                      transition: transform .15s; }
.nv details[open] > summary::before { transform: rotate(90deg); }
.nv summary > a { flex: 1 1 auto; min-width: 0; }
.pgs { min-width: 0; }
@media (min-width: 1141px) {
  .pgs { padding-left: 268px; }
  #nvt:checked ~ .nv { display: none; }
  #nvt:checked ~ .pgs { padding-left: 0; }
}
@media (max-width: 1140.9px) {
  .nv { display: none; box-shadow: 4px 0 24px rgba(15,23,42,.18); }
  #nvt:checked ~ .nv { display: block; }
}
`)
	b.WriteString(fontCSS)
	nav := ""
	if opt.OutlineNav {
		nav = htmlOutlineNav(d, opt.pageHref)
	}
	b.WriteString("</style>\n</head>\n<body>\n")
	if nav != "" {
		// Pure-CSS toggle: the checkbox drives the sidebar visibility (open
		// by default on wide screens, an overlay flyout on narrow ones) and
		// the pages wrapper's offset — no JavaScript.
		b.WriteString("<input type=\"checkbox\" id=\"nvt\">\n")
		b.WriteString("<label class=\"nvbtn\" for=\"nvt\" title=\"Toggle bookmarks\">&#9776;</label>\n")
		b.WriteString(nav)
		b.WriteString("<div class=\"pgs\">\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}

	ctx := &htmlWriteCtx{dpi: dpi, mode: opt.Mode, fonts: fonts,
		interactive: opt.InteractiveForms && visibleText,
		res:         htmlResourceSink(opt.ResourceWriter),
		pageHref:    opt.pageHref}
	if ctx.interactive {
		// Submit/reset push buttons only work inside a <form>; one wrapper
		// spans every page (radio groups and reset then work across pages).
		if action, method, wrap := htmlFormEnvelope(d); wrap {
			ctx.wrapForm = true
			attrs := ""
			if action != "" {
				attrs = fmt.Sprintf(" action=\"%s\" method=\"%s\"", html.EscapeString(action), method)
			}
			if _, err := io.WriteString(w, "<form"+attrs+">\n"); err != nil {
				return err
			}
		}
	}
	for _, hp := range hps {
		if err := writeHTMLPage(w, hp.page, hp.num, hp.lines, ctx); err != nil {
			return err
		}
	}
	tail := "</body>\n</html>\n"
	if nav != "" {
		tail = "</div>\n" + tail
	}
	if ctx.wrapForm {
		tail = "</form>\n" + tail
	}
	_, err := io.WriteString(w, tail)
	return err
}

// htmlWriteCtx carries the per-run state of one WriteHTML invocation into
// the page writer.
type htmlWriteCtx struct {
	dpi         float64
	mode        HTMLMode
	fonts       *htmlFontSet // embedded-font set (visible-text modes; nil otherwise)
	interactive bool         // InteractiveForms in a visible-text mode
	wrapForm    bool         // pages are wrapped in a document-level <form>
	dlSeq       int          // <datalist> id counter
	tabBase     int          // running tabindex offset across pages
	res         htmlResourceSink
	pageHref    func(int) string // #pageN rewriting for split output (nil = same file)
}

// writeHTMLPage emits one .page div: the rendered background image, the text
// layer (transparent or visible per mode), the link overlays and (when
// interactive) the form controls.
func writeHTMLPage(w io.Writer, p *Page, num int, lines []TextLine, ctx *htmlWriteCtx) error {
	sz, err := p.Size()
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<div class=\"page\" id=\"page%d\" style=\"width:%spt;height:%spt\">\n",
		num, htmlNum(sz.Width), htmlNum(sz.Height))

	if ctx.mode == HTMLModeNative {
		// No raster background: the page graphics are one inline SVG layer.
		svg, err := renderPageSVG(p, ctx.dpi, ctx.interactive, ctx.res, num)
		if err != nil {
			return err
		}
		b.WriteString(svg)
	} else {
		// Background raster: the full page in faithful mode, the glyph-less
		// graphics in text mode — inlined or externalized per ResourceWriter.
		img, err := p.renderImage(RenderOptions{DPI: ctx.dpi}, ctx.mode == HTMLModeText, ctx.interactive)
		if err != nil {
			return err
		}
		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, img); err != nil {
			return err
		}
		url, err := htmlResource(ctx.res, fmt.Sprintf("page%d.png", num), "image/png", pngBuf.Bytes())
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"page %d\" loading=\"lazy\">\n",
			html.EscapeString(url), num)
	}

	visibleText := ctx.mode == HTMLModeText || ctx.mode == HTMLModeNative
	layer := "tl"
	if visibleText {
		layer = "tv"
	}
	fmt.Fprintf(&b, "<div class=\"%s\">\n", layer)

	for _, line := range lines {
		for _, frag := range line.Fragments {
			if visibleText {
				var ef *htmlFont
				if ctx.fonts != nil {
					ef = ctx.fonts.resolve(p, frag.FontName)
				}
				writeHTMLVisibleFragment(&b, frag, sz.Height, ef)
			} else {
				writeHTMLFragment(&b, frag, sz.Height)
			}
		}
	}
	b.WriteString("</div>\n")
	writeHTMLLinks(&b, p, sz.Height, ctx.pageHref)
	if ctx.interactive {
		writeHTMLFormFields(&b, p, sz.Height, ctx)
	}
	b.WriteString("</div>\n")
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

// writeHTMLVisibleFragment emits one visible text span for HTMLModeText:
// real colour, size and style. With an embedded font (ef non-nil) the span
// references the WOFF face — advances then match the PDF by construction,
// so only the PDF's own character spacing (Tc) is mapped to letter-spacing.
// Without one, the substitute face is width-fitted to the PDF layout.
func writeHTMLVisibleFragment(b *strings.Builder, frag TextFragment, pageH float64, ef *htmlFont) {
	text := frag.Text
	if strings.TrimSpace(text) == "" {
		return // pure whitespace paints nothing
	}
	size := frag.FontSize
	if size <= 0 {
		size = 12
	}
	top := pageH - frag.Y - size*0.8
	family := fontFamilyClass(frag.FontName)
	class := ""
	switch {
	case ef != nil:
		class = " class=\"" + ef.id + "\""
	case family == "serif":
		class = " class=\"f-serif\""
	case family == "mono":
		class = " class=\"f-mono\""
	}
	style := fmt.Sprintf("left:%spt;top:%spt;font-size:%spt", htmlNum(frag.X), htmlNum(top), htmlNum(size))
	if frag.Bold {
		style += ";font-weight:bold"
	}
	if frag.Italic {
		style += ";font-style:italic"
	}
	if c := htmlColor(frag.Color); c != "#000000" {
		style += ";color:" + c
	}
	if ef != nil {
		if frag.CharSpacing > 0.01 || frag.CharSpacing < -0.01 {
			style += ";letter-spacing:" + htmlNum(frag.CharSpacing) + "pt"
		}
	} else {
		scale, spacing := htmlWidthFit(text, family, frag, size)
		if spacing != 0 {
			style += ";letter-spacing:" + htmlNum(spacing) + "pt"
		}
		if scale != 1 {
			style += fmt.Sprintf(";transform:scaleX(%.4f)", scale)
		}
	}
	fmt.Fprintf(b, "<span%s style=\"%s\">%s</span>\n", class, style, html.EscapeString(text))
}

// htmlWidthFit computes the scaleX factor and per-character letter-spacing
// (pt) that make the fragment's browser rendering span frag.Width. The
// natural browser width is estimated with the Standard-14 metrics of the
// substitute family — the same advances as the browser's default faces
// (Arial ≈ Helvetica, Times New Roman ≈ Times, Courier New ≈ Courier). A
// small mismatch is absorbed by letter-spacing alone (no glyph distortion);
// a large one by scaleX, with letter-spacing taking any clamped residual.
func htmlWidthFit(text, family string, frag TextFragment, size float64) (scale, spacing float64) {
	scale = 1
	if frag.Width <= 0 {
		return
	}
	widthFn, _, err := fontWidthAndAscent(substituteFontFor(family, frag.Bold, frag.Italic), size)
	if err != nil {
		return
	}
	natural := measureString(text, widthFn)
	if natural <= 0 {
		return
	}
	runes := float64(utf8.RuneCountInString(text))
	ratio := frag.Width / natural
	if math.Abs(ratio-1) < 0.005 {
		return // visually exact already; skip the no-op transform
	}
	if runes > 1 && ratio >= 0.95 && ratio <= 1.05 {
		// Browsers add letter-spacing after every character, so the divisor
		// is the rune count, not the gap count.
		return 1, (frag.Width - natural) / runes
	}
	scale = ratio
	const lo, hi = 0.5, 2.0
	if scale < lo {
		scale = lo
	} else if scale > hi {
		scale = hi
	}
	if runes > 1 && scale != ratio {
		// Clamped: letter-spacing (pre-transform space) covers the rest.
		spacing = (frag.Width/scale - natural) / runes
	}
	return
}

// substituteFontFor maps a generic family class + style onto the Standard-14
// face whose metrics match the browser's default font for that family.
func substituteFontFor(family string, bold, italic bool) Font {
	switch family {
	case "serif":
		switch {
		case bold && italic:
			return FontTimesBoldItalic
		case bold:
			return FontTimesBold
		case italic:
			return FontTimesItalic
		}
		return FontTimesRoman
	case "mono":
		switch {
		case bold && italic:
			return FontCourierBoldOblique
		case bold:
			return FontCourierBold
		case italic:
			return FontCourierOblique
		}
		return FontCourier
	}
	switch {
	case bold && italic:
		return FontHelveticaBoldOblique
	case bold:
		return FontHelveticaBold
	case italic:
		return FontHelveticaOblique
	}
	return FontHelvetica
}

// htmlColor formats a Color as a CSS hex colour (alpha ignored — extracted
// text colour is always opaque).
func htmlColor(c Color) string {
	to255 := func(v float64) int {
		if v <= 0 {
			return 0
		}
		if v >= 1 {
			return 255
		}
		return int(v*255 + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", to255(c.R), to255(c.G), to255(c.B))
}

// writeHTMLLinks emits one positioned <a> per link annotation the export can
// resolve: /URI actions become external links, /GoTo actions and page-ref
// /Dest arrays become #pageN anchors (rewritten across files by pageHref in
// split output). Unresolvable links are skipped.
func writeHTMLLinks(b *strings.Builder, p *Page, pageH float64, pageHref func(int) string) {
	if pageHref == nil {
		pageHref = func(n int) string { return fmt.Sprintf("#page%d", n) }
	}
	for _, a := range p.Annotations().All() {
		link, ok := a.(*LinkAnnotation)
		if !ok {
			continue
		}
		href := ""
		switch act := link.Action().(type) {
		case *GoToURIAction:
			href = act.URI()
		case *GoToAction:
			if act.PageNum() >= 1 {
				href = pageHref(act.PageNum())
			}
		}
		if href == "" {
			if n := linkDestPage(link); n >= 1 {
				href = pageHref(n)
			}
		}
		if href == "" {
			continue
		}
		r := link.Rect()
		if r.URX <= r.LLX || r.URY <= r.LLY {
			continue
		}
		fmt.Fprintf(b, "<a class=\"lnk\" href=\"%s\" style=\"left:%spt;top:%spt;width:%spt;height:%spt\"></a>\n",
			html.EscapeString(href), htmlNum(r.LLX), htmlNum(pageH-r.URY),
			htmlNum(r.URX-r.LLX), htmlNum(r.URY-r.LLY))
	}
}

// linkDestPage resolves a link's direct /Dest destination array to a 1-based
// page number (0 when absent or not a page-ref array; named destinations are
// not chased).
func linkDestPage(link *LinkAnnotation) int {
	dest, ok := resolveRef(link.doc.objects, link.dict["/Dest"]).(pdfArray)
	if !ok || len(dest) == 0 {
		return 0
	}
	ref, ok := dest[0].(pdfRef)
	if !ok {
		return 0
	}
	for i, p := range link.doc.pages {
		if p.Num == ref.Num {
			return i + 1
		}
	}
	return 0
}

// fontFamilyClass maps a PDF base-font name onto a generic CSS family for the
// text layer, so span geometry roughly tracks the glyphs.
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
