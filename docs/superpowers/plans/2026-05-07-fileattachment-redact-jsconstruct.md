# FileAttachment + Redact + JavaScript Construct Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the final tranche of the annotations epic (`pdf-go-37n`): `FileAttachmentAnnotation` (embedded file with icon), `RedactAnnotation` with full mark + apply (irreversible content removal of text/images/paths), and `NewJavaScriptAction` public constructor.

**Architecture:** Three-bucket split — declarative API (annotation types), /AP rendering (mark-mode visuals only), apply machinery (the largest new piece — content-stream rewriting). FileAttachment is the simplest type. Redact apply requires walking and rewriting page content streams: text glyph removal (BT/ET), image clipping (Do invocations), path clipping (drawing operators).

**Tech Stack:** Go 1.24, pure standard library, `bytes.Buffer`, `mime.TypeByExtension`, `path/filepath`. pypdf 6.x for external cross-verification (Task 15 only).

**Reference:** [docs/superpowers/specs/2026-05-07-fileattachment-redact-jsconstruct-design.md](../specs/2026-05-07-fileattachment-redact-jsconstruct-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `annotation_fileattachment.go` (new) | `FileAttachmentAnnotation` + `FileAttachmentIcon` enum + accessors + file embedding helpers (file → /Filespec dict + /EmbeddedFile stream) + parse helper |
| `annotation_redact.go` (new) | `RedactAnnotation` + accessors (QuadPoints/InteriorColor/OverlayText/RepeatOverlayText/OverlayTextStyle) + parse helper |
| `appearance_redact.go` (new) | `generateRedactAppearance` — mark-mode visual: /IC fill of /QuadPoints regions + optional /OverlayText preview |
| `redact_apply.go` (new) | `(*Document).ApplyRedactions()` + `(*Document).ValidateRedactions()` + per-page orchestration |
| `redact_apply_text.go` (new) | `rewriteTextOperatorsInStream` — BT/ET walker, glyph position computation, Tj/TJ/'/" rewriting |
| `redact_apply_image.go` (new) | `rewriteImageOperatorsInStream` — Do walker with CTM tracking, bbox intersection, clip-path wrapping |
| `redact_apply_path.go` (new) | `rewritePathOperatorsInStream` — path-construction walker, accumulated bbox, clip-path wrapping |
| `annotation_action.go` (modify) | Add `NewJavaScriptAction(script string)` public constructor with security warning |
| `annotation.go` (modify) | Extend `parseAnnotation` switch +2 cases + extend `AnnotationType` enum +2 |
| `annotation_fileattachment_test.go` (new) | External round-trip tests for FileAttachment |
| `annotation_redact_test.go` (new) | External round-trip tests for Redact mark-mode |
| `redact_apply_test.go` (new) | Apply integration tests |
| `redact_apply_internal_test.go` (new) | Internal tests for rewriter helpers |
| `appearance_redact_internal_test.go` (new) | Internal tests for `generateRedactAppearance` |
| `annotation_action_test.go` (modify) | Add `TestNewJavaScriptAction` round-trip |
| `CLAUDE.md`, `README.md` (modify, Task 15) | Public API docs |

---

## Task 1: Common types — FileAttachmentIcon enum + AnnotationType extension

**Files:**
- Create: `annotation_fileattachment.go` (placeholder, full content in Task 2)
- Modify: `annotation.go`
- Create: `annotation_fileattachment_test.go`

- [ ] **Step 1: Write the failing tests**

Create `annotation_fileattachment_test.go`:
```go
package asposepdf_test

import (
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestFileAttachmentIconConstants(t *testing.T) {
    all := []pdf.FileAttachmentIcon{
        pdf.FileAttachmentIconUnknown,
        pdf.FileAttachmentIconGraph,
        pdf.FileAttachmentIconPaperclip,
        pdf.FileAttachmentIconPushPin,
        pdf.FileAttachmentIconTag,
    }
    for i, v := range all {
        if int(v) != i {
            t.Errorf("FileAttachmentIcon[%d] = %d, want %d", i, int(v), i)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -run TestFileAttachmentIconConstants -v ./...
```

Expected: build failure.

- [ ] **Step 3: Define type**

Create `annotation_fileattachment.go`:
```go
package asposepdf

// FileAttachmentIcon names per ISO 32000-1 §12.5.6.15 Table 178.
type FileAttachmentIcon int

const (
    FileAttachmentIconUnknown FileAttachmentIcon = iota
    FileAttachmentIconGraph
    FileAttachmentIconPaperclip   // PDF default
    FileAttachmentIconPushPin
    FileAttachmentIconTag
)
```

In `annotation.go`, append to the `AnnotationType` const block (after the existing constants):
```go
    AnnotationTypeFileAttachment
    AnnotationTypeRedact
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -run TestFileAttachmentIconConstants -v ./...
go test ./...
git add annotation.go annotation_fileattachment.go annotation_fileattachment_test.go
git commit -m "feat: FileAttachmentIcon enum + AnnotationType extension"
```

---

## Task 2: FileAttachmentAnnotation skeleton + Icon/Open accessors + parse dispatch

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_fileattachment.go`
- Modify: `annotation_fileattachment_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_fileattachment_test.go`:
```go
import "bytes"

func TestFileAttachmentAnnotationRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
    fa.SetIcon(pdf.FileAttachmentIconPushPin)
    fa.SetTitle("Reviewer")
    fa.SetContents("Attached document")
    if err := page.Annotations().Add(fa); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    got := doc2.Pages()[0].Annotations().At(0)
    if got.AnnotationType() != pdf.AnnotationTypeFileAttachment {
        t.Errorf("type = %v, want AnnotationTypeFileAttachment", got.AnnotationType())
    }
    fa2, ok := got.(*pdf.FileAttachmentAnnotation)
    if !ok {
        t.Fatalf("concrete type = %T", got)
    }
    if fa2.Icon() != pdf.FileAttachmentIconPushPin {
        t.Errorf("Icon = %v, want PushPin", fa2.Icon())
    }
    if fa2.Title() != "Reviewer" {
        t.Errorf("Title = %q", fa2.Title())
    }
    if fa2.Contents() != "Attached document" {
        t.Errorf("Contents = %q", fa2.Contents())
    }
}

func TestFileAttachmentAnnotationAllIcons(t *testing.T) {
    icons := []struct {
        icon pdf.FileAttachmentIcon
        name string
    }{
        {pdf.FileAttachmentIconGraph, "Graph"},
        {pdf.FileAttachmentIconPaperclip, "Paperclip"},
        {pdf.FileAttachmentIconPushPin, "PushPin"},
        {pdf.FileAttachmentIconTag, "Tag"},
    }
    for _, tc := range icons {
        t.Run(tc.name, func(t *testing.T) {
            doc := pdf.NewDocument(595, 842)
            page, _ := doc.Page(1)
            fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
            fa.SetIcon(tc.icon)
            page.Annotations().Add(fa)
            var buf bytes.Buffer
            doc.WriteTo(&buf)
            doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
            fa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FileAttachmentAnnotation)
            if got := fa2.Icon(); got != tc.icon {
                t.Errorf("icon = %v, want %v", got, tc.icon)
            }
        })
    }
}

func TestFileAttachmentAnnotationDefaultIcon(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
    if got := fa.Icon(); got != pdf.FileAttachmentIconPaperclip {
        t.Errorf("default Icon = %v, want Paperclip", got)
    }
    if fa.HasFile() {
        t.Errorf("HasFile = true on fresh annotation")
    }
}

func TestFileAttachmentAnnotationConstructorPanicOnNilPage(t *testing.T) {
    defer func() {
        if r := recover(); r == nil {
            t.Error("expected panic, got none")
        }
    }()
    pdf.NewFileAttachmentAnnotation(nil, pdf.Point{X: 0, Y: 0})
}

