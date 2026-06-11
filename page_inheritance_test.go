// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildPDFWithInheritedPageAttrs returns bytes of a minimal valid PDF in which
// /MediaBox, /CropBox, and /Rotate are declared ONLY on the /Pages parent node,
// not on the /Page leaf. Per ISO 32000-1 §7.7.3.4 Table 30 these attributes
// are inheritable — such PDFs are fully spec-compliant and Adobe/Poppler/Chrome
// render them correctly.
func buildPDFWithInheritedPageAttrs() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	offsets := map[int]int{}
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R]"+
		" /MediaBox [0 0 595 842]"+
		" /CropBox [10 10 585 832]"+
		" /Rotate 90"+
		" >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << >> >>")

	xrefOff := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 4\n0000000000 65535 f \n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes()
}

// TestInheritedMediaBox verifies that when /MediaBox lives on a /Pages parent,
// the leaf /Page still reports correct dimensions after Open. Regression: Open
// strips /Pages from d.objects, and walkPageTree copies only /Resources down
// to leaves — so mediaBoxSize fails to walk /Parent because the parent is gone.
func TestInheritedMediaBox(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithInheritedPageAttrs()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("Size = %+v, want {Width:595 Height:842}", size)
	}
}

// TestInheritedCropBox verifies /CropBox inheritance on Page.CropBox() and
// related box helpers (TrimBox/BleedBox/ArtBox all fall back to CropBox then
// MediaBox per spec).
func TestInheritedCropBox(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithInheritedPageAttrs()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	crop, err := page.CropBox()
	if err != nil {
		t.Fatalf("CropBox: %v", err)
	}
	// CropBox is [10 10 585 832] (575 x 822).
	if crop.LLX != 10 || crop.LLY != 10 || crop.URX != 585 || crop.URY != 832 {
		t.Errorf("CropBox = %+v, want {LLX:10 LLY:10 URX:585 URY:832}", crop)
	}
}

// TestInheritedRotation verifies /Rotate inheritance on Page.Rotation().
// Regression: Page.Rotation() only reads /Rotate off the page dict directly
// and does not walk /Parent.
func TestInheritedRotation(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithInheritedPageAttrs()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	if got := page.Rotation(); got != pdf.Rotate90 {
		t.Errorf("Rotation = %d, want %d (Rotate90)", got, pdf.Rotate90)
	}
}

// TestAddTextWatermarkInheritedMediaBox verifies the public AddTextWatermark
// API against PDFs with inherited /MediaBox. This is the user-visible failure
// mode — watermarking a standards-compliant PDF errors out.
func TestAddTextWatermarkInheritedMediaBox(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithInheritedPageAttrs()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if err := doc.AddTextWatermark("WM", pdf.TextStyle{Size: 20}); err != nil {
		t.Errorf("AddTextWatermark: %v", err)
	}
}

// buildPDFWithoutMediaBox returns bytes of a minimal PDF in which neither the
// /Page leaf nor the /Pages root declares a /MediaBox. ISO 32000-1 requires
// one, but XFA form shells in the wild ship exactly this (the page is an empty
// placeholder; content lives in the XFA stream). Acrobat and MuPDF default
// such pages to US Letter (612x792).
func buildPDFWithoutMediaBox() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	offsets := map[int]int{}
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R >>")

	xrefOff := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 4\n0000000000 65535 f \n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes()
}

// TestMissingMediaBoxDefaultsToLetter verifies that a page tree with no
// /MediaBox anywhere falls back to US Letter instead of erroring (47647.pdf,
// an ACORD XFA form, failed with "render: object 3 not found").
func TestMissingMediaBoxDefaultsToLetter(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithoutMediaBox()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	mb, err := page.MediaBox()
	if err != nil {
		t.Fatalf("MediaBox: %v", err)
	}
	if mb.LLX != 0 || mb.LLY != 0 || mb.URX != 612 || mb.URY != 792 {
		t.Errorf("MediaBox = %+v, want US Letter {0 0 612 792}", mb)
	}
	if _, err := page.RenderImage(pdf.RenderOptions{DPI: 72}); err != nil {
		t.Errorf("RenderImage: %v", err)
	}
}
