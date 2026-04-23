# Aspose.PDF for Go FOSS

A pure Go library for PDF manipulation — split, merge, rotate, extract text and images, read metadata, and encrypt documents. No external dependencies.

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

// Merge multiple PDFs into one (Append mutates doc in place)
doc2, _ := pdf.Open("file2.pdf")
doc.Append(doc2)
doc.Save("merged.pdf")
```

## Features

- **Split** — split a document into individual pages
- **Extract** — build a new Document from selected page ranges without mutating the source
- **Merge** — combine multiple PDFs into a single document
- **Rotate** — rotate pages by 90°, 180°, or 270°
- **Page info** — read page count, dimensions, all PDF boxes (MediaBox, CropBox, TrimBox, BleedBox, ArtBox), and page labels
- **Metadata** — read document Info (title, author, dates, etc.)
- **Encrypt** — password-protect PDFs with RC4-128 (PDF 1.4 Standard Security Handler) and granular viewer permissions (print, copy, modify, annotate, form fill, accessibility, assembly, high-res print)
- **Validate** — check structural integrity of a PDF file
- **Text extraction** — extract text from pages in visual reading order with full layout info (coordinates, font, bold/italic, color, sub/superscript)
- **Image extraction** — extract images as JPEG (passthrough) or PNG with position, dimensions, and color space metadata; supports DeviceRGB, DeviceGray, DeviceCMYK, Indexed, ICCBased color spaces, soft masks (alpha), inline images, and Form XObjects
- **Add images** — place JPEG or PNG images onto existing pages with precise positioning via PDF rectangles
- **Image to PDF** — convert standalone images to single-page PDFs with DPI-aware sizing, configurable page dimensions and margins
- **Replace images** — swap image data on existing pages while preserving position and size
- **Remove images** — delete images from pages, cleaning up resources and content stream operators
- **Remove unused objects** — clean up orphaned objects after modifications to reduce file size
- **Optimize images** — reduce file size by downscaling images above a target DPI and converting opaque PNGs to JPEG
- **Create blank documents** — create single-page blank PDFs with custom dimensions or predefined page formats (A4, Letter, Legal, A3)
- **Add blank pages** — append or insert blank pages into existing documents at any position
- **Add text** — draw text on pages with font selection, alignment, word wrap, color, background, underline, strikethrough, rotation, and behind-content mode
- **Text watermarks** — apply text watermarks to all or selected pages with full styling control
- **Stream input** — open PDFs from any `io.Reader`, not just file paths

## API Reference

`*Document` methods mutate the receiver in place. `Split` and `Extract` return fresh, fully-independent documents; all other operations (rotation, reorder, metadata, passwords, etc.) modify the document they are called on.

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
doc.SetMetadata(pdf.Metadata{
    Title:  "My Document",
    Author: "Jane Smith",
    Custom: map[string]string{"Department": "Legal"},
})
doc.Save("output.pdf")

// Update a single field: read → modify → write
meta, _ = doc.Metadata()
meta.Title = "Updated Title"
doc.SetMetadata(meta)

// Strip all metadata
doc.ClearMetadata()
doc.Save("clean.pdf")
```

### Encryption

```go
// Standalone function — encrypts with default all-allow permissions
err := pdf.Encrypt("input.pdf", "output.pdf", "userpass", "ownerpass")

// Via Document (applied on Save/WriteTo)
doc, _ := pdf.Open("input.pdf")
doc.SetPassword("userpass", "ownerpass")
err = doc.Save("output.pdf")

// With explicit viewer permissions (RC4-128, Standard Security Handler R=3).
// Flags omitted from Permissions{} are denied; if SetPermissions is not
// called at all, every operation is allowed (backward compatible default).
doc.SetPermissions(pdf.Permissions{
    AllowPrint:         true,
    AllowCopy:          true,
    AllowAccessibility: true,
})
doc.Save("restricted.pdf")
```

