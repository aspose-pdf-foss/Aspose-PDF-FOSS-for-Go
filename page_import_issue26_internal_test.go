// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// letterhead is a one-page document with a line of text in an embedded
// TrueType font, saved and reopened.
func letterhead(t *testing.T) *Document {
	t.Helper()
	d := NewDocument(595, 842)
	f, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if err := mustPage(t, d).AddTextAt("Letterhead Ltd", TextStyle{Font: f, Size: 14}, 50, 800); err != nil {
		t.Fatal(err)
	}
	return reopenDoc(t, d)
}

func countObjects(d *Document, pred func(pdfDict) bool) int {
	n := 0
	for _, o := range d.objects {
		var dict pdfDict
		switch v := o.Value.(type) {
		case pdfDict:
			dict = v
		case *pdfStream:
			dict = v.Dict
		}
		if dict != nil && pred(dict) {
			n++
		}
	}
	return n
}

func hasFontProgram(d pdfDict) bool {
	_, l1 := d["/Length1"]
	return l1
}

func isForm(d pdfDict) bool { return dictGetName(d, "/Subtype") == "/Form" }

// GitHub issue #26 point 1: a PdfPageStamp imports its page once per target
// document, however many pages it is applied to.
func TestPdfPageStampImportsOnce(t *testing.T) {
	src := letterhead(t)
	st, err := NewPdfPageStamp(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.Background = true
	d := NewDocument(595, 842)
	for i := 1; i < 10; i++ {
		if err := d.AddBlankPage(595, 842); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.AddStamp(st); err != nil {
		t.Fatal(err)
	}
	if n := countObjects(d, isForm); n != 1 {
		t.Errorf("%d form XObjects, want 1", n)
	}
	got := reopenDoc(t, d)
	if n := countObjects(got, hasFontProgram); n != 1 {
		t.Errorf("%d font programs, want 1", n)
	}
	for i := 1; i <= 10; i++ {
		p, _ := got.Page(i)
		if txt, _ := p.ExtractText(); !strings.Contains(txt, "Letterhead Ltd") {
			t.Fatalf("page %d text %q", i, txt)
		}
	}
}

// GitHub issue #26 point 1b: ImportPage gives a reusable form of a page,
// sized as the page is shown, the same object when imported again.
func TestImportPage(t *testing.T) {
	src := letterhead(t)
	if err := src.SetRotation(Rotate90, 1); err != nil {
		t.Fatal(err)
	}
	d := NewDocument(842, 595)
	f, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := f.Size(); w != 842 || h != 595 {
		t.Errorf("form size %vx%v, want the rotated page 842x595", w, h)
	}
	again, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if again.formID != f.formID {
		t.Errorf("imported twice: forms %d and %d", f.formID, again.formID)
	}
	for i := 0; i < 3; i++ {
		if i > 0 {
			if err := d.AddBlankPage(842, 595); err != nil {
				t.Fatal(err)
			}
		}
		p, _ := d.Page(i + 1)
		if err := p.AddForm(f, Rectangle{URX: 842, URY: 595}); err != nil {
			t.Fatal(err)
		}
	}
	got := reopenDoc(t, d)
	if n := countObjects(got, isForm); n != 1 {
		t.Errorf("%d forms, want 1", n)
	}
	p3, _ := got.Page(3)
	if txt, _ := p3.ExtractText(); !strings.Contains(txt, "Letterhead Ltd") {
		t.Errorf("page 3 text %q", txt)
	}
	if _, err := d.ImportPage(src, 2); err == nil {
		t.Error("page 2 of a one-page source accepted")
	}
	if _, err := d.ImportPage(nil, 1); err == nil {
		t.Error("nil source accepted")
	}
	// A page of the document itself.
	own, err := d.ImportPage(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := own.Size(); w != 842 || h != 595 {
		t.Errorf("own page form %vx%v", w, h)
	}
}

// GitHub issue #26 point 3: stamping gives the same bytes every time.
func TestImportedPageOutputIsReproducible(t *testing.T) {
	src := letterhead(t)
	build := func() []byte {
		st, err := NewPdfPageStamp(src, 1)
		if err != nil {
			t.Fatal(err)
		}
		d := NewDocument(595, 842)
		if err := d.AddStamp(st); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := d.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	first := build()
	for i := 0; i < 10; i++ {
		if !bytes.Equal(first, build()) {
			t.Fatal("output differs between runs")
		}
	}
}

// GitHub issue #26 point 3, content rewrites: an inline dictionary is
// written the same way every time.
func TestInlineDictionaryWrittenReproducibly(t *testing.T) {
	op := contentOp{Operator: "BDC", Operands: []pdfValue{pdfName("/Span"), pdfDict{
		"/ActualText": "x", "/Lang": "de", "/MCID": 0, "/Alt": "y", "/E": "z",
	}}}
	first := string(serializeContentOps([]contentOp{op}))
	for i := 0; i < 20; i++ {
		if got := string(serializeContentOps([]contentOp{op})); got != first {
			t.Fatalf("%q then %q", first, got)
		}
	}
	if !strings.Contains(first, "/ActualText (x) /Alt (y) /E (z) /Lang (de) /MCID 0") {
		t.Errorf("keys not sorted: %q", first)
	}
}

// GitHub issue #26 point 4: a reference the source cannot resolve is null
// in the import, not the target's object of that number.
func TestImportNullsMissingReference(t *testing.T) {
	objs := pageWith("", "", "<< /Font << /F1 99 0 R >> >>", "BT /F1 12 Tf 10 10 Td (x) Tj ET")
	src := openHardening(t, hardeningPDF(objs...))
	d := NewDocument(595, 842)
	for d.nextID <= 100 {
		d.objects[d.nextID] = &pdfObject{Num: d.nextID, Value: pdfDict{"/Marker": pdfName("/Target")}}
		d.nextID++
	}
	f, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	form := d.objects[f.formID].Value.(*pdfStream)
	res := resolveRef(d.objects, form.Dict["/Resources"]).(pdfDict)
	fonts := resolveRef(d.objects, res["/Font"]).(pdfDict)
	if _, ok := fonts["/F1"].(pdfNull); !ok {
		t.Errorf("/F1 imported as %#v, want null", fonts["/F1"])
	}
}

// GitHub issue #26 point 5: the page's transparency group goes with it.
func TestImportPageKeepsGroup(t *testing.T) {
	objs := pageWith("", "/Group << /S /Transparency /CS /DeviceRGB /I true >>", "<< >>", "0 0 1 rg 0 0 10 10 re f")
	src := openHardening(t, hardeningPDF(objs...))
	d := NewDocument(595, 842)
	f, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	form := d.objects[f.formID].Value.(*pdfStream)
	g, ok := resolveRef(d.objects, form.Dict["/Group"]).(pdfDict)
	if !ok || dictGetName(g, "/S") != "/Transparency" || g["/I"] != true {
		t.Errorf("form /Group %#v", form.Dict["/Group"])
	}
}

// GitHub issue #26 point 6: a form given its own resources by ConvertToPDFA
// keeps the default colour space its caller gave it.
func TestPDFAFormResourcesKeepDefaultColorSpace(t *testing.T) {
	objs := pageWith("", "",
		"<< /Font << /F1 5 0 R >> /XObject << /X 6 0 R >> /ColorSpace << /DefaultRGB [/CalRGB << /WhitePoint [0.9505 1 1.089] >>] >> >>",
		"/X Do")
	objs = append(objs,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Length 0 >> stream\n1 0 0 rg 0 0 50 50 re f BT /F1 12 Tf 10 100 Td (x) Tj ET\nendstream",
	)
	d := openHardening(t, hardeningPDF(objs...))
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	form := d.objects[6].Value.(*pdfStream)
	res, _ := resolveRefToDict(d.objects, form.Dict["/Resources"])
	cs, _ := resolveRefToDict(d.objects, res["/ColorSpace"])
	if cs["/DefaultRGB"] == nil {
		t.Errorf("form resources %#v lack /DefaultRGB", res)
	}
	for _, iss := range d.ValidatePDFA(PDFA2B).Issues {
		if iss.Rule == "INHERITED_RESOURCES" {
			t.Errorf("still reported: %s", iss.Message)
		}
	}
}

// A form inheriting only the caller's default colour space is not
// reported: it names nothing.
func TestPDFADefaultColorSpaceAloneNotReported(t *testing.T) {
	objs := pageWith("", "",
		"<< /XObject << /X 5 0 R >> /ColorSpace << /DefaultRGB [/CalRGB << /WhitePoint [0.9505 1 1.089] >>] >> >>",
		"/X Do")
	objs = append(objs, "<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Length 0 >> stream\n1 0 0 rg 0 0 50 50 re f\nendstream")
	d := openHardening(t, hardeningPDF(objs...))
	for _, iss := range d.ValidatePDFA(PDFA2B).Issues {
		if iss.Rule == "INHERITED_RESOURCES" {
			t.Errorf("reported: %s", iss.Message)
		}
	}
}

// GitHub issue #26 point 7: placing the same form again reuses its name, and
// many placements and Behind draws stay linear.
func TestFormPlacementReusesName(t *testing.T) {
	d := NewDocument(595, 842)
	f := d.CreateForm(10, 10)
	if err := f.Canvas().DrawRectangle(Rectangle{0, 0, 5, 5}, ShapeStyle{FillColor: &Color{A: 1}}); err != nil {
		t.Fatal(err)
	}
	p := mustPage(t, d)
	for i := 0; i < 50; i++ {
		if err := p.AddForm(f, Rectangle{0, 0, 10, 10}); err != nil {
			t.Fatal(err)
		}
	}
	xo, _ := resolveRefToDict(d.objects, p.pageResources()["/XObject"])
	if len(xo) != 1 {
		t.Errorf("%d /XObject names for one form", len(xo))
	}
}

func TestFormPlacementAndBehindStayFast(t *testing.T) {
	const small, large = 2000, 8000
	assertLinearGrowth(t, "GitHub issue #26 AddForm", small, large, func(n int) {
		d := NewDocument(595, 842)
		f := d.CreateForm(10, 10)
		p, _ := d.Page(1)
		for i := 0; i < n; i++ {
			if err := p.AddForm(f, Rectangle{0, 0, 10, 10}); err != nil {
				t.Fatal(err)
			}
		}
	})
	assertLinearGrowth(t, "GitHub issue #26 Behind", small, large, func(n int) {
		d := NewDocument(595, 842)
		p, _ := d.Page(1)
		for i := 0; i < n; i++ {
			if err := p.prependToContentStream([]byte("q Q\n")); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// Behind draws stack in front of one another's backs: the last one drawn is
// first in /Contents, the page's own content last.
func TestPrependOrder(t *testing.T) {
	d := NewDocument(595, 842)
	p := mustPage(t, d)
	if err := p.appendToContentStream([]byte("% page\n")); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"% A\n", "% B\n", "% C\n"} {
		if err := p.prependToContentStream([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.appendToContentStream([]byte("% after\n")); err != nil {
		t.Fatal(err)
	}
	if err := p.prependToContentStream([]byte("% D\n")); err != nil {
		t.Fatal(err)
	}
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	last := -1
	for _, m := range []string{"% D", "% C", "% B", "% A", "% page", "% after"} {
		i := strings.Index(s, m)
		if i <= last {
			t.Fatalf("order wrong at %q: %q", m, s)
		}
		last = i
	}
}

// GitHub issue #26 point 2: one opened source stamped from several
// goroutines, each into its own document (run with -race to see a race).
func TestSharedSourceStampedConcurrently(t *testing.T) {
	src := letterhead(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := NewPdfPageStamp(src, 1)
			if err != nil {
				errs <- err
				return
			}
			d := NewDocument(595, 842)
			if err := d.AddStamp(st); err != nil {
				errs <- err
				return
			}
			var buf bytes.Buffer
			if _, err := d.WriteTo(&buf); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// twoPageSource has pages showing "ONE" and "TWO".
func twoPageSource(t *testing.T) *Document {
	t.Helper()
	d := NewDocument(300, 200)
	if err := d.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	for i, s := range []string{"ONE", "TWO"} {
		p, _ := d.Page(i + 1)
		if err := p.AddTextAt(s, TextStyle{Size: 20}, 20, 100); err != nil {
			t.Fatal(err)
		}
	}
	return reopenDoc(t, d)
}

// The import cache follows the page, not its number: after the source's
// pages move, page 1 is whatever page 1 now is.
func TestImportCacheFollowsPageNotNumber(t *testing.T) {
	src := twoPageSource(t)
	d := NewDocument(300, 200)
	if err := d.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	stamp := func(n int) {
		st, err := NewPdfPageStamp(src, 1)
		if err != nil {
			t.Fatal(err)
		}
		p, _ := d.Page(n)
		if err := p.AddStamp(st); err != nil {
			t.Fatal(err)
		}
	}
	stamp(1)
	if err := src.DeletePage(1); err != nil {
		t.Fatal(err)
	}
	stamp(2)
	got := reopenDoc(t, d)
	for i, want := range []string{"ONE", "TWO"} {
		p, _ := got.Page(i + 1)
		if txt, _ := p.ExtractText(); strings.TrimSpace(txt) != want {
			t.Errorf("page %d: %q, want %q", i+1, txt, want)
		}
	}

	src2 := twoPageSource(t)
	d2 := NewDocument(300, 200)
	f1, err := d2.ImportPage(src2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := src2.Reorder([]int{2, 1}); err != nil {
		t.Fatal(err)
	}
	f2, err := d2.ImportPage(src2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f1.formID == f2.formID {
		t.Error("after a reorder, page 1 came back as the form of the old page 1")
	}
}

// A page whose content was rewritten as a whole is imported again.
func TestImportCacheFollowsContentRewrite(t *testing.T) {
	src := twoPageSource(t)
	d := NewDocument(300, 200)
	f1, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.ReplaceText("ONE", "UNO"); err != nil {
		t.Fatal(err)
	}
	f2, err := d.ImportPage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f1.formID == f2.formID {
		t.Fatal("rewritten page not imported again")
	}
	p := mustPage(t, d)
	if err := p.AddForm(f2, Rectangle{URX: 300, URY: 200}); err != nil {
		t.Fatal(err)
	}
	if txt, _ := mustPage(t, reopenDoc(t, d)).ExtractText(); !strings.Contains(txt, "UNO") {
		t.Errorf("text %q", txt)
	}
}

// Importing a page of the document itself copies nothing, also after
// objects were added since an earlier self-import.
func TestSelfImportCopiesNothing(t *testing.T) {
	d := NewDocument(300, 200)
	if err := d.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ImportPage(d, 1); err != nil {
		t.Fatal(err)
	}
	f, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := d.Page(2)
	if err := p2.AddTextAt("page two", TextStyle{Font: f, Size: 14}, 20, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ImportPage(d, 2); err != nil {
		t.Fatal(err)
	}
	got := reopenDoc(t, d)
	if n := countObjects(got, hasFontProgram); n > 1 {
		t.Errorf("%d font programs, want 1", n)
	}
}

// A form drawn by a caller with a default colour space and by one without
// is not given the default: the second would change colour.
func TestPDFADefaultColorSpaceDisagreementNotForced(t *testing.T) {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 5 0 R /Resources << /XObject << /X 6 0 R >> /ColorSpace << /DefaultRGB [/CalRGB << /WhitePoint [0.9505 1 1.089] >>] >> >> >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 5 0 R /Resources << /XObject << /X 6 0 R >> >> >>",
		"<< /Length 0 >> stream\n/X Do\nendstream",
		"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Length 0 >> stream\n1 0 0 rg 0 0 50 50 re f\nendstream",
	}
	for _, kids := range []string{"[3 0 R 4 0 R]", "[4 0 R 3 0 R]"} {
		objs[1] = "<< /Type /Pages /Kids " + kids + " /Count 2 >>"
		d := openHardening(t, hardeningPDF(objs...))
		d.fixInheritedResources()
		form := d.objects[6].Value.(*pdfStream)
		if res, ok := resolveRefToDict(d.objects, form.Dict["/Resources"]); ok {
			cs, _ := resolveRefToDict(d.objects, res["/ColorSpace"])
			if cs["/DefaultRGB"] != nil {
				t.Errorf("kids %s: default colour space forced onto the caller that had none", kids)
			}
		}
	}
}
