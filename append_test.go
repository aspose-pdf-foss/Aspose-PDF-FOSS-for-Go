package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestDocumentAppend(t *testing.T) {
	outDir := filepath.Join(resultDir, "TestDocumentAppend")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	for _, group := range testGroups(t) {
		groupName := strings.Join(stems(group), "+")
		t.Run(groupName, func(t *testing.T) {
			if len(group) < 2 {
				t.Fatalf("group %q must have at least 2 files", groupName)
			}
			base, err := asposepdf.Open(group[0])
			if err != nil {
				t.Fatalf("Open %s: %v", group[0], err)
			}
			wantPages := base.PageCount()

			others := make([]*asposepdf.Document, 0, len(group)-1)
			for _, path := range group[1:] {
				doc, err := asposepdf.Open(path)
				if err != nil {
					t.Fatalf("Open %s: %v", path, err)
				}
				wantPages += doc.PageCount()
				others = append(others, doc)
			}

			base.Append(others...)
			if base.PageCount() != wantPages {
				t.Fatalf("expected %d pages, got %d", wantPages, base.PageCount())
			}

			outPath := filepath.Join(outDir, fmt.Sprintf("%s.pdf", groupName))
			if err := base.Save(outPath); err != nil {
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

// stems returns the stem (filename without extension) of each path.
func stems(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = stem(p)
	}
	return out
}
