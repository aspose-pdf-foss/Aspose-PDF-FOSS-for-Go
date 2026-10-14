// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// ValidateRedactions performs a pre-flight dry-run check on every page that
// has at least one RedactAnnotation: its content stream must parse, and
// every Form XObject a redaction region intersects must be one
// ApplyRedactions can safely redact into — a resolvable /BBox, an
// invertible placement matrix, and nesting/work within
// maxFormRedactDepth/maxFormRedactWork (see unsafeFormXObjectName; most
// intersecting Form XObjects ARE safely redactable since pdf-go-w5hl — this
// only flags the genuinely unsafe cases). Returns the first problem found,
// or nil if every redact-bearing page is safe to process. Pages without
// redact annotations are not inspected.
//
// Callers should invoke this before ApplyRedactions to surface problems
// up-front; ApplyRedactions has best-effort semantics and may leave
// partial state on failure.
func (d *Document) ValidateRedactions() error {
	for _, page := range d.Pages() {
		if !pageHasRedact(page) {
			continue
		}
		regions := collectRedactRegions(page)
		if len(regions) == 0 {
			continue
		}
		data, err := page.contentStreams()
		if err != nil {
			return fmt.Errorf("ValidateRedactions: page %d: %w", page.Number(), err)
		}
		if data == nil {
			continue
		}
		ops, err := parseContentStream(data)
		if err != nil {
			return fmt.Errorf("ValidateRedactions: page %d content stream: %w", page.Number(), err)
		}
		resources := page.pageResources()
		if name, found := unsafeFormXObjectName(ops, regions, d.objects, resources); found {
			return fmt.Errorf("ValidateRedactions: page %d: Form XObject %q cannot be safely redacted into (missing/malformed /BBox, a non-invertible placement matrix, or nesting/work beyond the redaction depth/work limits); flatten it into page content first, or keep redaction regions clear of it", page.Number(), name)
		}
	}
	return nil
}

// collectRedactRegions gathers the /QuadPoints (or /Rect fallback) of
// every RedactAnnotation on p.
func collectRedactRegions(p *Page) []QuadPoint {
	var regions []QuadPoint
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		ra, ok := coll.At(i).(*RedactAnnotation)
		if !ok {
			continue
		}
		qps := ra.QuadPoints()
		if len(qps) == 0 {
			qps = []QuadPoint{rectAsQuadPoint(ra.Rect())}
		}
		regions = append(regions, qps...)
	}
	return regions
}