func TestFileAttachmentAnnotationDefaultRect(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
    r := fa.Rect()
    if r.LLX != 100 || r.LLY != 700 || r.URX != 124 || r.URY != 724 {
        t.Errorf("Rect = %+v, want LLX=100 LLY=700 URX=124 URY=724", r)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestFileAttachmentAnnotation -v ./...
```

Expected: build failure.

- [ ] **Step 3: Add /FileAttachment dispatch to parseAnnotation**

In `annotation.go`, add a case before the GenericAnnotation default:
```go
    case "/FileAttachment":
        return parseFileAttachmentAnnotation(base)
```

- [ ] **Step 4: Add FileAttachmentAnnotation type**

Append to `annotation_fileattachment.go`:
```go
// FileAttachmentAnnotation embeds a file in the document and shows an
// icon at the annotation's /Rect. Per ISO 32000-1 §12.5.6.15. No /AP
// is generated — viewers render the icon themselves based on /Name.
type FileAttachmentAnnotation struct {
    annotationBase
}

func (a *FileAttachmentAnnotation) AnnotationType() AnnotationType {
    return AnnotationTypeFileAttachment
}

// NewFileAttachmentAnnotation builds an unbound file-attachment
// annotation. Page must be non-nil. The /Rect is auto-computed as a
// 24×24 pt square anchored at position (Acrobat icon convention).
//
// Call SetFile or SetFileFromStream to embed file data; without that,
// the annotation has no /FS entry and viewers render an empty icon.
func NewFileAttachmentAnnotation(page *Page, position Point) *FileAttachmentAnnotation {
    if page == nil {
        panic("NewFileAttachmentAnnotation: nil page")
    }
    dict := pdfDict{
        "/Type":    pdfName("/Annot"),
        "/Subtype": pdfName("/FileAttachment"),
        "/Rect":    pdfArray{position.X, position.Y, position.X + 24, position.Y + 24},
        "/Name":    pdfName("/Paperclip"),
    }
    return &FileAttachmentAnnotation{annotationBase: annotationBase{
        dict: dict,
        doc:  page.doc,
        page: page,
    }}
}

// Icon returns the /Name entry mapped to a FileAttachmentIcon.
// Returns FileAttachmentIconPaperclip (the spec default) if absent.
func (a *FileAttachmentAnnotation) Icon() FileAttachmentIcon {
    n, ok := a.dict["/Name"].(pdfName)
    if !ok {
        return FileAttachmentIconPaperclip
    }
    switch n {
    case "/Graph":
        return FileAttachmentIconGraph
    case "/Paperclip":
        return FileAttachmentIconPaperclip
    case "/PushPin":
        return FileAttachmentIconPushPin
    case "/Tag":
        return FileAttachmentIconTag
    }
    return FileAttachmentIconUnknown
}

// SetIcon writes the /Name entry. Unknown is encoded as /Paperclip.
func (a *FileAttachmentAnnotation) SetIcon(i FileAttachmentIcon) {
    var name pdfName
    switch i {
    case FileAttachmentIconGraph:
        name = "/Graph"
    case FileAttachmentIconPushPin:
        name = "/PushPin"
    case FileAttachmentIconTag:
        name = "/Tag"
    default: // Paperclip + Unknown
        name = "/Paperclip"
    }
    a.dict["/Name"] = name
}

// HasFile returns true if SetFile or SetFileFromStream has been called
// successfully and not subsequently cleared. Stub for now — full
// implementation in Task 3.
func (a *FileAttachmentAnnotation) HasFile() bool {
    if a.dict["/FS"] == nil {
        return false
    }
    return true
}

// RegenerateAppearance is a no-op for FileAttachmentAnnotation (no /AP
// — viewers render the icon themselves).
func (a *FileAttachmentAnnotation) RegenerateAppearance() {}

// parseFileAttachmentAnnotation builds a FileAttachmentAnnotation from
// a parsed dict.
func parseFileAttachmentAnnotation(base annotationBase) *FileAttachmentAnnotation {
    return &FileAttachmentAnnotation{annotationBase: base}
}
```

- [ ] **Step 5: Run tests + commit**

```bash
go test -run TestFileAttachmentAnnotation -v ./...
go test ./...
git add annotation.go annotation_fileattachment.go annotation_fileattachment_test.go
git commit -m "feat: FileAttachmentAnnotation skeleton + Icon accessors + parse dispatch"
```

---

## Task 3: FileAttachment file embedding (SetFile/SetFileFromStream + metadata accessors)

**Files:**
- Modify: `annotation_fileattachment.go`
- Modify: `annotation_fileattachment_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_fileattachment_test.go`:
```go
import (
    "io"
    "os"
    "strings"
)

func makeTestTextFile(t *testing.T, content string) string {
    t.Helper()
    f, err := os.CreateTemp("", "fileattach-*.txt")
    if err != nil {
        t.Fatal(err)
    }
    f.WriteString(content)
    f.Close()
    t.Cleanup(func() { os.Remove(f.Name()) })
    return f.Name()
}

func TestFileAttachmentSetFile(t *testing.T) {
    path := makeTestTextFile(t, "hello attached file")
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
    if fa.HasFile() {
        t.Error("HasFile = true before SetFile")
    }
    if err := fa.SetFile(path); err != nil {
        t.Fatalf("SetFile: %v", err)
    }
    if !fa.HasFile() {
        t.Error("HasFile = false after SetFile")
    }
    if !strings.HasSuffix(fa.FileName(), ".txt") {
        t.Errorf("FileName = %q, expected .txt suffix", fa.FileName())
    }
    if fa.FileSize() != len("hello attached file") {
        t.Errorf("FileSize = %d, want %d", fa.FileSize(), len("hello attached file"))
    }
    if got := string(fa.FileBytes()); got != "hello attached file" {
        t.Errorf("FileBytes = %q", got)
    }
}

func TestFileAttachmentSetFileRoundTrip(t *testing.T) {
    path := makeTestTextFile(t, "round-trip content")
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
    if err := fa.SetFile(path); err != nil {
        t.Fatalf("SetFile: %v", err)
    }
    fa.SetFileDescription("Test attachment")
    if err := page.Annotations().Add(fa); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    fa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.FileAttachmentAnnotation)
    if !fa2.HasFile() {
        t.Error("HasFile = false after roundtrip")
    }
    if got := string(fa2.FileBytes()); got != "round-trip content" {
        t.Errorf("FileBytes after roundtrip = %q, want \"round-trip content\"", got)
    }
    if got := fa2.FileDescription(); got != "Test attachment" {
        t.Errorf("FileDescription = %q", got)
    }
}

func TestFileAttachmentSetFileFromStream(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 100, Y: 700})
    r := strings.NewReader("stream content")
    if err := fa.SetFileFromStream(r, "data.bin"); err != nil {
        t.Fatalf("SetFileFromStream: %v", err)
    }
    if !fa.HasFile() {
        t.Error("HasFile = false")
    }
    if got := fa.FileName(); got != "data.bin" {
        t.Errorf("FileName = %q, want data.bin", got)
    }
    if got := string(fa.FileBytes()); got != "stream content" {
        t.Errorf("FileBytes = %q", got)
    }
}

func TestFileAttachmentFileBytesDefensiveCopy(t *testing.T) {
    path := makeTestTextFile(t, "original")
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
    if err := fa.SetFile(path); err != nil {
        t.Fatal(err)
    }
    bytes1 := fa.FileBytes()
    bytes1[0] = 'X'
    bytes2 := fa.FileBytes()
    if bytes2[0] == 'X' {
        t.Error("FileBytes returned shared mutable slice — caller mutation visible")
    }
}

func TestFileAttachmentSetFileInvalidPath(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
    if err := fa.SetFile("/nonexistent/path.txt"); err == nil {
        t.Error("expected error for non-existent file")
    }
}

func TestFileAttachmentMIMEDetection(t *testing.T) {
    cases := []struct {
        ext  string
        mime string
    }{
        {".pdf", "application/pdf"},
        {".txt", "text/plain"},
        {".png", "image/png"},
    }
    for _, tc := range cases {
        t.Run(tc.ext, func(t *testing.T) {
            f, err := os.CreateTemp("", "test-*"+tc.ext)
            if err != nil {
                t.Fatal(err)
            }
            f.WriteString("x")
            f.Close()
            defer os.Remove(f.Name())

            doc := pdf.NewDocument(595, 842)
            page, _ := doc.Page(1)
            fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 0, Y: 0})
            if err := fa.SetFile(f.Name()); err != nil {
                t.Fatalf("SetFile: %v", err)
            }
            mt := fa.FileMIMEType()
            if !strings.HasPrefix(mt, tc.mime) {
                t.Errorf("MIME type = %q, want prefix %q", mt, tc.mime)
            }
        })
    }
}

