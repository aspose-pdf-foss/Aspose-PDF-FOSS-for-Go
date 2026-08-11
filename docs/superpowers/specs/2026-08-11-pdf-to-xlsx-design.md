# PDF → XLSX Export — Design

Date: 2026-08-11 · Epic: pdf-go-xlsx (beads) · Status: approved

## Goal

Convert PDF documents to Excel workbooks, mirroring the intent of
Aspose.PDF for .NET's `Document.Save(SaveFormat.Excel)` + `ExcelSaveOptions`
in this library's Document-method idiom. Built on the existing
`TableAbsorber` (lattice + stream detection) — the epic that unlocked this
converter. Pure Go, zero dependencies, stdlib `archive/zip` only.

## Public API

- `(*Document).SaveXlsx(path string, opts ...XlsxSaveOptions) error`
- `(*Document).WriteXlsx(w io.Writer, opts ...XlsxSaveOptions) error`
- `XlsxSaveOptions{ Pages []int; Mode XlsxRecognitionMode; NoStyles bool }`
- `XlsxRecognitionMode`:
  - `XlsxTablesOnly` (zero value, default) — every detected table lands on a
    single worksheet named "Tables", stacked in document order, separated by
    one blank row, each preceded by a "Page N" caption cell (skipped when the
    previous table came from the same page). Mirrors Aspose's
    MinimizeTheNumberOfWorksheets shape per the user's choice.
  - `XlsxFullPage` — one worksheet per source page ("Page N"): the whole
    page's text is laid out into cells — layout lines become rows, wide
    intra-line gaps split cells (the stream detector's `splitStreamRow`),
    detected tables are placed on their exact logical grid. Nothing is lost;
    noisier than TablesOnly.

## Architecture (two layers, mirroring the DOCX writer)

1. **`xlsx_write.go` — model builder.** Walks selected pages, runs
   `NewTableAbsorber().Visit`, produces `xlsxSheet{name, cols []float64,
   rows []xlsxRow}` / `xlsxCell{text string; num *float64; numFmt int;
   style xlsxStyle; colSpan, rowSpan int; covered bool}`.
   - **Multi-page stitching (TablesOnly):** tables on consecutive pages with
     the same column signature (boundaries equal within ±3 pt after
     normalizing to table-left-relative coordinates) merge into one; the
     repeated header row (identical cell texts as the previous table's first
     row) is dropped.
   - **FullPage rows:** non-table lines → rows of gap-split cells; global
     sheet columns seeded from the union of cell left edges (clustered at
     6 pt); table regions override with their own colXs mapped into the
     sheet's column set.
2. **`xlsx_parts.go` — OPC serializer.** String-templated SpreadsheetML
   (the `docx_parts.go` pattern; `writeDocxZip` generalized to
   `writeOPCZip`): `[Content_Types].xml`, `_rels/.rels`, `xl/workbook.xml`
   + `xl/_rels/workbook.xml.rels`, `xl/worksheets/sheetN.xml`,
   `xl/styles.xml`. Strings are **inline** (`t="inlineStr"`) — no
   sharedStrings part in v1.

## Cell semantics (the value over text-in-cells)

- **Numbers become numbers.** A cell whose text is a plain number
  (`1234.56`, `-7`, `1 234,56` — thousand/decimal separators normalized),
  a percentage (`42%` → 0.42, numFmt `0%`), or a simple currency
  (`€17.00`, `$1,240.00`, `£9.99` — one leading/trailing currency symbol,
  no other text) is written as a numeric cell with the matching built-in or
  custom `numFmt` (e.g. `#,##0.00 "€"`), so SUM() works immediately.
  Anything ambiguous stays an inline string. Dates are NOT parsed in v1.
- **Merges** from `RowSpan`/`ColSpan` → `<mergeCells>`; `Covered`
  positions emit nothing.
- **Styles** (unless `NoStyles`): cell fill from `AbsorbedCell.Shading`,
  bold/italic/font colour from the dominant run (`absorbedCellRuns`),
  horizontal alignment (numbers right, else per fragment geometry), column
  widths from `colXs` deltas (pt → Excel character units ≈ pt/5.1, clamped
  to [4, 80]). Styles are deduplicated into `cellXfs` by a style-key map.

## Validation

Three-rung harness modeled on `tools/validate_docx.py` →
`tools/validate_xlsx.py`:
1. XSD against ECMA-376 transitional `sml.xsd` (schemas fetched next to the
   DOCX ones).
2. `openpyxl` round-trip (open, read every sheet/cell).
3. Excel COM (locally): open without repair, sheet/used-range sanity.
Corpus: all 1,014 openable documents convert panic-free; reference docs
(showcase bill + sales report, PdfWithTable, Binder1 payslip) checked by
hand in Excel — including that SUM over the bill's Total column works.

## Out of scope (v1)

Formulas, images/charts, FullPage cross-page stitching, date recognition,
RTL sheets, sharedStrings, defined names/tables (`tableParts`), CSV.
