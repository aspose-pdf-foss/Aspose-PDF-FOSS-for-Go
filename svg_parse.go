// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func parseSVGBytes(data []byte) (*SVG, error) {
	return parseSVGReader(strings.NewReader(string(data)))
}

func parseSVGReader(r io.Reader) (*SVG, error) {
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			return nil, errors.New("svg: no <svg> root element")
		}
		if err != nil {
			return nil, fmt.Errorf("svg: xml parse: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "svg" {
			return nil, fmt.Errorf("svg: expected <svg> root, got <%s>", start.Name.Local)
		}
		return parseSVGRoot(decoder, start)
	}
}

func parseSVGRoot(d *xml.Decoder, start xml.StartElement) (*SVG, error) {
	svg := &SVG{
		root:      &svgGroup{style: defaultSVGStyle()},
		gradients: make(map[string]svgGradient),
		defs:      make(map[string]svgNode),
		cssRules:  nil,
	}
	hasParAttr := false
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "width":
			if v, ok := parseSVGLength(a.Value); ok {
				svg.width = v
			}
		case "height":
			if v, ok := parseSVGLength(a.Value); ok {
				svg.height = v
			}
		case "viewBox":
			if vb, ok := parseViewBox(a.Value); ok {
				svg.viewBox = &vb
			}
		case "preserveAspectRatio":
			svg.par = parsePreserveAspect(a.Value)
			hasParAttr = true
		}
	}
	if !hasParAttr {
		svg.par = parsePreserveAspect("")
	}
	applySVGStyleAttrs(&svg.root.style, start.Attr)
	if err := parseSVGChildren(d, svg, svg.root); err != nil {
		return nil, err
	}
	// Resolve all <use> references — replaces *svgUse nodes with deep-cloned referents.
	visited := make(map[string]bool)
	_ = resolveUseReferences(svg, svg.root, visited)
	return svg, nil
}

func parseSVGChildren(d *xml.Decoder, svg *SVG, parent *svgGroup) error {
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return errors.New("svg: unexpected EOF")
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			child, err := parseSVGElement(d, svg, parent, t)
			if err != nil {
				return err
			}
			if child != nil {
				parent.children = append(parent.children, child)
			}
		}
	}
}

func parseSVGElement(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	switch start.Name.Local {
	case "style":
		var content strings.Builder
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			if _, ok := tok.(xml.EndElement); ok {
				break
			}
			if cd, ok := tok.(xml.CharData); ok {
				content.Write(cd)
			}
		}
		svg.cssRules = append(svg.cssRules, parseSVGCSS(content.String())...)
		return nil, nil
	case "g":
		g := &svgGroup{style: parent.style}
		applyStyleWithCSS(&g.style, start.Attr, svg, "g")
		for _, a := range start.Attr {
			if a.Name.Local == "transform" {
				if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
					g.transform = &m
				}
			}
		}
		if err := parseSVGChildren(d, svg, g); err != nil {
			return nil, err
		}
		return g, nil
	case "rect":
		return parseSVGRect(d, svg, parent, start)
	case "circle":
		return parseSVGCircle(d, svg, parent, start)
	case "ellipse":
		return parseSVGEllipse(d, svg, parent, start)
	case "line":
		return parseSVGLine(d, svg, parent, start)
	case "polyline":
		return parseSVGPolyline(d, svg, parent, start, false)
	case "polygon":
		return parseSVGPolyline(d, svg, parent, start, true)
	case "path":
		return parseSVGPath(d, svg, parent, start)
	case "defs":
		return nil, parseSVGDefs(d, svg)
	case "linearGradient":
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.gradients[id] = parseSVGLinearGradient(d, start)
		} else {
			_ = d.Skip()
		}
		return nil, nil
	case "radialGradient":
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.gradients[id] = parseSVGRadialGradient(d, start)
		} else {
			_ = d.Skip()
		}
		return nil, nil
	case "use":
		return parseSVGUse(d, svg, parent, start)
	case "symbol":
		return parseSVGSymbol(d, svg, parent, start)
	case "text":
		return parseSVGText(d, svg, parent, start)
	case "image":
		return parseSVGImage(d, svg, parent, start)
	case "clipPath":
		cp, err := parseSVGClipPath(d, svg, parent, start)
		if err != nil {
			return nil, err
		}
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.defs[id] = cp
		}
		return nil, nil
	case "mask":
		mask, err := parseSVGMask(d, svg, parent, start)
		if err != nil {
			return nil, err
		}
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.defs[id] = mask
		}
		return nil, nil
	case "filter":
		f, err := parseSVGFilter(d, svg, parent, start)
		if err != nil {
			return nil, err
		}
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.defs[id] = f
		}
		return nil, nil
	case "marker":
		mk, err := parseSVGMarker(d, svg, parent, start)
		if err != nil {
			return nil, err
		}
		if id := findAttr(start.Attr, "id"); id != "" {
			svg.defs[id] = mk
		}
		return nil, nil
	default:
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// applyStyleWithCSS captures class/id from attrs, applies any matching CSS rules
// from svg.cssRules, then applies presentation attrs + inline style (which override CSS).
// Pass svg=nil to skip CSS matching (fallback to legacy behavior).
func applyStyleWithCSS(s *svgStyle, attrs []xml.Attr, svg *SVG, elementType string) {
	// Capture class/id first so matchSVGCSS can use them
	for _, a := range attrs {
		switch a.Name.Local {
		case "class":
			s.cssClasses = strings.Fields(a.Value)
		case "id":
			s.cssID = a.Value
		}
	}
	if svg != nil && len(svg.cssRules) > 0 {
		matchSVGCSS(s, svg.cssRules, elementType)
	}
	applySVGStyleAttrs(s, attrs)
}

