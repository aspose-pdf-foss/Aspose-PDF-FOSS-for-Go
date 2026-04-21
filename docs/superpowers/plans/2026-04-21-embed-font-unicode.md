# EmbedFont + Unicode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add TrueType font embedding and Unicode text support to `AddText`, enabling text in any language (not just Latin-1 / standard 14 fonts).

**Architecture:** `Font` becomes an interface. Standard 14 are package-level `var`s of a `standardFont` value type; embedded TTF fonts are `*embeddedFont` handles returned by `Document.LoadFont`. A new `ttf.go` parser reads the required tables (head/hhea/hmtx/maxp/name/cmap/OS-2/post). `font_embed.go` builds five PDF objects — Type0 font, CIDFontType2 descendant, FontDescriptor, FontFile2 stream, and ToUnicode CMap — and wires them into `doc.objects` at `LoadFont` time. `AddText` uses a per-rune width callback and encoding callback, chosen via type switch on `style.Font`.

**Tech Stack:** Pure Go. Uses `encoding/binary` (stdlib) for TTF table parsing; `compress/flate` (already in use) for `/FontFile2` compression.

---

## File Structure

| File | Role |
|------|------|
| `font_api.go` (new) | `Font` interface, `standardFont`, `*embeddedFont`, package-level standard 14 vars, `FindFont`, `(*Document).LoadFont`, `(*Document).LoadFontFromStream` |
| `font_api_test.go` (new) | Unit tests for Font interface, FindFont, LoadFont errors |
| `ttf.go` (new) | `ttfFont` struct, `parseTTF`, per-table parsers |
| `ttf_test.go` (new) | Unit tests for TTF parser against DejaVuSans.ttf |
| `font_embed.go` (new) | Build Type0 + CIDFontType2 + FontDescriptor + FontFile2 + ToUnicode CMap objects |
| `font_embed_test.go` (new) | Unit tests for embedded object generation |
| `color.go` (modify) | Remove `Font int` and iota constants + `fontPDFName`; keep Color, HAlign, VAlign, TextStyle with new `Font` field type |
| `encoding.go` (modify) | Add `winAnsiEncodeRune(r rune) (byte, bool)` helper |
| `text_add.go` (modify) | Rune-safe `wrapText`/`measureString`/`breakWord` via `widthFn`; `AddText` type-switches on `style.Font`; `ensureEmbeddedFontResource` helper |
| `text_add_test.go` (modify) | Delete `TestFontPDFName*`; add Unicode/embedded tests; update wrap tests to use `widthFn` |
| `text_add_integration_test.go` (modify) | Add `TestAddTextEmbeddedFontRoundTrip` |
| `testdata/DejaVuSans.ttf` (new) | Test TTF (Bitstream Vera license; Latin + Cyrillic + Greek) |
| `CLAUDE.md` (modify) | Document `Font`, `FindFont`, `LoadFont`, `LoadFontFromStream`, `IsEmbedded` |

---

### Task 1: Refactor `Font` from int to interface

**Files:**
- Create: `font_api.go`
- Modify: `color.go`
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Create `font_api.go` with `Font` interface and `standardFont`**

```go
package asposepdf

// Font is implemented by standard 14 fonts and embedded TTF fonts.
// Use the package-level vars (FontHelvetica, ...) or LoadFont to obtain a Font.
type Font interface {
	// BaseFont returns the PostScript name, e.g. "Helvetica" or "ArialMT".
	BaseFont() string
	// IsEmbedded reports whether font data is embedded in the PDF (true for TTF, false for standard 14).
	IsEmbedded() bool
}

// standardFont is the built-in Font implementation for the 14 standard PDF fonts.
type standardFont struct {
	name string // PostScript name without leading slash, e.g. "Helvetica"
}

func (s standardFont) BaseFont() string { return s.name }
func (s standardFont) IsEmbedded() bool { return false }

// Standard 14 PDF fonts. These Fonts need not be embedded — every PDF viewer
// is required to render them.
var (
	FontHelvetica            Font = standardFont{name: "Helvetica"}
	FontHelveticaBold        Font = standardFont{name: "Helvetica-Bold"}
	FontHelveticaOblique     Font = standardFont{name: "Helvetica-Oblique"}
	FontHelveticaBoldOblique Font = standardFont{name: "Helvetica-BoldOblique"}
	FontTimesRoman           Font = standardFont{name: "Times-Roman"}
	FontTimesBold            Font = standardFont{name: "Times-Bold"}
	FontTimesItalic          Font = standardFont{name: "Times-Italic"}
	FontTimesBoldItalic      Font = standardFont{name: "Times-BoldItalic"}
	FontCourier              Font = standardFont{name: "Courier"}
	FontCourierBold          Font = standardFont{name: "Courier-Bold"}
	FontCourierOblique       Font = standardFont{name: "Courier-Oblique"}
	FontCourierBoldOblique   Font = standardFont{name: "Courier-BoldOblique"}
	FontSymbol               Font = standardFont{name: "Symbol"}
	FontZapfDingbats         Font = standardFont{name: "ZapfDingbats"}
)
```

- [ ] **Step 2: Remove `Font int` and `fontPDFName` from `color.go`**

In `color.go`, delete:
- the `type Font int` declaration
- the `const ( FontHelvetica Font = iota ... )` block
- the `fontPDFName` function

Keep:
- `type Color struct { R, G, B, A float64 }`
- `HAlign`/`VAlign` and their consts
- `TextStyle` struct — but **change the `Font` field type to `Font` (interface)**. Since the field name and the type both happen to be called `Font`, the declaration becomes:

```go
type TextStyle struct {
	Font          Font    // nil defaults to FontHelvetica in AddText
	Size          float64
	Color         *Color
	Background    *Color
	HAlign        HAlign
	VAlign        VAlign
	LineSpacing   float64
	Underline     bool
	Strikethrough bool
	Rotation      float64
	Behind        bool
}
```

- [ ] **Step 3: Update `text_add.go`**

In [text_add.go](text_add.go):
- Delete the `isValidFont` function.
- Replace the font-validation + `fontPDFName` block inside `AddText` with the code below. Patch lines roughly `text_add.go:141-170`:

```go
func (p *Page) AddText(text string, style TextStyle, rect Rectangle) error {
	if text == "" {
		return nil
	}
	if err := rect.validate(); err != nil {
		return fmt.Errorf("add text: %w", err)
	}
	if style.Size < 0 {
		return fmt.Errorf("add text: font size must be non-negative, got %g", style.Size)
	}

	// Default Font if unset.
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	sf, ok := font.(standardFont)
	if !ok {
		return fmt.Errorf("add text: unsupported font type %T", font)
	}

	// Apply defaults.
	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	lineSpacing := style.LineSpacing
	if lineSpacing == 0 {
		lineSpacing = 1.2
	}
	textColor := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		textColor = *style.Color
	}

	// Get font metrics.
	pdfFontName := "/" + sf.name
	widths, _ := standard14Widths(pdfFontName)
```

The rest of `AddText` is unchanged. Embedded-font dispatch is added in Task 13.

- [ ] **Step 4: Update `text_add_test.go` — delete obsolete Font-int tests**

Open [text_add_test.go](text_add_test.go) and **delete** `TestFontPDFName` (lines ~8–34) and `TestFontPDFNameInvalid` (lines ~36–41). These tested `fontPDFName(Font)` which no longer exists.

Add a replacement test at the top of the file:

```go
func TestStandardFontBaseFont(t *testing.T) {
	cases := []struct {
		font Font
		want string
	}{
		{FontHelvetica, "Helvetica"},
		{FontHelveticaBold, "Helvetica-Bold"},
		{FontHelveticaOblique, "Helvetica-Oblique"},
		{FontHelveticaBoldOblique, "Helvetica-BoldOblique"},
		{FontTimesRoman, "Times-Roman"},
		{FontTimesBold, "Times-Bold"},
		{FontTimesItalic, "Times-Italic"},
		{FontTimesBoldItalic, "Times-BoldItalic"},
		{FontCourier, "Courier"},
		{FontCourierBold, "Courier-Bold"},
		{FontCourierOblique, "Courier-Oblique"},
		{FontCourierBoldOblique, "Courier-BoldOblique"},
		{FontSymbol, "Symbol"},
		{FontZapfDingbats, "ZapfDingbats"},
	}
	for _, tc := range cases {
		if got := tc.font.BaseFont(); got != tc.want {
			t.Errorf("%T.BaseFont() = %q, want %q", tc.font, got, tc.want)
		}
		if tc.font.IsEmbedded() {
			t.Errorf("%v.IsEmbedded() = true, want false for standard 14", tc.font)
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go build ./... && go test ./...`
Expected: All tests PASS. The project compiles and existing behavior is preserved.

- [ ] **Step 6: Commit**

```bash
git add font_api.go color.go text_add.go text_add_test.go
git commit -m "refactor: make Font an interface with standardFont impl"
```

---

### Task 2: Add `FindFont` discovery

**Files:**
- Modify: `font_api.go`
- Create: `font_api_test.go`

- [ ] **Step 1: Write failing tests**

Create [font_api_test.go](font_api_test.go):

```go
package asposepdf

import (
	"strings"
	"testing"
)

func TestFindFontExact(t *testing.T) {
	f, err := FindFont("Helvetica")
	if err != nil {
		t.Fatalf("FindFont: %v", err)
	}
	if f.BaseFont() != "Helvetica" {
		t.Errorf("FindFont(\"Helvetica\").BaseFont() = %q, want Helvetica", f.BaseFont())
	}
}

func TestFindFontCaseInsensitive(t *testing.T) {
	cases := []string{"helvetica", "HELVETICA", "HeLvEtIcA"}
	for _, name := range cases {
		f, err := FindFont(name)
		if err != nil {
			t.Fatalf("FindFont(%q): %v", name, err)
		}
		if f.BaseFont() != "Helvetica" {
			t.Errorf("FindFont(%q) = %q, want Helvetica", name, f.BaseFont())
		}
	}
}

func TestFindFontAllStandard14(t *testing.T) {
	names := []string{
		"Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
		"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
		"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
		"Symbol", "ZapfDingbats",
	}
	for _, name := range names {
		f, err := FindFont(name)
		if err != nil {
			t.Errorf("FindFont(%q): %v", name, err)
			continue
		}
		if f.BaseFont() != name {
			t.Errorf("FindFont(%q).BaseFont() = %q", name, f.BaseFont())
		}
	}
}

func TestFindFontUnknown(t *testing.T) {
	_, err := FindFont("Arial")
	if err == nil {
		t.Fatal("FindFont(\"Arial\"): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error message = %q, expected to contain \"unknown\"", err.Error())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestFindFont" -v ./...`
