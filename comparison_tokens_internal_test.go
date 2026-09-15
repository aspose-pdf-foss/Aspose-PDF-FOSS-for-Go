// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// drawWordsAndReopen writes one line of Standard-14 text and reopens the saved
// document, so the tokenizer is fed what a reader of the finished file sees.
func drawWordsAndReopen(t *testing.T, text string) *Document {
	t.Helper()
	doc := NewDocument(400, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	style := TextStyle{Font: FontHelvetica, Size: 14}
	if err := page.AddText(text, style, Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 160}); err != nil {
		t.Fatalf("add text: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

func TestWordSpans(t *testing.T) {
	got := wordSpans("  alpha beta\tgamma ")
	want := [][2]int{{2, 7}, {8, 12}, {13, 18}}
	if len(got) != len(want) {
		t.Fatalf("got %d spans %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPageWordTokens(t *testing.T) {
	doc := drawWordsAndReopen(t, "alpha beta gamma")
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := pageWordTokens(page, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(tokens), tokens)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if tokens[i].text != want {
			t.Errorf("token %d = %q, want %q", i, tokens[i].text, want)
		}
		if tokens[i].page != 1 {
			t.Errorf("token %d page = %d, want 1", i, tokens[i].page)
		}
		if tokens[i].rect.URX <= tokens[i].rect.LLX || tokens[i].rect.URY <= tokens[i].rect.LLY {
			t.Errorf("token %d has an empty rect: %+v", i, tokens[i].rect)
		}
	}
	// Words are laid out left to right, so their rectangles must not overlap
	// and must advance.
	if tokens[0].rect.URX > tokens[1].rect.LLX+0.5 {
		t.Errorf("token 0 (%.2f..%.2f) overlaps token 1 (%.2f..%.2f)",
			tokens[0].rect.LLX, tokens[0].rect.URX, tokens[1].rect.LLX, tokens[1].rect.URX)
	}
}

func TestTokenKeyIgnoreCase(t *testing.T) {
	if got := tokenKey("Alpha", false); got != "Alpha" {
		t.Errorf("case-sensitive key = %q, want %q", got, "Alpha")
	}
	if got := tokenKey("Alpha", true); got != "alpha" {
		t.Errorf("case-insensitive key = %q, want %q", got, "alpha")
	}
}
