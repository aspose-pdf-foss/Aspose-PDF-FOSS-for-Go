// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// Position is an offset of a text fragment from its default place, in points.
// Mirrors Aspose.PDF for .NET's Position.
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
		Font:          styledFont(s.Font, s.FontStyle),
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
	return st
}

// styledFont selects the bold/italic Standard-14 face for a Standard-14 font.
func styledFont(f Font, fs FontStyle) Font {
	if f == nil || fs == FontStyleRegular || f.IsEmbedded() {
		return f
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
