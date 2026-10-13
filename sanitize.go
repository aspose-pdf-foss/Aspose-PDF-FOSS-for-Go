// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// HiddenDataSanitizationOptions configures (*Document).Sanitize. Mirrors
// Aspose.PDF for .NET's Aspose.Pdf.Security.HiddenDataSanitization.
// HiddenDataSanitizationOptions; this library composes it as a Document
// method (like Optimize/ConvertToPDFA/FlattenTransparency) rather than a
// separate executor object, since every operation the .NET HiddenDataSanitizer
// class performs already exists here as a primitive this type composes.
type HiddenDataSanitizationOptions struct {
	// RemoveMetadata clears the /Info dictionary and the XMP packet.
	RemoveMetadata bool
	// RemoveAttachments drops every embedded file, document-level and
	// page-pinned.
	RemoveAttachments bool
	// RemoveJavaScriptsAndActions strips document-level and per-annotation
	// JavaScript and Launch actions.
	RemoveJavaScriptsAndActions bool
	// RemoveAnnotations applies any pending redactions (ApplyRedactions)
	// and then removes every remaining annotation from every page, except
	// form-field widgets: those are FlattenForms's job (deleting a widget
	// without also removing its field from /AcroForm/Fields would leave a
	// dangling reference). Set FlattenForms too to also clear forms.
	RemoveAnnotations bool
	// RemoveSearchIndexAndPrivateInfo removes /PieceInfo private
	// application data (a known privacy leak: Illustrator/InDesign's "PDF
	// compatibility" mode keeps the original editable artwork here even
	// in an otherwise-flattened PDF) from the catalog and every page, and
	// drops any embedded file named like an Acrobat Catalog full-text
	// search index (a *.pdx attachment). Best-effort: Aspose does not
	// document this option's exact scope, so this is a defensible
	// narrower reading, not a byte-for-byte mirror.
	RemoveSearchIndexAndPrivateInfo bool
	// FlattenForms bakes every form field's current appearance into page
	// content and removes the AcroForm (Document.Flatten).
	FlattenForms bool
	// FlattenLayers removes the optional-content (layer) configuration
	// and every layer-scoped resource reference, so content that used to
	// be conditionally hidden under a layer is unconditionally visible
	// and the layer dictionaries (including their /Name) become
	// unreachable. Limitation: only checks each page's own
	// /Resources/Properties, not a nested Form XObject's.
	FlattenLayers bool
	// ConvertPagesToImages replaces every page's content with a single
	// rasterized image at ImageDPI, discarding whatever vector/text
	// content survived every option above as a last line of defense. It
	// does not touch annotations (a sticky note's /Contents, an
	// attachment's bytes, an annotation's own /AA JavaScript) — combine
	// with RemoveAnnotations/RemoveAttachments/RemoveJavaScriptsAndActions
	// if those must go too. Runs after every other option. When set,
	// ImageCompressionOptions is ignored.
	ConvertPagesToImages bool
	// ImageDPI is the resolution used by ConvertPagesToImages; 0 -> 150.
	ImageDPI float64
	// ImageCompressionOptions, used only when ConvertPagesToImages is
	// false, recompresses the document's existing images in place.
	ImageCompressionOptions *ImageCompressionOptions
}

// ImageCompressionOptions configures the lossy recompression Sanitize
// applies to a document's existing images when ConvertPagesToImages is not
// set. Mirrors Aspose.PDF for .NET's ImageCompressionOptions (consumed by
// HiddenDataSanitizationOptions.ImageCompressionOptions); implemented here
// as a thin mapping onto the existing (*Document).OptimizeImages.
type ImageCompressionOptions struct {
	// ResizeImages downscales images above MaxResolution DPI. Leaving
	// MaxResolution at its zero value together with ResizeImages true is
	// rejected by Sanitize (it would otherwise silently downscale nothing).
	ResizeImages bool
	// MaxResolution is the DPI ceiling used when ResizeImages is set.
	MaxResolution float64
	// CompressImages converts opaque PNG images to JPEG.
	CompressImages bool
}

// AllHiddenDataSanitizationOptions returns every removal and flattening
// option enabled, leaving page rasterization and image compression off —
// either can noticeably change how the document looks, so the caller opts
// into ConvertPagesToImages/ImageDPI or ImageCompressionOptions separately.
// Mirrors Aspose.PDF for .NET's static HiddenDataSanitizationOptions.All().
func AllHiddenDataSanitizationOptions() HiddenDataSanitizationOptions {
	return HiddenDataSanitizationOptions{
		RemoveMetadata:                  true,
		RemoveAttachments:               true,
		RemoveJavaScriptsAndActions:     true,
		RemoveAnnotations:               true,
		RemoveSearchIndexAndPrivateInfo: true,
		FlattenForms:                    true,
		FlattenLayers:                   true,
	}
}

