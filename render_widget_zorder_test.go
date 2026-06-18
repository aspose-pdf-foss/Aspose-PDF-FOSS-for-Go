// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestRenderWidgetDrawnOverLaterSquare is the regression guard for pdf-go-7i9 /
// pdf-go-3cl: a form-field widget must render on top of a non-widget markup
// annotation (here an opaque white /Square) that appears AFTER it in /Annots,
// matching Acrobat's separate top layer for form fields. 39103.pdf white-outs
// baked content with /Square boxes layered over the placeholder fields; drawing
// strictly in /Annots order erased every field's value.
func TestRenderWidgetDrawnOverLaterSquare(t *testing.T) {
	// Widget /AP/N: a black-filled box. Square /AP/N: a white-filled box over
	// the same rect. The square is later in /Annots, so a naive renderer paints
	// white over the widget; the two-pass renderer must keep the widget black.
	black := "0 0 0 rg 0 0 100 20 re f"
	white := "1 1 1 rg 0 0 100 20 re f"

	var b bytes.Buffer
	obj := func(n int, body string) { fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body) }
	streamObj := func(n, bw, bh int, data string) {
		fmt.Fprintf(&b, "%d 0 obj\n<< /Type/XObject /Subtype/Form /FormType 1 /BBox [0 0 %d %d] /Length %d >>\nstream\n%s\nendstream\nendobj\n", n, bw, bh, len(data), data)
	}
	b.WriteString("%PDF-1.7\n")
	obj(1, "<</Type/Catalog/Pages 2 0 R>>")
	obj(2, "<</Type/Pages/Kids[3 0 R]/Count 1>>")
	obj(3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 100]/Annots[4 0 R 5 0 R]>>")
	// Widget (index 0 in /Annots), with a black appearance.
	obj(4, "<</Type/Annot/Subtype/Widget/FT/Tx/Rect[50 40 150 60]/F 4/AP<</N 6 0 R>>>>")
	// Square (index 1 in /Annots) — later, opaque white over the same rect.
	obj(5, "<</Type/Annot/Subtype/Square/Rect[50 40 150 60]/F 4/AP<</N 7 0 R>>>>")
	streamObj(6, 100, 20, black)
	streamObj(7, 100, 20, white)
	b.WriteString("trailer\n<</Root 1 0 R/Size 8>>\n%%EOF\n")

	doc, err := pdf.OpenStream(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Center of the widget rect (100,50 in PDF user space → device, 72 DPI so
	// 1pt = 1px, origin top-left): x=100, y=100-50=50.
	r, g, bl, _ := img.At(100, 50).RGBA()
	if r > 0x4000 || g > 0x4000 || bl > 0x4000 {
		t.Errorf("widget pixel is not black (got r=%d g=%d b=%d) — a later /Square covered the widget", r>>8, g>>8, bl>>8)
	}
}
