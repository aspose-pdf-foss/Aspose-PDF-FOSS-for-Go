// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"reflect"
)

// Removing all text from a page or a document, mirroring Aspose.PDF for
// .NET's TextFragmentAbsorber.RemoveAllText(Page) / RemoveAllText(Document)
// (pdf-go-7ukm). Every text-showing operator goes — in the page content and in
// the Form XObjects it draws — while graphics, images and text state stay, and
// /ActualText, which extraction reports in place of the glyphs, goes with the
// text it stood for. Unlike ApplyRedactions over the whole page it needs no
// glyph geometry, so text in a font whose widths are unknown is removed too.
//
// A Form XObject holding text is cloned, not edited, so another page drawing
// the same form keeps it; within one call a form is cloned once however many
// pages draw it. Annotation appearances, tiling-pattern cells and Type3 glyph
// procedures are left as they are.

// RemoveAllText removes every text-showing operator from the page content.
func (p *Page) RemoveAllText() error {
	return p.doc.removeAllText([]*Page{p})
}

// RemoveAllText removes every text-showing operator from all pages.
func (d *Document) RemoveAllText() error {
	return d.removeAllText(d.Pages())
}

func (d *Document) removeAllText(pages []*Page) error {
	r := &textRemover{d: d, clones: map[formKey]formResult{}}
	var dropped []int
	for _, p := range pages {
		ids, err := r.page(p)
		if err != nil {
			return fmt.Errorf("remove text: page %d: %w", p.Number(), err)
		}
		dropped = append(dropped, ids...)
	}
	// The streams the pages and clones replaced, where nothing else uses
	// them, so the removed text does not stay in the saved file.
	d.dropUnreachable(append(dropped, r.replaced...))
	return nil
}

const maxTextRemoveDepth = 32

type textRemover struct {
	d *Document
	// clones remembers each form already stripped, so a form drawn many
	// times is read once and cloned once.
	clones   map[formKey]formResult
	replaced []int // forms that were cloned
}

// formKey names a form and, for one with no /Resources of its own, the
// resources its names resolve through (they decide what it draws).
type formKey struct {
	st  *pdfStream
	res uintptr
}

// formResult is a stripped form: its clone (0 when it holds no text) and
// the redirections it passes up to its caller's resources.
type formResult struct {
	num     int
	bubbled map[string]int
}

// page strips the page's content and installs it, returning the object
// numbers of the content streams it replaced.
func (r *textRemover) page(p *Page) ([]int, error) {
	data, err := p.contentStreams()
	if err != nil || data == nil {
		return nil, err
	}
	res := p.pageResources()
	out, changed, redirect, err := r.stream(data, res, 0)
	if err != nil || !changed {
		return nil, err
	}
	pd := p.pageDict()
	old := contentStreamIDs(pd["/Contents"])
	if len(redirect) > 0 {
		pd["/Resources"] = redirectXObjects(r.d.objects, res, redirect)
	}
	if err := replacePageContents(p, out); err != nil {
		return nil, err
	}
	return old, nil
}

// contentStreamIDs lists the object numbers a /Contents value refers to.
func contentStreamIDs(v pdfValue) []int {
	switch c := v.(type) {
	case pdfRef:
		return []int{c.Num}
	case pdfArray:
		var ids []int
		for _, e := range c {
			if ref, ok := e.(pdfRef); ok {
				ids = append(ids, ref.Num)
			}
		}
		return ids
	}
	return nil
}

// stream removes the text from one content stream whose names resolve
// through res. redirect maps the /XObject names in res whose forms were
// cloned to their clones; changed reports whether anything was removed.
func (r *textRemover) stream(data []byte, res pdfDict, depth int) (out []byte, changed bool, redirect map[string]int, err error) {
	if !mayChange(data) {
		return data, false, nil, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, false, nil, err
	}
	kept := ops[:0:0]
	for _, op := range ops {
		switch op.Operator {
		case "Tj", "TJ":
			changed = true
			continue
		case "'":
			// Move to the next line, then show: keep the move.
			changed = true
			op = contentOp{Operator: "T*"}
		case "\"":
			// Set word and character spacing, move, show: keep all but the show.
			changed = true
			if len(op.Operands) >= 2 {
				kept = append(kept,
					contentOp{Operator: "Tw", Operands: []pdfValue{op.Operands[0]}},
					contentOp{Operator: "Tc", Operands: []pdfValue{op.Operands[1]}})
			}
			op = contentOp{Operator: "T*"}
		case "BDC":
			if len(op.Operands) >= 2 {
				if props, ok := resolveBDCProperties(r.d.objects, res, op.Operands[1]); ok {
					if _, has := props["/ActualText"]; has {
						scrubbed := make(pdfDict, len(props))
						for k, v := range props {
							if k != "/ActualText" {
								scrubbed[k] = v
							}
						}
						op = contentOp{Operator: "BDC", Operands: []pdfValue{op.Operands[0], scrubbed}}
						changed = true
					}
				}
			}
		case "Do":
			if len(op.Operands) == 1 {
				if name, ok := op.Operands[0].(pdfName); ok {
					num, sub, err := r.form(res, string(name), depth)
					if err != nil {
						return nil, false, nil, err
					}
					if num != 0 {
						if redirect == nil {
							redirect = map[string]int{}
						}
						redirect[string(name)] = num
						changed = true
					}
					// A form with no /Resources of its own resolves its names
					// through ours, so what it redirects is ours to install.
					for k, v := range sub {
						if redirect == nil {
							redirect = map[string]int{}
						}
						redirect[k] = v
						changed = true
					}
				}
			}
		}
		kept = append(kept, op)
	}
	if !changed {
		return data, false, nil, nil
	}
	return serializeContentOps(kept), true, redirect, nil
}

