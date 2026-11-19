// SPDX-License-Identifier: MIT

package asposepdf

// maxFormWorkOps bounds the content operators one walk of a page runs inside
// nested Form XObjects (and, for the renderer, tiling cells and Type3 glyph
// procedures). Such a walk re-runs a form's content at every placement, as
// drawing does, so a file of a few kilobytes whose forms each draw the next
// one ten times grows tenfold per level up to the depth cap (pdf-go-u2lc,
// a follow-up to pdf-go-wa32). Once spent, the walk stops descending into
// forms and keeps the rest of the page. Ordinary pages stay below it: on
// 2,407 corpus documents the most any page used was 1.9 million (rendering)
// and 0.8 million (text, images, paths), a large form drawn once.
//
// A variable only so tests can lower it.
var maxFormWorkOps = 4_000_000

// formWorkBudget is one walk's allowance of maxFormWorkOps, shared by the
// recursion (and by a renderer's sub-renderers). A nil budget is unlimited.
type formWorkBudget struct {
	left int
}

func newFormWorkBudget() *formWorkBudget {
	return &formWorkBudget{left: maxFormWorkOps}
}

// exhausted reports whether nothing is left, so a walk can skip a form
// before parsing its content.
func (b *formWorkBudget) exhausted() bool {
	return b != nil && b.left <= 0
}

// spend charges n operators and reports whether they may run. A charge that
// does not fit is refused and exhausts the budget, so the walk stops
// descending from there on.
func (b *formWorkBudget) spend(n int) bool {
	if b == nil {
		return true
	}
	if n > b.left {
		b.left = 0
		return false
	}
	b.left -= n
	return true
}
