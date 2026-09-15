# PDF Comparison (phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compare two PDF documents word by word and report the differences with page numbers and rectangles, then write a marked-up copy of the original in which every insertion and deletion is a real annotation.

**Architecture:** A page is tokenized into words with rectangles by reusing the search machinery (`buildLineRuneMap` + `matchRect`), the word sequences are diffed with Myers, adjacent same-operation words are grouped into runs that break at line and page boundaries, and a markup writer turns those runs into Highlight / StrikeOut / Caret annotations on an independent copy of the document.

**Tech Stack:** Go 1.24 (toolchain 1.26), standard library only, package `asposepdf` (root of the repo). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-15-pdf-comparison-design.md`

## Global Constraints

- Every new `.go` file starts with `// SPDX-License-Identifier: MIT` on its own line, then a blank line, then `package asposepdf`.
- Pure Go, standard library only. Adding a module dependency is a plan failure.
- Public API mirrors Aspose.PDF for .NET naming where a counterpart exists (`ComparePages`, `CompareDocumentsPageByPage`, `CompareFlatDocuments`, `DiffOperation`, `Operation`, `ComparisonOptions`, `AssembleSourceText`, `AssembleDestinationText`).
- Options structs follow the `SearchOptions` idiom: zero value usable, passed variadically, last one wins.
- `go build ./...` and `go test ./...` must pass at the end of every task.
- Do not add entries to `testdata/testfiles.json` and do not add PDF files to `testdata/`. Every test in this plan builds its own document. If a task seems to need a real-world PDF, stop and ask the user which file to use.
- `result_files/` and `reports/` are gitignored; nothing in them is ever committed.
- Commit messages end with the line `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` after a blank line.
- Do not push and do not tag. The user does that.

## File Structure

| File | Responsibility |
|---|---|
| `comparison_tokens.go` (create) | Page → `[]wordToken` with rectangles; area and table filters |
| `comparison_diff.go` (create) | Myers diff over token keys; grouping edits into `DiffOperation` runs |
| `comparison.go` (create) | Public model, options, entry points, `ComparisonResult`, statistics |
| `comparison_markup.go` (create) | Marked-up copy: annotations + their appearance streams |
| `comparison_tokens_internal_test.go` (create) | Tokenizer and filter tests (package `asposepdf`) |
| `comparison_diff_internal_test.go` (create) | Diff algorithm and grouping tests (package `asposepdf`) |
| `comparison_test.go` (create) | Public behaviour tests (package `asposepdf_test`) |
| `CLAUDE.md`, `README.md`, `CHANGELOG.md` (modify) | Documentation, in the last task |
| `docs/superpowers/specs/2026-09-15-pdf-comparison-design.md` (modify) | One accuracy fix, in Task 7 |

Existing code this plan builds on, all in package `asposepdf`:

- `buildLineRuneMap(line *TextLine) lineRuneMap` — `text_search.go:190`. Returns `lineRuneMap{text []byte, runeByte []int, owner []int, local []int, runeCounts []int}`, already reordered into logical order for right-to-left lines.
- `matchRect(frags []TextFragment, owner, local, runeCounts []int, r0, r1 int) (Rectangle, bool)` — `text_search.go:297`. Turns the rune span `[r0, r1)` into a page rectangle.
- `(*Page).ExtractTextWithLayout() ([]TextLine, error)`, `(*Page).Number() int`, `(*Document).Page(n int) (*Page, error)`, `(*Document).PageCount() int`, `(*Document).WriteTo(w io.Writer) (int64, error)`, `OpenStream(r io.Reader) (*Document, error)`.
- `NewTableAbsorber() *TableAbsorber`, `(*TableAbsorber).Visit(p *Page) error`, `(*TableAbsorber).TableList() []*AbsorbedTable`, `AbsorbedTable.Rect Rectangle`.
- `NewHighlightAnnotation(page *Page, rect Rectangle) *HighlightAnnotation`, `NewStrikeOutAnnotation(page *Page, rect Rectangle) *StrikeOutAnnotation`, `NewCaretAnnotation(page *Page, rect Rectangle) *CaretAnnotation`, `(*HighlightAnnotation).SetQuadPoints(qp []QuadPoint)`, `(*StrikeOutAnnotation).SetQuadPoints(qp []QuadPoint)`, `SetColor(*Color)`, `SetTitle(string)`, `SetContents(string)`, `(*Page).Annotations() *AnnotationCollection`, `(*AnnotationCollection).Add(a Annotation) error`, `(*AnnotationCollection).Flatten() error`.
- `newAppearanceBuilder() *appearanceBuilder` with `SetFillColorRGB`, `SetStrokeColorRGB`, `SetLineWidth`, `Rect`, `MoveTo`, `LineTo`, `Fill`, `Stroke`, `Bytes`; `makeFormXObjectWithResources(content []byte, bbox Rectangle, resources pdfDict) *pdfStream`; `setAppearanceN(base *annotationBase, s *pdfStream)`.

---

### Task 1: Word tokens with rectangles

**Files:**
- Create: `comparison_tokens.go`
- Test: `comparison_tokens_internal_test.go`

**Interfaces:**
- Consumes: `buildLineRuneMap`, `matchRect`, `(*Page).ExtractTextWithLayout`, `(*Page).Number` (all existing).
- Produces:
  - `type wordToken struct { text, key string; page, line int; rect Rectangle }`
  - `func pageWordTokens(p *Page, ignoreCase bool) ([]wordToken, error)`
  - `func lineWordTokens(lines []TextLine, pageNum int, ignoreCase bool) []wordToken`
  - `func wordSpans(s string) [][2]int`
  - `func tokenKey(text string, ignoreCase bool) string`

- [ ] **Step 1: Write the failing test**

Create `comparison_tokens_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// drawAndReopen writes one line of Standard-14 text and reopens the saved
// document, so the tokenizer is fed what a reader of the finished file sees.
func drawAndReopen(t *testing.T, text string) *Document {
	t.Helper()
	doc := NewDocument(400, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	style := TextStyle{Font: FontHelvetica, Size: 14}
	if err := page.AddText(text, style, Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 160}); err != nil {
		t.Fatalf("add text: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

func TestWordSpans(t *testing.T) {
	got := wordSpans("  alpha beta\tgamma ")
	want := [][2]int{{2, 7}, {8, 12}, {13, 18}}
	if len(got) != len(want) {
		t.Fatalf("got %d spans %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPageWordTokens(t *testing.T) {
	doc := drawAndReopen(t, "alpha beta gamma")
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := pageWordTokens(page, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(tokens), tokens)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if tokens[i].text != want {
			t.Errorf("token %d = %q, want %q", i, tokens[i].text, want)
		}
		if tokens[i].page != 1 {
			t.Errorf("token %d page = %d, want 1", i, tokens[i].page)
		}
		if tokens[i].rect.URX <= tokens[i].rect.LLX || tokens[i].rect.URY <= tokens[i].rect.LLY {
			t.Errorf("token %d has an empty rect: %+v", i, tokens[i].rect)
		}
	}
	// Words are laid out left to right, so their rectangles must not overlap
	// and must advance.
	if tokens[0].rect.URX > tokens[1].rect.LLX+0.5 {
		t.Errorf("token 0 (%.2f..%.2f) overlaps token 1 (%.2f..%.2f)",
			tokens[0].rect.LLX, tokens[0].rect.URX, tokens[1].rect.LLX, tokens[1].rect.URX)
	}
}

func TestTokenKeyIgnoreCase(t *testing.T) {
	if got := tokenKey("Alpha", false); got != "Alpha" {
		t.Errorf("case-sensitive key = %q, want %q", got, "Alpha")
	}
	if got := tokenKey("Alpha", true); got != "alpha" {
		t.Errorf("case-insensitive key = %q, want %q", got, "alpha")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestWordSpans|TestPageWordTokens|TestTokenKeyIgnoreCase" ./...`
Expected: FAIL — `undefined: wordSpans`, `undefined: pageWordTokens`, `undefined: tokenKey`.

- [ ] **Step 3: Write the implementation**

Create `comparison_tokens.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strings"
	"unicode"
)

// wordToken is one word of a page together with the rectangle its glyphs
// occupy. Comparison works on these: the key drives matching, the rectangle
// drives the markup.
type wordToken struct {
	text string    // the word as it appears in the document
	key  string    // the comparison key (lower-cased when IgnoreCase is set)
	page int       // 1-based page number
	line int       // index of the layout line the word came from
	rect Rectangle // page-space bounding box
}

// pageWordTokens extracts the page's words in reading order.
func pageWordTokens(p *Page, ignoreCase bool) ([]wordToken, error) {
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return nil, err
	}
	return lineWordTokens(lines, p.Number(), ignoreCase), nil
}

// lineWordTokens splits each line's text at whitespace and maps every word
// back to a rectangle through the same rune map SearchText uses, so
// right-to-left lines and sub-fragment boundaries are handled identically.
func lineWordTokens(lines []TextLine, pageNum int, ignoreCase bool) []wordToken {
	var out []wordToken
	for li := range lines {
		line := &lines[li]
		if len(line.Fragments) == 0 {
			continue
		}
		m := buildLineRuneMap(line)
		if len(m.owner) == 0 {
			continue
		}
		for _, span := range wordSpans(string(m.text)) {
			r0 := sort.SearchInts(m.runeByte, span[0])
			r1 := sort.SearchInts(m.runeByte, span[1])
			rect, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, r0, r1)
			if !ok {
				continue
			}
			text := string(m.text[span[0]:span[1]])
			out = append(out, wordToken{
				text: text,
				key:  tokenKey(text, ignoreCase),
				page: pageNum,
				line: li,
				rect: rect,
			})
		}
	}
	return out
}

// wordSpans returns the [start, end) byte spans of the whitespace-separated
// words of s.
func wordSpans(s string) [][2]int {
	var spans [][2]int
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				spans = append(spans, [2]int{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		spans = append(spans, [2]int{start, len(s)})
	}
	return spans
}

// tokenKey is the string two words are matched on.
func tokenKey(text string, ignoreCase bool) string {
	if ignoreCase {
		return strings.ToLower(text)
	}
	return text
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run "TestWordSpans|TestPageWordTokens|TestTokenKeyIgnoreCase" ./...`
Expected: PASS.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS (nothing existing is touched).

- [ ] **Step 6: Commit**