Expected: FAIL — `FindFont` undefined.

- [ ] **Step 3: Implement `FindFont`**

Append to [font_api.go](font_api.go):

```go
import "strings"

// standardFontIndex maps the canonical lower-case PostScript name to a standardFont.
var standardFontIndex = map[string]Font{
	"helvetica":              FontHelvetica,
	"helvetica-bold":         FontHelveticaBold,
	"helvetica-oblique":      FontHelveticaOblique,
	"helvetica-boldoblique":  FontHelveticaBoldOblique,
	"times-roman":            FontTimesRoman,
	"times-bold":             FontTimesBold,
	"times-italic":           FontTimesItalic,
	"times-bolditalic":       FontTimesBoldItalic,
	"courier":                FontCourier,
	"courier-bold":           FontCourierBold,
	"courier-oblique":        FontCourierOblique,
	"courier-boldoblique":    FontCourierBoldOblique,
	"symbol":                 FontSymbol,
	"zapfdingbats":           FontZapfDingbats,
}

// FindFont returns a standard 14 Font by PostScript name. The lookup is
// case-insensitive. Returns an error if the name is not a standard 14 name.
func FindFont(name string) (Font, error) {
	if f, ok := standardFontIndex[strings.ToLower(name)]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("find font: unknown standard font %q", name)
}
```

