# Phase 3: Image Extraction — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract images from PDF pages as PNG or JPEG with metadata, position, and color space info.

**Architecture:** Content stream walker tracks CTM via `cm`/`q`/`Q`, collects image XObjects on `Do` and inline images on `BI`. Each image is decoded through a filter→color-space→PNG/JPEG pipeline. Soft masks are applied as alpha channels.

**Tech Stack:** Pure Go — `image`, `image/png`, `image/jpeg`, `image/color`, `compress/zlib`, `math` from stdlib. No external dependencies.

---

## File Structure

| File | Responsibility |
|---|---|
| `image.go` | Public types (`Image`, `ImageFormat`, `ImageColorSpace`), `Save`, `WriteTo`, `Page.ExtractImages`, `Document.ExtractImages`, content stream walker |
| `image_decode.go` | Color space resolution, pixel decoding, CMYK→RGB, Indexed expansion, soft mask merge, PNG encoding |
| `image_inline.go` | Inline image parsing (BI/ID/EI) — dict abbreviation expansion, data extraction |
| `image_test.go` | Integration tests (external package `asposepdf_test`) |
| `image_decode_test.go` | Unit tests for decode functions (internal package `asposepdf`) |

---

### Task 1: Image types and JPEG passthrough

Core types, content stream walker, and the simplest extraction path: DCTDecode → JPEG passthrough.

**Files:**
- Create: `image.go`
- Create: `image_test.go`
- Modify: `testdata/testfiles.json` (add `TestExtractImages` entry)

- [ ] **Step 1: Write failing integration test**

In `image_test.go` (external package `asposepdf_test`):

```go
package asposepdf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestExtractImages(t *testing.T) {
	groups := testGroups(t)
	for _, group := range groups {
		path := group[0]
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			doc, err := asposepdf.Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			allImages, err := doc.ExtractImages()
			if err != nil {
				t.Fatalf("ExtractImages: %v", err)
			}

			outDir := filepath.Join(resultDir, "TestExtractImages", name)
			os.MkdirAll(outDir, 0o755)

			total := 0
			for pageIdx, images := range allImages {
				for imgIdx, img := range images {
					ext := ".png"
					if img.Format == asposepdf.ImageFormatJPEG {
						ext = ".jpg"
					}
					outPath := filepath.Join(outDir, fmt.Sprintf("page%d_img%d%s", pageIdx+1, imgIdx+1, ext))
					if err := img.Save(outPath); err != nil {
						t.Errorf("save %s: %v", outPath, err)
					}
					if img.Width <= 0 || img.Height <= 0 {
						t.Errorf("page %d img %d: invalid dimensions %dx%d", pageIdx+1, imgIdx+1, img.Width, img.Height)
					}
					if len(img.Data) == 0 {
						t.Errorf("page %d img %d: empty data", pageIdx+1, imgIdx+1)
					}
					total++
				}
			}
			t.Logf("%s: extracted %d images to %s", name, total, outDir)
		})
	}
}
```

Add `"fmt"` to imports. Add entry to `testdata/testfiles.json`:

```json
"TestExtractImages": [
  ["marketing.pdf"],
  ["PdfWithTable.pdf"],
  ["alfa.pdf"]
]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExtractImages -v ./... 2>&1 | head -20`
Expected: compilation error — `ExtractImages` not defined.

- [ ] **Step 3: Implement types and JPEG passthrough**

Create `image.go`:

