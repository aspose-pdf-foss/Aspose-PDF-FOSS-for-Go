// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cmykTint(d *Document) pdfRef {
	return pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 2, "/Domain": pdfArray{0, 1}, "/N": 1, "/C0": pdfArray{0, 0, 0, 0}, "/C1": pdfArray{0, 1, 1, 0}})}
}

func cmykImageDict(cs pdfValue, w, h int) pdfDict {
	return pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Image"), "/Width": w, "/Height": h, "/BitsPerComponent": 8, "/ColorSpace": cs}
}

// cmykCase builds one way of painting in DeviceCMYK and returns the page content.
type cmykCase func(d *Document, p *Page, res pdfDict) string

var cmykCases = map[string]cmykCase{
	"k operator": func(d *Document, p *Page, res pdfDict) string { return "1 0 0 0 k 10 10 50 50 re f\n" },
	"K operator": func(d *Document, p *Page, res pdfDict) string { return "4 w 0 1 0 0 K 10 10 50 50 re S\n" },
	"cs by device name": func(d *Document, p *Page, res pdfDict) string {
		return "/DeviceCMYK cs 1 0 0 0 sc 10 10 50 50 re f\n"
	},
	"CS by resource name": func(d *Document, p *Page, res pdfDict) string {
		res["/ColorSpace"] = pdfDict{"/C0": pdfName("/DeviceCMYK")}
		return "4 w /C0 CS 0 1 0 0 SCN 10 10 50 50 re S\n"
	},
	"Separation": func(d *Document, p *Page, res pdfDict) string {
		res["/ColorSpace"] = pdfDict{"/C0": pdfArray{pdfName("/Separation"), pdfName("/Spot"), pdfName("/DeviceCMYK"), cmykTint(d)}}
		return "/C0 cs 0.5 sc 10 10 50 50 re f\n"
	},
	"DeviceN": func(d *Document, p *Page, res pdfDict) string {
		fn := d.addObject(&pdfStream{Dict: pdfDict{"/FunctionType": 4, "/Domain": pdfArray{0, 1, 0, 1}, "/Range": pdfArray{0, 1, 0, 1, 0, 1, 0, 1}}, Data: []byte("{ 0 0 }"), Decoded: true})
		res["/ColorSpace"] = pdfDict{"/C0": pdfArray{pdfName("/DeviceN"), pdfArray{pdfName("/A"), pdfName("/B")}, pdfName("/DeviceCMYK"), pdfRef{Num: fn}}}
		return "/C0 cs 0.75 0.25 scn 10 10 50 50 re f\n"
	},
	"Indexed image": func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: cmykImageDict(pdfArray{pdfName("/Indexed"), pdfName("/DeviceCMYK"), 1, "\x00\xff\x00\x00\xff\x00\x00\x00"}, 2, 1), Data: []byte{0, 1}, Decoded: true})
		res["/XObject"] = pdfDict{"/I": pdfRef{Num: id}}
		return "q 50 0 0 50 10 10 cm /I Do Q\n"
	},
	"image": func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: cmykImageDict(pdfName("/DeviceCMYK"), 2, 1), Data: []byte{255, 0, 0, 0, 0, 255, 255, 0}, Decoded: true})
		res["/XObject"] = pdfDict{"/I": pdfRef{Num: id}}
		return "q 50 0 0 50 10 10 cm /I Do Q\n"
	},
	"form": func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 100, 100}}, Data: []byte("1 0 0 0 k 10 10 50 50 re f\n"), Decoded: true})
		res["/XObject"] = pdfDict{"/F": pdfRef{Num: id}}
		return "/F Do\n"
	},
	"form using the page's resources": func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 100, 100}}, Data: []byte("/C0 cs 1 0 0 0 sc 10 10 50 50 re f\n"), Decoded: true})
		res["/XObject"] = pdfDict{"/F": pdfRef{Num: id}}
		res["/ColorSpace"] = pdfDict{"/C0": pdfName("/DeviceCMYK")}
		return "/F Do\n"
	},
	"shading": func(d *Document, p *Page, res pdfDict) string {
		res["/Shading"] = pdfDict{"/S": pdfDict{"/ShadingType": 2, "/ColorSpace": pdfName("/DeviceCMYK"), "/Coords": pdfArray{0, 0, 100, 0}, "/Function": cmykTint(d)}}
		return "q 10 10 80 80 re W n /S sh Q\n"
	},
	"shading pattern": func(d *Document, p *Page, res pdfDict) string {
		sh := d.addObject(pdfDict{"/ShadingType": 2, "/ColorSpace": pdfName("/DeviceCMYK"), "/Coords": pdfArray{0, 0, 100, 0}, "/Function": cmykTint(d)})
		res["/Pattern"] = pdfDict{"/P": pdfDict{"/PatternType": 2, "/Shading": pdfRef{Num: sh}}}
		return "/Pattern cs /P scn 10 10 80 80 re f\n"
	},
	"tiling pattern": func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/Pattern"), "/PatternType": 1, "/PaintType": 1, "/TilingType": 1, "/BBox": pdfArray{0, 0, 10, 10}, "/XStep": 10, "/YStep": 10, "/Resources": pdfDict{}}, Data: []byte("0 1 0 0 k 0 0 5 5 re f\n"), Decoded: true})
		res["/Pattern"] = pdfDict{"/P": pdfRef{Num: id}}
		return "/Pattern cs /P scn 10 10 50 50 re f\n"
	},
	"transparency group": func(d *Document, p *Page, res pdfDict) string {
		p.pageDict()["/Group"] = pdfDict{"/S": pdfName("/Transparency"), "/CS": pdfName("/DeviceCMYK")}
		return "1 0 0 rg 10 10 50 50 re f\n"
	},
	"inline image": func(d *Document, p *Page, res pdfDict) string {
		return "q 50 0 0 50 10 10 cm BI /W 1 /H 1 /BPC 8 /CS /CMYK /F /AHx ID ff000000> EI Q\n"
	},
	"inline Indexed image": func(d *Document, p *Page, res pdfDict) string {
		return "q 50 0 0 50 10 10 cm BI /W 1 /H 1 /BPC 8 /CS [/I /CMYK 0 <ff000000>] /F /AHx ID 00> EI Q\n"
	},
	"soft mask group": func(d *Document, p *Page, res pdfDict) string {
		g := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 100, 100}, "/Group": pdfDict{"/S": pdfName("/Transparency"), "/CS": pdfName("/DeviceGray")}}, Data: []byte("0 0 0 0 k 0 0 100 100 re f\n"), Decoded: true})
		res["/ExtGState"] = pdfDict{"/G": pdfDict{"/SMask": pdfDict{"/S": pdfName("/Luminosity"), "/G": pdfRef{Num: g}}}}
		return "/G gs 1 0 0 rg 10 10 50 50 re f\n"
	},
}

