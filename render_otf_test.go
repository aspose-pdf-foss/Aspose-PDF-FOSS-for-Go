// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"math"
	"sort"
	"testing"
)

// buildSFNT assembles a minimal sfnt (table directory + tables) with the given
// scaler tag. Offsets are computed; checksums and search params are left zero
// (the parser ignores them).
func buildSFNT(scaler uint32, tbls map[string][]byte) []byte {
	tags := make([]string, 0, len(tbls))
	for t := range tbls {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	header := make([]byte, 12)
	binary.BigEndian.PutUint32(header[0:4], scaler)
	binary.BigEndian.PutUint16(header[4:6], uint16(n))
	dir := make([]byte, n*16)
	dataOff := 12 + n*16
	var data []byte
	for i, t := range tags {
		b := tbls[t]
		rec := i * 16
		copy(dir[rec:rec+4], t)
		binary.BigEndian.PutUint32(dir[rec+8:rec+12], uint32(dataOff+len(data)))
		binary.BigEndian.PutUint32(dir[rec+12:rec+16], uint32(len(b)))
		data = append(data, b...)
	}
	return append(append(header, dir...), data...)
}

// TestParseOTF wraps the hand-built CFF in an OpenType ('OTTO') sfnt and checks
// parseTTF attaches the CFF and draws glyphs through it — the path a registered
// .otf font takes in the FontRepository.
func TestParseOTF(t *testing.T) {
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 3) // numOfLongHorMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint16(maxp[4:6], 3) // numGlyphs
	hmtx := make([]byte, 12)
	for i := 0; i < 3; i++ {
		binary.BigEndian.PutUint16(hmtx[i*4:i*4+2], 500)
	}
	otf := buildSFNT(0x4F54544F, map[string][]byte{
		"head": head, "hhea": hhea, "maxp": maxp, "hmtx": hmtx, "CFF ": buildMinimalCFF(),
	})

	f, err := parseTTF(otf)
	if err != nil {
		t.Fatalf("parseTTF(OTTO): %v", err)
	}
	if f.cff == nil {
		t.Fatal("CFF table not attached for an OTTO sfnt")
	}
	contours := f.glyphContours(1) // square, drawn via the attached CFF
	if len(contours) != 1 {
		t.Fatalf("glyphContours(1) = %d, want 1", len(contours))
	}
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range contours[0] {
		minx, miny = math.Min(minx, p.x), math.Min(miny, p.y)
		maxx, maxy = math.Max(maxx, p.x), math.Max(maxy, p.y)
	}
	if math.Abs(minx-10) > 0.5 || math.Abs(maxx-90) > 0.5 {
		t.Errorf("CFF-via-OTTO bbox x = [%.1f %.1f], want ~[10 90]", minx, maxx)
	}
	if math.Abs(miny-10) > 0.5 || math.Abs(maxy-90) > 0.5 {
		t.Errorf("CFF-via-OTTO bbox y = [%.1f %.1f], want ~[10 90]", miny, maxy)
	}
}
