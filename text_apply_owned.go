// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// textBackgroundTag is the private marked-content tag a text background is
// drawn in: /AsposeTextBackground <</N n>> BDC … EMC. The id n names the
// section within its page, so a redraw removes exactly the box the fragment
// drew, never a path that only looks like it. Not /Artifact: an artifact
// inside tagged content (text drawn in TagContent) breaks PDF/UA, while a
// section with a private tag changes nothing about what is tagged.
const textBackgroundTag = "/AsposeTextBackground"

// drawTextBackground fills box with c inside its own marked-content section
// and returns the section's id.
func (p *Page) drawTextBackground(box Rectangle, c *Color) (int, error) {
	if err := p.checkCanvasNotFrozen(); err != nil {
		return 0, err
	}
	id := p.nextTextBackgroundID()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s <</N %d>> BDC\nq\n", textBackgroundTag, id)
	gsOp, err := p.applyAlpha(nil, c)
	if err != nil {
		return 0, err
	}
	buf.WriteString(gsOp)
	emitRectangleToBuf(&buf, p, box, ShapeStyle{FillColor: c})
	buf.WriteString("Q\nEMC\n")
	return id, p.appendToContentStream(buf.Bytes())
}

// nextTextBackgroundID returns an id no background section on the page has.
// The page's content is read once per session for the ids already there (a
// document saved with backgrounds and opened again).
func (p *Page) nextTextBackgroundID() int {
	d, obj := p.doc, p.pageObj()
	if d.textBgNext == nil {
		d.textBgNext = map[*pdfObject]int{}
	}
	next, ok := d.textBgNext[obj]
	if !ok {
		next = 1
		if data, err := p.contentStreams(); err == nil {
			if ops, err := parseContentStream(data); err == nil {
				for _, op := range ops {
					if id := textBackgroundID(op); id >= next {
						next = id + 1
					}
				}
			}
		}
	}
	d.textBgNext[obj] = next + 1
	return next
}

// textBackgroundID returns the id of the background section op opens, or 0.
func textBackgroundID(op contentOp) int {
	if op.Operator != "BDC" || len(op.Operands) != 2 || op.Operands[0] != pdfName(textBackgroundTag) {
		return 0
	}
	props, ok := op.Operands[1].(pdfDict)
	if !ok {
		return 0
	}
	if n, ok := props["/N"].(int); ok && n > 0 {
		return n
	}
	return 0
}

// removeTextBackgrounds drops the background sections named by ids from the
// content data, each from its BDC to the matching EMC.
func removeTextBackgrounds(data []byte, ids map[int]bool) ([]byte, error) {
	if len(ids) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}
	out := ops[:0:0]
	depth := 0 // > 0 inside a section being dropped
	for _, op := range ops {
		if depth > 0 {
			switch op.Operator {
			case "BDC", "BMC":
				depth++
			case "EMC":
				depth--
			}
			continue
		}
		if ids[textBackgroundID(op)] {
			depth = 1
			continue
		}
		out = append(out, op)
	}
	if depth > 0 {
		return nil, fmt.Errorf("text background section without its EMC")
	}
	return serializeContentOps(out), nil
}

// formGraphRefs returns the Form and Image XObjects a resources dictionary
// names, and those named by the forms' own resources, transitively: what an
// edit may leave unreachable once it cloned the forms it reaches into.
func formGraphRefs(objects map[int]*pdfObject, res pdfDict) []int {
	var ids []int
	seen := map[int]bool{}
	var walk func(res pdfDict)
	walk = func(res pdfDict) {
		xo, ok := resolveRefToDict(objects, res["/XObject"])
		if !ok {
			return
		}
		for _, v := range xo {
			r, ok := v.(pdfRef)
			if !ok || seen[r.Num] {
				continue
			}
			seen[r.Num] = true
			ids = append(ids, r.Num)
			if o, ok := objects[r.Num]; ok {
				if s, ok := o.Value.(*pdfStream); ok {
					if sub, ok := resolveRefToDict(objects, s.Dict["/Resources"]); ok {
						walk(sub)
					}
				}
			}
		}
	}
	walk(res)
	return ids
}
