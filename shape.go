// SPDX-License-Identifier: MIT

package asposepdf

import "sort"

// The OpenType shaping engine (RTL support, phase 3; epic pdf-go-26u4).
// opentype.go reads a font's GSUB/GPOS/GDEF tables; this file runs them
// over a text run and produces positioned glyphs.
//
// The model follows HarfBuzz, minus what a PDF writer does not need: the
// buffer stays in logical order throughout (the caller reverses RTL runs
// afterwards), lookups are applied in lookup-list order, glyphs carry a
// feature mask that decides which lookups see them, and attachment
// (cursive, mark-to-base, mark-to-ligature, mark-to-mark) is recorded as a
// chain during positioning and resolved into absolute offsets at the end.
//
// Not implemented: the Indic/Khmer/Myanmar/Hangul reordering shapers,
// vertical writing, and alternate-glyph selection (an `aalt`-style feature
// takes its first alternate). Everything else degrades by doing less, never
// by failing: an unknown subtable simply does not match.

// shapedGlyph is one output glyph in font units.
type shapedGlyph struct {
	gid     uint16
	cluster int   // index of the rune this glyph came from
	advance int32 // horizontal advance
	xOff    int32 // placement offset, x
	yOff    int32 // placement offset, y

	mask      uint32 // features this glyph participates in
	isMark    bool   // Unicode says this is a combining mark (GDEF fallback)
	ignorable bool   // a default-ignorable character, hidden after shaping
	text      string // the characters this glyph draws, for /ToUnicode
	rtl       bool   // drawn in a right-to-left run (set by shapeLine)

	// deleted marks a glyph ligate/gsubMultiple consumed during the GSUB
	// lookup pass currently running. It is tombstoned rather than spliced
	// out of s.glyphs immediately — see compactGSUB — so every matcher
	// (otMatcher.skip, and so next/prev/matchForward/matchBackward too)
	// treats it as unconditionally skippable in the meantime.
	deleted bool

	// Ligature bookkeeping: which ligature a glyph belongs to and which of
	// its components, so a mark can attach to the right part of it.
	ligID   uint32
	ligComp uint8

	// Attachment chain, filled by GPOS and resolved by resolveAttachments.
	attachType  uint8
	attachChain int // relative index of the glyph this one attaches to
}

const (
	attachNone = iota
	attachMark
	attachCursive
)

// Feature masks. Every glyph carries otMaskGlobal; an Arabic letter also
// carries exactly one joining-form bit, so the font's isol/init/medi/fina
// lookups each see only the glyphs in that form.
const (
	otMaskGlobal uint32 = 1 << iota
	otMaskIsol
	otMaskInit
	otMaskMedi
	otMaskFina
)

// otFeatSpec is one entry of a feature plan: the feature tag and the glyph
// mask that gates it.
type otFeatSpec struct {
	tag  string
	mask uint32
}

// otPlanLookup is a lookup selected by the plan, with the union of the
// masks of the features that referenced it.
type otPlanLookup struct {
	index int
	mask  uint32
}

// Feature plans. GSUB order within a plan does not matter (lookups run in
// lookup-list order, as the spec requires); the tags decide what is on.
var (
	arabicGSUBFeatures = []otFeatSpec{
		{"ccmp", otMaskGlobal}, {"locl", otMaskGlobal},
		{"isol", otMaskIsol}, {"init", otMaskInit},
		{"medi", otMaskMedi}, {"fina", otMaskFina},
		// The Syriac variants of the joining forms ride the same masks;
		// an Arabic font never has them, a Syriac one is shaped roughly
		// rather than not at all.
		{"med2", otMaskMedi}, {"fin2", otMaskFina}, {"fin3", otMaskFina},
		{"rlig", otMaskGlobal}, {"rclt", otMaskGlobal},
		{"calt", otMaskGlobal}, {"liga", otMaskGlobal},
		{"mset", otMaskGlobal},
	}
	// abvm/blwm (above/below-base marks) are nominally Indic features, but
	// fonts such as Noto Sans Arabic file their mark-to-base lookups under
	// them, so every plan enables them — as HarfBuzz does.
	arabicGPOSFeatures = []otFeatSpec{
		{"curs", otMaskGlobal}, {"kern", otMaskGlobal}, {"dist", otMaskGlobal},
		{"abvm", otMaskGlobal}, {"blwm", otMaskGlobal},
		{"mark", otMaskGlobal}, {"mkmk", otMaskGlobal},
	}
	defaultGSUBFeatures = []otFeatSpec{
		{"ccmp", otMaskGlobal}, {"locl", otMaskGlobal},
		{"liga", otMaskGlobal}, {"clig", otMaskGlobal},
		{"calt", otMaskGlobal}, {"rclt", otMaskGlobal},
		{"rlig", otMaskGlobal},
	}
	defaultGPOSFeatures = []otFeatSpec{
		{"kern", otMaskGlobal}, {"dist", otMaskGlobal},
		{"abvm", otMaskGlobal}, {"blwm", otMaskGlobal},
		{"mark", otMaskGlobal}, {"mkmk", otMaskGlobal},
	}
)