```bash
git add comparison_tokens.go comparison_tokens_internal_test.go
git commit -m "$(cat <<'EOF'
feat: word tokens with rectangles for document comparison (pdf-go-175w)

A page's extracted lines become words carrying their page-space rectangle,
reusing the rune map and rectangle builder that SearchText already exercises
— so right-to-left lines and sub-fragment boundaries come for free.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Area and table filters

**Files:**
- Modify: `comparison_tokens.go` (append)
- Test: `comparison_tokens_internal_test.go` (append)

**Interfaces:**
- Consumes: `wordToken` (Task 1); `NewTableAbsorber`, `(*TableAbsorber).Visit`, `(*TableAbsorber).TableList` (existing).
- Produces:
  - `func filterTokens(tokens []wordToken, area *Rectangle, exclude []Rectangle) []wordToken`
  - `func tableRects(p *Page) ([]Rectangle, error)`
  - `func midpointIn(r, area Rectangle) bool`

- [ ] **Step 1: Write the failing test**

Append to `comparison_tokens_internal_test.go`:

```go
func TestFilterTokensByArea(t *testing.T) {
	tokens := []wordToken{
		{text: "in", rect: Rectangle{LLX: 10, LLY: 10, URX: 30, URY: 20}},
		{text: "out", rect: Rectangle{LLX: 200, LLY: 200, URX: 220, URY: 210}},
	}
	area := Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}
	got := filterTokens(tokens, &area, nil)
	if len(got) != 1 || got[0].text != "in" {
		t.Fatalf("got %+v, want only the token inside the area", got)
	}
}

func TestFilterTokensByExcludeArea(t *testing.T) {
	tokens := []wordToken{
		{text: "keep", rect: Rectangle{LLX: 10, LLY: 10, URX: 30, URY: 20}},
		{text: "drop", rect: Rectangle{LLX: 200, LLY: 200, URX: 220, URY: 210}},
	}
	exclude := []Rectangle{{LLX: 150, LLY: 150, URX: 250, URY: 250}}
	got := filterTokens(tokens, nil, exclude)
	if len(got) != 1 || got[0].text != "keep" {
		t.Fatalf("got %+v, want only the token outside the excluded area", got)
	}
}

// A word sitting on the boundary belongs to whichever region contains its
// midpoint — never to both.
func TestFilterTokensDecidesByMidpoint(t *testing.T) {
	tokens := []wordToken{
		{text: "straddles", rect: Rectangle{LLX: 90, LLY: 10, URX: 110, URY: 20}},
	}
	area := Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}
	if got := filterTokens(tokens, &area, nil); len(got) != 0 {
		t.Fatalf("midpoint x=100 is not inside [0,100); got %+v", got)
	}
	wider := Rectangle{LLX: 0, LLY: 0, URX: 101, URY: 100}
	if got := filterTokens(tokens, &wider, nil); len(got) != 1 {
		t.Fatalf("midpoint x=100 is inside [0,101]; got %+v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestFilterTokens ./...`
Expected: FAIL — `undefined: filterTokens`.

- [ ] **Step 3: Write the implementation**

Append to `comparison_tokens.go`:

```go
// filterTokens keeps the words whose rectangle midpoint lies inside area
// (when non-nil) and outside every excluded rectangle. Deciding by the
// midpoint means a word on a boundary belongs to exactly one region.
func filterTokens(tokens []wordToken, area *Rectangle, exclude []Rectangle) []wordToken {
	if area == nil && len(exclude) == 0 {
		return tokens
	}
	out := make([]wordToken, 0, len(tokens))
	for _, tk := range tokens {
		if area != nil && !midpointIn(tk.rect, *area) {
			continue
		}
		skip := false
		for _, ex := range exclude {
			if midpointIn(tk.rect, ex) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, tk)
		}
	}
	return out
}

// midpointIn reports whether the centre of r lies within area.
func midpointIn(r, area Rectangle) bool {
	cx := (r.LLX + r.URX) / 2
	cy := (r.LLY + r.URY) / 2
	return cx >= area.LLX && cx <= area.URX && cy >= area.LLY && cy <= area.URY
}

// tableRects returns the bounding rectangles of the tables detected on the
// page — ruled and borderless alike, since the absorber runs both passes.
func tableRects(p *Page) ([]Rectangle, error) {
	ta := NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		return nil, err
	}
	tables := ta.TableList()
	rects := make([]Rectangle, 0, len(tables))
	for _, t := range tables {
		rects = append(rects, t.Rect)
	}
	return rects, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestFilterTokens ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add comparison_tokens.go comparison_tokens_internal_test.go
git commit -m "$(cat <<'EOF'
feat: comparison token filters — area, exclusions, tables (pdf-go-175w)

Words are kept or dropped by the midpoint of their rectangle, so a word on a
boundary belongs to exactly one region. Table exclusion reads the bounding
rectangles straight from the TableAbsorber.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Myers diff over token keys

**Files:**
- Create: `comparison_diff.go`
- Test: `comparison_diff_internal_test.go`

**Interfaces:**
- Consumes: nothing beyond the standard library — this task is pure algorithm over `[]string`.
- Produces:
  - `type editKind int` with `editEqual`, `editInsert`, `editDelete`
  - `type edit struct { kind editKind; a, b int }` — `a` indexes the source sequence (`-1` for an insertion), `b` the destination (`-1` for a deletion)
  - `func diffKeys(a, b []string) ([]edit, bool)` — second result is false when the edit-distance cap was hit
  - `const maxEditDistance = 4096`

- [ ] **Step 1: Write the failing test**

Create `comparison_diff_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// applyEdits rebuilds both sequences from an edit script: the source is the
// equals plus the deletions, the destination the equals plus the insertions.
func applyEdits(edits []edit, a, b []string) (src, dst []string) {
	for _, e := range edits {
		switch e.kind {
		case editEqual:
			src = append(src, a[e.a])
			dst = append(dst, b[e.b])
		case editDelete:
			src = append(src, a[e.a])
		case editInsert:
			dst = append(dst, b[e.b])
		}
	}
	return src, dst
}

func TestDiffKeysRoundTrip(t *testing.T) {
	cases := []struct{ a, b string }{
		{"one two three", "one two three"},              // identical
		{"one two three", "one two three four"},         // append
		{"one two three", "one three"},                  // delete in the middle
		{"one two three", "one TWO three"},              // replacement
		{"", "alpha beta"},                              // empty source
		{"alpha beta", ""},                              // empty destination
		{"a b c d e f", "f e d c b a"},                  // reversal
		{"the quick brown fox", "the slow brown cat!"},  // two replacements
	}
	for _, c := range cases {
		a := strings.Fields(c.a)
		b := strings.Fields(c.b)
		edits, ok := diffKeys(a, b)
		if !ok {
			t.Fatalf("%q → %q: hit the edit-distance cap unexpectedly", c.a, c.b)
		}
		src, dst := applyEdits(edits, a, b)
		if strings.Join(src, " ") != strings.Join(a, " ") {
			t.Errorf("%q → %q: source rebuilt as %q", c.a, c.b, strings.Join(src, " "))
		}
		if strings.Join(dst, " ") != strings.Join(b, " ") {
			t.Errorf("%q → %q: destination rebuilt as %q", c.a, c.b, strings.Join(dst, " "))
		}
	}
}

func TestDiffKeysIdenticalIsAllEqual(t *testing.T) {
	a := strings.Fields("alpha beta gamma")
	edits, ok := diffKeys(a, a)
	if !ok {
		t.Fatal("hit the cap on identical input")
	}
	if len(edits) != 3 {
		t.Fatalf("got %d edits, want 3", len(edits))
	}
	for i, e := range edits {
		if e.kind != editEqual {
			t.Errorf("edit %d kind = %v, want equal", i, e.kind)
		}
	}
}

func TestDiffKeysMinimalChange(t *testing.T) {
	a := strings.Fields("total is 100 euro")
	b := strings.Fields("total is 200 euro")
	edits, ok := diffKeys(a, b)
	if !ok {
		t.Fatal("hit the cap")
	}
	var ins, del, eq int
	for _, e := range edits {
		switch e.kind {
		case editInsert:
			ins++
		case editDelete:
			del++
		case editEqual:
			eq++
		}
	}
	if eq != 3 || ins != 1 || del != 1 {
		t.Fatalf("got %d equal, %d insert, %d delete; want 3/1/1", eq, ins, del)
	}
}

func TestDiffKeysCapReturnsFalse(t *testing.T) {
	a := make([]string, maxEditDistance)
	b := make([]string, maxEditDistance)
	for i := range a {
		a[i] = "a" + string(rune('A'+i%26))
		b[i] = "b" + string(rune('A'+i%26))
	}
	if _, ok := diffKeys(a, b); ok {
		t.Fatal("two entirely different sequences of cap length should report the cap")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestDiffKeys ./...`
Expected: FAIL — `undefined: diffKeys`, `undefined: edit`.

- [ ] **Step 3: Write the implementation**

Create `comparison_diff.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

// maxEditDistance caps the Myers search. Two unrelated documents drive the
// edit distance towards the sum of their lengths, and the search cost grows
// with its square; past this point the comparer reports one wholesale
// deletion and one wholesale insertion instead of grinding for minutes.
const maxEditDistance = 4096

type editKind int

const (
	editEqual editKind = iota
	editInsert
	editDelete
)

// edit is one step of the script turning the source sequence into the
// destination one. a indexes the source (-1 for an insertion), b the
// destination (-1 for a deletion).
type edit struct {
	kind editKind
	a, b int
}

// diffKeys returns the shortest edit script turning a into b, comparing the
// strings for equality. The second result is false when the search exceeded
// maxEditDistance, in which case no script is returned.
func diffKeys(a, b []string) ([]edit, bool) {
	// Common prefix and suffix are equal by construction; trimming them keeps
	// the quadratic part of the search proportional to what actually changed.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre &&
		a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}

	mid, ok := myersScript(a[pre:len(a)-suf], b[pre:len(b)-suf])
	if !ok {
		return nil, false
	}

	out := make([]edit, 0, pre+len(mid)+suf)
	for i := 0; i < pre; i++ {
		out = append(out, edit{kind: editEqual, a: i, b: i})
	}
	for _, e := range mid {
		shifted := e
		if shifted.a >= 0 {
			shifted.a += pre
		}
		if shifted.b >= 0 {
			shifted.b += pre
		}
		out = append(out, shifted)
	}
	for i := 0; i < suf; i++ {
		out = append(out, edit{kind: editEqual, a: len(a) - suf + i, b: len(b) - suf + i})
	}
	return out, true
}

// myersScript is the greedy O(ND) algorithm of Myers (1986): it advances a
// furthest-reaching frontier one edit at a time, snapshotting the frontier so
// the script can be walked back once the end is reached.
func myersScript(a, b []string) ([]edit, bool) {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil, true
	}
	maxD := n + m
	if maxD > maxEditDistance {
		maxD = maxEditDistance
	}
	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([][]int, 0, maxD+1)

	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			switch {
			case k == -d:
				x = v[offset+k+1]
			case k != d && v[offset+k-1] < v[offset+k+1]:
				x = v[offset+k+1]
			default:
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrackScript(trace, offset, n, m), true
			}
		}
	}
	return nil, false
}

