// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestAutoTagCorpus runs AutoTag over a folder of PDFs (AUTOTAG_CORPUS) and
// checks, per document: no panic or error, extracted text unchanged, every
// page of documents up to 10 pages rendering pixel-identically, and the
// saved file's ValidatePDFUA result (figures without alt text aside).
// Problems are listed in result_files/autotag_corpus_problems.txt.
func TestAutoTagCorpus(t *testing.T) {
	dir := os.Getenv("AUTOTAG_CORPUS")
	if dir == "" {
		t.Skip("set AUTOTAG_CORPUS")
	}
	var files []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	stats := map[string]int{}
	var problems []string
	for _, f := range files {
		func() {
			defer func() {
				if r := recover(); r != nil {
					stats["panic"]++
					problems = append(problems, fmt.Sprintf("PANIC %s: %v", f, r))
				}
			}()
			doc, err := Open(f)
			if err != nil {
				stats["open-fail"]++
				return
			}
			if doc.PageCount() > 60 {
				stats["skipped-large"]++
				return
			}
			before, err := doc.ExtractText()
			if err != nil {
				stats["extract-fail"]++
				return
			}
			doRender := doc.PageCount() <= 10
			var pix [][]byte
			if doRender {
				for i := 1; i <= doc.PageCount(); i++ {
					img, err := doc.RenderImage(i, RenderOptions{DPI: 50})
					if err != nil {
						doRender = false
						break
					}
					pix = append(pix, append([]byte(nil), img.(*image.RGBA).Pix...))
				}
			}
			start := time.Now()
			res, err := doc.AutoTag(DefaultAutoTaggingSettings())
			el := time.Since(start)
			if el > 20*time.Second {
				problems = append(problems, fmt.Sprintf("SLOW %s: %v", f, el))
			}
			if errors.Is(err, ErrAlreadyTagged) {
				stats["already-tagged"]++
				return
			}
			if err != nil {
				stats["error"]++
				problems = append(problems, fmt.Sprintf("ERR %s: %v", f, err))
				return
			}
			stats["tagged"]++
			_ = res
			after, err := doc.ExtractText()
			if err != nil {
				problems = append(problems, fmt.Sprintf("EXTRACT-AFTER %s: %v", f, err))
				return
			}
			for i := range before {
				if before[i] != after[i] {
					stats["text-changed"]++
					problems = append(problems, fmt.Sprintf("TEXT %s p%d", f, i+1))
					break
				}
			}
			if doRender {
				stats["rendered"]++
				for i := 1; i <= doc.PageCount(); i++ {
					img, _ := doc.RenderImage(i, RenderOptions{DPI: 50})
					if !bytes.Equal(img.(*image.RGBA).Pix, pix[i-1]) {
						stats["render-changed"]++
						problems = append(problems, fmt.Sprintf("RENDER %s p%d", f, i))
						break
					}
				}
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				problems = append(problems, fmt.Sprintf("WRITE %s: %v", f, err))
				return
			}
			re, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				problems = append(problems, fmt.Sprintf("REOPEN %s: %v", f, err))
				return
			}
			ok := true
			for _, is := range re.ValidatePDFUA().Issues {
				if is.Rule != "UA_FIGURE_NO_ALT" {
					ok = false
					stats["ua:"+is.Rule]++
				}
			}
			if ok {
				stats["ua-ok(except-alt)"]++
			}
		}()
	}
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-28s %d", k, stats[k])
	}
	os.WriteFile("result_files/autotag_corpus_problems.txt", []byte(strings.Join(problems, "\n")), 0o644)
	t.Logf("%d files, %d problems (result_files/autotag_corpus_problems.txt)", len(files), len(problems))
}