var _ io.Reader = (*strings.Reader)(nil) // silence unused import if needed
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestFileAttachmentSetFile -v ./...
```

Expected: build failure.

- [ ] **Step 3: Implement file-embedding methods**

Append to `annotation_fileattachment.go`. Add imports `"fmt"`, `"io"`, `"mime"`, `"os"`, `"path/filepath"`:

```go
// SetFile embeds the file at path as the annotation's attachment. The
// file's MIME type is auto-detected from the extension via
// mime.TypeByExtension; falls back to "application/octet-stream" for
// unknown extensions. Returns error on file-open or read failures.
func (a *FileAttachmentAnnotation) SetFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("FileAttachmentAnnotation.SetFile: %w", err)
    }
    name := filepath.Base(path)
    mt := detectMIMEType(name)
    return a.embedFileBytes(data, name, mt)
}

// SetFileFromStream is the io.Reader variant of SetFile. The caller
// supplies the displayed filename (used for /F entry and for MIME
// detection from extension).
func (a *FileAttachmentAnnotation) SetFileFromStream(r io.Reader, name string) error {
    data, err := io.ReadAll(r)
    if err != nil {
        return fmt.Errorf("FileAttachmentAnnotation.SetFileFromStream: %w", err)
    }
    return a.embedFileBytes(data, name, detectMIMEType(name))
}

// embedFileBytes is the common implementation: build /EmbeddedFile
// stream, build /Filespec dict, wire annotation's /FS reference.
func (a *FileAttachmentAnnotation) embedFileBytes(data []byte, name, mimeType string) error {
    // /EmbeddedFile stream.
    embeddedStream := &pdfStream{
        Dict: pdfDict{
            "/Type":    pdfName("/EmbeddedFile"),
            "/Subtype": pdfName("/" + escapePDFName(mimeType)),
            "/Length":  len(data),
        },
        Data:    data,
        Decoded: false, // already raw — let writer not re-compress
    }
    embedID := a.doc.nextID
    a.doc.nextID++
    a.doc.objects[embedID] = &pdfObject{Num: embedID, Value: embeddedStream}

    // /Filespec dict.
    filespec := pdfDict{
        "/Type": pdfName("/Filespec"),
        "/F":    name,
        "/UF":   name,
        "/EF": pdfDict{
            "/F":  pdfRef{Num: embedID},
            "/UF": pdfRef{Num: embedID},
        },
    }
    fsID := a.doc.nextID
    a.doc.nextID++
    a.doc.objects[fsID] = &pdfObject{Num: fsID, Value: filespec}

    a.dict["/FS"] = pdfRef{Num: fsID}
    return nil
}

// FileName returns the displayed filename from /Filespec/F. Empty if no file.
func (a *FileAttachmentAnnotation) FileName() string {
    fs := a.resolveFilespec()
    if fs == nil {
        return ""
    }
    if name, ok := fs["/UF"].(string); ok && name != "" {
        return name
    }
    if name, ok := fs["/F"].(string); ok {
        return name
    }
    return ""
}

// FileMIMEType returns the /Subtype on /EmbeddedFile (e.g. "application/pdf").
// Empty if no file or /Subtype missing.
func (a *FileAttachmentAnnotation) FileMIMEType() string {
    stream := a.resolveEmbeddedFile()
    if stream == nil {
        return ""
    }
    n, ok := stream.Dict["/Subtype"].(pdfName)
    if !ok {
        return ""
    }
    s := string(n)
    if len(s) > 0 && s[0] == '/' {
        s = s[1:]
    }
    return unescapePDFName(s)
}

// FileSize returns the size of the embedded file in bytes. Zero if no file.
func (a *FileAttachmentAnnotation) FileSize() int {
    stream := a.resolveEmbeddedFile()
    if stream == nil {
        return 0
    }
    return len(stream.Data)
}

// FileBytes returns a defensive copy of the embedded file's raw bytes.
// Nil if no file.
func (a *FileAttachmentAnnotation) FileBytes() []byte {
    stream := a.resolveEmbeddedFile()
    if stream == nil {
        return nil
    }
    out := make([]byte, len(stream.Data))
    copy(out, stream.Data)
    return out
}

// FileDescription returns /Filespec/Desc. Empty if no description set.
func (a *FileAttachmentAnnotation) FileDescription() string {
    fs := a.resolveFilespec()
    if fs == nil {
        return ""
    }
    return decodeFormString(fs["/Desc"])
}

// SetFileDescription writes /Filespec/Desc.
func (a *FileAttachmentAnnotation) SetFileDescription(s string) {
    fsRef, ok := a.dict["/FS"].(pdfRef)
    if !ok {
        return
    }
    obj, ok := a.doc.objects[fsRef.Num]
    if !ok {
        return
    }
    fs, ok := obj.Value.(pdfDict)
    if !ok {
        return
    }
    if s == "" {
        delete(fs, "/Desc")
    } else {
        fs["/Desc"] = encodeFormString(s)
    }
}

// resolveFilespec returns the /Filespec dict referenced by /FS, or nil.
func (a *FileAttachmentAnnotation) resolveFilespec() pdfDict {
    ref, ok := a.dict["/FS"].(pdfRef)
    if !ok {
        return nil
    }
    obj, ok := a.doc.objects[ref.Num]
    if !ok {
        return nil
    }
    fs, ok := obj.Value.(pdfDict)
    if !ok {
        return nil
    }
    return fs
}

// resolveEmbeddedFile follows /FS/EF/F to the /EmbeddedFile stream, or nil.
func (a *FileAttachmentAnnotation) resolveEmbeddedFile() *pdfStream {
    fs := a.resolveFilespec()
    if fs == nil {
        return nil
    }
    ef, ok := fs["/EF"].(pdfDict)
    if !ok {
        return nil
    }
    ref, ok := ef["/F"].(pdfRef)
    if !ok {
        return nil
    }
    obj, ok := a.doc.objects[ref.Num]
    if !ok {
        return nil
    }
    stream, ok := obj.Value.(*pdfStream)
    if !ok {
        return nil
    }
    return stream
}

// detectMIMEType looks up the MIME type from the file extension via
// mime.TypeByExtension, with fallbacks for common types.
func detectMIMEType(name string) string {
    ext := filepath.Ext(name)
    if mt := mime.TypeByExtension(ext); mt != "" {
        // Strip charset suffix (e.g. "text/plain; charset=utf-8" → "text/plain").
        if i := strings.Index(mt, ";"); i >= 0 {
            mt = mt[:i]
        }
        return mt
    }
    // Fallbacks for common extensions not always in mime registry.
    switch ext {
    case ".pdf":
        return "application/pdf"
    case ".txt":
        return "text/plain"
    case ".docx":
        return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
    case ".xlsx":
        return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    case ".zip":
        return "application/zip"
    }
    return "application/octet-stream"
}

// escapePDFName escapes characters not allowed in a PDF name. The "/"
// in MIME types is the most common case (e.g. "application/pdf" →
// "application#2Fpdf").
func escapePDFName(s string) string {
    var b strings.Builder
    for i := 0; i < len(s); i++ {
        c := s[i]
        if c == '/' || c == '#' || c < 0x21 || c > 0x7E {
            b.WriteString(fmt.Sprintf("#%02X", c))
        } else {
            b.WriteByte(c)
        }
    }
    return b.String()
}

// unescapePDFName reverses escapePDFName.
func unescapePDFName(s string) string {
    var b strings.Builder
    for i := 0; i < len(s); i++ {
        if s[i] == '#' && i+2 < len(s) {
            var v int
            fmt.Sscanf(s[i+1:i+3], "%X", &v)
            b.WriteByte(byte(v))
            i += 2
            continue
        }
        b.WriteByte(s[i])
    }
    return b.String()
}
```

The `encodeFormString` and `decodeFormString` helpers exist in `form.go` — reuse them. The `strings` import is also needed; add it to the import block.

- [ ] **Step 4: Run tests + commit**

```bash
go test -run TestFileAttachment -v ./...
go test ./...
git add annotation_fileattachment.go annotation_fileattachment_test.go
git commit -m "feat: FileAttachment file embedding (SetFile + SetFileFromStream + metadata)"
```

---

## Task 4: NewJavaScriptAction public constructor

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_action_test.go` (or wherever JS action tests live)

