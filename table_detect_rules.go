// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
)

// Path-geometry visitor + ruling-line collection (epic pdf-go-w4ht, phase 1).
// visitPaths is the reusable CTM-aware walk over a content stream's painted
// paths — per-segment geometry in page space, paint kind, stroke width, and
// optional Form-XObject recursion. The table detector's rule collector sits
// on top: stroked axis-aligned segments and thin filled rectangles (the way
// Word/LaTeX/Excel draw table borders) become horizontal/vertical rulings,
// snapped and joined into canonical lines; larger filled rectangles are kept
// as fill boxes (cell shading). flow_vector.go's cluster detection rides the
// same visitor (bbox + paint kind), replacing its private walker.

// pathPaint describes how a visited path was painted.
type pathPaint int

const (
	paintNone   pathPaint = iota // n — clip-only path
	paintStroke                  // S, s
	paintFill                    // f, F, f*
	paintBoth                    // B, B*, b, b*
)

// pathSeg is one straight segment of a visited path, in page space.
type pathSeg struct {
	X1, Y1, X2, Y2 float64
}

// pathVisit is one painted path delivered to the visitPaths callback.
type pathVisit struct {
	segs     []pathSeg // straight segments (moveto breaks, re edges, h closes)
	rects    []Rectangle
	hasCurve bool // c/v/y present (curved paths are never table rules)
	bbox     Rectangle
	paint    pathPaint
	strokeW  float64 // line width scaled through the CTM
	depth    int     // 0 = page content, >0 = inside a Form XObject
}

const maxVisitFormDepth = 8

// visitPaths walks the content ops with CTM tracking and reports every
// painted path. With recurseForms it follows Do into Form XObjects (depth
// capped, cycles guarded), applying the form /Matrix.
func visitPaths(objects map[int]*pdfObject, ops []contentOp, resources pdfDict, recurseForms bool, fn func(pathVisit)) {
	st := &pathVisitState{objects: objects, recurse: recurseForms, fn: fn, active: map[int]bool{}}
	st.walk(ops, resources, identityMatrix(), 1.0, 0)
}

type pathVisitState struct {
	objects map[int]*pdfObject
	recurse bool
	fn      func(pathVisit)
	active  map[int]bool // form object numbers on the recursion stack
}

