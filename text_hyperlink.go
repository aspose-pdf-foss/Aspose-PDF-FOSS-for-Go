// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Hyperlink is where a text segment or fragment links to: a *WebHyperlink
// (a URI) or a *LocalHyperlink (a page of the same document). Drawing the
// text adds a link annotation over it. Mirrors Aspose.PDF for .NET's
// Hyperlink, WebHyperlink and LocalHyperlink (pdf-go-xe2v).
type Hyperlink interface {
	target() linkTarget
}

// WebHyperlink links to a URI.
type WebHyperlink struct {
	URL string
}

// NewWebHyperlink returns a link to url.
func NewWebHyperlink(url string) *WebHyperlink { return &WebHyperlink{URL: url} }

func (h *WebHyperlink) target() linkTarget {
	if h == nil || h.URL == "" {
		return linkTarget{}
	}
	return linkTarget{url: h.URL}
}

// LocalHyperlink links to a page of the same document. TargetPageNumber is
// 1-based; the link opens the page at its top.
type LocalHyperlink struct {
	TargetPageNumber int
}

// NewLocalHyperlink returns a link to page pageNum (1-based).
func NewLocalHyperlink(pageNum int) *LocalHyperlink { return &LocalHyperlink{TargetPageNumber: pageNum} }

func (h *LocalHyperlink) target() linkTarget {
	if h == nil || h.TargetPageNumber == 0 {
		return linkTarget{}
	}
	return linkTarget{page: h.TargetPageNumber}
}

// linkTarget is a hyperlink as a comparable value: a URI, a page, or
// neither. Snapshots record it so that an edit through the hyperlink's
// fields is seen as a change.
type linkTarget struct {
	url  string
	page int
}

func (l linkTarget) none() bool { return l == linkTarget{} }

// targetOf is h's target; a nil h (or a nil pointer in it) has none.
func targetOf(h Hyperlink) linkTarget {
	if h == nil {
		return linkTarget{}
	}
	return h.target()
}

// action is the link annotation's action for the target on d.
func (l linkTarget) action(d *Document) (Action, error) {
	if l.url != "" {
		return NewGoToURIAction(l.url), nil
	}
	if l.page < 1 || l.page > d.PageCount() {
		return nil, fmt.Errorf("hyperlink to page %d: the document has %d pages", l.page, d.PageCount())
	}
	top := 0.0
	if p, err := d.Page(l.page); err == nil {
		if box, err := p.CropBox(); err == nil {
			top = box.URY
		}
	}
	return NewGoToAction(l.page, top), nil
}

// linkOf is the hyperlink s is drawn with: its own, else the fragment's.
func (f *TextFragment) linkOf(s *TextSegment) Hyperlink {
	if s != nil && !targetOf(s.Hyperlink).none() {
		return s.Hyperlink
	}
	return f.Hyperlink
}
