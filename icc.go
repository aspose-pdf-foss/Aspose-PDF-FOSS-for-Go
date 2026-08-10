// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ICC profile parsing and colour transforms (epic pdf-go-16u). ICCBased
// colour spaces used to be passed through as their underlying
// DeviceRGB/Gray/CMYK with the profile discarded — correct for the sRGB-like
// profiles most PDFs embed, visibly wrong for wide-gamut ones (AdobeRGB
// treated as sRGB oversaturates). This file parses the profile classes the
// corpus actually contains (survey across 1,014 documents: 106 RGB
// matrix/TRC, 49 grayscale TRC, 1 CMYK LUT) and converts colours to sRGB
// through the D50 profile connection space. Pure Go; no lcms, no cgo.
//
// Scope: ICC v2 monochrome and three-component matrix/TRC profiles
// ('curv' and 'para' tone curves), and v2 LUT profiles (mft1/lut8,
// mft2/lut16) with XYZ or Lab PCS. v4 mAB pipelines and rendering intents
// beyond the default are not interpreted — parse failures simply leave the
// colour untransformed, which is the previous behaviour.

// iccCurve is one tone-reproduction curve, evaluated on [0,1].
type iccCurve struct {
	gamma float64   // used when table is nil (identity = 1.0)
	table []float64 // sampled curve, linear interpolation
}

func (c *iccCurve) eval(v float64) float64 {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if c == nil {
		return v
	}
	if c.table == nil {
		if c.gamma == 1 {
			return v
		}
		return math.Pow(v, c.gamma)
	}
	if len(c.table) == 1 {
		return c.table[0]
	}
	x := v * float64(len(c.table)-1)
	i := int(x)
	if i >= len(c.table)-1 {
		return c.table[len(c.table)-1]
	}
	f := x - float64(i)
	return c.table[i]*(1-f) + c.table[i+1]*f
}

// iccLUT is a v2 mft1/mft2 device→PCS pipeline.
type iccLUT struct {
	inCh, outCh int
	grid        int         // grid points per input channel
	inCurves    [][]float64 // per input channel, sampled
	clut        []float64   // grid^inCh entries × outCh, normalized [0,1]
	outCurves   [][]float64
}

// iccProfile is a parsed profile with everything needed to reach sRGB.
type iccProfile struct {
	space  string // "RGB", "GRAY", "CMYK"
	pcsLab bool   // PCS is Lab (else XYZ)
	nComp  int

	// Matrix/TRC path (RGB): columns are the r/g/b colorant XYZ values.
	m   [9]float64
	trc [3]*iccCurve

	// Monochrome path.
	kTRC *iccCurve

	// LUT path (CMYK and any profile carrying A2B0).
	a2b *iccLUT

	srgbLike bool // no-op transform: skip per-pixel work
}

const (
	iccHeaderSize = 128
	iccMinSize    = iccHeaderSize + 4
)

