// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"strings"
)

// MarkdownDiffOutputGenerator renders comparison differences as GFM Markdown:
// **inserted** text bold, ~~deleted~~ text struck through. Mirrors Aspose.PDF
// for .NET's MarkdownDiffOutputGenerator. As in Aspose, it takes no style
// options — Markdown's syntax already fixes how insert/delete are marked, and
// it cannot represent a whitespace-only change at all.
type MarkdownDiffOutputGenerator struct{}

// NewMarkdownDiffOutputGenerator creates a generator.
func NewMarkdownDiffOutputGenerator() *MarkdownDiffOutputGenerator {
	return &MarkdownDiffOutputGenerator{}
}

var _ StringDiffOutputGenerator = (*MarkdownDiffOutputGenerator)(nil)
var _ FileDiffOutputGenerator = (*MarkdownDiffOutputGenerator)(nil)

// GenerateOutput renders ops as one Markdown fragment, operations joined by a
// single space (matching AssembleSourceText/AssembleDestinationText).
func (g *MarkdownDiffOutputGenerator) GenerateOutput(ops []DiffOperation) (string, error) {
	var b strings.Builder
	writeMarkdownOps(&b, ops)
	return b.String(), nil
}

// GenerateOutputPages renders one "## Page N" section per page.
func (g *MarkdownDiffOutputGenerator) GenerateOutputPages(pages [][]DiffOperation) (string, error) {
	var b strings.Builder
	for i, ops := range pages {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## Page %d\n\n", i+1)
		writeMarkdownOps(&b, ops)
	}
	return b.String(), nil
}

func writeMarkdownOps(b *strings.Builder, ops []DiffOperation) {
	first := true
	for _, op := range ops {
		if op.Text == "" {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		text := mdEscapeText(op.Text)
		switch op.Operation {
		case OperationInsert:
			fmt.Fprintf(b, "**%s**", text)
		case OperationDelete:
			fmt.Fprintf(b, "~~%s~~", text)
		default:
			b.WriteString(text)
		}
	}
}

// WriteOutput writes GenerateOutput's Markdown to w.
func (g *MarkdownDiffOutputGenerator) WriteOutput(w io.Writer, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// WriteOutputPages writes GenerateOutputPages's Markdown to w.
func (g *MarkdownDiffOutputGenerator) WriteOutputPages(w io.Writer, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// SaveOutput writes GenerateOutput's Markdown to path.
func (g *MarkdownDiffOutputGenerator) SaveOutput(path string, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}

// SaveOutputPages writes GenerateOutputPages's Markdown to path.
func (g *MarkdownDiffOutputGenerator) SaveOutputPages(path string, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}
