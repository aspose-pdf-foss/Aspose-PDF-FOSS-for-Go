// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
)

// parseSVGText reads a <text> element with mixed content (CharData + <tspan>).
// Maintains a cursor advanced by each run's measured width.
// On exit, the </text> end element has been consumed.
func parseSVGText(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (*svgText, error) {
	style := parent.style
	applyStyleWithCSS(&style, start.Attr, svg, "text")

	t := &svgText{style: style}
	var cursor textCursor

	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			cursor.x, _ = parseSVGLength(a.Value)
		case "y":
			cursor.y, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				t.transform = &m
			}
		}
	}

	if err := walkSVGTextContent(d, &cursor, style, t); err != nil {
		return nil, err
	}
	return t, nil
}

// textCursor tracks the current text insertion position shared across
// all runs within a <text> element (including nested <tspan>s).
type textCursor struct {
	x, y float64
}

// walkSVGTextContent walks CharData and <tspan> children, emitting runs into t.runs.
// Advances the shared cursor by each run's measured width.
// Returns on the matching EndElement (</text> or </tspan>).
func walkSVGTextContent(d *xml.Decoder, cursor *textCursor, style svgStyle, t *svgText) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.CharData:
			text := normalizeSVGTextWhitespace(string(tt))
			if text == "" {
				continue
			}
			t.runs = append(t.runs, svgTextRun{
				text: text, x: cursor.x, y: cursor.y, style: style,
			})
			cursor.x += measureSVGTextWidth(text, style)
		case xml.StartElement:
			if tt.Name.Local != "tspan" {
				_ = d.Skip()
				continue
			}
			tspanStyle := style
			applySVGStyleAttrs(&tspanStyle, tt.Attr)
			// Apply abs x/y override or dx/dy offset BEFORE recursing.
			for _, a := range tt.Attr {
				switch a.Name.Local {
				case "x":
					if v, ok := parseSVGLength(a.Value); ok {
						cursor.x = v
					}
				case "y":
					if v, ok := parseSVGLength(a.Value); ok {
						cursor.y = v
					}
				case "dx":
					if v, ok := parseSVGLength(a.Value); ok {
						cursor.x += v
					}
				case "dy":
					if v, ok := parseSVGLength(a.Value); ok {
						cursor.y += v
					}
				}
			}
			if err := walkSVGTextContent(d, cursor, tspanStyle, t); err != nil {
				return err
			}
		}
	}
}
