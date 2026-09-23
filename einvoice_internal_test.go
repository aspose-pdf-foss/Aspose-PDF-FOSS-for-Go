// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
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

// --- Fix round 2 ---

// Ruled addition B: a kept foreign extension schema declares its own
// namespace's prefix (pdfaSchema:namespaceURI + pdfaSchema:prefix); the
// property in that namespace must be re-serialised under that declared
// prefix — not whatever generic prefix bindCustomPrefixes would otherwise
// pick — because a PDF/A validator compares the two.
func TestConvertToPDFAUsesForeignExtensionSchemaPrefix(t *testing.T) {
	doc := invoiceTestDoc(t)
	packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:acme="urn:example:acme#">
<acme:Code>ABC123</acme:Code>
</rdf:Description>
<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Acme Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Code</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>An ACME code</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
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
	if !strings.Contains(s, `xmlns:acme="urn:example:acme#"`) {
		t.Errorf("the foreign schema's own declared prefix was not used for its namespace:\n%s", s)
	}
	if !strings.Contains(s, "<acme:Code>ABC123</acme:Code>") {
		t.Errorf("acme:Code was not serialised under the schema's declared prefix:\n%s", s)
	}
}

// --- Final review fixes ---

// checkXMPWellFormed parses an XMP packet with encoding/xml and additionally
// checks what that decoder tolerates but a strict XML parser rejects: a
// duplicate attribute on one element, a namespace declaration whose prefix
// is not an NCName, and a declaration of the reserved xml / xmlns prefixes.
func checkXMPWellFormed(t *testing.T, raw []byte) {
	t.Helper()
	s := string(raw)
	if strings.Contains(s, "xmlns:xml=") || strings.Contains(s, "xmlns:xmlns=") {
		t.Errorf("packet declares a reserved xml/xmlns prefix:\n%s", s)
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			t.Fatalf("packet is not well-formed XML: %v\n%s", err, s)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		seen := map[xml.Name]bool{}
		for _, a := range se.Attr {
			if seen[a.Name] {
				t.Errorf("duplicate attribute %s:%s on <%s>:\n%s", a.Name.Space, a.Name.Local, se.Name.Local, s)
			}
			seen[a.Name] = true
			if a.Name.Space == "xmlns" && !testIsNCName(a.Name.Local) {
				t.Errorf("namespace prefix %q is not an NCName:\n%s", a.Name.Local, s)
			}
		}
	}
}

// testIsNCName is an independent, ASCII-strict NCName check for the tests.
func testIsNCName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
		case i > 0 && (r == '-' || r == '.' || (r >= '0' && r <= '9')):
		default:
			return false
		}
	}
	return true
}

// countXMPProperty counts the elements and attributes named space/local in a
// packet (namespace declarations excluded).
func countXMPProperty(t *testing.T, raw []byte, space, local string) int {
	t.Helper()
	n := 0
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return n
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space == space && se.Name.Local == local {
			n++
		}
		for _, a := range se.Attr {
			if a.Name.Space == space && a.Name.Local == local {
				n++
			}
		}
	}
}

// foreignPrefixXMP is a packet carrying a foreign property and the extension
// schema describing it, the schema declaring the given prefix (XML-escaped
// into the element text) for its namespace.
func foreignPrefixXMP(prefix string) string {
	return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:fgn="urn:example:foreign#">
<fgn:Code>F-42</fgn:Code>
</rdf:Description>
<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Foreign Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:foreign#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>` + xmlEscape(prefix) + `</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Code</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>A foreign code</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
}

