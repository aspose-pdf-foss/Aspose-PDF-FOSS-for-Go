# Auto-tagging existing PDFs — design

bd: `pdf-go-u8ol`. Author: Claude (the user opted out of the design
questions; every decision below is Claude's call unless marked otherwise).

## Intent

`TaggedContent` (tagged.go) builds a structure tree only for content the
caller draws through this library. A PDF made elsewhere — Word export,
Chrome print, a scanner with an OCR layer — has no tags, and
`ValidatePDFUA` / PDF/A "a" levels reject it. This feature adds tags to
such a document after the fact: every piece of painted content ends up
either inside a structure element (heading, paragraph, list, table cell,
figure) or marked as an artifact, and the structure tree carries the
reading order.

It is best-effort accessibility, not a replacement for human remediation.
The bar: a document that was untagged comes out passing `ValidatePDFUA`,
**renders pixel-identically**, **extracts identical text**, and has a
structure tree a screen reader can follow in a sensible order.

## Prior art mirrored

Aspose.PDF for .NET 25.4 added `AutoTaggingSettings`
(`EnableAutoTagging`, `HeadingRecognitionStrategy`, `HeadingLevels`) applied
through `PdfFormatConversionOptions.AutoTaggingSettings` during
`Document.Convert`. `HeadingRecognitionStrategy`: `Outlines`, `Heuristic`,
`Auto` (default — outlines when the document has bookmarks, else
heuristic), `None`.

## API (Claude's call)

```go
type HeadingRecognitionStrategy int
const (
    HeadingRecognitionAuto HeadingRecognitionStrategy = iota // zero value = Aspose's default
    HeadingRecognitionOutlines
    HeadingRecognitionHeuristic
    HeadingRecognitionNone
)

type AutoTaggingSettings struct {
    HeadingRecognitionStrategy HeadingRecognitionStrategy
    // HeadingLevels: font sizes (points), largest first. Text at or above
    // HeadingLevels[i] becomes H(i+1). Empty = the body-size ratio rule the
    // flow exporters already use (headingLevel: 1.7/1.35/1.14 × body).
    HeadingLevels []float64
    Language string // /Catalog/Lang when the document has none; default "en"
}

func DefaultAutoTaggingSettings() AutoTaggingSettings
func (d *Document) AutoTag(s AutoTaggingSettings) (*AutoTagResult, error)

type AutoTagResult struct {
    Pages, Headings, Paragraphs, Lists, Tables, Figures int
}
```

- A standalone `Document` method, matching how this library already exposes
  conversions (`ConvertToPDFA`, `Sanitize`, `Optimize`) as methods rather than
  an options-object `Convert`. `EnableAutoTagging` has no meaning outside a
  conversion options object, so it is not mirrored. Wiring auto-tagging into
  `ConvertToPDFA` (Aspose's entry point) is a follow-up; until then the
  documented recipe is `AutoTag` then `ConvertToPDFA(PDFA2A)`.
- Mirrors the intent of `HeadingLevels`; Aspose's own type for it is not
  documented publicly, so the `[]float64` shape is ours.

## Refusals

- A document that already has a `/StructTreeRoot` is refused with an error
  (`ErrAlreadyTagged`): merging into an existing tree is a different
  feature. Callers who want to re-tag can remove the tree themselves.
- Parse failure of any page's content: error, and nothing is changed. All
  pages are rewritten in memory first and committed together, so the call
  is atomic.
- A signed document is not refused, but the doc comment says tagging
  rewrites page content and so invalidates existing signatures.

## How it works

### 1. Analysis — reuse the flow core

`buildFlowDoc` (flow_doc.go) with `dropFurniture`, `dropRotated` and
`detectTables` on gives, per page, the reading-order blocks (paragraphs and
detected tables) with repeating headers/footers and rotated watermarks
already removed. Each paragraph is split with `segmentParagraph` into
segments; a segment becomes:

- a **heading** H1–H6 when the strategy says so (below);
- a **list item** when it opens with a list marker (`docSeg.marker`):
  consecutive item segments of one paragraph group into one `/L`, each item
  an `/LI` with `/Lbl` (the marker glyphs) and `/LBody` (the rest). Flat
  lists in v1 — nesting by indent is a follow-up;
- otherwise a **paragraph** `/P` (monospace code blocks included).

A detected table becomes `/Table → /TR → /TH|/TD` from `AbsorbedTable`
rows/cells; the first row is `/TH` when the table has more than one row;
`RowSpan`/`ColSpan` > 1 are written as `/A << /O /Table /RowSpan n /ColSpan m >>`;
covered cells produce no element.

Images (see step 2) become `/Figure` elements placed into the reading order
by their top edge, like `insertFlowImage` does. No `/Alt` is invented:
`ValidatePDFUA` then reports `UA_FIGURE_NO_ALT`, which
`ai.ImageDescriptionCopilot.FillAltTexts` fixes — the auto-tagger brackets
each image `Do` in its own MCID, exactly what `FiguresNeedingAltText` needs
to find it.

Each element gets **regions**: page-space rectangles covering its text
(one rect per fragment, the same text box `SearchText` uses:
`Y+descent … Y+Height`, padded 0.5pt). A table cell's region is the cell
rect. A list label's region is the marker's own glyphs (via the fragment's
per-rune `runeX`).

### Heading strategy

- `Outlines`: an outline item whose destination page is this page and whose
  title equals a segment's text (whitespace-collapsed, case-insensitive)
  makes that segment a heading at the outline depth + 1 (capped at 6).
- `Heuristic`: `HeadingLevels` if given, else `headingLevel(seg.size, bodySize, len)`.
- `Auto`: `Outlines` when the document has any outline items, else
  `Heuristic`. `None`: no headings.

### 2. Content rewrite — label every painting operator

A new walker over each page's top-level content ops, modeled on
redact_apply_text.go's state tracking (CTM, q/Q, BT/ET, text matrix, font,
Tc/Tw/Tz/TL/Ts) and reusing its glyph geometry helpers (`glyphAdvance`,
`glyphAdvanceCID`, CID decoding through the font's CMap, `matMul`), assigns
every painting operator an owner:

- **Text show** (`Tj`, `TJ`, `'`, `"`): each glyph's centre point on the
  baseline (the same point redaction tests) is looked up in the element
  regions; the first element in reading order whose region contains it owns
  the glyph, none → **artifact**. When one operator's glyphs belong to more
  than one owner, the operator is **split** at the owner boundaries into
  several operators of the same kind. This is exact: a show operator
  advances the text matrix by exactly the glyphs it shows, so `(AB) Tj (CD) Tj`
  draws what `(ABCD) Tj` draws; a `TJ` array is split between elements and
  each kerning number stays with the glyph before it; `'`/`"` keep their
  line-move on the first piece and the rest become `Tj`. Multi-byte fonts are
  split on code boundaries.
  A font with no usable widths (`fontInfo.known == false`): the whole
  operator goes to the owner of its start point — unlike redaction,
  approximate placement is a tagging-quality issue, not a leak.
- **Image** (`Do` of an Image XObject, `BI` inline image): `/Figure`, unless
  tiny (either side < 4pt → artifact) or a full-page background under text
  (≥ 70% of the page area on a page that has text — a scan under its OCR
  layer → artifact, so the text isn't announced twice).
- **Form XObject** `Do`: the element whose regions overlap the form's
  placement bbox the most owns the whole `Do` (v1 does not recurse into the
  form); no overlap → `/Figure` when the form draws an image, else artifact.
- **Paths, shadings** (`f`, `S`, `B`, …, `sh`): artifact. Clip-only paths
  (`W n`) and state operators paint nothing and stay unmarked.
- Content already inside an existing `/Artifact` marked-content section is
  left as it is.

### 3. Emitting marked content

Consecutive operators with the same owner share one marked-content section:
`/P <</MCID n>> BDC … EMC` for an element (the BDC tag is the element's
structure type), `/Artifact BMC … EMC` for artifacts. A section is closed
before any of: `BT`, `ET`, `q`, `Q`, an existing `BDC`/`BMC`/`EMC`, or an
owner change. That keeps every new section correctly nested with text
objects, graphics-state saves and the document's existing marked content
(optional content `/OC`, `/Span` ActualText) no matter how the producer
arranged them. State operators between two operators of the same owner are
carried inside the open section.

MCIDs per page start above any MCID already present in the stream.

### 4. Structure tree

Built through `TaggedContent`'s bookkeeping (root `/Document`, `/ParentTree`,
page `/StructParents`, `/MarkInfo`, `/ViewerPreferences /DisplayDocTitle`).
A new internal helper registers a leaf with a list of MCIDs on one page
(`/K` = the MCID or an array of them). An element whose content spans pages
is split into one element per page (as the Flow layout already does).
`/Lang` set from settings when absent; `/Info` title kept, and when missing
the first H1 (or first heading) is used as the title.

## Out of scope (v1, documented)

- Annotations: no `/Link` + `/OBJR`, no widget `/Form` elements.
- Nested lists; table header columns (`/TH` only on the first row);
  `/Caption`, `/TOC`, `/Note`.
- Recursing into Form XObjects (a form is one unit).
- Vector charts as `/Figure` (all vector paths are artifacts).
- Integration into `ConvertToPDFA` (Aspose's entry point) — follow-up.

## Verification

- Unit/integration tests on documents built with this library's own drawing
  API (untagged by default): headings, paragraphs, a list, a table with a
  span, an image, a header/footer repeated across pages, a two-column page.
  For each: `ValidatePDFUA` conformant (apart from `UA_FIGURE_NO_ALT` where
  a figure has no alt); structure tree types and order as expected; text
  extraction byte-identical before/after; rendering pixel-identical
  before/after; survives Save + Open.
- Operator splitting tested directly (single-byte, Type0 via Identity-H,
  `TJ` with kerning, `'`/`"`), each rendered before/after and compared.
- Refusals: already-tagged document, unparseable content (atomicity).
- Mutation tests on the splitting, the barrier set and the owner lookup.
- Corpus sweep over `external_testdata`: no panics; for every document that
  tags, extraction identical and render identical on a sample; count of
  `ValidatePDFUA`-conformant results reported. No veraPDF/PAC locally.
