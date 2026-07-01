// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestExtractTextRawVsReading: raw mode preserves content-stream emission order,
// while reading mode sorts top-to-bottom. The page emits "SECOND" (low) before
// "FIRST" (high), so the two modes order them oppositely.
func TestExtractTextRawVsReading(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	st := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 20}
	mustNoErr(t, p.AddText("SECOND", st, pdf.Rectangle{LLX: 50, LLY: 400, URX: 500, URY: 430}))
	mustNoErr(t, p.AddText("FIRST", st, pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 730}))

	reading, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := p.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractRaw})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Index(reading, "FIRST") > strings.Index(reading, "SECOND") {
		t.Errorf("reading order should put FIRST before SECOND: %q", reading)
	}
	if strings.Index(raw, "SECOND") > strings.Index(raw, "FIRST") {
		t.Errorf("raw order should preserve emission (SECOND before FIRST): %q", raw)
	}

	// The default (no options) equals explicit reading mode.
	def, _ := p.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractReading})
	if def != reading {
		t.Errorf("default extract %q != reading mode %q", reading, def)
	}

	// Document-level variant threads the option through.
	pages, err := doc.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractRaw})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || strings.Index(pages[0], "SECOND") > strings.Index(pages[0], "FIRST") {
		t.Errorf("document raw extract wrong: %v", pages)
	}
}
