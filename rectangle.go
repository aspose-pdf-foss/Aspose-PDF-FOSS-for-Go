package asposepdf

import "fmt"

// Rectangle represents a PDF rectangle [llx, lly, urx, ury] in points (1/72 inch).
type Rectangle struct {
	LLX, LLY float64 // lower-left corner
	URX, URY float64 // upper-right corner
}

// validate checks that the rectangle has positive width and height.
func (r Rectangle) validate() error {
	if r.URX <= r.LLX {
		return fmt.Errorf("invalid rectangle: URX (%.2f) must be greater than LLX (%.2f)", r.URX, r.LLX)
	}
	if r.URY <= r.LLY {
		return fmt.Errorf("invalid rectangle: URY (%.2f) must be greater than LLY (%.2f)", r.URY, r.LLY)
	}
	return nil
}
