// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"sync"
)

// Unicode canonical combining classes, and the canonical ordering they
// drive (UAX #15, D108). A shaper must see combining marks in canonical
// order or it attaches them in the order they were typed: a Hebrew word
// spelled with a dagesh before a sheva, or an Arabic letter carrying a
// shadda before a fatha, would otherwise stack its marks the wrong way up
// and disagree with every other text engine. This is the one piece of
// normalization the shaping path needs — full decomposition/recomposition
// is deliberately not done (see shape.go).
//
// Generated from the Unicode Character Database 16.0.0: every
// range of code points sharing one non-zero combining class.

type cccRange struct {
	lo, hi rune
	class  uint8
}

var cccRanges = []cccRange{
	{0x0300, 0x0314, 230},
	{0x0315, 0x0315, 232},
	{0x0316, 0x0319, 220},
	{0x031A, 0x031A, 232},
	{0x031B, 0x031B, 216},
	{0x031C, 0x0320, 220},
	{0x0321, 0x0322, 202},
	{0x0323, 0x0326, 220},
	{0x0327, 0x0328, 202},
	{0x0329, 0x0333, 220},
	{0x0334, 0x0338, 1},
	{0x0339, 0x033C, 220},
	{0x033D, 0x0344, 230},
	{0x0345, 0x0345, 240},
	{0x0346, 0x0346, 230},
	{0x0347, 0x0349, 220},
	{0x034A, 0x034C, 230},
	{0x034D, 0x034E, 220},
	{0x0350, 0x0352, 230},
	{0x0353, 0x0356, 220},
	{0x0357, 0x0357, 230},
	{0x0358, 0x0358, 232},
	{0x0359, 0x035A, 220},
	{0x035B, 0x035B, 230},
	{0x035C, 0x035C, 233},
	{0x035D, 0x035E, 234},
	{0x035F, 0x035F, 233},
	{0x0360, 0x0361, 234},
	{0x0362, 0x0362, 233},
	{0x0363, 0x036F, 230},
	{0x0483, 0x0487, 230},
	{0x0591, 0x0591, 220},
	{0x0592, 0x0595, 230},
	{0x0596, 0x0596, 220},
	{0x0597, 0x0599, 230},
	{0x059A, 0x059A, 222},
	{0x059B, 0x059B, 220},
	{0x059C, 0x05A1, 230},
	{0x05A2, 0x05A7, 220},
	{0x05A8, 0x05A9, 230},
	{0x05AA, 0x05AA, 220},
	{0x05AB, 0x05AC, 230},
	{0x05AD, 0x05AD, 222},
	{0x05AE, 0x05AE, 228},
	{0x05AF, 0x05AF, 230},
	{0x05B0, 0x05B0, 10},
	{0x05B1, 0x05B1, 11},
	{0x05B2, 0x05B2, 12},
	{0x05B3, 0x05B3, 13},
	{0x05B4, 0x05B4, 14},
	{0x05B5, 0x05B5, 15},
	{0x05B6, 0x05B6, 16},
	{0x05B7, 0x05B7, 17},
	{0x05B8, 0x05B8, 18},
	{0x05B9, 0x05BA, 19},
	{0x05BB, 0x05BB, 20},
	{0x05BC, 0x05BC, 21},
	{0x05BD, 0x05BD, 22},
	{0x05BF, 0x05BF, 23},
	{0x05C1, 0x05C1, 24},
	{0x05C2, 0x05C2, 25},
	{0x05C4, 0x05C4, 230},
	{0x05C5, 0x05C5, 220},
	{0x05C7, 0x05C7, 18},
	{0x0610, 0x0617, 230},
	{0x0618, 0x0618, 30},
	{0x0619, 0x0619, 31},
	{0x061A, 0x061A, 32},
	{0x064B, 0x064B, 27},
	{0x064C, 0x064C, 28},
	{0x064D, 0x064D, 29},
	{0x064E, 0x064E, 30},
	{0x064F, 0x064F, 31},
	{0x0650, 0x0650, 32},
	{0x0651, 0x0651, 33},
	{0x0652, 0x0652, 34},
	{0x0653, 0x0654, 230},
	{0x0655, 0x0656, 220},
	{0x0657, 0x065B, 230},
	{0x065C, 0x065C, 220},
	{0x065D, 0x065E, 230},
	{0x065F, 0x065F, 220},
	{0x0670, 0x0670, 35},
	{0x06D6, 0x06DC, 230},
	{0x06DF, 0x06E2, 230},
	{0x06E3, 0x06E3, 220},
	{0x06E4, 0x06E4, 230},
	{0x06E7, 0x06E8, 230},
	{0x06EA, 0x06EA, 220},
	{0x06EB, 0x06EC, 230},
	{0x06ED, 0x06ED, 220},
	{0x0711, 0x0711, 36},
	{0x0730, 0x0730, 230},
	{0x0731, 0x0731, 220},
	{0x0732, 0x0733, 230},
	{0x0734, 0x0734, 220},
	{0x0735, 0x0736, 230},
	{0x0737, 0x0739, 220},
	{0x073A, 0x073A, 230},
	{0x073B, 0x073C, 220},
	{0x073D, 0x073D, 230},
	{0x073E, 0x073E, 220},
	{0x073F, 0x0741, 230},
	{0x0742, 0x0742, 220},
	{0x0743, 0x0743, 230},
	{0x0744, 0x0744, 220},
	{0x0745, 0x0745, 230},
	{0x0746, 0x0746, 220},
	{0x0747, 0x0747, 230},
	{0x0748, 0x0748, 220},
	{0x0749, 0x074A, 230},
	{0x07EB, 0x07F1, 230},
	{0x07F2, 0x07F2, 220},
	{0x07F3, 0x07F3, 230},
	{0x07FD, 0x07FD, 220},
	{0x0816, 0x0819, 230},
	{0x081B, 0x0823, 230},
	{0x0825, 0x0827, 230},
	{0x0829, 0x082D, 230},
	{0x0859, 0x085B, 220},
	{0x0897, 0x0898, 230},
	{0x0899, 0x089B, 220},
	{0x089C, 0x089F, 230},
	{0x08CA, 0x08CE, 230},
	{0x08CF, 0x08D3, 220},
	{0x08D4, 0x08E1, 230},
	{0x08E3, 0x08E3, 220},
	{0x08E4, 0x08E5, 230},
	{0x08E6, 0x08E6, 220},
	{0x08E7, 0x08E8, 230},
	{0x08E9, 0x08E9, 220},
	{0x08EA, 0x08EC, 230},
	{0x08ED, 0x08EF, 220},
	{0x08F0, 0x08F0, 27},
	{0x08F1, 0x08F1, 28},
	{0x08F2, 0x08F2, 29},
	{0x08F3, 0x08F5, 230},
	{0x08F6, 0x08F6, 220},
	{0x08F7, 0x08F8, 230},
	{0x08F9, 0x08FA, 220},
	{0x08FB, 0x08FF, 230},
	{0x093C, 0x093C, 7},
	{0x094D, 0x094D, 9},
	{0x0951, 0x0951, 230},
	{0x0952, 0x0952, 220},
	{0x0953, 0x0954, 230},
	{0x09BC, 0x09BC, 7},
	{0x09CD, 0x09CD, 9},
	{0x09FE, 0x09FE, 230},
	{0x0A3C, 0x0A3C, 7},
	{0x0A4D, 0x0A4D, 9},
	{0x0ABC, 0x0ABC, 7},
	{0x0ACD, 0x0ACD, 9},
	{0x0B3C, 0x0B3C, 7},
	{0x0B4D, 0x0B4D, 9},
	{0x0BCD, 0x0BCD, 9},
	{0x0C3C, 0x0C3C, 7},
	{0x0C4D, 0x0C4D, 9},
	{0x0C55, 0x0C55, 84},
	{0x0C56, 0x0C56, 91},
	{0x0CBC, 0x0CBC, 7},
	{0x0CCD, 0x0CCD, 9},
	{0x0D3B, 0x0D3C, 9},
	{0x0D4D, 0x0D4D, 9},
	{0x0DCA, 0x0DCA, 9},
	{0x0E38, 0x0E39, 103},
	{0x0E3A, 0x0E3A, 9},
	{0x0E48, 0x0E4B, 107},
	{0x0EB8, 0x0EB9, 118},
	{0x0EBA, 0x0EBA, 9},
	{0x0EC8, 0x0ECB, 122},
	{0x0F18, 0x0F19, 220},
	{0x0F35, 0x0F35, 220},
	{0x0F37, 0x0F37, 220},
	{0x0F39, 0x0F39, 216},
	{0x0F71, 0x0F71, 129},
	{0x0F72, 0x0F72, 130},
	{0x0F74, 0x0F74, 132},
	{0x0F7A, 0x0F7D, 130},
	{0x0F80, 0x0F80, 130},
	{0x0F82, 0x0F83, 230},
	{0x0F84, 0x0F84, 9},
	{0x0F86, 0x0F87, 230},
	{0x0FC6, 0x0FC6, 220},
	{0x1037, 0x1037, 7},
	{0x1039, 0x103A, 9},
	{0x108D, 0x108D, 220},
	{0x135D, 0x135F, 230},
	{0x1714, 0x1715, 9},
	{0x1734, 0x1734, 9},
	{0x17D2, 0x17D2, 9},
	{0x17DD, 0x17DD, 230},
	{0x18A9, 0x18A9, 228},
	{0x1939, 0x1939, 222},
	{0x193A, 0x193A, 230},
	{0x193B, 0x193B, 220},
	{0x1A17, 0x1A17, 230},
	{0x1A18, 0x1A18, 220},
	{0x1A60, 0x1A60, 9},
	{0x1A75, 0x1A7C, 230},
	{0x1A7F, 0x1A7F, 220},
	{0x1AB0, 0x1AB4, 230},
	{0x1AB5, 0x1ABA, 220},
	{0x1ABB, 0x1ABC, 230},
	{0x1ABD, 0x1ABD, 220},
	{0x1ABF, 0x1AC0, 220},
	{0x1AC1, 0x1AC2, 230},
	{0x1AC3, 0x1AC4, 220},
	{0x1AC5, 0x1AC9, 230},
	{0x1ACA, 0x1ACA, 220},
	{0x1ACB, 0x1ACE, 230},
	{0x1B34, 0x1B34, 7},
	{0x1B44, 0x1B44, 9},
	{0x1B6B, 0x1B6B, 230},
	{0x1B6C, 0x1B6C, 220},
	{0x1B6D, 0x1B73, 230},
	{0x1BAA, 0x1BAB, 9},
	{0x1BE6, 0x1BE6, 7},
	{0x1BF2, 0x1BF3, 9},
	{0x1C37, 0x1C37, 7},
	{0x1CD0, 0x1CD2, 230},
	{0x1CD4, 0x1CD4, 1},
	{0x1CD5, 0x1CD9, 220},
	{0x1CDA, 0x1CDB, 230},
	{0x1CDC, 0x1CDF, 220},
	{0x1CE0, 0x1CE0, 230},
	{0x1CE2, 0x1CE8, 1},
	{0x1CED, 0x1CED, 220},
	{0x1CF4, 0x1CF4, 230},
	{0x1CF8, 0x1CF9, 230},
	{0x1DC0, 0x1DC1, 230},
	{0x1DC2, 0x1DC2, 220},
	{0x1DC3, 0x1DC9, 230},
	{0x1DCA, 0x1DCA, 220},
	{0x1DCB, 0x1DCC, 230},
	{0x1DCD, 0x1DCD, 234},
	{0x1DCE, 0x1DCE, 214},
	{0x1DCF, 0x1DCF, 220},
	{0x1DD0, 0x1DD0, 202},
	{0x1DD1, 0x1DF5, 230},
	{0x1DF6, 0x1DF6, 232},
	{0x1DF7, 0x1DF8, 228},
	{0x1DF9, 0x1DF9, 220},
	{0x1DFA, 0x1DFA, 218},
	{0x1DFB, 0x1DFB, 230},
	{0x1DFC, 0x1DFC, 233},
	{0x1DFD, 0x1DFD, 220},
	{0x1DFE, 0x1DFE, 230},
	{0x1DFF, 0x1DFF, 220},
	{0x20D0, 0x20D1, 230},
	{0x20D2, 0x20D3, 1},
	{0x20D4, 0x20D7, 230},
	{0x20D8, 0x20DA, 1},
	{0x20DB, 0x20DC, 230},
	{0x20E1, 0x20E1, 230},
	{0x20E5, 0x20E6, 1},
	{0x20E7, 0x20E7, 230},
	{0x20E8, 0x20E8, 220},
	{0x20E9, 0x20E9, 230},
	{0x20EA, 0x20EB, 1},
	{0x20EC, 0x20EF, 220},
	{0x20F0, 0x20F0, 230},
	{0x2CEF, 0x2CF1, 230},
	{0x2D7F, 0x2D7F, 9},
	{0x2DE0, 0x2DFF, 230},
	{0x302A, 0x302A, 218},
	{0x302B, 0x302B, 228},
	{0x302C, 0x302C, 232},
	{0x302D, 0x302D, 222},
	{0x302E, 0x302F, 224},
	{0x3099, 0x309A, 8},
	{0xA66F, 0xA66F, 230},
	{0xA674, 0xA67D, 230},
	{0xA69E, 0xA69F, 230},
	{0xA6F0, 0xA6F1, 230},
	{0xA806, 0xA806, 9},
	{0xA82C, 0xA82C, 9},
	{0xA8C4, 0xA8C4, 9},
	{0xA8E0, 0xA8F1, 230},
	{0xA92B, 0xA92D, 220},
	{0xA953, 0xA953, 9},
	{0xA9B3, 0xA9B3, 7},
	{0xA9C0, 0xA9C0, 9},
	{0xAAB0, 0xAAB0, 230},
	{0xAAB2, 0xAAB3, 230},
	{0xAAB4, 0xAAB4, 220},
	{0xAAB7, 0xAAB8, 230},
	{0xAABE, 0xAABF, 230},
	{0xAAC1, 0xAAC1, 230},
	{0xAAF6, 0xAAF6, 9},
	{0xABED, 0xABED, 9},
	{0xFB1E, 0xFB1E, 26},
	{0xFE20, 0xFE26, 230},
	{0xFE27, 0xFE2D, 220},
	{0xFE2E, 0xFE2F, 230},
	{0x101FD, 0x101FD, 220},
	{0x102E0, 0x102E0, 220},
	{0x10376, 0x1037A, 230},
	{0x10A0D, 0x10A0D, 220},
	{0x10A0F, 0x10A0F, 230},
	{0x10A38, 0x10A38, 230},
	{0x10A39, 0x10A39, 1},
	{0x10A3A, 0x10A3A, 220},
	{0x10A3F, 0x10A3F, 9},
	{0x10AE5, 0x10AE5, 230},
	{0x10AE6, 0x10AE6, 220},
	{0x10D24, 0x10D27, 230},
	{0x10D69, 0x10D6D, 230},
	{0x10EAB, 0x10EAC, 230},
	{0x10EFD, 0x10EFF, 220},
	{0x10F46, 0x10F47, 220},
	{0x10F48, 0x10F4A, 230},
	{0x10F4B, 0x10F4B, 220},
	{0x10F4C, 0x10F4C, 230},
	{0x10F4D, 0x10F50, 220},
	{0x10F82, 0x10F82, 230},
	{0x10F83, 0x10F83, 220},
	{0x10F84, 0x10F84, 230},
	{0x10F85, 0x10F85, 220},
	{0x11046, 0x11046, 9},
	{0x11070, 0x11070, 9},
	{0x1107F, 0x1107F, 9},
	{0x110B9, 0x110B9, 9},
	{0x110BA, 0x110BA, 7},
	{0x11100, 0x11102, 230},
	{0x11133, 0x11134, 9},
	{0x11173, 0x11173, 7},
	{0x111C0, 0x111C0, 9},
	{0x111CA, 0x111CA, 7},
	{0x11235, 0x11235, 9},
	{0x11236, 0x11236, 7},
	{0x112E9, 0x112E9, 7},
	{0x112EA, 0x112EA, 9},
	{0x1133B, 0x1133C, 7},
	{0x1134D, 0x1134D, 9},
	{0x11366, 0x1136C, 230},
	{0x11370, 0x11374, 230},
	{0x113CE, 0x113D0, 9},
	{0x11442, 0x11442, 9},
	{0x11446, 0x11446, 7},
	{0x1145E, 0x1145E, 230},
	{0x114C2, 0x114C2, 9},
	{0x114C3, 0x114C3, 7},
	{0x115BF, 0x115BF, 9},
	{0x115C0, 0x115C0, 7},
	{0x1163F, 0x1163F, 9},
	{0x116B6, 0x116B6, 9},
	{0x116B7, 0x116B7, 7},
	{0x1172B, 0x1172B, 9},
	{0x11839, 0x11839, 9},
	{0x1183A, 0x1183A, 7},
	{0x1193D, 0x1193E, 9},
	{0x11943, 0x11943, 7},
	{0x119E0, 0x119E0, 9},
	{0x11A34, 0x11A34, 9},
	{0x11A47, 0x11A47, 9},
	{0x11A99, 0x11A99, 9},
	{0x11C3F, 0x11C3F, 9},
	{0x11D42, 0x11D42, 7},
	{0x11D44, 0x11D45, 9},
	{0x11D97, 0x11D97, 9},
	{0x11F41, 0x11F42, 9},
	{0x1612F, 0x1612F, 9},
	{0x16AF0, 0x16AF4, 1},
	{0x16B30, 0x16B36, 230},
	{0x16FF0, 0x16FF1, 6},
	{0x1BC9E, 0x1BC9E, 1},
	{0x1D165, 0x1D166, 216},
	{0x1D167, 0x1D169, 1},
	{0x1D16D, 0x1D16D, 226},
	{0x1D16E, 0x1D172, 216},
	{0x1D17B, 0x1D182, 220},
	{0x1D185, 0x1D189, 230},
	{0x1D18A, 0x1D18B, 220},
	{0x1D1AA, 0x1D1AD, 230},
	{0x1D242, 0x1D244, 230},
	{0x1E000, 0x1E006, 230},
	{0x1E008, 0x1E018, 230},
	{0x1E01B, 0x1E021, 230},
	{0x1E023, 0x1E024, 230},
	{0x1E026, 0x1E02A, 230},
	{0x1E08F, 0x1E08F, 230},
	{0x1E130, 0x1E136, 230},
	{0x1E2AE, 0x1E2AE, 230},
	{0x1E2EC, 0x1E2EF, 230},
	{0x1E4EC, 0x1E4ED, 232},
	{0x1E4EE, 0x1E4EE, 220},
	{0x1E4EF, 0x1E4EF, 230},
	{0x1E5EE, 0x1E5EE, 230},
	{0x1E5EF, 0x1E5EF, 220},
	{0x1E8D0, 0x1E8D6, 220},
	{0x1E944, 0x1E949, 230},
	{0x1E94A, 0x1E94A, 7},
}