// HiddenDataSanitizationResult reports what Sanitize changed.
type HiddenDataSanitizationResult struct {
	MetadataRemoved    bool
	AttachmentsRemoved int
	ActionsRemoved     bool
	PrivateInfoRemoved int
	FormsFlattened     bool
	LayersRemoved      int
	RedactionsApplied  bool
	AnnotationsRemoved int
	PagesRasterized    int
	ImagesOptimized    int
}

// Sanitize removes hidden data per opts and returns a report of what
// changed. Mirrors the intent of Aspose.PDF for .NET's
// Aspose.Pdf.Security.HiddenDataSanitization.HiddenDataSanitizer +
// HiddenDataSanitizationOptions, composed here over this library's
// existing primitives (ClearInfo/ClearXMP, EmbeddedFiles, JavaScript/action
// stripping, ApplyRedactions, annotation deletion, Document.Flatten,
// optional-content removal, page rasterization, OptimizeImages) rather than
// new mechanics.
//
// Order: metadata -> attachments -> scripts/actions -> search index and
// private data -> flatten forms -> flatten layers -> apply redactions and
// remove annotations -> rasterize pages (or recompress existing images).
// Rasterization runs last, after every other step has already narrowed
// what there is to rasterize, matching Aspose's own ordering note.
// RemoveUnusedObjects runs once at the end when anything was removed,
// reclaiming whatever the steps above orphaned (OCG dictionaries,
// filespecs, action objects, …).
//
// On error, res still reports whatever an earlier step already completed
// in this call, including a step that failed partway through (e.g.
// ConvertPagesToImages: res.PagesRasterized counts the pages already
// rasterized in place before the failing one) — mirroring
// FlattenTransparency's own partial-progress-on-error convention.
func (d *Document) Sanitize(opts HiddenDataSanitizationOptions) (HiddenDataSanitizationResult, error) {
	var res HiddenDataSanitizationResult
	if opts.ImageDPI < 0 {
		return res, fmt.Errorf("asposepdf: Sanitize: ImageDPI must be >= 0, got %v", opts.ImageDPI)
	}
	if ico := opts.ImageCompressionOptions; ico != nil {
		if ico.MaxResolution < 0 {
			return res, fmt.Errorf("asposepdf: Sanitize: ImageCompressionOptions.MaxResolution must be >= 0, got %v", ico.MaxResolution)
		}
		if ico.ResizeImages && ico.MaxResolution == 0 {
			return res, fmt.Errorf("asposepdf: Sanitize: ImageCompressionOptions.ResizeImages is set but MaxResolution is 0, which would silently downscale nothing")
		}
	}

	didWork := false

	if opts.RemoveMetadata {
		d.ClearInfo()
		d.ClearXMP()
		res.MetadataRemoved = true
		didWork = true
	}

	if opts.RemoveAttachments {
		before := len(d.EmbeddedFiles().Names()) + len(d.pdfaFileAttachmentAnnotations())
		d.removePDFAEmbeddedFiles()
		after := len(d.EmbeddedFiles().Names()) + len(d.pdfaFileAttachmentAnnotations())
		res.AttachmentsRemoved = before - after
		didWork = true
	}

	if opts.RemoveJavaScriptsAndActions {
		d.stripPDFAActions()
		res.ActionsRemoved = true
		didWork = true
	}

	if opts.RemoveSearchIndexAndPrivateInfo {
		res.PrivateInfoRemoved = d.removeSearchIndexAndPrivateInfo()
		didWork = true
	}

	if opts.FlattenForms {
		if err := d.Flatten(); err != nil {
			return res, fmt.Errorf("asposepdf: Sanitize: FlattenForms: %w", err)
		}
		res.FormsFlattened = true
		didWork = true
	}

	if opts.FlattenLayers {
		res.LayersRemoved = d.flattenLayers()
		didWork = true
	}

	if opts.RemoveAnnotations {
		if err := d.ApplyRedactions(); err != nil {
			return res, fmt.Errorf("asposepdf: Sanitize: RemoveAnnotations: %w", err)
		}
		res.RedactionsApplied = true
		res.AnnotationsRemoved = d.removeNonWidgetAnnotations()
		didWork = true
	}

	if opts.ConvertPagesToImages {
		dpi := opts.ImageDPI
		if dpi <= 0 {
			dpi = DefaultDPI
		}
		pages := d.Pages()
		for i, p := range pages {
			if err := p.flattenTransparency(dpi); err != nil {
				// Pages [0,i) were already rasterized in place before this
				// one failed -- report that partial progress rather than
				// silently claiming nothing happened.
				res.PagesRasterized = i
				if didWork || i > 0 {
					d.RemoveUnusedObjects()
				}
				return res, fmt.Errorf("asposepdf: Sanitize: ConvertPagesToImages: page %d: %w", p.Number(), err)
			}
		}
		res.PagesRasterized = len(pages)
		didWork = true
	} else if ico := opts.ImageCompressionOptions; ico != nil && (ico.ResizeImages || ico.CompressImages) {
		maxDPI := 0.0
		if ico.ResizeImages {
			maxDPI = ico.MaxResolution
		}
		n, err := d.OptimizeImages(OptimizeImageOptions{MaxDPI: maxDPI, ConvertPNGToJPEG: ico.CompressImages})
		if err != nil {
			return res, fmt.Errorf("asposepdf: Sanitize: ImageCompressionOptions: %w", err)
		}
		res.ImagesOptimized = n
		didWork = true
	}

	if didWork {
		d.RemoveUnusedObjects()
	}
	return res, nil
}

