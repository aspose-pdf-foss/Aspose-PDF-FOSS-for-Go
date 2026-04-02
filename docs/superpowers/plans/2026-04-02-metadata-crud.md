# Metadata CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `SetMetadata`/`ClearMetadata` to `Document`, support custom fields in `Metadata`, and remove the standalone `GetMetadata` function.

**Architecture:** `metadataConfig` (nil | clear | Metadata) stored on `Document`, same pattern as `encryptConfig`. Writer serializes it as an `/Info` object and references it from the trailer. Reading populates `Custom` from non-standard keys in the Info dict.

**Tech Stack:** Pure Go, no dependencies. Tests with `go test ./...`.

---

## File Map

| File | Change |
|------|--------|
| `metadata.go` | Add `Custom` to `Metadata`; add `metadataConfig` type; add `SetMetadata`, `ClearMetadata`, `buildInfoDict`; remove `GetMetadata` |
| `document.go` | Add `metadataConfig *metadataConfig` field to `Document`; update `withCopiedPatches` |
| `writer.go` | Add `metaCfg *metadataConfig` param to `buildDocumentPDF`; write Info obj + trailer ref |
| `metadata_test.go` | Replace `TestGetMetadata` with `Open`+`Metadata()`; add write/clear/custom tests |

---

## Task 1: Add `Custom` field to `Metadata` and populate it on read

**Files:**
- Modify: `metadata.go`
- Modify: `metadata_test.go`

- [ ] **Step 1: Write the failing test**

Add to `metadata_test.go`:

```go
func TestMetadataCustomFieldsRoundTrip(t *testing.T) {
	// 4pages.pdf has no custom fields — Custom must be nil or empty.
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	meta, err := doc.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(meta.Custom) != 0 {
		t.Errorf("expected no custom fields, got %v", meta.Custom)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```
go test -run TestMetadataCustomFieldsRoundTrip ./...
```

Expected: FAIL — `Metadata` struct has no `Custom` field.

- [ ] **Step 3: Add `Custom` to `Metadata` struct**

In `metadata.go`, change the struct:

```go
type Metadata struct {
	Title        string
	Author       string
	Subject      string
	Keywords     string
	Creator      string
	Producer     string
	CreationDate string
	ModDate      string
	Custom       map[string]string // arbitrary Info dict entries
}
```

- [ ] **Step 4: Update `readMetadata` to populate `Custom`**

Replace the `readMetadata` function body with:

```go
func readMetadata(doc *rawDocument) (Metadata, error) {
	infoRef, ok := doc.trailer["/Info"]
	if !ok {
		return Metadata{}, nil
	}
	infoDict, err := doc.resolveDict(infoRef)
	if err != nil {
		return Metadata{}, fmt.Errorf("read Info dict: %w", err)
	}

	standardKeys := map[string]bool{
		"/Title": true, "/Author": true, "/Subject": true, "/Keywords": true,
		"/Creator": true, "/Producer": true, "/CreationDate": true, "/ModDate": true,
	}
	var custom map[string]string
	for k, v := range infoDict {
		if standardKeys[k] {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			if custom == nil {
				custom = make(map[string]string)
			}
			custom[strings.TrimPrefix(k, "/")] = s
		}
	}

	return Metadata{
		Title:        infoString(infoDict, "/Title"),
		Author:       infoString(infoDict, "/Author"),
		Subject:      infoString(infoDict, "/Subject"),
		Keywords:     infoString(infoDict, "/Keywords"),
		Creator:      infoString(infoDict, "/Creator"),
		Producer:     infoString(infoDict, "/Producer"),
		CreationDate: infoString(infoDict, "/CreationDate"),
		ModDate:      infoString(infoDict, "/ModDate"),
		Custom:       custom,
	}, nil
}
```

Add `"strings"` to the import in `metadata.go`.

- [ ] **Step 5: Run test to verify it passes**

```
go test -run TestMetadataCustomFieldsRoundTrip ./...
```

Expected: PASS.

- [ ] **Step 6: Run full suite**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add metadata.go metadata_test.go
git commit -m "Add Metadata.Custom field; populate from non-standard Info dict keys"
```

