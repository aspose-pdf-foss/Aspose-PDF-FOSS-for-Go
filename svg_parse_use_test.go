// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_UseStoresPlaceholder(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_simple.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	// After parse but BEFORE resolveUseReferences (Task 6), the IR contains *svgUse.
	useCount := 0
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			useCount++
		}
	}
	if useCount != 2 {
		t.Errorf("expected 2 *svgUse nodes, got %d", useCount)
	}
	// defs should contain the dot
	if _, ok := svg.defs["dot"].(*svgCircle); !ok {
		t.Errorf("defs[dot] = %T, want *svgCircle", svg.defs["dot"])
	}
}

func TestParseSVG_SymbolStored(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_symbol.svg")
	svg, _ := parseSVGBytes(data)
	sym, ok := svg.defs["star"].(*svgSymbol)
	if !ok {
		t.Fatalf("defs[star] = %T", svg.defs["star"])
	}
	if sym.viewBox == nil || sym.viewBox.w != 10 || sym.viewBox.h != 10 {
		t.Errorf("symbol viewBox = %+v", sym.viewBox)
	}
	if len(sym.children) == 0 {
		t.Error("symbol has no children")
	}
}
