// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestRenderNonEmbeddedType0WithToUnicode is the regression guard for
// pdf-go-5ws: a non-embedded Type0 / CIDFontType2 font (Identity-H, a Latin
// BaseFont, an explicit /CIDToGIDMap and a /ToUnicode CMap, with no
// /FontFile) — the shape WinZip/Acrobat visible signature appearances use —
// must render via a metric-compatible substitute (code=CID → ToUnicode rune →
// substitute glyph), not draw blank.
func TestRenderNonEmbeddedType0WithToUnicode(t *testing.T) {
	toUni := "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n" +
		"1 beginbfchar\n<0001> <0041>\nendbfchar\nendcmap\nend\nend"
	cidToGID := []byte{0x00, 0x00, 0x00, 0x05} // CID 0→GID 0, CID 1→GID 5 (irrelevant for a substitute)
	content := "BT /F0 48 Tf 20 25 Td <0001> Tj ET"

	var b bytes.Buffer
	obj := func(n int, body string) { fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body) }
	streamObj := func(n int, data []byte) {
		fmt.Fprintf(&b, "%d 0 obj\n<< /Length %d >>\nstream\n", n, len(data))
		b.Write(data)
		b.WriteString("\nendstream\nendobj\n")
	}
	b.WriteString("%PDF-1.7\n")
	obj(1, "<</Type/Catalog/Pages 2 0 R>>")
	obj(2, "<</Type/Pages/Kids[3 0 R]/Count 1>>")
	obj(3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 100]"+
		"/Resources<</Font<</F0 4 0 R>>>>/Contents 5 0 R>>")
	obj(4, "<</Type/Font/Subtype/Type0/BaseFont/Arial/Encoding/Identity-H"+
		"/DescendantFonts[6 0 R]/ToUnicode 7 0 R>>")
	obj(6, "<</Type/Font/Subtype/CIDFontType2/BaseFont/Arial"+
		"/CIDSystemInfo<</Registry(Adobe)/Ordering(Identity)/Supplement 0>>"+
		"/FontDescriptor 8 0 R/CIDToGIDMap 9 0 R/DW 1000/W[1[556]]>>")
	obj(8, "<</Type/FontDescriptor/FontName/Arial/Flags 32"+
		"/FontBBox[0 0 1000 1000]/ItalicAngle 0/Ascent 905/Descent -212"+
		"/CapHeight 716/StemV 80>>")
	streamObj(9, cidToGID)
	streamObj(7, []byte(toUni))
	streamObj(5, []byte(content))
	b.WriteString("trailer\n<</Root 1 0 R/Size 10>>\n%%EOF\n")

	doc, err := pdf.OpenStream(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	nonWhite := 0
	bnds := img.Bounds()
	for y := bnds.Min.Y; y < bnds.Max.Y; y++ {
		for x := bnds.Min.X; x < bnds.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || bl < 0xf000 {
				nonWhite++
			}
		}
	}
	if nonWhite == 0 {
		t.Error("non-embedded Type0 (Arial / Identity-H / explicit CIDToGIDMap / ToUnicode) rendered nothing")
	}
}
