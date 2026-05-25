// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
)

// parseSVGText reads a <text> element. Caller has received the StartElement.
// On exit, the </text> end element has been consumed.
//
// Phase 3b Task 4: handles single-line <text> with text content (no nested <tspan>
// — that's Task 5; nested elements are skipped silently here).
func parseSVGText(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (*svgText, error) {
	style := parent.style
	applySVGStyleAttrs(&style, start.Attr)

	t := &svgText{style: style}
	var x, y float64

	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			x, _ = parseSVGLength(a.Value)
		case "y":
			y, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				t.transform = &m
			}
		}
	}

	var sb []byte
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch tt := tok.(type) {
		case xml.EndElement:
			text := normalizeSVGTextWhitespace(string(sb))
			if text != "" {
				t.runs = append(t.runs, svgTextRun{
					text:  text,
					x:     x,
					y:     y,
					style: style,
				})
			}
			return t, nil
		case xml.CharData:
			sb = append(sb, tt...)
		case xml.StartElement:
			// Phase 3b Task 5 handles <tspan>; for now, just skip nested elements.
			_ = d.Skip()
		}
	}
}
