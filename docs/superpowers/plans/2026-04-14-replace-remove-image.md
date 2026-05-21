# ReplaceImage & RemoveImage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add methods to replace and remove images on existing PDF pages via the `ImageInfo` type.

**Architecture:** Methods on `*ImageInfo` leverage existing private fields (stream, objects, Name) plus a new `page *Page` field. Replace modifies the XObject stream in place. Remove filters the content stream and cleans page resources. A new `serializeContentOps` function converts parsed operators back to bytes.

**Tech Stack:** Pure Go, no external dependencies. Uses existing `parseContentStream`, `createImageXObject`, `detectImageFormat`, `formatFloat`.

---

### Task 1: Add `page *Page` field to ImageInfo

**Files:**
- Modify: `image.go`
- Modify: `image_internal_test.go` (if collectImageInfos call signature changes)

- [ ] **Step 1: Write failing test**

Create `image_replace_test.go`:

```go
package asposepdf

import (
	"os"
	"testing"
)

func TestImageInfoHasPage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image info")
	}
	if infos[0].page == nil {
		t.Error("ImageInfo.page should be set")
	}
}

// createDocWithImage builds a Document with one page containing a JPEG XObject.
func createDocWithImage() *Document {
	jpegData := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B, 0x08,
		0x00, 0x0A, 0x00, 0x0A, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xD9,
	}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestImageInfoHasPage -v ./... 2>&1 | head -10`
Expected: FAIL — `infos[0].page` is nil (field exists but not populated).

- [ ] **Step 3: Add `page` field and wire it through**

In `image.go`, add `page *Page` to the `ImageInfo` struct (after the `ctm` field, line 63):

```go
type ImageInfo struct {
	// ... existing fields ...
	ctm     [6]float64
	page    *Page // page this image belongs to (for Replace/Remove)
}
```

Update `collectImageInfos` signature to accept a `*Page`:

```go
func collectImageInfos(objects map[int]*pdfObject, ops []contentOp, resources pdfDict, page *Page) []ImageInfo {
```

In each place where an `ImageInfo` is constructed inside `collectImageInfos` (the `xobjectImageInfo` and `inlineImageInfo` calls), set the `page` field on the returned info. The simplest approach: after the call to `collectImageInfos`, iterate and set page:

Actually, it's cleaner to set it after the function returns. Update `(*Page).ImageInfos()`:

```go
func (p *Page) ImageInfos() ([]ImageInfo, error) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	resources := p.pageResources()
	infos := collectImageInfos(p.doc.objects, ops, resources)
	for i := range infos {
		infos[i].page = p
	}
	return infos, nil
}
```

This approach does NOT change the `collectImageInfos` signature — it keeps the change minimal and localized.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestImageInfoHasPage -v ./...`
Expected: PASS.

- [ ] **Step 5: Run all tests to check for regressions**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image.go image_replace_test.go
git commit -m "feat: add page field to ImageInfo for Replace/Remove support"
```

---

### Task 2: Implement Replace and ReplaceFromStream

**Files:**
- Create: `image_replace.go`
- Modify: `image_replace_test.go`

- [ ] **Step 1: Write failing tests**

Add to `image_replace_test.go`:

