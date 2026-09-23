// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

func catalogAFNums(d *Document) []int {
	var nums []int
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok {
			nums = append(nums, r.Num)
		}
	}
	return nums
}

func TestAFRelationshipSetAndList(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("data.csv", strings.NewReader("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.AFRelationship(); got != AFUnspecified {
		t.Errorf("new attachment relationship = %v, want AFUnspecified", got)
	}
	if len(catalogAFNums(doc)) != 0 {
		t.Error("an attachment with no relationship was listed in /AF")
	}

	f.SetAFRelationship(AFData)
	f = doc.EmbeddedFiles().Get("data.csv")
	if got := f.AFRelationship(); got != AFData {
		t.Errorf("relationship = %v, want AFData", got)
	}
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != f.ref.Num {
		t.Fatalf("/AF = %v, want exactly the file specification %d", n, f.ref.Num)
	}

	// Changing the relationship must not list the file twice.
	f.SetAFRelationship(AFSource)
	if n := catalogAFNums(doc); len(n) != 1 {
		t.Errorf("/AF has %d entries after a second SetAFRelationship, want 1", len(n))
	}
	if name, _ := f.filespec["/AFRelationship"].(pdfName); name != "/Source" {
		t.Errorf("/AFRelationship = %v, want /Source", f.filespec["/AFRelationship"])
	}
}

func TestAFRelationshipRoundTrip(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("source.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	f.SetAFRelationship(AFAlternative)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	g := back.EmbeddedFiles().Get("source.txt")
	if g == nil {
		t.Fatal("attachment lost in the round trip")
	}
	if got := g.AFRelationship(); got != AFAlternative {
		t.Errorf("relationship after round trip = %v, want AFAlternative", got)
	}
	if !back.isAssociatedFile(g.ref) {
		t.Error("the attachment is not listed in /AF after the round trip")
	}
}

func TestRemovingAttachmentRemovesItFromAF(t *testing.T) {
	doc := NewDocument(200, 200)
	a, _ := doc.EmbeddedFiles().AddFromStream("a.txt", strings.NewReader("a"))
	b, _ := doc.EmbeddedFiles().AddFromStream("b.txt", strings.NewReader("b"))
	a.SetAFRelationship(AFData)
	b.SetAFRelationship(AFData)
	doc.EmbeddedFiles().Remove("a.txt")
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != b.ref.Num {
		t.Errorf("/AF after removing a.txt = %v, want only b.txt's %d", n, b.ref.Num)
	}
	doc.EmbeddedFiles().Clear()
	if _, ok := doc.catalog["/AF"]; ok {
		t.Error("/AF survived Clear")
	}
}

// A file specification parsed from an existing PDF can sit directly in the
// /Names/EmbeddedFiles array instead of through an indirect reference; two
// handles onto the same direct dict both see ref.Num == 0 until one of them
// promotes it. SetAFRelationship must not let the second handle promote
// again and list the file twice in /AF.
func TestSetAFRelationshipDoesNotDuplicateDirectFilespec(t *testing.T) {
	doc := NewDocument(200, 200)
	data := []byte("x")
	embedded := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/EmbeddedFile"),
			"/Subtype": pdfName("/PlainText"),
			"/Params":  pdfDict{"/Size": len(data)},
			"/Length":  len(data),
		},
		Data:    data,
		Decoded: false,
	}
	embedID := doc.nextID
	doc.nextID++
	doc.objects[embedID] = &pdfObject{Num: embedID, Value: embedded}

	filespec := pdfDict{
		"/Type": pdfName("/Filespec"),
		"/F":    "x.txt",
		"/UF":   "x.txt",
		"/EF": pdfDict{
			"/F":  pdfRef{Num: embedID},
			"/UF": pdfRef{Num: embedID},
		},
	}
	doc.namesDict()["/EmbeddedFiles"] = pdfDict{"/Names": pdfArray{"x.txt", filespec}}

	a := doc.EmbeddedFiles().Get("x.txt")
	b := doc.EmbeddedFiles().Get("x.txt")
	if a.ref.Num != 0 || b.ref.Num != 0 {
		t.Fatalf("test setup: want both handles to see a direct filespec (ref.Num == 0), got a=%d b=%d", a.ref.Num, b.ref.Num)
	}

	a.SetAFRelationship(AFData)
	b.SetAFRelationship(AFSource)

	n := catalogAFNums(doc)
	if len(n) != 1 {
		t.Fatalf("/AF = %v, want exactly one entry after two SetAFRelationship calls on the same direct filespec", n)
	}

	got := doc.EmbeddedFiles().Get("x.txt")
	if got.ref.Num == 0 {
		t.Fatal("name tree still points at a direct dictionary after promotion")
	}
	if got.ref.Num != n[0] {
		t.Errorf("/AF entry %d does not match the name tree's object %d", n[0], got.ref.Num)
	}
	if rel := got.AFRelationship(); rel != AFSource {
		t.Errorf("relationship = %v, want AFSource", rel)
	}
}

