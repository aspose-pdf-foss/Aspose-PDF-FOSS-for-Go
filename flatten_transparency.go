// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// FlattenTransparency (flatten_transparency.go) eliminates transparency —
// needed because PDF/A-1 (ISO 19005-1) forbids it entirely (ValidatePDFA's
// TRANSPARENCY issue, PDF/A-1 only). Mirrors Aspose.PDF for .NET's
// Document.FlattenTransparency(). Design: docs/superpowers/specs/
// 2026-09-23-flatten-transparency-design.md.
//
// Scope (v1): whole-page rasterization. A page whose content stream's
// resource graph uses a transparency group, a soft mask, a non-Normal
// blend mode, or /ca//CA < 1 is rendered to an opaque raster with the
// built-in renderer and its content stream is replaced with one full-page
// image; a page without any of those is left completely untouched (still
// vector, still searchable). Annotation-level transparency (an
// annotation's own opacity, or transparency inside its /AP appearance
// stream) is out of scope: detection only walks the page's content +
// /Resources graph, not /Annots, so annotations keep rendering live from
// /Annots exactly as before — call Annotation.Flatten()/Form.Flatten()
// first to bake those into page content (where this pass then does catch
// them) if annotation transparency also needs to go.
//
// This is a different "flatten" than Document.Flatten()/Form.Flatten()
// (flatten.go — baking AcroForm/annotation appearances into page content);
// the two are unrelated and composable, not overlapping.

// FlattenTransparencyOptions configures FlattenTransparency.
type FlattenTransparencyOptions struct {
	DPI   float64 // dots per inch for the replacement raster; 0 -> DefaultDPI
	Pages []int   // 1-based page numbers to consider; empty -> every page
}

// FlattenTransparency rasterizes every page that uses transparency into a
// single opaque full-page image, leaving pages without transparency
// untouched. Returns the number of pages rasterized. A flattened page's old
// content-stream and resource objects (the very transparency constructs
// this method exists to remove) become unreachable; when at least one page
// was flattened, FlattenTransparency also calls RemoveUnusedObjects so
// those orphans are actually gone from the file — otherwise ValidatePDFA's
// TRANSPARENCY check, which scans every object rather than only reachable
// ones, would still trip on them.
func (d *Document) FlattenTransparency(opts ...FlattenTransparencyOptions) (int, error) {
	var opt FlattenTransparencyOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	dpi := opt.DPI
	if dpi <= 0 {
		dpi = DefaultDPI
	}

	pages := d.Pages()
	targets := pages
	if len(opt.Pages) > 0 {
		targets = make([]*Page, 0, len(opt.Pages))
		for _, n := range opt.Pages {
			if n < 1 || n > len(pages) {
				return 0, fmt.Errorf("asposepdf: FlattenTransparency: page %d out of range [1,%d]", n, len(pages))
			}
			targets = append(targets, pages[n-1])
		}
	}

	flattened := 0
	for _, p := range targets {
		needs, err := pageNeedsTransparencyFlatten(d.objects, p.pageDict())
		if err != nil {
			return flattened, err
		}
		if !needs {
			continue
		}
		if err := p.flattenTransparency(dpi); err != nil {
			return flattened, err
		}
		flattened++
	}
	if flattened > 0 {
		d.RemoveUnusedObjects()
	}
	return flattened, nil
}

// pageNeedsTransparencyFlatten reports whether pageDict's own /Group, or any
// object reachable from its /Resources (recursively through Form XObjects,
// Patterns, ExtGState, …), carries a transparency construct. Reuses
// pdfaDictHasTransparency (validate_pdfa.go) — the same four-key check
// ValidatePDFA's TRANSPARENCY issue already relies on — and
// collectValueDepsDoc (doc.go) for the resource-graph walk.
func pageNeedsTransparencyFlatten(objects map[int]*pdfObject, pageDict pdfDict) (bool, error) {
	if pageDict == nil {
		return false, nil
	}
	if pdfaDictHasTransparency(objects, pageDict) {
		return true, nil
	}
	resources, ok := pageDict["/Resources"]
	if !ok {
		return false, nil
	}
	deps := map[int]*pdfObject{}
	visited := map[int]bool{}
	collectValueDepsDoc(objects, resources, deps, visited)
	for _, obj := range deps {
		switch v := obj.Value.(type) {
		case pdfDict:
			if pdfaDictHasTransparency(objects, v) {
				return true, nil
			}
		case *pdfStream:
			if pdfaDictHasTransparency(objects, v.Dict) {
				return true, nil
			}
		}
	}
	return false, nil
}

