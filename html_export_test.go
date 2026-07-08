// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func makeHTMLDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("Hello HTML export", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740}))
	mustNoErr(t, p.AddText("Second line in Courier", pdf.TextStyle{Font: pdf.FontCourier, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 650, URX: 545, URY: 680}))
	mustNoErr(t, doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	p2, _ := doc.Page(2)
	mustNoErr(t, p2.AddText("Page two", pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}))
	return doc
}

// TestWriteHTMLStructure: the output carries one .page div per page, a valid
// base64 PNG background each, and transparent text spans with the page text.
func TestWriteHTMLStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := makeHTMLDoc(t).WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Title: "T"}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if got := strings.Count(s, `<div class="page"`); got != 2 {
		t.Errorf("page divs = %d, want 2", got)
	}
	for _, want := range []string{
		"<title>T</title>",
		"Hello HTML export",
		"Second line in Courier",
		"Page two",
		`class="f-mono"`,  // Courier maps to the mono family
		`class="f-serif"`, // Times maps to the serif family
		"font-weight:bold",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// Every background decodes as a real PNG.
	re := regexp.MustCompile(`data:image/png;base64,([A-Za-z0-9+/=]+)`)
	ms := re.FindAllStringSubmatch(s, -1)
	if len(ms) != 2 {
		t.Fatalf("embedded PNGs = %d, want 2", len(ms))
	}
	for i, m := range ms {
		raw, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			t.Fatalf("page %d: bad base64: %v", i+1, err)
		}
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			t.Fatalf("page %d: bad PNG: %v", i+1, err)
		}
	}
}

// TestWriteHTMLEscaping: text with HTML metacharacters is escaped.
func TestWriteHTMLEscaping(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("a < b & c > d", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720}))
	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "a &lt; b &amp; c &gt; d") {
		t.Error("metacharacters not escaped in the text layer")
	}
}

