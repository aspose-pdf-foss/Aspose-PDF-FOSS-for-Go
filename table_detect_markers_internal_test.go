// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func cellOf(text string) streamCellSpan {
	return streamCellSpan{frs: []TextFragment{{Text: text}}}
}

// TestListMarkerFromSymbolFont: a bullet a symbol font draws extracts as a
// private-use code point, which marks a list row like "•" does; a lone U+FFFD
// does before readable text, and does not on a page whose text is unmapped
// throughout.
func TestListMarkerFromSymbolFont(t *testing.T) {
	if !isListMarkerCell(cellOf("")) {
		t.Error("U+F0B7 not a list marker")
	}
	if isListMarkerCell(cellOf("�")) {
		t.Error("a lone U+FFFD counts as a marker whatever follows it")
	}
	readable := streamRow{cells: []streamCellSpan{cellOf("�"), cellOf("Test equipment")}}
	if !unmappedBullet(readable) {
		t.Error("U+FFFD before readable text not taken for a bullet")
	}
	unmapped := streamRow{cells: []streamCellSpan{cellOf("�"), cellOf("���")}}
	if unmappedBullet(unmapped) {
		t.Error("U+FFFD among unmapped text taken for a bullet")
	}
}
