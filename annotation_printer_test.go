// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

var (
	markTrim  = pdf.Rectangle{LLX: 60, LLY: 60, URX: 340, URY: 340}
	markBleed = pdf.Rectangle{LLX: 50, LLY: 50, URX: 350, URY: 350}
)

// markPage returns a 400x400 page with a TrimBox and a BleedBox, leaving a margin.
func markPage(t *testing.T) (*pdf.Document, *pdf.Page) {
	t.Helper()
	d := pdf.NewDocument(400, 400)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetTrimBox(markTrim); err != nil {
		t.Fatal(err)
	}
	if err := p.SetBleedBox(markBleed); err != nil {
		t.Fatal(err)
	}
	return d, p
}

func reopenMarks(t *testing.T, d *pdf.Document) (*pdf.Document, *pdf.Page) {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	p, err := got.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	return got, p
}

func emptyRect(r pdf.Rectangle) bool { return r.URX <= r.LLX || r.URY <= r.LLY }

func overlap(a, b pdf.Rectangle) float64 {
	w := min(a.URX, b.URX) - max(a.LLX, b.LLX)
	h := min(a.URY, b.URY) - max(a.LLY, b.LLY)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

func TestAddPrinterMarksAll(t *testing.T) {
	d, p := markPage(t)
	if err := p.AddPrinterMarks(pdf.PrinterMarksAll); err != nil {
		t.Fatal(err)
	}
	_, got := reopenMarks(t, d)
	count := map[pdf.AnnotationType]int{}
	for _, a := range got.Annotations().All() {
		count[a.AnnotationType()]++
		if emptyRect(a.Rect()) {
			t.Errorf("%v has an empty rectangle", a.AnnotationType())
		}
		if overlap(a.Rect(), markTrim) > 0.05 {
			t.Errorf("%v overlaps the TrimBox: %+v", a.AnnotationType(), a.Rect())
		}
	}
	for typ, want := range map[pdf.AnnotationType]int{
		pdf.AnnotationTypeTrimMark: 4, pdf.AnnotationTypeBleedMark: 4, pdf.AnnotationTypeRegistrationMark: 4,
		pdf.AnnotationTypeColorBar: 4, pdf.AnnotationTypePageInformation: 1,
	} {
		if count[typ] != want {
			t.Errorf("%v: %d marks, want %d", typ, count[typ], want)
		}
	}
	// None of the marks may collide with another.
	all := got.Annotations().All()
	for i, a := range all {
		for _, b := range all[i+1:] {
			if a.AnnotationType() == pdf.AnnotationTypeTrimMark || a.AnnotationType() == pdf.AnnotationTypeBleedMark ||
				b.AnnotationType() == pdf.AnnotationTypeTrimMark || b.AnnotationType() == pdf.AnnotationTypeBleedMark {
				continue // a trim and a bleed mark share their corner by design
			}
			if overlap(a.Rect(), b.Rect()) > 0 {
				t.Errorf("%v and %v collide: %+v %+v", a.AnnotationType(), b.AnnotationType(), a.Rect(), b.Rect())
			}
		}
	}
}

func TestAddPrinterMarksSelectsKinds(t *testing.T) {
	_, p := markPage(t)
	if err := p.AddPrinterMarks(pdf.PrinterMarksNone); err != nil || p.Annotations().Count() != 0 {
		t.Fatalf("None: %v, %d marks", err, p.Annotations().Count())
	}
	if err := p.AddPrinterMarks(pdf.PrinterMarksRegistrationMarks | pdf.PrinterMarksColorBars); err != nil {
		t.Fatal(err)
	}
	if n := p.Annotations().Count(); n != 8 {
		t.Errorf("%d marks, want 8", n)
	}
}

func TestAddPrinterMarksNoRoomChangesNothing(t *testing.T) {
	d, first := markPage(t)
	if err := d.AddBlankPage(400, 400); err != nil { // TrimBox defaults to the whole page
		t.Fatal(err)
	}
	err := d.AddPrinterMarks(pdf.PrinterMarksTrimMarks)
	if err == nil || !strings.Contains(err.Error(), "page 2") {
		t.Fatalf("error %v", err)
	}
	if n := first.Annotations().Count(); n != 0 {
		t.Errorf("page 1 got %d marks though the call failed", n)
	}
	for _, kind := range []pdf.PrinterMarksKind{pdf.PrinterMarksBleedMarks, pdf.PrinterMarksRegistrationMarks, pdf.PrinterMarksColorBars, pdf.PrinterMarksPageInformation} {
		second, _ := d.Page(2)
		if err := second.AddPrinterMarks(kind); err == nil {
			t.Errorf("kind %d: no error with no room", kind)
		}
	}
}

func TestPrinterMarksNotDisplayedWithoutRoom(t *testing.T) {
	d := pdf.NewDocument(400, 400)
	p, _ := d.Page(1)
	for _, a := range []pdf.Annotation{
		pdf.NewTrimMarkAnnotation(p, pdf.PrinterMarkCornerTopLeft),
		pdf.NewBleedMarkAnnotation(p, pdf.PrinterMarkCornerBottomRight),
		pdf.NewRegistrationMarkAnnotation(p, pdf.PrinterMarkSideLeft),
		pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 20}, pdf.ColorsOfCMYKBlack),
	} {
		if !emptyRect(a.Rect()) {
			t.Errorf("%v: rectangle %+v, want empty", a.AnnotationType(), a.Rect())
		}
		if err := p.Annotations().Add(a); err != nil {
			t.Fatal(err)
		}
	}
	reopenMarks(t, d) // and it is still a valid annotation
}