// combiningClass returns a rune's canonical combining class (0 for a
// starter, which is every character that is not a combining mark).
func combiningClass(r rune) uint8 {
	lo, hi := 0, len(cccRanges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < cccRanges[mid].lo:
			hi = mid - 1
		case r > cccRanges[mid].hi:
			lo = mid + 1
		default:
			return cccRanges[mid].class
		}
	}
	return 0
}

// decompositionOf returns r's one-level canonical decomposition (b is 0 for
// a singleton).
func decompositionOf(r rune) (a, b rune, ok bool) {
	lo, hi := 0, len(canonicalDecomps)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < canonicalDecomps[mid].r:
			hi = mid - 1
		case r > canonicalDecomps[mid].r:
			lo = mid + 1
		default:
			return canonicalDecomps[mid].a, canonicalDecomps[mid].b, true
		}
	}
	return 0, 0, false
}

var (
	compositionsOnce sync.Once
	compositions     map[[2]rune]rune
)

// composePair returns the primary composite of a followed by b, if any.
func composePair(a, b rune) (rune, bool) {
	compositionsOnce.Do(func() {
		compositions = make(map[[2]rune]rune, len(canonicalDecomps))
		for _, d := range canonicalDecomps {
			if d.composable {
				compositions[[2]rune{d.a, d.b}] = d.r
			}
		}
	})
	c, ok := compositions[[2]rune{a, b}]
	return c, ok
}

