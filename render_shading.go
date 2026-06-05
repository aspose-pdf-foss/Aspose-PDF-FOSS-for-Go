// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// This file rasterizes PDF shadings (ISO 32000-1 §8.7.4.5) for the page
// renderer: axial (type 2) and radial (type 3), driven by a 1-in/N-out function
// (types 0 sampled, 2 exponential, 3 stitching, or an array of 1-out functions).
// It serves both the `sh` operator and shading patterns (PatternType 2). Colour
// is derived from the function's output-component count (1=gray, 3=RGB, 4=CMYK).

// pdfFunc evaluates a 1-input PDF function, returning its output components.
type pdfFunc interface {
	eval(t float64) []float64
}

// --- function types -------------------------------------------------------

type fnExponential struct {
	c0, c1 []float64
	n      float64
	dom    [2]float64
}

func (f *fnExponential) eval(t float64) []float64 {
	t = clampf(t, f.dom[0], f.dom[1])
	tn := t
	if f.n != 1 {
		tn = math.Pow(t, f.n)
	}
	out := make([]float64, len(f.c0))
	for i := range f.c0 {
		out[i] = f.c0[i] + tn*(f.c1[i]-f.c0[i])
	}
	return out
}

type fnStitching struct {
	funcs          []pdfFunc
	bounds, encode []float64
	dom            [2]float64
}

func (f *fnStitching) eval(t float64) []float64 {
	t = clampf(t, f.dom[0], f.dom[1])
	k := 0
	for k < len(f.bounds) && t >= f.bounds[k] {
		k++
	}
	if k >= len(f.funcs) {
		k = len(f.funcs) - 1
	}
	lo := f.dom[0]
	if k > 0 {
		lo = f.bounds[k-1]
	}
	hi := f.dom[1]
	if k < len(f.bounds) {
		hi = f.bounds[k]
	}
	e0, e1 := 0.0, 1.0
	if 2*k+1 < len(f.encode) {
		e0, e1 = f.encode[2*k], f.encode[2*k+1]
	}
	tt := interp(t, lo, hi, e0, e1)
	return f.funcs[k].eval(tt)
}

type fnSampled struct {
	dom, encode, decode []float64
	size, bps, nout     int
	samples             []byte
}

func (f *fnSampled) eval(t float64) []float64 {
	e0, e1 := 0.0, float64(f.size-1)
	if len(f.encode) >= 2 {
		e0, e1 = f.encode[0], f.encode[1]
	}
	e := clampf(interp(t, f.dom[0], f.dom[1], e0, e1), 0, float64(f.size-1))
	i0 := int(math.Floor(e))
	frac := e - float64(i0)
	i1 := i0 + 1
	if i1 > f.size-1 {
		i1 = f.size - 1
	}
	maxv := float64(uint64(1)<<uint(f.bps) - 1)
	out := make([]float64, f.nout)
	for j := 0; j < f.nout; j++ {
		s0 := float64(f.sample(i0, j)) / maxv
		s1 := float64(f.sample(i1, j)) / maxv
		v := s0 + frac*(s1-s0)
		d0, d1 := 0.0, 1.0
		if 2*j+1 < len(f.decode) {
			d0, d1 = f.decode[2*j], f.decode[2*j+1]
		}
		out[j] = d0 + v*(d1-d0)
	}
	return out
}

// sample reads the j-th output component of input sample i (big-endian bits).
func (f *fnSampled) sample(i, j int) uint64 {
	bitPos := (i*f.nout + j) * f.bps
	var v uint64
	for b := 0; b < f.bps; b++ {
		bytePos := (bitPos + b) / 8
		if bytePos >= len(f.samples) {
			break
		}
		bit := (f.samples[bytePos] >> uint(7-(bitPos+b)%8)) & 1
		v = v<<1 | uint64(bit)
	}
	return v
}

type fnArray struct{ fns []pdfFunc }

func (f *fnArray) eval(t float64) []float64 {
	out := make([]float64, 0, len(f.fns))
	for _, fn := range f.fns {
		out = append(out, fn.eval(t)...)
	}
	return out
}

// fnConst is the best-effort fallback for unsupported function types (type 4
// PostScript calculator): a constant mid-grey so the shape isn't invisible.
type fnConst struct{ c []float64 }

func (f *fnConst) eval(float64) []float64 { return f.c }

