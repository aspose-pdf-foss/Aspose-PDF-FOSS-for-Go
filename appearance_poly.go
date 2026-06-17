// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// generatePolygonAppearance produces /AP/N for a Polygon annotation: a
// closed path through the vertices, filled with /IC (if set) and stroked
// with /C (if set). When neither colour is present nothing is drawn —
// the same "don't invent a border" convention as SquareAnnotation.
func generatePolygonAppearance(a *PolygonAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	verts := a.Vertices()
	bw := a.BorderWidth()
	stroke := a.Color()
	fill := a.InteriorColor()
	hasStroke := stroke != nil && bw > 0

	b := newAppearanceBuilder()
	if len(verts) >= 2 && (hasStroke || fill != nil) {
		b.PushState()
		if hasStroke {
			b.SetLineWidth(bw)
			b.SetStrokeColorRGB(*stroke)
			if a.BorderStyle() == BorderDashed {
				dp := a.DashPattern()
				if len(dp) == 0 {
					dp = []float64{3, 3}
				}
				b.SetDashPattern(dp, 0)
			}
		}
		if fill != nil {
			b.SetFillColorRGB(*fill)
		}
		b.MoveTo(verts[0].X-rect.LLX, verts[0].Y-rect.LLY)
		for _, p := range verts[1:] {
			b.LineTo(p.X-rect.LLX, p.Y-rect.LLY)
		}
		b.ClosePath()
		switch {
		case fill != nil && hasStroke:
			b.FillStroke()
		case fill != nil:
			b.Fill()
		default:
			b.Stroke()
		}
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// generatePolylineAppearance produces /AP/N for a PolyLine annotation: an
// open path through the vertices, stroked with /C, with optional line
// endings at the first and last vertex (filled with /IC for closed
// shapes).
func generatePolylineAppearance(a *PolylineAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	verts := a.Vertices()
	bw := a.BorderWidth()
	stroke := a.Color()

	b := newAppearanceBuilder()
	if len(verts) >= 2 && stroke != nil && bw > 0 {
		b.PushState()
		b.SetLineWidth(bw)
		b.SetStrokeColorRGB(*stroke)
		if a.BorderStyle() == BorderDashed {
			dp := a.DashPattern()
			if len(dp) == 0 {
				dp = []float64{3, 3}
			}
			b.SetDashPattern(dp, 0)
		}
		b.MoveTo(verts[0].X-rect.LLX, verts[0].Y-rect.LLY)
		for _, p := range verts[1:] {
			b.LineTo(p.X-rect.LLX, p.Y-rect.LLY)
		}
		b.Stroke()

		// Line endings. The start ending points outward from the first
		// vertex (direction v0←v1); the end ending points outward from the
		// last vertex (direction vN-1→vN). They inherit the stroke colour and
		// width; closed shapes use /IC as fill.
		ic := a.InteriorColor()
		v0 := Point{X: verts[0].X - rect.LLX, Y: verts[0].Y - rect.LLY}
		v1 := Point{X: verts[1].X - rect.LLX, Y: verts[1].Y - rect.LLY}
		n := len(verts)
		vlast := Point{X: verts[n-1].X - rect.LLX, Y: verts[n-1].Y - rect.LLY}
		vprev := Point{X: verts[n-2].X - rect.LLX, Y: verts[n-2].Y - rect.LLY}
		drawLineEnding(b, a.StartLineEnding(), v0.X, v0.Y, math.Atan2(v0.Y-v1.Y, v0.X-v1.X), bw, ic)
		drawLineEnding(b, a.EndLineEnding(), vlast.X, vlast.Y, math.Atan2(vlast.Y-vprev.Y, vlast.X-vprev.X), bw, ic)
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
