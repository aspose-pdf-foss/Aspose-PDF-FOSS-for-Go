// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// PDF/A-4 (ISO 19005-4) is built on PDF 2.0, which deprecates the /Info dictionary
// and leaves the document's metadata to XMP. It has no a, b or u levels: plain,
// e (engineering: 3D and RichMedia allowed) and f (any attachment, at least one).
// It also relaxes much of parts 1-3: JavaScript, some additional actions, XMP
// properties no extension schema declares and the implementation limits are no
// longer ruled out, so what the earlier parts remove stays.

const nsPDFX = "http://ns.adobe.com/pdfx/1.3/"

// pdfaAdditionalActionKeys are the triggers ISO 19005-4 §6.6.3 leaves on the
// catalog, a page or an annotation that is not a form field's widget.
var pdfaAdditionalActionKeys = map[string]bool{"/E": true, "/X": true, "/D": true, "/U": true, "/Fo": true, "/Bl": true}

// isFormFieldDict reports a form field or its widget, whose additional actions
// ISO 19005-4 leaves alone.
func isFormFieldDict(dict pdfDict) bool {
	sub := dictGetName(dict, "/Subtype")
	if sub != "" {
		return sub == "/Widget"
	}
	return dict["/FT"] != nil || dict["/T"] != nil
}

// filterAdditionalActions keeps of dict's /AA what PDF/A-4 allows. The dictionary is
// replaced, not edited, since another object may share it.
func (d *Document) filterAdditionalActions(dict pdfDict, format PDFAFormat) {
	aa, ok := resolveRefToDict(d.objects, dict["/AA"])
	if !ok {
		return
	}
	field := isFormFieldDict(dict)
	kept := pdfDict{}
	for k, v := range aa {
		if !field && !pdfaAdditionalActionKeys[k] {
			continue
		}
		if a, ok := resolveRefToDict(d.objects, v); ok && isPDFAForbiddenAction(a, format) {
			continue
		}
		kept[k] = v
	}
	if len(kept) == 0 {
		delete(dict, "/AA")
	} else if len(kept) != len(aa) {
		dict["/AA"] = kept
	}
}

// moveInfoToXMP empties the /Info dictionary, which PDF/A-4 forbids unless the
// catalog has /PieceInfo, and then allows only /ModDate. Everything else /Info holds
// is in the XMP by now (reconcileInfoAndXMP) but its custom entries, which go under
// the namespace Adobe's own producers use for them.
func (d *Document) moveInfoToXMP(meta *XMPMetadata) {
	d.moveCustomInfoToXMP(meta)
	modDate, hasDate := d.info["/ModDate"]
	_, hasPieceInfo := d.catalog["/PieceInfo"]
	d.info = nil
	if hasPieceInfo && hasDate {
		d.info = pdfDict{"/ModDate": modDate}
	}
}

// moveCustomInfoToXMP copies the /Info entries outside the standard fields into
// meta under the pdfx namespace; the standard ones reconcileInfoAndXMP covers.
func (d *Document) moveCustomInfoToXMP(meta *XMPMetadata) {
	if info, err := d.Info(); err == nil {
		for _, k := range slices.Sorted(maps.Keys(info.Custom)) {
			// A producer may already have written it there, and XMP allows a property once.
			name := xmlPropertyName(k)
			if !slices.ContainsFunc(meta.Custom, func(p XMPProperty) bool { return p.Namespace == nsPDFX && p.Name == name }) {
				meta.Custom = append(meta.Custom, XMPProperty{Namespace: nsPDFX, Prefix: "pdfx", Name: name, Value: info.Custom[k]})
			}
		}
	}
}

var reNotXMLName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// xmlPropertyName makes an /Info key usable as an XML element name.
func xmlPropertyName(key string) string {
	name := reNotXMLName.ReplaceAllString(key, "_")
	if name == "" || !(name[0] == '_' || name[0] >= 'A' && name[0] <= 'Z' || name[0] >= 'a' && name[0] <= 'z') {
		name = "_" + name
	}
	return name
}

// fixPDFA4 repairs what only PDF/A-4 asks: the catalog /Version must name PDF 2.x,
// and a graphics state may not hold a device-specific halftone origin.
func (d *Document) fixPDFA4() {
	if !reCatalogVersion2.MatchString(dictGetName(d.catalog, "/Version")) {
		delete(d.catalog, "/Version")
	}
	d.pdfaWalkDicts(func(dict pdfDict) {
		delete(dict, "/HTO")
	})
}

var reCatalogVersion2 = regexp.MustCompile(`^/2\.[0-9]$`)

