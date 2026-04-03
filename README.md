# Aspose.PDF for Go FOSS

A pure Go library for PDF manipulation — split, merge, rotate, extract pages, read metadata, and encrypt documents. No external dependencies.

## Quick Start

```go
import pdf "github.com/aspose/pdf-for-go"

// Open a PDF
doc, err := pdf.Open("input.pdf")

// Split into individual page documents
pages, err := doc.Split()
for i, p := range pages {
    p.Save(fmt.Sprintf("page%03d.pdf", i+1))
}

// Merge multiple PDFs into one
doc2, _ := pdf.Open("file2.pdf")
merged := doc.Append(doc2)
merged.Save("merged.pdf")
```

## Features

- **Split** — split a document into individual pages
- **Extract** — build a new Document from selected page ranges without mutating the source
- **Merge** — combine multiple PDFs into a single document
- **Rotate** — rotate pages by 90°, 180°, or 270°
- **Page info** — read page count, dimensions, all PDF boxes (MediaBox, CropBox, TrimBox, BleedBox, ArtBox), and page labels
- **Metadata** — read document Info (title, author, dates, etc.)
- **Encrypt** — password-protect PDFs with RC4-128 (PDF 1.4 Standard Security Handler)
- **Validate** — check structural integrity of a PDF file
- **Stream input** — open PDFs from any `io.Reader`, not just file paths

## API Reference

All `*Document` methods return a new `*Document`; the receiver is never modified.

### Opening documents

```go
// From a file path
doc, err := pdf.Open("input.pdf")

// From an io.Reader (stream, HTTP response, etc.)
doc, err = pdf.OpenStream(r)
```

### Splitting

```go
doc, err := pdf.Open("input.pdf")

// Split into individual page documents
pages, err := doc.Split()
for i, p := range pages {
    p.Save(fmt.Sprintf("page%03d.pdf", i+1))
}
```

### Extracting page ranges

```go
doc, err := pdf.Open("input.pdf")

// Build a new document with pages 1–3 and 7–9 (doc is not mutated)
extracted, err := doc.Extract(
    pdf.PageRange{From: 1, To: 3},
    pdf.PageRange{From: 7, To: 9},
)
extracted.Save("output.pdf")
```

### Page info

```go
doc, _ := pdf.Open("input.pdf")

// Total page count
fmt.Println(doc.PageCount())

// Dimensions of every page (width and height in points, 1/72 inch)
sizes, err := pdf.PageSizes("input.pdf")
for i, s := range sizes {
    fmt.Printf("Page %d: %.1f x %.1f pt\n", i+1, s.Width, s.Height)
}
```

### Metadata

```go
// Read
doc, _ := pdf.Open("input.pdf")
meta, _ := doc.Metadata()
fmt.Println(meta.Title, meta.Author, meta.CreationDate)

// Write (full replacement — unset fields are omitted from the PDF)
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

### Encryption

```go
// Standalone function
err := pdf.Encrypt("input.pdf", "output.pdf", "userpass", "ownerpass")

// Via Document (applied on Save/WriteTo)
doc, _ := pdf.Open("input.pdf")
doc = doc.SetPassword("userpass", "ownerpass")
err = doc.Save("output.pdf")
```

### Validation

```go
report, err := pdf.Validate("input.pdf")
if err != nil {
    log.Fatal(err)
}
if !report.Valid {
    for _, issue := range report.Issues {
        fmt.Println(issue.Code, issue.Message)
    }
}
```

Issue codes: `INVALID_HEADER`, `XREF_ERROR`, `OBJECT_ERROR`, `PAGE_TREE_ERROR`, `STREAM_ERROR`, `ENCRYPTED`.

### Document API

```go
doc, err := pdf.Open("input.pdf")

fmt.Println(doc.PageCount())   // total pages
fmt.Println(doc.Metadata())    // Info dictionary

// Rotate pages (returns new Document; rotation accumulates)
doc, err = doc.Rotate(pdf.Rotate90, 1, 2)
doc, err = doc.Rotate(pdf.Rotate90, 1, 2) // page 1 and 2 are now at 180°

// Set absolute rotation (replaces existing rotation)
doc, err = doc.SetRotation(pdf.Rotate90, 1) // page 1 is now exactly 90°
doc, err = doc.SetRotation(pdf.Rotate0)     // reset all pages to 0°

// Reorder pages (pages may be repeated or omitted)
doc, err = doc.Reorder([]int{3, 1, 2})

// Append pages from one or more documents
doc2, _ := pdf.Open("part2.pdf")
doc3, _ := pdf.Open("part3.pdf")
doc = doc.Append(doc2, doc3)

// Split into individual page documents
pages, err := doc.Split()

// Extract page ranges to a new Document
sub, err := doc.Extract(pdf.PageRange{From: 1, To: 3})

// Password-protect on save
doc = doc.SetPassword("userpass", "ownerpass")

// Save to file or writer
err = doc.Save("output.pdf")
// or
_, err = doc.WriteTo(w) // implements io.WriterTo
```

### Pages

```go
doc, _ := pdf.Open("input.pdf")
pages := doc.Pages()
for _, p := range pages {
    size, _ := p.Size()
    fmt.Printf("Page %d: %.0fx%.0f pt, rotation %d°, label %q\n",
        p.Number(), size.Width, size.Height, p.Rotation(), p.Label())
}

// PDF boxes — each falls back to the next in the chain if not set:
// ArtBox/TrimBox/BleedBox → CropBox → MediaBox
p, _ := doc.Page(1)
mediaBox, _ := p.Size()
cropBox,  _ := p.CropBox()
trimBox,  _ := p.TrimBox()
bleedBox, _ := p.BleedBox()
artBox,   _ := p.ArtBox()
```

## License

MIT License. See [LICENSE](LICENSE) for details.

## Product Page

[Aspose.PDF for Go FOSS](https://products.aspose.com/pdf/) — part of the Aspose family of document processing libraries.
