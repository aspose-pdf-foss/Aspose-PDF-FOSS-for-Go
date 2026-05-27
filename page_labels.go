// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// PageLabelStyle is the numbering style applied within a PageLabelRange.
// Mirrors the /S values from PDF spec §12.4.2 Table 159.
type PageLabelStyle int

const (
	PageLabelStyleNone       PageLabelStyle = iota // no /S — only the prefix is rendered
	PageLabelDecimal                               // /D — 1, 2, 3, …
	PageLabelRomanLower                            // /r — i, ii, iii, …
	PageLabelRomanUpper                            // /R — I, II, III, …
	PageLabelAlphabeticLower                       // /a — a, b, …, z, aa, ab, …
	PageLabelAlphabeticUpper                       // /A — A, B, …, Z, AA, AB, …
)

// PageLabelRange describes a contiguous run of pages that share a numbering
// style. The first range must start at page 1; subsequent ranges inherit
// every preceding range's style/prefix until overridden. Mirrors the entries
// of a /PageLabels /Nums leaf per PDF spec §12.4.2.
type PageLabelRange struct {
	StartPage int            // 1-based page where this range begins
	Style     PageLabelStyle // numbering style
	Prefix    string         // optional label prefix (e.g. "A-")
	StartNum  int            // first number for this range; treated as 1 if ≤ 0
}