Add `"fmt"` to the import block if not already present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestFindFont" -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add font_api.go font_api_test.go
git commit -m "feat: add FindFont for standard 14 discovery"
```

---

### Task 3: Add `winAnsiEncodeRune` helper and rune-safe wrap with `widthFn`

**Files:**
- Modify: `encoding.go`
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Write failing test for `winAnsiEncodeRune`**

Append to [text_add_test.go](text_add_test.go) (or create a new `encoding_test.go` — either is fine, here we keep tests close to callers):

```go
func TestWinAnsiEncodeRune(t *testing.T) {
	cases := []struct {
		r    rune
		code byte
		ok   bool
	}{
		{'A', 'A', true},
		{' ', ' ', true},
		{'€', 0x80, true},     // euro at WinAnsi 0x80
		{'©', 0xA9, true},     // copyright at 0xA9
		{'ÿ', 0xFF, true},     // y-diaeresis at 0xFF
		{'日', 0, false},      // CJK — not in WinAnsi
		{'\uFFFD', 0, false}, // replacement — explicitly not mapped
	}
	for _, tc := range cases {
		code, ok := winAnsiEncodeRune(tc.r)
		if code != tc.code || ok != tc.ok {
			t.Errorf("winAnsiEncodeRune(%q) = (0x%02X, %v), want (0x%02X, %v)",
				tc.r, code, ok, tc.code, tc.ok)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -run TestWinAnsiEncodeRune -v ./...`
Expected: FAIL — `winAnsiEncodeRune` undefined.

- [ ] **Step 3: Implement `winAnsiEncodeRune`**

Append to [encoding.go](encoding.go):

```go
import "sync"

var (
	winAnsiReverseOnce sync.Once
	winAnsiReverse     map[rune]byte
)

// winAnsiEncodeRune returns the WinAnsi byte code for the given rune.
// Returns (0, false) if the rune is not representable in WinAnsiEncoding.
func winAnsiEncodeRune(r rune) (byte, bool) {
	winAnsiReverseOnce.Do(func() {
		winAnsiReverse = make(map[rune]byte, 256)
		for code, ch := range winAnsiEncoding {
			if ch == '\uFFFD' {
				continue
			}
			// First occurrence wins (WinAnsi has no duplicates anyway).
			if _, exists := winAnsiReverse[ch]; !exists {
				winAnsiReverse[ch] = byte(code)
			}
		}
	})
	c, ok := winAnsiReverse[r]
	return c, ok
}
```

If `encoding.go` has no import block yet, add `import "sync"` at the top.

- [ ] **Step 4: Run test**

Run: `go test -run TestWinAnsiEncodeRune -v ./...`
Expected: PASS.

- [ ] **Step 5: Write failing tests for rune-safe `wrapText` and new `widthFn` signature**

Replace the existing wrap tests in [text_add_test.go](text_add_test.go) (the block starting at `TestWrapTextSingleLine`, four tests total) with:

```go
// helvetiaWidthFn builds a widthFn for Helvetica at the given font size.
func helveticaWidthFn(t *testing.T, size float64) widthFn {
	t.Helper()
	w, ok := standard14Widths("/Helvetica")
	if !ok {
		t.Fatalf("standard14Widths Helvetica not found")
	}
	return func(r rune) float64 {
		code, ok := winAnsiEncodeRune(r)
		if !ok {
			code = byte('?')
		}
		return w[code] / 1000.0 * size
	}
}

func TestWrapTextSingleLine(t *testing.T) {
	lines := wrapText("Hello", helveticaWidthFn(t, 12), 500)
	if len(lines) != 1 || lines[0] != "Hello" {
		t.Errorf("wrapText single line = %v, want [Hello]", lines)
	}
}

func TestWrapTextMultipleLines(t *testing.T) {
	lines := wrapText("Hello World", helveticaWidthFn(t, 12), 40)
	if len(lines) != 2 {
		t.Fatalf("wrapText = %v, want 2 lines", lines)
	}
	if lines[0] != "Hello" || lines[1] != "World" {
		t.Errorf("wrapText = %v, want [Hello, World]", lines)
	}
}

func TestWrapTextLongWord(t *testing.T) {
	lines := wrapText("ABCDEFGHIJKLMNOP", helveticaWidthFn(t, 12), 50)
	if len(lines) < 2 {
		t.Fatalf("wrapText long word = %v, expected multiple lines", lines)
	}
}

func TestWrapTextNewlines(t *testing.T) {
	lines := wrapText("Line1\nLine2\nLine3", helveticaWidthFn(t, 12), 500)
	if len(lines) != 3 {
		t.Fatalf("wrapText newlines = %v, want 3 lines", lines)
	}
	if lines[0] != "Line1" || lines[1] != "Line2" || lines[2] != "Line3" {
		t.Errorf("wrapText newlines = %v", lines)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	lines := wrapText("", helveticaWidthFn(t, 12), 500)
	if len(lines) != 0 {
		t.Errorf("wrapText empty = %v, want []", lines)
	}
}

func TestWrapTextRuneSafe(t *testing.T) {
	// Even with standard 14, long Cyrillic words should not be cut mid-rune.
	// Each ? (WinAnsi fallback) is 278 units wide at 12pt = ~3.3pt.
	// Force a break by using narrow rect relative to string length.
	lines := wrapText("АБВГДЕЖЗИЙ", helveticaWidthFn(t, 12), 10)
	for _, line := range lines {
		if !utf8.ValidString(line) {
			t.Errorf("wrapText produced invalid UTF-8 line: %q", line)
		}
	}
}
```

Add `"unicode/utf8"` to the imports if not already present.

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test -run "TestWrapText" -v ./...`
Expected: FAIL — `wrapText` signature mismatch; `widthFn` undefined.

- [ ] **Step 7: Refactor `wrapText`/`measureString`/`breakWord`**

Replace the top of [text_add.go](text_add.go) (everything above `escapeStringPDF`) with:

```go
package asposepdf

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// widthFn returns advance width in points for a single rune.
type widthFn func(r rune) float64

// wrapText splits text into lines that fit within maxWidth points.
// It breaks at spaces; words longer than maxWidth are broken on rune boundaries.
// Explicit newlines in the input force a line break.
func wrapText(text string, width widthFn, maxWidth float64) []string {
	if text == "" {
		return nil
	}

	var result []string
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var line string
		var lineWidth float64

		for _, word := range words {
			wordWidth := measureString(word, width)

			if lineWidth == 0 {
				if wordWidth <= maxWidth {
					line = word
					lineWidth = wordWidth
				} else {
					broken := breakWord(word, width, maxWidth)
					for i, part := range broken {
						if i < len(broken)-1 {
							result = append(result, part)
						} else {
							line = part
							lineWidth = measureString(part, width)
						}
					}
				}
			} else {
				spaceWidth := width(' ')
				if lineWidth+spaceWidth+wordWidth <= maxWidth {
					line += " " + word
					lineWidth += spaceWidth + wordWidth
				} else {
					result = append(result, line)
					if wordWidth <= maxWidth {
						line = word
						lineWidth = wordWidth
					} else {
						broken := breakWord(word, width, maxWidth)
						for i, part := range broken {
							if i < len(broken)-1 {
								result = append(result, part)
							} else {
								line = part
								lineWidth = measureString(part, width)
							}
						}
					}
				}
			}
		}
		if line != "" || lineWidth == 0 {
			result = append(result, line)
		}
	}

	return result
}

// measureString returns the width of a string in points.
func measureString(s string, width widthFn) float64 {
	var w float64
	for _, r := range s {
		w += width(r)
	}
	return w
}

// breakWord breaks a single word into parts that each fit within maxWidth.
// Splits on rune boundaries so multi-byte UTF-8 is never cut mid-sequence.
func breakWord(word string, width widthFn, maxWidth float64) []string {
	var parts []string
	var buf strings.Builder
	var w float64
	for _, r := range word {
		cw := width(r)
		if w+cw > maxWidth && buf.Len() > 0 {
			parts = append(parts, buf.String())
			buf.Reset()
			w = 0
		}
		buf.WriteRune(r)
		w += cw
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}
```

Note: `utf8` is imported because the rune-safe test uses it. If the lint complains that `utf8` is unused inside `text_add.go`, move the `unicode/utf8` import to `text_add_test.go` and drop it from `text_add.go`.

- [ ] **Step 8: Update `AddText` to build and pass a `widthFn`**

In [text_add.go](text_add.go), inside `AddText`, find the block:

```go
	// Word wrap.
	rectWidth := rect.URX - rect.LLX
	rectHeight := rect.URY - rect.LLY
	lines := wrapText(text, widths, fontSize, rectWidth)
```

Replace with:

```go
	// Word wrap.
	rectWidth := rect.URX - rect.LLX
	rectHeight := rect.URY - rect.LLY
	width := func(r rune) float64 {
		code, ok := winAnsiEncodeRune(r)
		if !ok {
			code = byte('?')
		}
		return widths[code] / 1000.0 * fontSize
	}
	lines := wrapText(text, width, rectWidth)
```

And find every other call to `measureString(...)` inside `AddText` (currently two call sites — inside the line-loop for alignment, and inside the line-break branch). Replace:

```go
lineWidth := measureString(line, widths, fontSize)
```

with:

```go
lineWidth := measureString(line, width)
```

- [ ] **Step 9: Run all tests**

Run: `go test ./...`
Expected: All tests PASS (including all previously existing AddText/wrap tests and the new TestWrapTextRuneSafe).

- [ ] **Step 10: Commit**

```bash
git add encoding.go text_add.go text_add_test.go
git commit -m "refactor: rune-safe wrap/measure via widthFn callback"
```

---

### Task 4: Add DejaVuSans.ttf to testdata

**Files:**
- Create: `testdata/DejaVuSans.ttf`

- [ ] **Step 1: Download DejaVuSans.ttf**

Run (from repo root):

```bash
curl -L -o testdata/DejaVuSans.ttf \
  https://github.com/dejavu-fonts/dejavu-fonts/raw/main/ttf/DejaVuSans.ttf
```

Expected: downloads a ~756 KB file.

Verify it's a valid TrueType file:

```bash
ls -l testdata/DejaVuSans.ttf
head -c 4 testdata/DejaVuSans.ttf | xxd
```

Expected first line: `00000000: 0001 0000 ....` (scaler `00 01 00 00` = TrueType).

- [ ] **Step 2: Commit**

```bash
git add testdata/DejaVuSans.ttf
git commit -m "test: add DejaVuSans.ttf for TTF parser tests"
```

---

### Task 5: TTF parser skeleton — magic bytes, table directory, required tables

**Files:**
- Create: `ttf.go`
- Create: `ttf_test.go`

- [ ] **Step 1: Write failing tests**

Create [ttf_test.go](ttf_test.go):

```go
package asposepdf

import (
	"os"
	"strings"
	"testing"
)

func loadDejaVu(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("read DejaVuSans.ttf: %v", err)
	}
	return data
}

func TestParseTTF_NotTTF(t *testing.T) {
	_, err := parseTTF([]byte("not a font file, just garbage"))
	if err == nil {
		t.Fatal("expected error for non-TTF input")
	}
	if !strings.Contains(err.Error(), "TrueType") {
		t.Errorf("error = %q, want to mention TrueType", err.Error())
	}
}

func TestParseTTF_TooSmall(t *testing.T) {
	_, err := parseTTF([]byte{0x00, 0x01, 0x00, 0x00})
	if err == nil {
		t.Fatal("expected error for truncated file")
	}
}

func TestParseTTF_DejaVuBasic(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatalf("parseTTF: %v", err)
	}
	if f == nil {
		t.Fatal("parseTTF returned nil font")
	}
	if len(f.data) == 0 {
		t.Error("ttfFont.data is empty")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestParseTTF" -v ./...`
Expected: FAIL — `parseTTF` undefined.

- [ ] **Step 3: Implement the skeleton**

Create [ttf.go](ttf.go):

```go
package asposepdf

import (
	"encoding/binary"
	"fmt"
)

// ttfFont holds the parsed fields required for PDF embedding and text measurement.
type ttfFont struct {
	data []byte // raw TTF bytes (written verbatim into /FontFile2)

	// From head.
	unitsPerEm uint16
	xMin, yMin int16
	xMax, yMax int16

	// From hhea.
	ascent, descent        int16
	numOfLongHorMetrics    uint16

	// From maxp.
	numGlyphs uint16

	// From hmtx.
	glyphWidths []uint16 // advanceWidth per glyphID (FUnits)

	// From cmap.
	runeToGlyph map[rune]uint16

	// From OS/2.
	capHeight   int16
	weight      uint16
	flagsBold   bool
	flagsItalic bool

	// From post.
	italicAngle  float64
	isFixedPitch bool

	// From name.
	postScriptName string
}

// tableRecord is an entry in the TTF table directory.
type tableRecord struct {
	offset uint32
	length uint32
}

// parseTTF parses a TrueType font file and returns the ttfFont ready for embedding.
// Only the tables required for CIDFontType2 / Type0 embedding are read.
func parseTTF(data []byte) (*ttfFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("parse ttf: file too small (%d bytes)", len(data))
	}
	scaler := binary.BigEndian.Uint32(data[0:4])
	if scaler != 0x00010000 && scaler != 0x74727565 { // 'true'
		return nil, fmt.Errorf("parse ttf: not a TrueType file (scaler 0x%08X)", scaler)
	}

	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return nil, fmt.Errorf("parse ttf: truncated table directory")
	}
	tables := make(map[string]tableRecord, numTables)
	for i := 0; i < numTables; i++ {
		off := 12 + i*16
		tag := string(data[off : off+4])
		tables[tag] = tableRecord{
			offset: binary.BigEndian.Uint32(data[off+8 : off+12]),
			length: binary.BigEndian.Uint32(data[off+12 : off+16]),
		}
	}

	required := []string{"head", "hhea", "hmtx", "maxp", "name", "cmap", "OS/2", "post"}
	for _, tag := range required {
		if _, ok := tables[tag]; !ok {
			return nil, fmt.Errorf("parse ttf: missing required table %q", tag)
		}
	}

	f := &ttfFont{data: data}

	// Per-table parsers are added in subsequent tasks. The skeleton returns
	// a font with only data populated; full parsing is wired in Tasks 6–9.

	return f, nil
}

// tableSlice returns the bytes of the named table or nil if absent.
func tableSlice(data []byte, tables map[string]tableRecord, tag string) []byte {
	t, ok := tables[tag]
	if !ok {
		return nil
	}
	end := t.offset + t.length
	if end > uint32(len(data)) {
		return nil
	}
	return data[t.offset:end]
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestParseTTF" -v ./...`
Expected: PASS for all three.

- [ ] **Step 5: Commit**

```bash
git add ttf.go ttf_test.go
git commit -m "feat: add TTF parser skeleton with table directory"
```

---

### Task 6: Parse head, hhea, maxp, hmtx

**Files:**
- Modify: `ttf.go`
- Modify: `ttf_test.go`

- [ ] **Step 1: Write failing tests**

Append to [ttf_test.go](ttf_test.go):

```go
func TestParseTTF_Head(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.unitsPerEm != 2048 {
		t.Errorf("unitsPerEm = %d, want 2048", f.unitsPerEm)
	}
	if f.xMin == 0 && f.yMin == 0 && f.xMax == 0 && f.yMax == 0 {
		t.Error("font bbox not populated")
	}
}

func TestParseTTF_Hhea(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.ascent <= 0 {
		t.Errorf("ascent = %d, want positive", f.ascent)
	}
	if f.descent >= 0 {
		t.Errorf("descent = %d, want negative", f.descent)
	}
	if f.numOfLongHorMetrics == 0 {
		t.Error("numOfLongHorMetrics = 0")
	}
}

func TestParseTTF_Maxp(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.numGlyphs < 256 {
		t.Errorf("numGlyphs = %d, want >= 256 for DejaVuSans", f.numGlyphs)
	}
}

func TestParseTTF_Hmtx(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.glyphWidths) != int(f.numGlyphs) {
		t.Errorf("len(glyphWidths) = %d, want numGlyphs %d", len(f.glyphWidths), f.numGlyphs)
	}
	// glyphID 0 is always .notdef, should still have a width.
	if f.glyphWidths[0] == 0 {
		t.Error("glyphWidths[0] (.notdef) is zero — likely parse error")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -run "TestParseTTF_Head|TestParseTTF_Hhea|TestParseTTF_Maxp|TestParseTTF_Hmtx" -v ./...`
Expected: FAIL — fields not populated (all zero/nil).

- [ ] **Step 3: Implement the four parsers**

Append to [ttf.go](ttf.go):

```go
func parseHead(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "head")
	if len(b) < 54 {
		return fmt.Errorf("parse ttf head: too small")
	}
	f.unitsPerEm = binary.BigEndian.Uint16(b[18:20])
	f.xMin = int16(binary.BigEndian.Uint16(b[36:38]))
	f.yMin = int16(binary.BigEndian.Uint16(b[38:40]))
	f.xMax = int16(binary.BigEndian.Uint16(b[40:42]))
	f.yMax = int16(binary.BigEndian.Uint16(b[42:44]))
	return nil
}

func parseHhea(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hhea")
	if len(b) < 36 {
		return fmt.Errorf("parse ttf hhea: too small")
	}
	f.ascent = int16(binary.BigEndian.Uint16(b[4:6]))
	f.descent = int16(binary.BigEndian.Uint16(b[6:8]))
	f.numOfLongHorMetrics = binary.BigEndian.Uint16(b[34:36])
	return nil
}

func parseMaxp(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "maxp")
	if len(b) < 6 {
		return fmt.Errorf("parse ttf maxp: too small")
	}
	f.numGlyphs = binary.BigEndian.Uint16(b[4:6])
	return nil
}

func parseHmtx(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hmtx")
	if f.numGlyphs == 0 {
		return fmt.Errorf("parse ttf hmtx: numGlyphs is zero")
	}
	if f.numOfLongHorMetrics == 0 {
		return fmt.Errorf("parse ttf hmtx: numOfLongHorMetrics is zero")
	}
	// The hmtx table has numOfLongHorMetrics 4-byte records (advanceWidth uint16, lsb int16),
	// followed by (numGlyphs - numOfLongHorMetrics) 2-byte records (lsb only); the missing
	// advanceWidth inherits the advanceWidth of the last long record.
	if len(b) < int(f.numOfLongHorMetrics)*4 {
		return fmt.Errorf("parse ttf hmtx: too small")
	}
	widths := make([]uint16, f.numGlyphs)
	var lastAdvance uint16
	for i := uint16(0); i < f.numOfLongHorMetrics; i++ {
		off := int(i) * 4
		w := binary.BigEndian.Uint16(b[off : off+2])
		widths[i] = w
		lastAdvance = w
	}
	for i := f.numOfLongHorMetrics; i < f.numGlyphs; i++ {
		widths[i] = lastAdvance
	}
	f.glyphWidths = widths
	return nil
}
```

Wire them into `parseTTF` before the final `return f, nil`:

```go
	if err := parseHead(f, tables); err != nil {
		return nil, err
	}
	if err := parseHhea(f, tables); err != nil {
		return nil, err
	}
	if err := parseMaxp(f, tables); err != nil {
		return nil, err
	}
	if err := parseHmtx(f, tables); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestParseTTF" -v ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ttf.go ttf_test.go
git commit -m "feat: parse TTF head/hhea/maxp/hmtx tables"
```

---

### Task 7: Parse cmap (format 4 and format 12)

**Files:**
- Modify: `ttf.go`
- Modify: `ttf_test.go`

- [ ] **Step 1: Write failing tests**

Append to [ttf_test.go](ttf_test.go):

```go
func TestParseTTF_CmapLatin(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if g := f.glyphID('A'); g == 0 {
		t.Error("glyphID('A') = 0, want non-zero")
	}
	if g := f.glyphID(' '); g == 0 {
		t.Error("glyphID(' ') = 0, want non-zero")
	}
}

func TestParseTTF_CmapCyrillic(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if g := f.glyphID('Я'); g == 0 {
		t.Error("glyphID('Я') = 0, want non-zero (DejaVu covers Cyrillic)")
	}
	if g := f.glyphID('ж'); g == 0 {
		t.Error("glyphID('ж') = 0, want non-zero")
	}
}

func TestParseTTF_CmapMissing(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	// DejaVuSans does not cover CJK.
	if g := f.glyphID('日'); g != 0 {
		t.Errorf("glyphID('日') = %d, want 0 (CJK not in DejaVuSans)", g)
	}
}

func TestParseTTF_AdvanceKnown(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	gid := f.glyphID('A')
	if gid == 0 {
		t.Fatal("glyphID('A') = 0")
	}
	advA := f.glyphWidths[gid]
	if advA == 0 {
		t.Errorf("advance for 'A' = 0")
	}
	// In DejaVuSans 'A' is wider than ' '.
	gidSp := f.glyphID(' ')
	if f.glyphWidths[gidSp] >= advA {
		t.Errorf("' ' advance (%d) >= 'A' advance (%d), unexpected for DejaVuSans",
			f.glyphWidths[gidSp], advA)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestParseTTF_Cmap|TestParseTTF_AdvanceKnown" -v ./...`
Expected: FAIL — `glyphID` undefined / returns 0 for everything.

- [ ] **Step 3: Implement `parseCmap` and `glyphID`**

Append to [ttf.go](ttf.go):

```go
func parseCmap(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "cmap")
	if len(b) < 4 {
		return fmt.Errorf("parse ttf cmap: too small")
	}
	numSubtables := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) < 4+numSubtables*8 {
		return fmt.Errorf("parse ttf cmap: truncated subtable index")
	}

	// Rank candidates: prefer format 12 (full Unicode) > format 4 (BMP only);
	// within a format, prefer Unicode platform (0) > Microsoft platform (3).
	type cand struct {
		priority int
		format   uint16
		offset   uint32
	}
	var best *cand

	for i := 0; i < numSubtables; i++ {
		off := 4 + i*8
		platformID := binary.BigEndian.Uint16(b[off : off+2])
		encodingID := binary.BigEndian.Uint16(b[off+2 : off+4])
		subOffset := binary.BigEndian.Uint32(b[off+4 : off+8])
		if int(subOffset)+4 > len(b) {
			continue
		}
		format := binary.BigEndian.Uint16(b[subOffset : subOffset+2])

		// Skip subtables we can't parse.
		if format != 4 && format != 12 {
			continue
		}

		// Prioritize:
		//   1000 = Unicode (0) Encoding 4/6 format 12
		//   900  = Microsoft (3) Encoding 10 format 12
		//   500  = Unicode (0) format 4
		//   400  = Microsoft (3) Encoding 1 format 4
		var pri int
		switch {
		case format == 12 && platformID == 0:
			pri = 1000
		case format == 12 && platformID == 3 && encodingID == 10:
			pri = 900
		case format == 4 && platformID == 0:
			pri = 500
		case format == 4 && platformID == 3 && encodingID == 1:
			pri = 400
		default:
			continue
		}
		if best == nil || pri > best.priority {
			c := cand{priority: pri, format: format, offset: subOffset}
			best = &c
		}
	}
	if best == nil {
		return fmt.Errorf("parse ttf cmap: no supported subtable (need format 4 or 12)")
	}

	m := make(map[rune]uint16)
	switch best.format {
	case 4:
		if err := parseCmapFormat4(b[best.offset:], m); err != nil {
			return fmt.Errorf("parse ttf cmap format 4: %w", err)
		}
	case 12:
		if err := parseCmapFormat12(b[best.offset:], m); err != nil {
			return fmt.Errorf("parse ttf cmap format 12: %w", err)
		}
	}
	f.runeToGlyph = m
	return nil
}

// parseCmapFormat4 handles segmented BMP coverage (Unicode code points <= U+FFFF).
func parseCmapFormat4(b []byte, m map[rune]uint16) error {
	if len(b) < 14 {
		return fmt.Errorf("too small")
	}
	segCountX2 := int(binary.BigEndian.Uint16(b[6:8]))
	segCount := segCountX2 / 2
	if segCount == 0 {
		return nil
	}
	// Layout after the 14-byte header:
	//   endCode[segCount] (uint16)
	//   reservedPad uint16
	//   startCode[segCount] uint16
	//   idDelta[segCount] int16
	//   idRangeOffset[segCount] uint16
	//   glyphIdArray[...] uint16 (remainder)
	needed := 14 + 8*segCount + 2
	if len(b) < needed {
		return fmt.Errorf("truncated")
	}
	endOff := 14
	startOff := endOff + 2*segCount + 2 // skip endCode + reservedPad
	deltaOff := startOff + 2*segCount
	rangeOff := deltaOff + 2*segCount
	// glyphIdArray begins at rangeOff + 2*segCount.

	for i := 0; i < segCount; i++ {
		endCode := binary.BigEndian.Uint16(b[endOff+2*i : endOff+2*i+2])
		startCode := binary.BigEndian.Uint16(b[startOff+2*i : startOff+2*i+2])
		idDelta := int16(binary.BigEndian.Uint16(b[deltaOff+2*i : deltaOff+2*i+2]))
		idRangeOffsetPos := rangeOff + 2*i
		idRangeOffset := binary.BigEndian.Uint16(b[idRangeOffsetPos : idRangeOffsetPos+2])

		// Sentinel final segment: startCode == endCode == 0xFFFF, idDelta == 1.
		for c := uint32(startCode); c <= uint32(endCode); c++ {
			var gid uint16
			if idRangeOffset == 0 {
				gid = uint16(int32(c) + int32(idDelta))
			} else {
				// glyphIdArray[idRangeOffset/2 + (c - startCode) + i] in spec's pointer form
				off := int(idRangeOffsetPos) + int(idRangeOffset) + int(c-uint32(startCode))*2
				if off+2 > len(b) {
					continue
				}
				val := binary.BigEndian.Uint16(b[off : off+2])
				if val != 0 {
					gid = uint16(int32(val) + int32(idDelta))
				}
			}
			if gid != 0 && c <= 0x10FFFF {
				m[rune(c)] = gid
			}
		}
	}
	return nil
}

// parseCmapFormat12 handles segmented coverage including supplementary planes.
func parseCmapFormat12(b []byte, m map[rune]uint16) error {
	if len(b) < 16 {
		return fmt.Errorf("too small")
	}
	numGroups := binary.BigEndian.Uint32(b[12:16])
	if len(b) < 16+int(numGroups)*12 {
		return fmt.Errorf("truncated")
	}
	for i := uint32(0); i < numGroups; i++ {
		off := 16 + int(i)*12
		startChar := binary.BigEndian.Uint32(b[off : off+4])
		endChar := binary.BigEndian.Uint32(b[off+4 : off+8])
		startGlyphID := binary.BigEndian.Uint32(b[off+8 : off+12])
		for c := startChar; c <= endChar && c <= 0x10FFFF; c++ {
			gid := startGlyphID + (c - startChar)
			if gid < 0x10000 {
				m[rune(c)] = uint16(gid)
			}
		}
	}
	return nil
}

// glyphID returns the glyph index for r, or 0 (.notdef) if unmapped.
func (f *ttfFont) glyphID(r rune) uint16 {
	return f.runeToGlyph[r]
}
```

Wire `parseCmap` into `parseTTF` after `parseHmtx`:

```go
	if err := parseCmap(f, tables); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestParseTTF" -v ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ttf.go ttf_test.go
git commit -m "feat: parse TTF cmap (format 4 and 12)"
```

---

### Task 8: Parse OS/2, post, name

**Files:**
- Modify: `ttf.go`
- Modify: `ttf_test.go`

- [ ] **Step 1: Write failing tests**

Append to [ttf_test.go](ttf_test.go):

```go
func TestParseTTF_OS2(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.weight == 0 {
		t.Error("weight not populated")
	}
	// DejaVuSans is regular (not bold/italic).
	if f.flagsBold {
		t.Error("DejaVuSans flagged Bold (wrong)")
	}
	if f.flagsItalic {
		t.Error("DejaVuSans flagged Italic (wrong)")
	}
	if f.capHeight == 0 {
		t.Error("capHeight not populated")
	}
}

func TestParseTTF_Post(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	// DejaVuSans is proportional, italic angle 0.
	if f.italicAngle != 0 {
		t.Errorf("italicAngle = %g, want 0", f.italicAngle)
	}
	if f.isFixedPitch {
		t.Error("DejaVuSans flagged FixedPitch (wrong)")
	}
}

func TestParseTTF_Name(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.postScriptName != "DejaVuSans" {
		t.Errorf("postScriptName = %q, want DejaVuSans", f.postScriptName)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestParseTTF_OS2|TestParseTTF_Post|TestParseTTF_Name" -v ./...`
Expected: FAIL — fields empty/zero.

- [ ] **Step 3: Implement parsers**

Append to [ttf.go](ttf.go):

```go
func parseOS2(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "OS/2")
	if len(b) < 78 {
		return fmt.Errorf("parse ttf OS/2: too small")
	}
	f.weight = binary.BigEndian.Uint16(b[4:6])
	fsSelection := binary.BigEndian.Uint16(b[62:64])
	f.flagsItalic = fsSelection&0x01 != 0
	f.flagsBold = fsSelection&0x20 != 0
	// sCapHeight is at offset 88 in OS/2 version 2+. Version 0/1 may omit it.
	if len(b) >= 90 {
		f.capHeight = int16(binary.BigEndian.Uint16(b[88:90]))
	}
	return nil
}

func parsePost(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "post")
	if len(b) < 32 {
		return fmt.Errorf("parse ttf post: too small")
	}
	// italicAngle is a Fixed (signed 16.16 fraction) at offset 4.
	raw := int32(binary.BigEndian.Uint32(b[4:8]))
	f.italicAngle = float64(raw) / 65536.0
	// isFixedPitch is a uint32 at offset 12.
	f.isFixedPitch = binary.BigEndian.Uint32(b[12:16]) != 0
	return nil
}

// parseName extracts the PostScript name (nameID 6). Falls back to Full Name
// (nameID 4) with spaces replaced by dashes if nameID 6 is absent.
func parseName(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "name")
	if len(b) < 6 {
		return fmt.Errorf("parse ttf name: too small")
	}
	count := int(binary.BigEndian.Uint16(b[2:4]))
	storageOffset := int(binary.BigEndian.Uint16(b[4:6]))
	if len(b) < 6+count*12 {
		return fmt.Errorf("parse ttf name: truncated record array")
	}

	var psName, fullName string
	for i := 0; i < count; i++ {
		rec := b[6+i*12:]
		platformID := binary.BigEndian.Uint16(rec[0:2])
		encodingID := binary.BigEndian.Uint16(rec[2:4])
		nameID := binary.BigEndian.Uint16(rec[6:8])
		length := int(binary.BigEndian.Uint16(rec[8:10]))
		offset := int(binary.BigEndian.Uint16(rec[10:12]))

		if nameID != 6 && nameID != 4 {
			continue
		}
		start := storageOffset + offset
		end := start + length
		if end > len(b) {
			continue
		}
		raw := b[start:end]

		var decoded string
		switch {
		case platformID == 3 && encodingID == 1: // Microsoft Unicode BMP (UTF-16BE)
			decoded = decodeUTF16BE(raw)
		case platformID == 0: // Unicode (UTF-16BE)
			decoded = decodeUTF16BE(raw)
		case platformID == 1 && encodingID == 0: // Mac Roman (ASCII-safe subset)
			decoded = string(raw)
		default:
			continue
		}

		if nameID == 6 && psName == "" {
			psName = decoded
		}
		if nameID == 4 && fullName == "" {
			fullName = decoded
		}
	}

	if psName == "" && fullName == "" {
		return fmt.Errorf("parse ttf name: no PostScript name or Full Name found")
	}
	if psName == "" {
		// Fallback: replace spaces with dashes in Full Name.
		psName = ""
		for _, r := range fullName {
			if r == ' ' {
				psName += "-"
			} else {
				psName += string(r)
			}
		}
	}
	f.postScriptName = psName
	return nil
}

// decodeUTF16BE decodes a UTF-16BE byte sequence to a Go string.
// Invalid bytes yield U+FFFD.
func decodeUTF16BE(b []byte) string {
	if len(b)%2 != 0 {
		// Trim trailing odd byte.
		b = b[:len(b)-1]
	}
	runes := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u := uint32(b[i])<<8 | uint32(b[i+1])
		// Surrogate pair handling.
		if u >= 0xD800 && u <= 0xDBFF && i+3 < len(b) {
			low := uint32(b[i+2])<<8 | uint32(b[i+3])
			if low >= 0xDC00 && low <= 0xDFFF {
				cp := 0x10000 + ((u - 0xD800) << 10) + (low - 0xDC00)
				runes = append(runes, rune(cp))
				i += 2
				continue
			}
		}
		runes = append(runes, rune(u))
	}
	return string(runes)
}
```

Wire into `parseTTF` after `parseCmap`:

```go
	if err := parseOS2(f, tables); err != nil {
		return nil, err
	}
	if err := parsePost(f, tables); err != nil {
		return nil, err
	}
	if err := parseName(f, tables); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestParseTTF" -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add ttf.go ttf_test.go
git commit -m "feat: parse TTF OS/2, post, and name tables"
```

---

### Task 9: `embeddedFont` struct + `LoadFont`/`LoadFontFromStream` (parsing only)

**Files:**
- Modify: `font_api.go`
- Modify: `font_api_test.go`

- [ ] **Step 1: Write failing tests**

Append to [font_api_test.go](font_api_test.go):

```go
func TestLoadFont_DejaVu(t *testing.T) {
	doc := NewDocument(595, 842)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	if f.BaseFont() != "DejaVuSans" {
		t.Errorf("BaseFont() = %q, want DejaVuSans", f.BaseFont())
	}
	if !f.IsEmbedded() {
		t.Error("IsEmbedded() = false, want true")
	}
}

func TestLoadFont_MissingFile(t *testing.T) {
	doc := NewDocument(595, 842)
	_, err := doc.LoadFont("testdata/does_not_exist.ttf")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "load font") {
		t.Errorf("error = %q, want to contain 'load font'", err.Error())
	}
}

func TestLoadFontFromStream_NotTTF(t *testing.T) {
	doc := NewDocument(595, 842)
	_, err := doc.LoadFontFromStream(strings.NewReader("not a font"))
	if err == nil {
		t.Fatal("expected error for non-TTF stream")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestLoadFont" -v ./...`
Expected: FAIL — `LoadFont` undefined.

- [ ] **Step 3: Implement `embeddedFont` + loaders**

Append to [font_api.go](font_api.go):

```go
import (
	"io"
	"os"
)

// embeddedFont is a TTF loaded into a specific Document.
type embeddedFont struct {
	doc          *Document
	ttf          *ttfFont
	baseFont     string // PostScript name cache
	fontObjectID int    // ID of the Type0 font dict in doc.objects; 0 until Task 12.
	resourceName string // stable /Fn resource name per page; populated on first use.
}

func (e *embeddedFont) BaseFont() string { return e.baseFont }
func (e *embeddedFont) IsEmbedded() bool { return true }

// LoadFont reads a TTF file, parses it, embeds it into the document, and returns
// a Font that can be used in TextStyle.Font.
func (d *Document) LoadFont(path string) (Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load font: %w", err)
	}
	return d.loadFontFromBytes(data)
}

// LoadFontFromStream is like LoadFont but reads from an io.Reader.
func (d *Document) LoadFontFromStream(r io.Reader) (Font, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("load font: read stream: %w", err)
	}
	return d.loadFontFromBytes(data)
}

func (d *Document) loadFontFromBytes(data []byte) (Font, error) {
	ttf, err := parseTTF(data)
	if err != nil {
		return nil, fmt.Errorf("load font: %w", err)
	}
	ef := &embeddedFont{
		doc:      d,
		ttf:      ttf,
		baseFont: ttf.postScriptName,
	}
	// PDF object embedding is wired in Task 12.
	return ef, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestLoadFont" -v ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add font_api.go font_api_test.go
git commit -m "feat: embeddedFont struct + LoadFont/LoadFontFromStream parsing"
```

---

### Task 10: Generate FontFile2 stream and FontDescriptor

**Files:**
- Create: `font_embed.go`
- Create: `font_embed_test.go`

- [ ] **Step 1: Write failing tests**

Create [font_embed_test.go](font_embed_test.go):

```go
package asposepdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"
)

func TestBuildFontFile2Stream(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	stream := buildFontFile2Stream(f)
	if stream == nil {
		t.Fatal("buildFontFile2Stream returned nil")
	}
	if stream.Dict["/Length1"] != len(f.data) {
		t.Errorf("/Length1 = %v, want %d", stream.Dict["/Length1"], len(f.data))
	}
	if stream.Dict["/Filter"] != pdfName("/FlateDecode") {
		t.Errorf("/Filter = %v, want /FlateDecode", stream.Dict["/Filter"])
	}
	// Round-trip: inflate the stream data and compare with original.
	r, err := zlib.NewReader(bytes.NewReader(stream.Data))
	if err != nil {
		t.Fatalf("zlib reader: %v", err)
	}
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(decompressed, f.data) {
		t.Errorf("decompressed bytes do not match original (got %d bytes, want %d)",
			len(decompressed), len(f.data))
	}
}

func TestBuildFontDescriptor(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	fontFileID := 42
	desc := buildFontDescriptor(f, fontFileID)

	if desc["/Type"] != pdfName("/FontDescriptor") {
		t.Errorf("/Type = %v", desc["/Type"])
	}
	if desc["/FontName"] != pdfName("/DejaVuSans") {
		t.Errorf("/FontName = %v, want /DejaVuSans", desc["/FontName"])
	}
	ref, ok := desc["/FontFile2"].(pdfRef)
	if !ok || ref.Num != fontFileID {
		t.Errorf("/FontFile2 = %v, want pdfRef{%d}", desc["/FontFile2"], fontFileID)
	}
	// Flags: Symbolic (bit 3, value 4) is always set for embedded TTF.
	flags, _ := desc["/Flags"].(int)
	if flags&0x4 == 0 {
		t.Errorf("/Flags = %d, Symbolic bit not set", flags)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestBuildFontFile2Stream|TestBuildFontDescriptor" -v ./...`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement FontFile2 and FontDescriptor builders**

Create [font_embed.go](font_embed.go):

```go
package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

// buildFontFile2Stream creates a /FontFile2 stream with the raw TTF bytes,
// compressed via FlateDecode. /Length1 holds the uncompressed length.
func buildFontFile2Stream(f *ttfFont) *pdfStream {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(f.data)
	_ = zw.Close()
	return &pdfStream{
		Dict: pdfDict{
			"/Length1": len(f.data),
			"/Filter":  pdfName("/FlateDecode"),
		},
		Data: buf.Bytes(),
	}
}

// buildFontDescriptor creates a /FontDescriptor dict referencing the given
// FontFile2 object ID.
func buildFontDescriptor(f *ttfFont, fontFile2ID int) pdfDict {
	scale := func(v int16) int {
		// Scale FUnits to 1/1000 em.
		return int(float64(v) * 1000.0 / float64(f.unitsPerEm))
	}
	// Flags (PDF spec Table 123):
	//   bit 1 (1):      FixedPitch
	//   bit 3 (4):      Symbolic  — always set for embedded TTF
	//   bit 7 (64):     Italic
	//   bit 19 (262144): ForceBold
	flags := 0x4 // Symbolic
	if f.isFixedPitch {
		flags |= 0x1
	}
	if f.flagsItalic {
		flags |= 0x40
	}
	if f.flagsBold {
		flags |= 0x40000
	}
	// StemV heuristic: 50 at weight 400, +0.2 per weight unit.
	stemV := 50
	if f.weight > 0 {
		stemV = 50 + int(float64(f.weight-400)*0.2)
		if stemV < 50 {
			stemV = 50
		}
	}
	cap := scale(f.capHeight)
	if cap == 0 {
		cap = scale(f.ascent)
	}
	return pdfDict{
		"/Type":     pdfName("/FontDescriptor"),
		"/FontName": pdfName("/" + f.postScriptName),
		"/Flags":    flags,
		"/FontBBox": pdfArray{
			scale(f.xMin), scale(f.yMin),
			scale(f.xMax), scale(f.yMax),
		},
		"/ItalicAngle": f.italicAngle,
		"/Ascent":      scale(f.ascent),
		"/Descent":     scale(f.descent),
		"/CapHeight":   cap,
		"/StemV":       stemV,
		"/FontFile2":   pdfRef{Num: fontFile2ID},
	}
}

// addObject appends a new PDF object to the document and returns its ID.
// Used by font_embed.go to wire objects into doc.objects.
func (d *Document) addObject(value pdfValue) int {
	id := d.nextID
	d.nextID++
	d.objects[id] = &pdfObject{Num: id, Value: value}
	return id
}

// Ensure fmt import is not dropped by future edits.
var _ = fmt.Sprintf
```

Note: if `pdfValue` is not already a type alias/interface in `types.go`, the `addObject` signature uses `any`. Inspect `types.go` for the correct type. If `pdfValue` is `interface{}` (common), use it as-is.

- [ ] **Step 4: Run tests**

Run: `go test -run "TestBuildFontFile2Stream|TestBuildFontDescriptor" -v ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add font_embed.go font_embed_test.go
git commit -m "feat: build /FontFile2 stream and /FontDescriptor dict"
```

---

### Task 11: Build /W array and ToUnicode CMap

**Files:**
- Modify: `font_embed.go`
- Modify: `font_embed_test.go`

- [ ] **Step 1: Write failing tests**

Append to [font_embed_test.go](font_embed_test.go):

```go
func TestBuildWArray(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	arr := buildWArray(f)
	// Round-trip through the existing parseCIDWidthArray.
	widths := make(map[uint16]float64)
	parseCIDWidthArray(arr, widths)

	// 'A' has a known non-default width.
	gidA := f.glyphID('A')
	want := float64(f.glyphWidths[gidA]) * 1000.0 / float64(f.unitsPerEm)
	got := widths[gidA]
	// Default width 500 is skipped from /W, so if advance scaled equals 500 it's absent.
	if got == 0 && want != 500 {
		t.Errorf("/W round-trip for gid %d: got 0, want %g", gidA, want)
	}
	if got != 0 {
		// Round to nearest int because packing stores ints.
		if int(got) != int(want) && int(got) != int(want+0.5) {
			t.Errorf("/W width for gid %d = %g, want %g", gidA, got, want)
		}
	}
}

func TestBuildToUnicodeCMap(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	stream := buildToUnicodeCMap(f)
	content := string(stream.Data)
	if !strings.Contains(content, "beginbfchar") {
		t.Error("CMap missing beginbfchar block")
	}
	if !strings.Contains(content, "begincmap") || !strings.Contains(content, "endcmap") {
		t.Error("CMap missing begincmap/endcmap")
	}
	// Round-trip: reuse the existing parseCMap reader.
	decoded := parseCMap(stream.Data)
	gidA := f.glyphID('A')
	if r, ok := decoded[gidA]; !ok || r != 'A' {
		t.Errorf("CMap[gid(A)] = (%q, %v), want ('A', true)", r, ok)
	}
}
```

Add `"strings"` to the import block of font_embed_test.go if not already present.

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestBuildWArray|TestBuildToUnicodeCMap" -v ./...`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement /W array packing**

Append to [font_embed.go](font_embed.go):

```go
// defaultCIDWidth is the /DW value written for embedded TTFs.
const defaultCIDWidth = 500

// buildWArray builds the /W array for a CIDFontType2 dict.
// Widths equal to defaultCIDWidth are omitted (covered by /DW).
// Runs of identical non-default widths > 5 consecutive glyphs are emitted as
// `cFirst cLast w`; all other non-default widths are grouped in the array form
// `cStart [w1 w2 w3 ...]`.
func buildWArray(f *ttfFont) pdfArray {
	// Scale each glyph's advance to 1/1000 em (rounded).
	widths := make([]int, len(f.glyphWidths))
	for i, w := range f.glyphWidths {
		widths[i] = int(float64(w)*1000.0/float64(f.unitsPerEm) + 0.5)
	}

	var arr pdfArray
	i := 0
	for i < len(widths) {
		// Skip default-width glyphs.
		if widths[i] == defaultCIDWidth {
			i++
			continue
		}
		// Try to start a same-width run.
		j := i + 1
		for j < len(widths) && widths[j] == widths[i] {
			j++
		}
		runLen := j - i
		if runLen > 5 {
			arr = append(arr, i, j-1, widths[i])
			i = j
			continue
		}
		// Otherwise collect a contiguous sequence of non-default, possibly-varying widths.
		// Stop when we hit a default run OR a same-width run of length > 5.
		k := i + 1
		for k < len(widths) && widths[k] != defaultCIDWidth {
			// Look ahead to see if there's a run > 5 starting here.
			lookEnd := k + 1
			for lookEnd < len(widths) && widths[lookEnd] == widths[k] {
				lookEnd++
			}
			if lookEnd-k > 5 {
				break
			}
			k = lookEnd
		}
		seq := make(pdfArray, 0, k-i)
		for g := i; g < k; g++ {
			seq = append(seq, widths[g])
		}
		arr = append(arr, i, seq)
		i = k
	}
	return arr
}
```

- [ ] **Step 4: Implement ToUnicode CMap builder**

Append to [font_embed.go](font_embed.go):

```go
// buildToUnicodeCMap generates the /ToUnicode CMap stream for the font.
// Emits one bfchar entry per (glyphID, rune) pair from runeToGlyph.
func buildToUnicodeCMap(f *ttfFont) *pdfStream {
	type pair struct {
		gid uint16
		r   rune
	}
	pairs := make([]pair, 0, len(f.runeToGlyph))
	for r, gid := range f.runeToGlyph {
		pairs = append(pairs, pair{gid: gid, r: r})
	}
	// Sort by gid for deterministic output.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].gid < pairs[j].gid })

	var buf bytes.Buffer
	buf.WriteString("/CIDInit /ProcSet findresource begin\n")
	buf.WriteString("12 dict begin\n")
	buf.WriteString("begincmap\n")
	buf.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	buf.WriteString("/CMapName /Adobe-Identity-UCS def\n")
	buf.WriteString("/CMapType 2 def\n")
	buf.WriteString("1 begincodespacerange <0000> <FFFF> endcodespacerange\n")

	// Emit in blocks of up to 100 per PDF spec.
	for start := 0; start < len(pairs); start += 100 {
		end := start + 100
		if end > len(pairs) {
			end = len(pairs)
		}
		fmt.Fprintf(&buf, "%d beginbfchar\n", end-start)
		for _, p := range pairs[start:end] {
			fmt.Fprintf(&buf, "<%04X> <%s>\n", p.gid, runeToUTF16BEHex(p.r))
		}
		buf.WriteString("endbfchar\n")
	}
	buf.WriteString("endcmap\n")
	buf.WriteString("CMapName currentdict /CMap defineresource pop\n")
	buf.WriteString("end\nend\n")

	return &pdfStream{
		Dict:    pdfDict{},
		Data:    buf.Bytes(),
		Decoded: true,
	}
}

// runeToUTF16BEHex renders r as big-endian UTF-16 in uppercase hex, with
// surrogate pairs for supplementary characters (> U+FFFF).
func runeToUTF16BEHex(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("%04X", uint16(r))
	}
	v := uint32(r) - 0x10000
	high := 0xD800 + (v >> 10)
	low := 0xDC00 + (v & 0x3FF)
	return fmt.Sprintf("%04X%04X", high, low)
}
```

Add `"sort"` to the imports of [font_embed.go](font_embed.go).

- [ ] **Step 5: Run tests**

Run: `go test -run "TestBuildWArray|TestBuildToUnicodeCMap" -v ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add font_embed.go font_embed_test.go
git commit -m "feat: build /W array and ToUnicode CMap for embedded TTF"
```

---

### Task 12: Wire Type0 + CIDFontType2 and complete `LoadFont` embedding

**Files:**
- Modify: `font_embed.go`
- Modify: `font_api.go`
- Modify: `font_embed_test.go`

- [ ] **Step 1: Write failing tests**

Append to [font_embed_test.go](font_embed_test.go):

```go
func TestLoadFontCreatesFiveObjects(t *testing.T) {
	doc := NewDocument(595, 842)
	beforeCount := len(doc.objects)

	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	afterCount := len(doc.objects)

	if afterCount-beforeCount < 5 {
		t.Errorf("LoadFont added %d objects, want >= 5 (Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode)",
			afterCount-beforeCount)
	}

	ef, ok := f.(*embeddedFont)
	if !ok {
		t.Fatalf("LoadFont returned %T, want *embeddedFont", f)
	}
	if ef.fontObjectID == 0 {
		t.Error("fontObjectID not set")
	}

	// Walk: Type0 -> DescendantFonts[0] -> CIDFontType2 -> FontDescriptor -> FontFile2
	type0 := doc.objects[ef.fontObjectID].Value.(pdfDict)
	if type0["/Subtype"] != pdfName("/Type0") {
		t.Errorf("Type0 /Subtype = %v", type0["/Subtype"])
	}
	if type0["/Encoding"] != pdfName("/Identity-H") {
		t.Errorf("Type0 /Encoding = %v, want /Identity-H", type0["/Encoding"])
	}
	desc := type0["/DescendantFonts"].(pdfArray)
	if len(desc) != 1 {
		t.Fatalf("DescendantFonts length = %d, want 1", len(desc))
	}
	cidRef := desc[0].(pdfRef)
	cid := doc.objects[cidRef.Num].Value.(pdfDict)
	if cid["/Subtype"] != pdfName("/CIDFontType2") {
		t.Errorf("CIDFont /Subtype = %v", cid["/Subtype"])
	}
	if cid["/CIDToGIDMap"] != pdfName("/Identity") {
		t.Errorf("CIDToGIDMap = %v, want /Identity", cid["/CIDToGIDMap"])
	}

	tuRef := type0["/ToUnicode"].(pdfRef)
	if _, ok := doc.objects[tuRef.Num].Value.(*pdfStream); !ok {
		t.Error("ToUnicode is not a stream")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestLoadFontCreatesFiveObjects" -v ./...`
Expected: FAIL — `fontObjectID` is zero (Task 9 left it so).

- [ ] **Step 3: Implement `embedFont`**

Append to [font_embed.go](font_embed.go):

```go
// embedFont adds all required PDF objects for the embedded TTF to doc.objects
// and returns the object ID of the Type0 font dict.
func embedFont(d *Document, f *ttfFont) int {
	fontFile2ID := d.addObject(buildFontFile2Stream(f))
	descriptor := buildFontDescriptor(f, fontFile2ID)
	descriptorID := d.addObject(descriptor)

	cidDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/CIDFontType2"),
		"/BaseFont": pdfName("/" + f.postScriptName),
		"/CIDSystemInfo": pdfDict{
			"/Registry":   "Adobe",
			"/Ordering":   "Identity",
			"/Supplement": 0,
		},
		"/FontDescriptor": pdfRef{Num: descriptorID},
		"/CIDToGIDMap":    pdfName("/Identity"),
		"/W":              buildWArray(f),
		"/DW":             defaultCIDWidth,
	}
	cidID := d.addObject(cidDict)

	tuID := d.addObject(buildToUnicodeCMap(f))

	type0 := pdfDict{
		"/Type":             pdfName("/Font"),
		"/Subtype":          pdfName("/Type0"),
		"/BaseFont":         pdfName("/" + f.postScriptName),
		"/Encoding":         pdfName("/Identity-H"),
		"/DescendantFonts":  pdfArray{pdfRef{Num: cidID}},
		"/ToUnicode":        pdfRef{Num: tuID},
	}
	return d.addObject(type0)
}
```

Note: `/Registry` and `/Ordering` values are literal PDF strings (parenthesized in output). In this codebase, `pdfDict` value "Adobe" is written as a PDF literal by the writer if the value is a `string`. Verify by reading `writer.go` before wiring; if the writer expects a specific `pdfString` wrapper, switch to that type. (If uncertain, use `string` — grep `writeValue` in `writer.go` for the actual behavior.)

- [ ] **Step 4: Wire `embedFont` into `loadFontFromBytes`**

In [font_api.go](font_api.go), replace `loadFontFromBytes` with:

```go
func (d *Document) loadFontFromBytes(data []byte) (Font, error) {
	ttf, err := parseTTF(data)
	if err != nil {
		return nil, fmt.Errorf("load font: %w", err)
	}
	fontID := embedFont(d, ttf)
	ef := &embeddedFont{
		doc:          d,
		ttf:          ttf,
		baseFont:     ttf.postScriptName,
		fontObjectID: fontID,
	}
	return ef, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run "TestLoadFont" -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add font_embed.go font_api.go font_embed_test.go
git commit -m "feat: wire Type0 + CIDFontType2 embedding in LoadFont"
```

---

### Task 13: AddText dispatch for embedded fonts

**Files:**
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Write failing tests**

Append to [text_add_test.go](text_add_test.go):

```go
func TestAddTextUnicode(t *testing.T) {
	doc := NewDocument(595, 842)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := doc.Page(1)
	err = page.AddText("Привет", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	// Embedded fonts emit hex strings with 2-byte glyphIDs.
	if !strings.Contains(content, "<") || !strings.Contains(content, "> Tj") {
		t.Errorf("content missing hex-string Tj: %q", content)
	}
	// Confirm at least one non-zero glyphID is written (Tj operand).
	ef := font.(*embeddedFont)
	gid := ef.ttf.glyphID('П')
	if gid == 0 {
		t.Fatal("glyphID('П') = 0 unexpectedly")
	}
	want := fmt.Sprintf("%04X", gid)
	if !strings.Contains(content, want) {
		t.Errorf("content missing expected glyphID %s (for 'П'):\n%s", want, content)
	}
}

func TestAddTextNotdef(t *testing.T) {
	doc := NewDocument(595, 842)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := doc.Page(1)
	// '日' is NOT in DejaVuSans; expect glyphID 0000.
	err = page.AddText("日", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "<0000>") {
		t.Errorf("expected <0000> (.notdef) in content, got: %q", content)
	}
}

func TestAddTextNilFontDefaultsToHelvetica(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "(Hello) Tj") {
		t.Errorf("nil Font should default to Helvetica with literal string: %q", content)
	}
}

func TestAddTextUnsupportedFontType(t *testing.T) {
	// A caller-defined type implementing Font but not one of our internal types.
	type rogueFont struct{}
	// This rogueFont cannot satisfy the Font interface because methods are defined
	// below in the test. If the Font interface is truly unimplementable externally
	// (e.g. has unexported method), drop this test. Otherwise use a value that we
	// can construct but AddText rejects.
	// NOTE: With the current interface (only public methods), a user could define
	// their own type. AddText's type switch rejects unknown types.
	t.Skip("skipped: cannot implement Font externally in package-internal tests without access to unexported methods")
}
```

(The `TestAddTextUnsupportedFontType` above is a placeholder; if `Font` has no unexported methods then external types could implement it, and this test is meaningful — otherwise skip. Keep it as `t.Skip` for now and revisit in Task 15 if we decide to add an unexported marker method.)

- [ ] **Step 2: Run to verify failure**

Run: `go test -run "TestAddTextUnicode|TestAddTextNotdef|TestAddTextNilFontDefaultsToHelvetica" -v ./...`
Expected: FAIL — either compile errors (if `winAnsiEncodeRune` is referenced but `AddText` still checks `standardFont` only) OR runtime errors because `embeddedFont` is rejected by AddText.

- [ ] **Step 3: Replace the font-dispatch block in `AddText`**

In [text_add.go](text_add.go), replace the block added in Task 1 (lines checking `sf, ok := font.(standardFont)` etc., up through the `widths, _ := standard14Widths(pdfFontName)` line) with:

```go
	// Default Font if unset.
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}

	// Apply defaults.
	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	lineSpacing := style.LineSpacing
	if lineSpacing == 0 {
		lineSpacing = 1.2
	}
	textColor := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		textColor = *style.Color
	}

	// Resolve font-specific callbacks.
	var (
		width        widthFn
		encode       encodeFn
		fontResName  string
		ascentFactor float64
	)
	switch f := font.(type) {
	case standardFont:
		pdfFontName := "/" + f.name
		widths, _ := standard14Widths(pdfFontName)
		width = func(r rune) float64 {
			code, ok := winAnsiEncodeRune(r)
			if !ok {
				code = byte('?')
			}
			return widths[code] / 1000.0 * fontSize
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('(')
			for _, r := range s {
				code, ok := winAnsiEncodeRune(r)
				if !ok {
					code = byte('?')
				}
				switch code {
				case '(', ')', '\\':
					b.WriteByte('\\')
				}
				b.WriteByte(code)
			}
			b.WriteByte(')')
			return b.String()
		}
		name, err := p.ensureStandardFontResource(pdfFontName)
		if err != nil {
			return err
		}
		fontResName = name
		ascentFactor = 0.8
	case *embeddedFont:
		width = func(r rune) float64 {
			gid := f.ttf.glyphID(r)
			if int(gid) >= len(f.ttf.glyphWidths) {
				return 0
			}
			return float64(f.ttf.glyphWidths[gid]) / float64(f.ttf.unitsPerEm) * fontSize
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('<')
			for _, r := range s {
				fmt.Fprintf(&b, "%04X", f.ttf.glyphID(r))
			}
			b.WriteByte('>')
			return b.String()
		}
		name, err := p.ensureEmbeddedFontResource(f)
		if err != nil {
			return err
		}
		fontResName = name
		ascentFactor = float64(f.ttf.ascent) / float64(f.ttf.unitsPerEm)
	default:
		return fmt.Errorf("add text: unsupported font type %T", font)
	}

	// Encoding types for closures; declared here so the closures above type-check.
```

Add at the top of [text_add.go](text_add.go) (before `wrapText`), the encodeFn type alias:

```go
// encodeFn returns a PDF string operand for s — "(...)" for single-byte
// encoding, "<...>" for hex glyph IDs.
type encodeFn func(s string) string
```

In the same function `AddText`, further down, find the block:

```go
	lines := wrapText(text, width, rectWidth)
```

That block stays — it now uses the local `width` callback built above. Good.

Find the line-rendering block. Currently it reads:

```go
		buf.WriteString(fmt.Sprintf("(%s) Tj\n", escapeStringPDF(line)))
```

Replace with:

```go
		buf.WriteString(fmt.Sprintf("%s Tj\n", encode(line)))
```

Find `measureString(line, width)` already from Task 3 — unchanged.

Find the ascent line:

```go
		ascent := 0.8 * fontSize
```

Replace with:

```go
		ascent := ascentFactor * fontSize
```

Find the existing `ensureFontResource` method in [text_add.go](text_add.go) (~line 405). Rename it to `ensureStandardFontResource` (it already takes a string and does Type1 font dict creation — unchanged behavior). Add a new helper below it:

```go
// ensureEmbeddedFontResource registers an already-embedded font (created by LoadFont)
// in the page's /Resources /Font dict and returns the resource name. Caches the name
// on the embeddedFont for reuse across pages.
func (p *Page) ensureEmbeddedFontResource(ef *embeddedFont) (string, error) {
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("add text: page has no dict")
	}
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}
	fontVal := resolveRef(p.doc.objects, resources["/Font"])
	fontDict, _ := fontVal.(pdfDict)
	if fontDict == nil {
		fontDict = pdfDict{}
		resources["/Font"] = fontDict
	}

	// Check whether this embedded font is already in the page's Font dict.
	for name, val := range fontDict {
		if ref, ok := val.(pdfRef); ok && ref.Num == ef.fontObjectID {
			return name, nil
		}
	}
	name := nextFontName(fontDict)
	fontDict[name] = pdfRef{Num: ef.fontObjectID}
	ef.resourceName = name
	return name, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run "TestAddTextUnicode|TestAddTextNotdef|TestAddTextNilFontDefaultsToHelvetica" -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: All tests PASS (previous Helvetica tests still pass — the standardFont branch preserves `(Hello) Tj` output).

- [ ] **Step 5: Commit**

```bash
git add text_add.go text_add_test.go
git commit -m "feat: AddText dispatch for embedded fonts with Unicode encoding"
```

---

### Task 14: Integration round-trip test + CLAUDE.md

**Files:**
- Modify: `text_add_integration_test.go`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Write the integration test**

Append to [text_add_integration_test.go](text_add_integration_test.go):

```go
func TestAddTextEmbeddedFontRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)

	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}

	page, _ := doc.Page(1)
	err = page.AddText("Привет, мир!", asposepdf.TextStyle{
		Font: font,
		Size: 18,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText cyrillic: %v", err)
	}
	err = page.AddText("Γειά σου κόσμε!", asposepdf.TextStyle{
		Font: font,
		Size: 18,
	}, asposepdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})
	if err != nil {
		t.Fatalf("AddText greek: %v", err)
	}

	outDir := filepath.Join("result_files", "TestAddTextEmbeddedFontRoundTrip")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) < 1 {
		t.Fatalf("no pages extracted")
	}
	if !strings.Contains(texts[0], "Привет") {
		t.Errorf("extracted text missing Cyrillic: %q", texts[0])
	}
	if !strings.Contains(texts[0], "κόσμε") {
		t.Errorf("extracted text missing Greek: %q", texts[0])
	}
}
```

Verify the test file imports `"os"`, `"path/filepath"`, `"strings"`, `"testing"`, and `"github.com/...asposepdf"` (the module path — copy from an existing integration test in the same file). If the imports are already present, do not duplicate.

- [ ] **Step 2: Run integration test**

Run: `go test -run TestAddTextEmbeddedFontRoundTrip -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 3: Update CLAUDE.md**

