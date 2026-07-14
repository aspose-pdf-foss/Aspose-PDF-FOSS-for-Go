// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mdSample = `# Project Report

An **important** paragraph with *emphasis*, ` + "`inline code`" + `, ~~old~~ and
a [link](https://example.com/docs).

## Details

- first bullet
- second bullet with **bold**
  - nested child
1. step one
2. step two

- [x] done task
- [ ] open task

> A quoted wisdom line,
> spanning two lines.

` + "```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```" + `

| Name | Qty | Price |
|:-----|:---:|------:|
| Foo  | 2   | 10.50 |
| Bar  | 1   | 3.00  |

---

Final paragraph after a rule.
`

func TestMarkdownToDocumentEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte(mdSample), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := MarkdownToDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PageCount() < 1 {
		t.Fatal("no pages")
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	text, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Project Report", "important", "inline code", "first bullet",
		"nested child", "step two", "done task", "quoted wisdom",
		"func main()", "Price", "10.50", "Final paragraph",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Heading is larger and bolder than body text.
	lines, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var h1Size, bodySize float64
	for _, ln := range lines {
		for _, fr := range ln.Fragments {
			if strings.Contains(fr.Text, "Project Report") {
				h1Size = fr.FontSize
			}
			if strings.Contains(fr.Text, "Final paragraph") {
				bodySize = fr.FontSize
			}
		}
	}
	if h1Size <= bodySize || bodySize == 0 {
		t.Errorf("h1 size %g vs body %g; want h1 larger", h1Size, bodySize)
	}

	// The link run carries a real annotation.
	var found bool
	for _, a := range page.Annotations().All() {
		if l, ok := a.(*LinkAnnotation); ok {
			if act, ok := l.Action().(*GoToURIAction); ok && act.URI() == "https://example.com/docs" {
				found = true
			}
		}
	}
	if !found {
		t.Error("no link annotation for the markdown link")
	}

	// Round-trip: output is a valid PDF.
	var sb strings.Builder
	if _, err := doc.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.PageCount() != doc.PageCount() {
		t.Errorf("round-trip page count %d != %d", reopened.PageCount(), doc.PageCount())
	}
}

func TestPageAddMarkdownBoxed(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := Rectangle{LLX: 60, LLY: 640, URX: 380, URY: 760}
	// Long content: must clip inside the rect without error.
	md := "## Boxed\n\n" + strings.Repeat("Repeated boxed sentence. ", 60)
	if err := p.AddMarkdown(md, rect); err != nil {
		t.Fatal(err)
	}
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Boxed") {
		t.Errorf("boxed markdown missing: %q", text)
	}
}

func TestFlowAddMarkdownMixed(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	flow := doc.NewFlow(FlowOptions{})
	flow.AddParagraph("plain flow paragraph", TextStyle{Size: 11})
	flow.AddMarkdown("### Md heading\n\nBody with **bold**.")
	flow.AddSpacer(10)
	flow.AddMarkdown("1. one\n2. two")
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"plain flow paragraph", "Md heading", "bold", "two"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
}

// TestMarkdownFontFamily: FontFamily resolves and embeds an installed family,
// enabling full-Unicode (Cyrillic) Markdown through the one-line API.
func TestMarkdownFontFamily(t *testing.T) {
	probe := NewDocumentFromFormat(PageFormatA4)
	if _, err := probe.LoadFontByName("Arial", false, false); err != nil {
		t.Skipf("Arial not installed: %v", err)
	}
	doc, err := MarkdownToDocumentFromStream(
		strings.NewReader("# Отчёт\n\nКириллица с **жирным** и стрелкой →.\n"),
		MarkdownOptions{FontFamily: "Arial"},
	)
	if err != nil {
		t.Fatal(err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Отчёт", "Кириллица", "жирным", "→"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q; got %q", want, text)
		}
	}
	if strings.Contains(text, "?") {
		t.Errorf("replacement '?' leaked into output: %q", text)
	}
}

func TestMarkdownPagination(t *testing.T) {
	md := "# Long\n\n" + strings.Repeat("A reasonably long paragraph of body text that wraps across several lines when rendered. ", 4)
	md = strings.Repeat(md+"\n\n", 30)
	doc, err := MarkdownToDocumentFromStream(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	if doc.PageCount() < 2 {
		t.Errorf("pages = %d; want pagination", doc.PageCount())
	}
}
