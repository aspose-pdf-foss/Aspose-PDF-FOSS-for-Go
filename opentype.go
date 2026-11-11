// SPDX-License-Identifier: MIT

package asposepdf

import "encoding/binary"

// OpenType Layout tables — GDEF, GSUB and GPOS (RTL support, phase 3; epic
// pdf-go-26u4). This file is the reader; shape.go applies what it reads.
//
// Everything here is defensive: an offset that leaves the table, a format
// number the spec does not define, a truncated array — each drops the
// subtable it belongs to instead of failing the font. A font whose layout
// tables are damaged still embeds and still draws; it just shapes less.
//
// Not read: Device and VariationIndex tables (we never write a variable
// font, so the deltas have nothing to instance against), the ligature caret
// list, and the feature-variations table of GSUB/GPOS 1.1.

// otLayout is a font's parsed layout tables. It hangs off ttfFont, built on
// the first shaping request and cached (nil when the font has neither GSUB
// nor GPOS, which is the common case for Latin-only subsets).
type otLayout struct {
	gdef *otGDEF
	gsub *otLayoutTable
	gpos *otLayoutTable
}

// otLayoutTable is the common shape of GSUB and GPOS: a script list
// selecting features, a feature list selecting lookups, and the lookups.
type otLayoutTable struct {
	scripts  map[string]*otScript
	features []otFeature
	lookups  []*otLookup
}

type otScript struct {
	defaultLang *otLangSys
	langs       map[string]*otLangSys
}

type otLangSys struct {
	required int // feature index, or -1
	features []int
}

type otFeature struct {
	tag     string
	lookups []int
}

// otLookup is one lookup: a type, the flags that decide which glyphs it
// skips, and its subtables (already resolved through any extension
// indirection, so kind is the real lookup type).
type otLookup struct {
	kind       uint16
	flag       uint16
	markFilter uint16 // mark filtering set index, when flag has otFlagUseMarkFilteringSet
	subtables  []any
}

// LookupFlag bits (OpenType 1.9 §5.2.3).
const (
	otFlagRightToLeft          = 0x0001
	otFlagIgnoreBaseGlyphs     = 0x0002
	otFlagIgnoreLigatures      = 0x0004
	otFlagIgnoreMarks          = 0x0008
	otFlagUseMarkFilteringSet  = 0x0010
	otFlagMarkAttachTypeMask   = 0xFF00
	otFlagMarkAttachTypeShift  = 8
	otMaxNestedLookupRecursion = 6
)

// GDEF glyph classes (§5.1.3).
const (
	otClassBase     = 1
	otClassLigature = 2
	otClassMark     = 3
)

type otGDEF struct {
	glyphClass     otClassDef
	markAttachment otClassDef
	markSets       []otCoverage
}

// --- primitive readers -------------------------------------------------

func otU16(b []byte, off int) uint16 {
	if off < 0 || off+2 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint16(b[off:])
}

func otI16(b []byte, off int) int16 { return int16(otU16(b, off)) }

func otU32(b []byte, off int) uint32 {
	if off < 0 || off+4 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint32(b[off:])
}

func otTag(b []byte, off int) string {
	if off < 0 || off+4 > len(b) {
		return ""
	}
	return string(b[off : off+4])
}

// otSub returns the sub-slice starting at off, or nil when off is out of
// range — so every "read a subtable at this offset" site is one check.
func otSub(b []byte, off int) []byte {
	if off <= 0 || off >= len(b) {
		return nil
	}
	return b[off:]
}

// --- coverage and class definition -------------------------------------

// otCoverage maps a glyph to its coverage index, or reports absence. Format
// 1 keeps the sorted glyph list; format 2 keeps ranges. Both are searched
// by bisection, which is what makes lookup application cheap.
type otCoverage struct {
	glyphs []uint16 // format 1
	ranges []otCovRange
}

type otCovRange struct {
	start, end, index uint16
}

// index returns the coverage index of g, or -1.
func (c otCoverage) index(g uint16) int {
	if len(c.glyphs) > 0 {
		lo, hi := 0, len(c.glyphs)-1
		for lo <= hi {
			mid := (lo + hi) / 2
			switch {
			case c.glyphs[mid] < g:
				lo = mid + 1
			case c.glyphs[mid] > g:
				hi = mid - 1
			default:
				return mid
			}
		}
		return -1
	}
	lo, hi := 0, len(c.ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		r := c.ranges[mid]
		switch {
		case g < r.start:
			hi = mid - 1
		case g > r.end:
			lo = mid + 1
		default:
			return int(r.index) + int(g-r.start)
		}
	}
	return -1
}

