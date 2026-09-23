// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"
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
//   - embeds non-embedded simple fonts (Standard-14 and other single-byte
//     Type1/TrueType) using the bundled metric-compatible substitutes;
//   - adds an sRGB ICC OutputIntent (so device colours are colour-managed);
//   - writes an XMP packet carrying the pdfaid identifier (synced from /Info).
//
// Symbol and ZapfDingbats have no Latin substitute and remain a reported
// violation; composite (Type0/CJK) and Type3 fonts, and PDF/A-1 transparency,
// are not auto-fixed. The returned report lists any remaining issues; when
// Conformant is true the document satisfies the checks in ValidatePDFA. Mirrors
// the intent of Aspose.PDF for .NET's Document.Convert(PdfFormat).
//
// The changes are applied to the in-memory document; call Save/WriteTo to write
// the converted file.
func (d *Document) ConvertToPDFA(format PDFAFormat) (*PDFAValidationReport, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("ConvertToPDFA: document has no pages")
	}
	// PDF/A-1 is built on PDF 1.4, which has no object streams: clear a
	// CompressObjects set earlier as a convenience. The actual enforcement
	// lives in the writer (buildDocumentPDF, via isPDFA1), which checks the
	// document's XMP at write time — so the guard also holds if Optimize runs
	// again after this call, or for a document opened from an existing
	// PDF/A-1 file that never went through ConvertToPDFA at all.
	if format.part() == 1 {
		d.compressObjects = false
	}
	d.RemoveEncryption()
	d.stripPDFAActions()
	if format.part() == 1 {
		d.removePDFAEmbeddedFiles()
	}
	if format.part() == 3 {
		d.associatePDFAEmbeddedFiles()
	}
	d.fixPDFAAnnotations()
	d.generatePDFAAppearances()
	d.clearNeedAppearances()
	d.embedStandardFonts()
	d.embedCompositeFonts()
	d.addSRGBOutputIntent()
	if err := d.setPDFAMetadata(format); err != nil {
		return nil, err
	}
	return d.ValidatePDFA(format), nil
}

// embedStandardFonts replaces every non-embedded simple font (Standard-14 or any
// other single-byte Type1/TrueType) with an embedded TrueType built from the
// bundled metric-compatible substitute (Arimo↔Helvetica, Tinos↔Times,
// Cousine↔Courier, …) so the document carries its own glyph programs. The
// existing character codes and encoding are preserved, so content streams are
// untouched. Symbol and ZapfDingbats have no Latin substitute and are left as a
// reported violation; composite (Type0) and Type3 fonts are out of scope.
func (d *Document) embedStandardFonts() {
	used := d.pdfaUsedFonts()
	for _, obj := range d.objects {
		dict, ok := obj.Value.(pdfDict)
		if !ok || dictGetName(dict, "/Type") != "/Font" {
			continue
		}
		switch dictGetName(dict, "/Subtype") {
		case "/Type1", "/TrueType", "/MMType1":
		default:
			continue
		}
		if fontDescriptorHasFile(d.objects, dict) {
			continue // already embedded
		}
		if !used[obj.Num] {
			// Nothing draws with it, so PDF/A does not ask for it to be
			// embedded and a font program would only add weight.
			continue
		}
		fi := resolveFont(d.objects, dict)
		f := fallbackFontFor(fi)
		if f == nil {
			f = symbolFaceFor(fi)
		}
		if f == nil {
			continue // no substitute and no installed face to embed
		}
		obj.Value = d.buildEmbeddedSimpleFont(dict, fi, f)
	}
}

// symbolFaceFor finds a face to embed for a font the bundled metric-compatible
// clones do not cover — Symbol and ZapfDingbats, whose glyphs no Latin
// substitute carries. A face registered through AddFontFile/AddFontFolder wins;
// otherwise an installed one of the same name is used, as Acrobat does. The
// font's own embedding permissions are respected, so a face its vendor marks
// as non-embeddable is left alone and the document keeps the violation.
func symbolFaceFor(fi fontInfo) *ttfFont {
	f := fontRepo.find(fi)
	if f == nil {
		f = fontRepo.findSystemExact(fi)
	}
	if f == nil {
		f = fontRepo.findSystemFamily(normalizeFontName(fi.name))
	}
	if f == nil || !f.embeddingAllowed() {
		return nil
	}
	return f
}

// buildEmbeddedSimpleFont constructs an embedded simple TrueType font dict that
// substitutes f for the non-embedded font orig, preserving its character codes,
// encoding and per-code widths (computed from f's own glyph advances so the
// dictionary and font program agree, as PDF/A requires).
func (d *Document) buildEmbeddedSimpleFont(orig pdfDict, fi fontInfo, f *ttfFont) pdfDict {
	scale := func(w uint16) int {
		return int(float64(w)*1000.0/float64(f.unitsPerEm) + 0.5)
	}
	widths := make(pdfArray, 256)
	for c := 0; c < 256; c++ {
		w := 0
		if gid := substituteGlyphFor(f, fi, c); int(gid) < len(f.glyphWidths) {
			w = scale(f.glyphWidths[gid])
		}
		widths[c] = w
	}
	fontFileID := d.addObject(buildFontFile2Stream(f))
	descID := d.addObject(buildFontDescriptor(f, "/FontFile2", fontFileID))

	out := pdfDict{
		"/Type":           pdfName("/Font"),
		"/Subtype":        pdfName("/TrueType"),
		"/BaseFont":       pdfName("/" + f.postScriptName),
		"/FirstChar":      0,
		"/LastChar":       255,
		"/Widths":         widths,
		"/FontDescriptor": pdfRef{Num: descID},
	}
	if enc, ok := orig["/Encoding"]; ok {
		out["/Encoding"] = enc // keep the original code→glyph mapping
	} else {
		out["/Encoding"] = pdfName("/WinAnsiEncoding")
	}
	return out
}

