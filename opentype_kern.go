// SPDX-License-Identifier: MIT

package asposepdf

// The legacy 'kern' table (OpenType version 0) and the two positioning steps
// HarfBuzz takes after GPOS for every script this engine shapes: kerning from
// that table when GPOS offers none for the script, and zero advances for marks
// (pdf-go-u3vk). Tahoma, Verdana and other older faces keep their Latin
// kerning only in 'kern'.

// otKernSubtable is the pairs of one format 0 subtable, keyed left<<16|right.
type otKernSubtable map[uint32]int16

// otParseKern reads the horizontal, along-the-line format 0 subtables of an
// OpenType 'kern' table. Apple's version 1 table, other formats and
// cross-stream subtables are left out.
func otParseKern(b []byte) []otKernSubtable {
	if len(b) < 4 || otU16(b, 0) != 0 {
		return nil
	}
	n := int(otU16(b, 2))
	var out []otKernSubtable
	off := 4
	for k := 0; k < n && off+6 <= len(b); k++ {
		length := int(otU16(b, off+2))
		coverage := otU16(b, off+4)
		next := off + length
		const horizontal, crossStream = 0x01, 0x04
		if coverage>>8 == 0 && off+14 <= len(b) {
			pairs := int(otU16(b, off+6))
			// A large subtable overflows its 16-bit length; the pair count
			// gives its true size.
			if end := off + 14 + 6*pairs; end > next {
				next = end
			}
			if coverage&horizontal != 0 && coverage&crossStream == 0 {
				st := make(otKernSubtable, pairs)
				for p, at := 0, off+14; p < pairs && at+6 <= len(b); p, at = p+1, at+6 {
					if v := otI16(b, at+4); v != 0 {
						st[uint32(otU16(b, at))<<16|uint32(otU16(b, at+2))] = v
					}
				}
				out = append(out, st)
			}
		}
		if length == 0 && next == off {
			break
		}
		off = next
	}
	return out
}

// otHasFeature reports whether the language system the plan for script uses
// lists feature tag — the test by which HarfBuzz decides that GPOS, not the
// 'kern' table, kerns the run.
func otHasFeature(t *otLayoutTable, script, tag string) bool {
	if t == nil {
		return false
	}
	sc := t.scripts[script]
	for _, alt := range []string{"DFLT", "dflt", "latn"} {
		if sc != nil {
			break
		}
		sc = t.scripts[alt]
	}
	if sc == nil || sc.defaultLang == nil {
		return false
	}
	indices := append([]int{}, sc.defaultLang.features...)
	if sc.defaultLang.required >= 0 {
		indices = append(indices, sc.defaultLang.required)
	}
	for _, fi := range indices {
		if fi >= 0 && fi < len(t.features) && t.features[fi].tag == tag {
			return true
		}
	}
	return false
}

// applyKernTable kerns each pair of glyphs adjacent in visual order, marks
// skipped, as HarfBuzz does: half the value (rounded down) widens the first
// glyph, the rest widens and shifts the second.
func (s *otShaper) applyKernTable(subtables []otKernSubtable) {
	n := len(s.glyphs)
	visual := make([]int, n)
	for i := range visual {
		visual[i] = i
		if s.rtl {
			visual[i] = n - 1 - i
		}
	}
	m := otMatcher{s: s, flag: otFlagIgnoreMarks}
	for _, st := range subtables {
		for v := 0; v < n; {
			i := visual[v]
			w := v + 1
			for w < n && m.skip(visual[w]) {
				w++
			}
			if w >= n {
				break
			}
			j := visual[w]
			if kern := int32(st[uint32(s.glyphs[i].gid)<<16|uint32(s.glyphs[j].gid)]); kern != 0 {
				k1 := kern >> 1
				k2 := kern - k1
				s.glyphs[i].advance += k1
				s.glyphs[j].advance += k2
				s.glyphs[j].xOff += k2
			}
			v = w
		}
	}
}

// zeroMarkAdvances gives every mark no advance, as HarfBuzz does once
// positioning is done and before attachments are resolved. A mark is what
// GDEF classes as one; with no glyph classes, a non-spacing mark. Without
// GPOS to place the marks, a left-to-right mark keeps its position by moving
// back by the advance it loses.
func (s *otShaper) zeroMarkAdvances(adjust bool) {
	classes := s.gdef != nil && !s.gdef.glyphClass.empty()
	for i := range s.glyphs {
		g := &s.glyphs[i]
		mark := g.isMark && !g.ignorable
		if classes {
			mark = s.gdef.glyphClass.class(g.gid) == otClassMark
		}
		if !mark {
			continue
		}
		if adjust && !s.rtl {
			g.xOff -= g.advance
		}
		g.advance = 0
	}
}