// backtrackScript walks the saved frontiers from the end back to the origin,
// emitting the diagonal (equal) moves and the single edit of each step.
func backtrackScript(trace [][]int, offset, n, m int) []edit {
	rev := make([]edit, 0, n+m)
	x, y := n, m
	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		switch {
		case k == -d:
			prevK = k + 1
		case k != d && v[offset+k-1] < v[offset+k+1]:
			prevK = k + 1
		default:
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, edit{kind: editEqual, a: x, b: y})
		}
		if prevK == k+1 {
			y--
			rev = append(rev, edit{kind: editInsert, a: -1, b: y})
		} else {
			x--
			rev = append(rev, edit{kind: editDelete, a: x, b: -1})
		}
		x, y = prevX, prevY
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, edit{kind: editEqual, a: x, b: y})
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestDiffKeys ./...`
Expected: PASS. If `TestDiffKeysCapReturnsFalse` is slow (more than a few seconds), that is expected — it walks the full capped frontier once.

- [ ] **Step 5: Commit**

```bash
git add comparison_diff.go comparison_diff_internal_test.go
git commit -m "$(cat <<'EOF'
feat: Myers diff over word keys for document comparison (pdf-go-175w)

Common prefix and suffix are trimmed, then the greedy O(ND) search runs over
the remaining keys and the saved frontiers are walked back into an edit
script. Unrelated documents hit an edit-distance cap and report it, rather
than grinding through a quadratic search.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: The public model and run grouping

**Files:**
- Create: `comparison.go`
- Modify: `comparison_diff.go` (append)
- Test: `comparison_diff_internal_test.go` (append)

**Interfaces:**
- Consumes: `edit`, `editKind`, `diffKeys` (Task 3); `wordToken` (Task 1).
- Produces:
  - `type Operation int` with `OperationEqual`, `OperationInsert`, `OperationDelete`, and `func (o Operation) String() string`
  - `type DiffOperation struct { Operation Operation; Text string; SourcePage, DestPage int; SourceRects, DestRects []Rectangle }`
  - `type EditOperationsOrder int` with `EditOperationsDeleteFirst`, `EditOperationsInsertFirst`
  - `func groupOperations(edits []edit, src, dst []wordToken, order EditOperationsOrder) []DiffOperation`
  - `func wholeReplacement(src, dst []wordToken, order EditOperationsOrder) []DiffOperation`

- [ ] **Step 1: Write the failing test**

Append to `comparison_diff_internal_test.go`:

```go
// tokensFrom builds tokens for a sentence, one line, one page, with
// non-overlapping rectangles so grouping has something real to union.
func tokensFrom(t *testing.T, s string, page int) []wordToken {
	t.Helper()
	var out []wordToken
	x := 0.0
	for _, w := range strings.Fields(s) {
		width := float64(len(w)) * 6
		out = append(out, wordToken{
			text: w,
			key:  w,
			page: page,
			line: 0,
			rect: Rectangle{LLX: x, LLY: 100, URX: x + width, URY: 112},
		})
		x += width + 3
	}
	return out
}

func TestGroupOperationsMergesRuns(t *testing.T) {
	src := tokensFrom(t, "alpha beta gamma delta", 1)
	dst := tokensFrom(t, "alpha delta", 1)
	edits, ok := diffKeys([]string{"alpha", "beta", "gamma", "delta"}, []string{"alpha", "delta"})
	if !ok {
		t.Fatal("cap hit")
	}
	ops := groupOperations(edits, src, dst, EditOperationsDeleteFirst)
	if len(ops) != 3 {
		t.Fatalf("got %d operations, want 3: %+v", len(ops), ops)
	}
	if ops[1].Operation != OperationDelete || ops[1].Text != "beta gamma" {
		t.Fatalf("operation 1 = %v %q, want delete %q", ops[1].Operation, ops[1].Text, "beta gamma")
	}
	if len(ops[1].SourceRects) != 1 {
		t.Fatalf("deleted run on one line must carry one rectangle, got %d", len(ops[1].SourceRects))
	}
	if ops[1].SourcePage != 1 || ops[1].DestPage != 0 {
		t.Errorf("deletion pages = src %d dst %d, want 1 and 0", ops[1].SourcePage, ops[1].DestPage)
	}
}

func TestGroupOperationsBreaksRunsAtLineAndPage(t *testing.T) {
	src := []wordToken{
		{text: "one", key: "one", page: 1, line: 0, rect: Rectangle{LLX: 0, LLY: 100, URX: 20, URY: 112}},
		{text: "two", key: "two", page: 1, line: 1, rect: Rectangle{LLX: 0, LLY: 80, URX: 20, URY: 92}},
		{text: "three", key: "three", page: 2, line: 0, rect: Rectangle{LLX: 0, LLY: 100, URX: 30, URY: 112}},
	}
	edits := []edit{
		{kind: editDelete, a: 0, b: -1},
		{kind: editDelete, a: 1, b: -1},
		{kind: editDelete, a: 2, b: -1},
	}
	ops := groupOperations(edits, src, nil, EditOperationsDeleteFirst)
	if len(ops) != 2 {
		t.Fatalf("a run must break at the page boundary: got %d operations %+v", len(ops), ops)
	}
	if ops[0].Text != "one two" || len(ops[0].SourceRects) != 2 {
		t.Errorf("first run = %q with %d rects, want %q with 2", ops[0].Text, len(ops[0].SourceRects), "one two")
	}
	if ops[1].SourcePage != 2 {
		t.Errorf("second run page = %d, want 2", ops[1].SourcePage)
	}
}

func TestGroupOperationsRespectsEditOperationsOrder(t *testing.T) {
	src := tokensFrom(t, "total is 100", 1)
	dst := tokensFrom(t, "total is 200", 1)
	edits, ok := diffKeys([]string{"total", "is", "100"}, []string{"total", "is", "200"})
	if !ok {
		t.Fatal("cap hit")
	}

	first := groupOperations(edits, src, dst, EditOperationsDeleteFirst)
	if first[1].Operation != OperationDelete || first[2].Operation != OperationInsert {
		t.Errorf("DeleteFirst gave %v then %v", first[1].Operation, first[2].Operation)
	}

	second := groupOperations(edits, src, dst, EditOperationsInsertFirst)
	if second[1].Operation != OperationInsert || second[2].Operation != OperationDelete {
		t.Errorf("InsertFirst gave %v then %v", second[1].Operation, second[2].Operation)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestGroupOperations ./...`
Expected: FAIL — `undefined: groupOperations`, `undefined: OperationDelete`, `undefined: EditOperationsDeleteFirst`.

- [ ] **Step 3: Write the model**

Create `comparison.go`:

```go
// SPDX-License-Identifier: MIT

// Document comparison. Mirrors the text half of Aspose.PDF for .NET's
// Aspose.Pdf.Comparison namespace (TextPdfComparer, DiffOperation,
// ComparisonOptions), extended with the location of every difference:
// Aspose's DiffOperation carries only an operation and its text, while these
// operations also carry the pages and rectangles the words occupy on both
// sides — which is what lets the result be drawn back onto the original.
package asposepdf

// Operation is the kind of a difference. Mirrors Aspose.PDF for .NET's
// Aspose.Pdf.Comparison.Operation.
type Operation int

const (
	// OperationEqual marks text present in both documents.
	OperationEqual Operation = iota
	// OperationInsert marks text present only in the second document.
	OperationInsert
	// OperationDelete marks text present only in the first document.
	OperationDelete
)

// String returns "equal", "insert" or "delete".
func (o Operation) String() string {
	switch o {
	case OperationInsert:
		return "insert"
	case OperationDelete:
		return "delete"
	default:
		return "equal"
	}
}

// DiffOperation is one run of adjacent words sharing an operation. Runs break
// at line and page boundaries, so every rectangle is a real box on a real
// page: a run spanning three lines carries three rectangles.
//
// Operation and Text mirror Aspose.PDF for .NET's DiffOperation; the page and
// rectangle fields are this library's addition.
type DiffOperation struct {
	Operation Operation
	// Text is the run's words joined with single spaces. For OperationEqual
	// it is the first document's spelling, which can differ from the second's
	// when ComparisonOptions.IgnoreCase is set.
	Text string
	// SourcePage is the 1-based page in the first document, 0 for an insertion.
	SourcePage int
	// DestPage is the 1-based page in the second document, 0 for a deletion.
	DestPage int
	// SourceRects holds one rectangle per line the run touches in the first
	// document; empty for an insertion.
	SourceRects []Rectangle
	// DestRects holds one rectangle per line the run touches in the second
	// document; empty for a deletion.
	DestRects []Rectangle
}

// EditOperationsOrder decides how the two halves of a replacement are
// ordered. Mirrors Aspose.PDF for .NET's EditOperationsOrder.
type EditOperationsOrder int

const (
	// EditOperationsDeleteFirst reports the removed text before the added
	// text. This is the zero value.
	EditOperationsDeleteFirst EditOperationsOrder = iota
	// EditOperationsInsertFirst reports the added text first.
	EditOperationsInsertFirst
)
```

- [ ] **Step 4: Write the grouping**

Append to `comparison_diff.go`:

