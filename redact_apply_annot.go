// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// redactAnnotationAppearances rewrites, in place, every non-/Redact
// annotation's /AP appearance stream(s) whose placement (/Rect, ISO
// 32000-1 §12.5.5) overlaps any redaction region — closing the
// annotation-appearance half of pdf-go-xp2u: ApplyRedactions used to
// rewrite only the page's own /Contents, so a FreeText note's text, a
// Stamp's drawn content, or — most importantly — a form Widget's /AP
// rendering of its OWN FIELD VALUE (a text field showing an SSN, say) all
// drew directly through their own appearance stream, completely outside
// the page-content rewriters' reach, and survived a redaction covering
// that exact spot untouched both visually and in extraction.
//
// Mechanically this is almost entirely a reuse of the Form-XObject
// redaction machinery already built for pdf-go-w5hl: an annotation's /AP/N
// (or /D, /R) stream IS a Form XObject (/Subtype /Form, its own /BBox and
// optional /Matrix) — the only difference from a page-content Form
// XObject is HOW it is placed: not via a "cm"-then-"Do" pair tracked
// through the content stream, but via the fixed ISO 32000-1 §12.5.5
// algorithm (annotAppearanceMatrix, render_annotations.go, already used by
// the renderer to draw these exact same streams) that maps the
// transformed /BBox onto /Rect. That computed matrix is exactly the "ctm"
// formPlacementBbox/redactFormXObject already expect — composed with the
// stream's own /Matrix the same way a Do's CTM is — so this function
// mostly just supplies that matrix and otherwise defers to
// redactFormXObject/formPlacementBbox/classifyBbox verbatim.
//
// Every state a /N, /D, or /R key can resolve to is processed (not only
// the one /AS currently selects) — a checkbox's unchecked appearance, a
// button's rollover/down look, etc. — mirroring pagePreservedXObjectNames'
// own reasoning: a viewer can show any of them, and an extraction tool may
// read any, so leaving an unselected state's content untouched would be
// just as real a leak as leaving the selected one alone.
//
// Clone-not-mutate, same discipline as redactFormDo/redactFormXObject:
// nothing here ever mutates an annotation's /AP dict, a per-state dict, or
// an appearance stream object in place. A fresh copy of /AP (and of any
// per-state dict actually touched) is built and only THAT copy is written
// back onto the one annotation being processed, and a changed appearance
// stream is cloned into a new object with its OWN private, pruned
// /Resources (redactFormXObject's own newResources) rather than reusing
// the original — the same two sharing hazards the Form-XObject fix
// documents (a shared stream object, and a shared resources dict) apply
// here just as much: nothing stops two different annotations, or two
// different state keys of the same annotation, from pointing at the
// literal same stream/dict object in a real file, and this method runs on
// arbitrary third-party input.
//
// depth starts at 0 for every annotation (each is its own independent
// placement root, not nested inside anything else), but shares budget with
// the caller's page-level Form-XObject redaction pass — see
// maxFormRedactWork's own doc comment for why the work budget is scoped
// per PAGE, not per placement tree.
func redactAnnotationAppearances(p *Page, regions []QuadPoint, pageResources pdfDict, budget *formRedactBudget) error {
	objects := p.doc.objects
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		a := coll.At(i)
		if _, isRedact := a.(*RedactAnnotation); isRedact {
			continue // about to be deleted anyway; its own mark-mode preview carries nothing worth redacting into
		}
		base := a.annotationBaseRef()

		rectFloats := shFloats(objects, base.dict["/Rect"])
		rect, ok := normRect(rectFloats)
		if !ok {
			continue // no usable placement box — nothing to redact into (this annotation draws nothing per spec either)
		}
		// Fast path: skip resolving /AP at all when the annotation's own
		// placement box doesn't even roughly overlap any region.
		if classifyBbox(rect[0], rect[1], rect[2], rect[3], regions) == keepDo {
			continue
		}

		apVal, hasAP := base.dict["/AP"]
		if !hasAP {
			continue
		}
		apDict, ok := resolveRefToDict(objects, apVal)
		if !ok {
			continue
		}

		freshAP := make(pdfDict, len(apDict))
		for k, v := range apDict {
			freshAP[k] = v
		}
		changedAny := false
		for _, key := range []string{"/N", "/D", "/R"} {
			v, ok := freshAP[key]
			if !ok {
				continue
			}
			newVal, changed, err := redactAnnotationAPEntry(p.doc, v, rect, regions, pageResources, budget)
			if err != nil {
				return fmt.Errorf("annotation appearance (%s): %w", key, err)
			}
			if changed {
				freshAP[key] = newVal
				changedAny = true
			}
		}
		if changedAny {
			// A fresh, private dict — never the original (possibly
			// shared-by-reference, or shared literally across annotations
			// in an unusual file) /AP dict — written back onto only THIS
			// annotation.
			base.dict["/AP"] = freshAP
		}
	}
	return nil
}

