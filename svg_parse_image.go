// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
)

// parseSVGImage reads an <image> element. Returns nil if href is missing,
// external (not data:), or has unsupported MIME / bad base64.
// Caller has received the StartElement; on exit </image> has been consumed.
func parseSVGImage(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	im := &svgImage{
		style: parent.style,
		par:   parsePreserveAspect(""),
	}
	var href string
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			im.x, _ = parseSVGLength(a.Value)
		case "y":
			im.y, _ = parseSVGLength(a.Value)
		case "width":
			im.w, _ = parseSVGLength(a.Value)
		case "height":
			im.h, _ = parseSVGLength(a.Value)
		case "preserveAspectRatio":
			im.par = parsePreserveAspect(a.Value)
		case "href":
			href = a.Value
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				im.transform = &m
			}
		}
	}
	// Also accept xlink:href (legacy SVG attribute namespace)
	if href == "" {
		for _, a := range start.Attr {
			if a.Name.Local == "href" && a.Name.Space != "" {
				href = a.Value
				break
			}
		}
	}
	applyStyleWithCSS(&im.style, start.Attr, svg, "image")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	if href == "" {
		return nil, nil
	}
	data, format, ok := decodeSVGDataURI(href)
	if !ok {
		return nil, nil // best-effort: skip non-data or unsupported MIME
	}
	im.data = data
	im.format = format
	return im, nil
}
