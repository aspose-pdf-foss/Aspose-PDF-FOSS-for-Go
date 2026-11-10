// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func twoPages(t *testing.T) (*pdf.Document, *pdf.Page) {
	t.Helper()
	d := pdf.NewDocument(400, 600)
	if err := d.AddBlankPage(400, 600); err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(2)
	return d, p
}

func reopenedGoTo(t *testing.T, d *pdf.Document) *pdf.GoToAction {
	t.Helper()
	g, ok := reopenBytes(t, d).OpenAction().(*pdf.GoToAction)
	if !ok {
		t.Fatal("open action is not a GoTo after reopening")
	}
	return g
}

func TestOpenActionEveryDestinationKind(t *testing.T) {
	cases := []struct {
		build func(p *pdf.Page) pdf.Destination
		check func(t *testing.T, d pdf.Destination)
	}{
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationXYZ(p, 10, 500, 1.5) }, func(t *testing.T, d pdf.Destination) {
			x := d.(*pdf.DestinationXYZ)
			if x.Left() != 10 || x.Top() != 500 || x.Zoom() != 1.5 {
				t.Errorf("XYZ %v %v %v", x.Left(), x.Top(), x.Zoom())
			}
		}},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFit(p) }, func(t *testing.T, d pdf.Destination) { _ = d.(*pdf.DestinationFit) }},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitH(p, 100) }, func(t *testing.T, d pdf.Destination) {
			if d.(*pdf.DestinationFitH).Top() != 100 {
				t.Error("FitH top")
			}
		}},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitV(p, 100) }, func(t *testing.T, d pdf.Destination) {
			if d.(*pdf.DestinationFitV).Left() != 100 {
				t.Error("FitV left")
			}
		}},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitR(p, 1, 2, 3, 4) }, func(t *testing.T, d pdf.Destination) {
			r := d.(*pdf.DestinationFitR)
			if r.Left() != 1 || r.Bottom() != 2 || r.Right() != 3 || r.Top() != 4 {
				t.Error("FitR rectangle")
			}
		}},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitB(p) }, func(t *testing.T, d pdf.Destination) { _ = d.(*pdf.DestinationFitB) }},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitBH(p, 100) }, func(t *testing.T, d pdf.Destination) {
			if d.(*pdf.DestinationFitBH).Top() != 100 {
				t.Error("FitBH top")
			}
		}},
		{func(p *pdf.Page) pdf.Destination { return pdf.NewDestinationFitBVUnchanged(p) }, func(t *testing.T, d pdf.Destination) {
			if d.(*pdf.DestinationFitBV).HasLeft() {
				t.Error("FitBV left should stay unchanged")
			}
		}},
	}
	for _, c := range cases {
		d, p := twoPages(t)
		want := c.build(p)
		d.SetOpenAction(pdf.NewGoToDestinationAction(want))
		g := reopenedGoTo(t, d)
		got := g.Destination()
		if got == nil || got.DestinationType() != want.DestinationType() {
			t.Fatalf("destination %v read back as %v", want.DestinationType(), got)
		}
		if g.PageNum() != 2 || got.Page().Number() != 2 {
			t.Errorf("%v: page %d", want.DestinationType(), g.PageNum())
		}
		c.check(t, got)
	}
}

func TestGoToDestinationTopAndSetters(t *testing.T) {
	_, p := twoPages(t)
	a := pdf.NewGoToDestinationAction(pdf.NewDestinationFitH(p, 300))
	if a.PageNum() != 2 || a.Top() != 300 {
		t.Errorf("PageNum %d Top %v, want 2 and 300", a.PageNum(), a.Top())
	}
	if pdf.NewGoToDestinationAction(pdf.NewDestinationFit(p)).Top() != 0 {
		t.Error("a Fit destination has no top")
	}
	if pdf.NewGoToDestinationAction(pdf.NewDestinationFitR(p, 0, 0, 1, 77)).Top() != 77 {
		t.Error("FitR top")
	}
	a.SetTop(50) // back to page and top
	if a.Destination() != nil || a.PageNum() != 2 || a.Top() != 50 {
		t.Errorf("after SetTop: dest %v page %d top %v", a.Destination(), a.PageNum(), a.Top())
	}
	a.SetDestination(pdf.NewDestinationFitBH(p, 9))
	if a.Top() != 9 {
		t.Error("SetDestination did not take over Top")
	}
	a.SetPageNum(1)
	if a.Destination() != nil || a.PageNum() != 1 || a.Top() != 9 {
		t.Errorf("after SetPageNum: dest %v page %d top %v", a.Destination(), a.PageNum(), a.Top())
	}
}

func TestOpenActionNamedDestination(t *testing.T) {
	d, p := twoPages(t)
	if err := d.NamedDestinations().Add("chapter", pdf.NewDestinationFit(p)); err != nil {
		t.Fatal(err)
	}
	d.SetOpenAction(pdf.NewGoToDestinationAction(pdf.NewNamedDestination(d, "chapter")))
	g := reopenedGoTo(t, d)
	nd, ok := g.Destination().(*pdf.NamedDestination)
	if !ok || nd.Name() != "chapter" {
		t.Fatalf("destination %#v, want the name chapter", g.Destination())
	}
	if g.PageNum() != 2 {
		t.Errorf("named destination resolves to page %d, want 2", g.PageNum())
	}
}

func TestLinkKeepsItsDestinationKind(t *testing.T) {
	d, p := twoPages(t)
	first, _ := d.Page(1)
	link := pdf.NewLinkAnnotation(first, pdf.Rectangle{URX: 50, URY: 20})
	link.SetAction(pdf.NewGoToDestinationAction(pdf.NewDestinationFitV(p, 33)))
	if err := first.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	re := reopenBytes(t, d)
	rp, _ := re.Page(1)
	g := rp.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.GoToAction)
	// Saving it again keeps /FitV; it used to be rewritten as /XYZ.
	re2 := reopenBytes(t, re)
	rp2, _ := re2.Page(1)
	g2 := rp2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.GoToAction)
	for _, g := range []*pdf.GoToAction{g, g2} {
		fv, ok := g.Destination().(*pdf.DestinationFitV)
		if !ok || fv.Left() != 33 || g.PageNum() != 2 {
			t.Errorf("link destination %#v page %d", g.Destination(), g.PageNum())
		}
	}
}