// mayChange reports whether a content stream could hold anything this walk
// changes: a show operator, marked content or a Do. Most large forms — maps,
// drawings — hold none, and are not parsed at all.
func mayChange(data []byte) bool {
	for _, tok := range [][]byte{[]byte("Tj"), []byte("TJ"), []byte("Do"), []byte("BDC"), []byte("'"), []byte(`"`)} {
		if bytes.Contains(data, tok) {
			return true
		}
	}
	return false
}

// form strips the Form XObject res names, returning the number of its
// text-free clone (0 when it holds no text or is not a form) and, for a form
// with no /Resources of its own, the redirections it needs in res.
func (r *textRemover) form(res pdfDict, name string, depth int) (int, map[string]int, error) {
	if res == nil {
		return 0, nil, nil
	}
	xobjs, ok := resolveRefToDict(r.d.objects, res["/XObject"])
	if !ok {
		return 0, nil, nil
	}
	ref, ok := xobjs[name].(pdfRef)
	if !ok {
		return 0, nil, nil
	}
	obj := r.d.objects[ref.Num]
	if obj == nil {
		return 0, nil, nil
	}
	st, ok := obj.Value.(*pdfStream)
	if !ok || st.Dict["/Subtype"] != pdfName("/Form") {
		return 0, nil, nil
	}
	own, hasOwn := resolveRefToDict(r.d.objects, st.Dict["/Resources"])
	key := formKey{st: st}
	formRes := own
	if !hasOwn {
		formRes = res
		key.res = reflect.ValueOf(res).Pointer()
	}
	if done, seen := r.clones[key]; seen {
		return done.num, done.bubbled, nil
	}
	if depth >= maxTextRemoveDepth {
		return 0, nil, fmt.Errorf("form XObjects nested deeper than %d", maxTextRemoveDepth)
	}
	r.clones[key] = formResult{} // a form drawing itself stops here
	out, changed, redirect, err := r.stream(decodedStreamData(st), formRes, depth+1)
	if err != nil || !changed {
		return 0, nil, err
	}
	dict := make(pdfDict, len(st.Dict))
	for k, v := range st.Dict {
		if k != "/Filter" && k != "/DecodeParms" && k != "/Length" {
			dict[k] = v
		}
	}
	var bubbled map[string]int
	if hasOwn {
		if len(redirect) > 0 {
			dict["/Resources"] = redirectXObjects(r.d.objects, own, redirect)
		}
	} else {
		bubbled = redirect
	}
	num := r.d.nextID
	r.d.nextID++
	dict["/Length"] = len(out)
	r.d.objects[num] = &pdfObject{Num: num, Value: &pdfStream{Dict: dict, Data: out, Decoded: true}}
	r.clones[key] = formResult{num: num, bubbled: bubbled}
	r.replaced = append(r.replaced, ref.Num)
	return num, bubbled, nil
}

// redirectXObjects returns a copy of res whose /XObject entries named in
// redirect point at the given objects; res itself is left alone, as other
// pages or forms may share it.
func redirectXObjects(objects map[int]*pdfObject, res pdfDict, redirect map[string]int) pdfDict {
	out := make(pdfDict, len(res)+1)
	for k, v := range res {
		out[k] = v
	}
	xobjs := pdfDict{}
	if src, ok := resolveRefToDict(objects, res["/XObject"]); ok {
		for k, v := range src {
			xobjs[k] = v
		}
	}
	for name, num := range redirect {
		xobjs[name] = pdfRef{Num: num}
	}
	out["/XObject"] = xobjs
	return out
}