```go
// groupOperations turns an edit script into runs. Adjacent edits of the same
// kind merge while they stay on the same page; their rectangles are unioned
// per line, so a run crossing two lines reports two boxes.
func groupOperations(edits []edit, src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	var ops []DiffOperation
	for i := 0; i < len(edits); {
		j := i + 1
		for j < len(edits) && edits[j].kind == edits[i].kind &&
			samePage(edits[i], edits[j], src, dst) {
			j++
		}
		ops = append(ops, buildOperation(edits[i:j], src, dst))
		i = j
	}
	if order == EditOperationsInsertFirst {
		swapReplacements(ops)
	}
	return ops
}

// samePage reports whether two edits of the same kind sit on the same page of
// the side they belong to.
func samePage(a, b edit, src, dst []wordToken) bool {
	switch a.kind {
	case editInsert:
		return dst[a.b].page == dst[b.b].page
	case editDelete:
		return src[a.a].page == src[b.a].page
	default:
		return src[a.a].page == src[b.a].page && dst[a.b].page == dst[b.b].page
	}
}

// buildOperation assembles one run into a DiffOperation.
func buildOperation(run []edit, src, dst []wordToken) DiffOperation {
	op := DiffOperation{}
	var srcTokens, dstTokens []wordToken
	for _, e := range run {
		if e.a >= 0 {
			srcTokens = append(srcTokens, src[e.a])
		}
		if e.b >= 0 {
			dstTokens = append(dstTokens, dst[e.b])
		}
	}
	switch run[0].kind {
	case editInsert:
		op.Operation = OperationInsert
		op.Text = joinTokenText(dstTokens)
	case editDelete:
		op.Operation = OperationDelete
		op.Text = joinTokenText(srcTokens)
	default:
		op.Operation = OperationEqual
		op.Text = joinTokenText(srcTokens)
	}
	if len(srcTokens) > 0 {
		op.SourcePage = srcTokens[0].page
		op.SourceRects = lineRects(srcTokens)
	}
	if len(dstTokens) > 0 {
		op.DestPage = dstTokens[0].page
		op.DestRects = lineRects(dstTokens)
	}
	return op
}

// joinTokenText joins the words of a run with single spaces.
func joinTokenText(tokens []wordToken) string {
	parts := make([]string, len(tokens))
	for i, tk := range tokens {
		parts[i] = tk.text
	}
	return strings.Join(parts, " ")
}

// lineRects unions the tokens' rectangles per layout line, keeping document
// order, so a run wrapping onto the next line reports one box per line.
func lineRects(tokens []wordToken) []Rectangle {
	var (
		rects []Rectangle
		cur   Rectangle
		line  = -1
		open  bool
	)
	for _, tk := range tokens {
		if !open || tk.line != line {
			if open {
				rects = append(rects, cur)
			}
			cur, line, open = tk.rect, tk.line, true
			continue
		}
		cur = unionRect(cur, tk.rect)
	}
	if open {
		rects = append(rects, cur)
	}
	return rects
}

// unionRect returns the smallest rectangle containing both inputs.
func unionRect(a, b Rectangle) Rectangle {
	return Rectangle{
		LLX: math.Min(a.LLX, b.LLX),
		LLY: math.Min(a.LLY, b.LLY),
		URX: math.Max(a.URX, b.URX),
		URY: math.Max(a.URY, b.URY),
	}
}

// swapReplacements puts the inserted half of a replacement before the deleted
// half, for EditOperationsInsertFirst.
func swapReplacements(ops []DiffOperation) {
	for i := 0; i+1 < len(ops); i++ {
		if ops[i].Operation == OperationDelete && ops[i+1].Operation == OperationInsert {
			ops[i], ops[i+1] = ops[i+1], ops[i]
			i++ // the pair is settled; do not reconsider it
		}
	}
}

// wholeReplacement is the answer when the edit-distance cap is hit: the whole
// source is reported deleted and the whole destination inserted, page by page
// so the rectangles stay usable.
func wholeReplacement(src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	edits := make([]edit, 0, len(src)+len(dst))
	for i := range src {
		edits = append(edits, edit{kind: editDelete, a: i, b: -1})
	}
	for i := range dst {
		edits = append(edits, edit{kind: editInsert, a: -1, b: i})
	}
	return groupOperations(edits, src, dst, order)
}
```

Add the imports `"math"` and `"strings"` to `comparison_diff.go`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run "TestGroupOperations|TestDiffKeys" ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add comparison.go comparison_diff.go comparison_diff_internal_test.go
git commit -m "$(cat <<'EOF'
feat: comparison model and run grouping (pdf-go-175w)

DiffOperation keeps Aspose's Operation and Text and adds the pages and
rectangles of the run on both sides. Adjacent edits of one kind merge while
they stay on a page, and their boxes are unioned per line, so a run that
wraps reports one rectangle per line.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Options, page comparison and text reassembly

**Files:**
- Modify: `comparison.go` (append)
- Test: `comparison_test.go` (create, package `asposepdf_test`)

**Interfaces:**
- Consumes: `pageWordTokens`, `filterTokens`, `tableRects` (Tasks 1-2); `diffKeys`, `groupOperations`, `wholeReplacement` (Tasks 3-4).
- Produces:
  - `type ComparisonOptions struct { ExtractionArea *Rectangle; ExcludeAreas1, ExcludeAreas2 []Rectangle; ExcludeTables bool; EditOperationsOrder EditOperationsOrder; IgnoreCase bool }`
  - `func ComparePages(p1, p2 *Page, opts ...ComparisonOptions) ([]DiffOperation, error)`
  - `func AssembleSourceText(ops []DiffOperation) string`
  - `func AssembleDestinationText(ops []DiffOperation) string`
  - `func comparePageTokens(p *Page, o ComparisonOptions, second bool) ([]wordToken, error)` (unexported, reused by Task 6)
  - `func diffTokens(src, dst []wordToken, order EditOperationsOrder) []DiffOperation` (unexported, reused by Task 6)

- [ ] **Step 1: Write the failing test**

Create `comparison_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildDoc writes one text block per page and reopens the saved document.
func buildDoc(t *testing.T, pages ...string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	for i, text := range pages {
		if i > 0 {
			if err := doc.AddBlankPage(400, 200); err != nil {
				t.Fatalf("add page: %v", err)
			}
		}
		page, err := doc.Page(i + 1)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14}
		if err := page.AddText(text, style, pdf.Rectangle{LLX: 20, LLY: 60, URX: 380, URY: 170}); err != nil {
			t.Fatalf("add text: %v", err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

func firstPages(t *testing.T, a, b *pdf.Document) (*pdf.Page, *pdf.Page) {
	t.Helper()
	p1, err := a.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := b.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	return p1, p2
}

func TestComparePagesFindsTheChangedWord(t *testing.T) {
	a := buildDoc(t, "total is 100 euro")
	b := buildDoc(t, "total is 200 euro")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var ins, del []pdf.DiffOperation
	for _, op := range ops {
		switch op.Operation {
		case pdf.OperationInsert:
			ins = append(ins, op)
		case pdf.OperationDelete:
			del = append(del, op)
		}
	}
	if len(ins) != 1 || ins[0].Text != "200" {
		t.Fatalf("insertions = %+v, want one carrying %q", ins, "200")
	}
	if len(del) != 1 || del[0].Text != "100" {
		t.Fatalf("deletions = %+v, want one carrying %q", del, "100")
	}
	if len(ins[0].DestRects) != 1 || ins[0].DestPage != 1 {
		t.Fatalf("insertion location = page %d rects %+v", ins[0].DestPage, ins[0].DestRects)
	}
	if len(del[0].SourceRects) != 1 || del[0].SourcePage != 1 {
		t.Fatalf("deletion location = page %d rects %+v", del[0].SourcePage, del[0].SourceRects)
	}
}

// The rectangle of a changed word must agree with what SearchText reports for
// the same word — two different paths to the same box.
func TestComparePagesRectMatchesSearch(t *testing.T) {
	a := buildDoc(t, "total is 100 euro")
	b := buildDoc(t, "total is 200 euro")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var got pdf.Rectangle
	for _, op := range ops {
		if op.Operation == pdf.OperationInsert {
			got = op.DestRects[0]
		}
	}
	matches, err := p2.SearchText("200")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("search found %d matches, want 1", len(matches))
	}
	want := matches[0].Rect
	const tol = 0.5
	if abs(got.LLX-want.LLX) > tol || abs(got.URX-want.URX) > tol ||
		abs(got.LLY-want.LLY) > tol || abs(got.URY-want.URY) > tol {
		t.Errorf("comparison rect %+v differs from search rect %+v", got, want)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// The operations must account for every word of both documents.
func TestAssembleTextRoundTrip(t *testing.T) {
	a := buildDoc(t, "the quick brown fox jumps")
	b := buildDoc(t, "the slow brown cat jumps over")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	srcText, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	dstText, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pdf.AssembleSourceText(ops), strings.Join(strings.Fields(srcText), " "); got != want {
		t.Errorf("AssembleSourceText = %q, want %q", got, want)
	}
	if got, want := pdf.AssembleDestinationText(ops), strings.Join(strings.Fields(dstText), " "); got != want {
		t.Errorf("AssembleDestinationText = %q, want %q", got, want)
	}
}

func TestComparePagesIgnoreCase(t *testing.T) {
	a := buildDoc(t, "Alpha Beta")
	b := buildDoc(t, "alpha beta")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("case-insensitive comparison reported %v %q", op.Operation, op.Text)
		}
	}
}

func TestComparePagesExtractionArea(t *testing.T) {
	a := buildDoc(t, "keep this line\nand change this one")
	b := buildDoc(t, "keep this line\nand CHANGED this one")
	p1, p2 := firstPages(t, a, b)

	// An area covering only the upper half of the text block: the edit sits
	// on the second line, below it.
	area := pdf.Rectangle{LLX: 0, LLY: 140, URX: 400, URY: 200}
	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{ExtractionArea: &area})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("edit outside the extraction area was reported: %v %q", op.Operation, op.Text)
		}
	}
}

func TestComparisonOptionsIncompatible(t *testing.T) {
	a := buildDoc(t, "alpha")
	b := buildDoc(t, "alpha")
	p1, p2 := firstPages(t, a, b)

	area := pdf.Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}
	if _, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{
		ExtractionArea: &area,
		ExcludeTables:  true,
	}); err == nil {
		t.Fatal("ExtractionArea together with ExcludeTables must be rejected")
	}
}

func TestComparePagesNilPage(t *testing.T) {
	a := buildDoc(t, "alpha")
	p1, err := a.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.ComparePages(p1, nil); err == nil {
		t.Fatal("a nil page must be rejected")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestComparePages|TestAssembleText|TestComparisonOptions" ./...`
Expected: FAIL — `undefined: pdf.ComparePages`, `undefined: pdf.ComparisonOptions`.

- [ ] **Step 3: Write the implementation**

Append to `comparison.go` (add imports `"errors"`, `"strings"`):

