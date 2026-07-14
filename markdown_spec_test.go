// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// mdTestHTML renders the block tree to CommonMark reference HTML — used only
// by the spec-conformance tests to compare against spec.json's expected
// output. Inline content is emitted by mdTestInline (raw escaped text until
// the phase-2 inline parser lands; then the real inline tree).
func mdTestHTML(doc *mdBlock) string {
	var b strings.Builder
	mdTestChildren(&b, doc, false)
	return b.String()
}

func mdTestCR(b *strings.Builder) {
	if s := b.String(); len(s) > 0 && !strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
	}
}

func mdTestEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// mdTestInline renders a leaf block's inline content. Phase 1: escaped raw
// text with soft line breaks preserved.
func mdTestInline(b *strings.Builder, node *mdBlock) {
	b.WriteString(mdTestEsc(strings.Join(node.content, "\n")))
}

func mdTestChildren(b *strings.Builder, node *mdBlock, tight bool) {
	for _, c := range node.children {
		mdTestBlock(b, c, tight)
	}
}

func mdTestBlock(b *strings.Builder, node *mdBlock, tight bool) {
	switch node.kind {
	case mdParagraph:
		if tight {
			// Tight-list paragraph: bare inline content; a following block's
			// leading cr supplies the line break.
			mdTestInline(b, node)
			return
		}
		mdTestCR(b)
		b.WriteString("<p>")
		mdTestInline(b, node)
		b.WriteString("</p>\n")
	case mdHeading:
		mdTestCR(b)
		fmt.Fprintf(b, "<h%d>", node.level)
		mdTestInline(b, node)
		fmt.Fprintf(b, "</h%d>\n", node.level)
	case mdCodeBlock:
		mdTestCR(b)
		b.WriteString("<pre><code")
		if node.info != "" {
			fmt.Fprintf(b, " class=\"language-%s\"", mdTestEsc(strings.Fields(node.info)[0]))
		}
		b.WriteString(">")
		b.WriteString(mdTestEsc(node.literal))
		b.WriteString("</code></pre>\n")
	case mdHTMLBlock:
		mdTestCR(b)
		b.WriteString(node.literal)
		mdTestCR(b)
	case mdThematicBreak:
		mdTestCR(b)
		b.WriteString("<hr />\n")
	case mdBlockQuote:
		mdTestCR(b)
		b.WriteString("<blockquote>\n")
		mdTestChildren(b, node, false)
		mdTestCR(b)
		b.WriteString("</blockquote>\n")
	case mdList:
		mdTestCR(b)
		if node.list.ordered {
			if node.list.start != 1 {
				fmt.Fprintf(b, "<ol start=\"%d\">\n", node.list.start)
			} else {
				b.WriteString("<ol>\n")
			}
		} else {
			b.WriteString("<ul>\n")
		}
		mdTestChildren(b, node, node.list.tight)
		if node.list.ordered {
			b.WriteString("</ol>\n")
		} else {
			b.WriteString("</ul>\n")
		}
	case mdListItem:
		b.WriteString("<li>")
		mdTestChildren(b, node, tight)
		b.WriteString("</li>\n")
	case mdTable:
		// GFM extension — not part of the core spec output.
	}
}

type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadSpecExamples(t *testing.T) []specExample {
	t.Helper()
	raw, err := os.ReadFile("testdata/commonmark_spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples []specExample
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	if len(examples) < 600 {
		t.Fatalf("suspiciously few spec examples: %d", len(examples))
	}
	return examples
}

// TestCommonMarkSpec runs the official CommonMark 0.31.2 test set. Until the
// phase-2 inline parser lands, examples whose expected HTML depends on inline
// processing fail — the floor below tracks the block-phase state and is
// raised as phases land (final target documented in the epic pdf-go-fh4l).
func TestCommonMarkSpec(t *testing.T) {
	examples := loadSpecExamples(t)

	type stat struct{ pass, total int }
	sections := map[string]*stat{}
	var order []string
	pass := 0
	var failures []int
	for _, ex := range examples {
		st := sections[ex.Section]
		if st == nil {
			st = &stat{}
			sections[ex.Section] = st
			order = append(order, ex.Section)
		}
		st.total++
		doc, _ := parseMarkdownBlocks(ex.Markdown)
		got := mdTestHTML(doc)
		if got == ex.HTML {
			pass++
			st.pass++
		} else if len(failures) < 2000 {
			failures = append(failures, ex.Example)
		}
	}
	for _, s := range order {
		st := sections[s]
		t.Logf("%-45s %3d/%3d", s, st.pass, st.total)
	}
	t.Logf("TOTAL %d/%d", pass, len(examples))

	// Block phase (pdf-go-fh4l.1): 341/652 — every remaining failure needs
	// the phase-2 inline parser (verified by eye over the per-section
	// diffs: emphasis, code spans, escapes, entities, inline HTML, hard
	// breaks in the expected output). Raised to the final target when
	// pdf-go-fh4l.2 lands.
	const floor = 341
	if pass < floor {
		t.Errorf("spec pass count %d below floor %d; failing examples: %v", pass, floor, failures)
	}
}
