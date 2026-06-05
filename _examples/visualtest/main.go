// visualtest renders a slice of PDFs from a folder to multi-page TIFFs for
// eyeballing the renderer's output. For each source file it makes a folder
// result_files/render/<filename>/ holding a copy of the source next to its
// rendered <stem>.tiff, so the original and the render sit side by side.
//
// Files in the folder root are sorted by name; the 1-based, inclusive index
// range [n1, n2] selects which to process.
//
// Usage:
//
//	go run ./_examples/visualtest <n1> <n2> <folder> [dpi]
//	go run ./_examples/visualtest 1 5 ./corpus        # first five, 150 DPI
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatalf("usage: go run ./_examples/visualtest <n1> <n2> <folder> [dpi]")
	}
	n1, err1 := strconv.Atoi(os.Args[1])
	n2, err2 := strconv.Atoi(os.Args[2])
	folder := os.Args[3]
	if err1 != nil || err2 != nil {
		log.Fatalf("n1 and n2 must be integers")
	}
	dpi := 150.0
	if len(os.Args) > 4 {
		if v, err := strconv.ParseFloat(os.Args[4], 64); err == nil && v > 0 {
			dpi = v
		}
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		log.Fatalf("read folder %q: %v", folder, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if n1 < 1 {
		n1 = 1
	}
	if n2 > len(files) {
		n2 = len(files)
	}
	if n1 > n2 {
		log.Fatalf("nothing to do: %d file(s) in %q, range [%d,%d]", len(files), folder, n1, n2)
	}

	outRoot := filepath.Join("result_files", "render")
	for i := n1; i <= n2; i++ {
		processOne(i, folder, files[i-1], outRoot, dpi)
	}
	fmt.Printf("done: files %d..%d of %d → %s\n", n1, n2, len(files), outRoot)
}

// processOne copies one source file and renders it to a multi-page TIFF beside
// it. Recovers from panics so one bad file does not abort the batch.
func processOne(i int, folder, name, outRoot string, dpi float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%d] %s: panic: %v", i, name, r)
		}
	}()

	src := filepath.Join(folder, name)
	dstDir := filepath.Join(outRoot, name)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		log.Printf("[%d] %s: mkdir: %v", i, name, err)
		return
	}
	if err := copyFile(src, filepath.Join(dstDir, name)); err != nil {
		log.Printf("[%d] %s: copy: %v", i, name, err)
	}

	doc, err := pdf.Open(src)
	if errors.Is(err, pdf.ErrEncrypted) {
		doc, err = pdf.OpenWithPassword(src, "") // many encrypted PDFs open with an empty user password
	}
	if err != nil {
		log.Printf("[%d] %s: open: %v (copied source only)", i, name, err)
		return
	}

	tiffPath := filepath.Join(dstDir, stem(name)+".tiff")
	f, err := os.Create(tiffPath)
	if err != nil {
		log.Printf("[%d] %s: create tiff: %v", i, name, err)
		return
	}
	defer f.Close()
	if err := doc.RenderTIFF(f, pdf.RenderOptions{DPI: dpi}); err != nil {
		log.Printf("[%d] %s: render: %v", i, name, err)
		return
	}
	fmt.Printf("[%d] %s — %d page(s) @ %.0f DPI → %s\n", i, name, doc.PageCount(), dpi, tiffPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func stem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