// pdfaCheck4 holds the rules only PDF/A-4 has.
func (d *Document) pdfaCheck4(format PDFAFormat, r *PDFAValidationReport) {
	_, hasPieceInfo := d.catalog["/PieceInfo"]
	if len(d.info) > 0 && !hasPieceInfo {
		r.add("INFO_FORBIDDEN", "the document has an /Info dictionary; PDF/A-4 allows one only with /PieceInfo in the catalog (ISO 19005-4 §6.1.3)")
	}
	if _, hasDate := d.info["/ModDate"]; len(d.info) > 0 && (len(d.info) != 1 || !hasDate) && hasPieceInfo {
		r.add("INFO_FORBIDDEN", "the /Info dictionary holds more than /ModDate, all PDF/A-4 allows in it (ISO 19005-4 §6.1.3)")
	}
	if v := dictGetName(d.catalog, "/Version"); v != "" && !reCatalogVersion2.MatchString(v) {
		r.add("CATALOG_VERSION", fmt.Sprintf("the catalog /Version is %s; PDF/A-4 requires 2.x (ISO 19005-4 §6.1.12)", v))
	}
	d.pdfaCheckOutputIntentRef(r)
	d.pdfaCheckDuplicateXMP(r)
	d.pdfaCheck4Dicts(format, r)
	for _, gf := range d.glyphFontsInUse() {
		if len(gf.invalid) > 0 {
			slices.Sort(gf.invalid)
			r.add("INVALID_UNICODE_VALUE", fmt.Sprintf("font %s: /ToUnicode maps code %d to 0, U+FEFF or U+FFFE, %d distinct codes in all", gf.name, gf.invalid[0], len(gf.invalid)))
		}
	}
	d.pdfaCheck4Content(r)
	d.pdfaCheck4Attachments(format, r)
}

// pdfaCheckDuplicateXMP reports a property the packet states twice, which XMP does
// not allow, so a reader rejects the whole packet and the identifier in it.
func (d *Document) pdfaCheckDuplicateXMP(r *PDFAValidationReport) {
	raw, err := d.XMPRaw()
	if err != nil {
		return
	}
	if name := repeatedXMPProperty(raw); name != "" {
		r.add("XMP_MALFORMED", fmt.Sprintf("the XMP packet states %s more than once", name))
	}
}

// repeatedXMPProperty returns the first top-level property stated twice, or "".
func repeatedXMPProperty(raw []byte) string {
	const nsRDF = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimRight(raw, "\x00 \t\r\n")))
	var stack []xml.Name
	seen := map[xml.Name]bool{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if n := len(stack); n >= 2 && stack[n-1] == (xml.Name{Space: nsRDF, Local: "Description"}) && stack[n-2] == (xml.Name{Space: nsRDF, Local: "RDF"}) {
				if seen[t.Name] {
					return t.Name.Local
				}
				seen[t.Name] = true
			}
			stack = append(stack, t.Name)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
}

func (d *Document) pdfaCheckOutputIntentRef(r *PDFAValidationReport) {
	arr, _ := resolveRefToArray(d.objects, d.catalog["/OutputIntents"])
	for _, e := range arr {
		if oi, ok := resolveRefToDict(d.objects, e); ok && oi["/DestOutputProfileRef"] != nil {
			r.add("OUTPUT_INTENT_PROFILE_REF", "an output intent has /DestOutputProfileRef; PDF/A-4 requires the profile to be embedded (ISO 19005-4 §6.2.3)")
			return
		}
	}
}

// pdfaCheck4Dicts reports what a dictionary anywhere in the file can hold.
func (d *Document) pdfaCheck4Dicts(format PDFAFormat, r *PDFAValidationReport) {
	badTrigger, badAction, hto, pua := "", "", false, false
	d.pdfaWalkDicts(func(dict pdfDict) {
		hto = hto || dict["/HTO"] != nil
		pua = pua || hasPrivateUse(decodeFormString(pdfStringValue(resolveRef(d.objects, dict["/ActualText"]))))
		aa, ok := resolveRefToDict(d.objects, dict["/AA"])
		if !ok {
			return
		}
		field := isFormFieldDict(dict)
		for _, k := range slices.Sorted(maps.Keys(aa)) {
			if !field && !pdfaAdditionalActionKeys[k] && badTrigger == "" {
				badTrigger = k
			}
			if a, ok := resolveRefToDict(d.objects, aa[k]); ok && isPDFAForbiddenAction(a, format) && badAction == "" {
				badAction = dictGetName(a, "/S")
			}
		}
	})
	if badTrigger != "" {
		r.add("ADDITIONAL_ACTIONS", fmt.Sprintf("an additional-actions dictionary has the trigger %s; PDF/A-4 allows only E, X, D, U, Fo and Bl outside form fields (ISO 19005-4 §6.6.3)", badTrigger))
	}
	if badAction != "" {
		r.add("FORBIDDEN_ACTION", fmt.Sprintf("an additional action is a %s action, which PDF/A-4 does not allow", badAction))
	}
	if hto {
		r.add("EXTGSTATE_HTO", "a graphics state has /HTO; PDF/A-4 forbids it (ISO 19005-4 §6.2.5)")
	}
	if pua {
		r.add("ACTUALTEXT_PRIVATE_USE", "an /ActualText holds a Private Use Area character, which PDF/A-4 forbids (ISO 19005-4 §6.2.10.8)")
	}
}

