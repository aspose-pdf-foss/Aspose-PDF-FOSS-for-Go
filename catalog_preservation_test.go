// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestSavePreservesCatalogFields verifies that catalog-level fields other
// than /Pages survive a Save + Reopen roundtrip.
//
// Regression: buildDocumentPDF writes a minimal /Catalog with only /Type
// and /Pages, silently stripping every other field from the original
// catalog. Bookmarks (/Outlines), form fields (/AcroForm), named
// destinations (/Names), custom page labels (/PageLabels), XMP metadata
// (/Metadata), tagged PDF (/StructTreeRoot), etc. all disappear on save.
//
// These testdata files were chosen for the spread of catalog fields they
// exercise — see the initial survey of testdata catalogs.
func TestSavePreservesCatalogFields(t *testing.T) {
	cases := []struct {
		file     string
		mustHave []string
	}{
		{"PdfWithAcroForm.pdf", []string{"/AcroForm", "/Outlines", "/Metadata", "/Lang", "/MarkInfo", "/StructTreeRoot"}},
		{"Binder1.pdf", []string{"/AcroForm", "/Outlines", "/Metadata"}},
		{"PdfWithTable.pdf", []string{"/Names", "/Outlines"}},
		{"PdfWithLinks.pdf", []string{"/AcroForm", "/Metadata", "/Names"}},
		{"marketing.pdf", []string{"/PageLabels", "/Metadata"}},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			doc, err := Open("testdata/" + tc.file)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				t.Fatalf("WriteTo: %v", err)
			}
			reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			for _, key := range tc.mustHave {
				if _, ok := reopened.catalog[key]; !ok {
					t.Errorf("catalog missing %s after Save+Reopen", key)
				}
			}
		})
	}
}