func cmykCaseDoc(t *testing.T, build cmykCase) (*Document, *Page) {
	t.Helper()
	d := NewDocument(100, 100)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	res := pdfDict{}
	p.pageDict()["/Resources"] = res
	content := build(d, p, res)
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte(content), Decoded: true})}
	d.SetInfo(DocumentInfo{Title: "CMYK"})
	return d, p
}

func cmykRender(t *testing.T, p *Page) *image.RGBA {
	t.Helper()
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return img.(*image.RGBA)
}

// cmykMaxDiff is the largest difference in one channel between two renders.
func cmykMaxDiff(a, b *image.RGBA) int {
	worst := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		worst = max(worst, d)
	}
	return worst
}

// Every way of painting in DeviceCMYK is reported, and conversion removes it without
// changing how the page looks, in memory and after saving. With CMYK_PDF_DIR set,
// the files are written there for veraPDF.
func TestConvertDeviceCMYKToRGB(t *testing.T) {
	dir := os.Getenv("CMYK_PDF_DIR")
	for name, build := range cmykCases {
		t.Run(name, func(t *testing.T) {
			d, p := cmykCaseDoc(t, build)
			before := cmykRender(t, p)
			if !hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
				t.Fatal("not reported before converting")
			}
			if err := d.ConvertDeviceCMYKToRGB(); err != nil {
				t.Fatal(err)
			}
			if rep := d.ValidatePDFA(PDFA2B); hasPDFARule(rep, "COLOR_INTENT_MISMATCH") {
				t.Errorf("still reported: %+v", rep.Issues)
			}
			if diff := cmykMaxDiff(before, cmykRender(t, p)); diff > 3 {
				t.Errorf("the page changed by %d", diff)
			}
			var buf bytes.Buffer
			if _, err := d.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if rep := reopened.ValidatePDFA(PDFA2B); hasPDFARule(rep, "COLOR_INTENT_MISMATCH") {
				t.Errorf("reported after saving: %+v", rep.Issues)
			}
			q, err := reopened.Page(1)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmykMaxDiff(before, cmykRender(t, q)); diff > 3 {
				t.Errorf("the page changed by %d after saving", diff)
			}
			if dir != "" {
				file := strings.NewReplacer(" ", "_", "'", "").Replace(name) + ".pdf"
				if err := os.WriteFile(filepath.Join(dir, file), buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// What veraPDF does not count is not reported either.
func TestDeviceCMYKNotPaintedNotReported(t *testing.T) {
	for name, build := range map[string]cmykCase{
		"unused colour space": func(d *Document, p *Page, res pdfDict) string {
			res["/ColorSpace"] = pdfDict{"/C0": pdfName("/DeviceCMYK")}
			return "1 0 0 rg 10 10 50 50 re f\n"
		},
		"unused image": func(d *Document, p *Page, res pdfDict) string {
			d.addObject(&pdfStream{Dict: cmykImageDict(pdfName("/DeviceCMYK"), 1, 1), Data: []byte{0, 0, 0, 0}, Decoded: true})
			return "1 0 0 rg 10 10 50 50 re f\n"
		},
		"ICCBased": func(d *Document, p *Page, res pdfDict) string {
			icc := d.addObject(&pdfStream{Dict: pdfDict{"/N": 4}, Data: []byte{0}, Decoded: true})
			res["/ColorSpace"] = pdfDict{"/C0": pdfArray{pdfName("/ICCBased"), pdfRef{Num: icc}}}
			return "/C0 cs 1 0 0 0 sc 10 10 50 50 re f\n"
		},
		"DefaultCMYK": func(d *Document, p *Page, res pdfDict) string {
			icc := d.addObject(&pdfStream{Dict: pdfDict{"/N": 4}, Data: []byte{0}, Decoded: true})
			res["/ColorSpace"] = pdfDict{"/DefaultCMYK": pdfArray{pdfName("/ICCBased"), pdfRef{Num: icc}}}
			return "1 0 0 0 k 10 10 50 50 re f\n"
		},
	} {
		d, _ := cmykCaseDoc(t, build)
		if hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
			t.Errorf("%s: reported", name)
		}
	}
}

func TestConvertDeviceCMYKLeavesTheRestAlone(t *testing.T) {
	// No CMYK: the page's content stream stays the same object, byte for byte.
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string { return "1 0 0 rg  10 10 50 50 re f\n" })
	ref := p.pageDict()["/Contents"]
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	s, _ := resolveRef(d.objects, ref).(*pdfStream)
	if p.pageDict()["/Contents"] != ref || s == nil || string(s.Data) != "1 0 0 rg  10 10 50 50 re f\n" {
		t.Errorf("content rewritten: %v", p.pageDict()["/Contents"])
	}

	// ICCBased CMYK has its own profile: operands, space and image all stay.
	d, p = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		icc := pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{"/N": 4}, Data: []byte{0}, Decoded: true})}
		cs := pdfArray{pdfName("/ICCBased"), icc}
		res["/ColorSpace"] = pdfDict{"/C0": cs}
		res["/XObject"] = pdfDict{"/I": pdfRef{Num: d.addObject(&pdfStream{Dict: cmykImageDict(cs, 1, 1), Data: []byte{1, 2, 3, 4}, Decoded: true})}}
		return "/C0 cs 1 0 0 0 sc 10 10 50 50 re f\n"
	})
	content := p.pageDict()["/Contents"]
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if p.pageDict()["/Contents"] != content {
		t.Error("ICCBased content rewritten")
	}
	xo, _ := resolveRefToDict(d.objects, p.pageResources()["/XObject"])
	img, _ := resolveRef(d.objects, xo["/I"]).(*pdfStream)
	if img == nil || !bytes.Equal(img.Data, []byte{1, 2, 3, 4}) {
		t.Error("ICCBased image converted")
	}

	// A CMYK output intent covers DeviceCMYK: converting would break it.
	d, p = cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string { return "1 0 0 0 k\n" })
	id := d.addObject(&pdfStream{Dict: pdfDict{"/N": 4}, Data: []byte{0}, Decoded: true})
	d.catalog = pdfDict{"/OutputIntents": pdfArray{pdfDict{"/S": pdfName("/GTS_PDFA1"), "/DestOutputProfile": pdfRef{Num: id}}}}
	content = p.pageDict()["/Contents"]
	if err := d.ConvertDeviceCMYKToRGB(); err == nil {
		t.Error("a CMYK output intent should refuse")
	}
	if p.pageDict()["/Contents"] != content {
		t.Error("refused but changed")
	}
}

