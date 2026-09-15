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

// tokensFrom builds tokens for a sentence, one line, one page, with
// non-overlapping rectangles so grouping has something real to union.
func tokensFrom(t *testing.T, s string, page int) []wordToken {
	t.Helper()
	var out []wordToken
	x := 0.0
	for _, w := range strings.Fields(s) {
		width := float64(len(w)) * 6
		out = append(out, wordToken{
			text: w,
			key:  w,
			page: page,
			line: 0,
			rect: Rectangle{LLX: x, LLY: 100, URX: x + width, URY: 112},
		})
		x += width + 3
	}
	return out
}

func TestGroupOperationsMergesRuns(t *testing.T) {
	src := tokensFrom(t, "alpha beta gamma delta", 1)
	dst := tokensFrom(t, "alpha delta", 1)
	edits, ok := diffKeys([]string{"alpha", "beta", "gamma", "delta"}, []string{"alpha", "delta"})
	if !ok {
		t.Fatal("cap hit")
	}
	ops := groupOperations(edits, src, dst, EditOperationsDeleteFirst)
	if len(ops) != 3 {
		t.Fatalf("got %d operations, want 3: %+v", len(ops), ops)
	}
	if ops[1].Operation != OperationDelete || ops[1].Text != "beta gamma" {
		t.Fatalf("operation 1 = %v %q, want delete %q", ops[1].Operation, ops[1].Text, "beta gamma")
	}
	if len(ops[1].SourceRects) != 1 {
		t.Fatalf("deleted run on one line must carry one rectangle, got %d", len(ops[1].SourceRects))
	}
	if ops[1].SourcePage != 1 || ops[1].DestPage != 0 {
		t.Errorf("deletion pages = src %d dst %d, want 1 and 0", ops[1].SourcePage, ops[1].DestPage)
	}
}

func TestGroupOperationsBreaksRunsAtLineAndPage(t *testing.T) {
	src := []wordToken{
		{text: "one", key: "one", page: 1, line: 0, rect: Rectangle{LLX: 0, LLY: 100, URX: 20, URY: 112}},
		{text: "two", key: "two", page: 1, line: 1, rect: Rectangle{LLX: 0, LLY: 80, URX: 20, URY: 92}},
		{text: "three", key: "three", page: 2, line: 0, rect: Rectangle{LLX: 0, LLY: 100, URX: 30, URY: 112}},
	}
	edits := []edit{
		{kind: editDelete, a: 0, b: -1},
		{kind: editDelete, a: 1, b: -1},
		{kind: editDelete, a: 2, b: -1},
	}
	ops := groupOperations(edits, src, nil, EditOperationsDeleteFirst)
	if len(ops) != 2 {
		t.Fatalf("a run must break at the page boundary: got %d operations %+v", len(ops), ops)
	}
	if ops[0].Text != "one two" || len(ops[0].SourceRects) != 2 {
		t.Errorf("first run = %q with %d rects, want %q with 2", ops[0].Text, len(ops[0].SourceRects), "one two")
	}
	if ops[1].SourcePage != 2 {
		t.Errorf("second run page = %d, want 2", ops[1].SourcePage)
	}
}

func TestGroupOperationsRespectsEditOperationsOrder(t *testing.T) {
	src := tokensFrom(t, "total is 100", 1)
	dst := tokensFrom(t, "total is 200", 1)
	edits, ok := diffKeys([]string{"total", "is", "100"}, []string{"total", "is", "200"})
	if !ok {
		t.Fatal("cap hit")
	}

	first := groupOperations(edits, src, dst, EditOperationsDeleteFirst)
	if first[1].Operation != OperationDelete || first[2].Operation != OperationInsert {
		t.Errorf("DeleteFirst gave %v then %v", first[1].Operation, first[2].Operation)
	}

	second := groupOperations(edits, src, dst, EditOperationsInsertFirst)
	if second[1].Operation != OperationInsert || second[2].Operation != OperationDelete {
		t.Errorf("InsertFirst gave %v then %v", second[1].Operation, second[2].Operation)
	}
}