---

## Task 2: Add `SetMetadata` and `ClearMetadata` (method stubs + Document field)

**Files:**
- Modify: `document.go`
- Modify: `metadata.go`

- [ ] **Step 1: Add `metadataConfig` type and `buildInfoDict` to `metadata.go`**

Append to `metadata.go` (before the closing of the file):

```go
// metadataConfig holds the metadata to write on Save, or a clear flag.
type metadataConfig struct {
	meta  Metadata
	clear bool // if true, omit the Info dictionary entirely
}

// buildInfoDict converts a Metadata value into a pdfDict for the Info object.
// Fields with empty string values are omitted. Custom keys are prefixed with "/".
// Custom keys that duplicate standard field names are ignored.
func buildInfoDict(meta Metadata) pdfDict {
	d := make(pdfDict)
	pairs := [][2]string{
		{"/Title", meta.Title},
		{"/Author", meta.Author},
		{"/Subject", meta.Subject},
		{"/Keywords", meta.Keywords},
		{"/Creator", meta.Creator},
		{"/Producer", meta.Producer},
		{"/CreationDate", meta.CreationDate},
		{"/ModDate", meta.ModDate},
	}
	for _, kv := range pairs {
		if kv[1] != "" {
			d[kv[0]] = kv[1]
		}
	}
	standardNames := map[string]bool{
		"Title": true, "Author": true, "Subject": true, "Keywords": true,
		"Creator": true, "Producer": true, "CreationDate": true, "ModDate": true,
	}
	for k, v := range meta.Custom {
		if v != "" && !standardNames[k] {
			d["/"+k] = v
		}
	}
	return d
}
```

- [ ] **Step 2: Add `metadataConfig` field to `Document` and update `withCopiedPatches`**

In `document.go`, change the `Document` struct:

```go
type Document struct {
	pages         []pageRef
	patches       map[patchKey]pdfDict
	encryptConfig *encryptConfig
	metadataConfig *metadataConfig
}
```

Update `withCopiedPatches`:

```go
func (d *Document) withCopiedPatches() *Document {
	return &Document{
		pages:          append([]pageRef{}, d.pages...),
		patches:        copyPatches(d.patches),
		encryptConfig:  d.encryptConfig,
		metadataConfig: d.metadataConfig,
	}
}
```

- [ ] **Step 3: Add `SetMetadata` and `ClearMetadata` to `metadata.go`**

Append to `metadata.go`:

```go
// SetMetadata returns a new Document configured to write meta as the PDF Info
// dictionary when saved. Empty string fields are omitted. This is a full
// replacement: any metadata from the source document is discarded on save.
//
// To update a single field, read the current metadata, modify it, and call
// SetMetadata with the updated struct:
//
//	meta, _ := doc.Metadata()
//	meta.Title = "New Title"
//	doc = doc.SetMetadata(meta)
func (d *Document) SetMetadata(meta Metadata) *Document {
	result := d.withCopiedPatches()
	result.metadataConfig = &metadataConfig{meta: meta}
	return result
}

// ClearMetadata returns a new Document configured to omit the Info dictionary
// entirely when saved. Use this to strip all metadata before publishing.
func (d *Document) ClearMetadata() *Document {
	result := d.withCopiedPatches()
	result.metadataConfig = &metadataConfig{clear: true}
	return result
}
```

- [ ] **Step 4: Build check**

```
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add metadata.go document.go
git commit -m "Add SetMetadata, ClearMetadata and metadataConfig to Document"
```

---

## Task 3: Writer — serialize `metadataConfig` into the output PDF

**Files:**
- Modify: `writer.go`
- Modify: `document.go` (update `WriteTo` call)
- Modify: `metadata_test.go` (add round-trip tests before implementing)

- [ ] **Step 1: Write failing tests**

Add to `metadata_test.go`:

