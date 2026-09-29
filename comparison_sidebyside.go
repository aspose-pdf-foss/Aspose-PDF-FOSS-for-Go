// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
)

// Side-by-side document comparison — phase 2 of the comparison epic
// (pdf-go-175w / pdf-go-dyu9). Mirrors Aspose.PDF for .NET's
// SideBySidePdfComparer: a two-column "before / after" spread with the
// differences highlighted. Built entirely on phase-1 machinery
// (comparison.go's word diff, comparison_markup.go's WriteMarkup) plus
// PdfPageStamp (stamp_pdfpage.go) to place each side — so, unlike Aspose's
// rasterized side-by-side output, the spread stays vector: each pane is the
// real page content, not a picture of it.

// SideBySideComparisonOptions configures a side-by-side spread. Embeds
// ComparisonOptions — phase 1's word-diff options (ExtractionArea,
// ExcludeAreas1/2, ExcludeTables, EditOperationsOrder, IgnoreCase) apply here
// too, since the spread is built on the same word-diff engine. Mirrors
// Aspose.PDF for .NET's SideBySideComparisonOptions, minus ComparisonMode
// (this library's tokenizer already splits words on whitespace, so behaviour
// already matches Aspose's IgnoreSpaces mode; the Normal/ParseSpaces
// whitespace-sensitivity distinction is out of scope for v1) and
// AdditionalChangeMarks (margin change bars — out of scope for v1).
type SideBySideComparisonOptions struct {
	ComparisonOptions
	// InsertColor / DeleteColor style the markup on each pane, same meaning
	// as DiffMarkupOptions. Defaults: green insertions, red deletions.
	InsertColor *Color
	DeleteColor *Color
}

func lastSideBySideOption(opts []SideBySideComparisonOptions) SideBySideComparisonOptions {
	if len(opts) == 0 {
		return SideBySideComparisonOptions{}
	}
	return opts[len(opts)-1]
}

func (o SideBySideComparisonOptions) markupOptions(side DiffMarkupSide) DiffMarkupOptions {
	// Flatten: true is required, not cosmetic — PdfPageStamp places each pane
	// via importPageAsXObject, which captures a page's content stream and
	// /Resources only, never /Annots (imposition.go). Without flattening, the
	// highlight/strikeout/caret markup WriteMarkup adds as real annotations
	// would be silently dropped the moment the marked-up page is stamped as a
	// Form XObject onto the spread.
	return DiffMarkupOptions{Side: side, InsertColor: o.InsertColor, DeleteColor: o.DeleteColor, Title: "Comparison", Flatten: true}
}

// SideBySidePagesComparisonResult reports a page-level side-by-side
// comparison. Mirrors Aspose.PDF for .NET's SideBySidePagesComparisonResult.
type SideBySidePagesComparisonResult struct {
	ops []DiffOperation
}

// HasChanges reports whether the two pages differ.
func (r *SideBySidePagesComparisonResult) HasChanges() bool {
	for _, op := range r.ops {
		if op.Operation != OperationEqual {
			return true
		}
	}
	return false
}

// Operations returns the underlying word-level differences (the same model
// ComparePages returns).
func (r *SideBySidePagesComparisonResult) Operations() []DiffOperation { return r.ops }

// SideBySideDocsComparisonResult reports a document-level side-by-side
// comparison. Mirrors Aspose.PDF for .NET's SideBySideDocsComparisonResult;
// embeds *ComparisonResult, so HasChanges/Operations/PageOperations/
// Statistics all apply directly to the spread's underlying comparison.
type SideBySideDocsComparisonResult struct {
	*ComparisonResult
}

// SideBySideComparePages lays page1 and page2 side by side on one output
// page — page1 (deletions struck through) on the left, page2 (insertions
// highlighted) on the right — and saves it as a new PDF at outputPath.
// Mirrors Aspose.PDF for .NET's
// SideBySidePdfComparer.Compare(Page, Page, string, SideBySideComparisonOptions).
func SideBySideComparePages(page1, page2 *Page, outputPath string, opts ...SideBySideComparisonOptions) (*SideBySidePagesComparisonResult, error) {
	var buf bytes.Buffer
	res, err := WriteSideBySidePages(page1, page2, &buf, opts...)
	if err != nil {
		return nil, err
	}
	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return nil, err
	}
	return res, nil
}

