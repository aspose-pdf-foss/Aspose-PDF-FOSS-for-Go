// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func conformDoc(t *testing.T) (*Document, *Page) {
	t.Helper()
	d := NewDocument(612, 792)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Some text on the page.", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 72, LLY: 680, URX: 540, URY: 720}); err != nil {
		t.Fatal(err)
	}
	return d, p
}

func addRawAnnot(d *Document, p *Page, dict pdfDict) pdfRef {
	r := pdfRef{Num: d.addObject(dict)}
	pd := p.pageDict()
	arr, _ := resolveRef(d.objects, pd["/Annots"]).(pdfArray)
	pd["/Annots"] = append(arr, r)
	return r
}

func convertA(t *testing.T, d *Document, f PDFAFormat) *PDFAValidationReport {
	t.Helper()
	r, err := d.ConvertToPDFA(f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func hasPDFARule(r *PDFAValidationReport, rule string) bool {
	return slices.ContainsFunc(r.Issues, func(i PDFAIssue) bool { return i.Rule == rule })
}

func TestPDFAActions(t *testing.T) {
	d, p := conformDoc(t)
	rect := pdfArray{72.0, 72.0, 100.0, 100.0}
	link := func(a pdfValue) pdfDict {
		return pdfDict{"/Subtype": pdfName("/Link"), "/Rect": rect, "/A": a}
	}
	print := link(pdfDict{"/S": pdfName("/Named"), "/N": pdfName("/Print")})
	next := link(pdfDict{"/S": pdfName("/Named"), "/N": pdfName("/NextPage")})
	reset := link(pdfRef{Num: d.addObject(pdfDict{"/S": pdfName("/ResetForm")})})
	uri := link(pdfDict{"/S": pdfName("/URI"), "/URI": "https://example.com"})
	chain := link(pdfDict{"/S": pdfName("/URI"), "/URI": "https://example.com/2", "/Next": pdfArray{
		pdfDict{"/S": pdfName("/Sound")}, pdfDict{"/S": pdfName("/GoTo")},
	}})
	aa := link(pdfDict{"/S": pdfName("/URI"), "/URI": "https://example.com/3"})
	aa["/AA"] = pdfDict{"/E": pdfDict{"/S": pdfName("/GoTo")}}
	for _, a := range []pdfDict{print, next, reset, uri, chain, aa} {
		addRawAnnot(d, p, a)
	}
	// An outline item's action is reached without going through a page.
	item := pdfDict{"/Title": "x", "/A": pdfDict{"/S": pdfName("/Hide")}}
	d.addObject(item)
	d.catalog = pdfDict{"/OpenAction": pdfDict{"/S": pdfName("/Named"), "/N": pdfName("/Print")}}

	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "FORBIDDEN_ACTION") {
		t.Errorf("forbidden actions not reported")
	}
	r := convertA(t, d, PDFA2B)
	if hasPDFARule(r, "FORBIDDEN_ACTION") {
		t.Errorf("still reported after conversion: %+v", r.Issues)
	}
	for name, dict := range map[string]pdfDict{"Print": print, "ResetForm": reset, "Hide": item} {
		if _, has := dict["/A"]; has {
			t.Errorf("%s action survived", name)
		}
	}
	if _, has := d.catalog["/OpenAction"]; has {
		t.Errorf("catalog /OpenAction survived")
	}
	for name, dict := range map[string]pdfDict{"NextPage": next, "URI": uri} {
		if _, has := dict["/A"]; !has {
			t.Errorf("allowed %s action was removed", name)
		}
	}
	nextChain := chain["/A"].(pdfDict)["/Next"].(pdfArray)
	if len(nextChain) != 1 || dictGetName(nextChain[0].(pdfDict), "/S") != "/GoTo" {
		t.Errorf("chain = %v, want only the GoTo", nextChain)
	}
	if _, has := aa["/AA"]; has {
		t.Errorf("/AA survived")
	}
}

func TestPDFAImageInterpolate(t *testing.T) {
	d, _ := conformDoc(t)
	id := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Image"), "/Interpolate": true}, Data: []byte{0}, Decoded: true})
	other := d.addObject(&pdfStream{Dict: pdfDict{"/Interpolate": true}, Data: []byte{0}, Decoded: true})
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "IMAGE_INTERPOLATE") {
		t.Errorf("not reported")
	}
	if hasPDFARule(convertA(t, d, PDFA2B), "IMAGE_INTERPOLATE") {
		t.Errorf("reported after conversion")
	}
	if _, has := d.objects[id].Value.(*pdfStream).Dict["/Interpolate"]; has {
		t.Errorf("image still interpolates")
	}
	if _, has := d.objects[other].Value.(*pdfStream).Dict["/Interpolate"]; !has {
		t.Errorf("a stream that is not an image was touched")
	}
}

