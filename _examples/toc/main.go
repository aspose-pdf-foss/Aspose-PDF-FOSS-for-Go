// Builds a small multi-section report, adds outline bookmarks, then
// generates a linked table of contents from those bookmarks. The TOC is
// inserted as a new page at the front; every entry is clickable and shows
// the (post-insertion) page number with a dotted leader.
//
// Output: result_files/toc.pdf
package main

import (
	"fmt"
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)

	// Build a few content pages, each with a heading, and a bookmark
	// pointing at it. (Page 1 already exists; add four more.)
	sections := []struct {
		title string
		level int
	}{
		{"Introduction", 0},
		{"Motivation", 1},
		{"Methodology", 0},
		{"Results", 0},
		{"Conclusion", 0},
	}

	root := doc.Outlines()
	var lastTop *pdf.OutlineItemCollection
	for i, s := range sections {
		if i > 0 {
			if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
				log.Fatalf("add page: %v", err)
			}
		}
		page, _ := doc.Page(doc.PageCount())

		// Section heading on the page.
		if err := page.AddText(s.title, pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 22, Color: &pdf.Color{A: 1},
		}, pdf.Rectangle{LLX: 54, LLY: 760, URX: 541, URY: 800}); err != nil {
			log.Fatalf("heading: %v", err)
		}

		// Bookmark → page (nested under the previous top-level for level 1).
		item := pdf.NewOutlineItemCollection(doc)
		item.SetTitle(s.title)
		item.SetDestination(pdf.NewDestinationFit(page))
		if s.level == 0 {
			if err := root.Add(item); err != nil {
				log.Fatalf("outline add: %v", err)
			}
			lastTop = item
		} else if lastTop != nil {
			if err := lastTop.Add(item); err != nil {
				log.Fatalf("outline add child: %v", err)
			}
		}
	}

	added, err := doc.GenerateTOC(pdf.TOCOptions{Title: "Table of Contents"})
	if err != nil {
		log.Fatalf("GenerateTOC: %v", err)
	}

	if err := doc.Save("result_files/toc.pdf"); err != nil {
		log.Fatalf("save: %v", err)
	}
	fmt.Printf("wrote result_files/toc.pdf (%d TOC page(s), %d total)\n", added, doc.PageCount())
}