// parseFunction builds a pdfFunc from a function dict/stream or an array of
// functions. Unsupported types fall back to a constant colour.
func parseFunction(objects map[int]*pdfObject, v pdfValue) pdfFunc {
	v = resolveRef(objects, v)
	if arr, ok := v.(pdfArray); ok {
		fns := make([]pdfFunc, 0, len(arr))
		for _, e := range arr {
			if fn := parseFunction(objects, e); fn != nil {
				fns = append(fns, fn)
			}
		}
		if len(fns) == 0 {
			return &fnConst{c: []float64{0.5}}
		}
		return &fnArray{fns: fns}
	}
	d, stream := asDictStream(v)
	if d == nil {
		return &fnConst{c: []float64{0.5}}
	}
	dom := shFloats(objects, d["/Domain"])
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
	switch int(operandFloat(resolveRef(objects, d["/FunctionType"]))) {
	case 2:
		c0 := shFloats(objects, d["/C0"])
		c1 := shFloats(objects, d["/C1"])
		if len(c0) == 0 {
			c0 = []float64{0}
		}
		if len(c1) == 0 {
			c1 = []float64{1}
		}
		n := operandFloat(resolveRef(objects, d["/N"]))
		if n == 0 {
			n = 1
		}
		return &fnExponential{c0: c0, c1: c1, n: n, dom: [2]float64{dom[0], dom[1]}}
	case 3:
		var fns []pdfFunc
		if arr, ok := resolveRefToArray(objects, d["/Functions"]); ok {
			for _, e := range arr {
				fns = append(fns, parseFunction(objects, e))
			}
		}
		if len(fns) == 0 {
			return &fnConst{c: []float64{0.5}}
		}
		return &fnStitching{
			funcs:  fns,
			bounds: shFloats(objects, d["/Bounds"]),
			encode: shFloats(objects, d["/Encode"]),
			dom:    [2]float64{dom[0], dom[1]},
		}
	case 0:
		if stream == nil {
			return &fnConst{c: []float64{0.5}}
		}
		size := shFloats(objects, d["/Size"])
		rng := shFloats(objects, d["/Range"])
		if len(size) < 1 || len(rng) < 2 {
			return &fnConst{c: []float64{0.5}}
		}
		decode := shFloats(objects, d["/Decode"])
		if len(decode) == 0 {
			decode = rng
		}
		return &fnSampled{
			dom:     []float64{dom[0], dom[1]},
			encode:  shFloats(objects, d["/Encode"]),
			decode:  decode,
			size:    int(size[0]),
			bps:     int(operandFloat(resolveRef(objects, d["/BitsPerSample"]))),
			nout:    len(rng) / 2,
			samples: decodedStreamData(stream),
		}
	default: // type 4 (PostScript) and anything else
		return &fnConst{c: []float64{0.5}}
	}
}

// --- shading --------------------------------------------------------------

type shading struct {
	stype  int       // 2 axial, 3 radial
	coords []float64 // [x0 y0 x1 y1] or [x0 y0 r0 x1 y1 r1]
	domain [2]float64
	extend [2]bool
	fn     pdfFunc
}

// parseShading reads a /Shading dict (type 2 or 3); returns nil for others.
func parseShading(objects map[int]*pdfObject, v pdfValue) *shading {
	d, _ := asDictStream(resolveRef(objects, v))
	if d == nil {
		return nil
	}
	st := int(operandFloat(resolveRef(objects, d["/ShadingType"])))
	if st != 2 && st != 3 {
		return nil // mesh/function-based shadings are out of P5 scope
	}
	coords := shFloats(objects, d["/Coords"])
	if (st == 2 && len(coords) < 4) || (st == 3 && len(coords) < 6) {
		return nil
	}
	dom := shFloats(objects, d["/Domain"])
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
	ext := []bool{false, false}
	if arr, ok := resolveRefToArray(objects, d["/Extend"]); ok && len(arr) >= 2 {
		ext[0], _ = arr[0].(bool)
		ext[1], _ = arr[1].(bool)
	}
	fn := parseFunction(objects, d["/Function"])
	if fn == nil {
		return nil
	}
	return &shading{stype: st, coords: coords, domain: [2]float64{dom[0], dom[1]}, extend: [2]bool{ext[0], ext[1]}, fn: fn}
}