// parseICCProfile parses the profile classes described above; error for
// anything else (caller falls back to the untransformed colour).
func parseICCProfile(data []byte) (*iccProfile, error) {
	if len(data) < iccMinSize {
		return nil, fmt.Errorf("icc: too short")
	}
	if string(data[36:40]) != "acsp" {
		return nil, fmt.Errorf("icc: bad signature")
	}
	p := &iccProfile{}
	switch string(data[16:20]) {
	case "RGB ":
		p.space, p.nComp = "RGB", 3
	case "GRAY":
		p.space, p.nComp = "GRAY", 1
	case "CMYK":
		p.space, p.nComp = "CMYK", 4
	default:
		return nil, fmt.Errorf("icc: unsupported space %q", data[16:20])
	}
	switch string(data[20:24]) {
	case "XYZ ":
	case "Lab ":
		p.pcsLab = true
	default:
		return nil, fmt.Errorf("icc: unsupported PCS %q", data[20:24])
	}

	tagCount := int(binary.BigEndian.Uint32(data[128:132]))
	if tagCount < 0 || tagCount > 1024 || len(data) < 132+tagCount*12 {
		return nil, fmt.Errorf("icc: bad tag table")
	}
	tags := map[string][]byte{}
	for i := 0; i < tagCount; i++ {
		off := 132 + i*12
		sig := string(data[off : off+4])
		start := binary.BigEndian.Uint32(data[off+4 : off+8])
		size := binary.BigEndian.Uint32(data[off+8 : off+12])
		if int64(start)+int64(size) > int64(len(data)) || size < 8 {
			continue
		}
		tags[sig] = data[start : start+size]
	}

	// LUT pipeline wins when present (the profile's own preferred transform
	// for non-matrix classes; required for CMYK).
	if a2b, ok := tags["A2B0"]; ok {
		lut, err := parseICCLUT(a2b, p.nComp)
		if err == nil {
			p.a2b = lut
			return p, nil
		}
		if p.space == "CMYK" {
			return nil, err // CMYK has no other path
		}
	}

	switch p.space {
	case "GRAY":
		c, err := parseICCCurve(tags["kTRC"])
		if err != nil {
			return nil, fmt.Errorf("icc: gray without usable kTRC: %w", err)
		}
		p.kTRC = c
		return p, nil
	case "RGB":
		if p.pcsLab {
			return nil, fmt.Errorf("icc: matrix RGB with Lab PCS")
		}
		cols := [3]string{"rXYZ", "gXYZ", "bXYZ"}
		for i, sig := range cols {
			x, y, z, err := parseICCXYZ(tags[sig])
			if err != nil {
				return nil, fmt.Errorf("icc: %s: %w", sig, err)
			}
			p.m[0+i], p.m[3+i], p.m[6+i] = x, y, z
		}
		curves := [3]string{"rTRC", "gTRC", "bTRC"}
		for i, sig := range curves {
			c, err := parseICCCurve(tags[sig])
			if err != nil {
				return nil, fmt.Errorf("icc: %s: %w", sig, err)
			}
			p.trc[i] = c
		}
		p.srgbLike = p.matrixIsSRGB()
		return p, nil
	}
	return nil, fmt.Errorf("icc: CMYK without A2B0")
}

func s15f16(b []byte) float64 {
	return float64(int32(binary.BigEndian.Uint32(b))) / 65536
}

func parseICCXYZ(tag []byte) (x, y, z float64, err error) {
	if len(tag) < 20 || string(tag[:4]) != "XYZ " {
		return 0, 0, 0, fmt.Errorf("bad XYZ tag")
	}
	return s15f16(tag[8:12]), s15f16(tag[12:16]), s15f16(tag[16:20]), nil
}

// parseICCCurve reads 'curv' (identity / gamma / sampled) and 'para'
// (parametric types 0-4) tone curves.
func parseICCCurve(tag []byte) (*iccCurve, error) {
	if len(tag) < 12 {
		return nil, fmt.Errorf("curve tag too short")
	}
	switch string(tag[:4]) {
	case "curv":
		n := int(binary.BigEndian.Uint32(tag[8:12]))
		switch {
		case n == 0:
			return &iccCurve{gamma: 1}, nil
		case n == 1:
			if len(tag) < 14 {
				return nil, fmt.Errorf("curv gamma short")
			}
			g := float64(binary.BigEndian.Uint16(tag[12:14])) / 256
			return &iccCurve{gamma: g}, nil
		default:
			if len(tag) < 12+2*n {
				return nil, fmt.Errorf("curv table short")
			}
			t := make([]float64, n)
			for i := 0; i < n; i++ {
				t[i] = float64(binary.BigEndian.Uint16(tag[12+2*i:])) / 65535
			}
			return &iccCurve{table: t}, nil
		}
	case "para":
		if len(tag) < 12+4 {
			return nil, fmt.Errorf("para short")
		}
		typ := binary.BigEndian.Uint16(tag[8:10])
		nPar := []int{1, 3, 4, 5, 7}
		if int(typ) >= len(nPar) {
			return nil, fmt.Errorf("para type %d", typ)
		}
		if len(tag) < 12+4*nPar[typ] {
			return nil, fmt.Errorf("para params short")
		}
		par := make([]float64, nPar[typ])
		for i := range par {
			par[i] = s15f16(tag[12+4*i:])
		}
		// Sample the parametric curve; 1024 points is plenty for 8/16-bit
		// sources and keeps eval branch-free.
		t := make([]float64, 1024)
		for i := range t {
			x := float64(i) / 1023
			t[i] = evalICCPara(typ, par, x)
		}
		return &iccCurve{table: t}, nil
	}
	return nil, fmt.Errorf("curve type %q", tag[:4])
}

