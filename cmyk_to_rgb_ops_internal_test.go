// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

// The renderer reads a colour by its operand count, so a render cannot tell whether
// the operands were converted. These look at what was written.

func cmykFormStream(data string, extra pdfDict) *pdfStream {
	dict := pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 100, 100}}
	for k, v := range extra {
		dict[k] = v
	}
	return &pdfStream{Dict: dict, Data: []byte(data), Decoded: true}
}

func cmykContentFields(t *testing.T, p *Page) string {
	t.Helper()
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(data)), " ")
}

func TestConvertDeviceCMYKWritesRGBOperands(t *testing.T) {
	for name, tc := range map[string]struct {
		build cmykCase
		want  string
	}{
		"named space": {func(d *Document, p *Page, res pdfDict) string {
			res["/ColorSpace"] = pdfDict{"/C0": pdfName("/DeviceCMYK")}
			return "/C0 cs 0 0 0 0 sc"
		}, "/DeviceRGB cs 1 1 1 sc"},
		"uncoloured pattern": {func(d *Document, p *Page, res pdfDict) string {
			res["/ColorSpace"] = pdfDict{"/C0": pdfArray{pdfName("/Pattern"), pdfName("/DeviceCMYK")}}
			return "/C0 cs 0 0 0 0 /P scn"
		}, "/C0 cs 1 1 1 /P scn"},
	} {
		d, p := cmykCaseDoc(t, tc.build)
		if err := d.ConvertDeviceCMYKToRGB(); err != nil {
			t.Fatal(err)
		}
		if got := cmykContentFields(t, p); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

// Replacing a page's stream leaves no copy of the old colours in the file.
func TestConvertDeviceCMYKDropsReplacedContent(t *testing.T) {
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string { return "0 0 0 0 k\n" })
	old := p.pageDict()["/Contents"].(pdfRef).Num
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if _, kept := d.objects[old]; kept {
		t.Error("the replaced stream stayed")
	}
}

// Every path to a content stream: resources, nesting, annotations, and none at all.
func TestConvertDeviceCMYKReachesContent(t *testing.T) {
	const named, want = "/C0 cs 0 0 0 0 sc", "/DeviceRGB cs 1 1 1 sc"
	check := func(name string, s *pdfStream) {
		t.Helper()
		if got := strings.Join(strings.Fields(string(decodedStreamData(s))), " "); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	colorSpaces := func() pdfDict { return pdfDict{"/C0": pdfName("/DeviceCMYK")} }

	// A form with no /Resources uses its caller's, also when nested.
	outer, inner := cmykFormStream(named, nil), cmykFormStream(named, nil)
	d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		res["/ColorSpace"] = colorSpaces()
		res["/XObject"] = pdfDict{"/F": pdfRef{Num: d.addObject(outer)}, "/N": pdfRef{Num: d.addObject(inner)}}
		return "/F Do\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	check("form", outer)
	check("sibling form", inner)

	nested := cmykFormStream(named, nil)
	holder := cmykFormStream("", pdfDict{"/Resources": pdfDict{"/ColorSpace": colorSpaces()}})
	d, _ = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		holder.Dict["/Resources"].(pdfDict)["/XObject"] = pdfDict{"/N": pdfRef{Num: d.addObject(nested)}}
		res["/XObject"] = pdfDict{"/F": pdfRef{Num: d.addObject(holder)}}
		return "/F Do\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	check("nested form", nested)

	// An appearance stream need not say it is a form.
	ap := &pdfStream{Dict: pdfDict{"/BBox": pdfArray{0, 0, 1, 1}, "/Resources": pdfDict{"/ColorSpace": colorSpaces()}}, Data: []byte(named), Decoded: true}
	d, _ = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		p.pageDict()["/Annots"] = pdfArray{pdfDict{"/AP": pdfDict{"/N": pdfRef{Num: d.addObject(ap)}}}}
		return ""
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	check("appearance", ap)

	// A form no page leads to is converted too; validators scan objects as well.
	orphan := cmykFormStream(named, pdfDict{"/Resources": pdfDict{"/ColorSpace": colorSpaces()}})
	d, _ = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		d.addObject(orphan)
		return ""
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	check("orphan form", orphan)

	// A rewritten stream no longer carries the filter it was stored with.
	filtered := &pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 1, 1}, "/Filter": pdfName("/ASCIIHexDecode")}, Data: []byte(fmt.Sprintf("%x>", "0 0 0 0 k"))}
	d, _ = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		res["/XObject"] = pdfDict{"/F": pdfRef{Num: d.addObject(filtered)}}
		return "/F Do\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if filtered.Dict["/Filter"] != nil || !strings.Contains(string(filtered.Data), "rg") {
		t.Errorf("%v %q", filtered.Dict, filtered.Data)
	}
}

// Resources inherited from the page tree are converted through a real file.
func TestConvertDeviceCMYKInheritedResources(t *testing.T) {
	pdf := "%PDF-1.7\n" +
		"1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1/MediaBox[0 0 100 100]/Resources<</ColorSpace<</C0[/Separation/S/DeviceCMYK 5 0 R]>>>>>>\nendobj\n" +
		"3 0 obj\n<</Type/Page/Parent 2 0 R/Contents 4 0 R>>\nendobj\n" +
		"4 0 obj\n<</Length 30>>\nstream\n/C0 cs 1 sc 10 10 50 50 re f\nendstream\nendobj\n" +
		"5 0 obj\n<</FunctionType 2/Domain[0 1]/N 1/C0[0 0 0 0]/C1[0 1 1 0]>>\nendobj\n" +
		"trailer\n<</Root 1 0 R>>\n"
	d, err := OpenStream(strings.NewReader(pdf))
	if err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Fatal("not reported")
	}
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if rep := d.ValidatePDFA(PDFA2B); hasPDFARule(rep, "COLOR_INTENT_MISMATCH") {
		t.Errorf("%+v", rep.Issues)
	}
}

