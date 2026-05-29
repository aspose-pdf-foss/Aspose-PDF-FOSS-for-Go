// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strconv"
	"strings"
)

// errFieldDetached is returned by mutators when the field has no backing
// node (e.g. a zero-value handle never returned by the Form API).
var errFieldDetached = fmt.Errorf("form field is not attached to a document")

// FieldStyle is the visual styling applied to a form field's widget(s).
// A zero value means "leave at the library default" for every property:
// nil colour pointers, zero BorderWidth, BorderSolid, nil DashPattern,
// nil TextFont (→ Helvetica), zero TextSize (→ 12pt), HAlignLeft.
//
// Styling is persisted into the PDF the way ISO 32000-1 specifies, so it
// round-trips and is also honoured by viewers that regenerate appearances:
//   - BorderColor / BackgroundColor → /MK /BC, /MK /BG (§12.5.6.19)
//   - BorderWidth / BorderStyle / DashPattern → /BS /W, /S, /D (§12.5.4)
//   - TextFont / TextSize / TextColor → /DA default-appearance string (§12.7.3.3)
//   - TextAlign → /Q quadding (0 left, 1 centre, 2 right)
//
// Because the library also pre-generates the widget /AP appearance stream
// from these same values, a styled field renders identically across
// Acrobat, Foxit, browser PDF viewers, MuPDF, and Poppler.
type FieldStyle struct {
	BorderColor     *Color      // /MK /BC — nil = no border colour set
	BackgroundColor *Color      // /MK /BG — nil = transparent/white default
	TextColor       *Color      // /DA fill colour — nil = black
	BorderWidth     float64     // /BS /W in points — 0 = hairline default
	BorderStyle     BorderStyle // /BS /S — BorderSolid (default), Dashed, Beveled, Inset, Underline
	DashPattern     []float64   // /BS /D — used only when BorderStyle == BorderDashed
	TextFont        Font        // /DA font — nil = Helvetica
	TextSize        float64     // /DA size in points — 0 = 12pt
	TextAlign       HAlign      // /Q — HAlignLeft (default), HAlignCenter, HAlignRight
}

// SetStyle applies s to the field, writing /MK, /BS, /DA, and /Q on the
// field and its widgets and regenerating each widget's /AP appearance so
// the new look is visible immediately in every viewer. Returns an error
// only if the field is detached or a styled font cannot be registered.
func (b *fieldBase) SetStyle(s FieldStyle) error {
	if b.node == nil {
		return errFieldDetached
	}
	form := b.node.form

	font := s.TextFont
	if font == nil {
		font = FontHelvetica
	}
	resName := "Helv"
	if form != nil {
		rn, err := form.ensureFont(font)
		if err != nil {
			return err
		}
		resName = rn
	}
	size := s.TextSize
	if size <= 0 {
		size = 12
	}

	// /DA + /Q live on the field dict (inherited by widgets).
	b.node.dict["/DA"] = buildDA(s.TextColor, resName, size)
	if q := hAlignToQ(s.TextAlign); q != 0 {
		b.node.dict["/Q"] = q
	} else {
		delete(b.node.dict, "/Q")
	}

	// /MK + /BS live on each widget.
	for _, w := range b.node.widgets {
		applyWidgetMKBS(w, s)
	}

	regenerateFieldAppearance(b.node)
	if form != nil {
		form.noteFormMutatedInForm()
	}
	return nil
}

// Style reads back the field's current styling from /DA, /Q, /MK, and
// /BS. Properties never set return their zero/default value. The first
// widget is used for /MK and /BS.
func (b *fieldBase) Style() FieldStyle {
	var s FieldStyle
	if b.node == nil {
		return s
	}
	size, color, fontRes := parseDA(dictGetString(b.node.dict, "/DA"))
	s.TextSize = size
	c := color
	s.TextColor = &c
	if b.node.form != nil {
		s.TextFont = resolveWidgetFont(b.node.form, fontRes)
	}
	s.TextAlign = qToHAlign(dictGetInt(b.node.dict, "/Q"))

	if len(b.node.widgets) > 0 {
		w := b.node.widgets[0]
		s.BackgroundColor = mkColor(w, "/BG")
		s.BorderColor = mkColor(w, "/BC")
		s.BorderWidth, s.BorderStyle, s.DashPattern = readBS(w)
	}
	return s
}

