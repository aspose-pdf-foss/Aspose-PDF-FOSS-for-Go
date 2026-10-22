// SPDX-License-Identifier: MIT

package asposepdf

// Pattern-resource pruning after redaction (pdf-go-ifik).
//
// A tiling pattern's cell is its own content stream — it can hold text,
// paths, images — and it is reached from a page only through the page's
// /Resources/Pattern dict, by name, as a fill or stroke colour
// ("/Pattern cs /P0 scn"). When every shape filled with it lies inside a
// redaction region, the path rewriter drops those shapes and the pattern
// never paints anything on the page again — but its object stays
// reachable through /Resources/Pattern, so RemoveUnusedObjects keeps it
// and the cell's content is still in the saved file: the same class of
// leak as pdf-go-ye1z was for Image XObjects.
//
// The name alone is not enough to decide: the path rewriter removes a
// path's construction and painting operators but leaves the colour
// operators before it, so "/P0 scn" survives even when nothing is ever
// painted with /P0 again. neededPatternNames therefore tracks the
// current fill and stroke pattern through the graphics state (q/Q, cs/CS,
// sc/scn/SC/SCN, the device-colour operators) and counts a pattern as
// needed only when a painting operator actually runs under it.
//
// Scope: the page's own /Resources/Pattern. A pattern used only inside a
// Form XObject that carries its own /Resources is pruned there by nothing
// — the form's own resources dict still names it — so that case stays a
// documented residual. A shape only PARTLY inside a region keeps its
// pattern; the cell repeats the same content everywhere, so what is
// clipped away is also visible outside the region.

// patternPaintOps are the operators that paint with the current fill
// and/or stroke colour. Do and BI are included because a Form XObject
// inherits the colour (and paints with it), and a stencil image mask
// paints with the current fill colour (ISO 32000-1 §8.9.6.2). Text-showing
// operators paint with it too (in any rendering mode but 3 and 7, which
// are not worth distinguishing for a conservative check).
var patternPaintOps = map[string]bool{
	"f": true, "F": true, "f*": true,
	"S": true, "s": true,
	"B": true, "B*": true, "b": true, "b*": true,
	"Tj": true, "TJ": true, "'": true, "\"": true,
	"Do": true, "BI": true,
}

// patternColourState is the part of the graphics state this scan tracks:
// the pattern name selected as the fill / stroke colour, or "" when the
// current colour is not a pattern.
type patternColourState struct {
	fill, stroke string
}

// neededPatternNames adds to needed every /Pattern resource name that some
// painting operator in data actually paints with, starting from state
// start. For a Form XObject with no own /Resources — which resolves its
// names, patterns included, through the same resources dict (ISO 32000-1
// §7.8.3) — it recurses into the form's content with the state in effect
// at its Do. Capped by depth, a cycle guard and the shared work budget;
// hitting a cap marks everything currently selected as needed and stops,
// since an under-report here would delete a pattern something still
// paints with, while an over-report only leaves one pattern unreclaimed.
func neededPatternNames(data []byte, objects map[int]*pdfObject, resources pdfDict, start patternColourState, depth int, budget *formRedactBudget, needed map[string]bool, visited map[*pdfStream]bool) bool {
	ops, err := parseContentStream(data)
	if err != nil {
		return false
	}
	cur := start
	var stack []patternColourState
	mark := func() {
		if cur.fill != "" {
			needed[cur.fill] = true
		}
		if cur.stroke != "" {
			needed[cur.stroke] = true
		}
	}
	for _, op := range ops {
		switch op.Operator {
		case "q":
			stack = append(stack, cur)
		case "Q":
			if n := len(stack); n > 0 {
				cur = stack[n-1]
				stack = stack[:n-1]
			}
		case "scn":
			cur.fill = trailingPatternName(op.Operands)
		case "SCN":
			cur.stroke = trailingPatternName(op.Operands)
		case "sc", "cs", "g", "rg", "k":
			cur.fill = ""
		case "SC", "CS", "G", "RG", "K":
			cur.stroke = ""
		}
		if !patternPaintOps[op.Operator] {
			continue
		}
		mark()
		if op.Operator != "Do" || len(op.Operands) == 0 {
			continue
		}
		st := resolveXObjectStream(objects, resources, operandName(op.Operands[0]))
		if st == nil || dictGetName(st.Dict, "/Subtype") != "/Form" {
			continue
		}
		if _, hasOwn := st.Dict["/Resources"]; hasOwn {
			continue // its own /Resources — its pattern names are not ours
		}
		if visited[st] {
			// Already scanned: the patterns it selects itself do not depend
			// on the state it starts from, and the inherited selection was
			// just marked above, at its Do.
			continue
		}
		if depth+1 >= maxFormRedactDepth || budget.consume() != nil {
			return false
		}
		visited[st] = true
		if !neededPatternNames(decodedStreamData(st), objects, resources, cur, depth+1, budget, needed, visited) {
			return false
		}
	}
	return true
}

// trailingPatternName returns the name operand that ends an scn/SCN
// operand list (the pattern selector), or "" when the colour is numeric.
func trailingPatternName(operands []pdfValue) string {
	if len(operands) == 0 {
		return ""
	}
	if n, ok := operands[len(operands)-1].(pdfName); ok {
		return string(n)
	}
	return ""
}

// prunePagePatterns removes from the page's /Resources/Pattern every entry
// that nothing on the page paints with any more: not the final page
// content (data), not a Form XObject that inherits these resources, not an
// annotation appearance stream without its own /Resources. It installs a
// fresh, pruned copy of the /Pattern dict rather than editing the original
// in place — the original may be shared with other pages, or with a Form
// XObject clone built earlier in this pass, which must keep resolving it.
//
// It prunes nothing when it cannot be sure: a content stream it fails to
// parse, a cap hit during the walk, or a Type3 font without its own
// /Resources among the page's fonts (its glyph procedures would resolve
// pattern names through the page's resources, and this scan does not
// follow them).
func prunePagePatterns(p *Page, data []byte, resources pdfDict, budget *formRedactBudget) {
	objects := p.doc.objects
	patterns, ok := resolveRefToDict(objects, resources["/Pattern"])
	if !ok || len(patterns) == 0 {
		return
	}
	if fonts, ok := resolveRefToDict(objects, resources["/Font"]); ok {
		for _, v := range fonts {
			fd, ok := resolveRefToDict(objects, v)
			if !ok {
				continue
			}
			if dictGetName(fd, "/Subtype") == "/Type3" {
				if _, hasOwn := fd["/Resources"]; !hasOwn {
					return
				}
			}
		}
	}

	needed := map[string]bool{}
	if !neededPatternNames(data, objects, resources, patternColourState{}, 0, budget, needed, map[*pdfStream]bool{}) {
		return
	}
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		apDict, ok := resolveRefToDict(objects, coll.At(i).annotationBaseRef().dict["/AP"])
		if !ok {
			continue
		}
		for _, key := range []string{"/N", "/D", "/R"} {
			v, ok := apDict[key]
			if !ok {
				continue
			}
			for _, st := range appearanceStreams(objects, v) {
				if _, hasOwn := st.Dict["/Resources"]; hasOwn {
					continue
				}
				if !neededPatternNames(decodedStreamData(st), objects, resources, patternColourState{}, 0, budget, needed, map[*pdfStream]bool{}) {
					return
				}
			}
		}
	}

	pruned := make(pdfDict, len(patterns))
	for name, v := range patterns {
		if needed[name] {
			pruned[name] = v
		}
	}
	if len(pruned) == len(patterns) {
		return
	}
	resources["/Pattern"] = pruned
}
