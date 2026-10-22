// SPDX-License-Identifier: MIT

package asposepdf

import (
	"runtime"
	"strings"
	"testing"
)

// pdf-go-ifpy point 3: the q/Q snapshots in the redaction text rewriter,
// the Do-rewriter and the text extractor used to copy the whole fontInfo
// (~3 KB: two 256-entry arrays) on every "q". A content stream that is
// nothing but unmatched "q" operators — two bytes each, compressing to
// almost nothing — therefore cost ~3 KB of live memory per operator, and
// ~17 KB of allocation per operator once the snapshot slice's growth
// copies are counted (measured: 17 386 bytes/q before, ~900 after). The
// snapshots now hold a pointer to one shared fontInfo per font.

// floodOfQ is a content stream selecting a font and then opening n
// graphics-state levels that are never closed.
func floodOfQ(n int) []byte {
	return []byte("BT /F1 12 Tf ET\n" + strings.Repeat("q ", n))
}

// bytesPerOp reports the bytes allocated per "q" by f over a flood of n.
func bytesPerOp(n int, f func(data []byte)) float64 {
	data := floodOfQ(n)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f(data)
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(n)
}

// The ceiling sits well below sizeof(fontInfo) (~3.1 KB), so a snapshot
// that copies the font again fails it, while the parsed op and the small
// snapshot struct themselves fit comfortably.
const maxBytesPerQ = 1500

func TestRedactRewriterQFloodDoesNotCopyFontInfo(t *testing.T) {
	fonts := testRewriteFontMap()
	regions := []QuadPoint{{X1: 0, Y1: 10, X2: 10, Y2: 10, X3: 0, Y3: 0, X4: 10, Y4: 0}}
	got := bytesPerOp(20000, func(data []byte) {
		if _, err := rewriteTextOperatorsInStream(data, regions, fonts, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("%.0f bytes per q", got)
	if got > maxBytesPerQ {
		t.Errorf("redaction rewriter allocates %.0f bytes per unmatched q, want <= %d — q is copying fontInfo again", got, maxBytesPerQ)
	}
}

// The Do-rewriter tracks the same text state (to hand it to a Form
// XObject that inherits its font, pdf-go-w7ye) and had the same copy.
func TestDoRewriterQFloodDoesNotCopyFontInfo(t *testing.T) {
	regions := []QuadPoint{{X1: 0, Y1: 10, X2: 10, Y2: 10, X3: 0, Y3: 0, X4: 10, Y4: 0}}
	got := bytesPerOp(20000, func(data []byte) {
		if _, err := rewriteXObjectDoOperatorsInStream(data, regions, nil, nil, 0, &formRedactBudget{}, nil, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("%.0f bytes per q", got)
	if got > maxBytesPerQ {
		t.Errorf("Do rewriter allocates %.0f bytes per unmatched q, want <= %d — q is copying fontInfo again", got, maxBytesPerQ)
	}
}

func TestExtractorQFloodDoesNotCopyFontInfo(t *testing.T) {
	fonts := testRewriteFontMap()
	got := bytesPerOp(20000, func(data []byte) {
		ops, err := parseContentStream(data)
		if err != nil {
			t.Fatal(err)
		}
		newTextExtractor(nil, fonts).process(ops, nil)
	})
	t.Logf("%.0f bytes per q", got)
	if got > maxBytesPerQ {
		t.Errorf("text extractor allocates %.0f bytes per unmatched q, want <= %d — q is copying fontInfo again", got, maxBytesPerQ)
	}
}
