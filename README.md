# Aspose.PDF for Go FOSS

A pure Go library for PDF manipulation — split, merge, rotate, extract pages, read metadata, and encrypt documents. No external dependencies.

## Quick Start

```go
import pdf "github.com/aspose/pdf-for-go"

// Open a PDF
doc, err := pdf.Open("input.pdf")

// Split into individual pages
paths, err := doc.Split("output_dir/", func(page, total int) string {
    return fmt.Sprintf("page%03d.pdf", page)
})

// Merge multiple PDFs into one
doc2, _ := pdf.Open("file2.pdf")
doc.AppendFrom(doc2)
doc.Save("merged.pdf")
```

## Features

- **Split** — split a document into individual pages with custom file naming
- **Extract** — build a new PDF from selected page ranges without mutating the source
- **Merge** — combine multiple PDFs into a single document
- **Rotate** — rotate pages by 90°, 180°, or 270°
- **Page info** — read page count and dimensions
- **Metadata** — read document Info (title, author, dates, etc.)
- **Encrypt** — password-protect PDFs with RC4-128 (PDF 1.4 Standard Security Handler)
- **Validate** — check structural integrity of a PDF file
- **Stream input** — open PDFs from any `io.Reader`, not just file paths

## API Reference

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

// Split all pages into separate files
paths, err := doc.Split("output_dir/", func(page, total int) string {
    return fmt.Sprintf("page%03d.pdf", page)
})

// Split a range: keep only pages 2–4, then split
doc.ExtractPages(pdf.PageRange{From: 2, To: 4})
paths, err = doc.Split("output_dir/", func(page, _ int) string {
    return fmt.Sprintf("page%03d.pdf", page)
})
```

### Extracting page ranges

```go
doc, err := pdf.Open("input.pdf")

// Save pages 1–3 and 7–9 to a new PDF (doc is not mutated)
err = doc.Extract("output.pdf",
    pdf.PageRange{From: 1, To: 3},
    pdf.PageRange{From: 7, To: 9},
)
```

### Merging

```go
err := pdf.Merge("merged.pdf", "a.pdf", "b.pdf", "c.pdf")
```

### Rotating

```go
// Rotate all pages 90° clockwise
err := pdf.Rotate("input.pdf", "output.pdf", pdf.Rotate90)

// Rotate specific pages (1-based)
err = pdf.Rotate("input.pdf", "output.pdf", pdf.Rotate180, 1, 3, 5)
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
meta, err := pdf.GetMetadata("input.pdf")
fmt.Println(meta.Title, meta.Author, meta.CreationDate)
```

### Encryption

```go
// Standalone function
err := pdf.Encrypt("input.pdf", "output.pdf", "userpass", "ownerpass")

// Via Document (applied on Save/WriteTo)
doc, _ := pdf.Open("input.pdf")
doc.SetPassword("userpass", "ownerpass")
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

### Mutable Document API

```go
doc, err := pdf.Open("input.pdf")

fmt.Println(doc.PageCount())   // total pages
fmt.Println(doc.Metadata())    // Info dictionary

// Rotate pages in-place
doc.Rotate(pdf.Rotate90, 1, 2)

// Keep only selected page ranges (mutates the document)
doc.ExtractPages(pdf.PageRange{From: 1, To: 5})

// Reorder pages (pages may be repeated or omitted)
doc.Reorder([]int{3, 1, 2})

// Append pages from another document
other, _ := pdf.Open("other.pdf")
doc.AppendFrom(other)

// Split into individual files
paths, err := doc.Split("output_dir/", func(page, total int) string {
    return fmt.Sprintf("page%03d.pdf", page)
})

// Extract page ranges to a new file (does not mutate doc)
err = doc.Extract("output.pdf", pdf.PageRange{From: 1, To: 3})

// Password-protect on save
doc.SetPassword("userpass", "ownerpass")

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
    fmt.Printf("Page %d: %.0fx%.0f pt, rotation %d°\n",
        p.Number(), size.Width, size.Height, p.Rotation())
}
```

## License

MIT License. See [LICENSE](LICENSE) for details.

## Product Page

[Aspose.PDF for Go FOSS](https://products.aspose.com/pdf/) — part of the Aspose family of document processing libraries.
