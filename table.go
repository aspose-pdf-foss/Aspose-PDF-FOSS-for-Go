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
	columnWidths      []float64
	border            BorderInfo
	defaultCellBorder BorderInfo
	defaultCellMargin MarginInfo
	defaultCellStyle  TextStyle
	rows              []*Row
}

// Row is a single row within a Table.
type Row struct {
	table  *Table
	cells  []*Cell
	height float64 // 0 = auto-fit
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
