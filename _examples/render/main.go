// render rasterizes every page of a PDF to a PNG under result_files/render/.
// Defaults to the unencrypted showcase; pass a path to render another file.
//
// Usage:
//
//	go run ./_examples/render                 # docs/feature_showcase_unencrypted.pdf
//	go run ./_examples/render <file.pdf> [dpi]
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	src := "docs/feature_showcase_unencrypted.pdf"
	if len(os.Args) > 1 {
		src = os.Args[1]
	}
	dpi := 120.0
	if len(os.Args) > 2 {
		if v, err := strconv.ParseFloat(os.Args[2], 64); err == nil {
			dpi = v
		}
	}

	doc, err := pdf.Open(src)
	if err != nil {
		log.Fatalf("open %q: %v", src, err)
	}
	outDir := filepath.Join("result_files", "render")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	for i := 1; i <= doc.PageCount(); i++ {
		out := filepath.Join(outDir, fmt.Sprintf("page-%02d.png", i))
		renderOne(doc, i, dpi, out)
	}
	fmt.Printf("rendered %d page(s) at %.0f DPI → %s\n", doc.PageCount(), dpi, outDir)
}

// renderOne renders a single page, recovering from any panic so one bad page
// does not abort the whole run.
func renderOne(doc *pdf.Document, n int, dpi float64, out string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("page %d: panic: %v", n, r)
		}
	}()
	p, err := doc.Page(n)
	if err != nil {
		log.Printf("page %d: %v", n, err)
		return
	}
	f, err := os.Create(out)
	if err != nil {
		log.Printf("page %d: %v", n, err)
		return
	}
	defer f.Close()
	if err := p.RenderPNG(f, pdf.RenderOptions{DPI: dpi}); err != nil {
		log.Printf("page %d: render: %v", n, err)
	}
}
