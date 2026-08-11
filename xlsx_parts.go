// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// SpreadsheetML OPC parts for the XLSX writer (epic pdf-go-3zgg). The same
// string-templating approach as docx_parts.go: transitional ECMA-376 markup,
// xmlEscape for text, no encoding/xml marshalling. Cell strings are inline
// (t="inlineStr") so there is no sharedStrings part.

const xlsxNSMain = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const xlsxNSRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

// xlsxContentTypes lists every part; one Override per worksheet.
func xlsxContentTypes(sheetCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for i := 1; i <= sheetCount; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	b.WriteString(`</Types>`)
	return b.String()
}

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`

// xlsxWorkbook lists the sheets; sheet rIds are rId1..rIdN, styles follow.
func xlsxWorkbook(sheetNames []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&b, `<workbook xmlns="%s" xmlns:r="%s"><sheets>`, xlsxNSMain, xlsxNSRel)
	for i, name := range sheetNames {
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(sheetSafeName(name)), i+1, i+1)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func xlsxWorkbookRels(sheetCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= sheetCount; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, sheetCount+1)
	b.WriteString(`</Relationships>`)
	return b.String()
}

// sheetSafeName clips to Excel's 31-char limit and strips forbidden chars.
func sheetSafeName(s string) string {
	repl := strings.NewReplacer(":", "-", "\\", "-", "/", "-", "?", "", "*", "", "[", "(", "]", ")")
	s = repl.Replace(s)
	r := []rune(s)
	if len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		s = "Sheet"
	}
	return s
}

// colLetters converts a 0-based column index to A1-style letters.
func colLetters(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}

// --- styles -----------------------------------------------------------------

// xlsxStyle is a deduplicable cell look. Zero value = default style (index 0).
type xlsxStyle struct {
	bold      bool
	italic    bool
	fontRGB   string // "FF0000" or "" for automatic
	fillRGB   string // cell background or ""
	alignH    string // "", "left", "center", "right"
	numFmt    string // "" = General; custom format code otherwise
	wrapText  bool
	vAlignTop bool
}

// xlsxStyleRegistry interns styles into cellXfs indexes.
type xlsxStyleRegistry struct {
	styles []xlsxStyle
	index  map[xlsxStyle]int
}

func newXlsxStyleRegistry() *xlsxStyleRegistry {
	r := &xlsxStyleRegistry{index: map[xlsxStyle]int{}}
	r.styles = append(r.styles, xlsxStyle{}) // xf 0 = default
	r.index[xlsxStyle{}] = 0
	return r
}

func (r *xlsxStyleRegistry) intern(s xlsxStyle) int {
	if i, ok := r.index[s]; ok {
		return i
	}
	r.styles = append(r.styles, s)
	r.index[s] = len(r.styles) - 1
	return len(r.styles) - 1
}

// build serializes styles.xml. Fonts, fills and numFmts are themselves
// deduplicated; custom numFmt ids start at 164 per the spec.
func (r *xlsxStyleRegistry) build() string {
	type fontKey struct {
		bold, italic bool
		rgb          string
	}
	fonts := []fontKey{{}}
	fontIdx := map[fontKey]int{{}: 0}
	fills := []string{"", "gray125"} // fill 0 none, fill 1 gray125 (required pair)
	fillIdx := map[string]int{"": 0}
	numFmts := []string{}
	numFmtIdx := map[string]int{}

	for _, s := range r.styles {
		fk := fontKey{s.bold, s.italic, s.fontRGB}
		if _, ok := fontIdx[fk]; !ok {
			fontIdx[fk] = len(fonts)
			fonts = append(fonts, fk)
		}
		if s.fillRGB != "" {
			if _, ok := fillIdx[s.fillRGB]; !ok {
				fillIdx[s.fillRGB] = len(fills)
				fills = append(fills, s.fillRGB)
			}
		}
		if s.numFmt != "" {
			if _, ok := numFmtIdx[s.numFmt]; !ok {
				numFmtIdx[s.numFmt] = 164 + len(numFmts)
				numFmts = append(numFmts, s.numFmt)
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&b, `<styleSheet xmlns="%s">`, xlsxNSMain)
	if len(numFmts) > 0 {
		fmt.Fprintf(&b, `<numFmts count="%d">`, len(numFmts))
		for i, f := range numFmts {
			fmt.Fprintf(&b, `<numFmt numFmtId="%d" formatCode="%s"/>`, 164+i, xmlEscape(f))
		}
		b.WriteString(`</numFmts>`)
	}
	fmt.Fprintf(&b, `<fonts count="%d">`, len(fonts))
	for _, f := range fonts {
		b.WriteString(`<font>`)
		if f.bold {
			b.WriteString(`<b/>`)
		}
		if f.italic {
			b.WriteString(`<i/>`)
		}
		b.WriteString(`<sz val="11"/>`)
		if f.rgb != "" {
			fmt.Fprintf(&b, `<color rgb="FF%s"/>`, f.rgb)
		}
		b.WriteString(`<name val="Calibri"/></font>`)
	}
	b.WriteString(`</fonts>`)
	fmt.Fprintf(&b, `<fills count="%d">`, len(fills))
	b.WriteString(`<fill><patternFill patternType="none"/></fill>`)
	b.WriteString(`<fill><patternFill patternType="gray125"/></fill>`)
	for _, f := range fills[2:] {
		fmt.Fprintf(&b, `<fill><patternFill patternType="solid"><fgColor rgb="FF%s"/><bgColor indexed="64"/></patternFill></fill>`, f)
	}
	b.WriteString(`</fills>`)
	b.WriteString(`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>`)
	b.WriteString(`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)
	fmt.Fprintf(&b, `<cellXfs count="%d">`, len(r.styles))
	for _, s := range r.styles {
		nf := 0
		if s.numFmt != "" {
			nf = numFmtIdx[s.numFmt]
		}
		fi := fontIdx[fontKey{s.bold, s.italic, s.fontRGB}]
		fl := 0
		if s.fillRGB != "" {
			fl = fillIdx[s.fillRGB]
		}
		fmt.Fprintf(&b, `<xf numFmtId="%d" fontId="%d" fillId="%d" borderId="0"`, nf, fi, fl)
		if nf != 0 {
			b.WriteString(` applyNumberFormat="1"`)
		}
		if fi != 0 {
			b.WriteString(` applyFont="1"`)
		}
		if fl != 0 {
			b.WriteString(` applyFill="1"`)
		}
		if s.alignH != "" || s.wrapText || s.vAlignTop {
			b.WriteString(` applyAlignment="1"><alignment`)
			if s.alignH != "" {
				fmt.Fprintf(&b, ` horizontal="%s"`, s.alignH)
			}
			if s.vAlignTop {
				b.WriteString(` vertical="top"`)
			}
			if s.wrapText {
				b.WriteString(` wrapText="1"`)
			}
			b.WriteString(`/></xf>`)
		} else {
			b.WriteString(`/>`)
		}
	}
	b.WriteString(`</cellXfs>`)
	b.WriteString(`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>`)
	b.WriteString(`</styleSheet>`)
	return b.String()
}

