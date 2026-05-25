// SPDX-License-Identifier: MIT

package asposepdf

// buildShadingFunction returns a *pdfObject containing a PDF function that maps t in [0,1]
// to an RGB color triple, suitable for use as the /Function entry of a PDF shading dictionary.
//
//   - 0 stops  → treated as single opaque-black stop
//   - 1 stop   → Type 2 exponential with C0 == C1 (constant color)
//   - 2 stops  → Type 2 exponential interpolating between the two stops
//   - 3+ stops → Type 3 stitching function containing (N-1) Type 2 sub-functions,
//     /Bounds at each internal stop offset, /Encode [0 1 …]
//
// The returned object has Num==0; the caller is responsible for assigning a real
// object number and inserting it into doc.objects before writing.
func buildShadingFunction(stops []svgGradientStop) *pdfObject {
	if len(stops) == 0 {
		stops = []svgGradientStop{
			{offset: 0, color: &Color{R: 0, G: 0, B: 0, A: 1}, opacity: 1},
		}
	}
	if len(stops) == 1 {
		return &pdfObject{Value: exponentialFunctionDict(stops[0].color, stops[0].color)}
	}
	if len(stops) == 2 {
		return &pdfObject{Value: exponentialFunctionDict(stops[0].color, stops[1].color)}
	}

	// 3+ stops: build a Type 3 stitching function.
	// Sub-functions: one Type 2 per adjacent stop pair.
	subFunctions := make(pdfArray, 0, len(stops)-1)
	for i := 0; i < len(stops)-1; i++ {
		subFunctions = append(subFunctions, exponentialFunctionDict(stops[i].color, stops[i+1].color))
	}

	// /Bounds: internal stop offsets (all except first and last).
	bounds := make(pdfArray, 0, len(stops)-2)
	for i := 1; i < len(stops)-1; i++ {
		bounds = append(bounds, stops[i].offset)
	}

	// /Encode: each sub-function maps its local [0,1] interval to [0 1].
	encode := make(pdfArray, 0, (len(stops)-1)*2)
	for i := 0; i < len(stops)-1; i++ {
		encode = append(encode, 0.0, 1.0)
	}

	dict := pdfDict{
		"/FunctionType": 3,
		"/Domain":       pdfArray{0.0, 1.0},
		"/Functions":    subFunctions,
		"/Bounds":       bounds,
		"/Encode":       encode,
	}
	return &pdfObject{Value: dict}
}

// gradientToShadingObject creates a /Shading dictionary indirect object for the gradient.
// The shading uses gradient coords as-is (no bbox/transform composition — that's the caller's
// responsibility via the /Matrix entry on the parent /Pattern dict).
//
// The /Function entry is stored as a pdfRef to the function object already registered in
// doc.objects (the caller — ensurePatternResource — handles that registration).
//
// Returns nil if grad is an unsupported type or has no document reference.
// The returned *pdfObject's Num is 0; ensurePatternResource assigns a real number.
func gradientToShadingObject(grad svgGradient, fnRef pdfRef) *pdfObject {
	shading := pdfDict{
		"/ColorSpace": pdfName("/DeviceRGB"),
		"/Extend":     pdfArray{true, true},
		"/Function":   fnRef,
	}
	switch g := grad.(type) {
	case *svgLinearGradient:
		shading["/ShadingType"] = 2
		shading["/Coords"] = pdfArray{g.x1, g.y1, g.x2, g.y2}
	case *svgRadialGradient:
		shading["/ShadingType"] = 3
		// Coords: [fx fy 0 cx cy r] — focal point as inner circle with radius 0.
		shading["/Coords"] = pdfArray{g.fx, g.fy, 0.0, g.cx, g.cy, g.r}
	default:
		return nil
	}
	return &pdfObject{Value: shading}
}

// exponentialFunctionDict returns an inline pdfDict (not wrapped in pdfObject) for a
// PDF Type 2 exponential function with N=1 interpolating between c0 and c1 in DeviceRGB.
func exponentialFunctionDict(c0, c1 *Color) pdfDict {
	return pdfDict{
		"/FunctionType": 2,
		"/Domain":       pdfArray{0.0, 1.0},
		"/C0":           pdfArray{c0.R, c0.G, c0.B},
		"/C1":           pdfArray{c1.R, c1.G, c1.B},
		"/N":            1,
	}
}
