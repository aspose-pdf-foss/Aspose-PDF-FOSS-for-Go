# Text Object Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aspose-style text authoring and editing — `TextState`, `TextSegment`, `TextFragment` (grown), `Position`, `TextBuilder`, `TextParagraph`, `TextFragmentAbsorber` — with edits to bound fragments applied when the document is written.

**Architecture:** A thin object layer over existing machinery. Drawing goes through `AddTextAt` (`text_addtextat.go`) and the styled-run layout (`flow_runs.go`: `textRun`, `layoutRuns`); finding goes through `SearchText`'s line search (`text_search.go`); removing goes through `stripTextInRegions` (`replace.go`) + `replacePageContents` + `dropUnreachable`. A document keeps the fragments bound to its pages and, before any PDF writer runs, redraws those whose state changed.

**Tech Stack:** Go, standard library only (root package `asposepdf`).

**Spec:** `docs/superpowers/specs/2026-11-16-text-object-model-design.md`

## Global Constraints

- Pure Go, no external dependencies; every new `.go` file starts with `// SPDX-License-Identifier: MIT`, a blank line, then `package`.
- Names mirror Aspose.PDF for .NET (`TextState.ForegroundColor`, `Position.XIndent`, `TextBuilder.AppendText`, …); Go adaptations only: errors, 0-based slices, `NewXxx` constructors.
- `TextFragment` keeps every existing field and meaning; `ExtractTextWithLayout` leaves the new fields empty.
- Edits to bound fragments apply on `WriteTo`/`Save`, `WriteToLinearized`/`SaveLinearized`, `WriteIncremental`/`SaveIncremental`, and `(*Document).ApplyTextChanges() error`; applying is idempotent.
- Defaults: Helvetica, 12pt, black; `HorizontalScaling` 0 means 100.
- Segment inheritance: a zero-valued segment property takes the fragment's; booleans only switch on.
- Public tests use synthetic fixtures built in the test; `go vet ./...`, `go test ./...`, `golangci-lint run ./...` pass after every task; mutation-test each task's branches.
- README, CLAUDE.md (Public API section) and CHANGELOG (`## [Unreleased]`) updated in the task that adds the API.

## Review Focus

1. A found fragment the caller does not touch: saving must leave the page byte-identical (no spurious redraw) — test in Task 6.
2. Saving twice: the second save applies nothing and the output is stable — test in Task 6.
3. New text not covered by the found fragment's embedded subset font: falls back like `ReplaceText` (family by name, else an error naming the character, page untouched) — test in Task 7.
4. A right-to-left found fragment edited: redrawn anchored at its right edge as `ReplaceText` does — test in Task 7.
5. `Text` set to `""` on a found fragment: the text is removed and nothing drawn — test in Task 7.

---

### Task 1: Position, FontStyle, TextRenderingMode, TextState

**Files:**
- Create: `text_state.go`
- Test: `text_state_test.go`

**Interfaces:**
- Produces:
  - `type Position struct{ XIndent, YIndent float64 }`
  - `type FontStyle int`; `FontStyleRegular = 0`, `FontStyleBold = 1`, `FontStyleItalic = 2`
  - `type TextRenderingMode int`; `RenderingFill`, `RenderingStroke`, `RenderingFillStroke`, `RenderingInvisible` (PDF `Tr` 0–3)
  - `type TextState struct { Font Font; FontSize float64; FontStyle FontStyle; ForegroundColor, BackgroundColor, StrokingColor *Color; RenderingMode TextRenderingMode; Underline, StrikeOut, Invisible bool; CharacterSpacing, WordSpacing, HorizontalScaling, LineSpacing, Rotation float64 }`
  - `func (s TextState) inherit(parent TextState) TextState` — zero fields take the parent's (bools OR)
  - `func (s TextState) resolved() TextState` — `inherit` of the defaults
  - `func (s TextState) MeasureString(text string) (float64, error)` — via `MeasureText` with the state's style
  - `func (s *TextState) ApplyChangesFrom(o TextState)` — copies o's non-zero fields
  - `func (s TextState) style() TextStyle` — maps to `TextStyle` (FontStyle selects the Standard-14 bold/italic face for a Standard-14 font: `FindFont` on the composed name)

