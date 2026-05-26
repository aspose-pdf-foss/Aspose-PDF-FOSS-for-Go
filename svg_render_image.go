// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
)

// renderSVGImage emits the PDF operators to draw an embedded raster image.
func renderSVGImage(buf *bytes.Buffer, p *Page, svg *SVG, im *svgImage) {
	if !im.style.display || im.w <= 0 || im.h <= 0 || len(im.data) == 0 {
		return
	}

	resName, intrinsicW, intrinsicH, err := p.addSVGImageXObject(im.data, im.format)
	if err != nil {
		return // best-effort: skip on error
	}

	// preserveAspectRatio mapping: fit image within (im.w × im.h).
	dstW, dstH := im.w, im.h
	var renderW, renderH, alignX, alignY float64
	if im.par.align == "none" || intrinsicW <= 0 || intrinsicH <= 0 {
		renderW, renderH = dstW, dstH
	} else {
		sx := dstW / intrinsicW
		sy := dstH / intrinsicH
		var s float64
		if im.par.meetOrSlice == "slice" {
			if sx > sy {
				s = sx
			} else {
				s = sy
			}
		} else {
			// "meet" (default)
			if sx < sy {
				s = sx
			} else {
				s = sy
			}
		}
		renderW = intrinsicW * s
		renderH = intrinsicH * s
		switch {
		case strings.HasPrefix(im.par.align, "xMin"):
			alignX = 0
		case strings.HasPrefix(im.par.align, "xMax"):
			alignX = dstW - renderW
		default: // xMid
			alignX = (dstW - renderW) / 2
		}
		switch {
		case strings.HasSuffix(im.par.align, "YMin"):
			// In PDF Y-flipped space, YMin (SVG top) maps to largest Y offset
			alignY = dstH - renderH
		case strings.HasSuffix(im.par.align, "YMax"):
			// YMax (SVG bottom) maps to Y=0
			alignY = 0
		default: // YMid
			alignY = (dstH - renderH) / 2
		}
	}

	buf.WriteString("q\n")
	if im.transform != nil {
		writeCMOperator(buf, *im.transform)
	}
	applyClipPath(buf, p, svg, im.style)
	applyMask(buf, p, svg, im.style, im)
	// Place the unit-square image XObject: [renderW 0 0 renderH originX originY] cm
	fmt.Fprintf(buf, "%s 0 0 %s %s %s cm\n",
		formatFloat(renderW),
		formatFloat(renderH),
		formatFloat(im.x+alignX),
		formatFloat(im.y+alignY))
	fmt.Fprintf(buf, "%s Do\n", resName)
	buf.WriteString("Q\n")
}