// The colour space is part of the graphics state, so Q brings it back.
func TestConvertDeviceCMYKFollowsGraphicsState(t *testing.T) {
	cv := &cmykConverter{d: NewDocument(10, 10), done: map[*pdfStream]bool{}, sampled: map[[2]int]pdfRef{}}
	for content, want := range map[string]string{
		"/DeviceCMYK cs q /DeviceRGB cs 1 0 0 sc Q 0 0 0 0 sc": "/DeviceRGB cs q /DeviceRGB cs 1 0 0 sc Q 1 1 1 sc",
		"/DeviceCMYK cs 0 g 0 0 0 0 sc":                        "/DeviceRGB cs 0 g 0 0 0 0 sc",
		"/DeviceCMYK CS 0 G 0 0 0 0 SC":                        "/DeviceRGB CS 0 G 0 0 0 0 SC",
		"/DeviceCMYK cs 0 0 0 rg 0 0 0 0 sc":                   "/DeviceRGB cs 0 0 0 rg 0 0 0 0 sc",
		"/DeviceCMYK CS 0 0 0 RG 0 0 0 0 SC":                   "/DeviceRGB CS 0 0 0 RG 0 0 0 0 SC",
		"/DeviceCMYK cs 0 0 0 0 k 0 0 0 0 sc":                  "/DeviceRGB cs 1 1 1 rg 0 0 0 0 sc",
		"/DeviceCMYK CS 0 0 0 0 K 0 0 0 0 SC":                  "/DeviceRGB CS 1 1 1 RG 0 0 0 0 SC",
		"/DeviceCMYK cs /DeviceGray CS 0 0 0 0 SC 0 0 0 0 sc":  "/DeviceRGB cs /DeviceGray CS 0 0 0 0 SC 1 1 1 sc",
		"Q /DeviceCMYK cs 0 0 0 0 sc":                          "Q /DeviceRGB cs 1 1 1 sc",
	} {
		ops, err := parseContentStream([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
		cv.rewriteOps(ops, pdfDict{})
		got := strings.Join(strings.Fields(string(serializeContentOps(ops))), " ")
		if got != want {
			t.Errorf("%q:\n got %q\nwant %q", content, got, want)
		}
	}
}

func TestConvertDeviceCMYKUncolouredPattern(t *testing.T) {
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/Pattern"), "/PatternType": 1, "/PaintType": 2, "/TilingType": 1, "/BBox": pdfArray{0, 0, 10, 10}, "/XStep": 10, "/YStep": 10, "/Resources": pdfDict{}}, Data: []byte("0 0 5 5 re f\n"), Decoded: true})
		res["/Pattern"] = pdfDict{"/P": pdfRef{Num: id}}
		res["/ColorSpace"] = pdfDict{"/C0": pdfArray{pdfName("/Pattern"), pdfName("/DeviceCMYK")}}
		return "/C0 cs 1 0 0 0 /P scn 10 10 50 50 re f\n"
	})
	before := cmykRender(t, p)
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	csd, _ := resolveRefToDict(d.objects, p.pageResources()["/ColorSpace"])
	if arr, _ := csd["/C0"].(pdfArray); len(arr) != 2 || arr[1] != pdfName("/DeviceRGB") {
		t.Errorf("space %v", csd["/C0"])
	}
	if diff := cmykMaxDiff(before, cmykRender(t, p)); diff > 3 {
		t.Errorf("the page changed by %d", diff)
	}
}

