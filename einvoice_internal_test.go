// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

// ciiInvoice returns a small CII invoice with the given guideline ID. Its
// content is a valid Factur-X MINIMUM invoice (the independent XSD check in
// the last task relies on that for the MINIMUM case).
func ciiInvoice(guidelineID string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
  <rsm:ExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>` + guidelineID + `</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:ExchangedDocumentContext>
  <rsm:ExchangedDocument>
    <ram:ID>INV-2026-001</ram:ID>
    <ram:TypeCode>380</ram:TypeCode>
    <ram:IssueDateTime><udt:DateTimeString format="102">20260922</udt:DateTimeString></ram:IssueDateTime>
  </rsm:ExchangedDocument>
  <rsm:SupplyChainTradeTransaction>
    <ram:ApplicableHeaderTradeAgreement>
      <ram:SellerTradeParty>
        <ram:Name>Seller GmbH</ram:Name>
        <ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress>
        <ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration>
      </ram:SellerTradeParty>
      <ram:BuyerTradeParty><ram:Name>Buyer SARL</ram:Name></ram:BuyerTradeParty>
    </ram:ApplicableHeaderTradeAgreement>
    <ram:ApplicableHeaderTradeDelivery/>
    <ram:ApplicableHeaderTradeSettlement>
      <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
        <ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>
        <ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>
        <ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>
        <ram:DuePayableAmount>119.00</ram:DuePayableAmount>
      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
    </ram:ApplicableHeaderTradeSettlement>
  </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>
`)
}

func TestInvoiceProfileFromGuideline(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want InvoiceProfile
	}{
		{"urn:factur-x.eu:1p0:minimum", InvoiceProfileMinimum},
		{"urn:factur-x.eu:1p0:basicwl", InvoiceProfileBasicWL},
		{"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic", InvoiceProfileBasic},
		{"urn:cen.eu:en16931:2017", InvoiceProfileEN16931},
		{"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended", InvoiceProfileExtended},
		{"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", InvoiceProfileXRechnung},
		{"urn:zugferd.de:2p0:minimum", InvoiceProfileMinimum},
		{"urn:zugferd.de:2p0:basicwl", InvoiceProfileBasicWL},
		{"urn:zugferd.de:2p0:basic", InvoiceProfileBasic},
		{"urn:zugferd.de:2p0:en16931", InvoiceProfileEN16931},
		{"urn:zugferd.de:2p0:extended", InvoiceProfileExtended},
		{"urn:ferd:CrossIndustryDocument:invoice:1p0:comfort", InvoiceProfileEN16931},
		{"  urn:factur-x.eu:1p0:minimum \n", InvoiceProfileMinimum},
		{"urn:example:unknown", InvoiceProfileUnknown},
	} {
		if got := invoiceProfileFromGuideline(tc.id); got != tc.want {
			t.Errorf("%q → %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestInvoiceProfileStringsAndLevels(t *testing.T) {
	for p, want := range map[InvoiceProfile]string{
		InvoiceProfileMinimum:   "MINIMUM",
		InvoiceProfileBasicWL:   "BASIC WL",
		InvoiceProfileBasic:     "BASIC",
		InvoiceProfileEN16931:   "EN 16931",
		InvoiceProfileExtended:  "EXTENDED",
		InvoiceProfileXRechnung: "XRECHNUNG",
		InvoiceProfileUnknown:   "UNKNOWN",
	} {
		if p.String() != want {
			t.Errorf("String() = %q, want %q", p.String(), want)
		}
		if p != InvoiceProfileUnknown && invoiceProfileFromLevel(want) != p {
			t.Errorf("invoiceProfileFromLevel(%q) did not round-trip", want)
		}
	}
	if invoiceProfileFromLevel("comfort") != InvoiceProfileEN16931 {
		t.Error("ZUGFeRD 1.0 COMFORT did not map to EN 16931")
	}
	if invoiceProfileFromLevel("en16931") != InvoiceProfileEN16931 {
		t.Error("EN16931 without a space did not map")
	}
	if InvoiceProfileXRechnung.defaultFileName() != "xrechnung.xml" || InvoiceProfileBasic.defaultFileName() != "factur-x.xml" {
		t.Error("default file names are wrong")
	}
	if InvoiceProfileMinimum.relationship() != AFData || InvoiceProfileBasicWL.relationship() != AFData ||
		InvoiceProfileBasic.relationship() != AFAlternative || InvoiceProfileEN16931.relationship() != AFAlternative {
		t.Error("relationships do not follow the Factur-X rule")
	}
}

func TestInvoiceGuideline(t *testing.T) {
	root, id, err := invoiceGuideline(ciiInvoice("urn:cen.eu:en16931:2017"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Local != "CrossIndustryInvoice" || root.Space != nsCII {
		t.Errorf("root = %v, want CII", root)
	}
	if id != "urn:cen.eu:en16931:2017" {
		t.Errorf("guideline = %q", id)
	}

	if _, _, err := invoiceGuideline([]byte("<rsm:CrossIndustryInvoice")); err == nil {
		t.Error("malformed XML accepted")
	}
	root, _, err = invoiceGuideline([]byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if root.Local == "CrossIndustryInvoice" {
		t.Error("a UBL root was reported as CII")
	}
}

func invoiceTestDoc(t *testing.T) *Document {
	t.Helper()
	doc := NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Invoice INV-2026-001", TextStyle{Font: FontHelvetica, Size: 14},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
		t.Fatal(err)
	}
	return doc
}

func saveAndReopen(t *testing.T, doc *Document) *Document {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return back
}

func TestAttachInvoiceRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		urn      string
		profile  InvoiceProfile
		fileName string
		rel      AFRelationship
	}{
		{"urn:factur-x.eu:1p0:minimum", InvoiceProfileMinimum, "factur-x.xml", AFData},
		{"urn:factur-x.eu:1p0:basicwl", InvoiceProfileBasicWL, "factur-x.xml", AFData},
		{"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic", InvoiceProfileBasic, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017", InvoiceProfileEN16931, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended", InvoiceProfileExtended, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", InvoiceProfileXRechnung, "xrechnung.xml", AFAlternative},
	} {
		t.Run(tc.profile.String(), func(t *testing.T) {
			xmlData := ciiInvoice(tc.urn)
			doc := invoiceTestDoc(t)
			report, err := doc.AttachInvoice(xmlData)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Conformant {
				t.Errorf("not PDF/A-3 conformant: %+v", report.Issues)
			}
			back := saveAndReopen(t, doc)
			inv, err := back.Invoice()
			if err != nil || inv == nil {
				t.Fatalf("Invoice() = %v, %v", inv, err)
			}
			if inv.Profile != tc.profile || inv.FileName != tc.fileName || inv.Version != "1.0" {
				t.Errorf("got profile %v, file %q, version %q", inv.Profile, inv.FileName, inv.Version)
			}
			if inv.Standard != "Factur-X 1.0 / ZUGFeRD 2.1+" {
				t.Errorf("Standard = %q", inv.Standard)
			}
			if !bytes.Equal(inv.XML, xmlData) {
				t.Error("the extracted XML differs from what was attached")
			}
			f := back.EmbeddedFiles().Get(tc.fileName)
			if f.AFRelationship() != tc.rel || !back.isAssociatedFile(f.ref) {
				t.Errorf("relationship %v (listed %v), want %v", f.AFRelationship(), back.isAssociatedFile(f.ref), tc.rel)
			}
			if f.MIMEType() != "text/xml" {
				t.Errorf("MIME type = %q, want text/xml", f.MIMEType())
			}
			raw, _ := back.XMPRaw()
			if n := strings.Count(string(raw), "pdfaExtension:schemas"); n != 2 { // open and close tag
				t.Errorf("extension schema appears %d/2 times", n)
			}
			if !strings.Contains(string(raw), nsFacturX) {
				t.Error("the Factur-X namespace is missing from the XMP")
			}
			if back.ValidatePDFA(PDFA3B).Conformant != report.Conformant {
				t.Error("the reopened file validates differently")
			}
		})
	}
}

// A second call replaces the invoice and the metadata instead of adding more.
func TestAttachInvoiceReplaces(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice(ciiInvoice("urn:factur-x.eu:1p0:minimum")); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	if n := doc.EmbeddedFiles().Count(); n != 1 {
		t.Errorf("%d attachments after a second AttachInvoice, want 1", n)
	}
	raw, _ := doc.XMPRaw()
	if n := strings.Count(string(raw), "<pdfaExtension:schemas>"); n != 1 {
		t.Errorf("extension schema block appears %d times, want 1", n)
	}
	inv, err := doc.Invoice()
	if err != nil || inv == nil || inv.Profile != InvoiceProfileEN16931 {
		t.Fatalf("Invoice() after replace = %+v, %v", inv, err)
	}
}

// ConvertToPDFA rewrites the XMP; it must keep extension schemas (ours and
// any another producer declared) and the properties they describe.
func TestConvertToPDFAKeepsExtensionSchemas(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice(ciiInvoice("urn:factur-x.eu:1p0:basicwl")); err != nil {
		t.Fatal(err)
	}
	back := saveAndReopen(t, doc)
	if _, err := back.ConvertToPDFA(PDFA3B); err != nil {
		t.Fatal(err)
	}
	raw, _ := back.XMPRaw()
	s := string(raw)
	if strings.Count(s, "<pdfaExtension:schemas>") != 1 {
		t.Errorf("extension schema lost or duplicated by ConvertToPDFA:\n%s", s)
	}
	if strings.Contains(s, "pdfaSchema:schema=") || strings.Contains(s, "<pdfaSchema:schema>Factur-X PDFA Extension Schema</pdfaSchema:schema>\n<pdfaSchema:schema>") {
		t.Error("the extension schema's fields leaked out as top-level properties")
	}
	inv, err := back.Invoice()
	if err != nil || inv == nil || inv.Profile != InvoiceProfileBasicWL {
		t.Fatalf("Invoice() after ConvertToPDFA = %+v, %v", inv, err)
	}
}

func TestAttachInvoiceErrors(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice([]byte("<rsm:CrossIndustryInvoice")); err == nil {
		t.Error("malformed XML accepted")
	}
	if _, err := doc.AttachInvoice([]byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`)); err == nil {
		t.Error("a UBL invoice accepted as CII")
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:example:unknown")); err == nil {
		t.Error("an unknown profile accepted without an explicit one")
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:example:unknown"), InvoiceOptions{Profile: InvoiceProfileEN16931}); err != nil {
		t.Errorf("an explicit profile was not honoured: %v", err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017"), InvoiceOptions{Format: PDFA2B}); err == nil {
		t.Error("a non-PDF/A-3 format accepted")
	}
}

