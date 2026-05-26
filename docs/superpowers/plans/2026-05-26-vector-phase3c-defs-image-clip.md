# Vector Graphics Phase 3c Implementation Plan — `<image>`, `<defs>`/`<use>`/`<symbol>`, `<clipPath>`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add three SVG features to the existing pipeline — inline raster images (`<image>` with data:image/png|jpeg base64), reusable elements (`<defs>`/`<use>`/`<symbol>` with parse-time deep-cloning), and clipping paths (`<clipPath>` mapping to PDF `W`/`W*`). All work internal — no public API changes.

**Architecture:** `<use>` is resolved at parse-end via a second pass that deep-clones the referenced subtree from `SVG.defs` and wraps in a translate-transform group; no `*svgUse` nodes remain at render time. `<image>` becomes a PDF Image XObject via existing image-add infrastructure. `<clipPath>` is stored in `defs` and emitted inline before each shape that references it via `clip-path="url(#id)"`.

**Tech Stack:** Go 1.24, stdlib only.

**Reference:** [docs/superpowers/specs/2026-05-26-vector-phase3c-defs-image-clip-design.md](../specs/2026-05-26-vector-phase3c-defs-image-clip-design.md)

**Beads:** [pdf-go-tq5](bd show pdf-go-tq5) (Phase 3c) under umbrella [pdf-go-ybu](bd show pdf-go-ybu).

---

## File Map

| File | Purpose |
|---|---|
| `svg_image.go` (new) | `svgImage` IR, `decodeSVGDataURI` parser |
| `svg_parse_image.go` (new) | XML parser for `<image>` |
| `svg_render_image.go` (new) | Renderer: register Image XObject + emit `Do` |
| `svg_use.go` (new) | `svgUse`, `svgSymbol` IR types, `resolveUseReferences`, `deepCloneSVGNode` |
| `svg_parse_use.go` (new) | XML parsers for `<use>` and `<symbol>` |
| `svg_clip.go` (new) | `svgClipPath` IR, `emitClipPathInline` |
| `svg_parse_clip.go` (new) | XML parser for `<clipPath>` |
| `svg_types.go` (modify) | Add IR types to svgNode set; extend `SVG.defs map[string]svgNode`; add `clipPath string` to `svgStyle` |
| `svg_parse.go` (modify) | Walker recognizes new elements; `<defs>` walker generalized to collect any id'd element; clip-path style prop handler |
| `svg_render.go` (modify) | Type switch dispatches new IR; pre-shape `q + clip W n` injection when style.clipPath != "" |
| Tests + fixtures | per-feature unit + integration |
| `CLAUDE.md` / `README.md` | Phase 3c updates |

---

## Task 1: `svgImage` IR + data URI decoder

**Files:**
- Create: `svg_image.go`
- Create: `svg_image_test.go`

### Step 1: Failing tests in `svg_image_test.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"testing"
)

func TestDecodeSVGDataURI_PNG(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	data, format, ok := decodeSVGDataURI(uri)
	if !ok { t.Fatal("decode failed") }
	if format != ImageFormatPNG {
		t.Errorf("format = %v", format)
	}
	if len(data) != len(raw) {
		t.Errorf("len(data) = %d, want %d", len(data), len(raw))
	}
}

func TestDecodeSVGDataURI_JPEG(t *testing.T) {
	raw := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic
	for _, mime := range []string{"image/jpeg", "image/jpg"} {
		uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
		_, format, ok := decodeSVGDataURI(uri)
		if !ok || format != ImageFormatJPEG {
			t.Errorf("%s: format = %v ok=%v", mime, format, ok)
		}
	}
}

func TestDecodeSVGDataURI_NotData(t *testing.T) {
	_, _, ok := decodeSVGDataURI("https://example.com/foo.png")
	if ok { t.Error("expected failure for non-data URI") }
}

func TestDecodeSVGDataURI_MalformedBase64(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/png;base64,!@#$%")
	if ok { t.Error("expected failure for malformed base64") }
}

func TestDecodeSVGDataURI_UnsupportedMIME(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/gif;base64,AAAA")
	if ok { t.Error("expected failure for unsupported MIME") }
}

func TestDecodeSVGDataURI_MissingBase64Marker(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/png,rawdata")
	if ok { t.Error("expected failure for URI without base64 marker (raw URL-encoded data not supported)") }
}
```

### Step 2: Run, observe failures

```
go test -run TestDecodeSVGDataURI -v ./...
```

### Step 3: Create `svg_image.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"strings"
)

// svgImage is the IR node for an SVG <image> element.
type svgImage struct {
	x, y, w, h float64
	par        svgPreserveAspect
	data       []byte
	format     ImageFormat
	style      svgStyle
	transform  *svgMatrix
}

func (*svgImage) svgNodeKind() string { return "image" }

// decodeSVGDataURI parses an SVG image href that is a base64-encoded data URI.
// Supports only data:image/png;base64,... and data:image/jpeg;base64,...
// (raw URL-encoded data is not supported).
// Returns ok=false for any other input shape.
func decodeSVGDataURI(s string) (data []byte, format ImageFormat, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return nil, 0, false
	}
	s = s[len(prefix):]
	semi := strings.IndexByte(s, ';')
	comma := strings.IndexByte(s, ',')
	if semi < 0 || comma < 0 || semi >= comma {
		return nil, 0, false
	}
	mime := strings.ToLower(strings.TrimSpace(s[:semi]))
	encodingAndData := s[semi+1:]
	if !strings.HasPrefix(encodingAndData, "base64,") {
		return nil, 0, false
	}
	encoded := encodingAndData[len("base64,"):]
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, 0, false
	}
	switch mime {
	case "image/png":
		return b, ImageFormatPNG, true
	case "image/jpeg", "image/jpg":
		return b, ImageFormatJPEG, true
	}
	return nil, 0, false
}
```

### Step 4: Run, ensure all 6 tests pass

```
go test -run TestDecodeSVGDataURI -v ./...
go test ./...
```

### Step 5: Commit

```
feat: svg — svgImage IR type + decodeSVGDataURI (data:image/png|jpeg base64)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 2: `<image>` XML parser

