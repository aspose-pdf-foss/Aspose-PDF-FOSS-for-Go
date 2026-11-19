// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// hardeningPDF assembles a classic-xref PDF from object bodies numbered from 1:
// objs[0] is object 1 and must be the catalog. A body holding "stream\n" gets
// its /Length filled in from the data between stream and endstream.
func hardeningPDF(objs ...string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		if s := strings.Index(o, "stream\n"); s >= 0 {
			e := strings.LastIndex(o, "\nendstream")
			o = strings.Replace(o, "/Length 0", fmt.Sprintf("/Length %d", e-(s+len("stream\n"))), 1)
		}
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj %s endobj\n", i+1, o)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xref)
	return buf.Bytes()
}

// refs returns "n 0 R" repeated k times.
func refs(n, k int) string {
	return strings.TrimSpace(strings.Repeat(fmt.Sprintf("%d 0 R ", n), k))
}

// withinDeadline runs fn and fails the test when it has not returned in time.
func withinDeadline(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not finish within %v", d)
	}
}

// openHardening opens one of the synthetic files above.
func openHardening(t *testing.T, data []byte) *Document {
	t.Helper()
	d, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// pageWith returns the catalog, page tree and one page whose content is
// content and whose /Resources are res; objects 1-4, the next free is 5.
// catalogExtra and pageExtra are spliced into those dictionaries.
func pageWith(catalogExtra, pageExtra, res, content string) []string {
	return []string{
		"<< /Type /Catalog /Pages 2 0 R " + catalogExtra + " >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources " + res + " " + pageExtra + " >>",
		"<< /Length 0 >> stream\n" + content + "\nendstream",
	}
}

// chain appends levels objects numbered from len(objs)+1, each made by inner
// from the number of the next one; the last is leaf.
func chain(objs []string, levels int, inner func(next int) string, leaf string) []string {
	first := len(objs) + 1
	for i := 0; i < levels; i++ {
		if i == levels-1 {
			objs = append(objs, leaf)
		} else {
			objs = append(objs, inner(first+i+1))
		}
	}
	return objs
}

// stitchingDAG is a type 3 function nesting levels deep whose /Functions
// refer fan times to the next level; the leaf is a type 2 function of n
// outputs.
func stitchingDAG(objs []string, fan, levels, n int) []string {
	var bounds, encode strings.Builder
	for i := 1; i < fan; i++ {
		fmt.Fprintf(&bounds, "%g ", float64(i)/float64(fan))
	}
	for i := 0; i < fan; i++ {
		encode.WriteString("0 1 ")
	}
	c0, c1 := strings.Repeat("0 ", n), strings.Repeat("1 ", n)
	return chain(objs, levels, func(next int) string {
		return fmt.Sprintf("<< /FunctionType 3 /Domain [0 1] /Functions [%s] /Bounds [%s] /Encode [%s] >>", refs(next, fan), bounds.String(), encode.String())
	}, fmt.Sprintf("<< /FunctionType 2 /Domain [0 1] /C0 [%s] /C1 [%s] /N 1 >>", c0, c1))
}

// The tests below pin pdf-go-wa32: walks over a document's objects (and over
// font programs) that followed shared references once per path. Each fixture
// names one object fan times at each of many levels, so an unfixed walk makes
// fan^levels calls (hours or more) on a file of a few kilobytes.

const hardeningDeadline = 20 * time.Second

func shadingDAG(fan, levels int) []byte {
	objs := pageWith("", "", "<< /Shading << /Sh0 << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 200 0] /Function 5 0 R >> >> >>", "/Sh0 sh")
	return hardeningPDF(stitchingDAG(objs, fan, levels, 3)...)
}

func TestParseFunctionSharedSubfunctions(t *testing.T) {
	// 30 levels: within maxFunctionDepth, so the leaf is still reached.
	d := openHardening(t, shadingDAG(2, 30))
	withinDeadline(t, hardeningDeadline, func() {
		if _, err := d.Pages()[0].RenderImage(RenderOptions{DPI: 10}); err != nil {
			t.Error(err)
		}
	})
	// One evaluation still reaches the leaf: white at 1.
	fn := parseFunction(d.objects, pdfRef{Num: 5})
	if got := fn.eval([]float64{1}); len(got) != 3 || got[0] != 1 {
		t.Errorf("eval(1) = %v, want white", got)
	}
}

func TestParseFunctionNamingItself(t *testing.T) {
	objs := map[int]*pdfObject{5: {Num: 5, Value: pdfDict{
		"/FunctionType": 3, "/Domain": pdfArray{0, 1}, "/Functions": pdfArray{pdfRef{Num: 5}},
		"/Bounds": pdfArray{}, "/Encode": pdfArray{0, 1},
	}}}
	withinDeadline(t, hardeningDeadline, func() {
		parseFunction(objs, pdfRef{Num: 5}).eval([]float64{0.5})
	})
}

func TestGrayscaleSharedFunctions(t *testing.T) {
	d := openHardening(t, shadingDAG(2, 40))
	withinDeadline(t, hardeningDeadline, func() {
		if err := d.ConvertToGrayscale(); err != nil {
			t.Error(err)
		}
	})
	leaf := d.objects[5+39].Value.(pdfDict)
	if c1, _ := leaf["/C1"].(pdfArray); len(c1) != 1 {
		t.Errorf("leaf function /C1 = %v, want one grey component", leaf["/C1"])
	}
}

func TestGrayscaleAppearanceNamingItself(t *testing.T) {
	objs := pageWith("", "/Annots [<< /Type /Annot /Subtype /Square /Rect [0 0 10 10] /AP 5 0 R >>]", "<< >>", "")
	objs = append(objs, "<< /N 5 0 R /D 6 0 R >>", "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >> stream\n1 0 0 rg 0 0 5 5 re f\nendstream")
	d := openHardening(t, hardeningPDF(objs...))
	withinDeadline(t, hardeningDeadline, func() {
		if err := d.ConvertToGrayscale(); err != nil {
			t.Error(err)
		}
	})
	if got := string(decodedStreamData(d.objects[6].Value.(*pdfStream))); strings.Contains(got, "rg") {
		t.Errorf("appearance reached through the dictionary not converted: %q", got)
	}
}

func TestCMYKSharedFunctions(t *testing.T) {
	objs := pageWith("", "", "<< /ColorSpace << /CS0 [/Separation /Spot /DeviceCMYK 5 0 R] >> >>", "/CS0 cs 1 scn 0 0 50 50 re f")
	d := openHardening(t, hardeningPDF(stitchingDAG(objs, 30, 8, 4)...))
	withinDeadline(t, hardeningDeadline, func() {
		if err := d.ConvertDeviceCMYKToRGB(); err != nil {
			t.Error(err)
		}
	})
	res := d.Pages()[0].pageResources()
	csd, _ := resolveRefToDict(d.objects, res["/ColorSpace"])
	if cs, _ := resolveRefToArray(d.objects, csd["/CS0"]); len(cs) < 3 || cs[2] != pdfName("/DeviceRGB") {
		t.Errorf("separation alternate = %v, want /DeviceRGB", csd["/CS0"])
	}
}

func TestCMYKSharedAppearanceDictionaries(t *testing.T) {
	const fan = 3000
	objs := pageWith("", "/Annots [<< /Type /Annot /Subtype /Square /Rect [0 0 10 10] /AP 5 0 R >>]", "<< >>", "")
	dict := func(next int) string {
		var b strings.Builder
		b.WriteString("<< ")
		for k := 0; k < fan; k++ {
			fmt.Fprintf(&b, "/K%d %d 0 R ", k, next)
		}
		return b.String() + ">>"
	}
	// Three levels of dictionaries of fan entries each, the last naming the
	// stream: unfixed, fan^3 visits.
	objs = append(objs, dict(6), dict(7), dict(8),
		"<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >> stream\n0 0 0 1 k 0 0 5 5 re f\nendstream")
	d := openHardening(t, hardeningPDF(objs...))
	withinDeadline(t, hardeningDeadline, func() {
		if err := d.ConvertDeviceCMYKToRGB(); err != nil {
			t.Error(err)
		}
	})
	if got := string(decodedStreamData(d.objects[8].Value.(*pdfStream))); strings.Contains(got, " k") {
		t.Errorf("appearance not converted: %q", got)
	}
}

const structHead = "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R /MarkInfo << /Marked true >> >>"

func structTreeDAG(fan, levels int) []byte {
	objs := []string{
		structHead,
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Type /StructTreeRoot /K 5 0 R >>",
	}
	return hardeningPDF(chain(objs, levels, func(next int) string {
		return fmt.Sprintf("<< /S /Div /K [%s] >>", refs(next, fan))
	}, "<< /S /Figure >>")...)
}

func TestStructTreeSharedElements(t *testing.T) {
	for name, data := range map[string][]byte{
		"shared": structTreeDAG(2, 40),
		// An element that is its own kid: unfixed, the walk never returns.
		"cycle": hardeningPDF(
			structHead,
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
			"<< /Type /StructTreeRoot /K 5 0 R >>",
			"<< /S /Div /K [5 0 R 6 0 R] >>",
			"<< /S /Figure >>"),
	} {
		t.Run(name, func(t *testing.T) {
			d := openHardening(t, data)
			withinDeadline(t, hardeningDeadline, func() {
				found := false
				for _, is := range d.ValidatePDFUA().Issues {
					found = found || is.Rule == "UA_FIGURE_NO_ALT"
				}
				if !found {
					t.Error("the figure below the shared elements was not reported")
				}
				figs, err := d.FiguresNeedingAltText()
				if err != nil || len(figs) != 1 {
					t.Errorf("FiguresNeedingAltText = %d figures, %v; want 1", len(figs), err)
				}
			})
		})
	}
}

func TestPageLabelsSharedNodes(t *testing.T) {
	objs := pageWith("/PageLabels 5 0 R", "", "<< >>", "")
	objs = chain(objs, 40, func(next int) string {
		return fmt.Sprintf("<< /Kids [%s] >>", refs(next, 2))
	}, "<< /Nums [0 << /S /r >>] >>")
	d := openHardening(t, hardeningPDF(objs...))
	withinDeadline(t, hardeningDeadline, func() {
		if got := d.Pages()[0].Label(); got != "i" {
			t.Errorf("Label() = %q, want i", got)
		}
	})
}

// fieldDAG is an /AcroForm whose fields nest levels deep, each naming the
// next fan times; the last is leaf.
func fieldDAG(fan, levels int, leaf string) []byte {
	objs := pageWith("/AcroForm << /Fields [5 0 R] >>", "", "<< >>", "")
	return hardeningPDF(chain(objs, levels, func(next int) string {
		return fmt.Sprintf("<< /T (a) /Kids [%s] >>", refs(next, fan))
	}, leaf)...)
}

func TestSignatureFieldsShared(t *testing.T) {
	d := openHardening(t, fieldDAG(2, 40, "<< /FT /Sig /T (s) /V << /Type /Sig >> >>"))
	withinDeadline(t, hardeningDeadline, func() {
		if got := d.SignatureNames(); len(got) != 1 || got[0] != "s" {
			t.Errorf("SignatureNames() = %v, want [s]", got)
		}
	})
}

func TestFormFieldsShared(t *testing.T) {
	d := openHardening(t, fieldDAG(2, 40, "<< /FT /Tx /T (x) /V (v) >>"))
	withinDeadline(t, hardeningDeadline, func() {
		// The last parent names the leaf twice, which stays two entries, as
		// before; it is the parents above that are now expanded once.
		fields := d.Form().Fields()
		if len(fields) != 2 || fields[0].Value() != "v" || !strings.HasSuffix(fields[0].FullName(), "a.a.x") {
			t.Errorf("Fields() = %d fields, want the leaf twice", len(fields))
		}
	})
}

func TestFDFFieldsShared(t *testing.T) {
	var b strings.Builder
	b.WriteString("%FDF-1.2\n1 0 obj << /FDF << /Fields [2 0 R] >> >> endobj\n")
	for i := 0; i < 40; i++ {
		if i == 39 {
			fmt.Fprintf(&b, "%d 0 obj << /T (x) /V (v) >> endobj\n", i+2)
		} else {
			fmt.Fprintf(&b, "%d 0 obj << /T (a) /Kids [%s] >> endobj\n", i+2, refs(i+3, 2))
		}
	}
	b.WriteString("trailer << /Root 1 0 R >>\n")
	withinDeadline(t, hardeningDeadline, func() {
		got, err := parseFDFFields([]byte(b.String()))
		if err != nil || len(got) != 1 {
			t.Errorf("parseFDFFields = %d values, %v; want 1", len(got), err)
		}
	})
}

func TestValidatePageTreeShared(t *testing.T) {
	pageTree := func(kids func(i, next int) string) []byte {
		objs := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
		for i := 0; i < 40; i++ {
			objs = append(objs, fmt.Sprintf("<< /Type /Pages %s /Count 1 >>", kids(i, i+3)))
		}
		objs = append(objs, "<< /Type /Page /Parent 41 0 R /MediaBox [0 0 200 200] >>")
		return hardeningPDF(objs...)
	}
	for name, data := range map[string][]byte{
		"shared": pageTree(func(_, next int) string { return "/Kids [" + refs(next, 2) + "]" }),
		"cycle": pageTree(func(i, next int) string {
			if i == 39 {
				return "/Kids [42 0 R 2 0 R]"
			}
			return "/Kids [" + refs(next, 1) + "]"
		}),
	} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/tree.pdf"
			if err := writeFile(path, data); err != nil {
				t.Fatal(err)
			}
			withinDeadline(t, hardeningDeadline, func() {
				rep, err := Validate(path)
				if err != nil {
					t.Fatal(err)
				}
				treeError := false
				for _, is := range rep.Issues {
					treeError = treeError || is.Code == "PAGE_TREE_ERROR"
				}
				if treeError != (name == "cycle") {
					t.Errorf("issues %v: a page tree error expected only for the cycle", rep.Issues)
				}
			})
		})
	}
}

