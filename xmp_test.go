// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestXMPRoundTrip writes a full XMP packet, saves, reopens, and reads it
// back, checking every modelled field survives.
func TestXMPRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	want := pdf.XMPMetadata{
		Title:        "Quarterly Report",
		Authors:      []string{"Alice Smith", "Bob Jones"},
		Description:  "Q3 2026 financial summary",
		Keywords:     []string{"finance", "report", "Q3"},
		CreatorTool:  "Aspose.PDF FOSS for Go",
		Producer:     "Aspose.PDF FOSS for Go",
		CreateDate:   "2026-05-29T12:00:00Z",
		ModifyDate:   "2026-05-29T13:30:00Z",
		MetadataDate: "2026-05-29T13:30:00Z",
		Custom: []pdf.XMPProperty{
			{Namespace: "http://ns.adobe.com/xap/1.0/mm/", Prefix: "xmpMM", Name: "DocumentID", Value: "uuid:1234"},
		},
	}
	if err := doc.SetXMP(want); err != nil {
		t.Fatalf("SetXMP: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	got, err := reopened.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}

	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if strings.Join(got.Authors, "|") != strings.Join(want.Authors, "|") {
		t.Errorf("Authors = %v, want %v", got.Authors, want.Authors)
	}
	if got.Description != want.Description {
		t.Errorf("Description = %q, want %q", got.Description, want.Description)
	}
	if strings.Join(got.Keywords, "|") != strings.Join(want.Keywords, "|") {
		t.Errorf("Keywords = %v, want %v", got.Keywords, want.Keywords)
	}
	if got.CreatorTool != want.CreatorTool {
		t.Errorf("CreatorTool = %q, want %q", got.CreatorTool, want.CreatorTool)
	}
	if got.Producer != want.Producer {
		t.Errorf("Producer = %q, want %q", got.Producer, want.Producer)
	}
	if got.CreateDate != want.CreateDate || got.ModifyDate != want.ModifyDate || got.MetadataDate != want.MetadataDate {
		t.Errorf("dates = %q/%q/%q, want %q/%q/%q",
			got.CreateDate, got.ModifyDate, got.MetadataDate,
			want.CreateDate, want.ModifyDate, want.MetadataDate)
	}
	// Custom: namespace/name/value round-trip (prefix is cosmetic and not
	// recoverable from encoding/xml, so it is not compared).
	var foundCustom bool
	for _, p := range got.Custom {
		if p.Namespace == "http://ns.adobe.com/xap/1.0/mm/" && p.Name == "DocumentID" {
			foundCustom = true
			if p.Value != "uuid:1234" {
				t.Errorf("custom DocumentID = %q, want %q", p.Value, "uuid:1234")
			}
		}
	}
	if !foundCustom {
		t.Errorf("custom property xmpMM:DocumentID not round-tripped; got %+v", got.Custom)
	}
	// Namespace declarations must NOT leak in as custom properties.
	if len(got.Custom) != 1 {
		t.Errorf("Custom has %d entries, want 1 (no xmlns leakage); got %+v", len(got.Custom), got.Custom)
	}
}

// TestXMPParseExternal parses a packet that mixes the attribute form
// (pdf:Producer on rdf:Description) with the element form (dc:title in an
// rdf:Alt, dc:creator in an rdf:Seq), like real-world producers emit.
func TestXMPParseExternal(t *testing.T) {
	packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
      xmlns:dc="http://purl.org/dc/elements/1.1/"
      xmlns:xmp="http://ns.adobe.com/xap/1.0/"
      xmlns:pdf="http://ns.adobe.com/pdf/1.3/"
      pdf:Producer="Acme PDF 2.0"
      xmp:CreateDate="2025-01-02T03:04:05Z">
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Hello &amp; Welcome</rdf:li></rdf:Alt></dc:title>
   <dc:creator><rdf:Seq><rdf:li>First Author</rdf:li><rdf:li>Second Author</rdf:li></rdf:Seq></dc:creator>
   <dc:subject><rdf:Bag><rdf:li>alpha</rdf:li><rdf:li>beta</rdf:li></rdf:Bag></dc:subject>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

	doc := pdf.NewDocument(595, 842)
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
		t.Fatalf("SetXMPRaw: %v", err)
	}
	got, err := doc.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}
	if got.Title != "Hello & Welcome" {
		t.Errorf("Title = %q, want %q", got.Title, "Hello & Welcome")
	}
	if got.Producer != "Acme PDF 2.0" {
		t.Errorf("Producer = %q, want %q", got.Producer, "Acme PDF 2.0")
	}
	if got.CreateDate != "2025-01-02T03:04:05Z" {
		t.Errorf("CreateDate = %q", got.CreateDate)
	}
	if strings.Join(got.Authors, "|") != "First Author|Second Author" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if strings.Join(got.Keywords, "|") != "alpha|beta" {
		t.Errorf("Keywords = %v", got.Keywords)
	}
}

