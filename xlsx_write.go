// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// PDF → XLSX export (epic pdf-go-3zgg): converts detected tables (and, in
// FullPage mode, whole pages) into an Excel workbook. Mirrors the intent of
// Aspose.PDF for .NET's Document.Save(SaveFormat.Excel) + ExcelSaveOptions
// in this library's Document-method idiom. Built on the TableAbsorber
// (lattice + stream); pure stdlib OPC output (xlsx_parts.go).
//
// The converter's value over text-in-cells: numeric-looking cell texts
// (plain numbers, percentages, simple currency amounts) become real numeric
// cells with a matching number format, so SUM() over an exported column
// works immediately; merged cells carry over; header shading, bold and
// alignment survive. Design: docs/superpowers/specs/2026-08-11-pdf-to-xlsx-
// design.md.

// XlsxRecognitionMode selects what lands in the workbook.
type XlsxRecognitionMode int

const (
	// XlsxTablesOnly (default): every detected table, stacked on a single
	// "Tables" worksheet in document order — a "Page N" caption above the
	// first table of each page, one blank row between tables. Tables
	// continuing across consecutive pages with the same column signature
	// are stitched into one (the repeated header row is dropped).
	XlsxTablesOnly XlsxRecognitionMode = iota
	// XlsxFullPage: one worksheet per source page carrying ALL page text —
	// layout lines become rows, wide intra-line gaps split cells, detected
	// tables keep their exact logical grid.
	XlsxFullPage
)

// XlsxSaveOptions configures SaveXlsx / WriteXlsx.
type XlsxSaveOptions struct {
	// Pages is a 1-based subset (in the given order); nil = all pages.
	Pages []int
	// Mode selects the recognition mode (default XlsxTablesOnly).
	Mode XlsxRecognitionMode
	// NoStyles suppresses fills, fonts, alignment and column widths.
	NoStyles bool
}

// SaveXlsx writes the document as an Excel (.xlsx) workbook file.
func (d *Document) SaveXlsx(path string, opts ...XlsxSaveOptions) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save xlsx: %w", err)
	}
	werr := d.WriteXlsx(f, opts...)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return fmt.Errorf("save xlsx: %w", cerr)
	}
	return nil
}

// WriteXlsx writes the document as an Excel (.xlsx) workbook to w.
func (d *Document) WriteXlsx(w io.Writer, opts ...XlsxSaveOptions) error {
	var opt XlsxSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Mode != XlsxTablesOnly && opt.Mode != XlsxFullPage {
		return fmt.Errorf("WriteXlsx: unsupported recognition mode %d", opt.Mode)
	}
	pages := d.Pages()
	sel := opt.Pages
	if len(sel) == 0 {
		sel = make([]int, len(pages))
		for i := range pages {
			sel[i] = i + 1
		}
	} else {
		for _, n := range sel {
			if n < 1 || n > len(pages) {
				return fmt.Errorf("WriteXlsx: page %d out of range 1..%d", n, len(pages))
			}
		}
	}

	reg := newXlsxStyleRegistry()
	var sheets []*xlsxSheet
	var err error
	if opt.Mode == XlsxTablesOnly {
		sheets, err = buildXlsxTablesOnly(pages, sel, reg, opt)
	} else {
		sheets, err = buildXlsxFullPage(pages, sel, reg, opt)
	}
	if err != nil {
		return err
	}
	if len(sheets) == 0 {
		// A workbook must carry at least one sheet.
		sheets = append(sheets, newXlsxSheet("Tables"))
	}

	names := make([]string, len(sheets))
	parts := []docxPart{}
	for i, s := range sheets {
		names[i] = s.name
		parts = append(parts, docxPart{fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), []byte(s.build())})
	}
	parts = append(parts,
		docxPart{"[Content_Types].xml", []byte(xlsxContentTypes(len(sheets)))},
		docxPart{"_rels/.rels", []byte(xlsxRootRels)},
		docxPart{"xl/workbook.xml", []byte(xlsxWorkbook(names))},
		docxPart{"xl/_rels/workbook.xml.rels", []byte(xlsxWorkbookRels(len(sheets)))},
		docxPart{"xl/styles.xml", []byte(reg.build())},
	)
	return writeDocxZip(w, parts)
}

