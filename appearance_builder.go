package asposepdf

import (
	"bytes"
	"strconv"
	"strings"
)

// LineCap is the /J line cap style per ISO 32000-1 §8.4.3.3 Table 54.
type LineCap int

const (
	LineCapButt   LineCap = 0
	LineCapRound  LineCap = 1
	LineCapSquare LineCap = 2
)

// LineJoin is the /j line join style per ISO 32000-1 §8.4.3.4 Table 55.
type LineJoin int

const (
	LineJoinMiter LineJoin = 0
	LineJoinRound LineJoin = 1
	LineJoinBevel LineJoin = 2
)

// appearanceBuilder accumulates PDF content-stream operators for use as
// a Form XObject /AP/N body. Operators are emitted in PDF spec form,
// one per line, separated by newlines.
type appearanceBuilder struct {
	buf bytes.Buffer
}

func newAppearanceBuilder() *appearanceBuilder {
	return &appearanceBuilder{}
}

// Bytes returns the accumulated content-stream bytes.
func (ab *appearanceBuilder) Bytes() []byte {
	return ab.buf.Bytes()
}

// formatFloat formats f as a compact fixed-point decimal: up to 6
// decimal places (sub-micron precision at 72dpi, the de facto PDF
// industry convention), trailing zeros and trailing decimal point
// trimmed.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// PushState saves the current graphics state (q operator).
func (ab *appearanceBuilder) PushState() {
	ab.buf.WriteString("q\n")
}

// PopState restores the last saved graphics state (Q operator).
func (ab *appearanceBuilder) PopState() {
	ab.buf.WriteString("Q\n")
}

// ConcatMatrix concatenates the given 2x3 matrix to the CTM (cm operator).
func (ab *appearanceBuilder) ConcatMatrix(a, b, c, d, e, f float64) {
	ab.buf.WriteString(formatFloat(a))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(b))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(c))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(d))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(e))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(f))
	ab.buf.WriteString(" cm\n")
}

// SetLineWidth sets the stroke line width (w operator).
func (ab *appearanceBuilder) SetLineWidth(w float64) {
	ab.buf.WriteString(formatFloat(w))
	ab.buf.WriteString(" w\n")
}

// SetLineCap sets the line-cap style (J operator).
func (ab *appearanceBuilder) SetLineCap(c LineCap) {
	ab.buf.WriteString(strconv.Itoa(int(c)))
	ab.buf.WriteString(" J\n")
}

// SetLineJoin sets the line-join style (j operator).
func (ab *appearanceBuilder) SetLineJoin(j LineJoin) {
	ab.buf.WriteString(strconv.Itoa(int(j)))
	ab.buf.WriteString(" j\n")
}

// SetMiterLimit sets the miter limit (M operator).
func (ab *appearanceBuilder) SetMiterLimit(m float64) {
	ab.buf.WriteString(formatFloat(m))
	ab.buf.WriteString(" M\n")
}

// SetDashPattern sets the line-dash pattern (d operator). A nil or empty
// pattern emits "[] phase d", which means a solid line.
func (ab *appearanceBuilder) SetDashPattern(pattern []float64, phase float64) {
	ab.buf.WriteByte('[')
	for i, v := range pattern {
		if i > 0 {
			ab.buf.WriteByte(' ')
		}
		ab.buf.WriteString(formatFloat(v))
	}
	ab.buf.WriteByte(']')
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(phase))
	ab.buf.WriteString(" d\n")
}

// SetStrokeColorRGB sets the stroke color to RGB (RG operator). The
// Alpha field of c is ignored; PDF transparency requires a separate
// gs operator with /CA in /ExtGState — out of scope for this builder.
func (ab *appearanceBuilder) SetStrokeColorRGB(c Color) {
	ab.buf.WriteString(formatFloat(c.R))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(c.G))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(c.B))
	ab.buf.WriteString(" RG\n")
}

// SetFillColorRGB sets the fill color to RGB (rg operator). The Alpha
// field of c is ignored; PDF transparency requires a separate gs
// operator with /ca in /ExtGState — out of scope for this builder.
func (ab *appearanceBuilder) SetFillColorRGB(c Color) {
	ab.buf.WriteString(formatFloat(c.R))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(c.G))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(c.B))
	ab.buf.WriteString(" rg\n")
}

// SetStrokeGray sets the stroke color to a grayscale value (G operator).
func (ab *appearanceBuilder) SetStrokeGray(g float64) {
	ab.buf.WriteString(formatFloat(g))
	ab.buf.WriteString(" G\n")
}

// SetFillGray sets the fill color to a grayscale value (g operator).
func (ab *appearanceBuilder) SetFillGray(g float64) {
	ab.buf.WriteString(formatFloat(g))
	ab.buf.WriteString(" g\n")
}

// MoveTo begins a new subpath at (x, y) (m operator).
func (ab *appearanceBuilder) MoveTo(x, y float64) {
	ab.buf.WriteString(formatFloat(x))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y))
	ab.buf.WriteString(" m\n")
}

// LineTo appends a straight line segment to (x, y) (l operator).
func (ab *appearanceBuilder) LineTo(x, y float64) {
	ab.buf.WriteString(formatFloat(x))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y))
	ab.buf.WriteString(" l\n")
}

// CurveTo appends a cubic Bezier curve from the current point through
// control points (x1, y1) and (x2, y2) to endpoint (x3, y3) (c operator).
func (ab *appearanceBuilder) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	ab.buf.WriteString(formatFloat(x1))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y1))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(x2))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y2))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(x3))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y3))
	ab.buf.WriteString(" c\n")
}

// Rect adds a closed rectangular subpath (re operator).
func (ab *appearanceBuilder) Rect(x, y, w, h float64) {
	ab.buf.WriteString(formatFloat(x))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(y))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(w))
	ab.buf.WriteByte(' ')
	ab.buf.WriteString(formatFloat(h))
	ab.buf.WriteString(" re\n")
}

// ClosePath closes the current subpath (h operator).
func (ab *appearanceBuilder) ClosePath() {
	ab.buf.WriteString("h\n")
}

// kappa is the standard control-point distance ratio for approximating
// a quarter-circle with a cubic Bezier. (4/3) * (sqrt(2) - 1).
const kappa = 0.5522847498307933

// Ellipse adds a closed elliptic subpath centered at (cx, cy) with
// semi-axes rx and ry, approximated by 4 cubic Beziers and an explicit
// h operator so the subpath is marked closed (matters for stroke joins
// at the seam point per ISO 32000-1 §8.5.2.1).
func (ab *appearanceBuilder) Ellipse(cx, cy, rx, ry float64) {
	dx := rx * kappa
	dy := ry * kappa
	// Start at right edge, going counter-clockwise.
	ab.MoveTo(cx+rx, cy)
	ab.CurveTo(cx+rx, cy+dy, cx+dx, cy+ry, cx, cy+ry)     // right → top
	ab.CurveTo(cx-dx, cy+ry, cx-rx, cy+dy, cx-rx, cy)     // top → left
	ab.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)     // left → bottom
	ab.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)     // bottom → right
	ab.ClosePath()
}
