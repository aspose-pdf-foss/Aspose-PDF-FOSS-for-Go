// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
)

// SVG page backend for the HTML exporter (epic pdf-go-rfom, phase 4: the
// no-background native mode). An svgDevice rides the content-stream renderer:
// the renderer keeps doing everything it already does — graphics-state
// tracking, colour resolution, clip rasterization, form recursion, optional
// content — but paint operations are redirected here and emitted as SVG
// elements instead of rasterized:
//
//   - path fills and strokes → <path> with true curves (the exec path
//     operators feed a parallel device-space recorder, so Béziers survive)
//     and native stroke attributes (width, caps, joins, dash);
//   - images → <image> with the PDF's own PNG/JPEG bytes (JPEG passes
//     through verbatim) placed by the CTM;
//   - W/W* and Tr 4-7 clips → chained <clipPath> definitions;
//   - constant alpha → fill-/stroke-opacity; blend modes → mix-blend-mode;
//   - anything without an SVG equivalent (shadings, tiling patterns, soft
//     masks, transparency-group compositing, stencil masks) degrades locally:
//     vecPatch renders just that operation through the ordinary raster
//     pipeline into a transparent page-sized buffer, crops it to its pixel
//     bounding box and emits it as a positioned PNG <image> — the rest of
//     the page stays vector.
//
// Coordinates are device pixels at the export DPI (the same base matrix as
// the rasterizer, Y already flipped), so the SVG viewBox spans the page and
// patches align with vector content by construction. Glyphs that reach the
// renderer (annotation appearances; page text is suppressed in favour of the
// HTML text layer) arrive through compositePath as flattened outlines.

// svgDevice accumulates the SVG markup for one page.
type svgDevice struct {
	els     strings.Builder // painted elements, in paint order
	defs    strings.Builder // <clipPath> definitions
	path    strings.Builder // current path data (true curves, device space)
	clipSeq int
}

// --- path recording (parallel to the flattener) ---------------------------

func (d *svgDevice) moveTo(x, y float64) {
	fmt.Fprintf(&d.path, "M%s %s", htmlNum(x), htmlNum(y))
}

func (d *svgDevice) lineTo(x, y float64) {
	fmt.Fprintf(&d.path, "L%s %s", htmlNum(x), htmlNum(y))
}

func (d *svgDevice) cubicTo(x1, y1, x2, y2, x3, y3 float64) {
	fmt.Fprintf(&d.path, "C%s %s %s %s %s %s",
		htmlNum(x1), htmlNum(y1), htmlNum(x2), htmlNum(y2), htmlNum(x3), htmlNum(y3))
}

func (d *svgDevice) closePath() { d.path.WriteByte('Z') }

func (d *svgDevice) pathData() string { return d.path.String() }

func (d *svgDevice) resetPath() { d.path.Reset() }

// devPathData converts a flattened device path (polyline subpaths) to SVG
// path data — used for glyph outlines and text clips.
func devPathData(dp *devPath) string {
	var b strings.Builder
	for _, sp := range dp.subs {
		if len(sp.pts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "M%s %s", htmlNum(sp.pts[0].x), htmlNum(sp.pts[0].y))
		for _, pt := range sp.pts[1:] {
			fmt.Fprintf(&b, "L%s %s", htmlNum(pt.x), htmlNum(pt.y))
		}
		if sp.closed {
			b.WriteByte('Z')
		}
	}
	return b.String()
}

// --- shared attributes -----------------------------------------------------

func svgColor(r, g, b uint8) string { return fmt.Sprintf("#%02x%02x%02x", r, g, b) }

// commonAttrs renders the clip and blend-mode attributes of the current
// graphics state.
func commonAttrs(rd *renderer) string {
	s := ""
	if rd.gs.vecClip > 0 {
		s += fmt.Sprintf(` clip-path="url(#c%d)"`, rd.gs.vecClip)
	}
	if rd.gs.blend.css != "" {
		s += ` style="mix-blend-mode:` + rd.gs.blend.css + `"`
	}
	return s
}

// --- element emission ------------------------------------------------------

// emitFill paints the recorder's current path with the fill colour.
func (d *svgDevice) emitFill(rd *renderer, rule fillRule) {
	pd := d.pathData()
	if pd == "" || rd.gs.fillA <= 0 {
		return
	}
	attrs := ` fill="` + svgColor(rd.gs.fillR, rd.gs.fillG, rd.gs.fillB) + `"`
	if rule == fillEvenOdd {
		attrs += ` fill-rule="evenodd"`
	}
	if rd.gs.fillA < 1 {
		attrs += fmt.Sprintf(` fill-opacity="%.3f"`, rd.gs.fillA)
	}
	fmt.Fprintf(&d.els, "<path d=\"%s\"%s%s/>\n", pd, attrs, commonAttrs(rd))
}

