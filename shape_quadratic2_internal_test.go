// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// pdf-go-sv5t / GHSA-vj25-h6h5-3phj, point 1: propagateAttachment
// (shape.go) summed every glyph's advance between a mark and its base on
// EVERY call — N marks sharing one base cost O(N²) overall. The original
// GHSA-7jjg-q5vf-8592 fix (pdf-go-velp) missed this because its own
// regression test used base letter "a": "a"+U+0301 recomposes to "á",
// which DejaVu Sans's mark-to-base lookup apparently handles through a
// path that never reaches this loop at all. The tests below deliberately
// cover several OTHER base letters/scripts (matching the reporter's own
// repro) plus a randomized differential test against the original
// per-call summation, so a fix that only happens to help one input shape
// cannot pass by coincidence — the same failure mode that let the first
// fix's own test miss this bug.

// referencePropagateAttachment is propagateAttachment's ORIGINAL
// per-call O(distance) summation, kept only as a differential-testing
// oracle for the O(1) prefix-sum rewrite.
func referencePropagateAttachment(glyphs []shapedGlyph, rtl bool, i int, seen map[int]bool) {
	g := &glyphs[i]
	if g.attachChain == 0 || seen[i] {
		return
	}
	seen[i] = true
	j := i + g.attachChain
	if j < 0 || j >= len(glyphs) {
		g.attachChain = 0
		return
	}
	referencePropagateAttachment(glyphs, rtl, j, seen)

	switch g.attachType {
	case attachMark:
		g.xOff += glyphs[j].xOff
		g.yOff += glyphs[j].yOff
		if j >= i {
			break
		}
		if !rtl {
			for k := j; k < i; k++ {
				g.xOff -= glyphs[k].advance
			}
		} else {
			for k := j + 1; k <= i; k++ {
				g.xOff += glyphs[k].advance
			}
		}
	case attachCursive:
		g.yOff += glyphs[j].yOff
	}
	g.attachChain = 0
}

// referenceResolveAttachments mirrors resolveAttachments' own top-level
// loop structure (a fresh `seen` map per starting index), calling the
// reference per-call implementation instead of the prefix-sum one.
func referenceResolveAttachments(glyphs []shapedGlyph, rtl bool) {
	for i := range glyphs {
		referencePropagateAttachment(glyphs, rtl, i, map[int]bool{})
	}
	for i := range glyphs {
		glyphs[i].attachType = attachNone
		glyphs[i].attachChain = 0
	}
}

// TestResolveAttachmentsMatchesReferenceOnRandomInput differentially
// tests the prefix-sum rewrite against the original per-call summation
// across many random attachment-chain configurations, LTR and RTL,
// including several marks chained onto the same base at varying
// distances (the shape that actually exercises the summed-advance loop).
func TestResolveAttachmentsMatchesReferenceOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 300; trial++ {
		rtl := trial%2 == 0
		n := rng.Intn(25) + 1
		glyphs := make([]shapedGlyph, n)
		for i := range glyphs {
			glyphs[i].advance = int32(rng.Intn(1000))
			glyphs[i].xOff = int32(rng.Intn(200) - 100)
			glyphs[i].yOff = int32(rng.Intn(200) - 100)
			if i == 0 {
				continue // index 0 can never attach backward or forward within bounds meaningfully for every offset; keep it a plain base
			}
			if rng.Intn(3) == 0 {
				continue // this glyph stays a plain base (no attachment)
			}
			// Attach backward to a random earlier glyph (the common
			// script order: marks follow their base) most of the time,
			// occasionally forward.
			var j int
			if rng.Intn(5) == 0 {
				j = i + rng.Intn(n-i)
			} else {
				j = rng.Intn(i)
			}
			glyphs[i].attachChain = j - i
			if rng.Intn(4) == 0 {
				glyphs[i].attachType = attachCursive
			} else {
				glyphs[i].attachType = attachMark
			}
		}

		want := append([]shapedGlyph(nil), glyphs...)
		got := append([]shapedGlyph(nil), glyphs...)

		referenceResolveAttachments(want, rtl)
		s := &otShaper{glyphs: got, rtl: rtl}
		s.resolveAttachments()

		for i := range want {
			if got[i].xOff != want[i].xOff || got[i].yOff != want[i].yOff {
				t.Fatalf("trial %d (rtl=%v, n=%d): glyph %d: got (xOff=%d,yOff=%d), want (xOff=%d,yOff=%d)",
					trial, rtl, n, i, got[i].xOff, got[i].yOff, want[i].xOff, want[i].yOff)
			}
		}
	}
}