```go
func TestSetMetadataRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := asposepdf.Metadata{
		Title:   "Test Title",
		Author:  "Test Author",
		Subject: "Test Subject",
	}
	doc = doc.SetMetadata(want)

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Title != want.Title {
		t.Errorf("Title: got %q, want %q", got.Title, want.Title)
	}
	if got.Author != want.Author {
		t.Errorf("Author: got %q, want %q", got.Author, want.Author)
	}
	if got.Subject != want.Subject {
		t.Errorf("Subject: got %q, want %q", got.Subject, want.Subject)
	}
	// Fields not set must be absent.
	if got.Keywords != "" {
		t.Errorf("Keywords: expected empty, got %q", got.Keywords)
	}
}

func TestSetMetadataCustomFields(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc = doc.SetMetadata(asposepdf.Metadata{
		Title:  "Doc",
		Custom: map[string]string{"Department": "Legal", "Version": "2.0"},
	})

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Custom["Department"] != "Legal" {
		t.Errorf("Department: got %q, want %q", got.Custom["Department"], "Legal")
	}
	if got.Custom["Version"] != "2.0" {
		t.Errorf("Version: got %q, want %q", got.Custom["Version"], "2.0")
	}
}

func TestSetMetadataReplaces(t *testing.T) {
	// Source doc has Title="Untitled"; SetMetadata with Title="" must omit it.
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc = doc.SetMetadata(asposepdf.Metadata{Author: "New Author"})

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Author != "New Author" {
		t.Errorf("Author: got %q, want %q", got.Author, "New Author")
	}
	// Title from source must NOT appear — SetMetadata is a full replacement.
	if got.Title != "" {
		t.Errorf("Title must be absent after SetMetadata without Title, got %q", got.Title)
	}
}

func TestClearMetadata(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// 4pages.pdf has non-empty metadata; ClearMetadata must strip it all.
	doc = doc.ClearMetadata()

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got != (asposepdf.Metadata{}) {
		t.Errorf("expected zero Metadata after ClearMetadata, got %+v", got)
	}
}
```

Add `"path/filepath"` to imports in `metadata_test.go`.

- [ ] **Step 2: Run to verify they fail**

```
go test -run "TestSetMetadata|TestClearMetadata" ./...
```

Expected: FAIL — writer does not yet write Info dict.

- [ ] **Step 3: Update `buildDocumentPDF` signature**

In `writer.go`, change:

```go
func buildDocumentPDF(entries []pageRef, patches map[patchKey]pdfDict, encCfg *encryptConfig) ([]byte, error) {
```

to:

```go
func buildDocumentPDF(entries []pageRef, patches map[patchKey]pdfDict, encCfg *encryptConfig, metaCfg *metadataConfig) ([]byte, error) {
```

Update the call in `buildMultiPagePDFEx` (writer.go line ~43):

```go
return buildDocumentPDF(entries, patches, nil, nil)
```

- [ ] **Step 4: Reserve an object number for the Info dict**

In `buildDocumentPDF`, after the block that reserves `encryptObjNum`, add:

```go
// Reserve an object number for the Info dictionary if metadata is being written.
var infoObjNum int
if metaCfg != nil && !metaCfg.clear {
	infoObjNum = newNum
	newNum++
}
```

- [ ] **Step 5: Write the Info object after the Catalog**

In `buildDocumentPDF`, after writing the Catalog object, add:

```go
// Write /Info dictionary (contains strings only — no remapping needed).
if infoObjNum > 0 {
	identity := func(n int) int { return n }
	offsets[infoObjNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n", infoObjNum)
	writeValue(&buf, buildInfoDict(metaCfg.meta), identity, nil)
	buf.WriteString("\nendobj\n")
}
```

- [ ] **Step 6: Add `/Info` reference to the trailer**

In `buildDocumentPDF`, in the trailer write section, after `/Root`:

```go
fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R", newNum, catalogNum)
if infoObjNum > 0 {
	fmt.Fprintf(&buf, " /Info %d 0 R", infoObjNum)
}
if encState != nil {
    // ... existing encrypt trailer code unchanged
```

