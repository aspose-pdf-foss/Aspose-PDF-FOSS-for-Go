package asposepdf

// BorderSide is a bitmask selecting which sides of a rectangular border are drawn.
type BorderSide int

const (
	BorderSideNone   BorderSide = 0
	BorderSideTop    BorderSide = 1
	BorderSideRight  BorderSide = 2
	BorderSideBottom BorderSide = 4
	BorderSideLeft   BorderSide = 8
	BorderSideAll               = BorderSideTop | BorderSideRight | BorderSideBottom | BorderSideLeft
)

// BorderInfo describes a border drawn around a table or cell.
// Mirrors Aspose.PDF for .NET's BorderInfo. Zero value means "no border".
type BorderInfo struct {
	Sides BorderSide
	Width float64 // in points; 0 means no border regardless of Sides
	Color *Color  // nil → black (R:0 G:0 B:0 A:1)
}

// MarginInfo describes margins or padding in points: Top / Right / Bottom / Left.
// Inside a Cell, MarginInfo represents the padding between the cell's border
// and its text content. Mirrors Aspose.PDF for .NET's MarginInfo.
type MarginInfo struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

// Table is a transient builder for a tabular layout drawn onto a Page.
// Mirrors Aspose.PDF for .NET's Table class. After (*Page).AddTable renders
// the table, the *Table is not held by the document.
type Table struct {
	columnWidths       []float64
	border             BorderInfo
	defaultCellBorder  BorderInfo
	defaultCellMargin  MarginInfo
	defaultCellStyle   TextStyle
	rows               []*Row
	repeatingRowsCount int
	overflowTop        float64 // 0 = use default 50
	overflowBottom     float64 // 0 = use default 50
	overflowSet        bool    // true once SetOverflowMargins has been called
}

// Row is a single row within a Table.
type Row struct {
	table  *Table
	cells  []*Cell
	height float64 // 0 = auto-fit
	// Phase 3: row-level styling layer between table defaults and per-cell overrides.
	background *Color
	textStyle  *TextStyle
	border     *BorderInfo
	margin     *MarginInfo
}

// Cell is a single cell within a Row.
type Cell struct {
	row        *Row
	text       string
	style      *TextStyle
	background *Color
	border     *BorderInfo
	margin     *MarginInfo
	hAlign     HAlign
	vAlign     VAlign
	hAlignSet  bool
	vAlignSet  bool
	colSpan    int // 0 == default 1
	rowSpan    int // 0 == default 1
}

// NewTable returns an empty table. Configure via Set* methods + AddRow.
// Mirrors Aspose.PDF for .NET's `new Table()` constructor.
func NewTable() *Table { return &Table{} }

// SetColumnWidths sets the column widths in points. Defensive-copies the slice.
func (t *Table) SetColumnWidths(widths []float64) *Table {
	cp := make([]float64, len(widths))
	copy(cp, widths)
	t.columnWidths = cp
	return t
}

// ColumnWidths returns a copy of the column widths.
func (t *Table) ColumnWidths() []float64 {
	cp := make([]float64, len(t.columnWidths))
	copy(cp, t.columnWidths)
	return cp
}

func (t *Table) SetBorder(b BorderInfo) *Table { t.border = b; return t }
func (t *Table) Border() BorderInfo            { return t.border }

func (t *Table) SetDefaultCellBorder(b BorderInfo) *Table { t.defaultCellBorder = b; return t }
func (t *Table) DefaultCellBorder() BorderInfo            { return t.defaultCellBorder }

func (t *Table) SetDefaultCellMargin(m MarginInfo) *Table { t.defaultCellMargin = m; return t }
func (t *Table) DefaultCellMargin() MarginInfo            { return t.defaultCellMargin }

func (t *Table) SetDefaultCellStyle(s TextStyle) *Table { t.defaultCellStyle = s; return t }
func (t *Table) DefaultCellStyle() TextStyle            { return t.defaultCellStyle }

// AddRow appends an empty row and returns it for further configuration.
func (t *Table) AddRow() *Row {
	r := &Row{table: t}
	t.rows = append(t.rows, r)
	return r
}

// AddRows is a convenience that creates one Row per inner slice and one Cell
// per string in that slice. Returns the rows in insertion order so callers
// can apply further per-row customization (SetBackground, SetHeight, etc.).
//
// Rows added through AddRows have plain cells — no ColSpan/RowSpan. For
// spanning, use the explicit AddRow + Cell.SetColSpan flow.
func (t *Table) AddRows(rows [][]string) []*Row {
	out := make([]*Row, 0, len(rows))
	for _, texts := range rows {
		r := t.AddRow()
		r.AddCells(texts...)
		out = append(out, r)
	}
	return out
}

// Rows returns the rows in order. The slice is the live backing — do not mutate.
func (t *Table) Rows() []*Row { return t.rows }

// RowCount returns the number of rows.
func (t *Table) RowCount() int { return len(t.rows) }

// Table returns the owning table.
func (r *Row) Table() *Table { return r.table }

// AddCell appends a cell with the given text.
func (r *Row) AddCell(text string) *Cell {
	c := &Cell{row: r, text: text}
	r.cells = append(r.cells, c)
	return c
}

// AddCells is a convenience that calls AddCell for each text in order.
func (r *Row) AddCells(texts ...string) []*Cell {
	out := make([]*Cell, len(texts))
	for i, s := range texts {
		out[i] = r.AddCell(s)
	}
	return out
}

