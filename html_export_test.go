// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
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

// TestWriteHTMLInteractiveForms: with InteractiveForms, AcroForm fields
// become positioned HTML controls carrying values, flags and styling, and
// their widget appearances vanish from the background raster (a page with
// only form fields renders pure white). Without the option, no controls.
func TestWriteHTMLInteractiveForms(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()

	name, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720}, "name")
	mustNoErr(t, err)
	mustNoErr(t, name.SetValue("Иван <Петров>"))
	name.SetMaxLen(30)
	name.SetRequired(true)
	red := pdf.Color{R: 1, G: 0, B: 0, A: 1}
	mustNoErr(t, name.SetStyle(pdf.FieldStyle{BorderColor: &red, BorderWidth: 2, TextSize: 14, TextAlign: pdf.HAlignRight}))

	notes, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 620, URX: 300, URY: 680}, "notes")
	mustNoErr(t, err)
	notes.SetMultiline(true)
	mustNoErr(t, notes.SetValue("line one"))
	notes.SetReadOnly(true)

	_, err = form.AddPasswordField(1, pdf.Rectangle{LLX: 50, LLY: 580, URX: 300, URY: 600}, "pin")
	mustNoErr(t, err)

	agree, err := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 540, URX: 66, URY: 556}, "agree")
	mustNoErr(t, err)
	agree.SetChecked(true)

	color, err := form.AddRadioGroup("color", []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 500, URX: 66, URY: 516}, Export: "red"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 90, LLY: 500, URX: 106, URY: 516}, Export: "blue"},
	})
	mustNoErr(t, err)
	mustNoErr(t, color.SetValue("blue"))

	city, err := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 460, URX: 200, URY: 480}, "city",
		[]pdf.ChoiceOption{{Value: "Praha"}, {Value: "Brno"}})
	mustNoErr(t, err)
	mustNoErr(t, city.SetValue("Brno"))

	pets, err := form.AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 380, URX: 200, URY: 440}, "pets",
		[]pdf.ChoiceOption{{Value: "cat"}, {Value: "dog"}, {Value: "fox"}})
	mustNoErr(t, err)
	pets.SetMultiSelect(true)
	mustNoErr(t, pets.SetSelected(0, 2))

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText, InteractiveForms: true}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	for _, want := range []string{
		`type="text" name="name" tabindex="1" required maxlength="30" value="Иван &lt;Петров&gt;"`,
		`border:2pt solid #ff0000`,
		`font-size:14pt`,
		`text-align:right`,
		`<textarea class="fw" name="notes" tabindex="2" readonly`,
		`>line one</textarea>`,
		`type="password"`,
		`type="checkbox" name="agree" tabindex="4" value="Yes" checked`,
		`type="radio" name="color" value="red" tabindex="5"`,
		`type="radio" name="color" value="blue" tabindex="6" checked`,
		`<select class="fw" name="city"`,
		`<option value="Brno" selected>`,
		` multiple`,
		`<option value="cat" selected>`,
		`<option value="fox" selected>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// Widget appearances must be gone from the background: the page carries
	// nothing but form fields, so the raster must be pure white.
	re := regexp.MustCompile(`data:image/png;base64,([A-Za-z0-9+/=]+)`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		t.Fatal("no background PNG")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	mustNoErr(t, err)
	img, err := png.Decode(bytes.NewReader(raw))
	mustNoErr(t, err)
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				t.Fatalf("background pixel (%d,%d) not white — widget /AP not suppressed", x, y)
			}
		}
	}

	// Without the option no controls are emitted and widgets stay rendered.
	buf.Reset()
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `class="fw"`) {
		t.Error("controls emitted without InteractiveForms")
	}
}

// TestWriteHTMLFormsPhase2: number/date fields become typed inputs (date
// values converted through the format mask to ISO), submit/reset push
// buttons become real form buttons inside a document-level <form>, and an
// unconvertible date mask falls back to a text input.
func TestWriteHTMLFormsPhase2(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()

	price, err := form.AddNumberField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720}, "price",
		pdf.NumberFormatOptions{Decimals: 2, CurrencySymbol: "$", CurrencyPrepend: true})
	mustNoErr(t, err)
	mustNoErr(t, price.SetValue("1234.50"))

	when, err := form.AddDateField(1, pdf.Rectangle{LLX: 50, LLY: 660, URX: 200, URY: 680}, "when", "dd.mm.yyyy")
	mustNoErr(t, err)
	mustNoErr(t, when.SetValue("24.12.2026"))

	badmask, err := form.AddDateField(1, pdf.Rectangle{LLX: 50, LLY: 620, URX: 200, URY: 640}, "badmask", "mmm d, yyyy")
	mustNoErr(t, err)
	mustNoErr(t, badmask.SetValue("Dec 24, 2026"))

	send, err := form.AddPushButton(1, pdf.Rectangle{LLX: 50, LLY: 560, URX: 150, URY: 590}, "send", "Send it")
	mustNoErr(t, err)
	send.SetAction(pdf.NewSubmitFormAction("https://example.com/submit?x=1", nil, 0))

	clear, err := form.AddPushButton(1, pdf.Rectangle{LLX: 170, LLY: 560, URX: 270, URY: 590}, "clear", "Clear")
	mustNoErr(t, err)
	clear.SetAction(pdf.NewResetFormAction(nil))

	static, err := form.AddPushButton(1, pdf.Rectangle{LLX: 290, LLY: 560, URX: 390, URY: 590}, "static", "No action")
	mustNoErr(t, err)
	_ = static

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Mode: pdf.HTMLModeText, InteractiveForms: true}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	for _, want := range []string{
		`type="number" step="0.01"`,
		`name="price"`,
		`value="1234.50"`,
		`type="date"`,
		`value="2026-12-24"`,
		`<form action="https://example.com/submit?x=1" method="post">`,
		`type="submit"`,
		`formaction="https://example.com/submit?x=1" formmethod="post"`,
		`>Send it</button>`,
		`type="reset"`,
		`>Clear</button>`,
		"</form>\n</body>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	// The unconvertible mask falls back to a text input keeping the value.
	if !regexp.MustCompile(`type="text" name="badmask"[^>]*value="Dec 24, 2026"`).MatchString(s) {
		t.Error("bad-mask date did not fall back to a text input")
	}
	// The actionless push button stays static — no <button> for it.
	if strings.Contains(s, ">No action</button>") {
		t.Error("actionless push button was converted")
	}
}

// TestWriteHTMLFlowMode: HTMLModeFlow re-assembles the document as
// reflowable HTML — headings inferred from font size, paragraphs in reading
// order, small print keeping its relative size, images flowing between
// paragraphs — with none of the fixed-layout machinery (no page divs, no
// absolute spans, no raster backgrounds).
func TestWriteHTMLFlowMode(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	blue := pdf.Color{R: 0, G: 0, B: 0.8, A: 1}
	mustNoErr(t, p.AddText("Document Title", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24, Color: &blue},
		pdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 790}))
	body := "This is the running body text of the article. It has enough words to be " +
		"recognised as an ordinary paragraph rather than a heading of any level."
	mustNoErr(t, p.AddText(body, pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 720}))
	mustNoErr(t, p.AddImage(filepath.Join("testdata", "aspose-logo.png"),
		pdf.Rectangle{LLX: 150, LLY: 420, URX: 350, URY: 560}))
	mustNoErr(t, p.AddText("Fine print at the very bottom of the page, set well below body size.",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 8},
		pdf.Rectangle{LLX: 50, LLY: 60, URX: 545, URY: 90}))

	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{Mode: pdf.HTMLModeFlow}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !regexp.MustCompile(`<h1[^>]*>Document Title</h1>`).MatchString(s) {
		t.Error("title did not become an <h1>")
	}
	if !regexp.MustCompile(`<p[^>]*class="f-serif"[^>]*>This is the running body`).MatchString(s) {
		t.Error("body paragraph missing or lost its serif family")
	}
	if !regexp.MustCompile(`<p[^>]*font-size:0\.6\dem[^>]*>Fine print`).MatchString(s) {
		t.Error("fine print lost its relative size")
	}
	if !strings.Contains(s, `<img src="data:image/png;base64,`) {
		t.Error("image missing from the flow")
	}
	// The image sits between the body paragraph and the fine print.
	iBody, iImg, iFine := strings.Index(s, "running body"), strings.Index(s, "<img"), strings.Index(s, "Fine print")
	if !(iBody < iImg && iImg < iFine) {
		t.Errorf("flow order wrong: body@%d img@%d fine@%d", iBody, iImg, iFine)
	}
	// None of the fixed-layout machinery leaks into flow mode.
	for _, forbid := range []string{`<div class="page"`, `position: absolute`, `class="tl"`, `class="tv"`, `alt="page`} {
		if strings.Contains(s, forbid) {
			t.Errorf("flow output contains fixed-layout artifact %q", forbid)
		}
	}
	if !strings.Contains(s, "color:#0000cc") {
		t.Error("title colour lost")
	}
}