func TestPDFAAppearances(t *testing.T) {
	d, p := conformDoc(t)
	stream := func() pdfRef {
		return pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 10, 10}}, Data: []byte("q Q"), Decoded: true})}
	}
	rect := pdfArray{72.0, 72.0, 100.0, 100.0}
	n, down := stream(), stream()
	push := pdfDict{"/Subtype": pdfName("/Widget"), "/FT": pdfName("/Btn"), "/Ff": 65536, "/Rect": rect, "/AP": pdfDict{"/N": n, "/D": down}}
	parent := pdfRef{Num: d.addObject(pdfDict{"/FT": pdfName("/Btn"), "/T": "group"})}
	yes := stream()
	check := pdfDict{"/Subtype": pdfName("/Widget"), "/Parent": parent, "/AS": pdfName("/Yes"), "/Rect": rect, "/AP": pdfDict{"/N": pdfDict{"/Yes": yes, "/Off": stream()}}}
	on := stream()
	stamp := pdfDict{"/Subtype": pdfName("/Stamp"), "/AS": pdfName("/On"), "/Rect": rect, "/AP": pdfDict{"/N": pdfDict{"/On": on, "/Off": stream()}}}
	for _, a := range []pdfDict{push, check, stamp} {
		addRawAnnot(d, p, a)
	}
	if r := d.ValidatePDFA(PDFA2B); !hasPDFARule(r, "ANNOTATION_AP_ENTRIES") {
		t.Errorf("/D not reported: %+v", r.Issues)
	}
	r := convertA(t, d, PDFA2B)
	if hasPDFARule(r, "ANNOTATION_AP_ENTRIES") || hasPDFARule(r, "ANNOTATION_AP_N_TYPE") {
		t.Errorf("still reported: %+v", r.Issues)
	}
	ap := push["/AP"].(pdfDict)
	if _, has := ap["/D"]; has {
		t.Errorf("/D survived")
	}
	states, ok := ap["/N"].(pdfDict)
	if !ok || states["/Default"] != n || dictGetName(push, "/AS") != "/Default" {
		t.Errorf("push button appearance = %v, /AS = %v", ap["/N"], push["/AS"])
	}
	if states, ok := check["/AP"].(pdfDict)["/N"].(pdfDict); !ok || states["/Yes"] != yes {
		t.Errorf("checkbox with an inherited /FT lost its states: %v", check["/AP"])
	}
	if stamp["/AP"].(pdfDict)["/N"] != on {
		t.Errorf("stamp /N = %v, want the /AS state's stream", stamp["/AP"])
	}
}

// The push button must still paint through its state dictionary.
func TestPDFAPushButtonStillRenders(t *testing.T) {
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	if _, err := d.Form().AddPushButton(1, Rectangle{LLX: 20, LLY: 20, URX: 180, URY: 180}, "b", "Go"); err != nil {
		t.Fatal(err)
	}
	painted := func() int {
		img, err := p.RenderImage(RenderOptions{DPI: 72})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for y := range 200 {
			for x := range 200 {
				if r, g, b, _ := img.At(x, y).RGBA(); r < 0xF000 || g < 0xF000 || b < 0xF000 {
					n++
				}
			}
		}
		return n
	}
	before := painted()
	if before == 0 {
		t.Fatal("button does not paint before conversion")
	}
	convertA(t, d, PDFA2B)
	if after := painted(); after < before/2 {
		t.Errorf("button painted %d pixels before conversion, %d after", before, after)
	}
}