// evalICCPara implements ICC parametricCurveType formulas 0-4.
func evalICCPara(typ uint16, p []float64, x float64) float64 {
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	switch typ {
	case 0: // Y = X^g
		return clamp(math.Pow(x, p[0]))
	case 1: // Y = (aX+b)^g for X >= -b/a else 0
		g, a, b := p[0], p[1], p[2]
		if a*x+b < 0 {
			return 0
		}
		return clamp(math.Pow(a*x+b, g))
	case 2: // Y = (aX+b)^g + c for X >= -b/a else c
		g, a, b, c := p[0], p[1], p[2], p[3]
		if a*x+b < 0 {
			return clamp(c)
		}
		return clamp(math.Pow(a*x+b, g) + c)
	case 3: // sRGB-style: X >= d ? (aX+b)^g : cX
		g, a, b, c, d := p[0], p[1], p[2], p[3], p[4]
		if x >= d {
			return clamp(math.Pow(a*x+b, g))
		}
		return clamp(c * x)
	case 4:
		g, a, b, c, d, e, f := p[0], p[1], p[2], p[3], p[4], p[5], p[6]
		if x >= d {
			return clamp(math.Pow(a*x+b, g) + e)
		}
		return clamp(c*x + f)
	}
	return x
}

// parseICCLUT reads mft1 (lut8) / mft2 (lut16) A2B pipelines.
func parseICCLUT(tag []byte, inCh int) (*iccLUT, error) {
	if len(tag) < 48 {
		return nil, fmt.Errorf("lut tag short")
	}
	sig := string(tag[:4])
	if sig != "mft1" && sig != "mft2" {
		return nil, fmt.Errorf("lut type %q", sig)
	}
	l := &iccLUT{
		inCh:  int(tag[8]),
		outCh: int(tag[9]),
		grid:  int(tag[10]),
	}
	if l.inCh != inCh || l.outCh < 3 || l.grid < 2 {
		return nil, fmt.Errorf("lut geometry in=%d out=%d grid=%d", l.inCh, l.outCh, l.grid)
	}
	// The 3x3 matrix at 12..48 applies only to XYZ PCS input on B2A; for
	// A2B device input it must be identity — ignore (spec-conformant for
	// device sources).
	pos := 48
	nIn, nOut := 256, 256
	wordSize := 1
	if sig == "mft2" {
		if len(tag) < 52 {
			return nil, fmt.Errorf("mft2 short")
		}
		nIn = int(binary.BigEndian.Uint16(tag[48:50]))
		nOut = int(binary.BigEndian.Uint16(tag[50:52]))
		pos = 52
		wordSize = 2
		if nIn < 2 || nOut < 2 {
			return nil, fmt.Errorf("mft2 table sizes")
		}
	}
	read := func(n int) ([]float64, error) {
		need := n * wordSize
		if len(tag) < pos+need {
			return nil, fmt.Errorf("lut data short")
		}
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			if wordSize == 1 {
				out[i] = float64(tag[pos+i]) / 255
			} else {
				out[i] = float64(binary.BigEndian.Uint16(tag[pos+2*i:])) / 65535
			}
		}
		pos += need
		return out, nil
	}
	for i := 0; i < l.inCh; i++ {
		c, err := read(nIn)
		if err != nil {
			return nil, err
		}
		l.inCurves = append(l.inCurves, c)
	}
	clutN := 1
	for i := 0; i < l.inCh; i++ {
		clutN *= l.grid
	}
	clut, err := read(clutN * l.outCh)
	if err != nil {
		return nil, err
	}
	l.clut = clut
	for i := 0; i < l.outCh; i++ {
		c, err := read(nOut)
		if err != nil {
			return nil, err
		}
		l.outCurves = append(l.outCurves, c)
	}
	return l, nil
}

