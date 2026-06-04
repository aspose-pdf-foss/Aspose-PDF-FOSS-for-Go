// SPDX-License-Identifier: MIT

package asposepdf

import (
	_ "embed"
	"sync"
)

// fallbackFontData is the glyph-outline font bundled for rendering text in
// non-embedded fonts (the Standard 14: Helvetica/Times/Courier/…). PDF only
// references these fonts by name and ships no outlines, so a renderer must
// supply substitute shapes. We reuse the DejaVu Sans program already in the
// repository: text is positioned with the font's real Standard-14 AFM metrics
// (resolved into fontInfo.widths) and only the glyph *shapes* come from this
// fallback, so layout is correct while letterforms are an approximation.
//
// Follow-up: bundle metric-compatible families (Liberation/Nimbus) and serif/
// mono variants for exact Helvetica/Times/Courier shapes. DejaVu Sans is
// distributed under the Bitstream Vera / DejaVu license (permissive); binary
// redistributions should include that license text.
//
//go:embed testdata/DejaVuSans.ttf
var fallbackFontData []byte

var (
	fallbackOnce   sync.Once
	fallbackParsed *ttfFont
)

// fallbackFont returns the parsed bundled fallback font (nil if it failed to
// parse, in which case non-embedded text is skipped).
func fallbackFont() *ttfFont {
	fallbackOnce.Do(func() {
		if f, err := parseTTF(fallbackFontData); err == nil {
			fallbackParsed = f
		}
	})
	return fallbackParsed
}
