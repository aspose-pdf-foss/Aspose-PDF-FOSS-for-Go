// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// pdfaForbiddenActions are the action types PDF/A does not allow (ISO 19005-1
// §6.6.1, ISO 19005-2 §6.5.1).
var pdfaForbiddenActions = map[string]bool{
	"/Launch": true, "/Sound": true, "/Movie": true, "/ResetForm": true,
	"/ImportData": true, "/JavaScript": true, "/Hide": true, "/SetOCGState": true,
	"/Rendition": true, "/Trans": true, "/GoTo3DView": true, "/SetState": true, "/NOP": true,
}

// pdfaAllowedNamedActions are the only named actions PDF/A allows.
var pdfaAllowedNamedActions = map[string]bool{
	"/NextPage": true, "/PrevPage": true, "/FirstPage": true, "/LastPage": true,
}

func isPDFAForbiddenAction(dict pdfDict, format PDFAFormat) bool {
	s := dictGetName(dict, "/S")
	if s == "/Named" {
		return !pdfaAllowedNamedActions[dictGetName(dict, "/N")]
	}
	switch {
	case format.part() == 4 && s == "/JavaScript":
		return false
	case format == PDFA4E && (s == "/SetOCGState" || s == "/GoTo3DView"):
		return false
	}
	return pdfaForbiddenActions[s]
}

// pdfaWalkDicts calls fn on every dictionary in the object table and the
// catalog, including direct ones nested inside others. A stream counts through
// its own dictionary.
func (d *Document) pdfaWalkDicts(fn func(pdfDict)) {
	var walk func(v pdfValue)
	walk = func(v pdfValue) {
		switch x := v.(type) {
		case pdfDict:
			fn(x)
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		case pdfArray:
			for _, e := range x {
				walk(e)
			}
		case *pdfStream:
			walk(x.Dict)
		}
	}
	if d.catalog != nil {
		walk(d.catalog)
	}
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		walk(d.objects[id].Value)
	}
}

// pdfaActionKeys are the dictionary entries that hold an action: an
// annotation's or outline item's /A, the catalog's /OpenAction, the next
// action of a chain.
var pdfaActionKeys = []string{"/A", "/OpenAction", "/Next"}

// removeForbiddenActions drops every action PDF/A does not allow wherever it is
// attached, and the additional-actions dictionaries, which parts 1-3 forbid on
// annotations, form fields, pages and the catalog alike and part 4 only restricts.
func (d *Document) removeForbiddenActions(format PDFAFormat) {
	d.pdfaWalkDicts(func(dict pdfDict) {
		if format.part() == 4 {
			d.filterAdditionalActions(dict, format)
		} else {
			delete(dict, "/AA")
		}
		for _, key := range pdfaActionKeys {
			switch v := resolveRef(d.objects, dict[key]).(type) {
			case pdfDict:
				if isPDFAForbiddenAction(v, format) {
					delete(dict, key)
				}
			case pdfArray: // a chain may be an array of actions
				var kept pdfArray
				for _, e := range v {
					if a, ok := resolveRefToDict(d.objects, e); !ok || !isPDFAForbiddenAction(a, format) {
						kept = append(kept, e)
					}
				}
				if len(kept) != len(v) {
					dict[key] = kept
				}
			}
		}
	})
}

// removeWidgetActions deletes the /A of every form field's widget, which every part
// forbids whatever the action (ISO 19005-1 §6.6.1, ISO 19005-2 §6.4.1). A button
// stops doing anything.
func (d *Document) removeWidgetActions() {
	d.pdfaWalkDicts(func(dict pdfDict) {
		if dictGetName(dict, "/Subtype") == "/Widget" {
			delete(dict, "/A")
		}
	})
}

// widgetWithAction reports a form field's widget carrying an action.
func (d *Document) widgetWithAction() bool {
	found := false
	d.pdfaWalkDicts(func(dict pdfDict) {
		if _, has := dict["/A"]; has && dictGetName(dict, "/Subtype") == "/Widget" {
			found = true
		}
	})
	return found
}

// firstForbiddenAction returns the type of the first action PDF/A does not
// allow, attached anywhere, or "".
func (d *Document) firstForbiddenAction(format PDFAFormat) string {
	found := ""
	note := func(a pdfDict) {
		if found == "" && isPDFAForbiddenAction(a, format) {
			found = dictGetName(a, "/S")
			if found == "/Named" {
				found = dictGetName(a, "/N")
			}
		}
	}
	d.pdfaWalkDicts(func(dict pdfDict) {
		for _, key := range pdfaActionKeys {
			switch v := resolveRef(d.objects, dict[key]).(type) {
			case pdfDict:
				note(v)
			case pdfArray:
				for _, e := range v {
					if a, ok := resolveRefToDict(d.objects, e); ok {
						note(a)
					}
				}
			}
		}
	})
	return found
}

// isInterpolatedImage reports an Image XObject asking the viewer to smooth it,
// which PDF/A forbids because the result would differ between viewers.
func isInterpolatedImage(s *pdfStream) bool {
	b, _ := s.Dict["/Interpolate"].(bool)
	return b && dictGetName(s.Dict, "/Subtype") == "/Image"
}

// clearImageInterpolation turns /Interpolate off on every image.
func (d *Document) clearImageInterpolation() {
	for _, obj := range d.objects {
		if s, ok := obj.Value.(*pdfStream); ok && isInterpolatedImage(s) {
			delete(s.Dict, "/Interpolate")
		}
	}
}

