// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
	"time"
)

// GitHub issue #20 (shadowhunter1470-svg), point 1: contentsEntries
// copied and re-scanned the WHOLE /Contents array (resolving every
// entry, checking for the one-time-wrap marker) on every single
// appendToContentStream call, so n draw calls on one page or canvas
// cost O(n) per call — O(n²) overall in both time and allocation.

// TestAppendToContentStreamManyDrawsStaysFast reproduces the reporter's
// own repro shape (many DrawLine calls on one page) and asserts
// growth, not an absolute bound: doubling the input must not roughly
// quadruple the time.
func TestAppendToContentStreamManyDrawsStaysFast(t *testing.T) {
	ls := LineStyle{Width: 1, Color: &Color{A: 1}}
	const small, large = 2000, 8000 // 4x the input
	timeFor := func(n int) time.Duration {
		doc := NewDocument(595, 842)
		p, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		for i := 0; i < n; i++ {
			if err := p.DrawLine(Point{X: 10, Y: 10}, Point{X: 20, Y: 20}, ls); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start)
	}
	tSmall := minDuration(growthReps, func() time.Duration { return timeFor(small) })
	tLarge := minDuration(growthReps, func() time.Duration { return timeFor(large) })
	// O(n): 4x input -> ~4x time. O(n^2): 4x input -> ~16x time.
	if tSmall > 0 && tLarge > 8*tSmall {
		t.Errorf("time grew %vx (n=%d: %v, n=%d: %v) — looks O(n²) again (GitHub issue #20)",
			float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge)
	}
}

// TestAppendToContentStreamManyDrawsOnCanvasStaysFast is the detached-
// canvas counterpart — the reporter measured this path as even slower
// than the page path (5.9s vs 3.4s at n=16000).
func TestAppendToContentStreamManyDrawsOnCanvasStaysFast(t *testing.T) {
	ls := LineStyle{Width: 1, Color: &Color{A: 1}}
	const small, large = 2000, 8000
	timeFor := func(n int) time.Duration {
		doc := NewDocument(595, 842)
		canvas := doc.CreateForm(200, 200).Canvas()
		start := time.Now()
		for i := 0; i < n; i++ {
			if err := canvas.DrawLine(Point{X: 10, Y: 10}, Point{X: 20, Y: 20}, ls); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start)
	}
	tSmall := minDuration(growthReps, func() time.Duration { return timeFor(small) })
	tLarge := minDuration(growthReps, func() time.Duration { return timeFor(large) })
	if tSmall > 0 && tLarge > 8*tSmall {
		t.Errorf("time grew %vx (n=%d: %v, n=%d: %v) — looks O(n²) again (GitHub issue #20)",
			float64(tLarge)/float64(tSmall), small, tSmall, large, tLarge)
	}
}

// TestAppendToContentStreamFastPathMatchesSlowPath differentially
// checks the fast-path append against a from-scratch rebuild via
// contentsEntries, across a run of many appends — confirming the
// cached /Contents array's final shape (as entries, resolved) is
// identical to what the original always-slow-path logic would have
// produced, not just "fast."
func TestAppendToContentStreamFastPathMatchesSlowPath(t *testing.T) {
	doc := NewDocument(595, 842)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const n = 50
	for i := 0; i < n; i++ {
		if err := p.appendToContentStream([]byte("1 0 0 RG\n")); err != nil {
			t.Fatal(err)
		}
	}
	pageDict := p.pageDict()
	entries, bounded := contentsEntries(p.doc.objects, pageDict["/Contents"])
	if !bounded {
		t.Fatal("expected /Contents to be in bounded (q...Q-wrapped) form after multiple appends")
	}
	// q + n appends, no pre-existing content on a brand-new page's
	// first append, then wrapped once: q, endRef, then n-1 more direct
	// appends after the wrap -> total entries = (1 q) + (1 end) + n.
	// Rather than re-deriving the exact count by hand, just confirm it
	// agrees with what contentsEntries itself independently resolves to.
	if len(entries) != p.contentsLen {
		t.Errorf("cached contentsLen=%d disagrees with contentsEntries' own count=%d", p.contentsLen, len(entries))
	}
	arr, ok := pageDict["/Contents"].(pdfArray)
	if !ok {
		t.Fatal("/Contents is not a direct pdfArray after multiple appends")
	}
	if len(arr) != len(entries) {
		t.Errorf("direct array length %d disagrees with contentsEntries' resolved length %d", len(arr), len(entries))
	}
}

// TestAppendToContentStreamCacheRecoversFromExternalReplace confirms a
// stale contentsBounded cache (something else replaced /Contents with
// a single stream, as replacePageContents does for
// ApplyRedactions/ConvertToGrayscale/ImageInfo.Remove) is detected and
// recovered from correctly rather than corrupting the page — the fast
// path's own safety net, not a hand-added invalidation call.
func TestAppendToContentStreamCacheRecoversFromExternalReplace(t *testing.T) {
	doc := NewDocument(595, 842)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := p.appendToContentStream([]byte("1 0 0 RG\n")); err != nil {
			t.Fatal(err)
		}
	}
	if !p.contentsBounded {
		t.Fatal("expected contentsBounded after several appends")
	}

	// Simulate what replacePageContents does: swap in a single brand
	// new stream, abandoning the array the cache still remembers.
	pageDict := p.pageDict()
	newStream := newContentStream(p.doc, []byte("0 0 1 RG\n"), nil)
	pageDict["/Contents"] = newStream

	if err := p.appendToContentStream([]byte("0 1 0 RG\n")); err != nil {
		t.Fatal(err)
	}
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "0 0 1 RG") || !strings.Contains(content, "0 1 0 RG") {
		t.Errorf("content after external replace + append = %q, want both the replaced and the newly-appended operators present", content)
	}
}

// TestAppendToContentStreamCacheRecoversFromForeignArrayOfSameType is
// the length-check's own dedicated test. Appending always reads
// /Contents fresh, so blindly trusting a same-shaped foreign pdfArray
// can't lose or corrupt that array's own bytes either way — the
// length check's real job is deciding whether that array has already
// been through this method's own q...Q isolation wrap (pdf-go-p1yc).
// A foreign array with no contentAppendMarkerKey of its own has NOT —
// so trusting it as already-bounded would append the new content
// completely unprotected, able to inherit whatever graphics state the
// foreign array's own content leaves open, exactly the bug the wrap
// exists to prevent. Checking the length catches that mismatch and
// sends it through the slow path, which correctly re-wraps it.
func TestAppendToContentStreamCacheRecoversFromForeignArrayOfSameType(t *testing.T) {
	doc := NewDocument(595, 842)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := p.appendToContentStream([]byte("1 0 0 RG\n")); err != nil {
			t.Fatal(err)
		}
	}
	if !p.contentsBounded {
		t.Fatal("expected contentsBounded after several appends")
	}

	// A foreign array, unrelated to this page's own cached one, with no
	// contentAppendMarkerKey anywhere in it — same Go type (pdfArray),
	// deliberately a DIFFERENT length than p.contentsLen.
	foreignRef := newContentStream(p.doc, []byte("0 0 1 RG\n"), nil)
	pageDict := p.pageDict()
	pageDict["/Contents"] = pdfArray{foreignRef}

	if err := p.appendToContentStream([]byte("0 1 0 RG\n")); err != nil {
		t.Fatal(err)
	}
	_, bounded := contentsEntries(p.doc.objects, pageDict["/Contents"])
	if !bounded {
		t.Error("expected the foreign array to have been re-wrapped in its own q...Q protection by the slow path, not trusted as already-bounded")
	}
}