// WriteSideBySidePages is the io.Writer variant of SideBySideComparePages.
func WriteSideBySidePages(page1, page2 *Page, w io.Writer, opts ...SideBySideComparisonOptions) (*SideBySidePagesComparisonResult, error) {
	if page1 == nil || page2 == nil {
		return nil, errors.New("WriteSideBySidePages: nil page")
	}
	o := lastSideBySideOption(opts)

	sub1, err := page1.doc.Extract(PageRange{From: page1.Number(), To: page1.Number()})
	if err != nil {
		return nil, fmt.Errorf("WriteSideBySidePages: extract page1: %w", err)
	}
	sub2, err := page2.doc.Extract(PageRange{From: page2.Number(), To: page2.Number()})
	if err != nil {
		return nil, fmt.Errorf("WriteSideBySidePages: extract page2: %w", err)
	}
	result, err := CompareDocumentsPageByPage(sub1, sub2, o.ComparisonOptions)
	if err != nil {
		return nil, err
	}
	out, err := buildSideBySideSpread(result, o, []int{1})
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteTo(w); err != nil {
		return nil, err
	}
	return &SideBySidePagesComparisonResult{ops: result.ops}, nil
}

// SideBySideCompareDocuments lays every page pair of doc1 and doc2 side by
// side — paired by index like CompareDocumentsPageByPage, so the longer
// document's tail pages get an empty pane on the missing side — one output
// page per pair, and saves the spread as a new PDF at outputPath. Mirrors
// Aspose.PDF for .NET's
// SideBySidePdfComparer.Compare(Document, Document, string, SideBySideComparisonOptions).
func SideBySideCompareDocuments(doc1, doc2 *Document, outputPath string, opts ...SideBySideComparisonOptions) (*SideBySideDocsComparisonResult, error) {
	var buf bytes.Buffer
	res, err := WriteSideBySideDocuments(doc1, doc2, &buf, opts...)
	if err != nil {
		return nil, err
	}
	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return nil, err
	}
	return res, nil
}

// WriteSideBySideDocuments is the io.Writer variant of SideBySideCompareDocuments.
func WriteSideBySideDocuments(doc1, doc2 *Document, w io.Writer, opts ...SideBySideComparisonOptions) (*SideBySideDocsComparisonResult, error) {
	if doc1 == nil || doc2 == nil {
		return nil, errors.New("WriteSideBySideDocuments: nil document")
	}
	o := lastSideBySideOption(opts)
	result, err := CompareDocumentsPageByPage(doc1, doc2, o.ComparisonOptions)
	if err != nil {
		return nil, err
	}
	count := doc1.PageCount()
	if doc2.PageCount() > count {
		count = doc2.PageCount()
	}
	if count == 0 {
		return nil, errors.New("WriteSideBySideDocuments: both documents have no pages")
	}
	pages := make([]int, count)
	for i := range pages {
		pages[i] = i + 1
	}
	out, err := buildSideBySideSpread(result, o, pages)
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteTo(w); err != nil {
		return nil, err
	}
	return &SideBySideDocsComparisonResult{ComparisonResult: result}, nil
}

const (
	sbsMargin  = 24.0
	sbsGutter  = 18.0
	sbsHeaderH = 20.0
)

