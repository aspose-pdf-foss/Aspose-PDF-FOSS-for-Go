// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
	"time"
)

// Printer's marks (ISO 32000-1 §12.5.6.20) guide a printing house and belong outside
// the area that is kept after trimming, so everything here is placed by the page's
// TrimBox and BleedBox rather than by a caller's coordinates. Mirrors Aspose.PDF for
// .NET's PrinterMarkAnnotation family.

// PrinterMarkCornerPosition is the page corner a trim or bleed mark belongs to.
type PrinterMarkCornerPosition int

const (
	PrinterMarkCornerTopLeft PrinterMarkCornerPosition = iota
	PrinterMarkCornerTopRight
	PrinterMarkCornerBottomLeft
	PrinterMarkCornerBottomRight
)

// PrinterMarkSidePosition is the page margin a registration mark is centred in.
type PrinterMarkSidePosition int

const (
	PrinterMarkSideTop PrinterMarkSidePosition = iota
	PrinterMarkSideBottom
	PrinterMarkSideLeft
	PrinterMarkSideRight
)

// ColorsOfCMYK names the process colour a ColorBarAnnotation shows.
type ColorsOfCMYK int

const (
	ColorsOfCMYKCyan ColorsOfCMYK = iota
	ColorsOfCMYKMagenta
	ColorsOfCMYKYellow
	ColorsOfCMYKBlack
)

// PrinterMarksKind selects the marks AddPrinterMarks adds; values combine with |.
type PrinterMarksKind int

const (
	PrinterMarksNone              PrinterMarksKind = 0
	PrinterMarksTrimMarks         PrinterMarksKind = 0x01
	PrinterMarksBleedMarks        PrinterMarksKind = 0x02
	PrinterMarksRegistrationMarks PrinterMarksKind = 0x04
	PrinterMarksColorBars         PrinterMarksKind = 0x08
	PrinterMarksPageInformation   PrinterMarksKind = 0x10
	PrinterMarksAll               PrinterMarksKind = 0x1F
)

// Sizes of the marks, in points.
const (
	markGap        = 6.0  // between the bleed area and a mark
	markLength     = 18.0 // of a trim or bleed line
	markMinLength  = 6.0  // below which a line is not worth drawing
	markStroke     = 0.25 // hairline, as prepress tools draw them
	markTargetSize = 12.0 // registration target
	markRowHeight  = 8.0  // a colour bar, or a line of page information
	markBarWidth   = 40.0 // one colour bar, when the page has the width
)

const printerMarkPrintFlag = 4 // /F bit 3: printed, which is the point of them

// printerMarkBoxes are the page boxes the marks are placed by.
type printerMarkBoxes struct {
	trim, bleed, visible Rectangle
}

// printerMarkBoxes reads the boxes. A BleedBox the page does not state falls back to
// the CropBox in ISO 32000, which says nothing about bleed, so it is taken as the
// TrimBox: no bleed at all.
func (p *Page) printerMarkBoxes() (printerMarkBoxes, error) {
	trim, err := p.TrimBox()
	if err != nil {
		return printerMarkBoxes{}, err
	}
	visible, err := p.CropBox()
	if err != nil {
		return printerMarkBoxes{}, err
	}
	b := printerMarkBoxes{trim: trim, bleed: trim, visible: visible}
	if d := p.pageDict(); d != nil && d["/BleedBox"] != nil {
		if b.bleed, err = p.BleedBox(); err != nil {
			return printerMarkBoxes{}, err
		}
	}
	return b, nil
}

// cornerMarkLayout is where a trim or bleed mark goes: lines through (x, y), each
// starting at (startX, startY) and running length outwards.
type cornerMarkLayout struct {
	rect                 Rectangle
	x, y, startX, startY float64
	dirX, dirY, length   float64
	ok                   bool
}

