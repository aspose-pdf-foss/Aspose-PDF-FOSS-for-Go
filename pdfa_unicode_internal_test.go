// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// unicodeFontDoc is a document drawing "A" in a symbolic font whose /ToUnicode maps
// that code to the given hex value, or with none when the value is empty.
func unicodeFontDoc(t *testing.T, value, content string) *Document {
	t.Helper()
	d := simpleFontDoc(t, nil, content)
	if value == "" {
		return d
	}
	cmap := "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n1 begincodespacerange\n<00> <FF>\nendcodespacerange\n1 beginbfchar\n<41> <" + value + ">\nendbfchar\nendcmap\nend\nend\n"
	stream := d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte(cmap), Decoded: true})
	for _, obj := range d.objects {
		if dict, ok := obj.Value.(pdfDict); ok && dictGetName(dict, "/Type") == "/Font" {
			dict["/ToUnicode"] = pdfRef{Num: stream}
		}
	}
	return d
}

func TestPDFAUnicodeLevels(t *testing.T) {
	const text = "BT /F1 12 Tf (A) Tj ET"
	if PDFA1B != 0 {
		t.Fatal("AttachInvoice relies on PDFA1B being the zero value")
	}
	for _, tc := range []struct {
		format PDFAFormat
		name   string
		part   int
		letter string
	}{
		{PDFA2U, "PDF/A-2U", 2, "U"},
		{PDFA3U, "PDF/A-3U", 3, "U"},
		{PDFA2B, "PDF/A-2B", 2, "B"},
		{PDFA2A, "PDF/A-2A", 2, "A"},
		{PDFA3B, "PDF/A-3B", 3, "B"},
	} {
		if tc.format.String() != tc.name || tc.format.part() != tc.part || tc.format.conformance() != tc.letter {
			t.Errorf("%s: %q part %d letter %q", tc.name, tc.format, tc.format.part(), tc.format.conformance())
		}
	}

	// Who asks for a mapping: every level but b.
	for format, want := range map[PDFAFormat]bool{
		PDFA1B: false, PDFA2B: false, PDFA3B: false,
		PDFA2U: true, PDFA3U: true,
		PDFA1A: true, PDFA2A: true, PDFA3A: true,
	} {
		d := unicodeFontDoc(t, "", text)
		if got := hasPDFARule(d.ValidatePDFA(format), "NO_UNICODE_MAPPING"); got != want {
			t.Errorf("%v with no mapping: reported %v", format, got)
		}
		d = unicodeFontDoc(t, "0416", text)
		if hasPDFARule(d.ValidatePDFA(format), "NO_UNICODE_MAPPING") {
			t.Errorf("%v with a mapping: reported", format)
		}
	}
}

func TestPDFAInvalidUnicodeValue(t *testing.T) {
	const text = "BT /F1 12 Tf (A) Tj ET"
	for _, value := range []string{"0000", "FEFF", "FFFE", "00410000"} {
		for format, want := range map[PDFAFormat]bool{PDFA2U: true, PDFA3U: true, PDFA2A: true, PDFA2B: false, PDFA1A: false, PDFA1B: false} {
			if got := hasPDFARule(unicodeFontDoc(t, value, text).ValidatePDFA(format), "INVALID_UNICODE_VALUE"); got != want {
				t.Errorf("%s under %v: reported %v", value, format, got)
			}
		}
	}
	if hasPDFARule(unicodeFontDoc(t, "0416", text).ValidatePDFA(PDFA2U), "INVALID_UNICODE_VALUE") {
		t.Error("an ordinary value reported")
	}
	// A code the page does not draw is no violation.
	if hasPDFARule(unicodeFontDoc(t, "0000", "BT /F1 12 Tf (B) Tj ET").ValidatePDFA(PDFA2U), "INVALID_UNICODE_VALUE") {
		t.Error("an undrawn code reported")
	}
}

