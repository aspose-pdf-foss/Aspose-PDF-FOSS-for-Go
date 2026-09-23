// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Code128 encoder (ISO/IEC 15417). Scope: Code Set B only — every value is
// encoded as a printable-ASCII character (space through '~', 0x20-0x7E), one
// symbol per input character. This omits the well-known Code Set C
// digit-pair compaction (denser output for long numeric runs) but keeps the
// encoder to a single, simply-checked code path: every input byte maps to
// exactly one 11-module symbol, with no subset-switching state machine to
// get wrong. Values outside printable ASCII are rejected — use
// BarcodeQR for extended text.

// code128Patterns holds the bar/space widths for symbol values 0-106, one
// digit string per value: values 0-105 are six widths (three bars, three
// spaces, alternating, each 1-4 modules, summing to 11); value 106 (STOP)
// is seven widths summing to 13 (the final bar that terminates the symbol).
// Widths concatenate directly across symbols with no gap — each 6-digit
// pattern already ends on a space, so the next symbol's leading bar follows
// immediately. This is the standard ISO/IEC 15417 Annex pattern table,
// shared by all three code sets (only the character each value represents
// differs by set; Code Set B's mapping is value = ord(ch)-32 for ch in
// [0x20,0x7E], values 96-102 are Code Set B's function characters (unused
// here), and 103/104/105 are START A/B/C).
var code128Patterns = [107]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213",
	"122312", "132212", "221213", "221312", "231212", "112232", "122132",
	"122231", "113222", "123122", "123221", "223211", "221132", "221231",
	"213212", "223112", "312131", "311222", "321122", "321221", "312212",
	"322112", "322211", "212123", "212321", "232121", "111323", "131123",
	"131321", "112313", "132113", "132311", "211313", "231113", "231311",
	"112133", "112331", "132131", "113123", "113321", "133121", "313121",
	"211331", "231131", "213113", "213311", "213131", "311123", "311321",
	"331121", "312113", "312311", "332111", "314111", "221411", "431111",
	"111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114",
	"413111", "241112", "134111", "111242", "121142", "121241", "114212",
	"124112", "124211", "411212", "421112", "421211", "212141", "214121",
	"412121", "111143", "111341", "131141", "114113", "114311", "411113",
	"411311", "113141", "114131", "311141", "411131",
	"211412",  // 103 START A
	"211214",  // 104 START B
	"211232",  // 105 START C
	"2331112", // 106 STOP
}

const (
	code128StartB = 104
	code128Stop   = 106
)

// code128ValueForRune maps a printable-ASCII rune to its Code Set B value
// (0-94), or ok=false if r is outside [0x20, 0x7E].
func code128ValueForRune(r rune) (int, bool) {
	if r < 0x20 || r > 0x7E {
		return 0, false
	}
	return int(r - 0x20), true
}

// encodeCode128Modules encodes value as a Code Set B Code128 symbol and
// returns its module grid (Rows=1, quiet zone included). Errors if value is
// empty or contains a character outside printable ASCII.
func encodeCode128Modules(value string) (barcodeModules, error) {
	if value == "" {
		return barcodeModules{}, fmt.Errorf("asposepdf: Code128 barcode value is empty")
	}
	symbolValues := make([]int, 0, len(value)+3)
	symbolValues = append(symbolValues, code128StartB)
	for _, r := range value {
		v, ok := code128ValueForRune(r)
		if !ok {
			return barcodeModules{}, fmt.Errorf("asposepdf: Code128 cannot encode %q (0x%04X) — only printable ASCII (0x20-0x7E) is supported; use BarcodeQR for extended text", r, r)
		}
		symbolValues = append(symbolValues, v)
	}

	// Symbol check character (ISO/IEC 15417 §4.3.2.1): the start value plus
	// each data value weighted by its 1-based position, mod 103.
	checksum := symbolValues[0]
	for i := 1; i < len(symbolValues); i++ {
		checksum += symbolValues[i] * i
	}
	checksum %= 103
	symbolValues = append(symbolValues, checksum, code128Stop)

	var widths []int
	for _, v := range symbolValues {
		pattern := code128Patterns[v]
		for _, c := range pattern {
			widths = append(widths, int(c-'0'))
		}
	}

	const quietModules = 10
	totalModules := quietModules * 2
	for _, w := range widths {
		totalModules += w
	}
	bits := make([]bool, totalModules)
	pos := quietModules
	dark := true
	for _, w := range widths {
		if dark {
			for i := 0; i < w; i++ {
				bits[pos+i] = true
			}
		}
		pos += w
		dark = !dark
	}

	return barcodeModules{Cols: totalModules, Rows: 1, Bits: bits}, nil
}
