package asposepdf

import (
	"encoding/binary"
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