func (c otCoverage) covers(g uint16) bool { return c.index(g) >= 0 }

func otParseCoverage(b []byte) (otCoverage, bool) {
	if len(b) < 4 {
		return otCoverage{}, false
	}
	switch otU16(b, 0) {
	case 1:
		n := int(otU16(b, 2))
		if 4+n*2 > len(b) {
			return otCoverage{}, false
		}
		gs := make([]uint16, n)
		for i := 0; i < n; i++ {
			gs[i] = otU16(b, 4+i*2)
		}
		return otCoverage{glyphs: gs}, true
	case 2:
		n := int(otU16(b, 2))
		if 4+n*6 > len(b) {
			return otCoverage{}, false
		}
		rs := make([]otCovRange, n)
		for i := 0; i < n; i++ {
			o := 4 + i*6
			rs[i] = otCovRange{otU16(b, o), otU16(b, o+2), otU16(b, o+4)}
		}
		return otCoverage{ranges: rs}, true
	}
	return otCoverage{}, false
}

// otClassDef assigns each glyph a class number; glyphs not listed are
// class 0.
type otClassDef struct {
	start  uint16   // format 1
	values []uint16 // format 1
	ranges []otClassRange
}

type otClassRange struct {
	start, end, class uint16
}

func (c otClassDef) empty() bool { return len(c.values) == 0 && len(c.ranges) == 0 }

func (c otClassDef) class(g uint16) uint16 {
	if len(c.values) > 0 {
		if g >= c.start && int(g-c.start) < len(c.values) {
			return c.values[g-c.start]
		}
		return 0
	}
	lo, hi := 0, len(c.ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		r := c.ranges[mid]
		switch {
		case g < r.start:
			hi = mid - 1
		case g > r.end:
			lo = mid + 1
		default:
			return r.class
		}
	}
	return 0
}

func otParseClassDef(b []byte) otClassDef {
	if len(b) < 4 {
		return otClassDef{}
	}
	switch otU16(b, 0) {
	case 1:
		start := otU16(b, 2)
		n := int(otU16(b, 4))
		if 6+n*2 > len(b) {
			return otClassDef{}
		}
		vs := make([]uint16, n)
		for i := 0; i < n; i++ {
			vs[i] = otU16(b, 6+i*2)
		}
		return otClassDef{start: start, values: vs}
	case 2:
		n := int(otU16(b, 2))
		if 4+n*6 > len(b) {
			return otClassDef{}
		}
		rs := make([]otClassRange, n)
		for i := 0; i < n; i++ {
			o := 4 + i*6
			rs[i] = otClassRange{otU16(b, o), otU16(b, o+2), otU16(b, o+4)}
		}
		return otClassDef{ranges: rs}
	}
	return otClassDef{}
}

// --- value records and anchors -----------------------------------------

// otValueRecord is a GPOS adjustment in font units. Device-table offsets in
// the source record are skipped, not applied.
type otValueRecord struct {
	xPlacement, yPlacement, xAdvance, yAdvance int16
}

func (v otValueRecord) zero() bool { return v == otValueRecord{} }

// otValueRecordSize is the byte length a record with the given format takes
// — one int16 per set bit, device offsets included.
func otValueRecordSize(format uint16) int {
	n := 0
	for i := 0; i < 8; i++ {
		if format&(1<<uint(i)) != 0 {
			n += 2
		}
	}
	return n
}

// otParseValueRecord reads a record of the given format at off.
func otParseValueRecord(b []byte, off int, format uint16) otValueRecord {
	var v otValueRecord
	p := off
	read := func(bit uint16, dst *int16) {
		if format&bit != 0 {
			*dst = otI16(b, p)
			p += 2
		}
	}
	read(0x0001, &v.xPlacement)
	read(0x0002, &v.yPlacement)
	read(0x0004, &v.xAdvance)
	read(0x0008, &v.yAdvance)
	return v
}

// otAnchor is an attachment point in font units. exists distinguishes "at
// the origin" from "no anchor here" (a NULL offset in the source).
type otAnchor struct {
	x, y   int16
	exists bool
}

func otParseAnchor(b []byte, off int) otAnchor {
	s := otSub(b, off)
	if len(s) < 6 {
		return otAnchor{}
	}
	switch otU16(s, 0) {
	case 1, 2, 3: // format 2 adds a contour point, 3 device tables — both ignorable
		return otAnchor{x: otI16(s, 2), y: otI16(s, 4), exists: true}
	}
	return otAnchor{}
}

// --- GSUB subtables ----------------------------------------------------

