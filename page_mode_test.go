// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func reopenBytes(t *testing.T, d *pdf.Document) *pdf.Document {
	t.Helper()
	got, err := pdf.OpenStream(bytes.NewReader(savedBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPageModeRoundTrip(t *testing.T) {
	if m := pdf.NewDocument(200, 200).PageMode(); m != pdf.PageModeUseNone {
		t.Errorf("new document PageMode = %v", m)
	}
	for _, m := range []pdf.PageMode{
		pdf.PageModeUseNone, pdf.PageModeUseOutlines, pdf.PageModeUseThumbs,
		pdf.PageModeFullScreen, pdf.PageModeUseOC, pdf.PageModeUseAttachments,
	} {
		d := pdf.NewDocument(200, 200)
		d.SetPageMode(m)
		if got := reopenBytes(t, d).PageMode(); got != m {
			t.Errorf("PageMode %v read back as %v", m, got)
		}
	}
}

func TestPageModeWritesTheName(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	d.SetPageMode(pdf.PageModeUseAttachments)
	if !bytes.Contains(savedBytes(t, d), []byte("/PageMode /UseAttachments")) {
		t.Error("no /PageMode /UseAttachments in the catalog")
	}
	for _, m := range []pdf.PageMode{pdf.PageModeUseNone, pdf.PageMode(42), pdf.PageMode(-1)} {
		d.SetPageMode(pdf.PageModeFullScreen)
		d.SetPageMode(m)
		if bytes.Contains(savedBytes(t, d), []byte("/PageMode")) {
			t.Errorf("SetPageMode(%d) left a /PageMode entry", m)
		}
		if d.PageMode() != pdf.PageModeUseNone {
			t.Errorf("SetPageMode(%d) reads back as %v", m, d.PageMode())
		}
	}
}