// redactAnnotationAPEntry redacts one /AP sub-entry (the value of /N, /D,
// or /R): either a single appearance stream, or — per ISO 32000-1
// §12.5.5 — a dict of appearance-state name to stream (e.g. a checkbox's
// /On and /Off looks). Returns the (possibly unchanged) value to store
// back at that key, and whether it actually changed.
func redactAnnotationAPEntry(doc *Document, v pdfValue, rect [4]float64, regions []QuadPoint, pageResources pdfDict, budget *formRedactBudget) (pdfValue, bool, error) {
	objects := doc.objects
	resolved := resolveRef(objects, v)
	switch rv := resolved.(type) {
	case *pdfStream:
		newRef, changed, err := redactOneAnnotationAppearance(doc, rv, rect, regions, pageResources, budget)
		if err != nil || !changed {
			return v, false, err
		}
		return newRef, true, nil
	case pdfDict:
		fresh := make(pdfDict, len(rv))
		for k, vv := range rv {
			fresh[k] = vv
		}
		changedAny := false
		for stateKey, sv := range rv {
			st, ok := resolveRef(objects, sv).(*pdfStream)
			if !ok {
				continue
			}
			newRef, changed, err := redactOneAnnotationAppearance(doc, st, rect, regions, pageResources, budget)
			if err != nil {
				return nil, false, err
			}
			if changed {
				fresh[stateKey] = newRef
				changedAny = true
			}
		}
		if !changedAny {
			return v, false, nil
		}
		return fresh, true, nil
	default:
		return v, false, nil
	}
}

// unsafeAnnotationAppearance is ValidateRedactions's read-only pre-flight
// counterpart to redactAnnotationAppearances: it reports whether any
// non-/Redact annotation on p, overlapping a redaction region, has an /AP
// appearance stream that cannot be safely resolved (a missing/malformed
// /BBox — the only way annotAppearanceMatrix/formPlacementBbox can fail
// here, since the placement matrix itself is always invertible by
// construction: ISO 32000-1 §12.5.5's own algorithm only ever produces an
// axis-aligned scale+translate from a non-degenerate transformed box, and
// a degenerate one — zero width or height — is already excluded by
// annotAppearanceMatrix's own tw<=0/th<=0 check, which folds into the
// same "not ok" result formPlacementBbox would report from a bad /BBox
// anyway). Uses a fresh, call-local work budget — this never shares state
// with an ApplyRedactions call's own per-page budget, since it never
// mutates anything.
func unsafeAnnotationAppearance(p *Page, objects map[int]*pdfObject, regions []QuadPoint) bool {
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		a := coll.At(i)
		if _, isRedact := a.(*RedactAnnotation); isRedact {
			continue
		}
		base := a.annotationBaseRef()
		rect, ok := normRect(shFloats(objects, base.dict["/Rect"]))
		if !ok {
			continue
		}
		if classifyBbox(rect[0], rect[1], rect[2], rect[3], regions) == keepDo {
			continue
		}
		apDict, ok := resolveRefToDict(objects, base.dict["/AP"])
		if !ok {
			continue
		}
		for _, key := range []string{"/N", "/D", "/R"} {
			v, ok := apDict[key]
			if !ok {
				continue
			}
			for _, st := range appearanceStreams(objects, v) {
				m, ok := annotAppearanceMatrix(objects, st, rect)
				if !ok {
					return true
				}
				if _, _, _, _, ok := formPlacementBbox(objects, st, m); !ok {
					return true
				}
			}
		}
	}
	return false
}

// redactOneAnnotationAppearance redacts a single appearance Form XObject
// stream, given the annotation's /Rect, and returns a pdfRef to a freshly
// cloned, redacted object plus changed=true only if the content actually
// differs — mirroring redactFormDo's own "clone only if genuinely
// different, never mutate the original" contract exactly, including reuse
// of its newResources-becomes-the-clone's-own-/Resources handling.
func redactOneAnnotationAppearance(doc *Document, st *pdfStream, rect [4]float64, regions []QuadPoint, pageResources pdfDict, budget *formRedactBudget) (pdfValue, bool, error) {
	objects := doc.objects

	m, ok := annotAppearanceMatrix(objects, st, rect)
	if !ok {
		// Malformed/missing /BBox: same "refuse rather than guess" stance
		// as formPlacementBbox's own ok=false case for a page-content Form
		// XObject — this annotation's placement box already passed the
		// coarse /Rect-vs-region check above, so there IS a region nearby
		// this appearance cannot be trusted to redact into.
		return nil, false, fmt.Errorf("asposepdf: ApplyRedactions: an annotation's appearance stream has no resolvable /BBox; cannot safely determine whether a redaction region intersects it")
	}

	minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, m)
	if !ok {
		return nil, false, fmt.Errorf("asposepdf: ApplyRedactions: an annotation's appearance stream has no resolvable /BBox; cannot safely determine whether a redaction region intersects it")
	}
	if classifyBbox(minX, minY, maxX, maxY, regions) == keepDo {
		return nil, false, nil // exact placement box, unlike the coarse /Rect check above, doesn't actually overlap
	}

	if err := budget.consume(); err != nil {
		return nil, false, err
	}

	// initial=nil: an annotation's /AP appearance stream is placed by a
	// fixed ISO 32000-1 §12.5.5 algorithm, not a content-stream Do with
	// some surrounding text state to inherit — there is nothing here to
	// pass through, same as the page-level call.
	newData, newResources, changed, err := redactFormXObject(doc, st, pageResources, m, regions, 0, budget, nil)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return nil, false, nil
	}

	newID := doc.nextID
	doc.nextID++
	newDict := make(pdfDict, len(st.Dict))
	for k, v := range st.Dict {
		newDict[k] = v
	}
	if newResources != nil {
		newDict["/Resources"] = newResources
	}
	newDict["/Length"] = len(newData)
	delete(newDict, "/Filter")
	delete(newDict, "/DecodeParms")
	doc.objects[newID] = &pdfObject{Num: newID, Value: &pdfStream{
		Dict:    newDict,
		Data:    newData,
		Decoded: true,
	}}

	return pdfRef{Num: newID}, true, nil
}
