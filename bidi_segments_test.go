// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math/rand"
	"strings"
	"testing"
)

// A table row extracted as one line is read as one bidi paragraph per cell:
// a Latin placeholder at the end of a Hebrew cell keeps its neutrals, even
// when the row's majority makes the whole line right to left and the cell
// beside it ends in Latin.
func TestBidiCellsAreSeparateParagraphs(t *testing.T) {
	d := ruledTwoCellDoc(t, "4000 per month.", "שלום עולם יפה מאוד &KEY&")
	p := mustPage(t, d)
	if ms, err := p.SearchText("&KEY&"); err != nil || len(ms) != 1 {
		t.Errorf("search: %d matches, %v", len(ms), err)
	}
	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "&KEY&") || !strings.Contains(txt, "4000 per month.") {
		t.Errorf("text %q lost a cell's order", txt)
	}
	// The line reads right to left: the right cell comes first.
	if strings.Index(txt, "שלום") > strings.Index(txt, "4000") {
		t.Errorf("text %q: the right-to-left row does not start with its right cell", txt)
	}
}

// A left-to-right row: the cells follow in page order, one space between.
func TestBidiCellsLTRRowText(t *testing.T) {
	d := ruledTwoCellDoc(t, "Employee and other words.", "שלום")
	lines, err := mustPage(t, d).ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if strings.Contains(l.Text, "Employee") {
			if l.Text != "Employee and other words. שלום" {
				t.Errorf("line %q", l.Text)
			}
			return
		}
	}
	t.Fatal("line not found")
}

func TestBidiVisualToLogicalSegments(t *testing.T) {
	he := func(s string) string { // visual order of a Hebrew word
		r := []rune(s)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r)
	}
	for _, c := range []struct {
		name, visual string
		cuts         []int
		want         string
	}{
		{"none", "abc " + he("שלום"), nil, "שלום abc"},
		// "ab." | "&K& םולש עולם": RTL majority, right cell first.
		{"rtl", "ab. &K& " + he("עולם") + " " + he("שלום"), []int{4}, "שלום עולם &K& ab."},
		{"ltr", "hello world. " + he("שלום"), []int{13}, "hello world. שלום"},
		// A cell of numbers in a right-to-left row reads right to left.
		{"numbers", "10% 3000 20% 6000 " + he("שלום"), []int{13}, "שלום 6000 20% 3000 10%"},
		// Hebrew and Latin even in a segment of a right-to-left line.
		{"tie", he("איש") + " man " + he("אנשים") + " men", []int{8}, "men אנשים man איש"},
		// A Hebrew cell with a placeholder in a left-to-right line.
		{"rtl cell", "hello world foo. &K& " + he("שלום"), []int{17}, "hello world foo. שלום &K&"},
		{"no space", "abcd" + he("שלום"), []int{4}, "abcdשלום"},
	} {
		t.Run(c.name, func(t *testing.T) {
			runes := []rune(c.visual)
			got, order := bidiVisualToLogicalSegments(runes, c.cuts)
			if string(got) != c.want {
				t.Errorf("got %q, want %q", string(got), c.want)
			}
			if len(order) != len(runes) {
				t.Fatalf("order has %d entries for %d runes", len(order), len(runes))
			}
			seen := map[int]bool{}
			for _, v := range order {
				if seen[v] || v < 0 || v >= len(runes) {
					t.Fatalf("order %v is not a permutation", order)
				}
				seen[v] = true
			}
		})
	}
}

// Whatever the cuts, the reordering is a permutation of the line.
func TestBidiVisualToLogicalSegmentsPermutes(t *testing.T) {
	alpha := []rune("ab אב 1.(&")
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 5000; n++ {
		runes := make([]rune, 1+r.Intn(14))
		for i := range runes {
			runes[i] = alpha[r.Intn(len(alpha))]
		}
		var cuts []int
		for i := 1; i < len(runes); i++ {
			if r.Intn(4) == 0 {
				cuts = append(cuts, i)
			}
		}
		out, order := bidiVisualToLogicalSegments(runes, cuts)
		seen := make([]bool, len(runes))
		for _, v := range order {
			if v < 0 || v >= len(runes) || seen[v] {
				t.Fatalf("%q cuts %v: order %v is not a permutation", string(runes), cuts, order)
			}
			seen[v] = true
		}
		if len(order) != len(runes) || len(out) != len(runes) {
			t.Fatalf("%q cuts %v -> %q", string(runes), cuts, string(out))
		}
	}
}
