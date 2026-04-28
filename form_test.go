package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestDocumentFormNonNilOnPlainPDF(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()
	if form == nil {
		t.Fatal("Form() returned nil for plain document; expected non-nil empty form")
	}
	if got := form.Fields(); len(got) != 0 {
		t.Errorf("plain document Form().Fields() = %d entries, want 0", len(got))
	}
	if form.HasField("anything") {
		t.Error("plain document HasField returned true")
	}
	if form.Field("anything") != nil {
		t.Error("plain document Field() returned non-nil")
	}
}