**Files:**
- Create: `svg_parse_image.go`
- Modify: `svg_parse.go` (dispatch `<image>`)
- Create: `svg_parse_image_test.go`
- Create: `testdata/svg/image_inline_png.svg`

### Step 1: Generate a small inline PNG fixture

Create `testdata/svg/image_inline_png.svg` with a tiny 4×4 red PNG embedded as base64:

```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <image x="10" y="20" width="80" height="60"
         href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAFklEQVQYV2P8z8DwnwEKGGEMRrwsAJpiAwEAg5IXAAAAAElFTkSuQmCC"/>
</svg>
```

(That base64 string is a real 4×4 red PNG — verified valid.)

### Step 2: Failing tests in `svg_parse_image_test.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_ImageInlinePNG(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/image_inline_png.svg")
	svg, err := parseSVGBytes(data)
	if err != nil { t.Fatal(err) }
	if len(svg.root.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(svg.root.children))
	}
	im, ok := svg.root.children[0].(*svgImage)
	if !ok { t.Fatalf("expected *svgImage, got %T", svg.root.children[0]) }
	if im.x != 10 || im.y != 20 || im.w != 80 || im.h != 60 {
		t.Errorf("dims = (%g,%g) %gx%g", im.x, im.y, im.w, im.h)
	}
	if im.format != ImageFormatPNG {
		t.Errorf("format = %v", im.format)
	}
	if len(im.data) == 0 {
		t.Error("data is empty")
	}
}

func TestParseSVG_ImageWithoutHrefIsSkipped(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><image x="0" y="0" width="10" height="10"/></svg>`))
	// No href → parser should produce nothing (or a skipped node).
	// Either no children OR no svgImage in children — both are acceptable best-effort behavior.
	for _, c := range svg.root.children {
		if _, ok := c.(*svgImage); ok {
			t.Errorf("expected no <image> node when href is missing, got %+v", c)
		}
	}
}

func TestParseSVG_ImageWithExternalHrefIsSkipped(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><image x="0" y="0" width="10" height="10" href="https://example.com/foo.png"/></svg>`))
	for _, c := range svg.root.children {
		if _, ok := c.(*svgImage); ok {
			t.Errorf("expected no <image> node for external URL, got %+v", c)
		}
	}
}
```

### Step 3: Create `svg_parse_image.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
)

// parseSVGImage reads an <image> element. Returns nil if href is missing,
// external (not data:), or has unsupported MIME / bad base64.
// Caller has received the StartElement; on exit </image> has been consumed.
func parseSVGImage(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	im := &svgImage{
		style: parent.style,
		par:   parsePreserveAspect(""),
	}
	var href string
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			im.x, _ = parseSVGLength(a.Value)
		case "y":
			im.y, _ = parseSVGLength(a.Value)
		case "width":
			im.w, _ = parseSVGLength(a.Value)
		case "height":
			im.h, _ = parseSVGLength(a.Value)
		case "preserveAspectRatio":
			im.par = parsePreserveAspect(a.Value)
		case "href":
			href = a.Value
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				im.transform = &m
			}
		}
	}
	// xlink:href fallback (legacy attribute namespace).
	if href == "" {
		for _, a := range start.Attr {
			if a.Name.Local == "href" {
				href = a.Value
				break
			}
		}
	}
	applySVGStyleAttrs(&im.style, start.Attr)
	if err := d.Skip(); err != nil {
		return nil, err
	}
	if href == "" {
		return nil, nil
	}
	data, format, ok := decodeSVGDataURI(href)
	if !ok {
		return nil, nil // best-effort: skip non-data or unsupported
	}
	im.data = data
	im.format = format
	return im, nil
}
```

### Step 4: Wire into `svg_parse.go` walker

In `parseSVGElement`, add a `case "image":` before `default:`:

```go
case "image":
	return parseSVGImage(d, parent, start)
```

### Step 5: Run, ensure tests pass

```
go test -run "TestParseSVG_Image" -v ./...
go test ./...
```

### Step 6: Commit

```
feat: svg — parse <image> element (data:image/png|jpeg base64 href; external URLs silently skipped)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

Stage `svg_parse_image.go`, `svg_parse_image_test.go`, `svg_parse.go`, `testdata/svg/image_inline_png.svg`.

---

## Task 3: `<image>` renderer (PDF Image XObject + Do)

**Files:**
- Create: `svg_render_image.go`
- Modify: `svg_render.go` (dispatch)
- Create: `svg_render_image_test.go`

### Step 1: Read existing image-add infrastructure

```
grep -n "func .*AddImageFromStream\|func .*registerImage" image_add.go
```

Look for the function that creates a PDF Image XObject from image bytes + format and registers it as a page resource. Either:
- `(*Page).AddImageFromStream(r, rect)` — public API; uses bytes from a Reader
- An internal helper that does just the XObject creation + resource registration

