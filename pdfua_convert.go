// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// nsPDFUAID is the PDF/UA identification XMP namespace (ISO 14289-1).
const nsPDFUAID = "http://www.aiim.org/pdfua/ns/id/"

// pdfuaExtensionSchema declares the pdfuaid: property. PDF/UA itself does not
// need it, but PDF/A rejects any property its own schemas do not cover, and a
// file may claim both.
const pdfuaExtensionSchema = `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>PDF/UA identification schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>` + nsPDFUAID + `</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>pdfuaid</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>part</pdfaProperty:name>
<pdfaProperty:valueType>Integer</pdfaProperty:valueType>
<pdfaProperty:category>internal</pdfaProperty:category>
<pdfaProperty:description>Part of ISO 14289 the file conforms to</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>`

// ConvertToPDFUA prepares a tagged document for PDF/UA-1 and returns a validation
// report of what remains. It embeds the fonts the document draws with, makes /Info
// and XMP agree, writes the pdfuaid identifier and the XMP title, and has viewers
// show the title.
//
// It does not build a structure tree: author one with TaggedContent or call
// AutoTag first. It cannot write alternate text for figures: the report names
// them, and ai.ImageDescriptionCopilot.FillAltTexts can. A document without a
// title stays non-conformant; set one with SetInfo or TaggedContent.SetTitle.
//
// A file may be both PDF/A and PDF/UA: call ConvertToPDFA and ConvertToPDFUA in
// either order. Mirrors the intent of Aspose.PDF for .NET's
// Document.Convert(PdfFormat.PDF_UA_1).
func (d *Document) ConvertToPDFUA() (*PDFUAValidationReport, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("ConvertToPDFUA: document has no pages")
	}
	d.signaturesRewritten = d.hasSignature()
	d.fixInheritedResources() // before anything reads a form with its own resources
	d.fixFontDicts(2)
	d.embedStandardFonts()
	d.embedCompositeFonts()
	part := 0
	if d.isPDFA1() {
		part = 1
	}
	d.fixCIDSets(part)
	d.fixSharedConformance()
	if err := d.setPDFUAMetadata(); err != nil {
		return nil, err
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	vp, ok := resolveRefToDict(d.objects, d.catalog["/ViewerPreferences"])
	if !ok {
		vp = pdfDict{}
		d.catalog["/ViewerPreferences"] = vp
	}
	vp["/DisplayDocTitle"] = true
	return d.ValidatePDFUA(), nil
}

// setPDFUAMetadata adds the pdfuaid identifier to the XMP packet, keeping what
// is there, including the extension schemas a PDF/A identification depends on.
func (d *Document) setPDFUAMetadata() error {
	var blocks []string
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		blocks, _ = xmpExtensionBlocks(string(raw))
	}
	meta, _ := d.XMP()
	d.reconcileInfoAndXMP(&meta)
	var custom []XMPProperty
	for _, p := range meta.Custom {
		if p.Namespace != nsPDFUAID && !strings.HasPrefix(p.Namespace, nsPDFAPrefix+"extension") {
			custom = append(custom, p)
		}
	}
	custom = append(custom, XMPProperty{Namespace: nsPDFUAID, Prefix: "pdfuaid", Name: "part", Value: "1"})
	if !hasPDFUASchema(blocks) {
		blocks = append(blocks, pdfuaExtensionSchema)
	}
	meta.Custom = preferExtensionSchemaPrefixes(custom, blocks)
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	raw, err := d.XMPRaw()
	if err != nil {
		return err
	}
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), blocks)))
}

func hasPDFUASchema(blocks []string) bool {
	_, ok := extensionSchemaPrefixes(blocks)[nsPDFUAID]
	return ok
}
