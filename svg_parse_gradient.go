// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"math"
	"strings"
)

// gradAxis distinguishes gradient coordinates by how percent units resolve
// against the viewport when gradientUnits is userSpaceOnUse:
//
//	x-axis coords (cx, fx, x1, x2) scale by viewport width
//	y-axis coords (cy, fy, y1, y2) scale by viewport height
//	radius scales by sqrt((vw² + vh²) / 2) per SVG 1.1 §7.10
type gradAxis int

const (
	gradAxisX gradAxis = iota
	gradAxisY
	gradAxisR
)

// gradCoord holds a parsed gradient coordinate before it is resolved against
// the gradient's coordinate system. value is already 0..1 when isPct is true
// (parseSVGGradientLength normalises percent to fraction).
type gradCoord struct {
	value float64
	isPct bool
	set   bool
}

// resolve returns the final gradient-space coordinate. For objectBoundingBox
// a percent stays as a 0..1 fraction (the bbox matrix at render time scales
// it). For userSpaceOnUse a percent multiplies the appropriate viewport
// dimension. Unset coords return the supplied default (interpreted under the
// same rules as a parsed value).
func (c gradCoord) resolve(def float64, defIsPct bool, units svgGradientUnits, svg *SVG, axis gradAxis) float64 {
	v, pct := c.value, c.isPct
	if !c.set {
		v, pct = def, defIsPct
	}
	if !pct || units == svgGradientObjectBBox {
		return v
	}
	vw, vh := gradientViewportDims(svg)
	switch axis {
	case gradAxisX:
		return v * vw
	case gradAxisY:
		return v * vh
	case gradAxisR:
		return v * math.Sqrt((vw*vw+vh*vh)/2)
	}
	return v
}

// gradientViewportDims returns the dimensions used to resolve userSpaceOnUse
// percents — the closest SVG viewport. Prefers viewBox (in user units) over
// width/height attributes; falls back to 100x100 when neither is set.
func gradientViewportDims(svg *SVG) (w, h float64) {
	if svg != nil && svg.viewBox != nil {
		return svg.viewBox.w, svg.viewBox.h
	}
	if svg != nil && svg.width > 0 && svg.height > 0 {
		return svg.width, svg.height
	}
	return 100, 100
}

// parseSVGLinearGradient reads a <linearGradient> element. Caller has received the StartElement.
// On exit, the </linearGradient> end element has been consumed.
//
// Per SVG 1.1 §13.2.2 the default gradientUnits is objectBoundingBox; default
// coords are x1=0%, y1=0%, x2=100%, y2=0% (i.e. a horizontal gradient across
// the shape's bounding box).
func parseSVGLinearGradient(d *xml.Decoder, start xml.StartElement, svg *SVG) *svgLinearGradient {
	g := &svgLinearGradient{units: svgGradientObjectBBox}
	var x1c, y1c, x2c, y2c gradCoord
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x1":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				x1c = gradCoord{v, pct, true}
			}
		case "y1":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				y1c = gradCoord{v, pct, true}
			}
		case "x2":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				x2c = gradCoord{v, pct, true}
			}
		case "y2":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				y2c = gradCoord{v, pct, true}
			}
		case "gradientUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				g.units = svgGradientUserSpace
			} else {
				g.units = svgGradientObjectBBox
			}
		case "gradientTransform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				g.transform = &m
			}
		case "spreadMethod":
			// Phase 3a: only pad supported; reflect/repeat fall back silently
			g.spread = svgSpreadPad
		}
	}
	g.x1 = x1c.resolve(0, true, g.units, svg, gradAxisX)
	g.y1 = y1c.resolve(0, true, g.units, svg, gradAxisY)
	g.x2 = x2c.resolve(1, true, g.units, svg, gradAxisX)
	g.y2 = y2c.resolve(0, true, g.units, svg, gradAxisY)
	g.stops = collectGradientStops(d)
	return g
}