If a low-level helper exists (returns the XObject resource name like "/Im0"), use it. Otherwise, the simplest path is to call `AddImageFromStream` with a `bytes.Reader` — but that emits its own `q ... Q + cm + Do + Q` content. For SVG we want to control the `cm` matrix ourselves (to honor SVG's positioning + transforms + preserveAspectRatio).

**If `AddImageFromStream` is the only option**, you may need to extract an internal helper `(*Page).addImageXObject(data []byte, format ImageFormat) (resourceName string, intrinsicW, intrinsicH float64, err error)` from `image_add.go`. State your decision in the status report.

### Step 2: Create `svg_render_image.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// renderSVGImage emits the PDF operators to draw an embedded raster image.
func renderSVGImage(buf *bytes.Buffer, p *Page, im *svgImage) {
	if !im.style.display || im.w <= 0 || im.h <= 0 || len(im.data) == 0 {
		return
	}

	// Register the image as an XObject on the page. Use the appropriate
	// helper from image_add.go (likely `addImageXObject` or similar after
	// possible extraction from AddImageFromStream).
	resName, intrinsicW, intrinsicH, err := p.addSVGImageXObject(im.data, im.format)
	if err != nil {
		return // best-effort: skip on encoding errors
	}

	// preserveAspectRatio mapping: compute fit-within-rect matrix.
	dstW, dstH := im.w, im.h
	// If preserveAspectRatio is "none", just fill rect; otherwise honor.
	var renderW, renderH, alignX, alignY float64
	if im.par.align == "none" || intrinsicW == 0 || intrinsicH == 0 {
		renderW, renderH = dstW, dstH
	} else {
		sx := dstW / intrinsicW
		sy := dstH / intrinsicH
		var s float64
		if im.par.meetOrSlice == "slice" {
			s = sx
			if sy > s { s = sy }
		} else {
			s = sx
			if sy < s { s = sy }
		}
		renderW = intrinsicW * s
		renderH = intrinsicH * s
		// alignment within rect (mirrors svg_viewbox.go logic):
		switch {
		case stringHasPrefix(im.par.align, "xMin"):
			alignX = 0
		case stringHasPrefix(im.par.align, "xMax"):
			alignX = dstW - renderW
		default:
			alignX = (dstW - renderW) / 2
		}
		switch {
		case stringHasSuffix(im.par.align, "YMin"):
			alignY = dstH - renderH // SVG Y-min = "top"; PDF Y-up after outer flip
		case stringHasSuffix(im.par.align, "YMax"):
			alignY = 0
		default:
			alignY = (dstH - renderH) / 2
		}
	}

	buf.WriteString("q\n")
	if im.transform != nil {
		writeCMOperator(buf, *im.transform)
	}
	// Place the unit-square image XObject:
	// [w 0 0 h x y] cm   — translates and scales unit square to render rect.
	fmt.Fprintf(buf, "%s 0 0 %s %s %s cm\n",
		formatFloat(renderW),
		formatFloat(renderH),
		formatFloat(im.x+alignX),
		formatFloat(im.y+alignY))
	fmt.Fprintf(buf, "%s Do\n", resName)
	buf.WriteString("Q\n")
}

// stringHasPrefix / stringHasSuffix wrappers for readability (replace with
// direct strings.HasPrefix / strings.HasSuffix and add "strings" import).
```

(Fix the helpers — just use `strings.HasPrefix` / `strings.HasSuffix` directly with the `strings` import. The placeholders above are just to avoid an import error if not careful.)

### Step 3: Add the `addSVGImageXObject` helper

If `image_add.go` doesn't already have a public-ish helper that creates an XObject and returns its name + intrinsic dimensions, **extract one**. The function should:
1. Decode image bytes (PNG/JPEG header to get intrinsic w/h, or full decode if needed)
2. Construct the PDF Image XObject (similar to what `(*Page).AddImageFromStream` does internally)
3. Register in page `/Resources/XObject/Imx` map
4. Return ("/Imx", intrinsicW, intrinsicH, nil)

Look at `(*Page).AddImageFromStream` body — the parts before `cm`/`Do`/`Q` emission are exactly what we need.

### Step 4: Wire into `svg_render.go`

In `renderSVGNode`'s type switch, add:

```go
case *svgImage:
	renderSVGImage(buf, p, node)
```

### Step 5: Smoke test in `svg_render_image_test.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderSVG_ImageEmitsDo(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/image_inline_png.svg")
	svg, _ := parseSVGBytes(data)
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, _ := page.contentStreams()
	for _, want := range []string{" cm\n", " Do\n", "q\n", "Q\n"} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("missing %q in stream", want)
		}
	}
}
```

### Step 6: Run + commit

```
go test -run "TestRenderSVG_Image" -v ./...
go test ./...
```

```
feat: svg — render <image> via PDF Image XObject (data:image/png|jpeg)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

Stage `svg_render_image.go`, `svg_render_image_test.go`, `svg_render.go`, `image_add.go` (if helper extracted).

---

## Task 4: `svgUse` / `svgSymbol` IR types + `SVG.defs` generalization

**Files:**
- Create: `svg_use.go`
- Modify: `svg_types.go` (extend SVG struct)

### Step 1: Create `svg_use.go` with types only

```go
// SPDX-License-Identifier: MIT

package asposepdf

// svgUse is a placeholder before resolveUseReferences replaces it with the
// cloned referent. After resolution, no *svgUse nodes remain in the IR tree.
type svgUse struct {
	refID     string
	x, y      float64
	style     svgStyle
	transform *svgMatrix
}

func (*svgUse) svgNodeKind() string { return "use" }

// svgSymbol is a container with its own viewBox. Not rendered directly;
// only referenced via <use href="#symbolId">.
type svgSymbol struct {
	viewBox  *svgViewBox
	children []svgNode
	style    svgStyle
}

func (*svgSymbol) svgNodeKind() string { return "symbol" }
```

