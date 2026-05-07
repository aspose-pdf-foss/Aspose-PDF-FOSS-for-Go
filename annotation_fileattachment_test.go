package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFileAttachmentIconConstants(t *testing.T) {
	all := []pdf.FileAttachmentIcon{
		pdf.FileAttachmentIconUnknown,
		pdf.FileAttachmentIconGraph,
		pdf.FileAttachmentIconPaperclip,
		pdf.FileAttachmentIconPushPin,
		pdf.FileAttachmentIconTag,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("FileAttachmentIcon[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestFileAttachmentAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
	fa.SetIcon(pdf.FileAttachmentIconPushPin)
	fa.SetTitle("Reviewer")
	fa.SetContents("Attached document")
	if err := page.Annotations().Add(fa); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeFileAttachment {
		t.Errorf("type = %v, want AnnotationTypeFileAttachment", got.AnnotationType())
	}
	fa2, ok := got.(*pdf.FileAttachmentAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if fa2.Icon() != pdf.FileAttachmentIconPushPin {
		t.Errorf("Icon = %v, want PushPin", fa2.Icon())
	}
	if fa2.Title() != "Reviewer" {
		t.Errorf("Title = %q", fa2.Title())
	}
	if fa2.Contents() != "Attached document" {
		t.Errorf("Contents = %q", fa2.Contents())
	}
}

func TestFileAttachmentAnnotationAllIcons(t *testing.T) {
	icons := []struct {
		icon pdf.FileAttachmentIcon
		name string
	}{
		{pdf.FileAttachmentIconGraph, "Graph"},
		{pdf.FileAttachmentIconPaperclip, "Paperclip"},
		{pdf.FileAttachmentIconPushPin, "PushPin"},
		{pdf.FileAttachmentIconTag, "Tag"},
	}
	for _, tc := range icons {
		t.Run(tc.name, func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
			fa.SetIcon(tc.icon)
			page.Annotations().Add(fa)
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			fa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FileAttachmentAnnotation)
			if got := fa2.Icon(); got != tc.icon {
				t.Errorf("icon = %v, want %v", got, tc.icon)
			}
		})
	}
}

func TestFileAttachmentAnnotationDefaultIcon(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
	if got := fa.Icon(); got != pdf.FileAttachmentIconPaperclip {
		t.Errorf("default Icon = %v, want Paperclip", got)
	}
	if fa.HasFile() {
		t.Errorf("HasFile = true on fresh annotation")
	}
}

func TestFileAttachmentAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	pdf.NewFileAttachmentAnnotation(nil, pdf.Point{X: 0, Y: 0})
}

func TestFileAttachmentAnnotationDefaultRect(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
	r := fa.Rect()
	if r.LLX != 100 || r.LLY != 700 || r.URX != 124 || r.URY != 724 {
		t.Errorf("Rect = %+v, want LLX=100 LLY=700 URX=124 URY=724", r)
	}
}