// otShaper holds the state of one shaping run.
type otShaper struct {
	font   *ttfFont
	layout *otLayout
	gdef   *otGDEF
	glyphs []shapedGlyph
	rtl    bool

	table      *otLayoutTable // the table currently being applied
	nesting    int
	ligCounter uint32

	// prevBaseCache[i], once built, is prevBase(i)'s answer — see
	// prevBase's own doc comment (GHSA-7jjg-q5vf-8592 / pdf-go-velp).
	// Built lazily on first use since most shaping runs never call
	// prevBase at all (no GPOS mark-attachment lookups apply).
	prevBaseCache      []int
	prevBaseCacheBuilt bool

	// hasDeleted and pendingInserts back the deferred-compaction scheme
	// ligate/gsubMultiple use to avoid an O(n) buffer shift on every
	// single substitution event (GHSA-vj25-h6h5-3phj / pdf-go-sv5t point
	// 2, a follow-up to GHSA-7jjg-q5vf-8592 / pdf-go-velp's own two
	// quadratic hot spots): a ligature or a zero-length multiple
	// substitution tombstones the glyphs it consumes (deleted=true)
	// instead of removing them, and a growing (one-to-many) multiple
	// substitution queues its extra glyphs here keyed by the position
	// they go after, instead of splicing them in immediately. Both are
	// applied in a single O(n) rebuild by compactGSUB once the whole
	// lookup's forward scan finishes, rather than once per event.
	hasDeleted     bool
	pendingInserts map[int][]shapedGlyph
}

// compactGSUB rebuilds s.glyphs in one O(n) pass, dropping every glyph
// tombstoned this lookup pass and splicing in whatever a growing
// multiple substitution queued in pendingInserts — see the otShaper
// field comments above for why this is deferred instead of running on
// every individual substitution. Called once, at the end of a top-level
// applyLookup's GSUB forward scan: by then every tombstone/pending
// insert this lookup produced, including ones a nested nested lookup
// queued via applyNested, has already been recorded, so this single
// pass leaves s.glyphs exactly as immediate splicing would have, just
// without the repeated O(remaining length) copying.
func (s *otShaper) compactGSUB() {
	if !s.hasDeleted && len(s.pendingInserts) == 0 {
		return
	}
	out := make([]shapedGlyph, 0, len(s.glyphs))
	for i := range s.glyphs {
		if !s.glyphs[i].deleted {
			out = append(out, s.glyphs[i])
		}
		if extra, ok := s.pendingInserts[i]; ok {
			out = append(out, extra...)
		}
	}
	s.glyphs = out
	s.hasDeleted = false
	s.pendingInserts = nil
}

// otScriptTag picks the OpenType script tag for a run from its characters.
// Only the scripts this engine has a plan for are named; everything else
// takes the default plan under "latn"/DFLT.
func otScriptTag(runes []rune) string {
	for _, r := range runes {
		switch {
		case bidiClass(r) == clsAL:
			return "arab"
		case r >= 0x0590 && r <= 0x05FF:
			return "hebr"
		}
	}
	return "latn"
}

// fontShapesArabic reports whether the font's GSUB shapes Arabic itself —
// the test that decides whether the Presentation Forms-B pass (phase 2)
// should stand aside. See shapeArabic in arabic_shape.go.
func fontShapesArabic(f *ttfFont) bool {
	l := f.layout()
	if l == nil || l.gsub == nil {
		return false
	}
	return l.gsub.hasFeature("arab", "init") ||
		l.gsub.hasFeature("arab", "medi") ||
		l.gsub.hasFeature("arab", "fina")
}

// fontHasShaping reports whether shaping this font can do anything at all —
// if not, the text pipeline keeps its plain rune-by-rune encoding.
func fontHasShaping(f *ttfFont) bool {
	l := f.layout()
	return l != nil && (l.gsub != nil || l.gpos != nil)
}

// shapeRun shapes one run of text (all of it in the same direction) and
// returns its glyphs in logical order, in font units. It returns nil when
// the font has no layout tables, so callers can fall back.
func shapeRun(f *ttfFont, text string, rtl bool) []shapedGlyph {
	l := f.layout()
	if l == nil {
		return nil
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return []shapedGlyph{}
	}
	// Decompose what the font lacks, order the marks, recompose what it has
	// (combining.go). Clusters index this normalized sequence.
	runes = normalizeForFont(f, runes)

	s := &otShaper{font: f, layout: l, gdef: l.gdef, rtl: rtl}
	s.glyphs = make([]shapedGlyph, len(runes))
	for i, r := range runes {
		s.glyphs[i] = shapedGlyph{
			gid:       f.glyphID(r),
			cluster:   i,
			mask:      otMaskGlobal,
			isMark:    bidiClass(r) == clsNSM,
			ignorable: isDefaultIgnorable(r),
			text:      string(r),
		}
	}

	script := otScriptTag(runes)
	if script == "arab" {
		s.applyArabicJoining(runes)
	}

	gsubFeats, gposFeats := defaultGSUBFeatures, defaultGPOSFeatures
	if script == "arab" {
		gsubFeats, gposFeats = arabicGSUBFeatures, arabicGPOSFeatures
	}

	if l.gsub != nil {
		s.table = l.gsub
		for _, pl := range otBuildPlan(l.gsub, script, gsubFeats) {
			s.applyLookup(pl, true)
		}
	}

	// Advances come from hmtx once substitution has settled — a ligature
	// carries its own, not the sum of its components'.
	for i := range s.glyphs {
		s.glyphs[i].advance = int32(f.glyphAdvance(s.glyphs[i].gid))
	}

	if l.gpos != nil {
		s.table = l.gpos
		for _, pl := range otBuildPlan(l.gpos, script, gposFeats) {
			s.applyLookup(pl, false)
		}
		s.resolveAttachments()
	}
	s.hideDefaultIgnorables()
	return s.glyphs
}

