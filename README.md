# Aspose.PDF for Go FOSS

A pure Go library for PDF manipulation — split, merge, rotate, extract pages, read metadata, and encrypt documents. No external dependencies.

## Quick Start

```go
// Split a PDF into individual pages
err := pdf.Split("input.pdf", "output_dir/")

// Merge multiple PDFs into one
err = pdf.Merge("merged.pdf", "file1.pdf", "file2.pdf", "file3.pdf")
```

## Features

- **Split** — split a PDF into individual pages or page ranges
- **Merge** — combine multiple PDFs into a single document
- **Rotate** — rotate pages by 90°, 180°, or 270°
- **Extract** — build a new PDF from selected page ranges
- **Page info** — read page count and dimensions
- **Metadata** — read document Info (title, author, dates, etc.)
- **Encrypt** — password-protect PDFs with RC4-128 (PDF 1.4 Standard Security Handler)
- **Mutable Document API** — open, modify, and save documents in a fluent style

## API Reference

### Splitting

```go
// Split all pages into separate files in outputDir
err := pdf.Split("input.pdf", "output_dir/")

// Split a page range (1-based; to=0 means last page)
err = pdf.SplitRange("input.pdf", "output_dir/", 2, 5)

// Build a new PDF from selected page ranges
err = pdf.Extract("input.pdf", "output.pdf",
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

### Page Info

```go
// Total page count
n, err := pdf.PageCount("input.pdf")

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
// Encrypt with user and owner passwords
err := pdf.Encrypt("input.pdf", "output.pdf", "userpass", "ownerpass")
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
if err != nil {
    log.Fatal(err)
}

fmt.Println(doc.PageCount())   // total pages
fmt.Println(doc.Metadata())    // Info dictionary

// Rotate pages in-place
doc.Rotate(90, 1, 2)

// Keep only selected page ranges
doc.ExtractPages(pdf.PageRange{From: 1, To: 5})

// Reorder pages (pages may be repeated or omitted)
doc.Reorder([]int{3, 1, 2})

// Append pages from another document
other, _ := pdf.Open("other.pdf")
doc.AppendFrom(other)

// Password-protect on save
doc.SetPassword("userpass", "ownerpass")

// Save to file or writer
err = doc.Save("output.pdf")
// or
err = doc.WriteTo(w) // implements io.WriterTo
```

## Page and PageSize

```go
doc, _ := pdf.Open("input.pdf")
pages := doc.Pages()
for _, p := range pages {
    fmt.Printf("Page %d: %.0fx%.0f pt, rotation %d°\n",
        p.Number(), p.Size().Width, p.Size().Height, p.Rotation())
}
```

## License

MIT License. See [LICENSE](LICENSE) for details.

## Product Page

[Aspose.PDF for Go FOSS](https://products.aspose.com/pdf/) — part of the Aspose family of document processing libraries.