```go
package asposepdf

import (
	"io"
	"math"
	"os"
)

// ImageFormat describes the output format of an extracted image.
type ImageFormat int

const (
	ImageFormatPNG  ImageFormat = iota
	ImageFormatJPEG
)

// ImageColorSpace describes the original color space of the image in the PDF.
type ImageColorSpace int

const (
	ColorSpaceDeviceRGB  ImageColorSpace = iota
	ColorSpaceDeviceGray
	ColorSpaceDeviceCMYK
	ColorSpaceIndexed
	ColorSpaceICCBased
)

// Image holds an extracted image with its encoded data and metadata.
type Image struct {
	Data       []byte          // encoded image bytes (PNG or JPEG)
	Format     ImageFormat     // output format
	Width      int             // pixel width
	Height     int             // pixel height
	BPC        int             // bits per component (original)
	ColorSpace ImageColorSpace // original PDF color space
	X, Y       float64         // position on page (lower-left, in points)
	PageWidth  float64         // display width on page (in points)
	PageHeight float64         // display height on page (in points)
	Inline     bool            // true if from inline image (BI/ID/EI)
}

// Save writes the image data to a file.
func (img *Image) Save(path string) error {
	return os.WriteFile(path, img.Data, 0o644)
}

// WriteTo writes the image data to w.
func (img *Image) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(img.Data)
	return int64(n), err
}

// ExtractImages returns images from all pages (one slice per page).
func (d *Document) ExtractImages() ([][]Image, error) {
	pages := d.Pages()
	result := make([][]Image, len(pages))
	for i, p := range pages {
		images, err := p.ExtractImages()
		if err != nil {
			return nil, err
		}
		result[i] = images
	}
	return result, nil
}

// ExtractImages returns all images found on the page.
func (p *Page) ExtractImages() ([]Image, error) {
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
	return extractImagesFromOps(p.doc.objects, ops, resources)
}

// extractImagesFromOps walks content stream ops, tracking CTM, and extracts images.
func extractImagesFromOps(objects map[int]*pdfObject, ops []contentOp, resources pdfDict) ([]Image, error) {
	var images []Image
	ctm := identityMatrix()
	var ctmStack [][6]float64

	for _, op := range ops {
		switch op.Operator {
		case "cm":
			if len(op.Operands) >= 6 {
				var m [6]float64
				for i := 0; i < 6; i++ {
					m[i] = operandFloat(op.Operands[i])
				}
				ctm = matMul(m, ctm)
			}
		case "q":
			ctmStack = append(ctmStack, ctm)
		case "Q":
			if len(ctmStack) > 0 {
				ctm = ctmStack[len(ctmStack)-1]
				ctmStack = ctmStack[:len(ctmStack)-1]
			}
		case "Do":
			if len(op.Operands) >= 1 {
				name := operandName(op.Operands[0])
				if img, ok := extractXObjectImage(objects, resources, name, ctm); ok {
					images = append(images, img)
				}
			}
		}
	}
	return images, nil
}

// extractXObjectImage extracts an image from an XObject reference.
// Returns false if the XObject is not an image or can't be decoded.
func extractXObjectImage(objects map[int]*pdfObject, resources pdfDict, name string, ctm [6]float64) (Image, bool) {
	if name == "" || resources == nil {
		return Image{}, false
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return Image{}, false
	}
	xobjDict, ok := resolveRefToDict(objects, xobjVal)
	if !ok {
		return Image{}, false
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return Image{}, false
	}
	resolved := resolveRef(objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return Image{}, false
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Image" {
		return Image{}, false
	}

	width := dictGetInt(stream.Dict, "/Width")
	height := dictGetInt(stream.Dict, "/Height")
	bpc := dictGetInt(stream.Dict, "/BitsPerComponent")
	if width <= 0 || height <= 0 {
		return Image{}, false
	}

	cs := resolveColorSpace(objects, stream.Dict)
	filter := primaryFilter(stream.Dict)

	img := Image{
		Width:      width,
		Height:     height,
		BPC:        bpc,
		ColorSpace: cs,
		X:          ctm[4],
		Y:          ctm[5],
		PageWidth:  math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1]),
		PageHeight: math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3]),
	}

	if filter == "/DCTDecode" {
		// JPEG passthrough — use raw stream bytes (before decoding).
		img.Data = stream.Data
		if stream.Decoded {
			// Stream was already decoded; we need the raw bytes.
			// For DCT, the raw data IS the JPEG. Re-fetch from object.
			img.Data = getRawStreamData(objects, formVal)
			if img.Data == nil {
				return Image{}, false
			}
		}
		img.Format = ImageFormatJPEG
		return img, true
	}

	// For now, skip non-DCT images (PNG encoding added in Task 2).
	return Image{}, false
}

// primaryFilter returns the first filter name, or "" if none.
func primaryFilter(d pdfDict) string {
	filterVal, ok := d["/Filter"]
	if !ok {
		return ""
	}
	if n, ok := filterVal.(pdfName); ok {
		return string(n)
	}
	if arr, ok := filterVal.(pdfArray); ok && len(arr) > 0 {
		if n, ok := arr[0].(pdfName); ok {
			return string(n)
		}
	}
	return ""
}

// resolveColorSpace determines the ImageColorSpace from a stream dict.
func resolveColorSpace(objects map[int]*pdfObject, d pdfDict) ImageColorSpace {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return ColorSpaceDeviceRGB
	}
	csVal = resolveRef(objects, csVal)

	switch v := csVal.(type) {
	case pdfName:
		return colorSpaceFromName(string(v))
	case pdfArray:
		if len(v) > 0 {
			if n, ok := v[0].(pdfName); ok {
				name := string(n)
				if name == "/ICCBased" {
					return ColorSpaceICCBased
				}
				if name == "/Indexed" {
					return ColorSpaceIndexed
				}
				return colorSpaceFromName(name)
			}
		}
	}
	return ColorSpaceDeviceRGB
}

func colorSpaceFromName(name string) ImageColorSpace {
	switch name {
	case "/DeviceRGB":
		return ColorSpaceDeviceRGB
	case "/DeviceGray":
		return ColorSpaceDeviceGray
	case "/DeviceCMYK":
		return ColorSpaceDeviceCMYK
	default:
		return ColorSpaceDeviceRGB
	}
}

// getRawStreamData re-reads raw (un-decoded) stream bytes for an object.
// Used for JPEG passthrough when the stream was already decoded by the parser.
func getRawStreamData(objects map[int]*pdfObject, val pdfValue) []byte {
	ref, ok := val.(pdfRef)
	if !ok {
		return nil
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return nil
	}
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		return nil
	}
	// If not decoded, Data is raw.
	if !stream.Decoded {
		return stream.Data
	}
	// If decoded, we need the original raw bytes.
	// The parser decoded them — we can't get them back easily.
	// But for DCTDecode, the decoded data IS the decompressed image pixels,
	// not a valid JPEG. We need to re-read from file.
	// Fallback: return nil (image will be skipped).
	return nil
}
```

