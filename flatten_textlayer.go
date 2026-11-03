// SPDX-License-Identifier: MIT

package asposepdf

import "reflect"

// A rasterized page keeps its text as an invisible layer over the image, so it can
// still be searched, selected and copied. The layer is the page's own text
// operators with everything that paints or depends on painting taken away, in text
// rendering mode 3 (ISO 32000-1 §9.3.6): the same fonts, positions and marked
// content, so extraction reads what it read before.

// maxTextLayerDepth caps Form XObject nesting, as text extraction does.
const maxTextLayerDepth = 32

type textLayerKey struct {
	stream *pdfStream
	res    uintptr // a form without /Resources resolves names through its caller's
}

type textLayerBuilder struct {
	d      *Document
	forms  map[textLayerKey]int // the clone's object number; 0 when the form has no text
	active map[*pdfStream]bool  // guards against a form drawing itself
}

func newTextLayerBuilder(d *Document) *textLayerBuilder {
	return &textLayerBuilder{d: d, forms: map[textLayerKey]int{}, active: map[*pdfStream]bool{}}
}

// rewrite returns the text of a content stream as an invisible layer, the resources
// that layer needs, and whether it shows any text at all.
func (b *textLayerBuilder) rewrite(data []byte, res pdfDict, depth int) ([]byte, pdfDict, bool) {
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, nil, false
	}
	objects := b.d.objects
	fonts, _ := resolveRefToDict(objects, res["/Font"])
	xobjects, _ := resolveRefToDict(objects, res["/XObject"])
	properties, _ := resolveRefToDict(objects, res["/Properties"])
	outFonts, outXObjects, outProperties := pdfDict{}, pdfDict{}, pdfDict{}

	var out []contentOp
	hasText, inText, showable := false, false, true
	var saved []bool // showable is part of the graphics state, as the font is
	for _, op := range ops {
		switch op.Operator {
		case "q":
			saved = append(saved, showable)
			out = append(out, op)
		case "Q":
			// A stray Q would pop what the page around this layer pushed.
			if n := len(saved); n > 0 {
				showable, saved = saved[n-1], saved[:n-1]
				out = append(out, op)
			}
		case "cm", "Tc", "Tw", "Tz", "TL", "Ts", "Td", "TD", "Tm", "T*", "BMC", "EMC", "MP":
			out = append(out, op)
		case "BDC", "DP":
			if len(op.Operands) == 2 {
				if name, ok := op.Operands[1].(pdfName); ok {
					if v, ok := properties[string(name)]; ok {
						outProperties[string(name)] = v
					}
				}
			}
			out = append(out, op)
		case "BT":
			inText = true
			// Set in every text object, so nothing the source does can undo it.
			out = append(out, op, contentOp{Operator: "Tr", Operands: []pdfValue{3}})
		case "ET":
			inText = false
			out = append(out, op)
		case "Tf":
			if len(op.Operands) == 0 {
				continue
			}
			name, ok := op.Operands[0].(pdfName)
			if !ok {
				continue
			}
			font, known := fonts[string(name)]
			if dict, ok := resolveRefToDict(objects, font); ok && dictGetName(dict, "/Subtype") == "/Type3" {
				// A Type3 glyph is a content stream, which mode 3 does not stop
				// from painting: it would draw over the image.
				showable = false
				continue
			}
			showable = true
			if known {
				outFonts[string(name)] = font
			}
			out = append(out, op)
		case "Tj", "TJ", "'", "\"":
			if inText && showable {
				hasText = true
				out = append(out, op)
			}
		case "Do":
			if len(op.Operands) == 0 {
				continue
			}
			name, ok := op.Operands[0].(pdfName)
			if !ok {
				continue
			}
			st, ok := resolveRef(objects, xobjects[string(name)]).(*pdfStream)
			if !ok || dictGetName(st.Dict, "/Subtype") != "/Form" {
				continue
			}
			if id := b.form(st, res, depth+1); id != 0 {
				outXObjects[string(name)] = pdfRef{Num: id}
				hasText = true
				out = append(out, op)
			}
		}
	}
	if !hasText {
		return nil, nil, false
	}
	for range saved {
		out = append(out, contentOp{Operator: "Q"})
	}
	outRes := pdfDict{}
	if len(outFonts) > 0 {
		outRes["/Font"] = outFonts
	}
	if len(outXObjects) > 0 {
		outRes["/XObject"] = outXObjects
	}
	if len(outProperties) > 0 {
		outRes["/Properties"] = outProperties
	}
	return serializeContentOps(out), outRes, true
}

// form returns the object number of a Form XObject holding only st's text, or 0
// when it has none. The original is left alone: another page may draw it.
func (b *textLayerBuilder) form(st *pdfStream, parent pdfDict, depth int) int {
	if depth > maxTextLayerDepth || b.active[st] {
		return 0
	}
	res := parent
	if own, ok := resolveRefToDict(b.d.objects, st.Dict["/Resources"]); ok {
		res = own
	}
	key := textLayerKey{stream: st, res: reflect.ValueOf(res).Pointer()}
	if id, done := b.forms[key]; done {
		return id
	}
	b.active[st] = true
	content, outRes, hasText := b.rewrite(decodedStreamData(st), res, depth)
	delete(b.active, st)
	if !hasText {
		b.forms[key] = 0
		return 0
	}
	clone := buildFlateStream(content)
	clone.Dict["/Type"], clone.Dict["/Subtype"] = pdfName("/XObject"), pdfName("/Form")
	clone.Dict["/Resources"] = outRes
	for _, k := range []string{"/BBox", "/Matrix"} {
		if v, ok := st.Dict[k]; ok {
			clone.Dict[k] = v
		}
	}
	id := b.d.addObject(clone)
	b.forms[key] = id
	return id
}

// textLayerFor builds the invisible layer for a page; ok is false when the page
// shows no text a layer could carry.
func (p *Page) textLayerFor() (content []byte, res pdfDict, ok bool) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, nil, false
	}
	return newTextLayerBuilder(p.doc).rewrite(data, p.pageResources(), 0)
}
