// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func runsMetrics(t *testing.T, runs []textRun) []runMetrics {
	t.Helper()
	ms := make([]runMetrics, len(runs))
	for i, r := range runs {
		m, err := metricsFor(r.style)
		if err != nil {
			t.Fatal(err)
		}
		ms[i] = m
	}
	return ms
}

func TestLayoutRunsCrossRunWord(t *testing.T) {
	// "unbreak" + "able" straddle a run border with no space: one cluster.
	runs := []textRun{
		{text: "aaa unbreak", style: TextStyle{Size: 12}},
		{text: "able zzz", style: TextStyle{Size: 12, Font: FontHelveticaBold}},
	}
	ms := runsMetrics(t, runs)
	// Width chosen so "aaa unbreakable" does not fit on one line.
	w := measureString("aaa unbreak", ms[0].width) + measureString("able", ms[1].width) - 1
	lines := layoutRuns(runs, ms, w)
	if len(lines) < 2 {
		t.Fatalf("lines = %d; want >= 2", len(lines))
	}
	first := lines[0]
	if len(first.segs) != 1 || first.segs[0].text != "aaa" {
		t.Errorf("line 1 = %+v; want just %q", first.segs, "aaa")
	}
	second := lines[1]
	if len(second.segs) != 2 || second.segs[0].text != "unbreak" || !strings.HasPrefix(second.segs[1].text, "able") {
		t.Errorf("line 2 = %+v; want the cross-run word intact", second.segs)
	}
	if second.segs[1].x <= second.segs[0].x {
		t.Errorf("segment x not advancing: %+v", second.segs)
	}
}

func TestLayoutRunsHardBreak(t *testing.T) {
	runs := []textRun{
		{text: "one", style: TextStyle{Size: 12}},
		{brk: true},
		{text: "two", style: TextStyle{Size: 12}},
	}
	ms := runsMetrics(t, runs)
	lines := layoutRuns(runs, ms, 500)
	if len(lines) != 2 {
		t.Fatalf("lines = %d; want 2 (hard break)", len(lines))
	}
}

func TestFlowRunsBaselineAndLinks(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	flow := doc.NewFlow(FlowOptions{})
	flow.addRuns([]textRun{
		{text: "Small before ", style: TextStyle{Size: 10}},
		{text: "BIG", style: TextStyle{Size: 24, Font: FontHelveticaBold}},
		{text: " and a ", style: TextStyle{Size: 10}},
		{text: "website link", style: TextStyle{Size: 10, Underline: true, Color: &Color{B: 0.8, A: 1}}, linkDest: "https://example.com/"},
	}, StructP)
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}

	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("extracted %d visual lines; want 1 (shared baseline)", len(lines))
	}
	text := lines[0].Text
	for _, want := range []string{"Small before", "BIG", "website link"} {
		if !strings.Contains(text, want) {
			t.Errorf("line text %q missing %q", text, want)
		}
	}

	// The link run must carry a real link annotation over its text.
	var link *LinkAnnotation
	for _, a := range page.Annotations().All() {
		if l, ok := a.(*LinkAnnotation); ok {
			link = l
		}
	}
	if link == nil {
		t.Fatal("no link annotation")
	}
	act, ok := link.Action().(*GoToURIAction)
	if !ok || act.URI() != "https://example.com/" {
		t.Fatalf("link action = %#v", link.Action())
	}
	matches, err := page.SearchText("website link")
	if err != nil || len(matches) != 1 {
		t.Fatalf("search: %v, %d matches", err, len(matches))
	}
	m := matches[0].Rect
	lr := link.Rect()
	if m.URX < lr.LLX || m.LLX > lr.URX || m.URY < lr.LLY || m.LLY > lr.URY {
		t.Errorf("annotation rect %+v does not intersect text %+v", lr, m)
	}
}

func TestFlowRunsPagination(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	flow := doc.NewFlow(FlowOptions{})
	word := strings.Repeat("word ", 60)
	for i := 0; i < 30; i++ {
		flow.addRuns([]textRun{
			{text: word, style: TextStyle{Size: 12}},
			{text: "tail" + word, style: TextStyle{Size: 12, Font: FontHelveticaOblique}},
		}, StructP)
	}
	pages, err := flow.Render()
	if err != nil {
		t.Fatal(err)
	}
	if pages < 2 {
		t.Fatalf("pages = %d; want pagination", pages)
	}
	if doc.PageCount() != pages {
		t.Errorf("PageCount = %d; Render reported %d", doc.PageCount(), pages)
	}
	// Content flowed onto the last page too.
	p, err := doc.Page(pages)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "word") {
		t.Errorf("last page has no flowed text")
	}
}