// SetPageLabels installs the document's /PageLabels number tree so PDF
// viewers display labels like "i, ii, 1, 2, …" in the navigation pane and
// page indicators. (*Page).Label() also reads from this tree.
//
// Ranges must be sorted by StartPage in strictly ascending order and the
// first range must begin at page 1; otherwise an error is returned.
// Passing a nil/empty slice clears any existing labels.
//
// Per PDF spec §12.4.2, /PageLabels is a number tree keyed by 0-based page
// index. This implementation emits a single-leaf number tree with one
// /Nums array, which is valid for any number of ranges.
func (d *Document) SetPageLabels(ranges []PageLabelRange) error {
	if len(ranges) == 0 {
		d.ClearPageLabels()
		return nil
	}
	if ranges[0].StartPage != 1 {
		return fmt.Errorf("page labels: first range must start at page 1, got %d", ranges[0].StartPage)
	}
	for i := 1; i < len(ranges); i++ {
		if ranges[i].StartPage <= ranges[i-1].StartPage {
			return fmt.Errorf("page labels: ranges must be sorted ascending by StartPage (range %d starts at %d, previous at %d)",
				i, ranges[i].StartPage, ranges[i-1].StartPage)
		}
	}

	nums := make(pdfArray, 0, len(ranges)*2)
	for _, r := range ranges {
		labelDict := pdfDict{}
		if name := pageLabelStyleName(r.Style); name != "" {
			labelDict["/S"] = pdfName(name)
		}
		if r.Prefix != "" {
			labelDict["/P"] = r.Prefix
		}
		if r.StartNum > 1 {
			labelDict["/St"] = r.StartNum
		}
		nums = append(nums, r.StartPage-1, labelDict)
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	d.catalog["/PageLabels"] = pdfDict{"/Nums": nums}
	return nil
}

// ClearPageLabels removes the document's /PageLabels entry. (*Page).Label()
// then falls back to the decimal page number.
func (d *Document) ClearPageLabels() {
	if d.catalog == nil {
		return
	}
	delete(d.catalog, "/PageLabels")
}

// pageLabelStyleName returns the PDF /S name for a style, or "" for
// PageLabelStyleNone (which means: emit no /S entry; only the prefix
// appears in the label).
func pageLabelStyleName(s PageLabelStyle) string {
	switch s {
	case PageLabelDecimal:
		return "/D"
	case PageLabelRomanLower:
		return "/r"
	case PageLabelRomanUpper:
		return "/R"
	case PageLabelAlphabeticLower:
		return "/a"
	case PageLabelAlphabeticUpper:
		return "/A"
	}
	return ""
}

// Label returns the formatted page label for this page as defined by the
// document's /PageLabels number tree (PDF spec §12.4.2).
//
// Common label styles:
//   - /D — decimal integers: 1, 2, 3, …
//   - /r — lowercase roman: i, ii, iii, …
//   - /R — uppercase roman: I, II, III, …
//   - /a — lowercase alphabetic: a, b, …, z, aa, ab, …
//   - /A — uppercase alphabetic: A, B, …, Z, AA, AB, …
//
// A /P prefix string is prepended when present (e.g. "A-1", "A-2").
//
// If the document has no /PageLabels entry, the decimal page number is returned.
func (p *Page) Label() string {
	label, err := computePageLabel(p.doc, p.index)
	if err != nil {
		return fmt.Sprintf("%d", p.index+1)
	}
	return label
}

// computePageLabel returns the formatted label for the page at 0-based pageIndex.
func computePageLabel(doc *Document, pageIndex int) (string, error) {
	labelsVal, ok := doc.catalog["/PageLabels"]
	if !ok {
		return fmt.Sprintf("%d", pageIndex+1), nil
	}
	pairs, err := flattenNumberTree(doc.objects, labelsVal)
	if err != nil || len(pairs) == 0 {
		return fmt.Sprintf("%d", pageIndex+1), nil
	}

	// Find the entry with the largest key ≤ pageIndex.
	rangeStart := 0
	var labelDict pdfDict
	for _, pair := range pairs {
		if pair.key <= pageIndex {
			rangeStart = pair.key
			labelDict = pair.dict
		}
	}

	return formatPageLabel(labelDict, pageIndex-rangeStart), nil
}

// numberTreeEntry is a single key→dict pair from a PDF number tree.
type numberTreeEntry struct {
	key  int
	dict pdfDict
}

// flattenNumberTree recursively collects all (key, dict) pairs from a PDF number tree.
func flattenNumberTree(objects map[int]*pdfObject, nodeVal pdfValue) ([]numberTreeEntry, error) {
	node, ok := resolveRefToDict(objects, nodeVal)
	if !ok {
		return nil, fmt.Errorf("number tree node is not a dict")
	}

	// Leaf node: /Nums [key value key value ...]
	if numsVal, ok := node["/Nums"]; ok {
		arr, ok := resolveRefToArray(objects, numsVal)
		if !ok {
			return nil, fmt.Errorf("/Nums is not an array")
		}
		var entries []numberTreeEntry
		for i := 0; i+1 < len(arr); i += 2 {
			key := toInt(arr[i])
			d, ok := resolveRefToDict(objects, arr[i+1])
			if !ok {
				continue
			}
			entries = append(entries, numberTreeEntry{key: key, dict: d})
		}
		return entries, nil
	}

	// Intermediate node: /Kids [child child ...]
	if kidsVal, ok := node["/Kids"]; ok {
		arr, ok := resolveRefToArray(objects, kidsVal)
		if !ok {
			return nil, fmt.Errorf("/Kids is not an array")
		}
		var entries []numberTreeEntry
		for _, kid := range arr {
			sub, err := flattenNumberTree(objects, kid)
			if err != nil {
				continue
			}
			entries = append(entries, sub...)
		}
		return entries, nil
	}

	return nil, nil
}

// formatPageLabel formats a page label dict entry for the given offset within its range.
func formatPageLabel(d pdfDict, offset int) string {
	prefix := ""
	if p, ok := d["/P"].(string); ok {
		prefix = p
	}

	start := 1
	if st, ok := d["/St"].(int); ok && st >= 1 {
		start = st
	}

	n := start + offset
	style := dictGetName(d, "/S")
	switch style {
	case "/D":
		return prefix + fmt.Sprintf("%d", n)
	case "/r":
		return prefix + toRoman(n, false)
	case "/R":
		return prefix + toRoman(n, true)
	case "/a":
		return prefix + toAlpha(n, false)
	case "/A":
		return prefix + toAlpha(n, true)
	default:
		// No /S — label is just the prefix.
		return prefix
	}
}

// toRoman converts n to a Roman numeral string.
// upper controls whether the result is upper- or lower-case.
func toRoman(n int, upper bool) string {
	if n <= 0 {
		return ""
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	s := b.String()
	if upper {
		return strings.ToUpper(s)
	}
	return s
}

// toAlpha converts n to an alphabetic label: 1→a, 2→b, …, 26→z, 27→aa, 28→ab, …
// upper controls whether the result is upper- or lower-case.
func toAlpha(n int, upper bool) string {
	if n <= 0 {
		return ""
	}
	n-- // convert to 0-based
	var buf []byte
	for {
		buf = append(buf, byte('a'+n%26))
		if n < 26 {
			break
		}
		n = n/26 - 1
	}
	// Reverse to get the correct order.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	s := string(buf)
	if upper {
		return strings.ToUpper(s)
	}
	return s
}
