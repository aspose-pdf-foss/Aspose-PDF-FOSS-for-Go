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