// TestWriteHTMLResourceWriter: with a ResourceWriter the heavy parts (page
// background, WOFF font) leave the HTML — the callback receives them and
// its URLs are referenced instead of data: URLs.
func TestWriteHTMLResourceWriter(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont(filepath.Join("testdata", "DejaVuSans.ttf"))
	mustNoErr(t, err)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("Внешние ресурсы", pdf.TextStyle{Font: font, Size: 16},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}))
	if _, err := doc.SubsetFonts(); err != nil {
		t.Fatal(err)
	}

	got := map[string][]byte{}
	var buf bytes.Buffer
	err = doc.WriteHTML(&buf, pdf.HTMLSaveOptions{
		DPI:  72,
		Mode: pdf.HTMLModeText,
		ResourceWriter: func(name string, data []byte) (string, error) {
			got[name] = append([]byte(nil), data...)
			return "res/" + name, nil
		},
	})
	mustNoErr(t, err)
	s := buf.String()

	if strings.Contains(s, "data:image/png") || strings.Contains(s, "data:font/woff") {
		t.Error("data: URLs remain despite ResourceWriter")
	}
	if !strings.Contains(s, `src="res/page1.png"`) {
		t.Error("background not referenced through the returned URL")
	}
	if !strings.Contains(s, `src:url('res/font_`) {
		t.Error("@font-face not referenced through the returned URL")
	}
	if len(got["page1.png"]) == 0 {
		t.Error("background bytes not delivered to the writer")
	}
	woffOK := false
	for name, data := range got {
		if strings.HasPrefix(name, "font_") && strings.HasSuffix(name, ".woff") &&
			len(data) > 4 && string(data[:4]) == "wOFF" {
			woffOK = true
		}
	}
	if !woffOK {
		t.Error("no WOFF delivered to the writer")
	}
}

