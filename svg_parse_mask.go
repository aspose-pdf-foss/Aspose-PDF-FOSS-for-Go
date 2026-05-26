// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGMask reads a <mask> element. Children are parsed recursively
// via the standard shape parsers. The mask itself is NOT added to the
// rendering tree; if it has an id, the caller stores it in svg.defs.
func parseSVGMask(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (*svgMask, error) {
	// SVG spec defaults: maskUnits=objectBoundingBox, maskContentUnits=userSpaceOnUse
	m := &svgMask{
		units:        svgGradientObjectBBox,
		contentUnits: svgGradientUserSpace,
	}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "maskUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				m.units = svgGradientUserSpace
			}
		case "maskContentUnits":
			if strings.TrimSpace(a.Value) == "objectBoundingBox" {
				m.contentUnits = svgGradientObjectBBox
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
