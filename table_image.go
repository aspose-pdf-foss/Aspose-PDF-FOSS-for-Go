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
