# Text object model: TextFragment, TextSegment, TextState, TextBuilder, TextParagraph

Issue: `pdf-go-4yq1`. Status: approved 2026-11-16 (decisions 1 and 2 as recommended).

## Intent

A Go developer who knows Aspose.PDF for .NET should write text code here the
way they write it there: build a `TextFragment` of `TextSegment`s, style it
through a `TextState`, place it with a `Position` or lay it out in a
`TextParagraph`, hand it to a `TextBuilder`; find text with a
`TextFragmentAbsorber` and change it by setting the same properties on the
fragments found. Today the library covers the same ground through other
shapes — `AddText`/`AddTextAt` with a `TextStyle`, `SearchText` returning
`TextMatch`, `ReplaceText` — which work, but are not what such a developer
reaches for, and cannot express per-run styling of found text (change the font
of one word inside a found phrase) or mixed-style lines.

## Goals

1. Author text the Aspose way: `TextFragment` with `TextState` and
   `Segments`, placed by `Position` through `TextBuilder.AppendText`.
2. Lay out paragraphs: `TextParagraph` with a rectangle, lines appended by
   `AppendLine`, word wrapping, horizontal/vertical alignment, justification,
   first-line and subsequent-line indents, per-segment font, size, colour and
   line spacing, drawn by `TextBuilder.AppendParagraph`.
3. Edit found text: `TextFragmentAbsorber` returns fragments whose `Text`,
   `TextState` (font, size, colours, underline, strike-out, character and word
   spacing, horizontal scaling, invisibility) and segments can be changed, and
   whose `Position` can be moved; changes reach the page.
4. Segment state inherits from the fragment: a property a segment leaves unset
   takes the fragment's.
5. Everything round-trips: what is drawn is found again by the absorber with
   the same text, font name, size and colours.

## Non-goals

- Aspose's engine internals (change logs, physical segments, operator
  layout) and its notification/registrar model.
- Re-flowing existing text paragraphs of a page beyond what `ReplaceText`
  (paragraph layout) and `TextParagraphAbsorber` already do.