// hideDefaultIgnorables makes joiners, direction marks and the like
// invisible once they have done their job of steering the lookups: each
// becomes a zero-advance space glyph, or is dropped when the font has no
// space. Drawing the font's own glyph for them risks a visible .notdef box.
func (s *otShaper) hideDefaultIgnorables() {
	space := s.font.glyphID(' ')
	out := s.glyphs[:0]
	for _, g := range s.glyphs {
		if g.ignorable {
			if space == 0 {
				continue
			}
			g.gid, g.advance, g.xOff, g.yOff = space, 0, 0, 0
		}
		out = append(out, g)
	}
	s.glyphs = out
}

// glyphAdvance is the glyph's hmtx advance, 0 for an out-of-range id.
func (f *ttfFont) glyphAdvance(gid uint16) uint16 {
	if int(gid) < len(f.glyphWidths) {
		return f.glyphWidths[gid]
	}
	return 0
}

// applyArabicJoining assigns each joining letter the single contextual-form
// mask its neighbours call for. Marks are transparent: a letter's neighbour
// is the nearest non-transparent character on each side.
func (s *otShaper) applyArabicJoining(runes []rune) {
	types := make([]joiningType, len(runes))
	for i, r := range runes {
		types[i] = joiningTypeOf(r)
	}
	neighbour := func(i, step int) joiningType {
		for j := i + step; j >= 0 && j < len(types); j += step {
			if types[j] != joinT {
				return types[j]
			}
		}
		return joinU
	}
	for i, t := range types {
		if t == joinU || t == joinT {
			continue
		}
		prevJoin := neighbour(i, -1).joinsFollowing() && t.joinsPreceding()
		nextJoin := neighbour(i, +1).joinsPreceding() && t.joinsFollowing()
		switch {
		case prevJoin && nextJoin:
			s.glyphs[i].mask |= otMaskMedi
		case prevJoin:
			s.glyphs[i].mask |= otMaskFina
		case nextJoin:
			s.glyphs[i].mask |= otMaskInit
		default:
			s.glyphs[i].mask |= otMaskIsol
		}
	}
}

// otBuildPlan resolves a feature plan into the lookups it selects, in
// ascending lookup-list order, each carrying the union of the masks of the
// features that asked for it.
func otBuildPlan(t *otLayoutTable, script string, feats []otFeatSpec) []otPlanLookup {
	sc := t.scripts[script]
	for _, alt := range []string{"DFLT", "dflt", "latn"} {
		if sc != nil {
			break
		}
		sc = t.scripts[alt]
	}
	if sc == nil || sc.defaultLang == nil {
		return nil
	}
	byTag := make(map[string]uint32, len(feats))
	for _, f := range feats {
		byTag[f.tag] |= f.mask
	}
	indices := sc.defaultLang.features
	if sc.defaultLang.required >= 0 {
		indices = append(append([]int{}, indices...), sc.defaultLang.required)
	}
	masks := map[int]uint32{}
	for _, fi := range indices {
		if fi < 0 || fi >= len(t.features) {
			continue
		}
		mask, want := byTag[t.features[fi].tag]
		if !want {
			continue
		}
		for _, li := range t.features[fi].lookups {
			if li >= 0 && li < len(t.lookups) {
				masks[li] |= mask
			}
		}
	}
	out := make([]otPlanLookup, 0, len(masks))
	for li, m := range masks {
		out = append(out, otPlanLookup{index: li, mask: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].index < out[j].index })
	return out
}

// --- glyph properties and skipping --------------------------------------

func (s *otShaper) glyphClass(g *shapedGlyph) uint16 {
	if s.gdef != nil && !s.gdef.glyphClass.empty() {
		if c := s.gdef.glyphClass.class(g.gid); c != 0 {
			return c
		}
	}
	if g.isMark {
		return otClassMark
	}
	return otClassBase
}

// otMatcher decides which glyphs a lookup steps over. LookupFlag says to
// ignore whole GDEF classes, or to accept only marks of one attachment
// class or mark-glyph set.
type otMatcher struct {
	s      *otShaper
	flag   uint16
	filter uint16
}