// parseSVGRadialGradient reads a <radialGradient> element.
//
// Per SVG 1.1 §13.2.3 the default gradientUnits is objectBoundingBox; default
// coords are cx=50%, cy=50%, r=50%, fx=cx, fy=cy.
func parseSVGRadialGradient(d *xml.Decoder, start xml.StartElement, svg *SVG) *svgRadialGradient {
	g := &svgRadialGradient{units: svgGradientObjectBBox}
	var cxc, cyc, rc, fxc, fyc gradCoord
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				cxc = gradCoord{v, pct, true}
			}
		case "cy":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				cyc = gradCoord{v, pct, true}
			}
		case "r":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				rc = gradCoord{v, pct, true}
			}
		case "fx":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				fxc = gradCoord{v, pct, true}
			}
		case "fy":
			if v, pct, ok := parseSVGGradientLength(a.Value); ok {
				fyc = gradCoord{v, pct, true}
			}
		case "gradientUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				g.units = svgGradientUserSpace
			} else {
				g.units = svgGradientObjectBBox
			}
		case "gradientTransform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				g.transform = &m
			}
		}
	}
	g.cx = cxc.resolve(0.5, true, g.units, svg, gradAxisX)
	g.cy = cyc.resolve(0.5, true, g.units, svg, gradAxisY)
	g.r = rc.resolve(0.5, true, g.units, svg, gradAxisR)
	// fx/fy default to cx/cy (already resolved).
	if fxc.set {
		g.fx = fxc.resolve(0, false, g.units, svg, gradAxisX)
	} else {
		g.fx = g.cx
	}
	if fyc.set {
		g.fy = fyc.resolve(0, false, g.units, svg, gradAxisY)
	} else {
		g.fy = g.cy
	}
	g.stops = collectGradientStops(d)
	return g
}

// collectGradientStops walks child elements until the end element of the gradient.
// Skips non-<stop> children silently.
func collectGradientStops(d *xml.Decoder) []svgGradientStop {
	var stops []svgGradientStop
	for {
		tok, err := d.Token()
		if err != nil {
			return stops
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return stops
		case xml.StartElement:
			if t.Name.Local == "stop" {
				stops = append(stops, parseSVGGradientStop(d, t))
			} else {
				_ = d.Skip()
			}
		}
	}
}

// findAttr looks up an XML attribute by local name.
func findAttr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// parseSVGDefs walks <defs> children, collecting gradient definitions into svg.gradients,
// symbol/clipPath into svg.defs, and any other id'd element into svg.defs.
// Returns once </defs> is consumed.
func parseSVGDefs(d *xml.Decoder, svg *SVG) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			id := findAttr(t.Attr, "id")
			switch t.Name.Local {
			case "linearGradient":
				if id != "" {
					svg.gradients[id] = parseSVGLinearGradient(d, t, svg)
				} else {
					_ = d.Skip()
				}
			case "radialGradient":
				if id != "" {
					svg.gradients[id] = parseSVGRadialGradient(d, t, svg)
				} else {
					_ = d.Skip()
				}
			case "symbol":
				_, _ = parseSVGSymbol(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
			case "clipPath":
				cp, err := parseSVGClipPath(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
				if err != nil {
					return err
				}
				if id != "" {
					svg.defs[id] = cp
				}
			case "mask":
				mask, err := parseSVGMask(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
				if err != nil {
					return err
				}
				if id != "" {
					svg.defs[id] = mask
				}
			case "filter":
				f, err := parseSVGFilter(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
				if err != nil {
					return err
				}
				if id != "" {
					svg.defs[id] = f
				}
			case "marker":
				mk, err := parseSVGMarker(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
				if err != nil {
					return err
				}
				if id != "" {
					svg.defs[id] = mk
				}
			default:
				if id != "" {
					// Generic element with id — parse via main dispatcher, store in defs
					child, err := parseSVGElement(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
					if err != nil {
						return err
					}
					if child != nil {
						svg.defs[id] = child
					}
				} else {
					_ = d.Skip()
				}
			}
		}
	}
}

// parseSVGGradientStop reads a <stop> element. Caller has already received the StartElement.
// On exit, the </stop> end element has been consumed.
func parseSVGGradientStop(d *xml.Decoder, start xml.StartElement) svgGradientStop {
	stop := svgGradientStop{
		color:   &Color{R: 0, G: 0, B: 0, A: 1},
		opacity: 1,
	}
	for _, a := range start.Attr {
		applyStopAttr(&stop, a.Name.Local, a.Value)
	}
	for _, a := range start.Attr {
		if a.Name.Local == "style" {
			for _, decl := range strings.Split(a.Value, ";") {
				kv := strings.SplitN(decl, ":", 2)
				if len(kv) != 2 {
					continue
				}
				applyStopAttr(&stop, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
			}
		}
	}
	_ = d.Skip()
	return stop
}

func applyStopAttr(s *svgGradientStop, name, val string) {
	switch name {
	case "offset":
		val = strings.TrimSpace(val)
		if strings.HasSuffix(val, "%") {
			n, ok := parseSVGNumber(strings.TrimSuffix(val, "%"))
			if ok {
				s.offset = n / 100
			}
		} else if n, ok := parseSVGNumber(val); ok {
			s.offset = n
		}
		s.offset = clamp01(s.offset)
	case "stop-color":
		if c, ok := parseSVGColor(val); ok && c != nil {
			s.color = c
		}
	case "stop-opacity":
		if n, ok := parseSVGNumber(val); ok {
			s.opacity = clamp01(n)
		}
	}
}
