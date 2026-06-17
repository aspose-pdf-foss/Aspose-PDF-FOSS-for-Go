// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// countLinks returns how many Link annotations are on the page.
func countLinks(p *pdf.Page) int {
	n := 0
	all := p.Annotations().All()
	for _, a := range all {
		if a.AnnotationType() == pdf.AnnotationTypeLink {
			n++
		}
	}
	return n
}

func TestAddTOCSuppliedList(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	mustNoErr(t, doc.AddBlankPage(595, 842))
	mustNoErr(t, doc.AddBlankPage(595, 842))
	p1, _ := doc.Page(1)
	p2, _ := doc.Page(2)
	p3, _ := doc.Page(3)

	tocPage, _ := doc.Page(1)
	entries := []pdf.TOCEntry{
		{Title: "Introduction", Level: 0, Page: p1},
		{Title: "Background and prior work", Level: 1, Page: p2},
		{Title: "Conclusion", Level: 0, Page: p3},
		{Title: "Unlinked note", Level: 0}, // no page → no link/number
	}
	added, err := tocPage.AddTOC(entries, pdf.Rectangle{LLX: 50, LLY: 50, URX: 545, URY: 780},
		pdf.TOCOptions{Title: "Table of Contents"})
	if err != nil {
		t.Fatalf("AddTOC: %v", err)
	}
	if added != 0 {
		t.Errorf("pagesAdded = %d, want 0 (fits on one page)", added)
	}
	// 3 entries have pages → 3 links.
	if got := countLinks(tocPage); got != 3 {
		t.Errorf("link count = %d, want 3", got)
	}

	// Round-trip: the first link must navigate to page 1.
	doc2 := reopen(t, doc)
	tp, _ := doc2.Page(1)
	var firstLink *pdf.LinkAnnotation
	for _, a := range tp.Annotations().All() {
		if l, ok := a.(*pdf.LinkAnnotation); ok {
			firstLink = l
			break
		}
	}
	if firstLink == nil {
		t.Fatal("no link annotation after round-trip")
	}
	if act, ok := firstLink.Action().(*pdf.GoToAction); !ok || act.PageNum() < 1 {
		t.Errorf("first link action = %v, want a GoTo with a page", firstLink.Action())
	}
}

func TestAddTOCPaginates(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	target, _ := doc.Page(1)
	var entries []pdf.TOCEntry
	for i := 0; i < 60; i++ {
		entries = append(entries, pdf.TOCEntry{Title: "Entry that takes a whole line", Page: target})
	}
	tocPage, _ := doc.Page(1)
	// Small rect so 60 entries cannot fit on one page.
	added, err := tocPage.AddTOC(entries, pdf.Rectangle{LLX: 50, LLY: 400, URX: 545, URY: 780})
	if err != nil {
		t.Fatalf("AddTOC: %v", err)
	}
	if added < 1 {
		t.Errorf("pagesAdded = %d, want >= 1 (overflow)", added)
	}
}

func TestGenerateTOCFromOutlines(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	mustNoErr(t, doc.AddBlankPage(595, 842))
	mustNoErr(t, doc.AddBlankPage(595, 842))
	mustNoErr(t, doc.AddBlankPage(595, 842))
	p1, _ := doc.Page(1)
	p3, _ := doc.Page(3)

	root := doc.Outlines()
	ch1 := pdf.NewOutlineItemCollection(doc)
	ch1.SetTitle("Chapter 1")
	ch1.SetDestination(pdf.NewDestinationFit(p1))
	mustNoErr(t, root.Add(ch1))
	sub := pdf.NewOutlineItemCollection(doc)
	sub.SetTitle("Section 1.1")
	sub.SetDestination(pdf.NewDestinationFit(p3))
	mustNoErr(t, ch1.Add(sub))
	ch2 := pdf.NewOutlineItemCollection(doc)
	ch2.SetTitle("Chapter 2")
	ch2.SetDestination(pdf.NewDestinationFit(p3))
	mustNoErr(t, root.Add(ch2))

	before := doc.PageCount()
	added, err := doc.GenerateTOC(pdf.TOCOptions{Title: "Contents"})
	if err != nil {
		t.Fatalf("GenerateTOC: %v", err)
	}
	if added < 1 {
		t.Fatalf("pagesAdded = %d, want >= 1", added)
	}
	if doc.PageCount() != before+added {
		t.Errorf("PageCount = %d, want %d", doc.PageCount(), before+added)
	}

	// The TOC page (now page 1) carries 3 links (one per outline item).
	toc, _ := doc.Page(1)
	if got := countLinks(toc); got != 3 {
		t.Errorf("TOC link count = %d, want 3", got)
	}

	// Round-trip and verify Chapter 1's link targets the shifted page.
	// Original page 1 is now page (1 + added).
	doc2 := reopen(t, doc)
	toc2, _ := doc2.Page(1)
	links := toc2.Annotations().All()
	var first *pdf.GoToAction
	for _, a := range links {
		if l, ok := a.(*pdf.LinkAnnotation); ok {
			if gt, ok := l.Action().(*pdf.GoToAction); ok {
				first = gt
				break
			}
		}
	}
	if first == nil {
		t.Fatal("no GoTo link after round-trip")
	}
	if first.PageNum() != 1+added {
		t.Errorf("Chapter 1 link PageNum = %d, want %d (original page 1 shifted by %d TOC pages)",
			first.PageNum(), 1+added, added)
	}
}

func TestGenerateTOCNoOutlines(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	added, err := doc.GenerateTOC()
	if err != nil {
		t.Fatalf("GenerateTOC: %v", err)
	}
	if added != 0 {
		t.Errorf("pagesAdded = %d, want 0 (no outlines)", added)
	}
	if doc.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1 (unchanged)", doc.PageCount())
	}
}
