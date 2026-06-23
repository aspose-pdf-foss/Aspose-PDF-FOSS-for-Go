// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// nsPDFAID is the PDF/A identification XMP namespace (ISO 19005, AIIM).
const nsPDFAID = "http://www.aiim.org/pdfa/ns/id/"

// ConvertToPDFA adjusts the document toward a PDF/A "b"-level conformance
// profile and returns a validation report describing any violations that
// remain. It applies the structural and metadata fixes the library can make
// safely in place:
//
//   - removes encryption;
//   - removes JavaScript and Launch actions (document-level and per-annotation);
//   - removes file attachments for PDF/A-1;
//   - sets annotation flags (Print on, Hidden/NoView off);
//   - adds an sRGB ICC OutputIntent (so device colours are colour-managed);
//   - writes an XMP packet carrying the pdfaid identifier (synced from /Info).
//
// It does NOT embed fonts that are not already embedded (Standard-14 fonts have
// no glyph program to embed — load a real font with LoadFont instead) and does
// not remove transparency for PDF/A-1. The returned report lists those and any
// other remaining issues; when Conformant is true the document satisfies the
// checks in ValidatePDFA. Mirrors the intent of Aspose.PDF for .NET's
// Document.Convert(PdfFormat).
//
// The changes are applied to the in-memory document; call Save/WriteTo to write
// the converted file.
func (d *Document) ConvertToPDFA(format PDFAFormat) (*PDFAValidationReport, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("ConvertToPDFA: document has no pages")
	}
	d.RemoveEncryption()
	d.stripPDFAActions()
	if format == PDFA1B {
		d.removePDFAEmbeddedFiles()
	}
	d.fixPDFAAnnotations()
	d.addSRGBOutputIntent()
	if err := d.setPDFAMetadata(format); err != nil {
		return nil, err
	}
	return d.ValidatePDFA(format), nil
}

func isPDFAForbiddenAction(dict pdfDict) bool {
	switch dictGetName(dict, "/S") {
	case "/JavaScript", "/Launch":
		return true
	}
	return false
}

// stripPDFAActions removes JavaScript and Launch actions from the catalog,
// annotations and the object table.
func (d *Document) stripPDFAActions() {
	if names, ok := resolveRefToDict(d.objects, d.catalog["/Names"]); ok {
		delete(names, "/JavaScript")
	}
	delete(d.catalog, "/AA")
	if act, ok := resolveRefToDict(d.objects, d.catalog["/OpenAction"]); ok && isPDFAForbiddenAction(act) {
		delete(d.catalog, "/OpenAction")
	}
	for _, page := range d.pages {
		pd, ok := page.Value.(pdfDict)
		if !ok {
			continue
		}
		annots, ok := resolveRefToArray(d.objects, pd["/Annots"])
		if !ok {
			continue
		}
		for _, a := range annots {
			ad, ok := resolveRefToDict(d.objects, a)
			if !ok {
				continue
			}
			delete(ad, "/AA")
			if act, ok := resolveRefToDict(d.objects, ad["/A"]); ok && isPDFAForbiddenAction(act) {
				delete(ad, "/A")
			}
		}
	}
	// Drop now-orphaned action objects so a re-scan sees no forbidden actions.
	for num, obj := range d.objects {
		if dict, ok := obj.Value.(pdfDict); ok && isPDFAForbiddenAction(dict) {
			delete(d.objects, num)
		}
	}
}

// removePDFAEmbeddedFiles drops the /Names/EmbeddedFiles name tree (PDF/A-1
// prohibits file attachments).
func (d *Document) removePDFAEmbeddedFiles() {
	if names, ok := resolveRefToDict(d.objects, d.catalog["/Names"]); ok {
		delete(names, "/EmbeddedFiles")
	}
}

// fixPDFAAnnotations sets the Print flag and clears Hidden/NoView on every
// non-Popup annotation.
func (d *Document) fixPDFAAnnotations() {
	const (
		flagHidden = 1 << 1
		flagPrint  = 1 << 2
		flagNoView = 1 << 5
	)
	for _, page := range d.pages {
		pd, ok := page.Value.(pdfDict)
		if !ok {
			continue
		}
		annots, ok := resolveRefToArray(d.objects, pd["/Annots"])
		if !ok {
			continue
		}
		for _, a := range annots {
			ad, ok := resolveRefToDict(d.objects, a)
			if !ok || dictGetName(ad, "/Subtype") == "/Popup" {
				continue
			}
			flags := toInt(resolveRef(d.objects, ad["/F"]))
			ad["/F"] = (flags | flagPrint) &^ (flagHidden | flagNoView)
		}
	}
}

// addSRGBOutputIntent adds an sRGB ICC OutputIntent to the catalog (idempotent).
func (d *Document) addSRGBOutputIntent() {
	if d.hasOutputIntent() {
		return
	}
	icc := srgbICCProfile()
	iccStream := &pdfStream{Dict: pdfDict{"/N": 3}, Data: icc, Decoded: true}
	iccID := d.nextID
	d.nextID++
	d.objects[iccID] = &pdfObject{Num: iccID, Value: iccStream}

	oi := pdfDict{
		"/Type":                      pdfName("/OutputIntent"),
		"/S":                         pdfName("/GTS_PDFA1"),
		"/OutputConditionIdentifier": encodeFormString("sRGB IEC61966-2.1"),
		"/Info":                      encodeFormString("sRGB IEC61966-2.1"),
		"/DestOutputProfile":         pdfRef{Num: iccID},
	}
	oiID := d.nextID
	d.nextID++
	d.objects[oiID] = &pdfObject{Num: oiID, Value: oi}

	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	d.catalog["/OutputIntents"] = pdfArray{pdfRef{Num: oiID}}
}