- [ ] **Step 7: Update `WriteTo` in `document.go` to pass `metadataConfig`**

Change:

```go
data, err := buildDocumentPDF(d.pages, d.patches, d.encryptConfig)
```

to:

```go
data, err := buildDocumentPDF(d.pages, d.patches, d.encryptConfig, d.metadataConfig)
```

- [ ] **Step 8: Run tests to verify they pass**

```
go test -run "TestSetMetadata|TestClearMetadata" ./...
```

Expected: all PASS.

- [ ] **Step 9: Run full suite**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add writer.go document.go metadata.go metadata_test.go
git commit -m "Writer: serialize metadataConfig as Info dict; add SetMetadata/ClearMetadata tests"
```

---

## Task 4: Remove `GetMetadata` and update existing tests

**Files:**
- Modify: `metadata.go`
- Modify: `metadata_test.go`

- [ ] **Step 1: Replace `TestGetMetadata` with Open+Metadata pattern**

In `metadata_test.go`, replace the entire `TestGetMetadata` function:

```go
func TestDocumentMetadataFields(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	meta, err := doc.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if meta.Title != "Untitled" {
		t.Errorf("Title: got %q, want %q", meta.Title, "Untitled")
	}
	if meta.Creator != "Acrobat Editor 9.0" {
		t.Errorf("Creator: got %q, want %q", meta.Creator, "Acrobat Editor 9.0")
	}
	if meta.Producer != "Adobe Acrobat 9.0.0" {
		t.Errorf("Producer: got %q, want %q", meta.Producer, "Adobe Acrobat 9.0.0")
	}
	if meta.CreationDate == "" {
		t.Error("CreationDate should not be empty")
	}
	if meta.ModDate == "" {
		t.Error("ModDate should not be empty")
	}
	if meta.Author != "" {
		t.Errorf("Author: expected empty, got %q", meta.Author)
	}
	if meta.Subject != "" {
		t.Errorf("Subject: expected empty, got %q", meta.Subject)
	}
}
```

- [ ] **Step 2: Remove `GetMetadata` from `metadata.go`**

Delete the entire `GetMetadata` function (lines with the comment, func signature, and body).

- [ ] **Step 3: Build to confirm no remaining references**

```
go build ./...
```

Expected: no errors. If any file still calls `GetMetadata`, fix it now.

- [ ] **Step 4: Run full suite**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add metadata.go metadata_test.go
git commit -m "Remove standalone GetMetadata; update tests to use Open+Metadata()"
```

---

## Task 5: Update CLAUDE.md and README

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update `CLAUDE.md` — metadata section**

In the `**metadata.go**` entry under Public API, replace:

```
- `GetMetadata(inputPath)` — reads Info metadata from a PDF file
- `Metadata` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate
```

with:

```
- `(*Document).SetMetadata(meta) *Document` — returns a new Document configured to write meta as the Info dictionary on save; full replacement, empty fields omitted
- `(*Document).ClearMetadata() *Document` — returns a new Document that omits the Info dictionary on save
- `Metadata` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate, Custom map[string]string
```

- [ ] **Step 2: Update README — Metadata section**

Find the Metadata example in `README.md` and replace it with:

```go
// Read
doc, _ := pdf.Open("input.pdf")
meta, _ := doc.Metadata()
fmt.Println(meta.Title, meta.Author)

// Write (full replacement — unset fields are omitted)
doc = doc.SetMetadata(pdf.Metadata{
    Title:  "My Document",
    Author: "Jane Smith",
    Custom: map[string]string{"Department": "Legal"},
})
doc.Save("output.pdf")

// Update a single field: read → modify → write
meta, _ = doc.Metadata()
meta.Title = "Updated Title"
doc = doc.SetMetadata(meta)

// Strip all metadata
doc = doc.ClearMetadata()
doc.Save("clean.pdf")
```

- [ ] **Step 3: Run full suite one last time**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "Update docs: SetMetadata, ClearMetadata, Metadata.Custom"
```