// Cells returns the row's cells. The slice is the live backing — do not mutate.
func (r *Row) Cells() []*Cell { return r.cells }

// CellCount returns the number of cells in this row.
func (r *Row) CellCount() int { return len(r.cells) }

// SetHeight sets the row's drawn height in points. Pass 0 to use auto-fit
// (the renderer measures cell contents to compute a row height that fits).
func (r *Row) SetHeight(h float64) *Row { r.height = h; return r }

// Height returns the configured row height. 0 means auto-fit.
func (r *Row) Height() float64 { return r.height }

// Row returns the owning row.
func (c *Cell) Row() *Row { return c.row }

func (c *Cell) SetText(text string) *Cell { c.text = text; return c }
func (c *Cell) Text() string              { return c.text }

// SetTextStyle overrides the table's DefaultCellStyle for this cell.
func (c *Cell) SetTextStyle(s TextStyle) *Cell {
	c.style = &s
	return c
}

// TextStyle returns the per-cell style override (or nil if the cell inherits the table default).
func (c *Cell) TextStyle() *TextStyle { return c.style }

func (c *Cell) SetBackground(col *Color) *Cell { c.background = col; return c }
func (c *Cell) Background() *Color             { return c.background }

func (c *Cell) SetBorder(b BorderInfo) *Cell {
	c.border = &b
	return c
}
func (c *Cell) Border() *BorderInfo { return c.border }

func (c *Cell) SetMargin(m MarginInfo) *Cell {
	c.margin = &m
	return c
}
func (c *Cell) Margin() *MarginInfo { return c.margin }

func (c *Cell) SetHAlign(h HAlign) *Cell { c.hAlign = h; c.hAlignSet = true; return c }
func (c *Cell) SetVAlign(v VAlign) *Cell { c.vAlign = v; c.vAlignSet = true; return c }

// SetColSpan sets the column span (cells the cell occupies horizontally).
// Default 1. Mirrors Aspose.PDF for .NET's Cell.ColSpan.
//
// When colSpan > 1, the caller does NOT add cells for the positions covered
// by the span — the row simply has fewer cells.
func (c *Cell) SetColSpan(n int) *Cell { c.colSpan = n; return c }

// ColSpan returns the cell's column span (1 if unset).
func (c *Cell) ColSpan() int {
	if c.colSpan < 1 {
		return 1
	}
	return c.colSpan
}

// SetRowSpan sets the row span (rows the cell occupies vertically).
// Default 1. Mirrors Aspose.PDF for .NET's Cell.RowSpan.
//
// When rowSpan > 1, the caller does NOT add cells in subsequent rows for the
// positions covered by the span — those rows simply have fewer cells.
func (c *Cell) SetRowSpan(n int) *Cell { c.rowSpan = n; return c }

// RowSpan returns the cell's row span (1 if unset).
func (c *Cell) RowSpan() int {
	if c.rowSpan < 1 {
		return 1
	}
	return c.rowSpan
}

// SetRepeatingRowsCount marks the first n rows as headers that repeat at the
// top of every continuation page. Default 0 (no repeat).
//
// Mirrors Aspose.PDF for .NET's Table.RepeatingRowsCount property.
func (t *Table) SetRepeatingRowsCount(n int) *Table {
	t.repeatingRowsCount = n
	return t
}

// RepeatingRowsCount returns the number of header rows that repeat on each
// continuation page (default 0).
func (t *Table) RepeatingRowsCount() int { return t.repeatingRowsCount }

// SetOverflowMargins sets the top/bottom margins (in points) used to compute
// the continuation-page bounding rectangle when the table overflows the
// original rect. Defaults: 50pt on each side.
//
// The continuation rect uses the same LLX/URX as the original rect; the Y
// range becomes [bottom, pageHeight - top].
func (t *Table) SetOverflowMargins(top, bottom float64) *Table {
	t.overflowTop = top
	t.overflowBottom = bottom
	t.overflowSet = true
	return t
}

// OverflowMargins returns the configured overflow margins (defaults 50/50 if
// SetOverflowMargins has not been called).
func (t *Table) OverflowMargins() (top, bottom float64) {
	if !t.overflowSet {
		return 50, 50
	}
	return t.overflowTop, t.overflowBottom
}

// SetBackground sets a row-level background color. Cells in the row inherit
// this background unless the cell itself calls SetBackground.
func (r *Row) SetBackground(col *Color) *Row { r.background = col; return r }

// Background returns the row-level background color, or nil if unset.
func (r *Row) Background() *Color { return r.background }

// SetTextStyle sets a row-level default text style. Cells in the row inherit
// this style (overlaid on table.DefaultCellStyle) unless overridden per-cell.
func (r *Row) SetTextStyle(s TextStyle) *Row { r.textStyle = &s; return r }

// TextStyle returns the row-level text style override, or nil if unset.
func (r *Row) TextStyle() *TextStyle { return r.textStyle }

// SetBorder sets a row-level default border. Cells inherit unless overridden.
func (r *Row) SetBorder(b BorderInfo) *Row { r.border = &b; return r }

// Border returns the row-level border override, or nil if unset.
func (r *Row) Border() *BorderInfo { return r.border }

// SetMargin sets a row-level default cell padding. Cells inherit unless overridden.
func (r *Row) SetMargin(m MarginInfo) *Row { r.margin = &m; return r }

// Margin returns the row-level margin override, or nil if unset.
func (r *Row) Margin() *MarginInfo { return r.margin }
