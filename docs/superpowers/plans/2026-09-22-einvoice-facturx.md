# E-Invoices (ZUGFeRD / Factur-X) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package a caller-supplied CII invoice XML into a PDF/A-3 document the way ZUGFeRD 2.x / Factur-X 1.0 require, and extract the invoice back out of incoming PDFs of all three ZUGFeRD generations.

**Architecture:** A general PDF/A-3 associated-files layer on embedded files (`/AFRelationship` on the file specification, the catalog `/AF` array, `/Params /ModDate`), a PDF/A-3 validator rule and converter step that use it, then an invoice layer on top: profile detection from the XML, `AttachInvoice` (embed + XMP `fx:*` properties + PDF/A extension schema + `ConvertToPDFA`) and `Invoice()` (find, extract, classify). `ConvertToPDFA` learns to keep XMP extension-schema blocks through its metadata rewrite.

**Tech Stack:** Go 1.24, standard library only (`encoding/xml`, `regexp`). The Python `factur-x` package (Akretion, BSD) as the independent check, installed only into a throwaway virtualenv under `result_files/`.

**Spec:** `docs/superpowers/specs/2026-09-22-einvoice-facturx-design.md`

## Global Constraints

- Every new `.go` file starts with `// SPDX-License-Identifier: MIT`, a blank line, then `package asposepdf`.
- Pure Go, standard library only. No new module dependency.
- No new entries in `testdata/testfiles.json`, no new files in `testdata/`; fixtures are built in memory.
- Default invoice file name `factur-x.xml`; `xrechnung.xml` for the XRechnung profile. MIME type written for the invoice: `text/xml`. Description: `Factur-X invoice`.
- Relationship: `AFData` for MINIMUM and BASIC WL, `AFAlternative` for BASIC, EN 16931, EXTENDED and XRECHNUNG.
- XMP written: namespace `urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#`, prefix `fx`; `fx:DocumentType` = `INVOICE`, `fx:DocumentFileName`, `fx:Version` = `1.0`, `fx:ConformanceLevel` = the profile spelling.
- Profile spellings (`InvoiceProfile.String()`): `MINIMUM`, `BASIC WL`, `BASIC`, `EN 16931`, `EXTENDED`, `XRECHNUNG`; unknown → `UNKNOWN`.
- CII root: element `CrossIndustryInvoice` in namespace `urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100`.
- The system Python is never modified; `pip install` only inside a virtualenv under `result_files/`.
- `gofmt -l` clean on touched files, `go vet ./...` silent, `go test ./...` passes, `golangci-lint run` → `0 issues`.
- Commits end with a blank line then `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`. Do not push, do not tag.

## File Structure

| File | Responsibility |
|---|---|
| `associated_files.go` (create) | `AFRelationship`, `EmbeddedFile.AFRelationship/SetAFRelationship`, catalog `/AF` maintenance |
| `embedded_files.go` (modify) | `EmbeddedFile.ref`, `/Params /ModDate`, `/AF` cleanup on Remove/Clear, `addBytesWith` |
| `validate_pdfa.go`, `pdfa_convert.go` (modify) | `EMBEDDED_FILE_NOT_ASSOCIATED`; conversion associates attachments; XMP extension blocks survive `setPDFAMetadata` |
| `einvoice.go` (create) | `InvoiceProfile`, profile detection, `AttachInvoice`, `Invoice`, `InvoiceOptions`, `Invoice` |
| `einvoice_xmp.go` (create) | XMP extension-schema block helpers and the Factur-X schema text |
| tests: `associated_files_internal_test.go`, `einvoice_internal_test.go` (create) | |
| `CLAUDE.md`, `README.md`, `CHANGELOG.md` (modify) | Documentation |

Existing code relied on (verified):

- `type EmbeddedFile struct { doc *Document; name string; filespec pdfDict }`; `(*EmbeddedFiles).Get(name)` resolves `raw()[name]` with `resolveRefToDict` (the returned `filespec` is the live map inside `d.objects`, so mutating it persists); `raw() map[string]pdfValue`; `writeBack(raw)`; `addBytes(name, data)` calls `d.buildEmbeddedFilespec(data, name, detectMIMEType(name), "")`; `Remove(name) bool`; `Clear()`; `(*EmbeddedFile).stream() *pdfStream`; `Names()`, `All()`.
- `buildEmbeddedFilespec(data, name, mimeType, description string) int` writes `/Params {"/Size": len(data)}`.
- `pdfDateString(t time.Time) string` (`sign.go`).
- `(*Document).resolveArray(v pdfValue) pdfArray` (`sign_verify.go`); `resolveRefToDict`.
- `ValidatePDFA` calls `d.pdfaCheck…(format, r)` in sequence; `r.add(rule, message string)`; `PDFAValidationReport{Format, Conformant bool, Issues []PDFAIssue{Rule, Message}}`; `format.part()` returns 1/2/3.
- `ConvertToPDFA(format)` runs transforms then `d.setPDFAMetadata(format)`, which reads `d.XMP()`, keeps `meta.Custom` entries whose `Prefix != "pdfaid"`, appends pdfaid properties, and calls `d.SetXMP(meta)`.
- XMP: `(*Document).XMP() (XMPMetadata, error)`, `SetXMP(XMPMetadata) error`, `XMPRaw() ([]byte, error)`, `SetXMPRaw([]byte) error`; `XMPProperty{Namespace, Prefix, Name, Value}`; the parser synthesises a prefix on read, so match custom properties by `Namespace`, never by `Prefix`.

---

### Task 1: Associated files on embedded files

**Files:**
- Create: `associated_files.go`
- Modify: `embedded_files.go`
- Test: `associated_files_internal_test.go`

