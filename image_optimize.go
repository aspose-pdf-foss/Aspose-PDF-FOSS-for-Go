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