### Step 2: Extend `SVG` struct in `svg_types.go`

Add `defs` field (in addition to existing `gradients`):

```go
type SVG struct {
	// ... existing fields including gradients ...

	// Phase 3c: generalized definition registry. Any element with `id` attribute
	// (inside <defs> or top-level) is collected here for <use> reference resolution
	// and clip-path lookup. Keys are bare ids (no #).
	defs map[string]svgNode
}
```

### Step 3: Initialize `defs` in `parseSVGRoot` (svg_parse.go)

Find `parseSVGRoot` and add `defs: make(map[string]svgNode)` to the SVG struct literal initialization alongside `gradients`.

### Step 4: Build + run tests

```
go build ./...
go test ./...
```

No regressions; types just added.

### Step 5: Commit

```
feat: svg — svgUse/svgSymbol IR types + SVG.defs generalized definitions registry

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 5: `<use>`/`<symbol>`/extended `<defs>` parsers

**Files:**
- Create: `svg_parse_use.go`
- Modify: `svg_parse.go` (extend defs walker + dispatch use/symbol)
- Create: `svg_parse_use_test.go`
- Create fixtures: `testdata/svg/use_simple.svg`, `testdata/svg/use_symbol.svg`

### Step 1: Create fixtures

`testdata/svg/use_simple.svg`:
```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <circle id="dot" cx="0" cy="0" r="5" fill="red"/>
  </defs>
  <use href="#dot" x="20" y="20"/>
  <use href="#dot" x="50" y="50"/>
</svg>
```

`testdata/svg/use_symbol.svg`:
```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
  <defs>
    <symbol id="star" viewBox="0 0 10 10">
      <polygon points="5,1 7,4 10,5 7,6 5,9 3,6 0,5 3,4" fill="gold"/>
    </symbol>
  </defs>
  <use href="#star" x="0" y="0" width="50" height="50"/>
  <use href="#star" x="100" y="0" width="50" height="50"/>
</svg>
```

### Step 2: Tests

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_UseStoresPlaceholder(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_simple.svg")
	svg, err := parseSVGBytes(data)
	if err != nil { t.Fatal(err) }
	// After parse but BEFORE resolveUseReferences, the IR contains *svgUse nodes.
	// (Task 6 wires in the resolve step; for now we verify the parse layer alone.)
	useCount := 0
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			useCount++
		}
	}
	if useCount != 2 {
		t.Errorf("expected 2 *svgUse nodes, got %d", useCount)
	}
	// defs should contain the dot
	if _, ok := svg.defs["dot"].(*svgCircle); !ok {
		t.Errorf("defs[dot] = %T, want *svgCircle", svg.defs["dot"])
	}
}

func TestParseSVG_SymbolStored(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_symbol.svg")
	svg, _ := parseSVGBytes(data)
	sym, ok := svg.defs["star"].(*svgSymbol)
	if !ok {
		t.Fatalf("defs[star] = %T", svg.defs["star"])
	}
	if sym.viewBox == nil || sym.viewBox.w != 10 || sym.viewBox.h != 10 {
		t.Errorf("symbol viewBox = %+v", sym.viewBox)
	}
	if len(sym.children) == 0 {
		t.Error("symbol has no children")
	}
}
```

### Step 3: Create `svg_parse_use.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGUse reads a <use> element into a placeholder. resolveUseReferences
// (called post-parse) replaces the placeholder with the cloned referent.
func parseSVGUse(d *xml.Decoder, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	u := &svgUse{style: parent.style}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			u.x, _ = parseSVGLength(a.Value)
		case "y":
			u.y, _ = parseSVGLength(a.Value)
		case "href":
			u.refID = strings.TrimPrefix(strings.TrimSpace(a.Value), "#")
		case "transform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				u.transform = &m
			}
		}
	}
	applySVGStyleAttrs(&u.style, start.Attr)
	if err := d.Skip(); err != nil { return nil, err }
	if u.refID == "" {
		return nil, nil
	}
	return u, nil
}