**Interfaces:**
- Produces:
  - `type AFRelationship int` with `AFUnspecified`, `AFSource`, `AFData`, `AFAlternative`, `AFSupplement`
  - `func (f *EmbeddedFile) AFRelationship() AFRelationship`
  - `func (f *EmbeddedFile) SetAFRelationship(r AFRelationship)`
  - `func (f *EmbeddedFile) hasAFRelationship() bool`
  - `func (d *Document) isAssociatedFile(ref pdfRef) bool`
  - `EmbeddedFile.ref pdfRef` (the file specification's object; `Num == 0` for a direct dictionary)
  - `func (e *EmbeddedFiles) addBytesWith(name string, data []byte, mimeType, description string) (*EmbeddedFile, error)`

- [ ] **Step 1: Write the failing tests**

Create `associated_files_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

func catalogAFNums(d *Document) []int {
	var nums []int
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok {
			nums = append(nums, r.Num)
		}
	}
	return nums
}

func TestAFRelationshipSetAndList(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("data.csv", strings.NewReader("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.AFRelationship(); got != AFUnspecified {
		t.Errorf("new attachment relationship = %v, want AFUnspecified", got)
	}
	if len(catalogAFNums(doc)) != 0 {
		t.Error("an attachment with no relationship was listed in /AF")
	}

	f.SetAFRelationship(AFData)
	f = doc.EmbeddedFiles().Get("data.csv")
	if got := f.AFRelationship(); got != AFData {
		t.Errorf("relationship = %v, want AFData", got)
	}
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != f.ref.Num {
		t.Fatalf("/AF = %v, want exactly the file specification %d", n, f.ref.Num)
	}

	// Changing the relationship must not list the file twice.
	f.SetAFRelationship(AFSource)
	if n := catalogAFNums(doc); len(n) != 1 {
		t.Errorf("/AF has %d entries after a second SetAFRelationship, want 1", len(n))
	}
	if name, _ := f.filespec["/AFRelationship"].(pdfName); name != "/Source" {
		t.Errorf("/AFRelationship = %v, want /Source", f.filespec["/AFRelationship"])
	}
}

func TestAFRelationshipRoundTrip(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("source.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	f.SetAFRelationship(AFAlternative)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	g := back.EmbeddedFiles().Get("source.txt")
	if g == nil {
		t.Fatal("attachment lost in the round trip")
	}
	if got := g.AFRelationship(); got != AFAlternative {
		t.Errorf("relationship after round trip = %v, want AFAlternative", got)
	}
	if !back.isAssociatedFile(g.ref) {
		t.Error("the attachment is not listed in /AF after the round trip")
	}
}

func TestRemovingAttachmentRemovesItFromAF(t *testing.T) {
	doc := NewDocument(200, 200)
	a, _ := doc.EmbeddedFiles().AddFromStream("a.txt", strings.NewReader("a"))
	b, _ := doc.EmbeddedFiles().AddFromStream("b.txt", strings.NewReader("b"))
	a.SetAFRelationship(AFData)
	b.SetAFRelationship(AFData)
	doc.EmbeddedFiles().Remove("a.txt")
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != b.ref.Num {
		t.Errorf("/AF after removing a.txt = %v, want only b.txt's %d", n, b.ref.Num)
	}
	doc.EmbeddedFiles().Clear()
	if _, ok := doc.catalog["/AF"]; ok {
		t.Error("/AF survived Clear")
	}
}

// PDF/A-3 requires /ModDate in an embedded file's parameters.
func TestEmbeddedFileHasModDate(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("x.txt", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := f.stream().Dict["/Params"].(pdfDict)
	md, _ := params["/ModDate"].(string)
	if !strings.HasPrefix(md, "D:") {
		t.Errorf("/Params /ModDate = %q, want a PDF date", md)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -run "TestAFRelationship|TestRemovingAttachment|TestEmbeddedFileHasModDate" .`
Expected: FAIL — `undefined: AFUnspecified`, `f.ref undefined`.

- [ ] **Step 3: Write `associated_files.go`**

```go
// SPDX-License-Identifier: MIT

package asposepdf

// Associated files (ISO 32000-2 §14.13; required of every embedded file by
// PDF/A-3, ISO 19005-3 §6.8): a file specification states how the file relates
// to the document through /AFRelationship, and the catalog lists it in /AF.

// AFRelationship says how an embedded file relates to the document. Mirrors
// Aspose.PDF for .NET's AFRelationship.
type AFRelationship int

const (
	// AFUnspecified states no particular relationship. The zero value, and
	// what a file with no /AFRelationship reports.
	AFUnspecified AFRelationship = iota
	// AFSource is the original the document was produced from.
	AFSource
	// AFData is data the document's content is built on, such as the table
	// behind a chart.
	AFData
	// AFAlternative is an alternative representation of the content, such as
	// the XML of a hybrid e-invoice.
	AFAlternative
	// AFSupplement is a supplemental representation of the content.
	AFSupplement
)

var afRelationshipNames = [...]pdfName{
	AFUnspecified: "/Unspecified",
	AFSource:      "/Source",
	AFData:        "/Data",
	AFAlternative: "/Alternative",
	AFSupplement:  "/Supplement",
}

// String names the relationship as PDF writes it, without the slash.
func (r AFRelationship) String() string { return string(r.pdfName()[1:]) }

func (r AFRelationship) pdfName() pdfName {
	if r >= 0 && int(r) < len(afRelationshipNames) {
		return afRelationshipNames[r]
	}
	return afRelationshipNames[AFUnspecified]
}

func afRelationshipFromName(n pdfName) AFRelationship {
	for r, name := range afRelationshipNames {
		if name == n {
			return AFRelationship(r)
		}
	}
	return AFUnspecified
}

// AFRelationship reports how the file relates to the document; a file with no
// /AFRelationship reports AFUnspecified.
func (f *EmbeddedFile) AFRelationship() AFRelationship {
	n, _ := f.filespec["/AFRelationship"].(pdfName)
	return afRelationshipFromName(n)
}

// SetAFRelationship records how the file relates to the document and lists it
// in the catalog's /AF array — both of which PDF/A-3 requires of every
// embedded file.
func (f *EmbeddedFile) SetAFRelationship(r AFRelationship) {
	f.filespec["/AFRelationship"] = r.pdfName()
	if f.ref.Num == 0 {
		f.ref = f.doc.EmbeddedFiles().promote(f.name, f.filespec)
	}
	f.doc.addAssociatedFile(f.ref)
}

// hasAFRelationship reports whether the file specification states a
// relationship at all.
func (f *EmbeddedFile) hasAFRelationship() bool {
	_, ok := f.filespec["/AFRelationship"].(pdfName)
	return ok
}

// isAssociatedFile reports whether the catalog's /AF lists ref.
func (d *Document) isAssociatedFile(ref pdfRef) bool {
	if ref.Num == 0 {
		return false
	}
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok && r.Num == ref.Num {
			return true
		}
	}
	return false
}

// addAssociatedFile lists ref in the catalog's /AF, once.
func (d *Document) addAssociatedFile(ref pdfRef) {
	if ref.Num == 0 || d.isAssociatedFile(ref) {
		return
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	af := append(pdfArray{}, d.resolveArray(d.catalog["/AF"])...)
	d.catalog["/AF"] = append(af, ref)
}

// removeAssociatedFile drops ref from the catalog's /AF; an emptied array is
// removed.
func (d *Document) removeAssociatedFile(ref pdfRef) {
	if ref.Num == 0 || d.catalog == nil {
		return
	}
	var kept pdfArray
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok && r.Num == ref.Num {
			continue
		}
		kept = append(kept, v)
	}
	if len(kept) == 0 {
		delete(d.catalog, "/AF")
		return
	}
	d.catalog["/AF"] = kept
}
```

- [ ] **Step 4: Modify `embedded_files.go`**

1. Add `ref pdfRef` to `EmbeddedFile`:

```go
type EmbeddedFile struct {
	doc      *Document
	name     string
	filespec pdfDict
	ref      pdfRef // the file specification object; Num 0 when it is a direct dictionary
}
```

2. In `Get`, set it:

```go
	ref, _ := val.(pdfRef)
	return &EmbeddedFile{doc: e.doc, name: name, filespec: fs, ref: ref}
```

3. Replace `addBytes` with a delegating pair:

```go
func (e *EmbeddedFiles) addBytes(name string, data []byte) (*EmbeddedFile, error) {
	return e.addBytesWith(name, data, detectMIMEType(name), "")
}

// addBytesWith embeds data under name with an explicit MIME type and
// description, replacing an attachment of the same name.
func (e *EmbeddedFiles) addBytesWith(name string, data []byte, mimeType, description string) (*EmbeddedFile, error) {
	if name == "" {
		return nil, fmt.Errorf("EmbeddedFiles: attachment name must not be empty")
	}
	raw := e.raw()
	if old, ok := raw[name].(pdfRef); ok {
		e.doc.removeAssociatedFile(old)
	}
	fsID := e.doc.buildEmbeddedFilespec(data, name, mimeType, description)
	raw[name] = pdfRef{Num: fsID}
	e.writeBack(raw)
	return e.Get(name), nil
}
```

4. `Remove` and `Clear` drop the file from `/AF`:

```go
func (e *EmbeddedFiles) Remove(name string) bool {
	raw := e.raw()
	v, ok := raw[name]
	if !ok {
		return false
	}
	if r, isRef := v.(pdfRef); isRef {
		e.doc.removeAssociatedFile(r)
	}
	delete(raw, name)
	e.writeBack(raw)
	return true
}

// Clear removes every attachment.
func (e *EmbeddedFiles) Clear() {
	for _, v := range e.raw() {
		if r, ok := v.(pdfRef); ok {
			e.doc.removeAssociatedFile(r)
		}
	}
	e.writeBack(map[string]pdfValue{})
}
```

5. Add `promote`, which turns a direct file-specification dictionary into an indirect object so `/AF` can reference it:

```go
// promote stores a direct file-specification dictionary as its own object
// and repoints the name tree at it, so the catalog's /AF can reference it.
func (e *EmbeddedFiles) promote(name string, fs pdfDict) pdfRef {
	num := e.doc.nextID
	e.doc.nextID++
	e.doc.objects[num] = &pdfObject{Num: num, Value: fs}
	raw := e.raw()
	raw[name] = pdfRef{Num: num}
	e.writeBack(raw)
	return pdfRef{Num: num}
}
```

6. In `buildEmbeddedFilespec`, give the parameters a modification date (add `"time"` to the imports):

```go
			"/Params":  pdfDict{"/Size": len(data), "/ModDate": pdfDateString(time.Now())},
```

- [ ] **Step 5: Run the tests**

Run: `go test -run "TestAFRelationship|TestRemovingAttachment|TestEmbeddedFileHasModDate|TestEmbeddedFiles|TestFileAttachment|TestCollection" .`
Expected: PASS (the existing attachment, file-attachment and portfolio tests included).

- [ ] **Step 6: Commit**

```bash
git add associated_files.go embedded_files.go associated_files_internal_test.go
git commit -m "$(cat <<'EOF'
feat: PDF/A-3 associated files on attachments (pdf-go-t6s9)

EmbeddedFile gains AFRelationship/SetAFRelationship (Source, Data,
Alternative, Supplement, Unspecified — mirroring Aspose's
FileSpecification.AFRelationship). Setting one writes /AFRelationship on the
file specification and lists it once in the catalog's /AF; removing or
replacing the attachment takes it out again. Embedded files now carry
/Params /ModDate, which PDF/A-3 requires.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: PDF/A-3 rule and conversion for attachments

**Files:**
- Modify: `validate_pdfa.go`, `pdfa_convert.go`
- Test: `associated_files_internal_test.go` (append)

**Interfaces:**
- Consumes: `hasAFRelationship`, `isAssociatedFile`, `SetAFRelationship`, `EmbeddedFile.ref` (Task 1).
- Produces: `func (d *Document) pdfaCheckAssociatedFiles(format PDFAFormat, r *PDFAValidationReport)`, `func (d *Document) associatePDFAEmbeddedFiles()`, rule code `EMBEDDED_FILE_NOT_ASSOCIATED`.

- [ ] **Step 1: Write the failing tests**

Append to `associated_files_internal_test.go`:

```go
func hasRule(r *PDFAValidationReport, rule string) bool {
	for _, is := range r.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

func TestPDFA3RequiresAssociatedFiles(t *testing.T) {
	doc := NewDocument(200, 200)
	if _, err := doc.EmbeddedFiles().AddFromStream("data.csv", strings.NewReader("1,2")); err != nil {
		t.Fatal(err)
	}
	if !hasRule(doc.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("PDF/A-3 accepted an attachment with no /AFRelationship")
	}
	if hasRule(doc.ValidatePDFA(PDFA2B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("the PDF/A-3 rule fired for PDF/A-2")
	}
}

func TestConvertToPDFA3AssociatesAttachments(t *testing.T) {
	doc := NewDocument(200, 200)
	if _, err := doc.EmbeddedFiles().AddFromStream("plain.csv", strings.NewReader("1,2")); err != nil {
		t.Fatal(err)
	}
	kept, _ := doc.EmbeddedFiles().AddFromStream("kept.txt", strings.NewReader("k"))
	kept.SetAFRelationship(AFSource)

	report, err := doc.ConvertToPDFA(PDFA3B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(report, "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Errorf("conversion left an unassociated attachment: %+v", report.Issues)
	}
	plain := doc.EmbeddedFiles().Get("plain.csv")
	if !plain.hasAFRelationship() || plain.AFRelationship() != AFUnspecified {
		t.Errorf("plain.csv relationship = %v (explicit %v), want an explicit Unspecified",
			plain.AFRelationship(), plain.hasAFRelationship())
	}
	if got := doc.EmbeddedFiles().Get("kept.txt").AFRelationship(); got != AFSource {
		t.Errorf("conversion changed an existing relationship to %v", got)
	}
	for _, f := range doc.EmbeddedFiles().All() {
		if !doc.isAssociatedFile(f.ref) {
			t.Errorf("%s is not listed in /AF after conversion", f.Name())
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -run "TestPDFA3RequiresAssociatedFiles|TestConvertToPDFA3AssociatesAttachments" .`
Expected: FAIL — the rule does not exist yet.

- [ ] **Step 3: Implement**

In `validate_pdfa.go`, add the check after `pdfaCheckEmbeddedFiles` in `ValidatePDFA`:

```go
	d.pdfaCheckAssociatedFiles(format, r)
```

and the function next to `pdfaCheckEmbeddedFiles`:

```go
// pdfaCheckAssociatedFiles enforces ISO 19005-3 §6.8: every embedded file
// states its relationship to the document and is listed in the catalog /AF.
func (d *Document) pdfaCheckAssociatedFiles(format PDFAFormat, r *PDFAValidationReport) {
	if format.part() != 3 {
		return
	}
	for _, f := range d.EmbeddedFiles().All() {
		if !f.hasAFRelationship() || !d.isAssociatedFile(f.ref) {
			r.add("EMBEDDED_FILE_NOT_ASSOCIATED", fmt.Sprintf(
				"embedded file %q has no /AFRelationship or is not listed in the catalog /AF; PDF/A-3 requires both", f.Name()))
		}
	}
}
```

(`fmt` is already imported in `validate_pdfa.go`; check and add if not.)

In `pdfa_convert.go`, right after the `if format == PDFA1B { d.removePDFAEmbeddedFiles() }` block:

```go
	if format.part() == 3 {
		d.associatePDFAEmbeddedFiles()
	}
```

and the function (below `ConvertToPDFA`):

```go
// associatePDFAEmbeddedFiles gives every attachment the association PDF/A-3
// requires: an existing /AFRelationship is kept, a missing one becomes
// Unspecified, and each file is listed in the catalog /AF with a /ModDate in
// its parameters.
func (d *Document) associatePDFAEmbeddedFiles() {
	for _, f := range d.EmbeddedFiles().All() {
		f.SetAFRelationship(f.AFRelationship())
		if st := f.stream(); st != nil {
			params, _ := st.Dict["/Params"].(pdfDict)
			if params == nil {
				params = pdfDict{}
				st.Dict["/Params"] = params
			}
			if _, ok := params["/ModDate"]; !ok {
				params["/ModDate"] = pdfDateString(time.Now())
			}
		}
	}
}
```

(add `"time"` to `pdfa_convert.go`'s imports if absent).

- [ ] **Step 4: Run the tests**

Run: `go test -run "TestPDFA|TestConvertToPDFA|TestAFRelationship|TestEmbedded" .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add validate_pdfa.go pdfa_convert.go associated_files_internal_test.go
git commit -m "$(cat <<'EOF'
feat: PDF/A-3 checks and repairs attachment associations (pdf-go-t6s9)

ValidatePDFA for PDF/A-3 reports EMBEDDED_FILE_NOT_ASSOCIATED for an
attachment without /AFRelationship or missing from the catalog /AF (ISO
19005-3 §6.8), and ConvertToPDFA to PDF/A-3 repairs it: an existing
relationship is kept, a missing one becomes Unspecified, every file is
listed in /AF and gets a /ModDate.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Invoice profiles and CII detection

**Files:**
- Create: `einvoice.go`
- Test: `einvoice_internal_test.go`

**Interfaces:**
- Produces:
  - `type InvoiceProfile int` with `InvoiceProfileUnknown`, `InvoiceProfileMinimum`, `InvoiceProfileBasicWL`, `InvoiceProfileBasic`, `InvoiceProfileEN16931`, `InvoiceProfileExtended`, `InvoiceProfileXRechnung`; `func (p InvoiceProfile) String() string`
  - `const nsCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"`
  - `func invoiceGuideline(data []byte) (root xml.Name, guidelineID string, err error)`
  - `func invoiceProfileFromGuideline(id string) InvoiceProfile`
  - `func invoiceProfileFromLevel(level string) InvoiceProfile`
  - `func (p InvoiceProfile) defaultFileName() string`
  - `func (p InvoiceProfile) relationship() AFRelationship`
  - Test helper `ciiInvoice(guidelineID string) []byte` (in `einvoice_internal_test.go`)

- [ ] **Step 1: Write the failing tests**

Create `einvoice_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// ciiInvoice returns a small CII invoice with the given guideline ID. Its
// content is a valid Factur-X MINIMUM invoice (the independent XSD check in
// the last task relies on that for the MINIMUM case).
func ciiInvoice(guidelineID string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
  <rsm:ExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>` + guidelineID + `</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:ExchangedDocumentContext>
  <rsm:ExchangedDocument>
    <ram:ID>INV-2026-001</ram:ID>
    <ram:TypeCode>380</ram:TypeCode>
    <ram:IssueDateTime><udt:DateTimeString format="102">20260922</udt:DateTimeString></ram:IssueDateTime>
  </rsm:ExchangedDocument>
  <rsm:SupplyChainTradeTransaction>
    <ram:ApplicableHeaderTradeAgreement>
      <ram:SellerTradeParty>
        <ram:Name>Seller GmbH</ram:Name>
        <ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress>
        <ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration>
      </ram:SellerTradeParty>
      <ram:BuyerTradeParty><ram:Name>Buyer SARL</ram:Name></ram:BuyerTradeParty>
    </ram:ApplicableHeaderTradeAgreement>
    <ram:ApplicableHeaderTradeDelivery/>
    <ram:ApplicableHeaderTradeSettlement>
      <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
        <ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>
        <ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>
        <ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>
        <ram:DuePayableAmount>119.00</ram:DuePayableAmount>
      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
    </ram:ApplicableHeaderTradeSettlement>
  </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>
`)
}

func TestInvoiceProfileFromGuideline(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want InvoiceProfile
	}{
		{"urn:factur-x.eu:1p0:minimum", InvoiceProfileMinimum},
		{"urn:factur-x.eu:1p0:basicwl", InvoiceProfileBasicWL},
		{"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic", InvoiceProfileBasic},
		{"urn:cen.eu:en16931:2017", InvoiceProfileEN16931},
		{"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended", InvoiceProfileExtended},
		{"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", InvoiceProfileXRechnung},
		{"urn:zugferd.de:2p0:minimum", InvoiceProfileMinimum},
		{"urn:zugferd.de:2p0:basicwl", InvoiceProfileBasicWL},
		{"urn:zugferd.de:2p0:basic", InvoiceProfileBasic},
		{"urn:zugferd.de:2p0:en16931", InvoiceProfileEN16931},
		{"urn:zugferd.de:2p0:extended", InvoiceProfileExtended},
		{"urn:ferd:CrossIndustryDocument:invoice:1p0:comfort", InvoiceProfileEN16931},
		{"  urn:factur-x.eu:1p0:minimum \n", InvoiceProfileMinimum},
		{"urn:example:unknown", InvoiceProfileUnknown},
	} {
		if got := invoiceProfileFromGuideline(tc.id); got != tc.want {
			t.Errorf("%q → %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestInvoiceProfileStringsAndLevels(t *testing.T) {
	for p, want := range map[InvoiceProfile]string{
		InvoiceProfileMinimum:   "MINIMUM",
		InvoiceProfileBasicWL:   "BASIC WL",
		InvoiceProfileBasic:     "BASIC",
		InvoiceProfileEN16931:   "EN 16931",
		InvoiceProfileExtended:  "EXTENDED",
		InvoiceProfileXRechnung: "XRECHNUNG",
		InvoiceProfileUnknown:   "UNKNOWN",
	} {
		if p.String() != want {
			t.Errorf("String() = %q, want %q", p.String(), want)
		}
		if p != InvoiceProfileUnknown && invoiceProfileFromLevel(want) != p {
			t.Errorf("invoiceProfileFromLevel(%q) did not round-trip", want)
		}
	}
	if invoiceProfileFromLevel("comfort") != InvoiceProfileEN16931 {
		t.Error("ZUGFeRD 1.0 COMFORT did not map to EN 16931")
	}
	if invoiceProfileFromLevel("en16931") != InvoiceProfileEN16931 {
		t.Error("EN16931 without a space did not map")
	}
	if InvoiceProfileXRechnung.defaultFileName() != "xrechnung.xml" || InvoiceProfileBasic.defaultFileName() != "factur-x.xml" {
		t.Error("default file names are wrong")
	}
	if InvoiceProfileMinimum.relationship() != AFData || InvoiceProfileBasicWL.relationship() != AFData ||
		InvoiceProfileBasic.relationship() != AFAlternative || InvoiceProfileEN16931.relationship() != AFAlternative {
		t.Error("relationships do not follow the Factur-X rule")
	}
}

func TestInvoiceGuideline(t *testing.T) {
	root, id, err := invoiceGuideline(ciiInvoice("urn:cen.eu:en16931:2017"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Local != "CrossIndustryInvoice" || root.Space != nsCII {
		t.Errorf("root = %v, want CII", root)
	}
	if id != "urn:cen.eu:en16931:2017" {
		t.Errorf("guideline = %q", id)
	}

	if _, _, err := invoiceGuideline([]byte("<rsm:CrossIndustryInvoice")); err == nil {
		t.Error("malformed XML accepted")
	}
	root, _, err = invoiceGuideline([]byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if root.Local == "CrossIndustryInvoice" {
		t.Error("a UBL root was reported as CII")
	}
	_ = strings.TrimSpace
}
```

Remove the last `_ = strings.TrimSpace` line and the `strings` import if nothing else in the file uses `strings` at the end of Task 4 — Task 4 appends tests that do.

- [ ] **Step 2: Run to verify they fail**

Run: `go test -run "TestInvoiceProfile|TestInvoiceGuideline" .`
Expected: FAIL — `undefined: InvoiceProfile`.

- [ ] **Step 3: Write `einvoice.go` (profiles and detection only; Task 4 appends the API)**

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Hybrid e-invoices (ZUGFeRD 2.x / Factur-X 1.0): a PDF/A-3 document with the
// invoice's UN/CEFACT Cross Industry Invoice (CII) XML embedded as an
// associated file and described in the XMP metadata. The caller supplies the
// XML; the library packages it and extracts it again.

// nsCII is the namespace of the CII root element.
const nsCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"

// InvoiceProfile is the Factur-X / ZUGFeRD conformance level of an invoice.
type InvoiceProfile int

const (
	InvoiceProfileUnknown   InvoiceProfile = iota
	InvoiceProfileMinimum                  // MINIMUM
	InvoiceProfileBasicWL                  // BASIC WL (without lines)
	InvoiceProfileBasic                    // BASIC
	InvoiceProfileEN16931                  // EN 16931 (ZUGFeRD 1.0 COMFORT)
	InvoiceProfileExtended                 // EXTENDED
	InvoiceProfileXRechnung                // XRECHNUNG
)

var invoiceProfileNames = [...]string{
	InvoiceProfileUnknown:   "UNKNOWN",
	InvoiceProfileMinimum:   "MINIMUM",
	InvoiceProfileBasicWL:   "BASIC WL",
	InvoiceProfileBasic:     "BASIC",
	InvoiceProfileEN16931:   "EN 16931",
	InvoiceProfileExtended:  "EXTENDED",
	InvoiceProfileXRechnung: "XRECHNUNG",
}

// String gives the Factur-X spelling, as written to fx:ConformanceLevel.
func (p InvoiceProfile) String() string {
	if p >= 0 && int(p) < len(invoiceProfileNames) {
		return invoiceProfileNames[p]
	}
	return invoiceProfileNames[InvoiceProfileUnknown]
}

// defaultFileName is the attachment name the standards prescribe.
func (p InvoiceProfile) defaultFileName() string {
	if p == InvoiceProfileXRechnung {
		return "xrechnung.xml"
	}
	return "factur-x.xml"
}

// relationship follows the Factur-X rule: the lower profiles are not a full
// representation of the invoice, so their XML is Data; from BASIC up it is an
// Alternative to the visible document.
func (p InvoiceProfile) relationship() AFRelationship {
	if p == InvoiceProfileMinimum || p == InvoiceProfileBasicWL {
		return AFData
	}
	return AFAlternative
}

// invoiceProfileFromGuideline maps a GuidelineSpecifiedDocumentContextParameter
// ID — Factur-X, ZUGFeRD 2.0 and ZUGFeRD 1.0 forms — to a profile.
func invoiceProfileFromGuideline(id string) InvoiceProfile {
	id = strings.ToLower(strings.TrimSpace(id))
	switch {
	case strings.Contains(id, "xrechnung"):
		return InvoiceProfileXRechnung
	case strings.HasSuffix(id, ":minimum"):
		return InvoiceProfileMinimum
	case strings.HasSuffix(id, ":basicwl"):
		return InvoiceProfileBasicWL
	case strings.HasSuffix(id, ":extended"):
		return InvoiceProfileExtended
	case strings.HasSuffix(id, ":basic"):
		return InvoiceProfileBasic
	case strings.HasSuffix(id, ":en16931"), strings.HasSuffix(id, ":comfort"), id == "urn:cen.eu:en16931:2017":
		return InvoiceProfileEN16931
	}
	return InvoiceProfileUnknown
}

// invoiceProfileFromLevel maps an XMP ConformanceLevel value to a profile.
func invoiceProfileFromLevel(level string) InvoiceProfile {
	switch strings.ToUpper(strings.Join(strings.Fields(level), " ")) {
	case "MINIMUM":
		return InvoiceProfileMinimum
	case "BASIC WL", "BASICWL":
		return InvoiceProfileBasicWL
	case "BASIC":
		return InvoiceProfileBasic
	case "EN 16931", "EN16931", "COMFORT":
		return InvoiceProfileEN16931
	case "EXTENDED":
		return InvoiceProfileExtended
	case "XRECHNUNG":
		return InvoiceProfileXRechnung
	}
	return InvoiceProfileUnknown
}

// invoiceGuideline reads the invoice XML only as far as it must: the root
// element's name, and the text of the ID inside
// GuidelineSpecifiedDocumentContextParameter ("" when there is none). A
// malformed document is an error.
func invoiceGuideline(data []byte) (root xml.Name, guidelineID string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var inParam, inID bool
	var id strings.Builder
	for {
		tok, terr := dec.Token()
		if errors.Is(terr, io.EOF) {
			break
		}
		if terr != nil {
			return root, "", fmt.Errorf("invoice XML: %w", terr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root.Local == "" {
				root = t.Name
			}
			switch {
			case t.Name.Local == "GuidelineSpecifiedDocumentContextParameter":
				inParam = true
			case inParam && t.Name.Local == "ID":
				inID = true
			}
		case xml.EndElement:
			switch {
			case inID && t.Name.Local == "ID":
				return root, strings.TrimSpace(id.String()), nil
			case t.Name.Local == "GuidelineSpecifiedDocumentContextParameter":
				inParam = false
			}
		case xml.CharData:
			if inID {
				id.Write(t)
			}
		}
	}
	if root.Local == "" {
		return root, "", fmt.Errorf("invoice XML: no root element")
	}
	return root, "", nil
}
```

The early return stops reading once the guideline ID is found, so a large invoice is never parsed in full; the remainder is not checked for well-formedness, which is acceptable — the caller's system produced it, and the XSD check belongs to a validator.

- [ ] **Step 4: Run the tests**

Run: `go test -run "TestInvoiceProfile|TestInvoiceGuideline" .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add einvoice.go einvoice_internal_test.go
git commit -m "$(cat <<'EOF'
feat: e-invoice profiles and CII detection (pdf-go-t6s9)

InvoiceProfile covers the Factur-X / ZUGFeRD levels (MINIMUM, BASIC WL,
BASIC, EN 16931, EXTENDED, XRECHNUNG), with the Factur-X spelling, default
attachment name and associated-file relationship of each. The profile is
read from the guideline ID in the CII XML — Factur-X, ZUGFeRD 2.0 and 1.0
forms — without parsing the rest of the invoice.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: AttachInvoice, Invoice, and XMP extension schemas

**Files:**
- Create: `einvoice_xmp.go`
- Modify: `einvoice.go` (append), `pdfa_convert.go` (`setPDFAMetadata`)
- Test: `einvoice_internal_test.go` (append)

**Interfaces:**
- Consumes: Task 1 (`addBytesWith`, `SetAFRelationship`, `AFRelationship`), Task 2 (conversion), Task 3 (profiles, `invoiceGuideline`, `nsCII`).
- Produces:
  - `const nsFacturX = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"`, `nsZUGFeRD2`, `nsZUGFeRD1`
  - `func xmpExtensionBlocks(packet string) (blocks []string, rest string)`
  - `func insertXMPDescriptions(packet string, blocks []string) string`
  - `const facturXExtensionSchema string`
  - `type InvoiceOptions struct { FileName string; Profile InvoiceProfile; Format PDFAFormat }`
  - `func (d *Document) AttachInvoice(xmlData []byte, opts ...InvoiceOptions) (*PDFAValidationReport, error)`
  - `type Invoice struct { XML []byte; FileName string; Profile InvoiceProfile; Standard string; Version string }`
  - `func (d *Document) Invoice() (*Invoice, error)`

- [ ] **Step 1: Write the failing tests**

Append to `einvoice_internal_test.go` (add `"bytes"` and `"fmt"` to the imports; keep `"strings"`, and delete the `_ = strings.TrimSpace` line from Task 3):

```go
func invoiceTestDoc(t *testing.T) *Document {
	t.Helper()
	doc := NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Invoice INV-2026-001", TextStyle{Font: FontHelvetica, Size: 14},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
		t.Fatal(err)
	}
	return doc
}

func saveAndReopen(t *testing.T, doc *Document) *Document {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return back
}

func TestAttachInvoiceRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		urn      string
		profile  InvoiceProfile
		fileName string
		rel      AFRelationship
	}{
		{"urn:factur-x.eu:1p0:minimum", InvoiceProfileMinimum, "factur-x.xml", AFData},
		{"urn:factur-x.eu:1p0:basicwl", InvoiceProfileBasicWL, "factur-x.xml", AFData},
		{"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic", InvoiceProfileBasic, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017", InvoiceProfileEN16931, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended", InvoiceProfileExtended, "factur-x.xml", AFAlternative},
		{"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", InvoiceProfileXRechnung, "xrechnung.xml", AFAlternative},
	} {
		t.Run(tc.profile.String(), func(t *testing.T) {
			xmlData := ciiInvoice(tc.urn)
			doc := invoiceTestDoc(t)
			report, err := doc.AttachInvoice(xmlData)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Conformant {
				t.Errorf("not PDF/A-3 conformant: %+v", report.Issues)
			}
			back := saveAndReopen(t, doc)
			inv, err := back.Invoice()
			if err != nil || inv == nil {
				t.Fatalf("Invoice() = %v, %v", inv, err)
			}
			if inv.Profile != tc.profile || inv.FileName != tc.fileName || inv.Version != "1.0" {
				t.Errorf("got profile %v, file %q, version %q", inv.Profile, inv.FileName, inv.Version)
			}
			if inv.Standard != "Factur-X 1.0 / ZUGFeRD 2.1+" {
				t.Errorf("Standard = %q", inv.Standard)
			}
			if !bytes.Equal(inv.XML, xmlData) {
				t.Error("the extracted XML differs from what was attached")
			}
			f := back.EmbeddedFiles().Get(tc.fileName)
			if f.AFRelationship() != tc.rel || !back.isAssociatedFile(f.ref) {
				t.Errorf("relationship %v (listed %v), want %v", f.AFRelationship(), back.isAssociatedFile(f.ref), tc.rel)
			}
			if f.MIMEType() != "text/xml" {
				t.Errorf("MIME type = %q, want text/xml", f.MIMEType())
			}
			raw, _ := back.XMPRaw()
			if n := strings.Count(string(raw), "pdfaExtension:schemas"); n != 2 { // open and close tag
				t.Errorf("extension schema appears %d/2 times", n)
			}
			if !strings.Contains(string(raw), nsFacturX) {
				t.Error("the Factur-X namespace is missing from the XMP")
			}
			if back.ValidatePDFA(PDFA3B).Conformant != report.Conformant {
				t.Error("the reopened file validates differently")
			}
		})
	}
}

// A second call replaces the invoice and the metadata instead of adding more.
func TestAttachInvoiceReplaces(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice(ciiInvoice("urn:factur-x.eu:1p0:minimum")); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	if n := doc.EmbeddedFiles().Count(); n != 1 {
		t.Errorf("%d attachments after a second AttachInvoice, want 1", n)
	}
	raw, _ := doc.XMPRaw()
	if n := strings.Count(string(raw), "<pdfaExtension:schemas>"); n != 1 {
		t.Errorf("extension schema block appears %d times, want 1", n)
	}
	inv, err := doc.Invoice()
	if err != nil || inv == nil || inv.Profile != InvoiceProfileEN16931 {
		t.Fatalf("Invoice() after replace = %+v, %v", inv, err)
	}
}

// ConvertToPDFA rewrites the XMP; it must keep extension schemas (ours and
// any another producer declared) and the properties they describe.
func TestConvertToPDFAKeepsExtensionSchemas(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice(ciiInvoice("urn:factur-x.eu:1p0:basicwl")); err != nil {
		t.Fatal(err)
	}
	back := saveAndReopen(t, doc)
	if _, err := back.ConvertToPDFA(PDFA3B); err != nil {
		t.Fatal(err)
	}
	raw, _ := back.XMPRaw()
	s := string(raw)
	if strings.Count(s, "<pdfaExtension:schemas>") != 1 {
		t.Errorf("extension schema lost or duplicated by ConvertToPDFA:\n%s", s)
	}
	if strings.Contains(s, "pdfaSchema:schema=") || strings.Contains(s, "<pdfaSchema:schema>Factur-X PDFA Extension Schema</pdfaSchema:schema>\n<pdfaSchema:schema>") {
		t.Error("the extension schema's fields leaked out as top-level properties")
	}
	inv, err := back.Invoice()
	if err != nil || inv == nil || inv.Profile != InvoiceProfileBasicWL {
		t.Fatalf("Invoice() after ConvertToPDFA = %+v, %v", inv, err)
	}
}

func TestAttachInvoiceErrors(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.AttachInvoice([]byte("<rsm:CrossIndustryInvoice")); err == nil {
		t.Error("malformed XML accepted")
	}
	if _, err := doc.AttachInvoice([]byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`)); err == nil {
		t.Error("a UBL invoice accepted as CII")
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:example:unknown")); err == nil {
		t.Error("an unknown profile accepted without an explicit one")
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:example:unknown"), InvoiceOptions{Profile: InvoiceProfileEN16931}); err != nil {
		t.Errorf("an explicit profile was not honoured: %v", err)
	}
	if _, err := doc.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017"), InvoiceOptions{Format: PDFA2B}); err == nil {
		t.Error("a non-PDF/A-3 format accepted")
	}
}

func TestInvoiceNotAnInvoice(t *testing.T) {
	doc := invoiceTestDoc(t)
	if _, err := doc.EmbeddedFiles().AddFromStream("notes.txt", strings.NewReader("hi")); err != nil {
		t.Fatal(err)
	}
	inv, err := doc.Invoice()
	if err != nil || inv != nil {
		t.Errorf("Invoice() on a plain document = %+v, %v; want nil, nil", inv, err)
	}
}

// Documents from the earlier generations, built by hand the way their
// producers wrote them.
func TestInvoiceReadsOlderGenerations(t *testing.T) {
	t.Run("ZUGFeRD 2.0", func(t *testing.T) {
		doc := invoiceTestDoc(t)
		if _, err := doc.EmbeddedFiles().AddFromStream("zugferd-invoice.xml",
			bytes.NewReader(ciiInvoice("urn:zugferd.de:2p0:en16931"))); err != nil {
			t.Fatal(err)
		}
		if err := doc.SetXMP(XMPMetadata{Custom: []XMPProperty{
			{Namespace: nsZUGFeRD2, Prefix: "fx", Name: "ConformanceLevel", Value: "EN 16931"},
			{Namespace: nsZUGFeRD2, Prefix: "fx", Name: "Version", Value: "2p0"},
		}}); err != nil {
			t.Fatal(err)
		}
		inv, err := saveAndReopen(t, doc).Invoice()
		if err != nil || inv == nil {
			t.Fatalf("Invoice() = %v, %v", inv, err)
		}
		if inv.Standard != "ZUGFeRD 2.0" || inv.Profile != InvoiceProfileEN16931 || inv.FileName != "zugferd-invoice.xml" {
			t.Errorf("got %+v", inv)
		}
	})
	t.Run("ZUGFeRD 1.0", func(t *testing.T) {
		zf1 := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryDocument xmlns:rsm="urn:ferd:CrossIndustryDocument:invoice:1p0" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:12">
  <rsm:SpecifiedExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>urn:ferd:CrossIndustryDocument:invoice:1p0:comfort</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:SpecifiedExchangedDocumentContext>
</rsm:CrossIndustryDocument>
`)
		doc := invoiceTestDoc(t)
		if _, err := doc.EmbeddedFiles().AddFromStream("ZUGFeRD-invoice.xml", bytes.NewReader(zf1)); err != nil {
			t.Fatal(err)
		}
		inv, err := saveAndReopen(t, doc).Invoice()
		if err != nil || inv == nil {
			t.Fatalf("Invoice() = %v, %v", inv, err)
		}
		if inv.Standard != "ZUGFeRD 1.0" || inv.Profile != InvoiceProfileEN16931 {
			t.Errorf("got standard %q profile %v", inv.Standard, inv.Profile)
		}
		_ = fmt.Sprint
	})
}
```

(Delete the trailing `_ = fmt.Sprint` and the `"fmt"` import if nothing uses `fmt`.)

- [ ] **Step 2: Run to verify they fail**

Run: `go test -run "TestAttachInvoice|TestConvertToPDFAKeepsExtensionSchemas|TestInvoiceNotAnInvoice|TestInvoiceReadsOlderGenerations" .`
Expected: FAIL — `doc.AttachInvoice undefined`.

- [ ] **Step 3: Write `einvoice_xmp.go`**

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"strings"
)

// XMP namespaces of the three generations of hybrid invoices. The first is
// written; all three are read.
const (
	nsFacturX  = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"
	nsZUGFeRD2 = "urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#"
	nsZUGFeRD1 = "urn:ferd:pdfa:CrossIndustryDocument:invoice:1p0#"
)

// nsPDFAPrefix covers the PDF/A metadata namespaces (identification and the
// extension-schema vocabulary), all under one URI prefix.
const nsPDFAPrefix = "http://www.aiim.org/pdfa/ns/"

// reXMPDescription matches one rdf:Description element. The extension
// schemas this library writes contain no nested rdf:Description (their
// structures use rdf:parseType="Resource"), so a non-greedy match is exact
// for them.
var reXMPDescription = regexp.MustCompile(`(?s)<rdf:Description\b.*?</rdf:Description>\s*`)

// xmpExtensionBlocks separates the rdf:Description elements that declare PDF/A
// extension schemas from the rest of an XMP packet.
func xmpExtensionBlocks(packet string) (blocks []string, rest string) {
	rest = reXMPDescription.ReplaceAllStringFunc(packet, func(m string) string {
		if strings.Contains(m, "pdfaExtension:schemas") {
			blocks = append(blocks, strings.TrimSpace(m))
			return ""
		}
		return m
	})
	return blocks, rest
}

// insertXMPDescriptions puts rdf:Description elements back into a packet,
// just before </rdf:RDF>.
func insertXMPDescriptions(packet string, blocks []string) string {
	if len(blocks) == 0 {
		return packet
	}
	i := strings.LastIndex(packet, "</rdf:RDF>")
	if i < 0 {
		return packet
	}
	return packet[:i] + strings.Join(blocks, "\n") + "\n" + packet[i:]
}

// facturXExtensionSchema declares the fx: properties to PDF/A validators,
// which reject properties from a namespace no extension schema describes.
const facturXExtensionSchema = `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">
<pdfaExtension:schemas>
<rdf:Bag>
<rdf:li rdf:parseType="Resource">
<pdfaSchema:schema>Factur-X PDFA Extension Schema</pdfaSchema:schema>
<pdfaSchema:namespaceURI>urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#</pdfaSchema:namespaceURI>
<pdfaSchema:prefix>fx</pdfaSchema:prefix>
<pdfaSchema:property>
<rdf:Seq>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>DocumentFileName</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The name of the embedded XML document</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>DocumentType</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The type of the hybrid document in capital letters, e.g. INVOICE or ORDER</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>Version</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The actual version of the standard applying to the embedded XML document</pdfaProperty:description>
</rdf:li>
<rdf:li rdf:parseType="Resource">
<pdfaProperty:name>ConformanceLevel</pdfaProperty:name>
<pdfaProperty:valueType>Text</pdfaProperty:valueType>
<pdfaProperty:category>external</pdfaProperty:category>
<pdfaProperty:description>The conformance level of the embedded XML document</pdfaProperty:description>
</rdf:li>
</rdf:Seq>
</pdfaSchema:property>
</rdf:li>
</rdf:Bag>
</pdfaExtension:schemas>
</rdf:Description>`
```

- [ ] **Step 4: Keep extension schemas through `setPDFAMetadata`**

In `pdfa_convert.go`, change `setPDFAMetadata` so it (a) captures extension blocks from the raw packet before rewriting, (b) drops every custom property in a PDF/A namespace (pdfaid is re-added; extension-schema fields that the parser may have flattened must not come back as top-level properties), and (c) puts the blocks back after `SetXMP`:

```go
func (d *Document) setPDFAMetadata(format PDFAFormat) error {
	var extensions []string
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		extensions, _ = xmpExtensionBlocks(string(raw))
	}
	meta, _ := d.XMP()
	info, _ := d.Info()
	if meta.Title == "" {
		meta.Title = info.Title
	}
	if len(meta.Authors) == 0 && info.Author != "" {
		meta.Authors = []string{info.Author}
	}
	if meta.Producer == "" {
		meta.Producer = info.Producer
	}
	if meta.CreatorTool == "" {
		meta.CreatorTool = info.Creator
	}
	// Replace the pdfaid properties, and drop anything else in the PDF/A
	// namespaces: extension schemas travel as whole blocks (below), never as
	// loose properties.
	var custom []XMPProperty
	for _, p := range meta.Custom {
		if p.Prefix != "pdfaid" && !strings.HasPrefix(p.Namespace, nsPDFAPrefix) {
			custom = append(custom, p)
		}
	}
	custom = append(custom,
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "part", Value: fmt.Sprintf("%d", format.part())},
		XMPProperty{Namespace: nsPDFAID, Prefix: "pdfaid", Name: "conformance", Value: format.conformance()},
	)
	meta.Custom = custom
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	if len(extensions) == 0 {
		return nil
	}
	raw, err := d.XMPRaw()
	if err != nil {
		return err
	}
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), extensions)))
}
```

(add `"strings"` to `pdfa_convert.go`'s imports if absent). If `SetXMPRaw` wraps or validates the packet in a way that rejects the inserted block, stop and report NEEDS_CONTEXT with the error rather than working around it.

- [ ] **Step 5: Append the invoice API to `einvoice.go`**

```go
// InvoiceOptions configures AttachInvoice. The zero value detects the profile
// from the XML, names the file as the standard prescribes, and produces
// PDF/A-3b.
type InvoiceOptions struct {
	FileName string         // default: "factur-x.xml", or "xrechnung.xml" for XRechnung
	Profile  InvoiceProfile // default: read from the XML's guideline ID
	Format   PDFAFormat     // PDFA3B or PDFA3A; the zero value (PDFA1B, never valid here) means PDFA3B
}

// Invoice is an e-invoice found in a document.
type Invoice struct {
	XML      []byte         // the embedded invoice, byte for byte
	FileName string         // the attachment name
	Profile  InvoiceProfile // from the XMP ConformanceLevel, else from the XML
	Standard string         // "Factur-X 1.0 / ZUGFeRD 2.1+", "ZUGFeRD 2.0" or "ZUGFeRD 1.0"
	Version  string         // the XMP Version property; "" when absent
}

// invoiceFileNames are the attachment names the standards have used.
var invoiceFileNames = []string{"factur-x.xml", "xrechnung.xml", "zugferd-invoice.xml"}

// AttachInvoice turns the document into a hybrid e-invoice (Factur-X 1.0 /
// ZUGFeRD 2.1+): it embeds the caller's CII XML as an associated file,
// writes the Factur-X XMP properties with the extension schema that declares
// them, and converts the document to PDF/A-3. Calling it again replaces the
// invoice. The returned report is the PDF/A result; conversion removes any
// encryption, as PDF/A requires. Mirrors the intent of Aspose.PDF for .NET's
// PdfFormat.ZUGFeRD conversion.
func (d *Document) AttachInvoice(xmlData []byte, opts ...InvoiceOptions) (*PDFAValidationReport, error) {
	var o InvoiceOptions
	if len(opts) > 0 {
		o = opts[len(opts)-1]
	}
	// PDFAFormat's zero value is PDFA1B, which an e-invoice can never be, so
	// the zero value means "not set" here.
	if o.Format == PDFA1B {
		o.Format = PDFA3B
	}
	if o.Format.part() != 3 {
		return nil, fmt.Errorf("AttachInvoice: an e-invoice must be PDF/A-3, not %v", o.Format)
	}
	root, guideline, err := invoiceGuideline(xmlData)
	if err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	if root.Local != "CrossIndustryInvoice" || root.Space != nsCII {
		return nil, fmt.Errorf("AttachInvoice: the XML is not a Cross Industry Invoice (root %s in %q)", root.Local, root.Space)
	}
	profile := o.Profile
	if profile == InvoiceProfileUnknown {
		profile = invoiceProfileFromGuideline(guideline)
	}
	if profile == InvoiceProfileUnknown {
		return nil, fmt.Errorf("AttachInvoice: unknown profile %q; set InvoiceOptions.Profile", guideline)
	}
	name := o.FileName
	if name == "" {
		name = profile.defaultFileName()
	}

	// Replace a previous invoice under any of the standard names.
	ef := d.EmbeddedFiles()
	for _, existing := range ef.Names() {
		for _, n := range invoiceFileNames {
			if strings.EqualFold(existing, n) {
				ef.Remove(existing)
			}
		}
	}
	f, err := ef.addBytesWith(name, xmlData, "text/xml", "Factur-X invoice")
	if err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	f.SetAFRelationship(profile.relationship())

	if err := d.setInvoiceXMP(name, profile); err != nil {
		return nil, fmt.Errorf("AttachInvoice: %w", err)
	}
	return d.ConvertToPDFA(o.Format)
}

// setInvoiceXMP writes the fx: properties and the extension schema declaring
// them, replacing any earlier invoice metadata of any generation.
func (d *Document) setInvoiceXMP(fileName string, profile InvoiceProfile) error {
	var extensions []string
	if raw, err := d.XMPRaw(); err == nil && len(raw) > 0 {
		blocks, _ := xmpExtensionBlocks(string(raw))
		for _, b := range blocks {
			if !strings.Contains(b, nsFacturX) {
				extensions = append(extensions, b) // another producer's schema: keep it
			}
		}
	}
	meta, _ := d.XMP()
	var custom []XMPProperty
	for _, p := range meta.Custom {
		switch {
		case p.Namespace == nsFacturX, p.Namespace == nsZUGFeRD2, p.Namespace == nsZUGFeRD1:
		case strings.HasPrefix(p.Namespace, nsPDFAPrefix) && p.Prefix != "pdfaid":
		default:
			custom = append(custom, p)
		}
	}
	custom = append(custom,
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "DocumentType", Value: "INVOICE"},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "DocumentFileName", Value: fileName},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "Version", Value: "1.0"},
		XMPProperty{Namespace: nsFacturX, Prefix: "fx", Name: "ConformanceLevel", Value: profile.String()},
	)
	meta.Custom = custom
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	raw, err := d.XMPRaw()
	if err != nil {
		return err
	}
	extensions = append(extensions, facturXExtensionSchema)
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), extensions)))
}

