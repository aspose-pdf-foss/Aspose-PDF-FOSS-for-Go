// SPDX-License-Identifier: MIT

package asposepdf

import (
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
	_ = strings.TrimSpace
}
