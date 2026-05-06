package asposepdf

// Point is a single point in PDF user-space coordinates.
type Point struct {
	X, Y float64
}

// BorderStyle controls the /BS dict for drawing annotations per
// ISO 32000-1 §12.5.4 Table 168.
type BorderStyle int

const (
	BorderSolid     BorderStyle = iota // /S = /S
	BorderDashed                       // /S = /D + /D dash array
	BorderBeveled                      // /S = /B (3D raised effect)
	BorderInset                        // /S = /I (3D recessed effect)
	BorderUnderline                    // /S = /U (only the bottom edge)
)

// LineEndingStyle is one of the 10 line-ending shapes per ISO 32000-1
// §12.5.6.7 Table 176, used in /Line annotations' /LE entry.
type LineEndingStyle int

const (
	LineEndingNone         LineEndingStyle = iota
	LineEndingSquare
	LineEndingCircle
	LineEndingDiamond
	LineEndingOpenArrow
	LineEndingClosedArrow
	LineEndingButt
	LineEndingROpenArrow   // OpenArrow rotated 180° (away from line)
	LineEndingRClosedArrow // ClosedArrow rotated 180°
	LineEndingSlash
)
