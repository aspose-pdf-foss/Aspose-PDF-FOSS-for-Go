# PDF Comparison (text diff + marked-up output) — Design

Date: 2026-09-15 · Epic: pdf-go-175w (beads) · Status: approved

## Goal

Compare two PDF documents and report what changed, mirroring
Aspose.PDF for .NET's `Aspose.Pdf.Comparison` namespace — specifically its
`TextPdfComparer` (page, page-by-page and flat document comparison) — in this
library's options idiom. Phase 1 covers the **text** comparer and one output:
a **marked-up copy of the original**, where insertions and deletions are real
annotations over the unchanged page. Pure Go, zero dependencies; built on the
existing layout-extraction, table-detection and annotation-appearance layers.

Where we go beyond the reference: Aspose's `DiffOperation` carries only
`Operation` + `Text` (no coordinates — rectangles appear only in the 26.7
side-by-side `EditContainer`), and its `PdfOutputGenerator` produces a newly
generated text document rather than the marked-up original. We carry page
numbers and rectangles on **both** sides of every operation from the start,
which is what makes marking up the real page possible.

## Public API

Entry points (package-level functions, as `TextPdfComparer`'s methods are
static; names mirror it):

- `ComparePages(p1, p2 *Page, opts ...ComparisonOptions) ([]DiffOperation, error)`
- `CompareDocumentsPageByPage(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error)`
- `CompareFlatDocuments(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error)`

Model:

```go
type Operation int // OperationEqual, OperationInsert, OperationDelete

type DiffOperation struct {
    Operation   Operation
    Text        string       // the run's words, joined with single spaces
    SourcePage  int          // 1-based page in d1; 0 for an insertion
    DestPage    int          // 1-based page in d2; 0 for a deletion
    SourceRects []Rectangle  // one per line the run touches, in d1
    DestRects   []Rectangle  // one per line the run touches, in d2
}
```

A *run* is a maximal sequence of adjacent words carrying the same operation.
Runs break at line and page boundaries, so every rectangle is a real box on a
real page; a run spanning three lines carries three rectangles. `Operation`,
`DiffOperation` and `Text` keep Aspose's names; the location fields are ours.

Result:

```go
type ComparisonResult struct{ /* unexported */ }

func (r *ComparisonResult) HasChanges() bool
func (r *ComparisonResult) Operations() []DiffOperation
func (r *ComparisonResult) PageOperations(pageNum int) []DiffOperation
func (r *ComparisonResult) Statistics() ComparisonStatistics
func (r *ComparisonResult) SaveMarkup(path string, opts ...DiffMarkupOptions) error
func (r *ComparisonResult) WriteMarkup(w io.Writer, opts ...DiffMarkupOptions) error

type ComparisonStatistics struct {
    EqualWords, InsertedWords, DeletedWords int
    ChangedPages []int // 1-based, ascending: the destination page for an
                       // inserted run, the source page for a deleted run —
                       // which in flat mode can mix the two numbering spaces
}
```

`HasChanges` mirrors `SideBySideDocsComparisonResult.HasChanges`;
`Statistics()` mirrors `CreateComparisonStatistics`. `PageOperations` filters
by the page on the side the operation physically lives (destination for
`Insert` and `Equal`, source for `Delete`).

Text restoration, mirroring `AssemblySourcePageText` /
`AssemblyDestinationPageText`:

- `AssembleSourceText(ops []DiffOperation) string` — `Equal` + `Delete`
- `AssembleDestinationText(ops []DiffOperation) string` — `Equal` + `Insert`

Options (zero value usable, last variadic wins — the `SearchOptions` idiom):

```go
type ComparisonOptions struct {
    ExtractionArea      *Rectangle   // compare only inside this area
    ExcludeAreas1       []Rectangle  // ignored regions in d1
    ExcludeAreas2       []Rectangle  // ignored regions in d2
    ExcludeTables       bool         // drop words inside detected tables
    EditOperationsOrder EditOperationsOrder
    IgnoreCase          bool         // ours; Aspose has no equivalent
}

type EditOperationsOrder int // EditOperationsDeleteFirst (default),
                             // EditOperationsInsertFirst

type DiffMarkupOptions struct {
    Side        DiffMarkupSide // DiffMarkupDestination (default), DiffMarkupSource
    InsertColor *Color     // default green (0.20, 0.72, 0.35)
    DeleteColor *Color     // default red   (0.88, 0.22, 0.22)
    Title       string     // annotation /T, default "Comparison"
    Flatten     bool
}
```

