// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// groupFixture builds a 100×100 page whose /XObject /Fm1 is a transparency
// group containing a red square (0,0)-(60,60) and an overlapping blue square
// (30,0)-(90,60), plus extra /Group entries merged from extra.
func groupFixture(t *testing.T, extraGroup pdfDict, content string) *Page {
	t.Helper()
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	group := pdfDict{"/S": pdfName("/Transparency")}
	for k, v := range extraGroup {
		group[k] = v
	}
	form := &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{0, 0, 100, 100},
			"/Group":     group,
			"/Resources": pdfDict{},
		},
		Data: []byte("1 0 0 rg 0 0 60 60 re f\n" +
			"0 0 1 rg 30 0 60 60 re f\n"),
		Decoded: true,
	}
	p.pageResources()["/XObject"] = pdfDict{"/Fm1": form}
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	return p
}

func px(t *testing.T, p *Page, x, y int) (r, g, b uint8) {
	t.Helper()
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	cr, cg, cb, _ := img.At(x, img.Bounds().Dy()-1-y).RGBA() // flip to PDF coords
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)
}

// TestGroupAlphaNoDoubleDarkening: drawing the group at /ca 0.5 must apply the
// alpha to the flattened result — in the red∕blue overlap the group is pure
// blue, so the page shows 50% blue over white: (128,128,255)-ish. The old
// inline path leaked red into the overlap (≈(128,64,191)).
func TestGroupAlphaNoDoubleDarkening(t *testing.T) {
	p := groupFixture(t, nil, "/GS1 gs /Fm1 Do\n")
	p.pageResources()["/ExtGState"] = pdfDict{"/GS1": pdfDict{"/ca": 0.5}}

	r, g, b := px(t, p, 45, 30) // inside the overlap
	if !(near(r, 128, 12) && near(g, 128, 12) && near(b, 255, 12)) {
		t.Errorf("overlap = (%d,%d,%d), want ≈(128,128,255): group alpha applied per element, not to the flattened group", r, g, b)
	}
	// Red-only region: 50% red over white.
	r, g, b = px(t, p, 15, 30)
	if !(near(r, 255, 12) && near(g, 128, 12) && near(b, 128, 12)) {
		t.Errorf("red region = (%d,%d,%d), want ≈(255,128,128)", r, g, b)
	}
}

// TestGroupOpaqueInlineUnchanged: with no group-level alpha/blend/mask the group
// draws inline (cheap path) and looks exactly like direct painting.
func TestGroupOpaqueInlineUnchanged(t *testing.T) {
	p := groupFixture(t, nil, "/Fm1 Do\n")
	if r, _, b := px(t, p, 45, 30); r > 40 || b < 200 {
		t.Errorf("opaque group overlap should be pure blue, got r=%d b=%d", r, b)
	}
	if r, _, _ := px(t, p, 15, 30); r < 200 {
		t.Errorf("opaque group red region lost: r=%d", r)
	}
}

// TestGroupKnockout: in a knockout group (/K true) the second square replaces
// the first where they overlap — drawing both at constant alpha 0.5 *inside*
// the group must leave the overlap pure 50%-blue (no red contribution), unlike
// normal accumulation.
func TestGroupKnockout(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)
	form := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0, 0, 100, 100},
			"/Group":   pdfDict{"/S": pdfName("/Transparency"), "/K": true},
			"/Resources": pdfDict{"/ExtGState": pdfDict{
				"/GS5": pdfDict{"/ca": 0.5},
			}},
		},
		Data: []byte("/GS5 gs 1 0 0 rg 0 0 60 60 re f\n" +
			"0 0 1 rg 30 0 60 60 re f\n"),
		Decoded: true,
	}
	p.pageResources()["/XObject"] = pdfDict{"/Fm1": form}
	if err := p.appendToContentStream([]byte("/Fm1 Do\n")); err != nil {
		t.Fatal(err)
	}

	// Overlap: the blue square (still at ca=0.5 — the gs persists) KNOCKED OUT
	// the red beneath it, so the group holds pure 50% blue there; over the
	// white page that is ≈(127,127,255). Regular src-over accumulation would
	// leave red showing through: ≈(127,64,191) — distinguishable in g and b.
	r, g, b := px(t, p, 45, 30)
	if !(near(r, 127, 12) && near(g, 127, 12) && near(b, 255, 12)) {
		t.Errorf("knockout overlap = (%d,%d,%d), want ≈(127,127,255) (top element replaces; no red contribution)", r, g, b)
	}
	// Red-only region keeps its 50% red over white.
	r, _, _ = px(t, p, 15, 30)
	if !near(r, 255, 12) {
		t.Errorf("knockout red region r=%d, want ≈255", r)
	}
}

func near(v uint8, want, tol int) bool {
	d := int(v) - want
	return d >= -tol && d <= tol
}
