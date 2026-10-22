// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// pdf-go-eys1: filterTjStringType0/filterTJArray's Type0 branch used to
// always consume Tj/TJ string operands 2 bytes at a time, which is only
// correct for Identity-H/V. A predefined CJK CMap (e.g. 90ms-RKSJ-H) or an
// embedded, non-Identity /Encoding CMap stream can have a VARIABLE-width
// codespace mixing 1- and multi-byte codes — font.go's resolveType0Encoding
// already models this via cidCMap, and the text extractor/renderer already
// decode through it; this redaction rewriter was the one place that still
// assumed a fixed stride regardless, which misdecodes glyph boundaries
// (and, independently, looked widths up by the raw code instead of the
// resolved CID) for any such font.

// testRewriteCJKFontMap builds a minimal Type0 font with a MIXED-width
// codespace (1-byte codes 0x00-0x7F, 2-byte codes 0x8100-0xFEFF — the same
// shape a real Shift-JIS-based CMap like 90ms-RKSJ-H has) and two mapped
// codes: the single byte 0x41 -> CID 65 (width 600/1000 em), and the two
// bytes 0x81 0x40 -> CID 500 (width 800/1000 em). An unmapped code falls
// back to cidForCode's .notdef (CID 0).
func testRewriteCJKFontMap() map[string]fontInfo {
	cm := &cidCMap{
		codespace: []cmapCodespace{
			{nbytes: 1, low: [4]byte{0x00}, high: [4]byte{0x7F}},
			{nbytes: 2, low: [4]byte{0x81, 0x00}, high: [4]byte{0xFE, 0xFF}},
		},
		singles: map[uint32]uint16{
			0x41:   65,
			0x8140: 500,
		},
	}
	cm.finalize()
	fi := fontInfo{
		name:     "/CJKFont",
		known:    true,
		isType0:  true,
		cidCMap:  cm,
		defaultW: 1000,
		cidWidths: map[uint16]float64{
			65:  600,
			500: 800,
		},
	}
	return map[string]fontInfo{"/F1": fi}
}

// buildCJKContent wraps raw (already-encoded Type0) bytes in a minimal
// BT...Tj...ET content stream, using "s" as the literal-string body so
// callers can pass raw, non-ASCII code bytes directly — none of the bytes
// this test uses (0x41, 0x81, 0x40) are PDF string delimiters, so no
// escaping is needed.
func buildCJKContent(raw []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("BT\n/F1 10 Tf\n0 0 Td\n(")
	buf.Write(raw)
	buf.WriteString(") Tj\nET\n")
	return buf.Bytes()
}

// TestRewriteTextType0MixedWidthDecodesCorrectGlyphBoundaries is the core
// reproduction: a 3-byte Type0 string containing one 1-byte glyph (code
// 0x41, CID 65, 6.0 text-space units wide at this 10pt font) followed by
// one 2-byte glyph (code 0x8140, CID 500, 8.0 units wide). Decoded
// correctly, the two glyphs' centers land at local x=3.0 and x=10.0. A
// region placed ONLY around the second glyph's real center must drop
// exactly that glyph's two bytes and leave the first glyph's one byte
// untouched — the fixed-2-byte-stride bug instead misreads byte 0 and 1
// together as one bogus code (0x4181, unmapped, falling back to the
// default 1000-unit width) centered at local x=5.0 — OUTSIDE this
// region — and never even looks at byte 2 at all (the stride loop's
// bound, i+1<len(s), stops after one malformed "glyph"), so the fixed
// code reports nothing dropped and returns the original string
// completely unredacted.
func TestRewriteTextType0MixedWidthDecodesCorrectGlyphBoundaries(t *testing.T) {
	raw := []byte{0x41, 0x81, 0x40}
	in := buildCJKContent(raw)
	regions := []QuadPoint{{X1: 8, Y1: 2, X2: 12, Y2: 2, X3: 8, Y3: -2, X4: 12, Y4: -2}}

	out, err := rewriteTextOperatorsInStream(in, regions, testRewriteCJKFontMap(), nil, nil, nil)
	if err != nil {
		t.Fatalf("rewriteTextOperatorsInStream: %v", err)
	}

	if bytes.Contains(out, []byte{0x81, 0x40}) {
		t.Errorf("the 2-byte glyph's raw bytes (the one actually inside the region) survived:\n%s", out)
	}
	if !bytes.Contains(out, []byte{0x41}) {
		t.Errorf("the 1-byte glyph's raw byte (outside the region) did not survive:\n%s", out)
	}
}

