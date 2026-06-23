// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"regexp"
	"strings"
)

// PDFAFormat identifies a PDF/A conformance level. The "b" (basic) levels cover
// visual reproducibility; the "a" (accessible) levels add the PDF/A "a"
// requirement of a Tagged PDF logical structure tree (author one with
// (*Document).TaggedContent). Mirrors the PDF/A members of Aspose.PDF for .NET's
// PdfFormat enum.
type PDFAFormat int

const (
	PDFA1B PDFAFormat = iota // PDF/A-1b (ISO 19005-1, PDF 1.4)
	PDFA2B                   // PDF/A-2b (ISO 19005-2, PDF 1.7)
	PDFA3B                   // PDF/A-3b (ISO 19005-3, PDF 1.7; allows embedded files)
	PDFA1A                   // PDF/A-1a (ISO 19005-1, accessible/tagged)
	PDFA2A                   // PDF/A-2a (ISO 19005-2, accessible/tagged)
	PDFA3A                   // PDF/A-3a (ISO 19005-3, accessible/tagged)
)

// String returns the human-readable conformance name, e.g. "PDF/A-1B".
func (f PDFAFormat) String() string {
	return fmt.Sprintf("PDF/A-%d%s", f.part(), f.conformance())
}

// part returns the PDF/A part number (1, 2 or 3) the format belongs to.
func (f PDFAFormat) part() int {
	switch f {
	case PDFA1B, PDFA1A:
		return 1
	case PDFA2B, PDFA2A:
		return 2
	case PDFA3B, PDFA3A:
		return 3
	default:
		return 0
	}
}

// conformance returns the conformance-level letter ("A" or "B").
func (f PDFAFormat) conformance() string {
	switch f {
	case PDFA1A, PDFA2A, PDFA3A:
		return "A"
	default:
		return "B"
	}
}

// PDFAIssue describes a single PDF/A conformance violation.
type PDFAIssue struct {
	Rule    string // short stable code, e.g. "FONT_NOT_EMBEDDED"
	Message string
}

// PDFAValidationReport is returned by (*Document).ValidatePDFA.
type PDFAValidationReport struct {
	Format PDFAFormat
	// Conformant is true when no violations were found for the requested level.
	Conformant bool
	Issues     []PDFAIssue
}

func (r *PDFAValidationReport) add(rule, msg string) {
	r.Conformant = false
	r.Issues = append(r.Issues, PDFAIssue{Rule: rule, Message: msg})
}

// ValidatePDFA checks the document against a PDF/A "b"-level conformance profile
// and returns a report of violations. It is a read-only diagnostic — it never
// mutates the document; use ConvertToPDFA to produce a conformant file.
//
// Checks performed (ISO 19005-1/2/3, basic level): an XMP metadata packet with a
// matching pdfaid:part/conformance; every font embedded (no Standard-14
// reliance); no encryption; no JavaScript or Launch actions; an ICC OutputIntent
// when device colours are used; no transparency for PDF/A-1; annotation flags
// (Print set, Hidden/NoView clear) and appearance streams; an uncompressed
// /Metadata stream; no LZWDecode (PDF/A-1); and no embedded files for PDF/A-1.
//
// Scope: the "a" (tagged/accessible) levels are not validated. This mirrors the
// intent of Aspose.PDF for .NET's Document.Validate(PdfFormat) but returns a
// structured report instead of writing an XML log.
func (d *Document) ValidatePDFA(format PDFAFormat) *PDFAValidationReport {
	r := &PDFAValidationReport{Format: format, Conformant: true}
	d.pdfaCheckXMP(format, r)
	d.pdfaCheckEncryption(r)
	d.pdfaCheckFonts(r)
	d.pdfaCheckActions(r)
	d.pdfaCheckColor(r)
	d.pdfaCheckTransparency(format, r)
	d.pdfaCheckAnnotations(r)
	d.pdfaCheckMetadata(r)
	d.pdfaCheckFilters(format, r)
	d.pdfaCheckEmbeddedFiles(format, r)
	d.pdfaCheckTagged(format, r)
	return r
}

var (
	rePDFAPartAttr = regexp.MustCompile(`pdfaid:part\s*=\s*["'](\d+)["']`)
	rePDFAPartElem = regexp.MustCompile(`<pdfaid:part\s*>\s*(\d+)`)
	rePDFAConfAttr = regexp.MustCompile(`pdfaid:conformance\s*=\s*["']([A-Za-z])["']`)
	rePDFAConfElem = regexp.MustCompile(`<pdfaid:conformance\s*>\s*([A-Za-z])`)
)

