// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// A Form XObject whose /Type, /Subtype, /FormType, /BBox, /Matrix and
// /Resources are indirect references (as Acrobat writes them), or arrays
// holding one, is read as a
// form: its text is extracted and found where its matrix puts it, and the
// page lists it (pdf-go-lcuo).
func TestIndirectFormXObjectKeys(t *testing.T) {
	objs := pageWith("", "", "<< /XObject << /Fm0 5 0 R >> >>", "q /Fm0 Do Q")
	objs = append(objs,
		"<< /Type 6 0 R /Subtype 7 0 R /FormType 8 0 R /BBox 9 0 R /Matrix [1 0 0 1 10 0 R 0] /Resources 11 0 R /Length 0 >> stream\n"+
			"BT /F1 12 Tf 10 100 Td (Inside the form) Tj ET\nendstream",
		"/XObject",
		"/Form",
		"1",
		"[0 0 200 13 0 R]", // the last element is itself a reference
		"20",               // the matrix's x offset
		"<< /Font << /F1 12 0 R >> >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	)
	objs = append(objs, "200")
	d := openHardening(t, hardeningPDF(objs...))
	p := mustPage(t, d)

	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if txt != "Inside the form" {
		t.Fatalf("text %q", txt)
	}
	ms, err := p.SearchText("Inside")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || math.Abs(ms[0].Rect.LLX-30) > 0.01 {
		t.Fatalf("match %+v, want it at x 30 (form matrix applied)", ms)
	}
	forms := p.Forms()
	if len(forms) != 1 {
		t.Fatalf("%d forms", len(forms))
	}
	if w, h := forms[0].Size(); w != 200 || h != 200 {
		t.Errorf("form size %vx%v", w, h)
	}
}

// Word gaps typed as TJ adjustments: a gap of 0.2 em, or a hair under it,
// is a space; kerning is not. "1 Tf" with the size in the text matrix
// (Acrobat, Word) used to make the space an em wide, so no TJ gap under
// 0.3 em started a fragment (pdf-go-q04p).
func TestTJWordGapsAreSpaces(t *testing.T) {
	for _, c := range []struct {
		name, content, want string
	}{
		{"size in Tf", "BT /F1 12 Tf 1 0 0 1 10 100 Tm [(word) -200 (gap) -198 (narrow) -100 (kern)] TJ ET", "word gap narrowkern"},
		{"size in Tm", "BT /F1 1 Tf 12 0 0 12 10 100 Tm [(word) -200 (gap) -198 (narrow) -100 (kern)] TJ ET", "word gap narrowkern"},
		{"wide space glyph", "BT /F2 1 Tf 12 0 0 12 10 100 Tm [(word) -250 (gap)] TJ ET", "word gap"},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := pageWith("", "", "<< /Font << /F1 5 0 R /F2 6 0 R >> >>", c.content)
			objs = append(objs,
				"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
				// code 32 is not a space in this subset: 1100/1000 em wide
				"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 32 /LastChar 122 /Widths [1100"+
					strings.Repeat(" 500", 90)+"] >>",
			)
			p := mustPage(t, openHardening(t, hardeningPDF(objs...)))
			txt, err := p.ExtractText()
			if err != nil {
				t.Fatal(err)
			}
			if txt != c.want {
				t.Errorf("text %q, want %q", txt, c.want)
			}
		})
	}
}