// ApplyRedactions destructively removes content (text glyphs, image
// XObjects, paths) inside every /Redact annotation's /QuadPoints (or
// /Rect if /QuadPoints is empty). Renders /OverlayText if set, then
// removes the redact annotations from the page.
//
// Calls RemoveUnusedObjects once at the end when at least one page was
// redacted, so the content stream(s) a page's /Contents pointed to before
// redaction — and any earlier dead copy left by prior drawing calls
// (image_add.go's appendToContentStream, see pdf-go-qgrn) — are actually
// gone from the saved file rather than merely unreferenced. Both still
// carry the redacted content in full, recoverable with any PDF tool, until
// this runs: replacePageContents (used here and by FlattenTransparency)
// only repoints /Contents, it was never the writer's job to drop what it
// replaces. Reported privately by shadowhunter1470-svg via GitHub's
// private vulnerability reporting (pdf-go-2qkq) — thank you for the
// careful, considerate report.
//
// This sweep shares RemoveUnusedObjects' usual trade-off: a page/catalog
// -unreachable object that is not yet "dangerous" — e.g. a Type3 glyph
// canvas or an XForm the caller built but has not placed on any page yet
// — is reclaimed too. Call ApplyRedactions only after placing everything
// you mean to keep, same as any other Optimize-family call.
//
// A Form XObject intersecting a redaction region — fully or partially —
// is now redacted into, not refused (pdf-go-w5hl, closed; a v0.11.0
// safety stopgap until then): the region is mapped from page space into
// the form's own local content-stream space by inverting the composed
// placement matrix (the form's /Matrix, then the CTM the Do invocation
// was under — formPlacementBbox's own convention, run in reverse), and
// the SAME three rewriters this method uses at the page level then run on
// the form's content with those local-space regions, recursing into a
// nested Form XObject the same way (up to maxFormRedactDepth levels, and
// a total maxFormRedactWork placements per PAGE, not across the whole
// call — see maxFormRedactWork's own doc comment for why that scope is
// intentional — a fan-out of region-intersecting placements can exhaust
// this long before the depth cap, bounding a pathologically
// fan-out-heavy or self-referential file). Neither cap is a silent blind
// spot: exceeding either one while a region genuinely intersects an
// unexamined Do is refused (an error), not left incomplete — an earlier
// draft of the depth cap skipped silently instead, caught by an
// independent review. The ORIGINAL form object, and its own /Resources
// dict, are never mutated: since the same Form XObject (and the same
// /Resources, shared by every placement of it) can be placed more than
// once — the same logo stamped on every page, say — only placements
// that actually need a change are redirected (their one Do operand only)
// to a freshly cloned, redacted copy with its own private /Resources,
// PRUNED to exactly the /XObject entries the clone's own (rewritten)
// content still needs (see neededXObjectNames) — every other placement
// of the original keeps rendering exactly as before. The same sharing
// hazard applies one level up too: a PAGE's own /Resources can itself be
// shared across pages via an indirect reference (another common producer
// size optimization), so this method gives each redacted page a private
// copy of its own before redacting into it, pruned the same way — minus
// whatever an annotation appearance stream with no own /Resources on
// that page still needs through it (see pagePreservedXObjectNames),
// since the page-level prune has no visibility into annotations on its
// own. Three sharing/pruning hazards here — a cloned form's own private
// resources still forwarding a reference to the very original object it
// replaces (through an inherited entry its own content never uses), a
// page-level prune removing a name a SIBLING Form XObject sharing that
// same dict still calls, and the two levels of resources-dict sharing
// this paragraph opened with — were all confirmed reachable (an
// unrelated, non-redacted page or placement silently losing content, or
// the original secret-bearing object staying reachable and
// un-collectable) across two rounds of independent review of this fix.
// Still refused outright, not guessed at: a Form XObject with no
// resolvable /BBox (formPlacementBbox's own ok=false — ISO 32000-1
// §8.10.2 requires one on every conformant form) or a placement matrix
// that doesn't invert (so a region can't be mapped into the form's space
// at all) — see redactFormDo/redactFormXObject.
//
// Known gaps (not closed by this sweep, tracked separately): an Image
// XObject fully inside a redaction region has its Do invocation dropped
// from the content stream, but its /Resources/XObject entry is left in
// place (graph-reachable, so RemoveUnusedObjects does not collect it) —
// its pixel data stays in the file (pdf-go-ye1z). A page's /Thumb
// thumbnail, if a producer embedded one, is never touched by this method
// and may still show a miniature of the pre-redaction page (pdf-go-ekp4).
// A non-/Redact annotation (FreeText, Stamp, Widget, markup, …) whose own
// appearance stream happens to overlap a redaction region keeps that
// content untouched — this method only rewrites page content and removes
// /Redact annotations themselves, it never examines another annotation's
// /AP, a tiling-pattern cell stream, or a Type3 font's /CharProcs
// (pdf-go-xp2u). Redacting a document that is later saved incrementally
// (already signed, or AddValidationInfo) keeps the original text in the
// earlier, byte-preserved revision by design.
//
// Best-effort semantics: partial state on failure. Call
// ValidateRedactions first to surface problems before mutating.
func (d *Document) ApplyRedactions() error {
	redacted := 0
	for _, page := range d.Pages() {
		if !pageHasRedact(page) {
			continue
		}
		if err := applyRedactionsToPage(page); err != nil {
			return fmt.Errorf("ApplyRedactions: page %d: %w", page.Number(), err)
		}
		redacted++
	}
	if redacted > 0 {
		d.RemoveUnusedObjects()
	}
	return nil
}