// corner places a mark. Both kinds start beyond the bleed area, which is trimmed off
// but still printed on.
func (b printerMarkBoxes) corner(pos PrinterMarkCornerPosition, bleedMark bool) cornerMarkLayout {
	box := b.trim
	if bleedMark {
		box = b.bleed
	}
	var l cornerMarkLayout
	var roomX, roomY float64
	if pos == PrinterMarkCornerTopLeft || pos == PrinterMarkCornerBottomLeft {
		l.x, l.startX, l.dirX = box.LLX, b.bleed.LLX-markGap, -1
		roomX = l.startX - b.visible.LLX
	} else {
		l.x, l.startX, l.dirX = box.URX, b.bleed.URX+markGap, 1
		roomX = b.visible.URX - l.startX
	}
	if pos == PrinterMarkCornerBottomLeft || pos == PrinterMarkCornerBottomRight {
		l.y, l.startY, l.dirY = box.LLY, b.bleed.LLY-markGap, -1
		roomY = l.startY - b.visible.LLY
	} else {
		l.y, l.startY, l.dirY = box.URY, b.bleed.URY+markGap, 1
		roomY = b.visible.URY - l.startY
	}
	l.length = min(markLength, roomX, roomY)
	l.ok = l.length >= markMinLength
	if !l.ok {
		l.length = 0
		l.rect = Rectangle{LLX: l.x, LLY: l.y, URX: l.x, URY: l.y}
		return l
	}
	// The rectangle reaches half a stroke inwards, so the lines are not cut lengthwise.
	half := markStroke / 2
	outerX, outerY := l.startX+l.dirX*l.length, l.startY+l.dirY*l.length
	innerX, innerY := l.x-l.dirX*half, l.y-l.dirY*half
	l.rect = Rectangle{LLX: min(outerX, innerX), LLY: min(outerY, innerY), URX: max(outerX, innerX), URY: max(outerY, innerY)}
	return l
}

// side places a registration target, centred along the trim edge.
func (b printerMarkBoxes) side(pos PrinterMarkSidePosition) (Rectangle, bool) {
	midX, midY := (b.trim.LLX+b.trim.URX)/2, (b.trim.LLY+b.trim.URY)/2
	half := markTargetSize / 2
	var r Rectangle
	var room float64
	switch pos {
	case PrinterMarkSideBottom:
		top := b.bleed.LLY - markGap
		r, room = Rectangle{LLX: midX - half, LLY: top - markTargetSize, URX: midX + half, URY: top}, top-b.visible.LLY
	case PrinterMarkSideLeft:
		right := b.bleed.LLX - markGap
		r, room = Rectangle{LLX: right - markTargetSize, LLY: midY - half, URX: right, URY: midY + half}, right-b.visible.LLX
	case PrinterMarkSideRight:
		left := b.bleed.URX + markGap
		r, room = Rectangle{LLX: left, LLY: midY - half, URX: left + markTargetSize, URY: midY + half}, b.visible.URX-left
	default:
		bottom := b.bleed.URY + markGap
		r, room = Rectangle{LLX: midX - half, LLY: bottom, URX: midX + half, URY: bottom + markTargetSize}, b.visible.URY-bottom
	}
	if room < markTargetSize {
		x, y := (r.LLX+r.URX)/2, (r.LLY+r.URY)/2
		return Rectangle{LLX: x, LLY: y, URX: x, URY: y}, false
	}
	return r, true
}

