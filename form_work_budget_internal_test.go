// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
	"testing"
)

// formBomb is a page drawing a Form XObject that draws the next one fan
// times, levels deep: fan^levels placements of the leaf from a few
// kilobytes. The page also shows its own text after the form.
func formBomb(fan, levels int) []byte {
	return formBombOf(fan, levels, false)
}

// formBombOf is formBomb; with groups every form is a transparency group
// drawn at half opacity, so the renderer composites each placement in a
// sub-renderer of its own.
func formBombOf(fan, levels int, groups bool) []byte {
	objs := pageWith("", "", "<< /XObject << /X 6 0 R >> /Font << /F1 5 0 R >> >>",
		"q /X Do Q BT /F1 12 Tf 10 10 Td (PageText) Tj ET")
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	form := func(body string, next int) string {
		res := "/Font << /F1 5 0 R >>"
		if next > 0 {
			res += fmt.Sprintf(" /XObject << /X %d 0 R >>", next)
		}
		return "<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << " + res +
			" >> /Length 0 >> stream\n" + body + "\nendstream"
	}
	draw := strings.TrimSpace(strings.Repeat("q /X Do Q ", fan))
	return hardeningPDF(chain(objs, levels, func(next int) string {
		return form(draw, next)
	}, form("BT /F1 1 Tf (x) Tj ET 0 0 10 10 re S", 0))...)
}

// Walks that re-run a form at every placement — text extraction, image
// collection, path visiting and rendering — stop descending into forms once
// a page's budget is spent, and keep the page's own content (pdf-go-u2lc).
func TestFormWorkBudgetBoundsNestedForms(t *testing.T) {
	// The real limit takes seconds per walk on this file; a lower one keeps
	// the test quick and shows the same stop.
	defer func(v int) { maxFormWorkOps = v }(maxFormWorkOps)
	maxFormWorkOps = 200_000
	data := formBomb(10, 9) // 1e8 leaf placements without a budget
	for _, w := range []struct {
		name string
		run  func(t *testing.T, p *Page)
	}{
		{"text", func(t *testing.T, p *Page) {
			txt, err := p.ExtractText()
			if err != nil || !strings.Contains(txt, "PageText") {
				t.Errorf("text %.80q, %v", txt, err)
			}
		}},
		{"images", func(t *testing.T, p *Page) {
			if _, err := p.ImageInfos(); err != nil {
				t.Error(err)
			}
		}},
		{"tables", func(t *testing.T, p *Page) {
			if err := NewTableAbsorber().Visit(p); err != nil {
				t.Error(err)
			}
		}},
		{"render", func(t *testing.T, p *Page) {
			if _, err := p.RenderImage(RenderOptions{DPI: 10}); err != nil {
				t.Error(err)
			}
		}},
		{"render groups", func(t *testing.T, p *Page) {
			// sub-renderers share the page's budget
			if _, err := p.RenderImage(RenderOptions{DPI: 10}); err != nil {
				t.Error(err)
			}
		}},
	} {
		t.Run(w.name, func(t *testing.T) {
			data := data
			if strings.HasSuffix(w.name, "groups") {
				data = formBombOf(30, 9, true)
			}
			p := mustPage(t, openHardening(t, data))
			withinDeadline(t, hardeningDeadline, func() { w.run(t, p) })
		})
	}
}

// A form within the budget is walked whole: every placement's text is read.
func TestFormWorkBudgetKeepsOrdinaryForms(t *testing.T) {
	p := mustPage(t, openHardening(t, formBomb(3, 3))) // 9 leaf placements
	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.ReplaceAll(txt, "PageText", ""), "x"); n != 9 {
		t.Errorf("%d leaf glyphs, want 9: %q", n, txt)
	}
}

func TestFormWorkBudgetSpend(t *testing.T) {
	var unlimited *formWorkBudget
	if !unlimited.spend(1<<40) || unlimited.exhausted() {
		t.Error("a nil budget is unlimited")
	}
	b := &formWorkBudget{left: 10}
	if !b.spend(4) || !b.spend(6) || b.exhausted() == false {
		t.Errorf("exact spend: left %d", b.left)
	}
	b = &formWorkBudget{left: 10}
	if b.spend(11) || !b.exhausted() {
		t.Error("an over-large charge is refused and exhausts the budget")
	}
}
