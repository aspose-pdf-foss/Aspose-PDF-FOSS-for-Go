# Markdown → PDF — design

**Date:** 2026-07-14 · **Status:** approved (brainstorm with Andrey) · **Epic:** beads ids assigned at filing

## Motivation

Render Markdown documents as typographically clean, paginated PDFs — pure Go, zero
dependencies, deterministic. Mirrors Aspose.PDF for .NET's Markdown import
(`new Document("file.md", new MdLoadOptions())`), adapted to this library's exported-func
idiom like `ImageToDocument`.

Why Markdown (and not HTML) is the right scope: a closed grammar with no cascade, no box
model and no scripting — the *renderer* owns the typography, so one well-made default
theme produces a good-looking PDF for any input (the LaTeX/Typst model). The lower half
of the stack already exists: flow layout with auto-pagination, tables with header repeat,
lists, images, floating boxes, font embedding. The Go ecosystem has no quality native
Markdown→PDF (existing options drive Chrome or draw crudely) — this can be the best one.

Synergies: LLMs emit Markdown natively → `ai.SummaryCopilot` can render *formatted*
summaries; `FlowOptions.Tagged` gives Markdown → PDF/UA in one call; the inline-runs
layouter built here is the exact prerequisite for a future HTML-lite importer.

## Decisions (from brainstorm)

1. **Dialect: CommonMark + GFM core** — full CommonMark (0.31) plus the GitHub
   extensions people actually use: **tables, strikethrough, task lists, autolinks**.
   Footnotes/definition lists/alerts = later, by demand.
2. **API: all three entry points**, thin wrappers over one parser+mapper:
   - `MarkdownToDocument(path, opts ...MarkdownOptions) (*Document, error)` +
     `MarkdownToDocumentFromStream(r io.Reader, opts ...)` — mirrors `MdLoadOptions`
   - `(*Flow).AddMarkdown(md string) *Flow` — insert into an existing flow
   - `(*Page).AddMarkdown(md string, rect Rectangle) error` — Rectangle paradigm
3. **Styling: one carefully-made default theme + basic options** (`MarkdownOptions`);
   a full per-element theme struct is a later, demand-driven addition.
4. **Conformance: run the official CommonMark `spec.json` (~650 cases)** in tests,
   comparing parsed structure (our AST → normalized HTML render for comparison is the
   standard trick — implement a minimal AST→HTML serializer used *only in tests*).
   Target: high pass rate with documented deviations. Provable parser quality.

## Package layout

