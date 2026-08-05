// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"testing"
)

// Word pairs a WinAnsi /Encoding with an embedded subset whose (1,0) cmap is
// keyed by MACROMAN codes; raw-code-first glyph selection rendered WinAnsi
// 0xFC ('u umlaut') as the MacRoman cedilla (pdf-go-zng8). The declared
// encoding must win for programs that carry a Unicode cmap.
func TestRenderWordSubsetUmlauts(t *testing.T) {
	doc, err := Open("testdata/word-umlauts.pdf")
	if err != nil {
		t.Fatal(err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	// Locate the Arial render font and check glyph selection directly.
	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if want := "Grüße aus München"; !containsStr(txt, want) {
		t.Fatalf("extraction lost the umlauts: %q", txt)
	}

	rd := newRenderer(p, image.NewRGBA(image.Rect(0, 0, 10, 10)), 10, 10, identityMatrix())
	res := p.pageResources()
	rd.res = res
	fontsDict, ok := resolveRefToDict(doc.objects, res["/Font"])
	if !ok {
		t.Fatal("no /Font resources")
	}
	checked := false
	for name := range fontsDict {
		rf := rd.buildRenderFont(name)
		if rf == nil || rf.isType0 || rf.prog == nil ||
			len(rf.prog.runeToGlyph) == 0 || len(rf.prog.codeToGlyph) == 0 {
			continue // the bug needs a simple font with BOTH cmap kinds
		}
		if rf.prog.glyphID('ü') == 0 {
			continue
		}
		checked = true
		wantGid := rf.prog.glyphID('ü')
		if got := rf.gid(0xFC); got != wantGid {
			t.Errorf("font %s: gid(0xFC) = %d; want %d (u-umlaut) — MacRoman raw-code glyph leaked", name, got, wantGid)
		}
	}
	if !checked {
		t.Fatal("no embedded TrueType with a Unicode cmap found")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
