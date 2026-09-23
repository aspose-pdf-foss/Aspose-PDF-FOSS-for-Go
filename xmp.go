// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// XMP namespace URIs (ISO 16684-1 / Adobe XMP Specification).
const (
	nsRDF     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsXMP     = "http://ns.adobe.com/xap/1.0/"
	nsPDF     = "http://ns.adobe.com/pdf/1.3/"
	nsXMPMM   = "http://ns.adobe.com/xap/1.0/mm/"
	nsXMPMeta = "adobe:ns:meta/" // the x: prefix on the packet's own <x:xmpmeta> root
)

// XMPProperty is a single simple (string-valued) XMP property in an
// arbitrary namespace. Used for properties outside the common Dublin
// Core / XMP Basic / PDF schemas modelled directly on XMPMetadata.
type XMPProperty struct {
	Namespace string // namespace URI, e.g. "http://ns.adobe.com/xap/1.0/mm/"
	Prefix    string // namespace prefix used in the serialised packet, e.g. "xmpMM"
	Name      string // local property name, e.g. "DocumentID"
	Value     string // property value
}

// XMPMetadata is the document's XMP packet modelled as common schema
// fields plus a list of arbitrary Custom properties. Empty fields are
// omitted when written. Dates are ISO 8601 strings (e.g.
// "2026-05-29T12:00:00Z") to round-trip losslessly.
//
// Field → XMP property mapping:
//   - Title        → dc:title        (language alternative, x-default)
//   - Authors      → dc:creator      (ordered rdf:Seq)
//   - Description  → dc:description  (language alternative, x-default)
//   - Keywords     → dc:subject      (unordered rdf:Bag)
//   - CreatorTool  → xmp:CreatorTool
//   - CreateDate   → xmp:CreateDate
//   - ModifyDate   → xmp:ModifyDate
//   - MetadataDate → xmp:MetadataDate
//   - Producer     → pdf:Producer
type XMPMetadata struct {
	Title        string
	Authors      []string
	Description  string
	Keywords     []string
	CreatorTool  string
	Producer     string
	CreateDate   string
	ModifyDate   string
	MetadataDate string
	Custom       []XMPProperty
}

// IsEmpty reports whether the XMP metadata carries no information.
func (m XMPMetadata) IsEmpty() bool {
	return m.Title == "" && len(m.Authors) == 0 && m.Description == "" &&
		len(m.Keywords) == 0 && m.CreatorTool == "" && m.Producer == "" &&
		m.CreateDate == "" && m.ModifyDate == "" && m.MetadataDate == "" &&
		len(m.Custom) == 0
}

// XMP returns the document's XMP metadata parsed from the
// /Catalog/Metadata stream. Returns a zero XMPMetadata (IsEmpty) when the
// document has no XMP packet. Parse errors in a malformed packet are
// returned; recognised properties parsed before the error are discarded.
func (d *Document) XMP() (XMPMetadata, error) {
	raw, err := d.XMPRaw()
	if err != nil {
		return XMPMetadata{}, err
	}
	if len(raw) == 0 {
		return XMPMetadata{}, nil
	}
	return parseXMP(raw)
}

// XMPRaw returns the raw bytes of the XMP packet from /Catalog/Metadata,
// or nil when the document has no XMP stream.
func (d *Document) XMPRaw() ([]byte, error) {
	if d.catalog == nil {
		return nil, nil
	}
	v, ok := d.catalog["/Metadata"]
	if !ok {
		return nil, nil
	}
	stream, ok := resolveRefToStream(d.objects, v)
	if !ok {
		return nil, nil
	}
	return stream.Data, nil
}

// SetXMP serialises meta into a standard XMP packet and stores it as the
// /Catalog/Metadata stream (uncompressed, /Type /Metadata /Subtype /XML
// per ISO 32000-1 §14.3.2). Replaces any existing packet. Passing an
// empty XMPMetadata writes an empty (but valid) packet; use ClearXMP to
// remove metadata entirely.
func (d *Document) SetXMP(meta XMPMetadata) error {
	return d.SetXMPRaw(buildXMP(meta))
}

