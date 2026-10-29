// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strconv"
)

// ContentsResizeValue is one dimension of a ContentsResizeParameters: a
// length in points, a percentage of the page's original size along the same
// axis, or — the zero value — automatic. Mirrors Aspose.PDF for .NET's
// PdfFileEditor.ContentsResizeValue.
type ContentsResizeValue struct {
	value   float64
	percent bool
	set     bool
}

// ResizeUnits is a length in points.
func ResizeUnits(points float64) ContentsResizeValue {
	return ContentsResizeValue{value: points, set: true}
}

// ResizePercents is a percentage of the page's original size along the same
// axis (width for horizontal values, height for vertical ones).
func ResizePercents(percent float64) ContentsResizeValue {
	return ContentsResizeValue{value: percent, percent: true, set: true}
}

// IsAuto reports whether the value is automatic (the zero value).
func (v ContentsResizeValue) IsAuto() bool { return !v.set }

func (v ContentsResizeValue) points(original float64) float64 {
	if v.percent {
		return v.value / 100 * original
	}
	return v.value
}

// ContentsResizeParameters says how ResizeContents rescales a page. Mirrors
// the intent of Aspose.PDF for .NET's PdfFileEditor.ContentsResizeParameters,
// with the page size made explicit.
//
// Per axis: the new page size is PageWidth/PageHeight (automatic: unchanged).
// The page's content is scaled into a box ContentsWidth × ContentsHeight
// (automatic: the page minus the margins) placed by the margins: with both
// margins of an axis automatic the box is centred, with one given it is
// placed from that side, with both given the left (bottom) one wins. Sizes
// and margins are in the page's displayed orientation (/Rotate taken into
// account). Percentages are of the original page size.
type ContentsResizeParameters struct {
	PageWidth, PageHeight                            ContentsResizeValue
	LeftMargin, RightMargin, TopMargin, BottomMargin ContentsResizeValue
	ContentsWidth, ContentsHeight                    ContentsResizeValue
	// PreserveAspectRatio scales the content by the same factor on both
	// axes (the smaller of the two), centred in its box, instead of
	// stretching it to fill the box exactly.
	PreserveAspectRatio bool
}

// ResizePage makes the page width × height points and scales its content
// to fill it. Mirrors ContentsResizeParameters.PageResize.
func ResizePage(width, height float64) ContentsResizeParameters {
	return ContentsResizeParameters{PageWidth: ResizeUnits(width), PageHeight: ResizeUnits(height)}
}

// ResizePagePct scales the page and its content to widthPct × heightPct
// percent of their size. Mirrors ContentsResizeParameters.PageResizePct.
func ResizePagePct(widthPct, heightPct float64) ContentsResizeParameters {
	return ContentsResizeParameters{PageWidth: ResizePercents(widthPct), PageHeight: ResizePercents(heightPct)}
}

// ResizeContentSize keeps the page size and scales the content to width ×
// height points, centred. Mirrors ContentsResizeParameters.ContentSize.
func ResizeContentSize(width, height float64) ContentsResizeParameters {
	return ContentsResizeParameters{ContentsWidth: ResizeUnits(width), ContentsHeight: ResizeUnits(height)}
}

// ResizeContentSizePercent keeps the page size and scales the content to
// widthPct × heightPct percent, centred. Mirrors
// ContentsResizeParameters.ContentSizePercent.
func ResizeContentSizePercent(widthPct, heightPct float64) ContentsResizeParameters {
	return ContentsResizeParameters{ContentsWidth: ResizePercents(widthPct), ContentsHeight: ResizePercents(heightPct)}
}

// ResizeMargins keeps the page size and shrinks the content to fit inside
// the given margins (points). Mirrors ContentsResizeParameters.Margins.
func ResizeMargins(left, right, top, bottom float64) ContentsResizeParameters {
	return ContentsResizeParameters{
		LeftMargin: ResizeUnits(left), RightMargin: ResizeUnits(right),
		TopMargin: ResizeUnits(top), BottomMargin: ResizeUnits(bottom),
	}
}

// ResizeMarginsPercent is ResizeMargins with margins in percent of the page
// size. Mirrors ContentsResizeParameters.MarginsPercent.
func ResizeMarginsPercent(left, right, top, bottom float64) ContentsResizeParameters {
	return ContentsResizeParameters{
		LeftMargin: ResizePercents(left), RightMargin: ResizePercents(right),
		TopMargin: ResizePercents(top), BottomMargin: ResizePercents(bottom),
	}
}