```go
func TestReplaceImageJPEG(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	// Create a different JPEG to replace with.
	newJPEG := buildMinimalJPEG(20, 15, 3)
	tmpFile := t.TempDir() + "/new.jpg"
	os.WriteFile(tmpFile, newJPEG, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Verify the stream was updated in place.
	if dictGetInt(infos[0].stream.Dict, "/Width") != 20 {
		t.Errorf("width = %d, want 20", dictGetInt(infos[0].stream.Dict, "/Width"))
	}
	if dictGetInt(infos[0].stream.Dict, "/Height") != 15 {
		t.Errorf("height = %d, want 15", dictGetInt(infos[0].stream.Dict, "/Height"))
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "/DCTDecode" {
		t.Errorf("filter = %s, want /DCTDecode", dictGetName(infos[0].stream.Dict, "/Filter"))
	}
}

func TestReplaceImagePNGToJPEG(t *testing.T) {
	// Build doc with PNG image that has SMask.
	doc := createDocWithPNGImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; !hasSMask {
		t.Fatal("setup: expected PNG image to have SMask")
	}

	newJPEG := buildMinimalJPEG(20, 15, 3)
	tmpFile := t.TempDir() + "/new.jpg"
	os.WriteFile(tmpFile, newJPEG, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; hasSMask {
		t.Error("SMask should be removed after replacing with JPEG")
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "/DCTDecode" {
		t.Error("filter should be /DCTDecode after JPEG replacement")
	}
}

func TestReplaceImageJPEGToPNGWithAlpha(t *testing.T) {
	doc := createDocWithImage() // has JPEG
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	pngData := createTestPNGAlpha(8, 8)
	tmpFile := t.TempDir() + "/new.png"
	os.WriteFile(tmpFile, pngData, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; !hasSMask {
		t.Error("SMask should be added after replacing with PNG with alpha")
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "" {
		t.Error("filter should be empty (Decoded=true, writer adds FlateDecode)")
	}
	if !infos[0].stream.Decoded {
		t.Error("stream should be Decoded=true for PNG replacement")
	}
}

func TestReplaceImageInvalidInfo(t *testing.T) {
	info := &ImageInfo{}
	err := info.Replace("testdata/Koala.jpg")
	if err == nil {
		t.Fatal("expected error for nil stream")
	}
}

// buildMinimalJPEG constructs a minimal JPEG with a SOF0 marker declaring the given dimensions.
func buildMinimalJPEG(width, height, components int) []byte {
	var buf []byte
	buf = append(buf, 0xFF, 0xD8) // SOI
	// SOF0: FF C0, length, precision, height, width, components
	buf = append(buf, 0xFF, 0xC0)
	segLen := 8 + components*3
	buf = append(buf, byte(segLen>>8), byte(segLen))
	buf = append(buf, 0x08) // precision
	buf = append(buf, byte(height>>8), byte(height))
	buf = append(buf, byte(width>>8), byte(width))
	buf = append(buf, byte(components))
	for i := 0; i < components; i++ {
		buf = append(buf, byte(i+1), 0x22, 0x00)
	}
	buf = append(buf, 0xFF, 0xD9) // EOI
	return buf
}

// createDocWithPNGImage builds a Document with one page containing a PNG XObject with SMask.
func createDocWithPNGImage() *Document {
	pngData := createTestPNGAlpha(4, 4)
	imgStream, smaskStream, _ := createImageXObject(pngData, ImageFormatPNG)

	smaskObj := &pdfObject{Num: 1, Value: smaskStream}
	imgStream.Dict["/SMask"] = pdfRef{Num: 1}
	imgObj := &pdfObject{Num: 2, Value: imgStream}

	contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 3, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 2},
			},
		},
		"/Contents": pdfRef{Num: 3},
	}
	pageObj := &pdfObject{Num: 4, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: smaskObj, 2: imgObj, 3: contentObj, 4: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  5,
	}
}

// createTestPNGAlpha creates a small PNG with alpha channel.
func createTestPNGAlpha(w, h int) []byte {
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}
	png.Encode(&buf, img)
	return buf.Bytes()
}
```

Add these imports to `image_replace_test.go`:

```go
import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestReplaceImage" -v ./... 2>&1 | head -10`
Expected: FAIL — `Replace` method not defined.

- [ ] **Step 3: Implement Replace and ReplaceFromStream**

Create `image_replace.go`:

