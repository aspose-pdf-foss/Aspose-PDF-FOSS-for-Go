// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"maps"
	"slices"
)

// PDFUAIssue describes a single PDF/UA (accessibility) conformance violation.
type PDFUAIssue struct {
	Rule    string // short stable code, e.g. "UA_FIGURE_NO_ALT"
	Message string
}

// PDFUAValidationReport is returned by (*Document).ValidatePDFUA.
type PDFUAValidationReport struct {
	// Conformant is true when no violations were found.
	Conformant bool
	Issues     []PDFUAIssue
}

func (r *PDFUAValidationReport) add(rule, msg string) {
	r.Conformant = false
	r.Issues = append(r.Issues, PDFUAIssue{Rule: rule, Message: msg})
}

// ValidatePDFUA checks the document against a subset of PDF/UA-1 (ISO 14289-1,
// universal accessibility) and returns a report of violations. It is a
// read-only diagnostic — it never mutates the document.
//
// Checks performed: the document is marked as Tagged PDF (/MarkInfo /Marked) and
// carries a logical structure tree (/StructTreeRoot with kids and a
// /ParentTree); a natural-language default (/Lang); an XMP dc:title that is shown
// rather than the file name (/ViewerPreferences /DisplayDocTitle); the XMP
// pdfuaid identifier; every font used for rendering is embedded; every /Figure
// (and /Formula) structure element has alternate text (/Alt or /ActualText); every
// /TH names its /Scope; headings start at H1 and skip no level; and accessibility
// is not blocked by encryption permissions. ConvertToPDFUA repairs what can be
// repaired without knowing the content.
//
// Scope: this does not verify reading-order correctness or that every content
// item is tagged. Those need deeper semantic analysis (and a real validator such
// as PAC or veraPDF for final sign-off). Mirrors the
// intent of Aspose.PDF for .NET's Document.Validate(PdfFormat.PDF_UA_1).
func (d *Document) ValidatePDFUA() *PDFUAValidationReport {
	r := &PDFUAValidationReport{Conformant: true}
	d.uaCheckTagged(r)
	d.uaCheckLang(r)
	d.uaCheckTitle(r)
	d.uaCheckFigures(r)
	d.uaCheckAccessibility(r)
	d.uaCheckIdentification(r)
	d.uaCheckFonts(r)
	d.uaCheckStructure(r)
	if rule, msg := d.sharedConformanceIssues(); rule != "" {
		r.add("UA_"+rule, msg)
	}
	return r
}

func (d *Document) uaCheckTagged(r *PDFUAValidationReport) {
	marked := false
	if mi, ok := resolveRefToDict(d.objects, d.catalog["/MarkInfo"]); ok {
		if b, ok := resolveRef(d.objects, mi["/Marked"]).(bool); ok && b {
			marked = true
		}
	}
	if !marked {
		r.add("UA_NOT_TAGGED", "catalog has no /MarkInfo with /Marked true; PDF/UA requires a Tagged PDF")
	}
	root, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	if !ok {
		r.add("UA_NO_STRUCT_TREE", "no /StructTreeRoot; PDF/UA requires a logical structure tree")
		return
	}
	if len(d.structElemKids(root)) == 0 {
		r.add("UA_STRUCT_TREE_EMPTY", "the structure tree has no structure elements")
	}
	if _, ok := root["/ParentTree"]; !ok {
		r.add("UA_NO_PARENT_TREE", "/StructTreeRoot has no /ParentTree mapping marked content back to structure")
	}
}

func (d *Document) uaCheckLang(r *PDFUAValidationReport) {
	if s := pdfStringValue(resolveRef(d.objects, d.catalog["/Lang"])); s == "" {
		r.add("UA_NO_LANG", "catalog has no /Lang; PDF/UA requires a default natural language")
	}
}

func (d *Document) uaCheckTitle(r *PDFUAValidationReport) {
	title := ""
	if meta, err := d.XMP(); err == nil {
		title = meta.Title
	}
	if title == "" {
		r.add("UA_NO_TITLE", "document has no XMP dc:title; PDF/UA requires one")
	}
	display := false
	if vp, ok := resolveRefToDict(d.objects, d.catalog["/ViewerPreferences"]); ok {
		if b, ok := resolveRef(d.objects, vp["/DisplayDocTitle"]).(bool); ok && b {
			display = true
		}
	}
	if !display {
		r.add("UA_DISPLAY_DOCTITLE", "/ViewerPreferences /DisplayDocTitle is not true; viewers will show the file name instead of the title")
	}
}

func (d *Document) uaCheckFigures(r *PDFUAValidationReport) {
	root, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	if !ok {
		return // already reported by uaCheckTagged
	}
	roleMap, _ := resolveRefToDict(d.objects, root["/RoleMap"])
	reported := map[string]bool{}
	var walk func(elem pdfDict)
	walk = func(elem pdfDict) {
		std := resolveStructType(roleMap, dictGetName(elem, "/S"))
		if (std == "/Figure" || std == "/Formula") && !reported[std] {
			if !hasAltText(d.objects, elem) {
				rule := "UA_FIGURE_NO_ALT"
				what := "figure"
				if std == "/Formula" {
					rule, what = "UA_FORMULA_NO_ALT", "formula"
				}
				r.add(rule, fmt.Sprintf("a %s structure element has no alternate text (/Alt or /ActualText); PDF/UA requires it", what))
				reported[std] = true
			}
		}
		for _, kid := range d.structElemKids(elem) {
			walk(kid)
		}
	}
	for _, kid := range d.structElemKids(root) {
		walk(kid)
	}
}

