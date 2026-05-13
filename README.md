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
- **Encrypt** — password-protect PDFs with AES-128 (default, ISO 32000-1 §7.6.3.2 V=4 R=4 `/CFM /AESV2`), AES-256 (ISO 32000-2 §7.6.4 V=5 R=6 `/CFM /AESV3`, PDF 2.0), or RC4-128 (legacy V=2 R=3); Standard Security Handler with user + owner passwords and granular viewer permissions (print, copy, modify, annotate, form fill, accessibility, assembly, high-res print). Round-trip preserves AcroForm fields, annotations, and embedded files
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
- **Forms (AcroForm)** — read, fill, and build from scratch all standard field types (text, checkbox, radio, combo box, list box, push button); programmatic field creation with `AddTextField`/`AddCheckbox`/`AddRadioGroup`/`AddComboBox`/`AddListBox`/`AddPushButton`; `RemoveField`; non-ASCII values encoded as UTF-16BE; viewers regenerate appearances via auto `/NeedAppearances=true`
- **Annotations** — Link (with /A actions: GoToURI, GoTo, Named, SubmitForm, ResetForm, JavaScript), Highlight, Underline, StrikeOut, Squiggly. Page-scoped collection API (`Page.Annotations()` with `Add`/`At`/`Delete`/`DeleteAt`); existing form widgets surface as read-only `WidgetAnnotation`. Drawing primitives (Square/Circle/Line/Ink) with full ISO 32000-1 border styles (Solid/Dashed/Beveled/Inset/Underline) and 10 line-ending styles. Text-bearing types (Text sticky note, FreeText with callout/typewriter/cloudy-border modes, Stamp with 14 predefined visuals + custom image override). FileAttachment with file embedding (path/stream + MIME detection); Redact with mark/apply modes (irreversible content removal of text glyphs, images, and paths via `Document.ApplyRedactions()`); `NewJavaScriptAction` public constructor with security warning. `/AP` appearance streams generated automatically — annotations render natively in any spec-conforming viewer

## API Reference

`*Document` methods mutate the receiver in place. `Split` and `Extract` return fresh, fully-independent documents; all other operations (rotation, reorder, metadata, passwords, etc.) modify the document they are called on.

### Opening documents

```go
// From a file path
doc, err := pdf.Open("input.pdf")

// From an io.Reader (stream, HTTP response, etc.)
doc, err = pdf.OpenStream(r)

// Encrypted files: returns ErrEncrypted from plain Open; supply a password
doc, err = pdf.Open("locked.pdf")
if errors.Is(err, pdf.ErrEncrypted) {
    doc, err = pdf.OpenWithPassword("locked.pdf", "secret")
}

// OpenWithPassword also works on plain PDFs (password is silently ignored),
// so it's a safe drop-in for code that doesn't know up front whether the
// input is encrypted. The password is tried as both user and owner.
doc, err = pdf.OpenWithPassword("maybe-encrypted.pdf", "secret")
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

// Simple case on a Document (applied on Save/WriteTo)
doc, _ := pdf.Open("input.pdf")
doc.SetPassword("userpass", "ownerpass")
err = doc.Save("output.pdf")

// Granular permissions (RC4-128, Standard Security Handler R=3).
// Fields omitted from Permissions{} are denied; if SetPermissions is not
// called at all, every operation is allowed (backward compatible default).
doc.SetPermissions(pdf.Permissions{
    AllowPrint:         true,
    AllowCopy:          true,
    AllowAccessibility: true,
})
doc.Save("restricted.pdf")

// One-call unified API via options — equivalent to SetPassword + SetPermissions
// in a single struct; replaces any prior encryption config on the document.
// Algorithm defaults to AES-128 (ISO 32000-1 V=4 R=4 /CFM /AESV2). Pass
// pdf.EncryptionAlgRC4_128 for legacy RC4-128 V=2 R=3 output, or
// pdf.EncryptionAlgAES256 for AES-256 V=5 R=6 (ISO 32000-2; output uses
// %PDF-2.0 header and requires Acrobat DC or another PDF 2.0 viewer).
doc.SetEncryption(pdf.EncryptionOptions{
    UserPassword:  "userpass",
    OwnerPassword: "ownerpass",
    Permissions:   &pdf.Permissions{AllowPrint: true, AllowCopy: true},
    // Algorithm:  pdf.EncryptionAlgAES128, // default
    // Algorithm:  pdf.EncryptionAlgAES256, // ISO 32000-2; produces %PDF-2.0
    // Algorithm:  pdf.EncryptionAlgRC4_128, // legacy
})
doc.Save("restricted.pdf")

// Reading permissions from an encrypted file (works after OpenWithPassword)
doc, _ = pdf.OpenWithPassword("restricted.pdf", "userpass")
perms, ok := doc.Permissions()
if ok {
    fmt.Printf("can print: %v, can copy: %v\n", perms.AllowPrint, perms.AllowCopy)
}

// Edit-in-place: OpenWithPassword preserves the password, so a plain Save
// re-encrypts with the same password. To produce a plaintext copy, call
// RemoveEncryption explicitly before Save.
doc, _ = pdf.OpenWithPassword("restricted.pdf", "userpass")
doc.AddTextWatermark("APPROVED", pdf.TextStyle{Size: 48})
doc.Save("restricted_signed.pdf")          // still encrypted
doc.RemoveEncryption()
doc.Save("decrypted_copy.pdf")             // plaintext
```

