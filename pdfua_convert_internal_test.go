// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

func uaDoc(t *testing.T) *Document {
	t.Helper()
	d := NewDocument(612, 792)
	d.SetInfo(DocumentInfo{Title: "A title"})
	p, _ := d.Page(1)
	tc := d.TaggedContent()
	tc.SetLanguage("en")
	if _, err := p.TagContent(nil, StructP, func() error {
		return p.AddText("Hello", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 72, LLY: 680, URX: 540, URY: 720})
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestConvertToPDFUA(t *testing.T) {
	d := uaDoc(t)
	before := d.ValidatePDFUA()
	for _, rule := range []string{"UA_NO_IDENTIFICATION", "UA_NO_TITLE", "UA_FONT_NOT_EMBEDDED"} {
		if !hasUARule(before, rule) {
			t.Errorf("%s not reported before conversion: %+v", rule, before.Issues)
		}
	}
	rep := uaConvert(t, d)
	if !rep.Conformant {
		t.Fatalf("not conformant: %+v", rep.Issues)
	}
	check := func(t *testing.T, d *Document) {
		t.Helper()
		if rep := d.ValidatePDFUA(); !rep.Conformant {
			t.Errorf("not conformant: %+v", rep.Issues)
		}
		meta, err := d.XMP()
		if err != nil || meta.Title != "A title" {
			t.Errorf("XMP title = %q, %v", meta.Title, err)
		}
		raw, err := d.XMPRaw()
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(raw), "<pdfaSchema:namespaceURI>"+nsPDFUAID); n != 1 {
			t.Errorf("schema declared %d times, want 1", n)
		}
		if n := strings.Count(string(raw), "<pdfuaid:part>"); n != 1 {
			t.Errorf("pdfuaid:part written %d times, want 1", n)
		}
	}
	check(t, d)
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	check(t, re)
	// Converting again changes nothing that matters.
	uaConvert(t, re)
	check(t, re)
}

func TestConvertToPDFUAWithoutTitleOrTree(t *testing.T) {
	d := NewDocument(612, 792)
	if rep := uaConvert(t, d); !hasUARule(rep, "UA_NO_TITLE") || !hasUARule(rep, "UA_NOT_TAGGED") {
		t.Errorf("report = %+v", rep.Issues)
	}
	if rep := uaConvert(t, d); hasUARule(rep, "UA_NO_IDENTIFICATION") || hasUARule(rep, "UA_DISPLAY_DOCTITLE") {
		t.Errorf("identification and display flag are always written: %+v", rep.Issues)
	}
}

func TestConvertToPDFUAKeepsExistingViewerPreferences(t *testing.T) {
	d := uaDoc(t)
	vp, _ := resolveRefToDict(d.objects, d.catalog["/ViewerPreferences"])
	vp["/FitWindow"] = true
	uaConvert(t, d)
	vp, _ = resolveRefToDict(d.objects, d.catalog["/ViewerPreferences"])
	if vp["/FitWindow"] != true || vp["/DisplayDocTitle"] != true {
		t.Errorf("/ViewerPreferences = %v", vp)
	}
}

// A file can be both, whichever conversion runs first, and neither loses the
// other's identifier or schema.
func TestPDFAAndPDFUATogether(t *testing.T) {
	for name, steps := range map[string][]func(t *testing.T, d *Document){
		"A then UA": {func(t *testing.T, d *Document) { convertA(t, d, PDFA2A) }, func(t *testing.T, d *Document) { uaConvert(t, d) }},
		"UA then A": {func(t *testing.T, d *Document) { uaConvert(t, d) }, func(t *testing.T, d *Document) { convertA(t, d, PDFA2A) }},
	} {
		t.Run(name, func(t *testing.T) {
			d := uaDoc(t)
			for _, s := range steps {
				s(t, d)
			}
			if rep := d.ValidatePDFUA(); !rep.Conformant {
				t.Errorf("PDF/UA: %+v", rep.Issues)
			}
			if rep := d.ValidatePDFA(PDFA2A); !rep.Conformant {
				t.Errorf("PDF/A: %+v", rep.Issues)
			}
			raw, _ := d.XMPRaw()
			if n := strings.Count(string(raw), "<pdfaSchema:namespaceURI>"+nsPDFUAID); n != 1 {
				t.Errorf("schema declared %d times", n)
			}
		})
	}
}

func uaThDoc(t *testing.T, a pdfValue, headingLevels ...int) *Document {
	t.Helper()
	d := uaDoc(t)
	tc := d.TaggedContent()
	for _, n := range headingLevels {
		tc.Root().AddChild(StructType("/H" + string(rune('0'+n))))
	}
	th := tc.Root().AddChild(StructTable).AddChild(StructTR).AddChild(StructTH)
	if a != nil {
		th.dict["/A"] = a
	}
	return d
}

func TestValidatePDFUATableAndHeadings(t *testing.T) {
	if !hasUARule(uaThDoc(t, nil).ValidatePDFUA(), "UA_TH_NO_SCOPE") {
		t.Errorf("TH without /Scope not reported")
	}
	scope := pdfDict{"/O": pdfName("/Table"), "/Scope": pdfName("/Column")}
	if hasUARule(uaThDoc(t, scope).ValidatePDFUA(), "UA_TH_NO_SCOPE") {
		t.Errorf("TH with /Scope reported")
	}
	if hasUARule(uaThDoc(t, pdfArray{pdfDict{"/O": pdfName("/Layout")}, scope}).ValidatePDFUA(), "UA_TH_NO_SCOPE") {
		t.Errorf("TH with /Scope in an attribute array reported")
	}
	withID := uaThDoc(t, nil)
	for _, o := range withID.objects {
		if e, ok := o.Value.(pdfDict); ok && dictGetName(e, "/S") == "/TH" {
			e["/ID"] = "h1"
		}
	}
	if hasUARule(withID.ValidatePDFUA(), "UA_TH_NO_SCOPE") {
		t.Errorf("TH that cells can reach through /Headers reported")
	}
	for levels, bad := range map[[3]int]bool{{1, 2, 3}: false, {1, 2, 2}: false, {1, 3, 3}: true, {2, 3, 3}: true, {1, 2, 1}: false} {
		rep := uaThDoc(t, scope, levels[0], levels[1], levels[2]).ValidatePDFUA()
		if hasUARule(rep, "UA_HEADING_ORDER") != bad {
			t.Errorf("levels %v: order reported = %v, want %v", levels, !bad, bad)
		}
	}
}

func TestAddTaggedTableScope(t *testing.T) {
	d := NewDocument(612, 792)
	d.SetInfo(DocumentInfo{Title: "t"})
	d.TaggedContent().SetLanguage("en")
	p, _ := d.Page(1)
	tb := NewTable().SetColumnWidths([]float64{100, 100}).SetRepeatingRowsCount(1)
	tb.AddRow().AddCells("A", "B")
	tb.AddRow().AddCells("1", "2")
	if _, _, err := p.AddTaggedTable(d.TaggedContent(), nil, tb, Rectangle{LLX: 72, LLY: 400, URX: 300, URY: 700}); err != nil {
		t.Fatal(err)
	}
	if rep := uaConvert(t, d); hasUARule(rep, "UA_TH_NO_SCOPE") || !rep.Conformant {
		t.Errorf("report = %+v", rep.Issues)
	}
}

func TestAutoTagTableScopeAndHeadingOrder(t *testing.T) {
	levels := []*atElem{
		{typ: "/H3"}, {typ: StructP}, {typ: "/H5"}, {typ: "/H2"}, {typ: "/H4"},
	}
	last := atNormalizeHeadings(levels[:2], 0)
	last = atNormalizeHeadings(levels[2:], last) // carried across pages
	want := []StructType{"/H1", StructP, "/H2", "/H2", "/H3"}
	for i, e := range levels {
		if e.typ != want[i] {
			t.Errorf("element %d = %s, want %s", i, e.typ, want[i])
		}
	}
	if last != 3 {
		t.Errorf("last = %d", last)
	}

	d := NewDocument(612, 792)
	p, _ := d.Page(1)
	if _, err := p.AddTable(func() *Table {
		tb := NewTable().SetColumnWidths([]float64{100, 100}).SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 1})
		tb.AddRow().AddCells("Name", "Value")
		tb.AddRow().AddCells("a", "1")
		tb.AddRow().AddCells("b", "2")
		return tb
	}(), Rectangle{LLX: 72, LLY: 400, URX: 300, URY: 700}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	if hasUARule(d.ValidatePDFUA(), "UA_TH_NO_SCOPE") {
		t.Errorf("AutoTag wrote a /TH with no /Scope")
	}
	count := 0
	for _, o := range d.objects {
		if e, ok := o.Value.(pdfDict); ok && dictGetName(e, "/S") == "/TH" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("%d /TH elements, want 2", count)
	}
}