// --- TablesOnly mode --------------------------------------------------------

// stitchKey is a table's column signature: boundaries relative to the table
// left edge, rounded, so tables continuing across pages compare equal.
func stitchKey(t *AbsorbedTable) string {
	var b strings.Builder
	for _, x := range t.colXs {
		fmt.Fprintf(&b, "%d;", int((x-t.Rect.LLX)/3+0.5))
	}
	return b.String()
}

// headerTexts returns the first row's cell texts (for repeated-header
// detection when stitching).
func headerTexts(t *AbsorbedTable) []string {
	if len(t.rows) == 0 {
		return nil
	}
	var out []string
	for _, c := range t.rows[0].cells {
		out = append(out, c.Text())
	}
	return out
}

func sameTexts(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildXlsxTablesOnly(pages []*Page, sel []int, reg *xlsxStyleRegistry, opt XlsxSaveOptions) ([]*xlsxSheet, error) {
	sheet := newXlsxSheet("Tables")
	row := 0
	lastPage := -1
	lastKey := ""
	var lastHeader []string
	captionXf := 0
	if !opt.NoStyles {
		captionXf = reg.intern(xlsxStyle{bold: true, fontRGB: "666666"})
	}

	for _, n := range sel {
		ta := NewTableAbsorber()
		if err := ta.Visit(pages[n-1]); err != nil {
			continue
		}
		for _, t := range ta.TableList() {
			key := stitchKey(t)
			rows := t.RowList()
			if len(rows) == 0 {
				continue
			}
			start := 0
			stitched := n == lastPage+1 && key == lastKey
			if stitched && sameTexts(headerTexts(t), lastHeader) {
				start = 1 // repeated header on the continuation page
			}
			if !stitched {
				if row > 0 {
					row++ // blank separator row
				}
				if n != lastPage {
					sheet.set(row, 0, xlsxCell{text: fmt.Sprintf("Page %d", n), styleXf: captionXf})
					row++
				}
				lastHeader = headerTexts(t)
			}
			appendTableRows(sheet, t, rows[start:], &row, reg, opt)
			// Column widths: keep the widest requirement seen per column.
			if !opt.NoStyles {
				for c := 0; c+1 < len(t.colXs); c++ {
					w := (t.colXs[c+1] - t.colXs[c]) / 5.1
					if w < 4 {
						w = 4
					} else if w > 80 {
						w = 80
					}
					for len(sheet.widths) <= c {
						sheet.widths = append(sheet.widths, 0)
					}
					if w > sheet.widths[c] {
						sheet.widths[c] = w
					}
				}
			}
			lastPage, lastKey = n, key
		}
	}
	if row == 0 && len(sheet.rows) == 0 {
		return nil, nil
	}
	return []*xlsxSheet{sheet}, nil
}

// appendTableRows emits the table's logical grid starting at *row.
func appendTableRows(sheet *xlsxSheet, t *AbsorbedTable, rows []*AbsorbedRow, row *int, reg *xlsxStyleRegistry, opt XlsxSaveOptions) {
	for _, tr := range rows {
		for c, cell := range tr.CellList() {
			if cell.Covered {
				continue
			}
			sheet.set(*row, c, makeXlsxCell(cell, reg, opt))
		}
		*row++
	}
}

// makeXlsxCell classifies the cell's text and look.
func makeXlsxCell(cell *AbsorbedCell, reg *xlsxStyleRegistry, opt XlsxSaveOptions) xlsxCell {
	text := cell.Text()
	// Multi-line cell text folds into one line with spaces (Excel cells are
	// single-line unless wrapped; wrapping tiny fragments hurts more).
	text = strings.Join(strings.Fields(text), " ")

	num, numFmt := parseCellNumber(text)

	var st xlsxStyle
	if !opt.NoStyles {
		if cell.Shading != nil {
			st.fillRGB = docxColor(*cell.Shading)
		}
		runs := absorbedCellRuns(cell)
		bold, italic, colored := false, false, ""
		for _, r := range runs {
			if r.text == "" {
				continue
			}
			if r.bold {
				bold = true
			}
			if r.italic {
				italic = true
			}
			if c := docxColor(r.color); c != "000000" && colored == "" {
				colored = c
			}
		}
		st.bold, st.italic, st.fontRGB = bold, italic, colored
		if num != nil {
			st.alignH = "right"
		}
	}
	st.numFmt = numFmt

	c := xlsxCell{colSpan: cell.ColSpan, rowSpan: cell.RowSpan}
	if num != nil {
		c.num = num
	} else {
		c.text = text
	}
	if st != (xlsxStyle{}) {
		c.styleXf = reg.intern(st)
	}
	return c
}

// --- numeric classification -------------------------------------------------

var currencyFmt = map[rune]string{
	'€': `#,##0.00\ "€"`,
	'$': `"$"#,##0.00`,
	'£': `"£"#,##0.00`,
	'¥': `"¥"#,##0.00`,
	'₽': `#,##0.00\ "₽"`,
}

// parseCellNumber recognizes plain numbers, percentages and simple currency
// amounts (one currency symbol, prefix or suffix, nothing else). Returns nil
// for anything ambiguous — a false negative just leaves text.
func parseCellNumber(text string) (*float64, string) {
	s := strings.TrimSpace(text)
	if s == "" || len(s) > 24 {
		return nil, ""
	}
	numFmt := ""
	percent := false

	// One currency symbol at either end.
	r := []rune(s)
	if f, ok := currencyFmt[r[0]]; ok {
		numFmt = f
		s = strings.TrimSpace(string(r[1:]))
	} else if f, ok := currencyFmt[r[len(r)-1]]; ok {
		numFmt = f
		s = strings.TrimSpace(string(r[:len(r)-1]))
	} else if strings.HasSuffix(s, "%") {
		percent = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	}
	if s == "" {
		return nil, ""
	}

	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "−"): // U+2212 minus sign
		neg = true
		s = s[len("−"):]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	if s == "" {
		return nil, ""
	}

	// Normalize separators: spaces (incl. NBSP/thin) are thousand
	// separators; the LAST '.' or ',' followed by 1-2 digits is the decimal
	// point; remaining ',' / '.' must group digits in threes.
	s = strings.NewReplacer(" ", "", " ", "", " ", "", " ", "").Replace(s)
	lastSep := -1
	for i, ch := range s {
		if ch == '.' || ch == ',' {
			lastSep = i
		}
	}
	intPart, fracPart := s, ""
	if lastSep >= 0 {
		tail := s[lastSep+1:]
		if len(tail) >= 1 && len(tail) <= 2 && allDigits(tail) {
			intPart, fracPart = s[:lastSep], tail
		} else if len(tail) == 3 && allDigits(tail) {
			// trailing group of three: thousands separator, no decimals
		} else {
			return nil, ""
		}
	}
	groups := strings.FieldsFunc(intPart, func(r rune) bool { return r == '.' || r == ',' })
	if len(groups) == 0 {
		return nil, ""
	}
	digits := ""
	for i, g := range groups {
		if !allDigits(g) || g == "" {
			return nil, ""
		}
		if i > 0 && len(g) != 3 {
			return nil, "" // malformed grouping
		}
		if i == 0 && len(groups) > 1 && len(g) > 3 {
			return nil, ""
		}
		digits += g
	}
	if len(digits) > 15 {
		return nil, ""
	}

	v := 0.0
	for _, ch := range digits {
		v = v*10 + float64(ch-'0')
	}
	if fracPart != "" {
		f := 0.0
		for _, ch := range fracPart {
			f = f*10 + float64(ch-'0')
		}
		v += f / math.Pow(10, float64(len(fracPart)))
		if numFmt == "" && !percent {
			numFmt = "#,##0.00"
		}
	}
	if neg {
		v = -v
	}
	if percent {
		v /= 100
		numFmt = "0%"
		if fracPart != "" {
			numFmt = "0.00%"
		}
	}
	return &v, numFmt
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- FullPage mode ----------------------------------------------------------

// buildXlsxFullPage lays every page's text into a worksheet: rows from the
// layout lines, cells split at wide gaps, detected tables on their grids.
func buildXlsxFullPage(pages []*Page, sel []int, reg *xlsxStyleRegistry, opt XlsxSaveOptions) ([]*xlsxSheet, error) {
	var sheets []*xlsxSheet
	for _, n := range sel {
		p := pages[n-1]
		sheet := newXlsxSheet(fmt.Sprintf("Page %d", n))
		ta := NewTableAbsorber()
		var tables []*AbsorbedTable
		if err := ta.Visit(p); err == nil {
			tables = ta.TableList()
		}
		var tableRects []Rectangle
		for _, t := range tables {
			tableRects = append(tableRects, t.Rect)
		}

		lines, err := p.ExtractTextWithLayout()
		if err != nil {
			continue
		}

		// Vertical bands: every table is one band; every non-table line is
		// its own band, ordered by Y descending.
		type band struct {
			top   float64
			table *AbsorbedTable
			row   streamRow
		}
		var bands []band
		for _, t := range tables {
			bands = append(bands, band{top: t.Rect.URY, table: t})
		}
		for _, line := range lines {
			if len(line.Fragments) == 0 || lineInsideAny(line, tableRects) {
				continue
			}
			sr := splitStreamRow(line)
			if len(sr.cells) == 0 {
				continue
			}
			bands = append(bands, band{top: sr.top, row: sr})
		}
		for i := 1; i < len(bands); i++ {
			for j := i; j > 0 && bands[j].top > bands[j-1].top; j-- {
				bands[j], bands[j-1] = bands[j-1], bands[j]
			}
		}

		// Global columns: cluster non-table cell left edges.
		var edges []float64
		for _, b := range bands {
			if b.table != nil {
				continue
			}
			for _, c := range b.row.cells {
				edges = append(edges, c.lo)
			}
		}
		cols := clusterEdges(edges, 6)

		row := 0
		for _, b := range bands {
			if b.table != nil {
				appendTableRows(sheet, b.table, b.table.RowList(), &row, reg, opt)
				row++ // gap after a table block
				continue
			}
			for _, c := range b.row.cells {
				col := nearestEdge(cols, c.lo)
				line := TextLine{Y: b.row.baseline, Fragments: c.frs}
				text := strings.Join(strings.Fields(lineText(line)), " ")
				num, numFmt := parseCellNumber(text)
				var st xlsxStyle
				st.numFmt = numFmt
				if num != nil && !opt.NoStyles {
					st.alignH = "right"
				}
				cell := xlsxCell{}
				if num != nil {
					cell.num = num
				} else {
					cell.text = text
				}
				if st != (xlsxStyle{}) {
					cell.styleXf = reg.intern(st)
				}
				if cell.text != "" || cell.num != nil {
					sheet.set(row, col, cell)
				}
			}
			row++
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

// lineText joins a line's fragments with gap-aware spaces.
func lineText(line TextLine) string {
	return streamCellText(streamCellSpan{frs: line.Fragments})
}

// clusterEdges merges close X positions into representative columns.
func clusterEdges(edges []float64, tol float64) []float64 {
	if len(edges) == 0 {
		return nil
	}
	sorted := append([]float64(nil), edges...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	var cols []float64
	for _, e := range sorted {
		if len(cols) == 0 || e-cols[len(cols)-1] > tol {
			cols = append(cols, e)
		}
	}
	return cols
}

// nearestEdge returns the index of the closest column.
func nearestEdge(cols []float64, x float64) int {
	best, bestD := 0, math.MaxFloat64
	for i, c := range cols {
		if d := math.Abs(c - x); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}