func (m otMatcher) skip(i int) bool {
	g := &m.s.glyphs[i]
	if g.deleted {
		return true
	}
	class := m.s.glyphClass(g)
	switch class {
	case otClassMark:
		if m.flag&otFlagIgnoreMarks != 0 {
			return true
		}
		if t := (m.flag & otFlagMarkAttachTypeMask) >> otFlagMarkAttachTypeShift; t != 0 {
			if m.s.gdef == nil || uint16(m.s.gdef.markAttachment.class(g.gid)) != t {
				return true
			}
		}
		if m.flag&otFlagUseMarkFilteringSet != 0 {
			if m.s.gdef == nil || int(m.filter) >= len(m.s.gdef.markSets) ||
				!m.s.gdef.markSets[m.filter].covers(g.gid) {
				return true
			}
		}
	case otClassLigature:
		if m.flag&otFlagIgnoreLigatures != 0 {
			return true
		}
	case otClassBase:
		if m.flag&otFlagIgnoreBaseGlyphs != 0 {
			return true
		}
	}
	return false
}

// next returns the first position after i the lookup does not skip, or -1.
func (m otMatcher) next(i int) int {
	for j := i + 1; j < len(m.s.glyphs); j++ {
		if !m.skip(j) {
			return j
		}
	}
	return -1
}

// prev is next's backward counterpart.
func (m otMatcher) prev(i int) int {
	for j := i - 1; j >= 0; j-- {
		if !m.skip(j) {
			return j
		}
	}
	return -1
}

func (s *otShaper) matcher(l *otLookup) otMatcher {
	return otMatcher{s: s, flag: l.flag, filter: l.markFilter}
}

// --- lookup application -------------------------------------------------

// applyLookup runs one planned lookup across the whole buffer.
func (s *otShaper) applyLookup(pl otPlanLookup, gsub bool) {
	if pl.index >= len(s.table.lookups) {
		return
	}
	l := s.table.lookups[pl.index]
	m := s.matcher(l)

	// Reverse chaining substitution is defined to run right to left, and
	// never changes the glyph count.
	if gsub && l.kind == 8 {
		for i := len(s.glyphs) - 1; i >= 0; i-- {
			if s.glyphs[i].mask&pl.mask == 0 || m.skip(i) {
				continue
			}
			s.applyReverseChain(l, i)
		}
		return
	}

	for i := 0; i < len(s.glyphs); {
		if s.glyphs[i].mask&pl.mask == 0 || m.skip(i) {
			i++
			continue
		}
		var next int
		if gsub {
			next = s.applyGSUB(l, i)
		} else {
			next = s.applyGPOS(l, i)
		}
		if next > i {
			i = next
		} else {
			i++
		}
	}
	if gsub {
		s.compactGSUB()
	}
}

// applyLookupAt applies one lookup at a single position — the entry point
// for a contextual lookup's nested records. Returns the position to
// continue from, or -1 when nothing matched.
func (s *otShaper) applyLookupAt(index, pos int, gsub bool) int {
	if index < 0 || index >= len(s.table.lookups) || pos < 0 || pos >= len(s.glyphs) {
		return -1
	}
	l := s.table.lookups[index]
	if gsub {
		if l.kind == 8 {
			return s.applyReverseChain(l, pos)
		}
		return s.applyGSUB(l, pos)
	}
	return s.applyGPOS(l, pos)
}

func (s *otShaper) applyGSUB(l *otLookup, i int) int {
	for _, st := range l.subtables {
		if n := s.applyGSUBSubtable(st, l, i); n > 0 {
			return n
		}
	}
	return -1
}

func (s *otShaper) applyGSUBSubtable(st any, l *otLookup, i int) int {
	g := s.glyphs[i].gid
	switch t := st.(type) {
	case *gsubSingle:
		ci := t.cov.index(g)
		if ci < 0 {
			return -1
		}
		if t.substs != nil {
			if ci >= len(t.substs) {
				return -1
			}
			s.glyphs[i].gid = t.substs[ci]
		} else {
			s.glyphs[i].gid = uint16(int32(g) + int32(t.delta))
		}
		return i + 1

	case *gsubMultiple:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.seqs) {
			return -1
		}
		seq := t.seqs[ci]
		if len(seq) == 0 {
			// A zero-length sequence deletes the glyph — tombstoned
			// rather than spliced out immediately; see compactGSUB.
			s.glyphs[i].deleted = true
			s.hasDeleted = true
			return i + 1
		}
		s.glyphs[i].gid = seq[0]
		if len(seq) > 1 {
			// Queue the extra glyphs to be spliced in after i by
			// compactGSUB instead of shifting the whole tail now; see
			// the otShaper.pendingInserts field comment.
			extra := make([]shapedGlyph, len(seq)-1)
			for k := range extra {
				extra[k] = s.glyphs[i]
				extra[k].gid = seq[k+1]
				extra[k].text = "" // the first glyph of the sequence carries the text
			}
			if s.pendingInserts == nil {
				s.pendingInserts = make(map[int][]shapedGlyph)
			}
			s.pendingInserts[i] = extra
		}
		return i + 1

	case *gsubAlternate:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.alts) || len(t.alts[ci]) == 0 {
			return -1
		}
		s.glyphs[i].gid = t.alts[ci][0]
		return i + 1

	case *gsubLigature:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.sets) {
			return -1
		}
		m := s.matcher(l)
		for _, lig := range t.sets[ci] {
			pos, ok := s.matchForward(i, m, len(lig.components), func(k int, gid uint16) bool {
				return gid == lig.components[k]
			})
			if ok {
				return s.ligate(i, pos, lig.glyph)
			}
		}
		return -1

	case *otContext1, *otContext2, *otContext3, *otChain1, *otChain2, *otChain3:
		return s.applyContext(st, l, i, true)
	}
	return -1
}