func TestCornerMarkPositions(t *testing.T) {
	for _, c := range []struct {
		pos  pdf.PrinterMarkCornerPosition
		x, y float64 // the trim corner
	}{
		{pdf.PrinterMarkCornerTopLeft, 60, 340},
		{pdf.PrinterMarkCornerTopRight, 340, 340},
		{pdf.PrinterMarkCornerBottomLeft, 60, 60},
		{pdf.PrinterMarkCornerBottomRight, 340, 60},
	} {
		d, p := markPage(t)
		a := pdf.NewTrimMarkAnnotation(p, c.pos)
		if err := p.Annotations().Add(a); err != nil {
			t.Fatal(err)
		}
		r := a.Rect()
		if r.LLX > c.x || r.URX < c.x || r.LLY > c.y || r.URY < c.y {
			t.Errorf("%v: rectangle %+v does not reach the corner", c.pos, r)
		}
		_, got := reopenMarks(t, d)
		if back := got.Annotations().At(0).(*pdf.TrimMarkAnnotation).Position(); back != c.pos {
			t.Errorf("%v came back as %v", c.pos, back)
		}
	}
}

func TestBleedMarkFollowsBleedBox(t *testing.T) {
	_, p := markPage(t)
	r := pdf.NewBleedMarkAnnotation(p, pdf.PrinterMarkCornerBottomLeft).Rect()
	if r.URX < 50-0.2 || r.URX > 50+0.2 || r.URY < 50-0.2 || r.URY > 50+0.2 {
		t.Errorf("bleed mark ends at (%v, %v), want the BleedBox corner (50, 50)", r.URX, r.URY)
	}
	r = pdf.NewTrimMarkAnnotation(p, pdf.PrinterMarkCornerBottomLeft).Rect()
	if r.URX < 60-0.2 || r.URX > 60+0.2 {
		t.Errorf("trim mark ends at x=%v, want the TrimBox corner 60", r.URX)
	}
}

