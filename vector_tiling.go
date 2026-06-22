// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// TilingPattern is a repeating fill (PatternType 1, ISO 32000-1 §8.7.3.1): a
// small cell of content tiled across whatever shape it fills. Draw the cell
// once via Canvas() (the full *Page drawing API), optionally adjust the tile
// spacing with SetStep, then use it as a fill by setting ShapeStyle.FillTiling
// on any Draw* call. Useful for hatching, textures and repeated motifs.
// Mirrors the colored (PaintType 1) tiling patterns of Aspose.PDF for .NET.
type TilingPattern struct {
	doc          *Document
	canvas       *Page     // detached drawing surface for the cell (not in doc.pages)
	bbox         Rectangle // cell box, [0 0 width height]
	xstep, ystep float64   // tile spacing (default = cell size; larger leaves gaps)
	patternID    int       // built /Pattern stream object; 0 until first use
}

// CreateTilingPattern creates an empty tiling pattern whose cell is width×height
// points. Draw the cell through Canvas(), then fill shapes with it via
// ShapeStyle.FillTiling.
func (d *Document) CreateTilingPattern(width, height float64) *TilingPattern {
	canvasDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
	}
	return &TilingPattern{
		doc:    d,
		canvas: &Page{doc: d, obj: &pdfObject{Value: canvasDict}},
		bbox:   Rectangle{URX: width, URY: height},
		xstep:  width,
		ystep:  height,
	}
}

// Canvas returns the cell's drawing surface (the full *Page drawing API). Draw
// everything before the pattern is first used as a fill — its content is frozen
// at that point.
func (t *TilingPattern) Canvas() *Page { return t.canvas }

// SetStep sets the horizontal and vertical tile spacing in points. The default
// is the cell size (tiles touch edge-to-edge); a larger step leaves gaps, a
// smaller one overlaps. Non-positive values are ignored.
func (t *TilingPattern) SetStep(xstep, ystep float64) {
	if xstep > 0 {
		t.xstep = xstep
	}
	if ystep > 0 {
		t.ystep = ystep
	}
}

// ensureBuilt materialises the /Pattern object (once) from the cell content.
func (t *TilingPattern) ensureBuilt() (int, error) {
	if t.patternID != 0 {
		return t.patternID, nil
	}
	content, err := t.canvas.contentStreams()
	if err != nil {
		return 0, err
	}
	resources := t.canvas.pageResources()
	if resources == nil {
		resources = pdfDict{}
	}
	stream := &pdfStream{
		Dict: pdfDict{
			"/Type":        pdfName("/Pattern"),
			"/PatternType": 1, // tiling
			"/PaintType":   1, // colored (the cell sets its own colours)
			"/TilingType":  1, // constant spacing
			"/BBox":        pdfArray{t.bbox.LLX, t.bbox.LLY, t.bbox.URX, t.bbox.URY},
			"/XStep":       t.xstep,
			"/YStep":       t.ystep,
			"/Resources":   resources,
			"/Matrix":      pdfArray{1.0, 0.0, 0.0, 1.0, 0.0, 0.0},
		},
		Data:    content,
		Decoded: true,
	}
	t.patternID = t.doc.nextID
	t.doc.nextID++
	t.doc.objects[t.patternID] = &pdfObject{Num: t.patternID, Value: stream}
	return t.patternID, nil
}

// resolveShapeTiling registers the style's TilingPattern on the page and sets
// its FillPattern resource name, so the shared /Pattern cs … scn emission paints
// it. Called from the draw path before a shape is painted.
func (p *Page) resolveShapeTiling(style *ShapeStyle) error {
	if style.FillTiling == nil || style.FillPattern != "" {
		return nil
	}
	if style.FillTiling.doc != p.doc {
		return fmt.Errorf("fill: tiling pattern belongs to a different document")
	}
	id, err := style.FillTiling.ensureBuilt()
	if err != nil {
		return err
	}
	style.FillPattern = p.registerPatternRef(pdfRef{Num: id})
	return nil
}

// registerPatternRef ensures the page /Resources/Pattern maps a name to the
// pattern object and returns that name (reusing an existing mapping).
func (p *Page) registerPatternRef(ref pdfRef) string {
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	pat, _ := resolveRef(p.doc.objects, resources["/Pattern"]).(pdfDict)
	if pat == nil {
		pat = pdfDict{}
		resources["/Pattern"] = pat
	}
	for k, v := range pat {
		if r, ok := v.(pdfRef); ok && r.Num == ref.Num {
			return k
		}
	}
	name := ""
	for i := 0; ; i++ {
		name = fmt.Sprintf("/P%d", i)
		if _, exists := pat[name]; !exists {
			break
		}
	}
	pat[name] = ref
	return name
}
