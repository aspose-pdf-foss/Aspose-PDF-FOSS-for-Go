// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// apStream returns the annotation's /AP/N content, failing the test when the
// annotation carries no appearance at all — the defect this file guards
// against (pdf-go-c4c4): a viewer synthesizes a markup appearance from
// /QuadPoints, but an /AP-only consumer such as this library's renderer
// paints nothing without a stream.
func apStream(t *testing.T, doc *Document, base *annotationBase) string {
	t.Helper()
	apDict, _ := base.dict["/AP"].(pdfDict)
	if apDict == nil {
		t.Fatal("/AP missing")
	}
	ref, ok := apDict["/N"].(pdfRef)
	if !ok {
		t.Fatal("/AP/N is not an indirect reference")
	}
	obj := doc.objects[ref.Num]
	if obj == nil {
		t.Fatalf("/AP/N points at missing object %d", ref.Num)
	}
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		t.Fatalf("/AP/N is not a stream: %T", obj.Value)
	}
	return string(stream.Data)
}

// The line of text the markup annotations in these tests mark up.
func markupTestPage(t *testing.T) (*Document, *Page) {
	t.Helper()
	doc := NewDocument(300, 120)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	style := TextStyle{Font: FontHelvetica, Size: 16}
	if err := page.AddText("marked words", style, Rectangle{LLX: 20, LLY: 50, URX: 280, URY: 80}); err != nil {
		t.Fatal(err)
	}
	return doc, page
}

var markupTestQuads = []QuadPoint{
	{X1: 20, Y1: 72, X2: 160, Y2: 72, X3: 20, Y3: 56, X4: 160, Y4: 56},
}

func TestHighlightAnnotationGeneratesAppearance(t *testing.T) {
	doc, page := markupTestPage(t)
	a := NewHighlightAnnotation(page, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
	a.SetQuadPoints(markupTestQuads)
	a.SetColor(&Color{R: 1, G: 1, B: 0, A: 1})

	out := apStream(t, doc, &a.annotationBase)
	if !strings.Contains(out, " re\n") || !strings.Contains(out, "f\n") {
		t.Errorf("expected a filled rectangle per quad, got %q", out)
	}
	// The wash must not hide the glyphs under it.
	if !strings.Contains(out, "/GSMul gs") {
		t.Errorf("expected the Multiply blend state, got %q", out)
	}
}

func TestUnderlineAnnotationGeneratesAppearance(t *testing.T) {
	doc, page := markupTestPage(t)
	a := NewUnderlineAnnotation(page, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
	a.SetQuadPoints(markupTestQuads)

	out := apStream(t, doc, &a.annotationBase)
	if !strings.Contains(out, " m\n") || !strings.Contains(out, " l\n") || !strings.Contains(out, "S\n") {
		t.Errorf("expected a stroked rule, got %q", out)
	}
}

func TestStrikeOutAnnotationGeneratesAppearance(t *testing.T) {
	doc, page := markupTestPage(t)
	a := NewStrikeOutAnnotation(page, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
	a.SetQuadPoints(markupTestQuads)

	out := apStream(t, doc, &a.annotationBase)
	if !strings.Contains(out, "S\n") {
		t.Errorf("expected a stroked rule, got %q", out)
	}
	// The rule crosses the text, so it sits above the quad's bottom edge.
	if !strings.Contains(out, " 8 m\n") {
		t.Errorf("expected the rule at the quad's middle (y=8 in form space), got %q", out)
	}
}

func TestSquigglyAnnotationGeneratesAppearance(t *testing.T) {
	doc, page := markupTestPage(t)
	a := NewSquigglyAnnotation(page, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
	a.SetQuadPoints(markupTestQuads)

	out := apStream(t, doc, &a.annotationBase)
	// A zigzag is many segments, not the single one an underline draws.
	if n := strings.Count(out, " l\n"); n < 4 {
		t.Errorf("expected a multi-segment wave, got %d line segments in %q", n, out)
	}
}

// Changing a property after construction must redraw the appearance — the
// convention every self-drawing annotation family follows.
func TestMarkupAnnotationSettersRegenerateAppearance(t *testing.T) {
	doc, page := markupTestPage(t)
	a := NewHighlightAnnotation(page, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
	a.SetQuadPoints(markupTestQuads)
	a.SetColor(&Color{R: 1, G: 0, B: 0, A: 1})
	red := apStream(t, doc, &a.annotationBase)

	a.SetColor(&Color{R: 0, G: 0, B: 1, A: 1})
	blue := apStream(t, doc, &a.annotationBase)

	if red == blue {
		t.Fatalf("appearance unchanged after SetColor: %q", red)
	}
	if !strings.Contains(blue, "0 0 1 rg") {
		t.Errorf("expected the new fill colour, got %q", blue)
	}
}

// The renderer paints /AP/N only, so the whole point of the appearance is
// that the annotation shows up in a render of the page.
func TestMarkupAnnotationRenders(t *testing.T) {
	render := func(t *testing.T, doc *Document) []byte {
		t.Helper()
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := page.RenderPNG(&buf, RenderOptions{DPI: 72}); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	plainDoc, _ := markupTestPage(t)
	plain := render(t, plainDoc)

	for _, tc := range []struct {
		name string
		add  func(*Page)
	}{
		{"highlight", func(p *Page) {
			a := NewHighlightAnnotation(p, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
			a.SetQuadPoints(markupTestQuads)
			a.SetColor(&Color{R: 1, G: 1, B: 0, A: 1})
			if err := p.Annotations().Add(a); err != nil {
				t.Fatal(err)
			}
		}},
		{"strikeout", func(p *Page) {
			a := NewStrikeOutAnnotation(p, Rectangle{LLX: 20, LLY: 56, URX: 160, URY: 72})
			a.SetQuadPoints(markupTestQuads)
			a.SetColor(&Color{R: 1, G: 0, B: 0, A: 1})
			if err := p.Annotations().Add(a); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, page := markupTestPage(t)
			tc.add(page)
			if bytes.Equal(plain, render(t, doc)) {
				t.Fatal("the annotated page renders identically to the unannotated one")
			}
		})
	}
}

// An annotation parsed from a file that carries no /AP — how most producers
// write a highlight — must still render: the renderer synthesizes one.
func TestMarkupAnnotationWithoutAPIsSynthesized(t *testing.T) {
	doc, page := markupTestPage(t)
	dict := pdfDict{
		"/Type":       pdfName("/Annot"),
		"/Subtype":    pdfName("/Highlight"),
		"/Rect":       pdfArray{20.0, 56.0, 160.0, 72.0},
		"/C":          pdfArray{1.0, 1.0, 0.0},
		"/QuadPoints": quadPointsToPDFArray(markupTestQuads),
	}
	rd := &renderer{page: page}
	if s := rd.synthesizeAnnotationAppearance(dict); s == nil {
		t.Fatal("no appearance synthesized for an /AP-less highlight")
	} else if !strings.Contains(string(s.Data), " re\n") {
		t.Errorf("synthesized appearance draws nothing: %q", s.Data)
	}
	_ = doc
}
