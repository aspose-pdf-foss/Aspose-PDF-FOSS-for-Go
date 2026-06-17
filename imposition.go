// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"math"
)

// NUpOrder controls the order in which source pages fill the grid cells of
// an N-up sheet.
type NUpOrder int

const (
	// NUpRowMajor fills left-to-right, then top-to-bottom (the default).
	NUpRowMajor NUpOrder = iota
	// NUpColumnMajor fills top-to-bottom, then left-to-right.
	NUpColumnMajor
)

// NUpOptions configures NUp imposition.
type NUpOptions struct {
	// Rows and Cols define the grid; both must be >= 1.
	Rows, Cols int
	// PageSize is the output sheet size. The zero value uses A4.
	PageSize PageFormat
	// Margin is the blank border around the whole grid, in points.
	Margin float64
	// Gutter is the spacing between adjacent cells, in points.
	Gutter float64
	// Order is the cell-filling order (row-major by default).
	Order NUpOrder
	// DrawBorder draws a thin frame around each placed page.
	DrawBorder bool
}

// BookletBinding selects the binding edge of a booklet.
type BookletBinding int

const (
	// BindingLeft places the lower page number on the right (LTR reading).
	BindingLeft BookletBinding = iota
	// BindingRight mirrors the spreads for right-to-left binding.
	BindingRight
)

// BookletOptions configures Booklet imposition.
type BookletOptions struct {
	// PageSize is the output sheet (a two-page spread). The zero value uses
	// twice the width by the height of the document's first page, so the
	// source pages are placed at 100% with no scaling.
	PageSize PageFormat
	// Binding selects the binding edge (left by default).
	Binding BookletBinding
}

// NUp returns a new Document that imposes this document's pages onto larger
// sheets in a Rows×Cols grid (e.g. 2×2 for 4-up). Each source page is wrapped
// in a Form XObject and scaled to fit its cell, preserving aspect ratio and
// centered. Sheets are filled until every page is placed; the last sheet may
// have empty cells. The receiver is not modified.
//
// This is a page-production helper (not an ISO 32000 feature); the output is
// an ordinary, spec-compliant PDF. It mirrors the intent of Aspose.PDF for
// .NET's PdfFileEditor.MakeNUp, adapted to this library's Document API.
func (d *Document) NUp(opts NUpOptions) (*Document, error) {
	if opts.Rows < 1 || opts.Cols < 1 {
		return nil, fmt.Errorf("NUp: Rows and Cols must be >= 1")
	}
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("NUp: document has no pages")
	}
	sheet := opts.PageSize
	if sheet == (PageFormat{}) {
		sheet = PageFormatA4
	}

	gridW := sheet.Width - 2*opts.Margin
	gridH := sheet.Height - 2*opts.Margin
	cellW := (gridW - opts.Gutter*float64(opts.Cols-1)) / float64(opts.Cols)
	cellH := (gridH - opts.Gutter*float64(opts.Rows-1)) / float64(opts.Rows)
	if cellW <= 0 || cellH <= 0 {
		return nil, fmt.Errorf("NUp: margins/gutter too large for the sheet")
	}
	perSheet := opts.Rows * opts.Cols

	result := &Document{objects: map[int]*pdfObject{}, nextID: 1}
	idMap := map[int]int{}
	xobjIDs, boxes, err := result.importAllPages(d, idMap)
	if err != nil {
		return nil, err
	}

	n := len(d.pages)
	for start := 0; start < n; start += perSheet {
		var placements []placement
		for cell := 0; cell < perSheet && start+cell < n; cell++ {
			src := start + cell
			var row, col int
			if opts.Order == NUpColumnMajor {
				col, row = cell/opts.Rows, cell%opts.Rows
			} else {
				row, col = cell/opts.Cols, cell%opts.Cols
			}
			cellLLX := opts.Margin + float64(col)*(cellW+opts.Gutter)
			cellURY := sheet.Height - opts.Margin - float64(row)*(cellH+opts.Gutter)
			rect := Rectangle{LLX: cellLLX, LLY: cellURY - cellH, URX: cellLLX + cellW, URY: cellURY}
			placements = append(placements, makePlacement(xobjIDs[src], boxes[src], rect, opts.DrawBorder))
		}
		result.addImposedSheet(sheet.Width, sheet.Height, placements)
	}
	return result, nil
}