// What PDF/A and PDF/UA share is reported under a UA_ code too, and repaired.
func TestPDFUASharedConformance(t *testing.T) {
	d := uaDoc(t)
	d.catalog["/NeedsRendering"] = true
	if !hasUARule(d.ValidatePDFUA(), "UA_XFA_FORM") {
		t.Errorf("not reported")
	}
	if hasUARule(uaConvert(t, d), "UA_XFA_FORM") {
		t.Errorf("not repaired")
	}
}

// Another XMP property does not stand in for the identifier.
func TestValidatePDFUAIdentificationIsSpecific(t *testing.T) {
	d := uaDoc(t)
	if err := d.SetXMP(XMPMetadata{Title: "T", Custom: []XMPProperty{{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "part", Value: "2"}}}); err != nil {
		t.Fatal(err)
	}
	if !hasUARule(d.ValidatePDFUA(), "UA_NO_IDENTIFICATION") {
		t.Errorf("pdfaid:part taken for the PDF/UA identifier")
	}
}

// The tag written into the content stream and the structure element agree on the
// adjusted level.
func TestAutoTagFirstHeadingBecomesH1(t *testing.T) {
	d := NewDocument(612, 792)
	p, _ := d.Page(1)
	if err := p.AddText("Small heading", TextStyle{Font: FontHelveticaBold, Size: 14}, Rectangle{LLX: 72, LLY: 700, URX: 540, URY: 730}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Body text under it.", TextStyle{Font: FontHelvetica, Size: 10}, Rectangle{LLX: 72, LLY: 650, URX: 540, URY: 680}); err != nil {
		t.Fatal(err)
	}
	s := DefaultAutoTaggingSettings()
	s.HeadingRecognitionStrategy = HeadingRecognitionHeuristic
	s.HeadingLevels = []float64{30, 14}
	res, err := d.AutoTag(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Headings != 1 {
		t.Fatalf("Headings = %d", res.Headings)
	}
	if hasUARule(d.ValidatePDFUA(), "UA_HEADING_ORDER") {
		t.Errorf("heading order reported: %+v", d.ValidatePDFUA().Issues)
	}
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/H1 <<")) || bytes.Contains(data, []byte("/H2 <<")) {
		t.Errorf("content stream tags the heading wrongly:\n%s", data)
	}
}