```go
package asposepdf

import (
	"fmt"
	"io"
	"os"
)

// Replace replaces the image data with a new image from a file.
// Format is detected by magic bytes (JPEG, PNG). Dimensions may change.
// Position and size on the page remain unchanged.
func (info *ImageInfo) Replace(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("replace image: %w", err)
	}
	return info.replaceFromBytes(data)
}

// ReplaceFromStream replaces the image data with a new image from a reader.
func (info *ImageInfo) ReplaceFromStream(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("replace image: %w", err)
	}
	return info.replaceFromBytes(data)
}

func (info *ImageInfo) replaceFromBytes(data []byte) error {
	if info.stream == nil {
		return fmt.Errorf("image info: no image data")
	}
	if len(data) == 0 {
		return fmt.Errorf("replace image: empty data")
	}

	format, err := detectImageFormat(data)
	if err != nil {
		return err
	}

	newStream, newSmask, err := createImageXObject(data, format)
	if err != nil {
		return err
	}

	// Update existing stream in place.
	info.stream.Data = newStream.Data
	info.stream.Decoded = newStream.Decoded

	// Replace dict fields from new stream.
	info.stream.Dict["/Width"] = newStream.Dict["/Width"]
	info.stream.Dict["/Height"] = newStream.Dict["/Height"]
	info.stream.Dict["/BitsPerComponent"] = newStream.Dict["/BitsPerComponent"]
	info.stream.Dict["/ColorSpace"] = newStream.Dict["/ColorSpace"]

	// Handle /Filter transition.
	if f, ok := newStream.Dict["/Filter"]; ok {
		info.stream.Dict["/Filter"] = f
	} else {
		delete(info.stream.Dict, "/Filter")
	}
	delete(info.stream.Dict, "/DecodeParms")

	// Handle SMask transition.
	if newSmask != nil {
		// Register new SMask in document objects.
		smaskID := info.objects[info.page.pageObj().Num].Num // get any valid ID reference
		// Actually we need the doc's nextID — access via page.
		smaskID = info.page.doc.nextID
		info.page.doc.nextID++
		info.page.doc.objects[smaskID] = &pdfObject{Num: smaskID, Value: newSmask}
		info.stream.Dict["/SMask"] = pdfRef{Num: smaskID}
	} else {
		delete(info.stream.Dict, "/SMask")
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestReplaceImage" -v ./...`
Expected: PASS.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image_replace.go image_replace_test.go
git commit -m "feat: add ImageInfo.Replace and ReplaceFromStream"
```

---

### Task 3: Implement serializeContentOps

**Files:**
- Create: `image_remove.go`
- Create: `image_remove_test.go`

- [ ] **Step 1: Write failing test for serializeContentOps**

Create `image_remove_test.go`:

```go
package asposepdf

import (
	"strings"
	"testing"
)

func TestSerializeContentOps(t *testing.T) {
	// Build a simple content stream, parse it, serialize it, parse again.
	original := "q\n10 0 0 20 50.5 100 cm\n/Im0 Do\nQ\n"
	ops, err := parseContentStream([]byte(original))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	serialized := serializeContentOps(ops)
	result := string(serialized)

	// Re-parse and verify structural equivalence.
	ops2, err := parseContentStream(serialized)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(ops2) != len(ops) {
		t.Fatalf("op count: got %d, want %d", len(ops2), len(ops))
	}

	for i, op := range ops {
		if ops2[i].Operator != op.Operator {
			t.Errorf("op[%d]: got %q, want %q", i, ops2[i].Operator, op.Operator)
		}
		if len(ops2[i].Operands) != len(op.Operands) {
			t.Errorf("op[%d] operands: got %d, want %d", i, len(ops2[i].Operands), len(op.Operands))
		}
	}

	// Verify key operators are present in output.
	if !strings.Contains(result, "cm") {
		t.Error("serialized should contain cm")
	}
	if !strings.Contains(result, "Do") {
		t.Error("serialized should contain Do")
	}
	if !strings.Contains(result, "/Im0") {
		t.Error("serialized should contain /Im0")
	}
}