func (d *Document) pdfaCheckXMP(format PDFAFormat, r *PDFAValidationReport) {
	raw, err := d.XMPRaw()
	if err != nil || len(raw) == 0 {
		r.add("XMP_MISSING", "no XMP metadata packet (/Catalog/Metadata); PDF/A requires one carrying the pdfaid identifier")
		return
	}
	s := string(raw)
	part := firstSubmatch(s, rePDFAPartAttr, rePDFAPartElem)
	conf := firstSubmatch(s, rePDFAConfAttr, rePDFAConfElem)
	if part == "" || conf == "" {
		r.add("XMP_PDFAID_MISSING", "XMP packet lacks the pdfaid:part/pdfaid:conformance identifier required by PDF/A")
		return
	}
	if want := fmt.Sprintf("%d", format.part()); part != want {
		r.add("XMP_PART_MISMATCH", fmt.Sprintf("XMP pdfaid:part is %q but %s requires %q", part, format, want))
	}
	if want := format.conformance(); !strings.EqualFold(conf, want) {
		r.add("XMP_CONFORMANCE_MISMATCH", fmt.Sprintf("XMP pdfaid:conformance is %q but %s requires %q", conf, format, want))
	}
}

// pdfaCheckTagged enforces the additional Tagged-PDF requirements of the
// accessible ("a") conformance levels.
func (d *Document) pdfaCheckTagged(format PDFAFormat, r *PDFAValidationReport) {
	if format.conformance() != "A" {
		return
	}
	marked := false
	if mi, ok := resolveRefToDict(d.objects, d.catalog["/MarkInfo"]); ok {
		if b, ok := resolveRef(d.objects, mi["/Marked"]).(bool); ok && b {
			marked = true
		}
	}
	if !marked {
		r.add("NOT_TAGGED", fmt.Sprintf("%s requires a Tagged PDF (/MarkInfo /Marked true)", format))
	}
	if _, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"]); !ok {
		r.add("NO_STRUCT_TREE", fmt.Sprintf("%s requires a logical structure tree (/StructTreeRoot)", format))
	}
	if pdfStringValue(resolveRef(d.objects, d.catalog["/Lang"])) == "" {
		r.add("NO_LANG", fmt.Sprintf("%s requires a default natural language (/Catalog/Lang)", format))
	}
}

func firstSubmatch(s string, res ...*regexp.Regexp) string {
	for _, re := range res {
		if m := re.FindStringSubmatch(s); m != nil {
			return m[1]
		}
	}
	return ""
}

func (d *Document) pdfaCheckEncryption(r *PDFAValidationReport) {
	if d.preserved != nil || d.encrypt != nil {
		r.add("ENCRYPTED", "document is encrypted; PDF/A prohibits encryption")
	}
}

func (d *Document) pdfaCheckFonts(r *PDFAValidationReport) {
	for _, obj := range d.objects {
		dict, ok := obj.Value.(pdfDict)
		if !ok || dictGetName(dict, "/Type") != "/Font" {
			continue
		}
		switch dictGetName(dict, "/Subtype") {
		case "/CIDFontType0", "/CIDFontType2":
			continue // checked via the parent Type0 font
		}
		if !pdfaFontEmbedded(d.objects, dict) {
			name := dictGetName(dict, "/BaseFont")
			if name == "" {
				name = fmt.Sprintf("object %d", obj.Num)
			}
			r.add("FONT_NOT_EMBEDDED", fmt.Sprintf("font %s is not embedded; PDF/A requires all fonts to be embedded", name))
		}
	}
}

func pdfaFontEmbedded(objects map[int]*pdfObject, fontDict pdfDict) bool {
	switch dictGetName(fontDict, "/Subtype") {
	case "/Type3":
		return true // glyphs are content streams in /CharProcs
	case "/Type0":
		descs, ok := resolveRefToArray(objects, fontDict["/DescendantFonts"])
		if !ok || len(descs) == 0 {
			return false
		}
		cid, ok := resolveRefToDict(objects, descs[0])
		if !ok {
			return false
		}
		return fontDescriptorHasFile(objects, cid)
	default:
		return fontDescriptorHasFile(objects, fontDict)
	}
}

func fontDescriptorHasFile(objects map[int]*pdfObject, fontDict pdfDict) bool {
	fd, ok := resolveRefToDict(objects, fontDict["/FontDescriptor"])
	if !ok {
		return false
	}
	for _, k := range []string{"/FontFile", "/FontFile2", "/FontFile3"} {
		if _, ok := fd[k]; ok {
			return true
		}
	}
	return false
}