// flattenTransparency rasterizes p's content (not its annotations, which
// keep rendering live from /Annots) into one opaque image at dpi, replaces
// /Resources with a fresh dict holding just that image, drops any
// page-level transparency group (its content has been rasterized away, so
// there is nothing left to group), and replaces /Contents with a single
// stream that draws the image over the page's render box (CropBox
// intersected with MediaBox — the same region RenderImage covers).
//
// Everything that can fail (rendering, PNG encoding, building the image
// XObject) happens before the page dict is touched at all, so a failure
// partway through never leaves the page with /Resources already wiped but
// /Contents still referencing names that no longer exist.
func (p *Page) flattenTransparency(dpi float64) error {
	img, box, err := p.renderContentForFlatten(dpi)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("asposepdf: FlattenTransparency: encode raster: %w", err)
	}

	// A rasterized page is always fully opaque (image.RGBA.Opaque() is true
	// by construction), so Go's PNG encoder drops the alpha channel and
	// createImageXObject never returns a soft mask here in practice — but
	// register one if it somehow did, rather than silently dropping it.
	imgStream, smaskStream, err := createImageXObject(buf.Bytes(), ImageFormatPNG)
	if err != nil {
		return fmt.Errorf("asposepdf: FlattenTransparency: embed raster: %w", err)
	}
	if smaskStream != nil {
		smaskID := p.doc.nextID
		p.doc.nextID++
		p.doc.objects[smaskID] = &pdfObject{Num: smaskID, Value: smaskStream}
		imgStream.Dict["/SMask"] = pdfRef{Num: smaskID}
	}
	imgID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[imgID] = &pdfObject{Num: imgID, Value: imgStream}

	pageDict := p.pageDict()
	if pageDict == nil {
		return fmt.Errorf("asposepdf: FlattenTransparency: page has no dict")
	}

	w := box.URX - box.LLX
	h := box.URY - box.LLY
	ops := fmt.Sprintf("q\n%s 0 0 %s %s %s cm\n/Im0 Do\nQ\n",
		formatFloat(w), formatFloat(h), formatFloat(box.LLX), formatFloat(box.LLY))

	pageDict["/Resources"] = pdfDict{"/XObject": pdfDict{"/Im0": pdfRef{Num: imgID}}}
	delete(pageDict, "/Group")
	return replacePageContents(p, []byte(ops))
}

// renderContentForFlatten rasterizes p's content only (no annotations) at
// dpi, in the page's own unrotated coordinate space — RenderImage bakes
// /Rotate into its output pixels for direct display, but a raster meant to
// be placed back into the content stream via cm+Do must NOT have rotation
// baked in twice: the page's existing /Rotate (left untouched) already
// applies it once, at display time, same as before flattening.
func (p *Page) renderContentForFlatten(dpi float64) (image.Image, Rectangle, error) {
	box, err := p.renderBox()
	if err != nil {
		return nil, Rectangle{}, fmt.Errorf("asposepdf: FlattenTransparency: %w", err)
	}
	scale := dpi / 72.0
	base, w, h := deviceMatrix(box, scale, Rotate0)
	if w <= 0 || h <= 0 {
		return nil, Rectangle{}, fmt.Errorf("asposepdf: FlattenTransparency: degenerate page size %dx%d", w, h)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillBackground(img, nil) // opaque white backdrop

	rd := newRenderer(p, img, w, h, base)
	rd.skipAnnotations = true
	rd.run()
	return img, box, nil
}
