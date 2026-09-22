// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
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

// nsPDFASchema is the pdfaSchema: element namespace used inside a PDF/A
// extension-schema block (ISO 19005-1 Annex E) to describe one schema.
const nsPDFASchema = "http://www.aiim.org/pdfa/ns/schema#"

// xmpExtensionBlocks separates the top-level rdf:Description elements that
// declare a PDF/A extension schema (pdfaExtension:schemas) from the rest of
// an XMP packet.
//
// This is a small hand-written scanner rather than a regular expression
// because an rdf:Description can legitimately nest further rdf:Description
// elements — this library's own facturXExtensionSchema does not (it uses
// rdf:li rdf:parseType="Resource" throughout), but a schema written by
// another producer may hold its bag entries as
// <rdf:li><rdf:Description>…</rdf:Description></rdf:li>. A non-greedy regex
// stops at the first </rdf:Description>, truncating such a block mid
// structure; xmpExtensionBlocks instead tracks nesting depth so each
// top-level Description is extracted whole. A self-closing
// <rdf:Description … /> (the form Ghostscript writes for a bare pdfaid
// identification) does not open a nesting level and is never merged into a
// following block.
func xmpExtensionBlocks(packet string) (blocks []string, rest string) {
	var out strings.Builder
	i := 0
	for i < len(packet) {
		open := strings.Index(packet[i:], "<rdf:Description")
		if open < 0 {
			out.WriteString(packet[i:])
			return blocks, out.String()
		}
		open += i
		nameEnd := open + len("<rdf:Description")
		if nameEnd >= len(packet) || !isXMLTagBoundary(packet[nameEnd]) {
			// Not actually this element (e.g. a hypothetical
			// <rdf:DescriptionX>) — copy one byte and keep scanning.
			out.WriteString(packet[i : open+1])
			i = open + 1
			continue
		}
		end, ok := descriptionBlockEnd(packet, open)
		if !ok {
			// Unterminated element: not well-formed XML either way: leave
			// the remainder untouched rather than guess.
			out.WriteString(packet[i:])
			return blocks, out.String()
		}
		// Trailing whitespace is consumed with the block (mirrors the \s*
		// tail the previous regex-based implementation matched), so a
		// removed block does not leave a blank line behind in rest.
		wsEnd := end
		for wsEnd < len(packet) && isXMLSpace(packet[wsEnd]) {
			wsEnd++
		}
		if strings.Contains(packet[open:end], "pdfaExtension:schemas") {
			blocks = append(blocks, strings.TrimSpace(packet[open:end]))
		} else {
			out.WriteString(packet[open:wsEnd])
		}
		i = wsEnd
	}
	return blocks, out.String()
}

// descriptionBlockEnd finds the index just past the closing tag that matches
// the <rdf:Description (self-closing or not) starting at open, counting
// nested rdf:Description elements so a block containing further
// rdf:Description children is captured whole. Returns ok=false when the
// element is never closed before the packet ends.
func descriptionBlockEnd(packet string, open int) (end int, ok bool) {
	tagEnd, selfClosing, ok := parseDescriptionOpenTag(packet, open)
	if !ok {
		return 0, false
	}
	if selfClosing {
		return tagEnd, true
	}
	depth := 1
	i := tagEnd
	for i < len(packet) {
		switch {
		case strings.HasPrefix(packet[i:], "</rdf:Description>"):
			depth--
			i += len("</rdf:Description>")
			if depth == 0 {
				return i, true
			}
		case strings.HasPrefix(packet[i:], "<rdf:Description") &&
			i+len("<rdf:Description") < len(packet) && isXMLTagBoundary(packet[i+len("<rdf:Description")]):
			te, sc, ok := parseDescriptionOpenTag(packet, i)
			if !ok {
				return 0, false
			}
			if !sc {
				depth++
			}
			i = te
		default:
			i++
		}
	}
	return 0, false
}