type gsubSingle struct {
	cov    otCoverage
	delta  int16    // format 1
	substs []uint16 // format 2
}

type gsubMultiple struct {
	cov  otCoverage
	seqs [][]uint16
}

type gsubAlternate struct {
	cov  otCoverage
	alts [][]uint16
}

type otLigature struct {
	glyph      uint16
	components []uint16 // second glyph onward
}

type gsubLigature struct {
	cov  otCoverage
	sets [][]otLigature
}

// gsubReverseChain is GSUB type 8: one-to-one substitution applied in
// reverse order with backtrack and lookahead context.
type gsubReverseChain struct {
	cov        otCoverage
	backtrack  []otCoverage
	lookahead  []otCoverage
	substitute []uint16
}

// --- GPOS subtables ----------------------------------------------------

type gposSingle struct {
	cov    otCoverage
	value  otValueRecord   // format 1: one value for the whole coverage
	values []otValueRecord // format 2
}

type otPairValue struct {
	second    uint16
	value1    otValueRecord
	value2    otValueRecord
	hasValue2 bool
}

type gposPair1 struct {
	cov  otCoverage
	sets [][]otPairValue
}

type gposPair2 struct {
	cov          otCoverage
	class1       otClassDef
	class2       otClassDef
	n1, n2       int
	values       []otValueRecord // n1*n2 first-glyph adjustments
	values2      []otValueRecord // n1*n2 second-glyph adjustments
	hasSecondVal bool
}

type gposCursive struct {
	cov   otCoverage
	entry []otAnchor
	exit  []otAnchor
}

// otMarkRecord is a mark glyph's class plus its attachment anchor.
type otMarkRecord struct {
	class  uint16
	anchor otAnchor
}

type gposMarkBase struct {
	markCov, baseCov otCoverage
	marks            []otMarkRecord
	classCount       int
	baseAnchors      [][]otAnchor // [baseIndex][markClass]
}

type gposMarkLig struct {
	markCov, ligCov otCoverage
	marks           []otMarkRecord
	classCount      int
	ligAnchors      [][][]otAnchor // [ligIndex][component][markClass]
}

type gposMarkMark struct {
	mark1Cov, mark2Cov otCoverage
	marks              []otMarkRecord
	classCount         int
	mark2Anchors       [][]otAnchor // [mark2Index][markClass]
}

// --- shared context subtables (GSUB 5/6, GPOS 7/8) ---------------------

type otSeqLookup struct {
	seqIndex, lookupIndex uint16
}

// otSeqRule is one glyph-sequence rule; input excludes the first glyph,
// which the subtable's coverage already matched.
type otSeqRule struct {
	input   []uint16
	lookups []otSeqLookup
}

type otContext1 struct {
	cov   otCoverage
	rules [][]otSeqRule
}

type otContext2 struct {
	cov     otCoverage
	classes otClassDef
	rules   [][]otSeqRule // input holds class values
}

type otContext3 struct {
	covs    []otCoverage
	lookups []otSeqLookup
}

type otChainRule struct {
	backtrack []uint16
	input     []uint16 // excludes the first glyph
	lookahead []uint16
	lookups   []otSeqLookup
}

type otChain1 struct {
	cov   otCoverage
	rules [][]otChainRule
}

type otChain2 struct {
	cov                        otCoverage
	backClass, inClass, aClass otClassDef
	rules                      [][]otChainRule // values are class numbers
}

type otChain3 struct {
	back, input, ahead []otCoverage
	lookups            []otSeqLookup
}

// --- table parsing -----------------------------------------------------

// parseOTLayout reads the font's layout tables. It returns nil when the
// font carries neither GSUB nor GPOS.
func parseOTLayout(f *ttfFont) *otLayout {
	gsub := otParseLayoutTable(tableSlice(f.data, f.tables, "GSUB"), true)
	gpos := otParseLayoutTable(tableSlice(f.data, f.tables, "GPOS"), false)
	if gsub == nil && gpos == nil {
		return nil
	}
	return &otLayout{
		gdef: otParseGDEF(tableSlice(f.data, f.tables, "GDEF")),
		gsub: gsub,
		gpos: gpos,
	}
}