// SetXMPRaw stores data verbatim as the /Catalog/Metadata stream — an
// escape hatch for callers that produce their own XMP packet. The bytes
// are written uncompressed so external tools can locate the xpacket
// markers. Reuses the existing Metadata object if present.
func (d *Document) SetXMPRaw(data []byte) error {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	stream := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/Metadata"),
			"/Subtype": pdfName("/XML"),
		},
		Data:    data,
		Decoded: false, // store as-is, no /Filter → uncompressed
	}
	if ref, ok := d.catalog["/Metadata"].(pdfRef); ok {
		if obj, exists := d.objects[ref.Num]; exists {
			obj.Value = stream
			return nil
		}
	}
	id := d.addObject(stream)
	d.catalog["/Metadata"] = pdfRef{Num: id}
	return nil
}

// ClearXMP removes the /Catalog/Metadata entry. The underlying stream
// object becomes unreferenced; call RemoveUnusedObjects to reclaim it.
func (d *Document) ClearXMP() {
	if d.catalog != nil {
		delete(d.catalog, "/Metadata")
	}
}

// SyncInfoToXMP builds an XMP packet from the document's current /Info
// dictionary and installs it, so the two metadata stores agree (ISO
// 32000-1 §14.3.2 recommends consistency when both are present). Info
// Title/Author/Subject/Keywords/Creator/Producer/dates map to the
// corresponding XMP properties. No-op-safe when /Info is absent.
func (d *Document) SyncInfoToXMP() error {
	meta, _ := d.Info()
	x := XMPMetadata{
		Title:       meta.Title,
		Description: meta.Subject,
		CreatorTool: meta.Creator,
		Producer:    meta.Producer,
		CreateDate:  pdfDateToISO8601(meta.CreationDate),
		ModifyDate:  pdfDateToISO8601(meta.ModDate),
	}
	if meta.Author != "" {
		x.Authors = []string{meta.Author}
	}
	if meta.Keywords != "" {
		x.Keywords = splitKeywords(meta.Keywords)
	}
	return d.SetXMP(x)
}