// normalizeForFont prepares a run for shaping the way HarfBuzz does: a
// character the font cannot draw is decomposed when its pieces can be
// drawn (an "ǻ" becomes "a" + ring + acute, positioned by GPOS), marks are
// put in canonical order, and a base plus mark is recomposed when the font
// has the precomposed glyph (decomposed "e" + U+0301 draws as the font's
// own "é", which is always better than stacking an accent).
func normalizeForFont(f *ttfFont, runes []rune) []rune {
	out := make([]rune, 0, len(runes))
	var leaves func(r rune, dst []rune) ([]rune, bool)
	leaves = func(r rune, dst []rune) ([]rune, bool) {
		if f.glyphID(r) != 0 {
			return append(dst, r), true
		}
		a, b, ok := decompositionOf(r)
		if !ok {
			return dst, false
		}
		if dst, ok = leaves(a, dst); !ok {
			return dst, false
		}
		if b != 0 {
			return leaves(b, dst)
		}
		return dst, true
	}
	for _, r := range runes {
		if f.glyphID(r) != 0 {
			out = append(out, r)
			continue
		}
		if parts, ok := leaves(r, nil); ok {
			out = append(out, parts...)
		} else {
			out = append(out, r)
		}
	}

	canonicalOrder(out)

	// Canonical composition: a mark composes with the last starter unless a
	// mark of the same or a lower class — or another starter — sits between.
	res := out[:0]
	starter, lastClass := -1, uint8(0)
	for _, r := range out {
		class := shapingClass(r)
		if starter >= 0 {
			adjacent := len(res)-1 == starter
			if adjacent || (lastClass != 0 && lastClass < class) {
				if c, ok := composePair(res[starter], r); ok && f.glyphID(c) != 0 {
					res[starter] = c
					continue
				}
			}
		}
		if class == 0 {
			starter, lastClass = len(res), 0
		} else {
			lastClass = class
		}
		res = append(res, r)
	}
	return res
}