// TestMeasureTextManyMarksOnNonARecomposingBaseStaysFast reproduces the
// advisory's own repro: a long run of combining marks on several base
// letters that do NOT recompose into a single precomposed character
// (unlike "a"+U+0301 -> "á"), asserting GROWTH rather than an absolute
// bound — doubling the input must not roughly quadruple the time — so a
// fix that only happens to help one particular base cannot pass by
// coincidence, which is exactly how the previous fix's own test missed
// this bug.
func TestMeasureTextManyMarksOnNonARecomposingBaseStaysFast(t *testing.T) {
	data, err := readFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	doc := NewDocument(595, 842)
	f, err := doc.LoadFontFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// Each base paired with a mark that actually has mark-to-base anchor
	// data for it in this font — not just any combining mark. A Latin
	// acute accent (U+0301) has no attachment data against an Arabic or
	// Hebrew base, so attachChain never gets set and the expensive loop
	// never runs regardless of n: that mismatch, not a fix, is why an
	// early draft of this test found no quadratic blowup for those two
	// scripts even with the bug still present.
	cases := []struct {
		name, base, mark string
	}{
		{"x+acute", "x", "́"},    // Latin, no precomposed x+acute exists
		{"и+acute", "и", "́"},    // Cyrillic и
		{"beh+fatha", "ب", "َ"},  // Arabic
		{"bet+qamats", "ב", "ָ"}, // Hebrew
	}
	const small, large = 4000, 16000 // 4x the input

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			timeFor := func(n int) time.Duration {
				text := tc.base + strings.Repeat(tc.mark, n)
				style := TextStyle{Font: f, Size: 10}
				start := time.Now()
				// MeasureText (not shapeLine directly) so Arabic/Hebrew
				// text goes through the same Arabic-shaping/bidi
				// detection resolveSingleLine applies before shaping —
				// shapeLine alone, called with a hardcoded bidi=false,
				// does not exercise the RTL attachment branch these
				// scripts actually take, which silently hid the bug
				// here during this fix's own development (first draft
				// of this test missed Arabic/Hebrew for exactly that
				// reason).
				if _, err := MeasureText(text, style); err != nil {
					t.Fatal(err)
				}
				return time.Since(start)
			}
			tSmall := timeFor(small)
			tLarge := timeFor(large)
			// O(n): 4x input -> ~4x time. O(n^2): 4x input -> ~16x time.
			// A generous 8x ceiling comfortably separates the two without
			// being sensitive to machine noise on the (already small)
			// absolute durations.
			if tSmall > 0 && tLarge > 8*tSmall {
				t.Errorf("%s: time grew %vx (n=%d: %v, n=%d: %v) going from %d to %d marks — looks O(n²) again (GHSA-vj25-h6h5-3phj)",
					tc.name, float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge, small, large)
			}
		})
	}
}

// --- point 2: ligate / gsubMultiple ----------------------------------
//
// ligate and the *gsubMultiple case of applyGSUBSubtable each spliced
// s.glyphs immediately on every substitution event: ligate's trailing
// `append(out, s.glyphs[last+1:]...)` and gsubMultiple's deletion/growth
// splices all copy the ENTIRE remaining tail of the buffer, which is
// O(remaining length) per call — O(n) events across a buffer of size
// O(n) (a long run of ligature-triggering text, or a lookup that deletes
// or expands many glyphs) therefore cost O(n²) overall. Both are now
// deferred: a tombstone flag (shapedGlyph.deleted) marks a consumed
// glyph instead of removing it, a growing substitution queues its extra
// glyphs in otShaper.pendingInserts instead of splicing them in, and
// applyLookup's compactGSUB rebuilds s.glyphs once, in one O(n) pass,
// after the whole lookup's forward scan finishes — not once per event.