Note: The `getRawStreamData` fallback will be refined. In practice, PDF parsers typically store raw stream data for DCTDecode streams since the "decoded" data is the JPEG bytes themselves (DCTDecode decompression produces pixels, but our parser likely doesn't attempt to decompress JPEG). Check how `decodeStream` handles `/DCTDecode` — it currently returns an error for unsupported filters, so the stream's `Data` will be the raw (un-decoded) JPEG bytes with `Decoded == false`. This means the passthrough path `stream.Data` will work directly.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestExtractImages -v ./... 2>&1`
Expected: PASS. Marketing PDF should extract JPEG images. Log shows count.

- [ ] **Step 5: Commit**

```bash
git add image.go image_test.go testdata/testfiles.json
git commit -m "feat: add image extraction with JPEG passthrough"
```

---

### Task 2: PNG encoding for FlateDecode/raw RGB and Gray images

Decode FlateDecode and unfiltered image streams to pixels, encode as PNG.

**Files:**
- Create: `image_decode.go`
- Create: `image_decode_test.go`
- Modify: `image.go` (call decode+PNG path instead of skipping non-DCT)

- [ ] **Step 1: Write unit tests for pixel-to-PNG encoding**

Create `image_decode_test.go`:

```go
package asposepdf

import (
	"bytes"
	"image/png"
	"testing"
)

func TestEncodePNGRGB(t *testing.T) {
	// 2x2 RGB image: red, green, blue, white
	pixels := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	data, err := encodePNG(pixels, 2, 2, 8, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify it's a valid PNG.
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Errorf("size=%dx%d, want 2x2", bounds.Dx(), bounds.Dy())
	}
	// Check top-left pixel is red.
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel(0,0)=(%d,%d,%d), want (255,0,0)", r>>8, g>>8, b>>8)
	}
}

func TestEncodePNGGray(t *testing.T) {
	// 2x2 grayscale: black, dark gray, light gray, white
	pixels := []byte{0, 85, 170, 255}
	data, err := encodePNG(pixels, 2, 2, 8, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Errorf("size=%dx%d, want 2x2", bounds.Dx(), bounds.Dy())
	}
}

