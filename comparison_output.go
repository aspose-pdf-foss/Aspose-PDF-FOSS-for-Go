// SPDX-License-Identifier: MIT

package asposepdf

import "io"

// Diff output generators (phase 3 of epic pdf-go-175w): serializers over the
// phase-1 DiffOperation model. Mirrors Aspose.PDF for .NET's
// Aspose.Pdf.Comparison.OutputGenerator namespace (IStringOutputGenerator,
// IFileOutputGenerator, HtmlDiffOutputGenerator, JsonDiffOutputGenerator,
// MarkdownDiffOutputGenerator, PdfOutputGenerator).
//
// Aspose overloads GenerateOutput on the argument shape — a single
// List<DiffOperation> (the ComparePages result) versus a
// List<List<DiffOperation>> (what CompareDocumentsPageByPage /
// CompareFlatDocuments return directly) that renders one section per page. Go
// has no overloading, so the two shapes get distinct method names instead:
// GenerateOutput/SaveOutput/WriteOutput for the flat case, and the Pages
// variants for the per-page case.

// StringDiffOutputGenerator produces comparison output as a string. Mirrors
// Aspose.PDF for .NET's IStringOutputGenerator.
type StringDiffOutputGenerator interface {
	// GenerateOutput renders a single, unlabeled list of operations — the
	// shape ComparePages returns.
	GenerateOutput(ops []DiffOperation) (string, error)
	// GenerateOutputPages renders one section per page — the shape
	// (*ComparisonResult).Pages returns.
	GenerateOutputPages(pages [][]DiffOperation) (string, error)
}

// FileDiffOutputGenerator writes comparison output to a file or writer.
// Mirrors Aspose.PDF for .NET's IFileOutputGenerator; the io.Writer variants
// are this library's addition, following the SaveXxx/WriteXxx convention used
// throughout the package.
type FileDiffOutputGenerator interface {
	SaveOutput(path string, ops []DiffOperation) error
	SaveOutputPages(path string, pages [][]DiffOperation) error
	WriteOutput(w io.Writer, ops []DiffOperation) error
	WriteOutputPages(w io.Writer, pages [][]DiffOperation) error
}

// DiffOutputStyle colors the three kinds of text a diff output generator
// marks. The zero value resolves to the same green/red pair
// DiffMarkupOptions defaults to, keeping every comparison output visually
// consistent. Mirrors the intent of Aspose.PDF for .NET's OutputTextStyle
// (its per-kind TextStyle{Color, BackgroundColor} pairs are flattened here,
// since TextStyle already names an unrelated type in this library).
type DiffOutputStyle struct {
	InsertColor, InsertBackground *Color
	DeleteColor, DeleteBackground *Color
	EqualColor, EqualBackground   *Color
	// StrikethroughDelete draws deleted text with a line through it. Mirrors
	// OutputTextStyle.StrikethroughDeleted; default false. Markdown has no
	// equivalent knob — a deletion is always struck through there, since GFM
	// strikethrough is the only way to represent one at all.
	StrikethroughDelete bool
}

// resolved fills in the default insert/delete colors.
func (o DiffOutputStyle) resolved() DiffOutputStyle {
	if o.InsertColor == nil {
		o.InsertColor = &Color{R: 0.20, G: 0.72, B: 0.35, A: 1}
	}
	if o.DeleteColor == nil {
		o.DeleteColor = &Color{R: 0.88, G: 0.22, B: 0.22, A: 1}
	}
	return o
}

func lastDiffOutputStyle(opts []DiffOutputStyle) DiffOutputStyle {
	if len(opts) == 0 {
		return DiffOutputStyle{}.resolved()
	}
	return opts[len(opts)-1].resolved()
}

// colorFor returns the style's color/background pair for op's kind.
func (o DiffOutputStyle) colorFor(op Operation) (*Color, *Color) {
	switch op {
	case OperationInsert:
		return o.InsertColor, o.InsertBackground
	case OperationDelete:
		return o.DeleteColor, o.DeleteBackground
	default:
		return o.EqualColor, o.EqualBackground
	}
}

// Pages returns the operations grouped one slice per page, the shape every
// diff output generator's *Pages method expects — the same shape
// CompareDocumentsPageByPage/CompareFlatDocuments return directly in
// Aspose.PDF for .NET. In page-by-page mode this is the result's own per-page
// storage; in flat mode it is built from PageOperations, since flat mode
// keeps only the flat op list.
func (r *ComparisonResult) Pages() [][]DiffOperation {
	if r.pages != nil {
		return r.pages
	}
	max := 0
	for _, op := range r.ops {
		if p := operationPage(op); p > max {
			max = p
		}
	}
	pages := make([][]DiffOperation, max)
	for i := range pages {
		pages[i] = r.PageOperations(i + 1)
	}
	return pages
}