// parseDescriptionOpenTag parses the <rdf:Description ...> opening tag (or
// its self-closing form <rdf:Description .../>) starting at i, returning the
// index just past its closing '>' and whether it is self-closing.
func parseDescriptionOpenTag(packet string, i int) (end int, selfClosing bool, ok bool) {
	gt := strings.IndexByte(packet[i:], '>')
	if gt < 0 {
		return 0, false, false
	}
	gt += i
	selfClosing = gt > i && packet[gt-1] == '/'
	return gt + 1, selfClosing, true
}

// isXMLTagBoundary reports whether c can follow a tag name, i.e. the match
// is the whole name and not just a prefix of a longer one.
func isXMLTagBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/'
}

func isXMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
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

// extensionSchemaPrefixes scans PDF/A extension-schema blocks (as
// xmpExtensionBlocks returns them) for each schema's declared
// pdfaSchema:namespaceURI / pdfaSchema:prefix pair — in either the element
// form this library writes (facturXExtensionSchema) or a producer's
// attribute form on the rdf:Description itself — and returns them as a
// namespace URI → prefix map. A PDF/A validator compares the prefix a
// schema declares for its namespace against the prefix actually used on the
// properties in that namespace, so a kept foreign schema's properties need
// to be serialised under the prefix it names, not whatever generic prefix
// bindCustomPrefixes would otherwise be free to pick. A declared prefix that
// cannot be written as an XML namespace prefix (not an NCName, or starting
// with "xml") is ignored: the schema is broken either way, and honouring it
// would make the whole packet malformed.
//
// Each block is itself a well-formed, self-contained XML fragment (it
// carries its own xmlns declarations), so it is parsed directly rather than
// scanned as text. A schema entry ends at the closing </rdf:li> or
// </rdf:Description> that holds it — the same element whichever of this
// library's rdf:li[rdf:parseType=Resource] shape or a nested
// rdf:li/rdf:Description shape the block uses — at which point any
// namespaceURI/prefix pair accumulated so far is committed and reset, so a
// block naming several schemas resolves each independently.
func extensionSchemaPrefixes(blocks []string) map[string]string {
	out := map[string]string{}
	for _, block := range blocks {
		dec := xml.NewDecoder(strings.NewReader(block))
		var nsURI, prefix string
		flush := func() {
			if nsURI != "" && isUsableXMLPrefix(prefix) {
				out[nsURI] = prefix
			}
			nsURI, prefix = "", ""
		}
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Space == nsPDFASchema {
					switch t.Name.Local {
					case "namespaceURI":
						s, _ := readPropValue(dec, t)
						nsURI = strings.TrimSpace(s)
						continue
					case "prefix":
						s, _ := readPropValue(dec, t)
						prefix = strings.TrimSpace(s)
						continue
					}
				}
				for _, a := range t.Attr {
					if a.Name.Space != nsPDFASchema {
						continue
					}
					switch a.Name.Local {
					case "namespaceURI":
						nsURI = a.Value
					case "prefix":
						prefix = a.Value
					}
				}
			case xml.EndElement:
				if t.Name.Space == nsRDF && (t.Name.Local == "li" || t.Name.Local == "Description") {
					flush()
				}
			}
		}
		flush()
	}
	return out
}

// preferExtensionSchemaPrefixes returns custom with each property's Prefix
// overridden to the one its namespace's kept extension schema declares
// (extensionSchemaPrefixes), when there is one; bindCustomPrefixes only
// honours a Prefix when it doesn't collide, so this is what actually makes
// a foreign schema's declared prefix win.
func preferExtensionSchemaPrefixes(custom []XMPProperty, extensions []string) []XMPProperty {
	prefixes := extensionSchemaPrefixes(extensions)
	if len(prefixes) == 0 {
		return custom
	}
	out := make([]XMPProperty, len(custom))
	for i, p := range custom {
		if pfx, ok := prefixes[p.Namespace]; ok {
			p.Prefix = pfx
		}
		out[i] = p
	}
	return out
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
