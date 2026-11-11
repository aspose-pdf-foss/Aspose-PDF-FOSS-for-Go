// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image/color"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// formTextDoc builds a two-page document whose only content on each page is
// the same Form XObject: a pale blue panel with "hello world" drawn on it.
func formTextDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	if err := doc.AddBlankPage(400, 200); err != nil {
		t.Fatal(err)
	}
	form := doc.CreateForm(400, 200)
	c := form.Canvas()
	panel := pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.6, G: 0.8, B: 1, A: 1}}
	if err := c.DrawRectangle(pdf.Rectangle{LLX: 10, LLY: 120, URX: 390, URY: 190}, panel); err != nil {
		t.Fatal(err)
	}
	if err := c.AddText("hello world", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 20, LLY: 150, URX: 380, URY: 185}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		p, _ := doc.Page(i)
		if err := p.AddForm(form, pdf.Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

// TestReplaceTextInsideFormXObject: text drawn by a Form XObject is replaced,
// not drawn over — the old words must be gone (pdf-go-hxi2).
func TestReplaceTextInsideFormXObject(t *testing.T) {
	doc := formTextDoc(t)
	p1, _ := doc.Page(1)
	n, err := p1.ReplaceText("hello", "Hi")
	if err != nil || n != 1 {
		t.Fatalf("ReplaceText = %d, %v; want 1, nil", n, err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := pdf.OpenStream(&buf)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := re.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(texts[0], "hello") || !strings.Contains(texts[0], "Hi") || !strings.Contains(texts[0], "world") {
		t.Errorf("page 1 text %q: want the replacement and no old word", texts[0])
	}
	// The form is shared: the other page's placement is untouched.
	if !strings.Contains(texts[1], "hello world") {
		t.Errorf("page 2 text %q: want the original text", texts[1])
	}

	// Only text is removed inside the form: the panel under the word stays.
	rp, _ := re.Page(1)
	img, err := rp.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	// "hello" spans x 20-58 and "Hi" only 20-37: between 40 and 56 the old
	// word is gone and the panel must show through.
	found := false
	for x := 40; x < 56 && !found; x++ {
		r, g, b, _ := img.At(x, 200-176).RGBA()
		if isPanel(color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}) {
			found = true
		}
	}
	if !found {
		t.Error("the panel behind the replaced word was removed")
	}
}

func isPanel(c color.RGBA) bool {
	near := func(v uint8, want int) bool { d := int(v) - want; return d > -12 && d < 12 }
	return near(c.R, 153) && near(c.G, 204) && near(c.B, 255)
}