// ResizeContents rescales the given pages (all when none are given): each
// page gets its new size and its content — text, graphics, images and
// annotations — is scaled and moved into place as params describe. The
// content streams are not rewritten; they are wrapped in one scaling
// transformation, so text stays extractable and searchable at its new
// position. Every page is checked before any is changed. Destinations that
// point at a position on a resized page (outline items, links) keep their
// old coordinates. Mirrors Aspose.PDF for .NET's
// PdfFileEditor.ResizeContents.
func (d *Document) ResizeContents(params ContentsResizeParameters, pageNums ...int) error {
	if len(pageNums) == 0 {
		for i := range d.pages {
			pageNums = append(pageNums, i+1)
		}
	}
	type plan struct {
		page *Page
		rp   resizePlan
	}
	var plans []plan
	seen := map[int]bool{}
	for _, n := range pageNums {
		if seen[n] {
			continue
		}
		seen[n] = true
		p, err := d.Page(n)
		if err != nil {
			return err
		}
		rp, err := p.planResize(params)
		if err != nil {
			return fmt.Errorf("ResizeContents: page %d: %w", n, err)
		}
		plans = append(plans, plan{p, rp})
	}
	for _, pl := range plans {
		pl.page.applyResize(pl.rp)
	}
	return nil
}

// ResizeContents rescales this page; see Document.ResizeContents.
func (p *Page) ResizeContents(params ContentsResizeParameters) error {
	rp, err := p.planResize(params)
	if err != nil {
		return fmt.Errorf("ResizeContents: %w", err)
	}
	p.applyResize(rp)
	return nil
}

// resizePlan is a page's new MediaBox (unrotated user space, origin 0,0)
// and the matrix taking the old content there.
type resizePlan struct {
	newW, newH float64 // unrotated
	m          [6]float64
}

func (p *Page) planResize(params ContentsResizeParameters) (resizePlan, error) {
	if p.obj != nil {
		return resizePlan{}, fmt.Errorf("a detached canvas cannot be resized")
	}
	crop, err := p.CropBox()
	if err != nil {
		return resizePlan{}, err
	}
	ow, oh := crop.URX-crop.LLX, crop.URY-crop.LLY
	if !(ow > 0 && oh > 0) {
		return resizePlan{}, fmt.Errorf("page has an empty visible area")
	}
	rot := ((int(p.Rotation())%360)+360)%360 == 90 || ((int(p.Rotation())%360)+360)%360 == 270

	// Work in the displayed orientation: swap axes for a quarter-turn page.
	dw, dh := ow, oh
	if rot {
		dw, dh = oh, ow
	}
	pw, cw, cx, err := resizeAxis(dw, params.PageWidth, params.ContentsWidth, params.LeftMargin, params.RightMargin)
	if err != nil {
		return resizePlan{}, fmt.Errorf("width: %w", err)
	}
	ph, ch, cy, err := resizeAxis(dh, params.PageHeight, params.ContentsHeight, params.BottomMargin, params.TopMargin)
	if err != nil {
		return resizePlan{}, fmt.Errorf("height: %w", err)
	}

	// Back to unrotated user space. A quarter turn swaps the axes; the
	// content box keeps its displayed position, which in unrotated space
	// mirrors along one axis depending on the turn direction.
	newW, newH := pw, ph
	boxX, boxY, boxW, boxH := cx, cy, cw, ch
	switch ((int(p.Rotation()) % 360) + 360) % 360 {
	case 90: // displayed x = unrotated y, displayed y = W - unrotated x
		newW, newH = ph, pw
		boxX, boxY, boxW, boxH = ph-cy-ch, cx, ch, cw
	case 180:
		boxX, boxY = pw-cx-cw, ph-cy-ch
	case 270: // displayed x = H - unrotated y, displayed y = unrotated x
		newW, newH = ph, pw
		boxX, boxY, boxW, boxH = cy, pw-cx-cw, ch, cw
	}

	sx, sy := boxW/ow, boxH/oh
	if params.PreserveAspectRatio {
		s := math.Min(sx, sy)
		boxX += (boxW - ow*s) / 2
		boxY += (boxH - oh*s) / 2
		sx, sy = s, s
	}
	m := [6]float64{sx, 0, 0, sy, boxX - crop.LLX*sx, boxY - crop.LLY*sy}
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return resizePlan{}, fmt.Errorf("parameters give a non-finite transformation")
		}
	}
	return resizePlan{newW: newW, newH: newH, m: m}, nil
}

// resizeAxis resolves one axis: the new page length, the content length and
// the content's offset from the low (left/bottom) edge.
func resizeAxis(orig float64, page, content, lowMargin, highMargin ContentsResizeValue) (pageLen, contentLen, offset float64, err error) {
	pageLen = orig
	if !page.IsAuto() {
		pageLen = page.points(orig)
	}
	if !(pageLen > 0) || math.IsInf(pageLen, 0) {
		return 0, 0, 0, fmt.Errorf("page size %v must be positive", pageLen)
	}
	lo, hi := 0.0, 0.0
	if !lowMargin.IsAuto() {
		lo = lowMargin.points(orig)
	}
	if !highMargin.IsAuto() {
		hi = highMargin.points(orig)
	}
	if lo < 0 || hi < 0 || math.IsNaN(lo) || math.IsNaN(hi) {
		return 0, 0, 0, fmt.Errorf("margins must not be negative")
	}
	if content.IsAuto() {
		contentLen = pageLen - lo - hi
		offset = lo
	} else {
		contentLen = content.points(orig)
		switch {
		case !lowMargin.IsAuto():
			offset = lo
		case !highMargin.IsAuto():
			offset = pageLen - hi - contentLen
		default:
			offset = (pageLen - contentLen) / 2
		}
	}
	if !(contentLen > 0) || math.IsInf(contentLen, 0) {
		return 0, 0, 0, fmt.Errorf("content size %v must be positive", contentLen)
	}
	if offset < -1e-9 || offset+contentLen > pageLen+1e-9 {
		return 0, 0, 0, fmt.Errorf("content (%v at %v) does not fit the page (%v)", contentLen, offset, pageLen)
	}
	return pageLen, contentLen, offset, nil
}

