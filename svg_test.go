// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestPage_AddSVG_FromPath(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/all_shapes.svg", pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save("result_files/TestPage_AddSVG_FromPath.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVGFromStream(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/all_shapes.svg")
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVGFromStream(bytes.NewReader(data), pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_InvalidXMLReturnsError(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	tmp := "result_files/_invalid.svg"
	os.WriteFile(tmp, []byte("<svg><not-closed"), 0644)
	defer os.Remove(tmp)
	if err := page.AddSVG(tmp, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err == nil {
		t.Error("expected error for malformed XML")
	}
}

func TestDocument_LoadSVG(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, err := doc.LoadSVG("testdata/svg/all_shapes.svg")
	if err != nil {
		t.Fatal(err)
	}
	if svg == nil {
		t.Fatal("nil SVG")
	}
}

func TestDocument_LoadSVGFromStream(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/all_shapes.svg")
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, err := doc.LoadSVGFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if svg == nil {
		t.Fatal("nil")
	}
}

func TestDocument_AddSVGWatermark_AllPages(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGWatermark("testdata/aspose-logo.svg"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		p, err := doc.Page(i)
		if err != nil {
			t.Errorf("page %d error: %v", i, err)
		} else if p == nil {
			t.Errorf("page %d nil", i)
		}
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	doc.Save("result_files/TestDocument_AddSVGWatermark_AllPages.pdf")
}

func TestDocument_AddSVGWatermark_SelectPages(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGWatermark("testdata/aspose-logo.svg", 1, 3); err != nil {
		t.Fatal(err)
	}
}

func TestDocument_AddSVGObjectWatermark(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, _ := doc.LoadSVG("testdata/aspose-logo.svg")
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGObjectWatermark(svg); err != nil {
		t.Fatal(err)
	}
}

func TestSVG_Inspectors(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, _ := doc.LoadSVG("testdata/svg/rect.svg")
	w, h := svg.Size()
	if w != 100 || h != 50 {
		t.Errorf("Size() = (%g, %g), want (100, 50)", w, h)
	}
	vx, vy, vw, vh := svg.ViewBox()
	if vx != 0 || vy != 0 || vw != 100 || vh != 50 {
		t.Errorf("ViewBox() = (%g, %g, %g, %g)", vx, vy, vw, vh)
	}
}

func TestAddSVG_AsposeLogoBlackTextRenders(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 750, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_AsposeLogoBlackTextRenders.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	// Re-open and validate structural integrity
	reopened, err := pdf.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.PageCount() != 1 {
		t.Errorf("page count = %d", reopened.PageCount())
	}
	report, err := pdf.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Errorf("validation failed: %+v", report.Issues)
	}
}

func TestAddSVG_AsposeLogoGradientShapesSkippedSilently(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	// Must not error — gradient refs are silently skipped per Phase 2 scope
	if err := page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 0, LLY: 0, URX: 595, URY: 100}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAddSVGWatermark_AsposeLogoOnEveryPage(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i := 0; i < 4; i++ {
		doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	}
	svg, _ := doc.LoadSVG("testdata/aspose-logo.svg")
	if err := doc.AddSVGObjectWatermark(svg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVGWatermark_AsposeLogoOnEveryPage.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	report, err := pdf.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Errorf("validation failed: %+v", report.Issues)
	}
}

func TestAddSVG_AES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/all_shapes.svg", pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "u",
		OwnerPassword: "o",
		Algorithm:     pdf.EncryptionAlgAES128,
	})
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_AES128Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenWithPassword(out, "u")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.PageCount() != 1 {
		t.Errorf("page count after roundtrip = %d", reopened.PageCount())
	}
}

func TestAddSVG_AES256Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "u",
		OwnerPassword: "o",
		Algorithm:     pdf.EncryptionAlgAES256,
	})
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_AES256Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestAddSVG_RC4Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/all_shapes.svg", pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "u",
		OwnerPassword: "o",
		Algorithm:     pdf.EncryptionAlgRC4_128,
	})
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_RC4Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestAddSVG_AsposeLogoFullColorAfterPhase3a(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_AsposeLogoFullColorAfterPhase3a.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	report, err := pdf.Validate(out)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		t.Errorf("validation failed: %+v", report.Issues)
	}
	reopened, err := pdf.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.PageCount() != 1 {
		t.Errorf("page count = %d", reopened.PageCount())
	}
}

