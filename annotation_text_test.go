// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestTextIconConstants(t *testing.T) {
	all := []pdf.TextIcon{
		pdf.TextIconUnknown,
		pdf.TextIconComment,
		pdf.TextIconKey,
		pdf.TextIconNote,
		pdf.TextIconHelp,
		pdf.TextIconNewParagraph,
		pdf.TextIconParagraph,
		pdf.TextIconInsert,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("TextIcon[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestFreeTextIntentConstants(t *testing.T) {
	if pdf.FreeTextIntentFreeText != 0 {
		t.Errorf("FreeTextIntentFreeText = %d, want 0", pdf.FreeTextIntentFreeText)
	}
	all := []pdf.FreeTextIntent{
		pdf.FreeTextIntentFreeText,
		pdf.FreeTextIntentCallout,
		pdf.FreeTextIntentTypewriter,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("FreeTextIntent[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestBorderEffectConstants(t *testing.T) {
	if pdf.BorderEffectNone != 0 {
		t.Errorf("BorderEffectNone = %d, want 0", pdf.BorderEffectNone)
	}
	if pdf.BorderEffectCloudy != 1 {
		t.Errorf("BorderEffectCloudy = %d, want 1", pdf.BorderEffectCloudy)
	}
}

func TestStampNameConstants(t *testing.T) {
	all := []pdf.StampName{
		pdf.StampNameUnknown,
		pdf.StampNameApproved,
		pdf.StampNameAsIs,
		pdf.StampNameConfidential,
		pdf.StampNameDepartmental,
		pdf.StampNameDraft,
		pdf.StampNameExperimental,
		pdf.StampNameExpired,
		pdf.StampNameFinal,
		pdf.StampNameForComment,
		pdf.StampNameForPublicRelease,
		pdf.StampNameNotApproved,
		pdf.StampNameNotForPublicRelease,
		pdf.StampNameSold,
		pdf.StampNameTopSecret,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("StampName[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestTextAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 100, Y: 700})
	ta.SetIcon(pdf.TextIconComment)
	ta.SetOpen(true)
	ta.SetTitle("Reviewer")
	ta.SetContents("Important note")
	if err := page.Annotations().Add(ta); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeText {
		t.Errorf("type = %v, want AnnotationTypeText", got.AnnotationType())
	}
	ta2, ok := got.(*pdf.TextAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if ta2.Icon() != pdf.TextIconComment {
		t.Errorf("Icon = %v, want TextIconComment", ta2.Icon())
	}
	if !ta2.Open() {
		t.Errorf("Open = false, want true")
	}
	if ta2.Title() != "Reviewer" {
		t.Errorf("Title = %q", ta2.Title())
	}
	if ta2.Contents() != "Important note" {
		t.Errorf("Contents = %q", ta2.Contents())
	}
}

func TestTextAnnotationAllIcons(t *testing.T) {
	icons := []struct {
		icon pdf.TextIcon
		name string
	}{
		{pdf.TextIconComment, "Comment"},
		{pdf.TextIconKey, "Key"},
		{pdf.TextIconNote, "Note"},
		{pdf.TextIconHelp, "Help"},
		{pdf.TextIconNewParagraph, "NewParagraph"},
		{pdf.TextIconParagraph, "Paragraph"},
		{pdf.TextIconInsert, "Insert"},
	}
	for _, tc := range icons {
		t.Run(tc.name, func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
			ta.SetIcon(tc.icon)
			page.Annotations().Add(ta)
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			ta2 := doc2.Pages()[0].Annotations().At(0).(*pdf.TextAnnotation)
			if got := ta2.Icon(); got != tc.icon {
				t.Errorf("icon = %v, want %v", got, tc.icon)
			}
		})
	}
}

func TestTextAnnotationDefaultIcon(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
	if got := ta.Icon(); got != pdf.TextIconNote {
		t.Errorf("default Icon = %v, want TextIconNote", got)
	}
	if ta.Open() {
		t.Errorf("default Open = true, want false")
	}
}

func TestTextAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	pdf.NewTextAnnotation(nil, pdf.Point{X: 0, Y: 0})
}

func TestTextAnnotationDefaultRect(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ta := pdf.NewTextAnnotation(page, pdf.Point{X: 100, Y: 700})
	r := ta.Rect()
	if r.LLX != 100 || r.LLY != 700 || r.URX != 124 || r.URY != 724 {
		t.Errorf("Rect = %+v, want LLX=100 LLY=700 URX=124 URY=724", r)
	}
}