// substituteGlyphFor maps one character code of the original font onto a glyph
// of the substitute. A text face is reached through Unicode; a symbol face has
// no Unicode cmap at all — the installed Symbol and Dingbats fonts carry a
// (3,0) "symbol" cmap keyed by the raw code in the 0xF000 page — so the code
// itself is looked up when the character does not resolve.
func substituteGlyphFor(f *ttfFont, fi fontInfo, code int) uint16 {
	if r := fi.encoding[code]; r != 0 {
		if gid := f.glyphID(r); gid != 0 {
			return gid
		}
	}
	if gid, ok := f.codeToGlyph[uint16(0xF000|code)]; ok {
		return gid
	}
	if gid, ok := f.codeToGlyph[uint16(code)]; ok {
		return gid
	}
	return 0
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

// removePDFAEmbeddedFiles drops the /Names/EmbeddedFiles name tree and the
// catalog /AF array (PDF/A-1 prohibits file attachments). Both are needed:
// /AF keeps its file specifications — and through them the embedded file
// streams — reachable, so dropping only the name tree would leave the
// attachment in the saved file, merely harder to find.
func (d *Document) removePDFAEmbeddedFiles() {
	if names, ok := resolveRefToDict(d.objects, d.catalog["/Names"]); ok {
		delete(names, "/EmbeddedFiles")
	}
	delete(d.catalog, "/AF")

	// Detaching the entries leaves the file specifications and their embedded
	// file streams in the object set, so the writer would still put the
	// attachment's bytes in the saved file. Drop the ones nothing references
	// any more — a filespec still reached from a page (a file-attachment
	// annotation) keeps its stream.
	reachable := collectReachableIDs(d.objects, d.pages)
	for key, v := range d.catalog {
		if catalogKeyRebuiltByWriter(d, key) {
			continue
		}
		markReachable(d.objects, v, reachable)
	}
	for num, obj := range d.objects {
		if reachable[num] {
			continue
		}
		var dict pdfDict
		switch v := obj.Value.(type) {
		case pdfDict:
			dict = v
		case *pdfStream:
			dict = v.Dict
		}
		switch dictGetName(dict, "/Type") {
		case "/Filespec", "/EmbeddedFile":
			delete(d.objects, num)
		}
	}
}

// associatePDFAEmbeddedFiles gives every attachment the association PDF/A-3
// requires: an existing /AFRelationship is kept, a missing one becomes
// Unspecified, and each file is listed in the catalog /AF with a /ModDate in
// its parameters.
func (d *Document) associatePDFAEmbeddedFiles() {
	for _, f := range d.EmbeddedFiles().All() {
		f.SetAFRelationship(f.AFRelationship())
		if st := f.stream(); st != nil {
			params, _ := st.Dict["/Params"].(pdfDict)
			if params == nil {
				params = pdfDict{}
				st.Dict["/Params"] = params
			}
			if _, ok := params["/ModDate"]; !ok {
				params["/ModDate"] = pdfDateString(time.Now())
			}
		}
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

// generatePDFAAppearances draws the normal appearance (/AP/N) of every
// annotation that lacks one. PDF/A requires each visible annotation but a Link
// to carry its own appearance, because a conforming reader is not allowed to
// synthesize one — a checkbox with no /AP would simply disappear in an
// archival viewer. Form-field widgets are drawn by the AcroForm appearance
// generators, and the drawing and markup annotations regenerate their own.
// Icon annotations (/Text sticky notes, /FileAttachment) draw no appearance of
// their own — viewers supply the icon — and stay a reported violation, as do
// subtypes this library does not draw.
func (d *Document) generatePDFAAppearances() {
	var form *Form
	for _, page := range d.Pages() {
		for _, a := range page.Annotations().All() {
			base := a.annotationBaseRef()
			if base == nil || base.dict == nil {
				continue
			}
			switch dictGetName(base.dict, "/Subtype") {
			case "/Link", "/Popup":
				continue
			case "/Widget":
				if _, ok := resolveRefToDict(d.objects, base.dict["/AP"]); ok {
					continue
				}
				if form == nil {
					form = d.Form()
				}
				regenerateWidgetAppearance(form, base.dict)
				continue
			}
			if _, ok := resolveRefToDict(d.objects, base.dict["/AP"]); ok {
				continue
			}
			if r, ok := a.(interface{ RegenerateAppearance() }); ok {
				r.RegenerateAppearance()
			}
		}
	}
}

// clearNeedAppearances turns off the AcroForm flag that asks a viewer to build
// field appearances itself. PDF/A forbids it: an archival file has to carry
// the appearances it is meant to show, which generatePDFAAppearances has just
// made sure of.
func (d *Document) clearNeedAppearances() {
	acro, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"])
	if !ok {
		return
	}
	delete(acro, "/NeedAppearances")
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
	var extensions []string
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		extensions, _ = xmpExtensionBlocks(string(raw))
	}
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
	// Replace the pdfaid properties, and drop anything else in the PDF/A
	// namespaces: extension schemas travel as whole blocks (below), never as
	// loose properties.
	var custom []XMPProperty
	for _, p := range meta.Custom {
		if p.Prefix != "pdfaid" && !strings.HasPrefix(p.Namespace, nsPDFAPrefix) {
			custom = append(custom, p)
		}
	}
	custom = append(custom,
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "part", Value: fmt.Sprintf("%d", format.part())},
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "conformance", Value: format.conformance()},
	)
	meta.Custom = preferExtensionSchemaPrefixes(custom, extensions)
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	if len(extensions) == 0 {
		return nil
	}
	raw, err := d.XMPRaw()
	if err != nil {
		return err
	}
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), extensions)))
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