// TestWriteHTMLTextMode: HTMLModeText emits a visible text layer with real
// colour and style, and the background raster carries no glyphs (a text-only
// page renders as a blank white image).
func TestWriteHTMLTextMode(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	blue := pdf.Color{R: 0, G: 0, B: 0.8, A: 1}
	mustNoErr(t, p.AddText("Visible blue text", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 18, Color: &blue},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}))

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	for _, want := range []string{
		`<div class="tv">`,
		"Visible blue text",
		"color:#0000cc",
		"font-weight:bold",
		`loading="lazy"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if strings.Contains(s, `<div class="tl">`) {
		t.Error("text mode must not emit the transparent layer")
	}

	// The background of a text-only page is pure white — glyphs suppressed.
	re := regexp.MustCompile(`data:image/png;base64,([A-Za-z0-9+/=]+)`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		t.Fatal("no embedded PNG background")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				t.Fatalf("background pixel (%d,%d) = %x/%x/%x, want white (glyphs not suppressed?)", x, y, r, g, b)
			}
		}
	}
}

// TestWriteHTMLPagesOption: Pages selects a subset; anchors keep source
// numbers; out-of-range pages error.
func TestWriteHTMLPagesOption(t *testing.T) {
	doc := makeHTMLDoc(t)

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Pages: []int{2}}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if got := strings.Count(s, `<div class="page"`); got != 1 {
		t.Errorf("page divs = %d, want 1", got)
	}
	if !strings.Contains(s, `id="page2"`) {
		t.Error("subset page lost its source-numbered anchor")
	}
	if strings.Contains(s, "Hello HTML export") || !strings.Contains(s, "Page two") {
		t.Error("subset exported the wrong page's text")
	}

	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{Pages: []int{3}}); err == nil {
		t.Error("out-of-range page did not error")
	}
}

// TestWriteHTMLLinks: link annotations become positioned <a> overlays in both
// modes — /URI to the outside, /GoTo to a #pageN anchor.
func TestWriteHTMLLinks(t *testing.T) {
	doc := makeHTMLDoc(t)
	p, _ := doc.Page(1)

	uri := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 740})
	uri.SetAction(pdf.NewGoToURIAction("https://example.com/a?b=1&c=2"))
	mustNoErr(t, p.Annotations().Add(uri))
	goto2 := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 650, URX: 200, URY: 680})
	goto2.SetAction(pdf.NewGoToAction(2, 700))
	mustNoErr(t, p.Annotations().Add(goto2))

	for _, mode := range []pdf.HTMLMode{pdf.HTMLModeFaithful, pdf.HTMLModeText} {
		var buf bytes.Buffer
		if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: mode}); err != nil {
			t.Fatal(err)
		}
		s := buf.String()
		if !strings.Contains(s, `href="https://example.com/a?b=1&amp;c=2"`) {
			t.Errorf("mode %d: external link missing or unescaped", mode)
		}
		if !strings.Contains(s, `href="#page2"`) {
			t.Errorf("mode %d: GoTo link missing", mode)
		}
		if !strings.Contains(s, `class="lnk"`) {
			t.Errorf("mode %d: no positioned link overlays", mode)
		}
	}
}

// TestWriteHTMLFontEmbedding: in text mode an embedded TTF becomes a WOFF
// @font-face and its spans reference the ef-class; NoFontEmbedding turns it
// all off. Runs on the subset font (CIDToGIDMap stream) — the common shape
// of real-world PDFs.
func TestWriteHTMLFontEmbedding(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont(filepath.Join("testdata", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("Проверка WOFF-шрифта", pdf.TextStyle{Font: font, Size: 16},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}))
	if _, err := doc.SubsetFonts(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "@font-face { font-family:'ef0'") {
		t.Fatal("no @font-face for the embedded font")
	}
	if !strings.Contains(s, `class="ef0"`) {
		t.Error("no span references the embedded face")
	}
	re := regexp.MustCompile(`data:font/woff;base64,([A-Za-z0-9+/=]+)`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		t.Fatal("no WOFF data URL")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 4 || string(raw[0:4]) != "wOFF" {
		t.Fatalf("data URL is not a WOFF (starts %q)", raw[:4])
	}
	if len(raw) > 100*1024 {
		t.Errorf("WOFF of a subset font is %d KB — subsetting lost?", len(raw)/1024)
	}
	// The embedded-face span must not carry the substitute width fitting.
	spanRe := regexp.MustCompile(`<span class="ef0"[^>]*>`)
	if sp := spanRe.FindString(s); sp == "" || strings.Contains(sp, "scaleX") {
		t.Errorf("ef0 span missing or width-fitted: %q", sp)
	}

	buf.Reset()
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText, NoFontEmbedding: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "@font-face") {
		t.Error("NoFontEmbedding still emitted @font-face")
	}
}

// TestWriteHTMLNativeMode: HTMLModeNative replaces the raster background
// with an inline SVG layer — vector shapes become <path> elements with true
// curves and native stroke attributes, images become <image> elements, and
// content SVG cannot express (a gradient fill) becomes a positioned raster
// patch. The visible text layer stays.
func TestWriteHTMLNativeMode(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	red := pdf.Color{R: 1, G: 0, B: 0, A: 1}
	blue := pdf.Color{R: 0, G: 0, B: 1, A: 1}
	mustNoErr(t, p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700},
		pdf.ShapeStyle{FillColor: &red}))
	mustNoErr(t, p.DrawCircle(pdf.Point{X: 300, Y: 650}, 40,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Color: &blue, Width: 2, DashPattern: []float64{6, 3}}}))
	grad := pdf.NewLinearGradient(50, 400, 200, 400,
		pdf.GradientStop{Offset: 0, Color: red}, pdf.GradientStop{Offset: 1, Color: blue})
	mustNoErr(t, p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 400, URX: 200, URY: 500},
		pdf.ShapeStyle{FillGradient: grad}))
	mustNoErr(t, p.AddText("Native mode text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 730, URX: 545, URY: 760}))

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeNative}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, `<svg class="vg"`) {
		t.Fatal("no SVG layer")
	}
	if strings.Contains(s, `alt="page`) {
		t.Error("native mode still emits a raster page background")
	}
	if !strings.Contains(s, `fill="#ff0000"`) {
		t.Error("no red vector fill")
	}
	if !strings.Contains(s, `stroke="#0000ff"`) || !strings.Contains(s, "stroke-dasharray=") {
		t.Error("no dashed blue vector stroke")
	}
	// The circle must survive as Bézier curves, not a flattened polygon.
	pathRe := regexp.MustCompile(`<path d="[^"]*C[^"]*"[^>]*stroke="#0000ff"`)
	if !pathRe.MatchString(s) {
		t.Error("stroked circle lost its curve segments")
	}
	// The gradient fill has no direct SVG form here — it must arrive as a
	// positioned raster patch inside the SVG.
	if !strings.Contains(s, `<image x="`) {
		t.Error("gradient fill did not produce a raster patch")
	}
	if !strings.Contains(s, `<div class="tv">`) || !strings.Contains(s, "Native mode text") {
		t.Error("visible text layer missing")
	}
}

// TestWriteHTMLNativeImage: an added raster picture is exported as an SVG
// <image> with the original bytes (PNG passthrough), not baked into a page
// raster.
func TestWriteHTMLNativeImage(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddImage(filepath.Join("testdata", "aspose-logo.png"),
		pdf.Rectangle{LLX: 100, LLY: 500, URX: 300, URY: 700}))

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeNative}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	re := regexp.MustCompile(`<image [^>]*transform="matrix\([^)]+\) translate\(0 1\) scale\(1 -1\)"[^>]*href="data:image/png;base64,([A-Za-z0-9+/=]+)"`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		t.Fatal("no CTM-placed SVG <image> with PNG data")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("embedded image is not a valid PNG: %v", err)
	}
}

// TestSaveHTMLRealFile: a real PDF exports to a well-formed non-empty file.
func TestSaveHTMLRealFile(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("result_files", "TestSaveHTMLRealFile")
	mustNoErr(t, os.MkdirAll(dir, 0o755))
	out := filepath.Join(dir, "out.html")
	if err := doc.SaveHTML(out, pdf.HTMLSaveOptions{DPI: 96}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "<!DOCTYPE html>") || !strings.HasSuffix(strings.TrimSpace(s), "</html>") {
		t.Error("output is not a complete HTML document")
	}
	if strings.Count(s, `<div class="page"`) != doc.PageCount() {
		t.Errorf("page divs = %d, want %d", strings.Count(s, `<div class="page"`), doc.PageCount())
	}
}
