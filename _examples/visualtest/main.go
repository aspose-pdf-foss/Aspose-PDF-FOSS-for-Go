// visualtest renders a slice of PDFs from a corpus folder to multi-page TIFFs
// for eyeballing the renderer's output against a reference viewer (Acrobat).
//
// For each source PDF it makes a folder result_files/render/<filename>/, MOVES
// the source PDF into it (it does not copy), and renders <stem>.tiff beside it,
// so the original and the render sit side by side.
//
// The move is deliberate and drives a simple review workflow: a processed file
// leaves the corpus, so it is never picked up again. After you compare a render
// against Acrobat, delete result_files/render/<filename>/ — that deletion means
// "verified OK". Folders you leave behind are the ones still needing work. The
// source is moved before rendering, so a file that fails to open or panics
// mid-render is still preserved in its folder rather than re-checked next run.
//
// Only *.pdf files in the folder root are considered (no recursion). They are
// sorted by name; the 1-based, inclusive index range [n1, n2] selects which of
// the *remaining* PDFs to process.
//
// The corpus folder comes from the [folder] argument, or the VISUALTEST_CORPUS
// environment variable when the argument is omitted (so no local path is baked
// into the source).
//
// Usage:
//
//	go run ./_examples/visualtest <n1> <n2> [folder] [dpi]
//	go run ./_examples/visualtest 1 10 ./corpus      # explicit folder
//	go run ./_examples/visualtest 1 10 ./corpus 200  # explicit DPI
//	VISUALTEST_CORPUS=/path/to/corpus go run ./_examples/visualtest 1 5
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
	if len(os.Args) < 3 {
		log.Fatalf("usage: go run ./_examples/visualtest <n1> <n2> [folder] [dpi]")
	}
	n1, err1 := strconv.Atoi(os.Args[1])
	n2, err2 := strconv.Atoi(os.Args[2])
	if err1 != nil || err2 != nil {
		log.Fatalf("n1 and n2 must be integers")
	}
	folder := ""
	if len(os.Args) > 3 {
		folder = os.Args[3]
	} else {
		folder = os.Getenv("VISUALTEST_CORPUS")
	}
	if folder == "" {
		log.Fatalf("no corpus folder: pass it as the 3rd argument or set VISUALTEST_CORPUS")
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
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue // PDFs only
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	if len(files) == 0 {
		fmt.Printf("no PDFs left in %s — all verified?\n", folder)
		return
	}
	if n1 < 1 {
		n1 = 1
	}
	if n2 > len(files) {
		n2 = len(files)
	}
	if n1 > n2 {
		log.Fatalf("nothing to do: %d PDF(s) left in %q, range [%d,%d]", len(files), folder, n1, n2)
	}

	outRoot := filepath.Join("result_files", "render")
	for i := n1; i <= n2; i++ {
		processOne(i, folder, files[i-1], outRoot, dpi)
	}
	fmt.Printf("done: PDFs %d..%d of %d remaining in %s → %s\n", n1, n2, len(files), folder, outRoot)
}

// processOne moves one source PDF into its own folder and renders it to a
// multi-page TIFF beside it. The move happens first so a file is never left in
// the corpus to be re-checked, even if opening or rendering fails. Recovers
// from panics so one bad file does not abort the batch.
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

	// Move the source out of the corpus and into its review folder first.
	moved := filepath.Join(dstDir, name)
	if err := moveFile(src, moved); err != nil {
		log.Printf("[%d] %s: move: %v", i, name, err)
		return
	}

	doc, err := pdf.Open(moved)
	if errors.Is(err, pdf.ErrEncrypted) {
		doc, err = pdf.OpenWithPassword(moved, "") // many encrypted PDFs open with an empty user password
	}
	if err != nil {
		log.Printf("[%d] %s: open: %v (source preserved in %s)", i, name, err, dstDir)
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

// moveFile renames src to dst, falling back to copy+remove when src and dst sit
// on different volumes (os.Rename returns an error there).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
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
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func stem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