// paramAt maps a point in shading space to the function parameter t, or reports
// that the point lies outside the (un-extended) shading.
func (s *shading) paramAt(x, y float64) (float64, bool) {
	if s.stype == 2 {
		x0, y0, x1, y1 := s.coords[0], s.coords[1], s.coords[2], s.coords[3]
		dx, dy := x1-x0, y1-y0
		dd := dx*dx + dy*dy
		var u float64
		if dd != 0 {
			u = ((x-x0)*dx + (y-y0)*dy) / dd
		}
		if u < 0 {
			if !s.extend[0] {
				return 0, false
			}
			u = 0
		} else if u > 1 {
			if !s.extend[1] {
				return 0, false
			}
			u = 1
		}
		return interp(u, 0, 1, s.domain[0], s.domain[1]), true
	}
	// radial (type 3)
	x0, y0, r0 := s.coords[0], s.coords[1], s.coords[2]
	x1, y1, r1 := s.coords[3], s.coords[4], s.coords[5]
	dcx, dcy, dr := x1-x0, y1-y0, r1-r0
	px, py := x-x0, y-y0
	a := dcx*dcx + dcy*dcy - dr*dr
	b := -2 * (px*dcx + py*dcy + r0*dr)
	c := px*px + py*py - r0*r0

	var roots []float64
	if math.Abs(a) < 1e-9 {
		if math.Abs(b) > 1e-12 {
			roots = []float64{-c / b}
		}
	} else {
		disc := b*b - 4*a*c
		if disc >= 0 {
			sq := math.Sqrt(disc)
			roots = []float64{(-b + sq) / (2 * a), (-b - sq) / (2 * a)}
		}
	}
	best, found := 0.0, false
	for _, u := range roots {
		if r0+u*dr < 0 { // negative radius is not a real circle
			continue
		}
		uu := u
		switch {
		case uu < 0:
			if !s.extend[0] {
				continue
			}
			uu = 0
		case uu > 1:
			if !s.extend[1] {
				continue
			}
			uu = 1
		}
		if !found || uu > best { // prefer the larger parameter (frontmost circle)
			best, found = uu, true
		}
	}
	if !found {
		return 0, false
	}
	return interp(best, 0, 1, s.domain[0], s.domain[1]), true
}

// colorAt evaluates the shading colour at parameter t as an 8-bit RGB triple,
// choosing the colour model from the function's output-component count.
func (s *shading) colorAt(t float64) (uint8, uint8, uint8) {
	c := s.fn.eval(t)
	switch len(c) {
	case 1:
		return gray8(c[0])
	case 4:
		return cmykToRGB8(c[0], c[1], c[2], c[3])
	default:
		if len(c) < 3 {
			return 0, 0, 0
		}
		return clamp8(c[0]), clamp8(c[1]), clamp8(c[2])
	}
}

// paintShading rasterizes the shading into the image. m maps shading space to
// device pixels; mask (nil = whole page) limits painting to a coverage region
// (a clip and/or a fill path). Painting uses the current fill alpha.
func (rd *renderer) paintShading(s *shading, m [6]float64, mask []float32) {
	inv, ok := invertMatrix(m)
	if !ok {
		return
	}
	for py := 0; py < rd.h; py++ {
		for px := 0; px < rd.w; px++ {
			mi := py*rd.w + px
			cov := 1.0
			if mask != nil {
				cov = float64(mask[mi])
				if cov <= 0 {
					continue
				}
			}
			ux, uy := applyPt(inv, float64(px)+0.5, float64(py)+0.5)
			t, ok := s.paramAt(ux, uy)
			if !ok {
				continue
			}
			r, g, b := s.colorAt(t)
			compositePixel(rd.img, mi*4, r, g, b, cov*rd.gs.fillA, rd.gs.blend)
		}
	}
}

// --- operator + pattern entry points -------------------------------------

// paintShOperator handles `sh`: paint the named shading over the current clip,
// in current user space.
func (rd *renderer) paintShOperator(name string) {
	objects := rd.page.doc.objects
	shDict, ok := resolveRefToDict(objects, rd.res["/Shading"])
	if !ok {
		return
	}
	s := parseShading(objects, shDict[name])
	if s == nil {
		return
	}
	rd.paintShading(s, rd.dmat(), rd.gs.clip)
}

// --- helpers --------------------------------------------------------------

func asDictStream(v pdfValue) (pdfDict, *pdfStream) {
	switch x := v.(type) {
	case pdfDict:
		return x, nil
	case *pdfStream:
		return x.Dict, x
	}
	return nil, nil
}

// shFloats resolves v to a []float64 (resolving indirect refs in elements).
func shFloats(objects map[int]*pdfObject, v pdfValue) []float64 {
	arr, ok := resolveRefToArray(objects, v)
	if !ok {
		return nil
	}
	out := make([]float64, len(arr))
	for i, e := range arr {
		out[i] = operandFloat(resolveRef(objects, e))
	}
	return out
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func interp(x, xmin, xmax, ymin, ymax float64) float64 {
	if xmax == xmin {
		return ymin
	}
	return ymin + (x-xmin)*(ymax-ymin)/(xmax-xmin)
}