// Booklet returns a new Document that imposes this document's pages two-up,
// reordered for saddle-stitch binding: print double-sided, fold the stack in
// half, and the pages read in order. The page count is padded up to a multiple
// of four with blank halves. The receiver is not modified.
//
// Like NUp, this is a production helper (not an ISO 32000 feature) producing an
// ordinary PDF; it mirrors the intent of Aspose.PDF for .NET's
// PdfFileEditor.MakeBooklet, adapted to this library's Document API.
func (d *Document) Booklet(opts BookletOptions) (*Document, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("Booklet: document has no pages")
	}
	n := len(d.pages)
	padded := ((n + 3) / 4) * 4 // round up to a multiple of 4

	sheetW, sheetH := 0.0, 0.0
	if opts.PageSize != (PageFormat{}) {
		sheetW, sheetH = opts.PageSize.Width, opts.PageSize.Height
	} else {
		first, err := d.Page(1)
		if err != nil {
			return nil, err
		}
		box, err := first.MediaBox()
		if err != nil {
			return nil, err
		}
		sheetW, sheetH = 2*(box.URX-box.LLX), box.URY-box.LLY
	}

	result := &Document{objects: map[int]*pdfObject{}, nextID: 1}
	idMap := map[int]int{}
	xobjIDs, boxes, err := result.importAllPages(d, idMap)
	if err != nil {
		return nil, err
	}

	half := sheetW / 2
	leftCell := Rectangle{LLX: 0, LLY: 0, URX: half, URY: sheetH}
	rightCell := Rectangle{LLX: half, LLY: 0, URX: sheetW, URY: sheetH}

	place := func(ps []placement, pageNum int, cell Rectangle) []placement {
		if pageNum >= 1 && pageNum <= n { // pages beyond n are blank padding
			ps = append(ps, makePlacement(xobjIDs[pageNum-1], boxes[pageNum-1], cell, false))
		}
		return ps
	}

	for i := 0; i < padded/2; i++ {
		// Nested saddle-stitch order: (high,low) on even spreads, (low,high) on odd.
		var left, right int
		if i%2 == 0 {
			left, right = padded-i, i+1
		} else {
			left, right = i+1, padded-i
		}
		if opts.Binding == BindingRight {
			left, right = right, left
		}
		var ps []placement
		ps = place(ps, left, leftCell)
		ps = place(ps, right, rightCell)
		result.addImposedSheet(sheetW, sheetH, ps)
	}
	return result, nil
}

// placement records where one Form XObject is drawn on a sheet: a uniform
// scale, a translation, and (optionally) a border rectangle around the drawn
// page in sheet coordinates.
type placement struct {
	xobjID         int
	scale, tx, ty  float64
	border         bool
	bx, by, bw, bh float64
}

// makePlacement computes the transform that fits a page's BBox into cell,
// preserving aspect ratio and centering it.
func makePlacement(xobjID int, box, cell Rectangle, border bool) placement {
	bw, bh := box.URX-box.LLX, box.URY-box.LLY
	cw, ch := cell.URX-cell.LLX, cell.URY-cell.LLY
	scale := 1.0
	if bw > 0 && bh > 0 {
		scale = math.Min(cw/bw, ch/bh)
	}
	drawnW, drawnH := bw*scale, bh*scale
	offX := cell.LLX + (cw-drawnW)/2
	offY := cell.LLY + (ch-drawnH)/2
	return placement{
		xobjID: xobjID,
		scale:  scale,
		tx:     offX - box.LLX*scale,
		ty:     offY - box.LLY*scale,
		border: border,
		bx:     offX, by: offY, bw: drawnW, bh: drawnH,
	}
}

// importAllPages wraps every source page in a Form XObject inside result,
// returning the per-page XObject IDs and their BBox rectangles. Objects shared
// between pages are copied once (tracked through idMap).
func (result *Document) importAllPages(src *Document, idMap map[int]int) ([]int, []Rectangle, error) {
	n := len(src.pages)
	xobjIDs := make([]int, n)
	boxes := make([]Rectangle, n)
	for i := 0; i < n; i++ {
		pg, err := src.Page(i + 1)
		if err != nil {
			return nil, nil, err
		}
		id, box, err := result.importPageAsXObject(src, pg, idMap)
		if err != nil {
			return nil, nil, err
		}
		xobjIDs[i], boxes[i] = id, box
	}
	return xobjIDs, boxes, nil
}