func sampleTable(t []float64, v float64) float64 {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	x := v * float64(len(t)-1)
	i := int(x)
	if i >= len(t)-1 {
		return t[len(t)-1]
	}
	f := x - float64(i)
	return t[i]*(1-f) + t[i+1]*f
}

// evalLUT runs the device values through the pipeline (multilinear CLUT
// interpolation over 2^inCh corners).
func (l *iccLUT) evalLUT(in []float64) []float64 {
	n := l.inCh
	pos := make([]float64, n)
	idx := make([]int, n)
	frac := make([]float64, n)
	for i := 0; i < n; i++ {
		pos[i] = sampleTable(l.inCurves[i], in[i]) * float64(l.grid-1)
		idx[i] = int(pos[i])
		if idx[i] >= l.grid-1 {
			idx[i] = l.grid - 2
			if idx[i] < 0 {
				idx[i] = 0
			}
		}
		frac[i] = pos[i] - float64(idx[i])
	}
	out := make([]float64, l.outCh)
	corners := 1 << n
	for c := 0; c < corners; c++ {
		w := 1.0
		flat := 0
		stride := 1
		// Index arithmetic: last channel varies fastest per ICC layout.
		for i := n - 1; i >= 0; i-- {
			bit := (c >> i) & 1
			gi := idx[i] + bit
			if bit == 1 {
				w *= frac[i]
			} else {
				w *= 1 - frac[i]
			}
			flat += gi * stride
			stride *= l.grid
		}
		if w == 0 {
			continue
		}
		base := flat * l.outCh
		for o := 0; o < l.outCh; o++ {
			out[o] += w * l.clut[base+o]
		}
	}
	for o := 0; o < l.outCh; o++ {
		out[o] = sampleTable(l.outCurves[o], out[o])
	}
	return out
}

// --- PCS → sRGB -------------------------------------------------------------

// xyzD50ToSRGB is the combined Bradford D50→D65 adaptation and sRGB
// XYZ→linear-RGB matrix (the standard ICC-workflow matrix).
var xyzD50ToSRGB = [9]float64{
	3.1338561, -1.6168667, -0.4906146,
	-0.9787684, 1.9161415, 0.0334540,
	0.0719453, -0.2289914, 1.4052427,
}

