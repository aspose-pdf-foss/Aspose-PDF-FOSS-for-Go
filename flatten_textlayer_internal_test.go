// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// translucentPage draws text and a translucent rectangle, so it gets rasterized.
func translucentPage(t *testing.T, extra func(d *Document, p *Page)) (*Document, *Page) {
	t.Helper()
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	if err := p.AddText("Hello world", TextStyle{Font: FontHelvetica, Size: 14}, Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 150}); err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 60}, ShapeStyle{FillColor: &Color{R: 1, A: 0.5}}); err != nil {
		t.Fatal(err)
	}
	if extra != nil {
		extra(d, p)
	}
	return d, p
}

func TestRasterizedPageKeepsItsText(t *testing.T) {
	d, p := translucentPage(t, nil)
	before, _ := p.RenderImage(RenderOptions{DPI: 72})
	n, err := d.FlattenTransparency()
	if err != nil || n != 1 {
		t.Fatalf("%d, %v", n, err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if text, _ := rp.ExtractText(); !strings.Contains(text, "Hello world") {
		t.Errorf("text %q", text)
	}
	matches, err := rp.SearchText("world")
	if err != nil || len(matches) != 1 || matches[0].Rect.LLX < 40 || matches[0].Rect.LLY < 90 {
		t.Errorf("search: %+v, %v", matches, err)
	}
	infos, _ := rp.ImageInfos()
	if len(infos) != 1 {
		t.Errorf("%d images", len(infos))
	}
	// Nothing visible was added: the text is painted once, by the image.
	after, _ := rp.RenderImage(RenderOptions{DPI: 72})
	if a, b := countInk(before), countInk(after); a == 0 || b > a+a/20 || b < a-a/20 {
		t.Errorf("ink %d -> %d", a, b)
	}
	data, _ := rp.contentStreams()
	if !strings.Contains(string(data), "3 Tr") {
		t.Errorf("no invisible mode in %s", data)
	}
	if n, _ := re.FlattenTransparency(); n != 0 {
		t.Errorf("flattened again: %d", n)
	}
	if pdfaHasRule(re.ValidatePDFA(PDFA1B), "TRANSPARENCY") {
		t.Error("transparency left")
	}
}

func TestNoTextLayerOption(t *testing.T) {
	d, p := translucentPage(t, nil)
	if _, err := d.FlattenTransparency(FlattenTransparencyOptions{NoTextLayer: true}); err != nil {
		t.Fatal(err)
	}
	if text, _ := p.ExtractText(); strings.TrimSpace(text) != "" {
		t.Errorf("text %q", text)
	}
	if res := p.pageResources(); res["/Font"] != nil {
		t.Errorf("fonts kept: %v", res)
	}
}

// Sanitize rasterizes so that nothing but the picture remains.
func TestSanitizeRasterizingLeavesNoText(t *testing.T) {
	d, _ := translucentPage(t, nil)
	if _, err := d.Sanitize(HiddenDataSanitizationOptions{ConvertPagesToImages: true}); err != nil {
		t.Fatal(err)
	}
	data := mustBytes(t, d)
	re, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if text, _ := rp.ExtractText(); strings.TrimSpace(text) != "" {
		t.Errorf("text %q", text)
	}
	if found, _ := documentContainsBytes(re, "Hello"); found {
		t.Error("the text is still in the file")
	}
}

func drawForm(t *testing.T, d *Document, p *Page, form *XForm) {
	t.Helper()
	if err := p.AddForm(form, Rectangle{LLX: 150, LLY: 20, URX: 290, URY: 60}); err != nil {
		t.Fatal(err)
	}
}

// Text inside a Form XObject comes along, nested forms too, and the original stays
// as it was for any other page drawing it.
func TestTextLayerReachesForms(t *testing.T) {
	d, p := translucentPage(t, nil)
	inner := d.CreateForm(140, 40)
	if err := inner.Canvas().AddText("Inner text", TextStyle{Font: FontTimesRoman, Size: 12}, Rectangle{LLX: 0, LLY: 0, URX: 140, URY: 40}); err != nil {
		t.Fatal(err)
	}
	outer := d.CreateForm(140, 40)
	if err := outer.Canvas().AddForm(inner, Rectangle{LLX: 0, LLY: 0, URX: 140, URY: 40}); err != nil {
		t.Fatal(err)
	}
	drawForm(t, d, p, outer)
	before, _ := p.ExtractText()
	if err := d.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	p2, _ := d.Page(2)
	drawForm(t, d, p2, outer)
	if !strings.Contains(before, "Inner text") {
		t.Fatalf("fixture: %q", before)
	}
	if _, err := d.FlattenTransparency(FlattenTransparencyOptions{Pages: []int{1}}); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	text, _ := rp.ExtractText()
	if !strings.Contains(text, "Inner text") || !strings.Contains(text, "Hello world") {
		t.Errorf("text %q", text)
	}
	rp2, _ := re.Page(2)
	if countInk2 := inkOf(t, rp2); countInk2 == 0 {
		t.Error("the other page's form was emptied")
	}
}

// A form with no text adds nothing to the layer.
func TestTextLayerSkipsFormsWithoutText(t *testing.T) {
	d, p := translucentPage(t, nil)
	form := d.CreateForm(100, 40)
	if err := form.Canvas().DrawRectangle(Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 40}, ShapeStyle{FillColor: &Color{G: 1, A: 1}}); err != nil {
		t.Fatal(err)
	}
	drawForm(t, d, p, form)
	if _, res, ok := p.textLayerFor(); !ok || res["/XObject"] != nil {
		t.Errorf("%v %v", ok, res)
	}
	// A page with no text at all has no layer.
	d = NewDocument(300, 200)
	p, _ = d.Page(1)
	if err := p.DrawRectangle(Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 60}, ShapeStyle{FillColor: &Color{R: 1, A: 0.5}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := p.textLayerFor(); ok {
		t.Error("a layer for no text")
	}
}

func layerFor(t *testing.T, res pdfDict, content string) (string, pdfDict, bool, *Document) {
	t.Helper()
	d := NewDocument(300, 200)
	data, out, ok := newTextLayerBuilder(d).rewrite([]byte(content), res, 0)
	return string(data), out, ok, d
}

func TestTextLayerRewrite(t *testing.T) {
	font := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/X")}
	res := pdfDict{"/Font": pdfDict{"/F1": font, "/Unused": font}}

	// What paints goes; what places text stays.
	out, outRes, ok, _ := layerFor(t, res, "1 0 0 rg 0 0 10 10 re f /G0 gs q 2 0 0 2 0 0 cm BT /F1 12 Tf 1 Tc 2 Tw 90 Tz 3 TL 1 Ts 5 5 Td 1 0 0 1 9 9 Tm (a) Tj T* [(b) -5 (c)] TJ (d) ' 1 2 (e) \" 0 Tr ET Q")
	if !ok {
		t.Fatal("no text")
	}
	for _, keep := range []string{"cm", "BT", "/F1 12 Tf", "1 Tc", "2 Tw", "90 Tz", "3 TL", "1 Ts", "5 5 Td", "Tm", "T*", "(a) Tj", "TJ", "(d) '", "(e) \"", "ET", "3 Tr", "q", "Q"} {
		if !strings.Contains(out, keep) {
			t.Errorf("%q dropped from %s", keep, out)
		}
	}
	for _, drop := range []string{"rg", "re", " f", "gs", "0 Tr"} {
		if strings.Contains(out, drop) {
			t.Errorf("%q kept in %s", drop, out)
		}
	}
	fonts, _ := outRes["/Font"].(pdfDict)
	if len(fonts) != 1 || fonts["/F1"] == nil {
		t.Errorf("fonts %v", fonts)
	}

	// Stray and missing Q would upset what surrounds the layer.
	out, _, _, _ = layerFor(t, res, "Q Q BT /F1 12 Tf (a) Tj ET q q")
	if strings.Count(out, "Q") != 2 || strings.Count(out, "q") != 2 {
		t.Errorf("unbalanced: %s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "BT") {
		t.Errorf("a stray Q kept: %s", out)
	}

	// Text outside a text object is not text.
	if _, _, ok, _ := layerFor(t, res, "/F1 12 Tf (a) Tj"); ok {
		t.Error("text outside BT kept")
	}

	// Marked content stays, and with it /ActualText and the MCIDs of a tagged page.
	props := pdfDict{"/ActualText": "x"}
	r2 := pdfDict{"/Font": pdfDict{"/F1": font}, "/Properties": pdfDict{"/P0": props, "/P1": props}}
	out, outRes, _, _ = layerFor(t, r2, "/Span /P0 BDC BT /F1 12 Tf (a) Tj ET EMC /P <</MCID 3>> BDC BT (b) Tj ET EMC /Artifact BMC EMC")
	for _, keep := range []string{"/P0 BDC", "/MCID 3", "EMC", "BMC"} {
		if !strings.Contains(out, keep) {
			t.Errorf("%q dropped from %s", keep, out)
		}
	}
	got, _ := outRes["/Properties"].(pdfDict)
	if len(got) != 1 || got["/P0"] == nil {
		t.Errorf("properties %v", got)
	}
}

// Mode 3 does not stop a Type3 glyph from painting, so its text stays out, and the
// font selected before it, restored by Q, comes back.
func TestTextLayerLeavesType3Out(t *testing.T) {
	plain := pdfDict{"/Subtype": pdfName("/TrueType")}
	type3 := pdfDict{"/Subtype": pdfName("/Type3")}
	res := pdfDict{"/Font": pdfDict{"/F1": plain, "/T3": type3}}
	if _, _, ok, _ := layerFor(t, res, "BT /T3 12 Tf (a) Tj ET"); ok {
		t.Error("Type3 text kept")
	}
	out, outRes, ok, _ := layerFor(t, res, "BT /F1 12 Tf q /T3 12 Tf (a) Tj Q (b) Tj ET")
	if !ok || strings.Contains(out, "(a)") || !strings.Contains(out, "(b)") || strings.Contains(out, "/T3") {
		t.Errorf("%v %s", ok, out)
	}
	if fonts, _ := outRes["/Font"].(pdfDict); fonts["/T3"] != nil {
		t.Errorf("Type3 font kept: %v", fonts)
	}
}

// A form that draws itself, or nests past the cap, must not hang.
func TestTextLayerFormCycle(t *testing.T) {
	d := NewDocument(300, 200)
	font := pdfDict{"/Subtype": pdfName("/TrueType")}
	st := buildFlateStream([]byte("/Self Do BT /F1 12 Tf (a) Tj ET"))
	st.Dict["/Subtype"] = pdfName("/Form")
	id := d.addObject(st)
	st.Dict["/Resources"] = pdfDict{"/XObject": pdfDict{"/Self": pdfRef{Num: id}}, "/Font": pdfDict{"/F1": font}}
	if clone := newTextLayerBuilder(d).form(st, pdfDict{}, 0); clone == 0 {
		t.Error("the text beside the cycle lost")
	}
	if newTextLayerBuilder(d).form(st, pdfDict{}, maxTextLayerDepth+1) != 0 {
		t.Error("past the cap")
	}
}

// A form without /Resources resolves names through its caller's, so the same form
// under two pages with different fonts needs two clones.
func TestTextLayerInheritedResources(t *testing.T) {
	d := NewDocument(300, 200)
	st := buildFlateStream([]byte("BT /F1 12 Tf (a) Tj ET"))
	st.Dict["/Subtype"] = pdfName("/Form")
	a := pdfDict{"/Font": pdfDict{"/F1": pdfDict{"/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/A")}}}
	b := pdfDict{"/Font": pdfDict{"/F1": pdfDict{"/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/B")}}}
	tb := newTextLayerBuilder(d)
	x, y := tb.form(st, a, 0), tb.form(st, b, 0)
	if x == 0 || y == 0 || x == y {
		t.Fatalf("clones %d %d", x, y)
	}
	if again := tb.form(st, a, 0); again != x {
		t.Errorf("the same form under the same resources cloned twice: %d, %d", x, again)
	}
	clone := d.objects[x].Value.(*pdfStream)
	res, _ := clone.Dict["/Resources"].(pdfDict)
	if fonts, _ := res["/Font"].(pdfDict); fonts["/F1"] == nil {
		t.Errorf("inherited font lost: %v", res)
	}
	if clone.Dict["/Subtype"] != pdfName("/Form") {
		t.Errorf("clone %v", clone.Dict)
	}
	withBox := buildFlateStream([]byte("BT /F1 12 Tf (a) Tj ET"))
	withBox.Dict["/Subtype"] = pdfName("/Form")
	withBox.Dict["/BBox"] = pdfArray{0, 0, 5, 5}
	withBox.Dict["/Matrix"] = pdfArray{2, 0, 0, 2, 0, 0}
	withBox.Dict["/Group"] = pdfDict{"/S": pdfName("/Transparency")}
	c := d.objects[newTextLayerBuilder(d).form(withBox, a, 0)].Value.(*pdfStream)
	if c.Dict["/BBox"] == nil || c.Dict["/Matrix"] == nil || c.Dict["/Group"] != nil {
		t.Errorf("clone %v", c.Dict)
	}
}

// The image's name must not collide with a form the layer kept.
func TestTextLayerImageNameIsUnique(t *testing.T) {
	d, p := translucentPage(t, nil)
	form := d.CreateForm(140, 40)
	if err := form.Canvas().AddText("Form text", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 0, LLY: 0, URX: 140, URY: 40}); err != nil {
		t.Fatal(err)
	}
	drawForm(t, d, p, form)
	res := p.pageResources()
	xo, _ := resolveRefToDict(d.objects, res["/XObject"])
	for k, v := range xo {
		delete(xo, k)
		xo["/Im0"] = v
	}
	data, _ := p.contentStreams()
	re := strings.NewReplacer("/Fm0 Do", "/Im0 Do")
	if err := replacePageContents(p, []byte(re.Replace(string(data)))); err != nil {
		t.Fatal(err)
	}
	if _, err := d.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := reopened.Page(1)
	if text, _ := rp.ExtractText(); !strings.Contains(text, "Form text") {
		t.Errorf("text %q", text)
	}
	if infos, _ := rp.ImageInfos(); len(infos) != 1 {
		t.Errorf("%d images", len(infos))
	}
}