// /Decode applies before the conversion and goes with it.
func TestConvertDeviceCMYKImageDecode(t *testing.T) {
	dict := cmykImageDict(pdfName("/DeviceCMYK"), 1, 1)
	dict["/Decode"] = pdfArray{1, 0, 1, 0, 1, 0, 1, 0}
	dict["/Filter"] = pdfName("/ASCIIHexDecode")
	s := &pdfStream{Dict: dict, Data: []byte("00ffffff>")}
	d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		res["/XObject"] = pdfDict{"/I": pdfRef{Num: d.addObject(s)}}
		return "/I Do\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	r, g, b := adobeCMYKToRGB(1, 0, 0, 0)
	if !bytes.Equal(s.Data, []byte{r, g, b}) || dict["/Decode"] != nil || dict["/Filter"] != nil || !s.Decoded {
		t.Errorf("%v %v", s.Data, dict)
	}
}

// Colours outside content: /C, /IC, /DA and /MK on annotations, a shading's
// /Background, a soft mask's /Matte and backdrop, and a colour-key mask.
func TestConvertDeviceCMYKColoursOutsideContent(t *testing.T) {
	d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string { return "" })
	annot := pdfDict{
		"/Subtype": pdfName("/FreeText"), "/Rect": pdfArray{0, 0, 10, 10},
		"/C": pdfArray{0, 0, 0, 0}, "/IC": pdfArray{0, 0, 0, 0}, "/DA": "/Helv 12 Tf 0 0 0 0 k",
		"/MK": pdfDict{"/BC": pdfArray{0, 0, 0, 0}, "/BG": pdfArray{0, 0, 0, 0}},
	}
	p.pageDict()["/Annots"] = pdfArray{pdfRef{Num: d.addObject(annot)}}
	shading := pdfDict{"/ShadingType": 2, "/ColorSpace": pdfName("/DeviceCMYK"), "/Coords": pdfArray{0, 0, 1, 0}, "/Function": cmykTint(d), "/Background": pdfArray{0, 0, 0, 0}}
	d.addObject(shading)
	// A mesh has its colours in its data, so it stays with everything it holds.
	mesh := &pdfStream{Dict: pdfDict{"/ShadingType": 4, "/ColorSpace": pdfName("/DeviceCMYK"), "/Background": pdfArray{0, 0, 0, 0}}, Data: []byte{1}, Decoded: true}
	d.addObject(mesh)
	iccGroup := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 1, 1}, "/Group": pdfDict{"/S": pdfName("/Transparency"), "/CS": pdfArray{pdfName("/ICCBased"), pdfRef{Num: 999}}}}, Decoded: true})
	smICC := pdfDict{"/S": pdfName("/Luminosity"), "/G": pdfRef{Num: iccGroup}, "/BC": pdfArray{0, 0, 0, 0}}
	d.addObject(smICC)
	plainGroup := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 1, 1}, "/Group": pdfDict{"/S": pdfName("/Transparency"), "/CS": pdfName("/DeviceCMYK")}}, Decoded: true})
	smPlain := pdfDict{"/S": pdfName("/Luminosity"), "/G": pdfRef{Num: plainGroup}, "/BC": pdfArray{0, 0, 0, 0}}
	d.addObject(smPlain)
	sm := &pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Image"), "/ColorSpace": pdfName("/DeviceGray"), "/Width": 1, "/Height": 1, "/BitsPerComponent": 8, "/Matte": pdfArray{0, 0, 0, 0}}, Data: []byte{9}, Decoded: true}
	smID := d.addObject(sm)
	imgDict := cmykImageDict(pdfName("/DeviceCMYK"), 2, 1)
	imgDict["/SMask"] = pdfRef{Num: smID}
	imgDict["/Mask"] = pdfArray{255, 255, 0, 0, 0, 0, 0, 0}
	img := &pdfStream{Dict: imgDict, Data: []byte{255, 0, 0, 0, 0, 255, 0, 0}, Decoded: true}
	d.addObject(img)

	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	three := func(v pdfValue, what string) {
		t.Helper()
		if arr, ok := v.(pdfArray); !ok || len(arr) != 3 {
			t.Errorf("%s: %v", what, v)
		}
	}
	three(annot["/C"], "/C")
	three(annot["/IC"], "/IC")
	mk := annot["/MK"].(pdfDict)
	three(mk["/BC"], "/MK/BC")
	three(mk["/BG"], "/MK/BG")
	if da := annot["/DA"].(string); !strings.Contains(da, "1 1 1 rg") || strings.Contains(da, " k") {
		t.Errorf("/DA %q", da)
	}
	three(shading["/Background"], "shading /Background")
	if shading["/ColorSpace"] != pdfName("/DeviceRGB") {
		t.Errorf("shading space %v", shading["/ColorSpace"])
	}
	if mesh.Dict["/ColorSpace"] != pdfName("/DeviceCMYK") || len(mesh.Dict["/Background"].(pdfArray)) != 4 {
		t.Errorf("mesh changed: %v", mesh.Dict)
	}
	if len(smICC["/BC"].(pdfArray)) != 4 {
		t.Errorf("the backdrop of an ICC group changed: %v", smICC["/BC"])
	}
	three(smPlain["/BC"], "soft mask /BC")
	three(sm.Dict["/Matte"], "/Matte")
	if img.Dict["/ColorSpace"] != pdfName("/DeviceRGB") || len(img.Data) != 6 {
		t.Fatalf("image %v", img.Dict)
	}
	// The first pixel falls within the key and is masked out, the second not.
	maskRef, ok := img.Dict["/Mask"].(pdfRef)
	if !ok {
		t.Fatalf("mask %v", img.Dict["/Mask"])
	}
	mask := resolveRef(d.objects, maskRef).(*pdfStream)
	if !isTrue(mask.Dict["/ImageMask"]) || mask.Data[0]&0xC0 != 0x80 {
		t.Errorf("stencil %v %08b", mask.Dict, mask.Data[0])
	}
}