Named `DiffMarkupOptions`/`DiffMarkupSide`/`DiffMarkupDestination`/`DiffMarkupSource`
rather than a bare `Markup*` prefix: `paragraph.go` already exports
`MarkupParagraph`, `MarkupSection` and `PageMarkup` for structural text
extraction, an unrelated namespace in this flat package.

`ExtractionArea` is rejected together with `ExcludeTables`, `ExcludeAreas1` or
`ExcludeAreas2` (an error, not a silent precedence) — the same incompatibility
Aspose documents.

## Architecture (four files)

### 1. `comparison_tokens.go` — words with rectangles

One `ExtractTextWithLayout` per page. For each `TextLine`, `buildLineRuneMap`
gives the logical rune → fragment mapping (already RTL-corrected, the same
path `searchLine` takes); `line.Text` is split at Unicode whitespace into rune
ranges, and each range becomes a rectangle through `matchRect`. Reusing the
search machinery means the geometry — including right-to-left lines and
sub-fragment boundaries — is the code that `SearchText` already exercises.

```go
type wordToken struct {
    text string    // as it appears in the document
    key  string    // comparison key (lower-cased when IgnoreCase)
    page int
    line int
    rect Rectangle
}
```

Filters act on tokens, deciding by the **midpoint** of the rectangle so a word
never falls into two regions at once: `ExtractionArea` keeps only the words
inside it, `ExcludeAreas*` drop the words inside them, and `ExcludeTables`
runs `NewTableAbsorber().Visit(page)` and drops the words inside detected
tables (lattice and stream alike).

### 2. `comparison_diff.go` — Myers diff and run grouping

Common prefix and suffix are trimmed first, then the greedy O(ND) Myers
algorithm runs over the token keys, keeping the V array per D and walking it
back into an edit script. Memory is O(D²), negligible for related documents.

Two unrelated documents drive D toward N+M, so `maxEditDistance` (2000,
internal) caps the search: the cost grows with the square of the edit
distance in both time and memory (the frontier snapshots — one per round —
bound a roughly maxEditDistance²-int allocation, about 64 MB at this value).
Beyond the cap the comparer emits one `Delete` carrying the whole source text
and one `Insert` carrying the whole destination text. An honest degenerate
answer beats an hours-long search.

When trimming the common prefix/suffix empties one side entirely, the answer
is already known — the other side is one run of deletions or insertions — so
`diffKeys` skips the Myers search altogether in that case rather than paying
its cost (up to the cap) to rediscover a pure insertion or deletion.

`EditOperationsOrder` decides only the order of the adjacent delete and insert
runs of a replacement.

Grouping merges adjacent same-operation edits into one `DiffOperation`, joins
their text with single spaces and groups their rectangles per line; a run is
forced to break when the page changes, so `SourcePage`/`DestPage` stay
meaningful. `Equal` runs carry both sides.

**Page-by-page** pairs page *i* with page *i*; the tail of the longer document
yields one operation per unmatched page carrying that page's whole text.
**Flat** concatenates every page's tokens into one sequence (each token
remembers its page) and diffs once, so text that moved across a page boundary
reads as a move rather than a delete plus an insert.

### 3. `comparison_markup.go` — annotations over the original

`WriteMarkup`/`SaveMarkup` pick the side (destination by default) and take an
independent copy of that document by serializing it to memory and reopening it
with `OpenStream`. The caller's `*Document` is never mutated — it can be
marked up repeatedly with different options. The copy is a new document:
signatures on the input do not survive it. Marking up an **encrypted**
document is not supported: a document configured for encryption re-encrypts
on the copy's `WriteTo`, so `OpenStream` would reject the serialized copy
with a bare `ErrEncrypted`. `copyDocument` detects this up front via
`(*Document).Permissions()`'s second return value and reports an error that
names the operation and wraps `ErrEncrypted`, instead of surfacing the
unexplained rejection.