func TestInvoiceNotAnInvoice(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.EmbeddedFiles().AddFromStream("notes.txt", strings.NewReader("hi")); err != nil {
		t.Fatal(err)
	}
	inv, err := doc.Invoice()
	if err != nil || inv != nil {
		t.Errorf("Invoice() on a plain document = %+v, %v; want nil, nil", inv, err)
	}
}

// Documents from the earlier generations, built by hand the way their
// producers wrote them.
func TestInvoiceReadsOlderGenerations(t *testing.T) {
	t.Run("ZUGFeRD 2.0", func(t *testing.T) {
		doc := invoiceTestDoc(t)
		if _, err := doc.EmbeddedFiles().AddFromStream("zugferd-invoice.xml",
			bytes.NewReader(ciiInvoice("urn:zugferd.de:2p0:en16931"))); err != nil {
			t.Fatal(err)
		}
		if err := doc.SetXMP(XMPMetadata{Custom: []XMPProperty{
			{Namespace: nsZUGFeRD2, Prefix: "fx", Name: "ConformanceLevel", Value: "EN 16931"},
			{Namespace: nsZUGFeRD2, Prefix: "fx", Name: "Version", Value: "2p0"},
		}}); err != nil {
			t.Fatal(err)
		}
		inv, err := saveAndReopen(t, doc).Invoice()
		if err != nil || inv == nil {
			t.Fatalf("Invoice() = %v, %v", inv, err)
		}
		if inv.Standard != "ZUGFeRD 2.0" || inv.Profile != InvoiceProfileEN16931 || inv.FileName != "zugferd-invoice.xml" {
			t.Errorf("got %+v", inv)
		}
	})
	t.Run("ZUGFeRD 1.0", func(t *testing.T) {
		zf1 := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryDocument xmlns:rsm="urn:ferd:CrossIndustryDocument:invoice:1p0" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:12">
  <rsm:SpecifiedExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>urn:ferd:CrossIndustryDocument:invoice:1p0:comfort</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:SpecifiedExchangedDocumentContext>
</rsm:CrossIndustryDocument>
`)
		doc := invoiceTestDoc(t)
		if _, err := doc.EmbeddedFiles().AddFromStream("ZUGFeRD-invoice.xml", bytes.NewReader(zf1)); err != nil {
			t.Fatal(err)
		}
		inv, err := saveAndReopen(t, doc).Invoice()
		if err != nil || inv == nil {
			t.Fatalf("Invoice() = %v, %v", inv, err)
		}
		if inv.Standard != "ZUGFeRD 1.0" || inv.Profile != InvoiceProfileEN16931 {
			t.Errorf("got standard %q profile %v", inv.Standard, inv.Profile)
		}
	})
}

// --- Fix round 1 ---

// Finding 1: a document that already carries a foreign custom XMP namespace
// (xmpMM:DocumentID, the way Word/Acrobat write it) must not have its
// namespace stolen by the Factur-X properties AttachInvoice adds — each
// namespace round-trips (through AttachInvoice's own XMP() re-read and
// ConvertToPDFA's setPDFAMetadata XMP() re-read) under its own prefix.
func TestAttachInvoicePreservesForeignNamespace(t *testing.T) {
	doc := invoiceTestDoc(t)
	if err := doc.SetXMP(XMPMetadata{Custom: []XMPProperty{
		{Namespace: "http://ns.adobe.com/xap/1.0/mm/", Prefix: "xmpMM", Name: "DocumentID", Value: "uuid:1234"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	back := saveAndReopen(t, doc)
	raw, err := back.XMPRaw()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `xmlns:fx="`+nsFacturX+`"`) {
		t.Errorf("fx: is not bound to the Factur-X namespace:\n%s", s)
	}
	if !strings.Contains(s, "<fx:DocumentType>") && !strings.Contains(s, "fx:DocumentType=") {
		t.Errorf("Factur-X properties were not serialised under fx:\n%s", s)
	}

	meta, err := back.XMP()
	if err != nil {
		t.Fatal(err)
	}
	var sawMM bool
	for _, p := range meta.Custom {
		if p.Namespace == "http://ns.adobe.com/xap/1.0/mm/" && p.Name == "DocumentID" {
			sawMM = true
			if p.Value != "uuid:1234" {
				t.Errorf("xmpMM:DocumentID value = %q, want %q", p.Value, "uuid:1234")
			}
		}
	}
	if !sawMM {
		t.Errorf("xmpMM:DocumentID was lost or merged into another namespace; custom = %+v", meta.Custom)
	}

	inv, err := back.Invoice()
	if err != nil || inv == nil {
		t.Fatalf("Invoice() = %v, %v", inv, err)
	}
	if inv.Version != "1.0" {
		t.Errorf("Invoice().Version = %q, want %q (fx:Version must not have landed in the xmpMM namespace)", inv.Version, "1.0")
	}
}

// nestedForeignExtensionXMP is a hand-written packet like a real producer
// might emit: a Ghostscript-style self-closing rdf:Description carrying the
// pdfaid identification, followed by a PDF/A extension-schema block whose
// bag entries use nested rdf:Description elements (rather than this
// library's own rdf:li[rdf:parseType=Resource] style) to hold the schema and
// property records.
const nestedForeignExtensionXMP = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/" pdfaid:part="3" pdfaid:conformance="B"/>
<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li>
<rdf:Description>
<pdfaSchema:schema>Foreign Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:foreign#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>fgn</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li>
<rdf:Description>
<pdfaProperty:name>Custom</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>A foreign property</pdfaProperty:description>
</rdf:Description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:Description>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

// Finding 2, part 1: xmpExtensionBlocks must extract the foreign extension
// block whole — including its nested rdf:Description entries — and must not
// swallow the preceding self-closing rdf:Description into it.
func TestXMPExtensionBlocksNestedDescription(t *testing.T) {
	blocks, rest := xmpExtensionBlocks(nestedForeignExtensionXMP)
	if len(blocks) != 1 {
		t.Fatalf("got %d extension blocks, want 1: %v", len(blocks), blocks)
	}
	block := blocks[0]
	if n, m := strings.Count(block, "<rdf:Description"), strings.Count(block, "</rdf:Description>"); n != m {
		t.Errorf("extracted block is not balanced (%d opens, %d closes):\n%s", n, m, block)
	}
	if !strings.Contains(block, "Foreign Schema") || !strings.Contains(block, "urn:example:foreign#") ||
		!strings.Contains(block, "A foreign property") {
		t.Errorf("extension block is truncated:\n%s", block)
	}
	if strings.Contains(rest, "Foreign Schema") {
		t.Error("extension block content leaked into rest")
	}
	if !strings.Contains(rest, `pdfaid:part="3"`) {
		t.Error("the preceding self-closing rdf:Description was swallowed into the extension block")
	}
}

// Finding 2, part 2: ConvertToPDFA must carry the foreign extension block
// through as well-formed XML, without truncating it.
func TestConvertToPDFAKeepsNestedForeignExtensionSchema(t *testing.T) {
	doc := invoiceTestDoc(t)
	if err := doc.SetXMPRaw([]byte(nestedForeignExtensionXMP)); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.ConvertToPDFA(PDFA3B); err != nil {
		t.Fatal(err)
	}
	raw, err := doc.XMPRaw()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if n := strings.Count(s, "Foreign Schema"); n != 1 {
		t.Errorf("foreign extension schema appears %d times, want 1:\n%s", n, s)
	}

	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("resulting XMP packet is not well-formed XML: %v\n%s", err, s)
		}
	}
	if _, err := doc.XMP(); err != nil {
		t.Fatalf("XMP() failed on the converted packet: %v", err)
	}
}
