// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

// pdf-go-velp / GHSA-7jjg-q5vf-8592: AddText with an embedded TrueType
// font took time quadratic in the number of CONSECUTIVE combining marks
// — a text value of only a few kilobytes could stall shaping for
// seconds. Two independent O(n²) hot spots, both fixed:
//
//   - canonicalOrder (combining.go) was an insertion sort over each
//     maximal run of marks.
//   - prevBase (shape.go) walked backward from every mark to its base
//     glyph, rescanning every earlier mark each time.

// referenceCanonicalOrder is the ORIGINAL O(n²) insertion-sort
// implementation, kept here only as a differential-testing oracle: any
// stable sort by the same total order (shaping class) as this one is
// mathematically guaranteed to produce the identical permutation
// (stability pins equal-class relative order; every other pair's order
// is fully determined by comparing classes either way), so agreement
// here across many random inputs is strong evidence the O(n log n)
// rewrite in combining.go is behavior-preserving, not just fast.
func referenceCanonicalOrder(runes []rune) {
	for i := 1; i < len(runes); i++ {
		c := shapingClass(runes[i])
		if c == 0 {
			continue
		}
		for j := i; j > 0; j-- {
			pc := shapingClass(runes[j-1])
			if pc == 0 || pc <= c {
				break
			}
			runes[j], runes[j-1] = runes[j-1], runes[j]
		}
	}
}

// TestCanonicalOrderMatchesReferenceOnRandomInput differentially tests
// the rewritten canonicalOrder against the original insertion sort
// across many random rune sequences mixing fixed points (class 0) and
// marks of several different classes, including multiple separate runs
// in one sequence.
func TestCanonicalOrderMatchesReferenceOnRandomInput(t *testing.T) {
	// A handful of runes spanning distinct shaping classes, plus a
	// class-0 "anchor" rune, matching the Hebrew/Arabic cases
	// TestCanonicalOrder (shape_internal_test.go) already pins.
	pool := []rune{'a', 0x05B0, 0x05BC, 0x0628, 0x064E, 0x0651, 0x0301, 0x0316, 0x05B4}
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 500; trial++ {
		n := rng.Intn(30)
		seq := make([]rune, n)
		for i := range seq {
			seq[i] = pool[rng.Intn(len(pool))]
		}
		got := append([]rune(nil), seq...)
		want := append([]rune(nil), seq...)
		canonicalOrder(got)
		referenceCanonicalOrder(want)
		if string(got) != string(want) {
			t.Fatalf("trial %d: input %U\ngot  %U\nwant %U", trial, seq, got, want)
		}
	}
}

// TestCanonicalOrderStaysFast reproduces the canonicalOrder half of the
// advisory directly (no font/shaping overhead): a long run of marks in
// descending-class order forces the maximum amount of insertion-sort
// bubbling in the original algorithm. A loose timing bound, matching
// the style of TestOptimizeManyAddTextCallsStaysFast (optimize_test.go)
// — on the machine this was written on, the fix measures low
// milliseconds for n=60000 and the regressed O(n²) code would take
// minutes (extrapolating the advisory's own 16000-mark/7.9s datapoint).
func TestCanonicalOrderStaysFast(t *testing.T) {
	const n = 60000
	runes := make([]rune, n)
	runes[0] = 'a'
	// Two alternating classes, assigned so the run is NOT already
	// sorted (worst case for an insertion sort: every mark must bubble
	// past every other mark of the opposite class ahead of it).
	for i := 1; i < n; i++ {
		if i%2 == 0 {
			runes[i] = 0x0301 // class 230
		} else {
			runes[i] = 0x0316 // class 220
		}
	}
	start := time.Now()
	canonicalOrder(runes)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("canonicalOrder took %v for %d runes, want well under 3s — looks O(n²) again (GHSA-7jjg-q5vf-8592)", elapsed, n)
	}
}

// TestPrevBaseCacheMatchesNaiveScan differentially tests
// buildPrevBaseCache's O(n) answer against a direct call to the ORIGINAL
// O(n) backward-scanning matcher (otMatcher{flag: otFlagIgnoreMarks}.prev,
// the pre-fix implementation of prevBase itself) at every position, over
// many random isMark patterns including runs of several consecutive
// marks — the case that actually requires skipping past more than one
// mark to reach the real base, which a single-isolated-mark-per-base
// test (where the immediately preceding glyph already happens to be the
// base) would never exercise. Built directly from a synthetic
// []shapedGlyph (no real font/GDEF needed — glyphClass falls back to
// isMark when gdef is nil), so this isolates the cache's own correctness
// from any particular font's anchor/typography data.
func TestPrevBaseCacheMatchesNaiveScan(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 200; trial++ {
		n := rng.Intn(40)
		glyphs := make([]shapedGlyph, n)
		for i := range glyphs {
			glyphs[i].isMark = rng.Intn(3) == 0 // marks cluster some of the time
		}
		s := &otShaper{glyphs: glyphs}
		naive := otMatcher{s: s, flag: otFlagIgnoreMarks}

		for i := 0; i < n; i++ {
			want := naive.prev(i)
			got := s.prevBase(i)
			if got != want {
				t.Fatalf("trial %d, i=%d: prevBase = %d, want %d (naive scan) — isMark pattern %v",
					trial, i, got, want, markPattern(glyphs))
			}
		}
	}
}

func markPattern(glyphs []shapedGlyph) []bool {
	out := make([]bool, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.isMark
	}
	return out
}

// TestShapeLineLongCombiningMarkRunStaysFast reproduces the advisory's
// own end-to-end repro directly against shapeLine (what AddText calls
// for an embedded font with GSUB/GPOS tables), covering BOTH hot spots
// together — canonicalOrder during normalization, and prevBase during
// GPOS mark-attachment positioning for all 16,000 marks.
func TestShapeLineLongCombiningMarkRunStaysFast(t *testing.T) {
	f := loadFontFile(t, "testdata/DejaVuSans.ttf")
	const n = 16000
	text := "a" + strings.Repeat("́", n) + strings.Repeat("̖", n)

	start := time.Now()
	shapeLine(f, text, 0, false)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("shapeLine took %v for a + %d + %d combining marks, want well under 3s "+
			"(the advisory measured 7.9s at this exact size before the fix) — GHSA-7jjg-q5vf-8592", elapsed, n, n)
	}
}

// BenchmarkShapeLineLongCombiningMarkRun is
// TestShapeLineLongCombiningMarkRunStaysFast's precise-timing
// counterpart: run with
// `go test -bench BenchmarkShapeLineLongCombiningMarkRun -benchtime=5x -run NONE .`
// to see the actual speedup (the advisory measured 7.9s per call at this
// exact size before the fix).
func BenchmarkShapeLineLongCombiningMarkRun(b *testing.B) {
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		b.Skipf("font not available: %v", err)
	}
	f, err := parseSFNTAt(data, 0)
	if err != nil {
		b.Fatal(err)
	}
	const n = 16000
	text := "a" + strings.Repeat("́", n) + strings.Repeat("̖", n)
	for i := 0; i < b.N; i++ {
		shapeLine(f, text, 0, false)
	}
}