`Permissions` fields map to ISO 32000-1 §7.6.3.2 Table 22 bits 3, 4, 5, 6, 9, 10, 11, 12. The library encodes them with the Adobe convention (reserved bits 7-8 and 13-32 set high). Permissions are enforced by PDF viewers — the library itself is not a DRM mechanism.

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

### Text Extraction

```go
doc, _ := pdf.Open("input.pdf")

// Simple text — one string per page
texts, err := doc.ExtractText()
for i, text := range texts {
    fmt.Printf("=== Page %d ===\n%s\n", i+1, text)
}

// Structured text with layout info
layouts, err := doc.ExtractTextWithLayout()
for _, lines := range layouts {
    for _, line := range lines {
        fmt.Printf("Y=%.1f: %s\n", line.Y, line.Text)
        for _, frag := range line.Fragments {
            fmt.Printf("  [%.1f,%.1f] %q font=%s size=%.1f bold=%v italic=%v\n",
                frag.X, frag.Y, frag.Text, frag.FontName, frag.FontSize,
                frag.Bold, frag.Italic)
        }
    }
}

// Per-page extraction
page, _ := doc.Page(1)
text, err := page.ExtractText()
lines, err := page.ExtractTextWithLayout()
```

### Image Extraction

```go
doc, _ := pdf.Open("input.pdf")

// Extract images from all pages
allImages, err := doc.ExtractImages()
for pageIdx, images := range allImages {
    for imgIdx, img := range images {
        ext := ".png"
        if img.Format == pdf.ImageFormatJPEG {
            ext = ".jpg"
        }
        img.Save(fmt.Sprintf("page%d_img%d%s", pageIdx+1, imgIdx+1, ext))

        fmt.Printf("  %dx%d %s at (%.1f, %.1f)\n",
            img.Width, img.Height, ext, img.X, img.Y)
    }
}

// Per-page extraction
page, _ := doc.Page(1)
images, err := page.ExtractImages()
```

```go
// List image metadata without decoding (fast)
allInfos, err := doc.ImageInfos()
for pageIdx, infos := range allInfos {
    for _, info := range infos {
        fmt.Printf("page %d: %dx%d %s\n",
            pageIdx+1, info.Width, info.Height, info.Name)
    }
}

// Selectively extract only large images
for _, infos := range allInfos {
    for i, info := range infos {
        if info.Width >= 500 {
            img, _ := infos[i].Extract()
            img.Save(fmt.Sprintf("large_%d.png", i))
        }
    }
}
```

Images are output as JPEG (passthrough for DCTDecode streams) or PNG (everything else). Supported color spaces: DeviceRGB, DeviceGray, DeviceCMYK (converted to RGB), Indexed (palette expansion), and ICCBased. Soft masks are applied as PNG alpha channels.

### Adding Images

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)

// Add a JPEG image at position (100, 600) with size 200x150
rect := pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 750}
page.AddImage("logo.jpg", rect)

// Add from stream
f, _ := os.Open("photo.png")
page.AddImageFromStream(f, pdf.Rectangle{LLX: 50, LLY: 50, URX: 250, URY: 250})
f.Close()

doc.Save("output.pdf")
```

### Image to PDF

```go
// Convert an image to a single-page PDF (DPI-aware page sizing)
doc, _ := pdf.ImageToDocument("photo.jpg")
doc.Save("photo.pdf")

// With explicit A4 page size and margins
doc, _ = pdf.ImageToDocument("logo.png", pdf.ImageToDocumentOptions{
    PageWidth:  595, // A4
    PageHeight: 842,
    MarginLeft: 72,  // 1 inch margins
    MarginRight: 72,
    MarginTop:  72,
    MarginBottom: 72,
})
doc.Save("logo_a4.pdf")
```

### Replacing and Removing Images

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)
infos, _ := page.ImageInfos()

// Replace first image with a new one
infos[0].Replace("new_logo.jpg")

// Replace from stream
f, _ := os.Open("photo.png")
infos[1].ReplaceFromStream(f)
f.Close()

// Remove an image
infos[2].Remove()

doc.Save("output.pdf")
```