- [ ] **Step 1: Write the failing test**

Append to `annotation_action_test.go`:
```go
func TestNewJavaScriptAction(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
    js := pdf.NewJavaScriptAction("app.alert('Hello from PDF');")
    link.SetAction(js)
    if err := page.Annotations().Add(link); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    link2 := doc2.Pages()[0].Annotations().At(0).(*pdf.LinkAnnotation)
    act := link2.Action()
    if act == nil {
        t.Fatal("Action() = nil")
    }
    js2, ok := act.(*pdf.JavaScriptAction)
    if !ok {
        t.Fatalf("type = %T, want *pdf.JavaScriptAction", act)
    }
    if js2.Script() != "app.alert('Hello from PDF');" {
        t.Errorf("Script = %q", js2.Script())
    }
}
```

If the test file doesn't exist or is named differently, find it via `grep -l TestLinkAnnotationGoToURIAction *.go` (the JS tests should live alongside other action tests).

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -run TestNewJavaScriptAction -v ./...
```

Expected: build failure — `NewJavaScriptAction` undefined.

- [ ] **Step 3: Add the constructor**

In `annotation_action.go`, find the existing `JavaScriptAction` type. After its existing methods, add:
```go
// NewJavaScriptAction builds a /JavaScript action carrying the given
// script. The action runs when its parent annotation is activated
// (clicked) by a viewer that supports JavaScript.
//
// SECURITY WARNING: Embedding JavaScript in a PDF can introduce
// security risks for recipients. Scripts execute in the viewer's
// JavaScript engine context with access to form fields, navigation,
// and viewer-specific APIs (varies by viewer). Use only with scripts
// you authored or audited; never embed user-supplied JS without
// careful review.
//
// JavaScript actions are commonly disabled by default in viewers
// (Acrobat: Preferences > Security > "Enable JavaScript") so behavior
// is not guaranteed across all rendering environments.
func NewJavaScriptAction(script string) *JavaScriptAction {
    return &JavaScriptAction{script: script}
}
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -run TestNewJavaScriptAction -v ./...
go test ./...
git add annotation_action.go annotation_action_test.go
git commit -m "feat: NewJavaScriptAction public constructor (with security warning)"
```

---

## Task 5: RedactAnnotation skeleton + QuadPoints + parse dispatch

**Files:**
- Modify: `annotation.go`
- Create: `annotation_redact.go`
- Create: `annotation_redact_test.go`
- Create: `appearance_redact.go` (placeholder stub for /AP)

- [ ] **Step 1: Write the failing tests**

Create `annotation_redact_test.go`:
```go
package asposepdf_test

import (
    "bytes"
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestRedactAnnotationBasicRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
    quads := []pdf.QuadPoint{
        {X1: 50, Y1: 650, X2: 300, Y2: 650, X3: 50, Y3: 600, X4: 300, Y4: 600},
    }
    ra.SetQuadPoints(quads)
    if err := page.Annotations().Add(ra); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    got := doc2.Pages()[0].Annotations().At(0)
    if got.AnnotationType() != pdf.AnnotationTypeRedact {
        t.Errorf("type = %v, want AnnotationTypeRedact", got.AnnotationType())
    }
    ra2, ok := got.(*pdf.RedactAnnotation)
    if !ok {
        t.Fatalf("concrete type = %T", got)
    }
    qp := ra2.QuadPoints()
    if len(qp) != 1 {
        t.Fatalf("QuadPoints len = %d, want 1", len(qp))
    }
    if qp[0].X1 != 50 || qp[0].Y4 != 600 {
        t.Errorf("QuadPoint = %+v", qp[0])
    }
}

func TestRedactAnnotationConstructorPanicOnNilPage(t *testing.T) {
    defer func() {
        if r := recover(); r == nil {
            t.Error("expected panic")
        }
    }()
    pdf.NewRedactAnnotation(nil, pdf.Rectangle{})
}

func TestRedactAnnotationDefaultQuadPointsEmpty(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
    qp := ra.QuadPoints()
    if len(qp) != 0 {
        t.Errorf("default QuadPoints = %v, want empty", qp)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestRedactAnnotation -v ./...
```

Expected: build failure.

- [ ] **Step 3: Add /Redact dispatch to parseAnnotation**

In `annotation.go`, add a case before the GenericAnnotation default:
```go
    case "/Redact":
        return parseRedactAnnotation(base)
```

- [ ] **Step 4: Create RedactAnnotation type**

Create `annotation_redact.go`:
```go
package asposepdf

// RedactAnnotation marks regions for redaction. Mark mode (this type)
// renders a semi-transparent fill of /QuadPoints regions. The
// destructive content removal happens when (*Document).ApplyRedactions
// is called — this annotation is then removed and the underlying page
// content is irreversibly rewritten. Per ISO 32000-1 §12.5.6.20.
type RedactAnnotation struct {
    drawingAnnotationBase
}

func (a *RedactAnnotation) AnnotationType() AnnotationType { return AnnotationTypeRedact }

// NewRedactAnnotation builds an unbound redact annotation. Page must
// be non-nil. By default, /QuadPoints is empty (rendering uses /Rect
// as a single quad). Callers typically call SetQuadPoints to specify
// multiple disjoint regions.
func NewRedactAnnotation(page *Page, rect Rectangle) *RedactAnnotation {
    if page == nil {
        panic("NewRedactAnnotation: nil page")
    }
    dict := pdfDict{
        "/Type":    pdfName("/Annot"),
        "/Subtype": pdfName("/Redact"),
        "/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
    }
    a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{
        annotationBase: annotationBase{
            dict: dict,
            doc:  page.doc,
            page: page,
        },
    }}
    a.regenerate = a.regenerateAP
    a.regenerateAP()
    return a
}

// QuadPoints returns the regions to redact in page space. Returns nil
// if /QuadPoints is absent (Apply uses /Rect as the single region).
func (a *RedactAnnotation) QuadPoints() []QuadPoint {
    return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil/empty slice removes the entry
// (Apply will then use /Rect as single region).
func (a *RedactAnnotation) SetQuadPoints(qp []QuadPoint) {
    if len(qp) == 0 {
        delete(a.dict, "/QuadPoints")
    } else {
        a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
    }
    a.regenerateAP()
}

// regenerateAP rebuilds /AP/N for mark-mode visual.
func (a *RedactAnnotation) regenerateAP() {
    setAppearanceN(&a.annotationBase, generateRedactAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt.
func (a *RedactAnnotation) RegenerateAppearance() {
    a.regenerateAP()
}

// parseRedactAnnotation builds a RedactAnnotation from a parsed dict.
func parseRedactAnnotation(base annotationBase) *RedactAnnotation {
    a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
    a.regenerate = a.regenerateAP
    return a
}
```

- [ ] **Step 5: Create generateRedactAppearance stub**

Create `appearance_redact.go`:
```go
package asposepdf

// generateRedactAppearance produces /AP/N for mark-mode display.
// Stub for now — full visual (quad fills + overlay text preview) in
// Task 7.
func generateRedactAppearance(a *RedactAnnotation) *pdfStream {
    rect := a.Rect()
    width := rect.URX - rect.LLX
    height := rect.URY - rect.LLY
    b := newAppearanceBuilder()
    return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, pdfDict{})
}
```

- [ ] **Step 6: Run tests + commit**

```bash
go test -run TestRedactAnnotation -v ./...
go test ./...
git add annotation.go annotation_redact.go annotation_redact_test.go appearance_redact.go
git commit -m "feat: RedactAnnotation skeleton + QuadPoints + parse dispatch"
```

---

## Task 6: RedactAnnotation accessors (InteriorColor + OverlayText + Repeat + OverlayTextStyle)

**Files:**
- Modify: `annotation_redact.go`
- Modify: `annotation_redact_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_redact_test.go`:
```go
func TestRedactAnnotationInteriorColorRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
    ra.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
    page.Annotations().Add(ra)
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
    ic := ra2.InteriorColor()
    if ic == nil || ic.R != 1 {
        t.Errorf("InteriorColor = %v, want red", ic)
    }
}

func TestRedactAnnotationOverlayTextRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
    ra.SetOverlayText("REDACTED")
    ra.SetRepeatOverlayText(true)
    page.Annotations().Add(ra)
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
    if got := ra2.OverlayText(); got != "REDACTED" {
        t.Errorf("OverlayText = %q", got)
    }
    if !ra2.RepeatOverlayText() {
        t.Error("RepeatOverlayText = false, want true")
    }
}

func TestRedactAnnotationOverlayTextStyleRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 650})
    ra.SetOverlayText("X")
    ra.SetOverlayTextStyle(pdf.TextStyle{
        Font:   pdf.FontHelveticaBold,
        Size:   14,
        Color:  &pdf.Color{R: 1, G: 1, B: 1, A: 1},
        HAlign: pdf.HAlignCenter,
    })
    page.Annotations().Add(ra)
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    ra2 := doc2.Pages()[0].Annotations().At(0).(*pdf.RedactAnnotation)
    style := ra2.OverlayTextStyle()
    if style.Size != 14 {
        t.Errorf("Size = %v, want 14", style.Size)
    }
    if style.HAlign != pdf.HAlignCenter {
        t.Errorf("HAlign = %v, want Center", style.HAlign)
    }
    if style.Color == nil || style.Color.R != 1 {
        t.Errorf("Color = %v", style.Color)
    }
}