func TestEncodePNGWithAlpha(t *testing.T) {
	// 2x1 RGB with soft mask (alpha)
	pixels := []byte{255, 0, 0, 0, 255, 0}
	alpha := []byte{255, 128}
	data, err := encodePNG(pixels, 2, 1, 8, 3, alpha)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	// Second pixel should have alpha=128.
	_, _, _, a := img.At(1, 0).RGBA()
	if a>>8 != 128 {
		t.Errorf("alpha(1,0)=%d, want 128", a>>8)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestEncodePNG" -v ./... 2>&1 | head -10`
Expected: FAIL — `encodePNG` undefined.

- [ ] **Step 3: Implement image_decode.go**

Create `image_decode.go`:

```go
package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// encodePNG encodes raw pixel data to PNG format.
// components: 1=gray, 3=RGB, 4=CMYK (converted to RGB).
// alpha: optional soft mask bytes (one byte per pixel, same dimensions), nil if no alpha.
func encodePNG(pixels []byte, width, height, bpc, components int, alpha []byte) ([]byte, error) {
	if components == 4 {
		pixels = cmykToRGB(pixels, width*height)
		components = 3
	}

	var img image.Image
	switch {
	case components == 1 && alpha != nil:
		img = buildGrayAlpha(pixels, alpha, width, height, bpc)
	case components == 1:
		img = buildGray(pixels, width, height, bpc)
	case components == 3 && alpha != nil:
		img = buildRGBAlpha(pixels, alpha, width, height)
	default:
		img = buildRGB(pixels, width, height)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildRGB(pixels []byte, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	stride := width * 3
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := y*stride + x*3
			if off+2 >= len(pixels) {
				break
			}
			img.SetNRGBA(x, y, color.NRGBA{R: pixels[off], G: pixels[off+1], B: pixels[off+2], A: 255})
		}
	}
	return img
}

func buildRGBAlpha(pixels, alpha []byte, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	stride := width * 3
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := y*stride + x*3
			aOff := y*width + x
			if off+2 >= len(pixels) {
				break
			}
			a := byte(255)
			if aOff < len(alpha) {
				a = alpha[aOff]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: pixels[off], G: pixels[off+1], B: pixels[off+2], A: a})
		}
	}
	return img
}

func buildGray(pixels []byte, width, height, bpc int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))
	if bpc == 8 {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				off := y*width + x
				if off >= len(pixels) {
					break
				}
				img.SetGray(x, y, color.Gray{Y: pixels[off]})
			}
		}
	} else if bpc < 8 {
		// Sub-byte grayscale: unpack bits.
		pixelsPerByte := 8 / bpc
		maxVal := (1 << bpc) - 1
		byteIdx := 0
		for y := 0; y < height; y++ {
			byteIdx = y * ((width*bpc + 7) / 8)
			for x := 0; x < width; x++ {
				if byteIdx >= len(pixels) {
					break
				}
				bitOffset := (pixelsPerByte - 1 - (x % pixelsPerByte)) * bpc
				val := (int(pixels[byteIdx]) >> bitOffset) & maxVal
				gray := byte(val * 255 / maxVal)
				img.SetGray(x, y, color.Gray{Y: gray})
				if x%pixelsPerByte == pixelsPerByte-1 {
					byteIdx++
				}
			}
		}
	}
	return img
}

func buildGrayAlpha(pixels, alpha []byte, width, height, bpc int) *image.NRGBA {
	gray := buildGray(pixels, width, height, bpc)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			g := gray.GrayAt(x, y).Y
			a := byte(255)
			aOff := y*width + x
			if aOff < len(alpha) {
				a = alpha[aOff]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: g, G: g, B: g, A: a})
		}
	}
	return img
}

// cmykToRGB converts CMYK pixel data to RGB.
// Formula: R=(1-C)*(1-K), G=(1-M)*(1-K), B=(1-Y)*(1-K)
func cmykToRGB(pixels []byte, pixelCount int) []byte {
	rgb := make([]byte, pixelCount*3)
	for i := 0; i < pixelCount; i++ {
		off := i * 4
		if off+3 >= len(pixels) {
			break
		}
		c := float64(pixels[off]) / 255.0
		m := float64(pixels[off+1]) / 255.0
		y := float64(pixels[off+2]) / 255.0
		k := float64(pixels[off+3]) / 255.0
		rgb[i*3] = byte((1 - c) * (1 - k) * 255)
		rgb[i*3+1] = byte((1 - m) * (1 - k) * 255)
		rgb[i*3+2] = byte((1 - y) * (1 - k) * 255)
	}
	return rgb
}
```

- [ ] **Step 4: Run unit tests**

Run: `go test -run "TestEncodePNG" -v ./... 2>&1`
Expected: PASS.

- [ ] **Step 5: Wire PNG path into image.go**

In `image.go`, replace the `// For now, skip non-DCT images` block in `extractXObjectImage` with:

```go
	// Decode pixels and encode as PNG.
	var rawPixels []byte
	if stream.Decoded {
		rawPixels = stream.Data
	} else {
		var err error
		rawPixels, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return Image{}, false
		}
	}

	components := colorSpaceComponents(objects, stream.Dict, cs)
	if bpc == 0 {
		bpc = 8
	}

	// Resolve soft mask for alpha channel.
	var alphaMask []byte
	if smaskVal, ok := stream.Dict["/SMask"]; ok {
		alphaMask = decodeSoftMask(objects, smaskVal)
	}

	pngData, err := encodePNG(rawPixels, width, height, bpc, components, alphaMask)
	if err != nil {
		return Image{}, false
	}

	img.Data = pngData
	img.Format = ImageFormatPNG
	return img, true
```

Add these helper functions to `image.go`:

```go
// colorSpaceComponents returns the number of color components for the image's color space.
func colorSpaceComponents(objects map[int]*pdfObject, d pdfDict, cs ImageColorSpace) int {
	switch cs {
	case ColorSpaceDeviceGray:
		return 1
	case ColorSpaceDeviceRGB:
		return 3
	case ColorSpaceDeviceCMYK:
		return 4
	case ColorSpaceICCBased:
		return iccBasedComponents(objects, d)
	case ColorSpaceIndexed:
		return 1
	default:
		return 3
	}
}

// iccBasedComponents reads /N from the ICCBased color space stream.
func iccBasedComponents(objects map[int]*pdfObject, d pdfDict) int {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return 3
	}
	csVal = resolveRef(objects, csVal)
	arr, ok := csVal.(pdfArray)
	if !ok || len(arr) < 2 {
		return 3
	}
	iccStream := resolveRef(objects, arr[1])
	if s, ok := iccStream.(*pdfStream); ok {
		n := dictGetInt(s.Dict, "/N")
		if n > 0 {
			return n
		}
	}
	return 3
}

// decodeSoftMask decodes a soft mask image XObject to raw grayscale bytes.
func decodeSoftMask(objects map[int]*pdfObject, smaskVal pdfValue) []byte {
	resolved := resolveRef(objects, smaskVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return nil
	}
	if stream.Decoded {
		return stream.Data
	}
	data, err := decodeStream(stream.Dict, stream.Data)
	if err != nil {
		return nil
	}
	return data
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./... 2>&1`
Expected: PASS. Integration test now extracts both JPEG and PNG images.

- [ ] **Step 7: Commit**

```bash
git add image.go image_decode.go image_decode_test.go
git commit -m "feat: add PNG encoding for FlateDecode/raw images with soft mask"
```

---

### Task 3: CMYK→RGB conversion and unit tests

**Files:**
- Modify: `image_decode_test.go` (add CMYK test)
- `image_decode.go` already has `cmykToRGB` — verify it works end-to-end

- [ ] **Step 1: Write CMYK unit test**

Add to `image_decode_test.go`:

```go
func TestCMYKToRGB(t *testing.T) {
	// Pure cyan: C=255, M=0, Y=0, K=0 → R=0, G=255, B=255
	cmyk := []byte{255, 0, 0, 0}
	rgb := cmykToRGB(cmyk, 1)
	if rgb[0] != 0 || rgb[1] != 255 || rgb[2] != 255 {
		t.Errorf("cyan → (%d,%d,%d), want (0,255,255)", rgb[0], rgb[1], rgb[2])
	}

	// Pure black: C=0, M=0, Y=0, K=255 → R=0, G=0, B=0
	cmyk = []byte{0, 0, 0, 255}
	rgb = cmykToRGB(cmyk, 1)
	if rgb[0] != 0 || rgb[1] != 0 || rgb[2] != 0 {
		t.Errorf("black → (%d,%d,%d), want (0,0,0)", rgb[0], rgb[1], rgb[2])
	}

	// White: C=0, M=0, Y=0, K=0 → R=255, G=255, B=255
	cmyk = []byte{0, 0, 0, 0}
	rgb = cmykToRGB(cmyk, 1)
	if rgb[0] != 255 || rgb[1] != 255 || rgb[2] != 255 {
		t.Errorf("white → (%d,%d,%d), want (255,255,255)", rgb[0], rgb[1], rgb[2])
	}
}

func TestEncodePNGCMYK(t *testing.T) {
	// 1x1 CMYK pixel (pure magenta) → should produce valid RGB PNG
	pixels := []byte{0, 255, 0, 0} // C=0, M=255, Y=0, K=0 → R=255, G=0, B=255
	data, err := encodePNG(pixels, 1, 1, 8, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 255 {
		t.Errorf("magenta → (%d,%d,%d), want (255,0,255)", r>>8, g>>8, b>>8)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test -run "TestCMYK|TestEncodePNGCMYK" -v ./... 2>&1`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add image_decode_test.go
git commit -m "test: add CMYK to RGB conversion tests"
```

---

### Task 4: Indexed (palette) color space support

**Files:**
- Modify: `image_decode.go` (add `expandIndexed`)
- Modify: `image.go` (call `expandIndexed` before `encodePNG` for Indexed images)
- Modify: `image_decode_test.go` (add test)

- [ ] **Step 1: Write unit test for indexed expansion**

Add to `image_decode_test.go`:

```go
func TestExpandIndexed(t *testing.T) {
	// Palette: index 0 = red, index 1 = green, index 2 = blue
	palette := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255}
	indices := []byte{0, 1, 2, 0}
	rgb := expandIndexed(indices, palette, 3)
	// Expected: red, green, blue, red
	expected := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 0, 0}
	if len(rgb) != len(expected) {
		t.Fatalf("len=%d, want %d", len(rgb), len(expected))
	}
	for i := range expected {
		if rgb[i] != expected[i] {
			t.Errorf("byte[%d]=%d, want %d", i, rgb[i], expected[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExpandIndexed -v ./... 2>&1 | head -5`
Expected: FAIL — `expandIndexed` undefined.

- [ ] **Step 3: Implement expandIndexed in image_decode.go**

Add to `image_decode.go`:

```go
// expandIndexed expands palette-indexed pixel data to the base color space.
// baseComponents is the number of components in the base color space (e.g., 3 for RGB).
func expandIndexed(indices, palette []byte, baseComponents int) []byte {
	out := make([]byte, len(indices)*baseComponents)
	for i, idx := range indices {
		off := int(idx) * baseComponents
		for c := 0; c < baseComponents; c++ {
			if off+c < len(palette) {
				out[i*baseComponents+c] = palette[off+c]
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Wire into image.go**

In `extractXObjectImage`, before the `encodePNG` call, add Indexed handling:

```go
	// Expand indexed pixels to base color space.
	if cs == ColorSpaceIndexed {
		palette, baseComponents := resolveIndexedPalette(objects, stream.Dict)
		rawPixels = expandIndexed(rawPixels, palette, baseComponents)
		components = baseComponents
	}
```

Add helper to `image.go`:

```go
// resolveIndexedPalette extracts the palette bytes and base component count
// from an Indexed color space array: [/Indexed base hival lookup].
func resolveIndexedPalette(objects map[int]*pdfObject, d pdfDict) ([]byte, int) {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return nil, 3
	}
	csVal = resolveRef(objects, csVal)
	arr, ok := csVal.(pdfArray)
	if !ok || len(arr) < 4 {
		return nil, 3
	}

	// Base color space (arr[1]).
	baseCS := ColorSpaceDeviceRGB
	baseComponents := 3
	switch v := resolveRef(objects, arr[1]).(type) {
	case pdfName:
		baseCS = colorSpaceFromName(string(v))
		baseComponents = componentsByCS(baseCS)
	case pdfArray:
		if len(v) > 0 {
			if n, ok := v[0].(pdfName); ok && string(n) == "/ICCBased" && len(v) > 1 {
				if s, ok := resolveRef(objects, v[1]).(*pdfStream); ok {
					baseComponents = dictGetInt(s.Dict, "/N")
					if baseComponents == 0 {
						baseComponents = 3
					}
				}
			}
		}
	}

	// Lookup table (arr[3]) — string or stream.
	var palette []byte
	switch v := resolveRef(objects, arr[3]).(type) {
	case string:
		palette = []byte(v)
	case *pdfStream:
		if v.Decoded {
			palette = v.Data
		} else {
			decoded, err := decodeStream(v.Dict, v.Data)
			if err == nil {
				palette = decoded
			}
		}
	}

	_ = baseCS
	return palette, baseComponents
}

func componentsByCS(cs ImageColorSpace) int {
	switch cs {
	case ColorSpaceDeviceGray:
		return 1
	case ColorSpaceDeviceCMYK:
		return 4
	default:
		return 3
	}
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./... 2>&1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image.go image_decode.go image_decode_test.go
git commit -m "feat: add Indexed (palette) color space support for image extraction"
```

---

### Task 5: Inline image parsing (BI/ID/EI)

**Files:**
- Create: `image_inline.go`
- Modify: `content_parser.go` (replace `skipInlineImage` with `parseInlineImage` that returns parsed data)
- Modify: `image.go` (handle `BI` operator in walker)
- Modify: `image_decode_test.go` (add test for inline parsing)

- [ ] **Step 1: Write unit test for inline image parsing**

Add to `image_decode_test.go`:

```go
func TestParseInlineImageDict(t *testing.T) {
	dict := pdfDict{
		"/W":   10,
		"/H":   5,
		"/BPC": 8,
		"/CS":  pdfName("/RGB"),
	}
	norm := normalizeInlineDict(dict)
	if dictGetInt(norm, "/Width") != 10 {
		t.Errorf("Width=%d, want 10", dictGetInt(norm, "/Width"))
	}
	if dictGetInt(norm, "/Height") != 5 {
		t.Errorf("Height=%d, want 5", dictGetInt(norm, "/Height"))
	}
	if dictGetInt(norm, "/BitsPerComponent") != 8 {
		t.Errorf("BPC=%d, want 8", dictGetInt(norm, "/BitsPerComponent"))
	}
	if dictGetName(norm, "/ColorSpace") != "/DeviceRGB" {
		t.Errorf("CS=%s, want /DeviceRGB", dictGetName(norm, "/ColorSpace"))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestParseInlineImageDict -v ./... 2>&1 | head -5`
Expected: FAIL — `normalizeInlineDict` undefined.

- [ ] **Step 3: Implement image_inline.go**

Create `image_inline.go`:

```go
package asposepdf

// normalizeInlineDict expands abbreviated keys and values in an inline image dict
// to their full names (PDF spec Tables 4.43 and 4.44).
func normalizeInlineDict(d pdfDict) pdfDict {
	norm := make(pdfDict, len(d))
	for k, v := range d {
		fullKey := expandInlineKey(k)
		norm[fullKey] = expandInlineValue(v)
	}
	return norm
}

func expandInlineKey(k string) string {
	switch k {
	case "/BPC":
		return "/BitsPerComponent"
	case "/CS":
		return "/ColorSpace"
	case "/D":
		return "/Decode"
	case "/DP":
		return "/DecodeParms"
	case "/F":
		return "/Filter"
	case "/H":
		return "/Height"
	case "/IM":
		return "/ImageMask"
	case "/W":
		return "/Width"
	default:
		return k
	}
}

func expandInlineValue(v pdfValue) pdfValue {
	n, ok := v.(pdfName)
	if !ok {
		return v
	}
	switch string(n) {
	case "/G":
		return pdfName("/DeviceGray")
	case "/RGB":
		return pdfName("/DeviceRGB")
	case "/CMYK":
		return pdfName("/DeviceCMYK")
	case "/I":
		return pdfName("/Indexed")
	case "/AHx":
		return pdfName("/ASCIIHexDecode")
	case "/A85":
		return pdfName("/ASCII85Decode")
	case "/LZW":
		return pdfName("/LZWDecode")
	case "/Fl":
		return pdfName("/FlateDecode")
	case "/RL":
		return pdfName("/RunLengthDecode")
	case "/CCF":
		return pdfName("/CCITTFaxDecode")
	case "/DCT":
		return pdfName("/DCTDecode")
	default:
		return v
	}
}

// parseInlineImage parses the key-value pairs and image data of an inline image.
// The lexer is positioned just after the "BI" keyword.
// Returns the normalized dict and raw image data bytes (between ID and EI).
func parseInlineImage(l *lexer) (pdfDict, []byte) {
	dict := make(pdfDict)

	// Parse key-value pairs until "ID" keyword.
	for {
		tok, err := l.Next()
		if err != nil || tok.kind == tokEOF {
			return nil, nil
		}
		if tok.kind == tokKeyword && string(tok.raw) == "ID" {
			break
		}
		// Key must be a name.
		if tok.kind != tokName {
			continue
		}
		key := "/" + string(tok.raw)

		// Value.
		valTok, err := l.Next()
		if err != nil || valTok.kind == tokEOF {
			return nil, nil
		}
		val, err := parseValueFromToken(valTok, l)
		if err != nil {
			continue
		}
		dict[key] = val
	}

	// Skip one whitespace byte after ID.
	if l.pos < len(l.data) {
		l.pos++
	}

	// Find the end: whitespace + "EI" + delimiter.
	start := l.pos
	for l.pos < len(l.data)-2 {
		if isWhitespace(l.data[l.pos]) &&
			l.data[l.pos+1] == 'E' && l.data[l.pos+2] == 'I' &&
			(l.pos+3 >= len(l.data) || isDelimiter(l.data[l.pos+3])) {
			data := l.data[start:l.pos]
			l.pos += 3
			return normalizeInlineDict(dict), data
		}
		l.pos++
	}
	l.pos = len(l.data)
	return nil, nil
}
```

- [ ] **Step 4: Modify content_parser.go to use parseInlineImage**

In `content_parser.go`, in the `parseContentStream` function, change the `BI` handling from:

```go
if kw == "BI" {
    skipInlineImage(l)
    ops = append(ops, contentOp{Operator: "BI"})
    operands = nil
    continue
}
```

to:

```go
if kw == "BI" {
    dict, imgData := parseInlineImage(l)
    var biOperands []pdfValue
    if dict != nil {
        biOperands = []pdfValue{pdfValue(dict), pdfValue(string(imgData))}
    }
    ops = append(ops, contentOp{Operator: "BI", Operands: biOperands})
    operands = nil
    continue
}
```

- [ ] **Step 5: Handle BI in image walker**

In `image.go`, add `BI` case to `extractImagesFromOps`:

```go
		case "BI":
			if len(op.Operands) >= 2 {
				if img, ok := extractInlineImage(op.Operands[0], op.Operands[1], ctm); ok {
					images = append(images, img)
				}
			}
```

Add the helper function to `image.go`:

```go
// extractInlineImage builds an Image from parsed inline image operands.
func extractInlineImage(dictVal, dataVal pdfValue, ctm [6]float64) (Image, bool) {
	dict, ok := dictVal.(pdfDict)
	if !ok {
		return Image{}, false
	}
	rawData, ok := dataVal.(string)
	if !ok {
		return Image{}, false
	}

	width := dictGetInt(dict, "/Width")
	height := dictGetInt(dict, "/Height")
	bpc := dictGetInt(dict, "/BitsPerComponent")
	if width <= 0 || height <= 0 {
		return Image{}, false
	}
	if bpc == 0 {
		bpc = 8
	}

	cs := resolveColorSpaceInline(dict)
	filter := primaryFilter(dict)

	img := Image{
		Width:      width,
		Height:     height,
		BPC:        bpc,
		ColorSpace: cs,
		X:          ctm[4],
		Y:          ctm[5],
		PageWidth:  math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1]),
		PageHeight: math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3]),
		Inline:     true,
	}

	data := []byte(rawData)

	if filter == "/DCTDecode" {
		img.Data = data
		img.Format = ImageFormatJPEG
		return img, true
	}

	// Decode filters.
	if filter != "" {
		var err error
		data, err = applyFilter(filter, data)
		if err != nil {
			return Image{}, false
		}
	}

	components := componentsByCS(cs)
	pngData, err := encodePNG(data, width, height, bpc, components, nil)
	if err != nil {
		return Image{}, false
	}
	img.Data = pngData
	img.Format = ImageFormatPNG
	return img, true
}

// resolveColorSpaceInline resolves color space from an inline image dict.
// Inline images use the already-normalized full names.
func resolveColorSpaceInline(dict pdfDict) ImageColorSpace {
	csVal, ok := dict["/ColorSpace"]
	if !ok {
		return ColorSpaceDeviceGray
	}
	if n, ok := csVal.(pdfName); ok {
		return colorSpaceFromName(string(n))
	}
	return ColorSpaceDeviceRGB
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./... 2>&1`
Expected: PASS. Existing content_parser_test.go and text tests still pass. Image extraction includes inline images.

- [ ] **Step 7: Commit**

```bash
git add image_inline.go image.go image_decode_test.go content_parser.go
git commit -m "feat: add inline image extraction (BI/ID/EI)"
```

---

### Task 6: DCTDecode + SMask → PNG re-encode

When a JPEG image has `/SMask`, we can't output JPEG (no alpha support). Decode JPEG pixels, apply alpha, output PNG.

**Files:**
- Modify: `image.go` (check SMask before JPEG passthrough)
- Modify: `image_decode.go` (add `decodeJPEG` helper)
- Modify: `image_decode_test.go` (add test)

- [ ] **Step 1: Write unit test**

Add to `image_decode_test.go`:

```go
import (
	"image/jpeg"
)

func TestDecodeJPEGToPixels(t *testing.T) {
	// Create a tiny JPEG in memory.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100})

	pixels, w, h, err := decodeJPEGToPixels(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if w != 2 || h != 2 {
		t.Errorf("size=%dx%d, want 2x2", w, h)
	}
	if len(pixels) != 2*2*3 {
		t.Errorf("pixel count=%d, want %d", len(pixels), 2*2*3)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDecodeJPEGToPixels -v ./... 2>&1 | head -5`
Expected: FAIL — `decodeJPEGToPixels` undefined.

- [ ] **Step 3: Implement decodeJPEGToPixels in image_decode.go**

Add to `image_decode.go`:

```go
import (
	"image/jpeg"
)

// decodeJPEGToPixels decodes JPEG bytes to raw RGB pixel data.
func decodeJPEGToPixels(data []byte) (pixels []byte, width, height int, err error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := img.Bounds()
	width = bounds.Dx()
	height = bounds.Dy()
	pixels = make([]byte, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			off := (y*width + x) * 3
			pixels[off] = byte(r >> 8)
			pixels[off+1] = byte(g >> 8)
			pixels[off+2] = byte(b >> 8)
		}
	}
	return pixels, width, height, nil
}
```

- [ ] **Step 4: Update JPEG path in image.go for SMask**

In `extractXObjectImage`, replace the DCTDecode block:

```go
	if filter == "/DCTDecode" {
		// Check for soft mask — JPEG can't hold alpha, must re-encode as PNG.
		if smaskVal, ok := stream.Dict["/SMask"]; ok {
			alphaMask := decodeSoftMask(objects, smaskVal)
			if alphaMask != nil {
				jpegData := stream.Data
				if stream.Decoded {
					jpegData = getRawStreamData(objects, formVal)
				}
				if jpegData == nil {
					return Image{}, false
				}
				pixels, _, _, err := decodeJPEGToPixels(jpegData)
				if err != nil {
					return Image{}, false
				}
				pngData, err := encodePNG(pixels, width, height, 8, 3, alphaMask)
				if err != nil {
					return Image{}, false
				}
				img.Data = pngData
				img.Format = ImageFormatPNG
				return img, true
			}
		}

		// No alpha — JPEG passthrough.
		img.Data = stream.Data
		img.Format = ImageFormatJPEG
		return img, true
	}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./... 2>&1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add image.go image_decode.go image_decode_test.go
git commit -m "feat: re-encode JPEG as PNG when soft mask present"
```

---

### Task 7: Form XObject recursion and CLAUDE.md update

Handle images inside Form XObjects (nested `Do` → `/Subtype /Form`), and update documentation.

**Files:**
- Modify: `image.go` (recurse into Form XObjects)
- Modify: `CLAUDE.md` (add image extraction docs)

- [ ] **Step 1: Add Form XObject recursion to image walker**

In `extractImagesFromOps`, add Form XObject handling to the `Do` case. Replace:

```go
		case "Do":
			if len(op.Operands) >= 1 {
				name := operandName(op.Operands[0])
				if img, ok := extractXObjectImage(objects, resources, name, ctm); ok {
					images = append(images, img)
				}
			}
```

with:

```go
		case "Do":
			if len(op.Operands) >= 1 {
				name := operandName(op.Operands[0])
				// Try as image first.
				if img, ok := extractXObjectImage(objects, resources, name, ctm); ok {
					images = append(images, img)
				} else {
					// Try as Form XObject — recurse.
					formImages := extractFormXObjectImages(objects, resources, name, ctm)
					images = append(images, formImages...)
				}
			}
```

Add helper:

```go
// extractFormXObjectImages extracts images from a Form XObject's content stream.
func extractFormXObjectImages(objects map[int]*pdfObject, resources pdfDict, name string, ctm [6]float64) []Image {
	if name == "" || resources == nil {
		return nil
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return nil
	}
	xobjDict, ok := resolveRefToDict(objects, xobjVal)
	if !ok {
		return nil
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return nil
	}
	resolved := resolveRef(objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return nil
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Form" {
		return nil
	}

	var data []byte
	if stream.Decoded {
		data = stream.Data
	} else {
		var err error
		data, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return nil
		}
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil
	}

	// Apply Form's /Matrix to CTM.
	formCTM := ctm
	if matVal, ok := stream.Dict["/Matrix"]; ok {
		if arr, ok := matVal.(pdfArray); ok && len(arr) == 6 {
			var fm [6]float64
			for i := 0; i < 6; i++ {
				fm[i] = operandFloat(arr[i])
			}
			formCTM = matMul(fm, ctm)
		}
	}

	// Use form's resources, falling back to parent.
	formResources := resources
	if resVal, ok := stream.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(objects, resVal); ok {
			formResources = rd
		}
	}

	images, _ := extractImagesFromOps(objects, ops, formResources)
	// Adjust positions for form CTM.
	for i := range images {
		images[i].X += formCTM[4] - ctm[4]
		images[i].Y += formCTM[5] - ctm[5]
	}
	return images
}
```

- [ ] **Step 2: Update CLAUDE.md**

Add to the Public API section under `page.go`:

```
- `(*Page).ExtractImages() ([]Image, error)` — returns all images found on the page
- `(*Document).ExtractImages() ([][]Image, error)` — returns images for all pages (one slice per page)
- `Image` struct — Data, Format, Width, Height, BPC, ColorSpace, X, Y, PageWidth, PageHeight, Inline
- `ImageFormat` — ImageFormatPNG, ImageFormatJPEG
- `ImageColorSpace` — ColorSpaceDeviceRGB, ColorSpaceDeviceGray, ColorSpaceDeviceCMYK, ColorSpaceIndexed, ColorSpaceICCBased
```

Add to the architecture section:

```
### Image extraction (`image.go`, `image_decode.go`, `image_inline.go`)

1. Content stream walker tracks CTM via `cm`/`q`/`Q` and collects images on `Do` (XObject) and `BI` (inline)
2. DCTDecode images are passed through as JPEG; all others are decoded to pixels and encoded as PNG
3. Color spaces: DeviceRGB, DeviceGray, DeviceCMYK (→RGB), Indexed (palette expansion), ICCBased (treated as underlying RGB/Gray/CMYK)
4. Soft masks (`/SMask`) are applied as PNG alpha channels; JPEG+SMask is re-encoded as PNG
5. Inline images (BI/ID/EI) are parsed with abbreviation expansion (PDF spec Tables 4.43/4.44)
6. Form XObjects are recursed into with inherited CTM and resources
```

- [ ] **Step 3: Run all tests**

Run: `go test ./... 2>&1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add image.go CLAUDE.md
git commit -m "feat: add Form XObject recursion and update CLAUDE.md for image extraction"
```

---

## Summary

| Task | Description | Key files |
|---|---|---|
| 1 | Types, walker, JPEG passthrough | `image.go`, `image_test.go` |
| 2 | PNG encoding for RGB/Gray/FlateDecode + soft mask | `image_decode.go`, `image_decode_test.go` |
| 3 | CMYK→RGB unit tests | `image_decode_test.go` |
| 4 | Indexed (palette) color space | `image.go`, `image_decode.go` |
| 5 | Inline images (BI/ID/EI) | `image_inline.go`, `content_parser.go` |
| 6 | JPEG + SMask → PNG re-encode | `image.go`, `image_decode.go` |
| 7 | Form XObject recursion + docs | `image.go`, `CLAUDE.md` |