// applyReverseChain applies a GSUB type 8 subtable at i. It substitutes one
// glyph and never changes the buffer length, so it returns i.
func (s *otShaper) applyReverseChain(l *otLookup, i int) int {
	m := s.matcher(l)
	for _, st := range l.subtables {
		t, ok := st.(*gsubReverseChain)
		if !ok {
			continue
		}
		ci := t.cov.index(s.glyphs[i].gid)
		if ci < 0 || ci >= len(t.substitute) {
			continue
		}
		if !s.matchBackward(i, m, len(t.backtrack), func(k int, g uint16) bool {
			return t.backtrack[k].covers(g)
		}) {
			continue
		}
		if _, ok := s.matchForward(i, m, len(t.lookahead), func(k int, g uint16) bool {
			return t.lookahead[k].covers(g)
		}); !ok {
			continue
		}
		s.glyphs[i].gid = t.substitute[ci]
		return i
	}
	return -1
}

// matchForward matches n glyphs after i, skipping what the matcher skips,
// and returns their positions.
func (s *otShaper) matchForward(i int, m otMatcher, n int, ok func(k int, g uint16) bool) ([]int, bool) {
	if n == 0 {
		return nil, true
	}
	pos := make([]int, 0, n)
	j := i
	for k := 0; k < n; k++ {
		j = m.next(j)
		if j < 0 || !ok(k, s.glyphs[j].gid) {
			return nil, false
		}
		pos = append(pos, j)
	}
	return pos, true
}

// matchBackward matches n glyphs before i in reverse order (the order
// backtrack sequences are written in).
func (s *otShaper) matchBackward(i int, m otMatcher, n int, ok func(k int, g uint16) bool) bool {
	j := i
	for k := 0; k < n; k++ {
		j = m.prev(j)
		if j < 0 || !ok(k, s.glyphs[j].gid) {
			return false
		}
	}
	return true
}

// ligate replaces the glyph at i with lig and tombstones the component
// glyphs at comps (deleted=true, rather than removing them from s.glyphs
// immediately — see compactGSUB), tagging any glyphs that were skipped
// between them (marks) with the ligature's id and the component they
// follow — which is what lets mark-to-ligature positioning find the right
// attachment point. It returns the position just past the last component.
func (s *otShaper) ligate(i int, comps []int, lig uint16) int {
	s.ligCounter++
	id := s.ligCounter
	s.glyphs[i].gid = lig
	s.glyphs[i].ligID = id
	s.glyphs[i].ligComp = 0

	last := comps[len(comps)-1]
	ci, comp := 0, uint8(1)
	for k := i + 1; k <= last; k++ {
		if ci < len(comps) && comps[ci] == k {
			s.glyphs[i].text += s.glyphs[k].text
			s.glyphs[k].deleted = true
			s.hasDeleted = true
			ci++
			comp++
			continue
		}
		s.glyphs[k].ligID = id
		s.glyphs[k].ligComp = comp
	}
	return last + 1
}

// --- contextual lookups (GSUB 5/6, GPOS 7/8) ----------------------------

func (s *otShaper) applyContext(st any, l *otLookup, i int, gsub bool) int {
	m := s.matcher(l)
	g := s.glyphs[i].gid

	switch t := st.(type) {
	case *otContext1:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.rules) {
			return -1
		}
		for _, r := range t.rules[ci] {
			if pos, ok := s.matchForward(i, m, len(r.input), func(k int, gg uint16) bool {
				return gg == r.input[k]
			}); ok {
				return s.applyNested(append([]int{i}, pos...), r.lookups, gsub)
			}
		}

	case *otContext2:
		if !t.cov.covers(g) {
			return -1
		}
		ci := int(t.classes.class(g))
		if ci >= len(t.rules) {
			return -1
		}
		for _, r := range t.rules[ci] {
			if pos, ok := s.matchForward(i, m, len(r.input), func(k int, gg uint16) bool {
				return t.classes.class(gg) == r.input[k]
			}); ok {
				return s.applyNested(append([]int{i}, pos...), r.lookups, gsub)
			}
		}

	case *otContext3:
		if len(t.covs) == 0 || !t.covs[0].covers(g) {
			return -1
		}
		if pos, ok := s.matchForward(i, m, len(t.covs)-1, func(k int, gg uint16) bool {
			return t.covs[k+1].covers(gg)
		}); ok {
			return s.applyNested(append([]int{i}, pos...), t.lookups, gsub)
		}

	case *otChain1:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.rules) {
			return -1
		}
		for _, r := range t.rules[ci] {
			if n := s.matchChain(i, m, r,
				func(k int, gg uint16) bool { return gg == r.backtrack[k] },
				func(k int, gg uint16) bool { return gg == r.input[k] },
				func(k int, gg uint16) bool { return gg == r.lookahead[k] },
				gsub); n > 0 {
				return n
			}
		}

	case *otChain2:
		if !t.cov.covers(g) {
			return -1
		}
		ci := int(t.inClass.class(g))
		if ci >= len(t.rules) {
			return -1
		}
		for _, r := range t.rules[ci] {
			if n := s.matchChain(i, m, r,
				func(k int, gg uint16) bool { return t.backClass.class(gg) == r.backtrack[k] },
				func(k int, gg uint16) bool { return t.inClass.class(gg) == r.input[k] },
				func(k int, gg uint16) bool { return t.aClass.class(gg) == r.lookahead[k] },
				gsub); n > 0 {
				return n
			}
		}

	case *otChain3:
		if len(t.input) == 0 || !t.input[0].covers(g) {
			return -1
		}
		if !s.matchBackward(i, m, len(t.back), func(k int, gg uint16) bool {
			return t.back[k].covers(gg)
		}) {
			return -1
		}
		pos, ok := s.matchForward(i, m, len(t.input)-1, func(k int, gg uint16) bool {
			return t.input[k+1].covers(gg)
		})
		if !ok {
			return -1
		}
		end := i
		if len(pos) > 0 {
			end = pos[len(pos)-1]
		}
		if _, ok := s.matchForward(end, m, len(t.ahead), func(k int, gg uint16) bool {
			return t.ahead[k].covers(gg)
		}); !ok {
			return -1
		}
		return s.applyNested(append([]int{i}, pos...), t.lookups, gsub)
	}
	return -1
}

