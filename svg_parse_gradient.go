// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGLinearGradient reads a <linearGradient> element. Caller has received the StartElement.
// On exit, the </linearGradient> end element has been consumed.
func parseSVGLinearGradient(d *xml.Decoder, start xml.StartElement) *svgLinearGradient {
	// SVG default: x1=0 y1=0 x2=1 y2=0 (objectBoundingBox units when units default).
	g := &svgLinearGradient{x2: 1}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x1":
			g.x1, _ = parseSVGLength(a.Value)
		case "y1":
			g.y1, _ = parseSVGLength(a.Value)
		case "x2":
			g.x2, _ = parseSVGLength(a.Value)
		case "y2":
			g.y2, _ = parseSVGLength(a.Value)
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
	g.stops = collectGradientStops(d)
	return g
}

// parseSVGRadialGradient reads a <radialGradient> element.
func parseSVGRadialGradient(d *xml.Decoder, start xml.StartElement) *svgRadialGradient {
	g := &svgRadialGradient{
		cx: 0.5, cy: 0.5, r: 0.5, // SVG defaults (in objectBoundingBox units)
	}
	hasFx, hasFy := false, false
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			g.cx, _ = parseSVGLength(a.Value)
		case "cy":
			g.cy, _ = parseSVGLength(a.Value)
		case "r":
			g.r, _ = parseSVGLength(a.Value)
		case "fx":
			g.fx, _ = parseSVGLength(a.Value)
			hasFx = true
		case "fy":
			g.fy, _ = parseSVGLength(a.Value)
			hasFy = true
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
	if !hasFx {
		g.fx = g.cx
	}
	if !hasFy {
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
					svg.gradients[id] = parseSVGLinearGradient(d, t)
				} else {
					_ = d.Skip()
				}
			case "radialGradient":
				if id != "" {
					svg.gradients[id] = parseSVGRadialGradient(d, t)
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