// TestRewriteTextType0MixedWidthDropsFirstGlyphOnly is the complementary
// case: a region placed around the FIRST (1-byte) glyph's real center
// (local x=3.0) only. Decoded correctly, only the 1-byte code 0x41 is
// dropped and the 2-byte glyph survives intact — the fixed-stride bug
// instead centers its one bogus glyph at local x=5.0, which this region
// does NOT cover, so it would report nothing dropped here too (the
// opposite failure direction from the test above: a secret in the FIRST
// glyph would survive instead).
func TestRewriteTextType0MixedWidthDropsFirstGlyphOnly(t *testing.T) {
	raw := []byte{0x41, 0x81, 0x40}
	in := buildCJKContent(raw)
	regions := []QuadPoint{{X1: 1, Y1: 2, X2: 5, Y2: 2, X3: 1, Y3: -2, X4: 5, Y4: -2}}

	out, err := rewriteTextOperatorsInStream(in, regions, testRewriteCJKFontMap(), nil, nil, nil)
	if err != nil {
		t.Fatalf("rewriteTextOperatorsInStream: %v", err)
	}

	if bytes.Contains(out, []byte{0x41}) {
		t.Errorf("the 1-byte glyph's raw byte (inside the region) survived:\n%s", out)
	}
	if !bytes.Contains(out, []byte{0x81, 0x40}) {
		t.Errorf("the 2-byte glyph's raw bytes (outside the region) did not survive:\n%s", out)
	}
}

// TestRewriteTJArrayType0MixedWidth is the TJ-array counterpart: the same
// two glyphs, but split across TWO string elements of one TJ array with a
// kerning number between them (a realistic shape — font with nonzero
// cross-glyph kerning) — exercising filterTJArray's own Type0 branch
// rather than filterTjStringType0's.
func TestRewriteTJArrayType0MixedWidth(t *testing.T) {
	in := []byte("BT\n/F1 10 Tf\n0 0 Td\n[(\x41) -50 (\x81\x40)] TJ\nET\n")
	// The -50 kerning number shifts the second glyph by
	// -(-50)/1000*10 = 0.5 units right of where it'd otherwise sit — a
	// small, deliberate perturbation so the test isn't accidentally
	// insensitive to whether kerning is tracked at all. Center of the
	// second glyph becomes 6.0+0.5+4.0 = 10.5; region below covers it.
	regions := []QuadPoint{{X1: 9, Y1: 2, X2: 12, Y2: 2, X3: 9, Y3: -2, X4: 12, Y4: -2}}

	out, err := rewriteTextOperatorsInStream(in, regions, testRewriteCJKFontMap(), nil, nil, nil)
	if err != nil {
		t.Fatalf("rewriteTextOperatorsInStream: %v", err)
	}
	if bytes.Contains(out, []byte{0x81, 0x40}) {
		t.Errorf("the 2-byte glyph's raw bytes (inside the region) survived:\n%s", out)
	}
	if !bytes.Contains(out, []byte{0x41}) {
		t.Errorf("the 1-byte glyph's raw byte (outside the region) did not survive:\n%s", out)
	}
}

// TestGlyphAdvanceCIDWidthKeyedByCID confirms glyphAdvanceCID looks width
// up by the RESOLVED CID, not the raw code — independent of pdf-go-eys1's
// byte-boundary bug, this was already wrong for any font where a code
// differs from its CID. CID 65 here is deliberately given a width that
// differs from what a WRONG code-keyed lookup (code 0x999 does not exist
// in cidWidths, so it would silently fall back to defaultW) would produce.
func TestGlyphAdvanceCIDWidthKeyedByCID(t *testing.T) {
	fi := fontInfo{
		defaultW:  1000,
		cidWidths: map[uint16]float64{65: 600},
	}
	// Call with a CID (65) that differs from a plausible "raw code" this
	// glyph might have had (e.g. 0x999, which has no entry): a
	// code-keyed lookup would wrongly fall back to defaultW (1000);
	// a CID-keyed one correctly finds 600.
	got := glyphAdvanceCID(&fi, 65, 2, 10, 0, 0, 1)
	want := 600.0 / 1000.0 * 10
	if got != want {
		t.Errorf("glyphAdvanceCID(cid=65) = %v, want %v (width should be keyed by CID)", got, want)
	}
}

// TestGlyphAdvanceCIDWordSpacingOnlyForSingleByteCode32 pins the
// pdf-go-ifpy point 2 fix that falls out of the same nbytes-aware rewrite:
// ISO 32000-1 §9.3.3 restricts Tw to the single-byte code 32, never a
// multi-byte one, even if a multi-byte code happens to resolve to CID 32.
func TestGlyphAdvanceCIDWordSpacingOnlyForSingleByteCode32(t *testing.T) {
	fi := fontInfo{defaultW: 1000, cidWidths: map[uint16]float64{32: 1000}}
	withTw := glyphAdvanceCID(&fi, 32, 1, 10, 0, 5, 1)
	withoutTw := glyphAdvanceCID(&fi, 32, 1, 10, 0, 0, 1)
	if withTw-withoutTw != 5 {
		t.Errorf("1-byte code 32 (CID 32): Tw not applied (delta %v, want 5)", withTw-withoutTw)
	}
	twoByte := glyphAdvanceCID(&fi, 32, 2, 10, 0, 5, 1)
	twoByteNoTw := glyphAdvanceCID(&fi, 32, 2, 10, 0, 0, 1)
	if twoByte != twoByteNoTw {
		t.Errorf("2-byte code resolving to CID 32: Tw wrongly applied (%v vs %v)", twoByte, twoByteNoTw)
	}
}