// parseSVGSymbol reads a <symbol> element. Its children are parsed recursively;
// the symbol itself is stored in svg.defs but does NOT appear in the rendering tree.
func parseSVGSymbol(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
	s := &svgSymbol{style: parent.style}
	for _, a := range start.Attr {
		if a.Name.Local == "viewBox" {
			if vb, ok := parseViewBox(a.Value); ok {
				s.viewBox = &vb
			}
		}
	}
	applySVGStyleAttrs(&s.style, start.Attr)

	// Walk children into a temporary group
	tmpGroup := &svgGroup{style: s.style}
	if err := parseSVGChildren(d, svg, tmpGroup); err != nil {
		return nil, err
	}
	s.children = tmpGroup.children

	// Register in defs if it has an id
	if id := findAttr(start.Attr, "id"); id != "" {
		svg.defs[id] = s
	}
	// <symbol> is NOT added to the rendering tree.
	return nil, nil
}
```

### Step 4: Extend `<defs>` walker + dispatch in `svg_parse.go`

(a) Modify the `<defs>` walker (likely `parseSVGDefs` from Phase 3a) to collect all id'd elements, not just gradients:

```go
func parseSVGDefs(d *xml.Decoder, svg *SVG) error {
	for {
		tok, err := d.Token()
		if err != nil { return err }
		switch t := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			id := findAttr(t.Attr, "id")
			switch t.Name.Local {
			case "linearGradient":
				if id != "" {
					svg.gradients[id] = parseSVGLinearGradient(d, t)
				} else {
					_ = d.Skip()
				}
			case "radialGradient":
				if id != "" {
					svg.gradients[id] = parseSVGRadialGradient(d, t)
				} else {
					_ = d.Skip()
				}
			case "symbol":
				// parseSVGSymbol stores itself in svg.defs and returns nil
				_, _ = parseSVGSymbol(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
			case "clipPath":
				// Task 8 will fill this in; for now, skip
				_ = d.Skip()
			default:
				if id != "" {
					// Generic element with id — parse via the main dispatcher,
					// store in defs (not added to rendering tree)
					child, err := parseSVGElement(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
					if err != nil { return err }
					if child != nil {
						svg.defs[id] = child
					}
				} else {
					_ = d.Skip()
				}
			}
		}
	}
}
```

(b) Add dispatch for `<use>` and top-level `<symbol>` to `parseSVGElement`:

```go
case "use":
	return parseSVGUse(d, parent, start)
case "symbol":
	// Top-level <symbol> (not inside <defs>) — still register in defs but don't render
	return parseSVGSymbol(d, svg, parent, start)
```

### Step 5: Run tests

```
go test -run "TestParseSVG_(Use|Symbol)" -v ./...
go test ./...
```

### Step 6: Commit

```
feat: svg — parse <use>/<symbol>; extend <defs> walker to collect any id'd element

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 6: `resolveUseReferences` (parse-end deep-clone pass)

**Files:**
- Modify: `svg_use.go` (append)
- Modify: `svg_parse.go` (call resolveUseReferences after walking root children)
- Create or modify: `svg_use_test.go` (verify post-resolve IR has no svgUse nodes)

### Step 1: Add `resolveUseReferences` + `deepCloneSVGNode` to `svg_use.go`

```go
// resolveUseReferences walks the IR tree, replacing each *svgUse with a deep
// clone of its referent (or nil if the ref is missing or cyclic). Called once
// at end of parseSVGBytes / parseSVGReader, after the full tree has been built
// and svg.defs is fully populated.
func resolveUseReferences(svg *SVG, node svgNode, visited map[string]bool) svgNode {
	switch n := node.(type) {
	case *svgUse:
		if visited[n.refID] {
			return nil // cycle — drop
		}
		target, ok := svg.defs[n.refID]
		if !ok || target == nil {
			return nil
		}
		visited[n.refID] = true
		cloned := deepCloneSVGNode(target)
		cloned = resolveUseReferences(svg, cloned, visited)
		delete(visited, n.refID)
		return wrapUseReferent(cloned, n)
	case *svgGroup:
		out := make([]svgNode, 0, len(n.children))
		for _, c := range n.children {
			resolved := resolveUseReferences(svg, c, visited)
			if resolved != nil {
				out = append(out, resolved)
			}
		}
		n.children = out
		return n
	}
	return node
}

// wrapUseReferent wraps the cloned referent in a group that applies the use's
// translation + transform + style as defaults.
func wrapUseReferent(referent svgNode, u *svgUse) svgNode {
	if referent == nil { return nil }
	// Build composite matrix: translate(x, y) ∘ use.transform
	matrix := matrixTranslate(u.x, u.y)
	if u.transform != nil {
		matrix = matrixMul(matrix, *u.transform)
	}
	var transformPtr *svgMatrix
	if matrix != matrixIdentity() {
		transformPtr = &matrix
	}
	// If referent is a symbol, expand its children with its viewBox applied
	if sym, ok := referent.(*svgSymbol); ok {
		// Compose viewBox matrix if applicable (Phase 3c keeps simple: treat
		// symbol viewBox as a 1:1 mapping into the use's reference frame)
		_ = sym
		// For Phase 3c, just expand the children directly.
		g := &svgGroup{
			style:     u.style,
			children:  sym.children,
			transform: transformPtr,
		}
		return g
	}
	// Wrap a generic single child in a group
	return &svgGroup{
		style:     u.style,
		children:  []svgNode{referent},
		transform: transformPtr,
	}
}

// deepCloneSVGNode returns a deep copy of the node. Shared parts (like the style
// struct) are copied; slices are reallocated.
func deepCloneSVGNode(n svgNode) svgNode {
	switch v := n.(type) {
	case *svgGroup:
		cloned := &svgGroup{
			transform: v.transform, // pointer copy OK (matrices immutable)
			style:     v.style,
		}
		cloned.children = make([]svgNode, len(v.children))
		for i, c := range v.children {
			cloned.children[i] = deepCloneSVGNode(c)
		}
		return cloned
	case *svgRect:
		cp := *v
		return &cp
	case *svgCircle:
		cp := *v
		return &cp
	case *svgEllipse:
		cp := *v
		return &cp
	case *svgLine:
		cp := *v
		return &cp
	case *svgPolyline:
		cp := *v
		cp.points = append([]Point(nil), v.points...)
		return &cp
	case *svgPolygon:
		cp := *v
		cp.points = append([]Point(nil), v.points...)
		return &cp
	case *svgPath:
		cp := *v
		cp.commands = append([]svgPathOp(nil), v.commands...)
		return &cp
	case *svgImage:
		cp := *v
		cp.data = append([]byte(nil), v.data...)
		return &cp
	case *svgText:
		cp := *v
		cp.runs = append([]svgTextRun(nil), v.runs...)
		return &cp
	case *svgSymbol:
		cloned := &svgSymbol{viewBox: v.viewBox, style: v.style}
		cloned.children = make([]svgNode, len(v.children))
		for i, c := range v.children {
			cloned.children[i] = deepCloneSVGNode(c)
		}
		return cloned
	}
	return n
}
```

### Step 2: Call `resolveUseReferences` after parsing root

In `parseSVGRoot` (svg_parse.go), AFTER `parseSVGChildren(d, svg, svg.root)`, add:

```go
// Resolve all <use> references — replaces *svgUse nodes with deep-cloned referents.
visited := make(map[string]bool)
_ = resolveUseReferences(svg, svg.root, visited)
```

### Step 3: Tests

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestResolveUseReferences_SimpleClone(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_simple.svg")
	svg, _ := parseSVGBytes(data)
	// After parse + resolve, no *svgUse nodes should remain.
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			t.Errorf("expected svgUse to be resolved, found: %+v", c)
		}
	}
	// Two uses → two top-level groups each containing a cloned circle.
	if len(svg.root.children) != 2 {
		t.Errorf("expected 2 wrapped clones, got %d", len(svg.root.children))
	}
}

