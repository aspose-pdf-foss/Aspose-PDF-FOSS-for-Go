// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// DOCX Textbox recognition mode (pdf-go-7qiu.6) — the fixed-layout
// counterpart of the Flow/EnhancedFlow writers, mirroring Aspose.PDF for
// .NET's DocRecognitionMode.Textbox: every text paragraph becomes an
// absolutely positioned floating text box (wp:anchor + wps:txbx) at its PDF
// coordinates, every image and vector-graphics patch an anchored picture.
// Maximal visual fidelity, limited editability (moving a box moves that box
// only); no reconstruction heuristics are involved, so nothing can re-wrap,
// merge or re-order. Each source page becomes its own Word section carrying
// the page's exact size (mixed portrait/landscape documents keep both).
//
// v1 scope: rotated text (diagonal watermarks) is skipped; table rulings are
// carried by the vector-patch pass only when they cluster as graphics; text
// inside a graphics cluster lives in the rendered patch, not in a box.

// writeDocxTextbox emits the whole document body in Textbox mode.
func (d *Document) writeDocxTextbox(pages []*Page, sel []int, opt DocSaveOptions, dw *docxWriter) ([]byte, error) {
	dw.relByURI = map[string]string{}
	dw.relByImage = map[[32]byte]string{}
	dw.rels = []docxRel{
		{id: "rId1", relType: relTypeStyles, target: "styles.xml"},
		{id: "rId2", relType: relTypeNumbering, target: "numbering.xml"},
	}

	var b strings.Builder
	b.WriteString(docxXMLHeader)
	fmt.Fprintf(&b, `<w:document xmlns:w="%s" xmlns:r="%s"><w:body>`, docxNSw, docxNSr)

	for i, n := range sel {
		p := pages[n-1]
		size, err := p.Size()
		if err != nil {
			return nil, err
		}
		if err := dw.writeTextboxPage(&b, p, size, opt); err != nil {
			return nil, err
		}
		sect := textboxSectPr(size)
		if i < len(sel)-1 {
			// Section break: closes the current page's section.
			fmt.Fprintf(&b, `<w:p><w:pPr>%s</w:pPr></w:p>`, sect)
		} else {
			b.WriteString(sect) // last section: body-level sectPr
		}
	}
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String()), nil
}

// textboxSectPr emits section properties with the page's exact size and
// minimal margins (content is absolutely positioned, margins are irrelevant
// but Word requires sane values).
func textboxSectPr(size PageSize) string {
	w, h := int(size.Width*20+0.5), int(size.Height*20+0.5)
	orient := ""
	if size.Width > size.Height {
		orient = ` w:orient="landscape"`
	}
	return fmt.Sprintf(`<w:sectPr><w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="720" w:right="720" w:bottom="720" w:left="720" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr>`,
		w, h, orient)
}

// writeTextboxPage emits one page: a single paragraph holding every anchored
// object (text boxes, pictures, vector patches).
func (dw *docxWriter) writeTextboxPage(b *strings.Builder, p *Page, size PageSize, opt DocSaveOptions) error {
	pm, err := p.Paragraphs()
	if err != nil {
		return err
	}
	links := pageLinkAreas(p)

	// Raster images with their placements.
	var images []Image
	if !opt.NoImages {
		if imgs, err := p.ExtractImages(); err == nil {
			for i := range imgs {
				if embeddableImage(&imgs[i]) {
					images = append(images, imgs[i])
				}
			}
		}
	}

	// Vector-graphics clusters (charts, logos): rendered patches carry both
	// the drawing and any text inside it.
	var vecBlocks []flowBlock
	var vecClusters []Rectangle
	if !opt.NoImages {
		var imageRects, textRects []Rectangle
		for _, img := range images {
			imageRects = append(imageRects, Rectangle{LLX: img.X, LLY: img.Y,
				URX: img.X + img.PageWidth, URY: img.Y + img.PageHeight})
		}
		for si := range pm.Sections {
			for pi := range pm.Sections[si].Paragraphs {
				for _, line := range pm.Sections[si].Paragraphs[pi].Lines {
					for _, fr := range line.Fragments {
						textRects = append(textRects, Rectangle{LLX: fr.X, LLY: fr.Y,
							URX: fr.X + fr.Width, URY: fr.Y + fr.Height})
					}
				}
			}
		}
		vecBlocks, vecClusters, _ = vectorGraphicBlocks(p, imageRects, textRects, nil)
	}

	b.WriteString(`<w:p>`)

	// Pictures first (usually backgrounds/logos), then patches, then text on
	// top — relativeHeight preserves this order.
	for i := range images {
		img := &images[i]
		r := Rectangle{LLX: img.X, LLY: img.Y, URX: img.X + img.PageWidth, URY: img.Y + img.PageHeight}
		if rectMostlyInside(r, vecClusters) {
			continue // lives inside a rendered patch
		}
		b.WriteString(dw.anchorImageRun(img, size))
	}
	for _, vb := range vecBlocks {
		b.WriteString(dw.anchorImageRun(vb.img, size))
	}
	for si := range pm.Sections {
		for pi := range pm.Sections[si].Paragraphs {
			para := &pm.Sections[si].Paragraphs[pi]
			if strings.TrimSpace(para.Text) == "" || isRotatedDecoration(para) {
				continue
			}
			if rectMostlyInside(para.Rectangle, vecClusters) {
				continue // text lives inside the patch
			}
			// One box per line-CELL (lines split at wide gaps, as in the
			// stream table detector): a line straddling two page columns or
			// aligned at tab stops keeps each piece at its own X, and every
			// line sits at its own Y — nothing can drift or re-pitch.
			for _, line := range para.Lines {
				row := splitStreamRow(line)
				for _, cell := range row.cells {
					b.WriteString(dw.anchorCellRun(cell, row, links, size))
				}
			}
		}
	}
	b.WriteString(`</w:p>`)
	return nil
}

