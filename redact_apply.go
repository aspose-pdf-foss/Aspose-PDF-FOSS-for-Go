// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// ValidateRedactions performs a pre-flight dry-run check on every page that
// has at least one RedactAnnotation: its content stream must parse, and no
// redaction region may intersect a Form XObject the page draws (see
// unsafeFormXObjectName — ApplyRedactions cannot safely redact into a Form
// XObject's own content, pdf-go-w5hl). Returns the first problem found, or
// nil if every redact-bearing page is safe to process. Pages without redact
// annotations are not inspected.
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
			return fmt.Errorf("ValidateRedactions: page %d: a redaction region intersects Form XObject %q; ApplyRedactions cannot safely redact a Form XObject's own content (flatten it into page content first, or keep redaction regions clear of it)", page.Number(), name)
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
// Known gaps (not closed by this sweep, tracked separately): an Image
// XObject fully inside a redaction region has its Do invocation dropped
// from the content stream, but its /Resources/XObject entry is left in
// place (graph-reachable, so RemoveUnusedObjects does not collect it) —
// its pixel data stays in the file. A Form XObject intersecting a region
// at all is refused outright rather than silently left incomplete — see
// unsafeFormXObjectName and pdf-go-w5hl — since unlike an image it can
// itself hold arbitrary text/path content this pipeline never descends
// into (the check maps the form's own /BBox+/Matrix through the CTM, not
// an image's unit square, but still only one Do deep: a Form XObject
// placed by ANOTHER Form XObject is invisible to it entirely, not even
// flagged — the same pre-existing blind spot every rewriter here has for
// nested forms). A page's /Thumb thumbnail, if a producer embedded one, is never
// touched by this method and may still show a miniature of the
// pre-redaction page. Redacting a document that is later saved
// incrementally (already signed, or AddValidationInfo) keeps the
// original text in the earlier, byte-preserved revision by design.
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

	// 3. Build font map from page resources.
	resources := p.pageResources()
	fontMap := resolveFontResources(p.doc.objects, resources)

	// 4. Run rewriters sequentially: text → image → path.
	data, err = rewriteTextOperatorsInStream(data, regions, fontMap)
	if err != nil {
		return fmt.Errorf("text rewriter: %w", err)
	}
	data, err = rewriteImageOperatorsInStream(data, regions, p.doc.objects, resources)
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
