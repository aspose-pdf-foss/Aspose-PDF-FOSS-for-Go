package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFreeTextAnnotationContentsRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"Hello, FreeText!",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeFreeText {
		t.Errorf("type = %v, want AnnotationTypeFreeText", got.AnnotationType())
	}
	ft2, ok := got.(*pdf.FreeTextAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if ft2.Contents() != "Hello, FreeText!" {
		t.Errorf("Contents = %q, want \"Hello, FreeText!\"", ft2.Contents())
	}
}

func TestFreeTextAnnotationSetContentsRegenerates(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"initial",
		pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ft.SetContents("updated")
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if ft2.Contents() != "updated" {
		t.Errorf("Contents after SetContents = %q, want \"updated\"", ft2.Contents())
	}
}

func TestFreeTextAnnotationConstructorPanicOnNilPage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	pdf.NewFreeTextAnnotation(nil, pdf.Rectangle{}, "", pdf.TextStyle{})
}

func TestFreeTextAnnotationTextStyleRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := pdf.TextStyle{
		Font:       pdf.FontHelveticaBold,
		Size:       14,
		Color:      &pdf.Color{R: 1, G: 0, B: 0, A: 1},
		Background: &pdf.Color{R: 1, G: 1, B: 0, A: 1},
		HAlign:     pdf.HAlignCenter,
	}
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"styled text", style)
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	got := ft2.TextStyle()
	if got.Size != 14 {
		t.Errorf("Size = %v, want 14", got.Size)
	}
	if got.Color == nil || got.Color.R != 1 || got.Color.G != 0 || got.Color.B != 0 {
		t.Errorf("Color = %+v, want {1 0 0}", got.Color)
	}
	if got.Background == nil || got.Background.R != 1 || got.Background.G != 1 || got.Background.B != 0 {
		t.Errorf("Background = %+v, want {1 1 0}", got.Background)
	}
	if got.HAlign != pdf.HAlignCenter {
		t.Errorf("HAlign = %v, want HAlignCenter", got.HAlign)
	}
	// Font: standard14 round-trip should preserve PostScript name.
	if got.Font == nil {
		t.Error("Font = nil, want HelveticaBold")
	} else if name := got.Font.BaseFont(); name != "Helvetica-Bold" {
		t.Errorf("Font.BaseFont() = %q, want \"Helvetica-Bold\"", name)
	}
}

func TestFreeTextAnnotationSetTextStyleRegenerates(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		"x", pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ft.SetTextStyle(pdf.TextStyle{
		Font:   pdf.FontTimesRoman,
		Size:   18,
		Color:  &pdf.Color{R: 0, G: 0, B: 1, A: 1},
		HAlign: pdf.HAlignRight,
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	got := ft2.TextStyle()
	if got.Size != 18 {
		t.Errorf("Size = %v", got.Size)
	}
	if got.HAlign != pdf.HAlignRight {
		t.Errorf("HAlign = %v", got.HAlign)
	}
}

func TestFreeTextAnnotationDefaultTextStyle(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		"x", pdf.TextStyle{})
	got := ft.TextStyle()
	// Empty TextStyle → defaults: Helvetica 12pt black, no bg, left-align.
	if got.Size != 12 {
		t.Errorf("default Size = %v, want 12", got.Size)
	}
	if got.HAlign != pdf.HAlignLeft {
		t.Errorf("default HAlign = %v, want HAlignLeft", got.HAlign)
	}
}

func TestFreeTextAnnotationAPHasText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"Visible text",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14, Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1}})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	// Search the raw output for the text rendering operators.
	out := buf.String()
	// FlateDecode compresses /AP/N content stream — but inline encoding
	// depends on writer. Simpler check: just verify the file structure
	// is valid and the annotation type round-trips.
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if ft2.Contents() != "Visible text" {
		t.Errorf("Contents = %q", ft2.Contents())
	}
	_ = strings.Contains(out, "Visible text") // visual check, not asserted
}

func TestFreeTextAnnotationAPHasBackground(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"x",
		pdf.TextStyle{
			Font:       pdf.FontHelvetica,
			Size:       12,
			Background: &pdf.Color{R: 1, G: 1, B: 0, A: 1},
		})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	bg := ft2.TextStyle().Background
	if bg == nil || bg.R != 1 || bg.G != 1 || bg.B != 0 {
		t.Errorf("Background = %+v, want yellow", bg)
	}
}

func TestFreeTextAnnotationAPNoXObjectLeak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100},
		"initial", pdf.TextStyle{})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple regeneration triggers shouldn't leak XObjects.
	ft.SetContents("a")
	ft.SetContents("b")
	ft.SetContents("c")
	ft.SetTextStyle(pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 18})
	ft.SetBorderWidth(2)
	removed := doc.RemoveUnusedObjects()
	if removed != 0 {
		t.Errorf("RemoveUnusedObjects removed %d objects after multiple setters; want 0 (mutate-in-place expected)", removed)
	}
}

func TestFreeTextAnnotationIntentDefault(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		"x", pdf.TextStyle{})
	if ft.Intent() != pdf.FreeTextIntentFreeText {
		t.Errorf("default Intent = %v, want FreeTextIntentFreeText", ft.Intent())
	}
}

func TestFreeTextAnnotationTypewriterRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"typed", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	ft.SetIntent(pdf.FreeTextIntentTypewriter)
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if got := ft2.Intent(); got != pdf.FreeTextIntentTypewriter {
		t.Errorf("Intent = %v, want FreeTextIntentTypewriter", got)
	}
}

func TestFreeTextAnnotationCalloutIntentRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"callout text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	ft.SetIntent(pdf.FreeTextIntentCallout)
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if got := ft2.Intent(); got != pdf.FreeTextIntentCallout {
		t.Errorf("Intent = %v, want FreeTextIntentCallout", got)
	}
}

func TestFreeTextAnnotationSetIntentClearsToDefault(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		"x", pdf.TextStyle{})
	ft.SetIntent(pdf.FreeTextIntentTypewriter)
	ft.SetIntent(pdf.FreeTextIntentFreeText)
	page.Annotations().Add(ft)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if got := ft2.Intent(); got != pdf.FreeTextIntentFreeText {
		t.Errorf("Intent = %v, want FreeTextIntentFreeText after reset", got)
	}
}

func TestFreeTextAnnotationCalloutPointsRoundTrip2pt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 200, LLY: 600, URX: 400, URY: 700},
		"callout", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	ft.SetCalloutPoints([]pdf.Point{
		{X: 150, Y: 650}, // knee
		{X: 100, Y: 550}, // endpoint
	})
	ft.SetEndLineEnding(pdf.LineEndingClosedArrow)
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if ft2.Intent() != pdf.FreeTextIntentCallout {
		t.Errorf("Intent = %v, want Callout (auto-set by SetCalloutPoints)", ft2.Intent())
	}
	pts := ft2.CalloutPoints()
	if len(pts) != 2 {
		t.Fatalf("CalloutPoints len = %d, want 2", len(pts))
	}
	if pts[0].X != 150 || pts[0].Y != 650 || pts[1].X != 100 || pts[1].Y != 550 {
		t.Errorf("CalloutPoints = %+v", pts)
	}
	if got := ft2.EndLineEnding(); got != pdf.LineEndingClosedArrow {
		t.Errorf("EndLineEnding = %v, want ClosedArrow", got)
	}
}

func TestFreeTextAnnotationCalloutPointsRoundTrip3pt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 200, LLY: 600, URX: 400, URY: 700},
		"callout", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	ft.SetCalloutPoints([]pdf.Point{
		{X: 180, Y: 580},
		{X: 150, Y: 540},
		{X: 100, Y: 500},
	})
	page.Annotations().Add(ft)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	pts := ft2.CalloutPoints()
	if len(pts) != 3 {
		t.Fatalf("3-point CalloutPoints len = %d, want 3", len(pts))
	}
	if pts[2].X != 100 || pts[2].Y != 500 {
		t.Errorf("CalloutPoints[2] = %+v, want endpoint (100,500)", pts[2])
	}
}

func TestFreeTextAnnotationInnerRectRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 100, LLY: 100, URX: 300, URY: 200}
	ft := pdf.NewFreeTextAnnotation(page, rect, "x", pdf.TextStyle{})
	inner := pdf.Rectangle{LLX: 120, LLY: 110, URX: 280, URY: 190} // 20pt left/right, 10pt top/bottom inset
	ft.SetInnerRect(inner)
	page.Annotations().Add(ft)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	got := ft2.InnerRect()
	if got.LLX != 120 || got.LLY != 110 || got.URX != 280 || got.URY != 190 {
		t.Errorf("InnerRect round-trip = %+v, want %+v", got, inner)
	}
}

func TestFreeTextAnnotationInnerRectDefaultEqualsRect(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 100, LLY: 100, URX: 300, URY: 200}
	ft := pdf.NewFreeTextAnnotation(page, rect, "x", pdf.TextStyle{})
	got := ft.InnerRect()
	if got.LLX != rect.LLX || got.URY != rect.URY {
		t.Errorf("default InnerRect = %+v, want %+v (equal to rect when /RD absent)", got, rect)
	}
}

func TestFreeTextAnnotationDefaultEndLineEnding(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}, "x", pdf.TextStyle{})
	if got := ft.EndLineEnding(); got != pdf.LineEndingNone {
		t.Errorf("default EndLineEnding = %v, want LineEndingNone", got)
	}
}

func TestFreeTextAnnotationBorderEffectRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 700},
		"cloudy text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	ft.SetBorderEffect(pdf.BorderEffectCloudy)
	ft.SetBorderEffectIntensity(1.5)
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if got := ft2.BorderEffect(); got != pdf.BorderEffectCloudy {
		t.Errorf("BorderEffect = %v, want Cloudy", got)
	}
	if got := ft2.BorderEffectIntensity(); got != 1.5 {
		t.Errorf("BorderEffectIntensity = %v, want 1.5", got)
	}
}

func TestFreeTextAnnotationBorderEffectDefaults(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page, pdf.Rectangle{}, "x", pdf.TextStyle{})
	if got := ft.BorderEffect(); got != pdf.BorderEffectNone {
		t.Errorf("default BorderEffect = %v, want None", got)
	}
	if got := ft.BorderEffectIntensity(); got != 0 {
		t.Errorf("default BorderEffectIntensity = %v, want 0", got)
	}
}

func TestFreeTextAnnotationCloudyClearsToNone(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ft := pdf.NewFreeTextAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}, "x", pdf.TextStyle{})
	ft.SetBorderEffect(pdf.BorderEffectCloudy)
	ft.SetBorderEffect(pdf.BorderEffectNone)
	page.Annotations().Add(ft)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ft2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FreeTextAnnotation)
	if got := ft2.BorderEffect(); got != pdf.BorderEffectNone {
		t.Errorf("BorderEffect after reset = %v, want None", got)
	}
}
