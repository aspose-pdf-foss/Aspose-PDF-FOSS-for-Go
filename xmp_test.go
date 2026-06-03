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