func otParseGDEF(b []byte) *otGDEF {
	if len(b) < 12 {
		return nil
	}
	g := &otGDEF{}
	if off := int(otU16(b, 4)); off > 0 {
		g.glyphClass = otParseClassDef(otSub(b, off))
	}
	if off := int(otU16(b, 10)); off > 0 {
		g.markAttachment = otParseClassDef(otSub(b, off))
	}
	// Mark glyph sets arrived in GDEF 1.2.
	if otU16(b, 0) == 1 && otU16(b, 2) >= 2 && len(b) >= 14 {
		if off := int(otU16(b, 12)); off > 0 {
			if s := otSub(b, off); len(s) >= 4 {
				n := int(otU16(s, 2))
				for i := 0; i < n; i++ {
					co := int(otU32(s, 4+i*4))
					if cov, ok := otParseCoverage(otSub(s, co)); ok {
						g.markSets = append(g.markSets, cov)
					} else {
						g.markSets = append(g.markSets, otCoverage{})
					}
				}
			}
		}
	}
	return g
}

func otParseLayoutTable(b []byte, isGSUB bool) *otLayoutTable {
	if len(b) < 10 {
		return nil
	}
	t := &otLayoutTable{scripts: map[string]*otScript{}}
	otParseScriptList(t, otSub(b, int(otU16(b, 4))))
	otParseFeatureList(t, otSub(b, int(otU16(b, 6))))
	otParseLookupList(t, otSub(b, int(otU16(b, 8))), isGSUB)
	if len(t.lookups) == 0 {
		return nil
	}
	return t
}

func otParseScriptList(t *otLayoutTable, b []byte) {
	if len(b) < 2 {
		return
	}
	n := int(otU16(b, 0))
	for i := 0; i < n; i++ {
		rec := 2 + i*6
		if rec+6 > len(b) {
			return
		}
		tag := otTag(b, rec)
		sb := otSub(b, int(otU16(b, rec+4)))
		if len(sb) < 4 {
			continue
		}
		sc := &otScript{langs: map[string]*otLangSys{}}
		if off := int(otU16(sb, 0)); off > 0 {
			sc.defaultLang = otParseLangSys(otSub(sb, off))
		}
		ln := int(otU16(sb, 2))
		for j := 0; j < ln; j++ {
			lrec := 4 + j*6
			if lrec+6 > len(sb) {
				break
			}
			if ls := otParseLangSys(otSub(sb, int(otU16(sb, lrec+4)))); ls != nil {
				sc.langs[otTag(sb, lrec)] = ls
			}
		}
		t.scripts[tag] = sc
	}
}

func otParseLangSys(b []byte) *otLangSys {
	if len(b) < 6 {
		return nil
	}
	ls := &otLangSys{required: -1}
	if r := otU16(b, 2); r != 0xFFFF {
		ls.required = int(r)
	}
	n := int(otU16(b, 4))
	if 6+n*2 > len(b) {
		n = (len(b) - 6) / 2
	}
	for i := 0; i < n; i++ {
		ls.features = append(ls.features, int(otU16(b, 6+i*2)))
	}
	return ls
}

func otParseFeatureList(t *otLayoutTable, b []byte) {
	if len(b) < 2 {
		return
	}
	n := int(otU16(b, 0))
	for i := 0; i < n; i++ {
		rec := 2 + i*6
		if rec+6 > len(b) {
			return
		}
		f := otFeature{tag: otTag(b, rec)}
		fb := otSub(b, int(otU16(b, rec+4)))
		if len(fb) >= 4 {
			ln := int(otU16(fb, 2))
			if 4+ln*2 > len(fb) {
				ln = (len(fb) - 4) / 2
			}
			for j := 0; j < ln; j++ {
				f.lookups = append(f.lookups, int(otU16(fb, 4+j*2)))
			}
		}
		t.features = append(t.features, f)
	}
}

func otParseLookupList(t *otLayoutTable, b []byte, isGSUB bool) {
	if len(b) < 2 {
		return
	}
	n := int(otU16(b, 0))
	for i := 0; i < n; i++ {
		if 2+i*2+2 > len(b) {
			return
		}
		lb := otSub(b, int(otU16(b, 2+i*2)))
		t.lookups = append(t.lookups, otParseLookup(lb, isGSUB))
	}
}