// TestShapeManyLigaturesStaysFast reproduces the advisory's point-2
// repro shape (many ligature-triggering pairs in one run) and asserts
// growth, not an absolute bound: doubling the input must not roughly
// quadruple the time.
func TestShapeManyLigaturesStaysFast(t *testing.T) {
	f := loadFontFile(t, "testdata/DejaVuSans.ttf")

	const small, large = 4000, 16000 // pairs, so 8000/32000 runes
	timeFor := func(n int) time.Duration {
		text := strings.Repeat("fi", n)
		start := time.Now()
		shapeRun(f, text, false)
		return time.Since(start)
	}
	tSmall := timeFor(small)
	tLarge := timeFor(large)
	// O(n): 4x input -> ~4x time. O(n^2): 4x input -> ~16x time.
	if tSmall > 0 && tLarge > 8*tSmall {
		t.Errorf("time grew %vx (n=%d pairs: %v, n=%d pairs: %v) — looks O(n²) again (GHSA-vj25-h6h5-3phj)",
			float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge)
	}
}

// TestShapeManyLigaturesProducesCorrectGlyphs shapes a long run of "fi"
// pairs and checks the tombstone+defer rewrite still produces exactly
// what immediate splicing did: one ligature glyph per pair, a distinct
// monotonically-increasing ligID per ligation (confirming each pair was
// ligated exactly once, not skipped or double-counted by the deferred
// bookkeeping), and — critically — no tombstoned glyph left behind by a
// missed compactGSUB call.
func TestShapeManyLigaturesProducesCorrectGlyphs(t *testing.T) {
	f := loadFontFile(t, "testdata/DejaVuSans.ttf")
	const n = 500
	shaped := shapeRun(f, strings.Repeat("fi", n), false)

	if len(shaped) != n {
		t.Fatalf("%d \"fi\" pairs shaped to %d glyphs, want %d (one ligature per pair)", n, len(shaped), n)
	}
	fiGID := shapeRun(f, "fi", false)[0].gid
	var lastLigID uint32
	for i, g := range shaped {
		if g.deleted {
			t.Fatalf("glyph %d: tombstoned glyph survived compactGSUB", i)
		}
		if g.gid != fiGID {
			t.Fatalf("glyph %d: gid %d, want the fi ligature glyph %d", i, g.gid, fiGID)
		}
		if g.ligID <= lastLigID {
			t.Fatalf("glyph %d: ligID %d not greater than previous %d — ligCounter bookkeeping broke under deferred compaction", i, g.ligID, lastLigID)
		}
		lastLigID = g.ligID
	}
}

// referenceGsubMultipleAll mimics applyLookup's exact forward-scan
// driver (`if next > i { i = next } else { i++ }`) but calls the
// ORIGINAL, immediately-splicing gsubMultiple handling instead of the
// tombstone+defer rewrite — a reference oracle for the random
// differential test below.
func referenceGsubMultipleAll(glyphs []shapedGlyph, delGID, growGID uint16, growSeq []uint16) []shapedGlyph {
	i := 0
	for i < len(glyphs) {
		switch glyphs[i].gid {
		case delGID:
			glyphs = append(glyphs[:i], glyphs[i+1:]...)
			next := i // the original returned i; applyLookup's own
			if next > i {
				i = next
			} else {
				i++
			}
		case growGID:
			extra := make([]shapedGlyph, len(growSeq)-1)
			for k := range extra {
				extra[k] = glyphs[i]
				extra[k].gid = growSeq[k+1]
				extra[k].text = ""
			}
			glyphs[i].gid = growSeq[0]
			rest := append(extra, glyphs[i+1:]...)
			glyphs = append(glyphs[:i+1], rest...)
			next := i + len(growSeq)
			if next > i {
				i = next
			} else {
				i++
			}
		default:
			i++
		}
	}
	return glyphs
}