// Important 1: a declared prefix is honoured only when it is a usable XML
// prefix, and never takes pdfaid or (when Factur-X is written) fx.
func TestForeignExtensionSchemaPrefixValidated(t *testing.T) {
	for _, prefix := range []string{`a"b`, "a b", "1bad", "xml", "xmlns", "XmlFoo", "pdfaid", "fx"} {
		t.Run(prefix, func(t *testing.T) {
			doc := invoiceTestDoc(t)
			if err := doc.SetXMPRaw([]byte(foreignPrefixXMP(prefix))); err != nil {
				t.Fatal(err)
			}
			invoice := prefix == "fx"
			if invoice {
				if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
					t.Fatal(err)
				}
			} else if _, err := doc.ConvertToPDFA(PDFA3B); err != nil {
				t.Fatal(err)
			}
			back := saveAndReopen(t, doc)
			raw, err := back.XMPRaw()
			if err != nil {
				t.Fatal(err)
			}
			checkXMPWellFormed(t, raw)
			s := string(raw)
			if !strings.Contains(s, `xmlns:pdfaid="`+nsPDFAID+`"`) || !strings.Contains(s, "<pdfaid:part>3</pdfaid:part>") {
				t.Errorf("pdfaid is not written under the pdfaid prefix:\n%s", s)
			}
			if invoice && (!strings.Contains(s, `xmlns:fx="`+nsFacturX+`"`) || !strings.Contains(s, "<fx:DocumentType>")) {
				t.Errorf("Factur-X is not written under the fx prefix:\n%s", s)
			}
			if n := countXMPProperty(t, raw, "urn:example:foreign#", "Code"); n != 1 {
				t.Errorf("foreign property appears %d times, want 1:\n%s", n, s)
			}
			if r := back.ValidatePDFA(PDFA3B); hasRule(r, "XMP_PDFAID_MISSING") || hasRule(r, "XMP_MALFORMED") {
				t.Errorf("reopened file: %+v", r.Issues)
			}
		})
	}
}

// Important 1, defence in depth: SetXMP itself refuses an unusable prefix.
func TestSetXMPRefusesInvalidPrefix(t *testing.T) {
	for _, prefix := range []string{`a"b`, "a b", "1bad", "xml", "xmlns", "", "pdfaid"} {
		doc := NewDocument(100, 100)
		if err := doc.SetXMP(XMPMetadata{Custom: []XMPProperty{
			{Namespace: "urn:example:x#", Prefix: prefix, Name: "P", Value: "v"},
		}}); err != nil {
			t.Fatal(err)
		}
		raw, _ := doc.XMPRaw()
		checkXMPWellFormed(t, raw)
		if n := countXMPProperty(t, raw, "urn:example:x#", "P"); n != 1 {
			t.Errorf("prefix %q: property appears %d times, want 1:\n%s", prefix, n, raw)
		}
	}
}

// Important 2: an extension schema sharing an rdf:Description with other
// properties is carried alone; the siblings are not duplicated.
func TestConvertToPDFAExtractsSchemasFromSharedDescription(t *testing.T) {
	packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:acme="urn:example:acme#" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#" acme:Mode="fast">
<pdfaid:part>2</pdfaid:part>
<pdfaid:conformance>B</pdfaid:conformance>
<dc:title><rdf:Alt><rdf:li xml:lang="x-default">Shared</rdf:li></rdf:Alt></dc:title>
<acme:Code>ABC123</acme:Code>
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Acme Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Code</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>An ACME code</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
	doc := invoiceTestDoc(t)
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.ConvertToPDFA(PDFA3B); err != nil {
		t.Fatal(err)
	}
	raw, _ := doc.XMPRaw()
	checkXMPWellFormed(t, raw)
	s := string(raw)
	for _, c := range []struct {
		space, local string
	}{
		{nsPDFAID, "part"}, {nsPDFAID, "conformance"}, {nsDC, "title"},
		{"urn:example:acme#", "Code"}, {"urn:example:acme#", "Mode"},
		{"http://www.aiim.org/pdfa/ns/extension/", "schemas"},
	} {
		if n := countXMPProperty(t, raw, c.space, c.local); n != 1 {
			t.Errorf("%s%s appears %d times, want 1:\n%s", c.space, c.local, n, s)
		}
	}
	if !strings.Contains(s, "<pdfaid:part>3</pdfaid:part>") {
		t.Errorf("pdfaid:part is not 3:\n%s", s)
	}
	if !strings.Contains(s, "Acme Extension Schema") {
		t.Errorf("the extension schema was lost:\n%s", s)
	}
}