- [ ] **Step 1: Write failing tests** `TestTextStateInherit` (segment `{FontSize: 0, ForegroundColor: &blue}` over fragment `{Font: FontTimesRoman, FontSize: 5}` → `{FontTimesRoman, 5, blue}`; bool `Underline` true in parent stays true), `TestTextStateResolvedDefaults` (zero → Helvetica 12 black, `HorizontalScaling` reads 100 after `resolved`), `TestTextStateMeasureString` (equals `MeasureText` for the same style; `HorizontalScaling: 50` halves it; `CharacterSpacing: 1` adds 1 per rune), `TestTextStateFontStyle` (`FontTimesRoman` + `FontStyleBold|FontStyleItalic` → `FontTimesBoldItalic`).
- [ ] **Step 2:** `go test -run TestTextState .` → FAIL (undefined).
- [ ] **Step 3:** Implement in `text_state.go`. `MeasureString` adds `CharacterSpacing`·runes and `WordSpacing`·spaces after `HorizontalScaling` scales the width (PDF §9.3: Tc/Tw scaled by Tz too — apply Tz to the whole sum).
- [ ] **Step 4:** `go test -run TestTextState .` → PASS.
- [ ] **Step 5:** Commit `feat: TextState, Position and text style enums`.

### Task 2: Character/word spacing, horizontal scaling, stroke and rendering mode in AddTextAt

**Files:**
- Modify: `color.go` (`TextStyle`), `text_addtextat.go` (`AddTextAt`, `MeasureText`, `resolveSingleLine`)
- Test: `text_addtextat_spacing_test.go`

**Interfaces:**
- Consumes: Task 1 `TextState.style()` will set these.
- Produces: `TextStyle` fields `CharSpacing, WordSpacing, HorizontalScaling float64` (0 = 100), `StrokeColor *Color`, `RenderMode TextRenderingMode`; honoured by `AddTextAt` and `MeasureText` only (`AddText` ignores them, documented).

- [ ] **Step 1: Failing tests** `TestAddTextAtSpacing`: draw "a b c" at `CharSpacing: 2, WordSpacing: 5, HorizontalScaling: 80`; reopen; `ExtractTextWithLayout` fragment `CharSpacing == 2`; the text's right edge (SearchText rect) equals `x + MeasureText(...)` within 0.1; `TestAddTextAtRenderMode`: `RenderMode: RenderingStroke, StrokeColor: &red` → content stream contains `1 Tr` and `1 0 0 RG`; `RenderingInvisible` equals `Invisible: true` output.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Emit `Tc`/`Tw`/`Tz`/`Tr`/stroke colour inside the text object `AddTextAt` builds; `Tw` reaches only single-byte space codes, so for an embedded (two-byte) font add the word spacing as `TJ` numbers before glyphs following a space (the shaped `showOps` already emits `TJ` arrays — add the adjustment there). `MeasureText` uses `TextState.MeasureString`'s formula.
- [ ] **Step 4:** run → PASS; full `go test ./...` unchanged elsewhere.
- [ ] **Step 5:** Commit `feat: AddTextAt character/word spacing, scaling and rendering mode`.

### Task 3: TextSegment, grown TextFragment, TextBuilder.AppendText

**Files:**
- Modify: `text_layout.go` (`TextFragment` fields)
- Create: `text_builder.go`
- Test: `text_builder_test.go`