// What cannot be converted stays as it was, and stays reported.
func TestConvertDeviceCMYKLeavesWhatItCannot(t *testing.T) {
	base := func(extra pdfDict) pdfDict {
		dict := pdfDict{"/Subtype": pdfName("/Image"), "/ColorSpace": pdfName("/DeviceCMYK"), "/Width": 1, "/Height": 1}
		for k, v := range extra {
			dict[k] = v
		}
		return dict
	}
	for name, dict := range map[string]pdfDict{
		"too short":            cmykImageDict(pdfName("/DeviceCMYK"), 10, 10),
		"odd bits":             base(pdfDict{"/BitsPerComponent": 3}),
		"no size":              {"/Subtype": pdfName("/Image"), "/ColorSpace": pdfName("/DeviceCMYK")},
		"damaged JPEG":         base(pdfDict{"/Filter": pdfName("/DCTDecode")}),
		"damaged JPEG 2000":    base(pdfDict{"/Filter": pdfName("/JPXDecode")}),
		"JPEG with colour key": base(pdfDict{"/Filter": pdfName("/DCTDecode"), "/Mask": pdfArray{0, 0, 0, 0, 0, 0, 0, 0}}),
	} {
		d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
			decoded := dictGetName(dict, "/Filter") == ""
			res["/XObject"] = pdfDict{"/I": pdfRef{Num: d.addObject(&pdfStream{Dict: dict, Data: []byte{1, 2}, Decoded: decoded})}}
			return "/I Do\n"
		})
		if err := d.ConvertDeviceCMYKToRGB(); err != nil {
			t.Fatal(err)
		}
		if dict["/ColorSpace"] != pdfName("/DeviceCMYK") {
			t.Errorf("%s: converted to %v", name, dict["/ColorSpace"])
		}
		if !hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
			t.Errorf("%s: no longer reported", name)
		}
	}

	// Too many colorants to tabulate, or a function that cannot be evaluated.
	names := make(pdfArray, 9)
	domain9 := pdfArray{}
	for i := range names {
		names[i] = pdfName("/C" + string(rune('a'+i)))
		domain9 = append(domain9, 0, 1)
	}
	sep := func(fn pdfRef) pdfArray {
		return pdfArray{pdfName("/Separation"), pdfName("/S"), pdfName("/DeviceCMYK"), fn}
	}
	for name, cs := range map[string]func(d *Document) pdfArray{
		"nine colorants": func(d *Document) pdfArray {
			fn := d.addObject(&pdfStream{Dict: pdfDict{"/FunctionType": 4, "/Domain": domain9, "/Range": pdfArray{0, 1, 0, 1, 0, 1, 0, 1}}, Data: []byte("{ }"), Decoded: true})
			return pdfArray{pdfName("/DeviceN"), names, pdfName("/DeviceCMYK"), pdfRef{Num: fn}}
		},
		"unknown function": func(d *Document) pdfArray {
			return sep(pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 9, "/Domain": pdfArray{0, 1}})})
		},
		"three outputs": func(d *Document) pdfArray {
			return sep(pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 2, "/Domain": pdfArray{0, 1}, "/N": 1, "/C0": pdfArray{0, 0, 0}, "/C1": pdfArray{1, 1, 1}})})
		},
		"wrong domain": func(d *Document) pdfArray {
			return sep(pdfRef{Num: d.addObject(pdfDict{"/FunctionType": 2, "/Domain": pdfArray{0, 1, 0, 1}, "/N": 1})})
		},
	} {
		d, p := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
			res["/ColorSpace"] = pdfDict{"/C0": cs(d)}
			return "/C0 cs 0 sc 0 0 10 10 re f\n"
		})
		if err := d.ConvertDeviceCMYKToRGB(); err != nil {
			t.Fatal(err)
		}
		csd, _ := resolveRefToDict(d.objects, p.pageResources()["/ColorSpace"])
		if arr := csd["/C0"].(pdfArray); arr[2] != pdfName("/DeviceCMYK") {
			t.Errorf("%s: converted %v", name, arr)
		}
		if !hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
			t.Errorf("%s: no longer reported", name)
		}
	}
}