// Invoice returns the e-invoice embedded in the document, or (nil, nil) when
// the document is not one. It reads Factur-X 1.0 / ZUGFeRD 2.1+, ZUGFeRD 2.0
// and ZUGFeRD 1.0 files: the attachment is found by the name the XMP gives,
// else by the names the standards use.
func (d *Document) Invoice() (*Invoice, error) {
	var ns, xmpName, level, version string
	if meta, err := d.XMP(); err == nil {
		for _, p := range meta.Custom {
			switch p.Namespace {
			case nsFacturX, nsZUGFeRD2, nsZUGFeRD1:
				ns = p.Namespace
				switch p.Name {
				case "DocumentFileName":
					xmpName = p.Value
				case "ConformanceLevel":
					level = p.Value
				case "Version":
					version = p.Value
				}
			}
		}
	}

	ef := d.EmbeddedFiles()
	var f *EmbeddedFile
	for _, existing := range ef.Names() {
		if xmpName != "" && strings.EqualFold(existing, xmpName) {
			f = ef.Get(existing)
			break
		}
	}
	if f == nil {
	search:
		for _, existing := range ef.Names() {
			for _, n := range invoiceFileNames {
				if strings.EqualFold(existing, n) {
					f = ef.Get(existing)
					break search
				}
			}
		}
	}
	if f == nil {
		return nil, nil
	}
	data, err := f.Data()
	if err != nil {
		return nil, fmt.Errorf("Invoice: %w", err)
	}

	root, guideline, _ := invoiceGuideline(data)
	profile := invoiceProfileFromLevel(level)
	if profile == InvoiceProfileUnknown {
		profile = invoiceProfileFromGuideline(guideline)
	}

	var standard string
	switch {
	case ns == nsZUGFeRD1, root.Local == "CrossIndustryDocument":
		standard = "ZUGFeRD 1.0"
	case ns == nsZUGFeRD2, ns == "" && strings.EqualFold(f.Name(), "zugferd-invoice.xml"):
		standard = "ZUGFeRD 2.0"
	default:
		standard = "Factur-X 1.0 / ZUGFeRD 2.1+"
	}
	return &Invoice{XML: data, FileName: f.Name(), Profile: profile, Standard: standard, Version: version}, nil
}
```

- [ ] **Step 6: Run the tests**

Run: `go test -run "TestAttachInvoice|TestConvertToPDFAKeepsExtensionSchemas|TestInvoice|TestPDFA|TestConvertToPDFA|TestXMP" .`
Expected: PASS. If `report.Conformant` is false in `TestAttachInvoiceRoundTrip`, print the issues: a rule unrelated to e-invoices (fonts, colour) means the test document needs adjusting, not the feature; a rule about XMP or attachments is a real defect to fix.

- [ ] **Step 7: Commit**

```bash
git add einvoice.go einvoice_xmp.go einvoice_internal_test.go pdfa_convert.go
git commit -m "$(cat <<'EOF'
feat: AttachInvoice and Invoice for ZUGFeRD / Factur-X (pdf-go-t6s9)