```go
// ComparisonOptions narrows what is compared. The zero value compares the
// whole page, case-sensitively, reporting deletions before insertions.
// Mirrors Aspose.PDF for .NET's ComparisonOptions, plus IgnoreCase.
type ComparisonOptions struct {
	// ExtractionArea limits the comparison to the words whose centre lies in
	// this rectangle. Cannot be combined with ExcludeTables or ExcludeAreas.
	ExtractionArea *Rectangle
	// ExcludeAreas1 lists regions of the first document to ignore.
	ExcludeAreas1 []Rectangle
	// ExcludeAreas2 lists regions of the second document to ignore.
	ExcludeAreas2 []Rectangle
	// ExcludeTables drops the words inside tables found by the TableAbsorber.
	ExcludeTables bool
	// EditOperationsOrder decides which half of a replacement is reported first.
	EditOperationsOrder EditOperationsOrder
	// IgnoreCase compares words without regard to letter case. This library's
	// addition; Aspose has no equivalent.
	IgnoreCase bool
}

// validate rejects the option combinations Aspose documents as incompatible.
func (o ComparisonOptions) validate() error {
	if o.ExtractionArea == nil {
		return nil
	}
	if o.ExcludeTables || len(o.ExcludeAreas1) > 0 || len(o.ExcludeAreas2) > 0 {
		return errors.New("ComparisonOptions: ExtractionArea cannot be combined with ExcludeTables or ExcludeAreas")
	}
	return nil
}

// lastComparisonOption returns the effective options, last one winning.
func lastComparisonOption(opts []ComparisonOptions) ComparisonOptions {
	if len(opts) == 0 {
		return ComparisonOptions{}
	}
	return opts[len(opts)-1]
}

// ComparePages compares the text of two pages and returns the differences in
// reading order. Mirrors Aspose.PDF for .NET's TextPdfComparer.ComparePages.
func ComparePages(p1, p2 *Page, opts ...ComparisonOptions) ([]DiffOperation, error) {
	if p1 == nil || p2 == nil {
		return nil, errors.New("ComparePages: nil page")
	}
	o := lastComparisonOption(opts)
	if err := o.validate(); err != nil {
		return nil, err
	}
	src, err := comparePageTokens(p1, o, false)
	if err != nil {
		return nil, err
	}
	dst, err := comparePageTokens(p2, o, true)
	if err != nil {
		return nil, err
	}
	return diffTokens(src, dst, o.EditOperationsOrder), nil
}

// comparePageTokens extracts one page's words and applies the option filters.
// second selects which ExcludeAreas list applies.
func comparePageTokens(p *Page, o ComparisonOptions, second bool) ([]wordToken, error) {
	tokens, err := pageWordTokens(p, o.IgnoreCase)
	if err != nil {
		return nil, err
	}
	exclude := o.ExcludeAreas1
	if second {
		exclude = o.ExcludeAreas2
	}
	if o.ExcludeTables {
		rects, err := tableRects(p)
		if err != nil {
			return nil, err
		}
		exclude = append(append([]Rectangle(nil), exclude...), rects...)
	}
	return filterTokens(tokens, o.ExtractionArea, exclude), nil
}

// diffTokens runs the diff and groups it, falling back to a wholesale
// replacement when the edit-distance cap is hit.
func diffTokens(src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	keysOf := func(tokens []wordToken) []string {
		keys := make([]string, len(tokens))
		for i, tk := range tokens {
			keys[i] = tk.key
		}
		return keys
	}
	edits, ok := diffKeys(keysOf(src), keysOf(dst))
	if !ok {
		return wholeReplacement(src, dst, order)
	}
	return groupOperations(edits, src, dst, order)
}

// AssembleSourceText rebuilds the first document's text from the operations —
// everything equal plus everything deleted. Mirrors Aspose.PDF for .NET's
// TextPdfComparer.AssemblySourcePageText.
func AssembleSourceText(ops []DiffOperation) string {
	return assembleText(ops, OperationDelete)
}

// AssembleDestinationText rebuilds the second document's text — everything
// equal plus everything inserted. Mirrors Aspose.PDF for .NET's
// TextPdfComparer.AssemblyDestinationPageText. With
// ComparisonOptions.IgnoreCase set, equal runs carry the first document's
// spelling, so the result can differ from the second document in letter case.
func AssembleDestinationText(ops []DiffOperation) string {
	return assembleText(ops, OperationInsert)
}

func assembleText(ops []DiffOperation, side Operation) string {
	var parts []string
	for _, op := range ops {
		if op.Operation == OperationEqual || op.Operation == side {
			if op.Text != "" {
				parts = append(parts, op.Text)
			}
		}
	}
	return strings.Join(parts, " ")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run "TestComparePages|TestAssembleText|TestComparisonOptions" ./...`
Expected: PASS.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add comparison.go comparison_test.go
git commit -m "$(cat <<'EOF'
feat: ComparePages, comparison options and text reassembly (pdf-go-175w)

ComparePages mirrors TextPdfComparer.ComparePages; the options mirror
ComparisonOptions (extraction area, excluded areas, excluded tables, edit
order) and add IgnoreCase. AssembleSourceText and AssembleDestinationText
rebuild either side from the operations, which is also the invariant the
tests lean on: nothing may be lost or duplicated.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Document comparison, result and statistics

**Files:**
- Modify: `comparison.go` (append)
- Test: `comparison_test.go` (append)

**Interfaces:**
- Consumes: `comparePageTokens`, `diffTokens`, `lastComparisonOption`, `ComparisonOptions.validate` (Task 5).
- Produces:
  - `type ComparisonResult struct` with `HasChanges() bool`, `Operations() []DiffOperation`, `PageOperations(pageNum int) []DiffOperation`, `Statistics() ComparisonStatistics`
  - `type ComparisonStatistics struct { EqualWords, InsertedWords, DeletedWords int; ChangedPages []int }`
  - `func CompareDocumentsPageByPage(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error)`
  - `func CompareFlatDocuments(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error)`
  - unexported fields `src`, `dst *Document` on `ComparisonResult`, read by Task 7

- [ ] **Step 1: Write the failing test**

Append to `comparison_test.go`:

```go
func TestCompareDocumentsPageByPage(t *testing.T) {
	a := buildDoc(t, "page one alpha", "page two beta")
	b := buildDoc(t, "page one alpha", "page two gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasChanges() {
		t.Fatal("HasChanges = false, want true")
	}
	if got := res.PageOperations(1); len(got) == 0 {
		t.Fatal("page 1 reported no operations at all")
	} else {
		for _, op := range got {
			if op.Operation != pdf.OperationEqual {
				t.Errorf("page 1 is unchanged but reported %v %q", op.Operation, op.Text)
			}
		}
	}
	var changed bool
	for _, op := range res.PageOperations(2) {
		if op.Operation == pdf.OperationInsert && op.Text == "gamma" {
			changed = true
		}
	}
	if !changed {
		t.Errorf("page 2 operations = %+v, want an insertion of %q", res.PageOperations(2), "gamma")
	}
	st := res.Statistics()
	if st.InsertedWords != 1 || st.DeletedWords != 1 {
		t.Errorf("statistics = %+v, want one word inserted and one deleted", st)
	}
	if len(st.ChangedPages) != 1 || st.ChangedPages[0] != 2 {
		t.Errorf("ChangedPages = %v, want [2]", st.ChangedPages)
	}
}

func TestCompareDocumentsIdenticalHasNoChanges(t *testing.T) {
	a := buildDoc(t, "alpha beta", "gamma delta")
	b := buildDoc(t, "alpha beta", "gamma delta")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasChanges() {
		t.Fatalf("identical documents reported changes: %+v", res.Operations())
	}
	flat, err := pdf.CompareFlatDocuments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if flat.HasChanges() {
		t.Fatalf("identical documents reported changes in flat mode: %+v", flat.Operations())
	}
}

func TestCompareDocumentsPageByPageExtraPage(t *testing.T) {
	a := buildDoc(t, "alpha")
	b := buildDoc(t, "alpha", "beta gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, op := range res.PageOperations(2) {
		if op.Operation == pdf.OperationInsert && op.Text == "beta gamma" {
			found = true
		}
	}
	if !found {
		t.Errorf("the added page's text = %+v, want one insertion of %q", res.PageOperations(2), "beta gamma")
	}
}

// Text that moved to another page reads as a move in flat mode: the words are
// equal, only their page changed.
func TestCompareFlatDocumentsAcrossPages(t *testing.T) {
	a := buildDoc(t, "alpha beta gamma delta", "")
	b := buildDoc(t, "alpha beta", "gamma delta")

	res, err := pdf.CompareFlatDocuments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range res.Operations() {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("moving text across a page break reported %v %q", op.Operation, op.Text)
		}
	}
}

func TestCompareDocumentsNil(t *testing.T) {
	a := buildDoc(t, "alpha")
	if _, err := pdf.CompareDocumentsPageByPage(a, nil); err == nil {
		t.Fatal("a nil document must be rejected")
	}
	if _, err := pdf.CompareFlatDocuments(nil, a); err == nil {
		t.Fatal("a nil document must be rejected")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestCompareDocuments|TestCompareFlat" ./...`
Expected: FAIL — `undefined: pdf.CompareDocumentsPageByPage`.

- [ ] **Step 3: Write the implementation**

Append to `comparison.go` (add import `"sort"`):