func (d *Document) pdfaCheckActions(r *PDFAValidationReport) {
	hasJS, hasLaunch := false, false
	flag := func(s string) {
		switch s {
		case "/JavaScript":
			hasJS = true
		case "/Launch":
			hasLaunch = true
		}
	}
	if names, ok := resolveRefToDict(d.objects, d.catalog["/Names"]); ok {
		if _, ok := names["/JavaScript"]; ok {
			hasJS = true
		}
	}
	if dict, ok := resolveRefToDict(d.objects, d.catalog["/OpenAction"]); ok {
		flag(dictGetName(dict, "/S"))
	}
	for _, obj := range d.objects {
		if dict, ok := obj.Value.(pdfDict); ok {
			flag(dictGetName(dict, "/S"))
		}
	}
	if hasJS {
		r.add("JAVASCRIPT", "document contains JavaScript; PDF/A prohibits JavaScript actions")
	}
	if hasLaunch {
		r.add("LAUNCH_ACTION", "document contains a Launch action; PDF/A prohibits launching external applications")
	}
}

func (d *Document) pdfaCheckColor(r *PDFAValidationReport) {
	if d.hasOutputIntent() {
		return
	}
	if op, page := d.firstDeviceColorUse(); op != "" {
		r.add("COLOR_NO_OUTPUT_INTENT", fmt.Sprintf("page %d uses a device colour (operator %q) but the document has no ICC OutputIntent; PDF/A requires a matching output intent", page, op))
	}
}

// hasOutputIntent reports whether the catalog declares an OutputIntent with a
// destination ICC profile.
func (d *Document) hasOutputIntent() bool {
	arr, ok := resolveRefToArray(d.objects, d.catalog["/OutputIntents"])
	if !ok {
		return false
	}
	for _, e := range arr {
		if oi, ok := resolveRefToDict(d.objects, e); ok {
			if _, ok := oi["/DestOutputProfile"]; ok {
				return true
			}
		}
	}
	return false
}

// firstDeviceColorUse scans page content streams for a device-colour-setting
// operator and returns the operator and 1-based page number, or "" if none.
func (d *Document) firstDeviceColorUse() (string, int) {
	deviceOps := map[string]bool{"rg": true, "RG": true, "g": true, "G": true, "k": true, "K": true}
	for i, page := range d.pages {
		dict, ok := page.Value.(pdfDict)
		if !ok {
			continue
		}
		for _, data := range d.pageContentStreams(dict) {
			ops, err := parseContentStream(data)
			if err != nil {
				continue
			}
			for _, op := range ops {
				if deviceOps[op.Operator] {
					return op.Operator, i + 1
				}
			}
		}
	}
	return "", 0
}

// pageContentStreams returns the decoded content-stream bytes for a page.
func (d *Document) pageContentStreams(pageDict pdfDict) [][]byte {
	var out [][]byte
	add := func(v pdfValue) {
		if s, ok := resolveRef(d.objects, v).(*pdfStream); ok {
			out = append(out, pdfaStreamBytes(s))
		}
	}
	switch c := pageDict["/Contents"].(type) {
	case pdfArray:
		for _, e := range c {
			add(e)
		}
	default:
		if arr, ok := resolveRefToArray(d.objects, pageDict["/Contents"]); ok {
			for _, e := range arr {
				add(e)
			}
		} else {
			add(pageDict["/Contents"])
		}
	}
	return out
}

func pdfaStreamBytes(s *pdfStream) []byte {
	if s.Decoded {
		return s.Data
	}
	if dec, err := decodeStream(s.Dict, s.Data); err == nil {
		return dec
	}
	return s.Data
}

func (d *Document) pdfaCheckTransparency(format PDFAFormat, r *PDFAValidationReport) {
	if format != PDFA1B {
		return // PDF/A-2 and -3 allow transparency
	}
	for _, obj := range d.objects {
		switch v := obj.Value.(type) {
		case pdfDict:
			if pdfaDictHasTransparency(d.objects, v) {
				r.add("TRANSPARENCY", "document uses transparency (group, blend mode, soft mask or constant alpha); PDF/A-1 prohibits transparency")
				return
			}
		case *pdfStream:
			if pdfaDictHasTransparency(d.objects, v.Dict) {
				r.add("TRANSPARENCY", "document uses transparency (group, blend mode, soft mask or constant alpha); PDF/A-1 prohibits transparency")
				return
			}
		}
	}
}

func pdfaDictHasTransparency(objects map[int]*pdfObject, dict pdfDict) bool {
	// Transparency group (page or Form XObject).
	if grp, ok := resolveRefToDict(objects, dict["/Group"]); ok {
		if dictGetName(grp, "/S") == "/Transparency" {
			return true
		}
	}
	// Soft mask — an image /SMask stream or an ExtGState /SMask other than /None.
	if sm, ok := dict["/SMask"]; ok {
		if pdfaResolveName(objects, sm) != "/None" {
			return true
		}
	}
	// Non-normal blend mode (ExtGState /BM).
	if bm, ok := dict["/BM"]; ok && pdfaBlendNonNormal(bm) {
		return true
	}
	// Constant alpha below 1 (ExtGState /ca, /CA).
	for _, k := range []string{"/ca", "/CA"} {
		if v, ok := dict[k]; ok {
			if f, err := toFloat(v); err == nil && f < 1.0 {
				return true
			}
		}
	}
	return false
}