func TestResolveUseReferences_MissingRefDropped(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<use href="#nonexistent" x="0" y="0"/>
		<rect x="0" y="0" width="10" height="10" fill="red"/>
	</svg>`))
	for _, c := range svg.root.children {
		if _, ok := c.(*svgUse); ok {
			t.Errorf("expected missing-ref use to be dropped, found: %+v", c)
		}
	}
	// rect should still be present
	foundRect := false
	for _, c := range svg.root.children {
		if _, ok := c.(*svgRect); ok {
			foundRect = true
		}
	}
	if !foundRect {
		t.Error("rect was dropped along with the missing use ref")
	}
}
```

### Step 4: Run + commit

```
go test -run "TestResolveUse" -v ./...
go test ./...
```

```
feat: svg — resolveUseReferences (parse-end deep-clone replaces <use> with referent + translate group)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 7: Verify `<use>` end-to-end rendering

**Files:**
- Modify: `svg_use_test.go` (append integration test)

After Task 6, `<use>` resolution produces normal `*svgGroup` nodes that the existing renderer handles automatically. This task just verifies end-to-end.

### Step 1: Smoke test

```go
func TestRenderSVG_UseRendersClones(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/use_simple.svg")
	svg, _ := parseSVGBytes(data)
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 400}); err != nil {
		t.Fatal(err)
	}
	stream, _ := page.contentStreams()
	// Each cloned circle emits a fill + circle path. With 2 clones, expect at
	// least 2 fill ops.
	import "bytes"
	count := bytes.Count(stream, []byte(" rg\n"))
	if count < 2 {
		t.Errorf("expected ≥2 rg (fill color) operators for 2 cloned circles, got %d:\n%s",
			count, stream)
	}
}
```

(Fix the import — move `"bytes"` to the file's import block.)

### Step 2: Run + commit

```
go test -run "TestRenderSVG_Use" -v ./...
```

```
test: svg — verify <use> renders cloned referents end-to-end

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 8: `svgClipPath` IR + parser

**Files:**
- Create: `svg_clip.go`
- Create: `svg_parse_clip.go`
- Modify: `svg_parse.go` (dispatch `<clipPath>` + extend defs walker)
- Create: `svg_clip_test.go`, `testdata/svg/clippath_circle.svg`

### Step 1: Fixture

`testdata/svg/clippath_circle.svg`:
```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <clipPath id="circle-clip">
      <circle cx="50" cy="50" r="40"/>
    </clipPath>
  </defs>
  <rect x="0" y="0" width="100" height="100" fill="red" clip-path="url(#circle-clip)"/>
</svg>
```

### Step 2: Test parser

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_ClipPathStoredInDefs(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/clippath_circle.svg")
	svg, _ := parseSVGBytes(data)
	cp, ok := svg.defs["circle-clip"].(*svgClipPath)
	if !ok {
		t.Fatalf("defs[circle-clip] = %T", svg.defs["circle-clip"])
	}
	if len(cp.children) != 1 {
		t.Errorf("expected 1 clip child, got %d", len(cp.children))
	}
	if _, ok := cp.children[0].(*svgCircle); !ok {
		t.Errorf("clip child[0] = %T, want *svgCircle", cp.children[0])
	}
}
```

### Step 3: Create `svg_clip.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

// svgClipPath defines a clipping path; referenced by shapes via clip-path="url(#id)".
type svgClipPath struct {
	units    svgGradientUnits // reuses enum: userSpaceOnUse | objectBoundingBox
	children []svgNode
}