func (st *pathVisitState) walk(ops []contentOp, resources pdfDict, baseCTM [6]float64, baseW float64, depth int) {
	ctm := baseCTM
	lineW := baseW
	type gsave struct {
		ctm   [6]float64
		lineW float64
	}
	var stack []gsave

	var cur pathVisit
	var curX, curY float64     // current point (path space)
	var startX, startY float64 // subpath start (for h)
	havePt := false

	toPage := func(x, y float64) (float64, float64) { return matApplyPoint(ctm, x, y) }
	grow := func(x, y float64) {
		if cur.bbox.URX < cur.bbox.LLX { // empty
			cur.bbox = Rectangle{LLX: x, LLY: y, URX: x, URY: y}
			return
		}
		cur.bbox.LLX, cur.bbox.LLY = minf(cur.bbox.LLX, x), minf(cur.bbox.LLY, y)
		cur.bbox.URX, cur.bbox.URY = maxf(cur.bbox.URX, x), maxf(cur.bbox.URY, y)
	}
	seg := func(x1, y1, x2, y2 float64) {
		px1, py1 := toPage(x1, y1)
		px2, py2 := toPage(x2, y2)
		cur.segs = append(cur.segs, pathSeg{px1, py1, px2, py2})
		grow(px1, py1)
		grow(px2, py2)
	}
	reset := func() {
		cur = pathVisit{bbox: Rectangle{LLX: 1, URX: 0}}
		havePt = false
	}
	reset()

	flush := func(paint pathPaint) {
		if len(cur.segs) > 0 || len(cur.rects) > 0 || cur.hasCurve {
			cur.paint = paint
			// The stroke width scales with the CTM (average axis scale).
			sx := math.Hypot(ctm[0], ctm[1])
			sy := math.Hypot(ctm[2], ctm[3])
			cur.strokeW = lineW * (sx + sy) / 2
			cur.depth = depth
			st.fn(cur)
		}
		reset()
	}

	nums := func(op contentOp) []float64 {
		out := make([]float64, len(op.Operands))
		for i, o := range op.Operands {
			out[i] = operandFloat(o)
		}
		return out
	}

	for _, op := range ops {
		switch op.Operator {
		case "cm":
			if v := nums(op); len(v) >= 6 {
				ctm = matMul([6]float64{v[0], v[1], v[2], v[3], v[4], v[5]}, ctm)
			}
		case "q":
			stack = append(stack, gsave{ctm, lineW})
		case "Q":
			if len(stack) > 0 {
				g := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				ctm, lineW = g.ctm, g.lineW
			}
		case "w":
			if v := nums(op); len(v) >= 1 {
				lineW = v[0]
			}
		case "m":
			if v := nums(op); len(v) >= 2 {
				curX, curY = v[0], v[1]
				startX, startY = curX, curY
				havePt = true
				px, py := toPage(curX, curY)
				grow(px, py)
			}
		case "l":
			if v := nums(op); len(v) >= 2 && havePt {
				seg(curX, curY, v[0], v[1])
				curX, curY = v[0], v[1]
			}
		case "c":
			if v := nums(op); len(v) >= 6 {
				cur.hasCurve = true
				for i := 0; i < 6; i += 2 {
					px, py := toPage(v[i], v[i+1])
					grow(px, py)
				}
				curX, curY = v[4], v[5]
			}
		case "v", "y":
			if v := nums(op); len(v) >= 4 {
				cur.hasCurve = true
				for i := 0; i < 4; i += 2 {
					px, py := toPage(v[i], v[i+1])
					grow(px, py)
				}
				curX, curY = v[2], v[3]
			}
		case "re":
			if v := nums(op); len(v) >= 4 {
				x, y, w, h := v[0], v[1], v[2], v[3]
				// Axis-aligned in page space only when the CTM has no shear.
				if ctm[1] == 0 && ctm[2] == 0 {
					px1, py1 := toPage(x, y)
					px2, py2 := toPage(x+w, y+h)
					cur.rects = append(cur.rects, Rectangle{
						LLX: minf(px1, px2), LLY: minf(py1, py2),
						URX: maxf(px1, px2), URY: maxf(py1, py2),
					})
					grow(px1, py1)
					grow(px2, py2)
				} else {
					seg(x, y, x+w, y)
					seg(x+w, y, x+w, y+h)
					seg(x+w, y+h, x, y+h)
					seg(x, y+h, x, y)
				}
				curX, curY = x, y
				startX, startY = x, y
				havePt = true
			}
		case "h":
			if havePt {
				seg(curX, curY, startX, startY)
				curX, curY = startX, startY
			}
		case "S":
			flush(paintStroke)
		case "s":
			if havePt {
				seg(curX, curY, startX, startY)
			}
			flush(paintStroke)
		case "f", "F", "f*":
			flush(paintFill)
		case "B", "B*":
			flush(paintBoth)
		case "b", "b*":
			if havePt {
				seg(curX, curY, startX, startY)
			}
			flush(paintBoth)
		case "n":
			flush(paintNone)
		case "Do":
			if !st.recurse || depth >= maxVisitFormDepth || len(op.Operands) < 1 {
				continue
			}
			st.enterForm(operandName(op.Operands[0]), resources, ctm, lineW, depth)
		}
	}
}

// enterForm recurses into a named Form XObject with the composed CTM.
func (st *pathVisitState) enterForm(name string, resources pdfDict, ctm [6]float64, lineW float64, depth int) {
	if name == "" || resources == nil {
		return
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return
	}
	xobjDict, ok := resolveRefToDict(st.objects, xobjVal)
	if !ok {
		return
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return
	}
	if ref, isRef := formVal.(pdfRef); isRef {
		if st.active[ref.Num] {
			return
		}
		st.active[ref.Num] = true
		defer delete(st.active, ref.Num)
	}
	stream, ok := resolveRef(st.objects, formVal).(*pdfStream)
	if !ok || dictGetName(stream.Dict, "/Subtype") != "/Form" {
		return
	}
	data := stream.Data
	if !stream.Decoded {
		var err error
		data, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return
		}
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return
	}
	formCTM := ctm
	if arr, ok := stream.Dict["/Matrix"].(pdfArray); ok && len(arr) == 6 {
		var fm [6]float64
		for i := 0; i < 6; i++ {
			fm[i] = operandFloat(arr[i])
		}
		formCTM = matMul(fm, ctm)
	}
	formRes := resources
	if rd, ok := resolveRefToDict(st.objects, stream.Dict["/Resources"]); ok {
		formRes = rd
	}
	st.walk(ops, formRes, formCTM, lineW, depth+1)
}

// --- ruling-line collection -------------------------------------------------------