func TestPDFAPrivateUseNeedsActualText(t *testing.T) {
	const plain = "BT /F1 12 Tf (A) Tj ET"
	const rule = "PRIVATE_USE_NO_ACTUALTEXT"
	for _, value := range []string{"E000", "F8FF", "DB80DC00", "DBFFDFFD"} {
		if !hasPDFARule(unicodeFontDoc(t, value, plain).ValidatePDFA(PDFA2A), rule) {
			t.Errorf("%s not reported", value)
		}
	}
	for _, value := range []string{"DFFF", "F900", "D7FF", "0416"} {
		if hasPDFARule(unicodeFontDoc(t, value, plain).ValidatePDFA(PDFA2A), rule) {
			t.Errorf("%s reported", value)
		}
	}
	for format, want := range map[PDFAFormat]bool{PDFA2A: true, PDFA3A: true, PDFA2U: false, PDFA2B: false, PDFA1A: false} {
		if got := hasPDFARule(unicodeFontDoc(t, "E000", plain).ValidatePDFA(format), rule); got != want {
			t.Errorf("%v: reported %v", format, got)
		}
	}
	for content, want := range map[string]bool{
		"/Span <</ActualText (x)>> BDC " + plain + " EMC":                        false,
		"/Span <</ActualText (x)>> BDC /P BMC " + plain + " EMC EMC":             false,
		"/Span <</ActualText (x)>> BDC /P <</MCID 0>> BDC " + plain + " EMC EMC": false,
		"/Span <</Lang (en)>> BDC " + plain + " EMC":                             true,
		"/P BMC " + plain + " EMC":                                               true,
		"/Span <</ActualText (x)>> BDC /P BMC EMC " + plain + " EMC":             false,
		"/Span <</ActualText (x)>> BDC EMC " + plain:                             true,
		"/Span <</ActualText (x)>> BDC " + plain + " EMC " + plain:               true, // the second has none
		plain + " /Span <</ActualText (x)>> BDC " + plain + " EMC":               true, // the first has none
		"EMC " + plain: true,
	} {
		if got := hasPDFARule(unicodeFontDoc(t, "E000", content).ValidatePDFA(PDFA2A), rule); got != want {
			t.Errorf("%q: reported %v", content, got)
		}
	}
	// /ActualText through a named property list.
	d := unicodeFontDoc(t, "E000", "/Span /P0 BDC "+plain+" EMC")
	page, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	page.pageResources()["/Properties"] = pdfDict{"/P0": pdfDict{"/ActualText": "x"}}
	if hasPDFARule(d.ValidatePDFA(PDFA2A), rule) {
		t.Error("named property list not read")
	}
}

// Conversion writes the letter validators read, for both new levels.
func TestConvertToPDFAUnicodeLevel(t *testing.T) {
	for format, want := range map[PDFAFormat]string{PDFA2U: `pdfaid:conformance`, PDFA3U: `pdfaid:conformance`} {
		d, _ := conformDoc(t)
		rep, err := d.ConvertToPDFA(format)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Conformant {
			t.Errorf("%v: %+v", format, rep.Issues)
		}
		raw, err := d.XMPRaw()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) || !strings.Contains(string(raw), ">U<") && !strings.Contains(string(raw), `conformance="U"`) {
			t.Errorf("%v: %s", format, raw)
		}
		// A file claiming one level does not pass as another.
		if !hasPDFARule(d.ValidatePDFA(PDFA2B), "XMP_CONFORMANCE_MISMATCH") {
			t.Errorf("%v accepted as b", format)
		}
	}
}

func TestAttachInvoiceTakesPDFA3U(t *testing.T) {
	d, _ := conformDoc(t)
	rep, err := d.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017"), InvoiceOptions{Format: PDFA3U})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != PDFA3U || !rep.Conformant {
		t.Errorf("%v %+v", rep.Format, rep.Issues)
	}
	d, _ = conformDoc(t)
	if _, err := d.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017"), InvoiceOptions{Format: PDFA2U}); err == nil {
		t.Error("an e-invoice must be PDF/A-3")
	}
}