// applyWidgetMKBS writes /MK (border + background colours) and /BS
// (border width, style, dash) onto a single widget dict from s, merging
// into any existing /MK so sibling entries like a push button's /CA
// caption are preserved.
func applyWidgetMKBS(w pdfDict, s FieldStyle) {
	mk, _ := w["/MK"].(pdfDict)
	if mk == nil {
		mk = pdfDict{}
	}
	if s.BackgroundColor != nil {
		mk["/BG"] = colorComponents(*s.BackgroundColor)
	} else {
		delete(mk, "/BG")
	}
	if s.BorderColor != nil {
		mk["/BC"] = colorComponents(*s.BorderColor)
	} else {
		delete(mk, "/BC")
	}
	if len(mk) > 0 {
		w["/MK"] = mk
	} else {
		delete(w, "/MK")
	}

	bs := pdfDict{"/Type": pdfName("/Border")}
	if s.BorderWidth > 0 {
		bs["/W"] = s.BorderWidth
	} else {
		bs["/W"] = 0
	}
	bs["/S"] = borderStyleName(s.BorderStyle)
	if s.BorderStyle == BorderDashed && len(s.DashPattern) > 0 {
		arr := make(pdfArray, len(s.DashPattern))
		for i, d := range s.DashPattern {
			arr[i] = d
		}
		bs["/D"] = arr
	}
	w["/BS"] = bs
}

// buildDA assembles a /DA default-appearance string: an optional fill
// colour ("r g b rg" or "0 g") followed by the font + size ("/Res sz Tf").
func buildDA(color *Color, resName string, size float64) string {
	var b strings.Builder
	if color != nil {
		b.WriteString(formatFloat(color.R))
		b.WriteByte(' ')
		b.WriteString(formatFloat(color.G))
		b.WriteByte(' ')
		b.WriteString(formatFloat(color.B))
		b.WriteString(" rg ")
	} else {
		b.WriteString("0 g ")
	}
	b.WriteByte('/')
	b.WriteString(resName)
	b.WriteByte(' ')
	b.WriteString(formatFloat(size))
	b.WriteString(" Tf")
	return b.String()
}

// borderStyleFromName is the inverse of borderStyleName (annotation_drawing.go).
func borderStyleFromName(n string) BorderStyle {
	switch n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	default:
		return BorderSolid
	}
}

// hAlignToQ maps an HAlign to the /Q quadding value (0/1/2).
func hAlignToQ(a HAlign) int {
	switch a {
	case HAlignCenter:
		return 1
	case HAlignRight:
		return 2
	default:
		return 0
	}
}

// qToHAlign is the inverse of hAlignToQ.
func qToHAlign(q int) HAlign {
	switch q {
	case 1:
		return HAlignCenter
	case 2:
		return HAlignRight
	default:
		return HAlignLeft
	}
}

// colorComponents renders a Color as a /MK colour array. RGB is emitted
// as a 3-element array; an exact grey (R==G==B) stays 3-element too for
// simplicity (viewers accept it).
func colorComponents(c Color) pdfArray {
	return pdfArray{c.R, c.G, c.B}
}

// mkColor reads a colour from widget /MK under key (/BC or /BG). Returns
// nil when absent or empty. Supports 1- (grey), 3- (RGB), and 4-element
// (CMYK→RGB) component arrays per ISO 32000-1 §12.5.6.19.
func mkColor(w pdfDict, key string) *Color {
	mk, ok := w["/MK"].(pdfDict)
	if !ok {
		return nil
	}
	arr, ok := mk[key].(pdfArray)
	if !ok || len(arr) == 0 {
		return nil
	}
	switch len(arr) {
	case 1:
		g, _ := toFloat(arr[0])
		return &Color{R: g, G: g, B: g, A: 1}
	case 3:
		r, _ := toFloat(arr[0])
		g, _ := toFloat(arr[1])
		b, _ := toFloat(arr[2])
		return &Color{R: r, G: g, B: b, A: 1}
	case 4:
		c, _ := toFloat(arr[0])
		m, _ := toFloat(arr[1])
		y, _ := toFloat(arr[2])
		k, _ := toFloat(arr[3])
		return &Color{R: (1 - c) * (1 - k), G: (1 - m) * (1 - k), B: (1 - y) * (1 - k), A: 1}
	}
	return nil
}

// readBS reads /BS width, style, and dash from a widget dict. Returns
// (0, BorderSolid, nil) when /BS is absent.
func readBS(w pdfDict) (width float64, style BorderStyle, dash []float64) {
	bs, ok := w["/BS"].(pdfDict)
	if !ok {
		return 0, BorderSolid, nil
	}
	if v, ok := bs["/W"]; ok {
		width, _ = toFloat(v)
	}
	if n, ok := bs["/S"].(pdfName); ok {
		style = borderStyleFromName(string(n))
	}
	if arr, ok := bs["/D"].(pdfArray); ok {
		dash = make([]float64, 0, len(arr))
		for _, v := range arr {
			f, _ := toFloat(v)
			dash = append(dash, f)
		}
	}
	return width, style, dash
}