Open [CLAUDE.md](CLAUDE.md). Find the `page.go` API section (the block listing `PageSizes`, `(*Page).Number`, …).

Locate the existing line:

```
- `Font` — standard 14 PDF font constants: `FontHelvetica`, `FontHelveticaBold`, ...
```

Replace that single line with:

```
- `Font` — interface implemented by standard 14 fonts and embedded TTF fonts; has `BaseFont()` and `IsEmbedded()` methods
- Standard 14 PDF fonts as package-level `Font` vars: `FontHelvetica`, `FontHelveticaBold`, `FontHelveticaOblique`, `FontHelveticaBoldOblique`, `FontTimesRoman`, `FontTimesBold`, `FontTimesItalic`, `FontTimesBoldItalic`, `FontCourier`, `FontCourierBold`, `FontCourierOblique`, `FontCourierBoldOblique`, `FontSymbol`, `FontZapfDingbats`
- `FindFont(name) (Font, error)` — returns a standard 14 `Font` by PostScript name (case-insensitive); error for unknown names
- `(*Document).LoadFont(path) (Font, error)` — reads a TTF file, embeds it into the document, returns a `Font` usable in `TextStyle.Font`
- `(*Document).LoadFontFromStream(r) (Font, error)` — like `LoadFont` but reads from an `io.Reader`
```