// The whole path: PDF/A-1 with the text kept and its fonts embedded.
func TestConvertToPDFA1KeepsRasterizedTextSearchable(t *testing.T) {
	d, _ := translucentPage(t, nil)
	rep, err := d.ConvertToPDFA(PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RasterizedPages) != 1 {
		t.Errorf("RasterizedPages = %v", rep.RasterizedPages)
	}
	for _, rule := range []string{"TRANSPARENCY", "FONT_NOT_EMBEDDED"} {
		if pdfaHasRule(rep, rule) {
			t.Errorf("%s: %+v", rule, rep.Issues)
		}
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if text, _ := rp.ExtractText(); !strings.Contains(text, "Hello world") {
		t.Errorf("text %q", text)
	}
}

// Selecting an ordinary font after a Type3 one brings the text back.
func TestTextLayerResumesAfterType3(t *testing.T) {
	res := pdfDict{"/Font": pdfDict{"/F1": pdfDict{"/Subtype": pdfName("/TrueType")}, "/T3": pdfDict{"/Subtype": pdfName("/Type3")}}}
	out, _, ok, _ := layerFor(t, res, "BT /T3 12 Tf (a) Tj /F1 12 Tf (b) Tj ET")
	if !ok || !strings.Contains(out, "(b)") || strings.Contains(out, "(a)") {
		t.Errorf("%v %s", ok, out)
	}
}

// Only a form is read as content: an image's samples could look like anything.
func TestTextLayerIgnoresImageData(t *testing.T) {
	img := buildFlateStream([]byte("BT /F1 12 Tf (a) Tj ET"))
	img.Dict["/Subtype"] = pdfName("/Image")
	d := NewDocument(300, 200)
	id := d.addObject(img)
	res := pdfDict{"/Font": pdfDict{"/F1": pdfDict{"/Subtype": pdfName("/TrueType")}}, "/XObject": pdfDict{"/I0": pdfRef{Num: id}}}
	if _, _, ok := newTextLayerBuilder(d).rewrite([]byte("/I0 Do"), res, 0); ok {
		t.Error("an image read as text")
	}
}

// A form drawing itself stops at once, not at the depth cap.
func TestTextLayerCycleMakesOneClone(t *testing.T) {
	d := NewDocument(300, 200)
	st := buildFlateStream([]byte("/Self Do BT /F1 12 Tf (a) Tj ET"))
	st.Dict["/Subtype"] = pdfName("/Form")
	id := d.addObject(st)
	st.Dict["/Resources"] = pdfDict{"/XObject": pdfDict{"/Self": pdfRef{Num: id}}, "/Font": pdfDict{"/F1": pdfDict{"/Subtype": pdfName("/TrueType")}}}
	before := len(d.objects)
	newTextLayerBuilder(d).form(st, pdfDict{}, 0)
	if got := len(d.objects) - before; got != 1 {
		t.Errorf("%d clones", got)
	}
}

// In a tagged document the picture must not count as untagged content.
func TestRasterIsAnArtifact(t *testing.T) {
	d, p := translucentPage(t, nil)
	if _, err := d.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}
	data, _ := p.contentStreams()
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "/Artifact BMC") {
		t.Errorf("content starts %.40q", data)
	}
}
