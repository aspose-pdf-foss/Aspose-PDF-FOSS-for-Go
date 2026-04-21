package asposepdf

import (
	"bytes"
	"compress/zlib"
)

// buildFontFile2Stream creates a /FontFile2 stream with the raw TTF bytes,
// compressed via FlateDecode. /Length1 holds the uncompressed length.
func buildFontFile2Stream(f *ttfFont) *pdfStream {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(f.data)
	_ = zw.Close()
	return &pdfStream{
		Dict: pdfDict{
			"/Length1": len(f.data),
			"/Filter":  pdfName("/FlateDecode"),
		},
		Data: buf.Bytes(),
	}
}

// buildFontDescriptor creates a /FontDescriptor dict referencing the given
// FontFile2 object ID.
func buildFontDescriptor(f *ttfFont, fontFile2ID int) pdfDict {
	scale := func(v int16) int {
		// Scale FUnits to 1/1000 em.
		return int(float64(v) * 1000.0 / float64(f.unitsPerEm))
	}
	// Flags (PDF spec Table 123):
	//   bit 1 (1):      FixedPitch
	//   bit 3 (4):      Symbolic  — always set for embedded TTF
	//   bit 7 (64):     Italic
	//   bit 19 (262144): ForceBold
	flags := 0x4 // Symbolic
	if f.isFixedPitch {
		flags |= 0x1
	}
	if f.flagsItalic {
		flags |= 0x40
	}
	if f.flagsBold {
		flags |= 0x40000
	}
	// StemV heuristic: 50 at weight 400, +0.2 per weight unit.
	stemV := 50
	if f.weight > 0 {
		stemV = 50 + int(float64(f.weight-400)*0.2)
		if stemV < 50 {
			stemV = 50
		}
	}
	cap := scale(f.capHeight)
	if cap == 0 {
		cap = scale(f.ascent)
	}
	return pdfDict{
		"/Type":     pdfName("/FontDescriptor"),
		"/FontName": pdfName("/" + f.postScriptName),
		"/Flags":    flags,
		"/FontBBox": pdfArray{
			scale(f.xMin), scale(f.yMin),
			scale(f.xMax), scale(f.yMax),
		},
		"/ItalicAngle": f.italicAngle,
		"/Ascent":      scale(f.ascent),
		"/Descent":     scale(f.descent),
		"/CapHeight":   cap,
		"/StemV":       stemV,
		"/FontFile2":   pdfRef{Num: fontFile2ID},
	}
}

// addObject appends a new PDF object to the document and returns its ID.
func (d *Document) addObject(value pdfValue) int {
	id := d.nextID
	d.nextID++
	d.objects[id] = &pdfObject{Num: id, Value: value}
	return id
}