// TestXMPClear removes the packet so a subsequent read is empty.
func TestXMPClear(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	_ = doc.SetXMP(pdf.XMPMetadata{Title: "temp"})
	if raw, _ := doc.XMPRaw(); len(raw) == 0 {
		t.Fatal("expected XMP packet after SetXMP")
	}
	doc.ClearXMP()
	raw, err := doc.XMPRaw()
	if err != nil {
		t.Fatalf("XMPRaw: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("expected no XMP after ClearXMP, got %d bytes", len(raw))
	}
	got, _ := doc.XMP()
	if !got.IsEmpty() {
		t.Errorf("expected empty XMP after clear, got %+v", got)
	}
}

// TestXMPCustomPrefixCollisionKeepsNamespacesSeparate: two custom properties
// whose caller-supplied Prefix collides (as happens after a round trip
// through XMP(), which synthesises a generic prefix for any namespace it
// doesn't specifically recognise) must still serialise into two distinct
// XML namespaces, not merge under whichever one was declared first.
func TestXMPCustomPrefixCollisionKeepsNamespacesSeparate(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	want := pdf.XMPMetadata{Custom: []pdf.XMPProperty{
		{Namespace: "urn:example:one#", Prefix: "ns", Name: "Foo", Value: "foo-value"},
		{Namespace: "urn:example:two#", Prefix: "ns", Name: "Bar", Value: "bar-value"},
	}}
	if err := doc.SetXMP(want); err != nil {
		t.Fatalf("SetXMP: %v", err)
	}
	back := saveAndReopenXMP(t, doc)
	meta, err := back.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}
	got := map[string]string{}
	for _, p := range meta.Custom {
		got[p.Namespace+"|"+p.Name] = p.Value
	}
	if got["urn:example:one#|Foo"] != "foo-value" {
		t.Errorf("urn:example:one#|Foo = %q, want %q; custom = %+v", got["urn:example:one#|Foo"], "foo-value", meta.Custom)
	}
	if got["urn:example:two#|Bar"] != "bar-value" {
		t.Errorf("urn:example:two#|Bar = %q, want %q; custom = %+v", got["urn:example:two#|Bar"], "bar-value", meta.Custom)
	}
}

// TestXMPCorePDFNamespaceDeclaredOnce: a pdf: property this library does not
// model (e.g. pdf:PDFVersion, which Acrobat/Word write) lands in Custom with
// Namespace nsPDF and Prefix "pdf" (xmlPrefixHint). buildXMP already
// declares xmlns:pdf unconditionally for the core pdf:Producer field; the
// custom-namespace binding must recognise that and not declare it a second
// time on the same rdf:Description, which is not well-formed XML (duplicate
// attribute) even though Go's own encoding/xml tolerates reading it back.
func TestXMPCorePDFNamespaceDeclaredOnce(t *testing.T) {
	packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
      xmlns:dc="http://purl.org/dc/elements/1.1/"
      xmlns:xmp="http://ns.adobe.com/xap/1.0/"
      xmlns:pdf="http://ns.adobe.com/pdf/1.3/"
      pdf:Producer="Acme PDF 2.0"
      pdf:PDFVersion="1.7"
      xmp:CreateDate="2025-01-02T03:04:05Z">
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Hi</rdf:li></rdf:Alt></dc:title>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
	doc := pdf.NewDocument(595, 842)
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
		t.Fatalf("SetXMPRaw: %v", err)
	}
	meta, err := doc.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}
	// Round-trip through SetXMP, as ConvertToPDFA's setPDFAMetadata and
	// AttachInvoice's setInvoiceXMP both do when they re-read and rewrite
	// the packet.
	if err := doc.SetXMP(meta); err != nil {
		t.Fatalf("SetXMP: %v", err)
	}
	raw, err := doc.XMPRaw()
	if err != nil {
		t.Fatalf("XMPRaw: %v", err)
	}
	s := string(raw)
	if n := strings.Count(s, "xmlns:pdf="); n != 1 {
		t.Errorf("xmlns:pdf= appears %d times, want 1:\n%s", n, s)
	}
	if n := strings.Count(s, "xmlns:dc="); n != 1 {
		t.Errorf("xmlns:dc= appears %d times, want 1:\n%s", n, s)
	}
	if n := strings.Count(s, "xmlns:xmp="); n != 1 {
		t.Errorf("xmlns:xmp= appears %d times, want 1:\n%s", n, s)
	}
	if !strings.Contains(s, "<pdf:PDFVersion>1.7</pdf:PDFVersion>") {
		t.Errorf("pdf:PDFVersion did not survive the round trip under the pdf: prefix:\n%s", s)
	}
}