func otParseLookup(b []byte, isGSUB bool) *otLookup {
	if len(b) < 6 {
		return &otLookup{}
	}
	l := &otLookup{kind: otU16(b, 0), flag: otU16(b, 2)}
	n := int(otU16(b, 4))
	if 6+n*2 > len(b) {
		n = (len(b) - 6) / 2
	}
	if l.flag&otFlagUseMarkFilteringSet != 0 && 6+n*2+2 <= len(b) {
		l.markFilter = otU16(b, 6+n*2)
	}
	declared := l.kind // l.kind becomes the extended type below
	for i := 0; i < n; i++ {
		sb := otSub(b, int(otU16(b, 6+i*2)))
		kind := declared
		// Extension (GSUB 7 / GPOS 9) is one level of indirection to a
		// subtable of another type, used when the table outgrew 16-bit
		// offsets. Resolve it here so the engine never sees it. Every
		// subtable of an extension lookup is an extension: testing l.kind,
		// which the first one changes, read the others' extension headers
		// as subtables of the extended type.
		if (isGSUB && declared == 7) || (!isGSUB && declared == 9) {
			if len(sb) < 8 || otU16(sb, 0) != 1 {
				continue
			}
			kind = otU16(sb, 2)
			sb = otSub(sb, int(otU32(sb, 4)))
			l.kind = kind
		}
		var st any
		if isGSUB {
			st = otParseGSUBSubtable(kind, sb)
		} else {
			st = otParseGPOSSubtable(kind, sb)
		}
		if st != nil {
			l.subtables = append(l.subtables, st)
		}
	}
	return l
}

func otParseGSUBSubtable(kind uint16, b []byte) any {
	if len(b) < 4 {
		return nil
	}
	format := otU16(b, 0)
	cov, covOK := otParseCoverage(otSub(b, int(otU16(b, 2))))

	switch kind {
	case 1: // single substitution
		if !covOK {
			return nil
		}
		switch format {
		case 1:
			return &gsubSingle{cov: cov, delta: otI16(b, 4)}
		case 2:
			n := int(otU16(b, 4))
			if 6+n*2 > len(b) {
				return nil
			}
			s := make([]uint16, n)
			for i := 0; i < n; i++ {
				s[i] = otU16(b, 6+i*2)
			}
			return &gsubSingle{cov: cov, substs: s}
		}

	case 2, 3: // multiple / alternate: identical layout, different meaning
		if !covOK || format != 1 {
			return nil
		}
		n := int(otU16(b, 4))
		seqs := make([][]uint16, 0, n)
		for i := 0; i < n; i++ {
			s := otSub(b, int(otU16(b, 6+i*2)))
			if len(s) < 2 {
				seqs = append(seqs, nil)
				continue
			}
			cnt := int(otU16(s, 0))
			if 2+cnt*2 > len(s) {
				seqs = append(seqs, nil)
				continue
			}
			g := make([]uint16, cnt)
			for j := 0; j < cnt; j++ {
				g[j] = otU16(s, 2+j*2)
			}
			seqs = append(seqs, g)
		}
		if kind == 2 {
			return &gsubMultiple{cov: cov, seqs: seqs}
		}
		return &gsubAlternate{cov: cov, alts: seqs}

	case 4: // ligature
		if !covOK || format != 1 {
			return nil
		}
		n := int(otU16(b, 4))
		sets := make([][]otLigature, 0, n)
		for i := 0; i < n; i++ {
			ls := otSub(b, int(otU16(b, 6+i*2)))
			var set []otLigature
			if len(ls) >= 2 {
				cnt := int(otU16(ls, 0))
				for j := 0; j < cnt; j++ {
					lg := otSub(ls, int(otU16(ls, 2+j*2)))
					if len(lg) < 4 {
						continue
					}
					compCount := int(otU16(lg, 2))
					if compCount < 1 || 4+(compCount-1)*2 > len(lg) {
						continue
					}
					comps := make([]uint16, compCount-1)
					for k := 0; k < compCount-1; k++ {
						comps[k] = otU16(lg, 4+k*2)
					}
					set = append(set, otLigature{glyph: otU16(lg, 0), components: comps})
				}
			}
			sets = append(sets, set)
		}
		return &gsubLigature{cov: cov, sets: sets}

	case 5:
		return otParseContext(b, format, cov, covOK)

	case 6:
		return otParseChainContext(b, format, cov, covOK)

	case 8: // reverse chaining single substitution
		if !covOK || format != 1 {
			return nil
		}
		p := 4
		back, np, ok := otParseCoverageArray(b, p)
		if !ok {
			return nil
		}
		ahead, np2, ok := otParseCoverageArray(b, np)
		if !ok {
			return nil
		}
		p = np2
		n := int(otU16(b, p))
		if p+2+n*2 > len(b) {
			return nil
		}
		subs := make([]uint16, n)
		for i := 0; i < n; i++ {
			subs[i] = otU16(b, p+2+i*2)
		}
		return &gsubReverseChain{cov: cov, backtrack: back, lookahead: ahead, substitute: subs}
	}
	return nil
}

