# OptimizeImages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Document.OptimizeImages(opts)` that reduces PDF file size by downscaling images above a target DPI and converting opaque PNGs to JPEG.

**Architecture:** Iterate all pages, collect `ImageInfos()`, process each XObject. Two strategies: bilinear downscale to target DPI, and opaque PNG→JPEG conversion. Track processed object IDs to avoid double-processing shared XObjects. Pure Go — uses `image/jpeg` and standard `image` types, no external dependencies.

**Tech Stack:** Go standard library (`image`, `image/jpeg`, `image/color`, `math`)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `image_optimize.go` | `OptimizeImageOptions`, `OptimizeImages`, `optimizeImage`, `rawPixelsToImage`, `downscaleImage` |
| `image_optimize_test.go` | Unit tests (package `asposepdf`) — 6 tests |
| `image_optimize_integration_test.go` | Integration test (package `asposepdf_test`) — 1 test |

---

### Task 1: `rawPixelsToImage` — reconstruct `image.Image` from raw PDF pixel bytes

**Files:**
- Create: `image_optimize.go`
- Create: `image_optimize_test.go`

This is a standalone helper. PNG images in PDF are stored as raw decoded pixels (not PNG file format). This function rebuilds an `image.Image` from those raw bytes.

- [ ] **Step 1: Write the failing test for `rawPixelsToImage`**

In `image_optimize_test.go`:

```go
package asposepdf

import (
	"image"
	"image/color"
	"testing"
)

func TestRawPixelsToImageRGB(t *testing.T) {
	// 2x2 red/green/blue/white image.
	pixels := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceRGB")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", bounds)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("pixel (0,0) = (%d,%d,%d,%d), want red", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestRawPixelsToImageGray(t *testing.T) {
	pixels := []byte{0, 128, 255, 64}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceGray")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	g := img.(*image.Gray)
	if g.GrayAt(0, 0).Y != 0 {
		t.Errorf("pixel (0,0) = %d, want 0", g.GrayAt(0, 0).Y)
	}
	if g.GrayAt(1, 0).Y != 128 {
		t.Errorf("pixel (1,0) = %d, want 128", g.GrayAt(1, 0).Y)
	}
}

func TestRawPixelsToImageUnsupported(t *testing.T) {
	img := rawPixelsToImage([]byte{0, 0, 0, 0}, 1, 1, "/DeviceCMYK")
	if img != nil {
		t.Error("expected nil for unsupported color space")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestRawPixelsToImage -v ./...`
Expected: compilation error — `rawPixelsToImage` undefined.

- [ ] **Step 3: Implement `rawPixelsToImage`**

Create `image_optimize.go`:

