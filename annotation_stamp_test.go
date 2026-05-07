package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestStampAnnotationConstructorBasic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 750}, pdf.StampNameApproved)
	if sa == nil {
		t.Fatal("NewStampAnnotation returned nil")
	}
	if sa.Name() != pdf.StampNameApproved {
		t.Errorf("Name = %v, want StampNameApproved", sa.Name())
	}
}

func TestStampAnnotationRoundTripSetName(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 750}, pdf.StampNameDraft)
	sa.SetName(pdf.StampNameConfidential)
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeStamp {
		t.Errorf("type = %v", got.AnnotationType())
	}
	sa2, ok := got.(*pdf.StampAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if sa2.Name() != pdf.StampNameConfidential {
		t.Errorf("Name = %v, want Confidential", sa2.Name())
	}
}

func TestStampAnnotationRawNameEscape(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}, pdf.StampNameDraft)
	sa.SetRawName("/MyCompanyStamp")
	page.Annotations().Add(sa)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
	if sa2.Name() != pdf.StampNameUnknown {
		t.Errorf("Name = %v, want Unknown for non-spec name", sa2.Name())
	}
	if sa2.RawName() != "/MyCompanyStamp" {
		t.Errorf("RawName = %q, want /MyCompanyStamp", sa2.RawName())
	}
}

func TestStampAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	pdf.NewStampAnnotation(nil, pdf.Rectangle{}, pdf.StampNameDraft)
}