`Permissions` fields map to ISO 32000-1 §7.6.3.2 Table 22 bits 3, 4, 5, 6, 9, 10, 11, 12. The library encodes them with the Adobe convention (reserved bits 7-8 and 13-32 set high). Permissions are enforced by PDF viewers — the library itself is not a DRM mechanism.

In `EncryptionOptions`, `Permissions` is a pointer so that `nil` (omitted) means "grant all", distinguishing the default from an explicit `&Permissions{}` which denies everything.

### Forms (AcroForm)

```go
doc, _ := pdf.Open("template.pdf")

// Iterate every form field
for _, f := range doc.Form().Fields() {
    fmt.Printf("%s = %q (type %v)\n", f.FullName(), f.Value(), pdf.FieldType(f))
}

// Set values by type
text := doc.Form().Field("name").(*pdf.TextBoxField)
text.SetValue("Jane Doe")

check := doc.Form().Field("subscribe").(*pdf.CheckboxField)
check.SetChecked(true)

radio := doc.Form().Field("plan").(*pdf.RadioButtonField)
radio.Options()[1].SetSelected(true)

combo := doc.Form().Field("country").(*pdf.ComboBoxField)
combo.SetSelected(0) // by index into combo.Options()

list := doc.Form().Field("interests").(*pdf.ListBoxField)
if list.MultiSelect() {
    list.SetSelected(0, 2, 3)
} else {
    list.SetSelected(1)
}

// Save — viewers regenerate appearances on open via auto /NeedAppearances=true
doc.Save("filled.pdf")
```

Field values containing non-ASCII characters (e.g. Cyrillic) are encoded as UTF-16BE with a BOM so any spec-conforming viewer reads them back correctly.

#### Building forms from scratch

```go
doc := pdf.NewDocument(595, 842)
form := doc.Form()

// Single-widget fields
tf, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 725}, "name")
tf.SetMaxLen(50)
tf.SetValue("Jane Doe")

cb, _ := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 660, URX: 70, URY: 680}, "subscribe")
cb.SetChecked(true)

combo, _ := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 625}, "country",
    []pdf.ChoiceOption{{Value: "USA"}, {Value: "Canada"}})
combo.SetSelected(0)

// Radio group: widgets can span multiple pages
rb, _ := form.AddRadioGroup("plan", []pdf.RadioItem{
    {PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 540, URX: 70, URY: 560}, Export: "basic"},
    {PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 510, URX: 70, URY: 530}, Export: "premium"},
})
rb.Options()[0].SetSelected(true)

form.AddPushButton(1, pdf.Rectangle{LLX: 50, LLY: 460, URX: 200, URY: 490}, "submit", "Submit")

// Remove a field by name
form.RemoveField("subscribe")

doc.Save("form.pdf")
```

