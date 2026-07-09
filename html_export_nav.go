// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"html"
	"strings"
)

// Outline navigation sidebar for the HTML exporter (pdf-go-uhfs): with
// HTMLSaveOptions.OutlineNav, the document's outline (bookmark) tree is
// emitted as a fixed <nav> sidebar of links to the #pageN anchors (or to
// the sibling files of a split export). Nesting is collapsible with pure
// HTML <details>/<summary> — no JavaScript, in keeping with the exporter.
// Neither Aspose.PDF for .NET nor pdf2htmlEX renders bookmarks in HTML.

// htmlOutlineNav renders the sidebar, or "" when the document has no
// outlines (the option is then a no-op).
func htmlOutlineNav(d *Document, pageHref func(int) string) string {
	root := d.Outlines()
	if root.Count() == 0 {
		return ""
	}
	if pageHref == nil {
		pageHref = func(n int) string { return fmt.Sprintf("#page%d", n) }
	}
	var b strings.Builder
	b.WriteString("<nav class=\"nv\">\n")
	writeNavLevel(&b, root, pageHref, 0)
	b.WriteString("</nav>\n")
	return b.String()
}

// writeNavLevel emits one <ul> level of the outline tree. The first two
// levels start expanded; deeper ones start collapsed.
func writeNavLevel(b *strings.Builder, coll *OutlineItemCollection, pageHref func(int) string, depth int) {
	if depth > 32 {
		return // cycle guard for hostile files
	}
	b.WriteString("<ul>\n")
	for _, item := range coll.All() {
		label := html.EscapeString(item.Title())
		if n := outlineItemPage(item); n >= 1 {
			label = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(pageHref(n)), label)
		}
		if item.Count() > 0 {
			open := ""
			if depth < 2 {
				open = " open"
			}
			fmt.Fprintf(b, "<li><details%s><summary>%s</summary>\n", open, label)
			writeNavLevel(b, item, pageHref, depth+1)
			b.WriteString("</details></li>\n")
		} else {
			fmt.Fprintf(b, "<li>%s</li>\n", label)
		}
	}
	b.WriteString("</ul>\n")
}

// outlineItemPage resolves an outline item to a 1-based page number: the
// /Dest destination first (per ISO 32000-1 §12.3.3 viewers honor it over
// /A), then a GoTo action. 0 = no page target.
func outlineItemPage(item *OutlineItemCollection) int {
	if dest := item.Destination(); dest != nil {
		if p := dest.Page(); p != nil {
			return p.Number()
		}
	}
	if act, ok := item.Action().(*GoToAction); ok && act.PageNum() >= 1 {
		return act.PageNum()
	}
	return 0
}