// applyRedactionsToPage collects regions from every RedactAnnotation on the
// page, runs the three content-stream rewriters, replaces the page content,
// and removes the redact annotations from the collection.
func applyRedactionsToPage(p *Page) error {
	// 1. Collect regions and redact annotation pointers.
	regions := collectRedactRegions(p)
	coll := p.Annotations()
	var redacts []*RedactAnnotation
	for i := 0; i < coll.Count(); i++ {
		if ra, ok := coll.At(i).(*RedactAnnotation); ok {
			redacts = append(redacts, ra)
		}
	}
	if len(regions) == 0 {
		return nil // no regions to apply
	}

	// 2. Get decoded page content stream bytes.
	data, err := p.contentStreams()
	if err != nil {
		return err
	}
	if data == nil {
		// Empty page — still remove the annotations below.
		data = []byte{}
	}

	// 3. Give this page its OWN private copy of the /XObject resources
	// sub-dict before any rewriter runs, and install it as the page's own
	// /Resources: a page's /Resources (and/or its /XObject sub-dict) can
	// be shared across several pages via an indirect reference — a common
	// PDF producer size optimization — and the Form XObject handling
	// below may add/remove /XObject entries for a redirected clone.
	// Mutating a shared dict in place would corrupt every OTHER page
	// sharing it, even ones with no redaction region of their own —
	// confirmed reachable (an unrelated, non-redacted page sharing a
	// redacted Form XObject's resources lost that form's content
	// entirely) by an independent review of an earlier draft that used
	// p.pageResources() directly here. Other pages keep pointing at the
	// original, untouched dict; only this one page's view changes.
	resources := privateXObjectResources(p.doc.objects, p.pageResources())
	p.pageDict()["/Resources"] = resources
	fontMap := resolveFontResources(p.doc.objects, resources)

	// One work budget for this page's whole Form XObject redaction pass
	// (see maxFormRedactWork — intentionally scoped per page, not per
	// ApplyRedactions call). pagePreservedXObjectNames reuses it for its
	// own (bounded) scan of annotation appearance streams, rather than
	// building a second, unbounded one.
	budget := &formRedactBudget{}

	// Names an annotation appearance stream with no own /Resources on
	// THIS page still resolves through resources — the page-level prune
	// below (inside rewriteXObjectDoOperatorsInStream, via
	// neededXObjectNames) only sees the page's own content stream, never
	// annotations, so without this an annotation could be silently
	// blanked the next time it renders. See pagePreservedXObjectNames.
	preserve := pagePreservedXObjectNames(p, p.doc.objects, resources, budget)

	// 4. Run rewriters sequentially: text → image → path.
	data, err = rewriteTextOperatorsInStream(data, regions, fontMap)
	if err != nil {
		return fmt.Errorf("text rewriter: %w", err)
	}
	data, err = rewriteXObjectDoOperatorsInStream(data, regions, p.doc, resources, 0, budget, preserve)
	if err != nil {
		return fmt.Errorf("image rewriter: %w", err)
	}
	data, err = rewritePathOperatorsInStream(data, regions)
	if err != nil {
		return fmt.Errorf("path rewriter: %w", err)
	}

	// 5. Replace the page's /Contents with a single new stream.
	if err := replacePageContents(p, data); err != nil {
		return fmt.Errorf("replace content: %w", err)
	}

	// 6. Render overlay (fill + optional text) for each redact region.
	// Must happen BEFORE deleting the annotations so accessors still work.
	for _, ra := range redacts {
		if err := renderRedactOverlay(p, ra); err != nil {
			return fmt.Errorf("overlay: %w", err)
		}
	}

	// 7. Remove every redact annotation from the collection.
	for _, ra := range redacts {
		coll.Delete(ra)
	}

	return nil
}

// replacePageContents allocates a new stream object holding data and sets
// the page dict's /Contents to a single pdfRef pointing at it. Any prior
// /Contents entries (single ref or array) are abandoned — the writer has
// no reachability pass of its own (sortedObjectIDs serializes every
// object in d.objects regardless), so an abandoned stream still ends up
// in the saved file unless the caller runs RemoveUnusedObjects. Both
// callers of this function do: ApplyRedactions (where an abandoned
// stream can carry the very content just redacted — pdf-go-2qkq) and
// FlattenTransparency.
func replacePageContents(p *Page, data []byte) error {
	pageDict := p.pageDict()
	if pageDict == nil {
		return fmt.Errorf("no page dict")
	}

	newID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[newID] = &pdfObject{
		Num: newID,
		Value: &pdfStream{
			Dict:    pdfDict{"/Length": len(data)},
			Data:    data,
			Decoded: true, // raw bytes; writer will FlateDecode on output
		},
	}

	pageDict["/Contents"] = pdfRef{Num: newID}
	return nil
}

// pagePreservedXObjectNames returns the /XObject resource names a page's
// OWN /Resources dict must keep no matter what the page's rewritten
// content stream needs, because some annotation appearance stream on the
// page — a /FreeText, /Stamp, /Widget, markup, … — has NO /Resources of
// its own and therefore resolves its own Do names through this exact
// same page resources dict (the same ISO 32000-1 §7.8.3 inheritance rule
// that applies to any Form XObject applies to an /AP one too).
//
// rewriteXObjectDoOperatorsInStream's own resource-pruning pass (via
// neededXObjectNames) only ever sees the page's own content stream — it
// has no visibility into annotations at all — so without this,
// redacting a region that happens to be the LAST thing in the page
// content referencing some name could silently prune an entry an
// untouched annotation's appearance still calls, blanking it the next
// time the page renders or is extracted. Annotations built by this
// library's own authoring code always get their own /Resources (see
// form_xobject.go's makeFormXObjectWithResources, which every /AP stream
// this library writes goes through), so this only bites a
// third-party-produced file — but ApplyRedactions runs on arbitrary
// third-party input by design, so that is not a theoretical concern.
// Confirmed as a real (if narrow) gap by an independent review of an
// earlier draft, which pruned page resources from the content stream
// alone.
//
// budget is shared with the caller's own redaction pass (not a fresh
// one) so this scan's cost counts against the same per-page work limit
// — see maxFormRedactWork.
func pagePreservedXObjectNames(p *Page, objects map[int]*pdfObject, resources pdfDict, budget *formRedactBudget) map[string]bool {
	preserve := map[string]bool{}
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		base := coll.At(i).annotationBaseRef()
		apVal, ok := base.dict["/AP"]
		if !ok {
			continue
		}
		apDict, ok := resolveRefToDict(objects, apVal)
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
					continue // own /Resources — does not resolve names through THIS dict
				}
				neededXObjectNames(decodedStreamData(st), objects, resources, 0, budget, preserve, map[*pdfStream]bool{})
			}
		}
	}
	return preserve
}