func srgbEncode(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func xyzToSRGB(x, y, z float64) (float64, float64, float64) {
	r := xyzD50ToSRGB[0]*x + xyzD50ToSRGB[1]*y + xyzD50ToSRGB[2]*z
	g := xyzD50ToSRGB[3]*x + xyzD50ToSRGB[4]*y + xyzD50ToSRGB[5]*z
	b := xyzD50ToSRGB[6]*x + xyzD50ToSRGB[7]*y + xyzD50ToSRGB[8]*z
	return srgbEncode(r), srgbEncode(g), srgbEncode(b)
}

// labToXYZ converts PCS Lab (D50) to XYZ.
func labToXYZ(L, a, b float64) (float64, float64, float64) {
	fy := (L + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	finv := func(t float64) float64 {
		if t > 6.0/29 {
			return t * t * t
		}
		return 3 * (6.0 / 29) * (6.0 / 29) * (t - 4.0/29)
	}
	// D50 white point.
	return 0.9642 * finv(fx), 1.0 * finv(fy), 0.8249 * finv(fz)
}

// toSRGB converts one colour (device components, [0,1]) to nonlinear sRGB.
func (p *iccProfile) toSRGB(in []float64) (r, g, b float64) {
	switch {
	case p.a2b != nil:
		out := p.a2b.evalLUT(in)
		if p.pcsLab {
			// ICC v2 16-bit Lab encoding: L in [0,100] maps to
			// [0,0xFF00/0xFFFF]; a,b in [-128,127+255/256].
			L := out[0] * 65535 / 65280 * 100
			A := out[1]*65535/65280*255 - 128
			B := out[2]*65535/65280*255 - 128
			return xyzToSRGB(labToXYZ(L, A, B))
		}
		// XYZ PCS: values are XYZ/2 per the u1Fixed15 LUT encoding.
		return xyzToSRGB(out[0]*(1+32767.0/32768), out[1]*(1+32767.0/32768), out[2]*(1+32767.0/32768))
	case p.kTRC != nil:
		y := p.kTRC.eval(in[0])
		v := srgbEncode(y)
		return v, v, v
	default:
		lr := p.trc[0].eval(in[0])
		lg := p.trc[1].eval(in[1])
		lb := p.trc[2].eval(in[2])
		x := p.m[0]*lr + p.m[1]*lg + p.m[2]*lb
		y := p.m[3]*lr + p.m[4]*lg + p.m[5]*lb
		z := p.m[6]*lr + p.m[7]*lg + p.m[8]*lb
		return xyzToSRGB(x, y, z)
	}
}

// matrixIsSRGB reports whether the matrix/TRC profile is close enough to
// sRGB that the transform is a no-op (spares per-pixel work for the most
// common embedded profile).
func (p *iccProfile) matrixIsSRGB() bool {
	// sRGB primaries, D50-adapted (as stored in sRGB ICC profiles).
	want := [9]float64{
		0.4360, 0.3851, 0.1431,
		0.2225, 0.7169, 0.0606,
		0.0139, 0.0971, 0.7139,
	}
	for i := range want {
		if math.Abs(p.m[i]-want[i]) > 0.02 {
			return false
		}
	}
	// TRC ≈ sRGB curve at a few probe points.
	for _, x := range []float64{0.02, 0.2, 0.5, 0.8} {
		wantY := math.Pow((x+0.055)/1.055, 2.4)
		if x <= 0.04045 {
			wantY = x / 12.92
		}
		for i := 0; i < 3; i++ {
			if math.Abs(p.trc[i].eval(x)-wantY) > 0.03 {
				return false
			}
		}
	}
	return true
}

// --- wiring helpers ---------------------------------------------------------

// iccImageProfile resolves an image dict's /ColorSpace [/ICCBased stream] to
// a parsed profile; nil when absent, unparseable, or effectively sRGB (the
// caller then keeps the untransformed fast path).
func iccImageProfile(objects map[int]*pdfObject, d pdfDict) *iccProfile {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return nil
	}
	return iccProfileFromCS(objects, csVal)
}

// iccProfileFromCS resolves an [/ICCBased stream] colour-space value.
func iccProfileFromCS(objects map[int]*pdfObject, csVal pdfValue) *iccProfile {
	arr, ok := resolveRefToArray(objects, csVal)
	if !ok || len(arr) < 2 || operandName(arr[0]) != "/ICCBased" {
		return nil
	}
	stream, ok := resolveRef(objects, arr[1]).(*pdfStream)
	if !ok {
		return nil
	}
	prof, err := parseICCProfile(decodedStreamData(stream))
	if err != nil || prof.srgbLike {
		return nil
	}
	return prof
}

// convertPixels transforms 8-bpc device samples (nComp interleaved channels)
// to 8-bpc sRGB. A memo over packed input colours keeps LUT profiles cheap —
// images repeat colours heavily.
func (p *iccProfile) convertPixels(samples []byte, pixelCount int) []byte {
	out := make([]byte, pixelCount*3)
	n := p.nComp
	memo := make(map[uint32][3]byte, 4096)
	in := make([]float64, n)
	for i := 0; i < pixelCount; i++ {
		off := i * n
		if off+n > len(samples) {
			break
		}
		var key uint32
		for c := 0; c < n; c++ {
			key = key<<8 | uint32(samples[off+c])
		}
		rgb, ok := memo[key]
		if !ok {
			for c := 0; c < n; c++ {
				in[c] = float64(samples[off+c]) / 255
			}
			r, g, b := p.toSRGB(in)
			rgb = [3]byte{uint8(r*255 + 0.5), uint8(g*255 + 0.5), uint8(b*255 + 0.5)}
			if len(memo) < 1<<16 {
				memo[key] = rgb
			}
		}
		out[i*3], out[i*3+1], out[i*3+2] = rgb[0], rgb[1], rgb[2]
	}
	return out
}