func (*svgClipPath) svgNodeKind() string { return "clipPath" }
```

### Step 4: Create `svg_parse_clip.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGClipPath reads a <clipPath> element. Children are parsed recursively
// using the standard shape parsers. The clipPath itself is NOT added to the
// rendering tree; if it has an id, the caller stores it in svg.defs.
func parseSVGClipPath(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (*svgClipPath, error) {
	cp := &svgClipPath{units: svgGradientUserSpace}
	for _, a := range start.Attr {
		if a.Name.Local == "clipPathUnits" {
			if strings.TrimSpace(a.Value) == "objectBoundingBox" {
				cp.units = svgGradientObjectBBox
			}
		}
	}
	tmp := &svgGroup{style: parent.style}
	if err := parseSVGChildren(d, svg, tmp); err != nil {
		return nil, err
	}
	cp.children = tmp.children
	return cp, nil
}
```

### Step 5: Wire into walker — both inside `<defs>` and top-level

In `parseSVGDefs`, add the `clipPath` case (replacing the Task 5 placeholder `_ = d.Skip()`):

```go
case "clipPath":
	cp, err := parseSVGClipPath(d, svg, &svgGroup{style: defaultSVGStyle()}, t)
	if err != nil { return err }
	if id != "" {
		svg.defs[id] = cp
	}
```

In `parseSVGElement`, add top-level dispatch:

```go
case "clipPath":
	cp, err := parseSVGClipPath(d, svg, parent, start)
	if err != nil { return nil, err }
	if id := findAttr(start.Attr, "id"); id != "" {
		svg.defs[id] = cp
	}
	return nil, nil // not rendered directly
```

### Step 6: Run + commit

```
go test -run "TestParseSVG_ClipPath" -v ./...
go test ./...
```

```
feat: svg — svgClipPath IR + parser (collect into defs; not in render tree)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 9: `clip-path` style property in cascade

**Files:**
- Modify: `svg_parse.go` (or wherever applySingleSVGStyleProp lives)
- Modify: `svg_parse_test.go` (or appropriate test file)

### Step 1: Test

```go
func TestApplyStyle_ClipPath(t *testing.T) {
	s := defaultSVGStyle()
	applySingleSVGStyleProp(&s, "clip-path", "url(#myclip)")
	if s.clipPath != "myclip" {
		t.Errorf("clipPath = %q, want 'myclip'", s.clipPath)
	}
	applySingleSVGStyleProp(&s, "clip-path", "none")
	if s.clipPath != "" {
		t.Errorf("clipPath should be cleared by 'none', got %q", s.clipPath)
	}
}
```

### Step 2: Add `clipPath` field to `svgStyle`

In `svg_types.go`, append to svgStyle:

```go
type svgStyle struct {
	// ... existing ...
	clipPath string // bare id (no #); empty = no clip
}
```

(Default zero value `""` is correct — no extension to `defaultSVGStyle` needed.)

### Step 3: Add case in `applySingleSVGStyleProp`

```go
case "clip-path":
	v := strings.TrimSpace(val)
	if v == "none" || v == "" {
		s.clipPath = ""
	} else if strings.HasPrefix(v, "url(") {
		end := strings.IndexByte(v, ')')
		if end > 0 {
			id := strings.Trim(v[4:end], "# \t")
			s.clipPath = id
		}
	}
```

### Step 4: Run + commit

```
go test -run "TestApplyStyle_ClipPath" -v ./...
go test ./...
```

```
feat: svg — handle clip-path="url(#id)" presentation attribute in style cascade

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 10: `<clipPath>` rendering (W operator + per-shape injection)

**Files:**
- Modify: `svg_clip.go` (append emit helper)
- Modify: `svg_render.go` (inject clip emission before each shape)
- Modify: `svg_clip_test.go` (append integration test)

### Step 1: Add `emitClipPathInline` to `svg_clip.go`

```go
import (
	"bytes"
	"fmt"
)

// emitClipPathInline writes path construction ops for all clip children, followed
// by W (nonzero) + n. The caller has already emitted q; the clip remains active
// until the matching Q.
//
// When units == objectBoundingBox, the caller must have applied the shape-bbox
// transform before calling this function (Phase 3c renderer does this).
func emitClipPathInline(buf *bytes.Buffer, p *Page, cp *svgClipPath) {
	if cp == nil || len(cp.children) == 0 { return }
	for _, c := range cp.children {
		emitClipChildPath(buf, c)
	}
	buf.WriteString("W\nn\n")
}

// emitClipChildPath writes path construction ops (m/l/c/h, NO paint op) for a single
// clip child. Supports rect/circle/ellipse/line/polyline/polygon/path; skips others.
func emitClipChildPath(buf *bytes.Buffer, n svgNode) {
	switch s := n.(type) {
	case *svgRect:
		fmt.Fprintf(buf, "%s %s %s %s re\n",
			formatFloat(s.x), formatFloat(s.y), formatFloat(s.w), formatFloat(s.h))
	case *svgCircle:
		// Convert to 4 cubic Beziers (reuse Phase 1 ellipsePathOps)
		buf.WriteString(ellipsePathOps(s.cx, s.cy, s.r, s.r))
	case *svgEllipse:
		buf.WriteString(ellipsePathOps(s.cx, s.cy, s.rx, s.ry))
	case *svgLine:
		fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.x1), formatFloat(s.y1))
		fmt.Fprintf(buf, "%s %s l\n", formatFloat(s.x2), formatFloat(s.y2))
	case *svgPolyline:
		if len(s.points) >= 1 {
			fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.points[0].X), formatFloat(s.points[0].Y))
			for _, pt := range s.points[1:] {
				fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
			}
		}
	case *svgPolygon:
		if len(s.points) >= 1 {
			fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.points[0].X), formatFloat(s.points[0].Y))
			for _, pt := range s.points[1:] {
				fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
			}
			buf.WriteString("h\n")
		}
	case *svgPath:
		path := NewPath()
		for _, op := range s.commands {
			switch op.kind {
			case 'M': path.MoveTo(op.args[0], op.args[1])
			case 'L': path.LineTo(op.args[0], op.args[1])
			case 'C': path.CurveTo(op.args[0], op.args[1], op.args[2], op.args[3], op.args[4], op.args[5])
			case 'Q': path.QuadTo(op.args[0], op.args[1], op.args[2], op.args[3])
			case 'Z': path.Close()
			}
		}
		buf.WriteString(pathOpsToOperators(path.ops))
	}
}
```

### Step 2: Modify each renderSVG<Shape> to inject clip emission

A clean way: add a helper that's called between `q\n` (and transform) and the shape's paint emission:

```go
// In svg_render.go (add helper):
func applyClipPath(buf *bytes.Buffer, p *Page, svg *SVG, style svgStyle) {
	if style.clipPath == "" || svg == nil { return }
	cp, ok := svg.defs[style.clipPath].(*svgClipPath)
	if !ok { return }
	emitClipPathInline(buf, p, cp)
}
```

Then in each `renderSVGRect`, `renderSVGCircle`, ..., `renderSVGPath`, `renderSVGText`, after the `q\n` + transform but BEFORE the shape's `emit*ToBuf`, add:

```go
applyClipPath(buf, p, svg, r.style) // or c.style, e.style, etc.
```

(Don't forget to also do this for `renderSVGGroup` if groups can have clip-path.)

### Step 3: Test

```go
func TestRenderSVG_ClipPathEmitsW(t *testing.T) {
	import "bytes"
	data, _ := os.ReadFile("testdata/svg/clippath_circle.svg")
	svg, _ := parseSVGBytes(data)
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200})
	stream, _ := page.contentStreams()
	if !bytes.Contains(stream, []byte("W\n")) {
		t.Errorf("expected W (clip) operator in stream:\n%s", stream)
	}
	if !bytes.Contains(stream, []byte("\nn\n")) {
		t.Error("expected n (end path) after W")
	}
}
```

(Move "bytes" import to the file's import block.)

### Step 4: Run + commit

```
go test -run "TestRenderSVG_ClipPath" -v ./...
go test ./...
```

```
feat: svg — render <clipPath> via W + n; clip-path style triggers per-shape clip injection

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 11: Integration tests + AES round-trip