// uaCheckIdentification reports a file that does not claim PDF/UA (ISO 14289-1 §5).
func (d *Document) uaCheckIdentification(r *PDFUAValidationReport) {
	if meta, err := d.XMP(); err == nil {
		for _, p := range meta.Custom {
			if p.Namespace == nsPDFUAID && p.Name == "part" && p.Value == "1" {
				return
			}
		}
	}
	r.add("UA_NO_IDENTIFICATION", "XMP has no pdfuaid:part 1; a PDF/UA file must identify itself")
}

// uaCheckFonts reports a font used for rendering that is not embedded (ISO 14289-1
// §7.21.4.1).
func (d *Document) uaCheckFonts(r *PDFUAValidationReport) {
	used := d.pdfaUsedFonts()
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		font, ok := d.objects[id].Value.(pdfDict)
		if !ok || !used[id] || pdfaFontEmbedded(d.objects, font) {
			continue
		}
		r.add("UA_FONT_NOT_EMBEDDED", fmt.Sprintf("font %s is not embedded; PDF/UA requires it", dictGetName(font, "/BaseFont")))
		break
	}
	for _, is := range d.fontDictIssues(2) {
		r.add("UA_"+is.rule, is.msg)
	}
	for _, is := range d.fontGlyphIssues(true) {
		r.add("UA_"+is.rule, is.msg)
	}
}

// uaCheckStructure walks the structure tree in reading order for what depends on
// the elements' order and attributes: every /TH has a /Scope (or the table links
// its cells through /Headers instead), and headings descend one level at a time
// from H1 (ISO 14289-1 §7.4.2, §7.5).
func (d *Document) uaCheckStructure(r *PDFUAValidationReport) {
	root, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	if !ok {
		return
	}
	roleMap, _ := resolveRefToDict(d.objects, root["/RoleMap"])
	last := 0
	scopeReported, orderReported := false, false
	var walk func(elem pdfDict)
	walk = func(elem pdfDict) {
		std := resolveStructType(roleMap, dictGetName(elem, "/S"))
		if std == "/TH" && !scopeReported && !d.thHasScope(elem) {
			r.add("UA_TH_NO_SCOPE", "a /TH structure element has no /Scope attribute; PDF/UA requires it")
			scopeReported = true
		}
		n := 0
		if _, err := fmt.Sscanf(string(std), "/H%d", &n); err == nil && n >= 1 && !orderReported {
			if n > last+1 {
				r.add("UA_HEADING_ORDER", fmt.Sprintf("heading H%d follows H%d; PDF/UA requires headings to start at H1 and skip no level", n, last))
				orderReported = true
			}
			last = n
		}
		for _, kid := range d.structElemKids(elem) {
			walk(kid)
		}
	}
	for _, kid := range d.structElemKids(root) {
		walk(kid)
	}
}

func (d *Document) thHasScope(elem pdfDict) bool {
	if _, ok := elem["/ID"]; ok {
		return true // cells may reach it through /Headers
	}
	has := func(v pdfValue) bool {
		a, ok := resolveRefToDict(d.objects, v)
		return ok && dictGetName(a, "/Scope") != ""
	}
	switch a := resolveRef(d.objects, elem["/A"]).(type) {
	case pdfDict:
		return has(a)
	case pdfArray:
		return slices.ContainsFunc(a, has)
	}
	return false
}

func (d *Document) uaCheckAccessibility(r *PDFUAValidationReport) {
	if perms, encrypted := d.Permissions(); encrypted && !perms.AllowAccessibility {
		r.add("UA_INACCESSIBLE", "encryption denies the accessibility permission; PDF/UA requires content to be available to assistive technology")
	}
}

// structElemKids returns the child structure elements (dicts with /S) reachable
// from elem's /K, skipping marked-content references (integers, /MCR, /OBJR).
func (d *Document) structElemKids(elem pdfDict) []pdfDict {
	var out []pdfDict
	add := func(v pdfValue) {
		if kd, ok := resolveRefToDict(d.objects, v); ok {
			if _, isStruct := kd["/S"]; isStruct {
				out = append(out, kd)
			}
		}
	}
	switch k := elem["/K"].(type) {
	case pdfArray:
		for _, e := range k {
			add(e)
		}
	case nil:
		// no kids
	default:
		if arr, ok := resolveRefToArray(d.objects, elem["/K"]); ok {
			for _, e := range arr {
				add(e)
			}
		} else {
			add(elem["/K"])
		}
	}
	return out
}

// resolveStructType maps a structure type through the role map to a standard
// type, with cycle protection.
func resolveStructType(roleMap pdfDict, s string) string {
	seen := map[string]bool{}
	for s != "" && !seen[s] {
		seen[s] = true
		mapped := dictGetName(roleMap, s)
		if mapped == "" {
			break
		}
		s = mapped
	}
	return s
}

// hasAltText reports whether a structure element carries alternate text.
func hasAltText(objects map[int]*pdfObject, elem pdfDict) bool {
	for _, k := range []string{"/Alt", "/ActualText"} {
		if s := pdfStringValue(resolveRef(objects, elem[k])); s != "" {
			return true
		}
	}
	return false
}

// pdfStringValue returns the text of a PDF string value, or "".
func pdfStringValue(v pdfValue) string {
	switch s := v.(type) {
	case string:
		return s
	case pdfHexString:
		return string(s)
	}
	return ""
}
