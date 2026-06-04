// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"math"
)

// gstate is the subset of the PDF graphics state the renderer tracks.
type gstate struct {
	ctm [6]float64

	fillR, fillG, fillB       uint8
	strokeR, strokeG, strokeB uint8
	fillA, strokeA            float64
	fillPattern               bool // fill colour is a pattern we can't render yet → skip fills
	strokePattern             bool

	lineWidth float64
	clip      []float32 // nil = unclipped
}

// renderer interprets a page's content stream and paints onto img.
type renderer struct {
	page *Page
	img  *image.RGBA
	w, h int
	base [6]float64 // user space → device pixels
	ras  *rasterizer

	gs    gstate
	stack []gstate
	fl    *flattener // current path, accumulated in device space
	res   pdfDict    // current /Resources (page, or a form XObject's)
	depth int        // form XObject recursion depth
}

func newRenderer(p *Page, img *image.RGBA, w, h int, base [6]float64) *renderer {
	return &renderer{
		page: p,
		img:  img,
		w:    w,
		h:    h,
		base: base,
		ras:  newRasterizer(w, h),
		gs: gstate{
			ctm:       identityMatrix(),
			fillA:     1,
			strokeA:   1,
			lineWidth: 1,
		},
		fl:  newFlattener(0.2),
		res: p.pageResources(),
	}
}

// run parses and executes the page content. Parse failures and unsupported
// operators are tolerated: the page renders what it can rather than erroring.
func (rd *renderer) run() {
	data, err := rd.page.contentStreams()
	if err != nil || len(data) == 0 {
		return
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return
	}
	rd.exec(ops)
}

// dmat returns the current user-space → device matrix.
func (rd *renderer) dmat() [6]float64 { return matMul(rd.gs.ctm, rd.base) }

func (rd *renderer) exec(ops []contentOp) {
	for _, op := range ops {
		o := op.Operands
		switch op.Operator {
		// --- graphics state ---
		case "q":
			rd.stack = append(rd.stack, rd.gs)
		case "Q":
			if n := len(rd.stack); n > 0 {
				rd.gs = rd.stack[n-1]
				rd.stack = rd.stack[:n-1]
			}
		case "cm":
			if len(o) >= 6 {
				cm := [6]float64{f(o[0]), f(o[1]), f(o[2]), f(o[3]), f(o[4]), f(o[5])}
				rd.gs.ctm = matMul(cm, rd.gs.ctm)
			}
		case "w":
			if len(o) >= 1 {
				rd.gs.lineWidth = f(o[0])
			}

		// --- path construction (transformed to device space immediately) ---
		case "m":
			if len(o) >= 2 {
				x, y := applyPt(rd.dmat(), f(o[0]), f(o[1]))
				rd.fl.moveTo(x, y)
			}
		case "l":
			if len(o) >= 2 {
				x, y := applyPt(rd.dmat(), f(o[0]), f(o[1]))
				rd.fl.lineTo(x, y)
			}
		case "c":
			if len(o) >= 6 {
				m := rd.dmat()
				x1, y1 := applyPt(m, f(o[0]), f(o[1]))
				x2, y2 := applyPt(m, f(o[2]), f(o[3]))
				x3, y3 := applyPt(m, f(o[4]), f(o[5]))
				rd.fl.cubicTo(x1, y1, x2, y2, x3, y3)
			}
		case "v":
			if len(o) >= 4 {
				m := rd.dmat()
				x2, y2 := applyPt(m, f(o[0]), f(o[1]))
				x3, y3 := applyPt(m, f(o[2]), f(o[3]))
				rd.fl.cubicTo(rd.fl.curX, rd.fl.curY, x2, y2, x3, y3) // first control = current point
			}
		case "y":
			if len(o) >= 4 {
				m := rd.dmat()
				x1, y1 := applyPt(m, f(o[0]), f(o[1]))
				x3, y3 := applyPt(m, f(o[2]), f(o[3]))
				rd.fl.cubicTo(x1, y1, x3, y3, x3, y3) // second control = endpoint
			}
		case "re":
			if len(o) >= 4 {
				m := rd.dmat()
				x, y, w, h := f(o[0]), f(o[1]), f(o[2]), f(o[3])
				ax, ay := applyPt(m, x, y)
				bx, by := applyPt(m, x+w, y)
				cx, cy := applyPt(m, x+w, y+h)
				dx, dy := applyPt(m, x, y+h)
				rd.fl.moveTo(ax, ay)
				rd.fl.lineTo(bx, by)
				rd.fl.lineTo(cx, cy)
				rd.fl.lineTo(dx, dy)
				rd.fl.close()
			}
		case "h":
			rd.fl.close()

		// --- path painting ---
		case "f", "F":
			rd.fill(fillNonZero)
		case "f*":
			rd.fill(fillEvenOdd)
		case "S":
			rd.stroke()
		case "s":
			rd.fl.close()
			rd.stroke()
		case "B", "b":
			if op.Operator == "b" {
				rd.fl.close()
			}
			rd.fillKeep(fillNonZero)
			rd.stroke()
		case "B*", "b*":
			if op.Operator == "b*" {
				rd.fl.close()
			}
			rd.fillKeep(fillEvenOdd)
			rd.stroke()
		case "n":
			rd.resetPath()
		case "W", "W*":
			// Clipping arrives in P5; for now the path is consumed by the
			// following painting operator and no clip is applied.

		// --- colour ---
		case "g":
			rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = gray8(f(o0(o)))
			rd.gs.fillPattern = false
		case "G":
			rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = gray8(f(o0(o)))
			rd.gs.strokePattern = false
		case "rg":
			if len(o) >= 3 {
				rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
				rd.gs.fillPattern = false
			}
		case "RG":
			if len(o) >= 3 {
				rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
				rd.gs.strokePattern = false
			}
		case "k":
			if len(o) >= 4 {
				rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
				rd.gs.fillPattern = false
			}
		case "K":
			if len(o) >= 4 {
				rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
				rd.gs.strokePattern = false
			}
		case "sc", "scn":
			rd.setColor(o, false)
		case "SC", "SCN":
			rd.setColor(o, true)

		// --- XObjects ---
		case "Do":
			if len(o) >= 1 {
				rd.doXObject(operandName(o[0]))
			}

		default:
			// Text (BT…ET), inline images (BI…EI), shadings (sh), ExtGState
			// (gs), etc. arrive in later phases — skipped for now.
		}
	}
}

