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
	// Pending text edits are drawn first: this rewrites page content.
	if err := p.doc.ApplyTextChanges(); err != nil {
		return err
	}
	return p.doc.removeAllText([]*Page{p})
}

// RemoveAllText removes every text-showing operator from all pages.
func (d *Document) RemoveAllText() error {
	// Pending text edits are drawn first: this rewrites page content.
	if err := d.ApplyTextChanges(); err != nil {
		return err
	}
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
	// The streams the pages and clones replaced, and the property lists
	// replaced by copies without /ActualText, where nothing else uses them,
	// so the removed text does not stay in the saved file.
	d.dropUnreachable(append(append(dropped, r.replaced...), r.scrubbed...))
	return nil
}

const maxTextRemoveDepth = 32

type textRemover struct {
	d *Document
	// clones remembers each form already stripped, so a form drawn many
	// times is read once and cloned once.
	clones   map[formKey]formResult
	replaced []int // forms that were cloned
	scrubbed []int // property lists replaced by a copy without /ActualText
}

// formKey names a form and, for one with no /Resources of its own, the
// resources its names resolve through (they decide what it draws).
type formKey struct {
	st  *pdfStream
	res uintptr
}

// formResult is a stripped form: its clone (0 when it holds no text) and
// the edits it passes up to its caller's resources.
type formResult struct {
	num     int
	bubbled resEdits
}

// resEdits are the changes a stripped stream needs in the resources its
// names resolve through: forms replaced by their clones (/XObject), and,
// when it named a property list carrying /ActualText, every such list
// replaced by a copy without it (/Properties), so the removed text is not
// left reachable from the page (GHSA-gc9q-3vx8-phx2).
type resEdits struct {
	xobj  map[string]int
	props bool
}

func (e *resEdits) empty() bool { return len(e.xobj) == 0 && !e.props }

func (e *resEdits) merge(o resEdits) {
	for k, v := range o.xobj {
		if e.xobj == nil {
			e.xobj = map[string]int{}
		}
		e.xobj[k] = v
	}
	e.props = e.props || o.props
}

// page strips the page's content and installs it, returning the object
// numbers of the content streams it replaced.
func (r *textRemover) page(p *Page) ([]int, error) {
	data, err := p.contentStreams()
	if err != nil || data == nil {
		return nil, err
	}
	res := p.pageResources()
	out, changed, edits, err := r.stream(data, res, 0)
	if err != nil || !changed {
		return nil, err
	}
	pd := p.pageDict()
	old := contentStreamIDs(pd["/Contents"])
	if !edits.empty() {
		pd["/Resources"] = r.editResources(res, edits)
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
// through res. edits are the changes res needs; changed reports whether
// anything was removed.
func (r *textRemover) stream(data []byte, res pdfDict, depth int) (out []byte, changed bool, edits resEdits, err error) {
	if !mayChange(data) {
		return data, false, edits, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, false, edits, err
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
						// A named list stays in /Properties: replace it there
						// too, or the text is still in the file.
						if _, ok := op.Operands[1].(pdfName); ok {
							edits.props = true
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
						return nil, false, edits, err
					}
					if num != 0 {
						edits.merge(resEdits{xobj: map[string]int{string(name): num}})
						changed = true
					}
					// A form with no /Resources of its own resolves its names
					// through ours, so what it changes is ours to install.
					if !sub.empty() {
						edits.merge(sub)
						changed = true
					}
				}
			}
		}
		kept = append(kept, op)
	}
	if !changed {
		return data, false, edits, nil
	}
	return serializeContentOps(kept), true, edits, nil
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
// with no /Resources of its own, the edits it needs in res.
func (r *textRemover) form(res pdfDict, name string, depth int) (int, resEdits, error) {
	var none resEdits
	if res == nil {
		return 0, none, nil
	}
	xobjs, ok := resolveRefToDict(r.d.objects, res["/XObject"])
	if !ok {
		return 0, none, nil
	}
	ref, ok := xobjs[name].(pdfRef)
	if !ok {
		return 0, none, nil
	}
	obj := r.d.objects[ref.Num]
	if obj == nil {
		return 0, none, nil
	}
	st, ok := obj.Value.(*pdfStream)
	if !ok || st.Dict["/Subtype"] != pdfName("/Form") {
		return 0, none, nil
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
		return 0, none, fmt.Errorf("form XObjects nested deeper than %d", maxTextRemoveDepth)
	}
	r.clones[key] = formResult{} // a form drawing itself stops here
	out, changed, edits, err := r.stream(decodedStreamData(st), formRes, depth+1)
	if err != nil || !changed {
		return 0, none, err
	}
	dict := make(pdfDict, len(st.Dict))
	for k, v := range st.Dict {
		if k != "/Filter" && k != "/DecodeParms" && k != "/Length" {
			dict[k] = v
		}
	}
	var bubbled resEdits
	if hasOwn {
		if !edits.empty() {
			dict["/Resources"] = r.editResources(own, edits)
		}
	} else {
		bubbled = edits
	}
	num := r.d.nextID
	r.d.nextID++
	dict["/Length"] = len(out)
	r.d.objects[num] = &pdfObject{Num: num, Value: &pdfStream{Dict: dict, Data: out, Decoded: true}}
	r.clones[key] = formResult{num: num, bubbled: bubbled}
	r.replaced = append(r.replaced, ref.Num)
	return num, bubbled, nil
}

// editResources returns a copy of res with edits installed; res itself is
// left alone, as other pages or forms may share it. The property lists are
// all scrubbed, the ones no stream here names as well: the copy is this
// page's alone, and a list left whole would keep the text in the file. A
// list it replaces is recorded, to be dropped once nothing uses it.
func (r *textRemover) editResources(res pdfDict, edits resEdits) pdfDict {
	out := redirectXObjects(r.d.objects, res, edits.xobj)
	if !edits.props {
		return out
	}
	src, ok := resolveRefToDict(r.d.objects, res["/Properties"])
	if !ok {
		return out
	}
	props := make(pdfDict, len(src))
	for name, v := range src {
		props[name] = v
		p, ok := resolveRefToDict(r.d.objects, v)
		if !ok || p["/ActualText"] == nil {
			continue
		}
		scrubbed := make(pdfDict, len(p))
		for k, pv := range p {
			if k != "/ActualText" {
				scrubbed[k] = pv
			}
		}
		props[name] = scrubbed
		if ref, ok := v.(pdfRef); ok {
			r.scrubbed = append(r.scrubbed, ref.Num)
		}
	}
	out["/Properties"] = props
	return out
}

// redirectXObjects returns a copy of res whose /XObject entries named in
// redirect point at the given objects; res itself is left alone, as other
// pages or forms may share it.
func redirectXObjects(objects map[int]*pdfObject, res pdfDict, redirect map[string]int) pdfDict {
	out := make(pdfDict, len(res)+1)
	for k, v := range res {
		out[k] = v
	}
	if len(redirect) == 0 {
		return out
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