// otParseCoverageArray reads a count-prefixed array of coverage offsets at
// off, returning the coverages and the offset just past the array.
func otParseCoverageArray(b []byte, off int) ([]otCoverage, int, bool) {
	if off+2 > len(b) {
		return nil, 0, false
	}
	n := int(otU16(b, off))
	if off+2+n*2 > len(b) {
		return nil, 0, false
	}
	covs := make([]otCoverage, n)
	for i := 0; i < n; i++ {
		c, ok := otParseCoverage(otSub(b, int(otU16(b, off+2+i*2))))
		if !ok {
			return nil, 0, false
		}
		covs[i] = c
	}
	return covs, off + 2 + n*2, true
}

func otParseSeqLookups(b []byte, off, count int) []otSeqLookup {
	if count <= 0 || off+count*4 > len(b) {
		return nil
	}
	out := make([]otSeqLookup, count)
	for i := 0; i < count; i++ {
		out[i] = otSeqLookup{otU16(b, off+i*4), otU16(b, off+i*4+2)}
	}
	return out
}

// otParseContext reads GSUB type 5 / GPOS type 7 (context) subtables.
func otParseContext(b []byte, format uint16, cov otCoverage, covOK bool) any {
	switch format {
	case 1, 2:
		if !covOK {
			return nil
		}
		var classes otClassDef
		setOff := 6
		if format == 2 {
			classes = otParseClassDef(otSub(b, int(otU16(b, 4))))
			setOff = 8
		}
		n := int(otU16(b, setOff-2))
		rules := make([][]otSeqRule, 0, n)
		for i := 0; i < n; i++ {
			rs := otSub(b, int(otU16(b, setOff+i*2)))
			var set []otSeqRule
			if len(rs) >= 2 {
				cnt := int(otU16(rs, 0))
				for j := 0; j < cnt; j++ {
					r := otSub(rs, int(otU16(rs, 2+j*2)))
					if len(r) < 4 {
						continue
					}
					glyphCount := int(otU16(r, 0))
					lookupCount := int(otU16(r, 2))
					if glyphCount < 1 || 4+(glyphCount-1)*2 > len(r) {
						continue
					}
					in := make([]uint16, glyphCount-1)
					for k := 0; k < glyphCount-1; k++ {
						in[k] = otU16(r, 4+k*2)
					}
					set = append(set, otSeqRule{
						input:   in,
						lookups: otParseSeqLookups(r, 4+(glyphCount-1)*2, lookupCount),
					})
				}
			}
			rules = append(rules, set)
		}
		if format == 1 {
			return &otContext1{cov: cov, rules: rules}
		}
		return &otContext2{cov: cov, classes: classes, rules: rules}

	case 3:
		glyphCount := int(otU16(b, 2))
		lookupCount := int(otU16(b, 4))
		if glyphCount < 1 || 6+glyphCount*2 > len(b) {
			return nil
		}
		covs := make([]otCoverage, glyphCount)
		for i := 0; i < glyphCount; i++ {
			c, ok := otParseCoverage(otSub(b, int(otU16(b, 6+i*2))))
			if !ok {
				return nil
			}
			covs[i] = c
		}
		return &otContext3{covs: covs, lookups: otParseSeqLookups(b, 6+glyphCount*2, lookupCount)}
	}
	return nil
}

// otParseChainContext reads GSUB type 6 / GPOS type 8 subtables.
func otParseChainContext(b []byte, format uint16, cov otCoverage, covOK bool) any {
	switch format {
	case 1, 2:
		if !covOK {
			return nil
		}
		var backC, inC, aheadC otClassDef
		setOff := 6
		if format == 2 {
			backC = otParseClassDef(otSub(b, int(otU16(b, 4))))
			inC = otParseClassDef(otSub(b, int(otU16(b, 6))))
			aheadC = otParseClassDef(otSub(b, int(otU16(b, 8))))
			setOff = 12
		}
		n := int(otU16(b, setOff-2))
		rules := make([][]otChainRule, 0, n)
		for i := 0; i < n; i++ {
			rs := otSub(b, int(otU16(b, setOff+i*2)))
			var set []otChainRule
			if len(rs) >= 2 {
				cnt := int(otU16(rs, 0))
				for j := 0; j < cnt; j++ {
					if r := otParseChainRule(otSub(rs, int(otU16(rs, 2+j*2)))); r != nil {
						set = append(set, *r)
					}
				}
			}
			rules = append(rules, set)
		}
		if format == 1 {
			return &otChain1{cov: cov, rules: rules}
		}
		return &otChain2{cov: cov, backClass: backC, inClass: inC, aClass: aheadC, rules: rules}

	case 3:
		back, p, ok := otParseCoverageArray(b, 2)
		if !ok {
			return nil
		}
		input, p2, ok := otParseCoverageArray(b, p)
		if !ok || len(input) == 0 {
			return nil
		}
		ahead, p3, ok := otParseCoverageArray(b, p2)
		if !ok {
			return nil
		}
		if p3+2 > len(b) {
			return nil
		}
		return &otChain3{
			back: back, input: input, ahead: ahead,
			lookups: otParseSeqLookups(b, p3+2, int(otU16(b, p3))),
		}
	}
	return nil
}

