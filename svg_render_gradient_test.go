// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderSVG_LinearGradientEmitsPattern(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte("/Pattern cs")) {
		t.Errorf("expected /Pattern cs in stream:\n%s", stream)
	}
	if !bytes.Contains(stream, []byte("scn")) {
		t.Error("expected pattern setter (scn op)")
	}
}

func TestRenderSVG_RadialGradientEmitsPattern(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/radial_gradient.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte("/Pattern cs")) {
		t.Errorf("expected /Pattern cs in stream for radial:\n%s", stream)
	}
}

// helper: extract the pdfDict from a pdfObject returned by buildShadingFunction.
func shadingDict(t *testing.T, fn *pdfObject) pdfDict {
	t.Helper()
	if fn == nil {
		t.Fatal("buildShadingFunction returned nil")
	}
	d, ok := fn.Value.(pdfDict)
	if !ok {
		t.Fatalf("expected pdfDict, got %T", fn.Value)
	}
	return d
}

// helper: get an int field from a pdfDict, fail if missing or wrong type.
func dictInt(t *testing.T, d pdfDict, key string) int {
	t.Helper()
	v, ok := d[key]
	if !ok {
		t.Fatalf("key %q missing from dict", key)
	}
	i, ok := v.(int)
	if !ok {
		t.Fatalf("key %q: expected int, got %T (%v)", key, v, v)
	}
	return i
}

// helper: get a pdfArray field from a pdfDict.
func dictArray(t *testing.T, d pdfDict, key string) pdfArray {
	t.Helper()
	v, ok := d[key]
	if !ok {
		t.Fatalf("key %q missing from dict", key)
	}
	a, ok := v.(pdfArray)
	if !ok {
		t.Fatalf("key %q: expected pdfArray, got %T (%v)", key, v, v)
	}
	return a
}

// helper: get float64 from a pdfArray element.
func arrayFloat(t *testing.T, a pdfArray, idx int) float64 {
	t.Helper()
	if idx >= len(a) {
		t.Fatalf("array index %d out of range (len=%d)", idx, len(a))
	}
	f, ok := a[idx].(float64)
	if !ok {
		t.Fatalf("array[%d]: expected float64, got %T (%v)", idx, a[idx], a[idx])
	}
	return f
}

func TestBuildShadingFunction_ZeroStops(t *testing.T) {
	fn := buildShadingFunction(nil)
	d := shadingDict(t, fn)
	if dictInt(t, d, "/FunctionType") != 2 {
		t.Error("expected FunctionType 2 for zero-stop fallback")
	}
	// C0 and C1 should be identical black
	c0 := dictArray(t, d, "/C0")
	c1 := dictArray(t, d, "/C1")
	for i := 0; i < 3; i++ {
		if arrayFloat(t, c0, i) != arrayFloat(t, c1, i) {
			t.Errorf("C0[%d] != C1[%d] for constant-color function", i, i)
		}
	}
}

func TestBuildShadingFunction_OneStop(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	d := shadingDict(t, fn)

	if dictInt(t, d, "/FunctionType") != 2 {
		t.Error("expected FunctionType 2 for single stop")
	}
	// C0 == C1 == red
	c0 := dictArray(t, d, "/C0")
	c1 := dictArray(t, d, "/C1")
	if arrayFloat(t, c0, 0) != 1.0 {
		t.Errorf("C0[R] expected 1.0, got %v", arrayFloat(t, c0, 0))
	}
	if arrayFloat(t, c1, 0) != 1.0 {
		t.Errorf("C1[R] expected 1.0, got %v", arrayFloat(t, c1, 0))
	}
}

func TestBuildShadingFunction_TwoStops_ExponentialType2(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 1, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	d := shadingDict(t, fn)

	if dictInt(t, d, "/FunctionType") != 2 {
		t.Error("expected FunctionType 2 for two stops")
	}
	if dictInt(t, d, "/N") != 1 {
		t.Error("expected /N 1")
	}

	// Domain [0 1]
	domain := dictArray(t, d, "/Domain")
	if arrayFloat(t, domain, 0) != 0.0 || arrayFloat(t, domain, 1) != 1.0 {
		t.Errorf("unexpected /Domain: %v", domain)
	}

	// C0 = red (R=1, G=0, B=0)
	c0 := dictArray(t, d, "/C0")
	if arrayFloat(t, c0, 0) != 1.0 {
		t.Errorf("C0 R: expected 1.0, got %v", arrayFloat(t, c0, 0))
	}
	if arrayFloat(t, c0, 1) != 0.0 {
		t.Errorf("C0 G: expected 0.0, got %v", arrayFloat(t, c0, 1))
	}

	// C1 = blue (R=0, G=0, B=1)
	c1 := dictArray(t, d, "/C1")
	if arrayFloat(t, c1, 0) != 0.0 {
		t.Errorf("C1 R: expected 0.0, got %v", arrayFloat(t, c1, 0))
	}
	if arrayFloat(t, c1, 2) != 1.0 {
		t.Errorf("C1 B: expected 1.0, got %v", arrayFloat(t, c1, 2))
	}
}

