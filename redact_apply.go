// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// ValidateRedactions performs a pre-flight dry-run parseability check on
// every page that has at least one RedactAnnotation. Returns the first
// content-stream parse error encountered, or nil if every redact-bearing
// page's content stream parses cleanly. Pages without redact annotations
// are not inspected.
//
// Callers should invoke this before ApplyRedactions to surface malformed
// content streams up-front; ApplyRedactions has best-effort semantics
// and may leave partial state on failure.
func (d *Document) ValidateRedactions() error {
	for _, page := range d.Pages() {
		if !pageHasRedact(page) {
			continue
		}
		data, err := page.contentStreams()
		if err != nil {
			return fmt.Errorf("ValidateRedactions: page %d: %w", page.Number(), err)
		}
		if data == nil {
			continue
		}
		if _, err := parseContentStream(data); err != nil {
			return fmt.Errorf("ValidateRedactions: page %d content stream: %w", page.Number(), err)
		}
	}
	return nil
}

// ApplyRedactions destructively removes content (text glyphs, image
// XObjects, paths) inside every /Redact annotation's /QuadPoints (or
// /Rect if /QuadPoints is empty). Renders /OverlayText if set, then
// removes the redact annotations from the page.
//
// Best-effort semantics: partial state on failure. Call
// ValidateRedactions first to surface parse errors before mutating.
func (d *Document) ApplyRedactions() error {
	for _, page := range d.Pages() {
		if !pageHasRedact(page) {
			continue
		}
		if err := applyRedactionsToPage(page); err != nil {
			return fmt.Errorf("ApplyRedactions: page %d: %w", page.Number(), err)
		}
	}
	return nil
}

// applyRedactionsToPage collects regions from every RedactAnnotation on the
// page, runs the three content-stream rewriters, replaces the page content,
// and removes the redact annotations from the collection.
func applyRedactionsToPage(p *Page) error {
	// 1. Collect regions and redact annotation pointers.
	var regions []QuadPoint
	coll := p.Annotations()
	var redacts []*RedactAnnotation
	for i := 0; i < coll.Count(); i++ {
		ra, ok := coll.At(i).(*RedactAnnotation)
		if !ok {
			continue
		}
		redacts = append(redacts, ra)
		qps := ra.QuadPoints()
		if len(qps) == 0 {
			// Fallback: treat /Rect as a single quad.
			qps = []QuadPoint{rectAsQuadPoint(ra.Rect())}
		}
		regions = append(regions, qps...)
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
	data, err = rewriteImageOperatorsInStream(data, regions)
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
// /Contents entries (single ref or array) are abandoned; the writer's
// reachability pass will drop them on save.
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
