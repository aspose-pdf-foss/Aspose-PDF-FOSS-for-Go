package asposepdf

import (
	"encoding/binary"
	"fmt"
)

// ttfFont holds the parsed fields required for PDF embedding and text measurement.
type ttfFont struct {
	data []byte // raw TTF bytes (written verbatim into /FontFile2)

	// From head.
	unitsPerEm uint16
	xMin, yMin int16
	xMax, yMax int16

	// From hhea.
	ascent, descent        int16
	numOfLongHorMetrics    uint16

	// From maxp.
	numGlyphs uint16

	// From hmtx.
	glyphWidths []uint16 // advanceWidth per glyphID (FUnits)

	// From cmap.
	runeToGlyph map[rune]uint16

	// From OS/2.
	capHeight   int16
	weight      uint16
	flagsBold   bool
	flagsItalic bool

	// From post.
	italicAngle  float64
	isFixedPitch bool

	// From name.
	postScriptName string
}

// tableRecord is an entry in the TTF table directory.
type tableRecord struct {
	offset uint32
	length uint32
}

// parseTTF parses a TrueType font file and returns the ttfFont ready for embedding.
// Only the tables required for CIDFontType2 / Type0 embedding are read.
func parseTTF(data []byte) (*ttfFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("parse ttf: file too small (%d bytes)", len(data))
	}
	scaler := binary.BigEndian.Uint32(data[0:4])
	if scaler != 0x00010000 && scaler != 0x74727565 { // 'true'
		return nil, fmt.Errorf("parse ttf: not a TrueType file (scaler 0x%08X)", scaler)
	}

	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return nil, fmt.Errorf("parse ttf: truncated table directory")
	}
	tables := make(map[string]tableRecord, numTables)
	for i := 0; i < numTables; i++ {
		off := 12 + i*16
		tag := string(data[off : off+4])
		tables[tag] = tableRecord{
			offset: binary.BigEndian.Uint32(data[off+8 : off+12]),
			length: binary.BigEndian.Uint32(data[off+12 : off+16]),
		}
	}

	required := []string{"head", "hhea", "hmtx", "maxp", "name", "cmap", "OS/2", "post"}
	for _, tag := range required {
		if _, ok := tables[tag]; !ok {
			return nil, fmt.Errorf("parse ttf: missing required table %q", tag)
		}
	}

	f := &ttfFont{data: data}

	if err := parseHead(f, tables); err != nil {
		return nil, err
	}
	if err := parseHhea(f, tables); err != nil {
		return nil, err
	}
	if err := parseMaxp(f, tables); err != nil {
		return nil, err
	}
	if err := parseHmtx(f, tables); err != nil {
		return nil, err
	}

	return f, nil
}

// tableSlice returns the bytes of the named table or nil if absent.
func tableSlice(data []byte, tables map[string]tableRecord, tag string) []byte {
	t, ok := tables[tag]
	if !ok {
		return nil
	}
	end := t.offset + t.length
	if end > uint32(len(data)) {
		return nil
	}
	return data[t.offset:end]
}

func parseHead(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "head")
	if len(b) < 54 {
		return fmt.Errorf("parse ttf head: too small")
	}
	f.unitsPerEm = binary.BigEndian.Uint16(b[18:20])
	f.xMin = int16(binary.BigEndian.Uint16(b[36:38]))
	f.yMin = int16(binary.BigEndian.Uint16(b[38:40]))
	f.xMax = int16(binary.BigEndian.Uint16(b[40:42]))
	f.yMax = int16(binary.BigEndian.Uint16(b[42:44]))
	return nil
}

func parseHhea(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hhea")
	if len(b) < 36 {
		return fmt.Errorf("parse ttf hhea: too small")
	}
	f.ascent = int16(binary.BigEndian.Uint16(b[4:6]))
	f.descent = int16(binary.BigEndian.Uint16(b[6:8]))
	f.numOfLongHorMetrics = binary.BigEndian.Uint16(b[34:36])
	return nil
}

func parseMaxp(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "maxp")
	if len(b) < 6 {
		return fmt.Errorf("parse ttf maxp: too small")
	}
	f.numGlyphs = binary.BigEndian.Uint16(b[4:6])
	return nil
}

func parseHmtx(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hmtx")
	if f.numGlyphs == 0 {
		return fmt.Errorf("parse ttf hmtx: numGlyphs is zero")
	}
	if f.numOfLongHorMetrics == 0 {
		return fmt.Errorf("parse ttf hmtx: numOfLongHorMetrics is zero")
	}
	if f.numOfLongHorMetrics > f.numGlyphs {
		return fmt.Errorf("parse ttf hmtx: numOfLongHorMetrics (%d) exceeds numGlyphs (%d)", f.numOfLongHorMetrics, f.numGlyphs)
	}
	// The hmtx table has numOfLongHorMetrics 4-byte records (advanceWidth uint16, lsb int16),
	// followed by (numGlyphs - numOfLongHorMetrics) 2-byte records (lsb only); the missing
	// advanceWidth inherits the advanceWidth of the last long record.
	if len(b) < int(f.numOfLongHorMetrics)*4 {
		return fmt.Errorf("parse ttf hmtx: too small")
	}
	widths := make([]uint16, f.numGlyphs)
	var lastAdvance uint16
	for i := uint16(0); i < f.numOfLongHorMetrics; i++ {
		off := int(i) * 4
		w := binary.BigEndian.Uint16(b[off : off+2])
		widths[i] = w
		lastAdvance = w
	}
	for i := f.numOfLongHorMetrics; i < f.numGlyphs; i++ {
		widths[i] = lastAdvance
	}
	f.glyphWidths = widths
	return nil
}