// normalizePDFAAppearances gives each annotation the appearance dictionary PDF/A
// asks for: only the normal appearance (rollover and down appearances would make
// the page look different under the pointer), a state dictionary for a button
// field, a single stream for anything else. The dictionary is replaced, not
// edited, since another annotation may share it.
// hasNormalAppearance reports an appearance dictionary whose /N leads to something.
// A reference to no object is written as null, which leaves an empty dictionary.
func (d *Document) hasNormalAppearance(ap pdfDict) bool {
	switch resolveRef(d.objects, ap["/N"]).(type) {
	case *pdfStream, pdfDict:
		return true
	}
	return false
}

// dropAppearancesWithoutN removes an appearance dictionary with no normal appearance,
// an empty one included, so that one can be drawn: it counts as having one otherwise,
// and PDF/A allows only /N in it.
func (d *Document) dropAppearancesWithoutN() {
	d.pdfaEachAnnotation(func(ad pdfDict) {
		if ap, ok := resolveRefToDict(d.objects, ad["/AP"]); ok && !d.hasNormalAppearance(ap) {
			delete(ad, "/AP")
		}
	})
}

func (d *Document) normalizePDFAAppearances() {
	d.pdfaEachAnnotation(func(ad pdfDict) {
		ap, ok := resolveRefToDict(d.objects, ad["/AP"])
		if !ok {
			return
		}
		n, hasN := ap["/N"]
		if !hasN {
			return
		}
		switch states, isDict := resolveRef(d.objects, n).(pdfDict); {
		case d.isButtonWidget(ad) && !isDict:
			n = pdfDict{pdfaButtonState: n}
			ad["/AS"] = pdfName("/" + pdfaButtonState[1:])
		case !d.isButtonWidget(ad) && isDict:
			if st, ok := states[dictGetName(ad, "/AS")]; ok {
				n = st
			}
		}
		ad["/AP"] = pdfDict{"/N": n}
	})
}

// pdfaButtonState names the only state of a button that has one appearance
// (a push button).
const pdfaButtonState = "/Default"

// isButtonWidget reports a widget of a button field; /FT may be inherited.
func (d *Document) isButtonWidget(ad pdfDict) bool {
	if dictGetName(ad, "/Subtype") != "/Widget" {
		return false
	}
	for range maxFormDepth {
		if ft := dictGetName(ad, "/FT"); ft != "" {
			return ft == "/Btn"
		}
		parent, ok := resolveRefToDict(d.objects, ad["/Parent"])
		if !ok {
			return false
		}
		ad = parent
	}
	return false
}

func (d *Document) pdfaEachAnnotation(fn func(pdfDict)) {
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
			if ad, ok := resolveRefToDict(d.objects, a); ok {
				fn(ad)
			}
		}
	}
}

// reconcileInfoAndXMP makes the /Info dictionary and the XMP packet agree, as
// PDF/A-1 asks: a value only one side has is copied to the other, and where both
// have one the XMP value wins, except for dates, where /Info decides because a
// PDF date converts to XMP form exactly. Empty /Info strings disappear (they
// count as a mismatch against an absent XMP property).
func (d *Document) reconcileInfoAndXMP(meta *XMPMetadata) {
	info, err := d.Info()
	if err != nil {
		return
	}
	pick := func(xmp, inf *string) {
		if *xmp == "" {
			*xmp = *inf
		}
		*inf = *xmp
	}
	pick(&meta.Title, &info.Title)
	pick(&meta.Description, &info.Subject)
	pick(&meta.Producer, &info.Producer)
	pick(&meta.CreatorTool, &info.Creator)
	switch {
	case len(meta.Authors) == 0 && info.Author != "":
		meta.Authors = []string{info.Author}
	case len(meta.Authors) == 1:
		info.Author = meta.Authors[0]
	case len(meta.Authors) > 1:
		info.Author = strings.Join(meta.Authors, ", ")
		meta.Authors = []string{info.Author}
	}
	// A date that is no date, like "5/10/2013 13:31:00", could be either of two; it
	// goes rather than being guessed at.
	date := func(xmp, inf *string) {
		if *inf != "" {
			*inf = pdfDateWithZone(*inf)
			if iso := pdfDateToISO8601(*inf); reXMPDate.MatchString(iso) {
				*xmp = iso
			} else {
				*inf = ""
			}
		}
		if !reXMPDate.MatchString(*xmp) {
			*xmp, *inf = "", ""
		}
	}
	date(&meta.CreateDate, &info.CreationDate)
	date(&meta.ModifyDate, &info.ModDate)
	if !reXMPDate.MatchString(meta.MetadataDate) {
		meta.MetadataDate = ""
	}
	if len(meta.Keywords) == 0 {
		meta.Keywords = splitKeywords(info.Keywords)
	}
	keywords := info.Keywords
	var custom []XMPProperty
	for _, p := range meta.Custom {
		if p.Namespace == nsPDF && p.Name == "Keywords" {
			if keywords == "" {
				keywords = p.Value
			}
			continue
		}
		custom = append(custom, p)
	}
	if keywords != "" {
		custom = append(custom, XMPProperty{Namespace: nsPDF, Prefix: "pdf", Name: "Keywords", Value: keywords})
	}
	info.Keywords = keywords
	meta.Custom = custom
	d.SetInfo(info)
}

// reXMPDate is the XMP Date type (ISO 8601, with the time optional); empty passes.
var reXMPDate = regexp.MustCompile(`^$|^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?)?)?)?$`)

