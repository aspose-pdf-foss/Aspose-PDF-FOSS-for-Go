// SPDX-License-Identifier: MIT

package asposepdf

// rewritePathOperatorsInStream removes path-construction sequences whose
// bbox falls entirely inside any redact region, and wraps partially-
// overlapping paths in a q...Q clip ensuring they don't paint inside
// redacted areas. Fully-outside paths pass through unchanged.
//
// Path-construction ops (m, l, c, v, y, re, h) are buffered until a
// paint terminator (S, s, f, F, f*, B, B*, b, b*, n) is seen; the
// bbox check then applies to the accumulated path. The CTM state is
// tracked across q/Q/cm so bbox is computed in user-space.
func rewritePathOperatorsInStream(data []byte, regions []QuadPoint) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	ctm := identityMatrix()
	var ctmStack [][6]float64

	// pending holds buffered path-construction ops not yet emitted.
	var pending []contentOp
	// pathHasPoints is true once the first point has been added to the current path.
	var pathHasPoints bool
	// Accumulated user-space bounding box of the current path.
	var pathMinX, pathMinY, pathMaxX, pathMaxY float64

	out := make([]contentOp, 0, len(ops))

	// flushPending emits all buffered path-construction ops unchanged
	// (used when a non-path/non-paint op interrupts path construction,
	// or at end-of-stream if no paint terminator was encountered).
	flushPending := func() {
		out = append(out, pending...)
		pending = nil
		pathHasPoints = false
	}

	// addPoint expands the current path bbox to include the user-space
	// projection of the given path-construction-space point under the
	// current CTM.
	addPoint := func(x, y float64) {
		ux, uy := matApplyPoint(ctm, x, y)
		if !pathHasPoints {
			pathMinX, pathMaxX = ux, ux
			pathMinY, pathMaxY = uy, uy
			pathHasPoints = true
			return
		}
		pathMinX = minF(pathMinX, ux)
		pathMaxX = maxF(pathMaxX, ux)
		pathMinY = minF(pathMinY, uy)
		pathMaxY = maxF(pathMaxY, uy)
	}

	// classifyPath returns the action to take for the accumulated path
	// bbox versus the redact regions.
	classifyPath := func() doActionKind {
		if !pathHasPoints {
			return keepDo
		}
		return classifyBbox(pathMinX, pathMinY, pathMaxX, pathMaxY, regions)
	}

	isPaintOp := func(op string) bool {
		switch op {
		case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n":
			return true
		}
		return false
	}

	isPathConstructionOp := func(op string) bool {
		switch op {
		case "m", "l", "c", "v", "y", "re", "h":
			return true
		}
		return false
	}

	for _, op := range ops {
		switch op.Operator {

		case "q":
			// If there's a pending path being built, flush it unchanged
			// (well-formed PDFs don't issue q inside a path; handle defensively).
			if len(pending) > 0 {
				flushPending()
			}
			ctmStack = append(ctmStack, ctm)
			out = append(out, op)

		case "Q":
			if len(pending) > 0 {
				flushPending()
			}
			if len(ctmStack) > 0 {
				ctm = ctmStack[len(ctmStack)-1]
				ctmStack = ctmStack[:len(ctmStack)-1]
			}
			out = append(out, op)

		case "cm":
			if len(pending) > 0 {
				flushPending()
			}
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
			out = append(out, op)

		default:
			if isPathConstructionOp(op.Operator) {
				// Accumulate bbox and buffer the op.
				switch op.Operator {
				case "m":
					// moveto: x y m — sets current point
					if len(op.Operands) >= 2 {
						addPoint(operandFloat(op.Operands[0]), operandFloat(op.Operands[1]))
					}
				case "l":
					// lineto: x y l — line to point
					if len(op.Operands) >= 2 {
						addPoint(operandFloat(op.Operands[0]), operandFloat(op.Operands[1]))
					}
				case "c":
					// curveto: x1 y1 x2 y2 x3 y3 c — cubic bezier
					// Include all control points in bbox for safety.
					if len(op.Operands) >= 6 {
						addPoint(operandFloat(op.Operands[0]), operandFloat(op.Operands[1]))
						addPoint(operandFloat(op.Operands[2]), operandFloat(op.Operands[3]))
						addPoint(operandFloat(op.Operands[4]), operandFloat(op.Operands[5]))
					}
				case "v":
					// curveto (current as cp1): x2 y2 x3 y3 v
					if len(op.Operands) >= 4 {
						addPoint(operandFloat(op.Operands[0]), operandFloat(op.Operands[1]))
						addPoint(operandFloat(op.Operands[2]), operandFloat(op.Operands[3]))
					}
				case "y":
					// curveto (endpoint as cp2): x1 y1 x3 y3 y
					if len(op.Operands) >= 4 {
						addPoint(operandFloat(op.Operands[0]), operandFloat(op.Operands[1]))
						addPoint(operandFloat(op.Operands[2]), operandFloat(op.Operands[3]))
					}
				case "re":
					// rectangle: x y w h re — adds 4 corners
					if len(op.Operands) >= 4 {
						x := operandFloat(op.Operands[0])
						y := operandFloat(op.Operands[1])
						w := operandFloat(op.Operands[2])
						h := operandFloat(op.Operands[3])
						addPoint(x, y)
						addPoint(x+w, y)
						addPoint(x+w, y+h)
						addPoint(x, y+h)
					}
				case "h":
					// closepath: no new points added to bbox
				}
				pending = append(pending, op)

			} else if isPaintOp(op.Operator) {
				// Paint terminator: classify and emit.
				action := classifyPath()
				switch action {
				case keepDo:
					// Fully outside or no path: pass through unchanged.
					out = append(out, pending...)
					out = append(out, op)
				case dropDo:
					// Fully inside: drop both path-construction and paint ops.
					// (emit nothing)
				case clipDo:
					// Partial overlap: wrap in q...Q with even-odd clip.
					bbox := Rectangle{
						LLX: pathMinX, LLY: pathMinY,
						URX: pathMaxX, URY: pathMaxY,
					}
					out = append(out, contentOp{Operator: "q"})
					out = append(out, buildImageClipPath(bbox, regions)...)
					out = append(out, pending...)
					out = append(out, op)
					out = append(out, contentOp{Operator: "Q"})
				}
				// Reset path state.
				pending = nil
				pathHasPoints = false

			} else {
				// Non-path, non-paint op (text, color, Do, etc.).
				// Flush any pending path-construction ops unchanged (shouldn't
				// happen in well-formed PDFs but handle defensively).
				if len(pending) > 0 {
					flushPending()
				}
				out = append(out, op)
			}
		}
	}

	// At end-of-stream: any pending without a paint terminator → emit unchanged.
	if len(pending) > 0 {
		flushPending()
	}

	return serializeContentOps(out), nil
}

// classifyBbox classifies an axis-aligned bounding box (minX, minY, maxX, maxY)
// against the given redact regions and returns the appropriate doActionKind.
//
// Returns keepDo if the bbox doesn't intersect any region, dropDo if it's
// fully contained within a single region, or clipDo for partial overlap.
func classifyBbox(minX, minY, maxX, maxY float64, regions []QuadPoint) doActionKind {
	anyIntersect := false
	for _, q := range regions {
		rMinX, rMinY, rMaxX, rMaxY := boundsOfQuad(q)

		// No intersection if disjoint.
		if maxX <= rMinX || minX >= rMaxX || maxY <= rMinY || minY >= rMaxY {
			continue
		}
		anyIntersect = true

		// Fully inside: bbox is entirely contained within this region.
		if minX >= rMinX && maxX <= rMaxX && minY >= rMinY && maxY <= rMaxY {
			return dropDo
		}
	}

	if !anyIntersect {
		return keepDo
	}
	return clipDo
}
