// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// maxFormRedactDepth bounds Form-XObject recursion while redacting into
// one, the same way maxFormDepth (text.go) bounds it during text
// extraction — a self-referential or pathologically deep nesting stops
// recursing rather than looping or blowing the stack; the Do invocation
// at that depth is left unredacted, same pre-existing nested-form blind
// spot text extraction already has, not a new one.
const maxFormRedactDepth = 32

// rewriteImageOperatorsInStream removes Image XObject invocations (Do)
// whose painted bbox falls entirely inside any redact region, wraps
// partially-overlapping invocations in a q...Q with an even-odd clip
// path that masks out the redacted area, and — this is the function's
// other half, despite the name kept for minimal API churn — recurses a
// redaction pass into a Form XObject's own content when its placement
// intersects a region at all (see rewriteXObjectDoOperatorsInStream and
// redactFormXObject for how; pdf-go-w5hl, the real fix for a v0.11.0
// safety stopgap that used to refuse outright here instead).
//
// Fully-outside Do operators are passed through unchanged. The CTM
// state machine tracks q/Q nesting and cm pre-multiplications.
// Operators other than q/Q/cm/Do are passed through unchanged.
func rewriteImageOperatorsInStream(data []byte, regions []QuadPoint, doc *Document, resources pdfDict) ([]byte, error) {
	return rewriteXObjectDoOperatorsInStream(data, regions, doc, resources, 0)
}

// rewriteXObjectDoOperatorsInStream is rewriteImageOperatorsInStream with
// an explicit recursion depth, so redactFormXObject can call back into it
// for a Form XObject's own content at depth+1 (nested forms) while the
// top-level, page-content call starts at depth 0.
func rewriteXObjectDoOperatorsInStream(data []byte, regions []QuadPoint, doc *Document, resources pdfDict, depth int) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}

	ctm := identityMatrix()
	var stack [][6]float64

	// replacedFormNames/survivingFormNames track, across every Do
	// invocation in THIS content stream, which original /XObject
	// resource names (not objects — a name is local to this one
	// resources dict) were redirected to a redacted clone at least once,
	// and which still have at least one occurrence passing through
	// unchanged (the SAME name used more than once, only some of which
	// need redaction). A name that was replaced everywhere it appeared
	// in this stream and never survives unchanged is pruned from the
	// resources dict after the loop — otherwise the dict keeps pointing
	// at the original, now content-stream-unreferenced form object,
	// which keeps it graph-reachable and un-collectable by
	// RemoveUnusedObjects, defeating the whole point of redacting it.
	// Caught by an independent review of this fix's first draft, which
	// only ever ADDED the clone's new name, never removing the old one.
	replacedFormNames := map[string]bool{}
	survivingFormNames := map[string]bool{}

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
			if len(op.Operands) == 0 {
				out = append(out, op)
				continue
			}
			name := operandName(op.Operands[0])
			st := resolveXObjectStream(objects, resources, name)
			if st != nil && dictGetName(st.Dict, "/Subtype") == "/Form" {
				newOp, replaced, err := redactFormDo(doc, st, ctm, regions, resources, depth, op)
				if err != nil {
					return nil, err
				}
				if replaced {
					replacedFormNames[name] = true
				} else {
					survivingFormNames[name] = true
				}
				out = append(out, newOp)
				continue
			}

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

	// A name redirected to a clone at least once, with no occurrence
	// surviving unchanged anywhere in this stream, no longer has any Do
	// operator in the rewritten output referencing it — remove it from
	// the resources dict so the original form object it pointed to loses
	// this reference and RemoveUnusedObjects can actually reclaim it.
	for name := range replacedFormNames {
		if survivingFormNames[name] {
			continue
		}
		if xobjDict, ok := resolveRefToDict(objects, resources["/XObject"]); ok {
			delete(xobjDict, name)
		}
	}

	return serializeContentOps(out), nil
}