// pdfDateWithZone completes a PDF date that names no time zone as UTC. A reader
// has to assume something for such a date, and a validator comparing it with its
// XMP form assumes different things for each side.
func pdfDateWithZone(s string) string {
	digits := strings.TrimPrefix(s, "D:")
	if len(digits) == len(s) || len(digits) < 4 {
		return s
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return s
		}
	}
	const rest = "0101000000" // month, day, hour, minute, second
	if len(digits) < 14 {
		digits += rest[len(digits)-4:]
	}
	return "D:" + digits + "Z"
}

// fixSharedConformance repairs what PDF/A and PDF/UA ask alike: a form may not be
// XFA, a layer configuration needs a name, no /AS (PDF/A-4 allows it, so keepAS) and
// an /Order naming every layer, and an embedded Type 2 CIDFont must state its
// /CIDToGIDMap.
func (d *Document) fixSharedConformance(keepAS bool) {
	d.fixFilespecNames()
	d.fixOutOfRangeIntegers()
	if d.catalog != nil {
		delete(d.catalog, "/NeedsRendering")
		if form, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"]); ok {
			delete(form, "/XFA")
		}
	}
	for i, cfg := range d.ocConfigs() {
		if !keepAS {
			delete(cfg, "/AS")
		}
		if name, _ := cfg["/Name"].(string); name == "" {
			cfg["/Name"] = encodeFormString(fmt.Sprintf("Layers %d", i+1))
		}
		if missing := d.ocMissingFromOrder(cfg); len(missing) > 0 {
			order, _ := resolveRef(d.objects, cfg["/Order"]).(pdfArray)
			cfg["/Order"] = append(slices.Clone(order), missing...)
		}
	}
	for _, id := range d.cidType2Missing() {
		d.objects[id].Value.(pdfDict)["/CIDToGIDMap"] = pdfName("/Identity")
	}
}

// removeTransferFunctions drops the transfer functions PDF/A forbids in a
// graphics state. They adjust output to a device, so an archive has no use for
// them. A soft mask's /TR is another matter (the dictionary has an /S).
func (d *Document) removeTransferFunctions() {
	d.pdfaWalkDicts(func(dict pdfDict) {
		if hasTransferFunction(dict) {
			delete(dict, "/TR")
			delete(dict, "/TR2")
		}
	})
}

func hasTransferFunction(dict pdfDict) bool {
	if _, isMask := dict["/S"]; isMask {
		return false
	}
	if _, ok := dict["/TR"]; ok {
		return true
	}
	tr2, ok := dict["/TR2"]
	return ok && tr2 != pdfName("/Default")
}

// ocConfigs returns the layer configurations: the default and the alternates.
func (d *Document) ocConfigs() []pdfDict {
	ocp, ok := resolveRefToDict(d.objects, d.catalog["/OCProperties"])
	if !ok {
		return nil
	}
	var out []pdfDict
	if cfg, ok := resolveRefToDict(d.objects, ocp["/D"]); ok {
		out = append(out, cfg)
	}
	if arr, ok := resolveRefToArray(d.objects, ocp["/Configs"]); ok {
		for _, e := range arr {
			if cfg, ok := resolveRefToDict(d.objects, e); ok {
				out = append(out, cfg)
			}
		}
	}
	return out
}

// ocMissingFromOrder returns the layers cfg's /Order does not mention.
func (d *Document) ocMissingFromOrder(cfg pdfDict) pdfArray {
	ocp, ok := resolveRefToDict(d.objects, d.catalog["/OCProperties"])
	if !ok {
		return nil
	}
	groups, ok := resolveRefToArray(d.objects, ocp["/OCGs"])
	if !ok {
		return nil
	}
	seen := map[int]bool{}
	var walk func(v pdfValue, depth int)
	walk = func(v pdfValue, depth int) {
		if r, ok := v.(pdfRef); ok {
			seen[r.Num] = true
		}
		if arr, ok := resolveRef(d.objects, v).(pdfArray); ok && depth < maxFormDepth {
			for _, e := range arr {
				walk(e, depth+1)
			}
		}
	}
	walk(cfg["/Order"], 0)
	var missing pdfArray
	for _, g := range groups {
		if r, ok := g.(pdfRef); ok && !seen[r.Num] {
			missing = append(missing, g)
		}
	}
	return missing
}

// cidType2Missing lists embedded Type 2 CIDFonts with no /CIDToGIDMap.
func (d *Document) cidType2Missing() []int {
	var out []int
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		font, ok := d.objects[id].Value.(pdfDict)
		if !ok || dictGetName(font, "/Subtype") != "/CIDFontType2" {
			continue
		}
		if _, has := font["/CIDToGIDMap"]; !has && fontDescriptorHasFile(d.objects, font) {
			out = append(out, id)
		}
	}
	return out
}

// sharedConformanceIssues lists what fixSharedConformance would repair.
func (d *Document) sharedConformanceIssues(keepAS bool) (rule, msg string) {
	if form, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"]); ok {
		if _, has := form["/XFA"]; has {
			return "XFA_FORM", "the form carries XFA; PDF/A and PDF/UA do not allow it"
		}
	}
	if _, has := d.catalog["/NeedsRendering"]; has {
		return "XFA_FORM", "the catalog has /NeedsRendering; PDF/A and PDF/UA do not allow it"
	}
	for _, cfg := range d.ocConfigs() {
		name, _ := cfg["/Name"].(string)
		_, hasAS := cfg["/AS"]
		if name == "" || (hasAS && !keepAS) || len(d.ocMissingFromOrder(cfg)) > 0 {
			return "LAYER_CONFIG", "a layer configuration lacks /Name, has /AS, or its /Order omits a layer"
		}
	}
	if n := d.filespecMissingName(); n != nil {
		return "FILESPEC_NO_UF", "an embedded file's specification lacks /F or /UF; PDF/A and PDF/UA ask for both"
	}
	if ids := d.cidType2Missing(); len(ids) > 0 {
		return "CIDFONT_NO_CIDTOGIDMAP", fmt.Sprintf("embedded Type 2 CIDFont object %d has no /CIDToGIDMap", ids[0])
	}
	return "", ""
}