func TestRegistrationMarkPositions(t *testing.T) {
	for _, pos := range []pdf.PrinterMarkSidePosition{pdf.PrinterMarkSideTop, pdf.PrinterMarkSideBottom, pdf.PrinterMarkSideLeft, pdf.PrinterMarkSideRight} {
		d, p := markPage(t)
		a := pdf.NewRegistrationMarkAnnotation(p, pos)
		if err := p.Annotations().Add(a); err != nil {
			t.Fatal(err)
		}
		r := a.Rect()
		if overlap(r, markBleed) != 0 {
			t.Errorf("%v overlaps the bleed area: %+v", pos, r)
		}
		if pos == pdf.PrinterMarkSideTop && (r.LLY < 350 || r.LLX+r.URX != 400) {
			t.Errorf("top target at %+v", r)
		}
		_, got := reopenMarks(t, d)
		if back := got.Annotations().At(0).(*pdf.RegistrationMarkAnnotation).Position(); back != pos {
			t.Errorf("%v came back as %v", pos, back)
		}
	}
}

func TestSetPositionMovesMark(t *testing.T) {
	_, p := markPage(t)
	a := pdf.NewTrimMarkAnnotation(p, pdf.PrinterMarkCornerTopLeft)
	before := a.Rect()
	a.SetPosition(pdf.PrinterMarkCornerBottomRight)
	if a.Rect() == before || a.Position() != pdf.PrinterMarkCornerBottomRight {
		t.Errorf("rect %+v, position %v", a.Rect(), a.Position())
	}
	reg := pdf.NewRegistrationMarkAnnotation(p, pdf.PrinterMarkSideTop)
	before = reg.Rect()
	reg.SetPosition(pdf.PrinterMarkSideLeft)
	if reg.Rect() == before || reg.Position() != pdf.PrinterMarkSideLeft {
		t.Errorf("rect %+v, position %v", reg.Rect(), reg.Position())
	}
}

func TestColorBarMovesOutsideTrim(t *testing.T) {
	_, p := markPage(t)
	// Wide, and nearer the top: it goes above the TrimBox.
	a := pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 320, URX: 200, URY: 330}, pdf.ColorsOfCMYKCyan)
	if r := a.Rect(); r.LLY != 340 || r.URY != 350 || r.LLX != 100 || r.URX != 200 {
		t.Errorf("wide bar %+v", r)
	}
	// Wide, and nearer the bottom: below it.
	a = pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 70, URX: 200, URY: 80}, pdf.ColorsOfCMYKCyan)
	if r := a.Rect(); r.URY != 60 || r.LLY != 50 {
		t.Errorf("wide bar near the bottom %+v", r)
	}
	// Tall: to the side.
	a = pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 320, LLY: 100, URX: 330, URY: 200}, pdf.ColorsOfCMYKCyan)
	if r := a.Rect(); r.LLX != 340 || r.URX != 350 || r.LLY != 100 {
		t.Errorf("tall bar %+v", r)
	}
	// Taller than the room: reduced.
	a = pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 200, URX: 300, URY: 290}, pdf.ColorsOfCMYKCyan)
	if r := a.Rect(); r.LLY != 340 || r.URY != 400 {
		t.Errorf("reduced bar %+v", r)
	}
	// Already outside: left alone.
	in := pdf.Rectangle{LLX: 100, LLY: 10, URX: 200, URY: 20}
	if r := pdf.NewColorBarAnnotation(p, in, pdf.ColorsOfCMYKCyan).Rect(); r != in {
		t.Errorf("bar outside moved to %+v", r)
	}
}

func TestColorBarFallsBackWhenNearerSideHasNoRoom(t *testing.T) {
	d := pdf.NewDocument(400, 400)
	p, _ := d.Page(1)
	if err := p.SetTrimBox(pdf.Rectangle{LLX: 0, LLY: 60, URX: 400, URY: 400}); err != nil {
		t.Fatal(err)
	}
	// Nearer the top, where nothing is left.
	a := pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 380, URX: 200, URY: 390}, pdf.ColorsOfCMYKCyan)
	if r := a.Rect(); r.URY != 60 || r.LLY != 50 {
		t.Errorf("bar %+v", r)
	}
}