// splitKeywords splits an /Info Keywords string on commas or semicolons
// into a trimmed list, dropping empties.
func splitKeywords(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// pdfDateToISO8601 converts a PDF date string ("D:YYYYMMDDHHmmSSOHH'mm'")
// to an ISO 8601 timestamp. Returns the input unchanged if it is not in
// the recognised PDF form (already ISO 8601, or empty).
func pdfDateToISO8601(s string) string {
	if !strings.HasPrefix(s, "D:") {
		return s
	}
	b := s[2:]
	get := func(start, n int) string {
		if start+n <= len(b) {
			return b[start : start+n]
		}
		return ""
	}
	year := get(0, 4)
	if len(year) < 4 {
		return s
	}
	month := orDefault(get(4, 2), "01")
	day := orDefault(get(6, 2), "01")
	hour := orDefault(get(8, 2), "00")
	min := orDefault(get(10, 2), "00")
	sec := orDefault(get(12, 2), "00")
	iso := fmt.Sprintf("%s-%s-%sT%s:%s:%s", year, month, day, hour, min, sec)
	// Timezone: Z, or ±HH'mm'.
	if len(b) > 14 {
		switch b[14] {
		case 'Z':
			iso += "Z"
		case '+', '-':
			tz := strings.NewReplacer("'", "").Replace(b[14:])
			if len(tz) >= 5 {
				iso += tz[:3] + ":" + tz[3:5]
			}
		}
	}
	return iso
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// resolveRefToStream resolves v to a *pdfStream, following a ref if needed.
func resolveRefToStream(objects map[int]*pdfObject, v pdfValue) (*pdfStream, bool) {
	rv := resolveRef(objects, v)
	s, ok := rv.(*pdfStream)
	return s, ok
}

// buildXMP serialises an XMPMetadata value into a complete XMP packet.
func buildXMP(meta XMPMetadata) []byte {
	var b strings.Builder
	// The xpacket header conventionally carries a UTF-8 BOM as the "begin"
	// marker; build it from the rune so the Go source stays BOM-free.
	bom := string(rune(0xFEFF))
	b.WriteString("<?xpacket begin=\"" + bom + "\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="` + nsXMPMeta + `">` + "\n")
	b.WriteString(`  <rdf:RDF xmlns:rdf="` + nsRDF + `">` + "\n")

	// Namespace declarations: always declare the three core schemas plus
	// one prefix per distinct custom-property namespace (bindCustomPrefixes
	// resolves any collision, e.g. two custom properties that both hint the
	// same prefix, so two namespaces are never serialised under one).
	nsDecls := []string{
		`xmlns:dc="` + nsDC + `"`,
		`xmlns:xmp="` + nsXMP + `"`,
		`xmlns:pdf="` + nsPDF + `"`,
	}
	nsOrder, prefixOf := bindCustomPrefixes(meta.Custom)
	for _, ns := range nsOrder {
		nsDecls = append(nsDecls, `xmlns:`+prefixOf[ns]+`="`+ns+`"`)
	}
	b.WriteString(`    <rdf:Description rdf:about=""` + "\n")
	for i, decl := range nsDecls {
		end := "\n"
		if i == len(nsDecls)-1 {
			end = ">\n"
		}
		b.WriteString("        " + decl + end)
	}

	if meta.Title != "" {
		writeLangAlt(&b, "dc:title", meta.Title)
	}
	if len(meta.Authors) > 0 {
		writeOrdered(&b, "dc:creator", "rdf:Seq", meta.Authors)
	}
	if meta.Description != "" {
		writeLangAlt(&b, "dc:description", meta.Description)
	}
	if len(meta.Keywords) > 0 {
		writeOrdered(&b, "dc:subject", "rdf:Bag", meta.Keywords)
	}
	writeSimple(&b, "xmp:CreatorTool", meta.CreatorTool)
	writeSimple(&b, "xmp:CreateDate", meta.CreateDate)
	writeSimple(&b, "xmp:ModifyDate", meta.ModifyDate)
	writeSimple(&b, "xmp:MetadataDate", meta.MetadataDate)
	writeSimple(&b, "pdf:Producer", meta.Producer)
	for _, p := range meta.Custom {
		if p.Namespace == "" || p.Name == "" {
			continue
		}
		writeSimple(&b, prefixOf[p.Namespace]+":"+p.Name, p.Value)
	}

	b.WriteString("    </rdf:Description>\n")
	b.WriteString("  </rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	// ~2 KB of padding is conventional so editors can grow the packet
	// in place; keep it modest here.
	b.WriteString(`<?xpacket end="w"?>`)
	return []byte(b.String())
}

// bindCustomPrefixes assigns exactly one serialisation prefix per distinct
// namespace among meta.Custom, in first-seen order. order lists only the
// namespaces that still need an xmlns declaration written — the three core
// namespaces buildXMP declares unconditionally (dc, xmp, pdf) are pre-bound
// in prefixOf but never added to order, so a custom property that happens to
// land in one of them (e.g. pdf:PDFVersion, which this library does not
// model but Acrobat/Word both write, so addCustom classifies it as Custom
// with Namespace nsPDF) reuses that declaration instead of emitting
// xmlns:pdf a second time on the same rdf:Description — a duplicate
// attribute, not well-formed XML, even though Go's own encoding/xml
// tolerates reading it back.
//
// Each namespace's own properties' Prefix is used when possible; if it is
// empty, or already bound to a *different* namespace, a fresh prefix ("ns1",
// "ns2", …, skipping any already in use) is allocated instead. This covers
// two custom properties that legitimately hint the same prefix
// (xmlPrefixHint falls back to a generic "ns" for any namespace it does not
// specifically recognise, and even two distinct recognised namespaces can
// share a hint, e.g. Factur-X and ZUGFeRD 2.0 both prefer "fx") — and it is
// also why "rdf" and "x" are pre-bound here to sentinel namespaces: they are
// the packet skeleton's own prefixes (rdf:RDF/rdf:Description, x:xmpmeta),
// and rebinding either to a custom namespace would desynchronise the
// resolved meaning of the packet's own structural elements from what the
// rest of buildXMP assumes (parseXMP would then fail to even recognise the
// rdf:Description it is looking for). Properties sharing a namespace always
// share its bound prefix, so writeSimple never emits two different
// namespaces under one XML prefix (which would silently merge them on the
// next parse).
//
// The PDF/A and e-invoice namespaces this library writes are bound first, to
// their canonical prefixes, before any other namespace is considered:
// pdfaid is reserved for the PDF/A identification whether or not it is
// present (a validator looks for pdfaid:part literally, so a foreign
// namespace holding that prefix would hide the identification), and fx / zf
// go to the Factur-X / ZUGFeRD namespaces when those are present. A
// caller-supplied Prefix that is not a usable XML prefix (see
// isUsableXMLPrefix) is ignored, so a bad hint degrades to an ns<N> prefix
// instead of a packet that is not well-formed.
func bindCustomPrefixes(custom []XMPProperty) (order []string, prefixOf map[string]string) {
	prefixOf = map[string]string{
		nsDC:  "dc",
		nsXMP: "xmp",
		nsPDF: "pdf",
	}
	boundTo := map[string]string{
		"dc":     nsDC,
		"xmp":    nsXMP,
		"pdf":    nsPDF,
		"rdf":    nsRDF,
		"x":      nsXMPMeta,
		"pdfaid": nsPDFAID,
	}
	present := map[string]bool{}
	for _, p := range custom {
		if p.Namespace != "" && p.Name != "" {
			present[p.Namespace] = true
		}
	}
	for _, c := range []struct{ ns, prefix string }{
		{nsPDFAID, "pdfaid"},
		{nsFacturX, "fx"},
		{nsZUGFeRD2, "fx"},
		{nsZUGFeRD1, "zf"},
	} {
		if !present[c.ns] {
			continue
		}
		if owner := boundTo[c.prefix]; owner != "" && owner != c.ns {
			continue // fx already went to Factur-X; ZUGFeRD 2.0 takes an ns<N> below
		}
		prefixOf[c.ns] = c.prefix
		boundTo[c.prefix] = c.ns
		order = append(order, c.ns)
	}
	next := 1
	for _, p := range custom {
		if p.Namespace == "" || p.Name == "" {
			continue
		}
		if _, ok := prefixOf[p.Namespace]; ok {
			continue // already bound — a core namespace, or seen earlier in this loop
		}
		prefix := p.Prefix
		if !isUsableXMLPrefix(prefix) || (boundTo[prefix] != "" && boundTo[prefix] != p.Namespace) {
			for {
				prefix = fmt.Sprintf("ns%d", next)
				next++
				if boundTo[prefix] == "" {
					break
				}
			}
		}
		prefixOf[p.Namespace] = prefix
		boundTo[prefix] = p.Namespace
		order = append(order, p.Namespace)
	}
	return order, prefixOf
}

// isUsableXMLPrefix reports whether s can be declared as a namespace prefix:
// an XML NCName (Namespaces in XML 1.0 §3) that does not begin with "xml" in
// any case — xml and xmlns are reserved outright, and every other name
// starting with those letters is reserved for future standardisation.
func isUsableXMLPrefix(s string) bool {
	if s == "" || strings.HasPrefix(strings.ToLower(s), "xml") {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i == 0:
			return false
		case r == '-' || r == '.' || r == 0xB7 || unicode.IsDigit(r) ||
			unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Nd, r):
		default:
			return false
		}
	}
	return true
}

// writeSimple emits "<tag>value</tag>" (XML-escaped) when value is non-empty.
func writeSimple(b *strings.Builder, tag, value string) {
	if value == "" {
		return
	}
	b.WriteString("      <" + tag + ">")
	b.WriteString(xmlEscape(value))
	b.WriteString("</" + tag + ">\n")
}

// writeLangAlt emits a language-alternative property (rdf:Alt with a
// single x-default rdf:li), used for dc:title and dc:description.
func writeLangAlt(b *strings.Builder, tag, value string) {
	b.WriteString("      <" + tag + ">\n")
	b.WriteString("        <rdf:Alt>\n")
	b.WriteString(`          <rdf:li xml:lang="x-default">` + xmlEscape(value) + "</rdf:li>\n")
	b.WriteString("        </rdf:Alt>\n")
	b.WriteString("      </" + tag + ">\n")
}

// writeOrdered emits an rdf:Seq (ordered) or rdf:Bag (unordered) list.
func writeOrdered(b *strings.Builder, tag, container string, items []string) {
	b.WriteString("      <" + tag + ">\n")
	b.WriteString("        <" + container + ">\n")
	for _, it := range items {
		b.WriteString("          <rdf:li>" + xmlEscape(it) + "</rdf:li>\n")
	}
	b.WriteString("        </" + container + ">\n")
	b.WriteString("      </" + tag + ">\n")
}

// xmlEscape escapes a string for use as XML element/attribute text.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// parseXMP parses an XMP packet into an XMPMetadata. It handles both the
// element form (<dc:title><rdf:Alt><rdf:li>...) and the attribute form
// (<rdf:Description pdf:Producer="..."/>), and the rdf:Alt/Seq/Bag
// containers. Properties outside the modelled schemas land in Custom.
func parseXMP(data []byte) (XMPMetadata, error) {
	var m XMPMetadata
	dec := xml.NewDecoder(bytes.NewReader(data))
	customSeen := map[string]bool{}

	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return m, fmt.Errorf("parse xmp: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space == nsRDF && se.Name.Local == "Description" {
			// Attribute-form simple properties.
			for _, a := range se.Attr {
				assignXMP(&m, a.Name.Space, a.Name.Local, a.Value, customSeen)
			}
			// Element-form child properties.
			if err := parseDescriptionChildren(dec, &m, customSeen); err != nil {
				return m, err
			}
		}
	}
	return m, nil
}

// parseDescriptionChildren reads the property elements directly under an
// rdf:Description until its closing tag.
func parseDescriptionChildren(dec *xml.Decoder, m *XMPMetadata, customSeen map[string]bool) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("parse xmp: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			single, list := readPropValue(dec, t)
			if len(list) > 0 {
				assignXMPList(m, t.Name.Space, t.Name.Local, list)
			} else {
				assignXMP(m, t.Name.Space, t.Name.Local, strings.TrimSpace(single), customSeen)
			}
		case xml.EndElement:
			return nil // closes rdf:Description
		}
	}
}

