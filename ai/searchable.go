// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"fmt"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// MakeSearchable OCRs the document's scanned pages and writes the recognized
// text back as an invisible text layer (text rendering mode 3), so the PDF
// becomes selectable, copyable and Ctrl+F-searchable in any viewer while its
// visual appearance is unchanged. Returns the number of pages that received a
// text layer. The document is modified in place; call Save/WriteTo to persist.
//
// Placement accuracy depends on the engine: with word/line boxes (Tesseract,
// cloud OCR adapters) the hidden text sits exactly under the printed text;
// with the coordinate-less LLMOCREngine the lines are laid out on an even
// grid over the scanned region — search and copy work, selection alignment is
// approximate. Pages with a non-zero /Rotate always use the grid layout.
func (c *OcrCopilot) MakeSearchable(ctx context.Context) (int, error) {
	targets, err := c.targetPages()
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, num := range targets {
		res, err := c.recognizePage(ctx, num)
		if err != nil {
			return processed, fmt.Errorf("ai: OCR page %d: %w", num, err)
		}
		if len(res.Lines) == 0 {
			continue
		}
		page, err := c.opts.Document.Page(num)
		if err != nil {
			return processed, err
		}
		if err := c.placeTextLayer(page, res); err != nil {
			return processed, fmt.Errorf("ai: page %d text layer: %w", num, err)
		}
		processed++
	}
	return processed, nil
}

// placeTextLayer writes one page's recognized text as invisible glyphs.
func (c *OcrCopilot) placeTextLayer(page *pdf.Page, res *OCRResult) error {
	crop, err := page.CropBox()
	if err != nil {
		return err
	}
	scale := 72.0 / c.opts.DPI

	// Box-accurate placement needs coordinates on every line and an
	// unrotated page (RenderImage bakes /Rotate into the raster, so pixel
	// boxes would need derotation — grid fallback instead).
	boxMode := page.Rotation() == 0
	for _, l := range res.Lines {
		if l.Box == nil && len(l.Words) == 0 {
			boxMode = false
			break
		}
	}
	if boxMode {
		for _, line := range res.Lines {
			if len(line.Words) > 0 {
				for _, w := range line.Words {
					if err := c.placeBoxedText(page, w.Text, w.Box, crop, scale); err != nil {
						return err
					}
				}
				continue
			}
			if err := c.placeBoxedText(page, line.Text, *line.Box, crop, scale); err != nil {
				return err
			}
		}
		return nil
	}
	return c.placeGridText(page, res, crop)
}

// placeBoxedText draws one invisible text run at its recognized position.
// box is in rendered-image pixel space (origin top-left, Y down); crop is the
// page's CropBox; scale converts pixels to points (72/DPI).
func (c *OcrCopilot) placeBoxedText(page *pdf.Page, text string, box OCRBox, crop pdf.Rectangle, scale float64) error {
	if text == "" || box.Right <= box.Left || box.Bottom <= box.Top {
		return nil
	}
	// Pixel box → PDF user space (Y flips against the CropBox top).
	rect := pdf.Rectangle{
		LLX: crop.LLX + box.Left*scale,
		LLY: crop.URY - box.Bottom*scale,
		URX: crop.LLX + box.Right*scale,
		URY: crop.URY - box.Top*scale,
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY

	// Font size from the box height, shrunk when the estimated advance
	// (≈0.5 em per rune) would overflow the box width.
	size := clampF(h*0.85, 2, 144)
	runes := len([]rune(text))
	if est := 0.5 * size * float64(runes); est > w && runes > 0 {
		size = clampF(w/(0.5*float64(runes)), 2, 144)
	}

	// Widen the draw rect so the single line never word-wraps (the width
	// estimate is approximate); invisible overflow is harmless.
	drawW := w
	if est := 0.5 * size * float64(runes) * 1.2; est > drawW {
		drawW = est
	}
	drawRect := pdf.Rectangle{LLX: rect.LLX, LLY: rect.URY - size*1.3, URX: rect.LLX + drawW + 2, URY: rect.URY}
	return page.AddText(text, pdf.TextStyle{Font: c.opts.Font, Size: size, Invisible: true}, drawRect)
}

// placeGridText lays the recognized lines out on an even vertical grid over
// the scanned region (the dominant image's rect when detected, the CropBox
// otherwise) — the coordinate-less fallback.
func (c *OcrCopilot) placeGridText(page *pdf.Page, res *OCRResult, crop pdf.Rectangle) error {
	region := crop
	if _, imgRect, err := pageNeedsOCR(page); err == nil && imgRect != nil {
		region = *imgRect
	}
	regionH := region.URY - region.LLY
	regionW := region.URX - region.LLX
	if regionH <= 0 || regionW <= 0 {
		return nil
	}
	lineH := regionH / float64(len(res.Lines))
	size := clampF(lineH*0.75, 4, 28)
	for i, line := range res.Lines {
		if line.Text == "" {
			continue
		}
		rect := pdf.Rectangle{
			LLX: region.LLX,
			LLY: region.URY - float64(i+1)*lineH,
			URX: region.URX,
			URY: region.URY - float64(i)*lineH,
		}
		if err := page.AddText(line.Text, pdf.TextStyle{Font: c.opts.Font, Size: size, Invisible: true}, rect); err != nil {
			return err
		}
	}
	return nil
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