// pdfaCheck4Content reports what a content stream asks for and its resources do not
// define, and an /ActualText written inline in marked content.
func (d *Document) pdfaCheck4Content(r *PDFAValidationReport) {
	undefined, pua := "", false
	d.forEachContentStream(func(data []byte, res pdfDict) {
		ops, err := parseContentStream(data)
		if err != nil {
			return
		}
		for _, u := range contentResourceUses(ops) {
			names, _ := resolveRefToDict(d.objects, res[u.category])
			// A reference to no object is written as null, which defines nothing.
			if !d.resourceDefined(names[u.name], u.category) && (undefined == "" || u.name < undefined) {
				undefined = u.name
			}
		}
		for _, op := range ops {
			if op.Operator == "BDC" {
				if props, ok := at(op, 1).(pdfDict); ok {
					pua = pua || hasPrivateUse(decodeFormString(pdfStringValue(props["/ActualText"])))
				}
			}
		}
	})
	if undefined != "" {
		r.add("UNDEFINED_RESOURCE", fmt.Sprintf("a content stream uses %s, which no resource dictionary defines; PDF/A-4 requires them all to be (ISO 19005-4 §6.2.2)", strings.TrimPrefix(undefined, "/")))
	}
	if pua {
		r.add("ACTUALTEXT_PRIVATE_USE", "an /ActualText holds a Private Use Area character, which PDF/A-4 forbids (ISO 19005-4 §6.2.10.8)")
	}
}

// resourceDefined reports whether a resource entry leads to something usable: a
// reference to no object is written as null, and an XObject needs a known /Subtype
// to be one at all.
func (d *Document) resourceDefined(v pdfValue, category string) bool {
	res := resolveRef(d.objects, v)
	if _, dangling := res.(pdfRef); res == nil || dangling {
		return false
	}
	if category != "/XObject" {
		return true
	}
	st, ok := res.(*pdfStream)
	return ok && (dictGetName(st.Dict, "/Subtype") == "/Image" || dictGetName(st.Dict, "/Subtype") == "/Form")
}

func hasPrivateUse(s string) bool {
	for _, c := range s {
		if c >= 0xE000 && c <= 0xF8FF || c >= 0xF0000 && c <= 0x10FFFD {
			return true
		}
	}
	return false
}

// pdfaCheck4Attachments enforces ISO 19005-4 §6.9. Unlike PDF/A-3 it does not ask for
// a listing in the catalog /AF, only for the relationship on the file specification.
func (d *Document) pdfaCheck4Attachments(format PDFAFormat, r *PDFAValidationReport) {
	reported := false
	d.pdfaWalkDicts(func(dict pdfDict) {
		if _, hasRel := dict["/AFRelationship"].(pdfName); !reported && dict["/EF"] != nil && !hasRel {
			reported = true
			r.add("EMBEDDED_FILE_NOT_ASSOCIATED", "an embedded file's specification has no /AFRelationship; PDF/A-4 requires one (ISO 19005-4 §6.9)")
		}
	})
	for _, s := range d.embeddedFileStreams() {
		if !validEmbeddedFileMIME(s) {
			r.add("EMBEDDED_FILE_NO_MIME", fmt.Sprintf("an embedded file's /Subtype is %q, not a MIME type; PDF/A-4 requires one, application/octet-stream if not known", dictGetName(s.Dict, "/Subtype")))
		}
	}
	switch format {
	case PDFA4:
		d.pdfaCheckAttachedPDFA(format, r)
	case PDFA4F:
		names, _ := resolveRefToDict(d.objects, d.catalog["/Names"])
		if names["/EmbeddedFiles"] == nil {
			r.add("EMBEDDED_FILES_REQUIRED", "PDF/A-4f requires an embedded file in the catalog's /Names /EmbeddedFiles (ISO 19005-4 §6.9); without one use PDF/A-4")
		}
	}
}

// ttfHasCmap reports an encoding record for platform and encoding in an sfnt's
// cmap table.
func ttfHasCmap(data []byte, platform, encoding uint16) bool {
	b := tableSlice(data, tableDir(data), "cmap")
	if len(b) < 4 {
		return false
	}
	for i, n := 0, int(binary.BigEndian.Uint16(b[2:4])); i < n && 4+8*(i+1) <= len(b); i++ {
		rec := b[4+8*i:]
		if binary.BigEndian.Uint16(rec) == platform && binary.BigEndian.Uint16(rec[2:]) == encoding {
			return true
		}
	}
	return false
}