func TestDescriptorEmbeddedNamingItself(t *testing.T) {
	font := pdfDict{"/Subtype": pdfName("/Type0"), "/DescendantFonts": pdfArray{pdfRef{Num: 5}}}
	objs := map[int]*pdfObject{5: {Num: 5, Value: font}}
	withinDeadline(t, hardeningDeadline, func() {
		if descriptorEmbedded(objs, font) {
			t.Error("a font with no descriptor reported embedded")
		}
	})
}

// TestReferenceRings covers values read by following references until one is
// not a reference: an object whose value is a reference to itself.
func TestReferenceRings(t *testing.T) {
	objs := pageWith("/Names << /JavaScript << /Names [(js) 5 0 R] >> /Dests << /Names [(nd) 6 0 R] >> >> /Outlines 7 0 R", "", "<< >>", "")
	objs = append(objs, "5 0 R", "6 0 R",
		"<< /Type /Outlines /First 8 0 R /Last 8 0 R /Count 1 >>",
		"<< /Title (o) /Parent 7 0 R /Dest 9 0 R >>", "9 0 R")
	d := openHardening(t, hardeningPDF(objs...))
	withinDeadline(t, hardeningDeadline, func() {
		if got := d.JavaScript().Get("js"); got != "" {
			t.Errorf("script = %q", got)
		}
		if d.NamedDestinations().Get("nd") != nil {
			t.Error("a destination read from a ring")
		}
		if items := d.Outlines().All(); len(items) != 1 || items[0].Destination() != nil {
			t.Errorf("outline = %d items", len(items))
		}
	})
}