```go
package asposepdf

import (
	"image"
	"image/color"
)

// rawPixelsToImage reconstructs an image.Image from raw PDF pixel bytes.
// PDF stores decoded image data as raw pixels, not as PNG file format.
// Returns nil for unsupported color spaces.
func rawPixelsToImage(data []byte, width, height int, colorSpace string) image.Image {
	switch colorSpace {
	case "/DeviceRGB":
		img := image.NewNRGBA(image.Rect(0, 0, width, height))
		stride := width * 3
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*stride + x*3
				if off+2 >= len(data) {
					break
				}
				img.SetNRGBA(x, y, color.NRGBA{R: data[off], G: data[off+1], B: data[off+2], A: 255})
			}
		}
		return img
	case "/DeviceGray":
		img := image.NewGray(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*width + x
				if off >= len(data) {
					break
				}
				img.SetGray(x, y, color.Gray{Y: data[off]})
			}
		}
		return img
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestRawPixelsToImage -v ./...`
Expected: all 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add image_optimize.go image_optimize_test.go
git commit -m "feat: add rawPixelsToImage for PDF pixel reconstruction"
```

---

### Task 2: `downscaleImage` — bilinear interpolation resize

**Files:**
- Modify: `image_optimize.go`
- Modify: `image_optimize_test.go`

Pure Go bilinear downscale. Input is any `image.Image`, output is `*image.NRGBA`.

- [ ] **Step 1: Write the failing test for `downscaleImage`**

Add to `image_optimize_test.go`:

```go
func TestDownscaleImage(t *testing.T) {
	// 4x4 image: top-left quadrant red, top-right green, bottom-left blue, bottom-right white.
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			var c color.NRGBA
			switch {
			case x < 2 && y < 2:
				c = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
			case x >= 2 && y < 2:
				c = color.NRGBA{R: 0, G: 255, B: 0, A: 255}
			case x < 2 && y >= 2:
				c = color.NRGBA{R: 0, G: 0, B: 255, A: 255}
			default:
				c = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}

	dst := downscaleImage(src, 2, 2)
	if dst.Bounds().Dx() != 2 || dst.Bounds().Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", dst.Bounds())
	}

	// Each output pixel should be the average of a 2x2 block from the source.
	// (0,0) = average of red block = (255, 0, 0)
	r, g, b, _ := dst.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel (0,0) = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}
	// (1,0) = average of green block = (0, 255, 0)
	r, g, b, _ = dst.At(1, 0).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Errorf("pixel (1,0) = (%d,%d,%d), want green", r>>8, g>>8, b>>8)
	}
}