func TestConvertDeviceCMYKSamples(t *testing.T) {
	want := func(c, m, y, k float64) []byte {
		r, g, b := adobeCMYKToRGB(c, m, y, k)
		return []byte{r, g, b}
	}
	for _, tc := range []struct {
		name    string
		samples []byte
		w, h    int
		bpc     int
		decode  []float64
		out     []byte
	}{
		{"8 bits", []byte{255, 0, 0, 0}, 1, 1, 8, nil, want(1, 0, 0, 0)},
		{"16 bits", []byte{255, 255, 0, 0, 0, 0, 128, 0}, 1, 1, 16, nil, want(1, 0, 0, 32768.0/65535)},
		{"4 bits", []byte{0xF0, 0x0F}, 1, 1, 4, nil, want(1, 0, 0, 1)},
		{"2 bits", []byte{0b11000001}, 1, 1, 2, nil, want(1, 0, 0, 1.0/3)},
		{"1 bit, rows padded", []byte{0b10000000, 0b01000000}, 1, 2, 1, nil, append(want(1, 0, 0, 0), want(0, 1, 0, 0)...)},
		{"/Decode", []byte{0, 255, 255, 255}, 1, 1, 8, []float64{1, 0, 1, 0, 1, 0, 1, 0}, want(1, 0, 0, 0)},
		{"second pixel differs", []byte{0, 0, 0, 0, 0, 0, 0, 255}, 2, 1, 8, nil, append(want(0, 0, 0, 0), want(0, 0, 0, 1)...)},
	} {
		got, ok := cmykSamplesToRGB(tc.samples, tc.w, tc.h, tc.bpc, tc.decode)
		if !ok || !bytes.Equal(got, tc.out) {
			t.Errorf("%s: %v %v, want %v", tc.name, got, ok, tc.out)
		}
	}
	for _, tc := range []struct {
		name          string
		samples       []byte
		w, h, bitsPer int
	}{
		{"short", []byte{0, 0, 0}, 1, 1, 8},
		{"zero", nil, 0, 1, 8},
		{"huge", nil, 1 << 30, 1 << 30, 8},
		{"bad bits", make([]byte, 8), 1, 1, 3},
	} {
		if _, ok := cmykSamplesToRGB(tc.samples, tc.w, tc.h, tc.bitsPer, nil); ok {
			t.Errorf("%s accepted", tc.name)
		}
	}
}