func TestPDFAXMPProperties(t *testing.T) {
	const keep = "urn:example:declared#"
	blocks := []string{`<rdf:Description rdf:about=""><pdfaExtension:schemas><rdf:Bag><rdf:li rdf:parseType="Resource"><pdfaSchema:namespaceURI>` + keep + `</pdfaSchema:namespaceURI><pdfaSchema:prefix>a"b</pdfaSchema:prefix></rdf:li></rdf:Bag></pdfaExtension:schemas></rdf:Description>`}
	in := []XMPProperty{
		{Namespace: keep, Name: "Declared"},
		{Namespace: nsPDF, Name: "PDFVersion"},
		{Namespace: nsXMPTPg, Name: "NPages"},
		{Namespace: nsXMPTPg, Name: "HasVisibleOverprint"},
		{Namespace: "urn:example:other#", Name: "Undeclared"},
	}
	kept, undeclared := splitDeclaredXMP(in, blocks)
	var names []string
	for _, p := range kept {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"Declared", "PDFVersion", "NPages"}) {
		t.Errorf("kept = %v", names)
	}
	names = nil
	for _, p := range undeclared {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"HasVisibleOverprint", "Undeclared"}) {
		t.Errorf("undeclared = %v", names)
	}

	d, _ := conformDoc(t)
	if err := d.SetXMP(XMPMetadata{Title: "T", Custom: []XMPProperty{{Namespace: "urn:example:other#", Prefix: "o", Name: "Undeclared", Value: "v"}}}); err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "XMP_UNDECLARED_PROPERTY") {
		t.Errorf("undeclared property not reported")
	}
	if hasPDFARule(convertA(t, d, PDFA2B), "XMP_UNDECLARED_PROPERTY") {
		t.Errorf("undeclared property survived conversion")
	}
}

func TestReconcileInfoAndXMP(t *testing.T) {
	d, _ := conformDoc(t)
	d.SetInfo(DocumentInfo{
		Title: "Info title", Subject: "Info subject", Author: "Info author", Keywords: "a, b",
		Creator: "Info creator", CreationDate: "D:20131202103459", ModDate: "D:2014",
	})
	meta := XMPMetadata{Title: "XMP title", Authors: []string{"One", "Two"}, CreateDate: "1999-01-01T00:00:00"}
	d.reconcileInfoAndXMP(&meta)
	info, err := d.Info()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "XMP title" || info.Title != "XMP title" {
		t.Errorf("title: xmp %q, info %q", meta.Title, info.Title)
	}
	if meta.Description != "Info subject" || info.Subject != "Info subject" {
		t.Errorf("subject: xmp %q, info %q", meta.Description, info.Subject)
	}
	if meta.CreatorTool != "Info creator" {
		t.Errorf("creator: %q", meta.CreatorTool)
	}
	if !slices.Equal(meta.Authors, []string{"One, Two"}) || info.Author != "One, Two" {
		t.Errorf("authors: xmp %v, info %q", meta.Authors, info.Author)
	}
	if info.CreationDate != "D:20131202103459Z" || meta.CreateDate != "2013-12-02T10:34:59Z" {
		t.Errorf("creation date: info %q, xmp %q", info.CreationDate, meta.CreateDate)
	}
	if info.ModDate != "D:20140101000000Z" || meta.ModifyDate != "2014-01-01T00:00:00Z" {
		t.Errorf("mod date: info %q, xmp %q", info.ModDate, meta.ModifyDate)
	}
	if !slices.Equal(meta.Keywords, []string{"a", "b"}) {
		t.Errorf("keywords: %v", meta.Keywords)
	}
	if !slices.ContainsFunc(meta.Custom, func(p XMPProperty) bool {
		return p.Namespace == nsPDF && p.Name == "Keywords" && p.Value == "a, b"
	}) {
		t.Errorf("pdf:Keywords missing: %+v", meta.Custom)
	}

	// /Info with a value XMP lacks is reported for PDF/A-1 only.
	d2, _ := conformDoc(t)
	d2.SetInfo(DocumentInfo{Title: "Only in info"})
	if err := d2.SetXMP(XMPMetadata{CreatorTool: "x"}); err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(d2.ValidatePDFA(PDFA1B), "INFO_XMP_MISMATCH") {
		t.Errorf("mismatch not reported for PDF/A-1")
	}
	if hasPDFARule(d2.ValidatePDFA(PDFA2B), "INFO_XMP_MISMATCH") {
		t.Errorf("mismatch reported for PDF/A-2, which does not ask")
	}
	if hasPDFARule(convertA(t, d2, PDFA1B), "INFO_XMP_MISMATCH") {
		t.Errorf("still mismatched after conversion")
	}
}

