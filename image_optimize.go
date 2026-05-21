// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

// downscaleImage resizes img to newWidth x newHeight using bilinear interpolation.
func downscaleImage(img image.Image, newWidth, newHeight int) *image.NRGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map destination pixel to source coordinates.
			srcX := (float64(x)+0.5)*float64(srcW)/float64(newWidth) - 0.5
			srcY := (float64(y)+0.5)*float64(srcH)/float64(newHeight) - 0.5

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

// bilinear performs bilinear interpolation on four uint32 corner values
// (as returned by color.RGBA()) given fractional offsets dx, dy in [0,1].
func bilinear(v00, v10, v01, v11 uint32, dx, dy float64) uint32 {
	top := float64(v00)*(1-dx) + float64(v10)*dx
	bot := float64(v01)*(1-dx) + float64(v11)*dx
	return uint32(top*(1-dy) + bot*dy)
}

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
	if isJPEG || needsPNGToJPEG {
		// downscaleImage always produces NRGBA (RGB); JPEG encodes as RGB.
		stream.Dict["/ColorSpace"] = pdfName("/DeviceRGB")
	}
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
