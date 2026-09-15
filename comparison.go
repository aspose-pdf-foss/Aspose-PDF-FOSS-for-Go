// SPDX-License-Identifier: MIT

// Document comparison. Mirrors the text half of Aspose.PDF for .NET's
// Aspose.Pdf.Comparison namespace (TextPdfComparer, DiffOperation,
// ComparisonOptions), extended with the location of every difference:
// Aspose's DiffOperation carries only an operation and its text, while these
// operations also carry the pages and rectangles the words occupy on both
// sides — which is what lets the result be drawn back onto the original.
package asposepdf

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