Root package (it's PDF functionality, not AI): `markdown.go` (public API + options),
`markdown_parse_block.go` (block parser), `markdown_parse_inline.go` (inline parser),
`markdown_ast.go` (node types), `markdown_render.go` (AST → flow mapping + theme),
`flow_runs.go` (styled-runs paragraph layout). Test data: official `spec.json` vendored
under `testdata/commonmark_spec.json` (CC-BY-SA license note in the file header).

## Architecture

### 1. Parser (CommonMark two-phase, per the spec's own algorithm)

- **Block phase** (`markdown_parse_block.go`): line-by-line open-block tree —
  paragraphs, ATX (`#`) + setext (`===`) headings, block quotes (`>` with lazy
  continuation), bullet/ordered lists (nesting, tightness, start numbers), fenced
  (``` ``` ```, info string) + indented code blocks, thematic breaks, link-reference
  definitions, GFM tables (header/delimiter/rows with `:---:` alignment), blank-line
  handling. Raw HTML *blocks* are recognized per spec but **skipped at render** (v1).
- **Inline phase** (`markdown_parse_inline.go`): per-paragraph/heading/cell —
  code spans, emphasis/strong with the delimiter-run algorithm (`*`/`_` rules),
  links `[text](dest "title")` + reference links + autolinks `<https://…>` + GFM bare
  URLs, images `![alt](src)`, backslash escapes, hard breaks (trailing `\` / two
  spaces), GFM `~~strike~~`, entity references via stdlib `html.UnescapeString`.
  Raw inline HTML: recognized, rendered as literal text minus the tags (i.e. skipped),
  except `<br>` → hard break.
- **AST** (`markdown_ast.go`): `mdBlock` (kind + children + fields) and `mdInline`
  (kind, text, dest, children). Unexported — the public surface stays the three entry
  points.

### 2. Styled-runs layout (`flow_runs.go`) — the substantial new machinery

A paragraph is a sequence of **runs**: `textRun{text, style TextStyle, linkDest}`.
New internal layouter `layoutRuns`:
- greedy line-breaking at word boundaries *across run borders*, measuring each word
  with its own run's font metrics (`fontWidthAndAscent`), honoring the flow width;
- emits each line as positioned per-run `AddText`-style segments on one shared
  baseline (mixed font sizes align by baseline; line height = max run height × spacing);
- records per-run rects so **links become real `LinkAnnotation`s** (URI action for
  external, GoTo for `#fragment` targets resolved to headings) and underline/strike
  decorations land exactly under their run;
- reusable by the future HTML-lite importer unchanged.

Flow gets one new element kind (`fkRuns`) so runs paragraphs paginate like plain ones
(split line-by-line across pages). `Page.AddMarkdown` reuses the boxed flow mode
(`errBoxFull` → clip, like FloatingBox).

### 3. Rendering / mapping (`markdown_render.go`)

AST → flow elements with the default theme:

| Markdown | Rendering |
|---|---|
| headings 1–6 | `fkRuns` with theme sizes (h1 ≈ 2×, h2 ≈ 1.5×, … h6 = base, bold; h1/h2 get a light bottom rule), kept-with-next spacing |
| paragraph | `fkRuns` (base font, justified-left, 1.35 line spacing) |
| bold/italic/strike | style overlays on runs (bold+italic compose; strike via `Strikethrough`) |
| inline code | mono font on a light-gray background run |
| code block | `FloatingBox`, gray background, mono font, no wrap-reflow (long lines wrap hard at rune boundary), preserved blank lines |
| block quote | `FloatingBox` with `BorderSideLeft` rule + inset, recursive content |
| lists | flow list machinery extended for nesting: bullets `•`/`◦`/`▪` by depth, ordered `1.`/`a.`/`i.` by depth, task items draw a vector checkbox (checked/unchecked) |
| GFM table | `Table` + `AddTable`: header row bold + `SetRepeatingRowsCount(1)`, column alignment from `:---:`, cell inlines via runs |
| image | `Flow.AddImage` (aspect preserved, scaled to column width); alt text used for tagging |
| thematic break | full-width light `DrawLine` |
| links | colored (theme blue) + underline + `LinkAnnotation` overlay |
| hard/soft breaks | line break / space |

**Images: local only.** `src` resolved against `MarkdownOptions.BasePath` (defaults to
the .md file's directory for `MarkdownToDocument`, cwd for stream/string forms);
`data:image/png|jpeg;base64,` supported. **Remote URLs are skipped with the alt text
rendered as a placeholder** — the root package stays network-free by policy.

### 4. Public options

```go
type MarkdownOptions struct {
    Format    PageFormat // zero → A4
    Margin    float64    // page margin, zero → 54pt
    BaseFont  Font       // body face; nil → Helvetica (Std-14, Latin-1) — pass LoadFont face for Cyrillic etc.
    BoldFont, ItalicFont, BoldItalicFont Font // optional matching styles for BaseFont (Std-14 defaults derive automatically)
    CodeFont  Font       // nil → Courier
    BaseSize  float64    // zero → 11pt; the whole theme scales from it
    BasePath  string     // image resolution root
    Tagged    bool       // build a structure tree (headings→/H1…, lists→/L, tables via AddTaggedTable, images→/Figure+Alt) → PDF/UA-ready
}
```

`Flow.AddMarkdown` inherits fonts/size from a `MarkdownOptions`-shaped setter on the
flow (`Flow.SetMarkdownOptions`) or uses defaults; `Page.AddMarkdown` takes optional
`MarkdownOptions` variadic.

## Testing

- **Conformance:** `TestCommonMarkSpec` runs every `spec.json` example through the
  parser and a minimal test-only AST→HTML serializer, comparing normalized output.
  Known deviations (if any) listed in one place with reasons. GFM extensions tested
  from a curated case set (tables/strike/tasks/autolinks).
- **Layout:** runs layouter unit tests (mixed-size baseline, cross-run wrapping, link
  rects); golden-structure tests via `ExtractTextWithLayout` (heading sizes/bold,
  code font, list indents); pagination test (long doc → page count + content split);
  `SearchText` finds link text and its `LinkAnnotation` rect intersects the match.
- **Visual:** render a showcase .md (README-like: headings, lists, table, code,
  quote, image, task list) to PNG at 150 DPI and **review by eye** (feedback:
  showcase-is-the-face standard); consider a feature-showcase page later.
- **Tagged:** `MarkdownOptions.Tagged` output passes `ValidatePDFUA`.

## Phases (tickets)

1. **Block parser + spec harness** — blocks only, spec.json cases for block structure.
2. **Inline parser** — full inline grammar + entities; spec pass rate target reached.
3. **Styled-runs layouter** (`flow_runs.go`, `fkRuns`) — mixed-style paragraphs in
   flow, link rects, pagination.
4. **Renderer + theme + public API** — AST→flow mapping, default theme, GFM extras,
   all three entry points, examples, docs.
5. **Tagged output + AI synergy** — Tagged option → PDF/UA; `ai.SummaryOptions`
   gains `Markdown bool` (render `GetSummaryDocument` through the Markdown pipeline).

## Out of scope (v1, documented)

Syntax highlighting in code blocks (needs language grammars); footnotes, definition
lists, GFM alerts; raw HTML rendering (skipped; `<br>` honored); remote image
fetching (network-free root package); YAML front matter (skipped if present — parsed
off, values exposed later if demanded); full theme struct; math/LaTeX.