// isDefaultIgnorable reports whether r is a Default_Ignorable_Code_Point:
// a format or control character that has no visible glyph of its own
// (joiners, direction marks, variation selectors, soft hyphen, …).
func isDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x00AD, r == 0x034F, r == 0x061C, r == 0x3164, r == 0xFEFF, r == 0xFFA0:
		return true
	case r >= 0x115F && r <= 0x1160, r >= 0x17B4 && r <= 0x17B5, r >= 0x180B && r <= 0x180F,
		r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2060 && r <= 0x206F,
		r >= 0xFE00 && r <= 0xFE0F, r >= 0xFFF0 && r <= 0xFFF8, r >= 0x1BCA0 && r <= 0x1BCA3,
		r >= 0x1D173 && r <= 0x1D17A, r >= 0xE0000 && r <= 0xE0FFF:
		return true
	}
	return false
}

// shapingClassRemap replaces the Hebrew, Arabic and a few Indic/SE-Asian
// combining classes with the values HarfBuzz sorts by. The Unicode numbers
// for those marks are historical (one class per mark, assigned in code
// point order), so sorting by them puts a Hebrew sheva before a dagesh or
// an Arabic fatha before a shadda — the reverse of how fonts attach them.
// Only the classes listed here change; every other class sorts as is.
var shapingClassRemap = map[uint8]uint8{
	// Hebrew: dagesh, rafe, shin/sin dots and holam come before the vowels.
	10: 22, 11: 15, 12: 16, 13: 17, 14: 23, 15: 18, 16: 19, 17: 20, 18: 21,
	19: 14, 20: 24, 21: 12, 22: 25, 23: 13, 24: 10, 25: 11, 26: 26,
	// Arabic: shadda comes before the vowel it carries.
	27: 28, 28: 29, 29: 30, 30: 31, 31: 32, 32: 33, 33: 27, 34: 34, 35: 35,
	// Telugu length marks are spacing; Thai sara u/uu sort early; Tibetan
	// vowel signs swap.
	84: 0, 91: 0, 103: 3, 130: 132, 132: 131,
}

