// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// TestSubsetTTFStructure checks the rebuilt font program: it is much
// smaller, exposes the minimal required glyf-outline tables, reports a
// compact glyph count in maxp, and the CIDToGIDMap remaps the original
// glyph IDs to a dense 0..N-1 range.
func TestSubsetTTFStructure(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatalf("parseTTF: %v", err)
	}
	used := map[uint16]bool{}
	for _, r := range "Hello, Мир! 123" {
		if g := f.glyphID(r); g != 0 {
			used[g] = true
		}
	}
	res, err := subsetTTF(f, used)
	if err != nil {
		t.Fatalf("subsetTTF: %v", err)
	}

	if res.newSize >= res.oldSize/4 {
		t.Errorf("subset not small enough: new=%d old=%d (want new < old/4)", res.newSize, res.oldSize)
	}

	tables, err := readTableDirectory(res.program)
	if err != nil {
		t.Fatalf("readTableDirectory(subset): %v", err)
	}
	for _, tag := range []string{"glyf", "loca", "head", "hhea", "hmtx", "maxp", "post"} {
		if _, ok := tables[tag]; !ok {
			t.Errorf("subset missing required table %q", tag)
		}
	}

	// maxp.numGlyphs should equal the kept-glyph count, which is far below
	// the original font's glyph count.
	maxp := tableSlice(res.program, tables, "maxp")
	if len(maxp) < 6 {
		t.Fatal("subset maxp too small")
	}
	subN := int(maxp[4])<<8 | int(maxp[5])
	if subN == 0 || subN >= int(f.numGlyphs) {
		t.Errorf("subset numGlyphs = %d, want in (0, %d)", subN, f.numGlyphs)
	}

	// CIDToGIDMap: glyph 0 → 0; each used original GID → a distinct
	// in-range subset GID.
	if len(res.cidToGID) < 2 {
		t.Fatal("empty CIDToGIDMap")
	}
	seen := map[uint16]bool{}
	for oldGID := range used {
		off := int(oldGID) * 2
		if off+2 > len(res.cidToGID) {
			t.Fatalf("CIDToGIDMap too short for GID %d", oldGID)
		}
		newGID := uint16(res.cidToGID[off])<<8 | uint16(res.cidToGID[off+1])
		if int(newGID) >= subN {
			t.Errorf("GID %d → %d out of range [0,%d)", oldGID, newGID, subN)
		}
		if seen[newGID] {
			t.Errorf("duplicate subset GID %d", newGID)
		}
		seen[newGID] = true
	}
}

// TestSubsetFontsRoundTrip exercises the public API end to end: embed a
// TTF, draw mixed Latin + Cyrillic text, subset, and confirm the output
// is dramatically smaller while the text still extracts correctly.
func TestSubsetFontsRoundTrip(t *testing.T) {
	doc := NewDocument(595, 842)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	page, _ := doc.Page(1)
	const text = "Привет, мир! Hello 42"
	if err := page.AddText(text, TextStyle{Font: font, Size: 16},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 760}); err != nil {
		t.Fatalf("AddText: %v", err)
	}

	// Size with the full font embedded.
	var full bytes.Buffer
	if _, err := doc.WriteTo(&full); err != nil {
		t.Fatalf("WriteTo (full): %v", err)
	}

	n, err := doc.SubsetFonts()
	if err != nil {
		t.Fatalf("SubsetFonts: %v", err)
	}
	if n != 1 {
		t.Fatalf("SubsetFonts subsetted %d fonts, want 1", n)
	}

	var sub bytes.Buffer
	if _, err := doc.WriteTo(&sub); err != nil {
		t.Fatalf("WriteTo (subset): %v", err)
	}

	if sub.Len() >= full.Len()/2 {
		t.Errorf("subset output not small enough: subset=%d full=%d (want subset < full/2)", sub.Len(), full.Len())
	}

	// The text must survive the subset (extraction via the trimmed
	// /ToUnicode keyed by the original glyph IDs).
	reopened, err := OpenStream(bytes.NewReader(sub.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream (subset): %v", err)
	}
	pages, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	got := strings.Join(pages, "\n")
	for _, want := range []string{"Привет", "мир", "Hello 42"} {
		if !strings.Contains(got, want) {
			t.Errorf("extracted text missing %q; got %q", want, got)
		}
	}
}

// TestSubsetFontsNoUsageIsNoop verifies that an embedded font with no
// drawn glyphs is skipped (count 0, no error).
func TestSubsetFontsNoUsageIsNoop(t *testing.T) {
	doc := NewDocument(595, 842)
	if _, err := doc.LoadFont("testdata/DejaVuSans.ttf"); err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	n, err := doc.SubsetFonts()
	if err != nil {
		t.Fatalf("SubsetFonts: %v", err)
	}
	if n != 0 {
		t.Errorf("SubsetFonts subsetted %d fonts, want 0 (no glyphs drawn)", n)
	}
}