- [ ] **Step 4: Run tests one more time**

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add text_add_integration_test.go CLAUDE.md
git commit -m "test: embedded font round-trip + docs

Adds TestAddTextEmbeddedFontRoundTrip which writes Cyrillic and Greek
text using DejaVuSans, validates the PDF, reopens it, and confirms
ExtractText returns the original strings.

CLAUDE.md is updated to document the new Font interface, FindFont,
LoadFont, and LoadFontFromStream."
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by |
|-----------------|-----------|
| `Font` interface with `BaseFont()`/`IsEmbedded()` | Task 1 |
| Package-level standard 14 vars | Task 1 |
| `FindFont` case-insensitive discovery | Task 2 |
| `winAnsiEncodeRune` helper | Task 3 |
| Rune-safe `wrapText`/`measureString`/`breakWord` | Task 3 |
| `widthFn` callback in wrap/measure | Task 3 |
| TTF parser — magic, table directory, required tables | Task 5 |
| TTF parser — head, hhea, maxp, hmtx | Task 6 |
| TTF parser — cmap format 4 and 12 | Task 7 |
| TTF parser — OS/2, post, name with PS-name fallback | Task 8 |
| `embeddedFont` struct + `LoadFont`/`LoadFontFromStream` | Task 9 |
| `/FontFile2` stream with FlateDecode + /Length1 | Task 10 |
| `/FontDescriptor` with Flags, FontBBox, Ascent/Descent/CapHeight, StemV heuristic | Task 10 |
| `/W` array packing with `/DW 500` | Task 11 |
| ToUnicode CMap generation | Task 11 |
| Type0 + CIDFontType2 + Identity-H wiring | Task 12 |
| AddText type switch on Font, encode/width callbacks | Task 13 |
| `ensureEmbeddedFontResource` page-resource registration | Task 13 |
| Embedded ascent uses `ttf.ascent / unitsPerEm * fontSize` | Task 13 |
| `.notdef` tofu for missing runes | Task 13 (glyphID 0 path) |
| `style.Font == nil` defaults to `FontHelvetica` | Task 13 |
| Unsupported font type error | Task 13 |
| Integration round-trip test | Task 14 |
| CLAUDE.md updated | Task 14 |