func TestPDFDateWithZone(t *testing.T) {
	for in, want := range map[string]string{
		"D:20131202103459":        "D:20131202103459Z",
		"D:2013":                  "D:20130101000000Z",
		"D:201312":                "D:20131201000000Z",
		"D:20131202103459Z":       "D:20131202103459Z",
		"D:20131202103459+01'00'": "D:20131202103459+01'00'",
		"D:20131202103459-05'00'": "D:20131202103459-05'00'",
		"2013-12-02T10:34:59":     "2013-12-02T10:34:59",
		"D:garbage":               "D:garbage",
		"":                        "",
	} {
		if got := pdfDateWithZone(in); got != want {
			t.Errorf("pdfDateWithZone(%q) = %q, want %q", in, got, want)
		}
	}
}

// A producer may store a date as a UTF-16 text string.
func TestInfoDecodesUTF16Date(t *testing.T) {
	d, _ := conformDoc(t)
	d.SetInfo(DocumentInfo{Title: "t"})
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if re.info == nil {
		t.Fatal("no info dict")
	}
	re.info["/CreationDate"] = "\xfe\xff\x00D\x00:\x002\x000\x001\x002"
	info, err := re.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.CreationDate != "D:2012" {
		t.Errorf("CreationDate = %q", info.CreationDate)
	}
}

func cidFontDoc(t *testing.T, name string) (*Document, pdfDict) {
	t.Helper()
	d := NewDocument(612, 792)
	p, _ := d.Page(1)
	f, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("A B", TextStyle{Font: f, Size: 12}, Rectangle{LLX: 72, LLY: 680, URX: 540, URY: 720}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SubsetFonts(); err != nil {
		t.Fatal(err)
	}
	for _, o := range d.objects {
		if font, ok := o.Value.(pdfDict); ok && dictGetName(font, "/Subtype") == "/CIDFontType2" {
			font["/BaseFont"] = pdfName(name)
			return d, font
		}
	}
	t.Fatal("no CID font")
	return nil, nil
}

func cidBit(set []byte, cid int) bool {
	return cid/8 < len(set) && set[cid/8]&(0x80>>(cid%8)) != 0
}

func TestPDFACIDSet(t *testing.T) {
	d, font := cidFontDoc(t, "/ABCDEF+DejaVu")
	desc, _ := resolveRefToDict(d.objects, font["/FontDescriptor"])
	delete(desc, "/CIDSet")
	if !hasPDFARule(d.ValidatePDFA(PDFA1B), "CIDSET_MISSING") {
		t.Errorf("missing /CIDSet not reported for PDF/A-1")
	}
	if hasPDFARule(d.ValidatePDFA(PDFA2B), "CIDSET_MISSING") {
		t.Errorf("missing /CIDSet reported for PDF/A-2, which does not ask")
	}
	desc["/CIDSet"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte{0x01}, Decoded: true})}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "CIDSET_WRONG") {
		t.Errorf("wrong /CIDSet not reported")
	}

	r := convertA(t, d, PDFA1B)
	if hasPDFARule(r, "CIDSET_MISSING") || hasPDFARule(r, "CIDSET_WRONG") {
		t.Errorf("reported after conversion: %+v", r.Issues)
	}
	s, ok := resolveRef(d.objects, desc["/CIDSet"]).(*pdfStream)
	if !ok {
		t.Fatal("no /CIDSet written")
	}
	set := decodedStreamData(s)
	var used map[uint16]bool
	for num, cids := range d.pdfaUsedCIDs() {
		if len(cids) > 0 {
			used = cids
			_ = num
		}
	}
	if len(used) == 0 {
		t.Fatal("no used CIDs")
	}
	for cid := range used { // includes the space, a glyph with no outline
		if !cidBit(set, int(cid)) {
			t.Errorf("used CID %d not in /CIDSet", cid)
		}
	}
	for cid := 1; cid < len(set)*8; cid++ {
		if cidBit(set, cid) && !used[uint16(cid)] {
			t.Errorf("CID %d is neither drawn nor present in the subset", cid)
			break
		}
	}

	// A later part just drops it, as does a font that is not a subset.
	d2, font2 := cidFontDoc(t, "/ABCDEF+DejaVu")
	desc2, _ := resolveRefToDict(d2.objects, font2["/FontDescriptor"])
	desc2["/CIDSet"] = pdfRef{Num: d2.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte{0x01}, Decoded: true})}
	if r := convertA(t, d2, PDFA2B); hasPDFARule(r, "CIDSET_WRONG") {
		t.Errorf("wrong /CIDSet survived PDF/A-2: %+v", r.Issues)
	}
	if _, has := desc2["/CIDSet"]; has {
		t.Errorf("/CIDSet kept for PDF/A-2")
	}
	d3, font3 := cidFontDoc(t, "/DejaVu")
	desc3, _ := resolveRefToDict(d3.objects, font3["/FontDescriptor"])
	convertA(t, d3, PDFA1B)
	if _, has := desc3["/CIDSet"]; has {
		t.Errorf("/CIDSet written for a font that is not a subset")
	}
}