// TestGsubMultipleMatchesReferenceOnRandomInput differentially tests the
// tombstone+defer rewrite (driven through the real applyLookup entry
// point, exactly as production code calls it) against the original
// immediate-splice behavior, across randomized sequences mixing
// deletion triggers, growth triggers, and plain glyphs — including
// triggers at the very start/end and a growth immediately followed by
// another trigger.
//
// The generator deliberately never places a deletion trigger immediately
// before another trigger. The *original* code has a pre-existing, narrow
// correctness quirk there, independent of this performance fix: its
// zero-length-sequence branch returns i, and applyLookup's own driver
// (`if next > i {...} else { i++ }`) treats a returned i exactly like "no
// match" and does i++ regardless — skipping, without ever checking, the
// glyph that the deletion's own array shift just moved into the
// now-vacated slot. So of several back-to-back deletion-triggering
// glyphs, only every other one actually got removed. The tombstone+defer
// rewrite never shifts anything mid-scan, so every original position is
// examined exactly once no matter how many of its neighbors were also
// tombstoned — which incidentally fixes that quirk rather than
// reproducing it. TestGsubMultipleConsecutiveDeletionsAllRemoved below
// pins that (intentional, documented) behavior change directly; this
// test's generator sidesteps it so it is comparing the new code against
// the cases where the two are meant to agree exactly.
func TestGsubMultipleMatchesReferenceOnRandomInput(t *testing.T) {
	const delGID, growGID uint16 = 900, 901
	growSeq := []uint16{901, 902, 903}
	cov := otCoverage{glyphs: []uint16{delGID, growGID}}
	sub := &gsubMultiple{cov: cov, seqs: [][]uint16{{}, growSeq}}
	lookup := &otLookup{kind: 2, subtables: []any{sub}}

	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 300; trial++ {
		n := rng.Intn(30) + 1
		glyphs := make([]shapedGlyph, n)
		prevWasDel := false
		for i := range glyphs {
			choice := rng.Intn(4)
			if prevWasDel && choice <= 1 {
				choice = 2 // force plain right after a deletion trigger
			}
			switch choice {
			case 0:
				glyphs[i] = shapedGlyph{gid: delGID, advance: 50, mask: otMaskGlobal, text: "d"}
				prevWasDel = true
				continue
			case 1:
				glyphs[i] = shapedGlyph{gid: growGID, advance: 50, mask: otMaskGlobal, text: "g"}
			default:
				glyphs[i] = shapedGlyph{gid: uint16(10 + i), advance: 50, mask: otMaskGlobal, text: "p"}
			}
			prevWasDel = false
		}

		want := referenceGsubMultipleAll(append([]shapedGlyph(nil), glyphs...), delGID, growGID, growSeq)

		s := &otShaper{
			glyphs: append([]shapedGlyph(nil), glyphs...),
			table:  &otLayoutTable{lookups: []*otLookup{lookup}},
		}
		s.applyLookup(otPlanLookup{index: 0, mask: otMaskGlobal}, true)
		got := s.glyphs

		if len(got) != len(want) {
			t.Fatalf("trial %d (n=%d): got %d glyphs, want %d", trial, n, len(got), len(want))
		}
		for i := range want {
			if got[i].deleted {
				t.Fatalf("trial %d: glyph %d: tombstoned glyph survived compactGSUB", trial, i)
			}
			if got[i].gid != want[i].gid || got[i].text != want[i].text {
				t.Fatalf("trial %d (n=%d): glyph %d: got (gid=%d,text=%q), want (gid=%d,text=%q)",
					trial, n, i, got[i].gid, got[i].text, want[i].gid, want[i].text)
			}
		}
	}
}

// TestGsubMultipleDeletionHeavyStaysFast isolates gsubMultiple's own
// quadratic hot spot (deletion-heavy content) from ligate's, timing
// applyLookup directly against a buffer that is almost entirely
// deletion triggers.
func TestGsubMultipleDeletionHeavyStaysFast(t *testing.T) {
	const delGID uint16 = 900
	cov := otCoverage{glyphs: []uint16{delGID}}
	sub := &gsubMultiple{cov: cov, seqs: [][]uint16{{}}}
	lookup := &otLookup{kind: 2, subtables: []any{sub}}

	const small, large = 4000, 16000
	timeFor := func(n int) time.Duration {
		glyphs := make([]shapedGlyph, n)
		for i := range glyphs {
			glyphs[i] = shapedGlyph{gid: delGID, advance: 50, mask: otMaskGlobal}
		}
		s := &otShaper{glyphs: glyphs, table: &otLayoutTable{lookups: []*otLookup{lookup}}}
		start := time.Now()
		s.applyLookup(otPlanLookup{index: 0, mask: otMaskGlobal}, true)
		return time.Since(start)
	}
	tSmall := timeFor(small)
	tLarge := timeFor(large)
	if tSmall > 0 && tLarge > 8*tSmall {
		t.Errorf("time grew %vx (n=%d: %v, n=%d: %v) — looks O(n²) again (GHSA-vj25-h6h5-3phj)",
			float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge)
	}
}