func TestPSProgramDeeplyNested(t *testing.T) {
	prog := parsePSProgram([]byte(strings.Repeat("{", 100000) + strings.Repeat("}", 100000)))
	depth := 0
	for len(prog) > 0 {
		sub, ok := prog[0].([]psItem)
		if !ok {
			break
		}
		prog, depth = sub, depth+1
	}
	if depth > maxPSDepth+1 {
		t.Errorf("procedures nested %d deep, want at most %d", depth, maxPSDepth+1)
	}
}

func TestPSCopyBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("{ 1 ")
	for n := 1; n <= 1<<20; n *= 2 {
		fmt.Fprintf(&b, "%d copy ", n)
	}
	b.WriteString("}")
	st := execPS(parsePSProgram([]byte(b.String())), nil, 0)
	if len(st) > maxPSStack {
		t.Errorf("stack grew to %d, want at most %d", len(st), maxPSStack)
	}
}

func TestType1SharedSubroutines(t *testing.T) {
	f := &type1Font{charstrings: map[string][]byte{"a": {139, 10, 14}}, glyphNames: []string{"a"}, unitsPerEm: 1000}
	for i := 0; i < 32; i++ {
		var b []byte
		if i < 31 {
			b = append(b, byte(i+1+139), 10, byte(i+1+139), 10)
		}
		f.subrs = append(f.subrs, append(b, 11))
	}
	withinDeadline(t, hardeningDeadline, func() {
		f.glyphContours(0)
		f.glyphAdvance(0)
	})
}

