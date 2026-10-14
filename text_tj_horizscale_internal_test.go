// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// TestExtractorTJKerningHonorsHorizScale pins a bug found by an
// independent review while checking the redaction text rewriter's own
// (already-fixed) analogous bug for consistency: textExtractor's own
// numeric-TJ-kerning handling (showTJ, the "case int"/"case float64"
// branches) computed the displacement as -N/1000*fontSize, omitting the
// horizScaling (Tz/100) factor ISO 32000-1 §9.4.4 applies to every term
// of tx — including this one — exactly like advanceGlyph/advanceGlyphCID
// already do for ordinary glyph advances just below it in the same
// function. This is the EXTRACTION pipeline, not just redaction: it
// drives SearchText, ExtractTextWithLayout, and therefore the documented
// "build a RedactAnnotation from a SearchText match" workflow itself, so
// an unscaled kerning read could report the WRONG position for any text
// after a kerning entry whenever Tz != 100 — independent of whether
// ApplyRedactions is ever even called.
func TestExtractorTJKerningHonorsHorizScale(t *testing.T) {
	fonts := testRewriteFontMap() // /F1, constant 500/1000-em width
	content := []byte("BT\n/F1 12 Tf\n50 Tz\n100 700 Td\n[(A) -1000 (SECRET)] TJ\nET\n")
	ops, err := parseContentStream(content)
	if err != nil {
		t.Fatal(err)
	}

	e := newTextExtractor(nil, fonts)
	e.process(ops, nil)

	// "A" advances by (500/1000*12)*0.5 = 3, landing at x=103. The
	// kerning entry -1000 then adds -(-1000)/1000*12*0.5 = 6 MORE
	// (correctly horizScale-scaled) before "SECRET" starts at x=109;
	// each of its 6 glyphs advances by 3 more, ending at x=127. Unscaled
	// (the bug), the kerning term alone would be 12 instead of 6,
	// shifting every one of these positions — and this final one — by
	// exactly that 6-unit difference, ending at x=133 instead.
	const wantX = 127.0
	if math.Abs(e.lastX-wantX) > 1e-9 {
		t.Errorf("final x position = %v, want %v (horizScale not applied to TJ numeric kerning?)", e.lastX, wantX)
	}
}