// shapingClass is the combining class a shaper orders marks by.
func shapingClass(r rune) uint8 {
	c := combiningClass(r)
	if m, ok := shapingClassRemap[c]; ok {
		return m
	}
	return c
}

// canonicalOrder sorts each run of combining marks by shaping class, in
// place and stably (marks of equal class keep their typed order, as the
// algorithm requires). Runes with class 0 are fixed points.
//
// Each maximal run of non-zero-class runes is sorted independently with
// sort.SliceStable, O(run log run) rather than the O(run²) insertion
// sort this used to be (GHSA-7jjg-q5vf-8592 / pdf-go-velp: a text value
// of a few KB consisting mostly of combining marks — plausible user
// input, e.g. a field using combining diacritics instead of precomposed
// characters — could stall AddText for seconds). A stable sort by the
// same total order as the old bubble pass is guaranteed to produce the
// IDENTICAL permutation: stability pins the relative order of marks
// that compare equal (same class), and every other pair's relative
// order is fully determined by comparing their classes either way —
// there is no input on which the two algorithms could legally disagree.
func canonicalOrder(runes []rune) {
	n := len(runes)
	for i := 0; i < n; i++ {
		if shapingClass(runes[i]) == 0 {
			continue
		}
		j := i + 1
		for j < n && shapingClass(runes[j]) != 0 {
			j++
		}
		sortMarkRun(runes[i:j])
		i = j // the for loop's own i++ then lands past this run
	}
}

// sortMarkRun stably sorts one maximal run of non-zero-class marks by
// shaping class.
func sortMarkRun(run []rune) {
	if len(run) < 2 {
		return
	}
	sort.SliceStable(run, func(a, b int) bool {
		return shapingClass(run[a]) < shapingClass(run[b])
	})
}
