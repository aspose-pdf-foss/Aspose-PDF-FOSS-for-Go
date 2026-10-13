// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func buildSanitizeDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Visible content", pdf.TextStyle{Size: 12}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 740}); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSanitizeRemoveMetadata(t *testing.T) {
	doc := buildSanitizeDoc(t)
	doc.SetInfo(pdf.DocumentInfo{Title: "Secret Title", Author: "Secret Author"})
	if err := doc.SetXMP(pdf.XMPMetadata{Title: "Secret Title"}); err != nil {
		t.Fatal(err)
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{RemoveMetadata: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.MetadataRemoved {
		t.Error("MetadataRemoved = false, want true")
	}
	info, err := doc.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "" || info.Author != "" {
		t.Errorf("Info not cleared: %+v", info)
	}
	xmp, err := doc.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if !xmp.IsEmpty() {
		t.Error("XMP not cleared")
	}
}

func TestSanitizeRemoveAttachments(t *testing.T) {
	doc := buildSanitizeDoc(t)
	if _, err := doc.EmbeddedFiles().AddFromStream("secret.txt", strings.NewReader("hidden data")); err != nil {
		t.Fatal(err)
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{RemoveAttachments: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.AttachmentsRemoved != 1 {
		t.Errorf("AttachmentsRemoved = %d, want 1", res.AttachmentsRemoved)
	}
	if n := doc.EmbeddedFiles().Count(); n != 0 {
		t.Errorf("EmbeddedFiles().Count() = %d, want 0", n)
	}
}

func TestSanitizeRemoveJavaScriptsAndActions(t *testing.T) {
	doc := buildSanitizeDoc(t)
	doc.SetOpenAction(pdf.NewJavaScriptAction("app.alert('hidden');"))

	if _, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{RemoveJavaScriptsAndActions: true}); err != nil {
		t.Fatal(err)
	}
	if act := doc.OpenAction(); act != nil {
		t.Errorf("OpenAction = %v, want nil", act)
	}
}

func TestSanitizeFlattenForms(t *testing.T) {
	doc := buildSanitizeDoc(t)
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620}, "Name"); err != nil {
		t.Fatal(err)
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{FlattenForms: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.FormsFlattened {
		t.Error("FormsFlattened = false, want true")
	}
	if n := len(doc.Form().Fields()); n != 0 {
		t.Errorf("Form().Fields() has %d fields after flatten, want 0", n)
	}
}

// TestSanitizeRemoveAnnotationsKeepsWidgets confirms RemoveAnnotations
// applies pending redactions, deletes a non-widget annotation, and leaves a
// form-field widget alone (left to FlattenForms) -- a widget deleted here
// without also clearing its field from /AcroForm/Fields would dangle.
func TestSanitizeRemoveAnnotationsKeepsWidgets(t *testing.T) {
	doc := buildSanitizeDoc(t)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	secretRect := pdf.Rectangle{LLX: 50, LLY: 650, URX: 500, URY: 680}
	if err := p.AddText("SECRETWORD", pdf.TextStyle{Size: 12}, secretRect); err != nil {
		t.Fatal(err)
	}
	h := pdf.NewHighlightAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
	if err := p.Annotations().Add(h); err != nil {
		t.Fatal(err)
	}
	ra := pdf.NewRedactAnnotation(p, secretRect)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 500, URX: 200, URY: 520}, "Name"); err != nil {
		t.Fatal(err)
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{RemoveAnnotations: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.RedactionsApplied {
		t.Error("RedactionsApplied = false, want true")
	}
	// ApplyRedactions already removes the redact annotation itself as part
	// of applying it, so only the highlight is left for the
	// annotation-deletion pass to count.
	if res.AnnotationsRemoved != 1 {
		t.Errorf("AnnotationsRemoved = %d, want 1 (highlight; redact already removed by ApplyRedactions)", res.AnnotationsRemoved)
	}
	pages, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(pages, ""), "SECRETWORD") {
		t.Error("redacted text still extracts after Sanitize(RemoveAnnotations)")
	}
	if !doc.Form().HasField("Name") {
		t.Error("widget field was removed by RemoveAnnotations; it should be left to FlattenForms")
	}
	count := 0
	for _, a := range p.Annotations().All() {
		if a.AnnotationType() != pdf.AnnotationTypeWidget {
			count++
		}
	}
	if count != 0 {
		t.Errorf("%d non-widget annotations remain after Sanitize(RemoveAnnotations)", count)
	}
}

func TestSanitizeFlattenLayers(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	layer := doc.AddLayer("Confidential")
	layer.SetVisible(false)
	if err := p.BeginLayer(layer); err != nil {
		t.Fatal(err)
	}
	mark := pdf.Rectangle{LLX: 10, LLY: 10, URX: 60, URY: 60}
	red := &pdf.Color{R: 1, G: 0, B: 0, A: 1}
	if err := p.DrawRectangle(mark, pdf.ShapeStyle{FillColor: red}); err != nil {
		t.Fatal(err)
	}
	if err := p.EndLayer(); err != nil {
		t.Fatal(err)
	}

	before, err := p.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if !isWhiteAt(before) {
		t.Fatal("precondition failed: hidden-layer mark already visible before Sanitize")
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{FlattenLayers: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.LayersRemoved != 1 {
		t.Errorf("LayersRemoved = %d, want 1", res.LayersRemoved)
	}
	if n := len(doc.Layers()); n != 0 {
		t.Errorf("Layers() reports %d after FlattenLayers, want 0", n)
	}

	p2, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	after, err := p2.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if isWhiteAt(after) {
		t.Error("previously-hidden layer content still not visible after FlattenLayers")
	}
}

// isWhiteAt reports whether the pixel near the page's bottom-left corner
// (where the test's mark rectangle, PDF-space [10,10]-[60,60], is drawn) is
// (near) white. RenderImage is Y-down while PDF space is Y-up, so a mark
// near the bottom of the page lands near the bottom of the rendered image;
// sampling at (30, height-30) falls inside the mark regardless of the
// rendering DPI.
func isWhiteAt(img image.Image) bool {
	bounds := img.Bounds()
	x := bounds.Min.X + 30
	y := bounds.Max.Y - 30
	c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
	return c.R > 240 && c.G > 240 && c.B > 240
}

func TestSanitizeConvertPagesToImages(t *testing.T) {
	doc := buildSanitizeDoc(t)

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{ConvertPagesToImages: true, ImageDPI: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.PagesRasterized != 1 {
		t.Errorf("PagesRasterized = %d, want 1", res.PagesRasterized)
	}
	pages, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(strings.Join(pages, "")) != "" {
		t.Errorf("text survives ConvertPagesToImages: %q", pages)
	}
	infos, err := doc.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || len(infos[0]) != 1 {
		t.Fatalf("ImageInfos() = %v, want exactly one image on page 1", infos)
	}
}

func TestSanitizeImageCompressionOptions(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 6), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := p.AddImageFromStream(bytes.NewReader(buf.Bytes()), pdf.Rectangle{LLX: 50, LLY: 50, URX: 300, URY: 300}); err != nil {
		t.Fatal(err)
	}

	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{
		ImageCompressionOptions: &pdf.ImageCompressionOptions{CompressImages: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ImagesOptimized != 1 {
		t.Errorf("ImagesOptimized = %d, want 1", res.ImagesOptimized)
	}
	infos, err := doc.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || len(infos[0]) != 1 || infos[0][0].Format != pdf.ImageFormatJPEG {
		t.Errorf("image not converted to JPEG: %+v", infos)
	}
}

func TestSanitizeConvertPagesToImagesIgnoresImageCompressionOptions(t *testing.T) {
	doc := buildSanitizeDoc(t)
	res, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{
		ConvertPagesToImages:    true,
		ImageDPI:                50,
		ImageCompressionOptions: &pdf.ImageCompressionOptions{CompressImages: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.PagesRasterized != 1 {
		t.Errorf("PagesRasterized = %d, want 1", res.PagesRasterized)
	}
	if res.ImagesOptimized != 0 {
		t.Errorf("ImagesOptimized = %d, want 0 (ImageCompressionOptions must be ignored when ConvertPagesToImages is set)", res.ImagesOptimized)
	}
}

func TestSanitizeAllOptions(t *testing.T) {
	doc := buildSanitizeDoc(t)
	doc.SetInfo(pdf.DocumentInfo{Title: "Secret"})
	if _, err := doc.EmbeddedFiles().AddFromStream("secret.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	doc.SetOpenAction(pdf.NewJavaScriptAction("app.alert('x');"))

	opts := pdf.AllHiddenDataSanitizationOptions()
	res, err := doc.Sanitize(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.MetadataRemoved || !res.FormsFlattened || !res.ActionsRemoved || !res.RedactionsApplied {
		t.Errorf("AllHiddenDataSanitizationOptions did not run every step: %+v", res)
	}
	info, _ := doc.Info()
	if info.Title != "" {
		t.Error("metadata survived AllHiddenDataSanitizationOptions")
	}
	if doc.EmbeddedFiles().Count() != 0 {
		t.Error("attachment survived AllHiddenDataSanitizationOptions")
	}
	if doc.OpenAction() != nil {
		t.Error("JavaScript open action survived AllHiddenDataSanitizationOptions")
	}

	// Lossless-for-content check: the saved document still opens and still
	// shows the ordinary visible text (nothing in All() rasterizes pages).
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := reopened.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(pages, ""), "Visible content") {
		t.Error("ordinary visible text lost by AllHiddenDataSanitizationOptions")
	}
}

func TestSanitizeNegativeOptionsError(t *testing.T) {
	doc := buildSanitizeDoc(t)
	if _, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{ImageDPI: -1}); err == nil {
		t.Error("ImageDPI = -1: want error")
	}
	if _, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{
		ImageCompressionOptions: &pdf.ImageCompressionOptions{MaxResolution: -1},
	}); err == nil {
		t.Error("ImageCompressionOptions.MaxResolution = -1: want error")
	}
	// ResizeImages with MaxResolution left at its zero value would
	// silently downscale nothing -- Sanitize rejects it instead.
	if _, err := doc.Sanitize(pdf.HiddenDataSanitizationOptions{
		ImageCompressionOptions: &pdf.ImageCompressionOptions{ResizeImages: true},
	}); err == nil {
		t.Error("ImageCompressionOptions{ResizeImages: true, MaxResolution: 0}: want error")
	}
}