// otParseChainRule reads one ChainSubRule / ChainSubClassRule — the two
// have identical layout, glyph ids in one and class values in the other.
// The input sequence is the odd one out: its count includes the first
// glyph, which the rule set's coverage (or class) already matched, so only
// count-1 values follow.
func otParseChainRule(b []byte) *otChainRule {
	if len(b) < 8 {
		return nil
	}
	read := func(off, drop int) ([]uint16, int, bool) {
		if off+2 > len(b) {
			return nil, 0, false
		}
		n := int(otU16(b, off)) - drop
		if n < 0 || off+2+n*2 > len(b) {
			return nil, 0, false
		}
		out := make([]uint16, n)
		for i := 0; i < n; i++ {
			out[i] = otU16(b, off+2+i*2)
		}
		return out, off + 2 + n*2, true
	}
	back, p, ok := read(0, 0)
	if !ok {
		return nil
	}
	input, p2, ok := read(p, 1)
	if !ok {
		return nil
	}
	ahead, p3, ok := read(p2, 0)
	if !ok || p3+2 > len(b) {
		return nil
	}
	return &otChainRule{
		backtrack: back,
		input:     input,
		lookahead: ahead,
		lookups:   otParseSeqLookups(b, p3+2, int(otU16(b, p3))),
	}
}

func otParseGPOSSubtable(kind uint16, b []byte) any {
	if len(b) < 4 {
		return nil
	}
	format := otU16(b, 0)
	cov, covOK := otParseCoverage(otSub(b, int(otU16(b, 2))))

	switch kind {
	case 1: // single adjustment
		if !covOK {
			return nil
		}
		vf := otU16(b, 4)
		size := otValueRecordSize(vf)
		switch format {
		case 1:
			return &gposSingle{cov: cov, value: otParseValueRecord(b, 6, vf)}
		case 2:
			n := int(otU16(b, 6))
			if 8+n*size > len(b) {
				return nil
			}
			vs := make([]otValueRecord, n)
			for i := 0; i < n; i++ {
				vs[i] = otParseValueRecord(b, 8+i*size, vf)
			}
			return &gposSingle{cov: cov, values: vs}
		}

	case 2: // pair adjustment
		if !covOK {
			return nil
		}
		vf1, vf2 := otU16(b, 4), otU16(b, 6)
		s1, s2 := otValueRecordSize(vf1), otValueRecordSize(vf2)
		switch format {
		case 1:
			n := int(otU16(b, 8))
			sets := make([][]otPairValue, 0, n)
			for i := 0; i < n; i++ {
				ps := otSub(b, int(otU16(b, 10+i*2)))
				var set []otPairValue
				if len(ps) >= 2 {
					cnt := int(otU16(ps, 0))
					rec := 2 + s1 + s2
					for j := 0; j < cnt; j++ {
						o := 2 + j*rec
						if o+rec > len(ps) {
							break
						}
						set = append(set, otPairValue{
							second:    otU16(ps, o),
							value1:    otParseValueRecord(ps, o+2, vf1),
							value2:    otParseValueRecord(ps, o+2+s1, vf2),
							hasValue2: vf2 != 0,
						})
					}
				}
				sets = append(sets, set)
			}
			return &gposPair1{cov: cov, sets: sets}
		case 2:
			cd1 := otParseClassDef(otSub(b, int(otU16(b, 8))))
			cd2 := otParseClassDef(otSub(b, int(otU16(b, 10))))
			n1, n2 := int(otU16(b, 12)), int(otU16(b, 14))
			if n1 <= 0 || n2 <= 0 || 16+n1*n2*(s1+s2) > len(b) {
				return nil
			}
			v1 := make([]otValueRecord, n1*n2)
			v2 := make([]otValueRecord, n1*n2)
			for i := 0; i < n1*n2; i++ {
				o := 16 + i*(s1+s2)
				v1[i] = otParseValueRecord(b, o, vf1)
				v2[i] = otParseValueRecord(b, o+s1, vf2)
			}
			return &gposPair2{
				cov: cov, class1: cd1, class2: cd2, n1: n1, n2: n2,
				values: v1, values2: v2, hasSecondVal: vf2 != 0,
			}
		}

	case 3: // cursive attachment
		if !covOK || format != 1 {
			return nil
		}
		n := int(otU16(b, 4))
		if 6+n*4 > len(b) {
			return nil
		}
		entry := make([]otAnchor, n)
		exit := make([]otAnchor, n)
		for i := 0; i < n; i++ {
			entry[i] = otParseAnchor(b, int(otU16(b, 6+i*4)))
			exit[i] = otParseAnchor(b, int(otU16(b, 6+i*4+2)))
		}
		return &gposCursive{cov: cov, entry: entry, exit: exit}

	case 4, 6: // mark-to-base, mark-to-mark (identical layout)
		if !covOK || format != 1 {
			return nil
		}
		secondCov, ok := otParseCoverage(otSub(b, int(otU16(b, 4))))
		if !ok {
			return nil
		}
		classCount := int(otU16(b, 6))
		marks := otParseMarkArray(otSub(b, int(otU16(b, 8))))
		anchors := otParseAnchorMatrix(otSub(b, int(otU16(b, 10))), classCount)
		if marks == nil || anchors == nil {
			return nil
		}
		if kind == 4 {
			return &gposMarkBase{markCov: cov, baseCov: secondCov, marks: marks,
				classCount: classCount, baseAnchors: anchors}
		}
		return &gposMarkMark{mark1Cov: cov, mark2Cov: secondCov, marks: marks,
			classCount: classCount, mark2Anchors: anchors}

	case 5: // mark-to-ligature
		if !covOK || format != 1 {
			return nil
		}
		ligCov, ok := otParseCoverage(otSub(b, int(otU16(b, 4))))
		if !ok {
			return nil
		}
		classCount := int(otU16(b, 6))
		marks := otParseMarkArray(otSub(b, int(otU16(b, 8))))
		if marks == nil {
			return nil
		}
		la := otSub(b, int(otU16(b, 10)))
		if len(la) < 2 {
			return nil
		}
		n := int(otU16(la, 0))
		ligs := make([][][]otAnchor, 0, n)
		for i := 0; i < n; i++ {
			at := otSub(la, int(otU16(la, 2+i*2)))
			if len(at) < 2 {
				ligs = append(ligs, nil)
				continue
			}
			comps := int(otU16(at, 0))
			rows := make([][]otAnchor, 0, comps)
			for c := 0; c < comps; c++ {
				row := make([]otAnchor, classCount)
				for k := 0; k < classCount; k++ {
					row[k] = otParseAnchor(at, int(otU16(at, 2+(c*classCount+k)*2)))
				}
				rows = append(rows, row)
			}
			ligs = append(ligs, rows)
		}
		return &gposMarkLig{markCov: cov, ligCov: ligCov, marks: marks,
			classCount: classCount, ligAnchors: ligs}

	case 7:
		return otParseContext(b, format, cov, covOK)

	case 8:
		return otParseChainContext(b, format, cov, covOK)
	}
	return nil
}