func TestSerializeContentOpsWithText(t *testing.T) {
	// Content stream with text operators and TJ array.
	original := "BT\n/F1 12 Tf\n100 700 Td\n[(Hello) -50 (World)] TJ\nET\n"
	ops, err := parseContentStream([]byte(original))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	serialized := serializeContentOps(ops)
	ops2, err := parseContentStream(serialized)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(ops2) != len(ops) {
		t.Fatalf("op count: got %d, want %d", len(ops2), len(ops))
	}

	// Verify TJ operator preserved.
	found := false
	for _, op := range ops2 {
		if op.Operator == "TJ" {
			found = true
			if len(op.Operands) != 1 {
				t.Errorf("TJ operands: got %d, want 1", len(op.Operands))
			}
		}
	}
	if !found {
		t.Error("TJ operator not found in re-parsed output")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestSerializeContentOps" -v ./... 2>&1 | head -10`
Expected: FAIL — `serializeContentOps` not defined.

- [ ] **Step 3: Implement serializeContentOps**

Create `image_remove.go`:

```go
package asposepdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// serializeContentOps converts parsed operators back to content stream bytes.
func serializeContentOps(ops []contentOp) []byte {
	var buf bytes.Buffer
	for _, op := range ops {
		if op.Operator == "BI" {
			// Inline image: write BI, dict key/values, ID, data, EI.
			serializeInlineImage(&buf, op)
			continue
		}
		for i, operand := range op.Operands {
			if i > 0 {
				buf.WriteByte(' ')
			}
			serializeOperand(&buf, operand)
		}
		if len(op.Operands) > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(op.Operator)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func serializeOperand(buf *bytes.Buffer, v pdfValue) {
	switch val := v.(type) {
	case int:
		buf.WriteString(strconv.Itoa(val))
	case float64:
		s := strconv.FormatFloat(val, 'f', 4, 64)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		buf.WriteString(s)
	case pdfName:
		buf.WriteString(string(val))
	case string:
		buf.WriteByte('(')
		buf.WriteString(escapeLiteral(val))
		buf.WriteByte(')')
	case pdfArray:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(' ')
			}
			serializeOperand(buf, item)
		}
		buf.WriteByte(']')
	case pdfDict:
		buf.WriteString("<<")
		for k, dv := range val {
			buf.WriteString(k)
			buf.WriteByte(' ')
			serializeOperand(buf, dv)
			buf.WriteByte(' ')
		}
		buf.WriteString(">>")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case pdfNull:
		buf.WriteString("null")
	default:
		fmt.Fprintf(buf, "%v", val)
	}
}

func serializeInlineImage(buf *bytes.Buffer, op contentOp) {
	if len(op.Operands) < 2 {
		return
	}
	buf.WriteString("BI\n")
	if dict, ok := op.Operands[0].(pdfDict); ok {
		for k, v := range dict {
			buf.WriteString(k)
			buf.WriteByte(' ')
			serializeOperand(buf, v)
			buf.WriteByte('\n')
		}
	}
	buf.WriteString("ID ")
	if data, ok := op.Operands[1].(string); ok {
		buf.WriteString(data)
	}
	buf.WriteString("\nEI\n")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestSerializeContentOps" -v ./...`
Expected: PASS.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image_remove.go image_remove_test.go
git commit -m "feat: add serializeContentOps for content stream rebuilding"
```

---

### Task 4: Implement Remove

**Files:**
- Modify: `image_remove.go`
- Modify: `image_remove_test.go`

- [ ] **Step 1: Write failing tests for Remove**

Add to `image_remove_test.go`:

```go
func TestRemoveImage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 image, got %d", len(infos))
	}

	err := infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify /XObject dict no longer has /Im0.
	resources := page.pageResources()
	xobjVal := resolveRef(page.doc.objects, resources["/XObject"])
	xobjDict, _ := xobjVal.(pdfDict)
	if xobjDict != nil {
		if _, exists := xobjDict["/Im0"]; exists {
			t.Error("/Im0 should be removed from XObject resources")
		}
	}

	// Verify content stream no longer has Do.
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "Do") {
		t.Error("content stream should not contain Do after removal")
	}
}

func TestRemoveImageNestedQ(t *testing.T) {
	// Content stream with nested q/Q: outer text block + inner image block.
	contentData := "q\nBT\n/F1 12 Tf\n100 700 Td\n(Hello) Tj\nET\nQ\nq\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"

	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    []byte{0xFF, 0xD8, 0xFF, 0xD9},
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
			"/Font": pdfDict{
				"/F1": pdfRef{Num: 99},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 image, got %d", len(infos))
	}

	err := infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Content stream should still have the text block but not the image Do.
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "Do") {
		t.Error("content stream should not contain Do after removal")
	}
	if !strings.Contains(content, "Tj") {
		t.Error("content stream should still contain Tj (text operator)")
	}
}