// SanitizeAllToImages rasterizes every page of doc at dpi (0 -> 150) after
// removing every other kind of hidden data first. Mirrors Aspose.PDF for
// .NET's static HiddenDataSanitizer.SanitizeAllToImages.
func SanitizeAllToImages(doc *Document, dpi float64) (HiddenDataSanitizationResult, error) {
	opts := AllHiddenDataSanitizationOptions()
	opts.ConvertPagesToImages = true
	opts.ImageDPI = dpi
	return doc.Sanitize(opts)
}

// removeNonWidgetAnnotations deletes every annotation on every page except
// WidgetAnnotations (form fields, left to FlattenForms). Returns the count
// removed.
func (d *Document) removeNonWidgetAnnotations() int {
	removed := 0
	for _, p := range d.Pages() {
		coll := p.Annotations()
		for _, a := range coll.All() {
			if a.AnnotationType() == AnnotationTypeWidget {
				continue
			}
			if coll.Delete(a) {
				removed++
			}
		}
	}
	return removed
}

// removeSearchIndexAndPrivateInfo strips /PieceInfo (private,
// producer-specific application data) from the catalog and every page,
// plus any embedded file named like an Acrobat Catalog full-text search
// index (*.pdx). Returns the number of items removed.
func (d *Document) removeSearchIndexAndPrivateInfo() int {
	n := 0
	if _, ok := d.catalog["/PieceInfo"]; ok {
		delete(d.catalog, "/PieceInfo")
		n++
	}
	for _, p := range d.Pages() {
		pd := p.pageDict()
		if pd == nil {
			continue
		}
		if _, ok := pd["/PieceInfo"]; ok {
			delete(pd, "/PieceInfo")
			n++
		}
	}
	for _, name := range d.EmbeddedFiles().Names() {
		if strings.HasSuffix(strings.ToLower(name), ".pdx") && d.EmbeddedFiles().Remove(name) {
			n++
		}
	}
	pages := d.Pages()
	for _, fa := range d.pdfaFileAttachmentAnnotations() {
		if !strings.HasSuffix(strings.ToLower(fa.FileName()), ".pdx") {
			continue
		}
		idx := fa.PageIndex()
		if idx < 1 || idx > len(pages) {
			continue
		}
		if pages[idx-1].Annotations().Delete(fa) {
			n++
		}
	}
	return n
}

// flattenLayers removes the optional-content configuration
// (/Catalog/OCProperties) and drops every /Resources/Properties entry on
// every page that points at an OCG or OCMD, so content that used to be
// conditionally hidden under a layer is unconditionally visible — a
// missing /OCProperties means no optional content group is ever reported
// OFF (ocOffSet, render_oc.go). Returns the number of layers that existed
// before flattening; the now-unreachable layer dictionaries (and their
// /Name) are reclaimed by the caller's RemoveUnusedObjects pass.
func (d *Document) flattenLayers() int {
	n := len(d.Layers())
	if n == 0 {
		return 0
	}
	delete(d.catalog, "/OCProperties")
	for _, p := range d.Pages() {
		pd := p.pageDict()
		if pd == nil {
			continue
		}
		resources, ok := resolveRefToDict(d.objects, pd["/Resources"])
		if !ok {
			continue
		}
		props, ok := resolveRefToDict(d.objects, resources["/Properties"])
		if !ok {
			continue
		}
		for name, v := range props {
			dict, ok := resolveRefToDict(d.objects, v)
			if !ok {
				continue
			}
			switch dictGetName(dict, "/Type") {
			case "/OCG", "/OCMD":
				delete(props, name)
			}
		}
	}
	return n
}