func applySVGStyleAttrs(s *svgStyle, attrs []xml.Attr) {
	for _, a := range attrs {
		applySingleSVGStyleProp(s, a.Name.Local, a.Value)
	}
	for _, a := range attrs {
		if a.Name.Local == "style" {
			for _, decl := range strings.Split(a.Value, ";") {
				kv := strings.SplitN(decl, ":", 2)
				if len(kv) != 2 {
					continue
				}
				applySingleSVGStyleProp(s, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
			}
		}
	}
}

func applySingleSVGStyleProp(s *svgStyle, prop, val string) {
	switch prop {
	case "fill":
		if p, ok := parseSVGPaint(val); ok {
			s.fill = p
		}
	case "stroke":
		if p, ok := parseSVGPaint(val); ok {
			s.stroke = p
		}
	case "stroke-width":
		if v, ok := parseSVGLength(val); ok {
			s.strokeWidth = v
		}
	case "opacity":
		if v, ok := parseSVGNumber(val); ok {
			s.opacity = clamp01(v)
		}
	case "fill-opacity":
		if v, ok := parseSVGNumber(val); ok {
			s.fillOpacity = clamp01(v)
		}
	case "stroke-opacity":
		if v, ok := parseSVGNumber(val); ok {
			s.strokeOpacity = clamp01(v)
		}
	case "fill-rule":
		v := strings.ToLower(strings.TrimSpace(val))
		if v == "nonzero" || v == "evenodd" {
			s.fillRule = v
		}
	case "display":
		s.display = !(strings.TrimSpace(val) == "none")
	case "visibility":
		s.display = !(strings.TrimSpace(val) == "hidden")
	case "stroke-linecap":
		switch strings.TrimSpace(val) {
		case "round":
			s.lineCap = LineCapRound
		case "square":
			s.lineCap = LineCapSquare
		default:
			s.lineCap = LineCapButt
		}
	case "stroke-linejoin":
		switch strings.TrimSpace(val) {
		case "round":
			s.lineJoin = LineJoinRound
		case "bevel":
			s.lineJoin = LineJoinBevel
		default:
			s.lineJoin = LineJoinMiter
		}
	case "stroke-dasharray":
		if val == "none" || val == "" {
			s.dashArray = nil
			return
		}
		if nums, ok := parseSVGNumberList(val); ok {
			s.dashArray = nums
		}
	case "stroke-dashoffset":
		if v, ok := parseSVGLength(val); ok {
			s.dashOffset = v
		}
	case "stroke-miterlimit":
		if v, ok := parseSVGNumber(val); ok {
			s.miterLimit = v
		}
	case "font-family":
		s.fontFamily = strings.TrimSpace(val)
	case "font-size":
		if v, ok := parseSVGLength(val); ok {
			s.fontSize = v
		}
	case "font-weight":
		v := strings.TrimSpace(strings.ToLower(val))
		if v == "bold" || v == "bolder" {
			s.bold = true
		} else if v == "normal" || v == "lighter" {
			s.bold = false
		} else if n, ok := parseSVGNumber(v); ok {
			s.bold = n >= 600
		}
	case "font-style":
		v := strings.TrimSpace(strings.ToLower(val))
		s.italic = v == "italic" || v == "oblique"
	case "text-anchor":
		switch strings.TrimSpace(strings.ToLower(val)) {
		case "middle":
			s.anchor = svgTextAnchorMiddle
		case "end":
			s.anchor = svgTextAnchorEnd
		default:
			s.anchor = svgTextAnchorStart
		}
	case "clip-path":
		v := strings.TrimSpace(val)
		if v == "none" || v == "" {
			s.clipPath = ""
		} else if strings.HasPrefix(v, "url(") {
			end := strings.IndexByte(v, ')')
			if end > 0 {
				id := strings.Trim(v[4:end], "# \t")
				s.clipPath = id
			}
		}
	case "mask":
		v := strings.TrimSpace(val)
		if v == "none" || v == "" {
			s.mask = ""
		} else if strings.HasPrefix(v, "url(") {
			end := strings.IndexByte(v, ')')
			if end > 0 {
				id := strings.Trim(v[4:end], "# \t")
				s.mask = id
			}
		}
	case "filter":
		v := strings.TrimSpace(val)
		if v == "none" || v == "" {
			s.filter = ""
		} else if strings.HasPrefix(v, "url(") {
			end := strings.IndexByte(v, ')')
			if end > 0 {
				id := strings.Trim(v[4:end], "# \t")
				s.filter = id
			}
		}
	}
}

func parseSVGRect(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	r := &svgRect{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			r.x, _ = parseSVGLength(a.Value)
		case "y":
			r.y, _ = parseSVGLength(a.Value)
		case "width":
			r.w, _ = parseSVGLength(a.Value)
		case "height":
			r.h, _ = parseSVGLength(a.Value)
		case "rx":
			r.rx, _ = parseSVGLength(a.Value)
		case "ry":
			r.ry, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				r.transform = &m
			}
		}
	}
	applyStyleWithCSS(&r.style, start.Attr, svg, "rect")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return r, nil
}

