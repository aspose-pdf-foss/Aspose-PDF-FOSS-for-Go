// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"os"
)

// ButtonIconPosition controls how a push button lays out its icon and
// caption — the /MK /TP entry per ISO 32000-1 §12.5.6.19 Table 189.
type ButtonIconPosition int

const (
	ButtonCaptionOnly      ButtonIconPosition = 0 // /TP 0 — caption only, no icon
	ButtonIconOnly         ButtonIconPosition = 1 // /TP 1 — icon only, no caption
	ButtonIconAboveCaption ButtonIconPosition = 2 // /TP 2 — caption below the icon
	ButtonCaptionOverIcon  ButtonIconPosition = 6 // /TP 6 — caption overlaid on the icon
)

// ButtonAppearance configures a push button's rich appearance: separate
// captions for the normal / rollover / down states, an optional icon
// image, and face/border/text colours. Applied via
// (*ButtonField).SetAppearance, which writes the /MK appearance-
// characteristics dict and bakes /AP/N, /AP/R, and /AP/D appearance
// streams so the button reacts to hover and press in every viewer.
//
// Out of scope (follow-up): caption rotation (/MK/R) and exposing the
// icon as a /MK/I Form XObject for viewer-side regeneration — the icon
// is rendered directly into the /AP streams here, which renders in any
// viewer without relying on regeneration.
type ButtonAppearance struct {
	Caption      string             // /MK/CA — normal-state caption
	RolloverText string             // /MK/RC — caption while hovered (falls back to Caption)
	DownText     string             // /MK/AC — caption while pressed (falls back to Caption)
	IconPath     string             // optional icon image (PNG/JPEG), baked into the /AP streams
	IconPosition ButtonIconPosition // /MK/TP — icon vs caption layout
	TextColor    *Color             // caption colour (default dark grey)
	FaceColor    *Color             // button face fill (default light grey)
	BorderColor  *Color             // button border (default grey)
}

// Action returns the button's activation action (/A on its widget), or nil
// if none is set or the action type is unsupported. Mirrors the
// LinkAnnotation action surface.
func (f *ButtonField) Action() Action {
	if f.node == nil || len(f.node.widgets) == 0 {
		return nil
	}
	d, ok := resolveRefToDict(f.node.form.doc.objects, f.node.widgets[0]["/A"])
	if !ok {
		return nil
	}
	act := parseAction(d)
	resolveGoToPage(f.node.form.doc, act, d)
	return act
}

// SetAction writes the button's activation action (/A) onto its widget(s) —
// typically a SubmitFormAction, ResetFormAction, GoTo/URI or JavaScript
// action. nil clears the action. Mirrors Aspose.PDF for .NET's
// ButtonField.OnActivated.
func (f *ButtonField) SetAction(act Action) {
	if f.node == nil {
		return
	}
	for _, w := range f.node.widgets {
		if act == nil {
			delete(w, "/A")
		} else {
			w["/A"] = act.encode()
		}
	}
	noteFormMutated(f.node)
}