// xmpPredefinedNamespaces are the namespaces PDF/A accepts without an extension
// schema (ISO 19005-1 §6.7.8; the XMP specification's predefined schemas).
var xmpPredefinedNamespaces = map[string]bool{
	nsDC: true, nsXMP: true, nsPDF: true, nsPDFAID: true,
	"http://ns.adobe.com/xap/1.0/mm/":              true,
	"http://ns.adobe.com/xap/1.0/rights/":          true,
	nsXMPTPg:                                       true,
	"http://ns.adobe.com/xap/1.0/bj/":              true,
	"http://ns.adobe.com/xmp/1.0/DynamicMedia/":    true,
	"http://ns.adobe.com/xmp/Identifier/qual/1.0/": true,
	"http://ns.adobe.com/photoshop/1.0/":           true,
	"http://ns.adobe.com/camera-raw-settings/1.0/": true,
	"http://ns.adobe.com/tiff/1.0/":                true,
	"http://ns.adobe.com/exif/1.0/":                true,
	"http://ns.adobe.com/exif/1.0/aux/":            true,
}

// xmpTPgProperties are the Paged-Text properties the XMP specification defines;
// producers write others in the same namespace, which PDF/A rejects.
var xmpTPgProperties = map[string]bool{
	"MaxPageSize": true, "NPages": true, "Fonts": true, "Colorants": true, "PlateNames": true,
}

const nsXMPTPg = "http://ns.adobe.com/xap/1.0/t/pg/"

// splitDeclaredXMP separates the properties PDF/A accepts from those it would
// reject: a namespace neither predefined nor declared by an extension schema in
// blocks.
func splitDeclaredXMP(custom []XMPProperty, blocks []string) (kept, undeclared []XMPProperty) {
	declared := extensionSchemaDeclarations(blocks)
	for _, p := range custom {
		_, isDeclared := declared[p.Namespace]
		if p.Namespace == nsXMPTPg && !xmpTPgProperties[p.Name] && !isDeclared {
			undeclared = append(undeclared, p)
		} else if xmpPredefinedNamespaces[p.Namespace] || isDeclared {
			kept = append(kept, p)
		} else {
			undeclared = append(undeclared, p)
		}
	}
	return kept, undeclared
}

// removeObjectMetadata drops the /Metadata of everything but the document. PDF/A
// holds such packets to the same schema rules as the document's own, a producer
// writes them freely (a placed image keeps its source's XMP), and no reader
// depends on them.
func (d *Document) removeObjectMetadata() {
	own, has := d.catalog["/Metadata"]
	d.pdfaWalkDicts(func(dict pdfDict) {
		delete(dict, "/Metadata")
	})
	if has {
		d.catalog["/Metadata"] = own
	}
}

// fixCIDSets brings every CID font's /CIDSet in line with the standard: PDF/A-1
// requires one on a subset, listing the CIDs the embedded program carries; the
// later parts and PDF/UA only require it to be right when present, so the
// entry, which no reader needs, is dropped there (part 0).
func (d *Document) fixCIDSets(part int) {
	used := d.cidFontUsage()
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		font, ok := d.objects[id].Value.(pdfDict)
		if !ok {
			continue
		}
		sub := dictGetName(font, "/Subtype")
		if sub != "/CIDFontType0" && sub != "/CIDFontType2" {
			continue
		}
		desc, ok := resolveRefToDict(d.objects, font["/FontDescriptor"])
		if !ok {
			continue
		}
		if part != 1 || !isSubsetFontName(dictGetName(font, "/BaseFont")) {
			delete(desc, "/CIDSet")
			continue
		}
		if set := d.cidSetFor(font, desc, used[id]); set != nil {
			desc["/CIDSet"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: set, Decoded: true})}
		}
	}
}

