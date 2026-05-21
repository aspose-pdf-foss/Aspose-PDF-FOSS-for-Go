// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// ImageToDocumentOptions controls page sizing for ImageToDocument.
type ImageToDocumentOptions struct {
	PageWidth    float64 // explicit page size in points; 0 = auto from image
	PageHeight   float64
	MarginLeft   float64
	MarginRight  float64
	MarginTop    float64
	MarginBottom float64
}

const defaultDPI = 72.0

// parseJPEGDPI reads DPI from JFIF APP0 marker. Returns defaultDPI if not found.
func parseJPEGDPI(data []byte) float64 {
	// Look for APP0 (FF E0) after SOI (FF D8).
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return defaultDPI
	}

	offset := 2
	for offset+4 < len(data) {
		if data[offset] != 0xFF {
			break
		}
		marker := data[offset+1]
		if marker == 0xE0 { // APP0
			segLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
			if offset+2+segLen > len(data) || segLen < 14 {
				return defaultDPI
			}
			seg := data[offset+4:]
			// Check JFIF identifier: "JFIF\0"
			if len(seg) < 12 || string(seg[:5]) != "JFIF\x00" {
				return defaultDPI
			}
			units := seg[7]
			xDensity := float64(binary.BigEndian.Uint16(seg[8:10]))
			switch units {
			case 1: // dots per inch
				if xDensity > 0 {
					return xDensity
				}
			case 2: // dots per cm
				if xDensity > 0 {
					return xDensity * 2.54
				}
			}
			return defaultDPI
		}
		// Skip segment.
		if offset+4 > len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		offset += 2 + segLen
	}
	return defaultDPI
}

// parsePNGDPI reads DPI from pHYs chunk. Returns defaultDPI if not found.
func parsePNGDPI(data []byte) float64 {
	// PNG signature is 8 bytes, then chunks.
	if len(data) < 8 {
		return defaultDPI
	}
	offset := 8
	for offset+12 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])

		if chunkType == "pHYs" && chunkLen == 9 && offset+8+9 <= len(data) {
			ppu := data[offset+8:]
			ppuX := binary.BigEndian.Uint32(ppu[0:4])
			unit := ppu[8]
			if unit == 1 && ppuX > 0 {
				// Pixels per meter → DPI.
				return float64(ppuX) / 39.3701
			}
			return defaultDPI
		}

		// Stop at IDAT — pHYs must appear before it.
		if chunkType == "IDAT" {
			break
		}

		offset += 12 + chunkLen // 4 len + 4 type + data + 4 crc
	}
	return defaultDPI
}

// ImageToDocument creates a new Document with a single page containing the image.
// Page size is determined by image dimensions and DPI metadata (default 72 DPI).
func ImageToDocument(path string, opts ...ImageToDocumentOptions) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("image to document: %w", err)
	}
	return imageToDocumentFromBytes(data, opts...)
}

// ImageToDocumentFromStream creates a new Document from an image reader.
// Format is detected by magic bytes.
func ImageToDocumentFromStream(r io.Reader, opts ...ImageToDocumentOptions) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("image to document: %w", err)
	}
	return imageToDocumentFromBytes(data, opts...)
}

func imageToDocumentFromBytes(data []byte, opts ...ImageToDocumentOptions) (*Document, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("image to document: empty data")
	}

	format, err := detectImageFormat(data)
	if err != nil {
		return nil, err
	}

	imgStream, smaskStream, err := createImageXObject(data, format)
	if err != nil {
		return nil, err
	}

	imgW := dictGetInt(imgStream.Dict, "/Width")
	imgH := dictGetInt(imgStream.Dict, "/Height")
	if imgW <= 0 || imgH <= 0 {
		return nil, fmt.Errorf("image to document: invalid image dimensions %dx%d", imgW, imgH)
	}

	// Determine DPI.
	var dpi float64
	switch format {
	case ImageFormatJPEG:
		dpi = parseJPEGDPI(data)
	case ImageFormatPNG:
		dpi = parsePNGDPI(data)
	}
	if dpi <= 0 {
		dpi = defaultDPI
	}

	// Image size in points.
	imgPtW := float64(imgW) / dpi * 72.0
	imgPtH := float64(imgH) / dpi * 72.0

	var opt ImageToDocumentOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	var pageW, pageH float64
	var imgRect Rectangle

	if opt.PageWidth > 0 && opt.PageHeight > 0 {
		// Explicit page size — fit image within margins.
		pageW = opt.PageWidth
		pageH = opt.PageHeight
		availW := pageW - opt.MarginLeft - opt.MarginRight
		availH := pageH - opt.MarginTop - opt.MarginBottom
		if availW <= 0 || availH <= 0 {
			return nil, fmt.Errorf("margins exceed page dimensions")
		}

		// Scale to fit, preserving aspect ratio.
		scaleX := availW / imgPtW
		scaleY := availH / imgPtH
		scale := scaleX
		if scaleY < scale {
			scale = scaleY
		}
		fitW := imgPtW * scale
		fitH := imgPtH * scale

		// Center in available area.
		x := opt.MarginLeft + (availW-fitW)/2
		y := opt.MarginBottom + (availH-fitH)/2
		imgRect = Rectangle{LLX: x, LLY: y, URX: x + fitW, URY: y + fitH}
	} else {
		// Auto page size from image + margins.
		pageW = imgPtW + opt.MarginLeft + opt.MarginRight
		pageH = imgPtH + opt.MarginTop + opt.MarginBottom
		imgRect = Rectangle{
			LLX: opt.MarginLeft,
			LLY: opt.MarginBottom,
			URX: opt.MarginLeft + imgPtW,
			URY: opt.MarginBottom + imgPtH,
		}
	}

	// Build document objects.
	nextID := 1

	// SMask (if present).
	var smaskID int
	if smaskStream != nil {
		smaskID = nextID
		nextID++
	}

	// Image XObject.
	imgID := nextID
	nextID++
	if smaskStream != nil {
		imgStream.Dict["/SMask"] = pdfRef{Num: smaskID}
	}

	// Content stream.
	w := imgRect.URX - imgRect.LLX
	h := imgRect.URY - imgRect.LLY
	contentData := fmt.Sprintf("q\n%s 0 0 %s %s %s cm\n/Im0 Do\nQ\n",
		formatFloat(w), formatFloat(h), formatFloat(imgRect.LLX), formatFloat(imgRect.LLY))
	contentID := nextID
	nextID++

	// Page.
	pageID := nextID
	nextID++

	objects := make(map[int]*pdfObject)

	if smaskStream != nil {
		objects[smaskID] = &pdfObject{Num: smaskID, Value: smaskStream}
	}
	objects[imgID] = &pdfObject{Num: imgID, Value: imgStream}
	objects[contentID] = &pdfObject{Num: contentID, Value: &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, pageW, pageH},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: imgID},
			},
		},
		"/Contents": pdfRef{Num: contentID},
	}
	pageObj := &pdfObject{Num: pageID, Value: pageDict}
	objects[pageID] = pageObj

	return &Document{
		objects: objects,
		pages:   []*pdfObject{pageObj},
		nextID:  nextID,
	}, nil
}