// PDF/A-3 requires /ModDate in an embedded file's parameters.
func TestEmbeddedFileHasModDate(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("x.txt", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := f.stream().Dict["/Params"].(pdfDict)
	md, _ := params["/ModDate"].(string)
	if !strings.HasPrefix(md, "D:") {
		t.Errorf("/Params /ModDate = %q, want a PDF date", md)
	}
}

func hasRule(r *PDFAValidationReport, rule string) bool {
	for _, is := range r.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

func TestPDFA3RequiresAssociatedFiles(t *testing.T) {
	doc := NewDocument(200, 200)
	if _, err := doc.EmbeddedFiles().AddFromStream("data.csv", strings.NewReader("1,2")); err != nil {
		t.Fatal(err)
	}
	if !hasRule(doc.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("PDF/A-3 accepted an attachment with no /AFRelationship")
	}
	if hasRule(doc.ValidatePDFA(PDFA2B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("the PDF/A-3 rule fired for PDF/A-2")
	}
}

func TestConvertToPDFA3AssociatesAttachments(t *testing.T) {
	doc := NewDocument(200, 200)
	if _, err := doc.EmbeddedFiles().AddFromStream("plain.csv", strings.NewReader("1,2")); err != nil {
		t.Fatal(err)
	}
	kept, _ := doc.EmbeddedFiles().AddFromStream("kept.txt", strings.NewReader("k"))
	kept.SetAFRelationship(AFSource)

	report, err := doc.ConvertToPDFA(PDFA3B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(report, "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Errorf("conversion left an unassociated attachment: %+v", report.Issues)
	}
	plain := doc.EmbeddedFiles().Get("plain.csv")
	if !plain.hasAFRelationship() || plain.AFRelationship() != AFUnspecified {
		t.Errorf("plain.csv relationship = %v (explicit %v), want an explicit Unspecified",
			plain.AFRelationship(), plain.hasAFRelationship())
	}
	if got := doc.EmbeddedFiles().Get("kept.txt").AFRelationship(); got != AFSource {
		t.Errorf("conversion changed an existing relationship to %v", got)
	}
	for _, f := range doc.EmbeddedFiles().All() {
		if !doc.isAssociatedFile(f.ref) {
			t.Errorf("%s is not listed in /AF after conversion", f.Name())
		}
	}
}

// Final review, Important 3: converting an e-invoice to PDF/A-1 removes the
// attachment completely — name tree, catalog /AF and the file itself.
func TestConvertToPDFA1RemovesAssociatedFiles(t *testing.T) {
	for _, format := range []PDFAFormat{PDFA1B, PDFA1A} {
		t.Run(format.String(), func(t *testing.T) {
			doc := invoiceTestDoc(t)
			if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
				t.Fatal(err)
			}
			if _, err := doc.ConvertToPDFA(format); err != nil {
				t.Fatal(err)
			}
			if _, ok := doc.catalog["/AF"]; ok {
				t.Error("catalog /AF survived the conversion")
			}
			for _, obj := range doc.objects {
				if st, ok := obj.Value.(*pdfStream); ok && dictGetName(st.Dict, "/Type") == "/EmbeddedFile" {
					if bytes.Contains(st.Data, []byte("CrossIndustryInvoice")) {
						t.Errorf("the invoice XML is still in object %d", obj.Num)
					}
				}
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			back, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := back.catalog["/AF"]; ok {
				t.Error("the saved file has a catalog /AF")
			}
			if n := back.EmbeddedFiles().Count(); n != 0 {
				t.Errorf("the saved file has %d attachments", n)
			}
			for _, obj := range back.objects {
				var d pdfDict
				switch v := obj.Value.(type) {
				case pdfDict:
					d = v
				case *pdfStream:
					d = v.Dict
				}
				if typ := dictGetName(d, "/Type"); typ == "/EmbeddedFile" || typ == "/Filespec" {
					t.Errorf("object %d (%s) survived in the saved file", obj.Num, typ)
				}
			}
			if r := back.ValidatePDFA(format); hasRule(r, "EMBEDDED_FILES") {
				t.Errorf("reopened file still reports EMBEDDED_FILES: %+v", r.Issues)
			}
		})
	}
}

// The EMBEDDED_FILES rule sees a catalog /AF, and applies to PDF/A-1a too.
func TestPDFA1FlagsAssociatedFiles(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	if names, ok := resolveRefToDict(doc.objects, doc.catalog["/Names"]); ok {
		delete(names, "/EmbeddedFiles") // only /AF is left
	}
	for _, format := range []PDFAFormat{PDFA1B, PDFA1A} {
		if !hasRule(doc.ValidatePDFA(format), "EMBEDDED_FILES") {
			t.Errorf("%v: a catalog /AF was not reported", format)
		}
	}
}
