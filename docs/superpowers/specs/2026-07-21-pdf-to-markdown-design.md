# PDF → Markdown export — design

**Date:** 2026-07-21 · **Status:** approved (options confirmed with Andrey) · **Epic:** assigned at filing

## Motivation

The reverse of the Markdown→PDF renderer: re-assemble a PDF's content as GFM-flavoured
Markdown. Primary audiences: docs pipelines and **AI/RAG** (Aspose's own RAG stack chunks
PDFs *via Markdown*; ours can feed the same pattern). Reuses the flow-mode HTML exporter's
structural analysis (`Paragraphs()` + heading inference + image interleaving) with a
Markdown serializer and richer inline fidelity.

## Decisions (confirmed)

1. **Images: files alongside by default** — `SaveMarkdown` writes them into
   `<stem>_files/` next to the .md (SHA-256 dedup, like the HTML exporter);
   `ImageWriter` callback externalizes anywhere (S3/CDN); `EmbedImages` switches to
   `data:` URLs; `NoImages` skips. `WriteMarkdown` (stream) has no directory — images
   are skipped unless `ImageWriter`/`EmbedImages` is set.
2. **Full inline markup in v1** — headings (font-size ratio vs the length-weighted body
   median, same 1.7/1.35/1.14 thresholds as HTML flow mode), paragraphs with inline
   `**bold**`/`*italic*`/`` `code` `` runs merged from fragments, links recovered from
   link annotations by rect intersection, list items by bullet-glyph/numbering
   detection, fenced code blocks for monospace paragraphs (indentation preserved from
   X offsets). **Tables deferred** (no table reader yet — cell text flows as
   paragraphs; documented).

## Public API (house style, mirrors SaveHTML's shape)

```go
type MarkdownSaveOptions struct {
    Pages       []int  // 1-based subset; nil = all
    ImageDir    string // SaveMarkdown: dir for image files; "" → "<stem>_files"
    ImageWriter func(name string, data []byte) (url string, err error)
    EmbedImages bool   // data: URLs instead of files
    NoImages    bool   // skip images entirely
}
func (d *Document) SaveMarkdown(path string, opts ...MarkdownSaveOptions) error
func (d *Document) WriteMarkdown(w io.Writer, opts ...MarkdownSaveOptions) error
```

## Pipeline (markdown_export.go)

1. **Block collection** — per selected page: `Paragraphs()` sections/paragraphs →
   blocks ordered by visual top; images interleaved via the existing
   `insertFlowImage`; body size via `weightedMedianSize` (both shared with
   `html_export_flow.go`).
2. **Per-block classification** — monospace-dominant (via `fontFamilyClass`) → fenced
   code block; heading by size ratio (<200 chars); leading `•`/`◦`/`-`/`–` + space →
   `- ` bullet, `^\d{1,3}[.)]` → ordered item (consecutive items stay blank-line-free);
   else paragraph.
3. **Inline runs** — walk `Lines[].Fragments[]`: run key = {bold, italic, mono-font,
   linkURI}; adjacent same-key fragments merge; lines join with a space. Emphasis
   markers hug non-space edges; Markdown specials escaped (code spans use backtick
   fencing instead). Links: page `Annotations()` → `LinkAnnotation`+`GoToURIAction`
   rects; a fragment joins a link when its midpoint falls inside → `[text](uri)`.
4. **Code blocks** — one output line per source `TextLine`; leading indent
   reconstructed from the first fragment's X offset (≈0.6 em per char for mono).
5. **Images** — `![](url)` through a sink chain like the HTML exporter's
   (name `pN_imgK.ext`, SHA-256 dedup across the document).

## Testing

- Unit: inline-run merging/escaping/emphasis-edge rules; list/heading/code
  classification on crafted docs.
- **Round-trip synergy**: render a known .md with `MarkdownToDocument`, export with
  `SaveMarkdown`, assert headings/bold/links/list markers/code fences survive.
- Images: file mode writes deduped files with relative links; embed mode produces
  `data:`; stream mode without writer skips.

## Out of scope (v1, documented)

Tables (needs a table reader — future `TableAbsorber` work); multi-column reading-order
beyond what `Paragraphs()` provides; front matter; footnotes; heading levels 4-6
(flow heuristic caps at 3); vector graphics (dropped, like HTML flow mode).
