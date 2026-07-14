// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarkdownTaggedPDFUA: MarkdownOptions.Tagged output must pass the
// library's PDF/UA validator (structure tree, language, displayed title,
// figure alt text).
func TestMarkdownTaggedPDFUA(t *testing.T) {
	dir := t.TempDir()

	// A small local image for the figure-with-alt check.
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	for i := range img.Pix {
		img.Pix[i] = 0xCC
	}
	img.Set(3, 3, color.RGBA{R: 200, A: 255})
	f, err := os.Create(filepath.Join(dir, "pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	md := `# Accessible Report

Body **text** with a [link](https://example.com/) and ` + "`code`" + `.

- one item
- [x] done task

![A tiny gray square](pic.png)

| A | B |
|---|---|
| 1 | 2 |

> A quoted line.

` + "```\nindented code\n```\n"
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := MarkdownToDocument(path, MarkdownOptions{Tagged: true})
	if err != nil {
		t.Fatal(err)
	}
	rep := doc.ValidatePDFUA()
	if !rep.Conformant {
		for _, iss := range rep.Issues {
			t.Errorf("PDF/UA issue %s: %s", iss.Rule, iss.Message)
		}
	}

	// Title derived from the first heading.
	info, err := doc.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Accessible Report" {
		t.Errorf("Info title = %q; want first heading", info.Title)
	}

	// Conformance survives Save + Open.
	var sb strings.Builder
	if _, err := doc.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	rep = reopened.ValidatePDFUA()
	if !rep.Conformant {
		for _, iss := range rep.Issues {
			t.Errorf("after round-trip, PDF/UA issue %s: %s", iss.Rule, iss.Message)
		}
	}
}