// readPropValue reads a property element's content, flattening any
// rdf:Alt/Seq/Bag containers into a list of rdf:li values. Returns the
// accumulated character data and the list (empty when the property is a
// simple text value).
func readPropValue(dec *xml.Decoder, start xml.StartElement) (string, []string) {
	var text strings.Builder
	var list []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return text.String(), list
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == nsRDF && (t.Name.Local == "Alt" || t.Name.Local == "Seq" || t.Name.Local == "Bag"):
				_, sub := readPropValue(dec, t)
				list = append(list, sub...)
			case t.Name.Space == nsRDF && t.Name.Local == "li":
				s, _ := readPropValue(dec, t)
				list = append(list, strings.TrimSpace(s))
			default:
				s, sub := readPropValue(dec, t)
				text.WriteString(s)
				list = append(list, sub...)
			}
		case xml.CharData:
			text.Write([]byte(t))
		case xml.EndElement:
			return text.String(), list
		}
	}
}

// assignXMP routes a simple (string) property into the right field by
// namespace + local name; unknown namespaced properties become Custom.
func assignXMP(m *XMPMetadata, space, local, value string, customSeen map[string]bool) {
	if value == "" {
		return
	}
	// Skip namespace declarations (xmlns / xmlns:prefix) and xml:* / rdf:*
	// housekeeping attributes — these are not metadata properties.
	if space == "xmlns" || local == "xmlns" || space == "http://www.w3.org/XML/1998/namespace" {
		return
	}
	switch space {
	case nsDC:
		switch local {
		case "title":
			m.Title = value
		case "description":
			m.Description = value
		case "creator":
			m.Authors = appendUnique(m.Authors, value)
		case "subject":
			m.Keywords = appendUnique(m.Keywords, value)
		}
	case nsXMP:
		switch local {
		case "CreatorTool":
			m.CreatorTool = value
		case "CreateDate":
			m.CreateDate = value
		case "ModifyDate":
			m.ModifyDate = value
		case "MetadataDate":
			m.MetadataDate = value
		}
	case nsPDF:
		if local == "Producer" {
			m.Producer = value
			return
		}
		addCustom(m, space, local, value, customSeen)
	case nsRDF, "":
		// rdf:about, xmlns, xml:lang etc — ignore.
	default:
		addCustom(m, space, local, value, customSeen)
	}
}