// isSubsetFontName recognises the "ABCDEF+" prefix of a font subset's name.
func isSubsetFontName(name string) bool {
	name = strings.TrimPrefix(name, "/")
	if len(name) < 8 || name[6] != '+' {
		return false
	}
	for _, c := range name[:6] {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// cidFontUsage returns the CIDs the document draws, per CIDFont object.
func (d *Document) cidFontUsage() map[int]map[uint16]bool {
	out := map[int]map[uint16]bool{}
	for num, cids := range d.pdfaUsedCIDs() {
		obj := d.objects[num]
		if obj == nil {
			continue
		}
		font, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		descs, ok := resolveRefToArray(d.objects, font["/DescendantFonts"])
		if !ok || len(descs) == 0 {
			continue
		}
		if ref, ok := descs[0].(pdfRef); ok {
			if out[ref.Num] == nil {
				out[ref.Num] = map[uint16]bool{}
			}
			maps.Copy(out[ref.Num], cids)
		}
	}
	return out
}

// cidSetFor builds a /CIDSet for a CID font from its embedded program: one bit
// per CID, most significant first, set when the program has a glyph for it or
// the document draws it (a space is drawn and has no outline, so a subset looks
// as if it lacked it). Returns nil when the program cannot be read.
func (d *Document) cidSetFor(font, desc pdfDict, used map[uint16]bool) []byte {
	var present []int
	if s, ok := resolveRef(d.objects, desc["/FontFile2"]).(*pdfStream); ok {
		prog, err := parseTTF(decodedStreamData(s))
		if err != nil {
			return nil
		}
		tables := prog.tables
		if tables == nil {
			tables = tableDir(prog.data)
		}
		loca, err := readLoca(prog, tables)
		if err != nil {
			return nil
		}
		has := func(gid int) bool {
			return gid+1 < len(loca) && loca[gid+1] > loca[gid]
		}
		if cidToGID := parseCIDToGIDMap(d.objects, font["/CIDToGIDMap"]); cidToGID != nil {
			for cid, gid := range cidToGID {
				if has(int(gid)) && (gid != 0 || cid == 0) {
					present = append(present, cid)
				}
			}
		} else {
			for gid := range int(prog.numGlyphs) {
				if has(gid) {
					present = append(present, gid)
				}
			}
		}
	} else if s, ok := resolveRef(d.objects, desc["/FontFile3"]).(*pdfStream); ok {
		prog, err := parseCFFProgram(decodedStreamData(s))
		if err != nil {
			return nil
		}
		if prog.isCID {
			for _, cid := range prog.charset {
				present = append(present, int(cid))
			}
		} else {
			for gid := range prog.charStrings {
				present = append(present, gid)
			}
		}
	}
	if len(present) == 0 {
		return nil
	}
	for cid := range used {
		present = append(present, int(cid))
	}
	set := make([]byte, slices.Max(present)/8+1)
	for _, cid := range present {
		set[cid/8] |= 0x80 >> (cid % 8)
	}
	return set
}

// fullDifferencesEncoding gives a non-symbolic substitute font an /Encoding with
// an explicit base. An /Encoding of only /Differences implies the font's own
// base, which PDF/A does not accept for a TrueType font: each code whose
// character differs from WinAnsi is named instead, so every code keeps its
// character.
func fullDifferencesEncoding(orig pdfDict, fi fontInfo) pdfDict {
	diffs, _ := orig["/Differences"].(pdfArray)
	names := differencesGlyphNames(diffs)
	var out pdfArray
	last := -2
	for c := range 256 {
		r := fi.encoding[c]
		if r == 0 || r == 0xFFFD || r == winAnsiEncoding[c] {
			continue
		}
		name := strings.TrimPrefix(names[byte(c)], "/")
		if _, known := glyphToRune[name]; !known {
			name = runeToStdGlyphName(r)
		}
		if name == "" {
			name = fmt.Sprintf("uni%04X", r)
		}
		if c != last+1 {
			out = append(out, c)
		}
		out = append(out, pdfName("/"+name))
		last = c
	}
	res := pdfDict{"/Type": pdfName("/Encoding"), "/BaseEncoding": pdfName("/WinAnsiEncoding")}
	if len(out) > 0 {
		res["/Differences"] = out
	}
	return res
}

// pdfaCheckConformity holds the rules found by comparing this validator with
// veraPDF: what ConvertToPDFA repairs, or cannot, but a document may still carry.
func (d *Document) pdfaCheckConformity(format PDFAFormat, r *PDFAValidationReport) {
	if s := d.firstForbiddenAction(format); s != "" && s != "/JavaScript" && s != "/Launch" {
		r.add("FORBIDDEN_ACTION", fmt.Sprintf("document contains a %s action, which PDF/A does not allow", s))
	}
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		if s, ok := d.objects[id].Value.(*pdfStream); ok && isInterpolatedImage(s) {
			r.add("IMAGE_INTERPOLATE", fmt.Sprintf("image object %d sets /Interpolate; PDF/A forbids it", id))
			break
		}
	}
	apFound := false
	d.pdfaEachAnnotation(func(ad pdfDict) {
		ap, ok := resolveRefToDict(d.objects, ad["/AP"])
		if !ok || apFound {
			return
		}
		if !d.hasNormalAppearance(ap) {
			apFound = true
			r.add("ANNOTATION_AP_ENTRIES", "an annotation's appearance dictionary has no /N; PDF/A allows only /N")
			return
		}
		for _, k := range slices.Sorted(maps.Keys(ap)) {
			if k != "/N" {
				apFound = true
				r.add("ANNOTATION_AP_ENTRIES", fmt.Sprintf("an annotation's appearance dictionary has %s; PDF/A allows only /N", k))
				return
			}
		}
		if _, isDict := resolveRef(d.objects, ap["/N"]).(pdfDict); isDict != d.isButtonWidget(ad) && ap["/N"] != nil {
			apFound = true
			r.add("ANNOTATION_AP_N_TYPE", "PDF/A requires /AP /N to be a state dictionary on a button field and a stream on anything else")
		}
	})
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		blocks, _ := xmpExtensionBlocks(string(raw))
		if meta, err := d.XMP(); err == nil {
			if _, undeclared := splitDeclaredXMP(meta.Custom, blocks); len(undeclared) > 0 && format.part() != 4 {
				r.add("XMP_UNDECLARED_PROPERTY", fmt.Sprintf("XMP property %s (%s) is neither predefined nor declared by an extension schema", undeclared[0].Name, undeclared[0].Namespace))
			}
			if format.part() != 4 { // PDF/A-4 has no rule on XMP value types
				pdfaCheckXMPDates(meta, r)
			}
			if format.part() == 1 {
				d.pdfaCheckInfoAgreement(meta, r)
			}
		}
	}
	if rule, msg := d.sharedConformanceIssues(format.part() == 4); rule != "" {
		r.add(rule, msg)
	}
	found := false
	d.pdfaWalkDicts(func(dict pdfDict) {
		if !found && hasTransferFunction(dict) {
			found = true
			r.add("EXTGSTATE_TRANSFER", "a graphics state has a transfer function (/TR or /TR2); PDF/A forbids it")
		}
	})
	if _, has := d.catalog["/OCProperties"]; has && format.part() == 1 {
		// Removing layers would show what is hidden, so this one is only reported.
		r.add("LAYERS_FORBIDDEN", "the document has layers (/OCProperties); PDF/A-1 does not allow them")
	}
	d.pdfaCheckCIDSets(format, r)
	d.pdfaCheckFontConformance(format, r)
	d.pdfaCheckLimits(format, r)
	d.pdfaCheckAnnotationTypes(format, r)
	if format.part() == 2 {
		d.pdfaCheckAttachedPDFA(format, r)
	}
	if format.part() == 4 {
		d.pdfaCheck4(format, r)
	}
	if (format.part() == 2 || format.part() == 3) && d.signatureStale() {
		r.add("SIGNATURE_STALE", "a signature's /ByteRange does not span the file; a conversion rewrites the file, so this signature no longer verifies")
	}
	d.pdfaCheckDeviceColorMatch(r)
	d.pdfaCheckInheritedResources(format, r)
	if d.widgetWithAction() {
		r.add("WIDGET_ACTION", "a form field's widget has an /A action, which PDF/A forbids whatever the action (ISO 19005-2 §6.4.1)")
	}
	if format.part() == 1 {
		d.pdfaCheckICCComponents(r)
	}
}

