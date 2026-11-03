// SPDX-License-Identifier: MIT

// content_sweep runs operations that should not change what a page says or shows
// over a folder of PDFs, and compares the text and the pixels before and after.
// Validators judge conformance, not preservation: a document can conform with its
// text gone, so this is what finds that. Run it before a release, and after any
// change to a conversion:
//
//	go run ./_examples/content_sweep -corpus path/to/pdfs -every 3
//	go run ./_examples/content_sweep -corpus path/to/pdfs -ops flatten,pdfa2b
//
// "pixels" lines list the pages that differ most: look at them, since a substitute
// font's letterforms differ too. "text" lines list words that went missing; a space
// more or less between words is not counted. pdfa1b rasterizes pages with real
// transparency by design, so thin lines there look paler (see RasterizedPages).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// pagesCompared caps the work per document; damage shows on the first pages.
const pagesCompared = 4

// darkChange is how far a pixel's brightness must move, out of 255, to count: it
// leaves antialiasing out and keeps glyphs that went missing or moved.
const darkChange = 90

type operation func(d *asposepdf.Document) error

var operations = map[string]operation{
	"resave": func(*asposepdf.Document) error { return nil },
	"optimize": func(d *asposepdf.Document) error {
		_, err := d.Optimize(asposepdf.DefaultOptimizationOptions())
		return err
	},
	"pdfa1b": func(d *asposepdf.Document) error {
		_, err := d.ConvertToPDFA(asposepdf.PDFA1B)
		return err
	},
	"pdfa2b": func(d *asposepdf.Document) error {
		_, err := d.ConvertToPDFA(asposepdf.PDFA2B)
		return err
	},
	"pdfua": func(d *asposepdf.Document) error {
		_, err := d.ConvertToPDFUA()
		return err
	},
	"autotag": func(d *asposepdf.Document) error {
		_, err := d.AutoTag(asposepdf.DefaultAutoTaggingSettings())
		return err
	},
	"flatten": func(d *asposepdf.Document) error { return d.Flatten() },
}

type result struct {
	file   string
	page   int
	diff   float64 // the worst page's share of pixels that changed
	lost   int     // words missing afterwards
	sample string
}

func main() {
	corpus := flag.String("corpus", "", "folder of PDFs, walked recursively (required)")
	ops := flag.String("ops", "resave,optimize,pdfa1b,pdfa2b,pdfua,autotag,flatten", "operations to compare")
	every := flag.Int("every", 1, "take every Nth file")
	maxSize := flag.Int64("maxsize", 8<<20, "skip larger files, in bytes")
	timeout := flag.Duration("timeout", 45*time.Second, "give up on a document after this long")
	flag.Parse()
	if *corpus == "" {
		flag.Usage()
		os.Exit(2)
	}
	var files []string
	err := filepath.WalkDir(*corpus, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range strings.Split(*ops, ",") {
		op, ok := operations[name]
		if !ok {
			log.Fatalf("unknown operation %q", name)
		}
		report(name, sweep(files, op, *every, *maxSize, *timeout))
	}
}

func sweep(files []string, op operation, every int, maxSize int64, timeout time.Duration) []result {
	var all []result
	for i, f := range files {
		if i%every != 0 {
			continue
		}
		if st, err := os.Stat(f); err != nil || st.Size() > maxSize {
			continue
		}
		done := make(chan *result, 1)
		go func() { done <- compareOne(f, op) }()
		select {
		case r := <-done:
			if r != nil {
				all = append(all, *r)
			}
		case <-time.After(timeout):
			log.Printf("timeout: %s", f)
		}
	}
	return all
}

// compareOne returns nil when the document does not apply: it does not open, or the
// operation refuses it (AutoTag on a tagged file).
func compareOne(path string, op operation) (res *result) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %s: %v", path, r)
			res = nil
		}
	}()
	d, err := asposepdf.Open(path)
	if err != nil {
		return nil
	}
	n := min(d.PageCount(), pagesCompared)
	text := make([]string, n)
	img := make([]image.Image, n)
	for i := range n {
		p, err := d.Page(i + 1)
		if err != nil {
			return nil
		}
		text[i], _ = p.ExtractText()
		img[i], _ = p.RenderImage(asposepdf.RenderOptions{DPI: 50})
	}
	if err := op(d); err != nil {
		return nil
	}
	r := &result{file: filepath.Base(path)}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		r.lost, r.sample = 1, "cannot save: "+err.Error()
		return r
	}
	re, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		r.lost, r.sample = 1, "cannot reopen: "+err.Error()
		return r
	}
	if re.PageCount() < n {
		r.lost, r.sample = 1, fmt.Sprintf("only %d pages left", re.PageCount())
		return r
	}
	for i := range n {
		p, err := re.Page(i + 1)
		if err != nil {
			continue
		}
		after, _ := p.ExtractText()
		countLost(r, text[i], after)
		if rendered, _ := p.RenderImage(asposepdf.RenderOptions{DPI: 50}); rendered != nil && img[i] != nil {
			if d := pixelChange(img[i], rendered); d > r.diff {
				r.diff, r.page = d, i+1
			}
		}
	}
	return r
}

func countLost(r *result, before, after string) {
	squeeze := func(s string) string { return strings.Join(strings.Fields(s), "") }
	if squeeze(before) == squeeze(after) {
		return // only the spacing moved
	}
	have := map[string]int{}
	for _, w := range strings.Fields(after) {
		have[w]++
	}
	for _, w := range strings.Fields(before) {
		if have[w] > 0 {
			have[w]--
			continue
		}
		r.lost++
		if r.sample == "" {
			r.sample = w
		}
	}
}

func pixelChange(a, b image.Image) float64 {
	bounds := a.Bounds()
	if bounds != b.Bounds() {
		return 1
	}
	changed := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			if d := int(ar+ag+ab)/3>>8 - int(br+bg+bb)/3>>8; d > darkChange || d < -darkChange {
				changed++
			}
		}
	}
	return float64(changed) / float64(bounds.Dx()*bounds.Dy())
}

func report(name string, all []result) {
	fmt.Printf("=== %s: %d documents\n", name, len(all))
	sort.Slice(all, func(i, j int) bool { return all[i].diff > all[j].diff })
	for _, r := range all[:min(10, len(all))] {
		if r.diff > 0.001 {
			fmt.Printf("  pixels %-36s page %d changed %.1f%%\n", r.file, r.page, r.diff*100)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].lost > all[j].lost })
	for _, r := range all[:min(10, len(all))] {
		if r.lost > 0 {
			fmt.Printf("  text   %-36s %d missing, such as %q\n", r.file, r.lost, r.sample)
		}
	}
}
