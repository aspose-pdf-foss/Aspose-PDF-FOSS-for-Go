// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"strings"
)

// XMP namespaces of the three generations of hybrid invoices. The first is
// written; all three are read.
const (
	nsFacturX  = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"
	nsZUGFeRD2 = "urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#"
	nsZUGFeRD1 = "urn:ferd:pdfa:CrossIndustryDocument:invoice:1p0#"
)

// nsPDFAPrefix covers the PDF/A metadata namespaces (identification and the
// extension-schema vocabulary), all under one URI prefix.
const nsPDFAPrefix = "http://www.aiim.org/pdfa/ns/"

// reXMPDescription matches one rdf:Description element. The extension
// schemas this library writes contain no nested rdf:Description (their
// structures use rdf:parseType="Resource"), so a non-greedy match is exact
// for them.
var reXMPDescription = regexp.MustCompile(`(?s)<rdf:Description\b.*?</rdf:Description>\s*`)

// xmpExtensionBlocks separates the rdf:Description elements that declare PDF/A
// extension schemas from the rest of an XMP packet.
func xmpExtensionBlocks(packet string) (blocks []string, rest string) {
	rest = reXMPDescription.ReplaceAllStringFunc(packet, func(m string) string {
		if strings.Contains(m, "pdfaExtension:schemas") {
			blocks = append(blocks, strings.TrimSpace(m))
			return ""
		}
		return m
	})
	return blocks, rest
}

// insertXMPDescriptions puts rdf:Description elements back into a packet,
// just before </rdf:RDF>.
func insertXMPDescriptions(packet string, blocks []string) string {
	if len(blocks) == 0 {
		return packet
	}
	i := strings.LastIndex(packet, "</rdf:RDF>")
	if i < 0 {
		return packet
	}
	return packet[:i] + strings.Join(blocks, "\n") + "\n" + packet[i:]
}

// facturXExtensionSchema declares the fx: properties to PDF/A validators,
// which reject properties from a namespace no extension schema describes.
const facturXExtensionSchema = `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Factur-X PDFA Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>fx</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>DocumentFileName</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The name of the embedded XML document</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>DocumentType</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The type of the hybrid document in capital letters, e.g. INVOICE or ORDER</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Version</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The actual version of the standard applying to the embedded XML document</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>ConformanceLevel</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The conformance level of the embedded XML document</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>`
