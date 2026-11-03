// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

// inkOf counts non-white pixels.
func inkOf(t *testing.T, p *Page) int {
	t.Helper()
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return countInk(img)
}

func countInk(img image.Image) int {
	n := 0
	r := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if cr, cg, cb, _ := img.At(x, y).RGBA(); cr>>8 < 200 || cg>>8 < 200 || cb>>8 < 200 {
				n++
			}
		}
	}
	return n
}

// formDoc returns a document with two filled text fields, with the page's /Annots
// and the form's /Fields written as references, as most producers write them.
func formDoc(t *testing.T) (*Document, *Page) {
	t.Helper()
	d := NewDocument(300, 200)
	for i, name := range []string{"a", "b"} {
		y := float64(20 + 60*i)
		f, err := d.Form().AddTextField(1, Rectangle{LLX: 20, LLY: y, URX: 280, URY: y + 40}, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetValue("WWWWWWWWWWW " + name); err != nil {
			t.Fatal(err)
		}
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := re.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	annots, _ := resolveRefToArray(re.objects, pd["/Annots"])
	pd["/Annots"] = pdfRef{Num: re.addObject(annots)}
	root, _ := resolveRefToDict(re.objects, re.catalog["/AcroForm"])
	fields, _ := resolveRefToArray(re.objects, root["/Fields"])
	root["/Fields"] = pdfRef{Num: re.addObject(fields)}
	if len(annots) != 2 || len(fields) != 2 {
		t.Fatalf("fixture: %d annots, %d fields", len(annots), len(fields))
	}
	return re, p
}

// With /Annots behind a reference nothing was baked, but the fields were still
// removed: the form vanished from the page.
func TestFlattenKeepsFormWithIndirectAnnots(t *testing.T) {
	d, p := formDoc(t)
	before := inkOf(t, p)
	if before == 0 {
		t.Fatal("fixture draws nothing")
	}
	if err := d.Flatten(); err != nil {
		t.Fatal(err)
	}
	after := inkOf(t, p)
	if after < before*9/10 || after > before*11/10 {
		t.Errorf("ink %d -> %d", before, after)
	}
	if annots, _ := resolveRefToArray(d.objects, p.pageDict()["/Annots"]); len(annots) != 0 {
		t.Errorf("widgets left on the page: %v", annots)
	}
	if _, has := d.catalog["/AcroForm"]; has {
		t.Error("/AcroForm left")
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if got := inkOf(t, rp); got < before*9/10 {
		t.Errorf("after a round trip: ink %d, was %d", got, before)
	}
}

func TestFieldFlattenWithIndirectArrays(t *testing.T) {
	d, p := formDoc(t)
	before := inkOf(t, p)
	if err := d.Form().Field("a").Flatten(); err != nil {
		t.Fatal(err)
	}
	if got := inkOf(t, p); got < before*9/10 {
		t.Errorf("ink %d -> %d", before, got)
	}
	if annots, _ := resolveRefToArray(d.objects, p.pageDict()["/Annots"]); len(annots) != 1 {
		t.Errorf("%d widgets left, want 1", len(annots))
	}
	root, _ := resolveRefToDict(d.objects, d.catalog["/AcroForm"])
	if fields, _ := resolveRefToArray(d.objects, root["/Fields"]); len(fields) != 1 {
		t.Errorf("%d fields left, want 1", len(fields))
	}
}

func TestRemoveFieldWithIndirectArrays(t *testing.T) {
	d, p := formDoc(t)
	if !d.Form().RemoveField("a") {
		t.Fatal("not removed")
	}
	if annots, _ := resolveRefToArray(d.objects, p.pageDict()["/Annots"]); len(annots) != 1 {
		t.Errorf("%d widgets left, want 1", len(annots))
	}
	if d.Form().HasField("a") || !d.Form().HasField("b") {
		t.Errorf("fields: %v", d.Form().Fields())
	}
	root, _ := resolveRefToDict(d.objects, d.catalog["/AcroForm"])
	if fields, _ := resolveRefToArray(d.objects, root["/Fields"]); len(fields) != 1 {
		t.Errorf("/Fields holds %d entries, want 1", len(fields))
	}
}

// Adding a field used to replace an indirect /Fields, losing every field in it.
func TestAddFieldKeepsIndirectFields(t *testing.T) {
	d, _ := formDoc(t)
	if _, err := d.Form().AddTextField(1, Rectangle{LLX: 20, LLY: 150, URX: 280, URY: 190}, "c"); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		if !re.Form().HasField(n) {
			t.Errorf("field %s lost", n)
		}
	}
}

// A page with the keys of an action is still a page: deleting it as one left a
// file with no pages at all.
func TestConvertToPDFAKeepsPageWithActionKeys(t *testing.T) {
	for _, format := range []PDFAFormat{PDFA1B, PDFA2B} {
		d := NewDocument(300, 200)
		p, _ := d.Page(1)
		if err := p.AddText("Hello", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 150}); err != nil {
			t.Fatal(err)
		}
		pd := p.pageObj().Value.(pdfDict)
		pd["/S"], pd["/JS"] = pdfName("/JavaScript"), "app.alert(1)"
		rep, err := d.ConvertToPDFA(format)
		if err != nil {
			t.Fatal(err)
		}
		if pdfaHasRule(rep, "JAVASCRIPT") {
			t.Errorf("%v: the keys stayed: %+v", format, rep.Issues)
		}
		if _, has := pd["/JS"]; has {
			t.Errorf("%v: /JS stayed on the page", format)
		}
		re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
		if err != nil {
			t.Fatal(err)
		}
		if re.PageCount() != 1 {
			t.Fatalf("%v: %d pages", format, re.PageCount())
		}
		rp, _ := re.Page(1)
		if text, _ := rp.ExtractText(); !strings.Contains(text, "Hello") {
			t.Errorf("%v: text %q", format, text)
		}
	}
}

// A real action object is still removed.
func TestStripPDFAActionsRemovesOrphans(t *testing.T) {
	d := NewDocument(300, 200)
	id := d.addObject(pdfDict{"/S": pdfName("/JavaScript"), "/JS": "x"})
	d.stripPDFAActions()
	if d.objects[id] != nil {
		t.Error("orphan action kept")
	}
}

func textPage(t *testing.T, group bool, gs pdfDict) (*Document, *Page) {
	t.Helper()
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	if err := p.AddText("Hello", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 150}); err != nil {
		t.Fatal(err)
	}
	pd := p.pageObj().Value.(pdfDict)
	if group {
		pd["/Group"] = pdfDict{"/S": pdfName("/Transparency"), "/CS": pdfName("/DeviceRGB")}
	}
	if gs != nil {
		res, _ := resolveRefToDict(d.objects, pd["/Resources"])
		res["/ExtGState"] = pdfDict{"/G0": gs}
	}
	return d, p
}

