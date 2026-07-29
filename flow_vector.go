// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/png"
)

// Vector-graphics recovery for the flow reconstruction (flow_doc.go): logos,
// charts and drawings are painted with path operators, which a flow exporter
// cannot re-express as text — so their page regions are rasterized with the
// library's own renderer and carried as ordinary image blocks; paragraphs
// contained in a cluster leave the flow (their text lives inside the patch).
// The same idea as the HTML native mode's raster patches, block-grained.

const (
	vecMinSizePt   = 16.0 // block clusters smaller than this per side are rules/decorations
	vecIconMinPt   = 5.0  // icon clusters: minimum side (filters rules/ticks)
	vecIconMaxPt   = 40.0 // icon clusters: maximum side (larger ones are blocks)
	vecMergeGapPt  = 8.0  // boxes closer than this merge into one cluster
	vecMaxAreaFrac = 0.70 // clusters covering more of the page are backgrounds
	vecMaxTextFrac = 0.15 // clusters denser in text than this are tables/underlays
	vecPadPt       = 2.0  // crop padding around a cluster
	vecRenderDPI   = 144.0
)

// vectorGraphicBlocks returns image blocks for the page's vector-graphics
// clusters plus the cluster rectangles themselves (the caller drops text
// paragraphs contained in a cluster — the patch is rendered WITH text, so a
// chart keeps its axis labels inside the picture instead of duplicating them
// as stray paragraphs). exclude lists regions already carried by raster
// images; textRects are the extracted text fragment boxes (clusters
// dominated by text — table grids, shaded text panels — are skipped, their
// content flows as text). Rotated pages are skipped (cropping math assumes
// an upright page).
func vectorGraphicBlocks(p *Page, exclude, textRects []Rectangle) ([]flowBlock, []Rectangle, []*Image) {
	if p.Rotation() != 0 {
		return nil, nil, nil
	}
	data, err := p.contentStreams()
	if err != nil || len(data) == 0 {
		return nil, nil, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, nil, nil
	}
	boxes := paintedPathBoxes(ops)
	if len(boxes) == 0 {
		return nil, nil, nil
	}

	// Drop boxes an extracted raster image already covers (>= 85% of the
	// box's area — the frame stroked AROUND an image is part of the image's
	// presentation, not standalone graphics).
	kept := boxes[:0]
	for _, b := range boxes {
		area := (b.URX - b.LLX) * (b.URY - b.LLY)
		covered := false
		for _, ex := range exclude {
			w := minf(b.URX, ex.URX) - maxf(b.LLX, ex.LLX)
			h := minf(b.URY, ex.URY) - maxf(b.LLY, ex.LLY)
			if w > 0 && h > 0 && (area <= 0 || w*h >= 0.85*area) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, b)
		}
	}
	clusters := clusterRects(kept, vecMergeGapPt)

	crop, err := p.CropBox()
	if err != nil {
		return nil, nil, nil
	}
	pageArea := (crop.URX - crop.LLX) * (crop.URY - crop.LLY)
	var wanted []Rectangle
	for _, c := range clusters {
		w, h := c.URX-c.LLX, c.URY-c.LLY
		if w < vecMinSizePt || h < vecMinSizePt {
			continue // rules, underlines, list ticks
		}
		if pageArea > 0 && w*h > vecMaxAreaFrac*pageArea {
			continue // page background / border frame
		}
		if textAreaWithin(c, textRects) > vecMaxTextFrac*w*h {
			continue // table grid or text panel — the text itself flows
		}
		wanted = append(wanted, c)
	}

	// Icon detection runs on a second clustering WITHOUT hairline rules: a
	// link's underline stroke otherwise fuses with the 12pt mark beside it
	// into one over-wide cluster. Candidates inside a block cluster (chart
	// tick marks) are the block's business, not standalone icons.
	var ruleFree []Rectangle
	for _, b := range kept {
		w, h := b.URX-b.LLX, b.URY-b.LLY
		if (h < 2.5 && w > 8) || (w < 2.5 && h > 8) {
			continue // underline / rule
		}
		ruleFree = append(ruleFree, b)
	}
	var iconRects []Rectangle
	for _, c := range clusterRects(ruleFree, 4) {
		w, h := c.URX-c.LLX, c.URY-c.LLY
		if w < vecIconMinPt || h < vecIconMinPt || w > vecIconMaxPt || h > vecIconMaxPt {
			continue
		}
		if textAreaWithin(c, textRects) >= 0.3*w*h {
			continue
		}
		inBlock := false
		for _, big := range clusters {
			if big.URX-big.LLX >= vecMinSizePt && big.URY-big.LLY >= vecMinSizePt &&
				rectMostlyInside(c, []Rectangle{big}) {
				inBlock = true
				break
			}
		}
		if !inBlock {
			iconRects = append(iconRects, c)
		}
	}
	if len(wanted) == 0 && len(iconRects) == 0 {
		return nil, nil, nil
	}

	// One full render serves every cluster on the page: text INSIDE a
	// cluster belongs to the picture (chart labels), and the caller removes
	// the corresponding paragraphs from the flow.
	frame, err := p.renderImage(RenderOptions{DPI: vecRenderDPI}, false, false)
	if err != nil {
		return nil, nil, nil
	}
	rgba, ok := frame.(*image.RGBA)
	if !ok {
		return nil, nil, nil
	}
	cropPatch := func(c Rectangle, pad float64) *Image {
		scale := vecRenderDPI / 72.0
		x0 := int((c.LLX - pad - crop.LLX) * scale)
		x1 := int((c.URX + pad - crop.LLX) * scale)
		y0 := int((crop.URY - (c.URY + pad)) * scale)
		y1 := int((crop.URY - (c.LLY - pad)) * scale)
		r := image.Rect(x0, y0, x1, y1).Intersect(rgba.Bounds())
		if r.Dx() < 4 || r.Dy() < 4 {
			return nil
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, rgba.SubImage(r)); err != nil {
			return nil
		}
		return &Image{
			Data:       buf.Bytes(),
			Format:     ImageFormatPNG,
			Width:      r.Dx(),
			Height:     r.Dy(),
			ColorSpace: ColorSpaceDeviceRGB,
			BPC:        8,
			X:          c.LLX - pad,
			Y:          c.LLY - pad,
			PageWidth:  c.URX - c.LLX + 2*pad,
			PageHeight: c.URY - c.LLY + 2*pad,
		}
	}
	var blocks []flowBlock
	for _, c := range wanted {
		if img := cropPatch(c, vecPadPt); img != nil {
			blocks = append(blocks, flowBlock{img: img, top: img.Y + img.PageHeight})
		}
	}
	var icons []*Image
	for _, c := range iconRects {
		if img := cropPatch(c, 1); img != nil {
			icons = append(icons, img)
		}
	}
	return blocks, wanted, icons
}