```go
// ComparisonResult holds the differences between two documents and can write
// a marked-up copy of either side.
type ComparisonResult struct {
	ops   []DiffOperation
	pages [][]DiffOperation // page-by-page mode only; index 0 is page 1
	src   *Document
	dst   *Document
}

// HasChanges reports whether anything differs. Mirrors Aspose.PDF for .NET's
// SideBySideDocsComparisonResult.HasChanges.
func (r *ComparisonResult) HasChanges() bool {
	for _, op := range r.ops {
		if op.Operation != OperationEqual {
			return true
		}
	}
	return false
}

// Operations returns every difference in document order.
func (r *ComparisonResult) Operations() []DiffOperation {
	return r.ops
}

// PageOperations returns the operations of one 1-based page. An operation is
// listed under the page it physically occupies: the second document's page
// for equal and inserted text, the first document's page for deleted text.
func (r *ComparisonResult) PageOperations(pageNum int) []DiffOperation {
	if r.pages != nil {
		if pageNum < 1 || pageNum > len(r.pages) {
			return nil
		}
		return r.pages[pageNum-1]
	}
	var out []DiffOperation
	for _, op := range r.ops {
		if operationPage(op) == pageNum {
			out = append(out, op)
		}
	}
	return out
}

// operationPage is the page an operation is listed under.
func operationPage(op DiffOperation) int {
	if op.Operation == OperationDelete {
		return op.SourcePage
	}
	if op.DestPage != 0 {
		return op.DestPage
	}
	return op.SourcePage
}

// ComparisonStatistics summarises a comparison. Mirrors the shape of
// Aspose.PDF for .NET's DocumentComparisonStatistics.
type ComparisonStatistics struct {
	EqualWords    int
	InsertedWords int
	DeletedWords  int
	// ChangedPages lists, ascending, the pages carrying a change: destination
	// pages, plus source pages that have no destination counterpart.
	ChangedPages []int
}

// Statistics counts the words on each side of the comparison. Mirrors
// Aspose.PDF for .NET's TextPdfComparer.CreateComparisonStatistics.
func (r *ComparisonResult) Statistics() ComparisonStatistics {
	var st ComparisonStatistics
	seen := map[int]bool{}
	for _, op := range r.ops {
		words := len(strings.Fields(op.Text))
		switch op.Operation {
		case OperationEqual:
			st.EqualWords += words
		case OperationInsert:
			st.InsertedWords += words
		case OperationDelete:
			st.DeletedWords += words
		}
		if op.Operation != OperationEqual {
			if p := operationPage(op); p > 0 {
				seen[p] = true
			}
		}
	}
	for p := range seen {
		st.ChangedPages = append(st.ChangedPages, p)
	}
	sort.Ints(st.ChangedPages)
	return st
}

// CompareDocumentsPageByPage compares page 1 with page 1, page 2 with page 2
// and so on; the pages of the longer document beyond the shorter one are
// reported wholly inserted or wholly deleted. Mirrors Aspose.PDF for .NET's
// TextPdfComparer.CompareDocumentsPageByPage.
func CompareDocumentsPageByPage(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error) {
	if d1 == nil || d2 == nil {
		return nil, errors.New("CompareDocumentsPageByPage: nil document")
	}
	o := lastComparisonOption(opts)
	if err := o.validate(); err != nil {
		return nil, err
	}

	count := d1.PageCount()
	if d2.PageCount() > count {
		count = d2.PageCount()
	}
	res := &ComparisonResult{src: d1, dst: d2, pages: make([][]DiffOperation, count)}
	for i := 1; i <= count; i++ {
		src, err := documentPageTokens(d1, i, o, false)
		if err != nil {
			return nil, err
		}
		dst, err := documentPageTokens(d2, i, o, true)
		if err != nil {
			return nil, err
		}
		ops := diffTokens(src, dst, o.EditOperationsOrder)
		res.pages[i-1] = ops
		res.ops = append(res.ops, ops...)
	}
	return res, nil
}

// CompareFlatDocuments compares the documents as one continuous text, so
// content that moved across a page boundary reads as unchanged. Mirrors
// Aspose.PDF for .NET's TextPdfComparer.CompareFlatDocuments.
func CompareFlatDocuments(d1, d2 *Document, opts ...ComparisonOptions) (*ComparisonResult, error) {
	if d1 == nil || d2 == nil {
		return nil, errors.New("CompareFlatDocuments: nil document")
	}
	o := lastComparisonOption(opts)
	if err := o.validate(); err != nil {
		return nil, err
	}
	src, err := documentTokens(d1, o, false)
	if err != nil {
		return nil, err
	}
	dst, err := documentTokens(d2, o, true)
	if err != nil {
		return nil, err
	}
	return &ComparisonResult{
		ops: diffTokens(src, dst, o.EditOperationsOrder),
		src: d1,
		dst: d2,
	}, nil
}

// documentPageTokens returns one page's filtered tokens, or nothing when the
// document has no such page.
func documentPageTokens(d *Document, pageNum int, o ComparisonOptions, second bool) ([]wordToken, error) {
	if pageNum > d.PageCount() {
		return nil, nil
	}
	page, err := d.Page(pageNum)
	if err != nil {
		return nil, err
	}
	return comparePageTokens(page, o, second)
}

// documentTokens concatenates every page's tokens, each remembering its page.
func documentTokens(d *Document, o ComparisonOptions, second bool) ([]wordToken, error) {
	var all []wordToken
	for i := 1; i <= d.PageCount(); i++ {
		tokens, err := documentPageTokens(d, i, o, second)
		if err != nil {
			return nil, err
		}
		all = append(all, tokens...)
	}
	return all, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run "TestCompareDocuments|TestCompareFlat" ./...`
Expected: PASS.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add comparison.go comparison_test.go
git commit -m "$(cat <<'EOF'
feat: document comparison, result and statistics (pdf-go-175w)

CompareDocumentsPageByPage pairs pages by index and reports the tail of the
longer document wholesale; CompareFlatDocuments treats each document as one
continuous text, so content that moved across a page break reads as
unchanged. The result answers HasChanges, per-page operations and word
counts, mirroring their comparison-statistics shape.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Marked-up copy

**Files:**
- Create: `comparison_markup.go`
- Modify: `docs/superpowers/specs/2026-09-15-pdf-comparison-design.md` (one line, see Step 5)
- Test: `comparison_test.go` (append)

**Interfaces:**
- Consumes: `ComparisonResult` fields `ops`, `src`, `dst` (Task 6); `NewHighlightAnnotation`, `NewStrikeOutAnnotation`, `NewCaretAnnotation`, `SetQuadPoints`, `SetColor`, `SetTitle`, `SetContents`, `(*Page).Annotations`, `(*AnnotationCollection).Add`, `(*AnnotationCollection).Flatten`, `newAppearanceBuilder`, `makeFormXObjectWithResources`, `setAppearanceN` (existing).
- Produces:
  - `type MarkupSide int` with `MarkupDestination`, `MarkupSource`
  - `type MarkupOptions struct { Side MarkupSide; InsertColor, DeleteColor *Color; Title string; Flatten bool }`
  - `func (r *ComparisonResult) SaveMarkup(path string, opts ...MarkupOptions) error`
  - `func (r *ComparisonResult) WriteMarkup(w io.Writer, opts ...MarkupOptions) error`

- [ ] **Step 1: Write the failing test**

Append to `comparison_test.go` (add `"os"` and `"path/filepath"` to the imports):

```go
func TestSaveMarkupAnnotatesTheDestination(t *testing.T) {
	a := buildDoc(t, "total is 100 euro")
	b := buildDoc(t, "total is 200 euro")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("result_files", "TestSaveMarkupAnnotatesTheDestination")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "markup.pdf")
	if err := res.SaveMarkup(out); err != nil {
		t.Fatal(err)
	}

	doc, err := pdf.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	var highlights, carets int
	for _, ann := range page.Annotations().All() {
		switch ann.AnnotationType() {
		case pdf.AnnotationTypeHighlight:
			highlights++
			if ann.Contents() != "200" {
				t.Errorf("highlight contents = %q, want %q", ann.Contents(), "200")
			}
			if ann.Title() != "Comparison" {
				t.Errorf("highlight title = %q, want %q", ann.Title(), "Comparison")
			}
		case pdf.AnnotationTypeCaret:
			carets++
			if ann.Contents() != "100" {
				t.Errorf("caret contents = %q, want the deleted text %q", ann.Contents(), "100")
			}
		}
	}
	if highlights != 1 || carets != 1 {
		t.Fatalf("got %d highlights and %d carets, want 1 and 1", highlights, carets)
	}
}

func TestSaveMarkupSourceSideStrikesOut(t *testing.T) {
	a := buildDoc(t, "total is 100 euro")
	b := buildDoc(t, "total is 200 euro")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.MarkupOptions{Side: pdf.MarkupSource}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	var strikes int
	for _, ann := range page.Annotations().All() {
		if ann.AnnotationType() == pdf.AnnotationTypeStrikeOut {
			strikes++
			if ann.Contents() != "100" {
				t.Errorf("strike-out contents = %q, want %q", ann.Contents(), "100")
			}
		}
	}
	if strikes != 1 {
		t.Fatalf("got %d strike-outs, want 1", strikes)
	}
}

// Marking up must not touch the documents the caller handed in.
func TestSaveMarkupLeavesInputsAlone(t *testing.T) {
	a := buildDoc(t, "alpha beta")
	b := buildDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []*pdf.Document{a, b} {
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		if n := page.Annotations().Count(); n != 0 {
			t.Errorf("input document gained %d annotations", n)
		}
	}
}

func TestSaveMarkupFlatten(t *testing.T) {
	a := buildDoc(t, "alpha beta")
	b := buildDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.MarkupOptions{Flatten: true}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if n := page.Annotations().Count(); n != 0 {
		t.Fatalf("flattened output still carries %d annotations", n)
	}
}

// The highlight must actually paint: its appearance stream is generated, so
// our own renderer shows it too.
func TestSaveMarkupRendersDifferently(t *testing.T) {
	a := buildDoc(t, "alpha beta")
	b := buildDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.MarkupOptions{Flatten: true}); err != nil {
		t.Fatal(err)
	}
	marked, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	before, err := renderPNG(t, b)
	if err != nil {
		t.Fatal(err)
	}
	after, err := renderPNG(t, marked)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("the marked-up page renders identically to the unmarked one")
	}
}

func renderPNG(t *testing.T, doc *pdf.Document) ([]byte, error) {
	t.Helper()
	page, err := doc.Page(1)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := page.RenderPNG(&buf, pdf.RenderOptions{DPI: 72}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSaveMarkup ./...`
Expected: FAIL — `undefined: pdf.MarkupOptions`, `res.SaveMarkup undefined`.

- [ ] **Step 3: Write the implementation**