// A sampled function reads several inputs, the first varying fastest (ISO 32000-1
// §7.10.2), and interpolates between them.
func TestSampledFunctionSeveralInputs(t *testing.T) {
	d := NewDocument(10, 10)
	s := &pdfStream{Dict: pdfDict{
		"/FunctionType": 0, "/Domain": pdfArray{0, 1, 0, 1}, "/Range": pdfArray{0, 1},
		"/Size": pdfArray{2, 3}, "/BitsPerSample": 8,
	}, Data: []byte{0, 51, 102, 153, 204, 255}, Decoded: true}
	fn := parseFunction(d.objects, s)
	for _, tc := range []struct{ x, y, want float64 }{
		{0, 0, 0},
		{1, 0, 0.2},
		{0, 0.5, 0.4},
		{1, 0.5, 0.6},
		{0, 1, 0.8},
		{1, 1, 1},
		{0.5, 0, 0.1},
		{0, 0.25, 0.2},
		{0.5, 0.75, 0.7},
	} {
		if got := fn.eval([]float64{tc.x, tc.y})[0]; got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	for _, size := range []pdfArray{{0}, {1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}} {
		s.Dict["/Size"] = size
		if _, ok := parseFunction(d.objects, s).(*fnConst); !ok {
			t.Errorf("size %v accepted", size)
		}
	}
}

// Damaged files can make a colour space or an appearance refer to itself.
func TestConvertDeviceCMYKSelfReference(t *testing.T) {
	d, _ := cmykCaseDoc(t, func(d *Document, p *Page, res pdfDict) string {
		id := d.addObject(pdfDict{})
		d.objects[id].Value = pdfArray{pdfName("/Indexed"), pdfRef{Num: id}, 0, ""}
		res["/ColorSpace"] = pdfDict{"/C0": pdfRef{Num: id}}
		ap := d.addObject(pdfDict{})
		d.objects[ap].Value = pdfDict{"/N": pdfRef{Num: ap}}
		p.pageDict()["/Annots"] = pdfArray{pdfDict{"/AP": pdfRef{Num: ap}}}
		return "/C0 cs 0 sc\n"
	})
	if err := d.ConvertDeviceCMYKToRGB(); err != nil {
		t.Fatal(err)
	}
	if hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Error("reported")
	}
}
