// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

// findBlocks returns all blocks of the given kind, depth-first.
func findBlocks(b *mdBlock, kind mdBlockKind) []*mdBlock {
	var out []*mdBlock
	if b.kind == kind {
		out = append(out, b)
	}
	for _, c := range b.children {
		out = append(out, findBlocks(c, kind)...)
	}
	return out
}

// findInlines returns all inline nodes of the given kind across the tree.
func findInlines(nodes []*mdInline, kind mdInlineKind) []*mdInline {
	var out []*mdInline
	for _, n := range nodes {
		if n.kind == kind {
			out = append(out, n)
		}
		out = append(out, findInlines(n.children, kind)...)
	}
	return out
}

func inlinePlainText(nodes []*mdInline) string {
	s := ""
	for _, n := range nodes {
		s += n.text + inlinePlainText(n.children)
	}
	return s
}

func TestGFMTable(t *testing.T) {
	doc := parseMarkdown("intro line\n\n| Name | Qty | Price |\n|:-----|:---:|------:|\n| Foo  |  2  | 10.50 |\n| Bar \\| Baz | 1 | 3 |\n| short |\n\nafter\n")
	tables := findBlocks(doc, mdTable)
	if len(tables) != 1 {
		t.Fatalf("tables = %d; want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.headerCells) != 3 || tbl.headerCells[0] != "Name" || tbl.headerCells[2] != "Price" {
		t.Errorf("header = %q", tbl.headerCells)
	}
	wantAligns := []mdAlign{mdAlignLeft, mdAlignCenter, mdAlignRight}
	for i, a := range wantAligns {
		if tbl.aligns[i] != a {
			t.Errorf("align[%d] = %d; want %d", i, tbl.aligns[i], a)
		}
	}
	if len(tbl.rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(tbl.rows))
	}
	if tbl.rows[1][0] != "Bar | Baz" {
		t.Errorf("escaped pipe cell = %q", tbl.rows[1][0])
	}
	if len(tbl.rows[2]) != 3 || tbl.rows[2][1] != "" {
		t.Errorf("short row not padded: %q", tbl.rows[2])
	}
	// The header line + delimiter row inside a longer paragraph: the intro
	// stays a paragraph.
	paras := findBlocks(doc, mdParagraph)
	if len(paras) != 2 {
		t.Errorf("paragraphs = %d; want 2 (intro + after)", len(paras))
	}
}

func TestGFMTableHeaderAfterParagraphLine(t *testing.T) {
	doc := parseMarkdown("some text\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	tables := findBlocks(doc, mdTable)
	if len(tables) != 1 {
		t.Fatalf("tables = %d; want 1", len(tables))
	}
	paras := findBlocks(doc, mdParagraph)
	if len(paras) != 1 || paras[0].content[0] != "some text" {
		t.Errorf("leading paragraph lost: %+v", paras)
	}
}

func TestGFMStrikethrough(t *testing.T) {
	doc := parseMarkdown("a ~~gone~~ b ~single~ c ~~~notstruck~~~\n")
	paras := findBlocks(doc, mdParagraph)
	strikes := findInlines(paras[0].inlines, mdStrike)
	if len(strikes) != 2 {
		t.Fatalf("strikes = %d; want 2 (~~ and ~)", len(strikes))
	}
	if got := inlinePlainText(strikes[0].children); got != "gone" {
		t.Errorf("strike text = %q", got)
	}
	if got := inlinePlainText(strikes[1].children); got != "single" {
		t.Errorf("single-tilde strike text = %q", got)
	}
}

func TestGFMBareAutolinks(t *testing.T) {
	doc := parseMarkdown("Visit https://example.com/a(b) now, or www.foo.bar/baz. Mail bob@example.com!\n")
	paras := findBlocks(doc, mdParagraph)
	links := findInlines(paras[0].inlines, mdLink)
	if len(links) != 3 {
		t.Fatalf("links = %d; want 3: %+v", len(links), links)
	}
	if links[0].dest != "https://example.com/a(b)" {
		t.Errorf("url dest = %q (balanced parens kept, comma trimmed)", links[0].dest)
	}
	if links[1].dest != "http://www.foo.bar/baz" {
		t.Errorf("www dest = %q", links[1].dest)
	}
	if links[2].dest != "mailto:bob@example.com" || inlinePlainText(links[2].children) != "bob@example.com" {
		t.Errorf("email link = %+v", links[2])
	}

	// No linkification in strict-core mode (spec harness path).
	core := parseMarkdownCore("see https://example.com here\n")
	cparas := findBlocks(core, mdParagraph)
	if n := len(findInlines(cparas[0].inlines, mdLink)); n != 0 {
		t.Errorf("core mode produced %d links; want 0", n)
	}
}

func TestGFMAutolinkNotInsideBrackets(t *testing.T) {
	doc := parseMarkdown("[text with https://example.com inside](/dest)\n")
	paras := findBlocks(doc, mdParagraph)
	links := findInlines(paras[0].inlines, mdLink)
	if len(links) != 1 || links[0].dest != "/dest" {
		t.Errorf("links = %+v; want only the explicit one", links)
	}
}
