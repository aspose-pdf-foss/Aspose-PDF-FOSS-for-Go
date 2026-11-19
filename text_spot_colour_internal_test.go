// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// spotColourPage has a Separation colour space /CS1 whose tint 1 is
// RGB (0, 0.5, 1) and tint 0 white.
func spotColourPage(t *testing.T, content string) *Document {
	t.Helper()
	objs := pageWith("", "", "<< /Font << /F1 5 0 R >> /ColorSpace << /CS1 6 0 R >> >>", content)
	objs = append(objs,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"[/Separation /Brand /DeviceRGB << /FunctionType 2 /Domain [0 1] /C0 [1 1 1] /C1 [0 0.5 1] /N 1 >>]",
	)
	return openHardening(t, hardeningPDF(objs...))
}

func colourNear(c *Color, r, g, b float64) bool {
	return c != nil && math.Abs(c.R-r) < 0.01 && math.Abs(c.G-g) < 0.01 && math.Abs(c.B-b) < 0.01
}

// Text filled in a Separation colour reads as the colour its tint transform
// gives, so an edit that redraws it (underline) keeps that colour instead of
// turning it white — gray 1 — and so invisible (pdf-go-n0c5).
func TestSpotColourTextKeepsColourWhenEdited(t *testing.T) {
	d := spotColourPage(t, "q BT /F1 20 Tf 1 0 0 1 10 150 Tm /CS1 cs 1 scn (Brand) Tj ET Q\n"+
		"q /CS1 cs Q BT /F1 20 Tf 1 0 0 1 10 100 Tm 0 scn (Plain) Tj ET")
	p := mustPage(t, d)
	frs := foundFragments(t, p, "Brand")
	if len(frs) != 1 || !colourNear(frs[0].TextState.ForegroundColor, 0, 0.5, 1) {
		t.Fatalf("spot colour read as %+v", frs)
	}
	// Q restores the device gray space, where 0 is black.
	if pl := foundFragments(t, p, "Plain"); len(pl) != 1 || !colourNear(pl[0].TextState.ForegroundColor, 0, 0, 0) {
		t.Fatalf("after Q: %+v", pl[0].TextState.ForegroundColor)
	}

	frs[0].TextState.Underline = true
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	got := foundFragments(t, mustPage(t, reopenDoc(t, d)), "Brand")
	if len(got) != 1 || !colourNear(got[0].TextState.ForegroundColor, 0, 0.5, 1) {
		t.Fatalf("after the edit: %+v", got)
	}
}