// setPDFAMetadata writes an XMP packet carrying the pdfaid identifier for the
// requested level, preserving existing XMP/Info-derived fields.
func (d *Document) setPDFAMetadata(format PDFAFormat) error {
	meta, _ := d.XMP()
	info, _ := d.Info()
	if meta.Title == "" {
		meta.Title = info.Title
	}
	if len(meta.Authors) == 0 && info.Author != "" {
		meta.Authors = []string{info.Author}
	}
	if meta.Producer == "" {
		meta.Producer = info.Producer
	}
	if meta.CreatorTool == "" {
		meta.CreatorTool = info.Creator
	}
	// Replace any existing pdfaid properties.
	var custom []XMPProperty
	for _, p := range meta.Custom {
		if p.Prefix != "pdfaid" {
			custom = append(custom, p)
		}
	}
	custom = append(custom,
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "part", Value: fmt.Sprintf("%d", format.part())},
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "conformance", Value: "B"},
	)
	meta.Custom = custom
	return d.SetXMP(meta)
}

// srgbICCProfile builds a minimal but valid ICC v2.1 RGB display profile for the
// sRGB colour space (D50-adapted primaries, gamma ~2.2 tone curves). Used as the
// DestOutputProfile of the PDF/A OutputIntent so no external file is required.
func srgbICCProfile() []byte {
	fix := func(f float64) uint32 { return uint32(int32(f*65536 + 0.5)) }
	be32 := func(b *bytes.Buffer, v uint32) {
		b.Write([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	xyz := func(x, y, z float64) []byte {
		var b bytes.Buffer
		b.WriteString("XYZ ")
		be32(&b, 0)
		be32(&b, fix(x))
		be32(&b, fix(y))
		be32(&b, fix(z))
		return b.Bytes()
	}
	curv := func() []byte {
		var b bytes.Buffer
		b.WriteString("curv")
		be32(&b, 0)
		be32(&b, 1)                 // one entry → a single u8Fixed8 gamma
		b.Write([]byte{0x02, 0x33}) // 2.2
		return b.Bytes()
	}
	desc := func(s string) []byte {
		var b bytes.Buffer
		b.WriteString("desc")
		be32(&b, 0)
		ascii := s + "\x00"
		be32(&b, uint32(len(ascii)))
		b.WriteString(ascii)
		be32(&b, 0)           // unicode language
		be32(&b, 0)           // unicode count
		b.Write([]byte{0, 0}) // macintosh script code
		b.WriteByte(0)        // macintosh count
		b.Write(make([]byte, 67))
		return b.Bytes()
	}
	text := func(s string) []byte {
		var b bytes.Buffer
		b.WriteString("text")
		be32(&b, 0)
		b.WriteString(s + "\x00")
		return b.Bytes()
	}

	curvData := curv()
	tags := []struct {
		sig  string
		data []byte
	}{
		{"desc", desc("sRGB IEC61966-2.1")},
		{"wtpt", xyz(0.9642, 1.0, 0.8249)},
		{"rXYZ", xyz(0.43607, 0.22249, 0.01392)},
		{"gXYZ", xyz(0.38515, 0.71687, 0.09708)},
		{"bXYZ", xyz(0.14307, 0.06061, 0.71410)},
		{"rTRC", curvData},
		{"gTRC", curvData},
		{"bTRC", curvData},
		{"cprt", text("Public Domain")},
	}

	n := len(tags)
	dataStart := 128 + 4 + 12*n
	offsets := map[string]uint32{}
	var dataBuf bytes.Buffer
	blockOffset := func(data []byte) (uint32, uint32) {
		key := string(data)
		if off, ok := offsets[key]; ok {
			return off, uint32(len(data))
		}
		off := uint32(dataStart + dataBuf.Len())
		offsets[key] = off
		dataBuf.Write(data)
		for dataBuf.Len()%4 != 0 {
			dataBuf.WriteByte(0)
		}
		return off, uint32(len(data))
	}

	var table bytes.Buffer
	be32(&table, uint32(n))
	for _, t := range tags {
		off, sz := blockOffset(t.data)
		table.WriteString(t.sig)
		be32(&table, off)
		be32(&table, sz)
	}

	totalSize := uint32(dataStart + dataBuf.Len())

	var h bytes.Buffer
	be32(&h, totalSize)   // profile size
	be32(&h, 0)           // preferred CMM
	be32(&h, 0x02100000)  // version 2.1.0
	h.WriteString("mntr") // device class: display
	h.WriteString("RGB ") // data colour space
	h.WriteString("XYZ ") // PCS
	h.Write(make([]byte, 12))
	h.WriteString("acsp") // file signature
	be32(&h, 0)           // platform
	be32(&h, 0)           // flags
	be32(&h, 0)           // device manufacturer
	be32(&h, 0)           // device model
	be32(&h, 0)
	be32(&h, 0) // device attributes (8 bytes)
	be32(&h, 0) // rendering intent: perceptual
	be32(&h, fix(0.9642))
	be32(&h, fix(1.0))
	be32(&h, fix(0.8249)) // PCS illuminant (D50)
	be32(&h, 0)           // profile creator
	h.Write(make([]byte, 44))

	var out bytes.Buffer
	out.Write(h.Bytes())
	out.Write(table.Bytes())
	out.Write(dataBuf.Bytes())
	return out.Bytes()
}
