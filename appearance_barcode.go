// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// generateBarcodeFieldAppearance renders a BarcodeField's current value as
// a barcode symbol filling the widget rect. Unlike the other /Tx appearance
// generators, no chrome is drawn by default (drawWidgetChrome's fill/border
// would eat into a barcode's required quiet zone) — background/border are
// drawn only when the caller set them explicitly via SetStyle.
func generateBarcodeFieldAppearance(form *Form, widget pdfDict) *pdfStream {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{})
	}

	b := newAppearanceBuilder()
	if bg := mkColor(widget, "/BG"); bg != nil {
		b.PushState()
		b.SetFillColorRGB(*bg)
		b.Rect(0, 0, width, height)
		b.Fill()
		b.PopState()
	}
	if bc := mkColor(widget, "/BC"); bc != nil {
		bw, bstyle, dash := readBS(widget)
		if bw <= 0 {
			bw = 1
		}
		drawStandardRectBorder(b, width, height, bstyle, bw, dash, bc)
	}

	value := decodeFormString(widget["/V"])
	if value == "" {
		return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
	}

	symbology := barcodeSymbologyFromDict(widget)
	mat, err := renderBarcodeModules(symbology, value)
	if err != nil {
		// Every public setter (AddBarcodeField, SetValue, SetSymbology)
		// validates value against symbology before writing /V or the
		// marker, so this is unreachable through the public API. It stays
		// a silent chrome-only fallback, not a panic, only in case /V or
		// the private marker was edited directly on the parsed dict.
		return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
	}
	_, inkColor, _ := parseDA(dictGetString(widget, "/DA"))
	drawBarcodeMatrix(b, mat, width, height, inkColor)

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawBarcodeMatrix paints mat's dark modules as filled rectangles.
// Horizontally-adjacent dark modules within a row are merged into a single
// rectangle to keep the content stream compact. A linear (Square==false)
// matrix stretches to the widget's full height; a square one (QR) scales
// uniformly to the largest size that fits and is centred.
func drawBarcodeMatrix(b *appearanceBuilder, mat barcodeModules, width, height float64, color Color) {
	if mat.Cols <= 0 || mat.Rows <= 0 {
		return
	}
	var moduleW, moduleH, offX, offY float64
	if mat.Square {
		size := math.Min(width, height) / float64(mat.Cols)
		moduleW, moduleH = size, size
		offX = (width - size*float64(mat.Cols)) / 2
		offY = (height - size*float64(mat.Rows)) / 2
	} else {
		moduleW = width / float64(mat.Cols)
		moduleH = height
	}

	b.PushState()
	b.SetFillColorRGB(color)
	for r := 0; r < mat.Rows; r++ {
		c := 0
		for c < mat.Cols {
			if !mat.Bits[r*mat.Cols+c] {
				c++
				continue
			}
			start := c
			for c < mat.Cols && mat.Bits[r*mat.Cols+c] {
				c++
			}
			x := offX + float64(start)*moduleW
			y := offY + float64(mat.Rows-1-r)*moduleH
			b.Rect(x, y, float64(c-start)*moduleW, moduleH)
		}
	}
	b.Fill()
	b.PopState()
}
