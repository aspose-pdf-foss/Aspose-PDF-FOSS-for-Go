package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func TestRedactAnnotationInteriorColorRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
	ra.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	page.Annotations().Add(ra)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
	ic := ra2.InteriorColor()
	if ic == nil || ic.R != 1 {
		t.Errorf("InteriorColor = %v, want red", ic)
	}
}

func TestRedactAnnotationOverlayTextRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
	ra.SetOverlayText("REDACTED")
	ra.SetRepeatOverlayText(true)
	page.Annotations().Add(ra)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
	if got := ra2.OverlayText(); got != "REDACTED" {
		t.Errorf("OverlayText = %q", got)
	}
	if !ra2.RepeatOverlayText() {
		t.Error("RepeatOverlayText = false, want true")
	}
}

func TestRedactAnnotationOverlayTextStyleRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
	ra.SetOverlayText("X")
	ra.SetOverlayTextStyle(pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   14,
		Color:  &pdf.Color{R: 1, G: 1, B: 1, A: 1},
		HAlign: pdf.HAlignCenter,
	})
	page.Annotations().Add(ra)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
	style := ra2.OverlayTextStyle()
	if style.Size != 14 {
		t.Errorf("Size = %v, want 14", style.Size)
	}
	if style.HAlign != pdf.HAlignCenter {
		t.Errorf("HAlign = %v, want Center", style.HAlign)
	}
	if style.Color == nil || style.Color.R != 1 {
		t.Errorf("Color = %v", style.Color)
	}
}

func TestRedactAnnotationDefaultInteriorColorIsNil(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{})
	if ic := ra.InteriorColor(); ic != nil {
		t.Errorf("default InteriorColor = %v, want nil", ic)
	}
}

func TestRedactAnnotationNoXObjectLeak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	page.Annotations().Add(ra)
	ra.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	ra.SetOverlayText("A")
	ra.SetOverlayText("B")
	ra.SetOverlayText("C")
	ra.SetRepeatOverlayText(true)
	if removed := doc.RemoveUnusedObjects(); removed != 0 {
		t.Errorf("RemoveUnusedObjects = %d after multiple setters; want 0", removed)
	}
}