// ensureFont registers font in /AcroForm/DR/Font and returns its
// resource name (without the leading slash). Reuses an existing resource
// with the same /BaseFont so the DR doesn't accumulate duplicates.
// Standard 14 fonts get a Type1 font dict; embedded fonts reference the
// font object already created by LoadFont. The Go Font is remembered in
// doc.formFonts under the resource name so widget /AP generators can
// render with the exact font, including embedded TTFs.
func (f *Form) ensureFont(font Font) (string, error) {
	if font == nil {
		font = FontHelvetica
	}
	f.ensureRoot()
	dr, _ := f.root["/DR"].(pdfDict)
	if dr == nil {
		dr = pdfDict{}
		f.root["/DR"] = dr
	}
	fonts, _ := dr["/Font"].(pdfDict)
	if fonts == nil {
		fonts = pdfDict{}
		dr["/Font"] = fonts
	}

	base := font.BaseFont()
	// Reuse an existing resource with the same /BaseFont.
	for name, v := range fonts {
		dict, ok := resolveRefToDict(f.doc.objects, v)
		if !ok {
			dict, ok = v.(pdfDict)
		}
		if ok {
			if bf, ok := dict["/BaseFont"].(pdfName); ok && string(bf) == "/"+base {
				resName := strings.TrimPrefix(name, "/")
				f.rememberFormFont(resName, font)
				return resName, nil
			}
		}
	}

	resName := newFontResourceName(font, fonts)
	if ef, ok := font.(*embeddedFont); ok {
		fonts["/"+resName] = pdfRef{Num: ef.fontObjectID}
	} else {
		fontDict := pdfDict{
			"/Type":     pdfName("/Font"),
			"/Subtype":  pdfName("/Type1"),
			"/BaseFont": pdfName("/" + base),
			"/Encoding": pdfName("/WinAnsiEncoding"),
		}
		id := f.doc.nextID
		f.doc.nextID++
		f.doc.objects[id] = &pdfObject{Num: id, Value: fontDict}
		fonts["/"+resName] = pdfRef{Num: id}
	}
	f.rememberFormFont(resName, font)
	return resName, nil
}

// newFontResourceName picks an unused /DR/Font resource name for font.
// Helvetica keeps the conventional "Helv"; others derive an alphanumeric
// name from the PostScript base font, disambiguated with a numeric suffix.
func newFontResourceName(font Font, existing pdfDict) string {
	if font.BaseFont() == "Helvetica" {
		if _, taken := existing["/Helv"]; !taken {
			return "Helv"
		}
	}
	var sb strings.Builder
	for _, r := range font.BaseFont() {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	cand := sb.String()
	if cand == "" {
		cand = "F"
	}
	name := cand
	for i := 1; ; i++ {
		if _, taken := existing["/"+name]; !taken {
			return name
		}
		name = cand + strconv.Itoa(i)
	}
}

// rememberFormFont records the resName→Font mapping on the document so
// widget /AP generators can resolve the exact Go Font later.
func (f *Form) rememberFormFont(resName string, font Font) {
	if f.doc.formFonts == nil {
		f.doc.formFonts = map[string]Font{}
	}
	f.doc.formFonts[resName] = font
}

// resolveWidgetFont returns the Go Font for a /DA font resource name.
// Prefers the in-session doc.formFonts registry (covers embedded TTFs);
// falls back to reconstructing a Standard 14 font from
// /AcroForm/DR/Font/<resName>/BaseFont; defaults to Helvetica.
func resolveWidgetFont(form *Form, resName string) Font {
	if form == nil || form.doc == nil || resName == "" {
		return FontHelvetica
	}
	if ft, ok := form.doc.formFonts[resName]; ok && ft != nil {
		return ft
	}
	if form.root != nil {
		if dr, ok := form.root["/DR"].(pdfDict); ok {
			if fonts, ok := dr["/Font"].(pdfDict); ok {
				if v, ok := fonts["/"+resName]; ok {
					if d, ok := resolveRefToDict(form.doc.objects, v); ok {
						if bf, ok := d["/BaseFont"].(pdfName); ok {
							if ft, err := FindFont(strings.TrimPrefix(string(bf), "/")); err == nil {
								return ft
							}
						}
					}
				}
			}
		}
	}
	return FontHelvetica
}
