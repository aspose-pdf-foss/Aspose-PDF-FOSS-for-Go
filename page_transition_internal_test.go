// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// Negative values in another producer's file read as the defaults.
func TestTransitionReadsNegativeAsDefault(t *testing.T) {
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	pd := p.pageDict()
	pd["/Dur"] = -3
	pd["/Trans"] = pdfDict{"/S": pdfName("/Fly"), "/D": -2.0, "/SS": -1}
	if v := p.DisplayDuration(); v != 0 {
		t.Errorf("DisplayDuration = %v, want 0", v)
	}
	tr, ok := p.Transition()
	if !ok || tr.Style != TransitionFly || tr.Duration != 0 || tr.Scale != 0 {
		t.Errorf("Transition = %+v, %v", tr, ok)
	}
}