// TestSaveHTMLResourceDirAndSplit: ResourceDir writes resources as files
// next to the HTML with relative URLs; SplitPages writes one HTML per page
// with cross-page GoTo links pointing at sibling files.
func TestSaveHTMLResourceDirAndSplit(t *testing.T) {
	doc := makeHTMLDoc(t)
	p, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
	link.SetAction(pdf.NewGoToAction(2, 700))
	mustNoErr(t, p.Annotations().Add(link))

	dir := filepath.Join("result_files", "TestSaveHTMLSplit")
	mustNoErr(t, os.MkdirAll(dir, 0o755))
	out := filepath.Join(dir, "doc.html")
	err := doc.SaveHTML(out, pdf.HTMLSaveOptions{
		DPI: 72, Mode: pdf.HTMLModeText, ResourceDir: "assets", SplitPages: true,
	})
	mustNoErr(t, err)

	for _, n := range []int{1, 2} {
		data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("doc_p%d.html", n)))
		mustNoErr(t, err)
		s := string(data)
		if !strings.Contains(s, fmt.Sprintf(`id="page%d"`, n)) {
			t.Errorf("doc_p%d.html lost its page anchor", n)
		}
		if !strings.Contains(s, fmt.Sprintf(`src="assets/page%d.png"`, n)) {
			t.Errorf("doc_p%d.html does not reference its external background", n)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "page1.png")); err != nil {
		t.Errorf("external background missing on disk: %v", err)
	}
	p1, _ := os.ReadFile(filepath.Join(dir, "doc_p1.html"))
	if !strings.Contains(string(p1), `href="doc_p2.html#page2"`) {
		t.Error("cross-page GoTo link not rewritten to the sibling file")
	}

	// SplitPages on a stream writer must refuse.
	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{SplitPages: true}); err == nil {
		t.Error("WriteHTML accepted SplitPages")
	}
}

// TestWriteHTMLOutlineNav: OutlineNav renders the bookmark tree as a
// no-JS collapsible sidebar with links to page anchors; documents without
// outlines get no sidebar; flow mode ignores the option.
func TestWriteHTMLOutlineNav(t *testing.T) {
	doc := makeHTMLDoc(t)
	root := doc.Outlines()
	p1, _ := doc.Page(1)
	p2, _ := doc.Page(2)

	ch1 := pdf.NewOutlineItemCollection(doc)
	ch1.SetTitle("Chapter <One>")
	ch1.SetDestination(pdf.NewDestinationFit(p1))
	mustNoErr(t, root.Add(ch1))
	sub := pdf.NewOutlineItemCollection(doc)
	sub.SetTitle("Section 1.1")
	sub.SetDestination(pdf.NewDestinationFit(p2))
	mustNoErr(t, ch1.Add(sub))

	var buf bytes.Buffer
	mustNoErr(t, doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, OutlineNav: true}))
	s := buf.String()

	for _, want := range []string{
		`<input type="checkbox" id="nvt">`, // pure-CSS open/close toggle
		`<label class="nvbtn" for="nvt"`,
		`<nav class="nv">`,
		`<div class="pgs">`, // pages wrapper shifts when the panel is open
		`<details open><summary><a href="#page1">Chapter &lt;One&gt;</a></summary>`,
		`<a href="#page2">Section 1.1</a>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// No outlines → no sidebar even with the option on.
	buf.Reset()
	mustNoErr(t, makeHTMLDoc(t).WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, OutlineNav: true}))
	if strings.Contains(buf.String(), `class="nv"`) {
		t.Error("sidebar emitted for a document without outlines")
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
