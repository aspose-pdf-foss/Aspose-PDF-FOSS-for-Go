# AddText Rotation & Behind Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add text rotation (arbitrary angle) and behind-content mode to the existing `AddText` method.

**Architecture:** Two new fields on `TextStyle`: `Rotation float64` (degrees CCW, pivot at rect's lower-left corner) and `Behind bool` (prepend operators to content stream instead of append). Rotation uses PDF `cm` operator to translate+rotate. Behind uses a new `prependToContentStream` method. Both features are orthogonal.

**Tech Stack:** Pure Go, no new dependencies. PDF content stream operators (`cm` for coordinate transformation).

---

## File Structure

| File | Change |
|------|--------|
| `color.go` | Add `Rotation float64` and `Behind bool` fields to `TextStyle` |
| `text_add.go` | Add `prependToContentStream`; modify `AddText` to emit rotation `cm` operators and choose prepend vs append |
| `text_add_test.go` | 4 new unit tests: rotation, rotation-zero, behind, combined |
| `text_add_integration_test.go` | 1 new integration test: rotated text behind content |
| `CLAUDE.md` | Update `TextStyle` field list |

---

### Task 1: Add Rotation and Behind fields to TextStyle

**Files:**
- Modify: `color.go:50-60`

- [ ] **Step 1: Add the two new fields to TextStyle**

In `color.go`, add `Rotation` and `Behind` fields to the `TextStyle` struct. Add them after `Strikethrough`:

```go
// TextStyle defines reusable text formatting properties.
type TextStyle struct {
	Font          Font
	Size          float64 // in points; 0 treated as 12
	Color         *Color  // nil → black opaque {0,0,0,1}
	Background    *Color  // nil → no background
	HAlign        HAlign  // default: HAlignLeft
	VAlign        VAlign  // default: VAlignTop
	LineSpacing   float64 // multiplier of font size; 0 treated as 1.2
	Underline     bool
	Strikethrough bool
	Rotation      float64 // degrees counter-clockwise; pivot = lower-left corner of rect; default 0
	Behind        bool    // if true, text is drawn under existing page content; default false
}
```

- [ ] **Step 2: Run tests to verify nothing breaks**

Run: `go test ./...`
Expected: All existing tests PASS (new fields have zero-value defaults).

- [ ] **Step 3: Commit**

```bash
git add color.go
git commit -m "feat: add Rotation and Behind fields to TextStyle"
```

---

### Task 2: Add prependToContentStream and unit test for Behind

**Files:**
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Write failing test for Behind mode**

Add to `text_add_test.go`:

```go
func TestAddTextBehind(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Add initial text (appears first in content stream).
	err := page.AddText("Foreground", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText foreground: %v", err)
	}

	// Add behind text (should appear before the foreground text).
	err = page.AddText("Background", TextStyle{Behind: true}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText behind: %v", err)
	}

	data, _ := page.contentStreams()
	content := string(data)
	bgIdx := strings.Index(content, "(Background) Tj")
	fgIdx := strings.Index(content, "(Foreground) Tj")
	if bgIdx < 0 || fgIdx < 0 {
		t.Fatalf("missing text operators; content:\n%s", content)
	}
	if bgIdx > fgIdx {
		t.Errorf("Behind text should appear before foreground text in content stream; bg at %d, fg at %d", bgIdx, fgIdx)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestAddTextBehind -v ./...`
Expected: FAIL — behind text appears after foreground text (both use `appendToContentStream`).

- [ ] **Step 3: Implement prependToContentStream**

Add to `text_add.go`, after the `AddText` method (before `ensureFontResource`):

```go
// prependToContentStream inserts data before the existing page content.
func (p *Page) prependToContentStream(data []byte) error {
	existing, err := p.contentStreams()
	if err != nil {
		return err
	}

	newData := make([]byte, 0, len(data)+len(existing))
	newData = append(newData, data...)
	newData = append(newData, existing...)
	newStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    newData,
		Decoded: true,
	}

	newID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[newID] = &pdfObject{Num: newID, Value: newStream}

	pageDict := p.pageDict()
	pageDict["/Contents"] = pdfRef{Num: newID}
	return nil
}
```

- [ ] **Step 4: Update AddText to use prependToContentStream when Behind is true**

At the end of the `AddText` method in `text_add.go`, replace the final return line:

```go
return p.appendToContentStream([]byte(buf.String()))
```

with:

```go
if style.Behind {
	return p.prependToContentStream([]byte(buf.String()))
}
return p.appendToContentStream([]byte(buf.String()))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestAddTextBehind -v ./...`
Expected: PASS

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add text_add.go text_add_test.go
git commit -m "feat: add prependToContentStream and Behind support for AddText"
```

---

### Task 3: Add rotation support and unit tests

**Files:**
- Modify: `text_add.go`
- Modify: `text_add_test.go`

- [ ] **Step 1: Write failing test for rotation**

Add to `text_add_test.go`:

```go
func TestAddTextRotation(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := TextStyle{Rotation: 45}
	err := page.AddText("Rotated", style, Rectangle{LLX: 100, LLY: 500, URX: 200, URY: 600})
	if err != nil {
		t.Fatalf("AddText rotation: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	// Should contain cm operators for translation and rotation.
	if !strings.Contains(content, "cm") {
		t.Error("content stream missing cm operator for rotation")
	}
	// Should still contain text operators.
	if !strings.Contains(content, "(Rotated) Tj") {
		t.Error("content stream missing text")
	}
}
```

- [ ] **Step 2: Write test that rotation=0 produces no cm operators**

Add to `text_add_test.go`:

```go
func TestAddTextRotationZero(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("NoRotation", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "cm") {
		t.Error("content stream should not contain cm operator when Rotation is 0")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test -run "TestAddTextRotation$" -v ./...`
Expected: FAIL — no `cm` operator in content stream.

Run: `go test -run "TestAddTextRotationZero" -v ./...`
Expected: PASS (no cm currently emitted — this confirms no regression).

- [ ] **Step 4: Implement rotation in AddText**

In `text_add.go`, add `"math"` to the import block:

```go
import (
	"fmt"
	"math"
	"strings"
)
```

In the `AddText` method, replace the block that builds the content stream operators. The key change: when `Rotation != 0`, emit `cm` operators after `q` and adjust all coordinates to be relative to (0, 0) instead of (LLX, LLY).

Replace the content stream building section (from `var buf strings.Builder` through `buf.WriteString("Q\n")`) with:

```go
	// Build content stream operators.
	var buf strings.Builder

	// Coordinate offsets: when rotated, all positions are relative to pivot (0,0).
	// When not rotated, positions are absolute.
	ox := 0.0 // offset X to subtract from all coordinates
	oy := 0.0 // offset Y to subtract from all coordinates

	// Save state + optional rotation transform.
	buf.WriteString("\nq\n")

	if style.Rotation != 0 {
		// Translate origin to pivot point (LLX, LLY), then rotate.
		buf.WriteString(fmt.Sprintf("1 0 0 1 %s %s cm\n",
			formatFloat(rect.LLX), formatFloat(rect.LLY)))
		rad := style.Rotation * math.Pi / 180.0
		cos := math.Cos(rad)
		sin := math.Sin(rad)
		buf.WriteString(fmt.Sprintf("%s %s %s %s 0 0 cm\n",
			formatFloat(cos), formatFloat(sin), formatFloat(-sin), formatFloat(cos)))
		// All subsequent coordinates are relative to pivot.
		ox = rect.LLX
		oy = rect.LLY
	}

	// Clipping path.
	buf.WriteString(fmt.Sprintf("%s %s %s %s re W n\n",
		formatFloat(rect.LLX-ox), formatFloat(rect.LLY-oy),
		formatFloat(rectWidth), formatFloat(rectHeight)))

	// Background fill.
	if style.Background != nil && style.Background.A > 0 {
		if bgGSName != "" {
			buf.WriteString(fmt.Sprintf("%s gs\n", bgGSName))
		}
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(style.Background.R), formatFloat(style.Background.G), formatFloat(style.Background.B)))
		buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(rect.LLX-ox), formatFloat(rect.LLY-oy),
			formatFloat(rectWidth), formatFloat(rectHeight)))
	}

	// Text opacity.
	if textGSName != "" {
		buf.WriteString(fmt.Sprintf("%s gs\n", textGSName))
	}

	// Text block.
	buf.WriteString("BT\n")
	buf.WriteString(fmt.Sprintf("%s %s Tf\n", fontResName, formatFloat(fontSize)))
	buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
		formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))

	// Track positions for underline/strikethrough.
	type linePos struct {
		x, y, width float64
	}
	var linePositions []linePos

	for i, line := range lines {
		if line == "" {
			continue
		}
		lineWidth := measureString(line, widths, fontSize)

		// Horizontal alignment.
		var x float64
		switch style.HAlign {
		case HAlignCenter:
			x = rect.LLX + (rectWidth-lineWidth)/2
		case HAlignRight:
			x = rect.LLX + (rectWidth - lineWidth)
		default: // HAlignLeft
			x = rect.LLX
		}

		// Baseline Y: top of line area minus ascent.
		ascent := 0.8 * fontSize
		y := startY - float64(i)*lineHeight - ascent

		// Apply coordinate offset for rotation.
		adjX := x - ox
		adjY := y - oy

		if len(linePositions) == 0 {
			buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(adjX), formatFloat(adjY)))
		} else {
			prevX := linePositions[len(linePositions)-1].x
			prevY := linePositions[len(linePositions)-1].y
			buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(adjX-prevX), formatFloat(adjY-prevY)))
		}

		buf.WriteString(fmt.Sprintf("(%s) Tj\n", escapeStringPDF(line)))
		linePositions = append(linePositions, linePos{x: adjX, y: adjY, width: lineWidth})
	}

	buf.WriteString("ET\n")

	// Underline.
	if style.Underline && len(linePositions) > 0 {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			ulY := lp.y - fontSize*0.1
			buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(ulY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Strikethrough.
	if style.Strikethrough && len(linePositions) > 0 {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			stY := lp.y + fontSize*0.3
			buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(stY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Restore state.
	buf.WriteString("Q\n")
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run "TestAddTextRotation" -v ./...`
Expected: Both `TestAddTextRotation` and `TestAddTextRotationZero` PASS.

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add text_add.go text_add_test.go
git commit -m "feat: add rotation support to AddText via cm operators"
```

---

### Task 4: Unit test for Behind + Rotation combined

**Files:**
- Modify: `text_add_test.go`

- [ ] **Step 1: Write test for combined Behind + Rotation**

Add to `text_add_test.go`:

```go
func TestAddTextBehindAndRotation(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Add foreground text first.
	err := page.AddText("Foreground", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText foreground: %v", err)
	}

	// Add rotated text behind.
	err = page.AddText("Watermark", TextStyle{
		Rotation: 45,
		Behind:   true,
	}, Rectangle{LLX: 100, LLY: 300, URX: 500, URY: 700})
	if err != nil {
		t.Fatalf("AddText behind+rotation: %v", err)
	}

	data, _ := page.contentStreams()
	content := string(data)

	// Watermark should appear before foreground.
	wmIdx := strings.Index(content, "(Watermark) Tj")
	fgIdx := strings.Index(content, "(Foreground) Tj")
	if wmIdx < 0 || fgIdx < 0 {
		t.Fatalf("missing text operators; content:\n%s", content)
	}
	if wmIdx > fgIdx {
		t.Errorf("watermark should appear before foreground; wm at %d, fg at %d", wmIdx, fgIdx)
	}

	// Watermark block should contain cm operators.
	wmBlock := content[:fgIdx]
	if !strings.Contains(wmBlock, "cm") {
		t.Error("watermark block missing cm operator for rotation")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test -run TestAddTextBehindAndRotation -v ./...`
Expected: PASS (both features already implemented in Tasks 2 and 3).

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add text_add_test.go
git commit -m "test: add combined Behind + Rotation unit test for AddText"
```

---

### Task 5: Integration test for rotation round-trip

**Files:**
- Modify: `text_add_integration_test.go`

- [ ] **Step 1: Write integration test**

Add to `text_add_integration_test.go`:

```go
func TestAddTextRotationRoundTrip(t *testing.T) {
	// Create a blank A4 document.
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	// Add normal foreground text.
	err = page.AddText("Normal text", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText normal: %v", err)
	}

	// Add rotated text behind content (watermark-style).
	gray := asposepdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3}
	err = page.AddText("CONFIDENTIAL", asposepdf.TextStyle{
		Font:     asposepdf.FontHelveticaBold,
		Size:     60,
		Color:    &gray,
		Rotation: 45,
		HAlign:   asposepdf.HAlignCenter,
		VAlign:   asposepdf.VAlignMiddle,
		Behind:   true,
	}, asposepdf.Rectangle{LLX: 50, LLY: 200, URX: 545, URY: 650})
	if err != nil {
		t.Fatalf("AddText rotated behind: %v", err)
	}

	// Save.
	outDir := filepath.Join("result_files", "TestAddTextRotationRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Reopen and extract text — both texts should be present.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) == 0 {
		t.Fatal("no text extracted")
	}
	if !strings.Contains(texts[0], "Normal") {
		t.Errorf("extracted text missing 'Normal': %q", texts[0])
	}
	if !strings.Contains(texts[0], "CONFIDENTIAL") {
		t.Errorf("extracted text missing 'CONFIDENTIAL': %q", texts[0])
	}
}
```

- [ ] **Step 2: Run integration test**

Run: `go test -run TestAddTextRotationRoundTrip -v ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add text_add_integration_test.go
git commit -m "test: add rotation round-trip integration test for AddText"
```

---

### Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update TextStyle description in CLAUDE.md**

Find the line:
```
- `TextStyle` struct — Font, Size, Color, Background, HAlign, VAlign, LineSpacing, Underline, Strikethrough
```

Replace with:
```
- `TextStyle` struct — Font, Size, Color, Background, HAlign, VAlign, LineSpacing, Underline, Strikethrough, Rotation, Behind
```

- [ ] **Step 2: Run tests to verify nothing broke**

Run: `go test ./...`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add Rotation and Behind to TextStyle in CLAUDE.md"
```