// SetAppearance applies a ButtonAppearance to the push button: it writes
// the /MK characteristics (/CA, /RC, /AC, /TP) and regenerates the
// widget's /AP with distinct /N (normal), /R (rollover), and /D (down)
// appearance streams. Returns an error if the field has no widget or the
// icon image cannot be read.
func (f *ButtonField) SetAppearance(a ButtonAppearance) error {
	if f.node == nil || len(f.node.widgets) == 0 {
		return errFieldDetached
	}
	widget := f.node.widgets[0]

	// Merge the textual/numeric characteristics into /MK (preserving any
	// existing entries like a /BG set via FieldStyle).
	mk, _ := widget["/MK"].(pdfDict)
	if mk == nil {
		mk = pdfDict{}
	}
	if a.Caption != "" {
		mk["/CA"] = encodeFormString(a.Caption)
	}
	if a.RolloverText != "" {
		mk["/RC"] = encodeFormString(a.RolloverText)
	}
	if a.DownText != "" {
		mk["/AC"] = encodeFormString(a.DownText)
	}
	mk["/TP"] = int(a.IconPosition)
	widget["/MK"] = mk

	// Resolve the three state captions.
	normal := a.Caption
	rollover := a.RolloverText
	if rollover == "" {
		rollover = normal
	}
	down := a.DownText
	if down == "" {
		down = normal
	}

	nStream, err := pushButtonStateAppearance(f.node.form, widget, a, normal, false)
	if err != nil {
		return err
	}
	rStream, err := pushButtonStateAppearance(f.node.form, widget, a, rollover, false)
	if err != nil {
		return err
	}
	dStream, err := pushButtonStateAppearance(f.node.form, widget, a, down, true)
	if err != nil {
		return err
	}

	doc := f.node.form.doc
	ap := pdfDict{
		"/N": pdfRef{Num: doc.addObject(nStream)},
		"/R": pdfRef{Num: doc.addObject(rStream)},
		"/D": pdfRef{Num: doc.addObject(dStream)},
	}
	widget["/AP"] = ap
	f.node.form.noteFormMutatedInForm()
	return nil
}

// pushButtonStateAppearance renders one push-button state into a Form
// XObject: the rounded face (darkened slightly when pressed), an optional
// icon, and the caption, laid out per a.IconPosition.
func pushButtonStateAppearance(form *Form, widget pdfDict, a ButtonAppearance, caption string, pressed bool) (*pdfStream, error) {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{}), nil
	}

	face := colorOr(a.FaceColor, Color{R: 0.93, G: 0.93, B: 0.95, A: 1})
	border := colorOr(a.BorderColor, Color{R: 0.55, G: 0.55, B: 0.60, A: 1})
	textColor := colorOr(a.TextColor, Color{R: 0.15, G: 0.15, B: 0.20, A: 1})
	if pressed {
		face = scaleColor(face, 0.92) // visual "depressed" cue
	}

	b := newAppearanceBuilder()
	radius := math.Min(6, height/3)
	drawRoundedRectPath(b, 0.5, 0.5, width-1, height-1, radius)
	b.PushState()
	b.SetFillColorRGB(face)
	b.SetStrokeColorRGB(border)
	b.SetLineWidth(0.7)
	b.FillStroke()
	b.PopState()

	// Pressed state nudges the content down-right by 0.5pt.
	var dx, dy float64
	if pressed {
		dx, dy = 0.5, -0.5
	}

	const pad = 4.0
	inner := Rectangle{LLX: pad, LLY: pad, URX: width - pad, URY: height - pad}
	iconRect, captionRect, showIcon, showCaption := layoutButtonContent(a.IconPosition, inner, a.IconPath != "", caption != "")

	resources := pdfDict{}

	if showIcon && a.IconPath != "" {
		ref, iw, ih, err := form.doc.embedButtonIcon(a.IconPath)
		if err != nil {
			return nil, err
		}
		name := registerIconResource(resources, ref)
		emitFitImage(b, name, iconRect.translate(dx, dy), iw, ih)
	}

	if showCaption && caption != "" {
		fontSize := math.Min((captionRect.URY-captionRect.LLY)*0.6, 14)
		if fontSize < 4 {
			fontSize = 4
		}
		style := TextStyle{
			Font:   FontHelveticaBold,
			Size:   fontSize,
			Color:  &textColor,
			HAlign: HAlignCenter,
			VAlign: VAlignMiddle,
		}
		resolve := func(font Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
			return resolveFontForXObject(font, fontSize, form.doc, resources)
		}
		_ = renderTextInBuilder(b, resources, caption, style, captionRect.translate(dx, dy), resolve, "", "")
	}

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources), nil
}