// importPageAsXObject copies srcPage's resource graph into result (remapping
// object IDs through idMap) and builds a Form XObject whose content is the
// page's content stream and whose /BBox is the page MediaBox. Returns the new
// XObject's object ID and the BBox.
func (result *Document) importPageAsXObject(src *Document, srcPage *Page, idMap map[int]int) (int, Rectangle, error) {
	content, err := srcPage.contentStreams()
	if err != nil {
		return 0, Rectangle{}, err
	}
	box, err := srcPage.MediaBox()
	if err != nil {
		return 0, Rectangle{}, err
	}

	var resVal pdfValue = pdfDict{}
	if pd := srcPage.pageDict(); pd != nil {
		if rv, ok := pd["/Resources"]; ok {
			resVal = rv
		}
	}
	remappedRes := result.importGraph(src.objects, resVal, idMap)

	xobjID := result.nextID
	result.nextID++
	result.objects[xobjID] = &pdfObject{Num: xobjID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/FormType":  1,
			"/BBox":      pdfArray{box.LLX, box.LLY, box.URX, box.URY},
			"/Matrix":    pdfArray{1.0, 0.0, 0.0, 1.0, 0.0, 0.0},
			"/Resources": remappedRes,
		},
		Data:    content,
		Decoded: true,
	}}
	return xobjID, box, nil
}

// importGraph copies every object reachable from root in srcObjects into
// result.objects, assigning fresh IDs via idMap (objects already copied are
// reused). Returns root with all its indirect references remapped.
func (result *Document) importGraph(srcObjects map[int]*pdfObject, root pdfValue, idMap map[int]int) pdfValue {
	deps := map[int]*pdfObject{}
	visited := map[int]bool{}
	collectValueDepsDoc(srcObjects, root, deps, visited)

	// Assign new IDs for all newly-seen objects before remapping any, so an
	// object that references a sibling in this batch resolves correctly.
	for oldID := range deps {
		if _, done := idMap[oldID]; !done {
			idMap[oldID] = result.nextID
			result.nextID++
		}
	}
	for oldID, obj := range deps {
		newID := idMap[oldID]
		if _, exists := result.objects[newID]; exists {
			continue // copied on a previous page
		}
		result.objects[newID] = &pdfObject{Num: newID, Value: rewriteRefs(deepCopyValue(obj.Value), idMap)}
	}
	return rewriteRefs(deepCopyValue(root), idMap)
}

// addImposedSheet appends a new sheet page of the given size, drawing every
// placement onto it via "q cm /Xn Do Q" with the XObjects registered in the
// page's /Resources.
func (result *Document) addImposedSheet(w, h float64, placements []placement) {
	pageObj := result.createBlankPage(w, h)
	pageDict := pageObj.Value.(pdfDict)

	xobjRes := pdfDict{}
	var buf bytes.Buffer
	for i, pl := range placements {
		name := fmt.Sprintf("/X%d", i)
		xobjRes[name] = pdfRef{Num: pl.xobjID}
		buf.WriteString("q\n")
		fmt.Fprintf(&buf, "%s 0 0 %s %s %s cm\n",
			formatFloat(pl.scale), formatFloat(pl.scale), formatFloat(pl.tx), formatFloat(pl.ty))
		buf.WriteString(name)
		buf.WriteString(" Do\nQ\n")
		if pl.border {
			fmt.Fprintf(&buf, "q\n0.5 w\n0.7 0.7 0.7 RG\n%s %s %s %s re\nS\nQ\n",
				formatFloat(pl.bx), formatFloat(pl.by), formatFloat(pl.bw), formatFloat(pl.bh))
		}
	}
	pageDict["/Resources"] = pdfDict{"/XObject": xobjRes}

	contentRef := pageDict["/Contents"].(pdfRef)
	cs := result.objects[contentRef.Num].Value.(*pdfStream)
	cs.Data = buf.Bytes()
	cs.Decoded = true

	result.pages = append(result.pages, pageObj)
}
