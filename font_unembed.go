// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// unembedWidthTolerance is the largest per-code advance-width difference (in
// 1/1000 em) tolerated between a font's own /Widths and the Standard-14 AFM
// table it's being matched against. PDF producers write exact AFM-derived
// integers for a genuine Arial/Times/Courier, so any real match is exact;
// this only absorbs float rounding.
const unembedWidthTolerance = 1.0

// unembedTargetName returns the exact Standard-14 name a font's own BaseFont
// is metric-compatible with — the Helvetica/Times/Courier families via
// standard14WidthAlias's name matching (Arial, Times New Roman, Courier New,
// …, the same substitution Acrobat performs), or the BaseFont itself when it
// already literally names one of those three families. Symbol and
// ZapfDingbats are deliberately excluded even though they're Standard-14
// names too: unlike Helvetica/Times/Courier they have no metric-compatible
// system/bundled substitute (render_std14.go), so dropping an embedded copy
// would leave the text unrenderable rather than merely reshaped. Returns ""
// when the name isn't recognized at all — an exotic or branded face, which
// UnembedFonts leaves alone since there's no substitute to fall back to.
func unembedTargetName(baseFont string) string {
	switch baseFont {
	case "/Helvetica", "/Helvetica-Bold", "/Helvetica-Oblique", "/Helvetica-BoldOblique",
		"/Times-Roman", "/Times-Bold", "/Times-Italic", "/Times-BoldItalic",
		"/Courier", "/Courier-Bold", "/Courier-Oblique", "/Courier-BoldOblique":
		return baseFont
	}
	if canon, ok := standard14WidthAlias(baseFont); ok {
		return canon
	}
	return ""
}

// unembedWidthsMatch reports whether dict's own /Widths (if any) agrees with
// target's Standard-14 AFM widths closely enough to trust that the embedded
// program really is that family, not just a font someone named similarly. A
// font with no /Widths (rare, but the descriptor still carries width-free
// metrics) is presumed to match — name is all there is to go on. A .notdef
// code (0-width in the font's own table) is skipped: it carries no shape
// information either way.
func unembedWidthsMatch(objects map[int]*pdfObject, dict pdfDict, target string) bool {
	wVal, ok := dict["/Widths"]
	if !ok {
		return true
	}
	arr, ok := resolveRef(objects, wVal).(pdfArray)
	if !ok {
		return true
	}
	std, ok := standard14Widths(target)
	if !ok {
		return false
	}
	firstChar := dictGetInt(dict, "/FirstChar")
	for i, v := range arr {
		code := firstChar + i
		if code < 0 || code > 255 {
			continue
		}
		w := operandFloat(v)
		if w == 0 {
			continue
		}
		if math.Abs(w-std[code]) > unembedWidthTolerance {
			return false
		}
	}
	return true
}

// unembedFonts drops the embedded font program from every simple
// (Type1/TrueType/MMType1) font whose BaseFont and /Widths are both
// metric-compatible with a Helvetica/Times/Courier Standard-14 face,
// rewriting the font dict in place (same object number, so content streams
// and resource refs are untouched) to rely on the renderer's non-embedded
// substitution instead (render_fontrepo.go: registered font → installed
// metric-equivalent → bundled clone — the same path a document that was
// never embedded in the first place already takes). The FontDescriptor keeps
// every other entry (/Flags, /FontBBox, /Ascent, …) — only the font-program
// keys (/FontFile, /FontFile2, /FontFile3, and /Length1-3 which describe
// only FontFile's own segmentation) are removed. Composite (Type0/CID) and
// Type3 fonts are out of scope — Standard-14 is Latin-only, so there is no
// substitute story for a CID-keyed font; Symbol/ZapfDingbats are excluded by
// unembedTargetName for the same reason. Lossy in the sense Aspose.PDF for
// .NET's OptimizationOptions.UnembedFonts documents it: an exotic subset of
// glyphs the original font drew slightly differently than the substitute
// (accented Latin, ligatures) may render with the substitute's own shapes —
// advance widths are untouched either way, so layout does not shift. Returns
// the number of fonts unembedded; the orphaned FontFile stream objects are
// left for the caller's RemoveUnusedObjects pass (mirrors Optimize's own
// images → fonts → … → remove-unused-objects ordering).
func (d *Document) unembedFonts() int {
	n := 0
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
		fd, ok := resolveRefToDict(d.objects, dict["/FontDescriptor"])
		if !ok {
			continue
		}
		hasFile := false
		for _, k := range []string{"/FontFile", "/FontFile2", "/FontFile3"} {
			if _, ok := fd[k]; ok {
				hasFile = true
				break
			}
		}
		if !hasFile {
			continue
		}
		target := unembedTargetName(dictGetName(dict, "/BaseFont"))
		if target == "" {
			continue
		}
		if !unembedWidthsMatch(d.objects, dict, target) {
			continue
		}
		for _, k := range []string{"/FontFile", "/FontFile2", "/FontFile3", "/Length1", "/Length2", "/Length3"} {
			delete(fd, k)
		}
		n++
	}
	return n
}

// UnembedFonts drops the embedded font program from every simple font whose
// name and metrics are those of a Helvetica/Times/Courier Standard-14 face
// (see unembedFonts for the full eligibility rule), then reclaims the
// now-orphaned font-program stream objects. Call before Save/WriteTo.
// Returns the number of fonts unembedded. Mirrors Aspose.PDF for .NET's
// OptimizationOptions.UnembedFonts (phase 2 of the Optimize epic,
// pdf-go-o0wy/pdf-go-vb4b) — opt-in (not part of DefaultOptimizationOptions)
// since, unlike SubsetFonts, it can change how the document actually looks
// in a viewer that substitutes a different Arial/Times/Courier clone.
func (d *Document) UnembedFonts() (int, error) {
	n := d.unembedFonts()
	if n > 0 {
		d.RemoveUnusedObjects()
	}
	return n, nil
}