// fitOutsideTrim moves a rectangle that overlaps the TrimBox to the nearest place
// beyond it, along the rectangle's short side, shrinking it to what there is. With no
// room it comes back empty, so the mark exists but shows nothing.
func (b printerMarkBoxes) fitOutsideTrim(r Rectangle) Rectangle {
	if r.LLX > r.URX {
		r.LLX, r.URX = r.URX, r.LLX
	}
	if r.LLY > r.URY {
		r.LLY, r.URY = r.URY, r.LLY
	}
	overlaps := r.LLX < b.trim.URX && r.URX > b.trim.LLX && r.LLY < b.trim.URY && r.URY > b.trim.LLY
	if overlaps {
		if r.URX-r.LLX >= r.URY-r.LLY {
			r.LLY, r.URY = fitBeyond(r.LLY, r.URY, b.trim.LLY, b.trim.URY, b.visible.LLY, b.visible.URY)
		} else {
			r.LLX, r.URX = fitBeyond(r.LLX, r.URX, b.trim.LLX, b.trim.URX, b.visible.LLX, b.visible.URX)
		}
	}
	r.LLX, r.URX = max(r.LLX, b.visible.LLX), min(r.URX, b.visible.URX)
	r.LLY, r.URY = max(r.LLY, b.visible.LLY), min(r.URY, b.visible.URY)
	if r.URX <= r.LLX || r.URY <= r.LLY {
		return Rectangle{LLX: r.LLX, LLY: r.LLY, URX: r.LLX, URY: r.LLY}
	}
	return r
}

// fitBeyond places the interval [lo, hi] on the nearer side of [boxLo, boxHi] that has
// any room within [limitLo, limitHi].
func fitBeyond(lo, hi, boxLo, boxHi, limitLo, limitHi float64) (float64, float64) {
	size := hi - lo
	roomBelow, roomAbove := boxLo-limitLo, limitHi-boxHi
	above := boxHi-lo <= hi-boxLo
	if above && roomAbove <= 0 {
		above = false
	} else if !above && roomBelow <= 0 {
		above = true
	}
	if above {
		return boxHi, boxHi + min(size, max(roomAbove, 0))
	}
	return boxLo - min(size, max(roomBelow, 0)), boxLo
}

func newPrinterMarkBase(page *Page, name string, rect Rectangle) drawingAnnotationBase {
	return drawingAnnotationBase{annotationBase: annotationBase{
		dict: pdfDict{
			"/Type":    pdfName("/Annot"),
			"/Subtype": pdfName("/PrinterMark"),
			"/MN":      pdfName(name),
			"/F":       printerMarkPrintFlag,
			"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
		},
		doc:  page.doc,
		page: page,
	}}
}

// markStrokeColor selects the stroke colour: the annotation's own, or the Separation
// /All that prints on every plate, which is what registration needs. Its alternate is
// DeviceGray, so a viewer shows black and a PDF/A output intent has nothing to object to.
func markStrokeColor(b *appearanceBuilder, c *Color) pdfDict {
	if c != nil {
		b.SetStrokeColorRGB(*c)
		return pdfDict{}
	}
	b.buf.WriteString("/CS0 CS 1 SCN\n")
	return pdfDict{"/ColorSpace": pdfDict{"/CS0": pdfArray{pdfName("/Separation"), pdfName("/All"), pdfName("/DeviceGray"), pdfDict{
		"/FunctionType": 2, "/Domain": pdfArray{0, 1}, "/C0": pdfArray{1}, "/C1": pdfArray{0}, "/N": 1,
	}}}}
}

// cornerPrinterMark is what trim and bleed marks share. Mirrors Aspose.PDF for .NET's
// CornerPrinterMarkAnnotation.
type cornerPrinterMark struct {
	drawingAnnotationBase
	position  PrinterMarkCornerPosition
	bleedMark bool
}

func (a *cornerPrinterMark) boxes() printerMarkBoxes {
	b, _ := a.page.printerMarkBoxes()
	return b
}

// Position returns the corner the mark is in.
func (a *cornerPrinterMark) Position() PrinterMarkCornerPosition { return a.position }

// SetPosition moves the mark to another corner.
func (a *cornerPrinterMark) SetPosition(pos PrinterMarkCornerPosition) {
	a.position = pos
	a.regenerateAP()
}

// RegenerateAppearance redraws the mark. Its rectangle follows from the page's boxes
// and the position, so a rectangle set with SetRect does not last.
func (a *cornerPrinterMark) RegenerateAppearance() { a.regenerateAP() }