// matchChain matches one chain rule's backtrack, input and lookahead
// sequences around i and applies its nested lookups.
func (s *otShaper) matchChain(i int, m otMatcher, r otChainRule,
	back, input, ahead func(k int, g uint16) bool, gsub bool) int {
	if !s.matchBackward(i, m, len(r.backtrack), back) {
		return -1
	}
	pos, ok := s.matchForward(i, m, len(r.input), input)
	if !ok {
		return -1
	}
	end := i
	if len(pos) > 0 {
		end = pos[len(pos)-1]
	}
	if _, ok := s.matchForward(end, m, len(r.lookahead), ahead); !ok {
		return -1
	}
	return s.applyNested(append([]int{i}, pos...), r.lookups, gsub)
}

// applyNested runs a contextual lookup's nested records against the matched
// positions, keeping those positions valid as the buffer changes length
// underneath them. It returns the position just past the matched input.
func (s *otShaper) applyNested(matched []int, recs []otSeqLookup, gsub bool) int {
	end := matched[len(matched)-1]
	if s.nesting >= otMaxNestedLookupRecursion {
		return end + 1
	}
	s.nesting++
	defer func() { s.nesting-- }()

	for _, r := range recs {
		idx := int(r.seqIndex)
		if idx >= len(matched) {
			continue
		}
		p := matched[idx]
		before := len(s.glyphs)
		if s.applyLookupAt(int(r.lookupIndex), p, gsub) < 0 {
			continue
		}
		if delta := len(s.glyphs) - before; delta != 0 {
			for k := range matched {
				if matched[k] > p {
					matched[k] += delta
				}
			}
			end += delta
			if end < p {
				end = p
			}
		}
	}
	return end + 1
}

// --- GPOS ---------------------------------------------------------------

func (s *otShaper) applyGPOS(l *otLookup, i int) int {
	for _, st := range l.subtables {
		if n := s.applyGPOSSubtable(st, l, i); n > 0 {
			return n
		}
	}
	return -1
}

func (s *otShaper) applyGPOSSubtable(st any, l *otLookup, i int) int {
	g := s.glyphs[i].gid
	switch t := st.(type) {
	case *gposSingle:
		ci := t.cov.index(g)
		if ci < 0 {
			return -1
		}
		v := t.value
		if t.values != nil {
			if ci >= len(t.values) {
				return -1
			}
			v = t.values[ci]
		}
		s.applyValue(i, v)
		return i + 1

	case *gposPair1:
		ci := t.cov.index(g)
		if ci < 0 || ci >= len(t.sets) {
			return -1
		}
		j := s.matcher(l).next(i)
		if j < 0 {
			return -1
		}
		for _, pv := range t.sets[ci] {
			if pv.second != s.glyphs[j].gid {
				continue
			}
			s.applyValue(i, pv.value1)
			if pv.hasValue2 {
				s.applyValue(j, pv.value2)
				return j + 1
			}
			return j
		}
		return -1

	case *gposPair2:
		if !t.cov.covers(g) {
			return -1
		}
		j := s.matcher(l).next(i)
		if j < 0 {
			return -1
		}
		c1, c2 := int(t.class1.class(g)), int(t.class2.class(s.glyphs[j].gid))
		if c1 >= t.n1 || c2 >= t.n2 {
			return -1
		}
		idx := c1*t.n2 + c2
		if idx >= len(t.values) {
			return -1
		}
		s.applyValue(i, t.values[idx])
		if t.hasSecondVal {
			s.applyValue(j, t.values2[idx])
			return j + 1
		}
		return j

	case *gposCursive:
		return s.applyCursive(t, l, i)

	case *gposMarkBase:
		return s.applyMarkBase(t, i)

	case *gposMarkLig:
		return s.applyMarkLig(t, i)

	case *gposMarkMark:
		return s.applyMarkMark(t, l, i)

	case *otContext1, *otContext2, *otContext3, *otChain1, *otChain2, *otChain3:
		return s.applyContext(st, l, i, false)
	}
	return -1
}