`/AcroForm/NeedAppearances` is auto-set on every Add or structural mutation, so any standards-compliant viewer regenerates the field appearances at display time.

Out of scope for this release: self-rendered `/AP` appearances (separate epic — `/NeedAppearances=true` covers most viewers), and form flattening.

### Annotations

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)

// Add a hyperlink
link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
link.SetAction(pdf.NewGoToURIAction("https://example.com"))
link.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
link.SetHighlight(pdf.LinkHighlightOutline) // /H click feedback: None / Invert (default) / Outline / Push
page.Annotations().Add(link)

// Highlight a passage
hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
hl.SetTitle("Reviewer")
hl.SetContents("Important paragraph")
hl.SetQuadPoints([]pdf.QuadPoint{
    {X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600},
})
page.Annotations().Add(hl)

// Iterate and filter
for _, a := range page.Annotations().All() {
    if a.AnnotationType() != pdf.AnnotationTypeLink {
        continue
    }
    if uri, ok := a.(*pdf.LinkAnnotation).Action().(*pdf.GoToURIAction); ok {
        fmt.Println(uri.URI())
    }
}

// Wire a form push button to a server submit
submit := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 80})
submit.SetAction(pdf.NewSubmitFormAction(
    "https://example.com/api/subscribe",
    []string{"name", "email"},
    pdf.SubmitGetMethod|pdf.SubmitExportFormat,
))
page.Annotations().Add(submit)

doc.Save("with_annotations.pdf")
```

Supported subtypes: `Link`, `Highlight`, `Underline`, `StrikeOut`, `Squiggly`, `Square`, `Circle`, `Line`, `Ink`, `Text`, `FreeText`, `Stamp`, `FileAttachment`, `Redact`. Existing form widgets surface as `WidgetAnnotation` for read-only inspection — to mutate form fields use the `Form` API.

### Drawing annotations (Square / Circle / Line / Ink)

```go
doc := pdf.NewDocument(595, 842)
page, _ := doc.Page(1)

// Filled rectangle with dashed red border
sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 800})
sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
sq.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
sq.SetBorderStyle(pdf.BorderDashed)
sq.SetDashPattern([]float64{4, 2})
page.Annotations().Add(sq)

// Blue circle, beveled border
c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 250, LLY: 700, URX: 400, URY: 800})
c.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
c.SetBorderStyle(pdf.BorderBeveled)
c.SetBorderWidth(3)
page.Annotations().Add(c)

// Arrow line with closed-arrow heads on both ends
ln := pdf.NewLineAnnotation(page,
    pdf.Point{X: 50, Y: 600}, pdf.Point{X: 400, Y: 500})
ln.SetStartLineEnding(pdf.LineEndingClosedArrow)
ln.SetEndLineEnding(pdf.LineEndingClosedArrow)
ln.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
page.Annotations().Add(ln)

// Smooth ink stroke (Catmull-Rom smoothing applied automatically for 3+ points)
ink := pdf.NewInkAnnotation(page, [][]pdf.Point{
    {{X: 50, Y: 400}, {X: 100, Y: 450}, {X: 150, Y: 420},
     {X: 200, Y: 460}, {X: 250, Y: 400}},
})
ink.SetColor(&pdf.Color{R: 0, G: 0.5, B: 0, A: 1})
page.Annotations().Add(ink)