// emitStroke paints the recorder's current path outline with native SVG
// stroke attributes; geometry parameters are scaled to device pixels exactly
// like the raster stroke path.
func (d *svgDevice) emitStroke(rd *renderer) {
	pd := d.pathData()
	if pd == "" || rd.gs.strokeA <= 0 {
		return
	}
	m := rd.dmat()
	scale := devScale(m)
	dw := rd.gs.lineWidth * scale
	if dw < 1 {
		dw = 1
	}
	attrs := ` fill="none" stroke="` + svgColor(rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB) + `"`
	attrs += ` stroke-width="` + htmlNum(dw) + `"`
	switch rd.gs.lineCap {
	case LineCapRound:
		attrs += ` stroke-linecap="round"`
	case LineCapSquare:
		attrs += ` stroke-linecap="square"`
	}
	switch rd.gs.lineJoin {
	case LineJoinRound:
		attrs += ` stroke-linejoin="round"`
	case LineJoinBevel:
		attrs += ` stroke-linejoin="bevel"`
	}
	if rd.gs.miterLimit > 0 && rd.gs.miterLimit != 10 { // 10 is the SVG default too... but PDF's is 10 as well
		attrs += ` stroke-miterlimit="` + htmlNum(rd.gs.miterLimit) + `"`
	}
	if len(rd.gs.dash) > 0 {
		parts := make([]string, len(rd.gs.dash))
		for i, v := range rd.gs.dash {
			parts[i] = htmlNum(v * scale)
		}
		attrs += ` stroke-dasharray="` + strings.Join(parts, " ") + `"`
		if rd.gs.dashPhase != 0 {
			attrs += ` stroke-dashoffset="` + htmlNum(rd.gs.dashPhase*scale) + `"`
		}
	}
	if rd.gs.strokeA < 1 {
		attrs += fmt.Sprintf(` stroke-opacity="%.3f"`, rd.gs.strokeA)
	}
	fmt.Fprintf(&d.els, "<path d=\"%s\"%s%s/>\n", pd, attrs, commonAttrs(rd))
}

// emitDevPath paints an already-flattened device path (glyph outlines and
// other paints arriving through compositePath).
func (d *svgDevice) emitDevPath(rd *renderer, dp *devPath, rule fillRule, r, g, b uint8, alpha float64) {
	pd := devPathData(dp)
	if pd == "" || alpha <= 0 {
		return
	}
	attrs := ` fill="` + svgColor(r, g, b) + `"`
	if rule == fillEvenOdd {
		attrs += ` fill-rule="evenodd"`
	}
	if alpha < 1 {
		attrs += fmt.Sprintf(` fill-opacity="%.3f"`, alpha)
	}
	fmt.Fprintf(&d.els, "<path d=\"%s\"%s%s/>\n", pd, attrs, commonAttrs(rd))
}

// emitImage places an extracted image (PNG or JPEG bytes, passed through
// verbatim) into the unit square mapped by the current CTM, flipping the
// image's top-down rows into PDF's bottom-up image space.
func (d *svgDevice) emitImage(rd *renderer, img *Image) {
	if len(img.Data) == 0 {
		return
	}
	mime := "image/png"
	if img.Format == ImageFormatJPEG {
		mime = "image/jpeg"
	}
	m := rd.dmat()
	attrs := ""
	if rd.gs.fillA < 1 { // /ca applies to image XObjects (non-stroking)
		attrs += fmt.Sprintf(` opacity="%.3f"`, rd.gs.fillA)
	}
	fmt.Fprintf(&d.els,
		"<image x=\"0\" y=\"0\" width=\"1\" height=\"1\" preserveAspectRatio=\"none\" transform=\"matrix(%s %s %s %s %s %s) translate(0 1) scale(1 -1)\"%s%s href=\"data:%s;base64,%s\"/>\n",
		htmlNum(m[0]), htmlNum(m[1]), htmlNum(m[2]), htmlNum(m[3]), htmlNum(m[4]), htmlNum(m[5]),
		attrs, commonAttrs(rd), mime, base64.StdEncoding.EncodeToString(img.Data))
}

// addClip registers a new <clipPath> chained onto parent (0 = none) and
// returns its id.
func (d *svgDevice) addClip(pathData string, rule fillRule, parent int) int {
	if pathData == "" {
		return parent
	}
	d.clipSeq++
	id := d.clipSeq
	chain := ""
	if parent > 0 {
		chain = fmt.Sprintf(` clip-path="url(#c%d)"`, parent)
	}
	ruleAttr := ""
	if rule == fillEvenOdd {
		ruleAttr = ` clip-rule="evenodd"`
	}
	fmt.Fprintf(&d.defs, "<clipPath id=\"c%d\"%s><path d=\"%s\"%s/></clipPath>\n", id, chain, pathData, ruleAttr)
	return id
}