// --- worksheet --------------------------------------------------------------

// xlsxCell is one resolved cell ready for serialization.
type xlsxCell struct {
	text    string
	num     *float64
	styleXf int
	// span anchors; covered cells are simply absent from the row map.
	colSpan, rowSpan int
}

// xlsxSheet is one worksheet: sparse rows of cells plus column widths.
type xlsxSheet struct {
	name   string
	widths []float64          // per column, Excel character units (0 = default)
	rows   map[int][]sheetRun // 0-based row → cells with explicit columns
	maxRow int
	merges []string // "A1:C2"
}

type sheetRun struct {
	col  int
	cell xlsxCell
}

func newXlsxSheet(name string) *xlsxSheet {
	return &xlsxSheet{name: name, rows: map[int][]sheetRun{}}
}

func (s *xlsxSheet) set(row, col int, c xlsxCell) {
	s.rows[row] = append(s.rows[row], sheetRun{col: col, cell: c})
	if row > s.maxRow {
		s.maxRow = row
	}
	if c.colSpan > 1 || c.rowSpan > 1 {
		cs, rs := c.colSpan, c.rowSpan
		if cs < 1 {
			cs = 1
		}
		if rs < 1 {
			rs = 1
		}
		s.merges = append(s.merges, fmt.Sprintf("%s%d:%s%d",
			colLetters(col), row+1, colLetters(col+cs-1), row+rs))
	}
}

// build serializes one worksheet part.
func (s *xlsxSheet) build() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&b, `<worksheet xmlns="%s">`, xlsxNSMain)
	if len(s.widths) > 0 {
		b.WriteString(`<cols>`)
		for i, w := range s.widths {
			if w <= 0 {
				continue
			}
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%.2f" customWidth="1"/>`, i+1, i+1, w)
		}
		b.WriteString(`</cols>`)
	}
	b.WriteString(`<sheetData>`)
	for r := 0; r <= s.maxRow; r++ {
		runs := s.rows[r]
		if len(runs) == 0 {
			continue
		}
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for _, run := range runs {
			ref := fmt.Sprintf("%s%d", colLetters(run.col), r+1)
			c := run.cell
			switch {
			case c.num != nil:
				fmt.Fprintf(&b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, c.styleXf, trimFloat(*c.num))
			case c.text != "":
				fmt.Fprintf(&b, `<c r="%s" s="%d" t="inlineStr"><is><t>%s</t></is></c>`,
					ref, c.styleXf, xmlEscape(c.text))
			case c.styleXf != 0:
				fmt.Fprintf(&b, `<c r="%s" s="%d"/>`, ref, c.styleXf)
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
	if len(s.merges) > 0 {
		fmt.Fprintf(&b, `<mergeCells count="%d">`, len(s.merges))
		for _, m := range s.merges {
			fmt.Fprintf(&b, `<mergeCell ref="%s"/>`, m)
		}
		b.WriteString(`</mergeCells>`)
	}
	b.WriteString(`</worksheet>`)
	return b.String()
}

// trimFloat formats a float compactly (no trailing zeros, no exponent for
// typical magnitudes).
func trimFloat(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
