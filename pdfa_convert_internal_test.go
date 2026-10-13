// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestConvertToPDFAStandard14SubstituteIsNonsymbolic pins GitHub issue #12:
// embedStandardFonts substitutes a Standard-14 font with a bundled
// metric-compatible TrueType clone and always equips it with /Encoding
// (buildEmbeddedSimpleFont preserves the original encoding, or defaults to
// WinAnsiEncoding) -- a simple TrueType font with /Encoding must be flagged
// Nonsymbolic, not Symbolic (ISO 32000-1 §9.6.6.2), or PDF/A validators that
// actually check this rule (veraPDF 6.2.11.4.1-2 / 6.2.11.6-3/-4) reject the
// file even though ValidatePDFA reports it conformant (ValidatePDFA does not
// check this particular rule, by design -- see the issue).
func TestConvertToPDFAStandard14SubstituteIsNonsymbolic(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Hello", TextStyle{Font: FontHelvetica, Size: 14},
		Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}

	rep, err := doc.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("not conformant: %+v", rep.Issues)
	}

	fontDict, desc := findEmbeddedSimpleFont(t, doc)
	if _, ok := fontDict["/Encoding"]; !ok {
		t.Fatal("precondition failed: substitute font has no /Encoding (test no longer exercises the bug)")
	}
	flags, _ := desc["/Flags"].(int)
	if flags&0x20 == 0 {
		t.Errorf("/Flags = %d, Nonsymbolic bit (0x20) not set on a Standard-14 substitute carrying /Encoding", flags)
	}
	if flags&0x4 != 0 {
		t.Errorf("/Flags = %d, Symbolic bit (0x4) set alongside /Encoding -- PDF/A violation (veraPDF 6.2.11.4.1-2)", flags)
	}
}

// findEmbeddedSimpleFont locates the one embedded simple (non-Type0) font's
// dict and its resolved /FontDescriptor in doc.objects.
func findEmbeddedSimpleFont(t *testing.T, doc *Document) (pdfDict, pdfDict) {
	t.Helper()
	for _, obj := range doc.objects {
		dict, ok := obj.Value.(pdfDict)
		if !ok || dictGetName(dict, "/Type") != "/Font" || dictGetName(dict, "/Subtype") != "/TrueType" {
			continue
		}
		desc, ok := resolveRefToDict(doc.objects, dict["/FontDescriptor"])
		if !ok {
			continue
		}
		return dict, desc
	}
	t.Fatal("no embedded simple TrueType font found")
	return nil, nil
}
