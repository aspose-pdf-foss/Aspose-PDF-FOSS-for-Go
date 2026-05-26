// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGClipPath reads a <clipPath> element. Children are parsed recursively
// via the standard shape parsers. The clipPath itself is NOT added to the
// rendering tree; if it has an id, the caller stores it in svg.defs.
func parseSVGClipPath(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (*svgClipPath, error) {
	cp := &svgClipPath{units: svgGradientUserSpace}
	for _, a := range start.Attr {
		if a.Name.Local == "clipPathUnits" {
			if strings.TrimSpace(a.Value) == "objectBoundingBox" {
				cp.units = svgGradientObjectBBox
			}
		}
	}
	tmp := &svgGroup{style: parent.style}
	if err := parseSVGChildren(d, svg, tmp); err != nil {
		return nil, err
	}
	cp.children = tmp.children
	return cp, nil
}
