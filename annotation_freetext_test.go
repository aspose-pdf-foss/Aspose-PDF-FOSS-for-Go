package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFreeTextAnnotationContentsRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"Hello, FreeText!",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeFreeText {
		t.Errorf("type = %v, want AnnotationTypeFreeText", got.AnnotationType())
	}
	ft2, ok := got.(*pdf.FreeTextAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if ft2.Contents() != "Hello, FreeText!" {
		t.Errorf("Contents = %q, want \"Hello, FreeText!\"", ft2.Contents())
	}
}

func TestFreeTextAnnotationSetContentsRegenerates(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"initial",
		pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ft.SetContents("updated")
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if ft2.Contents() != "updated" {
		t.Errorf("Contents after SetContents = %q, want \"updated\"", ft2.Contents())
	}
}

func TestFreeTextAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	pdf.NewFreeTextAnnotation(nil, pdf.Rectangle{}, "", pdf.TextStyle{})
}