// emitPatch crops a raster patch buffer to its populated bounding box and
// places it as a positioned PNG. Alpha, blend, clip and soft mask were all
// applied during the raster pass, so no attributes are repeated here.
func (d *svgDevice) emitPatch(buf *image.RGBA) {
	x0, y0, x1, y1 := rgbaAlphaBounds(buf)
	if x0 >= x1 || y0 >= y1 {
		return // nothing painted
	}
	crop := buf.SubImage(image.Rect(x0, y0, x1, y1))
	var b strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &b)
	if err := png.Encode(enc, crop); err != nil {
		return
	}
	if err := enc.Close(); err != nil {
		return
	}
	fmt.Fprintf(&d.els, "<image x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" href=\"data:image/png;base64,%s\"/>\n",
		x0, y0, x1-x0, y1-y0, b.String())
}

// rgbaAlphaBounds returns the tight bounding box of pixels with non-zero
// alpha (x1/y1 exclusive; empty box when fully transparent).
func rgbaAlphaBounds(img *image.RGBA) (x0, y0, x1, y1 int) {
	b := img.Bounds()
	x0, y0, x1, y1 = b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+b.Max.X*4]
		for x := b.Min.X; x < b.Max.X; x++ {
			if row[x*4+3] != 0 {
				if x < x0 {
					x0 = x
				}
				if x >= x1 {
					x1 = x + 1
				}
				if y < y0 {
					y0 = y
				}
				if y >= y1 {
					y1 = y + 1
				}
			}
		}
	}
	return
}

// svg assembles the final markup for a page of w×h device pixels.
func (d *svgDevice) svg(w, h int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<svg class=\"vg\" viewBox=\"0 0 %d %d\" xmlns=\"http://www.w3.org/2000/svg\">\n", w, h)
	if d.defs.Len() > 0 {
		b.WriteString("<defs>\n")
		b.WriteString(d.defs.String())
		b.WriteString("</defs>\n")
	}
	b.WriteString(d.els.String())
	b.WriteString("</svg>\n")
	return b.String()
}

// devScale is the isotropic device scale of an affine matrix (the same
// formula the raster stroke path uses for line widths).
func devScale(m [6]float64) float64 {
	return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
}

// --- raster patches ---------------------------------------------------------

// vecPatch renders one paint operation through the ordinary raster pipeline —
// a vec-less sub-renderer inheriting the full graphics state (clip, soft
// mask, alpha, blend) over a transparent page-sized buffer — and emits the
// populated region as a positioned PNG. This is the local-degradation path
// for content SVG cannot express.
func (rd *renderer) vecPatch(paint func(*renderer)) {
	if rd.vec == nil || rd.w <= 0 || rd.h <= 0 {
		return
	}
	buf := image.NewRGBA(image.Rect(0, 0, rd.w, rd.h))
	sub := newRenderer(rd.page, buf, rd.w, rd.h, rd.base)
	sub.gs = rd.gs
	sub.gs.vecClip = 0
	sub.res = rd.res
	sub.ts = rd.ts
	sub.depth = rd.depth
	sub.suppressText = rd.suppressText
	sub.fontCache = rd.fontCache
	sub.ocOff = rd.ocOff
	paint(sub)
	rd.vec.emitPatch(buf)
}

// renderPageSVG interprets the page content with an SVG backend attached and
// returns the page's <svg> markup. Geometry (CropBox∩MediaBox region, DPI
// scale, rotation) mirrors RenderImage, so the result is drop-in aligned
// with the text layer. Page text is suppressed (the HTML text layer carries
// it); annotation-appearance text is emitted as outline paths.
func renderPageSVG(p *Page, dpi float64, hideFormWidgets bool) (string, error) {
	box, err := p.CropBox()
	if err != nil {
		return "", fmt.Errorf("render svg: %w", err)
	}
	if mb, mbErr := p.MediaBox(); mbErr == nil {
		if ix := intersectRects(box, mb); ix.URX > ix.LLX && ix.URY > ix.LLY {
			box = ix
		} else {
			box = mb
		}
	}
	scale := dpi / 72.0
	base, w, h := deviceMatrix(box, scale, p.Rotation())
	if w <= 0 || h <= 0 {
		return "", fmt.Errorf("render svg: degenerate page size %dx%d", w, h)
	}
	rd := newRenderer(p, image.NewRGBA(image.Rect(0, 0, w, h)), w, h, base)
	rd.vec = &svgDevice{}
	rd.suppressText = true
	rd.hideFormWidgets = hideFormWidgets
	rd.run()
	return rd.vec.svg(w, h), nil
}