// Word and Chrome write a page group whether or not anything is translucent.
func TestIdlePageGroupDoesNotRasterize(t *testing.T) {
	d, p := textPage(t, true, nil)
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "TRANSPARENCY") {
		t.Error("PDF/A-1 forbids the group itself, so it is reported until removed")
	}
	rep, err := d.ConvertToPDFA(PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RasterizedPages) != 0 {
		t.Errorf("rasterized: %v", rep.RasterizedPages)
	}
	if pdfaHasRule(rep, "TRANSPARENCY") {
		t.Errorf("group left: %+v", rep.Issues)
	}
	if _, has := p.pageDict()["/Group"]; has {
		t.Error("/Group left")
	}
	if text, _ := p.ExtractText(); !strings.Contains(text, "Hello") {
		t.Errorf("text %q", text)
	}
}

func TestRealTransparencyIsRasterizedAndReported(t *testing.T) {
	for name, gs := range map[string]pdfDict{
		"alpha": {"/ca": 0.5},
		"blend": {"/BM": pdfName("/Multiply")},
	} {
		d, p := textPage(t, true, gs)
		rep, err := d.ConvertToPDFA(PDFA1B)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.RasterizedPages) != 1 || rep.RasterizedPages[0] != 1 {
			t.Errorf("%s: rasterized = %v", name, rep.RasterizedPages)
		}
		if text, _ := p.ExtractText(); !strings.Contains(text, "Hello") {
			t.Errorf("%s: text lost in rasterizing: %q", name, text)
		}
	}
	// PDF/A-2 allows transparency: nothing happens to it.
	d, p := textPage(t, true, pdfDict{"/ca": 0.5})
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RasterizedPages) != 0 {
		t.Errorf("PDF/A-2: %v", rep.RasterizedPages)
	}
	if _, has := p.pageDict()["/Group"]; !has {
		t.Error("PDF/A-2 keeps the group")
	}
}

func formStream(d *Document, group bool, resources pdfDict) (int, *pdfStream) {
	st := buildFlateStream([]byte("0 0 10 10 re f"))
	st.Dict["/Type"], st.Dict["/Subtype"] = pdfName("/XObject"), pdfName("/Form")
	st.Dict["/BBox"] = pdfArray{0, 0, 10, 10}
	if group {
		st.Dict["/Group"] = pdfDict{"/S": pdfName("/Transparency")}
	}
	if resources != nil {
		st.Dict["/Resources"] = resources
	}
	return d.addObject(st), st
}