func otParseMarkArray(b []byte) []otMarkRecord {
	if len(b) < 2 {
		return nil
	}
	n := int(otU16(b, 0))
	if 2+n*4 > len(b) {
		return nil
	}
	out := make([]otMarkRecord, n)
	for i := 0; i < n; i++ {
		out[i] = otMarkRecord{
			class:  otU16(b, 2+i*4),
			anchor: otParseAnchor(b, int(otU16(b, 2+i*4+2))),
		}
	}
	return out
}

// otParseAnchorMatrix reads a BaseArray / Mark2Array: one row of
// classCount anchors per covered glyph.
func otParseAnchorMatrix(b []byte, classCount int) [][]otAnchor {
	if len(b) < 2 || classCount <= 0 {
		return nil
	}
	n := int(otU16(b, 0))
	if 2+n*classCount*2 > len(b) {
		return nil
	}
	out := make([][]otAnchor, n)
	for i := 0; i < n; i++ {
		row := make([]otAnchor, classCount)
		for k := 0; k < classCount; k++ {
			row[k] = otParseAnchor(b, int(otU16(b, 2+(i*classCount+k)*2)))
		}
		out[i] = row
	}
	return out
}

// --- feature selection --------------------------------------------------

// hasFeature reports whether the script advertises the named feature — the
// test that decides whether the font shapes Arabic itself.
func (t *otLayoutTable) hasFeature(script, tag string) bool {
	if t == nil {
		return false
	}
	sc := t.scripts[script]
	if sc == nil || sc.defaultLang == nil {
		return false
	}
	for _, fi := range sc.defaultLang.features {
		if fi >= 0 && fi < len(t.features) && t.features[fi].tag == tag {
			return true
		}
	}
	return false
}