// buildSideBySideSpread marks up both sides of result (source: deletions
// struck through; destination: insertions highlighted — comparison_markup.go),
// reopens each marked-up copy, and for every page number in pageNums stamps
// the source's page (if present) on the left and the destination's (if
// present) on the right of one output page, sized to fit both panes at their
// native size. Each pane is placed with PdfPageStamp — a vector Form XObject
// placement, not a raster — so the spread's content stays real page content.
func buildSideBySideSpread(result *ComparisonResult, o SideBySideComparisonOptions, pageNums []int) (*Document, error) {
	var srcBuf, dstBuf bytes.Buffer
	if err := result.WriteMarkup(&srcBuf, o.markupOptions(DiffMarkupSource)); err != nil {
		return nil, fmt.Errorf("side-by-side: mark up source: %w", err)
	}
	if err := result.WriteMarkup(&dstBuf, o.markupOptions(DiffMarkupDestination)); err != nil {
		return nil, fmt.Errorf("side-by-side: mark up destination: %w", err)
	}
	srcDoc, err := OpenStream(bytes.NewReader(srcBuf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("side-by-side: reopen marked-up source: %w", err)
	}
	dstDoc, err := OpenStream(bytes.NewReader(dstBuf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("side-by-side: reopen marked-up destination: %w", err)
	}

	out := NewDocumentFromFormat(PageFormatA4)
	for i, n := range pageNums {
		left, right, err := spreadPaneSizes(srcDoc, dstDoc, n)
		if err != nil {
			return nil, err
		}
		width := left.Width + right.Width + 2*sbsMargin + sbsGutter
		height := math.Max(left.Height, right.Height) + 2*sbsMargin + sbsHeaderH

		var page *Page
		if i == 0 {
			page, err = out.Page(1)
			if err == nil {
				err = page.SetPageSize(width, height)
			}
		} else {
			if err = out.AddBlankPage(width, height); err == nil {
				page, err = out.Page(out.PageCount())
			}
		}
		if err != nil {
			return nil, err
		}

		leftRect := Rectangle{LLX: sbsMargin, LLY: sbsMargin, URX: sbsMargin + left.Width, URY: sbsMargin + left.Height}
		rightRect := Rectangle{
			LLX: leftRect.URX + sbsGutter, LLY: sbsMargin,
			URX: leftRect.URX + sbsGutter + right.Width, URY: sbsMargin + right.Height,
		}

		if n <= srcDoc.PageCount() {
			if err := stampSideBySidePane(page, srcDoc, n, leftRect); err != nil {
				return nil, err
			}
		}
		if n <= dstDoc.PageCount() {
			if err := stampSideBySidePane(page, dstDoc, n, rightRect); err != nil {
				return nil, err
			}
		}
		if err := drawSideBySideHeader(page, leftRect, rightRect, height); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// spreadPaneSizes returns the page size of pageNum in each document; a
// document missing that page reuses the other side's size, so both panes are
// still drawn at a sensible, symmetric scale.
func spreadPaneSizes(srcDoc, dstDoc *Document, pageNum int) (left, right PageSize, err error) {
	if pageNum <= srcDoc.PageCount() {
		p, e := srcDoc.Page(pageNum)
		if e != nil {
			return PageSize{}, PageSize{}, e
		}
		if left, err = p.Size(); err != nil {
			return PageSize{}, PageSize{}, err
		}
	}
	if pageNum <= dstDoc.PageCount() {
		p, e := dstDoc.Page(pageNum)
		if e != nil {
			return PageSize{}, PageSize{}, e
		}
		if right, err = p.Size(); err != nil {
			return PageSize{}, PageSize{}, err
		}
	}
	if left == (PageSize{}) {
		left = right
	}
	if right == (PageSize{}) {
		right = left
	}
	if left == (PageSize{}) {
		left = PageSize{Width: PageFormatA4.Width, Height: PageFormatA4.Height}
		right = left
	}
	return left, right, nil
}

func stampSideBySidePane(page *Page, srcDoc *Document, pageNum int, rect Rectangle) error {
	stamp, err := NewPdfPageStamp(srcDoc, pageNum)
	if err != nil {
		return err
	}
	stamp.Rect = rect
	return page.AddStamp(stamp)
}

func drawSideBySideHeader(page *Page, leftRect, rightRect Rectangle, pageHeight float64) error {
	grey := &Color{R: 0.45, G: 0.45, B: 0.5, A: 1}
	style := TextStyle{Font: FontHelveticaBold, Size: 10, Color: grey, HAlign: HAlignCenter}
	top := pageHeight - sbsMargin
	if err := page.AddText("Before", style, Rectangle{LLX: leftRect.LLX, LLY: top - 14, URX: leftRect.URX, URY: top}); err != nil {
		return err
	}
	return page.AddText("After", style, Rectangle{LLX: rightRect.LLX, LLY: top - 14, URX: rightRect.URX, URY: top})
}
