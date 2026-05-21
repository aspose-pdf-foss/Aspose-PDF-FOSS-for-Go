package asposepdf_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func makeTestTextFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "fileattach-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestFileAttachmentSetFile(t *testing.T) {
	path := makeTestTextFile(t, "hello attached file")
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
	if fa.HasFile() {
		t.Error("HasFile = true before SetFile")
	}
	if err := fa.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	if !fa.HasFile() {
		t.Error("HasFile = false after SetFile")
	}
	if !strings.HasSuffix(fa.FileName(), ".txt") {
		t.Errorf("FileName = %q, expected .txt suffix", fa.FileName())
	}
	if fa.FileSize() != len("hello attached file") {
		t.Errorf("FileSize = %d, want %d", fa.FileSize(), len("hello attached file"))
	}
	if got := string(fa.FileBytes()); got != "hello attached file" {
		t.Errorf("FileBytes = %q", got)
	}
}

func TestFileAttachmentSetFileRoundTrip(t *testing.T) {
	path := makeTestTextFile(t, "round-trip content")
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
	if err := fa.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	fa.SetFileDescription("Test attachment")
	if err := page.Annotations().Add(fa); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	fa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FileAttachmentAnnotation)
	if !fa2.HasFile() {
		t.Error("HasFile = false after roundtrip")
	}
	if got := string(fa2.FileBytes()); got != "round-trip content" {
		t.Errorf("FileBytes after roundtrip = %q, want \"round-trip content\"", got)
	}
	if got := fa2.FileDescription(); got != "Test attachment" {
		t.Errorf("FileDescription = %q", got)
	}
}

func TestFileAttachmentSetFileFromStream(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
	r := strings.NewReader("stream content")
	if err := fa.SetFileFromStream(r, "data.bin"); err != nil {
		t.Fatalf("SetFileFromStream: %v", err)
	}
	if !fa.HasFile() {
		t.Error("HasFile = false")
	}
	if got := fa.FileName(); got != "data.bin" {
		t.Errorf("FileName = %q, want data.bin", got)
	}
	if got := string(fa.FileBytes()); got != "stream content" {
		t.Errorf("FileBytes = %q", got)
	}
}

func TestFileAttachmentFileBytesDefensiveCopy(t *testing.T) {
	path := makeTestTextFile(t, "original")
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
	if err := fa.SetFile(path); err != nil {
		t.Fatal(err)
	}
	bytes1 := fa.FileBytes()
	bytes1[0] = 'X'
	bytes2 := fa.FileBytes()
	if bytes2[0] == 'X' {
		t.Error("FileBytes returned shared mutable slice — caller mutation visible")
	}
}

func TestFileAttachmentSetFileInvalidPath(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
	if err := fa.SetFile("/nonexistent/path.txt"); err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileAttachmentMIMEDetection(t *testing.T) {
	cases := []struct {
		ext  string
		mime string
	}{
		{".pdf", "application/pdf"},
		{".txt", "text/plain"},
		{".png", "image/png"},
	}
	for _, tc := range cases {
		t.Run(tc.ext, func(t *testing.T) {
			f, err := os.CreateTemp("", "test-*"+tc.ext)
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString("x")
			f.Close()
			defer os.Remove(f.Name())

			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
			if err := fa.SetFile(f.Name()); err != nil {
				t.Fatalf("SetFile: %v", err)
			}
			mt := fa.FileMIMEType()
			if !strings.HasPrefix(mt, tc.mime) {
				t.Errorf("MIME type = %q, want prefix %q", mt, tc.mime)
			}
		})
	}
}

var _ io.Reader = (*strings.Reader)(nil) // silence unused import if needed
