package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestRedactAnnotationBasicRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
	quads := []pdf.QuadPoint{
		{X1: 50, Y1: 650, X2: 300, Y2: 650, X3: 50, Y3: 600, X4: 300, Y4: 600},
	}
	ra.SetQuadPoints(quads)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeRedact {
		t.Errorf("type = %v, want AnnotationTypeRedact", got.AnnotationType())
	}
	ra2, ok := got.(*pdf.RedactAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	qp := ra2.QuadPoints()
	if len(qp) != 1 {
		t.Fatalf("QuadPoints len = %d, want 1", len(qp))
	}
	if qp[0].X1 != 50 || qp[0].Y4 != 600 {
		t.Errorf("QuadPoint = %+v", qp[0])
	}
}

func TestRedactAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	pdf.NewRedactAnnotation(nil, pdf.Rectangle{})
}

func TestRedactAnnotationDefaultQuadPointsEmpty(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	qp := ra.QuadPoints()
	if len(qp) != 0 {
		t.Errorf("default QuadPoints = %v, want empty", qp)
	}
}