func TestConvertDeviceCMYKKnownFunctions(t *testing.T) {
	// parseFunction falls back to a constant for what it does not know, and four of
	// those look like a CMYK function.
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		unknown := pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 9, "/Domain": pdfArray{0, 1}})}
		res["/Shading"] = pdfDict{"/S": pdfDict{"/ShadingType": 2, "/ColorSpace": pdfName("/DeviceCMYK"), "/Coords": pdfArray{0, 0, 1, 0}, "/Function": pdfArray{unknown, unknown, unknown, unknown}}}
		return "/S sh\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	sh, _ := resolveRefToDict(d.objects, p.pageResources()["/Shading"])
	if sh["/S"].(pdfDict)["/ColorSpace"] != pdfName("/DeviceCMYK") {
		t.Error("an unknown function was tabulated")
	}

	for name, tc := range map[string]struct {
		inputs int
		fn     func(d *Document) pdfRef
	}{
		"too many inputs to tabulate": {9, func(d *Document) pdfRef {
			domain := pdfArray{}
			for range 9 {
				domain = append(domain, 0, 1)
			}
			return pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{"/FunctionType": 4, "/Domain": domain, "/Range": pdfArray{0, 1, 0, 1, 0, 1, 0, 1}}, Data: []byte("{ pop pop pop pop pop }"), Decoded: true})}
		}},
		"domain of another size": {1, func(d *Document) pdfRef {
			return pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 2, "/Domain": pdfArray{0, 1, 0, 1}, "/N": 1, "/C0": pdfArray{0, 0, 0, 0}, "/C1": pdfArray{1, 1, 1, 1}})}
		}},
	} {
		cv := &cmykConverter{d: NewDocument(10, 10), done: map[*pdfStream]bool{}, sampled: map[[2]int]pdfRef{}}
		if _, ok := cv.sampledRGB(tc.fn(cv.d), tc.inputs); ok {
			t.Errorf("%s: tabulated", name)
		}
	}
}

func TestConvertDeviceCMYKInlineWrite(t *testing.T) {
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		return "q 50 0 0 50 10 10 cm BI /W 1 /H 1 /BPC 8 /CS /CMYK ID \xff\x00\x00\x00 EI Q\n"
	})
	before := cmykRender(t, p)
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, op := range ops {
		if op.Operator != "BI" {
			continue
		}
		found = true
		dict := op.Operands[0].(pdfDict)
		if dict["/Filter"] != pdfName("/ASCIIHexDecode") || dict["/ColorSpace"] != pdfName("/DeviceRGB") {
			t.Errorf("dict %v", dict)
		}
		if s := op.Operands[1].(string); len(s) != 7 || s[6] != '>' {
			t.Errorf("data %q", s)
		}
	}
	if !found {
		t.Fatal("no inline image")
	}
	if diff := cmykMaxDiff(before, cmykRender(t, p)); diff > 3 {
		t.Errorf("the page changed by %d", diff)
	}
}

func cmykTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		img.Set(i%4, i/4, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A JPEG is stored decoded afterwards. A colour key cannot be carried over from one:
// its ranges are in samples the conversion replaces, so such an image stays.
func TestConvertDeviceCMYKJPEG(t *testing.T) {
	data := cmykTestJPEG(t)
	plain := cmykImageDict(pdfName("/DeviceCMYK"), 4, 4)
	plain["/Filter"] = pdfName("/DCTDecode")
	keyed := cmykImageDict(pdfName("/DeviceCMYK"), 4, 4)
	keyed["/Filter"] = pdfName("/DCTDecode")
	keyed["/Mask"] = pdfArray{0, 0, 0, 0, 0, 0, 0, 0}
	ps, ks := &pdfStream{Dict: plain, Data: data}, &pdfStream{Dict: keyed, Data: data}
	d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		res["/XObject"] = pdfDict{"/A": pdfRef{Num: d.addObject(ps)}, "/B": pdfRef{Num: d.addObject(ks)}}
		return "/A Do /B Do\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if plain["/ColorSpace"] != pdfName("/DeviceRGB") || plain["/Filter"] != nil || !ps.Decoded || len(ps.Data) != 48 {
		t.Errorf("plain %v %d", plain, len(ps.Data))
	}
	if keyed["/ColorSpace"] != pdfName("/DeviceCMYK") || !bytes.Equal(ks.Data, data) {
		t.Errorf("keyed %v", keyed)
	}
}

// A size whose product wraps around must not pass as fitting an empty stream.
func TestCMYKSamplesSizeWrapping(t *testing.T) {
	big := 1
	big <<= 31
	if _, ok := cmykSamplesToRGB(nil, big, big, 8, nil); ok {
		t.Error("accepted")
	}
}

func TestDeviceCMYKDefaultCoversNamedSpace(t *testing.T) {
	d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		icc := d.addObject(&pdfStream{Dict: pdfDict{"/N": 4}, Data: []byte{0}, Decoded: true})
		res["/ColorSpace"] = pdfDict{"/DefaultCMYK": pdfArray{pdfName("/ICCBased"), pdfRef{Num: icc}}}
		return "/DeviceCMYK cs 1 0 0 0 sc\n"
	})
	if hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Error("DefaultCMYK makes it device-independent")
	}
}