// Important 4: an older invoice generation's schema entry is replaced even
// when it shares a bag with another producer's schema, which is kept.
func TestAttachInvoiceReplacesOlderSchemaEntry(t *testing.T) {
	packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:zf2="urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#" xmlns:acme="urn:example:acme#">
<zf2:DocumentType>INVOICE</zf2:DocumentType>
<zf2:ConformanceLevel>EN 16931</zf2:ConformanceLevel>
<acme:Code>ABC123</acme:Code>
</rdf:Description>
<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>ZUGFeRD PDFA Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>fx</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>DocumentType</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>INVOICE</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Acme Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Code</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>An ACME code</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
	doc := invoiceTestDoc(t)
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	raw, _ := doc.XMPRaw()
	checkXMPWellFormed(t, raw)
	s := string(raw)
	if n := strings.Count(s, "<pdfaSchema:namespaceURI>"+nsFacturX+"</pdfaSchema:namespaceURI>"); n != 1 {
		t.Errorf("Factur-X schema entry appears %d times, want 1:\n%s", n, s)
	}
	if strings.Contains(s, "<pdfaSchema:namespaceURI>"+nsZUGFeRD2) {
		t.Errorf("the ZUGFeRD 2.0 schema entry was kept:\n%s", s)
	}
	if n := strings.Count(s, "Acme Extension Schema"); n != 1 {
		t.Errorf("the acme schema entry appears %d times, want 1:\n%s", n, s)
	}
	if n := countXMPProperty(t, raw, "urn:example:acme#", "Code"); n != 1 {
		t.Errorf("acme:Code appears %d times, want 1:\n%s", n, s)
	}
	if strings.Contains(s, nsZUGFeRD2) {
		t.Errorf("ZUGFeRD 2.0 properties survived:\n%s", s)
	}
	if !strings.Contains(s, `xmlns:acme="urn:example:acme#"`) {
		t.Errorf("acme lost its declared prefix:\n%s", s)
	}
}

// Minor 3: a packet that is not well-formed XML is reported.
func TestValidatePDFAReportsMalformedXMP(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	if hasRule(doc.ValidatePDFA(PDFA2B), "XMP_MALFORMED") {
		t.Fatal("a well-formed packet was reported as malformed")
	}
	raw, _ := doc.XMPRaw()
	broken := strings.Replace(string(raw), "</rdf:Description>", "</rdf:Descriptio>", 1)
	if err := doc.SetXMPRaw([]byte(broken)); err != nil {
		t.Fatal(err)
	}
	if !hasRule(doc.ValidatePDFA(PDFA2B), "XMP_MALFORMED") {
		t.Error("a malformed packet was not reported")
	}
}

// A foreign extension schema is kept when it merely names an invoice
// namespace in prose, and when its bag uses a container shape this scanner
// does not recognise — losing it would leave that producer's properties
// undeclared, which is the violation the block exists to prevent.
func TestDropInvoiceSchemaEntriesKeepsForeignSchemas(t *testing.T) {
	mentions := `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas><rdf:Bag><rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Acme Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
<pdfaSchema:property><rdf:Seq><rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Code</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>Mirrors ` + nsFacturX + ` semantics</pdfaProperty:description>
</rdf:li></rdf:Seq></pdfaSchema:property>
</rdf:li></rdf:Bag></pdfaExtension:schemas></rdf:Description>`
	unrecognised := `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#">
<pdfaExtension:schemas><rdf:Bag><rdf:_1 rdf:parseType="Resource">
<pdfaSchema:schema>Acme Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
</rdf:_1></rdf:Bag></pdfaExtension:schemas></rdf:Description>`
	for name, block := range map[string]string{"mentions-facturx": mentions, "unrecognised-container": unrecognised} {
		t.Run(name, func(t *testing.T) {
			out, ok := dropInvoiceSchemaEntries(block)
			if !ok {
				t.Fatalf("the block was dropped entirely:\n%s", block)
			}
			if !strings.Contains(out, "Acme Extension Schema") {
				t.Errorf("the acme schema entry was removed:\n%s", out)
			}
		})
	}
	// An invoice entry is still recognised through its declared namespace.
	if _, ok := dropInvoiceSchemaEntries(facturXExtensionSchema); ok {
		t.Error("the Factur-X block was kept")
	}
}

// A schema block declaring its prefixes on an ancestor still resolves every
// schema it names: the block is parsed with those bindings supplied.
func TestExtensionSchemaPrefixesWithInheritedDeclarations(t *testing.T) {
	block := `<rdf:Description rdf:about="">
<pdfaExtension:schemas><rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Acme</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:acme#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>acme</pdfaSchema:prefix>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Beta</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:example:beta#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>beta</pdfaSchema:prefix>
</rdf:li>
</rdf:Bag></pdfaExtension:schemas></rdf:Description>`
	got := extensionSchemaPrefixes([]string{block})
	for ns, want := range map[string]string{"urn:example:acme#": "acme", "urn:example:beta#": "beta"} {
		if got[ns] != want {
			t.Errorf("%s resolved to %q, want %q (all: %v)", ns, got[ns], want, got)
		}
	}
}
