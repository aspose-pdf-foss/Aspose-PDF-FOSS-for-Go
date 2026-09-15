// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// applyEdits rebuilds both sequences from an edit script: the source is the
// equals plus the deletions, the destination the equals plus the insertions.
func applyEdits(edits []edit, a, b []string) (src, dst []string) {
	for _, e := range edits {
		switch e.kind {
		case editEqual:
			src = append(src, a[e.a])
			dst = append(dst, b[e.b])
		case editDelete:
			src = append(src, a[e.a])
		case editInsert:
			dst = append(dst, b[e.b])
		}
	}
	return src, dst
}

func TestDiffKeysRoundTrip(t *testing.T) {
	cases := []struct{ a, b string }{
		{"one two three", "one two three"},             // identical
		{"one two three", "one two three four"},        // append
		{"one two three", "one three"},                 // delete in the middle
		{"one two three", "one TWO three"},             // replacement
		{"", "alpha beta"},                             // empty source
		{"alpha beta", ""},                             // empty destination
		{"a b c d e f", "f e d c b a"},                 // reversal
		{"the quick brown fox", "the slow brown cat!"}, // two replacements
	}
	for _, c := range cases {
		a := strings.Fields(c.a)
		b := strings.Fields(c.b)
		edits, ok := diffKeys(a, b)
		if !ok {
			t.Fatalf("%q → %q: hit the edit-distance cap unexpectedly", c.a, c.b)
		}
		src, dst := applyEdits(edits, a, b)
		if strings.Join(src, " ") != strings.Join(a, " ") {
			t.Errorf("%q → %q: source rebuilt as %q", c.a, c.b, strings.Join(src, " "))
		}
		if strings.Join(dst, " ") != strings.Join(b, " ") {
			t.Errorf("%q → %q: destination rebuilt as %q", c.a, c.b, strings.Join(dst, " "))
		}
	}
}

func TestDiffKeysIdenticalIsAllEqual(t *testing.T) {
	a := strings.Fields("alpha beta gamma")
	edits, ok := diffKeys(a, a)
	if !ok {
		t.Fatal("hit the cap on identical input")
	}
	if len(edits) != 3 {
		t.Fatalf("got %d edits, want 3", len(edits))
	}
	for i, e := range edits {
		if e.kind != editEqual {
			t.Errorf("edit %d kind = %v, want equal", i, e.kind)
		}
	}
}

func TestDiffKeysMinimalChange(t *testing.T) {
	a := strings.Fields("total is 100 euro")
	b := strings.Fields("total is 200 euro")
	edits, ok := diffKeys(a, b)
	if !ok {
		t.Fatal("hit the cap")
	}
	var ins, del, eq int
	for _, e := range edits {
		switch e.kind {
		case editInsert:
			ins++
		case editDelete:
			del++
		case editEqual:
			eq++
		}
	}
	if eq != 3 || ins != 1 || del != 1 {
		t.Fatalf("got %d equal, %d insert, %d delete; want 3/1/1", eq, ins, del)
	}
}

func TestDiffKeysCapReturnsFalse(t *testing.T) {
	a := make([]string, maxEditDistance)
	b := make([]string, maxEditDistance)
	for i := range a {
		a[i] = "a" + string(rune('A'+i%26))
		b[i] = "b" + string(rune('A'+i%26))
	}
	if _, ok := diffKeys(a, b); ok {
		t.Fatal("two entirely different sequences of cap length should report the cap")
	}
}