// pdfaResolveName returns the name of v (resolving an indirect reference), or ""
// if v is not a name.
func pdfaResolveName(objects map[int]*pdfObject, v pdfValue) string {
	if n, ok := resolveRef(objects, v).(pdfName); ok {
		return string(n)
	}
	return ""
}

// pdfaBlendNonNormal reports whether a /BM value (a name or an array of names)
// selects any blend mode other than Normal/Compatible.
func pdfaBlendNonNormal(v pdfValue) bool {
	check := func(n pdfName) bool {
		s := string(n)
		return s != "" && s != "/Normal" && s != "/Compatible"
	}
	switch t := v.(type) {
	case pdfName:
		return check(t)
	case pdfArray:
		for _, e := range t {
			if n, ok := e.(pdfName); ok && check(n) {
				return true
			}
		}
	}
	return false
}

func (d *Document) pdfaCheckAnnotations(r *PDFAValidationReport) {
	const (
		flagHidden = 1 << 1 // bit 2
		flagPrint  = 1 << 2 // bit 3
		flagNoView = 1 << 5 // bit 6
	)
	reported := map[string]bool{}
	for i, page := range d.pages {
		dict, ok := page.Value.(pdfDict)
		if !ok {
			continue
		}
		annots, ok := resolveRefToArray(d.objects, dict["/Annots"])
		if !ok {
			continue
		}
		for _, a := range annots {
			ad, ok := resolveRefToDict(d.objects, a)
			if !ok {
				continue
			}
			sub := dictGetName(ad, "/Subtype")
			if sub == "/Popup" {
				continue
			}
			flags := toInt(resolveRef(d.objects, ad["/F"]))
			if flags&flagPrint == 0 && !reported["print"] {
				r.add("ANNOTATION_FLAGS", fmt.Sprintf("page %d has an annotation without the Print flag set; PDF/A requires annotations to be printable", i+1))
				reported["print"] = true
			}
			if flags&(flagHidden|flagNoView) != 0 && !reported["hidden"] {
				r.add("ANNOTATION_FLAGS", fmt.Sprintf("page %d has a Hidden or NoView annotation; PDF/A prohibits them", i+1))
				reported["hidden"] = true
			}
			if sub != "/Link" {
				if _, ok := resolveRefToDict(d.objects, ad["/AP"]); !ok {
					if !reported["ap"] {
						r.add("ANNOTATION_NO_AP", fmt.Sprintf("page %d has a %s annotation without a normal appearance (/AP/N); PDF/A requires one", i+1, strings.TrimPrefix(sub, "/")))
						reported["ap"] = true
					}
				}
			}
		}
	}
}

func (d *Document) pdfaCheckMetadata(r *PDFAValidationReport) {
	s, ok := resolveRefToStream(d.objects, d.catalog["/Metadata"])
	if !ok {
		return // absence is reported by the XMP check
	}
	if _, ok := s.Dict["/Filter"]; ok {
		r.add("METADATA_COMPRESSED", "the /Metadata stream is filtered; PDF/A requires the XMP packet to be stored uncompressed")
	}
}

func (d *Document) pdfaCheckFilters(format PDFAFormat, r *PDFAValidationReport) {
	if format != PDFA1B {
		return // LZWDecode is permitted in PDF/A-2 and -3
	}
	for _, obj := range d.objects {
		s, ok := obj.Value.(*pdfStream)
		if !ok {
			continue
		}
		if pdfaFilterHas(s.Dict["/Filter"], "/LZWDecode") {
			r.add("LZW_FILTER", "document uses LZWDecode; PDF/A-1 prohibits the LZW filter")
			return
		}
	}
}

func pdfaFilterHas(v pdfValue, name string) bool {
	switch f := v.(type) {
	case pdfName:
		return string(f) == name
	case pdfArray:
		for _, e := range f {
			if n, ok := e.(pdfName); ok && string(n) == name {
				return true
			}
		}
	}
	return false
}

func (d *Document) pdfaCheckEmbeddedFiles(format PDFAFormat, r *PDFAValidationReport) {
	if format != PDFA1B {
		return // PDF/A-2 allows PDF/A attachments; PDF/A-3 allows any
	}
	if names, ok := resolveRefToDict(d.objects, d.catalog["/Names"]); ok {
		if _, ok := names["/EmbeddedFiles"]; ok {
			r.add("EMBEDDED_FILES", "document has embedded files; PDF/A-1 prohibits file attachments (use PDF/A-3 for attachments)")
		}
	}
}
