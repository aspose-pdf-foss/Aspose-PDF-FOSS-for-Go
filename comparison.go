// SPDX-License-Identifier: MIT

// Document comparison. Mirrors the text half of Aspose.PDF for .NET's
// Aspose.Pdf.Comparison namespace (TextPdfComparer, DiffOperation,
// ComparisonOptions), extended with the location of every difference:
// Aspose's DiffOperation carries only an operation and its text, while these
// operations also carry the pages and rectangles the words occupy on both
// sides — which is what lets the result be drawn back onto the original.
package asposepdf

import (
	"errors"
	"strings"
)

// Operation is the kind of a difference. Mirrors Aspose.PDF for .NET's
// Aspose.Pdf.Comparison.Operation.
type Operation int

const (
	// OperationEqual marks text present in both documents.
	OperationEqual Operation = iota
	// OperationInsert marks text present only in the second document.
	OperationInsert
	// OperationDelete marks text present only in the first document.
	OperationDelete
)

// String returns "equal", "insert" or "delete".
func (o Operation) String() string {
	switch o {
	case OperationInsert:
		return "insert"
	case OperationDelete:
		return "delete"
	default:
		return "equal"
	}
}

// DiffOperation is one run of adjacent words sharing an operation. Runs break
// at line and page boundaries, so every rectangle is a real box on a real
// page: a run spanning three lines carries three rectangles.
//
// Operation and Text mirror Aspose.PDF for .NET's DiffOperation; the page and
// rectangle fields are this library's addition.
type DiffOperation struct {
	Operation Operation
	// Text is the run's words joined with single spaces. For OperationEqual
	// it is the first document's spelling, which can differ from the second's
	// when ComparisonOptions.IgnoreCase is set.
	Text string
	// SourcePage is the 1-based page in the first document, 0 for an insertion.
	SourcePage int
	// DestPage is the 1-based page in the second document, 0 for a deletion.
	DestPage int
	// SourceRects holds one rectangle per line the run touches in the first
	// document; empty for an insertion.
	SourceRects []Rectangle
	// DestRects holds one rectangle per line the run touches in the second
	// document; empty for a deletion.
	DestRects []Rectangle
}

// EditOperationsOrder decides how the two halves of a replacement are
// ordered. Mirrors Aspose.PDF for .NET's EditOperationsOrder.
type EditOperationsOrder int

const (
	// EditOperationsDeleteFirst reports the removed text before the added
	// text. This is the zero value.
	EditOperationsDeleteFirst EditOperationsOrder = iota
	// EditOperationsInsertFirst reports the added text first.
	EditOperationsInsertFirst
)

// ComparisonOptions narrows what is compared. The zero value compares the
// whole page, case-sensitively, reporting deletions before insertions.
// Mirrors Aspose.PDF for .NET's ComparisonOptions, plus IgnoreCase.
type ComparisonOptions struct {
	// ExtractionArea limits the comparison to the words whose centre lies in
	// this rectangle. Cannot be combined with ExcludeTables or ExcludeAreas.
	ExtractionArea *Rectangle
	// ExcludeAreas1 lists regions of the first document to ignore.
	ExcludeAreas1 []Rectangle
	// ExcludeAreas2 lists regions of the second document to ignore.
	ExcludeAreas2 []Rectangle
	// ExcludeTables drops the words inside tables found by the TableAbsorber.
	ExcludeTables bool
	// EditOperationsOrder decides which half of a replacement is reported first.
	EditOperationsOrder EditOperationsOrder
	// IgnoreCase compares words without regard to letter case. This library's
	// addition; Aspose has no equivalent.
	IgnoreCase bool
}

// validate rejects the option combinations Aspose documents as incompatible.
func (o ComparisonOptions) validate() error {
	if o.ExtractionArea == nil {
		return nil
	}
	if o.ExcludeTables || len(o.ExcludeAreas1) > 0 || len(o.ExcludeAreas2) > 0 {
		return errors.New("ComparisonOptions: ExtractionArea cannot be combined with ExcludeTables or ExcludeAreas")
	}
	return nil
}

// lastComparisonOption returns the effective options, last one winning.
func lastComparisonOption(opts []ComparisonOptions) ComparisonOptions {
	if len(opts) == 0 {
		return ComparisonOptions{}
	}
	return opts[len(opts)-1]
}

// ComparePages compares the text of two pages and returns the differences in
// reading order. Mirrors Aspose.PDF for .NET's TextPdfComparer.ComparePages.
func ComparePages(p1, p2 *Page, opts ...ComparisonOptions) ([]DiffOperation, error) {
	if p1 == nil || p2 == nil {
		return nil, errors.New("ComparePages: nil page")
	}
	o := lastComparisonOption(opts)
	if err := o.validate(); err != nil {
		return nil, err
	}
	src, err := comparePageTokens(p1, o, false)
	if err != nil {
		return nil, err
	}
	dst, err := comparePageTokens(p2, o, true)
	if err != nil {
		return nil, err
	}
	return diffTokens(src, dst, o.EditOperationsOrder), nil
}

// comparePageTokens extracts one page's words and applies the option filters.
// second selects which ExcludeAreas list applies.
func comparePageTokens(p *Page, o ComparisonOptions, second bool) ([]wordToken, error) {
	tokens, err := pageWordTokens(p, o.IgnoreCase)
	if err != nil {
		return nil, err
	}
	exclude := o.ExcludeAreas1
	if second {
		exclude = o.ExcludeAreas2
	}
	if o.ExcludeTables {
		rects, err := tableRects(p)
		if err != nil {
			return nil, err
		}
		exclude = append(append([]Rectangle(nil), exclude...), rects...)
	}
	return filterTokens(tokens, o.ExtractionArea, exclude), nil
}

// diffTokens runs the diff and groups it, falling back to a wholesale
// replacement when the edit-distance cap is hit.
func diffTokens(src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	keysOf := func(tokens []wordToken) []string {
		keys := make([]string, len(tokens))
		for i, tk := range tokens {
			keys[i] = tk.key
		}
		return keys
	}
	edits, ok := diffKeys(keysOf(src), keysOf(dst))
	if !ok {
		return wholeReplacement(src, dst, order)
	}
	return groupOperations(edits, src, dst, order)
}

// AssembleSourceText rebuilds the first document's text from the operations —
// everything equal plus everything deleted. Mirrors Aspose.PDF for .NET's
// TextPdfComparer.AssemblySourcePageText.
func AssembleSourceText(ops []DiffOperation) string {
	return assembleText(ops, OperationDelete)
}

// AssembleDestinationText rebuilds the second document's text — everything
// equal plus everything inserted. Mirrors Aspose.PDF for .NET's
// TextPdfComparer.AssemblyDestinationPageText. With
// ComparisonOptions.IgnoreCase set, equal runs carry the first document's
// spelling, so the result can differ from the second document in letter case.
func AssembleDestinationText(ops []DiffOperation) string {
	return assembleText(ops, OperationInsert)
}

func assembleText(ops []DiffOperation, side Operation) string {
	var parts []string
	for _, op := range ops {
		if op.Operation == OperationEqual || op.Operation == side {
			if op.Text != "" {
				parts = append(parts, op.Text)
			}
		}
	}
	return strings.Join(parts, " ")
}
