// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// savedHolds reports whether the text survives a save of d, every object of
// the reopened file decoded and searched.
func savedHolds(t *testing.T, d *Document, text string) bool {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	found, _ := documentContainsBytes(openHardening(t, buf.Bytes()), text)
	return found
}

// pdf-go-silc: RemoveAllText leaves no text in the file through a resources
// dictionary that lists what no content draws.
func TestRemoveAllTextListedButNotDrawn(t *testing.T) {
	const secret = "Secret 4711"
	const font = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	textForm := func(res string) string {
		return "<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] " + res +
			" /Length 0 >> stream\nBT /F1 12 Tf 10 10 Td (" + secret + ") Tj ET\nendstream"
	}
	cases := []struct {
		name string
		objs []string
	}{
		{
			// Pages 1 and 2 share resources; only page 1 draws the form.
			"shared resources",
			[]string{
				"<< /Type /Catalog /Pages 2 0 R >>",
				"<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>",
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources 5 0 R >>",
				"<< /Length 0 >> stream\n/Fm0 Do\nendstream",
				"<< /Font << /F1 8 0 R >> /XObject << /Fm0 7 0 R >> >>",
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 9 0 R /Resources 5 0 R >>",
				textForm("/Resources << /Font << /F1 8 0 R >> >>"),
				font,
				"<< /Length 0 >> stream\nBT /F1 12 Tf 10 100 Td (page two) Tj ET\nendstream",
			},
		},
		{
			// The only page lists the form and draws nothing at all.
			"drawn nowhere",
			append(pageWith("", "", "<< /Font << /F1 6 0 R >> /XObject << /Fm0 5 0 R >> >>", ""),
				textForm(""), font),
		},
		{
			// The page draws a form whose own resources list another, which
			// the form does not draw.
			"listed inside a form",
			append(pageWith("", "", "<< /XObject << /Fm0 5 0 R >> >>", "/Fm0 Do"),
				"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /XObject << /Fm1 6 0 R >> >> /Length 0 >> stream\n0 0 m 1 1 l S\nendstream",
				textForm("/Resources << /Font << /F1 7 0 R >> >>"), font),
		},
		{
			// A property list no marked-content sequence names.
			"property list named by nothing",
			append(pageWith("", "", "<< /Font << /F1 6 0 R >> /Properties << /MC0 5 0 R >> >>", "BT /F1 12 Tf 10 10 Td (x) Tj ET"),
				"<< /ActualText ("+secret+") >>", font),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := openHardening(t, hardeningPDF(tc.objs...))
			if !savedHolds(t, d, secret) {
				t.Fatalf("the fixture does not hold %q to begin with", secret)
			}
			d = openHardening(t, hardeningPDF(tc.objs...))
			if err := d.RemoveAllText(); err != nil {
				t.Fatal(err)
			}
			if savedHolds(t, d, secret) {
				t.Errorf("%q is still in the saved file after RemoveAllText", secret)
			}
		})
	}

	// Removing the text of page 1 only leaves page 2 as it was.
	d := openHardening(t, hardeningPDF(cases[0].objs...))
	if err := d.Pages()[0].RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	if text, _ := d.Pages()[1].ExtractText(); !strings.Contains(text, "page two") {
		t.Errorf("page 2 reads %q, want its own text", text)
	}
	if text, _ := d.Pages()[0].ExtractText(); strings.TrimSpace(text) != "" {
		t.Errorf("page 1 still reads %q", text)
	}
}
