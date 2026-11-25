// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// Position is where a text fragment's (or segment's) baseline starts: an
// absolute point on the page in user space, in points from the page's
// bottom-left corner, not an offset. Mirrors Aspose.PDF for .NET's Position.
//
// The zero value means unset (the default place: the CropBox's top-left
// corner less 72pt), so the page origin itself cannot be targeted; set a tiny
// non-zero value, such as XIndent 0.001, to place text at (0, 0).
type Position struct{ XIndent, YIndent float64 }

// FontStyle is a bit set of font style flags. Mirrors Aspose.PDF for .NET's FontStyles.
type FontStyle int

const (
	FontStyleRegular FontStyle = 0
	FontStyleBold    FontStyle = 1
	FontStyleItalic  FontStyle = 2
)

// TextRenderingMode is the PDF text rendering mode (Tr 0-3).
type TextRenderingMode int

const (
	RenderingFill TextRenderingMode = iota
	RenderingStroke
	RenderingFillStroke
	RenderingInvisible
)

// TextState describes the look of a piece of text. Zero fields mean
// "unspecified" and are inherited from the enclosing level. Mirrors Aspose.PDF
// for .NET's TextState.
type TextState struct {
	Font                                             Font
	FontSize                                         float64
	FontStyle                                        FontStyle
	ForegroundColor, BackgroundColor, StrokingColor  *Color
	RenderingMode                                    TextRenderingMode
	Underline, StrikeOut, Invisible                  bool
	CharacterSpacing, WordSpacing, HorizontalScaling float64
	LineSpacing, Rotation                            float64

	// fakedBold marks the state of found text drawn with fill and stroke
	// over a regular face to look bold: the stroke is part of FontStyleBold,
	// so the text is redrawn as bold (a bold face, or the emulation in
	// style), not stroked in its old stroking colour.
	fakedBold bool
}

// inherit fills s's zero fields from parent (bools are ORed).
func (s TextState) inherit(parent TextState) TextState {
	if s.Font == nil {
		s.Font = parent.Font
	}
	if s.FontSize == 0 {
		s.FontSize = parent.FontSize
	}
	if s.FontStyle == 0 {
		s.FontStyle = parent.FontStyle
	}
	if s.ForegroundColor == nil {
		s.ForegroundColor = parent.ForegroundColor
	}
	if s.BackgroundColor == nil {
		s.BackgroundColor = parent.BackgroundColor
	}
	if s.StrokingColor == nil {
		s.StrokingColor = parent.StrokingColor
	}
	if s.RenderingMode == 0 {
		s.RenderingMode = parent.RenderingMode
	}
	if s.CharacterSpacing == 0 {
		s.CharacterSpacing = parent.CharacterSpacing
	}
	if s.WordSpacing == 0 {
		s.WordSpacing = parent.WordSpacing
	}
	if s.HorizontalScaling == 0 {
		s.HorizontalScaling = parent.HorizontalScaling
	}
	if s.LineSpacing == 0 {
		s.LineSpacing = parent.LineSpacing
	}
	if s.Rotation == 0 {
		s.Rotation = parent.Rotation
	}
	s.Underline = s.Underline || parent.Underline
	s.StrikeOut = s.StrikeOut || parent.StrikeOut
	s.Invisible = s.Invisible || parent.Invisible
	s.fakedBold = s.fakedBold || parent.fakedBold
	return s
}

// resolved returns s with unspecified fields taken from the defaults
// (Helvetica 12, black, 100% scaling).
func (s TextState) resolved() TextState {
	return s.inherit(TextState{
		Font:              FontHelvetica,
		FontSize:          12,
		ForegroundColor:   &Color{0, 0, 0, 1},
		HorizontalScaling: 100,
	})
}

// ApplyChangesFrom copies o's non-zero fields into s.
func (s *TextState) ApplyChangesFrom(o TextState) {
	*s = o.inherit(*s)
}