### Cleaning Up Unused Objects

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)
infos, _ := page.ImageInfos()
infos[0].Remove()

removed := doc.RemoveUnusedObjects()
fmt.Printf("removed %d unused objects\n", removed)
doc.Save("output.pdf") // smaller file
```

### Optimizing Images

```go
doc, _ := pdf.Open("large.pdf")
optimized, err := doc.OptimizeImages(pdf.OptimizeImageOptions{
    MaxDPI:           150,
    JPEGQuality:      75,
    ConvertPNGToJPEG: true,
})
fmt.Printf("optimized %d images\n", optimized)
doc.Save("smaller.pdf")
```

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

### Adding Blank Pages

```go
doc, _ := pdf.Open("input.pdf")

// Append a blank A4 page
doc.AddBlankPageFromFormat(pdf.PageFormatA4)

// Insert a landscape Letter page at position 2
doc.InsertBlankPageFromFormat(2, pdf.PageFormatLetter.Landscape())

doc.Save("output.pdf")
```

### Adding Text

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)

// Define a reusable text style
title := pdf.TextStyle{
    Font:   pdf.FontHelveticaBold,
    Size:   24,
    Color:  &pdf.Color{R: 0, G: 0, B: 0.8, A: 1},
    HAlign: pdf.HAlignCenter,
}

// Draw text inside a rectangle (word wrap + clipping)
page.AddText("Hello, PDF!", title, pdf.Rectangle{
    LLX: 50, LLY: 700, URX: 545, URY: 750,
})

// Rotated text (e.g. column headers)
page.AddText("Revenue", pdf.TextStyle{
    Font:     pdf.FontHelveticaBold,
    Size:     10,
    Rotation: 45, // degrees counter-clockwise
}, pdf.Rectangle{LLX: 100, LLY: 500, URX: 130, URY: 600})

doc.Save("output.pdf")
```

### Text Watermarks

```go
doc, _ := pdf.Open("input.pdf")

// Add watermark to all pages
doc.AddTextWatermark("CONFIDENTIAL", pdf.TextStyle{
    Font:     pdf.FontHelveticaBold,
    Size:     60,
    Color:    &pdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3},
    Rotation: 45,
    HAlign:   pdf.HAlignCenter,
    VAlign:   pdf.VAlignMiddle,
    Behind:   true, // draw under existing content
})

// Watermark on specific pages only
doc.AddTextWatermark("DRAFT", pdf.TextStyle{
    Font:     pdf.FontHelvetica,
    Size:     48,
    Color:    &pdf.Color{R: 1, G: 0, B: 0, A: 0.2},
    Rotation: -45,
    HAlign:   pdf.HAlignCenter,
    VAlign:   pdf.VAlignMiddle,
    Behind:   true,
}, 1, 3) // pages 1 and 3

doc.Save("output.pdf")
```

### Document API

```go
doc, err := pdf.Open("input.pdf")

fmt.Println(doc.PageCount())   // total pages
fmt.Println(doc.Metadata())    // Info dictionary

// Rotate pages (mutates in place; rotation accumulates)
err = doc.Rotate(pdf.Rotate90, 1, 2)
err = doc.Rotate(pdf.Rotate90, 1, 2) // pages 1 and 2 are now at 180°

// Set absolute rotation (replaces existing rotation)
err = doc.SetRotation(pdf.Rotate90, 1) // page 1 is now exactly 90°
err = doc.SetRotation(pdf.Rotate0)     // reset all pages to 0°

// Reorder pages (pages may be repeated or omitted)
err = doc.Reorder([]int{3, 1, 2})

// Append pages from one or more documents (mutates receiver)
doc2, _ := pdf.Open("part2.pdf")
doc3, _ := pdf.Open("part3.pdf")
doc.Append(doc2, doc3)

// Split into individual page documents (fresh independent Documents)
pages, err := doc.Split()

// Extract page ranges to a new independent Document
sub, err := doc.Extract(pdf.PageRange{From: 1, To: 3})

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
