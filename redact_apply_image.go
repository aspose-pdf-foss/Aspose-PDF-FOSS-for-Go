// SPDX-License-Identifier: MIT

package asposepdf

// rewriteImageOperatorsInStream removes XObject invocations (Do) whose
// painted bbox falls entirely inside any redact region, and wraps
// partially-overlapping invocations in a q...Q with an even-odd clip
// path that masks out the redacted area.
//
// Fully-outside Do operators are passed through unchanged. The CTM
// state machine tracks q/Q nesting and cm pre-multiplications.
// Operators other than q/Q/cm/Do are passed through unchanged.
func rewriteImageOperatorsInStream(data []byte, regions []QuadPoint) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	ctm := identityMatrix()
	var stack [][6]float64

	out := make([]contentOp, 0, len(ops))
	for _, op := range ops {
		switch op.Operator {
		case "q":
			stack = append(stack, ctm)
			out = append(out, op)
		case "Q":
			if len(stack) > 0 {
				ctm = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			out = append(out, op)
		case "cm":
			// cm operands: a b c d e f — set CTM = [a b c d e f] ∘ current_CTM
			// Per ISO 32000-1 §8.4.4: the new CTM = matrix_from_operands × old_CTM
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
			out = append(out, op)
		case "Do":
			action := classifyDoBbox(ctm, regions)
			switch action.kind {
			case keepDo:
				out = append(out, op)
			case dropDo:
				// skip emission — Do and its visual contribution are suppressed
			case clipDo:
				// Wrap Do in a local q...Q with an even-odd clip path that masks
				// out the redacted region(s) while keeping the unredacted area.
				out = append(out, contentOp{Operator: "q"})
				clipOps := buildImageClipPath(action.bbox, regions)
				out = append(out, clipOps...)
				out = append(out, op)
				out = append(out, contentOp{Operator: "Q"})
			}
		default:
			out = append(out, op)
		}
	}
	return serializeContentOps(out), nil
}

// doActionKind categorises what to do with a Do operator.
type doActionKind int

const (
	keepDo doActionKind = iota // no intersection — pass through
	dropDo                     // fully inside at least one region — suppress
	clipDo                     // partial overlap — clip the painting
)

// doAction is the classification result for a single Do operator.
type doAction struct {
	kind doActionKind
	bbox Rectangle // image bbox in page space (used for clipDo)
}

// classifyDoBbox computes the axis-aligned bbox of the unit-square XObject
// under the given CTM, then classifies its relationship to the redact regions.
//
// XObjects (both image and form) paint within [0,1]×[0,1] in their local
// space. We map the four corners through the CTM to get the page-space bbox.
func classifyDoBbox(ctm [6]float64, regions []QuadPoint) doAction {
	// Map unit-square corners through CTM.
	x0, y0 := matApplyPoint(ctm, 0, 0)
	x1, y1 := matApplyPoint(ctm, 1, 0)
	x2, y2 := matApplyPoint(ctm, 0, 1)
	x3, y3 := matApplyPoint(ctm, 1, 1)

	minX := minF(minF(x0, x1), minF(x2, x3))
	maxX := maxF(maxF(x0, x1), maxF(x2, x3))
	minY := minF(minF(y0, y1), minF(y2, y3))
	maxY := maxF(maxF(y0, y1), maxF(y2, y3))

	bbox := Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY}

	// Check each region's axis-aligned bbox against the image bbox.
	anyIntersect := false
	for _, q := range regions {
		rMinX, rMinY, rMaxX, rMaxY := boundsOfQuad(q)

		// No intersection if disjoint.
		if maxX <= rMinX || minX >= rMaxX || maxY <= rMinY || minY >= rMaxY {
			continue
		}
		anyIntersect = true

		// Fully inside: image bbox is entirely contained within this region.
		if minX >= rMinX && maxX <= rMaxX && minY >= rMinY && maxY <= rMaxY {
			return doAction{kind: dropDo, bbox: bbox}
		}
	}

	if !anyIntersect {
		return doAction{kind: keepDo, bbox: bbox}
	}
	return doAction{kind: clipDo, bbox: bbox}
}

// buildImageClipPath produces content ops for an even-odd clip path that
// preserves the image area EXCEPT where it overlaps with a redact region.
//
// Algorithm (ISO 32000-1 §8.5.3.3.2 even-odd rule):
//   - Outer rectangle = full image bbox
//   - For each overlapping region: add a rectangle for the intersection
//   - Under the even-odd rule, the inner rectangles are "holes" inside
//     the outer bbox — paint occurs in bbox minus the holes.
//
// The clip is followed by "n" (no-op path paint) to apply it without marking.
func buildImageClipPath(bbox Rectangle, regions []QuadPoint) []contentOp {
	var ops []contentOp

	// Outer rectangle: the full image bbox.
	ops = append(ops, reOp(bbox.LLX, bbox.LLY, bbox.URX-bbox.LLX, bbox.URY-bbox.LLY))

	// Inner rectangles: intersections of image bbox with each redact region.
	for _, q := range regions {
		rMinX, rMinY, rMaxX, rMaxY := boundsOfQuad(q)

		// Compute intersection of image bbox and region aabb.
		iMinX := maxF(bbox.LLX, rMinX)
		iMinY := maxF(bbox.LLY, rMinY)
		iMaxX := minF(bbox.URX, rMaxX)
		iMaxY := minF(bbox.URY, rMaxY)

		if iMaxX > iMinX && iMaxY > iMinY {
			ops = append(ops, reOp(iMinX, iMinY, iMaxX-iMinX, iMaxY-iMinY))
		}
	}

	// Even-odd clip + no-paint path operator.
	ops = append(ops, contentOp{Operator: "W*"})
	ops = append(ops, contentOp{Operator: "n"})
	return ops
}

// reOp builds a rectangle path operator: x y w h re
func reOp(x, y, w, h float64) contentOp {
	return contentOp{
		Operator: "re",
		Operands: []pdfValue{
			pdfValue(x),
			pdfValue(y),
			pdfValue(w),
			pdfValue(h),
		},
	}
}

// readCMMatrix parses the 6 operands of a cm operator into a [6]float64 matrix.
// Returns (matrix, true) on success, (zero, false) if operands are missing/invalid.
func readCMMatrix(operands []pdfValue) ([6]float64, bool) {
	if len(operands) < 6 {
		return [6]float64{}, false
	}
	var m [6]float64
	for i := 0; i < 6; i++ {
		switch v := operands[i].(type) {
		case float64:
			m[i] = v
		case int:
			m[i] = float64(v)
		default:
			return [6]float64{}, false
		}
	}
	return m, true
}

// boundsOfQuad returns the axis-aligned bounding box of a QuadPoint.
func boundsOfQuad(q QuadPoint) (minX, minY, maxX, maxY float64) {
	minX = minF(minF(q.X1, q.X2), minF(q.X3, q.X4))
	maxX = maxF(maxF(q.X1, q.X2), maxF(q.X3, q.X4))
	minY = minF(minF(q.Y1, q.Y2), minF(q.Y3, q.Y4))
	maxY = maxF(maxF(q.Y1, q.Y2), maxF(q.Y3, q.Y4))
	return
}
