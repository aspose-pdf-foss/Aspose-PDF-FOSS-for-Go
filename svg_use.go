// SPDX-License-Identifier: MIT

package asposepdf

// svgUse is a placeholder before resolveUseReferences (Task 6) replaces it
// with the cloned referent. After resolution, no *svgUse nodes remain in
// the IR tree.
type svgUse struct {
	refID     string
	x, y      float64
	style     svgStyle
	transform *svgMatrix
}

func (*svgUse) svgNodeKind() string { return "use" }

// svgSymbol is a container with its own viewBox. Not rendered directly;
// only referenced via <use href="#symbolId">.
type svgSymbol struct {
	viewBox  *svgViewBox
	children []svgNode
	style    svgStyle
}

func (*svgSymbol) svgNodeKind() string { return "symbol" }

// resolveUseReferences walks the IR tree, replacing each *svgUse with a deep
// clone of its referent (or dropping it if the ref is missing or cyclic).
// Called once at end of parseSVGRoot, after svg.defs is fully populated.
func resolveUseReferences(svg *SVG, node svgNode, visited map[string]bool) svgNode {
	switch n := node.(type) {
	case *svgUse:
		if visited[n.refID] {
			return nil // cycle — drop
		}
		target, ok := svg.defs[n.refID]
		if !ok || target == nil {
			return nil
		}
		visited[n.refID] = true
		cloned := deepCloneSVGNode(target)
		cloned = resolveUseReferences(svg, cloned, visited)
		delete(visited, n.refID)
		return wrapUseReferent(cloned, n)
	case *svgGroup:
		out := make([]svgNode, 0, len(n.children))
		for _, c := range n.children {
			resolved := resolveUseReferences(svg, c, visited)
			if resolved != nil {
				out = append(out, resolved)
			}
		}
		n.children = out
		return n
	case *svgSymbol:
		out := make([]svgNode, 0, len(n.children))
		for _, c := range n.children {
			resolved := resolveUseReferences(svg, c, visited)
			if resolved != nil {
				out = append(out, resolved)
			}
		}
		n.children = out
		return n
	}
	return node
}

// wrapUseReferent wraps the cloned referent in a group that applies the use's
// translation + transform + style as defaults.
func wrapUseReferent(referent svgNode, u *svgUse) svgNode {
	if referent == nil {
		return nil
	}
	// Composite matrix: translate(x, y) ∘ use.transform
	matrix := matrixTranslate(u.x, u.y)
	if u.transform != nil {
		matrix = matrixMul(matrix, *u.transform)
	}
	var transformPtr *svgMatrix
	if matrix != matrixIdentity() {
		transformPtr = &matrix
	}
	// If referent is a symbol, expand its children
	if sym, ok := referent.(*svgSymbol); ok {
		return &svgGroup{
			style:     u.style,
			children:  sym.children,
			transform: transformPtr,
		}
	}
	// Wrap any other referent in a group
	return &svgGroup{
		style:     u.style,
		children:  []svgNode{referent},
		transform: transformPtr,
	}
}

// deepCloneSVGNode returns a deep copy of the IR node. Slices are reallocated;
// styles and matrices are shallow-copied (they're value types or immutable).
func deepCloneSVGNode(n svgNode) svgNode {
	switch v := n.(type) {
	case *svgGroup:
		cloned := &svgGroup{transform: v.transform, style: v.style}
		cloned.children = make([]svgNode, len(v.children))
		for i, c := range v.children {
			cloned.children[i] = deepCloneSVGNode(c)
		}
		return cloned
	case *svgRect:
		cp := *v
		return &cp
	case *svgCircle:
		cp := *v
		return &cp
	case *svgEllipse:
		cp := *v
		return &cp
	case *svgLine:
		cp := *v
		return &cp
	case *svgPolyline:
		cp := *v
		cp.points = append([]Point(nil), v.points...)
		return &cp
	case *svgPolygon:
		cp := *v
		cp.points = append([]Point(nil), v.points...)
		return &cp
	case *svgPath:
		cp := *v
		cp.commands = append([]svgPathOp(nil), v.commands...)
		return &cp
	case *svgImage:
		cp := *v
		cp.data = append([]byte(nil), v.data...)
		return &cp
	case *svgText:
		cp := *v
		cp.runs = append([]svgTextRun(nil), v.runs...)
		return &cp
	case *svgSymbol:
		cloned := &svgSymbol{viewBox: v.viewBox, style: v.style}
		cloned.children = make([]svgNode, len(v.children))
		for i, c := range v.children {
			cloned.children[i] = deepCloneSVGNode(c)
		}
		return cloned
	}
	return n
}
