package asposepdf_test

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func TestStampAnnotationAllPredefinedNamesRoundTrip(t *testing.T) {
	names := []pdf.StampName{
		pdf.StampNameApproved, pdf.StampNameAsIs, pdf.StampNameConfidential,
		pdf.StampNameDepartmental, pdf.StampNameDraft, pdf.StampNameExperimental,
		pdf.StampNameExpired, pdf.StampNameFinal, pdf.StampNameForComment,
		pdf.StampNameForPublicRelease, pdf.StampNameNotApproved,
		pdf.StampNameNotForPublicRelease, pdf.StampNameSold, pdf.StampNameTopSecret,
	}
	for _, name := range names {
		t.Run(name.String(), func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			sa := pdf.NewStampAnnotation(page,
				pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750}, name)
			if err := page.Annotations().Add(sa); err != nil {
				t.Fatalf("Add: %v", err)
			}
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
			if got := sa2.Name(); got != name {
				t.Errorf("Name round-trip = %v, want %v", got, name)
			}
		})
	}
}

func makeTestPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range img.Pix {
		if i%4 == 0 {
			img.Pix[i] = 0xFF // R
		} else if i%4 == 3 {
			img.Pix[i] = 0xFF // A
		}
	}
	f, err := os.CreateTemp("", "stamp-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestStampAnnotationCustomImageFromFile(t *testing.T) {
	path := makeTestPNG(t)
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 800}, pdf.StampNameDraft)
	if sa.HasCustomImage() {
		t.Error("HasCustomImage = true before SetCustomImage")
	}
	if err := sa.SetCustomImage(path); err != nil {
		t.Fatalf("SetCustomImage: %v", err)
	}
	if !sa.HasCustomImage() {
		t.Error("HasCustomImage = false after SetCustomImage")
	}
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
	if !sa2.HasCustomImage() {
		t.Error("HasCustomImage = false after roundtrip")
	}
}

func TestStampAnnotationCustomImageFromStream(t *testing.T) {
	path := makeTestPNG(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 800}, pdf.StampNameDraft)
	if err := sa.SetCustomImageFromStream(f); err != nil {
		t.Fatalf("SetCustomImageFromStream: %v", err)
	}
	if !sa.HasCustomImage() {
		t.Error("HasCustomImage = false")
	}
}

func TestStampAnnotationClearCustomImage(t *testing.T) {
	path := makeTestPNG(t)
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.StampNameDraft)
	sa.SetCustomImage(path)
	sa.ClearCustomImage()
	if sa.HasCustomImage() {
		t.Error("HasCustomImage = true after Clear")
	}
}

func TestStampAnnotationCustomImageInvalidFormat(t *testing.T) {
	f, err := os.CreateTemp("", "stamp-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("not an image")
	f.Close()
	defer os.Remove(f.Name())

	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.StampNameDraft)
	if err := sa.SetCustomImage(f.Name()); err == nil {
		t.Error("expected error for non-image file")
	}
}

func TestStampAnnotationAPNoXObjectLeak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}, pdf.StampNameDraft)
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple regenerations from setter calls — must reuse the same
	// font XObject (mutate-in-place semantics), not allocate fresh
	// each time.
	sa.SetName(pdf.StampNameApproved)
	sa.SetName(pdf.StampNameConfidential)
	sa.SetName(pdf.StampNameFinal)
	sa.SetBorderWidth(2)
	if removed := doc.RemoveUnusedObjects(); removed != 0 {
		t.Errorf("RemoveUnusedObjects = %d after multiple setters; want 0 (mutate-in-place expected)", removed)
	}
}
