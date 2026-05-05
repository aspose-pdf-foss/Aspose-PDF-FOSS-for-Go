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

func TestLinkAnnotationSubmitFormAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewSubmitFormAction(
		"https://example.com/submit",
		[]string{"name", "email"},
		pdf.SubmitGetMethod|pdf.SubmitExportFormat,
	))
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
	sf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.SubmitFormAction)
	if !ok {
		t.Fatalf("not a SubmitFormAction")
	}
	if sf.URL() != "https://example.com/submit" {
		t.Errorf("URL = %q", sf.URL())
	}
	got := sf.FieldNames()
	if len(got) != 2 || got[0] != "name" || got[1] != "email" {
		t.Errorf("FieldNames = %v, want [name email]", got)
	}
	if sf.Flags()&pdf.SubmitGetMethod == 0 {
		t.Error("SubmitGetMethod flag not set")
	}
	if sf.Flags()&pdf.SubmitExportFormat == 0 {
		t.Error("SubmitExportFormat flag not set")
	}
}

func TestSubmitFormActionEmptyFieldsAndFlags(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewSubmitFormAction("https://example.com/all", nil, 0))
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
	sf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.SubmitFormAction)
	if !ok {
		t.Fatal("not a SubmitFormAction")
	}
	if sf.URL() != "https://example.com/all" {
		t.Errorf("URL = %q", sf.URL())
	}
	if got := sf.FieldNames(); len(got) != 0 {
		t.Errorf("FieldNames = %v, want empty", got)
	}
	if sf.Flags() != 0 {
		t.Errorf("Flags = %d, want 0", sf.Flags())
	}
}

func TestLinkAnnotationResetFormAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewResetFormAction([]string{"name", "email"}))
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
	rf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.ResetFormAction)
	if !ok {
		t.Fatalf("not a ResetFormAction")
	}
	got := rf.FieldNames()
	if len(got) != 2 || got[0] != "name" || got[1] != "email" {
		t.Errorf("FieldNames = %v, want [name email]", got)
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

func TestPdfWithLinksReadAllActions(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)
	ac := page.Annotations()
	if ac.Count() != 6 {
		t.Fatalf("Count = %d, want 6", ac.Count())
	}
	// Per the fixture survey: indices 0..5 carry GoTo, Launch, URI,
	// JavaScript, Named, SubmitForm respectively. /Launch is unsupported
	// — Action() returns nil for it.
	wantTypes := []pdf.ActionType{
		pdf.ActionTypeGoTo,
		pdf.ActionTypeUnknown, // /Launch is out of scope
		pdf.ActionTypeGoToURI,
		pdf.ActionTypeJavaScript,
		pdf.ActionTypeNamed,
		pdf.ActionTypeSubmitForm,
	}
	for i, a := range ac.All() {
		link, ok := a.(*pdf.LinkAnnotation)
		if !ok {
			t.Errorf("annotation[%d]: type = %T, want *LinkAnnotation", i, a)
			continue
		}
		act := link.Action()
		gotType := pdf.ActionTypeUnknown
		if act != nil {
			gotType = act.ActionType()
		}
		if gotType != wantTypes[i] {
			t.Errorf("annotation[%d]: action type = %v, want %v", i, gotType, wantTypes[i])
		}
	}

	// Spot-check JavaScript: action[3] should be JS with non-empty script.
	// The loop above already confirmed ac.At(3) is *LinkAnnotation with ActionTypeJavaScript.
	link3 := ac.At(3).(*pdf.LinkAnnotation)
	js, ok := link3.Action().(*pdf.JavaScriptAction)
	if !ok {
		t.Fatal("annotation[3] action is not *JavaScriptAction")
	}
	if js.Script() == "" {
		t.Error("JavaScriptAction.Script() returned empty string")
	}
}

func TestResetFormActionAllFields(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewResetFormAction(nil)) // "reset all"
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
	rf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.ResetFormAction)
	if !ok {
		t.Fatalf("not a ResetFormAction")
	}
	if got := rf.FieldNames(); len(got) != 0 {
		t.Errorf("FieldNames = %v, want empty (reset-all semantics)", got)
	}
}

func TestUnderlineAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewUnderlineAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	if err := page.Annotations().Add(a); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeUnderline {
		t.Errorf("type = %v, want AnnotationTypeUnderline", got.AnnotationType())
	}
}

func TestStrikeOutAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewStrikeOutAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	if err := page.Annotations().Add(a); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeStrikeOut {
		t.Errorf("type = %v, want AnnotationTypeStrikeOut", got.AnnotationType())
	}
}

func TestSquigglyAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewSquigglyAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	if err := page.Annotations().Add(a); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeSquiggly {
		t.Errorf("type = %v, want AnnotationTypeSquiggly", got.AnnotationType())
	}
}

func TestHighlightAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	hl.SetTitle("Reviewer")
	hl.SetContents("Important")
	hl.SetQuadPoints([]pdf.QuadPoint{
		{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600},
	})
	if err := page.Annotations().Add(hl); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	got := page2.Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeHighlight {
		t.Errorf("type = %v, want AnnotationTypeHighlight", got.AnnotationType())
	}
	hl2 := got.(*pdf.HighlightAnnotation)
	if hl2.Title() != "Reviewer" {
		t.Errorf("Title = %q", hl2.Title())
	}
	if hl2.Contents() != "Important" {
		t.Errorf("Contents = %q, want \"Important\"", hl2.Contents())
	}
	c := hl2.Color()
	if c == nil {
		t.Errorf("Color = nil, want yellow RGB")
	} else if c.R != 1 || c.G != 1 || c.B != 0 {
		t.Errorf("Color = %+v, want {R:1 G:1 B:0 A:1}", *c)
	}
	qp := hl2.QuadPoints()
	if len(qp) != 1 {
		t.Fatalf("QuadPoints len = %d, want 1", len(qp))
	}
	if qp[0].X1 != 50 || qp[0].Y4 != 600 {
		t.Errorf("QuadPoint mismatch: %+v", qp[0])
	}
}

func TestAnnotationCollectionDeleteAt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	page.Annotations().Add(link)
	if err := page.Annotations().DeleteAt(0); err != nil {
		t.Fatalf("DeleteAt(0): %v", err)
	}
	if page.Annotations().Count() != 0 {
		t.Errorf("Count after DeleteAt = %d, want 0", page.Annotations().Count())
	}
	if err := page.Annotations().DeleteAt(0); err == nil {
		t.Error("DeleteAt on empty collection should return error")
	}
}

func TestAnnotationCollectionDeleteAtMiddle(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	a := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	a.SetContents("a")
	if err := page.Annotations().Add(a); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	b := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	b.SetContents("b")
	if err := page.Annotations().Add(b); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	c := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	c.SetContents("c")
	if err := page.Annotations().Add(c); err != nil {
		t.Fatalf("Add c: %v", err)
	}

	// Remove the middle annotation; "a" and "c" should remain in order.
	if err := page.Annotations().DeleteAt(1); err != nil {
		t.Fatalf("DeleteAt(1): %v", err)
	}
	ac := page.Annotations()
	if ac.Count() != 2 {
		t.Fatalf("Count after middle DeleteAt = %d, want 2", ac.Count())
	}
	if got := ac.At(0).Contents(); got != "a" {
		t.Errorf("At(0).Contents = %q, want \"a\"", got)
	}
	if got := ac.At(1).Contents(); got != "c" {
		t.Errorf("At(1).Contents = %q, want \"c\" (middle removed)", got)
	}
}

func TestAnnotationCollectionIdempotentAdd(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := page.Annotations().Add(link); err != nil {
		t.Errorf("second Add same page should be no-op success, got: %v", err)
	}
	if page.Annotations().Count() != 1 {
		t.Errorf("Count after redundant Add = %d, want 1", page.Annotations().Count())
	}
}

func TestAnnotationCollectionReattachError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	page1, _ := doc.Page(1)
	page2, _ := doc.Page(2)
	link := pdf.NewLinkAnnotation(page1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	if err := page1.Annotations().Add(link); err != nil {
		t.Fatalf("Add to page 1: %v", err)
	}
	if err := page2.Annotations().Add(link); err == nil {
		t.Error("Add to page 2 should error — already attached to page 1")
	}
}

func TestAnnotationsCoexistWithForm(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)

	// Form API still works.
	if doc.Form().HasField("textField") == false {
		t.Fatal("textField missing — Form parsing broke")
	}

	// Annotations() returns existing form widgets as WidgetAnnotation.
	widgetCount := 0
	for _, a := range page.Annotations().All() {
		if a.AnnotationType() == pdf.AnnotationTypeWidget {
			widgetCount++
		}
	}
	if widgetCount == 0 {
		t.Fatal("expected at least one WidgetAnnotation")
	}

	// Add a new LinkAnnotation; ensure the form continues to roundtrip.
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 70})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add link: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if doc2.Form().HasField("textField") == false {
		t.Error("textField missing after annotations + roundtrip")
	}
	// The new link should be there too.
	page2, _ := doc2.Page(1)
	hasLink := false
	for _, a := range page2.Annotations().All() {
		if a.AnnotationType() == pdf.AnnotationTypeLink {
			hasLink = true
			break
		}
	}
	if !hasLink {
		t.Error("LinkAnnotation lost after roundtrip with form widgets")
	}
}
