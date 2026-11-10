// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestOpenActionBareDestination(t *testing.T) {
	d := NewDocument(200, 200)
	d.catalog = pdfDict{"/OpenAction": pdfArray{pdfRef{Num: d.pages[0].Num}, pdfName("/Fit")}}
	g, ok := d.OpenAction().(*GoToAction)
	if !ok {
		t.Fatalf("OpenAction = %#v", d.OpenAction())
	}
	if _, ok := g.Destination().(*DestinationFit); !ok || g.PageNum() != 1 {
		t.Errorf("destination %#v page %d", g.Destination(), g.PageNum())
	}
	d.catalog["/OpenAction"] = pdfArray{pdfName("/x")}
	if d.OpenAction() != nil {
		t.Error("a malformed destination read as an action")
	}
}
