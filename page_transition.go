// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
)

// TransitionStyle is the visual effect of a page transition in a presentation
// (the /S of a /Trans dictionary, ISO 32000-1 Table 162). Mirrors Aspose.PDF
// for .NET's transition styles (PdfPageEditor.TransitionType).
type TransitionStyle int

const (
	TransitionReplace  TransitionStyle = iota // the new page replaces the old one (the default)
	TransitionSplit                           // two lines sweep across the screen
	TransitionBlinds                          // several lines sweep, like window blinds
	TransitionBox                             // a rectangle grows or shrinks
	TransitionWipe                            // a single line sweeps in Direction
	TransitionDissolve                        // the old page dissolves into the new one
	TransitionGlitter                         // a dissolve sweeping in Direction
	TransitionFly                             // the page flies in or out
	TransitionPush                            // the new page pushes the old one off
	TransitionCover                           // the new page slides over the old one
	TransitionUncover                         // the old page slides off the new one
	TransitionFade                            // the new page fades in
)

var transitionStyleNames = [...]string{"/R", "/Split", "/Blinds", "/Box", "/Wipe", "/Dissolve",
	"/Glitter", "/Fly", "/Push", "/Cover", "/Uncover", "/Fade"}

// TransitionNoDirection as PageTransition.Direction is /Di /None: a Fly
// transition that only scales, with no direction of motion.
const TransitionNoDirection = -1

// PageTransition is the effect a viewer in presentation mode uses when it moves
// to a page (the page /Trans dictionary, ISO 32000-1 §12.4.4). The zero value is
// the PDF default: Replace, one second.
type PageTransition struct {
	Style    TransitionStyle
	Duration float64 // seconds (/D); 0 = the default of 1
	// Vertical sets /Dm /V for Split and Blinds; horizontal otherwise.
	Vertical bool
	// Outward sets /M /O for Split, Box and Fly: the motion runs from the
	// centre out rather than from the edges in.
	Outward bool
	// Direction (/Di) in degrees counterclockwise, for Wipe, Glitter, Fly,
	// Cover, Uncover and Push: 0 (left to right), 90 (bottom to top), 180,
	// 270, or 315 (top-left to bottom-right, Glitter only); or
	// TransitionNoDirection for Fly.
	Direction int
	// Scale (/SS) is where a Fly transition starts or ends, relative to the
	// page; 0 = the default of 1.
	Scale float64
	// Opaque (/B) makes the flying area of a Fly transition opaque.
	Opaque bool
}

func (t PageTransition) validate() error {
	if t.Style < TransitionReplace || int(t.Style) >= len(transitionStyleNames) {
		return fmt.Errorf("transition style %d is not defined", t.Style)
	}
	switch t.Direction {
	case 0, 90, 180, 270, 315, TransitionNoDirection:
	default:
		return fmt.Errorf("transition direction %d: use 0, 90, 180, 270, 315 or TransitionNoDirection", t.Direction)
	}
	for name, v := range map[string]float64{"duration": t.Duration, "scale": t.Scale} {
		if !(v >= 0) || math.IsInf(v, 0) {
			return fmt.Errorf("transition %s %v is not a non-negative number", name, v)
		}
	}
	return nil
}

func (t PageTransition) encode() pdfDict {
	d := pdfDict{"/Type": pdfName("/Trans"), "/S": pdfName(transitionStyleNames[t.Style])}
	if t.Duration > 0 {
		d["/D"] = t.Duration
	}
	switch t.Style {
	case TransitionSplit, TransitionBlinds:
		if t.Vertical {
			d["/Dm"] = pdfName("/V")
		}
	}
	switch t.Style {
	case TransitionSplit, TransitionBox, TransitionFly:
		if t.Outward {
			d["/M"] = pdfName("/O")
		}
	}
	switch t.Style {
	case TransitionWipe, TransitionGlitter, TransitionFly, TransitionCover, TransitionUncover, TransitionPush:
		if t.Direction == TransitionNoDirection {
			d["/Di"] = pdfName("/None")
		} else if t.Direction != 0 {
			d["/Di"] = t.Direction
		}
	}
	if t.Style == TransitionFly {
		if t.Scale > 0 && t.Scale != 1 {
			d["/SS"] = t.Scale
		}
		if t.Opaque {
			d["/B"] = true
		}
	}
	return d
}

func parseTransition(d pdfDict) PageTransition {
	var t PageTransition
	if s, ok := d["/S"].(pdfName); ok {
		for i, n := range transitionStyleNames {
			if string(s) == n {
				t.Style = TransitionStyle(i)
			}
		}
	}
	if v, err := toFloat(d["/D"]); err == nil && v > 0 {
		t.Duration = v
	}
	t.Vertical = d["/Dm"] == pdfName("/V")
	t.Outward = d["/M"] == pdfName("/O")
	if d["/Di"] == pdfName("/None") {
		t.Direction = TransitionNoDirection
	} else if v, err := toFloat(d["/Di"]); err == nil {
		t.Direction = int(v)
	}
	if v, err := toFloat(d["/SS"]); err == nil && v > 0 {
		t.Scale = v
	}
	t.Opaque, _ = d["/B"].(bool)
	return t
}

// Transition returns the effect a viewer in presentation mode uses when it
// moves to this page; ok is false when the page has none. Mirrors the
// transition of Aspose.PDF for .NET's PdfPageEditor.
func (p *Page) Transition() (t PageTransition, ok bool) {
	pd := p.pageDict()
	if pd == nil {
		return PageTransition{}, false
	}
	d, ok := resolveRefToDict(p.doc.objects, pd["/Trans"])
	if !ok {
		return PageTransition{}, false
	}
	return parseTransition(d), true
}

// SetTransition sets the page's presentation-mode transition. It errors on a
// style, direction, duration or scale PDF does not define.
func (p *Page) SetTransition(t PageTransition) error {
	if err := t.validate(); err != nil {
		return fmt.Errorf("SetTransition: %w", err)
	}
	pd := p.pageDict()
	if pd == nil {
		return fmt.Errorf("SetTransition: page has no dictionary")
	}
	pd["/Trans"] = t.encode()
	return nil
}

// RemoveTransition removes the page's transition.
func (p *Page) RemoveTransition() {
	if pd := p.pageDict(); pd != nil {
		delete(pd, "/Trans")
	}
}

// DisplayDuration returns how many seconds a viewer in presentation mode shows
// the page before advancing by itself (/Dur); 0 when it waits for the user.
// Mirrors Aspose.PDF for .NET's PdfPageEditor.DisplayDuration.
func (p *Page) DisplayDuration() float64 {
	pd := p.pageDict()
	if pd == nil {
		return 0
	}
	v, err := toFloat(pd["/Dur"])
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// SetDisplayDuration sets how many seconds the page is shown before a viewer in
// presentation mode advances; 0 removes it, so the viewer waits for the user.
func (p *Page) SetDisplayDuration(seconds float64) error {
	if !(seconds >= 0) || math.IsInf(seconds, 0) {
		return fmt.Errorf("SetDisplayDuration: %v is not a non-negative number of seconds", seconds)
	}
	pd := p.pageDict()
	if pd == nil {
		return fmt.Errorf("SetDisplayDuration: page has no dictionary")
	}
	if seconds == 0 {
		delete(pd, "/Dur")
	} else {
		pd["/Dur"] = seconds
	}
	return nil
}