// applyResize installs the plan: the content wrapped in "q <m> cm … Q",
// the new MediaBox (the visible area is the whole new page), the other
// boxes and the annotations moved with the content.
func (p *Page) applyResize(rp resizePlan) {
	d := p.doc
	dict := p.pageDict()

	entries, _ := contentsEntries(d.objects, dict["/Contents"])
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	cm := fmt.Sprintf("q %s %s %s %s %s %s cm\n", num(rp.m[0]), num(rp.m[1]), num(rp.m[2]), num(rp.m[3]), num(rp.m[4]), num(rp.m[5]))
	start := newContentStream(d, []byte(cm), nil)
	// The closing Q carries the append marker: the original content is now
	// isolated in its own q…Q, so later drawing appends after it, in the
	// new page's coordinates, without another wrap.
	end := newContentStream(d, []byte("\nQ\n"), pdfDict{contentAppendMarkerKey: true})
	wrapped := make(pdfArray, 0, len(entries)+2)
	wrapped = append(wrapped, start)
	wrapped = append(wrapped, entries...)
	wrapped = append(wrapped, end)
	dict["/Contents"] = wrapped
	p.contentsBounded, p.contentsLen = true, len(wrapped)

	for _, key := range []string{"/TrimBox", "/BleedBox", "/ArtBox"} {
		if r, ok := p.ownBox(key); ok {
			x0, y0, x1, y1 := transformedBBox(rp.m, r.LLX, r.LLY, r.URX, r.URY)
			dict[key] = pdfArray{x0, y0, x1, y1}
		}
	}
	delete(dict, "/CropBox")
	dict["/MediaBox"] = pdfArray{0, 0, rp.newW, rp.newH}

	annots, _ := resolveRef(d.objects, dict["/Annots"]).(pdfArray)
	for _, a := range annots {
		ad, ok := resolveRefToDict(d.objects, a)
		if !ok {
			continue
		}
		resizeAnnotation(d.objects, ad, rp.m)
	}
}

// ownBox reads a box set directly on the page dict (not inherited).
func (p *Page) ownBox(key string) (Rectangle, bool) {
	arr, ok := resolveRef(p.doc.objects, p.pageDict()[key]).(pdfArray)
	if !ok || len(arr) != 4 {
		return Rectangle{}, false
	}
	return Rectangle{LLX: operandFloat(arr[0]), LLY: operandFloat(arr[1]), URX: operandFloat(arr[2]), URY: operandFloat(arr[3])}, true
}

// resizeAnnotation moves an annotation's geometry through m: its /Rect and
// every coordinate array that positions it on the page. The appearance
// stream follows /Rect by itself (ISO 32000-1 §12.5.5).
func resizeAnnotation(objects map[int]*pdfObject, ad pdfDict, m [6]float64) {
	if arr, ok := resolveRef(objects, ad["/Rect"]).(pdfArray); ok && len(arr) == 4 {
		x0, y0, x1, y1 := transformedBBox(m, operandFloat(arr[0]), operandFloat(arr[1]), operandFloat(arr[2]), operandFloat(arr[3]))
		ad["/Rect"] = pdfArray{x0, y0, x1, y1}
	}
	points := func(arr pdfArray) pdfArray {
		out := make(pdfArray, len(arr))
		copy(out, arr)
		for i := 0; i+1 < len(arr); i += 2 {
			x, y := matApplyPoint(m, operandFloat(arr[i]), operandFloat(arr[i+1]))
			out[i], out[i+1] = x, y
		}
		return out
	}
	for _, key := range []string{"/QuadPoints", "/Vertices", "/L", "/CL"} {
		if arr, ok := resolveRef(objects, ad[key]).(pdfArray); ok {
			ad[key] = points(arr)
		}
	}
	if ink, ok := resolveRef(objects, ad["/InkList"]).(pdfArray); ok {
		out := make(pdfArray, len(ink))
		for i, stroke := range ink {
			if arr, ok := resolveRef(objects, stroke).(pdfArray); ok {
				out[i] = points(arr)
			} else {
				out[i] = stroke
			}
		}
		ad["/InkList"] = out
	}
}