// assignXMPList routes a container (Seq/Bag/Alt) property into the right
// field. dc:creator/subject become lists; dc:title/description take the
// first (x-default) entry.
func assignXMPList(m *XMPMetadata, space, local string, list []string) {
	if space != nsDC {
		return
	}
	switch local {
	case "creator":
		for _, v := range list {
			m.Authors = appendUnique(m.Authors, v)
		}
	case "subject":
		for _, v := range list {
			m.Keywords = appendUnique(m.Keywords, v)
		}
	case "title":
		if len(list) > 0 {
			m.Title = list[0]
		}
	case "description":
		if len(list) > 0 {
			m.Description = list[0]
		}
	}
}

// addCustom appends a custom property, de-duplicating by namespace+name.
func addCustom(m *XMPMetadata, space, local, value string, seen map[string]bool) {
	key := space + " " + local
	if seen[key] {
		return
	}
	seen[key] = true
	m.Custom = append(m.Custom, XMPProperty{
		Namespace: space,
		Prefix:    xmlPrefixHint(space),
		Name:      local,
		Value:     value,
	})
}

// xmlPrefixHint derives a serialisation prefix for a custom property.
// encoding/xml does not surface the original prefix, so fall back to a
// short synthetic one for a namespace this library specifically writes
// elsewhere (so a round trip through XMP()/SetXMP keeps its conventional
// prefix), else a generic one that bindCustomPrefixes will make unique.
func xmlPrefixHint(space string) string {
	switch space {
	case nsDC:
		return "dc"
	case nsXMP:
		return "xmp"
	case nsPDF:
		return "pdf"
	case nsFacturX, nsZUGFeRD2:
		return "fx"
	case nsZUGFeRD1:
		return "zf"
	case nsPDFAID:
		return "pdfaid"
	case nsXMPMM:
		return "xmpMM"
	case "http://ns.adobe.com/photoshop/1.0/":
		return "photoshop"
	case "http://ns.adobe.com/xap/1.0/rights/":
		return "xmpRights"
	case "http://ns.adobe.com/xap/1.0/t/pg/":
		return "xmpTPg"
	case "http://ns.adobe.com/tiff/1.0/":
		return "tiff"
	case "http://ns.adobe.com/exif/1.0/":
		return "exif"
	case "http://ns.adobe.com/pdfx/1.3/":
		return "pdfx"
	}
	return "ns"
}

// xmpWellFormed reports whether a raw XMP packet parses as XML. PDF/A
// requires a parsable packet, and a producer that assembled one by string
// surgery can leave it broken — ValidatePDFA reports that as XMP_MALFORMED
// rather than silently reading past it with the regexes above.
func xmpWellFormed(raw []byte) bool {
	// A packet is padded after its trailer, conventionally with spaces but
	// sometimes with NULs, which are not XML characters.
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimRight(raw, "\x00 \t\r\n")))
	for {
		_, err := dec.Token()
		if err != nil {
			return errors.Is(err, io.EOF)
		}
	}
}

// appendUnique appends v to list if not already present.
func appendUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}
