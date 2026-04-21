# EmbedFont + Unicode — Design Spec

## Goal

Add support for embedding TrueType (TTF) fonts into PDF documents and using them in `AddText` so that Unicode text (beyond Latin-1 WinAnsi) can be written. Reader side already supports Type0/CIDFont/ToUnicode; this spec focuses on the writer side.

## Scope

**In scope:**
- TTF font loading from file or `io.Reader`
- Full TTF embedding (no subsetting) via `/FontFile2`
- Composite Type0 font with CIDFontType2 descendant, Identity-H encoding
- ToUnicode CMap generation for text extraction
- `Font` as an interface; standard 14 fonts as package-level vars
- Aspose-style `FindFont(name)` discovery
- Per-rune width callback unifying standard 14 and embedded paths in `AddText`
- Rune-safe line wrapping (word breaks + fallback character break for words longer than rect)
- `.notdef` (glyphID 0) fallback for runes not in the font

**Out of scope (follow-up):**
- TTF glyph subsetting (tracked in beads)
- OTF / CFF fonts (Type1C)
- TTC (TrueType Collection) files
- System font discovery by name
- `IsEmbedded` toggle / `FontOptions` equivalent (standard 14 never embed, TTF always embeds)
- Sharing a loaded font across multiple `*Document` instances

## Public API

```go
// Font is implemented by both standard 14 fonts and embedded TTF fonts.
type Font interface {
    BaseFont() string  // PDF /BaseFont name, e.g. "Helvetica" or "ArialMT"
    IsEmbedded() bool  // false for standard 14, true for loaded TTF
}

// Standard 14 fonts (replace existing Font int constants).
var (
    FontHelvetica            Font
    FontHelveticaBold        Font
    FontHelveticaOblique     Font
    FontHelveticaBoldOblique Font
    FontTimesRoman           Font
    FontTimesBold            Font
    FontTimesItalic          Font
    FontTimesBoldItalic      Font
    FontCourier              Font
    FontCourierBold          Font
    FontCourierOblique       Font
    FontCourierBoldOblique   Font
    FontSymbol               Font
    FontZapfDingbats         Font
)

// FindFont returns a standard 14 font by case-insensitive name.
// Aspose-compatible entry point.
//   FindFont("Helvetica") -> FontHelvetica, nil
//   FindFont("helvetica") -> FontHelvetica, nil
//   FindFont("Arial")     -> nil, error
func FindFont(name string) (Font, error)

// LoadFont reads a TTF file, parses it, embeds it into the document,
// and returns a Font usable in TextStyle.
func (d *Document) LoadFont(path string) (Font, error)

// LoadFontFromStream is like LoadFont but reads from an io.Reader.
func (d *Document) LoadFontFromStream(r io.Reader) (Font, error)

// TextStyle.Font changes type from int to Font interface.
// style.Font == nil defaults to FontHelvetica.
type TextStyle struct {
    Font Font
    // ... remaining fields unchanged
}
```

### Usage

```go
doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)

// Standard 14 — no change from today beyond type switch.
page, _ := doc.Page(1)
page.AddText("Hello", asposepdf.TextStyle{
    Font: asposepdf.FontHelvetica,
    Size: 14,
}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})

// Embedded TTF with Unicode.
dejavu, err := doc.LoadFont("testdata/DejaVuSans.ttf")
if err != nil {
    return err
}
page.AddText("Привет, мир!", asposepdf.TextStyle{
    Font: dejavu,
    Size: 14,
}, asposepdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})

// Aspose-style discovery.
helv, _ := asposepdf.FindFont("Helvetica")
_ = helv.IsEmbedded() // false
_ = dejavu.IsEmbedded() // true
```

## Internal design

### Font type hierarchy

`Font` is an interface with two concrete implementations:

```go
type standardFont struct {
    name string // PostScript name, e.g. "Helvetica"
}

func (s standardFont) BaseFont() string { return s.name }
func (s standardFont) IsEmbedded() bool { return false }

type embeddedFont struct {
    doc          *Document
    ttf          *ttfFont    // parsed TTF structure
    fontObjectID int         // ID of the Type0 font dict in doc.objects
    resourceName string      // stable resource name, e.g. "/F0" when used on a page
                             // (populated on first ensureEmbeddedFontResource call)
    baseFont     string      // PostScript name from name table
}

func (e *embeddedFont) BaseFont() string { return e.baseFont }
func (e *embeddedFont) IsEmbedded() bool { return true }
```