// redactFormDo decides what to do with one Do invocation of a Form
// XObject: pass it through unchanged when its placement (formPlacementBbox,
// using the form's own /BBox+/Matrix, not an Image XObject's unit square)
// doesn't intersect any region at all, refuse (error) when the placement
// can't be trusted (bad/missing /BBox) or its matrix can't be inverted to
// map regions into the form's content, or redirect it to a freshly cloned,
// redacted copy of the form when redactFormXObject actually changed
// something.
//
// A clone, never an in-place mutation of the original form object: the
// same Form XObject can be Do-invoked from several placements — on this
// page or another, the same stream shared via indirect reference, think a
// logo stamped on every page — and only some of those placements may
// overlap a redaction region. Rewriting the shared content stream in
// place would blank it everywhere; redirecting only THIS Do operand to a
// private copy (a fresh object number, a fresh resource name scoped to
// this one invocation) leaves every other reference to the original
// object — this stream, on this page or any other — completely
// untouched. Confirmed necessary by an independent review of this
// function's first draft, which redacted the shared stream directly.
// The bool return reports whether this Do operand was redirected to a
// new, private clone (true) or passed through referencing the original
// name unchanged (false) — the caller tallies this per resource name
// across the whole content stream, since the SAME name can appear more
// than once (see replacedFormNames/survivingFormNames in the caller).
func redactFormDo(doc *Document, st *pdfStream, ctm [6]float64, regions []QuadPoint, resources pdfDict, depth int, original contentOp) (contentOp, bool, error) {
	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}
	minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, ctm)
	if !ok {
		name := ""
		if len(original.Operands) > 0 {
			name = operandName(original.Operands[0])
		}
		return contentOp{}, false, fmt.Errorf("asposepdf: ApplyRedactions: Form XObject %q has no resolvable /BBox; cannot safely determine whether a redaction region intersects it", name)
	}
	if classifyBbox(minX, minY, maxX, maxY, regions) == keepDo {
		return original, false, nil
	}

	newData, changed, err := redactFormXObject(doc, st, resources, ctm, regions, depth+1)
	if err != nil {
		return contentOp{}, false, err
	}
	if !changed {
		return original, false, nil
	}

	newID := doc.nextID
	doc.nextID++
	newDict := make(pdfDict, len(st.Dict))
	for k, v := range st.Dict {
		newDict[k] = v
	}
	newDict["/Length"] = len(newData)
	// /Filter + /DecodeParms described the OLD (possibly compressed)
	// bytes; newData is raw/decoded, same convention as
	// replacePageContents — the writer re-Flates on output.
	delete(newDict, "/Filter")
	delete(newDict, "/DecodeParms")
	doc.objects[newID] = &pdfObject{Num: newID, Value: &pdfStream{
		Dict:    newDict,
		Data:    newData,
		Decoded: true,
	}}

	xobjDict, ok := resolveRefToDict(doc.objects, resources["/XObject"])
	if !ok {
		return contentOp{}, false, fmt.Errorf("asposepdf: ApplyRedactions: internal error: Form XObject resolved but its /XObject resources dict did not")
	}
	newName := nextFormXObjectResourceName(xobjDict)
	xobjDict[newName] = pdfRef{Num: newID}

	return contentOp{Operator: "Do", Operands: []pdfValue{pdfName(newName)}}, true, nil
}

// nextFormXObjectResourceName returns the next available /Fm* resource
// name in xobjDict — the same convention flatten.go's registerFormXObject
// uses for freshly-created Form XObjects.
func nextFormXObjectResourceName(xobjDict pdfDict) string {
	for i := 0; ; i++ {
		name := fmt.Sprintf("/Fm%d", i)
		if _, exists := xobjDict[name]; !exists {
			return name
		}
	}
}

// redactFormXObject recurses a redaction pass into a Form XObject's own
// content stream for the region(s) that intersect it. Regions are given
// in page space (the same space every other redaction entry point uses);
// this maps them into the form's LOCAL content-stream coordinate space by
// applying the INVERSE of the placement matrix in effect for this specific
// Do invocation — the form's own /Matrix composed with ctm, the same
// composition formPlacementBbox uses to go the other direction. Once the
// regions are in the form's own space, the form's content is just another
// content stream: the same three rewriters process it (this function
// recurses into rewriteXObjectDoOperatorsInStream at depth+1 for a nested
// Form XObject, so one level of nesting — or more, up to
// maxFormRedactDepth — is handled, not only the top one).
//
// Returns the redacted bytes and changed=true only if they actually
// differ from the original; the caller (redactFormDo) skips cloning
// entirely when nothing changed (the coarse placement bbox intersected a
// region, but nothing the form actually draws was inside it once mapped
// precisely).
func redactFormXObject(doc *Document, st *pdfStream, parentResources pdfDict, ctm [6]float64, regions []QuadPoint, depth int) (newData []byte, changed bool, err error) {
	if depth >= maxFormRedactDepth {
		return nil, false, nil
	}

	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}

	formMatrix := identityMatrix()
	if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
		formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
	}
	composed := matMul(formMatrix, ctm)
	inv, ok := invertMatrix(composed)
	if !ok {
		return nil, false, fmt.Errorf("asposepdf: ApplyRedactions: a Form XObject's placement matrix is not invertible; cannot map a redaction region into its content")
	}

	localRegions := make([]QuadPoint, len(regions))
	for i, q := range regions {
		localRegions[i] = transformQuad(q, inv)
	}

	formResources := parentResources
	if resVal, ok := st.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(objects, resVal); ok {
			formResources = rd
		}
	}

	original := decodedStreamData(st)
	fontMap := resolveFontResources(objects, formResources)

	data, err := rewriteTextOperatorsInStream(original, localRegions, fontMap)
	if err != nil {
		return nil, false, err
	}
	data, err = rewriteXObjectDoOperatorsInStream(data, localRegions, doc, formResources, depth)
	if err != nil {
		return nil, false, err
	}
	data, err = rewritePathOperatorsInStream(data, localRegions)
	if err != nil {
		return nil, false, err
	}

	if string(data) == string(original) {
		return nil, false, nil
	}
	return data, true, nil
}