func TestIsSubsetFontName(t *testing.T) {
	for name, want := range map[string]bool{
		"/ABCDEF+Arial": true, "ABCDEF+X": true, "/abcdef+Arial": false,
		"/ABCDE+Arial": false, "/Arial": false, "/ABCDEF+": false, "/ABC1EF+Arial": false,
	} {
		if got := isSubsetFontName(name); got != want {
			t.Errorf("isSubsetFontName(%q) = %v", name, got)
		}
	}
}

// A substitute font with an /Encoding of only /Differences gets an explicit base,
// and every code keeps its character.
func TestPDFADifferencesGetBase(t *testing.T) {
	d, p := conformDoc(t)
	font := pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfDict{"/Differences": pdfArray{65, pdfName("/Euro"), pdfName("/fi")}},
	}
	num := d.addObject(font)
	res, _ := resolveRefToDict(d.objects, p.pageDict()["/Resources"])
	res["/Font"] = pdfDict{"/F9": pdfRef{Num: num}}
	if err := p.appendToContentStream([]byte("BT /F9 12 Tf 72 300 Td (ABC\xad') Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	before := resolveFont(d.objects, font).encoding
	before2, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	convertA(t, d, PDFA2B)
	after := d.objects[num].Value.(pdfDict)
	enc, ok := after["/Encoding"].(pdfDict)
	if !ok || dictGetName(enc, "/BaseEncoding") != "/WinAnsiEncoding" {
		t.Fatalf("/Encoding = %v", after["/Encoding"])
	}
	diffs, _ := enc["/Differences"].(pdfArray)
	for _, v := range diffs {
		if n, ok := v.(pdfName); ok && (strings.HasPrefix(string(n), "/uniFFFD") || n == "/.notdef") {
			t.Errorf("/Differences names %v, which is not in the Adobe Glyph List", n)
		}
	}
	got := resolveFont(d.objects, after).encoding
	for c := range 256 {
		if before[c] != 0 && before[c] != 0xFFFD && got[c] != before[c] {
			t.Errorf("code %d: %U became %U", c, before[c], got[c])
		}
	}
	after2, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if before2 != after2 {
		t.Errorf("text changed: %q -> %q", before2, after2)
	}
}

func TestPDFAObjectMetadata(t *testing.T) {
	d, _ := conformDoc(t)
	img := pdfDict{"/Subtype": pdfName("/Image"), "/Metadata": pdfRef{Num: 1}}
	id := d.addObject(&pdfStream{Dict: img, Data: []byte{0}, Decoded: true})
	if err := d.SetXMP(XMPMetadata{Title: "Kept"}); err != nil {
		t.Fatal(err)
	}
	convertA(t, d, PDFA2B)
	if _, has := d.objects[id].Value.(*pdfStream).Dict["/Metadata"]; has {
		t.Errorf("an image kept its /Metadata")
	}
	meta, err := d.XMP()
	if err != nil || meta.Title != "Kept" {
		t.Errorf("the document's own XMP was lost: %+v, %v", meta, err)
	}
}

func TestSharedConformance(t *testing.T) {
	d, _ := conformDoc(t)
	ocg1, ocg2 := pdfRef{Num: d.addObject(pdfDict{"/Type": pdfName("/OCG"), "/Name": "a"})}, pdfRef{Num: d.addObject(pdfDict{"/Type": pdfName("/OCG"), "/Name": "b"})}
	cfgD := pdfDict{"/AS": pdfArray{}, "/Order": pdfArray{ocg1}}
	alt := pdfDict{"/Name": "Alt", "/Order": pdfArray{pdfArray{ocg1, ocg2}}}
	form := pdfDict{"/XFA": "x", "/Fields": pdfArray{}}
	d.catalog = pdfDict{
		"/NeedsRendering": true,
		"/AcroForm":       form,
		"/OCProperties":   pdfDict{"/OCGs": pdfArray{ocg1, ocg2}, "/D": cfgD, "/Configs": pdfArray{alt}},
	}
	gs := pdfDict{"/Type": pdfName("/ExtGState"), "/TR": pdfName("/Identity"), "/TR2": pdfName("/X")}
	gsDefault := pdfDict{"/Type": pdfName("/ExtGState"), "/TR2": pdfName("/Default")}
	mask := pdfDict{"/S": pdfName("/Luminosity"), "/TR": pdfName("/Identity")}
	d.addObject(gs)
	d.addObject(gsDefault)
	d.addObject(mask)
	cid := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/X"),
		"/FontDescriptor": pdfDict{"/FontFile2": pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte{0}, Decoded: true})}}}
	cidID := d.addObject(cid)
	noFile := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/Y")})

	check := func(rule string) {
		t.Helper()
		if !hasPDFARule(d.ValidatePDFA(PDFA2B), rule) {
			t.Errorf("%s not reported", rule)
		}
	}
	check("XFA_FORM")
	check("EXTGSTATE_TRANSFER")
	delete(form, "/XFA")
	check("XFA_FORM") // /NeedsRendering alone
	delete(d.catalog, "/NeedsRendering")
	check("LAYER_CONFIG")
	saved := cfgD["/AS"]
	delete(cfgD, "/AS")
	cfgD["/Name"] = "n"
	check("LAYER_CONFIG") // /Order omits a layer
	cfgD["/Order"] = pdfArray{ocg1, ocg2}
	if r := d.ValidatePDFA(PDFA2B); hasPDFARule(r, "LAYER_CONFIG") {
		t.Errorf("complete configuration reported: %+v", r.Issues)
	}
	check("CIDFONT_NO_CIDTOGIDMAP")
	cfgD["/Order"] = pdfArray{ocg1}
	delete(cfgD, "/Name")
	cfgD["/AS"] = saved
	form["/XFA"] = "x"
	d.catalog["/NeedsRendering"] = true

	r := convertA(t, d, PDFA2B)
	for _, rule := range []string{"XFA_FORM", "EXTGSTATE_TRANSFER", "LAYER_CONFIG", "CIDFONT_NO_CIDTOGIDMAP"} {
		if hasPDFARule(r, rule) {
			t.Errorf("%s still reported", rule)
		}
	}
	if _, has := form["/XFA"]; has {
		t.Errorf("/XFA survived")
	}
	if _, has := d.catalog["/NeedsRendering"]; has {
		t.Errorf("/NeedsRendering survived")
	}
	if _, has := cfgD["/AS"]; has {
		t.Errorf("/AS survived")
	}
	if name, _ := cfgD["/Name"].(string); name == "" {
		t.Errorf("configuration has no name")
	}
	if alt["/Name"] != "Alt" {
		t.Errorf("an existing name was replaced: %v", alt["/Name"])
	}
	if o, _ := cfgD["/Order"].(pdfArray); len(o) != 2 || o[0] != ocg1 || o[1] != ocg2 {
		t.Errorf("/Order = %v", cfgD["/Order"])
	}
	if o, _ := alt["/Order"].(pdfArray); len(o) != 1 {
		t.Errorf("a nested /Order was treated as incomplete: %v", alt["/Order"])
	}
	if _, has := gs["/TR"]; has {
		t.Errorf("/TR survived")
	}
	if _, has := gs["/TR2"]; has {
		t.Errorf("/TR2 survived")
	}
	if gsDefault["/TR2"] != pdfName("/Default") {
		t.Errorf("/TR2 /Default was removed")
	}
	if _, has := mask["/TR"]; !has {
		t.Errorf("a soft mask's /TR was removed")
	}
	if cid["/CIDToGIDMap"] != pdfName("/Identity") {
		t.Errorf("/CIDToGIDMap = %v", cid["/CIDToGIDMap"])
	}
	if _, has := d.objects[noFile].Value.(pdfDict)["/CIDToGIDMap"]; has {
		t.Errorf("a font with no program got a /CIDToGIDMap")
	}
	_ = cidID
}

