// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Hybrid e-invoices (ZUGFeRD 2.x / Factur-X 1.0): a PDF/A-3 document with the
// invoice's UN/CEFACT Cross Industry Invoice (CII) XML embedded as an
// associated file and described in the XMP metadata. The caller supplies the
// XML; the library packages it and extracts it again.

// nsCII is the namespace of the CII root element.
const nsCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"

// InvoiceProfile is the Factur-X / ZUGFeRD conformance level of an invoice.
type InvoiceProfile int

const (
	InvoiceProfileUnknown   InvoiceProfile = iota
	InvoiceProfileMinimum                  // MINIMUM
	InvoiceProfileBasicWL                  // BASIC WL (without lines)
	InvoiceProfileBasic                    // BASIC
	InvoiceProfileEN16931                  // EN 16931 (ZUGFeRD 1.0 COMFORT)
	InvoiceProfileExtended                 // EXTENDED
	InvoiceProfileXRechnung                // XRECHNUNG
)

var invoiceProfileNames = [...]string{
	InvoiceProfileUnknown:   "UNKNOWN",
	InvoiceProfileMinimum:   "MINIMUM",
	InvoiceProfileBasicWL:   "BASIC WL",
	InvoiceProfileBasic:     "BASIC",
	InvoiceProfileEN16931:   "EN 16931",
	InvoiceProfileExtended:  "EXTENDED",
	InvoiceProfileXRechnung: "XRECHNUNG",
}

// String gives the Factur-X spelling, as written to fx:ConformanceLevel.
func (p InvoiceProfile) String() string {
	if p >= 0 && int(p) < len(invoiceProfileNames) {
		return invoiceProfileNames[p]
	}
	return invoiceProfileNames[InvoiceProfileUnknown]
}

// defaultFileName is the attachment name the standards prescribe.
func (p InvoiceProfile) defaultFileName() string {
	if p == InvoiceProfileXRechnung {
		return "xrechnung.xml"
	}
	return "factur-x.xml"
}

// relationship follows the Factur-X rule: the lower profiles are not a full
// representation of the invoice, so their XML is Data; from BASIC up it is an
// Alternative to the visible document.
func (p InvoiceProfile) relationship() AFRelationship {
	if p == InvoiceProfileMinimum || p == InvoiceProfileBasicWL {
		return AFData
	}
	return AFAlternative
}

// invoiceProfileFromGuideline maps a GuidelineSpecifiedDocumentContextParameter
// ID — Factur-X, ZUGFeRD 2.0 and ZUGFeRD 1.0 forms — to a profile.
func invoiceProfileFromGuideline(id string) InvoiceProfile {
	id = strings.ToLower(strings.TrimSpace(id))
	switch {
	case strings.Contains(id, "xrechnung"):
		return InvoiceProfileXRechnung
	case strings.HasSuffix(id, ":minimum"):
		return InvoiceProfileMinimum
	case strings.HasSuffix(id, ":basicwl"):
		return InvoiceProfileBasicWL
	case strings.HasSuffix(id, ":extended"):
		return InvoiceProfileExtended
	case strings.HasSuffix(id, ":basic"):
		return InvoiceProfileBasic
	case strings.HasSuffix(id, ":en16931"), strings.HasSuffix(id, ":comfort"), id == "urn:cen.eu:en16931:2017":
		return InvoiceProfileEN16931
	}
	return InvoiceProfileUnknown
}

// invoiceProfileFromLevel maps an XMP ConformanceLevel value to a profile.
func invoiceProfileFromLevel(level string) InvoiceProfile {
	switch strings.ToUpper(strings.Join(strings.Fields(level), " ")) {
	case "MINIMUM":
		return InvoiceProfileMinimum
	case "BASIC WL", "BASICWL":
		return InvoiceProfileBasicWL
	case "BASIC":
		return InvoiceProfileBasic
	case "EN 16931", "EN16931", "COMFORT":
		return InvoiceProfileEN16931
	case "EXTENDED":
		return InvoiceProfileExtended
	case "XRECHNUNG":
		return InvoiceProfileXRechnung
	}
	return InvoiceProfileUnknown
}

// invoiceGuideline reads the invoice XML only as far as it must: the root
// element's name, and the text of the ID inside
// GuidelineSpecifiedDocumentContextParameter ("" when there is none). A
// malformed document is an error.
func invoiceGuideline(data []byte) (root xml.Name, guidelineID string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var inParam, inID bool
	var id strings.Builder
	for {
		tok, terr := dec.Token()
		if errors.Is(terr, io.EOF) {
			break
		}
		if terr != nil {
			return root, "", fmt.Errorf("invoice XML: %w", terr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root.Local == "" {
				root = t.Name
			}
			switch {
			case t.Name.Local == "GuidelineSpecifiedDocumentContextParameter":
				inParam = true
			case inParam && t.Name.Local == "ID":
				inID = true
			}
		case xml.EndElement:
			switch {
			case inID && t.Name.Local == "ID":
				return root, strings.TrimSpace(id.String()), nil
			case t.Name.Local == "GuidelineSpecifiedDocumentContextParameter":
				inParam = false
			}
		case xml.CharData:
			if inID {
				id.Write(t)
			}
		}
	}
	if root.Local == "" {
		return root, "", fmt.Errorf("invoice XML: no root element")
	}
	return root, "", nil
}
