// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// rewriteImageOperatorsInStream removes Image XObject invocations (Do)
// whose painted bbox falls entirely inside any redact region, and wraps
// partially-overlapping invocations in a q...Q with an even-odd clip
// path that masks out the redacted area.
//
// A Form XObject (which, unlike an Image XObject, can itself contain
// arbitrary text/path content — the very thing this feature exists to
// remove) whose placed bbox intersects any region at all, fully or
// partially, is refused rather than silently dropped/clipped: dropping
// or clipping only the Do invocation leaves the form's own content
// stream untouched and fully recoverable, since nothing in this
// redaction pipeline recurses into a Form XObject's content (see
// unsafeFormXObjectName). Confirmed reachable through this library's own
// AddForm/NUp/Booklet/PdfPageStamp — all build on Form XObjects — by an
// independent review of pdf-go-2qkq's fix. pdf-go-w5hl tracks the real
// fix (recursing into Form XObject content, CTM-aware, per placement).
//
// Fully-outside Do operators are passed through unchanged. The CTM
// state machine tracks q/Q nesting and cm pre-multiplications.
// Operators other than q/Q/cm/Do are passed through unchanged.
func rewriteImageOperatorsInStream(data []byte, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}
	if name, found := unsafeFormXObjectName(ops, regions, objects, resources); found {
		return nil, fmt.Errorf("asposepdf: ApplyRedactions: a redaction region intersects Form XObject %q; redacting a Form XObject's own content is not supported yet (pdf-go-w5hl) — flatten it into page content first, or keep redaction regions clear of it", name)
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

// classifyDoBbox computes the axis-aligned bbox of the unit-square Image
// XObject under the given CTM, then classifies its relationship to the
// redact regions. Image space is always the unit square [0,1]×[0,1] (ISO
// 32000-1 §8.9.5.1) — this must NOT be reused for a Form XObject, whose
// real painted extent is its own /BBox + /Matrix, not the unit square; see
// formPlacementBbox.
func classifyDoBbox(ctm [6]float64, regions []QuadPoint) doAction {
	x0, y0 := matApplyPoint(ctm, 0, 0)
	x1, y1 := matApplyPoint(ctm, 1, 0)
	x2, y2 := matApplyPoint(ctm, 0, 1)
	x3, y3 := matApplyPoint(ctm, 1, 1)
	minX := minF(minF(x0, x1), minF(x2, x3))
	maxX := maxF(maxF(x0, x1), maxF(x2, x3))
	minY := minF(minF(y0, y1), minF(y2, y3))
	maxY := maxF(maxF(y0, y1), maxF(y2, y3))
	return doAction{
		kind: classifyBbox(minX, minY, maxX, maxY, regions),
		bbox: Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY},
	}
}

// formPlacementBbox computes the real page-space axis-aligned bbox of a
// Form XObject placed with the given CTM: its own /BBox (ISO 32000-1
// §8.10.2 — any rectangle, NOT necessarily the unit square an Image
// XObject always uses) composed with its own /Matrix (§8.10.1, default
// identity, applied in form space before ctm). Reusing classifyDoBbox's
// unit-square assumption for a Form XObject is wrong and was shown, by an
// independent review, to silently defeat unsafeFormXObjectName for the
// common case of a non-trivial placement scale or a redaction region that
// doesn't happen to cover the form's CTM-origin corner (pdf-go-w5hl).
// Falls back to the unit square only if /BBox is missing or malformed —
// conservatively wrong in the same direction classifyDoBbox always was,
// not a new gap.
// ok is false when /BBox can't be resolved to at least 4 numbers — ISO
// 32000-1 §8.10.2 requires /BBox on every Form XObject, so its absence
// means non-conformant or adversarially-malformed input, exactly what a
// redaction-safety check should trust least. An earlier draft guessed the
// unit square in that case, which an independent review flagged as its
// own false-negative risk (the same "guess a safe-looking value instead
// of refusing" mistake that produced the two bugs before it in this
// chain) — callers must treat ok==false as unsafe outright, not fall back
// to any estimate.
func formPlacementBbox(objects map[int]*pdfObject, st *pdfStream, ctm [6]float64) (minX, minY, maxX, maxY float64, ok bool) {
	bb := shFloats(objects, st.Dict["/BBox"])
	if len(bb) < 4 {
		return 0, 0, 0, 0, false
	}
	bbox := Rectangle{LLX: bb[0], LLY: bb[1], URX: bb[2], URY: bb[3]}

	formMatrix := identityMatrix()
	if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
		formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
	}
	effective := matMul(formMatrix, ctm)

	x0, y0 := matApplyPoint(effective, bbox.LLX, bbox.LLY)
	x1, y1 := matApplyPoint(effective, bbox.URX, bbox.LLY)
	x2, y2 := matApplyPoint(effective, bbox.LLX, bbox.URY)
	x3, y3 := matApplyPoint(effective, bbox.URX, bbox.URY)
	minX = minF(minF(x0, x1), minF(x2, x3))
	maxX = maxF(maxF(x0, x1), maxF(x2, x3))
	minY = minF(minF(y0, y1), minF(y2, y3))
	maxY = maxF(maxF(y0, y1), maxF(y2, y3))
	return minX, minY, maxX, maxY, true
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

// resolveXObjectStream resolves name within resources' /XObject dict to
// its stream object, or nil if it can't be resolved.
func resolveXObjectStream(objects map[int]*pdfObject, resources pdfDict, name string) *pdfStream {
	if resources == nil {
		return nil
	}
	xobjDict, ok := resolveRefToDict(objects, resources["/XObject"])
	if !ok {
		return nil
	}
	ref, ok := xobjDict[name]
	if !ok {
		return nil
	}
	st, _ := resolveRef(objects, ref).(*pdfStream)
	return st
}

// unsafeFormXObjectName scans ops (CTM-aware, same q/Q/cm tracking as
// rewriteImageOperatorsInStream) for a Do invocation of a Form XObject
// whose REAL placed bbox — its own /BBox+/Matrix via formPlacementBbox,
// not the unit square classifyDoBbox uses for images — intersects any
// region, fully or partially. An earlier version of this check reused
// classifyDoBbox directly, which silently cleared almost every realistic
// partial-coverage placement (anything but a region covering the form's
// CTM-origin corner) — caught by an independent review. Shared by
// ValidateRedactions (read-only pre-flight) and rewriteImageOperatorsInStream
// (which refuses to proceed rather than silently drop/clip it) — see the
// package comment on rewriteImageOperatorsInStream for why a Form XObject
// needs this and an Image XObject doesn't.
func unsafeFormXObjectName(ops []contentOp, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict) (string, bool) {
	if len(regions) == 0 {
		return "", false
	}
	ctm := identityMatrix()
	var stack [][6]float64
	for _, op := range ops {
		switch op.Operator {
		case "q":
			stack = append(stack, ctm)
		case "Q":
			if len(stack) > 0 {
				ctm = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "cm":
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
		case "Do":
			if len(op.Operands) == 0 {
				continue
			}
			name := operandName(op.Operands[0])
			st := resolveXObjectStream(objects, resources, name)
			if st == nil || dictGetName(st.Dict, "/Subtype") != "/Form" {
				continue
			}
			minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, ctm)
			if !ok || classifyBbox(minX, minY, maxX, maxY, regions) != keepDo {
				return name, true
			}
		}
	}
	return "", false
}
