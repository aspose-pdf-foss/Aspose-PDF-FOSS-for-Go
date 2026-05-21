# NewDocument Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `NewDocument` and `NewDocumentFromFormat` to create blank single-page PDFs with given dimensions or predefined page formats (A4, Letter, Legal, A3).

**Architecture:** Define `PageFormat` struct with predefined vars, `Landscape()` method, and two constructors that build a minimal `Document` with one empty page. Follows the same document construction pattern as `imageToDocumentFromBytes` in `image_convert.go`.

**Tech Stack:** Go standard library only (fmt for `formatFloat`)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `document_new.go` | `PageFormat`, predefined formats, `Landscape`, `NewDocument`, `NewDocumentFromFormat` |
| `document_new_test.go` | Unit tests (package `asposepdf`) |
| `document_new_integration_test.go` | Integration test (package `asposepdf_test`) |

---

### Task 1: `PageFormat` type, predefined formats, and `Landscape`

**Files:**
- Create: `document_new.go`
- Create: `document_new_test.go`

- [ ] **Step 1: Write the failing test for `PageFormat` and `Landscape`**

Create `document_new_test.go`:

```go
package asposepdf

import "testing"

func TestPageFormatLandscapeSwaps(t *testing.T) {
	portrait := PageFormat{Width: 595, Height: 842}
	landscape := portrait.Landscape()
	if landscape.Width != 842 || landscape.Height != 595 {
		t.Errorf("Landscape() = {%.0f, %.0f}, want {842, 595}", landscape.Width, landscape.Height)
	}
}

func TestPageFormatConstants(t *testing.T) {
	cases := []struct {
		name   string
		format PageFormat
		width  float64
		height float64
	}{
		{"A3", PageFormatA3, 842, 1191},
		{"A4", PageFormatA4, 595, 842},
		{"Letter", PageFormatLetter, 612, 792},
		{"Legal", PageFormatLegal, 612, 1008},
	}
	for _, tc := range cases {
		if tc.format.Width != tc.width || tc.format.Height != tc.height {
			t.Errorf("%s = {%.0f, %.0f}, want {%.0f, %.0f}",
				tc.name, tc.format.Width, tc.format.Height, tc.width, tc.height)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestPageFormat" -v ./...`
Expected: compilation error — `PageFormat` undefined.

- [ ] **Step 3: Implement `PageFormat`, predefined formats, and `Landscape`**

Create `document_new.go`:

```go
package asposepdf

// PageFormat describes a page size in points (1/72 inch).
type PageFormat struct {
	Width  float64
	Height float64
}

// Predefined page formats (portrait orientation).
var (
	PageFormatA3     = PageFormat{Width: 842, Height: 1191}
	PageFormatA4     = PageFormat{Width: 595, Height: 842}
	PageFormatLetter = PageFormat{Width: 612, Height: 792}
	PageFormatLegal  = PageFormat{Width: 612, Height: 1008}
)

// Landscape returns the format with width and height swapped.
func (f PageFormat) Landscape() PageFormat {
	return PageFormat{Width: f.Height, Height: f.Width}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestPageFormat" -v ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add document_new.go document_new_test.go
git commit -m "feat: add PageFormat type with predefined page sizes and Landscape"
```

---

### Task 2: `NewDocument` and `NewDocumentFromFormat`

**Files:**
- Modify: `document_new.go`
- Modify: `document_new_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `document_new_test.go`:

```go
func TestNewDocument(t *testing.T) {
	doc := NewDocument(595, 842)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestNewDocumentFromFormat(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestNewDocumentLandscape(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4.Landscape())
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width != 842 || size.Height != 595 {
		t.Errorf("size = {%.0f, %.0f}, want {842, 595}", size.Width, size.Height)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestNewDocument" -v ./...`
Expected: compilation error — `NewDocument` undefined.

- [ ] **Step 3: Implement `NewDocument` and `NewDocumentFromFormat`**

Add to `document_new.go`:

```go
// NewDocument creates a single-page blank document with the given dimensions in points.
func NewDocument(width, height float64) *Document {
	// Empty content stream.
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte{},
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 1, Value: contentStream}

	// Page dict.
	pageDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
		"/Contents":  pdfRef{Num: 1},
	}
	pageObj := &pdfObject{Num: 2, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: contentObj, 2: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  3,
	}
}

// NewDocumentFromFormat creates a single-page blank document using a predefined page format.
func NewDocumentFromFormat(format PageFormat) *Document {
	return NewDocument(format.Width, format.Height)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestNewDocument|TestPageFormat" -v ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add document_new.go document_new_test.go
git commit -m "feat: add NewDocument and NewDocumentFromFormat for blank PDFs"
```

---

### Task 3: Integration test

**Files:**
- Create: `document_new_integration_test.go`

- [ ] **Step 1: Write the integration test**

Create `document_new_integration_test.go`:

```go
package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestNewDocumentRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}

	outDir := filepath.Join("result_files", "TestNewDocumentRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "blank_a4.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and validate.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Verify dimensions survived round-trip.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.PageCount() != 1 {
		t.Fatalf("reopened PageCount() = %d, want 1", reopened.PageCount())
	}
	page, _ := reopened.Page(1)
	size, _ := page.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("reopened size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test -run TestNewDocumentRoundTrip -v ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add document_new_integration_test.go
git commit -m "test: add NewDocument round-trip integration test"
```

---

### Task 4: Update CLAUDE.md and README.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md**

Add to the `document.go` section of the Public API (after the `(*Document).RemoveUnusedObjects` entry and the `OptimizeImages` entries):

```markdown
- `PageFormat` struct — Width, Height in points; predefined: `PageFormatA3`, `PageFormatA4`, `PageFormatLetter`, `PageFormatLegal`
- `(PageFormat).Landscape()` — returns the format with width and height swapped
- `NewDocument(width, height) *Document` — creates a single-page blank document with given dimensions
- `NewDocumentFromFormat(format) *Document` — creates a single-page blank document from a predefined page format
```

- [ ] **Step 2: Update README.md**

Add `**Create blank documents**` to the Features list (after "Optimize images"):

```markdown
- **Create blank documents** — create single-page blank PDFs with custom dimensions or predefined page formats (A4, Letter, Legal, A3)
```

Add a new "Creating Blank Documents" section after "Optimizing Images":

```markdown
### Creating Blank Documents

```go
// From explicit dimensions (in points, 1/72 inch)
doc := pdf.NewDocument(595, 842)
doc.Save("blank.pdf")

// From predefined format
doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
doc.Save("a4.pdf")

// Landscape orientation
doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4.Landscape())
doc.Save("a4_landscape.pdf")
```
```

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: add NewDocument and PageFormat to CLAUDE.md and README.md"
```
