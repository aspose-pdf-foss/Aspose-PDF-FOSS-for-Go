// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestAddBarcodeFieldCode128(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 740}
	bc, err := form.AddBarcodeField(1, rect, "bc128", pdf.BarcodeCode128, "HELLO123")
	if err != nil {
		t.Fatal(err)
	}
	if bc.Value() != "HELLO123" {
		t.Errorf("Value() = %q, want HELLO123", bc.Value())
	}
	if bc.Symbology() != pdf.BarcodeCode128 {
		t.Errorf("Symbology() = %v, want BarcodeCode128", bc.Symbology())
	}
	if pdf.FieldType(bc) != pdf.FormFieldTypeBarcode {
		t.Errorf("FieldType = %v, want FormFieldTypeBarcode", pdf.FieldType(bc))
	}
}

func TestAddBarcodeFieldQR(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := pdf.Rectangle{LLX: 50, LLY: 600, URX: 150, URY: 700}
	bc, err := form.AddBarcodeField(1, rect, "bcqr", pdf.BarcodeQR, "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if bc.Value() != "https://example.com/" {
		t.Errorf("Value() = %q", bc.Value())
	}
	if bc.Symbology() != pdf.BarcodeQR {
		t.Errorf("Symbology() = %v, want BarcodeQR", bc.Symbology())
	}
}

func TestBarcodeFieldRejectsUnencodableValue(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 740}

	if _, err := form.AddBarcodeField(1, rect, "bad", pdf.BarcodeCode128, "héllo"); err == nil {
		t.Error("expected an error adding a Code128 field with non-ASCII value")
	}
	if form.HasField("bad") {
		t.Error("a rejected AddBarcodeField call must not create the field")
	}

	bc, err := form.AddBarcodeField(1, rect, "bc", pdf.BarcodeCode128, "OK")
	if err != nil {
		t.Fatal(err)
	}
	if err := bc.SetValue("héllo"); err == nil {
		t.Error("expected an error setting a non-ASCII value on a Code128 field")
	}
	if bc.Value() != "OK" {
		t.Errorf("value changed despite rejected SetValue: got %q", bc.Value())
	}
	if err := bc.SetSymbology(pdf.BarcodeQR); err != nil {
		t.Fatalf("SetSymbology(QR): %v", err)
	}
	if bc.Symbology() != pdf.BarcodeQR {
		t.Error("SetSymbology did not change the symbology")
	}
	if err := bc.SetValue("héllo"); err != nil {
		t.Errorf("QR should accept non-ASCII text: %v", err)
	}

	// The reverse direction: switching a QR field holding a non-ASCII value
	// back to Code128 must be rejected too, leaving the symbology unchanged.
	if err := bc.SetSymbology(pdf.BarcodeCode128); err == nil {
		t.Error("expected an error switching to Code128 while holding a non-ASCII value")
	}
	if bc.Symbology() != pdf.BarcodeQR {
		t.Error("symbology changed despite a rejected SetSymbology call")
	}
}

func TestBarcodeFieldEmptyValueError(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 740}
	if _, err := form.AddBarcodeField(1, rect, "empty", pdf.BarcodeQR, ""); err == nil {
		t.Error("expected an error for an empty barcode value")
	}
}

func TestBarcodeFieldRoundTrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	if _, err := form.AddBarcodeField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 740}, "bc128", pdf.BarcodeCode128, "ABC123"); err != nil {
		t.Fatal(err)
	}
	if _, err := form.AddBarcodeField(1, pdf.Rectangle{LLX: 50, LLY: 550, URX: 200, URY: 690}, "bcqr", pdf.BarcodeQR, "https://example.com/x"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	outForm := out.Form()

	bc128, ok := outForm.Field("bc128").(*pdf.BarcodeField)
	if !ok {
		t.Fatalf("bc128 is %T, want *BarcodeField", outForm.Field("bc128"))
	}
	if bc128.Symbology() != pdf.BarcodeCode128 || bc128.Value() != "ABC123" {
		t.Errorf("bc128: symbology=%v value=%q", bc128.Symbology(), bc128.Value())
	}

	bcqr, ok := outForm.Field("bcqr").(*pdf.BarcodeField)
	if !ok {
		t.Fatalf("bcqr is %T, want *BarcodeField", outForm.Field("bcqr"))
	}
	if bcqr.Symbology() != pdf.BarcodeQR || bcqr.Value() != "https://example.com/x" {
		t.Errorf("bcqr: symbology=%v value=%q", bcqr.Symbology(), bcqr.Value())
	}
}

// TestBarcodeFieldInFormData: barcode fields export/import through the
// generic text-family JSON path (asTextField), with SetValue's symbology
// validation still enforced on import.
func TestBarcodeFieldInFormData(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	if _, err := form.AddBarcodeField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 740}, "bc", pdf.BarcodeCode128, "HELLO"); err != nil {
		t.Fatal(err)
	}

	data, err := form.ExportJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"barcode"`)) {
		t.Errorf("exported JSON missing barcode type tag: %s", data)
	}
	if !bytes.Contains(data, []byte("HELLO")) {
		t.Errorf("exported JSON missing value: %s", data)
	}

	replaced := bytes.ReplaceAll(data, []byte("HELLO"), []byte("WORLD"))
	n, err := form.ImportJSON(replaced)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("ImportJSON applied %d fields, want 1", n)
	}
	if v := form.Field("bc").Value(); v != "WORLD" {
		t.Errorf("value after import = %q, want WORLD", v)
	}

	// A value the field's symbology cannot encode is rejected by
	// BarcodeField.SetValue and, like any other per-field import error,
	// silently skipped by ImportJSON (0 applied, no error, field unchanged).
	bad := bytes.ReplaceAll(data, []byte("HELLO"), []byte("héllo"))
	applied, err := form.ImportJSON(bad)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Errorf("ImportJSON applied %d fields for an unencodable value, want 0", applied)
	}
	if v := form.Field("bc").Value(); v != "WORLD" {
		t.Errorf("value changed despite an unencodable import value: got %q", v)
	}
}

// TestBarcodeFieldRenders: the widget's /AP actually paints something (a
// full end-to-end sanity check through the shared PDF rasterizer, not just
// the appearance-stream code that built it).
func TestBarcodeFieldRenders(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 780}
	if _, err := form.AddBarcodeField(1, rect, "bc", pdf.BarcodeQR, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 100})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	dark := false
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y && !dark; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0x4000 && g < 0x4000 && bl < 0x4000 {
				dark = true
				break
			}
		}
	}
	if !dark {
		t.Error("rendered page has no dark pixels — barcode did not paint")
	}
}