func TestCFFSharedSubroutines(t *testing.T) {
	f := &cffFont{unitsPerEm: 1000, charStrings: [][]byte{{32, 10, 14}}}
	for i := 0; i < 11; i++ {
		var b []byte
		if i < 10 {
			for k := 0; k < 1000; k++ {
				b = append(b, byte(i+1+32), 10)
			}
		}
		f.localSubrs = append(f.localSubrs, append(b, 11))
	}
	withinDeadline(t, hardeningDeadline, func() {
		f.glyphContours(0)
		f.glyphAdvance(0)
	})
}

func TestTrueTypeSharedComponents(t *testing.T) {
	const fan, levels = 1000, 9
	var glyf []byte
	loca := []uint32{0}
	for i := 0; i < levels; i++ {
		if i < levels-1 {
			glyf = append(glyf, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0)
			for k := 0; k < fan; k++ {
				flags := uint16(0x0002)
				if k < fan-1 {
					flags |= 0x0020
				}
				glyf = append(glyf, byte(flags>>8), byte(flags), byte((i+1)>>8), byte(i+1), 0, 0)
			}
		}
		loca = append(loca, uint32(len(glyf)))
	}
	f := &ttfFont{}
	withinDeadline(t, hardeningDeadline, func() {
		budget := maxCompositeWork
		f.glyphContoursRec(0, loca, glyf, 0, &budget)
	})
}
