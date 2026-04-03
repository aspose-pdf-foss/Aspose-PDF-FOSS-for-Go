package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

const appendTestData = "testdata/append"

func TestDocumentAppend(t *testing.T) {
	entries, err := os.ReadDir(appendTestData)
	if err != nil {
		t.Fatalf("read %s: %v", appendTestData, err)
	}

	// Collect openable file paths.
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(appendTestData, e.Name())
		doc, err := asposepdf.Open(p)
		if err != nil {
			t.Logf("skipping %s: %v", e.Name(), err)
			continue
		}
		_ = doc
		paths = append(paths, p)
	}
	if len(paths) < 2 {
		t.Skipf("need at least 2 openable files in %s, got %d", appendTestData, len(paths))
	}

	outDir := filepath.Join(resultDir, "TestDocumentAppend")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Merge each file with each other file (once per pair).
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			pa, pb := paths[i], paths[j]
			t.Run(fmt.Sprintf("%s+%s", stem(pa), stem(pb)), func(t *testing.T) {
				a, err := asposepdf.Open(pa)
				if err != nil {
					t.Fatalf("Open a: %v", err)
				}
				b, err := asposepdf.Open(pb)
				if err != nil {
					t.Fatalf("Open b: %v", err)
				}
				want := a.PageCount() + b.PageCount()
				a.Append(b)
				if a.PageCount() != want {
					t.Fatalf("expected %d pages, got %d", want, a.PageCount())
				}

				outPath := filepath.Join(outDir, fmt.Sprintf("%s+%s.pdf", stem(pa), stem(pb)))
				if err := a.Save(outPath); err != nil {
					t.Fatalf("Save: %v", err)
				}

				report, err := asposepdf.Validate(outPath)
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				checkValidation(t, outPath, report)
			})
		}
	}

	// Merge all openable files into one document.
	t.Run("all", func(t *testing.T) {
		docs := make([]*asposepdf.Document, len(paths))
		wantPages := 0
		for i, p := range paths {
			d, err := asposepdf.Open(p)
			if err != nil {
				t.Fatalf("Open %s: %v", p, err)
			}
			docs[i] = d
			wantPages += d.PageCount()
		}

		docs[0].Append(docs[1:]...)
		if docs[0].PageCount() != wantPages {
			t.Fatalf("expected %d pages, got %d", wantPages, docs[0].PageCount())
		}

		outPath := filepath.Join(outDir, "all.pdf")
		if err := docs[0].Save(outPath); err != nil {
			t.Fatalf("Save: %v", err)
		}

		report, err := asposepdf.Validate(outPath)
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
		checkValidation(t, outPath, report)
		t.Logf("merged %d files → %d pages", len(docs), docs[0].PageCount())
	})
}

func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
