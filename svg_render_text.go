// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// renderSVGText emits a PDF text block for each run in the <text> element.
// Each run is wrapped in its own BT/ET block using Tm for point-positioned text.
func renderSVGText(buf *bytes.Buffer, p *Page, svg *SVG, t *svgText) {
	if !t.style.display || len(t.runs) == 0 {
		return
	}
	buf.WriteString("q\n")
	if t.transform != nil {
		writeCMOperator(buf, *t.transform)
	}
	applyClipPath(buf, p, svg, t.style)
	for _, run := range t.runs {
		font := resolveSVGFont(p.doc, run.style)
		if font == nil {
			continue
		}
		emitSVGTextRun(buf, p, svg, run, font)
	}
	buf.WriteString("Q\n")
}

// resolveSVGFont picks the font for a text run: user resolver first,
// then built-in heuristic. Never returns nil.
func resolveSVGFont(doc *Document, style svgStyle) Font {
	if doc != nil && doc.svgFontResolver != nil {
		if f := doc.svgFontResolver(style.fontFamily, style.bold, style.italic); f != nil {
			return f
		}
	}
	return heuristicFont(style.fontFamily, style.bold, style.italic)
}

// emitSVGTextRun writes a PDF BT/ET block for a single text run.
// Uses Tm with matrix [1 0 0 -1 x y] to place text at (x, y) in SVG space
// while compensating for the outer CTM's Y-flip.
func emitSVGTextRun(buf *bytes.Buffer, p *Page, svg *SVG, run svgTextRun, font Font) {
	fontSize := run.style.fontSize
	if fontSize <= 0 {
		fontSize = 16
	}

	// Register font on page and get resource name + encoder function.
	// resolveFontForPage handles both standard Type1 fonts (WinAnsi → "(..)" literals)
	// and embedded TTF fonts (CID glyph IDs → "<xxxx>" hex literals).
	resName, widthFn, encode, _, _, err := p.resolveFontForPage(font, fontSize)
	if err != nil {
		return // best-effort: skip run on error
	}

	// Anchor-adjust x using font metrics.
	xAdj := run.x
	if run.style.anchor != svgTextAnchorStart {
		width := measureString(run.text, widthFn)
		switch run.style.anchor {
		case svgTextAnchorMiddle:
			xAdj -= width / 2
		case svgTextAnchorEnd:
			xAdj -= width
		}
	}

	buf.WriteString("BT\n")
	fmt.Fprintf(buf, "%s %s Tf\n", resName, formatFloat(fontSize))

	// Fill: gradient first (Phase 3a /Pattern cs path), then plain color.
	if name := resolveGradientFill(p, svg, run.style.fill, nil); name != "" {
		fmt.Fprintf(buf, "/Pattern cs\n%s scn\n", name)
	} else if run.style.fill != nil && run.style.fill.color != nil {
		c := run.style.fill.color
		fmt.Fprintf(buf, "%s %s %s rg\n",
			formatFloat(c.R), formatFloat(c.G), formatFloat(c.B))
	}

	// Text matrix [1 0 0 -1 x y]:
	// The outer viewBox CTM flips Y (a=scale, b=0, c=0, d=-scale, e=tx, f=ty).
	// Using d=-1 in Tm undoes that flip so text glyphs appear right-side-up.
	fmt.Fprintf(buf, "1 0 0 -1 %s %s Tm\n",
		formatFloat(xAdj), formatFloat(run.y))

	// Show text — encode() produces a complete PDF string literal.
	fmt.Fprintf(buf, "%s Tj\n", encode(run.text))
	buf.WriteString("ET\n")
}