func (s *otShaper) applyValue(i int, v otValueRecord) {
	if v.zero() {
		return
	}
	g := &s.glyphs[i]
	g.xOff += int32(v.xPlacement)
	g.yOff += int32(v.yPlacement)
	g.advance += int32(v.xAdvance)
}

// applyCursive joins the exit anchor of the glyph at i to the entry anchor
// of the next one, the way a cursive script's strokes must meet.
func (s *otShaper) applyCursive(t *gposCursive, l *otLookup, i int) int {
	ci := t.cov.index(s.glyphs[i].gid)
	if ci < 0 || ci >= len(t.exit) || !t.exit[ci].exists {
		return -1
	}
	j := s.matcher(l).next(i)
	if j < 0 {
		return -1
	}
	cj := t.cov.index(s.glyphs[j].gid)
	if cj < 0 || cj >= len(t.entry) || !t.entry[cj].exists {
		return -1
	}
	exitX, exitY := int32(t.exit[ci].x), int32(t.exit[ci].y)
	entryX, entryY := int32(t.entry[cj].x), int32(t.entry[cj].y)

	if s.rtl {
		d := exitX + s.glyphs[i].xOff
		s.glyphs[i].advance -= d
		s.glyphs[i].xOff -= d
		s.glyphs[j].advance = entryX + s.glyphs[j].xOff
	} else {
		s.glyphs[i].advance = exitX + s.glyphs[i].xOff
		d := entryX + s.glyphs[j].xOff
		s.glyphs[j].advance -= d
		s.glyphs[j].xOff -= d
	}

	// The two glyphs sit on one stroke: one of them is pulled onto the
	// other's baseline offset. Which one is the child depends on the
	// lookup's right-to-left flag.
	child, parent := i, j
	yOff := entryY - exitY
	if l.flag&otFlagRightToLeft == 0 {
		child, parent = j, i
		yOff = -yOff
	}
	s.glyphs[child].attachType = attachCursive
	s.glyphs[child].attachChain = parent - child
	s.glyphs[child].yOff = yOff
	return j
}

// prevBase returns the nearest preceding glyph that is not a mark,
// using a cache built once per shaping run (GHSA-7jjg-q5vf-8592 /
// pdf-go-velp): a naive backward scan here made attaching N consecutive
// marks to their base O(N²) overall, since each mark's own scan walked
// back through every mark already seen — a text value of a few KB of
// combining marks could stall AddText for seconds. The cache is valid
// for the lifetime of one otShaper (a fresh otShaper is built per
// shaping call): a glyph's mark/base classification (glyphClass, driven
// by GDEF class + the precomputed isMark flag — see glyphClass) never
// changes once GPOS positioning begins, since gids and the glyph count
// are already fixed by then. prevBase's own matcher flag
// (otFlagIgnoreMarks only, no mark-filtering-set, no ignore-ligatures)
// is a fixed constant too, independent of whichever GPOS lookup is
// calling it — so, unlike a cache keyed to one lookup's own flag, this
// one answer is correct for every caller throughout the whole run.
func (s *otShaper) prevBase(i int) int {
	if !s.prevBaseCacheBuilt {
		s.buildPrevBaseCache()
	}
	if i < 0 || i >= len(s.prevBaseCache) {
		return -1
	}
	return s.prevBaseCache[i]
}

// buildPrevBaseCache computes prevBase's answer for every position in
// one forward O(n) pass.
func (s *otShaper) buildPrevBaseCache() {
	m := otMatcher{s: s, flag: otFlagIgnoreMarks}
	s.prevBaseCache = make([]int, len(s.glyphs))
	last := -1
	for i := range s.glyphs {
		s.prevBaseCache[i] = last
		if !m.skip(i) {
			last = i
		}
	}
	s.prevBaseCacheBuilt = true
}

func (s *otShaper) attachMarkTo(i, base int, mark otMarkRecord, anchor otAnchor) int {
	if !mark.anchor.exists || !anchor.exists {
		return -1
	}
	s.glyphs[i].xOff = int32(anchor.x) - int32(mark.anchor.x)
	s.glyphs[i].yOff = int32(anchor.y) - int32(mark.anchor.y)
	s.glyphs[i].attachType = attachMark
	s.glyphs[i].attachChain = base - i
	return i + 1
}

func (s *otShaper) applyMarkBase(t *gposMarkBase, i int) int {
	mi := t.markCov.index(s.glyphs[i].gid)
	if mi < 0 || mi >= len(t.marks) {
		return -1
	}
	base := s.prevBase(i)
	if base < 0 {
		return -1
	}
	bi := t.baseCov.index(s.glyphs[base].gid)
	if bi < 0 || bi >= len(t.baseAnchors) {
		return -1
	}
	mark := t.marks[mi]
	if int(mark.class) >= len(t.baseAnchors[bi]) {
		return -1
	}
	return s.attachMarkTo(i, base, mark, t.baseAnchors[bi][mark.class])
}

