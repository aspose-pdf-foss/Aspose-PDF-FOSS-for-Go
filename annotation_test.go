package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestPageAnnotationsWalkExistingPDF(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)
	ac := page.Annotations()
	if ac.Count() == 0 {
		t.Fatal("expected non-zero annotations on PdfWithAcroForm.pdf (form widgets)")
	}
	// Every annotation here is a form widget — verify type detection.
	for i, a := range ac.All() {
		if a.AnnotationType() != pdf.AnnotationTypeWidget {
			t.Errorf("annotation[%d]: type = %v, want AnnotationTypeWidget (form widget)", i, a.AnnotationType())
		}
		if _, ok := a.(*pdf.WidgetAnnotation); !ok {
			t.Errorf("annotation[%d]: concrete type = %T, want *pdf.WidgetAnnotation", i, a)
		}
		// Wired-accessor smoke check: every form widget has a /Rect.
		if r := a.Rect(); r.LLX == 0 && r.LLY == 0 && r.URX == 0 && r.URY == 0 {
			t.Errorf("annotation[%d]: Rect = empty, expected non-zero on form widget", i)
		}
	}
}

func TestPageAnnotationsNonNilOnPlainDoc(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	ac := page.Annotations()
	if ac == nil {
		t.Fatal("Annotations() returned nil; want non-nil empty collection")
	}
	if got := ac.Count(); got != 0 {
		t.Errorf("Count() = %d on plain doc, want 0", got)
	}
	if got := ac.All(); len(got) != 0 {
		t.Errorf("All() len = %d, want 0", len(got))
	}
}
