package asposepdf

import (
	"bytes"
	"strconv"
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
func (b *appearanceBuilder) Bytes() []byte {
	return b.buf.Bytes()
}

// formatFloat formats f without scientific notation and without trailing
// zeros. Matches the convention used elsewhere in the project.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// PushState saves the current graphics state (q operator).
func (b *appearanceBuilder) PushState() {
	b.buf.WriteString("q\n")
}

// PopState restores the last saved graphics state (Q operator).
func (b *appearanceBuilder) PopState() {
	b.buf.WriteString("Q\n")
}

// ConcatMatrix concatenates the given 2x3 matrix to the CTM (cm operator).
func (b *appearanceBuilder) ConcatMatrix(a, bb, c, d, e, f float64) {
	b.buf.WriteString(formatFloat(a))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(bb))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(d))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(e))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(f))
	b.buf.WriteString(" cm\n")
}

// SetLineWidth sets the stroke line width (w operator).
func (b *appearanceBuilder) SetLineWidth(w float64) {
	b.buf.WriteString(formatFloat(w))
	b.buf.WriteString(" w\n")
}

// SetLineCap sets the line-cap style (J operator).
func (b *appearanceBuilder) SetLineCap(c LineCap) {
	b.buf.WriteString(strconv.Itoa(int(c)))
	b.buf.WriteString(" J\n")
}

// SetLineJoin sets the line-join style (j operator).
func (b *appearanceBuilder) SetLineJoin(j LineJoin) {
	b.buf.WriteString(strconv.Itoa(int(j)))
	b.buf.WriteString(" j\n")
}

// SetMiterLimit sets the miter limit (M operator).
func (b *appearanceBuilder) SetMiterLimit(m float64) {
	b.buf.WriteString(formatFloat(m))
	b.buf.WriteString(" M\n")
}

// SetDashPattern sets the line-dash pattern (d operator). A nil or empty
// pattern emits "[] phase d", which means a solid line.
func (b *appearanceBuilder) SetDashPattern(pattern []float64, phase float64) {
	b.buf.WriteByte('[')
	for i, v := range pattern {
		if i > 0 {
			b.buf.WriteByte(' ')
		}
		b.buf.WriteString(formatFloat(v))
	}
	b.buf.WriteByte(']')
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(phase))
	b.buf.WriteString(" d\n")
}