func (s *otShaper) applyMarkLig(t *gposMarkLig, i int) int {
	mi := t.markCov.index(s.glyphs[i].gid)
	if mi < 0 || mi >= len(t.marks) {
		return -1
	}
	lig := s.prevBase(i)
	if lig < 0 {
		return -1
	}
	li := t.ligCov.index(s.glyphs[lig].gid)
	if li < 0 || li >= len(t.ligAnchors) {
		return -1
	}
	comps := t.ligAnchors[li]
	if len(comps) == 0 {
		return -1
	}
	// A mark tagged with this ligature's id attaches to the component it
	// followed; anything else attaches to the last one.
	idx := len(comps) - 1
	if id := s.glyphs[lig].ligID; id != 0 && s.glyphs[i].ligID == id && s.glyphs[i].ligComp > 0 {
		if c := int(s.glyphs[i].ligComp) - 1; c < idx {
			idx = c
		}
	}
	mark := t.marks[mi]
	if int(mark.class) >= len(comps[idx]) {
		return -1
	}
	return s.attachMarkTo(i, lig, mark, comps[idx][mark.class])
}

func (s *otShaper) applyMarkMark(t *gposMarkMark, l *otLookup, i int) int {
	mi := t.mark1Cov.index(s.glyphs[i].gid)
	if mi < 0 || mi >= len(t.marks) {
		return -1
	}
	// Mark-to-mark stacks one mark on another, so marks must not be
	// skipped here whatever the lookup flag says about them.
	// The mark filtering set still applies: it is what keeps a vowel from
	// stacking on an unrelated mark.
	m := otMatcher{s: s, flag: l.flag &^ (otFlagIgnoreMarks | otFlagIgnoreBaseGlyphs | otFlagIgnoreLigatures), filter: l.markFilter}
	prev := m.prev(i)
	if prev < 0 || s.glyphClass(&s.glyphs[prev]) != otClassMark {
		return -1
	}
	pi := t.mark2Cov.index(s.glyphs[prev].gid)
	if pi < 0 || pi >= len(t.mark2Anchors) {
		return -1
	}
	mark := t.marks[mi]
	if int(mark.class) >= len(t.mark2Anchors[pi]) {
		return -1
	}
	return s.attachMarkTo(i, prev, mark, t.mark2Anchors[pi][mark.class])
}

// resolveAttachments turns the attachment chains GPOS recorded into
// absolute offsets: a mark's offset is relative to the glyph it attached
// to, which may itself be attached to something else, and the pen has
// already advanced past everything in between.
// resolveAttachments resolves every glyph's attachment chain (mark-to-base,
// mark-to-ligature, cursive) into absolute xOff/yOff offsets.
//
// advancePrefix[i] is the sum of s.glyphs[0:i].advance, built once here
// before any propagation runs — propagateAttachment never mutates
// .advance (only .xOff/.yOff), so the sums stay valid for the whole pass.
// Passing it down lets propagateAttachment turn its "sum every advance
// between the base and the mark" loop into one O(1) subtraction
// (GHSA-vj25-h6h5-3phj / pdf-go-sv5t point 1, a follow-up to
// GHSA-7jjg-q5vf-8592 / pdf-go-velp: that fix's own regression test used
// base letter "a", which DejaVu Sans's mark-to-base lookup apparently
// recomposes through a path that never reaches this loop at all, so the
// O(N²) cost of N marks sharing one base — any base OTHER than the few
// the test happened to try — went unnoticed).
func (s *otShaper) resolveAttachments() {
	advancePrefix := make([]int32, len(s.glyphs)+1)
	for i, g := range s.glyphs {
		advancePrefix[i+1] = advancePrefix[i] + g.advance
	}
	for i := range s.glyphs {
		s.propagateAttachment(i, map[int]bool{}, advancePrefix)
	}
	for i := range s.glyphs {
		s.glyphs[i].attachType = attachNone
		s.glyphs[i].attachChain = 0
	}
}

func (s *otShaper) propagateAttachment(i int, seen map[int]bool, advancePrefix []int32) {
	g := &s.glyphs[i]
	if g.attachChain == 0 || seen[i] {
		return
	}
	seen[i] = true
	j := i + g.attachChain
	if j < 0 || j >= len(s.glyphs) {
		g.attachChain = 0
		return
	}
	s.propagateAttachment(j, seen, advancePrefix)

	switch g.attachType {
	case attachMark:
		g.xOff += s.glyphs[j].xOff
		g.yOff += s.glyphs[j].yOff
		// Undo the advances between the base and the mark: the offset was
		// measured from the base's origin, but by the time the mark is
		// drawn the pen has moved. Which glyphs lie in between depends on
		// the direction — an RTL run is reversed before it is drawn, so
		// the mark comes out ahead of its base rather than behind it.
		// sum(s.glyphs[k].advance for k in [lo, hi)) == advancePrefix[hi] - advancePrefix[lo].
		if j >= i {
			break
		}
		if !s.rtl {
			g.xOff -= advancePrefix[i] - advancePrefix[j]
		} else {
			g.xOff += advancePrefix[i+1] - advancePrefix[j+1]
		}
	case attachCursive:
		g.yOff += s.glyphs[j].yOff
	}
	g.attachChain = 0
}