Create `comparison_markup.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"io"
)

// MarkupSide selects which document the markup is drawn on.
type MarkupSide int

const (
	// MarkupDestination marks the second document: insertions highlighted,
	// deletions shown as carets carrying the removed text. The zero value.
	MarkupDestination MarkupSide = iota
	// MarkupSource marks the first document: deletions struck out,
	// insertions shown as carets carrying the added text.
	MarkupSource
)

// MarkupOptions styles the marked-up copy. The zero value marks the second
// document in green and red, titles every annotation "Comparison" and keeps
// the annotations live.
type MarkupOptions struct {
	Side        MarkupSide
	InsertColor *Color
	DeleteColor *Color
	Title       string
	Flatten     bool
}

// resolved fills in the defaults.
func (o MarkupOptions) resolved() MarkupOptions {
	if o.InsertColor == nil {
		o.InsertColor = &Color{R: 0.20, G: 0.72, B: 0.35, A: 1}
	}
	if o.DeleteColor == nil {
		o.DeleteColor = &Color{R: 0.88, G: 0.22, B: 0.22, A: 1}
	}
	if o.Title == "" {
		o.Title = "Comparison"
	}
	return o
}

func lastMarkupOption(opts []MarkupOptions) MarkupOptions {
	if len(opts) == 0 {
		return MarkupOptions{}.resolved()
	}
	return opts[len(opts)-1].resolved()
}

// SaveMarkup writes a copy of one of the compared documents with every
// difference marked as an annotation. The documents handed to the comparer
// are left untouched.
func (r *ComparisonResult) SaveMarkup(path string, opts ...MarkupOptions) error {
	var buf bytes.Buffer
	if err := r.WriteMarkup(&buf, opts...); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

// WriteMarkup writes the marked-up copy to w.
func (r *ComparisonResult) WriteMarkup(w io.Writer, opts ...MarkupOptions) error {
	o := lastMarkupOption(opts)
	source := r.src
	if o.Side == MarkupDestination {
		source = r.dst
	}
	if source == nil {
		return errors.New("WriteMarkup: the comparison has no document on that side")
	}
	doc, err := copyDocument(source)
	if err != nil {
		return err
	}
	if err := r.annotate(doc, o); err != nil {
		return err
	}
	if o.Flatten {
		for i := 1; i <= doc.PageCount(); i++ {
			page, err := doc.Page(i)
			if err != nil {
				return err
			}
			if err := page.Annotations().Flatten(); err != nil {
				return err
			}
		}
	}
	_, err = doc.WriteTo(w)
	return err
}

// copyDocument serializes a document and opens the bytes again, yielding an
// independent document the caller's is unaffected by.
func copyDocument(d *Document) (*Document, error) {
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return OpenStream(bytes.NewReader(buf.Bytes()))
}

// annotate draws every difference onto doc.
func (r *ComparisonResult) annotate(doc *Document, o MarkupOptions) error {
	for i, op := range r.ops {
		if op.Operation == OperationEqual {
			continue
		}
		pageNum, rects := markupTarget(op, o.Side)
		if len(rects) > 0 {
			if err := addSpanAnnotation(doc, pageNum, rects, op, o); err != nil {
				return err
			}
			continue
		}
		// The run has no text on this side — a deletion seen on the new
		// document, or an insertion seen on the old one. Anchor a caret
		// between the neighbouring unchanged words.
		anchorPage, anchor, ok := anchorFor(r.ops, i, o.Side)
		if !ok {
			continue // the whole page is gone: nowhere to put a mark
		}
		if err := addCaretAnnotation(doc, anchorPage, anchor, op, o); err != nil {
			return err
		}
	}
	return nil
}

// markupTarget returns the page and rectangles an operation occupies on the
// marked side, if any.
func markupTarget(op DiffOperation, side MarkupSide) (int, []Rectangle) {
	if side == MarkupSource {
		return op.SourcePage, op.SourceRects
	}
	return op.DestPage, op.DestRects
}

// anchorFor finds where to put a caret for the operation at index i: the
// right edge of the last unchanged word before it, else the left edge of the
// first unchanged word after it.
func anchorFor(ops []DiffOperation, i int, side MarkupSide) (int, Rectangle, bool) {
	for j := i - 1; j >= 0; j-- {
		if ops[j].Operation != OperationEqual {
			continue
		}
		page, rects := markupTarget(ops[j], side)
		if len(rects) == 0 {
			continue
		}
		last := rects[len(rects)-1]
		h := last.URY - last.LLY
		return page, Rectangle{LLX: last.URX, LLY: last.LLY, URX: last.URX + h/2, URY: last.URY}, true
	}
	for j := i + 1; j < len(ops); j++ {
		if ops[j].Operation != OperationEqual {
			continue
		}
		page, rects := markupTarget(ops[j], side)
		if len(rects) == 0 {
			continue
		}
		first := rects[0]
		h := first.URY - first.LLY
		return page, Rectangle{LLX: first.LLX - h/2, LLY: first.LLY, URX: first.LLX, URY: first.URY}, true
	}
	return 0, Rectangle{}, false
}

// addSpanAnnotation highlights an insertion or strikes out a deletion.
func addSpanAnnotation(doc *Document, pageNum int, rects []Rectangle, op DiffOperation, o MarkupOptions) error {
	page, err := doc.Page(pageNum)
	if err != nil {
		return err
	}
	bbox := rects[0]
	quads := make([]QuadPoint, 0, len(rects))
	for _, r := range rects {
		bbox = unionRect(bbox, r)
		quads = append(quads, rectAsQuadPoint(r))
	}

	colour := o.InsertColor
	if op.Operation == OperationDelete {
		colour = o.DeleteColor
	}

	if op.Operation == OperationInsert {
		a := NewHighlightAnnotation(page, bbox)
		a.SetQuadPoints(quads)
		a.SetColor(colour)
		a.SetTitle(o.Title)
		a.SetContents(op.Text)
		setAppearanceN(&a.annotationBase, highlightAppearance(rects, bbox, *colour))
		return page.Annotations().Add(a)
	}

	a := NewStrikeOutAnnotation(page, bbox)
	a.SetQuadPoints(quads)
	a.SetColor(colour)
	a.SetTitle(o.Title)
	a.SetContents(op.Text)
	setAppearanceN(&a.annotationBase, strikeOutAppearance(rects, bbox, *colour))
	return page.Annotations().Add(a)
}

// addCaretAnnotation marks the point where text was removed or added.
func addCaretAnnotation(doc *Document, pageNum int, rect Rectangle, op DiffOperation, o MarkupOptions) error {
	page, err := doc.Page(pageNum)
	if err != nil {
		return err
	}
	colour := o.DeleteColor
	if op.Operation == OperationInsert {
		colour = o.InsertColor
	}
	a := NewCaretAnnotation(page, rect)
	a.SetColor(colour)
	a.SetTitle(o.Title)
	a.SetContents(op.Text)
	return page.Annotations().Add(a)
}

// highlightAppearance paints the marked words in a translucent wash. Multiply
// blending keeps the glyphs readable through the colour, which is what a
// highlighter pen does and what viewers synthesize for /Highlight.
func highlightAppearance(rects []Rectangle, bbox Rectangle, colour Color) *pdfStream {
	b := newAppearanceBuilder()
	b.SetFillColorRGB(colour)
	for _, r := range rects {
		b.Rect(r.LLX-bbox.LLX, r.LLY-bbox.LLY, r.URX-r.LLX, r.URY-r.LLY)
	}
	b.Fill()
	content := append([]byte("/GSMul gs\n"), b.Bytes()...)
	resources := pdfDict{
		"/ExtGState": pdfDict{
			"/GSMul": pdfDict{
				"/Type": pdfName("/ExtGState"),
				"/BM":   pdfName("/Multiply"),
				"/ca":   1.0,
			},
		},
	}
	return makeFormXObjectWithResources(content, Rectangle{URX: bbox.URX - bbox.LLX, URY: bbox.URY - bbox.LLY}, resources)
}

// strikeOutAppearance draws a line through the middle of each marked run.
func strikeOutAppearance(rects []Rectangle, bbox Rectangle, colour Color) *pdfStream {
	b := newAppearanceBuilder()
	b.SetStrokeColorRGB(colour)
	b.SetLineWidth(1)
	for _, r := range rects {
		y := (r.LLY+r.URY)/2 - bbox.LLY
		b.MoveTo(r.LLX-bbox.LLX, y)
		b.LineTo(r.URX-bbox.LLX, y)
	}
	b.Stroke()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: bbox.URX - bbox.LLX, URY: bbox.URY - bbox.LLY}, pdfDict{})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -run TestSaveMarkup ./...`
Expected: PASS.

If `TestSaveMarkupRendersDifferently` fails because both renders are identical, the highlight appearance is not painting: check that `setAppearanceN` was called with a non-nil stream and that the form's `/BBox` is the annotation rectangle translated to the origin.

- [ ] **Step 5: Correct one line of the spec**

The spec says appearances come from the existing builders. Markup annotations have no appearance generator in this library — the comparison writer builds theirs. In `docs/superpowers/specs/2026-09-15-pdf-comparison-design.md`, replace:

```
Appearances come from the existing builders, so the markup renders identically
in Acrobat, in our own renderer and in MuPDF.
```

with:

```
Highlight and strike-out annotations have no appearance generator in this
library (viewers synthesize one from `/QuadPoints`), so the markup writer
builds theirs with the shared appearance builder — a Multiply-blended wash for
the highlight, a centre line for the strike-out. Carets generate their own.
With an `/AP` present, the markup renders identically in Acrobat, in our own
renderer and in MuPDF.
```

- [ ] **Step 6: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add comparison_markup.go comparison_test.go docs/superpowers/specs/2026-09-15-pdf-comparison-design.md
git commit -m "$(cat <<'EOF'
feat: marked-up copy of a compared document (pdf-go-175w)

SaveMarkup/WriteMarkup copy one side of the comparison and annotate it:
insertions highlighted, deletions struck out on the source side and shown as
carets carrying the removed text on the destination side. The copy goes
through serialize-and-reopen, so the caller's documents are untouched.
Appearance streams are generated here, since markup annotations have no
generator of their own — so the result paints in our renderer too.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Right-to-left case, corpus sweep and documentation

