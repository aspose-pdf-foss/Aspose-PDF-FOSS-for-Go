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
	fillPattern               bool // fill colour is a pattern → fills skipped unless fillShading is set
	strokePattern             bool

	// fillShading is set when the fill pattern is a shading pattern (PatternType
	// 2): fills then paint the shading clipped to the path. m maps shading space
	// to device pixels. fillTiling is set for a PatternType 1 (tiling) fill.
	// Both nil → solid colour or an unsupported pattern.
	fillShading  *shading
	fillShadingM [6]float64
	fillTiling   *tilingPattern

	lineWidth  float64
	lineCap    LineCap
	lineJoin   LineJoin
	miterLimit float64
	dash       []float64  // empty = solid
	dashPhase  float64
	blend      blendMode // zero value = Normal (src-over); set by gs /BM
	clip       []float32 // nil = unclipped (geometric W/W* clip)
	softMask   []float32 // nil = none; per-pixel alpha from a gs /SMask group

	// Separation/DeviceN tint transform for the current fill/stroke colour
	// space (set by cs/CS); nil → device colour by operand count.
	fillTint   tintFunc
	strokeTint tintFunc
}

// effectiveClip combines the geometric clip with the soft mask (both per-pixel
// alpha multipliers) for painting. The common case (no soft mask) returns the
// clip unchanged; only a live soft mask pays for an intersection.
func (rd *renderer) effectiveClip() []float32 {
	if rd.gs.softMask == nil {
		return rd.gs.clip
	}
	if rd.gs.clip == nil {
		return rd.gs.softMask
	}
	return intersectClip(rd.gs.clip, rd.gs.softMask)
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

	ts        textState              // current text-object state
	tsStack   []textState            // text state saved by q (parallels stack)
	fontCache map[string]*renderFont // resolved fonts by resource name

	// pendingClip records a W / W* seen since the last paint: 0 none, 1 nonzero,
	// 2 even-odd. The clip takes effect after the next painting operator (ISO
	// 32000-1 §8.5.4), so it is applied there against the path still in flight.
	pendingClip int

	// Optional Content: ocOff is the set of OCG object numbers hidden by the
	// default config; ocHidden counts nested marked-content sections currently
	// hidden by OC; mcStack records, per BDC/BMC level, whether it hid content.
	ocOff    map[int]bool
	ocHidden int
	mcStack  []bool
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
			ctm:        identityMatrix(),
			fillA:      1,
			strokeA:    1,
			lineWidth:  1,
			miterLimit: 10,
		},
		fl:        newFlattener(0.2),
		res:       p.pageResources(),
		ts:        textState{hScale: 1},
		fontCache: map[string]*renderFont{},
		ocOff:     ocOffSet(p.doc.objects, p.doc.catalog),
	}
}

