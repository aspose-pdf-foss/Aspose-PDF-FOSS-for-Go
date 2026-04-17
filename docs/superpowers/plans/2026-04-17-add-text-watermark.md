# AddTextWatermark Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Document.AddTextWatermark` convenience method that applies a text watermark to all or selected pages using the existing `AddText` infrastructure.

**Architecture:** `AddTextWatermark` reuses `resolvePageIndices` for page selection (same pattern as `Rotate`), gets each page's MediaBox dimensions, builds a full-page `Rectangle`, and delegates to `page.AddText`. No new PDF operators — pure composition of existing building blocks.

**Tech Stack:** Pure Go, no new dependencies.

---

## File Structure

| File | Change |
|------|--------|
| `text_add.go` | Add `AddTextWatermark` method on `*Document` |
| `text_add_test.go` | 4 unit tests |
| `text_add_integration_test.go` | 1 integration test |
| `CLAUDE.md` | Add `AddTextWatermark` to API docs |

---

### Task 1: Implement AddTextWatermark with unit tests

**Files:**
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Write failing test for all-pages watermark**

Add to `text_add_test.go`:

```go
func TestAddTextWatermarkAllPages(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)
	// Now 3 pages.

	err := doc.AddTextWatermark("DRAFT", TextStyle{
		Font:     FontHelveticaBold,
		Size:     48,
		Rotation: 45,
		Behind:   true,
	})
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	for i := 0; i < doc.PageCount(); i++ {
		page, _ := doc.Page(i + 1)
		data, _ := page.contentStreams()
		content := string(data)
		if !strings.Contains(content, "(DRAFT) Tj") {
			t.Errorf("page %d missing watermark text", i+1)
		}
	}
}
```

- [ ] **Step 2: Write failing test for selected-pages watermark**

Add to `text_add_test.go`:

```go
func TestAddTextWatermarkSelectedPages(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)
	// Now 3 pages.

	err := doc.AddTextWatermark("SECRET", TextStyle{
		Font: FontHelvetica,
		Size: 36,
	}, 1, 3)
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	// Pages 1 and 3 should have watermark.
	for _, n := range []int{1, 3} {
		page, _ := doc.Page(n)
		data, _ := page.contentStreams()
		if !strings.Contains(string(data), "(SECRET) Tj") {
			t.Errorf("page %d should have watermark", n)
		}
	}

	// Page 2 should NOT have watermark.
	page2, _ := doc.Page(2)
	data2, _ := page2.contentStreams()
	if strings.Contains(string(data2), "(SECRET) Tj") {
		t.Error("page 2 should not have watermark")
	}
}
```

- [ ] **Step 3: Write failing test for invalid page number**

Add to `text_add_test.go`:

```go
func TestAddTextWatermarkInvalidPage(t *testing.T) {
	doc := NewDocument(595, 842)

	err := doc.AddTextWatermark("TEST", TextStyle{}, 0)
	if err == nil {
		t.Error("expected error for page 0")
	}

	err = doc.AddTextWatermark("TEST", TextStyle{}, 2)
	if err == nil {
		t.Error("expected error for page > PageCount")
	}
}
```

- [ ] **Step 4: Write failing test for empty text**

Add to `text_add_test.go`:

```go
func TestAddTextWatermarkEmpty(t *testing.T) {
	doc := NewDocument(595, 842)
	err := doc.AddTextWatermark("", TextStyle{})
	if err != nil {
		t.Fatalf("expected nil for empty text, got: %v", err)
	}
}
```

- [ ] **Step 5: Run tests to verify they fail**

Run: `go test -run "TestAddTextWatermark" -v ./...`
Expected: FAIL — `AddTextWatermark` method does not exist yet.

- [ ] **Step 6: Implement AddTextWatermark**

Add to `text_add.go`, after the `prependToContentStream` method and before `ensureFontResource`:

```go
// AddTextWatermark adds a text watermark to selected pages of the document.
// If no page numbers are given, the watermark is applied to all pages.
// Page numbers are 1-based. The watermark covers the full page area (MediaBox).
// The caller controls all styling via TextStyle (rotation, behind, color, etc.).
func (d *Document) AddTextWatermark(text string, style TextStyle, pageNums ...int) error {
	if text == "" {
		return nil
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return fmt.Errorf("add text watermark: %w", err)
	}
	for _, i := range indices {
		page := &Page{doc: d, index: i}
		size, err := page.Size()
		if err != nil {
			return fmt.Errorf("add text watermark: page %d: %w", i+1, err)
		}
		rect := Rectangle{LLX: 0, LLY: 0, URX: size.Width, URY: size.Height}
		if err := page.AddText(text, style, rect); err != nil {
			return fmt.Errorf("add text watermark: page %d: %w", i+1, err)
		}
	}
	return nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test -run "TestAddTextWatermark" -v ./...`
Expected: All 4 tests PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 8: Commit**

```bash
git add text_add.go text_add_test.go
git commit -m "feat: add AddTextWatermark convenience method on Document"
```

---

### Task 2: Integration test

**Files:**
- Modify: `text_add_integration_test.go`

- [ ] **Step 1: Write integration test**

Add to `text_add_integration_test.go`:

```go
func TestAddTextWatermarkRoundTrip(t *testing.T) {
	// Create a 2-page document with some content.
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	doc.AddBlankPageFromFormat(asposepdf.PageFormatA4)

	page1, _ := doc.Page(1)
	page1.AddText("Page one content", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})

	page2, _ := doc.Page(2)
	page2.AddText("Page two content", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})

	// Add watermark to all pages.
	gray := asposepdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3}
	err := doc.AddTextWatermark("CONFIDENTIAL", asposepdf.TextStyle{
		Font:     asposepdf.FontHelveticaBold,
		Size:     60,
		Color:    &gray,
		Rotation: 45,
		HAlign:   asposepdf.HAlignCenter,
		VAlign:   asposepdf.VAlignMiddle,
		Behind:   true,
	})
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	// Save.
	outDir := filepath.Join("result_files", "TestAddTextWatermarkRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Reopen and verify text on both pages.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) < 2 {
		t.Fatalf("expected 2 pages, got %d", len(texts))
	}
	for i, text := range texts {
		if !strings.Contains(text, "CONFIDENTIAL") {
			t.Errorf("page %d missing watermark text: %q", i+1, text)
		}
	}
	if !strings.Contains(texts[0], "Page one") {
		t.Errorf("page 1 missing original content: %q", texts[0])
	}
	if !strings.Contains(texts[1], "Page two") {
		t.Errorf("page 2 missing original content: %q", texts[1])
	}
}
```

- [ ] **Step 2: Run integration test**

Run: `go test -run TestAddTextWatermarkRoundTrip -v ./...`
Expected: PASS

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add text_add_integration_test.go
git commit -m "test: add AddTextWatermark round-trip integration test"
```

---

### Task 3: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

Find the line:
```
- `(*Page).AddText(text, style, rect) error` — draws text inside a rectangle with word wrap, alignment, clipping, optional underline/strikethrough, rotation, and behind-content mode
```

Add after it:
```
- `(*Document).AddTextWatermark(text, style, pageNums...) error` — applies a text watermark to all or selected pages using full-page rectangle from MediaBox
```

- [ ] **Step 2: Run tests**

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add AddTextWatermark to CLAUDE.md"
```