// transformQuad maps every corner of q through the affine matrix m.
func transformQuad(q QuadPoint, m [6]float64) QuadPoint {
	x1, y1 := matApplyPoint(m, q.X1, q.Y1)
	x2, y2 := matApplyPoint(m, q.X2, q.Y2)
	x3, y3 := matApplyPoint(m, q.X3, q.Y3)
	x4, y4 := matApplyPoint(m, q.X4, q.Y4)
	return QuadPoint{X1: x1, Y1: y1, X2: x2, Y2: y2, X3: x3, Y3: y3, X4: x4, Y4: y4}
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

// unsafeFormXObjectName is ValidateRedactions's read-only pre-flight
// counterpart to redactFormDo/redactFormXObject: it reports a Form
// XObject placement that intersects a redaction region AND cannot be
// safely redacted into — a missing/malformed /BBox (formPlacementBbox's
// own ok=false), or a placement matrix that composes to something
// non-invertible (so a region can't be mapped into the form's local
// content-stream space at all). A placement that intersects but CAN be
// redacted into — the common case, since pdf-go-w5hl — is no longer
// reported; only before that fix did any intersection count as unsafe.
//
// Recurses into a safely-resolvable form's own content at the SAME
// depth redactFormXObject would, so a problem several levels of nesting
// down (a nested Form XObject with its own bad /BBox, say) is caught
// here too, before ApplyRedactions attempts any mutation — mirroring
// redactFormXObject's own recursion exactly (same formPlacementBbox/
// invertMatrix checks, same region-transform-via-inverse-matrix, same
// maxFormRedactDepth cap) is deliberate: ApplyRedactions's own mutation
// path is clone-based and never corrupts anything in place even if an
// unvalidated deeper level later fails and surfaces as its own error, so
// this check's only job is to give the caller advance warning, not to
// gate correctness.
func unsafeFormXObjectName(ops []contentOp, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict) (string, bool) {
	if len(regions) == 0 {
		return "", false
	}
	return unsafeFormXObjectNameDepth(ops, regions, objects, resources, 0)
}

func unsafeFormXObjectNameDepth(ops []contentOp, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict, depth int) (string, bool) {
	if depth >= maxFormRedactDepth {
		// Matches redactFormXObject's own depth-cap behavior: not
		// attempted, so not flagged unsafe either — a true blind spot,
		// not a false negative this check is responsible for.
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
			if !ok {
				return name, true
			}
			if classifyBbox(minX, minY, maxX, maxY, regions) == keepDo {
				continue
			}

			formMatrix := identityMatrix()
			if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
				formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
			}
			inv, invOK := invertMatrix(matMul(formMatrix, ctm))
			if !invOK {
				return name, true
			}

			localRegions := make([]QuadPoint, len(regions))
			for i, q := range regions {
				localRegions[i] = transformQuad(q, inv)
			}
			formResources := resources
			if resVal, ok := st.Dict["/Resources"]; ok {
				if rd, ok := resolveRefToDict(objects, resVal); ok {
					formResources = rd
				}
			}
			innerOps, err := parseContentStream(decodedStreamData(st))
			if err != nil {
				return name, true
			}
			if innerName, unsafe := unsafeFormXObjectNameDepth(innerOps, localRegions, objects, formResources, depth+1); unsafe {
				return innerName, true
			}
		}
	}
	return "", false
}
