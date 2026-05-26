// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"strings"
)

// svgImage is the IR node for an SVG <image> element.
type svgImage struct {
	x, y, w, h float64
	par        svgPreserveAspect
	data       []byte
	format     ImageFormat
	style      svgStyle
	transform  *svgMatrix
}

func (*svgImage) svgNodeKind() string { return "image" }

// decodeSVGDataURI parses an SVG image href that is a base64-encoded data URI.
// Supports only data:image/png;base64,... and data:image/jpeg;base64,...
// (raw URL-encoded data is not supported).
// Returns ok=false for any other input shape.
func decodeSVGDataURI(s string) (data []byte, format ImageFormat, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return nil, 0, false
	}
	s = s[len(prefix):]
	semi := strings.IndexByte(s, ';')
	comma := strings.IndexByte(s, ',')
	if semi < 0 || comma < 0 || semi >= comma {
		return nil, 0, false
	}
	mime := strings.ToLower(strings.TrimSpace(s[:semi]))
	encodingAndData := s[semi+1:]
	if !strings.HasPrefix(encodingAndData, "base64,") {
		return nil, 0, false
	}
	encoded := encodingAndData[len("base64,"):]
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, 0, false
	}
	switch mime {
	case "image/png":
		return b, ImageFormatPNG, true
	case "image/jpeg", "image/jpg":
		return b, ImageFormatJPEG, true
	}
	return nil, 0, false
}