func TestDropIdleGroups(t *testing.T) {
	// A form with nothing translucent inside loses its group, and so stays vector.
	d, p := textPage(t, false, nil)
	num, form := formStream(d, true, nil)
	res, _ := resolveRefToDict(d.objects, p.pageDict()["/Resources"])
	res["/XObject"] = pdfDict{"/F0": pdfRef{Num: num}}
	if n, err := d.FlattenTransparency(); err != nil || n != 0 {
		t.Errorf("rasterized %d, %v", n, err)
	}
	if _, has := form.Dict["/Group"]; has {
		t.Error("idle form group left")
	}
	// A form that paints translucently keeps it, in an annotation's appearance too.
	d, _ = textPage(t, false, nil)
	_, translucent := formStream(d, true, pdfDict{"/ExtGState": pdfDict{"/G0": pdfDict{"/ca": 0.5}}})
	if _, err := d.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}
	if _, has := translucent.Dict["/Group"]; !has {
		t.Error("the group of a translucent form removed")
	}
	// A soft mask's source has to stay a group.
	d, _ = textPage(t, false, nil)
	maskNum, mask := formStream(d, true, nil)
	d.addObject(pdfDict{"/SMask": pdfDict{"/S": pdfName("/Luminosity"), "/G": pdfRef{Num: maskNum}}})
	if _, err := d.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}
	if _, has := mask.Dict["/Group"]; !has {
		t.Error("a soft mask's group removed")
	}
	// A group of another kind is not ours to touch.
	d, _ = textPage(t, false, nil)
	_, other := formStream(d, false, nil)
	other.Dict["/Group"] = pdfDict{"/S": pdfName("/Other")}
	if _, err := d.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}
	if _, has := other.Dict["/Group"]; !has {
		t.Error("a non-transparency group removed")
	}
}

// With only some pages selected, a form another page may draw under its alpha is
// left alone: without its group it would composite differently there.
func TestDropIdleGroupsWithASelection(t *testing.T) {
	build := func(otherTranslucent bool) (*Document, *pdfStream, pdfDict) {
		d, p := textPage(t, true, nil)
		if err := d.AddBlankPage(300, 200); err != nil {
			t.Fatal(err)
		}
		num, form := formStream(d, true, nil)
		res, _ := resolveRefToDict(d.objects, p.pageDict()["/Resources"])
		res["/XObject"] = pdfDict{"/F0": pdfRef{Num: num}}
		p2, _ := d.Page(2)
		otherRes := pdfDict{"/XObject": pdfDict{"/F0": pdfRef{Num: num}}}
		if otherTranslucent {
			otherRes["/ExtGState"] = pdfDict{"/G0": pdfDict{"/ca": 0.5}}
		}
		p2.pageDict()["/Resources"] = otherRes
		return d, form, p.pageDict()
	}
	d, form, page := build(true)
	if _, err := d.FlattenTransparency(FlattenTransparencyOptions{Pages: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if _, has := form.Dict["/Group"]; !has {
		t.Error("a form page 2 may draw under alpha lost its group")
	}
	if _, has := page["/Group"]; has {
		t.Error("the selected page's own group left")
	}
	d, form, _ = build(false)
	if _, err := d.FlattenTransparency(FlattenTransparencyOptions{Pages: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if _, has := form.Dict["/Group"]; has {
		t.Error("nothing is translucent, yet the form kept its group")
	}
}

// A radio group's widgets are /Kids, which may sit behind a reference as well.
func TestFormReadsIndirectKids(t *testing.T) {
	d := NewDocument(300, 200)
	if _, err := d.Form().AddRadioGroup("r", []RadioItem{
		{PageNum: 1, Rect: Rectangle{LLX: 20, LLY: 20, URX: 40, URY: 40}, Export: "a"},
		{PageNum: 1, Rect: Rectangle{LLX: 60, LLY: 20, URX: 80, URY: 40}, Export: "b"},
	}); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := resolveRefToDict(re.objects, re.catalog["/AcroForm"])
	fields, _ := resolveRefToArray(re.objects, root["/Fields"])
	parent, _ := resolveRefToDict(re.objects, fields[0])
	kids, _ := resolveRefToArray(re.objects, parent["/Kids"])
	parent["/Kids"] = pdfRef{Num: re.addObject(kids)}
	f := re.Form().Field("r")
	if f == nil {
		t.Fatal("field lost")
	}
	if _, ok := f.(*RadioButtonField); !ok {
		t.Fatalf("field is %T", f)
	}
	if len(f.(*RadioButtonField).node.widgets) != 2 {
		t.Errorf("%d widgets, want 2", len(f.(*RadioButtonField).node.widgets))
	}
}

// The report names the pages by their number.
func TestRasterizedPagesNumbers(t *testing.T) {
	d, _ := textPage(t, false, nil)
	if err := d.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	p2, _ := d.Page(2)
	p2.pageDict()["/Resources"] = pdfDict{"/ExtGState": pdfDict{"/G0": pdfDict{"/ca": 0.5}}}
	rep, err := d.ConvertToPDFA(PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RasterizedPages) != 1 || rep.RasterizedPages[0] != 2 {
		t.Errorf("RasterizedPages = %v, want [2]", rep.RasterizedPages)
	}
}
