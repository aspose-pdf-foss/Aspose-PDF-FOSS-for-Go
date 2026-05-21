// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

func TestDestinationTypeConstants(t *testing.T) {
	all := []DestinationType{
		DestinationTypeXYZ,
		DestinationTypeFit,
		DestinationTypeFitH,
		DestinationTypeFitV,
		DestinationTypeFitR,
		DestinationTypeFitB,
		DestinationTypeFitBH,
		DestinationTypeFitBV,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("DestinationType[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestNewDestinationXYZ_AllExplicit(t *testing.T) {
	page := &Page{}
	d := NewDestinationXYZ(page, 100, 800, 1.5)
	if d.DestinationType() != DestinationTypeXYZ {
		t.Errorf("Type = %v", d.DestinationType())
	}
	if d.Page() != page {
		t.Error("Page mismatch")
	}
	if d.Left() != 100 || d.Top() != 800 || d.Zoom() != 1.5 {
		t.Errorf("coords: left=%v top=%v zoom=%v", d.Left(), d.Top(), d.Zoom())
	}
	if !d.HasLeft() || !d.HasTop() || !d.HasZoom() {
		t.Error("All Has* should be true when constructed with NewDestinationXYZ")
	}
}

func TestNewDestinationXYZUnchanged_PartialFields(t *testing.T) {
	page := &Page{}
	d := NewDestinationXYZUnchanged(page, 0, false, 800, true, 0, false)
	if d.HasLeft() || !d.HasTop() || d.HasZoom() {
		t.Errorf("Has*: L=%v T=%v Z=%v", d.HasLeft(), d.HasTop(), d.HasZoom())
	}
}

func TestNewDestinationFit(t *testing.T) {
	page := &Page{}
	d := NewDestinationFit(page)
	if d.DestinationType() != DestinationTypeFit {
		t.Error("wrong type")
	}
	if d.Page() != page {
		t.Error("Page mismatch")
	}
}

func TestNewDestinationFitH(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitH(page, 700)
	if d.DestinationType() != DestinationTypeFitH || d.Top() != 700 || !d.HasTop() {
		t.Errorf("FitH: type=%v top=%v hasTop=%v", d.DestinationType(), d.Top(), d.HasTop())
	}
}

func TestNewDestinationFitV(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitV(page, 50)
	if d.DestinationType() != DestinationTypeFitV || d.Left() != 50 || !d.HasLeft() {
		t.Errorf("FitV: type=%v left=%v hasLeft=%v", d.DestinationType(), d.Left(), d.HasLeft())
	}
}

func TestNewDestinationFitR(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitR(page, 10, 20, 100, 200)
	if d.DestinationType() != DestinationTypeFitR {
		t.Error("wrong type")
	}
	if d.Left() != 10 || d.Bottom() != 20 || d.Right() != 100 || d.Top() != 200 {
		t.Errorf("FitR coords: %v %v %v %v", d.Left(), d.Bottom(), d.Right(), d.Top())
	}
}

func TestNewDestinationFitB(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitB(page)
	if d.DestinationType() != DestinationTypeFitB {
		t.Error("wrong type")
	}
}

func TestNewDestinationFitBH(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitBH(page, 700)
	if d.DestinationType() != DestinationTypeFitBH || d.Top() != 700 {
		t.Errorf("FitBH: type=%v top=%v", d.DestinationType(), d.Top())
	}
}

func TestNewDestinationFitBV(t *testing.T) {
	page := &Page{}
	d := NewDestinationFitBV(page, 50)
	if d.DestinationType() != DestinationTypeFitBV || d.Left() != 50 {
		t.Errorf("FitBV: type=%v left=%v", d.DestinationType(), d.Left())
	}
}
