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
	svg := &SVG{root: &svgGroup{style: defaultSVGStyle()}}
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
	if err := parseSVGChildren(d, svg.root); err != nil {
		return nil, err
	}
	return svg, nil
}

func parseSVGChildren(d *xml.Decoder, parent *svgGroup) error {
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
			child, err := parseSVGElement(d, parent, t)
			if err != nil {
				return err
			}
			if child != nil {
				parent.children = append(parent.children, child)
			}
		}
	}
}

func parseSVGElement(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	switch start.Name.Local {
	case "g":
		g := &svgGroup{style: parent.style}
		applySVGStyleAttrs(&g.style, start.Attr)
		for _, a := range start.Attr {
			if a.Name.Local == "transform" {
				if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
					g.transform = &m
				}
			}
		}
		if err := parseSVGChildren(d, g); err != nil {
			return nil, err
		}
		return g, nil
	case "rect":
		return parseSVGRect(d, parent, start)
	case "circle":
		return parseSVGCircle(d, parent, start)
	case "ellipse":
		return parseSVGEllipse(d, parent, start)
	case "line":
		return parseSVGLine(d, parent, start)
	case "polyline":
		return parseSVGPolyline(d, parent, start, false)
	case "polygon":
		return parseSVGPolyline(d, parent, start, true)
	case "path":
		return parseSVGPath(d, parent, start)
	default:
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return nil, nil
	}
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
		if c, ok := parseSVGColor(val); ok {
			if c == nil {
				s.fill = nil
			} else {
				s.fill = &svgPaint{color: c}
			}
		}
	case "stroke":
		if c, ok := parseSVGColor(val); ok {
			if c == nil {
				s.stroke = nil
			} else {
				s.stroke = &svgPaint{color: c}
			}
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
	}
}

func parseSVGRect(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
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
	applySVGStyleAttrs(&r.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return r, nil
}

func parseSVGCircle(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
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
	applySVGStyleAttrs(&c.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return c, nil
}

func parseSVGEllipse(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
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
	applySVGStyleAttrs(&e.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return e, nil
}

func parseSVGLine(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
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
	applySVGStyleAttrs(&l.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return l, nil
}

func parseSVGPolyline(d *xml.Decoder, parent *svgGroup, start xml.StartElement, closed bool) (svgNode, error) {
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
		applySVGStyleAttrs(&p.style, start.Attr)
		return p, nil
	}
	p := &svgPolyline{points: points, style: parent.style, transform: transform}
	applySVGStyleAttrs(&p.style, start.Attr)
	return p, nil
}

func parseSVGPath(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
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
	applySVGStyleAttrs(&p.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	return p, nil
}
