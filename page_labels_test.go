// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestPageLabelDefaultDecimal verifies that pages without /PageLabels return
// their decimal page numbers ("1", "2", …).
func TestPageLabelDefaultDecimal(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, p := range doc.Pages() {
		want := fmt.Sprintf("%d", p.Number())
		if got := p.Label(); got != want {
			t.Errorf("page %d: Label()=%q, want %q", p.Number(), got, want)
		}
	}
}

// TestSetPageLabels_RomanThenDecimal exercises the typical "front matter +
// body" pattern: first two pages numbered i, ii (lowercase roman), then the
// body numbered 1, 2, 3 (decimal restarting at 1). Verifies the round-trip
// through Save+Open and the Page.Label() reader.
func TestSetPageLabels_RomanThenDecimal(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	for i := 0; i < 4; i++ { // total 5 pages
		if err := doc.AddBlankPageFromFormat(asposepdf.PageFormatA4); err != nil {
			t.Fatalf("AddBlankPage: %v", err)
		}
	}
	err := doc.SetPageLabels([]asposepdf.PageLabelRange{
		{StartPage: 1, Style: asposepdf.PageLabelRomanLower},
		{StartPage: 3, Style: asposepdf.PageLabelDecimal, StartNum: 1},
	})
	if err != nil {
		t.Fatalf("SetPageLabels: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := asposepdf.OpenStream(&buf)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	want := []string{"i", "ii", "1", "2", "3"}
	pages := reopened.Pages()
	if len(pages) != len(want) {
		t.Fatalf("page count = %d, want %d", len(pages), len(want))
	}
	for i, p := range pages {
		if got := p.Label(); got != want[i] {
			t.Errorf("page %d Label() = %q, want %q", i+1, got, want[i])
		}
	}
}

// TestSetPageLabels_PrefixAndCustomStart covers the /P (prefix) and /St
// (start number) entries — labels like "A-5, A-6, A-7".
func TestSetPageLabels_PrefixAndCustomStart(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	for i := 0; i < 2; i++ {
		_ = doc.AddBlankPageFromFormat(asposepdf.PageFormatA4)
	}
	err := doc.SetPageLabels([]asposepdf.PageLabelRange{
		{StartPage: 1, Style: asposepdf.PageLabelDecimal, Prefix: "A-", StartNum: 5},
	})
	if err != nil {
		t.Fatalf("SetPageLabels: %v", err)
	}
	var buf bytes.Buffer
	_, _ = doc.WriteTo(&buf)
	reopened, _ := asposepdf.OpenStream(&buf)
	want := []string{"A-5", "A-6", "A-7"}
	for i, p := range reopened.Pages() {
		if got := p.Label(); got != want[i] {
			t.Errorf("page %d Label() = %q, want %q", i+1, got, want[i])
		}
	}
}

// TestSetPageLabels_Validation rejects ranges that don't start at page 1 or
// aren't strictly ascending. Empty input clears existing labels.
func TestSetPageLabels_Validation(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	if err := doc.SetPageLabels([]asposepdf.PageLabelRange{
		{StartPage: 2, Style: asposepdf.PageLabelDecimal},
	}); err == nil {
		t.Error("expected error when first range starts at page 2, got nil")
	}
	if err := doc.SetPageLabels([]asposepdf.PageLabelRange{
		{StartPage: 1, Style: asposepdf.PageLabelDecimal},
		{StartPage: 1, Style: asposepdf.PageLabelRomanLower}, // not ascending
	}); err == nil {
		t.Error("expected error for non-ascending ranges, got nil")
	}

	// Set then clear via empty slice — Label() falls back to decimal.
	_ = doc.SetPageLabels([]asposepdf.PageLabelRange{
		{StartPage: 1, Style: asposepdf.PageLabelRomanLower},
	})
	if err := doc.SetPageLabels(nil); err != nil {
		t.Fatalf("SetPageLabels(nil): %v", err)
	}
	page, _ := doc.Page(1)
	if got := page.Label(); got != "1" {
		t.Errorf("after clear, Label() = %q, want %q", got, "1")
	}
}
