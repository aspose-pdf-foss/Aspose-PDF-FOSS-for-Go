// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

func parseSVGMarker(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (*svgMarker, error) {
	// SVG spec defaults: markerWidth=3, markerHeight=3, markerUnits=strokeWidth, orient=0
	m := &svgMarker{markerW: 3, markerH: 3}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "viewBox":
			if vb, ok := parseViewBox(a.Value); ok {
				m.viewBox = &vb
			}
		case "refX":
			m.refX, _ = parseSVGLength(a.Value)
		case "refY":
			m.refY, _ = parseSVGLength(a.Value)
		case "markerWidth":
			m.markerW, _ = parseSVGLength(a.Value)
		case "markerHeight":
			m.markerH, _ = parseSVGLength(a.Value)
		case "markerUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				m.units = svgMarkerUserSpace
			}
		case "orient":
			v := strings.TrimSpace(a.Value)
			if v == "auto" || v == "auto-start-reverse" {
				m.orient.auto = true
			} else {
				// Strip optional "deg" suffix
				v = strings.TrimSuffix(v, "deg")
				v = strings.TrimSpace(v)
				if n, ok := parseSVGNumber(v); ok {
					m.orient.angle = n
				}
			}
		}
	}
	tmp := &svgGroup{style: parent.style}
	if err := parseSVGChildren(d, svg, tmp); err != nil {
		return nil, err
	}
	m.children = tmp.children
	return m, nil
}
