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

	// NoTextLayer leaves a rasterized page with only its image. By default the
	// page's text stays over it as an invisible layer, so it can still be searched
	// and copied. Text in a Type3 font is not kept: its glyphs would paint over the
	// image, since the invisible rendering mode does not apply to them.
	NoTextLayer bool
}

// FlattenTransparency rasterizes every page that paints transparently (constant
// alpha, a blend mode, a soft mask) into a single opaque full-page image, keeping
// its text over the image as an invisible layer (see
// FlattenTransparencyOptions.NoTextLayer). Pages that only carry a transparency group with nothing
// translucent inside lose the group and stay as they are: many producers write one
// on every page. Returns the number of pages rasterized. A flattened page's old
// content-stream and resource objects (the very transparency constructs
// this method exists to remove) become unreachable; when at least one page
// was flattened, FlattenTransparency also calls RemoveUnusedObjects so
// those orphans are actually gone from the file — otherwise ValidatePDFA's
// TRANSPARENCY check, which scans every object rather than only reachable
// ones, would still trip on them.
func (d *Document) FlattenTransparency(opts ...FlattenTransparencyOptions) (int, error) {
	pages, err := d.flattenTransparencyPages(opts...)
	return len(pages), err
}

// flattenTransparencyPages does the work and returns the 1-based numbers of the
// pages it rasterized.
func (d *Document) flattenTransparencyPages(opts ...FlattenTransparencyOptions) ([]int, error) {
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
				return nil, fmt.Errorf("asposepdf: FlattenTransparency: page %d out of range [1,%d]", n, len(pages))
			}
			targets = append(targets, pages[n-1])
		}
	}

	var flattened []int
	for _, p := range targets {
		needs, err := pageNeedsTransparencyFlatten(d.objects, p.pageDict())
		if err != nil {
			return flattened, err
		}
		if !needs {
			continue
		}
		if err := p.flattenTransparency(dpi, !opt.NoTextLayer); err != nil {
			return flattened, err
		}
		flattened = append(flattened, p.Number())
	}
	if len(flattened) > 0 {
		d.sweepUnused()
	}
	d.dropIdleGroups(targets, len(opt.Pages) > 0)
	return flattened, nil
}

// pageNeedsTransparencyFlatten reports whether anything reachable from pageDict's
// /Resources (recursively through Form XObjects, Patterns, ExtGState, …) paints
// transparently. A group alone does not count: dropIdleGroups removes those.
// collectValueDepsDoc (doc.go) does the resource-graph walk.
func pageNeedsTransparencyFlatten(objects map[int]*pdfObject, pageDict pdfDict) (bool, error) {
	if pageDict == nil {
		return false, nil
	}
	resources, ok := pageDict["/Resources"]
	if !ok {
		return false, nil
	}
	return resourcesPaintTransparently(objects, resources), nil
}

func resourcesPaintTransparently(objects map[int]*pdfObject, resources pdfValue) bool {
	if valuePaintsTransparently(objects, resources) {
		return true // written directly into the resources
	}
	deps := map[int]*pdfObject{}
	collectValueDepsDoc(objects, resources, deps, map[int]bool{})
	for _, obj := range deps {
		if valuePaintsTransparently(objects, obj.Value) {
			return true
		}
	}
	return false
}

// dropIdleGroups removes the transparency groups that change nothing, so the
// file has no transparency left and its pages stay vector. Run after rasterizing:
// a form with nothing translucent inside still composites differently as a group
// under an outer alpha, so a page that has one must be rendered first.
//
// A group is the soft mask's source when a mask names it, and stays. When only
// some pages are targeted, a form is left alone if a page outside the selection
// paints transparently, since it may draw the form under its alpha.
func (d *Document) dropIdleGroups(targets []*Page, subset bool) {
	maskSources := map[int]bool{}
	d.pdfaWalkDicts(func(dict pdfDict) {
		if s := dictGetName(dict, "/S"); s == "/Luminosity" || s == "/Alpha" {
			if ref, ok := dict["/G"].(pdfRef); ok {
				maskSources[ref.Num] = true
			}
		}
	})
	dropForm := func(num int, st *pdfStream) {
		if maskSources[num] || !dictHasTransparencyGroup(d.objects, st.Dict) {
			return
		}
		if res, ok := st.Dict["/Resources"]; ok && resourcesPaintTransparently(d.objects, res) {
			return
		}
		delete(st.Dict, "/Group")
	}
	formsToo := true
	if subset {
		inTargets := map[*pdfObject]bool{}
		for _, p := range targets {
			inTargets[p.pageObj()] = true
		}
		for _, obj := range d.pages {
			if inTargets[obj] {
				continue
			}
			if need, _ := pageNeedsTransparencyFlatten(d.objects, obj.Value.(pdfDict)); need {
				formsToo = false
			}
		}
	}
	for _, p := range targets {
		pd := p.pageDict()
		if need, _ := pageNeedsTransparencyFlatten(d.objects, pd); !need && pd != nil {
			if dictHasTransparencyGroup(d.objects, pd) {
				delete(pd, "/Group")
			}
			if res, ok := pd["/Resources"]; ok && subset && formsToo {
				deps := map[int]*pdfObject{}
				collectValueDepsDoc(d.objects, res, deps, map[int]bool{})
				for num, obj := range deps {
					if st, ok := obj.Value.(*pdfStream); ok {
						dropForm(num, st)
					}
				}
			}
		}
	}
	if subset {
		return
	}
	for num, obj := range d.objects { // appearance streams and patterns as well
		if st, ok := obj.Value.(*pdfStream); ok {
			dropForm(num, st)
		}
	}
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
func (p *Page) flattenTransparency(dpi float64, keepText bool) error {
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

	// Read before the page's resources and content are replaced below.
	res := pdfDict{}
	var layer []byte
	if keepText {
		if content, layerRes, ok := p.textLayerFor(); ok {
			layer, res = content, layerRes
		}
	}
	xobjects, _ := res["/XObject"].(pdfDict)
	if xobjects == nil {
		xobjects = pdfDict{}
		res["/XObject"] = xobjects
	}
	name := "/Im0"
	for i := 1; xobjects[name] != nil; i++ {
		name = fmt.Sprintf("/Im%d", i)
	}
	xobjects[name] = pdfRef{Num: imgID}

	w := box.URX - box.LLX
	h := box.URY - box.LLY
	// The picture is an artifact: what it shows is the text the layer carries, and
	// in a tagged document that text keeps its place in the structure tree.
	ops := fmt.Sprintf("/Artifact BMC\nq\n%s 0 0 %s %s %s cm\n%s Do\nQ\nEMC\n",
		formatFloat(w), formatFloat(h), formatFloat(box.LLX), formatFloat(box.LLY), name)
	if layer != nil {
		ops += "q\n" + string(layer) + "\nQ\n"
	}

	pageDict["/Resources"] = res
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