func TestAddSVG_GradientAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "u",
		OwnerPassword: "o",
		Algorithm:     pdf.EncryptionAlgAES128,
	})
	os.MkdirAll("result_files", 0755)
	out := "result_files/TestAddSVG_GradientAES128Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestAddSVG_GradientAES256Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "u",
		OwnerPassword: "o",
		Algorithm:     pdf.EncryptionAlgAES256,
	})
	os.MkdirAll("result_files", 0755)
	out := "result_files/TestAddSVG_GradientAES256Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_TextWithFontResolver(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	deja, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	doc.SetSVGFontResolver(func(family string, bold, italic bool) pdf.Font {
		if strings.EqualFold(family, "DejaVu Sans") {
			return deja
		}
		return nil
	})
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/text_cyrillic.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 400, URY: 700}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save("result_files/TestPage_AddSVG_TextWithFontResolver.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_TextHeuristicWithoutResolver(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/text_basic.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 400, URY: 700}); err != nil {
		t.Fatal(err)
	}
	// No resolver — falls back to heuristic (Arial → FontHelvetica)
}

func TestAddSVG_TextAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	page.AddSVG("testdata/svg/text_tspan.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 400, URY: 700})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "u", OwnerPassword: "o", Algorithm: pdf.EncryptionAlgAES128,
	})
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_TextAES128Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_ImageInline(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/image_inline_png.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save("result_files/TestPage_AddSVG_ImageInline.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_UseRefs(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/use_simple.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_UseRefs.pdf")
}

func TestPage_AddSVG_ClipPath(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/clippath_circle.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_ClipPath.pdf")
}

func TestAddSVG_DefsImageClipAES128Roundtrip(t *testing.T) {
	for _, fixture := range []string{"image_inline_png.svg", "use_simple.svg", "clippath_circle.svg"} {
		t.Run(fixture, func(t *testing.T) {
			doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
			page, _ := doc.Page(1)
			if err := page.AddSVG("testdata/svg/"+fixture,
				pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
				t.Fatal(err)
			}
			doc.SetEncryption(pdf.EncryptionOptions{
				UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128,
			})
			os.MkdirAll("result_files", 0755)
			out := "result_files/TestAddSVG_AES_" + fixture + ".pdf"
			if err := doc.Save(out); err != nil {
				t.Fatal(err)
			}
			if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPage_AddSVG_MaskCircle(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/mask_circle.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save("result_files/TestPage_AddSVG_MaskCircle.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestAddSVG_MaskAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/mask_circle.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128,
	})
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestAddSVG_MaskAES128Roundtrip.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_StyleClasses(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/style_classes.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_StyleClasses.pdf")
}

func TestPage_AddSVG_FilterDropShadow(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/filter_dropshadow.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_FilterDropShadow.pdf")
}

func TestPage_AddSVG_MarkerArrow(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/marker_arrow.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_MarkerArrow.pdf")
}

func TestAddSVG_Phase3dAES128Roundtrip(t *testing.T) {
	for _, fixture := range []string{
		"style_classes.svg",
		"mask_circle.svg",
		"filter_dropshadow.svg",
		"marker_arrow.svg",
	} {
		t.Run(fixture, func(t *testing.T) {
			doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
			page, _ := doc.Page(1)
			if err := page.AddSVG("testdata/svg/"+fixture,
				pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
				t.Fatal(err)
			}
			doc.SetEncryption(pdf.EncryptionOptions{
				UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128,
			})
			os.MkdirAll("result_files", 0755)
			out := "result_files/TestAddSVG_3d_AES_" + fixture + ".pdf"
			if err := doc.Save(out); err != nil {
				t.Fatal(err)
			}
			if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
