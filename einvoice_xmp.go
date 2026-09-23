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
		out.WriteString(packet[i:open]) // the packet skeleton around the element
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
			block, remainder := splitExtensionDescription(packet[open:end])
			blocks = append(blocks, strings.TrimSpace(block))
			if remainder != "" {
				// The element also held ordinary properties: the block
				// carries the schemas alone, so those stay here in rest.
				out.WriteString(remainder)
				out.WriteString(packet[end:wsEnd])
			}
		} else {
			out.WriteString(packet[open:wsEnd])
		}
		i = wsEnd
	}
	return blocks, out.String()
}

// splitExtensionDescription takes one rdf:Description element holding a
// pdfaExtension:schemas element and returns the block to carry across a
// metadata rewrite and, when the element also held ordinary properties, the
// rdf:Description that holds those alone.
//
// A producer may put pdfaid, dc:title, its own custom properties and its
// extension schemas in a single rdf:Description. Carrying such an element
// whole would reinsert those properties beside the ones the rewritten packet
// already states — two pdfaid:part values, two dc:title elements — so only
// the schemas element travels, in a fresh rdf:Description carrying the
// xmlns declarations it uses. An element that holds nothing but the schemas
// is returned unchanged.
func splitExtensionDescription(elem string) (block, remainder string) {
	tagEnd, selfClosing, ok := parseDescriptionOpenTag(elem, 0)
	if !ok || selfClosing || !strings.HasSuffix(elem, "</rdf:Description>") {
		return elem, ""
	}
	openTag := elem[:tagEnd]
	content := elem[tagEnd : len(elem)-len("</rdf:Description>")]

	start := strings.Index(content, "<pdfaExtension:schemas")
	if start < 0 {
		return elem, ""
	}
	closeTag := "</pdfaExtension:schemas>"
	stop := strings.Index(content[start:], closeTag)
	if stop < 0 {
		return elem, ""
	}
	stop += start + len(closeTag)
	schemas := content[start:stop]
	rest := strings.TrimSpace(content[:start] + content[stop:])
	if rest == "" && !descriptionHasProperties(openTag) {
		return elem, "" // nothing but the schemas: keep the element as it is
	}
	return buildExtensionDescription(openTag, schemas), openTag + "\n" + rest + "\n</rdf:Description>"
}

// descriptionHasProperties reports whether an rdf:Description opening tag
// carries properties as attributes (the abbreviated form), as opposed to
// only namespace declarations and rdf:about.
func descriptionHasProperties(openTag string) bool {
	for _, a := range xmlTagAttributes(openTag) {
		if a.name == "rdf:about" || a.name == "xmlns" || strings.HasPrefix(a.name, "xmlns:") {
			continue
		}
		return true
	}
	return false
}

// buildExtensionDescription wraps a pdfaExtension:schemas element in a fresh
// rdf:Description, declaring the namespace prefixes the element uses out of
// those the original opening tag declared. rdf is always declared: each
// block is parsed on its own (extensionSchemaPrefixes) and its rdf:Bag /
// rdf:li structure has to resolve there too.
func buildExtensionDescription(openTag, schemas string) string {
	var b strings.Builder
	b.WriteString(`<rdf:Description rdf:about=""`)
	declared := map[string]bool{}
	for _, a := range xmlTagAttributes(openTag) {
		prefix, ok := strings.CutPrefix(a.name, "xmlns:")
		if !ok || !strings.Contains(schemas, prefix+":") {
			continue
		}
		declared[prefix] = true
		b.WriteString(" " + a.name + `="` + escapeXMLAttr(a.value) + `"`)
	}
	// A packet may declare these on rdf:RDF or x:xmpmeta rather than on the
	// element itself, and the block has to stand on its own: it is parsed
	// alone (extensionSchemaPrefixes) and reinserted elsewhere in the packet.
	for _, known := range []struct{ prefix, uri string }{
		{"rdf", nsRDF},
		{"pdfaExtension", nsPDFAPrefix + "extension/"},
		{"pdfaSchema", nsPDFAPrefix + "schema#"},
		{"pdfaProperty", nsPDFAPrefix + "property#"},
		{"pdfaType", nsPDFAPrefix + "type#"},
		{"pdfaField", nsPDFAPrefix + "field#"},
	} {
		if !declared[known.prefix] &&
			(known.prefix == "rdf" || strings.Contains(schemas, known.prefix+":")) {
			b.WriteString(` xmlns:` + known.prefix + `="` + known.uri + `"`)
		}
	}
	b.WriteString(">\n" + strings.TrimSpace(schemas) + "\n</rdf:Description>")
	return b.String()
}

// escapeXMLAttr escapes a quote that could not appear literally in a
// double-quoted attribute value — the value may have come from a
// single-quoted one. Entities are left as they are: the value is copied from
// valid XML, so an ampersand in it already starts one.
func escapeXMLAttr(v string) string {
	return strings.ReplaceAll(v, `"`, "&quot;")
}

type xmlAttr struct{ name, value string }

// xmlTagAttributes reads the name="value" pairs of an opening tag. Values may
// be single- or double-quoted; entities are left as they are, since the
// attributes are copied verbatim into another tag.
func xmlTagAttributes(tag string) []xmlAttr {
	var out []xmlAttr
	i := 0
	// Skip "<name".
	for i < len(tag) && !isXMLSpace(tag[i]) {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		start := i
		for i < len(tag) && tag[i] != '=' && !isXMLSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		name := tag[start:i]
		if name == "" || i >= len(tag) || tag[i] != '=' {
			return out
		}
		i++ // '='
		if i >= len(tag) || (tag[i] != '"' && tag[i] != '\'') {
			return out
		}
		quote := tag[i]
		i++
		vStart := i
		for i < len(tag) && tag[i] != quote {
			i++
		}
		if i >= len(tag) {
			return out
		}
		out = append(out, xmlAttr{name: name, value: tag[vStart:i]})
		i++
	}
	return out
}

