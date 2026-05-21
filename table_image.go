// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // register decoder
	_ "image/png"  // register decoder
	"io"
	"os"
)

// measureImage returns the natural (pixel) dimensions of a PNG or JPEG image.
//
// If path != "", the file is opened and decoded for dimensions. If path == ""
// and data != nil, the bytes are decoded directly. Returns an error if neither
// source is valid or the data is not a supported image format.
//
// Uses image.DecodeConfig (header-only decode — does not allocate pixel buffers).
func measureImage(path string, data []byte) (width, height float64, err error) {
	var r io.Reader
	switch {
	case path != "":
		f, ferr := os.Open(path)
		if ferr != nil {
			return 0, 0, fmt.Errorf("measureImage: %w", ferr)
		}
		defer f.Close()
		r = f
	case data != nil:
		r = bytes.NewReader(data)
	default:
		return 0, 0, fmt.Errorf("measureImage: empty path and nil data")
	}
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, fmt.Errorf("measureImage: %w", err)
	}
	return float64(cfg.Width), float64(cfg.Height), nil
}

// drawImageInCell renders the cell's image into the given interior rectangle,
// scaling proportionally to fit (width-first; height-constrain if needed)
// while preserving aspect ratio. HAlign/VAlign place the image within any
// extra interior space.
func drawImageInCell(page *Page, cell *Cell, interior Rectangle, style TextStyle) error {
	var src []byte
	if cell.imageStream != nil {
		src = cell.imageStream
	}
	natW, natH, err := measureImage(cell.imagePath, src)
	if err != nil {
		return err
	}
	if natW <= 0 || natH <= 0 {
		return fmt.Errorf("image has zero dimension")
	}
	intW := interior.URX - interior.LLX
	intH := interior.URY - interior.LLY
	aspect := natW / natH
	// Scale by width first, then constrain by height if too tall.
	scaleW := intW
	scaleH := intW / aspect
	if scaleH > intH {
		scaleH = intH
		scaleW = intH * aspect
	}
	// Position by alignment within (intW × intH).
	var llx, lly float64
	switch style.HAlign {
	case HAlignCenter:
		llx = interior.LLX + (intW-scaleW)/2
	case HAlignRight:
		llx = interior.URX - scaleW
	default:
		llx = interior.LLX
	}
	switch style.VAlign {
	case VAlignMiddle:
		lly = interior.LLY + (intH-scaleH)/2
	case VAlignTop:
		lly = interior.URY - scaleH
	default:
		lly = interior.LLY
	}
	rect := Rectangle{LLX: llx, LLY: lly, URX: llx + scaleW, URY: lly + scaleH}
	if cell.imageStream != nil {
		return page.AddImageFromStream(bytes.NewReader(cell.imageStream), rect)
	}
	return page.AddImage(cell.imagePath, rect)
}
