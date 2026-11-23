// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
)

// GHSA-gc9q-3vx8-phx2 point 1: clipped extraction copied a used
// strings.Builder by value, leaving the heap pointing into a dead stack
// frame; with the garbage collector running often, the process stopped with
// "found bad pointer in Go heap". An ordinary page with many fragments.
func TestClippedExtractionSurvivesGC(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 1300; i++ {
		fmt.Fprintf(&b, "BT /F1 4 Tf %d %d Td (p%d word) Tj ET\n", 10+(i%20)*28, 10+(i/20)*12, i)
	}
	d := NewDocument(595, 842)
	p := mustPage(t, d)
	if _, err := p.ensureStandardFontResource("/Helvetica"); err != nil {
		t.Fatal(err)
	}
	if err := p.appendToContentStream([]byte(strings.ReplaceAll(b.String(), "/F1", "/F0"))); err != nil {
		t.Fatal(err)
	}
	p = mustPage(t, reopenDoc(t, d))
	defer debug.SetGCPercent(debug.SetGCPercent(1))
	for i := 0; i < 60; i++ {
		txt, err := p.ExtractText(TextExtractOptions{LimitToPageBounds: true})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(txt, "p1299 word") {
			t.Fatalf("text lost: %.60q", txt)
		}
		if _, err := p.ExtractText(TextExtractOptions{Rectangle: &Rectangle{0, 0, 300, 400}}); err != nil {
			t.Fatal(err)
		}
	}
}
