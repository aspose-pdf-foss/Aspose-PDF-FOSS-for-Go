// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestLayers authors two layers, assigns content to each, and checks that the
// renderer honors a hidden layer's default visibility — and that the layers
// (names + visibility) round-trip through Save/Open.
func TestLayers(t *testing.T) {
	renderPx := func(doc *pdf.Document) int {
		img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 120})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return nonWhitePixels(img)
	}

	doc := pdf.NewDocument(300, 200)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24, Color: &pdf.Color{A: 1}}

	a := doc.AddLayer("Layer A")
	if err := p.BeginLayer(a); err != nil {
		t.Fatalf("BeginLayer A: %v", err)
	}
	p.AddText("AAAA", style, pdf.Rectangle{LLX: 20, LLY: 140, URX: 280, URY: 180})
	p.EndLayer()

	b := doc.AddLayer("Layer B")
	p.BeginLayer(b)
	p.AddText("BBBB", style, pdf.Rectangle{LLX: 20, LLY: 90, URX: 280, URY: 130})
	p.EndLayer()

	if got := doc.Layers(); len(got) != 2 {
		t.Fatalf("Layers() = %d, want 2", len(got))
	}

	bothPx := renderPx(doc)

	// Hide layer B — the renderer must drop its content.
	b.SetVisible(false)
	if b.IsVisible() {
		t.Error("layer B still reports visible after SetVisible(false)")
	}
	if a.IsVisible() != true {
		t.Error("layer A should stay visible")
	}
	onlyAPx := renderPx(doc)
	if onlyAPx >= bothPx {
		t.Errorf("hiding a layer did not reduce rendered content (both=%d, onlyA=%d)", bothPx, onlyAPx)
	}
	if onlyAPx == 0 {
		t.Error("the visible layer rendered nothing")
	}

	// Round-trip: layer names and visibility survive Save/Open, and the hidden
	// layer stays hidden when re-rendered.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	layers := out.Layers()
	if len(layers) != 2 {
		t.Fatalf("after reopen: Layers() = %d, want 2", len(layers))
	}
	byName := map[string]bool{}
	for _, l := range layers {
		byName[l.Name()] = l.IsVisible()
	}
	if v, ok := byName["Layer A"]; !ok || !v {
		t.Errorf("Layer A after reopen: visible=%v present=%v", v, ok)
	}
	if v, ok := byName["Layer B"]; !ok || v {
		t.Errorf("Layer B after reopen: visible=%v present=%v (want hidden)", v, ok)
	}
	if px := renderPx(out); px != onlyAPx {
		t.Errorf("reopened render = %d px, want %d (hidden layer honored)", px, onlyAPx)
	}
}