AttachInvoice embeds the caller's CII XML as factur-x.xml (xrechnung.xml
for XRechnung) with MIME text/xml and the relationship the profile calls
for, writes the fx: XMP properties with the extension schema that declares
them, and converts to PDF/A-3; a second call replaces the invoice.
Invoice() finds and classifies an embedded invoice from Factur-X / ZUGFeRD
2.1+, ZUGFeRD 2.0 and ZUGFeRD 1.0 files.

ConvertToPDFA used to rebuild the XMP from parsed fields and so dropped
every PDF/A extension-schema block — ours and any other producer's — which
PDF/A requires for custom namespaces. It now carries them through whole.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Independent check with factur-x, documentation

**Files:**
- Throwaway (never committed): `zz_einvoice_dump_test.go` in the repo root while running; `result_files/einvoice/` (venv, outputs, `check.py`)
- Modify: `CLAUDE.md`, `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Write sample files**

Create `zz_einvoice_dump_test.go` (deleted in Step 4):

```go
package asposepdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestZZEInvoiceDump(t *testing.T) {
	dir := filepath.Join("result_files", "einvoice")
	_ = os.MkdirAll(dir, 0o755)
	for name, urn := range map[string]string{
		"minimum":  "urn:factur-x.eu:1p0:minimum",
		"en16931":  "urn:cen.eu:en16931:2017",
		"extended": "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended",
	} {
		doc := invoiceTestDoc(t)
		if _, err := doc.AttachInvoice(ciiInvoice(urn)); err != nil {
			t.Fatal(err)
		}
		if err := doc.Save(filepath.Join(dir, name+".pdf")); err != nil {
			t.Fatal(err)
		}
	}
}
```

Run: `go test -run TestZZEInvoiceDump -count=1 .`
Expected: `result_files/einvoice/{minimum,en16931,extended}.pdf`.

- [ ] **Step 2: Install factur-x into a throwaway virtualenv**

```bash
python -m venv result_files/einvoice/venv
result_files/einvoice/venv/Scripts/python -m pip install -q factur-x
```

(on a POSIX shell the interpreter is `result_files/einvoice/venv/bin/python`). Never install into the system Python.

- [ ] **Step 3: Check the files**

Create `result_files/einvoice/check.py`. The factur-x API names changed between releases; confirm them first with `result_files/einvoice/venv/Scripts/python -c "import facturx; print([n for n in dir(facturx) if not n.startswith('_')])"` and adapt the calls below to what is exported (the functions for extracting the XML from a PDF and for checking XML against the XSD):

```python
import sys, pathlib
import facturx