func TestBuildShadingFunction_ThreeStops_StitchingType3(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 0.5, color: &Color{R: 0, G: 1, B: 0, A: 1}, opacity: 1},
		{offset: 1, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	d := shadingDict(t, fn)

	if dictInt(t, d, "/FunctionType") != 3 {
		t.Error("expected FunctionType 3 for three stops")
	}

	// /Functions must have 2 entries (one per adjacent pair)
	funcs := dictArray(t, d, "/Functions")
	if len(funcs) != 2 {
		t.Errorf("/Functions: expected 2 entries, got %d", len(funcs))
	}

	// Each entry should be an inline pdfDict with FunctionType 2
	for i, f := range funcs {
		fd, ok := f.(pdfDict)
		if !ok {
			t.Errorf("/Functions[%d]: expected pdfDict, got %T", i, f)
			continue
		}
		ft, ok := fd["/FunctionType"].(int)
		if !ok || ft != 2 {
			t.Errorf("/Functions[%d]/FunctionType: expected 2, got %v", i, fd["/FunctionType"])
		}
	}

	// /Bounds must have 1 entry (the internal stop at 0.5)
	bounds := dictArray(t, d, "/Bounds")
	if len(bounds) != 1 {
		t.Errorf("/Bounds: expected 1 entry, got %d", len(bounds))
	}
	if arrayFloat(t, bounds, 0) != 0.5 {
		t.Errorf("/Bounds[0]: expected 0.5, got %v", arrayFloat(t, bounds, 0))
	}

	// /Encode must have 4 entries: [0 1 0 1]
	encode := dictArray(t, d, "/Encode")
	if len(encode) != 4 {
		t.Errorf("/Encode: expected 4 entries, got %d", len(encode))
	}
	for i, expected := range []float64{0, 1, 0, 1} {
		if arrayFloat(t, encode, i) != expected {
			t.Errorf("/Encode[%d]: expected %v, got %v", i, expected, arrayFloat(t, encode, i))
		}
	}

	// /Domain [0 1]
	domain := dictArray(t, d, "/Domain")
	if arrayFloat(t, domain, 0) != 0.0 || arrayFloat(t, domain, 1) != 1.0 {
		t.Errorf("unexpected /Domain: %v", domain)
	}
}

func TestBuildShadingFunction_FourStops_StitchingType3(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 0.25, color: &Color{R: 1, G: 1, B: 0, A: 1}, opacity: 1},
		{offset: 0.75, color: &Color{R: 0, G: 1, B: 0, A: 1}, opacity: 1},
		{offset: 1, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	d := shadingDict(t, fn)

	if dictInt(t, d, "/FunctionType") != 3 {
		t.Error("expected FunctionType 3 for four stops")
	}

	funcs := dictArray(t, d, "/Functions")
	if len(funcs) != 3 {
		t.Errorf("/Functions: expected 3 entries, got %d", len(funcs))
	}

	bounds := dictArray(t, d, "/Bounds")
	if len(bounds) != 2 {
		t.Errorf("/Bounds: expected 2 entries, got %d", len(bounds))
	}
	if arrayFloat(t, bounds, 0) != 0.25 {
		t.Errorf("/Bounds[0]: expected 0.25, got %v", arrayFloat(t, bounds, 0))
	}
	if arrayFloat(t, bounds, 1) != 0.75 {
		t.Errorf("/Bounds[1]: expected 0.75, got %v", arrayFloat(t, bounds, 1))
	}

	encode := dictArray(t, d, "/Encode")
	if len(encode) != 6 {
		t.Errorf("/Encode: expected 6 entries, got %d", len(encode))
	}
}

// PDF spec §7.10.4 requires /Bounds in a Type 3 stitching function to be
// strictly increasing. SVG allows duplicate stop offsets (sharp color
// transitions); we must bump duplicates by epsilon to satisfy the spec.
// Acrobat rejects files with non-monotonic bounds.
func TestBuildShadingFunction_DuplicateOffsets_BoundsStrictlyIncreasing(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0.0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 0.3, color: &Color{R: 0, G: 1, B: 0, A: 1}, opacity: 1},
		{offset: 0.7, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
		{offset: 0.7, color: &Color{R: 1, G: 1, B: 0, A: 1}, opacity: 1}, // duplicate
		{offset: 1.0, color: &Color{R: 1, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	d := shadingDict(t, fn)
	bounds := dictArray(t, d, "/Bounds")
	prev := 0.0
	for i := 0; i < len(bounds); i++ {
		b := arrayFloat(t, bounds, i)
		if b <= prev {
			t.Errorf("/Bounds[%d] = %v, not strictly greater than previous %v", i, b, prev)
		}
		prev = b
	}
}