**Interfaces:**
- Consumes: Task 1, Task 2.
- Produces:
  - `type TextSegment struct { Text string; TextState TextState; Hyperlink string; Position Position; Rectangle Rectangle }` (Position/Rectangle filled for drawn and found segments)
  - `func NewTextSegment(text string) *TextSegment`
  - new `TextFragment` fields: `TextState TextState`, `Segments []*TextSegment`, `Position Position`, `bind *fragmentBinding` (unexported)
  - `func NewTextFragment(text string) *TextFragment` — one segment holding text, `Text` = text
  - `func (f *TextFragment) segmentsToDraw() []*TextSegment` — `Segments` if any, else one segment from `Text`; each with `TextState.inherit(f.TextState)`
  - `type TextBuilder struct{ page *Page }`; `func NewTextBuilder(p *Page) *TextBuilder`; `func (b *TextBuilder) AppendText(f *TextFragment) error`
  - `type fragmentBinding struct { page *Page; drawn []QuadPoint; snap fragmentSnapshot }` and `func snapshotOf(f *TextFragment) fragmentSnapshot` (text, position, each segment's text + resolved state; colours by value, fonts by identity)
  - `func (d *Document) bindFragment(f *TextFragment)` — appends to `d.boundFragments []*TextFragment`

- [ ] **Step 1: Failing tests** `TestAppendTextSegments` (fragment "segment1" at (100,600), state `{FontTimesRoman, 5, gray background, red}`, segments "segment2" `{blue}` and "segment3" `{FontCourier, 12}`; reopen; SearchText each: segment2 found right after segment1 on the same baseline, its fragment `FontName` Times, size 5; segment3 Courier 12; background: `SearchForTextRelatedGraphics` gives gray for segment1), `TestAppendTextDefaultPosition` (no position → baseline at the CropBox's top-left less 72pt margins), `TestAppendTextHyperlink` (segment `Hyperlink: "https://example.com"` → page has one `LinkAnnotation` whose rect covers the segment), `TestNewTextFragmentText` (`NewTextFragment("x").Segments[0].Text == "x"`).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** `AppendText` draws each segment with `AddTextAt` at the running x (advance by `MeasureString`), fills a background rectangle first when set, adds a link annotation for `Hyperlink`, records segment `Position`/`Rectangle`, sets `f.Text` to the concatenation, then binds the fragment (snapshot after drawing). Default position: `CropBox.LLX+72`, `CropBox.URY-72-ascent`.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5:** Commit `feat: TextSegment, TextFragment state and TextBuilder.AppendText`.

### Task 4: TextParagraph and AppendParagraph

**Files:**
- Create: `text_paragraph.go`
- Test: `text_paragraph_test.go`

**Interfaces:**
- Consumes: Task 3 `segmentsToDraw`, `TextState.style`; `flow_runs.go` `textRun`, `metricsFor`, `layoutRuns`; `HorizontalAlignment` (`text_paragraph_absorber.go`).
- Produces:
  - `type WordWrapMode int`: `WrapNoWrap`, `WrapByWords` (default zero is `WrapByWords`), `WrapByWordsBreakLong`
  - `type TextFormattingOptions struct { WrapMode WordWrapMode; FirstLineIndent, SubsequentLinesIndent float64 }`
  - `type TextParagraph struct { Rectangle Rectangle; HorizontalAlignment HorizontalAlignment; VerticalAlignment VAlign; Justify bool; Margin MarginInfo; FormattingOptions TextFormattingOptions; lines []*TextFragment }`
  - `func NewTextParagraph() *TextParagraph`; `func (p *TextParagraph) AppendLine(f *TextFragment)`
  - `func (b *TextBuilder) AppendParagraph(p *TextParagraph) error`

- [ ] **Step 1: Failing tests** `TestAppendParagraphWrap` (rect 100..200 × 300..700, one line of 20 words at Helvetica 12 → several lines, every line's right edge ≤ 200, first baseline = 700 − ascent), `TestAppendParagraphMixedRuns` (line of "the"(Arial 20) + "text is justified 1 2 3"(12) → one wrapped paragraph, words found in order), `TestAppendParagraphJustify` (all lines but the last span 100..200 within 0.5), `TestAppendParagraphAlign` (Right: lines end at 200; Center), `TestAppendParagraphVertical` (VAlign Bottom: last baseline − descent ≥ 300 and within one line of it), `TestAppendParagraphIndents` (FirstLineIndent 20 → first line starts at 120, SubsequentLinesIndent 10 → others at 110), `TestAppendParagraphNoWrap` (one line, clipped at 200: SearchText of words past 200 empty), `TestAppendParagraphLineSpacing` (segment `LineSpacing: 30` → baselines 30 apart).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Convert each line's segments to `textRun`s (style from resolved state), `layoutRuns` to the rect width less margins and indents (first line narrower by the first-line indent), place lines from the top (or bottom/middle per VAlign) using the largest `LineSpacing` (else 1.2·size) of each line, draw runs with `AddTextAt`; justify by spreading word gaps as `drawParagraphLine` does; clip by wrapping the drawing in `q re W n … Q` (`appendToContentStream`). Bind each line fragment (Task 3 binding).
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5:** Commit `feat: TextParagraph and TextBuilder.AppendParagraph`.

### Task 5: TextFragmentAbsorber

**Files:**
- Create: `text_fragment_absorber.go`
- Test: `text_fragment_absorber_test.go`

**Interfaces:**
- Consumes: `compileSearch`, `searchLines`/`findAcrossLines`, `buildLineRuneMap`, `matchRect` (`text_search.go`); `standard14ForFragment` (`replace.go`).
- Produces:
  - `type TextFragmentAbsorber struct { phrase string; opts SearchOptions; frags []*TextFragment }`
  - `func NewTextFragmentAbsorber(phrase string, opts ...SearchOptions) *TextFragmentAbsorber` (empty phrase = every line's text, Aspose's no-argument absorber)
  - `func (a *TextFragmentAbsorber) Visit(p *Page) error`, `VisitDocument(d *Document) error`, `TextFragments() []*TextFragment`
  - `type documentFont struct{ name string; embedded bool }` implementing `Font` (`BaseFont`, `IsEmbedded`) — a found font; usable as `TextState.Font` for reading; drawing with it goes through Task 7's fallback.

- [ ] **Step 1: Failing tests** `TestAbsorberSegments` (draw "plain " Helvetica + "bold" Helvetica-Bold + " tail" Times on one line; absorber "plain bold tail" → one fragment, 3 segments, `Segments[1].TextState.Font.BaseFont()=="Helvetica-Bold"`, fragment `TextState` = first segment's, `Position` = baseline start, existing fields `X/Y/FontName` filled too), `TestAbsorberAllText` (empty phrase → one fragment per line), `TestAbsorberRegexAcrossLines` (regex option → fragment whose segments span two lines), `TestAbsorberColors` (`ForegroundColor` read back; `BackgroundColor` filled when `SearchForTextRelatedGraphics`).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** For each match, walk its runes over the line's fragments (rune map owner) and group consecutive runes of one fragment style (font name, size, colour, bold, italic) into segments with their text boxes (`matchRect`); bind every fragment (snapshot); `Document` for `VisitDocument`.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5:** Commit `feat: TextFragmentAbsorber`.

### Task 6: Deferred application and ApplyTextChanges

**Files:**
- Create: `text_apply.go`
- Modify: `document.go` (`WriteTo`), `linearize.go` (`WriteToLinearized`), `incremental_save.go` (`buildIncrementalPDF`), `document.go` (`boundFragments` field)
- Test: `text_apply_test.go`

**Interfaces:**
- Consumes: Task 3 binding and `snapshotOf`; `stripTextInRegions`, `replacePageContents`, `contentStreamIDs`, `dropUnreachable`.
- Produces:
  - `func (d *Document) ApplyTextChanges() error` — public; applies every changed bound fragment, refreshes snapshots
  - `func (d *Document) applyFragment(f *TextFragment) error` — remove old segment boxes, redraw via the builder path at `Position` (whole fragment when `Text` or `Position` changed or segments were added/removed; else only changed segments)
  - every PDF writer calls `d.ApplyTextChanges()` first and returns its error

- [ ] **Step 1: Failing tests** `TestApplyUntouchedIsNoop` (absorb, touch nothing, WriteTo bytes == WriteTo bytes of a document never absorbed), `TestApplyTwice` (edit, `ApplyTextChanges`, then WriteTo twice → equal bytes), `TestApplyText` (found "Figure" → `Text = "Picture"`, `TextState.FontSize = 8`, blue/green → reopened: "Picture" at the old baseline, size 8, no "Figure"), `TestApplyAfterAppend` (AppendText then add a segment, save → both segments found, Helvetica), `TestApplyMove` (`Position` changed → found at the new place, gone from the old), `TestApplyOnIncremental` (edit then `WriteIncremental` → reopened shows the edit).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement; drawn regions come from the binding (`drawn` boxes for appended fragments, segment rectangles for found ones).
- [ ] **Step 4:** run → PASS; `go test ./...` green.
- [ ] **Step 5:** Commit `feat: apply text fragment changes on save`.

### Task 7: Editing segments, fonts and the hard cases

**Files:**
- Modify: `text_apply.go`
- Test: `text_apply_edit_test.go`

**Interfaces:**
- Consumes: Task 6; `replaceFonts.fontFor` (`replace.go`) for fallback; `bidiBaseLevel`.
- Produces: no new API; error texts name the fragment (`text "…" on page N: …`).

- [ ] **Step 1: Failing tests** `TestApplySegmentFont` (change `Segments[1].TextState.Font` to Courier in a 3-segment fragment → only that segment redrawn: the other segments' content bytes untouched, the changed one found in Courier at its old start), `TestApplyFontFallback` (found text in an embedded subset font, new `Text` with a character the subset lacks → drawn in the original family found by name, or an error naming the character and the page unchanged), `TestApplyRTL` (Hebrew fragment edited → right edge kept), `TestApplyEmptyText` (`Text = ""` → removed, nothing drawn), `TestApplyRotatedRefused` (fragment on a 45° baseline edited → `ApplyTextChanges` error naming it, page unchanged), `TestApplyFormXObject` (text drawn in a form placed on two pages, edited on one → the other keeps the original).
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** Implement: plan every change first, refuse before touching any page (rotated, unknown positions via `fontInfo.positioned`, no font covering the text), then apply.
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5:** Commit `feat: segment edits, font fallback and refusals for text fragments`.

### Task 8: Documentation, examples, corpus sweep

**Files:**
- Modify: `README.md`, `CLAUDE.md`, `CHANGELOG.md`, `examples_test.go`

- [ ] **Step 1:** Add `ExampleTextBuilder_AppendText` and `ExampleTextFragmentAbsorber` with `// Output:` comments (found text printed after an edit and reopen).
- [ ] **Step 2:** Document the API in CLAUDE.md's Public API (new bullet group "Text object model"), README feature list, CHANGELOG Added.
- [ ] **Step 3:** Corpus round trip (debug tool, not committed): for each PDF, absorb every line, change nothing → identical save; change the first fragment's text → reopened finds it and other pages' text is unchanged. Record counts in CLAUDE.md.
- [ ] **Step 4:** `go vet ./...`, `go test ./...`, `golangci-lint run ./...` → clean.
- [ ] **Step 5:** Commit `docs: text object model`; close `pdf-go-4yq1`.