- Vertical writing, rotated or skewed text in edits (refused, as in
  `ReplaceText`'s shift).
- Discretionary hyphenation dictionaries: `WrapMode` offers no wrap, by
  words, and by words breaking an over-long word; nothing language-aware.

## Anti-goals

- A second, competing text pipeline: drawing goes through the existing
  `AddTextAt`/shaping/font-embedding path, removal through the redaction
  rewriter, layout through the existing wrapping (`lineMeasurer`) and
  styled-run engine (`flow_runs.go`). No new glyph positioning code.
- Breaking existing callers of `ExtractTextWithLayout`, `SearchText`,
  `AddText`, `TextStyle` or `ReplaceText`.

## Design

### Types (all in the root package)

```go
type Position struct{ XIndent, YIndent float64 } // baseline start, PDF user space

type FontStyle int // FontStyleRegular, FontStyleBold, FontStyleItalic (bit flags)

type TextState struct {
    Font              Font
    FontSize          float64
    FontStyle         FontStyle
    ForegroundColor   *Color
    BackgroundColor   *Color
    StrokingColor     *Color
    RenderingMode     TextRenderingMode // fill, stroke, fill+stroke, invisible…
    Underline, StrikeOut, Invisible bool
    CharacterSpacing, WordSpacing float64
    HorizontalScaling float64 // percent; 0 = 100
    LineSpacing       float64 // points between this line's baseline and the next
    Rotation          float64
}
func (s TextState) MeasureString(text string) (float64, error)
func (s *TextState) ApplyChangesFrom(o TextState)

type TextSegment struct {
    Text      string
    TextState TextState
    Hyperlink string // URI; drawn as a link annotation over the segment
    // found segments also carry Position and Rectangle (read)
}
func NewTextSegment(text string) *TextSegment

func NewTextFragment(text string) *TextFragment // one segment holding text

type TextBuilder struct{ … }
func NewTextBuilder(p *Page) *TextBuilder
func (b *TextBuilder) AppendText(f *TextFragment) error
func (b *TextBuilder) AppendParagraph(p *TextParagraph) error

type TextParagraph struct {
    Rectangle             Rectangle
    HorizontalAlignment   HorizontalAlignment // reused from TextParagraphAbsorber
    VerticalAlignment     VAlign
    Justify               bool
    FirstLineIndent, SubsequentLinesIndent float64
    Margin                MarginInfo
    FormattingOptions     TextFormattingOptions // WrapMode: NoWrap, ByWords, ByWordsBreakLong
}
func NewTextParagraph() *TextParagraph
func (p *TextParagraph) AppendLine(f *TextFragment)

type TextFragmentAbsorber struct{ … }
func NewTextFragmentAbsorber(phrase string, opts ...SearchOptions) *TextFragmentAbsorber
func (a *TextFragmentAbsorber) Visit(p *Page) error
func (a *TextFragmentAbsorber) VisitDocument(d *Document) error
func (a *TextFragmentAbsorber) TextFragments() []*TextFragment
```

Names follow Aspose; Go adaptations: errors instead of exceptions, 0-based
slices (`Segments[0]` is Aspose's `Segments[1]`), exported fields where Aspose
has plain properties, constructors as `NewXxx`.

### Decision 1 — one `TextFragment` type (approved)

The name `TextFragment` is already taken by the read-only value
`ExtractTextWithLayout` returns in `TextLine.Fragments` (Text, X, Y, Width,
FontName, FontSize, Height, Bold, Italic, CharSpacing, Color, …). Aspose has a
single `TextFragment` for both found and authored text, so the proposal is to
**grow the existing struct** rather than add a second type:

- keep every existing field with its meaning (no caller breaks);
- add `TextState TextState`, `Segments []*TextSegment`, `Position Position`
  and an unexported binding to the page it was found on or drawn to;
- `ExtractTextWithLayout` keeps returning plain values with the new fields
  empty (no cost added to extraction); the absorber and the builder fill them.

The alternative is a separate type under another name (e.g. `TextItem`), which
keeps the read value clean but makes every Aspose example differ in its most
used type. Recommended: grow the existing struct.

### Decision 2 — changes are applied when the document is saved (approved)

In Aspose, setting `fragment.TextState.Font` changes the page at once. Go
fields cannot run code when set, so a fragment bound to a page (found by the
absorber, or appended by a builder) records what it looked like, and the
document compares and applies the differences:

- on `Save`/`WriteTo`/`WriteIncremental` (and every other writer), before
  anything is written;
- on demand with `(*Document).ApplyTextChanges() error`, for a caller who
  wants to read the page back before saving.

Applying a changed fragment removes its old glyphs by position (the redaction
text rewriter, as `ReplaceText` does) and draws it again from its current
state at its `Position` through the builder path. Unchanged fragments cost
nothing. The alternative is explicit setter methods (`f.SetText`,
`f.SetFont`, …) applied immediately — simpler, but every Aspose snippet
changes shape. Recommended: deferred application.

### Decision 3 — inheritance (my call)

A segment property left at its zero value (nil font, 0 size, nil colour,
unset style) takes the fragment's; the fragment's unset properties take the
defaults (Helvetica 12, black). Booleans (`Underline`, `StrikeOut`,
`Invisible`) can only be switched on by a segment, not off — the one place
the zero value cannot mean "unset". Found fragments read their segments' state
back from the page, so `TextState` of a found fragment is its first segment's.

### Drawing (my call)

- `AppendText`: segments are drawn one after another on one baseline from
  `Position` (no wrapping), each through `AddTextAt` with the segment's
  resolved state mapped to a `TextStyle` (new `TextStyle` fields only where a
  state has no counterpart yet: character/word spacing, horizontal scaling,
  stroking colour and rendering mode). A background colour fills the segment's
  text box; a hyperlink adds a `LinkAnnotation` over it.
- `AppendParagraph`: the paragraph's lines are broken into words across
  segments (the styled-run tokenizer), wrapped to the rectangle less margins
  and indents with the shaped measurer, placed line by line using the tallest
  run's line spacing, aligned and justified as `drawParagraphLine` does for
  replacements, clipped to the rectangle, and drawn run by run.
- A fragment appended without a position and outside a paragraph starts at the
  page's top-left margin, as Aspose does.

### Finding and editing (my call)

- The absorber searches like `SearchText` (same options, regular expressions
  across lines included); each match becomes a fragment whose segments are the
  match's runs of one font, size and colour, with their text boxes.
- Text drawn inside Form XObjects is edited the same way `ReplaceText` edits
  it (the form is cloned for that placement).
- A fragment whose font positions are unknown, or on a rotated baseline, is
  found but refuses edits with an error at application time naming it.
- Setting `Text` on a found fragment replaces the text of all its segments
  with one segment in the first segment's state (Aspose's behaviour);
  changing one segment's state redraws that segment only; moving `Position`
  redraws the fragment there.
- A new font lacking a glyph for the text goes through the same choice as
  `ReplaceText` (the original family by name, else an error naming the
  character).

### Relation to existing API

`AddText`/`AddTextAt`/`TextStyle`, `SearchText`/`TextMatch` and `ReplaceText`
stay as the compact Go-style API and are what the new types are built on. The
docs describe both: the object model for code coming from Aspose and for
mixed styling, the functions for one-call jobs.

## Phases

1. `Position`, `TextState`, `TextSegment`, `TextFragment` fields,
   `TextBuilder.AppendText`, inheritance, `MeasureString`.
2. `TextParagraph` and `TextBuilder.AppendParagraph`.
3. `TextFragmentAbsorber` and editing found fragments (deferred application,
   `ApplyTextChanges`), segments, moving.
4. Hyperlinks on segments, background colours, rendering modes; docs and
   examples.

Each phase ships with its own tests (synthetic fixtures), mutation testing and
a corpus round-trip sweep (draw/edit, reopen, find again).

## Risks

- Deferred application touches every writer; it must run once, before
  `assemble`, and be idempotent (a second save applies nothing).
- Snapshot comparison must not mistake an untouched found fragment for a
  changed one (fonts compared by identity, colours by value).
- Growing `TextFragment` adds pointer fields to a value type used in large
  slices; extraction leaves them nil, so the cost is the struct size only.