func TestRedactAnnotationDefaultInteriorColorIsNil(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{})
    if ic := ra.InteriorColor(); ic != nil {
        t.Errorf("default InteriorColor = %v, want nil", ic)
    }
}

func TestRedactAnnotationNoXObjectLeak(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
    page.Annotations().Add(ra)
    ra.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
    ra.SetOverlayText("A")
    ra.SetOverlayText("B")
    ra.SetOverlayText("C")
    ra.SetRepeatOverlayText(true)
    if removed := doc.RemoveUnusedObjects(); removed != 0 {
        t.Errorf("RemoveUnusedObjects = %d after multiple setters; want 0", removed)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestRedactAnnotationInterior|TestRedactAnnotationOverlay|TestRedactAnnotationDefault|TestRedactAnnotationNoXObjectLeak' -v ./...
```

Expected: build failure.

- [ ] **Step 3: Implement accessors**

Append to `annotation_redact.go`:
```go
// InteriorColor returns the /IC fill color (used for both mark visual
// and post-apply overlay). Returns nil if absent.
func (a *RedactAnnotation) InteriorColor() *Color {
    arr, ok := a.dict["/IC"].(pdfArray)
    if !ok || len(arr) != 3 {
        return nil
    }
    r, _ := toFloat(arr[0])
    g, _ := toFloat(arr[1])
    bl, _ := toFloat(arr[2])
    return &Color{R: r, G: g, B: bl, A: 1}
}

// SetInteriorColor writes /IC. nil deletes the entry.
func (a *RedactAnnotation) SetInteriorColor(c *Color) {
    if c == nil {
        delete(a.dict, "/IC")
    } else {
        a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
    }
    a.regenerateAP()
}

// OverlayText returns /OverlayText. Empty string if absent.
func (a *RedactAnnotation) OverlayText() string {
    return decodeFormString(a.dict["/OverlayText"])
}

// SetOverlayText writes /OverlayText. Empty string deletes the entry.
func (a *RedactAnnotation) SetOverlayText(s string) {
    if s == "" {
        delete(a.dict, "/OverlayText")
    } else {
        a.dict["/OverlayText"] = encodeFormString(s)
    }
    a.regenerateAP()
}

// RepeatOverlayText returns /Repeat. False if absent.
func (a *RedactAnnotation) RepeatOverlayText() bool {
    v, _ := a.dict["/Repeat"].(bool)
    return v
}

// SetRepeatOverlayText writes /Repeat. False removes the entry.
func (a *RedactAnnotation) SetRepeatOverlayText(repeat bool) {
    if repeat {
        a.dict["/Repeat"] = true
    } else {
        delete(a.dict, "/Repeat")
    }
    a.regenerateAP()
}

// OverlayTextStyle returns the style reconstructed from /DA + /Q.
// Background is not relevant for redact overlay.
func (a *RedactAnnotation) OverlayTextStyle() TextStyle {
    var style TextStyle
    daRaw, _ := a.dict["/DA"].(string)
    style.Font, style.Size, style.Color = parseDefaultAppearance(daRaw)
    if q, ok := a.dict["/Q"]; ok {
        switch toInt(q) {
        case 1:
            style.HAlign = HAlignCenter
        case 2:
            style.HAlign = HAlignRight
        default:
            style.HAlign = HAlignLeft
        }
    }
    return style
}

// SetOverlayTextStyle writes /DA + /Q.
func (a *RedactAnnotation) SetOverlayTextStyle(s TextStyle) {
    a.dict["/DA"] = formatDefaultAppearance(s)
    switch s.HAlign {
    case HAlignCenter:
        a.dict["/Q"] = 1
    case HAlignRight:
        a.dict["/Q"] = 2
    default:
        delete(a.dict, "/Q")
    }
    a.regenerateAP()
}
```

The `parseDefaultAppearance` and `formatDefaultAppearance` helpers exist in `annotation_freetext.go` from Subepic 2 — reuse.

- [ ] **Step 4: Run tests + commit**

```bash
go test -run 'TestRedactAnnotation' -v ./...
go test ./...
git add annotation_redact.go annotation_redact_test.go
git commit -m "feat: RedactAnnotation accessors (InteriorColor + OverlayText + Repeat + OverlayTextStyle)"
```

---

## Task 7: generateRedactAppearance — full mark-mode visual

**Files:**
- Modify: `appearance_redact.go`
- Create: `appearance_redact_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Create `appearance_redact_internal_test.go`:
```go
package asposepdf

import (
    "strings"
    "testing"
)

func TestGenerateRedactAppearanceSingleQuadFill(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := NewRedactAnnotation(page, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
    ra.SetInteriorColor(&Color{R: 1, G: 0, B: 0, A: 1})
    ra.SetQuadPoints([]QuadPoint{
        {X1: 100, Y1: 650, X2: 300, Y2: 650, X3: 100, Y3: 600, X4: 300, Y4: 600},
    })
    // Inspect /AP/N stream Data.
    apDict, _ := ra.dict["/AP"].(pdfDict)
    if apDict == nil {
        t.Fatal("/AP missing")
    }
    ref, _ := apDict["/N"].(pdfRef)
    obj := doc.objects[ref.Num]
    stream := obj.Value.(*pdfStream)
    out := string(stream.Data)
    // Expect at least one fill operator (re + f) and stroke color set.
    if !strings.Contains(out, " re\n") {
        t.Errorf("expected re op for quad fill, got %q", out)
    }
    if !strings.Contains(out, " rg\n") {
        t.Errorf("expected rg op for fill color, got %q", out)
    }
    if !strings.Contains(out, "f\n") {
        t.Errorf("expected f op for fill, got %q", out)
    }
}

func TestGenerateRedactAppearanceMultipleQuads(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 500, URY: 500})
    ra.SetQuadPoints([]QuadPoint{
        {X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 0, X4: 100, Y4: 0},
        {X1: 200, Y1: 100, X2: 300, Y2: 100, X3: 200, Y3: 0, X4: 300, Y4: 0},
        {X1: 400, Y1: 100, X2: 500, Y2: 100, X3: 400, Y3: 0, X4: 500, Y4: 0},
    })
    apDict, _ := ra.dict["/AP"].(pdfDict)
    ref, _ := apDict["/N"].(pdfRef)
    stream := doc.objects[ref.Num].Value.(*pdfStream)
    out := string(stream.Data)
    // Three quads → three re ops at minimum.
    if cnt := strings.Count(out, " re\n"); cnt < 3 {
        t.Errorf("expected 3+ re ops for 3 quads, got %d in %q", cnt, out)
    }
}

func TestGenerateRedactAppearanceWithOverlayText(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 50})
    ra.SetQuadPoints([]QuadPoint{
        {X1: 0, Y1: 50, X2: 200, Y2: 50, X3: 0, Y3: 0, X4: 200, Y4: 0},
    })
    ra.SetOverlayText("HIDDEN")
    apDict, _ := ra.dict["/AP"].(pdfDict)
    ref, _ := apDict["/N"].(pdfRef)
    stream := doc.objects[ref.Num].Value.(*pdfStream)
    out := string(stream.Data)
    // Overlay text → BT/ET/Tj operators.
    if !strings.Contains(out, "BT\n") {
        t.Errorf("expected BT for overlay text, got %q", out)
    }
    if !strings.Contains(out, "ET\n") {
        t.Errorf("expected ET for overlay text, got %q", out)
    }
}

func TestGenerateRedactAppearanceDefaultBlackFill(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
    ra.SetQuadPoints([]QuadPoint{
        {X1: 0, Y1: 50, X2: 100, Y2: 50, X3: 0, Y3: 0, X4: 100, Y4: 0},
    })
    // No SetInteriorColor — should default to black (rgb 0 0 0).
    apDict, _ := ra.dict["/AP"].(pdfDict)
    ref, _ := apDict["/N"].(pdfRef)
    stream := doc.objects[ref.Num].Value.(*pdfStream)
    out := string(stream.Data)
    if !strings.Contains(out, "0 0 0 rg") {
        t.Errorf("expected default black fill (0 0 0 rg), got %q", out)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestGenerateRedactAppearance -v ./...
```

Expected: tests run but fail (stub doesn't emit operators).

- [ ] **Step 3: Replace generateRedactAppearance with full implementation**

In `appearance_redact.go`, REPLACE the stub with:
```go
package asposepdf

// generateRedactAppearance produces /AP/N for mark-mode display:
// each /QuadPoints region is filled with /IC (default black at 50% transparency-equivalent
// — actually opaque black since builder doesn't support transparency; visual contrast
// signals the redact mark) and optional /OverlayText is rendered centered in each quad.
func generateRedactAppearance(a *RedactAnnotation) *pdfStream {
    rect := a.Rect()
    width := rect.URX - rect.LLX
    height := rect.URY - rect.LLY

    b := newAppearanceBuilder()
    resources := existingAPNResources(&a.annotationBase)
    if resources == nil {
        resources = pdfDict{}
    }

    // Determine fill color — default black if /IC absent.
    fill := Color{R: 0, G: 0, B: 0, A: 1}
    if ic := a.InteriorColor(); ic != nil {
        fill = *ic
    }

    quads := a.QuadPoints()
    if len(quads) == 0 {
        // Default: full /Rect as single quad.
        quads = []QuadPoint{rectAsQuadPoint(rect)}
    }

    // 1. Fill each quad in /IC.
    b.PushState()
    b.SetFillColorRGB(fill)
    for _, qp := range quads {
        // Translate quad to local /BBox space (subtract rect.LLX, rect.LLY).
        // Use an axis-aligned bounding rect derived from the quad's corner points.
        local := localizeQuadAsRect(qp, rect)
        b.Rect(local.LLX, local.LLY, local.URX-local.LLX, local.URY-local.LLY)
    }
    b.Fill()
    b.PopState()

    // 2. Optional /OverlayText preview (centered in each quad).
    if overlay := a.OverlayText(); overlay != "" {
        style := a.OverlayTextStyle()
        // Default text color: white if /IC is dark, black otherwise — heuristic.
        if style.Color == nil {
            white := Color{R: 1, G: 1, B: 1, A: 1}
            style.Color = &white
        }
        // Default font/size if not set.
        if style.Font == nil {
            style.Font = FontHelvetica
        }
        if style.Size == 0 {
            style.Size = 10
        }
        for _, qp := range quads {
            quadRect := localizeQuadAsRect(qp, rect)
            resolve := func(font Font, _ pdfDict) (resName string, w widthFn, e encodeFn, asc, desc float64, err error) {
                return resolveFontForXObject(font, style.Size, a.doc, resources)
            }
            _ = renderTextInBuilder(b, resources, overlay, style, quadRect, resolve, "", "")
        }
    }

    return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// rectAsQuadPoint converts a Rectangle to a QuadPoint covering the
// same area (corners as defined by ISO 32000-1 §12.5.6.10: UL/UR/LL/LR).
func rectAsQuadPoint(r Rectangle) QuadPoint {
    return QuadPoint{
        X1: r.LLX, Y1: r.URY, // UL
        X2: r.URX, Y2: r.URY, // UR
        X3: r.LLX, Y3: r.LLY, // LL
        X4: r.URX, Y4: r.LLY, // LR
    }
}

// localizeQuadAsRect converts a page-space QuadPoint to a local /BBox-space
// axis-aligned Rectangle. Acceptable for redact rendering since redact
// regions are typically rectangular; full quad geometry preserved in the
// /QuadPoints array but visual fill uses the bounding box.
func localizeQuadAsRect(qp QuadPoint, rect Rectangle) Rectangle {
    minX := min(min(qp.X1, qp.X2), min(qp.X3, qp.X4))
    maxX := max(max(qp.X1, qp.X2), max(qp.X3, qp.X4))
    minY := min(min(qp.Y1, qp.Y2), min(qp.Y3, qp.Y4))
    maxY := max(max(qp.Y1, qp.Y2), max(qp.Y3, qp.Y4))
    return Rectangle{
        LLX: minX - rect.LLX,
        LLY: minY - rect.LLY,
        URX: maxX - rect.LLX,
        URY: maxY - rect.LLY,
    }
}
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -run 'TestGenerateRedactAppearance|TestRedactAnnotation' -v ./...
go test ./...
git add appearance_redact.go appearance_redact_internal_test.go
git commit -m "feat: generateRedactAppearance — full mark-mode visual (quad fills + overlay text)"
```

---

## Tasks 8-13: Redact apply machinery

The remaining six tasks build the apply machinery. Due to plan size, these are summarized with the same TDD shape (failing test → impl → run → commit). Each task references the existing `parseContentStream` infrastructure from text-extraction (`content_parser.go`).

### Task 8: redact_apply.go skeleton + Document.ValidateRedactions

- Create `redact_apply.go` with `(*Document).ValidateRedactions() error` — walks pages with redact annotations, calls `parseContentStream` on each page's content stream; returns first parse error or nil.
- Create `redact_apply_test.go` with `TestValidateRedactionsEmpty` (no redacts → nil) and `TestValidateRedactionsWithRedact` (page has redact, content stream parseable → nil).
- Stub `(*Document).ApplyRedactions() error` returns `nil` for now (full impl in Task 12).

### Task 9: rewriteTextOperatorsInStream — text glyph removal

- Create `redact_apply_text.go` with `rewriteTextOperatorsInStream(data []byte, regions []QuadPoint, fontMap map[string]fontInfo) ([]byte, error)`.
- Walks BT/ET blocks tracking text matrix. On `Tj`/`TJ`/`'`/`"`, computes glyph rectangles via existing `widthFn` callbacks, removes glyphs whose center is inside any redact region.
- TJ kerning numbers preceding removed glyphs are also dropped.
- Re-serializes the modified token stream as bytes.
- Tests: single glyph removal (input `(Hello world) Tj` + region over "world" → output `(Hello ) Tj`); TJ kerning drop; multi-line preservation.

### Task 10: rewriteImageOperatorsInStream — image clipping

- Create `redact_apply_image.go` with `rewriteImageOperatorsInStream(data []byte, regions []QuadPoint) ([]byte, error)`.
- Walks content stream tracking CTM (`q`, `Q`, `cm`).
- On `Do`: computes image bbox in page-space (CTM × unit square). Intersects with regions:
  - Fully outside → keep `Do` unchanged.
  - Fully inside → drop `Do`.
  - Partial → wrap in `q + clip path + Do + Q` with clip = page rect minus redact quads (even-odd fill).
- Tests: full inside, full outside, partial overlap, multiple Do operators on one page, CTM affecting bbox.

### Task 11: rewritePathOperatorsInStream — path clipping

- Create `redact_apply_path.go` with `rewritePathOperatorsInStream(data []byte, regions []QuadPoint) ([]byte, error)`.
- Walks path-construction operators (`m`, `l`, `c`, `v`, `y`, `re`, `h`) accumulating bounding box.
- On paint operator (`S`, `s`, `f`, `F`, `f*`, `B`, `B*`, `b`, `b*`, `n`):
  - Compute path's bbox via accumulated points + CTM.
  - Same clip-or-remove logic as image rewriter.
  - Reset accumulated path after paint.
- Tests: rect inside / outside / partial, multi-segment path, mixed text + path on one page.

### Task 12: Apply orchestration — full ApplyRedactions implementation

- Replace stub in `redact_apply.go` with full implementation:
  1. For each page with redact annotations: collect all /QuadPoints regions.
  2. Get current content stream bytes (combine multiple if /Contents is an array).
  3. Get font map from page /Resources.
  4. Run text rewriter, then image rewriter, then path rewriter (sequential).
  5. Append overlay text content if any redact has /OverlayText set.
  6. Replace page's content stream with rewritten bytes.
  7. Remove redact annotations from /Annots and from doc.objects.
- Tests in `redact_apply_test.go`:
  - `TestApplyRedactionsRemovesText` — page with "Hello world" text + redact region over "world" → ApplyRedactions → ExtractText returns "Hello".
  - `TestApplyRedactionsPreservesNonRedactedText` — text outside redact region survives.
  - `TestApplyRedactionsAcrossMultiplePages` — redactions on pages 1, 3, 5 → all applied.
  - `TestApplyRedactionsRemovesAnnotation` — redact annotation gone from /Annots after apply.
  - `TestApplyRedactionsCoexistsWithOtherAnnotations` — Link + Highlight + Redact on one page → after apply, Redact gone, others preserved.

### Task 13: Overlay text rendering after apply

- Extend `redact_apply.go` orchestration: after content rewrite, build overlay-text fragment via `appearanceBuilder` for each redact annotation with /OverlayText.
- Fill quad rectangles with /IC color (visual block in final document).
- Render /OverlayText centered (or tiled if /Repeat true) per /DA + /Q style.
- Append fragment to page content stream.
- Tests in `redact_apply_test.go`:
  - `TestApplyRedactionsOverlayText` — page text "Confidential" replaced by "REDACTED" (color black on white).
  - `TestApplyRedactionsRepeatOverlay` — /Repeat=true → "X" tiled across redacted region.

---

## Task 14: Cross-cutting integration tests

**Files:**
- Create: `redact_apply_integration_test.go` (or extend `redact_apply_test.go`)

- [ ] **Step 1: Append integration tests**

```go
func TestApplyRedactionsValidate(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    page.AddText("Confidential", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
        pdf.Rectangle{LLX: 100, LLY: 700, URX: 300, URY: 720})
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 700, URX: 300, URY: 720})
    page.Annotations().Add(ra)
    if err := doc.ValidateRedactions(); err != nil {
        t.Errorf("ValidateRedactions returned error: %v", err)
    }
}

func TestApplyRedactionsEmpty(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    if err := doc.ApplyRedactions(); err != nil {
        t.Errorf("ApplyRedactions on empty doc returned error: %v", err)
    }
}

func TestApplyRedactionsTextExtractionPostApply(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    page.AddText("Public info Confidential data more public info",
        pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730})
    // Add redact over approximate "Confidential data" position.
    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 130, LLY: 700, URX: 250, URY: 730})
    page.Annotations().Add(ra)
    if err := doc.ApplyRedactions(); err != nil {
        t.Fatalf("ApplyRedactions: %v", err)
    }
    // Save + extract text.
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    text, err := doc2.ExtractText()
    if err != nil {
        t.Fatal(err)
    }
    pageText := strings.Join(text, "\n")
    // "Confidential" should NOT appear in extracted text.
    if strings.Contains(pageText, "Confidential") {
        t.Errorf("ExtractText returned redacted content: %q", pageText)
    }
    // "Public info" should still appear.
    if !strings.Contains(pageText, "Public") {
        t.Errorf("ExtractText missing non-redacted content: %q", pageText)
    }
}

func TestSubepic4FilterByType(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
    page.Annotations().Add(fa)

    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
    page.Annotations().Add(ra)

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    page2, _ := doc2.Page(1)

    counts := map[pdf.AnnotationType]int{}
    for _, a := range page2.Annotations().All() {
        counts[a.AnnotationType()]++
    }
    if counts[pdf.AnnotationTypeFileAttachment] != 1 {
        t.Errorf("FileAttachment count = %d, want 1", counts[pdf.AnnotationTypeFileAttachment])
    }
    if counts[pdf.AnnotationTypeRedact] != 1 {
        t.Errorf("Redact count = %d, want 1", counts[pdf.AnnotationTypeRedact])
    }
}

func TestSubepic4RegenerateAppearance(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
    page.Annotations().Add(fa)
    fa.RegenerateAppearance() // no-op for FileAttachment

    ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
    page.Annotations().Add(ra)
    ra.RegenerateAppearance() // mark-mode visual rebuild
}
```

- [ ] **Step 2: Run tests + commit**

```bash
go test ./...
go vet ./...
git add redact_apply_integration_test.go redact_apply_test.go
git commit -m "test: Subepic 4 cross-cutting integration tests"
```

---

## Task 15: pypdf cross-check + CLAUDE.md + README + close bd-37n

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: pypdf cross-check (manual)**

Create `/d/tmp/check_subepic4/main.go`:
```go
package main

import (
    "log"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    // FileAttachment with embedded text file
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 800})
    fa.SetIcon(pdf.FileAttachmentIconPushPin)
    fa.SetTitle("Reviewer")
    fa.SetContents("Click to download")
    fa.SetFileFromStream(strings.NewReader("attached file content"), "data.txt")
    fa.SetFileDescription("Test attachment")
    page.Annotations().Add(fa)

    // Redact mark (with overlay text)
    page.AddText("Confidential information here",
        pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730})
    ra := pdf.NewRedactAnnotation(page,
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730})
    ra.SetOverlayText("REDACTED")
    ra.SetInteriorColor(&pdf.Color{R: 0, G: 0, B: 0, A: 1})
    page.Annotations().Add(ra)

    // Link with JavaScript action
    link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
    link.SetAction(pdf.NewJavaScriptAction("app.alert('Hello from PDF');"))
    page.Annotations().Add(link)

    if err := doc.Save("D:/tmp/subepic4_built.pdf"); err != nil {
        log.Fatal(err)
    }
}
```

`/d/tmp/check_subepic4/go.mod` — same pattern as prior subepics.

Run + python check:
```bash
mkdir -p /d/tmp/check_subepic4
# write main.go and go.mod
cd /d/tmp/check_subepic4 && go run main.go
python -c "
from pypdf import PdfReader
r = PdfReader('D:/tmp/subepic4_built.pdf')
av = r.pages[0].get('/Annots')
ann = av.get_object() if hasattr(av, 'get_object') else av
print('count:', len(ann))
for i, a in enumerate(ann):
    ao = a.get_object() if hasattr(a, 'get_object') else a
    sub = ao.get('/Subtype', '?')
    extras = []
    if '/FS' in ao:
        extras.append('FS')
    if '/OverlayText' in ao:
        extras.append('OverlayText')
    if '/A' in ao:
        a_obj = ao['/A']
        a_dict = a_obj.get_object() if hasattr(a_obj, 'get_object') else a_obj
        a_s = a_dict.get('/S', '')
        extras.append(f'A.S={a_s}')
    extras_str = ' ' + ' '.join(extras) if extras else ''
    print(f'  [{i}] /Subtype={sub}{extras_str}')
"
```

Expected output:
```
count: 3
  [0] /Subtype=/FileAttachment FS
  [1] /Subtype=/Redact OverlayText
  [2] /Subtype=/Link A.S=/JavaScript
```

If output mismatch → STOP, report BLOCKED.

Cleanup: `rm -rf /d/tmp/check_subepic4 /d/tmp/subepic4_built.pdf`.

- [ ] **Step 2: Test ApplyRedactions end-to-end with pypdf text extraction**

Add a separate test program that calls ApplyRedactions and verifies pypdf's text extraction does NOT return redacted content. Output should show "REDACTED" overlay text and NOT "Confidential information here".

- [ ] **Step 3: Update CLAUDE.md**

Append a NEW block after the existing text-bearing-annotations block:

```markdown
**`annotation_fileattachment.go` / `annotation_redact.go` / `appearance_redact.go` / `redact_apply*.go`**
- `FileAttachmentAnnotation` — embedded file annotation. `Icon()/SetIcon(i)`, `SetFile(path)/SetFileFromStream(r, name)`, `HasFile()`, read-only metadata `FileName/FileMIMEType/FileSize/FileBytes/FileDescription/SetFileDescription`. Constructor `NewFileAttachmentAnnotation(page, position Point)` — auto-bbox 24×24pt. No /AP — viewers render the icon
- `FileAttachmentIcon` enum — `FileAttachmentIconPaperclip` (default), `FileAttachmentIconGraph`, `FileAttachmentIconPushPin`, `FileAttachmentIconTag`, `FileAttachmentIconUnknown`
- `RedactAnnotation` — mark + apply redaction. `QuadPoints()/SetQuadPoints`, `InteriorColor/SetInteriorColor`, `OverlayText/SetOverlayText`, `RepeatOverlayText/SetRepeatOverlayText`, `OverlayTextStyle/SetOverlayTextStyle`. Border via `drawingAnnotationBase`. Mark-mode /AP/N renders quad fills + optional overlay preview; destructive content removal via `(*Document).ApplyRedactions()`. Constructor `NewRedactAnnotation(page, rect)`
- `(*Document).ApplyRedactions() error` — destructively removes content (text glyphs, image XObjects, paths) inside every /Redact annotation's /QuadPoints regions; renders /OverlayText if set; removes the redact annotations after rewrite. Best-effort semantics — partial state on failure
- `(*Document).ValidateRedactions() error` — pre-flight dry-run parseability check; recommended before ApplyRedactions
- `NewJavaScriptAction(script string) *JavaScriptAction` — public constructor for JS actions (parse-only since Subepic 1). Includes documented security warning
```

Update the `AnnotationType` enum line to include `AnnotationTypeFileAttachment` and `AnnotationTypeRedact`.

- [ ] **Step 4: Update README.md**

Update the `### Annotations` section's "Supported subtypes" line:
```
Supported subtypes: Link, Highlight, Underline, StrikeOut, Squiggly, Square, Circle,
Line, Ink, Text, FreeText, Stamp, FileAttachment, Redact.
```

Add a new `### Specialised annotations (FileAttachment / Redact / JavaScript actions)` section before `### Validation`:

````markdown
### Specialised annotations (FileAttachment / Redact / JavaScript actions)

```go
doc := pdf.NewDocument(595, 842)
page, _ := doc.Page(1)

// FileAttachment — embed a file with an icon
fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
fa.SetIcon(pdf.FileAttachmentIconPushPin)
fa.SetFile("report.pdf")
fa.SetFileDescription("Q3 financial report")
fa.SetTitle("Reviewer")
fa.SetContents("Attached: Q3 report")
page.Annotations().Add(fa)

// Redact — mark for redaction (apply destructively below)
page.AddText("Confidential data here",
    pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
    pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 630})
ra := pdf.NewRedactAnnotation(page,
    pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 630})
ra.SetInteriorColor(&pdf.Color{R: 0, G: 0, B: 0, A: 1})
ra.SetOverlayText("REDACTED")
page.Annotations().Add(ra)

// Apply destructively — text under /QuadPoints is irreversibly removed
if err := doc.ApplyRedactions(); err != nil {
    log.Fatal(err)
}

// JavaScript action on a Link
link := pdf.NewLinkAnnotation(page,
    pdf.Rectangle{LLX: 50, LLY: 500, URX: 200, URY: 520})
link.SetAction(pdf.NewJavaScriptAction("app.alert('Hello from PDF');"))
page.Annotations().Add(link)

doc.Save("specialised.pdf")
```

`FileAttachmentAnnotation` icons: `Paperclip` (default), `Graph`, `PushPin`, `Tag`.
MIME type auto-detected from file extension. After save, viewers display the icon
and let users save/open the embedded file. `RedactAnnotation` operates in two modes:
mark mode (annotation alone, decorative — content still extractable) and apply mode
(after `Document.ApplyRedactions()`, page content is rewritten to remove text
glyphs, image XObjects, and paths inside /QuadPoints regions; the redact annotation
is then deleted). Use `ValidateRedactions()` first as a pre-flight parseability
check. **NewJavaScriptAction** carries a documented security warning — embedded
JavaScript executes in the recipient's viewer.
````

Update the `## Features` Annotations bullet to add:
> ... FileAttachment with file embedding (path/stream + MIME detection); Redact with mark/apply (irreversible content removal of text/images/paths via `Document.ApplyRedactions()`); JavaScript action constructor `NewJavaScriptAction`.

- [ ] **Step 5: Run full suite**

```bash
go test ./...
go vet ./...
```

Expected: PASS.

- [ ] **Step 6: Commit docs**

```bash
git add CLAUDE.md README.md
git commit -m "docs: FileAttachment + Redact + JS construct (Subepic 4) in CLAUDE.md and README"
```

- [ ] **Step 7: Close bd-37n umbrella**

After verifying full test suite green:
```bash
bd update pdf-go-37n --status closed --append-notes "Subepic 4 (FileAttachment + Redact mark+apply + NewJavaScriptAction) shipped 2026-05-07. All 4 subepics complete:
- Subepic 1 (Link + Highlight family + Actions) — 2026-05-05
- Subepic 3 + /AP infra (Square/Circle/Line/Ink) — 2026-05-06
- Subepic 2 (Text/FreeText/Stamp) — 2026-05-07
- Subepic 4 (FileAttachment/Redact/JS construct) — 2026-05-07
Annotations API surface complete per ISO 32000-1 §12.5 (excluding deferred Polygon/PolyLine + multimedia subtypes)."
```

---

## Self-review

**Spec coverage:** every spec section maps to at least one task.

| Spec section | Tasks |
|---|---|
| FileAttachment annotation type + 4 icons | 1, 2 |
| File embedding (path/stream + MIME detection + metadata) | 3 |
| NewJavaScriptAction public constructor | 4 |
| RedactAnnotation type + QuadPoints | 5 |
| Redact accessors (InteriorColor/OverlayText/Repeat/OverlayTextStyle) | 6 |
| Redact mark-mode /AP rendering | 7 |
| Redact apply orchestration | 8, 12 |
| Text rewriter | 9 |
| Image rewriter | 10 |
| Path rewriter | 11 |
| Overlay text after apply | 13 |
| Cross-cutting integration | 14 |
| Documentation + pypdf cross-check + bd-37n close | 15 |

**Placeholder scan:** Tasks 1-7 contain full code blocks. Tasks 8-13 are summarized due to plan-size constraints; they reference the established TDD pattern and existing infrastructure (`parseContentStream`, `appearanceBuilder`, font subsystem) used by similar tasks in prior subepics. Subagent will request specific details if needed.

**Type consistency:**
- `FileAttachmentIcon` enum (5 values) declared in Task 1, used in Task 2.
- `RedactAnnotation` declared in Task 5, accessors added in Task 6, /AP rendering in Task 7, apply machinery in Tasks 8-13.
- `parseDefaultAppearance` and `formatDefaultAppearance` from Subepic 2 reused for /OverlayText style serialization in Task 6.
- `existingAPNResources` from prior subepic reused in Task 7 for no-leak regen.
- `parseContentStream` from text-extraction reused as base for Tasks 9-11.

No type-consistency issues found.

---

## Execution Handoff

After saving this plan, two execution options:

**1. Subagent-Driven** — fresh subagent per task, review between tasks.
**2. Inline Execution** — execute tasks in this session via executing-plans, batch checkpoints.
