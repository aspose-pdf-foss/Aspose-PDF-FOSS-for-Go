// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"math"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func transitionRoundTrip(t *testing.T, tr pdf.PageTransition) (pdf.PageTransition, []byte) {
	t.Helper()
	d := pdf.NewDocument(200, 200)
	p, _ := d.Page(1)
	if err := p.SetTransition(tr); err != nil {
		t.Fatal(err)
	}
	data := savedBytes(t, d)
	re, err := pdf.OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	got, ok := rp.Transition()
	if !ok {
		t.Fatal("transition lost on save")
	}
	return got, data
}

func TestTransitionEveryStyle(t *testing.T) {
	for s := pdf.TransitionReplace; s <= pdf.TransitionFade; s++ {
		if got, _ := transitionRoundTrip(t, pdf.PageTransition{Style: s}); got.Style != s {
			t.Errorf("style %d read back as %d", s, got.Style)
		}
	}
}

func TestTransitionParameters(t *testing.T) {
	for _, tr := range []pdf.PageTransition{
		{Style: pdf.TransitionSplit, Duration: 2.5, Vertical: true, Outward: true},
		{Style: pdf.TransitionBlinds, Vertical: true},
		{Style: pdf.TransitionBox, Outward: true},
		{Style: pdf.TransitionWipe, Direction: 270},
		{Style: pdf.TransitionGlitter, Direction: 315},
		{Style: pdf.TransitionFly, Outward: true, Direction: pdf.TransitionNoDirection, Scale: 0.5, Opaque: true},
		{Style: pdf.TransitionCover, Direction: 90},
		{Style: pdf.TransitionUncover, Direction: 180},
		{Style: pdf.TransitionPush, Direction: 90, Duration: 0.25},
	} {
		if got, _ := transitionRoundTrip(t, tr); got != tr {
			t.Errorf("transition %+v read back as %+v", tr, got)
		}
	}
}

func TestTransitionWritesOnlyWhatTheStyleUses(t *testing.T) {
	// A Dissolve has no dimension, motion, direction or scale.
	got, data := transitionRoundTrip(t, pdf.PageTransition{Style: pdf.TransitionDissolve,
		Vertical: true, Outward: true, Direction: 90, Scale: 2, Opaque: true})
	if got != (pdf.PageTransition{Style: pdf.TransitionDissolve}) {
		t.Errorf("read back %+v", got)
	}
	for _, key := range []string{"/Dm ", "/M ", "/Di ", "/SS ", "/B "} {
		if bytes.Contains(data, []byte(key)) {
			t.Errorf("Dissolve written with %s", key)
		}
	}
	if !bytes.Contains(data, []byte("/S /Dissolve")) {
		t.Error("no /S /Dissolve")
	}
}

func TestTransitionRejectsAndRemoves(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	p, _ := d.Page(1)
	if _, ok := p.Transition(); ok {
		t.Error("a new page has a transition")
	}
	for _, bad := range []pdf.PageTransition{
		{Style: pdf.TransitionStyle(12)}, {Style: -1}, {Direction: 45},
		{Duration: -1}, {Duration: math.NaN()}, {Scale: math.Inf(1)},
	} {
		if err := p.SetTransition(bad); err == nil {
			t.Errorf("SetTransition(%+v) accepted", bad)
		}
	}
	if _, ok := p.Transition(); ok {
		t.Error("a rejected transition was set")
	}
	_ = p.SetTransition(pdf.PageTransition{Style: pdf.TransitionFade})
	p.RemoveTransition()
	if _, ok := p.Transition(); ok {
		t.Error("RemoveTransition left it")
	}
}

func TestDisplayDuration(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	p, _ := d.Page(1)
	if p.DisplayDuration() != 0 {
		t.Error("a new page has a display duration")
	}
	if err := p.SetDisplayDuration(4.5); err != nil {
		t.Fatal(err)
	}
	re, _ := pdf.OpenStream(bytes.NewReader(savedBytes(t, d)))
	rp, _ := re.Page(1)
	if rp.DisplayDuration() != 4.5 {
		t.Errorf("DisplayDuration after reopen = %v", rp.DisplayDuration())
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := p.SetDisplayDuration(bad); err == nil {
			t.Errorf("SetDisplayDuration(%v) accepted", bad)
		}
	}
	_ = p.SetDisplayDuration(0)
	if bytes.Contains(savedBytes(t, d), []byte("/Dur")) {
		t.Error("SetDisplayDuration(0) left /Dur")
	}
}