// anchorEMU converts a PDF-user-space rect to page-anchored EMU offsets.
func anchorEMU(r Rectangle, size PageSize) (x, y, cx, cy int64) {
	x = int64(r.LLX*12700 + 0.5)
	y = int64((size.Height-r.URY)*12700 + 0.5)
	cx = int64((r.URX-r.LLX)*12700 + 0.5)
	cy = int64((r.URY-r.LLY)*12700 + 0.5)
	return
}

// anchorImageRun emits one absolutely positioned picture.
func (dw *docxWriter) anchorImageRun(img *Image, size PageSize) string {
	r := Rectangle{LLX: img.X, LLY: img.Y, URX: img.X + img.PageWidth, URY: img.Y + img.PageHeight}
	x, y, cx, cy := anchorEMU(r, size)
	if cx <= 0 || cy <= 0 {
		return ""
	}
	relID := dw.imageRel(img)
	dw.drawingID++
	id := dw.drawingID
	var b strings.Builder
	b.WriteString(`<w:r><w:drawing>`)
	fmt.Fprintf(&b, `<wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="%d" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">`, id)
	b.WriteString(`<wp:simplePos x="0" y="0"/>`)
	fmt.Fprintf(&b, `<wp:positionH relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionH>`, x)
	fmt.Fprintf(&b, `<wp:positionV relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionV>`, y)
	fmt.Fprintf(&b, `<wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/>`, cx, cy)
	fmt.Fprintf(&b, `<wp:docPr id="%d" name="Picture %d"/><wp:cNvGraphicFramePr/>`, id, id)
	b.WriteString(`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`)
	b.WriteString(`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr>`)
	fmt.Fprintf(&b, `<pic:cNvPr id="%d" name="Picture %d"/><pic:cNvPicPr/></pic:nvPicPr>`, id, id)
	fmt.Fprintf(&b, `<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`, relID)
	fmt.Fprintf(&b, `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`, cx, cy)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`)
	return b.String()
}

// vmlShapetype is the canonical VML text-box shape (t202), declared once per
// document before its first use.
const vmlShapetype = `<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office"><v:stroke joinstyle="miter"/><v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>`

// anchorCellRun emits one line-cell as an absolutely positioned floating
// text box at the cell's own coordinates. VML (w:pict + v:shape) rather
// than wps: the 2010 wordprocessingShape namespace is not part of ECMA-376
// transitional, so wps fails strict XSD validation, while w:pict admits the
// VML namespace explicitly (processContents="lax") — and every Word version
// renders VML text boxes. mso-fit-shape-to-text is off and the box carries
// no wrap, so the glyphs stay where the PDF put them; a slightly wider
// substitute face just overflows the invisible box.
func (dw *docxWriter) anchorCellRun(cell streamCellSpan, row streamRow, links []linkArea, size PageSize) string {
	box := Rectangle{LLX: cell.lo, LLY: row.bot - 1, URX: cell.hi + 6, URY: row.top + 1}
	wPt, hPt := box.URX-box.LLX, box.URY-box.LLY
	if wPt <= 0 || hPt <= 0 {
		return ""
	}

	line := TextLine{Y: row.baseline, Fragments: cell.frs}
	lineRuns := segmentLineRuns(docSeg{lines: []TextLine{line}}, links)
	if len(lineRuns) == 0 {
		return ""
	}

	dw.drawingID++
	id := dw.drawingID
	var b strings.Builder
	b.WriteString(`<w:r><w:pict>`)
	if !dw.shapetypeDone {
		b.WriteString(vmlShapetype)
		dw.shapetypeDone = true
	}
	fmt.Fprintf(&b, `<v:shape id="tb%d" type="#_x0000_t202" style="position:absolute;margin-left:%.2fpt;margin-top:%.2fpt;width:%.2fpt;height:%.2fpt;mso-position-horizontal-relative:page;mso-position-vertical-relative:page;z-index:%d" filled="f" stroked="f" xmlns:v="urn:schemas-microsoft-com:vml">`,
		id, box.LLX, size.Height-box.URY, wPt, hPt, 100000+id)
	b.WriteString(`<v:textbox inset="0,0,0,0"><w:txbxContent>`)
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="0"/></w:pPr>`)
	dw.writeRuns(&b, lineRuns[0], false)
	b.WriteString(`</w:p>`)
	b.WriteString(`</w:txbxContent></v:textbox></v:shape></w:pict></w:r>`)
	return b.String()
}