// appearanceStreams resolves one /AP sub-entry (/N, /D, or /R) to every
// stream it can refer to per ISO 32000-1 §12.5.5: either a single Form
// XObject stream directly, or — when the annotation has multiple
// appearance states, selected at render time by /AS — a dict of state
// name to stream, in which case every state's stream is returned; this
// scan has no reason to assume any one state is the "current" one.
func appearanceStreams(objects map[int]*pdfObject, v pdfValue) []*pdfStream {
	resolved := resolveRef(objects, v)
	if st, ok := resolved.(*pdfStream); ok {
		return []*pdfStream{st}
	}
	if d, ok := resolved.(pdfDict); ok {
		var out []*pdfStream
		for _, sub := range d {
			if st, ok := resolveRef(objects, sub).(*pdfStream); ok {
				out = append(out, st)
			}
		}
		return out
	}
	return nil
}

// pageHasRedact reports whether the given page has at least one
// RedactAnnotation in its annotation collection.
func pageHasRedact(p *Page) bool {
	coll := p.Annotations()
	for i := 0; i < coll.Count(); i++ {
		if _, ok := coll.At(i).(*RedactAnnotation); ok {
			return true
		}
	}
	return false
}

// renderRedactOverlay draws the post-redaction visual for one RedactAnnotation:
//  1. An opaque fill block in the /IC color (default black) over each quad.
//  2. Overlay text centered in each quad (if /OverlayText is non-empty).
//     If /Repeat is true the text is tiled horizontally across the quad.
func renderRedactOverlay(p *Page, ra *RedactAnnotation) error {
	// 1. Determine fill color: default to opaque black.
	fill := Color{R: 0, G: 0, B: 0, A: 1}
	if ic := ra.InteriorColor(); ic != nil {
		fill = *ic
	}

	// 2. Determine quads: fall back to /Rect as a single quad.
	quads := ra.QuadPoints()
	if len(quads) == 0 {
		quads = []QuadPoint{rectAsQuadPoint(ra.Rect())}
	}

	// 3. Emit fill block for each quad.
	for _, q := range quads {
		minX, minY, maxX, maxY := boundsOfQuad(q)
		ops := fmt.Sprintf("\nq\n%s %s %s rg\n%s %s %s %s re\nf\nQ\n",
			formatFloat(fill.R), formatFloat(fill.G), formatFloat(fill.B),
			formatFloat(minX), formatFloat(minY),
			formatFloat(maxX-minX), formatFloat(maxY-minY))
		if err := p.appendToContentStream([]byte(ops)); err != nil {
			return err
		}
	}

	// 4. Render overlay text if /OverlayText is present.
	overlay := ra.OverlayText()
	if overlay == "" {
		return nil
	}

	style := ra.OverlayTextStyle()
	if style.Font == nil {
		style.Font = FontHelvetica
	}
	if style.Size == 0 {
		style.Size = 10
	}
	// Default text color: white on dark fill, black on light fill.
	if style.Color == nil {
		if isDarkColor(fill) {
			white := Color{R: 1, G: 1, B: 1, A: 1}
			style.Color = &white
		} else {
			black := Color{R: 0, G: 0, B: 0, A: 1}
			style.Color = &black
		}
	}
	// Default horizontal alignment: center.
	if style.HAlign == HAlignLeft {
		style.HAlign = HAlignCenter
	}

	repeat := ra.RepeatOverlayText()
	for _, q := range quads {
		minX, minY, maxX, maxY := boundsOfQuad(q)
		rect := Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY}

		text := overlay
		if repeat {
			// Estimate text width: ~0.5em per character at the given font size.
			estWidth := float64(len(overlay)) * style.Size * 0.5
			rectWidth := maxX - minX
			copies := 1
			if estWidth > 0 {
				copies = int(rectWidth/estWidth) + 1
				if copies < 1 {
					copies = 1
				}
			}
			text = strings.Repeat(overlay+" ", copies)
		}

		if err := p.AddText(text, style, rect); err != nil {
			return err
		}
	}

	return nil
}

// isDarkColor returns true if the color's perceived luminance is below 0.5.
// Uses the standard relative luminance formula (ITU-R BT.709).
func isDarkColor(c Color) bool {
	return 0.2126*c.R+0.7152*c.G+0.0722*c.B < 0.5
}