// setColor handles sc/scn (fill) and SC/SCN (stroke): all-numeric operands set
// a Gray/RGB/CMYK colour; a trailing name means a pattern we can't render yet,
// so subsequent fills/strokes with it are skipped.
func (rd *renderer) setColor(o []pdfValue, stroke bool) {
	if len(o) > 0 {
		if _, ok := o[len(o)-1].(pdfName); ok {
			if stroke {
				rd.gs.strokePattern = true
			} else {
				rd.gs.fillPattern = true
			}
			return
		}
	}
	var r, g, b uint8
	switch len(o) {
	case 1:
		r, g, b = gray8(f(o[0]))
	case 3:
		r, g, b = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
	case 4:
		r, g, b = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
	default:
		return
	}
	if stroke {
		rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB, rd.gs.strokePattern = r, g, b, false
	} else {
		rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillPattern = r, g, b, false
	}
}

func (rd *renderer) fill(rule fillRule) {
	rd.fillKeep(rule)
	rd.resetPath()
}

// fillKeep fills the current path without clearing it (used by B/b which fill
// then stroke the same path).
func (rd *renderer) fillKeep(rule fillRule) {
	if rd.gs.fillPattern {
		return
	}
	cov := rd.ras.coverage(rd.fl.path(), rule)
	compositeCoverage(rd.img, rd.w, cov, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillA, rd.gs.clip)
}

func (rd *renderer) resetPath() { rd.fl = newFlattener(0.2) }

// stroke paints the current path's outline with the stroke colour, then clears
// the path. Line width is converted from user space to device pixels via the
// CTM's linear scale, with a 1px floor (covers lineWidth 0 = thinnest line).
func (rd *renderer) stroke() {
	dp := rd.fl.path()
	defer rd.resetPath()
	if rd.gs.strokePattern {
		return
	}
	m := rd.dmat()
	scale := math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
	dw := rd.gs.lineWidth * scale
	if dw < 1 {
		dw = 1
	}
	outline := strokeToFill(dp, dw/2)
	cov := rd.ras.coverage(outline, fillNonZero)
	compositeCoverage(rd.img, rd.w, cov, rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB, rd.gs.strokeA, rd.gs.clip)
}

func f(v pdfValue) float64    { return operandFloat(v) }
func o0(o []pdfValue) pdfValue {
	if len(o) > 0 {
		return o[0]
	}
	return nil
}

func gray8(v float64) (uint8, uint8, uint8) {
	g := clamp8(v)
	return g, g, g
}