func TestPDFAReportedViolations(t *testing.T) {
	icc := func(n int, space string) *pdfStream {
		b := make([]byte, 128)
		copy(b[16:], space)
		copy(b[36:], "acsp")
		return &pdfStream{Dict: pdfDict{"/N": n}, Data: b, Decoded: true}
	}
	withIntent := func(d *Document, n int, space string) {
		id := d.addObject(icc(n, space))
		oi := d.addObject(pdfDict{"/S": pdfName("/GTS_PDFA1"), "/DestOutputProfile": pdfRef{Num: id}})
		if d.catalog == nil {
			d.catalog = pdfDict{}
		}
		d.catalog["/OutputIntents"] = pdfArray{pdfRef{Num: oi}}
	}

	// DeviceCMYK under an RGB intent, by operator and by image.
	d, p := conformDoc(t)
	withIntent(d, 3, "RGB ")
	if hasPDFARule(d.ValidatePDFA(PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Errorf("RGB content under an RGB intent reported")
	}
	if err := p.appendToContentStream([]byte("0 0 0 1 k\n")); err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Errorf("k under an RGB intent not reported")
	}
	if !hasPDFARule(convertA(t, d, PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Errorf("conversion should still report DeviceCMYK")
	}
	d, p = conformDoc(t)
	withIntent(d, 3, "RGB ")
	p.pageResources()["/XObject"] = pdfDict{"/I": pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Image"), "/ColorSpace": pdfName("/DeviceCMYK")}, Data: []byte{0}, Decoded: true})}}
	if err := p.appendToContentStream([]byte("/I Do\n")); err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Errorf("CMYK image under an RGB intent not reported")
	}
	// DeviceRGB under a CMYK intent.
	d, p = conformDoc(t)
	withIntent(d, 4, "CMYK")
	if err := p.appendToContentStream([]byte("1 0 0 rg\n")); err != nil {
		t.Fatal(err)
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "COLOR_INTENT_MISMATCH") {
		t.Errorf("rg under a CMYK intent not reported")
	}

	// Layers: PDF/A-1 forbids them; conversion must not reveal what they hide.
	d, _ = conformDoc(t)
	d.AddLayer("L")
	if !hasPDFARule(d.ValidatePDFA(PDFA1B), "LAYERS_FORBIDDEN") {
		t.Errorf("layers not reported for PDF/A-1")
	}
	if hasPDFARule(d.ValidatePDFA(PDFA2B), "LAYERS_FORBIDDEN") {
		t.Errorf("layers reported for PDF/A-2")
	}
	convertA(t, d, PDFA1B)
	if _, has := d.catalog["/OCProperties"]; !has {
		t.Errorf("conversion removed the layers")
	}

	// An ICC profile holding another component count.
	d, _ = conformDoc(t)
	d.addObject(icc(4, "RGB "))
	if !hasPDFARule(d.ValidatePDFA(PDFA1B), "ICC_COMPONENTS_MISMATCH") {
		t.Errorf("ICC /N mismatch not reported")
	}
	d, _ = conformDoc(t)
	d.addObject(icc(3, "RGB "))
	if hasPDFARule(d.ValidatePDFA(PDFA1B), "ICC_COMPONENTS_MISMATCH") {
		t.Errorf("a correct ICC profile reported")
	}
}