// layoutButtonContent splits the inner rect into icon and caption regions
// per the icon position, and reports which elements to draw.
func layoutButtonContent(pos ButtonIconPosition, inner Rectangle, hasIcon, hasCaption bool) (iconRect, captionRect Rectangle, showIcon, showCaption bool) {
	switch pos {
	case ButtonIconOnly:
		return inner, Rectangle{}, hasIcon, false
	case ButtonIconAboveCaption:
		if !hasIcon {
			return Rectangle{}, inner, false, hasCaption
		}
		split := inner.LLY + (inner.URY-inner.LLY)*0.35
		icon := Rectangle{LLX: inner.LLX, LLY: split, URX: inner.URX, URY: inner.URY}
		cap := Rectangle{LLX: inner.LLX, LLY: inner.LLY, URX: inner.URX, URY: split}
		return icon, cap, hasIcon, hasCaption
	case ButtonCaptionOverIcon:
		return inner, inner, hasIcon, hasCaption
	default: // ButtonCaptionOnly
		return Rectangle{}, inner, false, hasCaption
	}
}

// emitFitImage draws the icon scaled to fit region while preserving its
// aspect ratio, centred. iw/ih are the icon's intrinsic pixel dimensions.
func emitFitImage(b *appearanceBuilder, name pdfName, region Rectangle, iw, ih float64) {
	rw := region.URX - region.LLX
	rh := region.URY - region.LLY
	if rw <= 0 || rh <= 0 || iw <= 0 || ih <= 0 {
		return
	}
	scale := math.Min(rw/iw, rh/ih)
	dw := iw * scale
	dh := ih * scale
	tx := region.LLX + (rw-dw)/2
	ty := region.LLY + (rh-dh)/2
	b.PushState()
	b.ConcatMatrix(dw, 0, 0, dh, tx, ty)
	b.DoXObject(name)
	b.PopState()
}

// registerIconResource adds the image XObject ref to a resources dict's
// /XObject subdict and returns the chosen resource name.
func registerIconResource(resources pdfDict, ref pdfRef) pdfName {
	xobj, _ := resources["/XObject"].(pdfDict)
	if xobj == nil {
		xobj = pdfDict{}
		resources["/XObject"] = xobj
	}
	name := nextXObjectName(xobj)
	xobj[name] = ref
	return pdfName(name)
}

// embedButtonIcon reads an image file, builds an Image XObject (plus soft
// mask for alpha), registers it in the document, and returns a reference
// plus the intrinsic pixel dimensions.
func (d *Document) embedButtonIcon(path string) (pdfRef, float64, float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pdfRef{}, 0, 0, fmt.Errorf("button icon: %w", err)
	}
	format, err := detectImageFormat(data)
	if err != nil {
		return pdfRef{}, 0, 0, fmt.Errorf("button icon: %w", err)
	}
	imgStream, smaskStream, err := createImageXObject(data, format)
	if err != nil {
		return pdfRef{}, 0, 0, fmt.Errorf("button icon: %w", err)
	}
	if smaskStream != nil {
		smaskID := d.addObject(smaskStream)
		imgStream.Dict["/SMask"] = pdfRef{Num: smaskID}
	}
	imgID := d.addObject(imgStream)
	w, _ := imgStream.Dict["/Width"].(int)
	h, _ := imgStream.Dict["/Height"].(int)
	return pdfRef{Num: imgID}, float64(w), float64(h), nil
}

// translate returns r shifted by (dx, dy).
func (r Rectangle) translate(dx, dy float64) Rectangle {
	return Rectangle{LLX: r.LLX + dx, LLY: r.LLY + dy, URX: r.URX + dx, URY: r.URY + dy}
}

// colorOr returns *c if non-nil, else def.
func colorOr(c *Color, def Color) Color {
	if c != nil {
		return *c
	}
	return def
}

// scaleColor multiplies a colour's RGB channels by k (clamped to [0,1]),
// darkening (k<1) or lightening (k>1) it.
func scaleColor(c Color, k float64) Color {
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	return Color{R: clamp(c.R * k), G: clamp(c.G * k), B: clamp(c.B * k), A: c.A}
}