ok = True
for name in ["minimum", "en16931", "extended"]:
    pdf = pathlib.Path("result_files/einvoice") / f"{name}.pdf"
    xml_filename, xml_bytes = facturx.get_xml_from_pdf(str(pdf), check_xsd=False)
    print(name, "embedded file:", xml_filename, len(xml_bytes), "bytes")
    level = facturx.get_level(facturx.etree.fromstring(xml_bytes)) if hasattr(facturx, "get_level") else "?"
    print(name, "level detected by factur-x:", level)
    if name == "minimum":
        try:
            facturx.xml_check_xsd(xml_bytes, flavor="factur-x", level="minimum")
            print(name, "XSD: valid")
        except Exception as e:
            ok = False
            print(name, "XSD FAILED:", e)
    try:
        facturx.get_facturx_xml_from_pdf(str(pdf), check_xsd=False)  # exercises the XMP/attachment lookup path
    except Exception as e:
        print(name, "lookup note:", e)
sys.exit(0 if ok else 1)
```

Run: `result_files/einvoice/venv/Scripts/python result_files/einvoice/check.py`
Expected: each file reports `factur-x.xml` and its byte length, the detected level matches, and the MINIMUM XML is XSD-valid.

Also inspect the XMP of one file with the library's metadata reader if it exposes one (for example a function returning the fx: fields), or with pikepdf: `result_files/einvoice/venv/Scripts/python -m pip install -q pikepdf` and print `pdf.open_metadata()` entries for the `fx` namespace — `DocumentType` INVOICE, `DocumentFileName` factur-x.xml, `Version` 1.0, `ConformanceLevel` as written.

If the MINIMUM XSD check fails on the *sample XML*, fix the sample in `ciiInvoice` (it is test data) until it validates, re-run Steps 1 and 3, and include that change in this task's commit. If factur-x cannot find the XML or the XMP fields, that is a defect in the feature: report DONE_WITH_CONCERNS with the output rather than changing library code in this task.

- [ ] **Step 4: Remove the throwaway test**

Run: `rm zz_einvoice_dump_test.go` and confirm `git status` does not list it.

- [ ] **Step 5: Documentation**

`CLAUDE.md` — add after the `collection.go` block in the Public API section:

```markdown
**`associated_files.go` / `einvoice.go` / `einvoice_xmp.go`** — PDF/A-3 associated files and hybrid e-invoices (ZUGFeRD 2.x / Factur-X 1.0; epic `pdf-go-t6s9`; design `docs/superpowers/specs/2026-09-22-einvoice-facturx-design.md`). Mirrors Aspose.PDF for .NET's `FileSpecification.AFRelationship` and `PdfFormat.ZUGFeRD`
- `AFRelationship` (`AFUnspecified`/`AFSource`/`AFData`/`AFAlternative`/`AFSupplement`) — `(*EmbeddedFile).AFRelationship()` / `SetAFRelationship(r)` write `/AFRelationship` on the file specification and list it once in the catalog `/AF` (a direct file-spec dict is promoted to an object first); removing or replacing the attachment drops it from `/AF`. Every embedded file now carries `/Params /ModDate`. `ValidatePDFA` for PDF/A-3 reports `EMBEDDED_FILE_NOT_ASSOCIATED` (ISO 19005-3 §6.8) and `ConvertToPDFA` to PDF/A-3 repairs it (`associatePDFAEmbeddedFiles`: existing relationship kept, missing → Unspecified, `/AF` + `/ModDate`)
- `(*Document).AttachInvoice(xml, InvoiceOptions{FileName, Profile, Format})` — the caller supplies CII XML (generation is out of scope); checked for the `CrossIndustryInvoice` root in `urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100`, profile read from the guideline ID (`invoiceGuideline` stops at it) unless given; embedded as `factur-x.xml` (`xrechnung.xml` for XRechnung), MIME `text/xml`, relationship `Data` for MINIMUM/BASIC WL and `Alternative` above; `fx:DocumentType/DocumentFileName/Version/ConformanceLevel` written in `urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#` plus the PDF/A extension schema declaring them; then `ConvertToPDFA` (PDFA3B default, PDFA3A allowed, anything else an error). A second call replaces the invoice and its metadata
- `(*Document).Invoice() (*Invoice, error)` — `Invoice{XML, FileName, Profile, Standard, Version}`; `(nil, nil)` when absent. Finds the attachment by the XMP `DocumentFileName`, else `factur-x.xml`/`xrechnung.xml`/`zugferd-invoice.xml` (case-insensitive, so ZUGFeRD 1.0's `ZUGFeRD-invoice.xml` too); profile from the XMP `ConformanceLevel` (`invoiceProfileFromLevel`, COMFORT → EN 16931), else the XML; `Standard` from the XMP namespace (Factur-X/ZUGFeRD 2.1+, ZUGFeRD 2.0 `…invoice:2p0#`, ZUGFeRD 1.0 `urn:ferd:…`), else the XML root/file name
- **XMP extension schemas survive `ConvertToPDFA`**: `setPDFAMetadata` rebuilds the packet from parsed fields, which used to drop every `pdfaExtension:schemas` block — ours and any other producer's — though PDF/A requires one for each custom namespace. It now lifts those `rdf:Description` blocks out first (`xmpExtensionBlocks`), drops custom properties in the `http://www.aiim.org/pdfa/ns/` namespaces so flattened schema fields never reappear as loose properties, and re-inserts the blocks (`insertXMPDescriptions`)
- Validated with the Python `factur-x` library (Akretion): it finds `factur-x.xml`, detects the level, and the MINIMUM sample passes the official XSD. Full PDF/A-3 conformance still needs veraPDF (JRE absent). Out of scope: CII generation, EN 16931 Schematron, writing ZUGFeRD 1.0/2.0, UBL, Order-X
```

`README.md` — add a bullet next to the PDF/A bullet (match the surrounding indentation and wrapping; find it with `grep -n "PDF/A" README.md`):

```markdown
- **E-invoices (ZUGFeRD / Factur-X)** — `Document.AttachInvoice(xml)` turns a document into a hybrid
  e-invoice: the CII XML is embedded as `factur-x.xml` with the associated-file relationship its
  profile calls for, the Factur-X metadata and extension schema are written, and the document is
  converted to PDF/A-3. `Document.Invoice()` extracts the invoice from incoming Factur-X and ZUGFeRD
  2.x/1.0 files together with its profile. Attachments also gain PDF/A-3 associated-file
  relationships (`EmbeddedFile.SetAFRelationship`).
```

and rows in the types table, in alphabetical position:

```markdown
| `AFRelationship` | AFRelationship says how an embedded file relates to the document. |
| `Invoice` | Invoice is an e-invoice found in a document. |
| `InvoiceOptions` | InvoiceOptions configures AttachInvoice. |
| `InvoiceProfile` | InvoiceProfile is the Factur-X / ZUGFeRD conformance level of an invoice. |
```

`CHANGELOG.md` — first bullet under `## [Unreleased]` → `### Added`:

```markdown
- **E-invoices: ZUGFeRD / Factur-X** — `Document.AttachInvoice(xml, InvoiceOptions{...})` packages a caller-supplied Cross Industry Invoice as a hybrid e-invoice: embedded as `factur-x.xml` (or `xrechnung.xml`) with the relationship its profile requires, described by the `fx:` XMP properties and the PDF/A extension schema that declares them, and converted to PDF/A-3. `Document.Invoice()` extracts the XML, file name, profile (MINIMUM … EXTENDED, XRECHNUNG) and standard from Factur-X 1.0 / ZUGFeRD 2.1+, ZUGFeRD 2.0 and ZUGFeRD 1.0 files. Checked with the `factur-x` reference library, including the official XSD. Attachments gain PDF/A-3 associated-file relationships (`EmbeddedFile.AFRelationship`/`SetAFRelationship`, catalog `/AF`, `/ModDate`); `ValidatePDFA` reports attachments without one for PDF/A-3 and `ConvertToPDFA` repairs them. Mirrors Aspose.PDF for .NET's `PdfFormat.ZUGFeRD` and `FileSpecification.AFRelationship`. (`pdf-go-t6s9`)
```

and under `### Fixed`:

```markdown
- **`ConvertToPDFA` dropped XMP extension schemas** — rewriting the metadata packet lost every `pdfaExtension:schemas` block, which PDF/A requires for each custom namespace a document uses, so converting a document that carried one (from any producer) broke its conformance. They are now kept whole.
```

- [ ] **Step 6: Full gate and commit**

Run: `gofmt -l associated_files.go embedded_files.go validate_pdfa.go pdfa_convert.go einvoice.go einvoice_xmp.go associated_files_internal_test.go einvoice_internal_test.go; go vet ./... && go test ./... && golangci-lint run`
Expected: gofmt lists nothing, vet silent, tests pass, `0 issues`.

```bash
git add CLAUDE.md README.md CHANGELOG.md einvoice_internal_test.go
git commit -m "$(cat <<'EOF'
docs: ZUGFeRD / Factur-X e-invoices (pdf-go-t6s9)

Checked with the factur-x reference library: it finds factur-x.xml in our
output, detects the level, and the MINIMUM sample passes the official XSD.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
bd comments add pdf-go-t6s9 "ZUGFeRD / Factur-X increment shipped: AttachInvoice, Invoice(), PDF/A-3 associated files. Remaining in this epic: PDF/A-2U/3U, PDF/A-4, PDF/X."
```

(`einvoice_internal_test.go` is in the `git add` only if Step 3 changed the sample XML. If `bd comments add` is not the installed CLI's syntax, use `bd update pdf-go-t6s9 --notes "…"` or skip it and say so.)

---

## Self-Review

**Spec coverage.** Associated files API, `/AF` maintenance and `/ModDate` — Task 1. PDF/A-3 rule and conversion repair — Task 2. Profiles, URN table (Factur-X, ZUGFeRD 2.0 and 1.0 forms), spellings, file names, relationships — Task 3. `AttachInvoice` steps 1–6, replacement on a second call, `InvoiceOptions` defaults and the PDF/A-3-only rule, XMP properties and extension schema — Task 4. `Invoice()` with the three generations, `(nil, nil)` for non-invoices, profile from XMP else XML, `Standard` from namespace else root/file name — Task 4. Errors listed in the spec — `TestAttachInvoiceErrors`. Independent check with factur-x in a throwaway virtualenv, and the veraPDF limit — Task 5. The extension-schema loss in `ConvertToPDFA` found while planning is fixed in Task 4 and recorded under Fixed in Task 5.

**Placeholder scan.** Task 5 Step 3 asks the implementer to confirm the factur-x function names before running, because they have changed between releases; the intent of each call is stated. Task 3 Step 1 and Task 4 Step 1 each end with a named line to delete once later code uses the import.

**Type consistency.** `AFRelationship` constants, `EmbeddedFile.ref`, `hasAFRelationship`, `isAssociatedFile`, `addBytesWith` (Task 1) are used unchanged in Tasks 2 and 4. `InvoiceProfile`, `invoiceGuideline`, `invoiceProfileFromGuideline`, `invoiceProfileFromLevel`, `defaultFileName`, `relationship`, `nsCII` (Task 3) are used unchanged in Task 4. `xmpExtensionBlocks`/`insertXMPDescriptions`/`nsPDFAPrefix` (Task 4, `einvoice_xmp.go`) are used by both `setPDFAMetadata` and `setInvoiceXMP`. Test helpers `ciiInvoice`, `invoiceTestDoc`, `saveAndReopen`, `hasRule`, `catalogAFNums` are each defined once.