**Files:**
- Test: `comparison_test.go` (append)
- Create (temporary, not committed): `_examples/compare_corpus/main.go`
- Modify: `CLAUDE.md`, `README.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: everything from Tasks 1-7.
- Produces: no new API. Documentation and a recorded corpus result.

- [ ] **Step 1: Write the right-to-left test**

Append to `comparison_test.go`:

```go
// Arabic is drawn in visual order; comparison must work on the logical order
// extraction restores, so the changed word is the one that is reported.
func TestComparePagesArabic(t *testing.T) {
	build := func(text string) *pdf.Document {
		doc := pdf.NewDocument(400, 200)
		font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
		if err != nil {
			t.Fatalf("load font: %v", err)
		}
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.AddText(text, pdf.TextStyle{Font: font, Size: 18},
			pdf.Rectangle{LLX: 20, LLY: 80, URX: 380, URY: 140}); err != nil {
			t.Fatalf("add text: %v", err)
		}
		var buf bytes.Buffer
		if _, err := doc.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		return re
	}

	a := build("\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064a\u0643\u0645")  // as-salamu alaykum
	b := build("\u0627\u0644\u0633\u0644\u0627\u0645 \u0644\u0644\u0639\u0627\u0644\u0645") // as-salamu lil-alam
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var equal, changed int
	for _, op := range ops {
		switch op.Operation {
		case pdf.OperationEqual:
			equal += len(strings.Fields(op.Text))
		default:
			changed += len(strings.Fields(op.Text))
		}
	}
	if equal != 1 {
		t.Errorf("got %d unchanged words, want the shared first word", equal)
	}
	if changed != 2 {
		t.Errorf("got %d changed words, want one deleted and one inserted", changed)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test -run TestComparePagesArabic ./...`
Expected: PASS. If the shared word is not reported equal, the tokenizer is reading visual order — check that `lineWordTokens` uses `buildLineRuneMap` (which applies `logicalOrder`) and not the raw fragments.

- [ ] **Step 3: Commit the test**

```bash
git add comparison_test.go
git commit -m "$(cat <<'EOF'
test: comparison of right-to-left text (pdf-go-175w)

Arabic sits on the page in visual order; the tokenizer reads the logical
order extraction restores, so the word that actually changed is the word
reported.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Write the corpus harness**

Create `_examples/compare_corpus/main.go` (the leading underscore keeps it out of `go build ./...`; it is a throwaway harness — do not commit it):

```go
// SPDX-License-Identifier: MIT

// Command compare_corpus compares every corpus document with itself. A
// self-comparison that reports a change means the tokenizer or the diff is
// not deterministic.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	root := `D:\aspose\claude\external_testdata`
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Println("read dir:", err)
		os.Exit(1)
	}
	var total, compared, changed, failed int
	start := time.Now()
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pdf" {
			continue
		}
		total++
		path := filepath.Join(root, e.Name())
		a, err := pdf.Open(path)
		if err != nil {
			failed++
			continue
		}
		b, err := pdf.Open(path)
		if err != nil {
			failed++
			continue
		}
		res, err := pdf.CompareFlatDocuments(a, b)
		if err != nil {
			failed++
			fmt.Printf("ERROR %s: %v\n", e.Name(), err)
			continue
		}
		compared++
		if res.HasChanges() {
			changed++
			st := res.Statistics()
			fmt.Printf("CHANGED %s: +%d -%d\n", e.Name(), st.InsertedWords, st.DeletedWords)
		}
	}
	fmt.Printf("\n%d pdfs, %d compared, %d unopenable, %d reported changes, %s\n",
		total, compared, failed, changed, time.Since(start).Round(time.Second))
}
```

- [ ] **Step 5: Run the corpus sweep**

Run: `go run ./_examples/compare_corpus`
Expected: `0 reported changes`. Every `CHANGED` line is a bug — a document whose extraction is not deterministic, or a tokenizer asymmetry. Fix the cause before continuing; rerun until the count is zero. Record the document count and the elapsed time; they go into the changelog entry and the epic note.

- [ ] **Step 6: Delete the harness**

```bash
rm -r _examples/compare_corpus
```

- [ ] **Step 7: Document the feature in CLAUDE.md**

Insert a new section in `CLAUDE.md` after the `**`table_detect.go` …**` block (the table-detection section), keeping the surrounding style:

```markdown
**`comparison.go` / `comparison_tokens.go` / `comparison_diff.go` / `comparison_markup.go`** — document comparison (epic `pdf-go-175w`, phase 1: text); mirrors the text half of Aspose.PDF for .NET's `Aspose.Pdf.Comparison` namespace (`TextPdfComparer`, `DiffOperation`, `ComparisonOptions`). Design: `docs/superpowers/specs/2026-09-15-pdf-comparison-design.md`
- `ComparePages(p1, p2 *Page, opts ...ComparisonOptions) ([]DiffOperation, error)`, `CompareDocumentsPageByPage(d1, d2 *Document, opts...) (*ComparisonResult, error)`, `CompareFlatDocuments(d1, d2 *Document, opts...) (*ComparisonResult, error)` — mirror `TextPdfComparer.ComparePages` / `CompareDocumentsPageByPage` / `CompareFlatDocuments`. Page-by-page pairs pages by index (the longer document's tail is reported wholesale); flat treats each document as one continuous text, so content that moved across a page break reads as unchanged
- `DiffOperation{Operation, Text, SourcePage, DestPage, SourceRects, DestRects}` — `Operation` (`OperationEqual`/`Insert`/`Delete`) and `Text` mirror Aspose's `DiffOperation`; the page and rectangle fields are ours (Aspose carries coordinates only in the 26.7 side-by-side `EditContainer`). A run is a maximal sequence of adjacent words with one operation, broken at line and page boundaries, so each rectangle is a real box on a real page
- `ComparisonResult` — `HasChanges()`, `Operations()`, `PageOperations(n)`, `Statistics() ComparisonStatistics{EqualWords, InsertedWords, DeletedWords, ChangedPages}`, `SaveMarkup(path, opts...)` / `WriteMarkup(w, opts...)`
- `ComparisonOptions{ExtractionArea, ExcludeAreas1, ExcludeAreas2, ExcludeTables, EditOperationsOrder, IgnoreCase}` — mirrors theirs plus `IgnoreCase`; `ExtractionArea` together with `ExcludeTables`/`ExcludeAreas*` is an error, as in Aspose. `AssembleSourceText`/`AssembleDestinationText` rebuild either side from the operations (their `AssemblySourcePageText`/`AssemblyDestinationPageText`)
- **Tokenization** (`comparison_tokens.go`): one `ExtractTextWithLayout` per page; each line's text is split at whitespace and every word's rectangle comes from `buildLineRuneMap` + `matchRect` — the machinery `SearchText` uses, so right-to-left lines (logical order) and sub-fragment boundaries are handled identically. Filters decide by the rectangle's midpoint, so a word on a boundary belongs to exactly one region; `ExcludeTables` takes the bounding rectangles from `NewTableAbsorber().Visit`
- **Diff** (`comparison_diff.go`): common prefix/suffix trimmed, then Myers O(ND) over the word keys with the frontier snapshots walked back into an edit script; adjacent same-kind edits group into runs (broken at page changes, rectangles unioned per line). `maxEditDistance` (4096) caps the search — beyond it the comparer reports the whole source deleted and the whole destination inserted rather than grinding quadratically
- **Markup** (`comparison_markup.go`): `SaveMarkup`/`WriteMarkup` copy one side (serialize + reopen, so the caller's documents are untouched) and annotate it — destination side: insertions as `HighlightAnnotation` over the run's rectangles, deletions as a `CaretAnnotation` anchored between the neighbouring unchanged words with the removed text in `/Contents`; source side: deletions as `StrikeOutAnnotation`, insertions as carets. All carry `/T` (default `"Comparison"`), so a viewer's comments panel is the change list. Highlight and strike-out appearance streams are generated here (markup annotations have no generator in this library): a Multiply-blended wash and a centre line, so the markup paints in our own renderer as well as in Acrobat. `MarkupOptions{Side, InsertColor, DeleteColor, Title, Flatten}`
- Out of scope in phase 1: graphical (pixel) comparison, side-by-side output, the HTML/JSON/Markdown diff generators and the Aspose-style generated text report, character-level refinement inside a changed word, and de-hyphenation (a word broken across lines compares as two words)
```

- [ ] **Step 8: Document the feature in README.md**

`README.md` is maintained in the corporate Aspose format: keep its structure and write the feature into the existing prose. Add one bullet to the capability list, next to the table-detection bullet, in the same voice:

```markdown
- **Document comparison** — `CompareDocumentsPageByPage` and `CompareFlatDocuments` report what
  changed between two PDFs word by word, with the page and rectangle of every difference;
  `ComparisonResult.SaveMarkup` writes a copy of either document in which insertions are
  highlighted, deletions struck out and removed text kept in the annotation note, so a reviewer
  reads the change list in any PDF viewer. Mirrors Aspose.PDF for .NET's `TextPdfComparer`.
```

Check with `grep -n "Table detection" README.md` where the neighbouring bullet sits, and match the surrounding indentation and line width exactly.

- [ ] **Step 9: Add the changelog entry**

In `CHANGELOG.md`, under `## [Unreleased]` → `### Added`, add as the first entry (use the corpus numbers recorded in Step 5):

```markdown
- **Document comparison** — `CompareDocumentsPageByPage(d1, d2)` / `CompareFlatDocuments(d1, d2)` / `ComparePages(p1, p2)` report the differences between two PDFs word by word, mirroring Aspose.PDF for .NET's `TextPdfComparer`. Each `DiffOperation` carries its operation and text like theirs and, beyond theirs, **the page and rectangle of the run on both sides** — so the result can be drawn back onto the document: `ComparisonResult.SaveMarkup` writes a copy with insertions highlighted, deletions struck out (or marked with a caret carrying the removed text on the new document), every annotation titled so a viewer's comments panel becomes the change list. Words come from the layout extractor through the same rune-map machinery `SearchText` uses, so right-to-left text compares in logical order; `ComparisonOptions` mirrors theirs (`ExtractionArea`, `ExcludeAreas1/2`, `ExcludeTables` via the `TableAbsorber`, `EditOperationsOrder`) and adds `IgnoreCase`. Corpus: every one of the NNNN documents compares against itself with no differences reported, in MM. Graphical and side-by-side comparison are the next phases. (`pdf-go-175w`)
```

Replace `NNNN` and `MM` with the counts from Step 5.

- [ ] **Step 10: Run the whole suite and the formatter**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l` prints nothing, vet is silent, tests pass.

- [ ] **Step 11: Commit**

```bash
git add CLAUDE.md README.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs: document comparison in CLAUDE.md, README and changelog (pdf-go-175w)

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 12: Update the tracker**

```bash
bd update pdf-go-175w --status closed
bd create "Document comparison phase 2: graphical and side-by-side" -t feature -p 3 -d "Phase 2 of pdf-go-175w. GraphicalPdfComparer: render both pages with the built-in rasterizer and report an ImagesDifference-equivalent (difference image plus the changed-pixel ratio). SideBySidePdfComparer: a two-column before/after spread with the differences highlighted, built on the phase-1 model. Design: docs/superpowers/specs/2026-09-15-pdf-comparison-design.md (Out of scope section)."
bd create "Document comparison phase 3: diff output generators" -t feature -p 3 -d "Phase 3 of pdf-go-175w. Serializers over the phase-1 DiffOperation model, mirroring Aspose's IStringOutputGenerator/IFileOutputGenerator: HtmlDiffOutputGenerator, JsonDiffOutputGenerator, MarkdownDiffOutputGenerator, and the Aspose-style generated text report (PdfOutputGenerator) through our flow layer. Design: docs/superpowers/specs/2026-09-15-pdf-comparison-design.md (Out of scope section)."
```

---

## Self-Review

**Spec coverage.** Public API — Tasks 4-7. Tokenization with rectangles — Task 1. Filters (`ExtractionArea`, `ExcludeAreas1/2`, `ExcludeTables`) and their incompatibility — Tasks 2 and 5. Myers with the edit-distance cap — Task 3. Run grouping, page/line breaks, `EditOperationsOrder` — Task 4. `ComparePages` — Task 5. Page-by-page and flat, `ComparisonResult`, statistics — Task 6. Markup with anchoring, `Flatten`, untouched inputs — Task 7. Validation: algorithm (Task 3), completeness invariant (Task 5), geometry against `SearchText` (Task 5), options (Tasks 2 and 5), markup (Task 7), right-to-left and corpus (Task 8). Documentation — Task 8.

**Placeholders.** `NNNN` and `MM` in the changelog entry of Task 8 Step 9 are filled from the corpus run in Step 5, which is why Step 5 says to record them; every other step carries its actual content.

**Type consistency.** `wordToken` fields (`text`, `key`, `page`, `line`, `rect`) are used identically in Tasks 1, 2, 4, 5 and 6. `edit{kind, a, b}` and `editKind` are produced in Task 3 and consumed in Task 4. `DiffOperation` field names are fixed in Task 4 and used unchanged in Tasks 5, 6 and 7. `unionRect` and `lineRects` are defined once (Task 4) and reused in Task 7. `rectAsQuadPoint` and `setAppearanceN` are existing functions (`appearance_redact.go:73`, `appearance.go`), not redefined. `comparePageTokens` and `diffTokens` are introduced in Task 5 and consumed in Task 6.