func TestColorBarColorRoundTrip(t *testing.T) {
	for _, ink := range []pdf.ColorsOfCMYK{pdf.ColorsOfCMYKCyan, pdf.ColorsOfCMYKMagenta, pdf.ColorsOfCMYKYellow, pdf.ColorsOfCMYKBlack} {
		d, p := markPage(t)
		a := pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 10, URX: 200, URY: 20}, pdf.ColorsOfCMYKCyan)
		a.SetColorOfCMYK(ink)
		if err := p.Annotations().Add(a); err != nil {
			t.Fatal(err)
		}
		_, got := reopenMarks(t, d)
		if back := got.Annotations().At(0).(*pdf.ColorBarAnnotation).ColorOfCMYK(); back != ink {
			t.Errorf("%v came back as %v", ink, back)
		}
	}
}

func TestPageInformationText(t *testing.T) {
	_, p := markPage(t)
	a := pdf.NewPageInformationAnnotation(p, pdf.Rectangle{LLX: 60, LLY: 356, URX: 200, URY: 364})
	if c := a.Contents(); !strings.HasPrefix(c, "Page 1  ") {
		t.Errorf("contents %q", c)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	d, _ := markPage(t)
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	opened, err := pdf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := opened.Page(1)
	if c := pdf.NewPageInformationAnnotation(page, markBleed).Contents(); !strings.HasPrefix(c, "job.pdf  Page 1") {
		t.Errorf("contents %q", c)
	}
}

func TestPageInformationKeepsItsDate(t *testing.T) {
	d, p := markPage(t)
	a := pdf.NewPageInformationAnnotation(p, pdf.Rectangle{LLX: 60, LLY: 356, URX: 200, URY: 364})
	a.SetContents("Fixed text")
	if err := p.Annotations().Add(a); err != nil {
		t.Fatal(err)
	}
	_, got := reopenMarks(t, d)
	back := got.Annotations().At(0).(*pdf.PageInformationAnnotation)
	if back.Contents() != "Fixed text" {
		t.Errorf("contents %q", back.Contents())
	}
	back.RegenerateAppearance()
	if back.Contents() != "Fixed text" {
		t.Errorf("regenerating changed the text to %q", back.Contents())
	}
}

// inkIn reports whether the box, in user space, has a pixel darker than the page.
func inkIn(img image.Image, dpi float64, r pdf.Rectangle) bool {
	scale := dpi / 72
	h := float64(img.Bounds().Dy())
	for y := int(h - r.URY*scale); y <= int(h-r.LLY*scale); y++ {
		for x := int(r.LLX * scale); x <= int(r.URX*scale); x++ {
			if c := color.GrayModel.Convert(img.At(x, y)).(color.Gray); c.Y < 200 {
				return true
			}
		}
	}
	return false
}

func TestPrinterMarksRender(t *testing.T) {
	const dpi = 288
	d, p := markPage(t)
	if err := p.AddPrinterMarks(pdf.PrinterMarksAll); err != nil {
		t.Fatal(err)
	}
	_, got := reopenMarks(t, d)
	img, err := got.RenderImage(pdf.RenderOptions{DPI: dpi})
	if err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]pdf.Rectangle{
		"trim mark, horizontal line": {LLX: 30, LLY: 59, URX: 42, URY: 61},
		"trim mark, vertical line":   {LLX: 59, LLY: 30, URX: 61, URY: 42},
		"bleed mark, horizontal":     {LLX: 30, LLY: 49, URX: 42, URY: 51},
		"registration, top":          {LLX: 199, LLY: 358, URX: 201, URY: 366},
		"registration, left":         {LLX: 34, LLY: 199, URX: 42, URY: 201},
		"colour bar":                 {LLX: 68, LLY: 38, URX: 75, URY: 42},
		"registration ring":          {LLX: 202.3, LLY: 364.3, URX: 203.3, URY: 365.3},
	} {
		if !inkIn(img, dpi, r) {
			t.Errorf("%s: nothing drawn in %+v", name, r)
		}
	}
	for name, r := range map[string]pdf.Rectangle{
		"inside the page":     {LLX: 62, LLY: 62, URX: 338, URY: 338},
		"between the marks":   {LLX: 43, LLY: 62, URX: 49, URY: 190},
		"gap before the line": {LLX: 45, LLY: 59.8, URX: 49.5, URY: 60.2},
	} {
		if inkIn(img, dpi, r) {
			t.Errorf("%s: unexpected ink in %+v", name, r)
		}
	}
}