// TestGsubMultipleConsecutiveDeletionsAllRemoved pins a deliberate
// behavior change over the original code, found while building the
// random differential test above: several back-to-back zero-length-
// sequence deletion matches used to leave every other one behind,
// because the original code's deletion branch returned i and
// applyLookup's driver (`if next > i {...} else { i++ }`) treats a
// returned i exactly like "no match", advancing past — without ever
// examining — the glyph that the deletion's own array shift had just
// moved into the now-vacated slot. The tombstone+defer rewrite never
// shifts s.glyphs mid-scan, so it has no such blind spot: every original
// position is visited exactly once regardless of how many of its
// neighbors were also tombstoned, so a run of N deletion triggers now
// removes all N instead of only every other one. This is an accepted
// side effect of removing the quadratic cost (GHSA-vj25-h6h5-3phj /
// pdf-go-sv5t point 2), not a goal in itself — the original defect was
// narrow enough (it needs several *separate* coverage matches that each
// individually resolve to an empty sequence, sitting immediately next to
// each other) that it was never caught before, but there is no reason to
// deliberately preserve it once the rewrite no longer has the mechanism
// that produced it.
func TestGsubMultipleConsecutiveDeletionsAllRemoved(t *testing.T) {
	const delGID uint16 = 900
	cov := otCoverage{glyphs: []uint16{delGID}}
	sub := &gsubMultiple{cov: cov, seqs: [][]uint16{{}}}
	lookup := &otLookup{kind: 2, subtables: []any{sub}}

	glyphs := []shapedGlyph{
		{gid: 10, advance: 50, mask: otMaskGlobal, text: "A"},
		{gid: delGID, advance: 50, mask: otMaskGlobal, text: "d"},
		{gid: delGID, advance: 50, mask: otMaskGlobal, text: "d"},
		{gid: delGID, advance: 50, mask: otMaskGlobal, text: "d"},
		{gid: 11, advance: 50, mask: otMaskGlobal, text: "B"},
	}
	s := &otShaper{glyphs: glyphs, table: &otLayoutTable{lookups: []*otLookup{lookup}}}
	s.applyLookup(otPlanLookup{index: 0, mask: otMaskGlobal}, true)

	var gids []uint16
	for _, g := range s.glyphs {
		if g.deleted {
			t.Fatalf("tombstoned glyph survived compactGSUB: %+v", g)
		}
		gids = append(gids, g.gid)
	}
	want := []uint16{10, 11}
	if len(gids) != len(want) {
		t.Fatalf("got %v, want %v — all three consecutive deletion triggers should be removed", gids, want)
	}
	for i := range want {
		if gids[i] != want[i] {
			t.Fatalf("got %v, want %v", gids, want)
		}
	}
}

// TestAddTextManyMarksStaysFast is the end-to-end AddText counterpart
// (the advisory's own observation that AddText is hit harder than
// MeasureText, since it reshapes each line more than once).
func TestAddTextManyMarksStaysFast(t *testing.T) {
	doc := NewDocument(595, 842)
	data, err := readFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	f, err := doc.LoadFontFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	const small, large = 4000, 16000
	timeFor := func(n int) time.Duration {
		text := "x" + strings.Repeat("́", n)
		start := time.Now()
		if err := p.AddText(text, TextStyle{Font: f, Size: 10}, Rectangle{LLX: 0, LLY: 0, URX: 10000, URY: 50}); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	tSmall := timeFor(small)
	tLarge := timeFor(large)
	if tSmall > 0 && tLarge > 8*tSmall {
		t.Errorf("time grew %vx (n=%d: %v, n=%d: %v) — looks O(n²) again (GHSA-vj25-h6h5-3phj)",
			float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge)
	}
}
