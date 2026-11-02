// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
)

// atAnnot is one annotation of a page on its way into the structure tree.
type atAnnot struct {
	num  int // annotation object
	dict pdfDict
	rect Rectangle
	typ  StructType
	alt  string
}

// atPageAnnots lists the annotations of p that belong in the structure tree:
// PDF/UA leaves out hidden ones, popups (which only show their parent's text),
// printer's marks and trap networks. A direct annotation dictionary is promoted
// to an object, since a reference needs a number to point at.
func atPageAnnots(d *Document, p *Page) []atAnnot {
	annots, _ := resolveRef(d.objects, p.pageDict()["/Annots"]).(pdfArray)
	var out []atAnnot
	for i, v := range annots {
		dict, ok := resolveRefToDict(d.objects, v)
		if !ok {
			continue
		}
		sub := dictGetName(dict, "/Subtype")
		flags, _ := dict["/F"].(int)
		if sub == "/Popup" || sub == "/PrinterMark" || sub == "/TrapNet" || flags&(2|32) != 0 {
			continue
		}
		ref, isRef := v.(pdfRef)
		if !isRef {
			ref = pdfRef{Num: d.addObject(dict)}
			annots[i] = ref
		}
		a := atAnnot{num: ref.Num, dict: dict, rect: atAnnotRect(dict), typ: StructAnnot}
		switch sub {
		case "/Link":
			a.typ = StructLink
			if contents, _ := dict["/Contents"].(string); contents == "" {
				dict["/Contents"] = encodeFormString(atLinkDescription(d, p, a.rect, dict))
			}
		case "/Widget":
			a.typ = StructForm
			if atFieldTooltip(d, dict) == "" {
				a.alt = atFieldName(d, dict)
			}
		default:
			if contents, _ := dict["/Contents"].(string); contents == "" {
				a.alt = strings.TrimPrefix(sub, "/") + " annotation"
			}
		}
		out = append(out, a)
	}
	return out
}

func atAnnotRect(dict pdfDict) Rectangle {
	arr, _ := dict["/Rect"].(pdfArray)
	if len(arr) != 4 {
		return Rectangle{}
	}
	x0, _ := toFloat(arr[0])
	y0, _ := toFloat(arr[1])
	x1, _ := toFloat(arr[2])
	y1, _ := toFloat(arr[3])
	return Rectangle{LLX: min(x0, x1), LLY: min(y0, y1), URX: max(x0, x1), URY: max(y0, y1)}
}

// atLinkDescription gives a link the alternate description PDF/UA requires:
// the text under it, else its URI, else a generic word.
func atLinkDescription(d *Document, p *Page, rect Rectangle, dict pdfDict) string {
	var words []string
	if lines, err := p.ExtractTextWithLayout(); err == nil {
		for _, l := range lines {
			for _, f := range l.Fragments {
				if atContains(rect, f.X+f.Width/2, f.Y+f.Height/2) {
					words = append(words, strings.TrimSpace(f.Text))
				}
			}
		}
	}
	if text := strings.TrimSpace(strings.Join(words, " ")); text != "" {
		return text
	}
	if act, ok := resolveRefToDict(d.objects, dict["/A"]); ok {
		if uri, ok := act["/URI"].(string); ok && uri != "" {
			return uri
		}
	}
	return "Link"
}

// atFieldTooltip returns the field's /TU, which may sit on a parent field.
func atFieldTooltip(d *Document, dict pdfDict) string {
	for range maxFormDepth {
		if tu, ok := dict["/TU"].(string); ok && tu != "" {
			return tu
		}
		parent, ok := resolveRefToDict(d.objects, dict["/Parent"])
		if !ok {
			return ""
		}
		dict = parent
	}
	return ""
}

// atFieldName returns the field's name, the best description available when
// it has no tooltip.
func atFieldName(d *Document, dict pdfDict) string {
	for range maxFormDepth {
		if t, ok := dict["/T"].(string); ok && t != "" {
			return decodeFormString(t)
		}
		parent, ok := resolveRefToDict(d.objects, dict["/Parent"])
		if !ok {
			break
		}
		dict = parent
	}
	return "Form field"
}

// atPlaceAnnots gives each annotation its place in the tree: under the text
// element it overlaps most, otherwise among the top-level elements, before
// the first that starts below it.
func atPlaceAnnots(tops []*atElem, annots []atAnnot) []*atElem {
	leaves := atLeaves(tops, nil)
	for i := range annots {
		a := &annots[i]
		var best *atElem
		bestArea := 0.0
		for _, e := range leaves {
			if len(e.mcids) == 0 {
				continue
			}
			for _, r := range e.regions {
				if area := atOverlap(r, a.rect.LLX, a.rect.LLY, a.rect.URX, a.rect.URY); area > bestArea {
					best, bestArea = e, area
				}
			}
		}
		if best != nil {
			best.annots = append(best.annots, a)
			continue
		}
		el := &atElem{typ: a.typ, leaf: true, annots: []*atAnnot{a}}
		at := len(tops)
		for j, e := range tops {
			if t, ok := atElemTop(e); ok && t < a.rect.URY {
				at = j
				break
			}
		}
		tops = append(tops[:at], append([]*atElem{el}, tops[at:]...)...)
	}
	return tops
}

// atElemTop returns the top of e's content, if it has any.
func atElemTop(e *atElem) (float64, bool) {
	top, ok := 0.0, false
	for _, l := range atLeaves([]*atElem{e}, nil) {
		if len(l.mcids) == 0 {
			continue
		}
		for _, r := range l.regions {
			if !ok || r.URY > top {
				top, ok = r.URY, true
			}
		}
		if len(l.regions) == 0 && (!ok || l.top > top) {
			top, ok = l.top, true
		}
	}
	return top, ok
}

// atWriteAnnot writes a's element under parent, on page pageNum.
func atWriteAnnot(tc *TaggedContent, parent *StructElement, a *atAnnot, pageNum int, res *AutoTagResult) {
	el := parent.AddChild(a.typ)
	el.dict["/Pg"] = pdfRef{Num: pageNum}
	if a.alt != "" {
		el.dict["/Alt"] = encodeFormString(a.alt)
	}
	el.addAnnotRef(pageNum, a.num, a.dict)
	res.Annotations++
}
