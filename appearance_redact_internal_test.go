package asposepdf

import (
	"strings"
	"testing"
)

func TestGenerateRedactAppearanceSingleQuadFill(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := NewRedactAnnotation(page, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
	ra.SetInteriorColor(&Color{R: 1, G: 0, B: 0, A: 1})
	ra.SetQuadPoints([]QuadPoint{
		{X1: 100, Y1: 650, X2: 300, Y2: 650, X3: 100, Y3: 600, X4: 300, Y4: 600},
	})
	// Inspect /AP/N stream Data.
	apDict, _ := ra.dict["/AP"].(pdfDict)
	if apDict == nil {
		t.Fatal("/AP missing")
	}
	ref, _ := apDict["/N"].(pdfRef)
	obj := doc.objects[ref.Num]
	stream := obj.Value.(*pdfStream)
	out := string(stream.Data)
	// Expect at least one fill operator (re + f) and stroke color set.
	if !strings.Contains(out, " re\n") {
		t.Errorf("expected re op for quad fill, got %q", out)
	}
	if !strings.Contains(out, " rg\n") {
		t.Errorf("expected rg op for fill color, got %q", out)
	}
	if !strings.Contains(out, "f\n") {
		t.Errorf("expected f op for fill, got %q", out)
	}
}

func TestGenerateRedactAppearanceMultipleQuads(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 500, URY: 500})
	ra.SetQuadPoints([]QuadPoint{
		{X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 0, X4: 100, Y4: 0},
		{X1: 200, Y1: 100, X2: 300, Y2: 100, X3: 200, Y3: 0, X4: 300, Y4: 0},
		{X1: 400, Y1: 100, X2: 500, Y2: 100, X3: 400, Y3: 0, X4: 500, Y4: 0},
	})
	apDict, _ := ra.dict["/AP"].(pdfDict)
	ref, _ := apDict["/N"].(pdfRef)
	stream := doc.objects[ref.Num].Value.(*pdfStream)
	out := string(stream.Data)
	// Three quads → three re ops at minimum.
	if cnt := strings.Count(out, " re\n"); cnt < 3 {
		t.Errorf("expected 3+ re ops for 3 quads, got %d in %q", cnt, out)
	}
}

func TestGenerateRedactAppearanceWithOverlayText(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 50})
	ra.SetQuadPoints([]QuadPoint{
		{X1: 0, Y1: 50, X2: 200, Y2: 50, X3: 0, Y3: 0, X4: 200, Y4: 0},
	})
	ra.SetOverlayText("HIDDEN")
	apDict, _ := ra.dict["/AP"].(pdfDict)
	ref, _ := apDict["/N"].(pdfRef)
	stream := doc.objects[ref.Num].Value.(*pdfStream)
	out := string(stream.Data)
	// Overlay text → BT/ET/Tj operators.
	if !strings.Contains(out, "BT\n") {
		t.Errorf("expected BT for overlay text, got %q", out)
	}
	if !strings.Contains(out, "ET\n") {
		t.Errorf("expected ET for overlay text, got %q", out)
	}
}

func TestGenerateRedactAppearanceDefaultBlackFill(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := NewRedactAnnotation(page, Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	ra.SetQuadPoints([]QuadPoint{
		{X1: 0, Y1: 50, X2: 100, Y2: 50, X3: 0, Y3: 0, X4: 100, Y4: 0},
	})
	// No SetInteriorColor — should default to black (rgb 0 0 0).
	apDict, _ := ra.dict["/AP"].(pdfDict)
	ref, _ := apDict["/N"].(pdfRef)
	stream := doc.objects[ref.Num].Value.(*pdfStream)
	out := string(stream.Data)
	if !strings.Contains(out, "0 0 0 rg") {
		t.Errorf("expected default black fill (0 0 0 rg), got %q", out)
	}
}