// pdfaCheckXMPDates reports a date the XMP Date type does not allow.
func pdfaCheckXMPDates(meta XMPMetadata, r *PDFAValidationReport) {
	for _, c := range []struct{ name, value string }{
		{"xmp:CreateDate", meta.CreateDate}, {"xmp:ModifyDate", meta.ModifyDate}, {"xmp:MetadataDate", meta.MetadataDate},
	} {
		if !reXMPDate.MatchString(c.value) {
			r.add("XMP_DATE_INVALID", fmt.Sprintf("%s is %q, not a date in the form XMP requires", c.name, c.value))
		}
	}
}

// pdfaCheckInfoAgreement reports /Info entries with no matching XMP property
// (ISO 19005-1 §6.7.3).
func (d *Document) pdfaCheckInfoAgreement(meta XMPMetadata, r *PDFAValidationReport) {
	info, err := d.Info()
	if err != nil {
		return
	}
	author := ""
	if len(meta.Authors) == 1 {
		author = meta.Authors[0]
	}
	for _, c := range []struct{ key, inf, xmp string }{
		{"/Title", info.Title, meta.Title},
		{"/Subject", info.Subject, meta.Description},
		{"/Author", info.Author, author},
		{"/Creator", info.Creator, meta.CreatorTool},
		{"/Producer", info.Producer, meta.Producer},
	} {
		if c.inf != "" && c.inf != c.xmp {
			r.add("INFO_XMP_MISMATCH", fmt.Sprintf("/Info %s (%q) does not match its XMP property (%q)", c.key, c.inf, c.xmp))
		}
	}
	if info.CreationDate != "" && meta.CreateDate == "" {
		r.add("INFO_XMP_MISMATCH", "/Info /CreationDate has no matching xmp:CreateDate")
	}
	if info.ModDate != "" && meta.ModifyDate == "" {
		r.add("INFO_XMP_MISMATCH", "/Info /ModDate has no matching xmp:ModifyDate")
	}
}

// pdfaCheckCIDSets reports a subset CID font with no /CIDSet (PDF/A-1) and a
// /CIDSet that does not match the embedded program (every part).
func (d *Document) pdfaCheckCIDSets(format PDFAFormat, r *PDFAValidationReport) {
	if format.part() == 4 {
		return // PDF/A-4 has no rule on /CIDSet
	}
	used := d.cidFontUsage()
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		font, ok := d.objects[id].Value.(pdfDict)
		if !ok {
			continue
		}
		sub := dictGetName(font, "/Subtype")
		if sub != "/CIDFontType0" && sub != "/CIDFontType2" {
			continue
		}
		desc, ok := resolveRefToDict(d.objects, font["/FontDescriptor"])
		if !ok {
			continue
		}
		name := dictGetName(font, "/BaseFont")
		s, has := resolveRef(d.objects, desc["/CIDSet"]).(*pdfStream)
		if !has {
			if format.part() == 1 && isSubsetFontName(name) && d.cidSetFor(font, desc, used[id]) != nil {
				r.add("CIDSET_MISSING", fmt.Sprintf("subset font %s has no /CIDSet; PDF/A-1 requires one", name))
				return
			}
			continue
		}
		if want := d.cidSetFor(font, desc, used[id]); want != nil && !cidSetsEqual(decodedStreamData(s), want) {
			r.add("CIDSET_WRONG", fmt.Sprintf("font %s: /CIDSet does not match the embedded program", name))
			return
		}
	}
}

// cidSetsEqual compares two bit sets, ignoring trailing zero bytes.
func cidSetsEqual(a, b []byte) bool {
	trim := func(s []byte) []byte {
		for len(s) > 0 && s[len(s)-1] == 0 {
			s = s[:len(s)-1]
		}
		return s
	}
	return slices.Equal(trim(a), trim(b))
}