func parseSVGCircle(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	c := &svgCircle{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			c.cx, _ = parseSVGLength(a.Value)
		case "cy":
			c.cy, _ = parseSVGLength(a.Value)
		case "r":
			c.r, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				c.transform = &m
			}
		}
	}
	applyStyleWithCSS(&c.style, start.Attr, svg, "circle")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return c, nil
}

func parseSVGEllipse(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	e := &svgEllipse{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			e.cx, _ = parseSVGLength(a.Value)
		case "cy":
			e.cy, _ = parseSVGLength(a.Value)
		case "rx":
			e.rx, _ = parseSVGLength(a.Value)
		case "ry":
			e.ry, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				e.transform = &m
			}
		}
	}
	applyStyleWithCSS(&e.style, start.Attr, svg, "ellipse")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return e, nil
}

func parseSVGLine(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	l := &svgLine{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x1":
			l.x1, _ = parseSVGLength(a.Value)
		case "y1":
			l.y1, _ = parseSVGLength(a.Value)
		case "x2":
			l.x2, _ = parseSVGLength(a.Value)
		case "y2":
			l.y2, _ = parseSVGLength(a.Value)
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				l.transform = &m
			}
		}
	}
	applyStyleWithCSS(&l.style, start.Attr, svg, "line")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return l, nil
}

func parseSVGPolyline(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement, closed bool) (svgNode, error) {
	var points []Point
	var transform *svgMatrix
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "points":
			nums, _ := parseSVGNumberList(a.Value)
			for i := 0; i+1 < len(nums); i += 2 {
				points = append(points, Point{X: nums[i], Y: nums[i+1]})
			}
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				transform = &m
			}
		}
	}
	if err := d.Skip(); err != nil {
		return nil, err
	}
	if closed {
		p := &svgPolygon{points: points, style: parent.style, transform: transform}
		applyStyleWithCSS(&p.style, start.Attr, svg, "polygon")
		return p, nil
	}
	p := &svgPolyline{points: points, style: parent.style, transform: transform}
	applyStyleWithCSS(&p.style, start.Attr, svg, "polyline")
	return p, nil
}

func parseSVGPath(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	p := &svgPath{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "d":
			ops, err := parseSVGPathData(a.Value)
			if err != nil {
				_ = d.Skip()
				return nil, nil
			}
			p.commands = ops
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				p.transform = &m
			}
		}
	}
	applyStyleWithCSS(&p.style, start.Attr, svg, "path")
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return p, nil
}