// TestXMPReservesRDFAndXPrefixes: a custom property whose caller-supplied
// Prefix is "rdf" or "x" (the packet skeleton's own prefixes, for rdf:RDF /
// rdf:Description and x:xmpmeta) must not have those prefixes rebound to a
// different namespace — that would desynchronise the meaning of the
// packet's own structural elements from what the rest of buildXMP assumes.
func TestXMPReservesRDFAndXPrefixes(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	want := pdf.XMPMetadata{Custom: []pdf.XMPProperty{
		{Namespace: "urn:example:one#", Prefix: "rdf", Name: "Foo", Value: "foo-value"},
		{Namespace: "urn:example:two#", Prefix: "x", Name: "Bar", Value: "bar-value"},
	}}
	if err := doc.SetXMP(want); err != nil {
		t.Fatalf("SetXMP: %v", err)
	}
	raw, err := doc.XMPRaw()
	if err != nil {
		t.Fatalf("XMPRaw: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `xmlns:rdf="urn:example:one#"`) {
		t.Errorf("the rdf: prefix was rebound to a custom namespace:\n%s", s)
	}
	if strings.Contains(s, `xmlns:x="urn:example:two#"`) {
		t.Errorf("the x: prefix was rebound to a custom namespace:\n%s", s)
	}

	back := saveAndReopenXMP(t, doc)
	meta, err := back.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}
	got := map[string]string{}
	for _, p := range meta.Custom {
		got[p.Namespace+"|"+p.Name] = p.Value
	}
	if got["urn:example:one#|Foo"] != "foo-value" {
		t.Errorf("urn:example:one#|Foo = %q, want %q; custom = %+v", got["urn:example:one#|Foo"], "foo-value", meta.Custom)
	}
	if got["urn:example:two#|Bar"] != "bar-value" {
		t.Errorf("urn:example:two#|Bar = %q, want %q; custom = %+v", got["urn:example:two#|Bar"], "bar-value", meta.Custom)
	}
}

func saveAndReopenXMP(t *testing.T, doc *pdf.Document) *pdf.Document {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	back, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	return back
}

// TestSyncInfoToXMP maps the /Info dictionary into the XMP packet.
func TestSyncInfoToXMP(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetInfo(pdf.DocumentInfo{
		Title:        "Synced Doc",
		Author:       "Jane Author",
		Subject:      "An abstract",
		Keywords:     "go, pdf, xmp",
		Creator:      "MyTool",
		Producer:     "MyProducer",
		CreationDate: "D:20260529120000Z",
	})
	if err := doc.SyncInfoToXMP(); err != nil {
		t.Fatalf("SyncInfoToXMP: %v", err)
	}
	got, err := doc.XMP()
	if err != nil {
		t.Fatalf("XMP: %v", err)
	}
	if got.Title != "Synced Doc" {
		t.Errorf("Title = %q", got.Title)
	}
	if strings.Join(got.Authors, "|") != "Jane Author" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if got.Description != "An abstract" {
		t.Errorf("Description = %q", got.Description)
	}
	if strings.Join(got.Keywords, "|") != "go|pdf|xmp" {
		t.Errorf("Keywords = %v", got.Keywords)
	}
	if got.Producer != "MyProducer" || got.CreatorTool != "MyTool" {
		t.Errorf("Producer/CreatorTool = %q/%q", got.Producer, got.CreatorTool)
	}
	if got.CreateDate != "2026-05-29T12:00:00Z" {
		t.Errorf("CreateDate = %q, want ISO 8601 from PDF date", got.CreateDate)
	}
}

// Common predefined namespaces keep their conventional prefix through an
// XMP()/SetXMP round trip.
func TestXMPPredefinedNamespacePrefixes(t *testing.T) {
	doc := pdf.NewDocument(100, 100)
	packet := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:photoshop="http://ns.adobe.com/photoshop/1.0/" xmlns:xmpRights="http://ns.adobe.com/xap/1.0/rights/" xmlns:tiff="http://ns.adobe.com/tiff/1.0/" xmlns:exif="http://ns.adobe.com/exif/1.0/" xmlns:pdfx="http://ns.adobe.com/pdfx/1.3/" xmlns:xmpTPg="http://ns.adobe.com/xap/1.0/t/pg/">
<photoshop:ColorMode>3</photoshop:ColorMode>
<xmpRights:Marked>True</xmpRights:Marked>
<tiff:Orientation>1</tiff:Orientation>
<exif:ColorSpace>1</exif:ColorSpace>
<pdfx:Company>Acme</pdfx:Company>
<xmpTPg:NPages>1</xmpTPg:NPages>
</rdf:Description></rdf:RDF></x:xmpmeta>`
	if err := doc.SetXMPRaw([]byte(packet)); err != nil {
		t.Fatal(err)
	}
	meta, err := doc.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetXMP(meta); err != nil {
		t.Fatal(err)
	}
	raw, _ := doc.XMPRaw()
	for _, want := range []string{
		"<photoshop:ColorMode>3</photoshop:ColorMode>",
		"<xmpRights:Marked>True</xmpRights:Marked>",
		"<tiff:Orientation>1</tiff:Orientation>",
		"<exif:ColorSpace>1</exif:ColorSpace>",
		"<pdfx:Company>Acme</pdfx:Company>",
		"<xmpTPg:NPages>1</xmpTPg:NPages>",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s:\n%s", want, raw)
		}
	}
}