const (
	ruleAxisTolPt  = 0.5 // max off-axis drift for a segment to count as a rule
	ruleThinFillPt = 2.5 // filled rects at most this thin are rules
	ruleSnapTolPt  = 2.0 // parallel rulings within this distance merge
	ruleJoinTolPt  = 3.0 // collinear gaps up to this join (dashes, cell-drawn)
	ruleMinLenPt   = 8.0 // shorter merged rulings are noise
)

// rule is one canonical ruling line. For horizontal rules pos = Y and lo/hi
// span X; vertical rules mirror.
type rule struct {
	pos    float64
	lo, hi float64
}

// pageRules extracts the page's ruling lines and fill boxes. Rules come from
// stroked axis-aligned segments, thin filled rectangles, and the edges of
// stroked rectangles; fillBoxes are the larger filled rectangles (cell
// shading), usable as secondary edge evidence.
func pageRules(p *Page) (hRules, vRules []rule, fillBoxes []Rectangle) {
	data, err := p.contentStreams()
	if err != nil || len(data) == 0 {
		return nil, nil, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, nil, nil
	}
	var hRaw, vRaw []rule
	visitPaths(p.doc.objects, ops, p.pageResources(), true, func(pv pathVisit) {
		switch pv.paint {
		case paintNone:
			return
		case paintStroke, paintBoth:
			for _, s := range pv.segs {
				if math.Abs(s.Y2-s.Y1) <= ruleAxisTolPt {
					y := (s.Y1 + s.Y2) / 2
					hRaw = append(hRaw, rule{pos: y, lo: minf(s.X1, s.X2), hi: maxf(s.X1, s.X2)})
				} else if math.Abs(s.X2-s.X1) <= ruleAxisTolPt {
					x := (s.X1 + s.X2) / 2
					vRaw = append(vRaw, rule{pos: x, lo: minf(s.Y1, s.Y2), hi: maxf(s.Y1, s.Y2)})
				}
			}
			for _, r := range pv.rects {
				hRaw = append(hRaw, rule{pos: r.LLY, lo: r.LLX, hi: r.URX}, rule{pos: r.URY, lo: r.LLX, hi: r.URX})
				vRaw = append(vRaw, rule{pos: r.LLX, lo: r.LLY, hi: r.URY}, rule{pos: r.URX, lo: r.LLY, hi: r.URY})
			}
		}
		if pv.paint == paintFill || pv.paint == paintBoth {
			for _, r := range pv.rects {
				w, h := r.URX-r.LLX, r.URY-r.LLY
				switch {
				case h <= ruleThinFillPt && w > h:
					hRaw = append(hRaw, rule{pos: (r.LLY + r.URY) / 2, lo: r.LLX, hi: r.URX})
				case w <= ruleThinFillPt && h > w:
					vRaw = append(vRaw, rule{pos: (r.LLX + r.URX) / 2, lo: r.LLY, hi: r.URY})
				default:
					fillBoxes = append(fillBoxes, r)
				}
			}
		}
	})
	return normalizeRules(hRaw), normalizeRules(vRaw), fillBoxes
}

// normalizeRules snaps parallel rulings within ruleSnapTolPt to a canonical
// position (length-weighted mean), joins collinear pieces whose gaps are at
// most ruleJoinTolPt, and drops results shorter than ruleMinLenPt.
func normalizeRules(raw []rule) []rule {
	if len(raw) == 0 {
		return nil
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].pos < raw[j].pos })

	var out []rule
	for i := 0; i < len(raw); {
		j := i + 1
		for j < len(raw) && raw[j].pos-raw[j-1].pos <= ruleSnapTolPt {
			j++
		}
		group := raw[i:j]
		// Canonical position: length-weighted mean.
		wsum, psum := 0.0, 0.0
		for _, r := range group {
			w := maxf(r.hi-r.lo, 0.1)
			wsum += w
			psum += r.pos * w
		}
		pos := psum / wsum
		// Join collinear pieces along the span axis.
		sort.Slice(group, func(a, b int) bool { return group[a].lo < group[b].lo })
		curLo, curHi := group[0].lo, group[0].hi
		for _, r := range group[1:] {
			if r.lo <= curHi+ruleJoinTolPt {
				curHi = maxf(curHi, r.hi)
				continue
			}
			if curHi-curLo >= ruleMinLenPt {
				out = append(out, rule{pos: pos, lo: curLo, hi: curHi})
			}
			curLo, curHi = r.lo, r.hi
		}
		if curHi-curLo >= ruleMinLenPt {
			out = append(out, rule{pos: pos, lo: curLo, hi: curHi})
		}
		i = j
	}
	return out
}