func TestColorBarRendersItsTints(t *testing.T) {
	const dpi = 144
	d, p := markPage(t)
	a := pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: 100, LLY: 10, URX: 180, URY: 20}, pdf.ColorsOfCMYKCyan)
	if err := p.Annotations().Add(a); err != nil {
		t.Fatal(err)
	}
	_, got := reopenMarks(t, d)
	img, err := got.RenderImage(pdf.RenderOptions{DPI: dpi})
	if err != nil {
		t.Fatal(err)
	}
	at := func(x, y float64) color.RGBA {
		h := float64(img.Bounds().Dy())
		return color.RGBAModel.Convert(img.At(int(x*dpi/72), int(h-y*dpi/72))).(color.RGBA)
	}
	full, light := at(110, 15), at(170, 15)
	if full.R >= 100 || full.B <= full.R {
		t.Errorf("full tint is not cyan: %+v", full)
	}
	if light.R <= full.R {
		t.Errorf("the last step is not lighter: %+v vs %+v", light, full)
	}
}

func TestPrinterMarksArePrintedAndAllowedInPDFA(t *testing.T) {
	d, p := markPage(t)
	if err := p.AddPrinterMarks(pdf.PrinterMarksTrimMarks | pdf.PrinterMarksRegistrationMarks | pdf.PrinterMarksPageInformation); err != nil {
		t.Fatal(err)
	}
	rep, err := d.ConvertToPDFA(pdf.PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Errorf("not conformant: %+v", rep.Issues)
	}
}

func TestPrinterMarkRowsClearTheTrimLine(t *testing.T) {
	_, p := markPage(t)
	if err := p.AddPrinterMarks(pdf.PrinterMarksColorBars | pdf.PrinterMarksPageInformation); err != nil {
		t.Fatal(err)
	}
	for _, a := range p.Annotations().All() {
		if a.Rect().LLX <= markTrim.LLX {
			t.Errorf("%v starts on the trim line: %+v", a.AnnotationType(), a.Rect())
		}
		if a.Rect().URX >= 194 {
			t.Errorf("%v reaches the registration target: %+v", a.AnnotationType(), a.Rect())
		}
	}
}

func TestPrinterMarksWithoutABleedBox(t *testing.T) {
	d := pdf.NewDocument(400, 400)
	p, _ := d.Page(1)
	if err := p.SetTrimBox(markTrim); err != nil {
		t.Fatal(err)
	}
	if err := p.AddPrinterMarks(pdf.PrinterMarksTrimMarks | pdf.PrinterMarksBleedMarks); err != nil {
		t.Fatalf("a page with no BleedBox has no bleed, and room: %v", err)
	}
	// The line then starts just beyond the trim edge.
	if r := p.Annotations().At(0).Rect(); r.URY != 60+6+18 && r.URY != 340+6+18 {
		t.Errorf("rectangle %+v", r)
	}
}

func TestCornerMarkRunsAwayFromTheBleedArea(t *testing.T) {
	_, p := markPage(t)
	r := pdf.NewTrimMarkAnnotation(p, pdf.PrinterMarkCornerTopLeft).Rect()
	if r.URY != 350+6+18 || r.LLX != 50-6-18 {
		t.Errorf("top left mark %+v, want it to reach (26, 374)", r)
	}
}

func TestColorBarIsKeptOnThePage(t *testing.T) {
	_, p := markPage(t)
	r := pdf.NewColorBarAnnotation(p, pdf.Rectangle{LLX: -30, LLY: 10, URX: 40, URY: 20}, pdf.ColorsOfCMYKCyan).Rect()
	if r.LLX != 0 || r.URX != 40 {
		t.Errorf("bar %+v, want it cut at the page edge", r)
	}
}
