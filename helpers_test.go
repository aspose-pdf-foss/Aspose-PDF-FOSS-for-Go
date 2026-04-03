package asposepdf_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

const resultDir = "result_files"
const testdataDir = "testdata"

// Expected properties of specific test files used in assertions.
const (
	marketingPages = 2
	fourPagesCount = 4
	letterWidth    = 612.0
	letterHeight   = 792.0
)

// testGroups reads testdata/testfiles.json and returns the file groups for the
// current test (keyed by t.Name()). Each group is a slice of resolved file paths.
// One group = one logical test run; multiple groups = the test is run multiple times.
func testGroups(t *testing.T) [][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, "testfiles.json"))
	if err != nil {
		t.Fatalf("read testfiles.json: %v", err)
	}
	var cfg map[string][][]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse testfiles.json: %v", err)
	}
	groups, ok := cfg[t.Name()]
	if !ok {
		t.Fatalf("no entry for %q in testdata/testfiles.json", t.Name())
	}
	result := make([][]string, len(groups))
	for i, group := range groups {
		result[i] = make([]string, len(group))
		for j, f := range group {
			result[i][j] = filepath.Join(testdataDir, f)
		}
	}
	return result
}

// testFiles returns a flat list of file paths for the current test.
// Convenience wrapper for tests where every group contains exactly one file.
func testFiles(t *testing.T) []string {
	t.Helper()
	groups := testGroups(t)
	files := make([]string, len(groups))
	for i, g := range groups {
		if len(g) != 1 {
			t.Fatalf("testFiles: group %d of %q has %d files (expected 1)", i, t.Name(), len(g))
		}
		files[i] = g[0]
	}
	return files
}

// testFile returns the single file path configured for the current test.
// Convenience wrapper for tests with exactly one group containing one file.
func testFile(t *testing.T) string {
	t.Helper()
	files := testFiles(t)
	if len(files) != 1 {
		t.Fatalf("testFile: %q has %d groups (expected 1)", t.Name(), len(files))
	}
	return files[0]
}

// stem returns the filename without its extension.
func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// pageCountFromFile opens a PDF file and returns its page count.
func pageCountFromFile(t *testing.T, path string) int {
	t.Helper()
	doc, err := asposepdf.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	return doc.PageCount()
}