// descriptionBlockEnd finds the index just past the closing tag that matches
// the <rdf:Description (self-closing or not) starting at open, counting
// nested rdf:Description elements so a block containing further
// rdf:Description children is captured whole. Returns ok=false when the
// element is never closed before the packet ends.
func descriptionBlockEnd(packet string, open int) (end int, ok bool) {
	return xmlElementEnd(packet, open, "rdf:Description")
}

// xmlElementEnd finds the index just past the closing tag matching the
// element of the given qualified name (self-closing or not) that starts at
// open, counting nested elements of the same name. Returns ok=false when the
// element is never closed before the text ends.
func xmlElementEnd(packet string, open int, name string) (end int, ok bool) {
	openTag, closeTag := "<"+name, "</"+name+">"
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
		case strings.HasPrefix(packet[i:], closeTag):
			depth--
			i += len(closeTag)
			if depth == 0 {
				return i, true
			}
		case strings.HasPrefix(packet[i:], openTag) &&
			i+len(openTag) < len(packet) && isXMLTagBoundary(packet[i+len(openTag)]):
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

// dropInvoiceSchemaEntries removes from an extension-schema block the schema
// entries describing one of the hybrid-invoice namespaces, keeping every
// other producer's entry in the same bag; ok reports whether anything is
// left to keep.
//
// An earlier generation's metadata is replaced, not accumulated: a ZUGFeRD
// 2.0 file upgraded to Factur-X would otherwise carry two schemas declaring
// the prefix fx, for two different namespaces. Filtering whole blocks would
// be wrong in the other direction — a producer may describe its own schema
// in the same rdf:Bag as the invoice one, and that entry has to survive.
func dropInvoiceSchemaEntries(block string) (string, bool) {
	var out strings.Builder
	seen, kept := 0, 0
	i := 0
	for i < len(block) {
		open := strings.Index(block[i:], "<rdf:li")
		if open < 0 {
			out.WriteString(block[i:])
			break
		}
		open += i
		if nameEnd := open + len("<rdf:li"); nameEnd >= len(block) || !isXMLTagBoundary(block[nameEnd]) {
			out.WriteString(block[i : open+1])
			i = open + 1
			continue
		}
		end, ok := xmlElementEnd(block, open, "rdf:li")
		if !ok {
			out.WriteString(block[i:]) // not well-formed: leave it alone
			seen = 0                   // nothing was really recognised: keep the block
			break
		}
		out.WriteString(block[i:open])
		seen++
		if !isInvoiceSchemaEntry(block[open:end]) {
			out.WriteString(block[open:end])
			kept++
		}
		i = end
	}
	// A block whose entries this scanner does not recognise at all (an
	// rdf:_1 container, an unterminated element) is kept whole rather than
	// dropped: leaving a foreign schema in place is harmless, while losing
	// it would leave that producer's properties undeclared.
	return out.String(), kept > 0 || seen == 0
}

// isInvoiceSchemaEntry reports whether an extension-schema entry describes
// one of the hybrid-invoice namespaces. The namespace is matched where it is
// declared — as the pdfaSchema:namespaceURI element or attribute — not
// anywhere in the entry, so another producer's schema is not deleted for
// merely naming Factur-X in a property description.
func isInvoiceSchemaEntry(entry string) bool {
	for _, declared := range schemaEntryNamespaces(entry) {
		switch declared {
		case nsFacturX, nsZUGFeRD2, nsZUGFeRD1:
			return true
		}
	}
	return false
}

// schemaEntryNamespaces returns the values a schema entry gives for
// pdfaSchema:namespaceURI, in either the element form this library writes or
// the attribute form a producer may use.
func schemaEntryNamespaces(entry string) []string {
	const key = "pdfaSchema:namespaceURI"
	var out []string
	for i := 0; ; {
		at := strings.Index(entry[i:], key)
		if at < 0 {
			return out
		}
		at += i + len(key)
		i = at
		if at >= len(entry) {
			return out
		}
		switch entry[at] {
		case '>': // <pdfaSchema:namespaceURI>value</…>
			if stop := strings.IndexByte(entry[at:], '<'); stop > 0 {
				out = append(out, strings.TrimSpace(entry[at+1:at+stop]))
			}
		case '=': // pdfaSchema:namespaceURI="value"
			rest := strings.TrimLeft(entry[at+1:], " \t\r\n")
			if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
				if stop := strings.IndexByte(rest[1:], rest[0]); stop >= 0 {
					out = append(out, strings.TrimSpace(rest[1:1+stop]))
				}
			}
		}
	}
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

// xmpBlockWrapperOpen binds the prefixes an extension-schema block may have
// left to an ancestor, so a block can be parsed on its own. A block that
// declares them itself shadows these bindings.
const xmpBlockWrapperOpen = `<xmpwrap xmlns:rdf="` + nsRDF +
	`" xmlns:pdfaExtension="` + nsPDFAPrefix + `extension/" xmlns:pdfaSchema="` + nsPDFAPrefix +
	`schema#" xmlns:pdfaProperty="` + nsPDFAPrefix + `property#">`

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
		// A block carried unchanged from another producer's packet often
		// leaves xmlns:rdf to an ancestor, and parsed on its own its
		// rdf:li/rdf:Description ends would then resolve to the literal
		// prefix rather than the RDF namespace — the per-entry flush below
		// would never fire and a block naming several schemas would yield
		// one pair. Supply the binding rather than match on the prefix.
		dec := xml.NewDecoder(strings.NewReader(xmpBlockWrapperOpen + block + `</xmpwrap>`))
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