// style maps the state to a TextStyle.
func (s TextState) style() TextStyle {
	st := TextStyle{
		Font:          styledFont(drawableFont(s.Font, s.FontStyle), s.FontStyle),
		Size:          s.FontSize,
		Color:         s.ForegroundColor,
		Background:    s.BackgroundColor,
		Underline:     s.Underline,
		Strikethrough: s.StrikeOut,
		Invisible:     s.Invisible || s.RenderingMode == RenderingInvisible,
		Rotation:      s.Rotation,

		CharSpacing:       s.CharacterSpacing,
		WordSpacing:       s.WordSpacing,
		HorizontalScaling: s.HorizontalScaling,
		StrokeColor:       s.StrokingColor,
		RenderMode:        s.RenderingMode,
	}
	if s.LineSpacing > 0 && s.FontSize > 0 {
		st.LineSpacing = s.LineSpacing / s.FontSize
	}
	if s.fakedBold && st.RenderMode == RenderingFillStroke {
		st.RenderMode, st.StrokeColor = RenderingFill, nil
	}
	// A style the document's font lacks is emulated: bold as fill and
	// stroke, italic as a slant, so the glyphs keep their widths.
	if pf, ok := s.Font.(*pageFont); ok {
		bold, italic, _ := pf.emulatedStyle(s.FontStyle)
		if bold && st.RenderMode == RenderingFill && !st.Invisible {
			st.RenderMode = RenderingFillStroke
			if st.StrokeColor == nil {
				st.StrokeColor = s.ForegroundColor
			}
			st.strokeWidth = s.FontSize / 30
		}
		if italic {
			st.Skew = 12
		}
	}
	return st
}

// styledFont selects the bold/italic Standard-14 face for a Standard-14 font.
// drawableFont is f, except that a font found on a page, carried by a
// fragment copied off it (CloneWithSegments) or set from one, is drawn in the
// closest Standard-14 face of its family and style, as an edit would be.
func drawableFont(f Font, fs FontStyle) Font {
	if df, ok := f.(documentFont); ok {
		return standard14ForFragment(df.name, fs&FontStyleBold != 0, fs&FontStyleItalic != 0)
	}
	return f
}

func styledFont(f Font, fs FontStyle) Font {
	if f == nil || fs == FontStyleRegular || f.IsEmbedded() {
		return f
	}
	if _, ok := f.(*pageFont); ok {
		return f // a style it lacks is emulated (TextState.style)
	}
	bold, italic := fs&FontStyleBold != 0, fs&FontStyleItalic != 0
	name := f.BaseFont()
	var cand string
	switch {
	case strings.HasPrefix(name, "Times"):
		switch {
		case bold && italic:
			cand = "Times-BoldItalic"
		case bold:
			cand = "Times-Bold"
		case italic:
			cand = "Times-Italic"
		}
	case strings.HasPrefix(name, "Helvetica"), strings.HasPrefix(name, "Courier"):
		fam := "Helvetica"
		if strings.HasPrefix(name, "Courier") {
			fam = "Courier"
		}
		switch {
		case bold && italic:
			cand = fam + "-BoldOblique"
		case bold:
			cand = fam + "-Bold"
		case italic:
			cand = fam + "-Oblique"
		}
	}
	if cand == "" {
		return f
	}
	if g, err := FindFont(cand); err == nil {
		return g
	}
	return f
}

// spacedWidth applies character spacing tc (per rune), word spacing tw (per
// U+0020) and horizontal scaling tz (percent; 0 means 100) to a base width.
func spacedWidth(base float64, text string, tc, tw, tz float64) float64 {
	if tc == 0 && tw == 0 && (tz == 0 || tz == 100) {
		return base
	}
	runes, spaces := 0, 0
	for _, r := range text {
		runes++
		if r == ' ' {
			spaces++
		}
	}
	if tz == 0 {
		tz = 100
	}
	return (base + tc*float64(runes) + tw*float64(spaces)) * tz / 100
}

// MeasureString returns the advance width of text in this state.
func (s TextState) MeasureString(text string) (float64, error) {
	r := s.resolved()
	return MeasureText(text, r.style())
}