// pdfaCheckDeviceColorMatch reports device colours the output intent does not
// cover: DeviceCMYK under an RGB intent, DeviceRGB under a CMYK one (ISO 19005-1
// §6.2.3.3, ISO 19005-2 §6.2.4.3). ConvertToPDFA adds an sRGB intent and has no
// CMYK profile to offer, so a CMYK document stays a reported violation until
// ConvertDeviceCMYKToRGB recolours it. Only what the pages paint with counts, as
// for veraPDF: a colour space or an image nothing uses is no violation.
func (d *Document) pdfaCheckDeviceColorMatch(r *PDFAValidationReport) {
	n := d.outputIntentComponents()
	if n != 3 && n != 4 {
		return
	}
	s := &deviceColorScan{d: d, space: "/DeviceCMYK", abbrev: "/CMYK", remap: "/DefaultCMYK", fill: "k", stroke: "K", seen: map[*pdfStream]bool{}}
	if n == 4 {
		s.space, s.abbrev, s.remap, s.fill, s.stroke = "/DeviceRGB", "/RGB", "/DefaultRGB", "rg", "RG"
	}
	for i := range d.pages {
		page, err := d.Page(i + 1)
		if err != nil {
			continue
		}
		if use := s.page(page); use != "" {
			r.add("COLOR_INTENT_MISMATCH", fmt.Sprintf("page %d uses %s (%s), which the output intent's %d-component profile does not cover", i+1, s.space[1:], use, n))
			return
		}
	}
}

// deviceColorScan looks for the first use of one device colour space in what a
// page paints: its content, the forms, patterns, shadings, images, soft masks and
// Type3 glyphs that content reaches, and its annotations' normal appearances.
type deviceColorScan struct {
	d                    *Document
	space, abbrev, remap string // the space, its inline abbreviation, its Default* key
	fill, stroke         string // the operators setting it directly
	seen                 map[*pdfStream]bool
}

func (s *deviceColorScan) page(p *Page) string {
	objects := s.d.objects
	dict, res := p.pageDict(), p.pageResources()
	if dict == nil {
		return ""
	}
	if use := s.group(dict, res); use != "" {
		return use
	}
	for _, data := range s.d.pageContentStreams(dict) {
		if use := s.content(data, res); use != "" {
			return use
		}
	}
	annots, _ := resolveRefToArray(objects, dict["/Annots"])
	for _, v := range annots {
		ad, ok := resolveRefToDict(objects, v)
		if !ok {
			continue
		}
		ap, ok := resolveRefToDict(objects, ad["/AP"])
		if !ok {
			continue
		}
		if use := s.appearance(ap["/N"], 0); use != "" {
			return use
		}
	}
	return ""
}

// appearance follows a normal appearance, a stream or a state dictionary of them.
func (s *deviceColorScan) appearance(v pdfValue, depth int) string {
	switch t := resolveRef(s.d.objects, v).(type) {
	case *pdfStream:
		return s.xobject(t, nil)
	case pdfDict:
		if depth > 0 {
			return ""
		}
		for _, key := range slices.Sorted(maps.Keys(t)) {
			if use := s.appearance(t[key], depth+1); use != "" {
				return use
			}
		}
	}
	return ""
}

func (s *deviceColorScan) group(dict, res pdfDict) string {
	g, ok := resolveRefToDict(s.d.objects, dict["/Group"])
	if ok && s.colorSpace(g["/CS"], res, 0) {
		return "transparency group"
	}
	return ""
}

// remapped reports a Default* colour space in res, which makes the device space
// device-independent there (ISO 32000-1 §8.6.5.6).
func (s *deviceColorScan) remapped(res pdfDict) bool {
	csd, ok := resolveRefToDict(s.d.objects, res["/ColorSpace"])
	return ok && csd[s.remap] != nil
}

// colorSpace reports whether a colour-space value is the device space or rests on
// it. A name not naming a device space is looked up in res.
func (s *deviceColorScan) colorSpace(v pdfValue, res pdfDict, depth int) bool {
	if depth > maxColorSpaceDepth {
		return false
	}
	objects := s.d.objects
	switch t := resolveRef(objects, v).(type) {
	case pdfName:
		if string(t) == s.space || string(t) == s.abbrev {
			return !s.remapped(res)
		}
		if csd, ok := resolveRefToDict(objects, res["/ColorSpace"]); ok && csd[string(t)] != nil {
			return s.colorSpace(csd[string(t)], res, depth+1)
		}
	case pdfArray:
		if len(t) < 2 {
			return false
		}
		switch operandName(t[0]) {
		case "/Indexed", "/I":
			return s.colorSpace(t[1], res, depth+1)
		case "/Separation", "/DeviceN":
			return len(t) >= 3 && s.colorSpace(t[2], res, depth+1)
		}
	}
	return false
}

func (s *deviceColorScan) content(data []byte, res pdfDict) string {
	ops, err := parseContentStream(data)
	if err != nil {
		return ""
	}
	objects := s.d.objects
	sub := func(key string, name pdfValue) pdfValue {
		n, ok := name.(pdfName)
		if !ok {
			return nil
		}
		dict, _ := resolveRefToDict(objects, res[key])
		return dict[string(n)]
	}
	for _, op := range ops {
		use := ""
		switch op.Operator {
		case s.fill, s.stroke:
			if !s.remapped(res) {
				use = fmt.Sprintf("operator %q", op.Operator)
			}
		case "cs", "CS":
			if s.colorSpace(at(op, 0), res, 0) {
				use = fmt.Sprintf("operator %q", op.Operator)
			}
		case "sh":
			use = s.shading(sub("/Shading", at(op, 0)), res)
		case "scn", "SCN":
			if n := len(op.Operands); n > 0 {
				use = s.pattern(sub("/Pattern", op.Operands[n-1]), res)
			}
		case "Do":
			stream, _ := resolveRef(objects, sub("/XObject", at(op, 0))).(*pdfStream)
			use = s.xobject(stream, res)
		case "BI":
			if dict, ok := at(op, 0).(pdfDict); ok && s.colorSpace(dict["/ColorSpace"], res, 0) {
				use = "inline image"
			}
		case "gs":
			gs, _ := resolveRefToDict(objects, sub("/ExtGState", at(op, 0)))
			if sm, ok := resolveRefToDict(objects, gs["/SMask"]); ok {
				form, _ := resolveRef(objects, sm["/G"]).(*pdfStream)
				use = s.xobject(form, res)
			}
		case "Tf":
			use = s.type3(sub("/Font", at(op, 0)), res)
		}
		if use != "" {
			return use
		}
	}
	return ""
}

