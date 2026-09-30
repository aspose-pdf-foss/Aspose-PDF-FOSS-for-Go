// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"html"
	"io"
	"strings"
)

// HTMLDiffOutputGenerator renders comparison differences as an HTML fragment
// — colored spans for insertions/deletions, meant to be embedded into the
// caller's own page or email, not a standalone document. Mirrors Aspose.PDF
// for .NET's HtmlDiffOutputGenerator.
type HTMLDiffOutputGenerator struct {
	style DiffOutputStyle
}

// NewHTMLDiffOutputGenerator creates a generator with the given style (last
// one wins); the zero value uses DiffOutputStyle's defaults.
func NewHTMLDiffOutputGenerator(opts ...DiffOutputStyle) *HTMLDiffOutputGenerator {
	return &HTMLDiffOutputGenerator{style: lastDiffOutputStyle(opts)}
}

var _ StringDiffOutputGenerator = (*HTMLDiffOutputGenerator)(nil)
var _ FileDiffOutputGenerator = (*HTMLDiffOutputGenerator)(nil)

// GenerateOutput renders ops as one HTML fragment, operations joined by a
// single space (matching AssembleSourceText/AssembleDestinationText). Equal
// text is emitted plain unless DiffOutputStyle.EqualColor/EqualBackground is
// set.
func (g *HTMLDiffOutputGenerator) GenerateOutput(ops []DiffOperation) (string, error) {
	var b strings.Builder
	writeHTMLOps(&b, ops, g.style)
	return b.String(), nil
}

// GenerateOutputPages renders one <div class="cmp-page"> section per page,
// each headed by "Page N".
func (g *HTMLDiffOutputGenerator) GenerateOutputPages(pages [][]DiffOperation) (string, error) {
	var b strings.Builder
	for i, ops := range pages {
		fmt.Fprintf(&b, `<div class="cmp-page" data-page="%d"><h3>Page %d</h3>`, i+1, i+1)
		writeHTMLOps(&b, ops, g.style)
		b.WriteString("</div>")
	}
	return b.String(), nil
}

func writeHTMLOps(b *strings.Builder, ops []DiffOperation, style DiffOutputStyle) {
	first := true
	for _, op := range ops {
		if op.Text == "" {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		writeHTMLOp(b, op, style)
	}
}

func writeHTMLOp(b *strings.Builder, op DiffOperation, style DiffOutputStyle) {
	text := html.EscapeString(op.Text)
	color, bg := style.colorFor(op.Operation)
	var css strings.Builder
	if color != nil {
		fmt.Fprintf(&css, "color:%s;", cssColor(*color))
	}
	if bg != nil {
		fmt.Fprintf(&css, "background-color:%s;", cssColor(*bg))
	}
	if op.Operation == OperationDelete && style.StrikethroughDelete {
		css.WriteString("text-decoration:line-through;")
	}
	if css.Len() == 0 {
		b.WriteString(text)
		return
	}
	fmt.Fprintf(b, `<span style="%s">%s</span>`, css.String(), text)
}

// cssColor formats a Color as a CSS rgb()/rgba() function.
func cssColor(c Color) string {
	r, g, bl := colorByte(c.R), colorByte(c.G), colorByte(c.B)
	if c.A >= 1 || c.A == 0 {
		return fmt.Sprintf("rgb(%d,%d,%d)", r, g, bl)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.3g)", r, g, bl, c.A)
}

func colorByte(v float64) int {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return int(v*255 + 0.5)
}

// WriteOutput writes GenerateOutput's fragment to w.
func (g *HTMLDiffOutputGenerator) WriteOutput(w io.Writer, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// WriteOutputPages writes GenerateOutputPages's fragment to w.
func (g *HTMLDiffOutputGenerator) WriteOutputPages(w io.Writer, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// SaveOutput writes GenerateOutput's fragment to path.
func (g *HTMLDiffOutputGenerator) SaveOutput(path string, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}

// SaveOutputPages writes GenerateOutputPages's fragment to path.
func (g *HTMLDiffOutputGenerator) SaveOutputPages(path string, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}
