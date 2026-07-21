// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestMarkdownExportRoundTrip renders a known Markdown document with our own
// renderer, exports it back with WriteMarkdown, and checks that the structure
// and inline markup survive the loop.
func TestMarkdownExportRoundTrip(t *testing.T) {
	src := `# Round Trip Title

Intro paragraph with **bold words** and *italic words* and a
[project link](https://example.com/repo) plus ` + "`inline code`" + `.

## Second Level

- first bullet
- second bullet

1. step one
2. step two

` + "```\nfunc main() {\n    call()\n}\n```" + `

### Third Level

Closing plain paragraph.
`
	doc, err := pdf.MarkdownToDocumentFromStream(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := doc.WriteMarkdown(&out); err != nil {
		t.Fatal(err)
	}
	md := out.String()

	for _, want := range []string{
		"# Round Trip Title",
		"## Second Level",
		"### Third Level",
		"**bold words**",
		"*italic words*",
		"[project link](https://example.com/repo)",
		"`inline code`",
		"- first bullet",
		"- second bullet",
		"1. step one",
		"2. step two",
		"```",
		"func main() {",
		"call()",
		"Closing plain paragraph.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("exported markdown missing %q\n--- got ---\n%s", want, md)
		}
	}
	// The code block keeps its indentation.
	if !strings.Contains(md, "    call()") && !strings.Contains(md, "   call()") {
		t.Errorf("code indentation lost:\n%s", md)
	}
}

// TestMarkdownExportImages: SaveMarkdown writes image files with relative
// links; EmbedImages produces data: URLs; a stream without a writer skips.
func TestMarkdownExportImages(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	writeTestPNG(t, imgPath)

	src := "# With Image\n\nBefore.\n\n![alt](pic.png)\n\nAfter.\n"
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.MarkdownToDocument(mdPath)
	if err != nil {
		t.Fatal(err)
	}

	// File mode (default): images land in <stem>_files with relative links.
	outPath := filepath.Join(dir, "out.md")
	if err := doc.SaveMarkdown(outPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "![](out_files/p1_img1.png)") {
		t.Errorf("file-mode link missing:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "out_files", "p1_img1.png")); err != nil {
		t.Errorf("image file not written: %v", err)
	}

	// Embed mode: data: URL.
	var embedded strings.Builder
	if err := doc.WriteMarkdown(&embedded, pdf.MarkdownSaveOptions{EmbedImages: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(embedded.String(), "![](data:image/png;base64,") {
		t.Error("embed mode did not produce a data: URL")
	}

	// Stream without a writer: images skipped.
	var plain strings.Builder
	if err := doc.WriteMarkdown(&plain); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "![](") {
		t.Error("stream mode without writer must skip images")
	}
	if !strings.Contains(plain.String(), "Before.") || !strings.Contains(plain.String(), "After.") {
		t.Error("text around the image lost")
	}
}

// TestMarkdownExportEscaping: literal Markdown-special characters in the PDF
// text must come back escaped, not as live markup.
func TestMarkdownExportEscaping(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Price is 5 * 3 [approx] under_score", pdf.TextStyle{Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := doc.WriteMarkdown(&out); err != nil {
		t.Fatal(err)
	}
	md := out.String()
	for _, want := range []string{`5 \* 3`, `\[approx\]`, `under\_score`} {
		if !strings.Contains(md, want) {
			t.Errorf("missing escaped form %q in:\n%s", want, md)
		}
	}
}

// writeTestPNG writes a small opaque PNG.
func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = 0x7F
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