`TextStyle.Font` is checked via type switch in `AddText`. Any unknown implementation of `Font` yields an error (defensive — user can't implement it since members are unexported, but the switch stays explicit).

### TTF parser (`ttf.go`)

Parses only the tables needed for PDF embedding and text measurement. The full raw file is retained for `/FontFile2`.

```go
type ttfFont struct {
    data            []byte           // raw file bytes (for /FontFile2)
    unitsPerEm      uint16
    ascent          int16            // FUnits, from hhea.ascent
    descent         int16            // FUnits, negative
    capHeight       int16            // from OS/2.sCapHeight (0 if absent)
    xMin, yMin      int16            // font bbox from head
    xMax, yMax      int16
    italicAngle     float64          // from post
    isFixedPitch    bool             // from post
    weight          uint16           // from OS/2.usWeightClass
    flagsBold       bool             // from OS/2.fsSelection bit 5
    flagsItalic     bool             // from OS/2.fsSelection bit 0
    postScriptName  string           // from name table (nameID 6)
    numGlyphs       uint16           // from maxp
    glyphWidths     []uint16         // advanceWidth per glyphID, FUnits
    runeToGlyph     map[rune]uint16  // from cmap
}

func parseTTF(data []byte) (*ttfFont, error)
func (f *ttfFont) glyphID(r rune) uint16     // 0 if absent (.notdef)
func (f *ttfFont) advanceEm(gid uint16) float64 // advance / unitsPerEm
```

**Required tables:** `head`, `hhea`, `hmtx`, `maxp`, `name`, `cmap`, `OS/2`, `post`. Missing any → error.

**Magic bytes:** first 4 bytes must be `00 01 00 00` (TrueType) or `74 72 75 65` ("true"). Other (`OTTO` for OpenType/CFF) → error.

**cmap subtable selection priority:**
1. Platform 0 (Unicode), Encoding 4 (Unicode full repertoire), format 12
2. Platform 0 Encoding 3 (Unicode BMP), format 4
3. Platform 3 (Microsoft) Encoding 10 (UCS-4), format 12
4. Platform 3 Encoding 1 (Unicode BMP), format 4

Formats supported: 4 (segmented BMP), 12 (segmented coverage). Other formats → error if no supported subtable found.

**`name` table:** walk all records, prefer nameID 6 (PostScript name). Decode Platform 3 Encoding 1 (UTF-16BE) or Platform 1 (Mac Roman ASCII-safe subset). If nameID 6 absent, fall back to nameID 4 (Full Name) with spaces replaced by dashes.

### PDF object generation (`font_embed.go`)

Invoked from `LoadFont` / `LoadFontFromStream` after `parseTTF` succeeds. Creates five objects in `doc.objects` and returns the Type0 font's object ID.

**Object 1 — Type0 font dictionary:**
```
<< /Type /Font
   /Subtype /Type0
   /BaseFont /ArialMT
   /Encoding /Identity-H
   /DescendantFonts [ <ref to CIDFont> ]
   /ToUnicode <ref to ToUnicode CMap stream>
>>
```

**Object 2 — CIDFontType2 dictionary:**
```
<< /Type /Font
   /Subtype /CIDFontType2
   /BaseFont /ArialMT
   /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>
   /FontDescriptor <ref to FontDescriptor>
   /CIDToGIDMap /Identity
   /W [ ... per /W packing rules ... ]
   /DW 500
>>
```

**Object 3 — FontDescriptor dictionary:**
```
<< /Type /FontDescriptor
   /FontName /ArialMT
   /Flags <computed>
   /FontBBox [ xMin yMin xMax yMax ] (scaled from FUnits to 1/1000 em)
   /ItalicAngle <italicAngle>
   /Ascent <scaled ascent>
   /Descent <scaled descent>
   /CapHeight <scaled capHeight, or scaled ascent if zero>
   /StemV <computed from usWeightClass>
   /FontFile2 <ref to FontFile2 stream>
>>
```

`/Flags` bits (PDF spec Table 123):
- bit 1 (FixedPitch): set if `isFixedPitch`
- bit 3 (Symbolic): always set for embedded TTF (we encode via CID)
- bit 6 (Nonsymbolic): not set
- bit 7 (Italic): set if `flagsItalic`
- bit 19 (ForceBold): set if `flagsBold`

`/StemV` approximation: `max(50, 50 + (weight - 400) * 0.2)`. For weight 400 → 50; for 700 (Bold) → 110. Acrobat and other engines use similar heuristics when the TTF lacks hinting data.

**Object 4 — FontFile2 stream:**
```
<< /Length1 <raw TTF byte length>
   /Filter /FlateDecode
>>
stream
<zlib-compressed raw TTF bytes>
endstream
```

**Object 5 — ToUnicode CMap stream:**
Generated text content (per PDF spec Adobe Technical Note #5411):

```
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange <0000> <FFFF> endcodespacerange
<N> beginbfchar
<gid1> <unicode-utf16be>
<gid2> <unicode-utf16be>
...
endbfchar
endcmap
CMapName currentdict /CMap defineresource pop
end
end
```

- One `bfchar` entry per `(glyphID, rune)` pair from `runeToGlyph`.
- If >100 entries, emit multiple `beginbfchar`/`endbfchar` blocks (max 100 each).
- Unicode encoded as UTF-16BE hex; supplementary characters (>U+FFFF) use surrogate pairs.
- Stream compressed with `/Filter /FlateDecode`.

**`/W` array packing:**
- `/DW` fixed at 500 (simple, predictable; most widths are explicit anyway).
- Build sequential width arrays: `c_start [w1 w2 w3 ...]` for contiguous runs of glyphIDs with non-default widths.
- If a run of identical widths covers >5 consecutive glyphIDs, emit as `cFirst cLast w` range form instead.
- Skip glyphIDs whose width equals 500 (covered by `/DW`).

### AddText integration

**New helper types in `text_add.go`:**
```go
type widthFn func(r rune) float64  // advance in points for rune
type encodeFn func(s string) string // full PDF string operand, "(...)" or "<...>"
```

**Width callback construction:**

For `standardFont`:
```go
// winAnsiEncodeRune is a new helper in encoding.go: reverse map of winAnsiEncoding
// ([256]rune). Built once via sync.Once. Returns (code byte, ok bool).
widthOf := func(r rune) float64 {
    code, ok := winAnsiEncodeRune(r)
    if !ok {
        code = byte('?')
    }
    return widths[code] / 1000.0 * fontSize
}
```

For `embeddedFont`:
```go
widthOf := func(r rune) float64 {
    gid := f.ttf.glyphID(r)
    return f.ttf.advanceEm(gid) * fontSize
}
```

**Encoding callback construction:**

For `standardFont` — returns literal string (parentheses) with WinAnsi byte codes and PDF-escaped special bytes:
```go
encode := func(s string) string {
    var b strings.Builder
    b.WriteByte('(')
    for _, r := range s {
        code, _ := winAnsiEncodeRune(r)
        switch code {
        case '(', ')', '\\':
            b.WriteByte('\\')
        }
        b.WriteByte(code)
    }
    b.WriteByte(')')
    return b.String()
}
```

For `embeddedFont` — returns hex string with 2-byte glyphIDs:
```go
encode := func(s string) string {
    var b strings.Builder
    b.WriteByte('<')
    for _, r := range s {
        gid := f.ttf.glyphID(r)  // 0 if missing → .notdef tofu
        fmt.Fprintf(&b, "%04X", gid)
    }
    b.WriteByte('>')
    return b.String()
}
```

**Font resource registration:**

`ensureFontResource` becomes two helpers:
- `ensureStandardFontResource(pdfName string) (resName string, err error)` — current behavior (create Type1 font dict lazily).
- `ensureEmbeddedFontResource(ef *embeddedFont) (resName string, err error)` — adds reference to the pre-built Type0 font object into the page's `/Resources /Font`. Caches `resName` on `ef` (one name per document, reused across pages).

**Baseline ascent:**
- `standardFont`: `ascent := 0.8 * fontSize` (existing).
- `embeddedFont`: `ascent := float64(ef.ttf.ascent) / float64(ef.ttf.unitsPerEm) * fontSize`.

**Line wrapping rune-safety:**
`wrapText`, `measureString`, and `breakWord` are rewritten to iterate runes (via `for _, r := range s`) instead of bytes. `breakWord` splits on rune boundaries so multi-byte UTF-8 isn't cut mid-sequence.

All other `AddText` logic (rotation, behind, alignment, background, underline, strikethrough) operates on resolved x/y/width values and is unaffected.

### Lifecycle

- `LoadFont` eagerly parses the TTF and creates the five PDF objects in `doc.objects`.
- If the font is never referenced by any page, `RemoveUnusedObjects()` can drop it on demand.
- Multiple calls to `LoadFont(samePath)` create distinct `embeddedFont` values and distinct object sets. Users who want deduplication can cache the returned `Font` themselves.
- Font objects are included in `Save`/`WriteTo` just like any other `doc.objects` entry — no special handling in `writer.go`.

## Error handling

| Situation | Behavior |
|-----------|----------|
| `LoadFont(path)`, file does not exist or unreadable | `fmt.Errorf("load font: open %s: %w", path, err)` |
| First 4 bytes not a TTF signature | `fmt.Errorf("load font: not a TrueType file: %s", path)` |
| TTF missing required table (head/hhea/hmtx/maxp/name/cmap/OS-2/post) | `fmt.Errorf("load font: missing required table %s", tag)` |
| No supported cmap subtable (format 4 or 12) | `fmt.Errorf("load font: unsupported cmap format")` |
| `name` table lacks both nameID 6 and nameID 4 | `fmt.Errorf("load font: no PostScript name in name table")` |
| `FindFont(name)` — name not in standard 14 | `fmt.Errorf("find font: unknown standard font %q", name)` |
| `AddText` with rune not in embedded font | Emit glyphID 0 (`.notdef`), no error |
| `AddText("")` | `nil`, no-op (unchanged) |
| `style.Font == nil` | Default to `FontHelvetica`, no error |
| `style.Font` is neither `standardFont` nor `*embeddedFont` | `fmt.Errorf("add text: unsupported font type %T", style.Font)` |

**TTF validation depth:** no checksum verification (YAGNI). Parser fails gracefully on malformed offsets / lengths by returning descriptive errors.

## Files

| File | Change |
|------|--------|
| `font_api.go` | New. `Font` interface, `standardFont`, `embeddedFont`, `FindFont`, `LoadFont`, `LoadFontFromStream` |
| `ttf.go` | New. `parseTTF`, `ttfFont` struct, table parsers |
| `ttf_test.go` | New. Unit tests for TTF parsing against DejaVuSans.ttf |
| `font_embed.go` | New. Generation of Type0 / CIDFont / FontDescriptor / FontFile2 / ToUnicode CMap objects |
| `font_embed_test.go` | New. Unit tests for object generation |
| `font_api_test.go` | New. Unit tests for `Font` interface, `FindFont`, `LoadFont` |
| `color.go` | Modify. `Font` becomes interface; iota constants removed; `var FontHelvetica ...`; delete `fontPDFName` (replaced by `BaseFont()` method). `TextStyle.Font` type now `Font` interface. |
| `text_add.go` | Modify. `wrapText`/`measureString`/`breakWord` use `widthFn` callback and iterate runes. `AddText` does type switch on `style.Font`. Split `ensureFontResource` into `ensureStandardFontResource` + `ensureEmbeddedFontResource`. Rune-safe ascent computation for embedded fonts. |
| `text_add_test.go` | Modify. Adjust existing tests where internal ascent/encoding assumptions changed; add `TestAddTextUnicode`, `TestAddTextNotdef`, `TestAddTextNilFontDefaults`. |
| `text_add_integration_test.go` | Modify. Add `TestAddTextEmbeddedFontRoundTrip` — write Cyrillic, save, validate, reopen, extract. |
| `testdata/DejaVuSans.ttf` | New. Test TTF (Bitstream Vera license, covers Latin + Cyrillic + Greek). |
| `testdata/testfiles.json` | Modify. Entry for any test that reads a PDF file (none new here — TTF is separate). |
| `CLAUDE.md` | Modify. Add `Font` interface, `FindFont`, `LoadFont`, `LoadFontFromStream`, `IsEmbedded` to API docs. |

## Testing

### Unit tests

**TTF parser (`ttf_test.go`):**
- `TestParseTTF_DejaVu` — unitsPerEm == 2048, ascent > 0, descent < 0, postScriptName == "DejaVuSans"
- `TestParseTTF_CmapLatin` — `glyphID('A') != 0`
- `TestParseTTF_CmapCyrillic` — `glyphID('Я') != 0`
- `TestParseTTF_CmapMissing` — `glyphID('日') == 0`
- `TestParseTTF_AdvanceWidth` — known widths for space and 'A'
- `TestParseTTF_NotTTF` — error on garbage bytes

**Font embedding (`font_embed_test.go`):**
- `TestEmbedFontCreatesFiveObjects` — after `LoadFont`, `doc.objects` has the Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode CMap entries
- `TestEmbedFontType0Refs` — Type0 dict's `/DescendantFonts` references CIDFont ID, `/ToUnicode` references CMap stream ID
- `TestEmbedFontToUnicodeCMap` — CMap text contains `beginbfchar` and at least one `<gid> <unicode>` pair
- `TestEmbedFontWArray` — `/W` array round-trips through `parseCIDWidthArray` to recover a known glyph's width
- `TestEmbedFontFlags` — bold TTF yields FontDescriptor with bit 19 set

**Font API (`font_api_test.go`):**
- `TestFindFontExact` — `FindFont("Helvetica")` returns `FontHelvetica`
- `TestFindFontCaseInsensitive` — `FindFont("helvetica")` returns `FontHelvetica`
- `TestFindFontUnknown` — `FindFont("Typo")` returns error
- `TestFontIsEmbedded` — standard 14 → false; loaded TTF → true
- `TestFontBaseFont` — `FontHelvetica.BaseFont() == "Helvetica"`; loaded DejaVuSans → `"DejaVuSans"`
- `TestLoadFontMissingFile` — error for nonexistent path
- `TestLoadFontNotTTF` — error for non-TTF bytes via `LoadFontFromStream`

**AddText with embedded fonts (`text_add_test.go` additions):**
- `TestAddTextUnicode` — write Cyrillic text with DejaVuSans; content stream contains `<...> Tj` with expected glyphIDs
- `TestAddTextNotdef` — rune outside font renders as `<0000>`
- `TestAddTextNilFontDefaults` — `TextStyle{}` (Font nil) produces Helvetica output identical to explicit `FontHelvetica`
- `TestAddTextUnicodeWrapping` — Cyrillic paragraph wraps on space boundaries and character boundaries for long runs
- `TestAddTextStandardFontUnchanged` — existing Helvetica test output bit-for-bit identical after refactor (regression guard)

### Integration test

**`text_add_integration_test.go`:**
- `TestAddTextEmbeddedFontRoundTrip` — create A4 doc, `LoadFont("testdata/DejaVuSans.ttf")`, write `"Привет, мир! Γειά σου κόσμε!"` on page 1, save to `result_files/TestAddTextEmbeddedFontRoundTrip/output.pdf`, `Validate` returns Valid, reopen, `ExtractText()[0]` contains the original strings.

## Scope boundary

**This spec covers:**
- `Font` interface with `standardFont` and `embeddedFont` implementations
- TTF parser (tables: head, hhea, hmtx, maxp, name, cmap, OS/2, post)
- Full TTF embedding via Type0 + CIDFontType2 + Identity-H + ToUnicode
- `FindFont`, `LoadFont`, `LoadFontFromStream`
- `AddText` integration with per-rune width callback, hex-string output for embedded fonts
- Rune-safe line wrapping

**This spec does NOT cover:**
- TTF glyph subsetting (beads issue pdf-go-ftw — separate follow-up)
- OTF / CFF fonts
- TTC collections
- System font discovery
- `IsEmbedded` as a user-settable flag / `FontOptions`
- Cross-document font reuse
- Font substitution / fallback chains (user picks one Font per `AddText` call)
