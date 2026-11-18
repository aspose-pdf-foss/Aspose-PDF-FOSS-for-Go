// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildRotatePDF builds a one-page PDF whose /Pages node carries rotateVal
// (raw PDF text, e.g. "90", "-90", "5 0 R") and whose leaf has no /Rotate.
func buildRotatePDF(rotateVal string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	obj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	obj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 200 100] /Rotate "+rotateVal+" >>")
	obj(3, "<< /Type /Page /Parent 2 0 R /Resources << >> /Contents 4 0 R >>")
	obj(4, "<< /Length 21 >>\nstream\n0 g 0 0 100 100 re f\nendstream")
	obj(5, "450")
	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		if off, ok := offsets[i]; ok {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		} else {
			buf.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}

func TestInheritedRotateNormalisation(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want pdf.RotationAngle
	}{
		{"90", pdf.Rotate90},
		{"-90", pdf.Rotate270},
		{"450", pdf.Rotate90},
		{"5 0 R", pdf.Rotate90}, // indirect, 450
		{"45", pdf.Rotate0},
	} {
		doc, err := pdf.OpenStream(bytes.NewReader(buildRotatePDF(tc.val)))
		if err != nil {
			t.Fatalf("%s: %v", tc.val, err)
		}
		p, _ := doc.Page(1)
		if got := p.Rotation(); got != tc.want {
			t.Errorf("/Rotate %s: Rotation = %d, want %d", tc.val, got, tc.want)
		}
	}
}

func TestInheritedRotateRoundTripRenderAndPatch(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildRotatePDF("90")))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	img, err := p.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 100 || b.Dy() != 200 {
		t.Errorf("render = %dx%d, want 100x200 (inherited /Rotate 90)", b.Dx(), b.Dy())
	}
	var out bytes.Buffer
	if _, err := doc.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if rp.Rotation() != pdf.Rotate90 {
		t.Errorf("after round trip Rotation = %d, want 90", rp.Rotation())
	}
	// Library patches still work on top of the inherited value.
	if err := re.Rotate(pdf.Rotate90); err != nil {
		t.Fatal(err)
	}
	if rp.Rotation() != pdf.Rotate180 {
		t.Errorf("Rotate(90) on inherited 90 = %d, want 180", rp.Rotation())
	}
	if err := re.SetRotation(pdf.Rotate0); err != nil {
		t.Fatal(err)
	}
	if rp.Rotation() != pdf.Rotate0 {
		t.Errorf("SetRotation(0) = %d", rp.Rotation())
	}
}

func TestInheritedRotateNUpPlacement(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildRotatePDF("90")))
	if err != nil {
		t.Fatal(err)
	}
	out, err := doc.NUp(pdf.NUpOptions{Rows: 1, Cols: 1, PageSize: pdf.PageFormat{Width: 100, Height: 200}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := out.Page(1)
	img, err := p.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 100 || b.Dy() != 200 {
		t.Fatalf("sheet %dx%d", b.Dx(), b.Dy())
	}
	// Source left half is black; rotated 90 clockwise it is the top half.
	if r, _, _, _ := img.At(50, 20).RGBA(); r != 0 {
		t.Errorf("top should be black, got %d", r)
	}
	if r, _, _, _ := img.At(50, 180).RGBA(); r == 0 {
		t.Errorf("bottom should be white")
	}
}