func (a *cornerPrinterMark) regenerateAP() {
	l := a.boxes().corner(a.position, a.bleedMark)
	r := l.rect
	a.dict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
	b := newAppearanceBuilder()
	resources := pdfDict{}
	if l.ok {
		b.PushState()
		resources = markStrokeColor(b, a.Color())
		b.SetLineWidth(markStroke)
		b.MoveTo(l.startX-r.LLX, l.y-r.LLY)
		b.LineTo(l.startX+l.dirX*l.length-r.LLX, l.y-r.LLY)
		b.Stroke()
		b.MoveTo(l.x-r.LLX, l.startY-r.LLY)
		b.LineTo(l.x-r.LLX, l.startY+l.dirY*l.length-r.LLY)
		b.Stroke()
		b.PopState()
	}
	setAppearanceN(&a.annotationBase, makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: r.URX - r.LLX, URY: r.URY - r.LLY}, resources))
}

func newCornerPrinterMark(page *Page, name string, pos PrinterMarkCornerPosition, bleed bool) cornerPrinterMark {
	return cornerPrinterMark{drawingAnnotationBase: newPrinterMarkBase(page, name, Rectangle{}), position: pos, bleedMark: bleed}
}

// TrimMarkAnnotation shows where a page is cut: two lines at a corner of the TrimBox,
// starting beyond the bleed area. Mirrors Aspose.PDF for .NET's TrimMarkAnnotation.
type TrimMarkAnnotation struct {
	cornerPrinterMark
}

func (a *TrimMarkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeTrimMark }

// NewTrimMarkAnnotation builds a trim mark for a corner of the page. Add it with
// page.Annotations().Add. With no room outside the TrimBox, which is the case when it
// is as large as the page, the mark has an empty rectangle and shows nothing.
func NewTrimMarkAnnotation(page *Page, position PrinterMarkCornerPosition) *TrimMarkAnnotation {
	if page == nil {
		panic("NewTrimMarkAnnotation: nil page")
	}
	a := &TrimMarkAnnotation{newCornerPrinterMark(page, "/CutMark", position, false)}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// BleedMarkAnnotation shows the corner of the BleedBox, how far artwork runs past the
// cut. A page with no BleedBox has no bleed, so the mark then falls on the trim mark.
// Mirrors Aspose.PDF for .NET's BleedMarkAnnotation.
type BleedMarkAnnotation struct {
	cornerPrinterMark
}

func (a *BleedMarkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeBleedMark }

// NewBleedMarkAnnotation builds a bleed mark for a corner of the page. See
// NewTrimMarkAnnotation for what happens with no room.
func NewBleedMarkAnnotation(page *Page, position PrinterMarkCornerPosition) *BleedMarkAnnotation {
	if page == nil {
		panic("NewBleedMarkAnnotation: nil page")
	}
	a := &BleedMarkAnnotation{newCornerPrinterMark(page, "/BleedMark", position, true)}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// RegistrationMarkAnnotation is the target used to line up the printing plates.
// Mirrors Aspose.PDF for .NET's RegistrationMarkAnnotation.
type RegistrationMarkAnnotation struct {
	drawingAnnotationBase
	position PrinterMarkSidePosition
}

func (a *RegistrationMarkAnnotation) AnnotationType() AnnotationType {
	return AnnotationTypeRegistrationMark
}

// NewRegistrationMarkAnnotation builds a target centred in one margin of the page. Add
// it with page.Annotations().Add. With no room, it has an empty rectangle and shows
// nothing.
func NewRegistrationMarkAnnotation(page *Page, position PrinterMarkSidePosition) *RegistrationMarkAnnotation {
	if page == nil {
		panic("NewRegistrationMarkAnnotation: nil page")
	}
	a := &RegistrationMarkAnnotation{drawingAnnotationBase: newPrinterMarkBase(page, "/RegistrationTarget", Rectangle{}), position: position}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// Position returns the margin the target is in.
func (a *RegistrationMarkAnnotation) Position() PrinterMarkSidePosition { return a.position }

// SetPosition moves the target to another margin.
func (a *RegistrationMarkAnnotation) SetPosition(pos PrinterMarkSidePosition) {
	a.position = pos
	a.regenerateAP()
}

// RegenerateAppearance redraws the target. Its rectangle follows from the page's boxes
// and the position, so a rectangle set with SetRect does not last.
func (a *RegistrationMarkAnnotation) RegenerateAppearance() { a.regenerateAP() }

func (a *RegistrationMarkAnnotation) regenerateAP() {
	boxes, _ := a.page.printerMarkBoxes()
	r, ok := boxes.side(a.position)
	a.dict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
	b := newAppearanceBuilder()
	resources := pdfDict{}
	if ok {
		c := markTargetSize / 2
		b.PushState()
		resources = markStrokeColor(b, a.Color())
		b.SetLineWidth(markStroke)
		b.MoveTo(0, c)
		b.LineTo(markTargetSize, c)
		b.Stroke()
		b.MoveTo(c, 0)
		b.LineTo(c, markTargetSize)
		b.Stroke()
		b.Ellipse(c, c, markTargetSize/3, markTargetSize/3)
		b.Stroke()
		b.PopState()
	}
	setAppearanceN(&a.annotationBase, makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: r.URX - r.LLX, URY: r.URY - r.LLY}, resources))
}

// ColorBarAnnotation is a strip of tints of one process colour, for a press operator to
// check ink density. Mirrors Aspose.PDF for .NET's ColorBarAnnotation; as there, the
// annotation's Color is not used.
type ColorBarAnnotation struct {
	drawingAnnotationBase
	color ColorsOfCMYK
}

func (a *ColorBarAnnotation) AnnotationType() AnnotationType { return AnnotationTypeColorBar }

// NewColorBarAnnotation builds a colour bar; wider than tall it runs horizontally,
// otherwise vertically. A rectangle overlapping the TrimBox moves to the nearest place
// outside it, reduced to fit, and with no room the bar is left empty and shows nothing.
func NewColorBarAnnotation(page *Page, rect Rectangle, color ColorsOfCMYK) *ColorBarAnnotation {
	if page == nil {
		panic("NewColorBarAnnotation: nil page")
	}
	a := &ColorBarAnnotation{drawingAnnotationBase: newPrinterMarkBase(page, "/ColorBar", rect), color: color}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// ColorOfCMYK returns the colour the bar shows.
func (a *ColorBarAnnotation) ColorOfCMYK() ColorsOfCMYK { return a.color }

// SetColorOfCMYK changes the colour the bar shows.
func (a *ColorBarAnnotation) SetColorOfCMYK(c ColorsOfCMYK) {
	a.color = c
	a.regenerateAP()
}

// RegenerateAppearance redraws the bar.
func (a *ColorBarAnnotation) RegenerateAppearance() { a.regenerateAP() }

func (a *ColorBarAnnotation) regenerateAP() {
	boxes, _ := a.page.printerMarkBoxes()
	r := boxes.fitOutsideTrim(a.Rect())
	a.dict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
	w, h := r.URX-r.LLX, r.URY-r.LLY
	b := newAppearanceBuilder()
	if w > 0 && h > 0 {
		tints := []float64{1, 0.75, 0.5, 0.25}
		for i, t := range tints {
			cmyk := [4]float64{}
			cmyk[a.color] = t
			fmt.Fprintf(&b.buf, "%s %s %s %s k\n", formatFloat(cmyk[0]), formatFloat(cmyk[1]), formatFloat(cmyk[2]), formatFloat(cmyk[3]))
			n := float64(len(tints))
			if w >= h {
				b.Rect(w*float64(i)/n, 0, w/n, h)
			} else {
				b.Rect(0, h-h*float64(i+1)/n, w, h/n)
			}
			b.Fill()
		}
	}
	setAppearanceN(&a.annotationBase, makeFormXObject(b.Bytes(), Rectangle{URX: w, URY: h}))
}

// PageInformationAnnotation states the file name, page number, date and time, for
// whoever handles the printed sheet. Mirrors Aspose.PDF for .NET's
// PageInformationAnnotation. The text is the annotation's Contents, written when it is
// created, so saving the document later does not change the date on it.
type PageInformationAnnotation struct {
	drawingAnnotationBase
}

func (a *PageInformationAnnotation) AnnotationType() AnnotationType {
	return AnnotationTypePageInformation
}

// NewPageInformationAnnotation builds the page information at rect, as given. The file
// name is that of the path the document was opened from, and is left out for a
// document built in memory or read from a stream.
func NewPageInformationAnnotation(page *Page, rect Rectangle) *PageInformationAnnotation {
	if page == nil {
		panic("NewPageInformationAnnotation: nil page")
	}
	a := &PageInformationAnnotation{drawingAnnotationBase: newPrinterMarkBase(page, "/PageInformation", rect)}
	parts := []string{fmt.Sprintf("Page %d", page.Number()), time.Now().Format("2006-01-02 15:04:05")}
	if page.doc.sourceName != "" {
		parts = append([]string{page.doc.sourceName}, parts...)
	}
	a.annotationBase.SetContents(strings.Join(parts, "  "))
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// SetContents replaces the text and redraws it.
func (a *PageInformationAnnotation) SetContents(s string) {
	a.annotationBase.SetContents(s)
	a.regenerateAP()
}

// RegenerateAppearance redraws the text.
func (a *PageInformationAnnotation) RegenerateAppearance() { a.regenerateAP() }

func (a *PageInformationAnnotation) regenerateAP() {
	r := a.Rect()
	w, h := r.URX-r.LLX, r.URY-r.LLY
	b := newAppearanceBuilder()
	resources := existingAPNResources(&a.annotationBase)
	if resources == nil {
		resources = pdfDict{}
	}
	if w > 0 && h > 0 {
		size := min(6, h/1.25)
		color := a.Color()
		if color == nil {
			color = &Color{A: 1}
		}
		resolve := func(f Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
			return resolveFontForXObject(f, size, a.doc, resources)
		}
		style := TextStyle{Font: FontHelvetica, Size: size, Color: color, VAlign: VAlignMiddle}
		_ = renderTextInBuilder(b, resources, a.Contents(), style, Rectangle{URX: w, URY: h}, resolve, "", "")
	}
	setAppearanceN(&a.annotationBase, makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: w, URY: h}, resources))
}

// parsePrinterMark reads a /PrinterMark annotation. Its position is not stored, so it
// comes from where the mark sits on the page; a mark this library did not write and
// whose /MN it does not know stays generic.
func parsePrinterMark(base annotationBase) Annotation {
	r := Rectangle{}
	if arr, ok := resolveRefToArray(base.doc.objects, base.dict["/Rect"]); ok && len(arr) == 4 {
		r = Rectangle{LLX: operandFloat(arr[0]), LLY: operandFloat(arr[1]), URX: operandFloat(arr[2]), URY: operandFloat(arr[3])}
	}
	var boxes printerMarkBoxes
	if base.page != nil {
		boxes, _ = base.page.printerMarkBoxes()
	}
	draw := drawingAnnotationBase{annotationBase: base}
	switch dictGetName(base.dict, "/MN") {
	case "/CutMark":
		a := &TrimMarkAnnotation{cornerPrinterMark{drawingAnnotationBase: draw, position: boxes.cornerOf(r)}}
		a.regenerate = a.regenerateAP
		return a
	case "/BleedMark":
		a := &BleedMarkAnnotation{cornerPrinterMark{drawingAnnotationBase: draw, position: boxes.cornerOf(r), bleedMark: true}}
		a.regenerate = a.regenerateAP
		return a
	case "/RegistrationTarget":
		a := &RegistrationMarkAnnotation{drawingAnnotationBase: draw, position: boxes.sideOf(r)}
		a.regenerate = a.regenerateAP
		return a
	case "/ColorBar":
		a := &ColorBarAnnotation{drawingAnnotationBase: draw, color: base.colorBarInk()}
		a.regenerate = a.regenerateAP
		return a
	case "/PageInformation":
		a := &PageInformationAnnotation{drawingAnnotationBase: draw}
		a.regenerate = a.regenerateAP
		return a
	}
	return &GenericAnnotation{annotationBase: base}
}

func (b printerMarkBoxes) center() (float64, float64) {
	return (b.trim.LLX + b.trim.URX) / 2, (b.trim.LLY + b.trim.URY) / 2
}

func (b printerMarkBoxes) cornerOf(r Rectangle) PrinterMarkCornerPosition {
	cx, cy := b.center()
	left, bottom := (r.LLX+r.URX)/2 < cx, (r.LLY+r.URY)/2 < cy
	switch {
	case left && bottom:
		return PrinterMarkCornerBottomLeft
	case bottom:
		return PrinterMarkCornerBottomRight
	case left:
		return PrinterMarkCornerTopLeft
	}
	return PrinterMarkCornerTopRight
}

func (b printerMarkBoxes) sideOf(r Rectangle) PrinterMarkSidePosition {
	cx, cy := b.center()
	dx, dy := (r.LLX+r.URX)/2-cx, (r.LLY+r.URY)/2-cy
	if tw, th := b.trim.URX-b.trim.LLX, b.trim.URY-b.trim.LLY; tw > 0 && th > 0 {
		dx, dy = dx/tw, dy/th
	}
	if dx*dx > dy*dy {
		if dx < 0 {
			return PrinterMarkSideLeft
		}
		return PrinterMarkSideRight
	}
	if dy < 0 {
		return PrinterMarkSideBottom
	}
	return PrinterMarkSideTop
}

// colorBarInk reads which process colour a saved bar shows from the first colour its
// appearance sets.
func (a *annotationBase) colorBarInk() ColorsOfCMYK {
	ap, ok := resolveRefToDict(a.doc.objects, a.dict["/AP"])
	if !ok {
		return ColorsOfCMYKCyan
	}
	st, ok := resolveRef(a.doc.objects, ap["/N"]).(*pdfStream)
	if !ok {
		return ColorsOfCMYKCyan
	}
	ops, err := parseContentStream(decodedStreamData(st))
	if err != nil {
		return ColorsOfCMYKCyan
	}
	for _, op := range ops {
		if op.Operator == "k" && len(op.Operands) == 4 {
			best := 0
			for i := range op.Operands {
				if operandFloat(op.Operands[i]) > operandFloat(op.Operands[best]) {
					best = i
				}
			}
			return ColorsOfCMYK(best)
		}
	}
	return ColorsOfCMYKCyan
}

// planPrinterMarks works out the marks kind asks for on the page without changing
// anything, returning the steps that add them.
func (p *Page) planPrinterMarks(kind PrinterMarksKind) ([]func() Annotation, error) {
	if kind&PrinterMarksAll == 0 {
		return nil, nil
	}
	boxes, err := p.printerMarkBoxes()
	if err != nil {
		return nil, fmt.Errorf("AddPrinterMarks: page %d: %w", p.Number(), err)
	}
	noRoom := func(what string) error {
		return fmt.Errorf("AddPrinterMarks: page %d has no room outside its TrimBox for %s; set a smaller TrimBox or a larger MediaBox", p.Number(), what)
	}
	var plan []func() Annotation
	corners := []PrinterMarkCornerPosition{PrinterMarkCornerTopLeft, PrinterMarkCornerTopRight, PrinterMarkCornerBottomLeft, PrinterMarkCornerBottomRight}
	if kind&PrinterMarksTrimMarks != 0 {
		for _, c := range corners {
			if !boxes.corner(c, false).ok {
				return nil, noRoom("trim marks")
			}
			plan = append(plan, func() Annotation { return NewTrimMarkAnnotation(p, c) })
		}
	}
	if kind&PrinterMarksBleedMarks != 0 {
		for _, c := range corners {
			if !boxes.corner(c, true).ok {
				return nil, noRoom("bleed marks")
			}
			plan = append(plan, func() Annotation { return NewBleedMarkAnnotation(p, c) })
		}
	}
	if kind&PrinterMarksRegistrationMarks != 0 {
		for _, s := range []PrinterMarkSidePosition{PrinterMarkSideTop, PrinterMarkSideBottom, PrinterMarkSideLeft, PrinterMarkSideRight} {
			if _, ok := boxes.side(s); !ok {
				return nil, noRoom("registration marks")
			}
			plan = append(plan, func() Annotation { return NewRegistrationMarkAnnotation(p, s) })
		}
	}
	// The bars and the page information share the top and bottom margins with the
	// registration targets at their centres, so each keeps to the left half, clear of
	// the trim mark's line at the edge.
	halfWidth := (boxes.trim.URX-boxes.trim.LLX)/2 - markTargetSize/2 - 2*markGap
	rowLeft := boxes.trim.LLX + markGap
	if kind&PrinterMarksColorBars != 0 {
		barWidth := min(markBarWidth, halfWidth/4)
		top := boxes.bleed.LLY - markGap
		if top-boxes.visible.LLY < markRowHeight || barWidth < markMinLength {
			return nil, noRoom("colour bars")
		}
		for i, ink := range []ColorsOfCMYK{ColorsOfCMYKCyan, ColorsOfCMYKMagenta, ColorsOfCMYKYellow, ColorsOfCMYKBlack} {
			x := rowLeft + float64(i)*barWidth
			rect := Rectangle{LLX: x, LLY: top - markRowHeight, URX: x + barWidth, URY: top}
			plan = append(plan, func() Annotation { return NewColorBarAnnotation(p, rect, ink) })
		}
	}
	if kind&PrinterMarksPageInformation != 0 {
		bottom := boxes.bleed.URY + markGap
		if boxes.visible.URY-bottom < markRowHeight || halfWidth < markBarWidth {
			return nil, noRoom("page information")
		}
		rect := Rectangle{LLX: rowLeft, LLY: bottom, URX: rowLeft + halfWidth, URY: bottom + markRowHeight}
		plan = append(plan, func() Annotation { return NewPageInformationAnnotation(p, rect) })
	}
	return plan, nil
}

func runPrinterMarkPlan(p *Page, plan []func() Annotation) error {
	for _, step := range plan {
		if err := p.Annotations().Add(step()); err != nil {
			return fmt.Errorf("AddPrinterMarks: page %d: %w", p.Number(), err)
		}
	}
	return nil
}

// AddPrinterMarks adds the marks kind asks for to the page, in the margin outside its
// TrimBox. Mirrors Aspose.PDF for .NET's PrinterMarkAnnotation.AddPrinterMarks, except
// that a page with no room for them is an error rather than marks nobody can see. A
// page whose TrimBox is as large as the page, as most are, has none: use SetTrimBox or
// SetMediaBox first. On an error nothing is added.
func (p *Page) AddPrinterMarks(kind PrinterMarksKind) error {
	plan, err := p.planPrinterMarks(kind)
	if err != nil {
		return err
	}
	return runPrinterMarkPlan(p, plan)
}

// AddPrinterMarks adds the marks kind asks for to every page. See
// (*Page).AddPrinterMarks; if any page has no room, no page is changed.
func (d *Document) AddPrinterMarks(kind PrinterMarksKind) error {
	pages := d.Pages()
	plans := make([][]func() Annotation, len(pages))
	for i, p := range pages {
		plan, err := p.planPrinterMarks(kind)
		if err != nil {
			return err
		}
		plans[i] = plan
	}
	for i, p := range pages {
		if err := runPrinterMarkPlan(p, plans[i]); err != nil {
			return err
		}
	}
	return nil
}