func (s *deviceColorScan) type3(v pdfValue, res pdfDict) string {
	objects := s.d.objects
	font, _ := resolveRefToDict(objects, v)
	if dictGetName(font, "/Subtype") != "/Type3" {
		return ""
	}
	if own, ok := resolveRefToDict(objects, font["/Resources"]); ok {
		res = own
	}
	procs, _ := resolveRefToDict(objects, font["/CharProcs"])
	for _, key := range slices.Sorted(maps.Keys(procs)) {
		proc, _ := resolveRef(objects, procs[key]).(*pdfStream)
		if proc == nil || s.seen[proc] {
			continue
		}
		s.seen[proc] = true
		if use := s.content(decodedStreamData(proc), res); use != "" {
			return use
		}
	}
	return ""
}

// xobject scans an image or a form. A form with no /Resources uses its caller's.
func (s *deviceColorScan) xobject(stream *pdfStream, res pdfDict) string {
	if stream == nil || s.seen[stream] {
		return ""
	}
	s.seen[stream] = true
	if dictGetName(stream.Dict, "/Subtype") == "/Image" {
		if s.colorSpace(stream.Dict["/ColorSpace"], res, 0) {
			return "image"
		}
		return ""
	}
	if own, ok := resolveRefToDict(s.d.objects, stream.Dict["/Resources"]); ok {
		res = own
	}
	if use := s.group(stream.Dict, res); use != "" {
		return use
	}
	return s.content(decodedStreamData(stream), res)
}

func (s *deviceColorScan) pattern(v pdfValue, res pdfDict) string {
	switch t := resolveRef(s.d.objects, v).(type) {
	case *pdfStream: // tiling
		if s.seen[t] {
			return ""
		}
		s.seen[t] = true
		if own, ok := resolveRefToDict(s.d.objects, t.Dict["/Resources"]); ok {
			res = own
		}
		return s.content(decodedStreamData(t), res)
	case pdfDict:
		return s.shading(t["/Shading"], res)
	}
	return ""
}

func (s *deviceColorScan) shading(v pdfValue, res pdfDict) string {
	dict, _ := asDictStream(resolveRef(s.d.objects, v))
	if dict != nil && s.colorSpace(dict["/ColorSpace"], res, 0) {
		return "shading"
	}
	return ""
}

// pdfaCheckICCComponents reports an ICC profile whose stream declares a component
// count other than the profile's own (ISO 19005-1 §6.2.3.2).
func (d *Document) pdfaCheckICCComponents(r *PDFAValidationReport) {
	spaces := map[string]int{"GRAY": 1, "RGB ": 3, "Lab ": 3, "CMYK": 4}
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		s, ok := d.objects[id].Value.(*pdfStream)
		if !ok {
			continue
		}
		n, hasN := s.Dict["/N"].(int)
		if !hasN {
			continue
		}
		b := decodedStreamData(s)
		if len(b) < 40 || string(b[36:40]) != "acsp" {
			continue
		}
		if want, known := spaces[string(b[16:20])]; known && want != n {
			r.add("ICC_COMPONENTS_MISMATCH", fmt.Sprintf("ICC profile object %d declares /N %d but holds a %d-component profile", id, n, want))
			return
		}
	}
}

// outputIntentComponents returns the component count of the output intent's
// destination profile, or 0 when there is none.
func (d *Document) outputIntentComponents() int {
	arr, ok := resolveRefToArray(d.objects, d.catalog["/OutputIntents"])
	if !ok {
		return 0
	}
	for _, e := range arr {
		oi, ok := resolveRefToDict(d.objects, e)
		if !ok {
			continue
		}
		if s, ok := resolveRef(d.objects, oi["/DestOutputProfile"]).(*pdfStream); ok {
			return toInt(resolveRef(d.objects, s.Dict["/N"]))
		}
	}
	return 0
}

// filespecMissingName returns a file specification that embeds a file but lacks
// /F or /UF, or nil.
func (d *Document) filespecMissingName() pdfDict {
	var found pdfDict
	d.pdfaWalkDicts(func(dict pdfDict) {
		if _, embeds := dict["/EF"]; !embeds || found != nil {
			return
		}
		f, _ := resolveRef(d.objects, dict["/F"]).(string)
		uf, _ := resolveRef(d.objects, dict["/UF"]).(string)
		if f == "" || uf == "" {
			found = dict
		}
	})
	return found
}

// fixFilespecNames gives each file specification both names from whichever it has.
func (d *Document) fixFilespecNames() {
	d.pdfaWalkDicts(func(dict pdfDict) {
		if _, embeds := dict["/EF"]; !embeds {
			return
		}
		f, _ := resolveRef(d.objects, dict["/F"]).(string)
		uf, _ := resolveRef(d.objects, dict["/UF"]).(string)
		switch {
		case f == "" && uf != "":
			dict["/F"] = uf
		case uf == "" && f != "":
			dict["/UF"] = encodeFormString(f)
		}
	})
}