func TestRemoveImageInvalidInfo(t *testing.T) {
	info := &ImageInfo{}
	err := info.Remove()
	if err == nil {
		t.Fatal("expected error for nil page/stream")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestRemoveImage" -v ./... 2>&1 | head -10`
Expected: FAIL — `Remove` method not defined.

- [ ] **Step 3: Implement Remove**

Add to `image_remove.go`:

```go
// Remove removes the image from the page.
// Deletes the XObject reference from page resources and the drawing
// operators from the content stream.
func (info *ImageInfo) Remove() error {
	if info.page == nil || info.stream == nil {
		return fmt.Errorf("image info: no image data")
	}
	if info.Name == "" {
		return fmt.Errorf("remove image: inline images cannot be removed")
	}

	// 1. Remove from page resources.
	resources := info.page.pageResources()
	if resources != nil {
		xobjVal := resolveRef(info.page.doc.objects, resources["/XObject"])
		if xobjDict, ok := xobjVal.(pdfDict); ok {
			delete(xobjDict, info.Name)
		}
	}

	// 2. Remove drawing operators from content stream.
	data, err := info.page.contentStreams()
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}

	filtered := removeImageOps(ops, info.Name)
	newData := serializeContentOps(filtered)

	// 3. Replace content stream.
	newStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    newData,
		Decoded: true,
	}
	newID := info.page.doc.nextID
	info.page.doc.nextID++
	info.page.doc.objects[newID] = &pdfObject{Num: newID, Value: newStream}

	pageDict := info.page.pageDict()
	pageDict["/Contents"] = pdfRef{Num: newID}

	return nil
}

// removeImageOps removes the q...Do...Q block containing a Do for the given image name.
func removeImageOps(ops []contentOp, name string) []contentOp {
	// Find all Do operators for this name and their enclosing q/Q blocks.
	type qEntry struct {
		index int
		depth int
	}

	// First pass: find indices of Do operators for this image.
	var doIndices []int
	for i, op := range ops {
		if op.Operator == "Do" && len(op.Operands) >= 1 {
			if operandName(op.Operands[0]) == name {
				doIndices = append(doIndices, i)
			}
		}
	}
	if len(doIndices) == 0 {
		return ops
	}

	// For each Do, find the enclosing q...Q block.
	type removeRange struct {
		start, end int
	}
	var ranges []removeRange

	for _, doIdx := range doIndices {
		// Walk backward to find the matching q.
		qIdx := -1
		depth := 0
		for i := doIdx - 1; i >= 0; i-- {
			if ops[i].Operator == "Q" {
				depth++
			} else if ops[i].Operator == "q" {
				if depth == 0 {
					qIdx = i
					break
				}
				depth--
			}
		}

		// Walk forward to find the matching Q.
		qEndIdx := -1
		depth = 0
		for i := doIdx + 1; i < len(ops); i++ {
			if ops[i].Operator == "q" {
				depth++
			} else if ops[i].Operator == "Q" {
				if depth == 0 {
					qEndIdx = i
					break
				}
				depth--
			}
		}

		if qIdx >= 0 && qEndIdx >= 0 {
			ranges = append(ranges, removeRange{qIdx, qEndIdx})
		} else {
			// No enclosing q/Q — remove just the cm before Do and the Do itself.
			start := doIdx
			if doIdx > 0 && ops[doIdx-1].Operator == "cm" {
				start = doIdx - 1
			}
			ranges = append(ranges, removeRange{start, doIdx})
		}
	}

	// Build result excluding the remove ranges.
	removed := make(map[int]bool)
	for _, r := range ranges {
		for i := r.start; i <= r.end; i++ {
			removed[i] = true
		}
	}

	var result []contentOp
	for i, op := range ops {
		if !removed[i] {
			result = append(result, op)
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestRemoveImage" -v ./...`
Expected: PASS.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image_remove.go image_remove_test.go
git commit -m "feat: add ImageInfo.Remove with content stream filtering"
```

---

### Task 5: Integration tests and docs update

**Files:**
- Create: `image_replace_integration_test.go`
- Create: `image_remove_integration_test.go`
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Write Replace integration test**

Create `image_replace_integration_test.go`:

```go
package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestReplaceImageRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}

	origWidth := infos[0].Width
	t.Logf("original image: %dx%d %s", infos[0].Width, infos[0].Height, infos[0].Name)

	// Replace first image with a different one.
	err = infos[0].Replace("testdata/Koala.jpg")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	outDir := filepath.Join("result_files", "TestReplaceImageRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, err := p.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos after reopen: %v", err)
	}
	if len(newInfos) == 0 {
		t.Fatal("expected at least 1 image after reopen")
	}

	// Dimensions should differ from original (Koala.jpg has different size).
	if newInfos[0].Width == origWidth {
		t.Logf("warning: replacement image has same width as original (%d)", origWidth)
	}
	t.Logf("replaced image: %dx%d", newInfos[0].Width, newInfos[0].Height)
}

func TestReplaceImageFromStreamRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}

	f, err := os.Open("testdata/aspose-logo.png")
	if err != nil {
		t.Fatalf("open image: %v", err)
	}
	defer f.Close()

	err = infos[0].ReplaceFromStream(f)
	if err != nil {
		t.Fatalf("ReplaceFromStream: %v", err)
	}

	outDir := filepath.Join("result_files", "TestReplaceImageFromStreamRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Log("replaced image from stream, saved to", outPath)
}
```

- [ ] **Step 2: Write Remove integration test**

Create `image_remove_integration_test.go`:

```go
package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestRemoveImageRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}
	origCount := len(infos)
	t.Logf("original image count: %d", origCount)

	// Remove first image.
	err = infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	outDir := filepath.Join("result_files", "TestRemoveImageRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify image count decreased.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, err := p.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos after reopen: %v", err)
	}

	if len(newInfos) >= origCount {
		t.Errorf("expected fewer images after removal: got %d, original %d", len(newInfos), origCount)
	}
	t.Logf("image count after removal: %d", len(newInfos))
}

func TestRemoveAllImagesRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	t.Logf("removing %d images", len(infos))

	for _, info := range infos {
		if err := info.Remove(); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}

	outDir := filepath.Join("result_files", "TestRemoveAllImagesRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify no images remain.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, _ := p.ImageInfos()
	if len(newInfos) > 0 {
		t.Errorf("expected 0 images after removing all, got %d", len(newInfos))
	}
}
```

- [ ] **Step 3: Run integration tests**

Run: `go test -run "TestReplaceImage|TestRemoveImage" -v ./...`
Expected: PASS.

- [ ] **Step 4: Update CLAUDE.md**

After the `ImageToDocumentOptions` line in the Public API section, add:

```
- `(*ImageInfo).Replace(path) error` — replaces image data from a file; format detected by magic bytes (JPEG, PNG); position unchanged
- `(*ImageInfo).ReplaceFromStream(r) error` — replaces image data from an io.Reader
- `(*ImageInfo).Remove() error` — removes image from page (resources + content stream); XObject stays in doc objects
```

- [ ] **Step 5: Update README.md**

After the "Image to PDF" section (before "### Document API"), add:

```markdown
### Replacing and Removing Images

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)
infos, _ := page.ImageInfos()

// Replace first image with a new one
infos[0].Replace("new_logo.jpg")

// Replace from stream
f, _ := os.Open("photo.png")
infos[1].ReplaceFromStream(f)
f.Close()

// Remove an image
infos[2].Remove()

doc.Save("output.pdf")
```
```

Also add to the Features list, after "Image to PDF":
```
- **Replace images** — swap image data on existing pages while preserving position and size
- **Remove images** — delete images from pages, cleaning up resources and content stream operators
```

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add image_replace_integration_test.go image_remove_integration_test.go CLAUDE.md README.md
git commit -m "docs: add Replace/Remove integration tests, update CLAUDE.md and README"
```