// paintedPathBoxes walks the content ops with CTM tracking and returns the
// page-space bounding box of every painted (stroked/filled) path. Top-level
// content only — Form XObjects are not entered (a whole imported page would
// register as one giant cluster).
func paintedPathBoxes(ops []contentOp) []Rectangle {
	ctm := identityMatrix()
	var stack [][6]float64
	var boxes []Rectangle

	cur := Rectangle{LLX: 1e18, LLY: 1e18, URX: -1e18, URY: -1e18}
	havePt := false
	addPt := func(x, y float64) {
		dx, dy := matApplyPoint(ctm, x, y)
		cur.LLX, cur.LLY = minf(cur.LLX, dx), minf(cur.LLY, dy)
		cur.URX, cur.URY = maxf(cur.URX, dx), maxf(cur.URY, dy)
		havePt = true
	}
	reset := func() {
		cur = Rectangle{LLX: 1e18, LLY: 1e18, URX: -1e18, URY: -1e18}
		havePt = false
	}
	nums := func(op contentOp) []float64 {
		out := make([]float64, len(op.Operands))
		for i, o := range op.Operands {
			out[i] = operandFloat(o)
		}
		return out
	}

	for _, op := range ops {
		switch op.Operator {
		case "cm":
			if v := nums(op); len(v) >= 6 {
				ctm = matMul([6]float64{v[0], v[1], v[2], v[3], v[4], v[5]}, ctm)
			}
		case "q":
			stack = append(stack, ctm)
		case "Q":
			if len(stack) > 0 {
				ctm = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "m", "l":
			if v := nums(op); len(v) >= 2 {
				addPt(v[0], v[1])
			}
		case "c":
			if v := nums(op); len(v) >= 6 {
				addPt(v[0], v[1])
				addPt(v[2], v[3])
				addPt(v[4], v[5])
			}
		case "v", "y":
			if v := nums(op); len(v) >= 4 {
				addPt(v[0], v[1])
				addPt(v[2], v[3])
			}
		case "re":
			if v := nums(op); len(v) >= 4 {
				addPt(v[0], v[1])
				addPt(v[0]+v[2], v[1])
				addPt(v[0], v[1]+v[3])
				addPt(v[0]+v[2], v[1]+v[3])
			}
		case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*":
			if havePt {
				boxes = append(boxes, cur)
			}
			reset()
		case "n":
			reset() // clip-only path
		}
	}
	return boxes
}

// clusterRects merges rectangles whose gap-expanded bounds intersect,
// repeating until stable.
func clusterRects(rects []Rectangle, gap float64) []Rectangle {
	out := append([]Rectangle(nil), rects...)
	for {
		merged := false
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				a, b := out[i], out[j]
				if a.LLX-gap <= b.URX && b.LLX-gap <= a.URX &&
					a.LLY-gap <= b.URY && b.LLY-gap <= a.URY {
					out[i] = Rectangle{
						LLX: minf(a.LLX, b.LLX), LLY: minf(a.LLY, b.LLY),
						URX: maxf(a.URX, b.URX), URY: maxf(a.URY, b.URY),
					}
					out = append(out[:j], out[j+1:]...)
					merged = true
					j--
				}
			}
		}
		if !merged {
			return out
		}
	}
}

// textAreaWithin sums the area of text rects clipped to r.
func textAreaWithin(r Rectangle, textRects []Rectangle) float64 {
	total := 0.0
	for _, t := range textRects {
		w := minf(t.URX, r.URX) - maxf(t.LLX, r.LLX)
		h := minf(t.URY, r.URY) - maxf(t.LLY, r.LLY)
		if w > 0 && h > 0 {
			total += w * h
		}
	}
	return total
}
