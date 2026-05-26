// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGUse reads a <use> element into a placeholder. resolveUseReferences
// (Task 6, called post-parse) replaces the placeholder with the cloned referent.
func parseSVGUse(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	u := &svgUse{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			u.x, _ = parseSVGLength(a.Value)
		case "y":
			u.y, _ = parseSVGLength(a.Value)
		case "href":
			u.refID = strings.TrimPrefix(strings.TrimSpace(a.Value), "#")
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				u.transform = &m
			}
		}
	}
	applyStyleWithCSS(&u.style, start.Attr, svg, "use")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	if u.refID == "" {
		return nil, nil
	}
	return u, nil
}

// parseSVGSymbol reads a <symbol> element. Its children are parsed recursively.
// The symbol itself is stored in svg.defs (if it has id) but does NOT appear
// in the rendering tree.
func parseSVGSymbol(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	s := &svgSymbol{style: parent.style}
	for _, a := range start.Attr {
		if a.Name.Local == "viewBox" {
			if vb, ok := parseViewBox(a.Value); ok {
				s.viewBox = &vb
			}
		}
	}
	applyStyleWithCSS(&s.style, start.Attr, svg, "symbol")

	// Walk children into a temporary group
	tmpGroup := &svgGroup{style: s.style}
	if err := parseSVGChildren(d, svg, tmpGroup); err != nil {
		return nil, err
	}
	s.children = tmpGroup.children

	// Register in defs if it has an id
	if id := findAttr(start.Attr, "id"); id != "" {
		svg.defs[id] = s
	}
	// <symbol> is NOT added to the rendering tree.
	return nil, nil
}