// run parses and executes the page content. Parse failures and unsupported
// operators are tolerated: the page renders what it can rather than erroring.
func (rd *renderer) run() {
	if data, err := rd.page.contentStreams(); err == nil && len(data) > 0 {
		if ops, err := parseContentStream(data); err == nil {
			rd.exec(ops)
		}
	}
	// Annotation appearances (form-field widgets, stamps, highlights, …) live in
	// /Annots, not the page content stream, so a viewer paints them on top.
	rd.renderAnnotations()
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
			rd.tsStack = append(rd.tsStack, rd.ts)
		case "Q":
			if n := len(rd.stack); n > 0 {
				rd.gs = rd.stack[n-1]
				rd.stack = rd.stack[:n-1]
			}
			if n := len(rd.tsStack); n > 0 {
				// The text-state parameters (Tf, Tc, Tw, Th, Tl, Tmode, Ts) are
				// part of the graphics state and revert on Q (ISO 32000-1 Table
				// 52). The text matrices Tm/Tlm are text-object state, not
				// graphics state, so keep the current ones — q/Q may nest inside
				// a BT/ET block.
				saved := rd.tsStack[n-1]
				rd.tsStack = rd.tsStack[:n-1]
				saved.tm, saved.lm = rd.ts.tm, rd.ts.lm
				rd.ts = saved
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
		case "J":
			if len(o) >= 1 {
				rd.gs.lineCap = LineCap(int(f(o[0])))
			}
		case "j":
			if len(o) >= 1 {
				rd.gs.lineJoin = LineJoin(int(f(o[0])))
			}
		case "M":
			if len(o) >= 1 {
				rd.gs.miterLimit = f(o[0])
			}
		case "d":
			if len(o) >= 2 {
				rd.gs.dash, rd.gs.dashPhase = dashArray(o[0]), f(o[1])
			}
		case "gs":
			if len(o) >= 1 {
				rd.applyExtGState(operandName(o[0]))
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
			rd.applyPendingClip()
			rd.fill(fillNonZero)
		case "f*":
			rd.applyPendingClip()
			rd.fill(fillEvenOdd)
		case "S":
			rd.applyPendingClip()
			rd.stroke()
		case "s":
			rd.fl.close()
			rd.applyPendingClip()
			rd.stroke()
		case "B", "b":
			if op.Operator == "b" {
				rd.fl.close()
			}
			rd.applyPendingClip()
			rd.fillKeep(fillNonZero)
			rd.stroke()
		case "B*", "b*":
			if op.Operator == "b*" {
				rd.fl.close()
			}
			rd.applyPendingClip()
			rd.fillKeep(fillEvenOdd)
			rd.stroke()
		case "n":
			rd.applyPendingClip()
			rd.resetPath()
		case "W":
			rd.pendingClip = 1
		case "W*":
			rd.pendingClip = 2

		// --- colour ---
		case "g":
			rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = gray8(f(o0(o)))
			rd.gs.fillPattern, rd.gs.fillShading, rd.gs.fillTiling, rd.gs.fillTint = false, nil, nil, nil
		case "G":
			rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = gray8(f(o0(o)))
			rd.gs.strokePattern, rd.gs.strokeTint = false, nil
		case "rg":
			if len(o) >= 3 {
				rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
				rd.gs.fillPattern, rd.gs.fillShading, rd.gs.fillTiling, rd.gs.fillTint = false, nil, nil, nil
			}
		case "RG":
			if len(o) >= 3 {
				rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
				rd.gs.strokePattern, rd.gs.strokeTint = false, nil
			}
		case "k":
			if len(o) >= 4 {
				rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
				rd.gs.fillPattern, rd.gs.fillShading, rd.gs.fillTiling, rd.gs.fillTint = false, nil, nil, nil
			}
		case "K":
			if len(o) >= 4 {
				rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
				rd.gs.strokePattern, rd.gs.strokeTint = false, nil
			}
		case "cs":
			if len(o) >= 1 {
				rd.gs.fillTint = rd.tintConverter(rd.namedColorSpace(operandName(o[0])))
			}
		case "CS":
			if len(o) >= 1 {
				rd.gs.strokeTint = rd.tintConverter(rd.namedColorSpace(operandName(o[0])))
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

		// --- shading ---
		case "sh":
			if len(o) >= 1 {
				rd.paintShOperator(operandName(o[0]))
			}

		// --- inline image ---
		case "BI":
			rd.drawInlineImage(o)

		// --- marked content (Optional Content visibility) ---
		case "BMC":
			rd.mcStack = append(rd.mcStack, false)
		case "BDC":
			hidden := len(o) >= 2 && operandName(o[0]) == "/OC" && !rd.ocVisible(rd.ocProperty(o[1]))
			rd.mcStack = append(rd.mcStack, hidden)
			if hidden {
				rd.ocHidden++
			}
		case "EMC":
			if n := len(rd.mcStack); n > 0 {
				if rd.mcStack[n-1] {
					rd.ocHidden--
				}
				rd.mcStack = rd.mcStack[:n-1]
			}

		// --- text ---
		case "BT":
			rd.textBegin()
		case "ET":
			// no-op
		case "Tf":
			if len(o) >= 2 {
				rd.setFont(operandName(o[0]), f(o[1]))
			}
		case "Td":
			if len(o) >= 2 {
				rd.textMove(f(o[0]), f(o[1]))
			}
		case "TD":
			if len(o) >= 2 {
				rd.ts.leading = -f(o[1])
				rd.textMove(f(o[0]), f(o[1]))
			}
		case "Tm":
			if len(o) >= 6 {
				rd.textSetMatrix([6]float64{f(o[0]), f(o[1]), f(o[2]), f(o[3]), f(o[4]), f(o[5])})
			}
		case "T*":
			rd.textNextLine()
		case "TL":
			if len(o) >= 1 {
				rd.ts.leading = f(o[0])
			}
		case "Tc":
			if len(o) >= 1 {
				rd.ts.charSpace = f(o[0])
			}
		case "Tw":
			if len(o) >= 1 {
				rd.ts.wordSpace = f(o[0])
			}
		case "Tz":
			if len(o) >= 1 {
				rd.ts.hScale = f(o[0]) / 100
			}
		case "Ts":
			if len(o) >= 1 {
				rd.ts.rise = f(o[0])
			}
		case "Tr":
			if len(o) >= 1 {
				rd.ts.renderMode = int(f(o[0]))
			}
		case "Tk":
			// text knockout — not modelled
		case "Tj":
			if len(o) >= 1 {
				if s, ok := o[0].(string); ok {
					rd.showText(s)
				}
			}
		case "'":
			rd.textNextLine()
			if len(o) >= 1 {
				if s, ok := o[0].(string); ok {
					rd.showText(s)
				}
			}
		case "\"":
			if len(o) >= 3 {
				rd.ts.wordSpace = f(o[0])
				rd.ts.charSpace = f(o[1])
				rd.textNextLine()
				if s, ok := o[2].(string); ok {
					rd.showText(s)
				}
			}
		case "TJ":
			if len(o) >= 1 {
				rd.showTJ(o[0])
			}

		default:
			// Operators still unsupported (tiling patterns, Type3 glyphs,
			// blend modes, …) are skipped so the page always renders.
		}
	}
}

// setColor handles sc/scn (fill) and SC/SCN (stroke): all-numeric operands set
// a Gray/RGB/CMYK colour; a trailing name selects a pattern. A shading pattern
// (PatternType 2) is rendered on fill; other patterns (tiling) are skipped.
func (rd *renderer) setColor(o []pdfValue, stroke bool) {
	if len(o) > 0 {
		if name, ok := o[len(o)-1].(pdfName); ok {
			if stroke {
				rd.gs.strokePattern = true
			} else {
				rd.gs.fillPattern = true
				// Leading numeric operands give an uncolored (PaintType 2)
				// pattern its colour.
				if lead := o[:len(o)-1]; len(lead) > 0 {
					rd.setFillColor(lead)
				}
				rd.setFillPattern(string(name))
			}
			return
		}
	}
	// Separation/DeviceN: run the tint operands through the colour space's
	// transform regardless of operand count.
	tint := rd.gs.fillTint
	if stroke {
		tint = rd.gs.strokeTint
	}
	var r, g, b uint8
	if tint != nil && len(o) > 0 {
		r, g, b = tint(operandFloats(o))
	} else {
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
	}
	if stroke {
		rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB, rd.gs.strokePattern = r, g, b, false
	} else {
		rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillPattern, rd.gs.fillShading, rd.gs.fillTiling = r, g, b, false, nil, nil
	}
}

// operandFloats converts numeric colour operands to a float slice.
func operandFloats(o []pdfValue) []float64 {
	out := make([]float64, len(o))
	for i, v := range o {
		out[i] = operandFloat(v)
	}
	return out
}

// resolveShadingPattern looks up a /Pattern resource; if it is a shading pattern
// (PatternType 2) it returns the parsed shading and its shading-space → device
// matrix (the pattern /Matrix composed with the page base). Otherwise nil.
func (rd *renderer) resolveShadingPattern(name string) (*shading, [6]float64) {
	objects := rd.page.doc.objects
	pats, ok := resolveRefToDict(objects, rd.res["/Pattern"])
	if !ok {
		return nil, identityMatrix()
	}
	pd, _ := asDictStream(resolveRef(objects, pats[name]))
	if pd == nil || int(operandFloat(resolveRef(objects, pd["/PatternType"]))) != 2 {
		return nil, identityMatrix()
	}
	s := parseShading(objects, pd["/Shading"])
	if s == nil {
		return nil, identityMatrix()
	}
	m := rd.base
	if pm := shFloats(objects, pd["/Matrix"]); len(pm) == 6 {
		m = matMul([6]float64{pm[0], pm[1], pm[2], pm[3], pm[4], pm[5]}, rd.base)
	}
	return s, m
}

// compositePath rasterizes dp's coverage over just its bounding box and paints
// it with the given colour and alpha (honouring the clip). This is the hot path
// for fills, strokes and glyphs — bbox-scoped work instead of whole-frame.
func (rd *renderer) compositePath(dp *devPath, rule fillRule, sr, sg, sb uint8, alpha float64) {
	if rd.ocHidden > 0 {
		return
	}
	cov, x0, y0, x1, y1 := rd.ras.coverageBBox(dp, rule)
	if cov == nil {
		return
	}
	compositeCoverageBBox(rd.img, rd.w, cov, x0, y0, x1, y1, sr, sg, sb, alpha, rd.effectiveClip(), rd.gs.blend)
}

func (rd *renderer) fill(rule fillRule) {
	rd.fillKeep(rule)
	rd.resetPath()
}

// fillKeep fills the current path without clearing it (used by B/b which fill
// then stroke the same path).
func (rd *renderer) fillKeep(rule fillRule) {
	if rd.gs.fillPattern {
		switch {
		case rd.gs.fillShading != nil:
			cov := rd.ras.coverage(rd.fl.path(), rule)
			rd.paintShading(rd.gs.fillShading, rd.gs.fillShadingM, intersectClip(rd.effectiveClip(), cov))
		case rd.gs.fillTiling != nil:
			rd.fillTilingPattern(rd.gs.fillTiling, rule)
		}
		return // unsupported patterns: skip
	}
	rd.compositePath(rd.fl.path(), rule, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillA)
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
	// Dash lengths/phase are in user space; scale them like the line width into
	// device space before splitting the (already device-space) path.
	if len(rd.gs.dash) > 0 {
		scaled := make([]float64, len(rd.gs.dash))
		for i, d := range rd.gs.dash {
			scaled[i] = d * scale
		}
		dp = applyDash(dp, scaled, rd.gs.dashPhase*scale)
	}
	st := strokeStyle{hw: dw / 2, cap: rd.gs.lineCap, join: rd.gs.lineJoin, miterLimit: rd.gs.miterLimit}
	outline := strokeToFill(dp, st)
	rd.compositePath(outline, fillNonZero, rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB, rd.gs.strokeA)
}

// dashArray converts a PDF dash-array operand into a slice of lengths.
func dashArray(v pdfValue) []float64 {
	arr, ok := v.(pdfArray)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		out = append(out, operandFloat(e))
	}
	return out
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
