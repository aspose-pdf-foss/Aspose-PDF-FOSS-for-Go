// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
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

// jpegHasAdobeMarker reports whether a JPEG carries an APP14 "Adobe" marker,
// which signals Photoshop/Acrobat CMYK or YCCK data stored with inverted ink
// values. It walks the marker segments rather than scanning raw bytes so an
// "Adobe" byte sequence inside entropy-coded data can't trigger a false match.
func jpegHasAdobeMarker(data []byte) bool {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return false
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return false
		}
		marker := data[i+1]
		i += 2
		if marker == 0xD9 || marker == 0xDA { // EOI or start of scan
			return false
		}
		if marker >= 0xD0 && marker <= 0xD7 { // RSTn: no length
			continue
		}
		if i+2 > len(data) {
			return false
		}
		segLen := int(data[i])<<8 | int(data[i+1])
		if segLen < 2 {
			return false
		}
		if marker == 0xEE && i+2+5 <= len(data) && string(data[i+2:i+2+5]) == "Adobe" {
			return true
		}
		i += segLen
	}
	return false
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

// decodeJPEGToPixels decodes JPEG bytes to raw RGB pixel data.
func decodeJPEGToPixels(data []byte) (pixels []byte, width, height int, err error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	// Adobe CMYK/YCCK JPEGs store their channels inverted (0 = full ink) and
	// carry an APP14 "Adobe" marker; Go's decoder returns the raw inverted CMYK,
	// so a plain CMYK→RGB turns white into black. Re-invert before converting.
	if cmyk, ok := img.(*image.CMYK); ok && jpegHasAdobeMarker(data) {
		for i := range cmyk.Pix {
			cmyk.Pix[i] = 255 - cmyk.Pix[i]
		}
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
