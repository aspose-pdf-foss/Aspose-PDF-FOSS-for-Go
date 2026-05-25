// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

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