func TestDownscaleImagePreservesNonSquare(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	dst := downscaleImage(src, 50, 25)
	if dst.Bounds().Dx() != 50 || dst.Bounds().Dy() != 25 {
		t.Fatalf("bounds = %v, want 50x25", dst.Bounds())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestDownscaleImage -v ./...`
Expected: compilation error — `downscaleImage` undefined.

- [ ] **Step 3: Implement `downscaleImage`**

Add to `image_optimize.go`:

```go
// downscaleImage resizes img to newWidth x newHeight using bilinear interpolation.
func downscaleImage(img image.Image, newWidth, newHeight int) *image.NRGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map destination pixel to source coordinates.
			srcX := (float64(x) + 0.5) * float64(srcW) / float64(newWidth) - 0.5
			srcY := (float64(y) + 0.5) * float64(srcH) / float64(newHeight) - 0.5

			x0 := int(srcX)
			y0 := int(srcY)
			if x0 < 0 {
				x0 = 0
			}
			if y0 < 0 {
				y0 = 0
			}
			x1 := x0 + 1
			y1 := y0 + 1
			if x1 >= srcW {
				x1 = srcW - 1
			}
			if y1 >= srcH {
				y1 = srcH - 1
			}

			dx := srcX - float64(x0)
			dy := srcY - float64(y0)
			if dx < 0 {
				dx = 0
			}
			if dy < 0 {
				dy = 0
			}

			r00, g00, b00, a00 := img.At(bounds.Min.X+x0, bounds.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := img.At(bounds.Min.X+x1, bounds.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := img.At(bounds.Min.X+x0, bounds.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := img.At(bounds.Min.X+x1, bounds.Min.Y+y1).RGBA()

			r := bilinear(r00, r10, r01, r11, dx, dy)
			g := bilinear(g00, g10, g01, g11, dx, dy)
			b := bilinear(b00, b10, b01, b11, dx, dy)
			a := bilinear(a00, a10, a01, a11, dx, dy)

			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}
	return dst
}

func bilinear(v00, v10, v01, v11 uint32, dx, dy float64) uint32 {
	top := float64(v00)*(1-dx) + float64(v10)*dx
	bot := float64(v01)*(1-dx) + float64(v11)*dx
	return uint32(top*(1-dy) + bot*dy)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestDownscaleImage -v ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add image_optimize.go image_optimize_test.go
git commit -m "feat: add downscaleImage with bilinear interpolation"
```

---

### Task 3: `OptimizeImageOptions` and `OptimizeImages` — main optimization method

**Files:**
- Modify: `image_optimize.go`
- Modify: `image_optimize_test.go`

This is the core method. It iterates pages and calls the per-image optimizer.

- [ ] **Step 1: Write the failing test — JPEG below MaxDPI (no-op)**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesNoOp(t *testing.T) {
	// JPEG 10x10 displayed at 10x10 pt → DPI = 10/(10/72) = 72.
	// MaxDPI=150 → no downscale needed, JPEG stays as-is.
	doc := createDocWithImage() // 10x10 JPEG, CTM has 10pt display
	origData := make([]byte, len(doc.objects[1].Value.(*pdfStream).Data))
	copy(origData, doc.objects[1].Value.(*pdfStream).Data)

	count, err := doc.OptimizeImages(OptimizeImageOptions{MaxDPI: 150})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (JPEG already below MaxDPI)", count)
	}

	// Verify data untouched.
	stream := doc.objects[1].Value.(*pdfStream)
	if len(stream.Data) != len(origData) {
		t.Errorf("data length changed: %d → %d", len(origData), len(stream.Data))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestOptimizeImagesNoOp -v ./...`
Expected: compilation error — `OptimizeImages` undefined.

- [ ] **Step 3: Write the failing test — downscale**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesDownscale(t *testing.T) {
	// Build a doc with a 100x100 JPEG displayed at 50x50 pt.
	// DPI = 100 / (50/72) = 144. MaxDPI=72 → should downscale to ~50x50.
	jpegData := buildMinimalJPEG(100, 100, 3)
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            100,
			"/Height":           100,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	// CTM: 50 0 0 50 0 0 → 50pt x 50pt display.
	contentData := "q\n50 0 0 50 10 10 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 612.0, 792.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
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

	count, err := doc.OptimizeImages(OptimizeImageOptions{MaxDPI: 72})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Width and Height should be reduced.
	w := dictGetInt(imgStream.Dict, "/Width")
	h := dictGetInt(imgStream.Dict, "/Height")
	if w >= 100 || h >= 100 {
		t.Errorf("expected downscaled dimensions, got %dx%d", w, h)
	}
	if w != 50 || h != 50 {
		t.Errorf("dimensions = %dx%d, want 50x50", w, h)
	}
}
```

- [ ] **Step 4: Write the failing test — PNG→JPEG conversion**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesPNGToJPEG(t *testing.T) {
	// Build doc with opaque PNG (no SMask, Decoded=true, no /Filter).
	pixels := make([]byte, 10*10*3) // 10x10 RGB
	for i := range pixels {
		pixels[i] = 128
	}
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
		},
		Data:    pixels,
		Decoded: true,
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

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify filter changed to DCTDecode.
	filter := dictGetName(imgStream.Dict, "/Filter")
	if filter != "/DCTDecode" {
		t.Errorf("filter = %s, want /DCTDecode", filter)
	}
	if imgStream.Decoded {
		t.Error("stream should be Decoded=false after JPEG encoding")
	}
}
```

- [ ] **Step 5: Write the failing test — PNG with alpha NOT converted**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesPNGWithAlphaNotConverted(t *testing.T) {
	// PNG with SMask (alpha) — must NOT be converted to JPEG.
	pixels := make([]byte, 4*4*3)
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            4,
			"/Height":           4,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/SMask":            pdfRef{Num: 10}, // has alpha
		},
		Data:    pixels,
		Decoded: true,
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

	smaskStream := &pdfStream{
		Dict:    pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Image")},
		Data:    make([]byte, 16),
		Decoded: true,
	}
	smaskObj := &pdfObject{Num: 10, Value: smaskStream}

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj, 10: smaskObj},
		pages:   []*pdfObject{pageObj},
		nextID:  11,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (PNG with alpha should not be converted)", count)
	}

	// Filter should still be empty (PNG).
	filter := dictGetName(imgStream.Dict, "/Filter")
	if filter != "" {
		t.Errorf("filter = %s, want empty (PNG preserved)", filter)
	}
}
```

- [ ] **Step 6: Write the failing test — shared XObject**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesSharedXObject(t *testing.T) {
	// One image XObject shared by two pages — should be optimized once.
	pixels := make([]byte, 10*10*3)
	for i := range pixels {
		pixels[i] = 200
	}
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
		},
		Data:    pixels,
		Decoded: true,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	makePageObj := func(num int) *pdfObject {
		contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
		cs := &pdfStream{Dict: pdfDict{}, Data: []byte(contentData), Decoded: true}
		csObj := &pdfObject{Num: num, Value: cs}
		pd := pdfDict{
			"/Type":     pdfName("/Page"),
			"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
			"/Resources": pdfDict{"/XObject": pdfDict{"/Im0": pdfRef{Num: 1}}},
			"/Contents": pdfRef{Num: num},
		}
		pageObj := &pdfObject{Num: num + 1, Value: pd}
		return pageObj
	}

	page1Obj := makePageObj(2)
	page2Obj := makePageObj(4)

	// Build content objects separately.
	cs1 := &pdfStream{Dict: pdfDict{}, Data: []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"), Decoded: true}
	cs2 := &pdfStream{Dict: pdfDict{}, Data: []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"), Decoded: true}
	csObj1 := &pdfObject{Num: 2, Value: cs1}
	csObj2 := &pdfObject{Num: 4, Value: cs2}

	doc := &Document{
		objects: map[int]*pdfObject{
			1: imgObj,
			2: csObj1, 3: page1Obj,
			4: csObj2, 5: page2Obj,
		},
		pages:  []*pdfObject{page1Obj, page2Obj},
		nextID: 6,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (shared XObject optimized once)", count)
	}
}
```

- [ ] **Step 7: Run all tests to verify they fail**

Run: `go test -run "TestOptimizeImages|TestDownscale" -v ./...`
Expected: compilation errors — `OptimizeImages`, `OptimizeImageOptions` undefined.

- [ ] **Step 8: Implement `OptimizeImageOptions`, `OptimizeImages`, and `optimizeImage`**

Add to `image_optimize.go` (after the existing functions):

```go
import (
	"bytes"
	"fmt"
	"image/jpeg"
	"math"
)

// OptimizeImageOptions controls image optimization behavior.
type OptimizeImageOptions struct {
	MaxDPI           float64 // images above this DPI are downscaled; 0 = no downscaling
	JPEGQuality      int     // JPEG quality 1-100; 0 = default (75)
	ConvertPNGToJPEG bool    // convert opaque PNG (no alpha) to JPEG
}

// OptimizeImages optimizes all images in the document to reduce file size.
// Returns the number of images optimized.
func (d *Document) OptimizeImages(opts OptimizeImageOptions) (int, error) {
	if opts.JPEGQuality != 0 && (opts.JPEGQuality < 1 || opts.JPEGQuality > 100) {
		return 0, fmt.Errorf("optimize images: JPEGQuality must be 1-100, got %d", opts.JPEGQuality)
	}
	quality := opts.JPEGQuality
	if quality == 0 {
		quality = 75
	}

	processed := make(map[int]bool)
	count := 0

	for _, p := range d.Pages() {
		infos, err := p.ImageInfos()
		if err != nil {
			continue
		}
		for i := range infos {
			info := &infos[i]
			if info.Inline || info.stream == nil {
				continue
			}

			// Get object ID from formVal to track shared XObjects.
			ref, ok := info.formVal.(pdfRef)
			if !ok {
				continue
			}
			if processed[ref.Num] {
				continue
			}
			processed[ref.Num] = true

			if optimizeImage(info, opts, quality) {
				count++
			}
		}
	}
	return count, nil
}

func optimizeImage(info *ImageInfo, opts OptimizeImageOptions, quality int) bool {
	stream := info.stream

	// Compute effective DPI.
	displayWidth := info.PageWidth  // in points
	displayHeight := info.PageHeight
	if displayWidth <= 0 || displayHeight <= 0 {
		return false // no CTM info, skip
	}

	imageDPIX := float64(info.Width) / (displayWidth / 72.0)
	imageDPIY := float64(info.Height) / (displayHeight / 72.0)
	imageDPI := math.Max(imageDPIX, imageDPIY)

	filter := primaryFilter(stream.Dict)
	isJPEG := filter == "/DCTDecode"
	isPNG := !isJPEG // Decoded=true with no DCTDecode filter
	_, hasSMask := stream.Dict["/SMask"]

	needsDownscale := opts.MaxDPI > 0 && imageDPI > opts.MaxDPI
	needsPNGToJPEG := opts.ConvertPNGToJPEG && isPNG && !hasSMask

	if !needsDownscale && !needsPNGToJPEG {
		return false
	}
	// JPEG that doesn't need downscaling: skip (avoid lossy re-encode).
	if isJPEG && !needsDownscale {
		return false
	}

	// Decode pixels.
	var img image.Image
	if isJPEG {
		decoded, err := jpeg.Decode(bytes.NewReader(stream.Data))
		if err != nil {
			return false // skip corrupt
		}
		img = decoded
	} else {
		// PNG-stored: raw pixel bytes.
		csName := "/DeviceRGB"
		switch info.ColorSpace {
		case ColorSpaceDeviceGray:
			csName = "/DeviceGray"
		case ColorSpaceDeviceRGB:
			csName = "/DeviceRGB"
		default:
			return false // unsupported color space
		}
		img = rawPixelsToImage(stream.Data, info.Width, info.Height, csName)
		if img == nil {
			return false
		}
	}

	// Downscale if needed.
	newWidth := info.Width
	newHeight := info.Height
	if needsDownscale {
		newWidth = int(math.Round(displayWidth / 72.0 * opts.MaxDPI))
		newHeight = int(math.Round(displayHeight / 72.0 * opts.MaxDPI))
		if newWidth < 1 {
			newWidth = 1
		}
		if newHeight < 1 {
			newHeight = 1
		}
		img = downscaleImage(img, newWidth, newHeight)
	}

	// Encode back.
	if isJPEG || needsPNGToJPEG {
		// Encode to JPEG.
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return false
		}
		stream.Data = buf.Bytes()
		stream.Dict["/Filter"] = pdfName("/DCTDecode")
		stream.Decoded = false
		delete(stream.Dict, "/SMask")
	} else {
		// Keep as PNG: extract raw pixels back.
		bounds := img.Bounds()
		w := bounds.Dx()
		h := bounds.Dy()
		csName := "/DeviceRGB"
		if info.ColorSpace == ColorSpaceDeviceGray {
			csName = "/DeviceGray"
		}
		stream.Data = imageToRawPixels(img, w, h, csName)
		stream.Decoded = true
		delete(stream.Dict, "/Filter")
	}

	// Update dict.
	stream.Dict["/Width"] = newWidth
	stream.Dict["/Height"] = newHeight
	stream.Dict["/BitsPerComponent"] = 8
	delete(stream.Dict, "/DecodeParms")

	return true
}

// imageToRawPixels extracts raw pixel bytes from an image.Image.
func imageToRawPixels(img image.Image, width, height int, colorSpace string) []byte {
	bounds := img.Bounds()
	if colorSpace == "/DeviceGray" {
		data := make([]byte, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				// Average to gray.
				gray := (r + g + b) / 3
				data[y*width+x] = byte(gray >> 8)
			}
		}
		return data
	}
	// RGB
	data := make([]byte, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			off := (y*width + x) * 3
			data[off] = byte(r >> 8)
			data[off+1] = byte(g >> 8)
			data[off+2] = byte(b >> 8)
		}
	}
	return data
}
```

**Note:** The full import list at the top of `image_optimize.go` should be:

```go
import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)
```

- [ ] **Step 9: Run all unit tests to verify they pass**

Run: `go test -run "TestOptimizeImages|TestDownscale|TestRawPixels" -v ./...`
Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add image_optimize.go image_optimize_test.go
git commit -m "feat: add OptimizeImages with downscale and PNG-to-JPEG conversion"
```

---

### Task 4: `OptimizeImages` validation test

**Files:**
- Modify: `image_optimize_test.go`

Test that invalid `JPEGQuality` returns an error.

- [ ] **Step 1: Write the failing test**

Add to `image_optimize_test.go`:

```go
func TestOptimizeImagesInvalidQuality(t *testing.T) {
	doc := createDocWithImage()
	_, err := doc.OptimizeImages(OptimizeImageOptions{JPEGQuality: 101})
	if err == nil {
		t.Fatal("expected error for JPEGQuality=101")
	}
	_, err = doc.OptimizeImages(OptimizeImageOptions{JPEGQuality: -1})
	if err == nil {
		t.Fatal("expected error for JPEGQuality=-1")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test -run TestOptimizeImagesInvalidQuality -v ./...`
Expected: PASS (validation already implemented in Task 3).

- [ ] **Step 3: Commit**

```bash
git add image_optimize_test.go
git commit -m "test: add OptimizeImages JPEGQuality validation test"
```

---

### Task 5: Integration test

**Files:**
- Create: `image_optimize_integration_test.go`

Round-trip test with a real PDF.

- [ ] **Step 1: Write the integration test**

Create `image_optimize_integration_test.go`:

```go
package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestOptimizeImagesRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	count, err := doc.OptimizeImages(asposepdf.OptimizeImageOptions{
		MaxDPI:           150,
		JPEGQuality:      75,
		ConvertPNGToJPEG: true,
	})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	t.Logf("optimized %d images", count)

	outDir := filepath.Join("result_files", "TestOptimizeImagesRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate the output.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Verify file size decreased.
	origInfo, _ := os.Stat("testdata/PdfWithImages.pdf")
	outInfo, _ := os.Stat(outPath)
	t.Logf("original: %d bytes, optimized: %d bytes", origInfo.Size(), outInfo.Size())
	if outInfo.Size() >= origInfo.Size() {
		t.Error("expected smaller file size after optimization")
	}
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test -run TestOptimizeImagesRoundTrip -v ./...`
Expected: PASS. File size should decrease.

- [ ] **Step 3: Commit**

```bash
git add image_optimize_integration_test.go
git commit -m "test: add OptimizeImages integration test with real PDF"
```

---

### Task 6: Update CLAUDE.md and README.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md**

Add to the `page.go` section of the Public API in `CLAUDE.md`, after the `ImageToDocumentOptions` struct entry:

```markdown
- `OptimizeImageOptions` struct — MaxDPI, JPEGQuality, ConvertPNGToJPEG
- `(*Document).OptimizeImages(opts) (int, error)` — optimizes images to reduce file size; downscales above MaxDPI, converts opaque PNG to JPEG
```

- [ ] **Step 2: Update README.md**

Add `**Optimize images**` to the Features list:

```markdown
- **Optimize images** — reduce file size by downscaling images above a target DPI and converting opaque PNGs to JPEG
```

Add a new section after "Cleaning Up Unused Objects":

```markdown
### Optimizing Images

```go
doc, _ := pdf.Open("large.pdf")
optimized, err := doc.OptimizeImages(pdf.OptimizeImageOptions{
    MaxDPI:           150,
    JPEGQuality:      75,
    ConvertPNGToJPEG: true,
})
fmt.Printf("optimized %d images\n", optimized)
doc.Save("smaller.pdf")
```
```

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: add OptimizeImages to CLAUDE.md and README.md"
```