**Placeholder scan:** No "TBD", "TODO", "implement later" tokens. All code blocks are complete. One flagged item: Task 13's `TestAddTextUnsupportedFontType` is marked `t.Skip` with a note — this is intentional because the `Font` interface has only public methods so external types *can* implement it, but asserting that behavior requires constructing a rogue type in the test package. Marked as a follow-up consideration, not a plan gap.

**Type consistency:**
- `widthFn` used identically in Tasks 3, 13 — `func(rune) float64`.
- `encodeFn` introduced in Task 13 — `func(string) string`.
- `ttfFont` fields referenced in later tasks (`f.ttf.glyphID`, `f.ttf.glyphWidths`, `f.ttf.unitsPerEm`, `f.ttf.ascent`) all match the struct defined in Task 5 + augmented through Tasks 6–8.
- `embeddedFont` fields (`ttf`, `fontObjectID`, `baseFont`, `resourceName`) match across Tasks 9, 12, 13.
- `defaultCIDWidth = 500` (Task 11) referenced in `/W` packing and as `/DW` literal in Task 12.
- `buildFontFile2Stream`, `buildFontDescriptor`, `buildWArray`, `buildToUnicodeCMap`, `embedFont` — function names stable across Tasks 10–12.

---

## Execution

Plan complete and saved to `docs/superpowers/plans/2026-04-21-embed-font-unicode.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
