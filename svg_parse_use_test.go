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
	// After parse + resolveUseReferences (Task 6), no *svgUse nodes remain.
	useCount := 0
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			useCount++
		}
	}
	if useCount != 0 {
		t.Errorf("expected 0 *svgUse nodes after resolution, got %d", useCount)
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

func TestResolveUseReferences_SimpleClone(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_simple.svg")
	svg, _ := parseSVGBytes(data)
	// After parse + resolve, no *svgUse nodes should remain.
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			t.Errorf("expected svgUse to be resolved, found: %+v", c)
		}
	}
	// Two uses → two top-level groups (each containing a cloned circle).
	if len(svg.root.children) != 2 {
		t.Errorf("expected 2 wrapped clones, got %d", len(svg.root.children))
	}
}

func TestResolveUseReferences_MissingRefDropped(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<use href="#nonexistent" x="0" y="0"/>
		<rect x="0" y="0" width="10" height="10" fill="red"/>
	</svg>`))
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			t.Errorf("expected missing-ref use to be dropped, found: %+v", c)
		}
	}
	// rect should still be present
	foundRect := false
	for _, c := range svg.root.children {
		if _, ok := c.(*svgRect); ok {
			foundRect = true
		}
	}
	if !foundRect {
		t.Error("rect was dropped along with the missing use ref")
	}
}

func TestResolveUseReferences_CycleDropped(t *testing.T) {
	// <defs><g id="a"><use href="#a"/></g></defs><use href="#a"/>
	// Recursive use should be detected and dropped (cycle).
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><g id="a"><use href="#a"/></g></defs>
		<use href="#a"/>
	</svg>`))
	// Should not crash. The outer use resolves to the group, and the inner
	// use reference to "a" should be detected as cycle and dropped.
	// We just verify no infinite loop and no panic.
	_ = svg
}
