// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// TestPostScriptFunction checks the Type 4 calculator: arithmetic and ifelse.
func TestPostScriptFunction(t *testing.T) {
	f := &fnPostScript{
		prog:     parsePSProgram([]byte("{ 2 mul 1 add }")), // y = 2x + 1
		domain:   []float64{0, 10},
		rangeArr: []float64{0, 100},
	}
	if got := f.eval([]float64{3}); len(got) != 1 || math.Abs(got[0]-7) > 1e-9 {
		t.Errorf("(2x+1)(3) = %v, want [7]", got)
	}

	step := &fnPostScript{
		prog:     parsePSProgram([]byte("{ 0.5 lt { 0 } { 1 } ifelse }")),
		domain:   []float64{0, 1},
		rangeArr: []float64{0, 1},
	}
	if got := step.eval([]float64{0.3}); got[0] != 0 {
		t.Errorf("step(0.3) = %v, want [0]", got)
	}
	if got := step.eval([]float64{0.7}); got[0] != 1 {
		t.Errorf("step(0.7) = %v, want [1]", got)
	}
}

// TestRenderSeparationColor fills with a Separation colour whose tint transform
// (a Type 4 function) maps tint t → RGB (t,0,0); tint 0.7 must render ~red.
func TestRenderSeparationColor(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	tintFn := &pdfStream{
		Dict: pdfDict{
			"/FunctionType": 4,
			"/Domain":       pdfArray{0, 1},
			"/Range":        pdfArray{0, 1, 0, 1, 0, 1},
		},
		Data:    []byte("{ 0 0 }"), // input tint stays as R; push G=0, B=0
		Decoded: true,
	}
	sep := pdfArray{pdfName("/Separation"), pdfName("/Spot1"), pdfName("/DeviceRGB"), tintFn}
	p.pageResources()["/ColorSpace"] = pdfDict{"/CS0": sep}

	if err := p.appendToContentStream([]byte("/CS0 cs\n0.7 scn\n0 0 100 100 re f\n")); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(50, 50).RGBA()
	if !(r>>8 > 160 && r>>8 < 200 && g>>8 < 40 && b>>8 < 40) {
		t.Errorf("Separation tint 0.7 = (%d,%d,%d), want ~(178,0,0)", r>>8, g>>8, b>>8)
	}
}
