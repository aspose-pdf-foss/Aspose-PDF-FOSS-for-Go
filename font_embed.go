// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
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

// defaultCIDWidth is the /DW value written for embedded TTFs.
const defaultCIDWidth = 500

// buildWArray builds the /W array for a CIDFontType2 dict.
// Widths equal to defaultCIDWidth are omitted (covered by /DW).
// Runs of identical non-default widths > 5 consecutive glyphs are emitted as
// `cFirst cLast w`; all other non-default widths are grouped in the array form
// `cStart [w1 w2 w3 ...]`.
func buildWArray(f *ttfFont) pdfArray {
	// Scale each glyph's advance to 1/1000 em (rounded).
	widths := make([]int, len(f.glyphWidths))
	for i, w := range f.glyphWidths {
		widths[i] = int(float64(w)*1000.0/float64(f.unitsPerEm) + 0.5)
	}

	var arr pdfArray
	i := 0
	for i < len(widths) {
		if widths[i] == defaultCIDWidth {
			i++
			continue
		}
		j := i + 1
		for j < len(widths) && widths[j] == widths[i] {
			j++
		}
		runLen := j - i
		if runLen > 5 {
			arr = append(arr, i, j-1, widths[i])
			i = j
			continue
		}
		k := i + 1
		for k < len(widths) && widths[k] != defaultCIDWidth {
			lookEnd := k + 1
			for lookEnd < len(widths) && widths[lookEnd] == widths[k] {
				lookEnd++
			}
			if lookEnd-k > 5 {
				break
			}
			k = lookEnd
		}
		seq := make(pdfArray, 0, k-i)
		for g := i; g < k; g++ {
			seq = append(seq, widths[g])
		}
		arr = append(arr, i, seq)
		i = k
	}
	return arr
}

// buildToUnicodeCMap generates the /ToUnicode CMap stream for the font.
// Emits one bfchar entry per (glyphID, rune) pair from runeToGlyph.
func buildToUnicodeCMap(f *ttfFont) *pdfStream {
	type pair struct {
		gid uint16
		r   rune
	}
	pairs := make([]pair, 0, len(f.runeToGlyph))
	for r, gid := range f.runeToGlyph {
		pairs = append(pairs, pair{gid: gid, r: r})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].gid < pairs[j].gid })

	var buf bytes.Buffer
	buf.WriteString("/CIDInit /ProcSet findresource begin\n")
	buf.WriteString("12 dict begin\n")
	buf.WriteString("begincmap\n")
	buf.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	buf.WriteString("/CMapName /Adobe-Identity-UCS def\n")
	buf.WriteString("/CMapType 2 def\n")
	buf.WriteString("1 begincodespacerange <0000> <FFFF> endcodespacerange\n")

	for start := 0; start < len(pairs); start += 100 {
		end := start + 100
		if end > len(pairs) {
			end = len(pairs)
		}
		fmt.Fprintf(&buf, "%d beginbfchar\n", end-start)
		for _, p := range pairs[start:end] {
			fmt.Fprintf(&buf, "<%04X> <%s>\n", p.gid, runeToUTF16BEHex(p.r))
		}
		buf.WriteString("endbfchar\n")
	}
	buf.WriteString("endcmap\n")
	buf.WriteString("CMapName currentdict /CMap defineresource pop\n")
	buf.WriteString("end\nend\n")

	return &pdfStream{
		Dict:    pdfDict{},
		Data:    buf.Bytes(),
		Decoded: true,
	}
}

// embedFont adds all required PDF objects for the embedded TTF to doc.objects
// and returns the object ID of the Type0 font dict.
func embedFont(d *Document, f *ttfFont) int {
	fontFile2ID := d.addObject(buildFontFile2Stream(f))
	descriptor := buildFontDescriptor(f, fontFile2ID)
	descriptorID := d.addObject(descriptor)

	cidDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/CIDFontType2"),
		"/BaseFont": pdfName("/" + f.postScriptName),
		"/CIDSystemInfo": pdfDict{
			"/Registry":   "Adobe",
			"/Ordering":   "Identity",
			"/Supplement": 0,
		},
		"/FontDescriptor": pdfRef{Num: descriptorID},
		"/CIDToGIDMap":    pdfName("/Identity"),
		"/W":              buildWArray(f),
		"/DW":             defaultCIDWidth,
	}
	cidID := d.addObject(cidDict)

	tuID := d.addObject(buildToUnicodeCMap(f))

	type0 := pdfDict{
		"/Type":            pdfName("/Font"),
		"/Subtype":         pdfName("/Type0"),
		"/BaseFont":        pdfName("/" + f.postScriptName),
		"/Encoding":        pdfName("/Identity-H"),
		"/DescendantFonts": pdfArray{pdfRef{Num: cidID}},
		"/ToUnicode":       pdfRef{Num: tuID},
	}
	return d.addObject(type0)
}

// runeToUTF16BEHex renders r as big-endian UTF-16 in uppercase hex, with
// surrogate pairs for supplementary characters (> U+FFFF).
func runeToUTF16BEHex(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("%04X", uint16(r))
	}
	v := uint32(r) - 0x10000
	high := 0xD800 + (v >> 10)
	low := 0xDC00 + (v & 0x3FF)
	return fmt.Sprintf("%04X%04X", high, low)
}