On the destination side:

| Operation | Annotation | Colour | `/Contents` |
|---|---|---|---|
| `Insert` | `HighlightAnnotation` over `DestRects` | insert | inserted text |
| `Delete` | `CaretAnnotation` at the anchor | delete | deleted text |
| `Equal` | — | — | — |

On the source side the picture mirrors: `Delete` → `StrikeOutAnnotation` over
`SourceRects`, `Insert` → `CaretAnnotation` at the anchor — the classic
redline.

Each rectangle contributes one `QuadPoint`; the annotation `/Rect` is their
union. `/T` is `Title` for every annotation, so Acrobat groups them into one
comment thread and the comments panel reads as a change list.

**Anchoring** an operation that has no rectangles on the chosen side (a
deletion seen on the new document): take the right edge of the last word of
the preceding `Equal` run, else the left edge of the first word of the
following one; the caret is drawn one line high and about half as wide. When
neither neighbour exists — the whole page is gone, so the destination page
does not exist — there is nowhere to anchor: the operation stays in the model
and in the statistics but draws nothing. This is the one case where the markup
is poorer than the data.

`Flatten: true` finishes by calling `Annotations().Flatten()` per page.

Highlight and strike-out annotations have no appearance generator in this
library (viewers synthesize one from `/QuadPoints`), so the markup writer
builds theirs with the shared appearance builder — a Multiply-blended wash for
the highlight, a centre line for the strike-out. Carets generate their own.
With an `/AP` present, the markup renders identically in Acrobat, in our own
renderer and in MuPDF.

### 4. `comparison.go` — API surface

Options validation, the three entry points, `ComparisonResult` and its
accessors, `ComparisonStatistics`, and the two assemble helpers.

## Validation

- **Algorithm** (internal tests over synthetic token sequences): identity,
  pure insert, pure delete, replacement, move, empty side, prefix/suffix
  trimming, the edit-distance cap, and both `EditOperationsOrder` values.
- **Completeness invariant** — the safety line: for any comparison,
  `AssembleSourceText(ops)` equals the first document's text and
  `AssembleDestinationText(ops)` equals the second's. A dropped or duplicated
  token fails immediately, on any input.
- **Geometry against an independent oracle:** build a document, change one
  word, compare, and assert the operation's rectangle matches what
  `SearchText` returns for that word. The two reach the rectangle by different
  paths over a shared base, so the test catches a tokenizer bug instead of
  checking a function against itself. A separate case covers Arabic and Hebrew
  (logical-order extraction landed in `pdf-go-l3qf`).
- **Options:** `ExtractionArea` limits the compared region; `ExcludeAreas`
  drop theirs; `ExcludeTables` over a document whose only edit is inside an
  `AddTable` cell reports no changes; incompatible options return an error.
- **Markup:** reopen the output and assert annotation types, count, colours,
  `QuadPoints` and `/Contents`; assert the caller's document still has zero
  annotations; with `Flatten` assert no annotations remain but the render
  differs from the unmarked page.
- **Corpus** (the decisive run): every document in `external_testdata`
  compared against itself must report `HasChanges() == false` — this catches
  nondeterminism, tokenizer asymmetry and panics in one sweep. Then, on a
  sample, a copy with one `ReplaceText` edit must yield exactly one
  insert/delete pair. Record the wall-clock time in the epic.

## Out of scope (v1, later phases of the epic)

- `GraphicalPdfComparer` / `ImagesDifference` — pixel comparison over the
  built-in rasterizer.
- `SideBySidePdfComparer` — the two-column "before / after" spread.
- `HtmlDiffOutputGenerator` / `JsonDiffOutputGenerator` /
  `MarkdownDiffOutputGenerator` and the Aspose-style generated text report
  (`PdfOutputGenerator`) — serializers over the same model.
- Character-level refinement inside a changed word pair.
- De-hyphenation: a word broken across lines ("compar-" + "ison") is compared
  as two words. Rejoining needs a heuristic that misfires on genuine hyphens.
- A showcase page for comparison.
