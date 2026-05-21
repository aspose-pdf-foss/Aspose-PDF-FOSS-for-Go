// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestSubepic2FilterByType verifies that Text, FreeText, and Stamp
// annotations can coexist on one page and each round-trips to the
// correct AnnotationType after a save / re-open cycle.
func TestSubepic2FilterByType(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
	if err := page.Annotations().Add(ta); err != nil {
		t.Fatalf("Add TextAnnotation: %v", err)
	}

	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700},
		"free text", pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add FreeTextAnnotation: %v", err)
	}

	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 350, LLY: 600, URX: 500, URY: 700}, pdf.StampNameApproved)
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add StampAnnotation: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page2.Annotations().All() {
		counts[a.AnnotationType()]++
	}
	if counts[pdf.AnnotationTypeText] != 1 {
		t.Errorf("AnnotationTypeText: got %d, want 1", counts[pdf.AnnotationTypeText])
	}
	if counts[pdf.AnnotationTypeFreeText] != 1 {
		t.Errorf("AnnotationTypeFreeText: got %d, want 1", counts[pdf.AnnotationTypeFreeText])
	}
	if counts[pdf.AnnotationTypeStamp] != 1 {
		t.Errorf("AnnotationTypeStamp: got %d, want 1", counts[pdf.AnnotationTypeStamp])
	}
}

// TestSubepic2RegenerateAppearance verifies that RegenerateAppearance()
// is callable without error on all three text-bearing annotation types.
func TestSubepic2RegenerateAppearance(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
	if err := page.Annotations().Add(ta); err != nil {
		t.Fatalf("Add TextAnnotation: %v", err)
	}
	ta.RegenerateAppearance() // no-op but must not panic or error

	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700},
		"x", pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add FreeTextAnnotation: %v", err)
	}
	ft.RegenerateAppearance()

	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 350, LLY: 600, URX: 500, URY: 700}, pdf.StampNameDraft)
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add StampAnnotation: %v", err)
	}
	sa.RegenerateAppearance()
}

// TestSubepic2CoexistsWithSubepic1And3 verifies that Subepic 2
// (Text/FreeText/Stamp) annotations coexist with Subepic 1
// (Link/Highlight) and Subepic 3 (Square) types on the same page,
// all round-tripping to the correct types after save/re-open.
func TestSubepic2CoexistsWithSubepic1And3(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Subepic 1: Link + Highlight.
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 800, URX: 150, URY: 820})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add LinkAnnotation: %v", err)
	}

	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 760, URX: 200, URY: 780})
	if err := page.Annotations().Add(hl); err != nil {
		t.Fatalf("Add HighlightAnnotation: %v", err)
	}

	// Subepic 3: Square.
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 720, URX: 200, URY: 750})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add SquareAnnotation: %v", err)
	}

	// Subepic 2: Text + FreeText + Stamp.
	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 250, Y: 800})
	if err := page.Annotations().Add(ta); err != nil {
		t.Fatalf("Add TextAnnotation: %v", err)
	}

	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 250, LLY: 720, URX: 450, URY: 770},
		"free text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add FreeTextAnnotation: %v", err)
	}

	st := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 250, LLY: 600, URX: 500, URY: 700}, pdf.StampNameApproved)
	if err := page.Annotations().Add(st); err != nil {
		t.Fatalf("Add StampAnnotation: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page2.Annotations().All() {
		counts[a.AnnotationType()]++
	}

	expectations := map[pdf.AnnotationType]int{
		pdf.AnnotationTypeLink:      1,
		pdf.AnnotationTypeHighlight: 1,
		pdf.AnnotationTypeSquare:    1,
		pdf.AnnotationTypeText:      1,
		pdf.AnnotationTypeFreeText:  1,
		pdf.AnnotationTypeStamp:     1,
	}
	for typ, want := range expectations {
		if got := counts[typ]; got != want {
			t.Errorf("AnnotationType %v: got %d, want %d", typ, got, want)
		}
	}
}

// subepic2TestPNG writes a small all-black PNG to a temp file and
// registers cleanup. Used by TestSubepic2RemoveUnusedObjects.
func subepic2TestPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	f, err := os.CreateTemp("", "subepic2-stamp-*.png")
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

// TestSubepic2RemoveUnusedObjects verifies that after SetCustomImage +
// ClearCustomImage, the image XObject becomes orphan and
// RemoveUnusedObjects reclaims it (returns removed count ≥ 1).
func TestSubepic2RemoveUnusedObjects(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	sa := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700}, pdf.StampNameDraft)
	if err := sa.SetCustomImage(subepic2TestPNG(t)); err != nil {
		t.Fatalf("SetCustomImage: %v", err)
	}
	if err := page.Annotations().Add(sa); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Clear the custom image — the image XObject is now unreachable.
	sa.ClearCustomImage()

	removed := doc.RemoveUnusedObjects()
	if removed < 1 {
		t.Errorf("RemoveUnusedObjects removed %d objects, want >= 1 (the orphaned image XObject)", removed)
	}
}
