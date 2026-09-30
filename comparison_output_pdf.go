// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
)

// PDFDiffReportOptions configures PDFDiffOutputGenerator. The zero value
// resolves to A4, the default DiffOutputStyle colors, and the title
// "Comparison Report". Mirrors the constructor overloads of Aspose.PDF for
// .NET's PdfOutputGenerator (OutputTextStyle, PageInfo), folded into one
// options struct since Go has no overloading.
type PDFDiffReportOptions struct {
	DiffOutputStyle
	Format PageFormat // zero → A4
	Title  string     // zero → "Comparison Report"
}

func (o PDFDiffReportOptions) resolved() PDFDiffReportOptions {
	o.DiffOutputStyle = o.DiffOutputStyle.resolved()
	if o.Format.Width <= 0 || o.Format.Height <= 0 {
		o.Format = PageFormatA4
	}
	if o.Title == "" {
		o.Title = "Comparison Report"
	}
	return o
}

func lastPDFDiffReportOptions(opts []PDFDiffReportOptions) PDFDiffReportOptions {
	if len(opts) == 0 {
		return PDFDiffReportOptions{}.resolved()
	}
	return opts[len(opts)-1].resolved()
}

// PDFDiffOutputGenerator renders comparison differences as a generated PDF
// report — colored, word-wrapped, paginated text — built through this
// library's Flow layer rather than Aspose's own (presumably fixed-layout)
// renderer. Mirrors Aspose.PDF for .NET's PdfOutputGenerator. Unlike
// HTML/JSON/Markdown, a PDF cannot be produced as a string, so this type only
// implements FileDiffOutputGenerator, matching Aspose (PdfOutputGenerator
// does not implement IStringOutputGenerator either).
type PDFDiffOutputGenerator struct {
	opts PDFDiffReportOptions
}

// NewPDFDiffOutputGenerator creates a generator with the given options (last
// one wins); the zero value uses PDFDiffReportOptions's defaults.
func NewPDFDiffOutputGenerator(opts ...PDFDiffReportOptions) *PDFDiffOutputGenerator {
	return &PDFDiffOutputGenerator{opts: lastPDFDiffReportOptions(opts)}
}

var _ FileDiffOutputGenerator = (*PDFDiffOutputGenerator)(nil)

// diffRuns turns ops into styled text runs for the Flow runs layout
// (flow_runs.go), one run per operation, colored by style. A leading space is
// prepended to every run after the first so words at an operation boundary
// don't fuse into one unbreakable cluster — flowRunsIndent only inserts a
// break between tokens it sees space between.
func diffRuns(ops []DiffOperation, style DiffOutputStyle) []textRun {
	var runs []textRun
	for _, op := range ops {
		if op.Text == "" {
			continue
		}
		text := op.Text
		if len(runs) > 0 {
			text = " " + text
		}
		ts := TextStyle{}
		ts.Color, ts.Background = style.colorFor(op.Operation)
		if op.Operation == OperationDelete && style.StrikethroughDelete {
			ts.Strikethrough = true
		}
		runs = append(runs, textRun{text: text, style: ts})
	}
	return runs
}

func (g *PDFDiffOutputGenerator) newFlow(doc *Document) *Flow {
	flow := doc.NewFlow(FlowOptions{Format: g.opts.Format})
	flow.AddHeading(1, g.opts.Title, TextStyle{})
	return flow
}

func (g *PDFDiffOutputGenerator) buildSingle(ops []DiffOperation) (*Document, error) {
	doc := NewDocumentFromFormat(g.opts.Format)
	flow := g.newFlow(doc)
	flow.addRuns(diffRuns(ops, g.opts.DiffOutputStyle), StructP)
	if _, err := flow.Render(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (g *PDFDiffOutputGenerator) buildPages(pages [][]DiffOperation) (*Document, error) {
	doc := NewDocumentFromFormat(g.opts.Format)
	flow := g.newFlow(doc)
	for i, ops := range pages {
		flow.AddHeading(2, fmt.Sprintf("Page %d", i+1), TextStyle{})
		flow.addRuns(diffRuns(ops, g.opts.DiffOutputStyle), StructP)
	}
	if _, err := flow.Render(); err != nil {
		return nil, err
	}
	return doc, nil
}

// WriteOutput writes a single-section report to w.
func (g *PDFDiffOutputGenerator) WriteOutput(w io.Writer, ops []DiffOperation) error {
	doc, err := g.buildSingle(ops)
	if err != nil {
		return err
	}
	_, err = doc.WriteTo(w)
	return err
}

// WriteOutputPages writes a report with one heading section per page to w.
func (g *PDFDiffOutputGenerator) WriteOutputPages(w io.Writer, pages [][]DiffOperation) error {
	doc, err := g.buildPages(pages)
	if err != nil {
		return err
	}
	_, err = doc.WriteTo(w)
	return err
}

// SaveOutput writes a single-section report to path.
func (g *PDFDiffOutputGenerator) SaveOutput(path string, ops []DiffOperation) error {
	doc, err := g.buildSingle(ops)
	if err != nil {
		return err
	}
	return doc.Save(path)
}

// SaveOutputPages writes a report with one heading section per page to path.
func (g *PDFDiffOutputGenerator) SaveOutputPages(path string, pages [][]DiffOperation) error {
	doc, err := g.buildPages(pages)
	if err != nil {
		return err
	}
	return doc.Save(path)
}
