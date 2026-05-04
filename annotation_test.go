package asposepdf_test

import (
	"bytes"
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

func TestAnnotationCollectionAddLinkRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetTitle("reviewer")
	link.SetContents("note")
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
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
	ac2 := page2.Annotations()
	if ac2.Count() != 1 {
		t.Fatalf("Count after roundtrip = %d, want 1", ac2.Count())
	}
	got := ac2.At(0)
	if got.AnnotationType() != pdf.AnnotationTypeLink {
		t.Errorf("type = %v, want AnnotationTypeLink", got.AnnotationType())
	}
	if _, ok := got.(*pdf.LinkAnnotation); !ok {
		t.Errorf("concrete type = %T, want *pdf.LinkAnnotation", got)
	}
	if got.Title() != "reviewer" {
		t.Errorf("Title = %q, want \"reviewer\"", got.Title())
	}
	if got.Contents() != "note" {
		t.Errorf("Contents = %q, want \"note\"", got.Contents())
	}
	r := got.Rect()
	if r.LLX != 50 || r.LLY != 700 || r.URX != 200 || r.URY != 720 {
		t.Errorf("Rect = %+v, want {50 700 200 720}", r)
	}
}

func TestLinkAnnotationGoToURIAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://example.com/path"))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
	act := link2.Action()
	if act == nil {
		t.Fatal("Action() = nil after roundtrip")
	}
	if act.ActionType() != pdf.ActionTypeGoToURI {
		t.Errorf("ActionType = %v, want ActionTypeGoToURI", act.ActionType())
	}
	uri, ok := act.(*pdf.GoToURIAction)
	if !ok {
		t.Fatalf("concrete type = %T, want *pdf.GoToURIAction", act)
	}
	if uri.URI() != "https://example.com/path" {
		t.Errorf("URI = %q, want %q", uri.URI(), "https://example.com/path")
	}
}

func TestLinkAnnotationGoToAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	page1, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToAction(2, 800))
	if err := page1.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
	act := link2.Action()
	if act == nil {
		t.Fatal("Action() = nil")
	}
	gt, ok := act.(*pdf.GoToAction)
	if !ok {
		t.Fatalf("concrete = %T, want *pdf.GoToAction", act)
	}
	if gt.PageNum() != 2 {
		t.Errorf("PageNum = %d, want 2", gt.PageNum())
	}
	if gt.Top() != 800 {
		t.Errorf("Top = %f, want 800", gt.Top())
	}
}

func TestGoToActionPdfRefEncodePath(t *testing.T) {
	// Build a 2-page doc with one GoTo link (int-fallback encode path).
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	page1, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToAction(2, 800))
	if err := page1.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf1 bytes.Buffer
	if _, err := doc.WriteTo(&buf1); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	// Reopen — Action() post-process binds doc onto the parsed GoToAction.
	doc2, err := pdf.OpenStream(bytes.NewReader(buf1.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2P1, _ := doc2.Page(1)
	link2 := page2P1.Annotations().At(0).(*pdf.LinkAnnotation)
	act := link2.Action().(*pdf.GoToAction)

	// Reuse the parsed action on a NEW link — encode() now writes /D[0]
	// as a pdfRef because act.doc is set. This exercises the spec-correct
	// pdfRef branch.
	newLink := pdf.NewLinkAnnotation(page2P1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
	newLink.SetAction(act)
	if err := page2P1.Annotations().Add(newLink); err != nil {
		t.Fatalf("second Add: %v", err)
	}
	var buf2 bytes.Buffer
	if _, err := doc2.WriteTo(&buf2); err != nil {
		t.Fatalf("second WriteTo: %v", err)
	}

	// Reopen and verify both links resolve to PageNum=2.
	doc3, err := pdf.OpenStream(bytes.NewReader(buf2.Bytes()))
	if err != nil {
		t.Fatalf("third OpenStream: %v", err)
	}
	page3P1, _ := doc3.Page(1)
	ac := page3P1.Annotations()
	if ac.Count() != 2 {
		t.Fatalf("Count after second roundtrip = %d, want 2", ac.Count())
	}
	for i := 0; i < ac.Count(); i++ {
		l := ac.At(i).(*pdf.LinkAnnotation)
		gt, ok := l.Action().(*pdf.GoToAction)
		if !ok {
			t.Errorf("link[%d]: action = %T, want *pdf.GoToAction", i, l.Action())
			continue
		}
		if gt.PageNum() != 2 {
			t.Errorf("link[%d]: PageNum = %d, want 2", i, gt.PageNum())
		}
		if gt.Top() != 800 {
			t.Errorf("link[%d]: Top = %f, want 800", i, gt.Top())
		}
	}
}

func TestLinkAnnotationReadFromExistingPDF(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)
	ac := page.Annotations()
	if ac.Count() == 0 {
		t.Fatal("expected non-zero annotations on PdfWithLinks.pdf")
	}
	// Confirm at least one link's Action() resolves through an indirect /A.
	gotAnyAction := false
	for _, a := range ac.All() {
		link, ok := a.(*pdf.LinkAnnotation)
		if !ok {
			continue
		}
		if link.Action() != nil {
			gotAnyAction = true
			break
		}
	}
	if !gotAnyAction {
		t.Fatal("no LinkAnnotation has a non-nil Action() — indirect /A resolution broken")
	}
}

func TestLinkAnnotationNamedAction(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  pdf.NamedActionType
	}{
		{"FirstPage", pdf.NamedActionFirstPage},
		{"LastPage", pdf.NamedActionLastPage},
		{"NextPage", pdf.NamedActionNextPage},
		{"PrevPage", pdf.NamedActionPrevPage},
		{"Print", pdf.NamedActionPrint},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
			link.SetAction(pdf.NewNamedAction(tc.val))
			if err := page.Annotations().Add(link); err != nil {
				t.Fatalf("Add: %v", err)
			}
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("OpenStream: %v", err)
			}
			page2, _ := doc2.Page(1)
			link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
			na, ok := link2.Action().(*pdf.NamedAction)
			if !ok {
				t.Fatalf("type = %T, want *pdf.NamedAction", link2.Action())
			}
			if na.Name() != tc.val {
				t.Errorf("Name = %v, want %v", na.Name(), tc.val)
			}
		})
	}
}
