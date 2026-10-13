// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// TestSanitizeConvertPagesToImagesReportsPartialProgressOnError confirms a
// review finding on the first draft: when ConvertPagesToImages fails partway
// through a multi-page document, the pages rasterized before the failing
// one must still be reported in res.PagesRasterized (and left rasterized in
// the document) rather than the whole call reporting 0 -- mirroring
// FlattenTransparency's own partial-progress-on-error convention.
func TestSanitizeConvertPagesToImagesReportsPartialProgressOnError(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p1, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p1.AddText("page one", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 740}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	p2, err := d.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	// Force page 2's rasterization to fail (renderBox/deviceMatrix reports
	// a degenerate page size for a zero-area MediaBox).
	p2.pageDict()["/MediaBox"] = pdfArray{0.0, 0.0, 0.0, 0.0}

	res, err := d.Sanitize(HiddenDataSanitizationOptions{ConvertPagesToImages: true, ImageDPI: 50})
	if err == nil {
		t.Fatal("Sanitize succeeded; want an error from page 2's degenerate MediaBox")
	}
	if res.PagesRasterized != 1 {
		t.Errorf("PagesRasterized = %d, want 1 (page 1 was already rasterized before page 2 failed)", res.PagesRasterized)
	}
	infos, err := p1.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Errorf("page 1 has %d images, want 1 (it should already be rasterized in the document, not just reported)", len(infos))
	}
}

// TestRemoveSearchIndexAndPrivateInfoStripsPieceInfo covers the part of
// RemoveSearchIndexAndPrivateInfo's scope that has no public getter to
// assert through: /PieceInfo, private producer-specific application data
// (a document-level-and-page-level corner no existing black-box test can
// reach).
func TestRemoveSearchIndexAndPrivateInfoStripsPieceInfo(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	d.catalog = pdfDict{"/PieceInfo": pdfDict{"/Illustrator": pdfDict{"/Private": "data"}}}
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	pd := p.pageDict()
	pd["/PieceInfo"] = pdfDict{"/Illustrator": pdfDict{"/Private": "data"}}

	n := d.removeSearchIndexAndPrivateInfo()
	if n != 2 {
		t.Errorf("removeSearchIndexAndPrivateInfo() = %d, want 2 (catalog + page)", n)
	}
	if _, ok := d.catalog["/PieceInfo"]; ok {
		t.Error("catalog /PieceInfo survived")
	}
	if _, ok := pd["/PieceInfo"]; ok {
		t.Error("page /PieceInfo survived")
	}
}

// TestRemoveSearchIndexAndPrivateInfoRemovesPdxAttachmentOnly confirms only
// the *.pdx-named attachment (the Acrobat Catalog search-index heuristic)
// is removed, not an unrelated attachment -- RemoveAttachments is the
// separate, broader option for "remove everything".
func TestRemoveSearchIndexAndPrivateInfoRemovesPdxAttachmentOnly(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if _, err := d.EmbeddedFiles().AddFromStream("catalog-index.PDX", strings.NewReader("index bytes")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.EmbeddedFiles().AddFromStream("invoice.pdf", strings.NewReader("keep me")); err != nil {
		t.Fatal(err)
	}

	n := d.removeSearchIndexAndPrivateInfo()
	if n != 1 {
		t.Errorf("removeSearchIndexAndPrivateInfo() = %d, want 1", n)
	}
	if d.EmbeddedFiles().Has("catalog-index.PDX") {
		t.Error(".pdx attachment survived")
	}
	if !d.EmbeddedFiles().Has("invoice.pdf") {
		t.Error("unrelated attachment was removed too")
	}
}

// TestFlattenLayersPrunesPageProperties confirms flattenLayers removes
// /Catalog/OCProperties AND the page's /Resources/Properties entry that
// pointed at the OCG -- mutation-tested by temporarily skipping the
// Properties-pruning loop below and confirming
// TestSanitizeFlattenLayers (sanitize_test.go, pixel-level) then fails,
// since deleting only /OCProperties while leaving the page's /OC BDC
// property mapping intact is not exercised by this internal test alone
// (the property entry would still resolve to the now-orphaned-but-not-yet-
// collected OCG dict, which render_oc.go does not consult without
// /OCProperties regardless -- the real risk this guards is the OCG
// dictionary, and its /Name, staying reachable and never reclaimed).
func TestFlattenLayersPrunesPageProperties(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	layer := d.AddLayer("Confidential")
	if err := p.BeginLayer(layer); err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10}, ShapeStyle{FillColor: &Color{R: 1, A: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := p.EndLayer(); err != nil {
		t.Fatal(err)
	}

	pd := p.pageDict()
	resources, ok := resolveRefToDict(d.objects, pd["/Resources"])
	if !ok {
		t.Fatal("page has no /Resources after BeginLayer")
	}
	props, ok := resolveRefToDict(d.objects, resources["/Properties"])
	if !ok || len(props) == 0 {
		t.Fatal("page has no /Resources/Properties after BeginLayer")
	}

	if n := d.flattenLayers(); n != 1 {
		t.Errorf("flattenLayers() = %d, want 1", n)
	}
	if _, ok := d.catalog["/OCProperties"]; ok {
		t.Error("/Catalog/OCProperties survived flattenLayers")
	}
	for name, v := range props {
		dict, ok := resolveRefToDict(d.objects, v)
		if !ok {
			continue
		}
		if t2 := dictGetName(dict, "/Type"); t2 == "/OCG" || t2 == "/OCMD" {
			t.Errorf("page /Resources/Properties still has an OCG/OCMD entry %q after flattenLayers", name)
		}
	}
}

// TestRemoveNonWidgetAnnotationsKeepsWidgets is the direct-call counterpart
// of TestSanitizeRemoveAnnotationsKeepsWidgets (sanitize_test.go): confirms
// the widget-exclusion guard by type, not just by behavior.
func TestRemoveNonWidgetAnnotationsKeepsWidgets(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHighlightAnnotation(p, Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	if err := p.Annotations().Add(h); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Form().AddTextField(1, Rectangle{LLX: 20, LLY: 20, URX: 60, URY: 40}, "Name"); err != nil {
		t.Fatal(err)
	}

	if got := d.removeNonWidgetAnnotations(); got != 1 {
		t.Errorf("removeNonWidgetAnnotations() = %d, want 1", got)
	}
	var kinds []AnnotationType
	for _, a := range p.Annotations().All() {
		kinds = append(kinds, a.AnnotationType())
	}
	if len(kinds) != 1 || kinds[0] != AnnotationTypeWidget {
		t.Errorf("page annotations after removeNonWidgetAnnotations = %v, want exactly [Widget]", kinds)
	}
}