doc.Save("drawing.pdf")
```

Border styles available: `BorderSolid`, `BorderDashed`, `BorderBeveled`, `BorderInset`,
`BorderUnderline` (per ISO 32000-1 Table 168). Line endings: `LineEndingNone`,
`Square`, `Circle`, `Diamond`, `OpenArrow`, `ClosedArrow`, `Butt`, `ROpenArrow`,
`RClosedArrow`, `Slash` (Table 176). Each property setter (`SetBorderStyle`,
`SetColor`, `SetStrokes`, etc.) immediately regenerates the annotation's
appearance stream so `/AP/N` is always in sync; no `/NeedAppearances=true`
required, drawing annotations render in any spec-conforming viewer.

### Text-bearing annotations (Text / FreeText / Stamp)

```go
doc := pdf.NewDocument(595, 842)
page, _ := doc.Page(1)

// Sticky-note annotation (icon-based, no /AP needed — viewer renders icon)
ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
ta.SetIcon(pdf.TextIconComment)
ta.SetTitle("Reviewer")
ta.SetContents("Important comment about this paragraph.")
ta.SetOpen(true)
page.Annotations().Add(ta)

// FreeText — text drawn directly on the page
ft := pdf.NewFreeTextAnnotation(page,
    pdf.Rectangle{LLX: 100, LLY: 600, URX: 400, URY: 660},
    "Highlighted note\nMulti-line supported",
    pdf.TextStyle{
        Font:       pdf.FontHelveticaBold,
        Size:       14,
        Color:      &pdf.Color{R: 0, G: 0, B: 0.5, A: 1},
        Background: &pdf.Color{R: 1, G: 1, B: 0.85, A: 1},
        HAlign:     pdf.HAlignCenter,
        VAlign:     pdf.VAlignMiddle,
    })
ft.SetBorderWidth(2)
ft.SetBorderEffect(pdf.BorderEffectCloudy)
page.Annotations().Add(ft)

// FreeText with callout pointer (Acrobat-style "comment with arrow")
ftc := pdf.NewFreeTextAnnotation(page,
    pdf.Rectangle{LLX: 300, LLY: 500, URX: 500, URY: 570},
    "Callout note",
    pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})
ftc.SetCalloutPoints([]pdf.Point{
    {X: 250, Y: 530}, // knee
    {X: 200, Y: 480}, // endpoint (arrow tip)
})
ftc.SetEndLineEnding(pdf.LineEndingClosedArrow)
page.Annotations().Add(ftc)

// Stamp — predefined "Approved" with built-in green visual
sap := pdf.NewStampAnnotation(page,
    pdf.Rectangle{LLX: 100, LLY: 400, URX: 300, URY: 470},
    pdf.StampNameApproved)
page.Annotations().Add(sap)

// Stamp with custom image (PNG / JPEG via path or io.Reader)
sci := pdf.NewStampAnnotation(page,
    pdf.Rectangle{LLX: 320, LLY: 400, URX: 500, URY: 470},
    pdf.StampNameDraft) // /Name still set per spec, image overrides /AP
sci.SetCustomImage("logo.png")
page.Annotations().Add(sci)

doc.Save("text_bearing.pdf")
```

`TextAnnotation` icon names: `TextIconNote` (default), `Comment`, `Key`, `Help`, `NewParagraph`,
`Paragraph`, `Insert`. `FreeTextAnnotation` supports three intents — `FreeTextIntentFreeText`
(default), `Callout` (with arrow), `Typewriter` (no border/background). Border effects: `BorderEffectNone`
(default) and `BorderEffectCloudy` (wavy border per ISO 32000-1 §12.5.4 /BE). All 14 stamp names
from ISO 32000-1 Table 184 (`StampNameApproved` through `StampNameTopSecret`) ship with
library-default colored visuals — green for positive (Approved/Final/ForPublicRelease), red for
warning (Confidential/Expired/etc.), orange for informational (Draft/AsIs/etc.), gray for neutral
(Departmental). Custom images (JPEG / PNG) override the default visual.

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
(after `Document.ApplyRedactions()`, page content is rewritten to remove text glyphs,
image XObjects, and paths inside `/QuadPoints` regions; the redact annotation is then
deleted). Use `ValidateRedactions()` first as a pre-flight parseability check.
**`NewJavaScriptAction`** carries a documented security warning — embedded JavaScript
executes in the recipient's viewer.

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