**Files:**
- Modify: `svg_test.go` (external `package asposepdf_test`)
- Create or use existing fixtures

### Step 1: Add integration tests

```go
func TestPage_AddSVG_ImageInline(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/image_inline_png.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	if err := doc.Save("result_files/TestPage_AddSVG_ImageInline.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_UseRefs(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/use_simple.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_UseRefs.pdf")
}

func TestPage_AddSVG_ClipPath(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/clippath_circle.svg",
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	doc.Save("result_files/TestPage_AddSVG_ClipPath.pdf")
}

func TestAddSVG_DefsImageClipAES128Roundtrip(t *testing.T) {
	for _, fixture := range []string{"image_inline_png.svg", "use_simple.svg", "clippath_circle.svg"} {
		t.Run(fixture, func(t *testing.T) {
			doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
			page, _ := doc.Page(1)
			page.AddSVG("testdata/svg/"+fixture, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800})
			doc.SetEncryption(pdf.EncryptionOptions{
				UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128,
			})
			os.MkdirAll("result_files", 0755)
			out := "result_files/TestAddSVG_AES_" + fixture + ".pdf"
			doc.Save(out)
			if _, err := pdf.OpenWithPassword(out, "u"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
```

### Step 2: Run

```
go test -run "TestPage_AddSVG_(ImageInline|UseRefs|ClipPath)|TestAddSVG_DefsImageClipAES" -v ./...
go test ./...
```

### Step 3: Commit

```
test: svg — Phase 3c integration tests (image + use + clipPath + AES round-trip)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

---

## Task 12: Documentation + close beads + final cleanup

**Files:**
- Modify: `CLAUDE.md` (SVG block)
- Modify: `README.md` (SVG embedding section)

### Step 1: Update CLAUDE.md SVG block

Find the SVG documentation block. Add a new line after the Phase 3b entry:

```markdown
- **Added in Phase 3c**: `<image>` (data:image/png and data:image/jpeg base64 inline; external URLs silently skipped); `<defs>`/`<use>`/`<symbol>` (reusable elements with parse-end deep-clone resolution, forward refs supported, cycle detection); `<clipPath>` (children = shape elements, `clipPathUnits` userSpaceOnUse + objectBoundingBox, multi-child union, maps to PDF `W`/`W*` operators); `clip-path="url(#id)"` presentation attribute on any shape/text/image.
```

Update "Out of scope" — remove `<image>`, `<defs>`/`<use>`, masks/clipPath partial mentions; keep `<mask>`, external image URLs, data:image/svg+xml recursion, CSS shape clip-path.

### Step 2: Update README.md SVG embedding section

Add to the supported list: `<image>` (data URIs), `<use>`/`<symbol>` (reusable elements), `<clipPath>` (clipping). Remove from unsupported list.

### Step 3: gofmt -s sweep

```
gofmt -s -l .
```

Apply if needed:
```
gofmt -s -w .
git add -u
```

### Step 4: Close beads

```
bd update pdf-go-tq5 --status closed
```

### Step 5: Final commit

```
feat: svg — Phase 3c shipped (image + use/symbol + clipPath + docs)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

(Plus a separate `style: apply gofmt -s after Phase 3c` commit if gofmt found anything.)

---

## Self-Review

Coverage:
- IR types ✅ (Tasks 1, 4, 8)
- Image parser + renderer ✅ (Tasks 2, 3)
- Use parser + resolver + rendering ✅ (Tasks 5, 6, 7)
- ClipPath parser + cascade + renderer ✅ (Tasks 8, 9, 10)
- Integration + AES ✅ (Task 11)
- Docs + beads ✅ (Task 12)

Implementer freedom:
- Task 3: may need to extract a helper from `image_add.go` for image XObject creation
- Task 10: clip-injection requires touching every renderSVG<Shape> — be thorough; missing one means clip-path silently fails for that shape type

Risk: clipPath rendering interacts with gradient fill (both wrap shapes in q/Q). Order matters: emit clip path BEFORE pattern setter. Should naturally fall out since clip is path-only ops + W (no paint), then the shape's actual rendering follows.
