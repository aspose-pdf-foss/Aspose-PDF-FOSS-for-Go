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

// InvoiceOptions configures AttachInvoice. The zero value detects the profile
// from the XML, names the file as the standard prescribes, and produces
// PDF/A-3b.
type InvoiceOptions struct {
	FileName string         // default: "factur-x.xml", or "xrechnung.xml" for XRechnung
	Profile  InvoiceProfile // default: read from the XML's guideline ID
	Format   PDFAFormat     // PDFA3B or PDFA3A; the zero value (PDFA1B, never valid here) means PDFA3B
}

// Invoice is an e-invoice found in a document.
type Invoice struct {
	XML      []byte         // the embedded invoice, byte for byte
	FileName string         // the attachment name
	Profile  InvoiceProfile // from the XMP ConformanceLevel, else from the XML
	Standard string         // "Factur-X 1.0 / ZUGFeRD 2.1+", "ZUGFeRD 2.0" or "ZUGFeRD 1.0"
	Version  string         // the XMP Version property; "" when absent
}

// invoiceFileNames are the attachment names the standards have used.
var invoiceFileNames = []string{"factur-x.xml", "xrechnung.xml", "zugferd-invoice.xml"}

// AttachInvoice turns the document into a hybrid e-invoice (Factur-X 1.0 /
// ZUGFeRD 2.1+): it embeds the caller's CII XML as an associated file,
// writes the Factur-X XMP properties with the extension schema that declares
// them, and converts the document to PDF/A-3. Calling it again replaces the
// invoice. The returned report is the PDF/A result; conversion removes any
// encryption, as PDF/A requires. Mirrors the intent of Aspose.PDF for .NET's
// PdfFormat.ZUGFeRD conversion.
func (d *Document) AttachInvoice(xmlData []byte, opts ...InvoiceOptions) (*PDFAValidationReport, error) {
	var o InvoiceOptions
	if len(opts) > 0 {
		o = opts[len(opts)-1]
	}
	// PDFAFormat's zero value is PDFA1B, which an e-invoice can never be, so
	// the zero value means "not set" here.
	if o.Format == PDFA1B {
		o.Format = PDFA3B
	}
	if o.Format.part() != 3 {
		return nil, fmt.Errorf("AttachInvoice: an e-invoice must be PDF/A-3, not %v", o.Format)
	}
	root, guideline, err := invoiceGuideline(xmlData)
	if err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	if root.Local != "CrossIndustryInvoice" || root.Space != nsCII {
		return nil, fmt.Errorf("AttachInvoice: the XML is not a Cross Industry Invoice (root %s in %q)", root.Local, root.Space)
	}
	profile := o.Profile
	if profile == InvoiceProfileUnknown {
		profile = invoiceProfileFromGuideline(guideline)
	}
	if profile == InvoiceProfileUnknown {
		return nil, fmt.Errorf("AttachInvoice: unknown profile %q; set InvoiceOptions.Profile", guideline)
	}
	name := o.FileName
	if name == "" {
		name = profile.defaultFileName()
	}

	// Replace a previous invoice under any of the standard names.
	ef := d.EmbeddedFiles()
	for _, existing := range ef.Names() {
		for _, n := range invoiceFileNames {
			if strings.EqualFold(existing, n) {
				ef.Remove(existing)
			}
		}
	}
	f, err := ef.addBytesWith(name, xmlData, "text/xml", "Factur-X invoice")
	if err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	f.SetAFRelationship(profile.relationship())

	if err := d.setInvoiceXMP(name, profile); err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	return d.ConvertToPDFA(o.Format)
}

// setInvoiceXMP writes the fx: properties and the extension schema declaring
// them, replacing any earlier invoice metadata of any generation.
func (d *Document) setInvoiceXMP(fileName string, profile InvoiceProfile) error {
	var extensions []string
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		blocks, _ := xmpExtensionBlocks(string(raw))
		for _, b := range blocks {
			// Any earlier invoice generation's schema entry goes; other
			// producers' entries in the same bag stay.
			if kept, ok := dropInvoiceSchemaEntries(b); ok {
				extensions = append(extensions, kept)
			}
		}
	}
	meta, _ := d.XMP()
	var custom []XMPProperty
	for _, p := range meta.Custom {
		switch {
		case p.Namespace == nsFacturX, p.Namespace == nsZUGFeRD2, p.Namespace == nsZUGFeRD1:
		case strings.HasPrefix(p.Namespace, nsPDFAPrefix) && p.Prefix != "pdfaid":
		default:
			custom = append(custom, p)
		}
	}
	custom = append(custom,
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "DocumentType", Value: "INVOICE"},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "DocumentFileName", Value: fileName},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "Version", Value: "1.0"},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "ConformanceLevel", Value: profile.String()},
	)
	meta.Custom = preferExtensionSchemaPrefixes(custom, extensions)
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	raw, err := d.XMPRaw()
	if err != nil {
		return err
	}
	extensions = append(extensions, facturXExtensionSchema)
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), extensions)))
}

// Invoice returns the e-invoice embedded in the document, or (nil, nil) when
// the document is not one. It reads Factur-X 1.0 / ZUGFeRD 2.1+, ZUGFeRD 2.0
// and ZUGFeRD 1.0 files: the attachment is found by the name the XMP gives,
// else by the names the standards use.
func (d *Document) Invoice() (*Invoice, error) {
	var ns, xmpName, level, version string
	if meta, err := d.XMP(); err == nil {
		for _, p := range meta.Custom {
			switch p.Namespace {
			case nsFacturX, nsZUGFeRD2, nsZUGFeRD1:
				ns = p.Namespace
				switch p.Name {
				case "DocumentFileName":
					xmpName = p.Value
				case "ConformanceLevel":
					level = p.Value
				case "Version":
					version = p.Value
				}
			}
		}
	}

	ef := d.EmbeddedFiles()
	var f *EmbeddedFile
	for _, existing := range ef.Names() {
		if xmpName != "" && strings.EqualFold(existing, xmpName) {
			f = ef.Get(existing)
			break
		}
	}
	if f == nil {
	search:
		for _, existing := range ef.Names() {
			for _, n := range invoiceFileNames {
				if strings.EqualFold(existing, n) {
					f = ef.Get(existing)
					break search
				}
			}
		}
	}
	if f == nil {
		return nil, nil
	}
	data, err := f.Data()
	if err != nil {
		return nil, fmt.Errorf("Invoice: %w", err)
	}

	root, guideline, _ := invoiceGuideline(data)
	profile := invoiceProfileFromLevel(level)
	if profile == InvoiceProfileUnknown {
		profile = invoiceProfileFromGuideline(guideline)
	}

	var standard string
	switch {
	case ns == nsZUGFeRD1, root.Local == "CrossIndustryDocument":
		standard = "ZUGFeRD 1.0"
	case ns == nsZUGFeRD2, ns == "" && strings.EqualFold(f.Name(), "zugferd-invoice.xml"):
		standard = "ZUGFeRD 2.0"
	default:
		standard = "Factur-X 1.0 / ZUGFeRD 2.1+"
	}
	return &Invoice{XML: data, FileName: f.Name(), Profile: profile, Standard: standard, Version: version}, nil
}
