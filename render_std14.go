// SPDX-License-Identifier: MIT

package asposepdf

import (
	"embed"
	"strings"
	"sync"
)

// PDF references the "Standard 14" fonts (Helvetica/Times/Courier families,
// plus Symbol and ZapfDingbats) by name only and ships no outlines, so a
// renderer must supply substitute glyph shapes. We bundle metric-compatible
// open families so positioning and letterforms both match:
//
//	Helvetica/Arial → Arimo, Times → Tinos, Courier → Cousine
//
// These (SIL OFL 1.1, see fonts/LICENSE.txt) have the same advance widths as
// the fonts they replace, so word-wrapped layout is preserved and narrow
// glyphs aren't distorted.
// Symbol/ZapfDingbats have no metric-compatible free substitute and are not
// bundled: fallbackFontFor returns nil for them. ZapfDingbats instead gets
// synthesized outlines for its common marks (see render_dingbats.go), chiefly
// so checkbox/radio widget appearances render; Symbol still draws nothing.
//
//go:embed fonts/Arimo-Regular.ttf fonts/Arimo-Bold.ttf fonts/Arimo-Italic.ttf fonts/Arimo-BoldItalic.ttf
//go:embed fonts/Tinos-Regular.ttf fonts/Tinos-Bold.ttf fonts/Tinos-Italic.ttf fonts/Tinos-BoldItalic.ttf
//go:embed fonts/Cousine-Regular.ttf fonts/Cousine-Bold.ttf fonts/Cousine-Italic.ttf fonts/Cousine-BoldItalic.ttf
var stdFontsFS embed.FS

var (
	stdFontMu    sync.Mutex
	stdFontCache = map[string]*ttfFont{}
)

// loadStdFont parses and caches a bundled substitute font by file name.
func loadStdFont(file string) *ttfFont {
	stdFontMu.Lock()
	defer stdFontMu.Unlock()
	if f, ok := stdFontCache[file]; ok {
		return f
	}
	var parsed *ttfFont
	if data, err := stdFontsFS.ReadFile("fonts/" + file); err == nil {
		if f, err := parseTTF(data); err == nil {
			parsed = f
		}
	}
	stdFontCache[file] = parsed
	return parsed
}

// fallbackFontFor picks a metric-compatible substitute for a non-embedded font,
// choosing the family from the base font name and the style from the resolved
// bold/italic flags (and the name, as a backstop).
func fallbackFontFor(fi fontInfo) *ttfFont {
	name := strings.ToLower(fi.name)

	// Symbol / ZapfDingbats have no Latin metric-compatible substitute; their
	// code→Unicode encoding is wired (see defaultEncodingForFont), so an embedded
	// copy or a FontRepository-registered covering font renders, but the bundled
	// Latin fonts would only draw .notdef boxes — render nothing instead.
	if strings.Contains(name, "symbol") || strings.Contains(name, "dingbat") {
		return nil
	}

	family := "Arimo" // Helvetica / Arial / sans / default
	switch {
	case strings.Contains(name, "courier") || strings.Contains(name, "mono") || strings.Contains(name, "consol"):
		family = "Cousine"
	case strings.Contains(name, "times") || strings.Contains(name, "serif") || strings.Contains(name, "georgia") || strings.Contains(name, "roman"):
		family = "Tinos"
	case fi.serif:
		// The /FontDescriptor marks this a serif face (e.g. Garamond, Minion)
		// with no Times/serif keyword in the name — use the serif substitute so
		// its proportions and look match better than the sans default.
		family = "Tinos"
	}

	bold := fi.bold || strings.Contains(name, "bold")
	italic := fi.italic || strings.Contains(name, "italic") || strings.Contains(name, "oblique")
	style := "Regular"
	switch {
	case bold && italic:
		style = "BoldItalic"
	case bold:
		style = "Bold"
	case italic:
		style = "Italic"
	}

	if f := loadStdFont(family + "-" + style + ".ttf"); f != nil {
		return f
	}
	return loadStdFont(family + "-Regular.ttf")
}
