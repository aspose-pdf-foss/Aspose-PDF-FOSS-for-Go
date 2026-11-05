// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// A form with no /Resources takes the names it uses from whatever draws it, which
// ISO 32000-1 §7.8.3 keeps for compatibility and PDF/A-2 and later forbid (ISO 19005-2
// §6.2.2). It also hides the form's fonts from every pass that reads a stream with its
// own resources, so they stayed unembedded.

// resourceUse is a name a content stream looks up in its resources.
type resourceUse struct{ category, name string }

// contentResourceUses returns the names a content stream looks up.
func contentResourceUses(ops []contentOp) []resourceUse {
	var uses []resourceUse
	add := func(category string, v pdfValue) {
		if n, ok := v.(pdfName); ok {
			uses = append(uses, resourceUse{category, string(n)})
		}
	}
	colorSpace := func(v pdfValue) {
		if n, ok := v.(pdfName); ok {
			switch n {
			case "/DeviceGray", "/DeviceRGB", "/DeviceCMYK", "/Pattern", "/G", "/RGB", "/CMYK", "/I":
				return
			}
			add("/ColorSpace", n)
		}
	}
	for _, op := range ops {
		switch op.Operator {
		case "Do":
			add("/XObject", at(op, 0))
		case "gs":
			add("/ExtGState", at(op, 0))
		case "Tf":
			add("/Font", at(op, 0))
		case "sh":
			add("/Shading", at(op, 0))
		case "cs", "CS":
			colorSpace(at(op, 0))
		case "scn", "SCN":
			if n := len(op.Operands); n > 0 {
				add("/Pattern", op.Operands[n-1])
			}
		case "BDC", "DP":
			add("/Properties", at(op, 1))
		case "BI":
			if dict, ok := at(op, 0).(pdfDict); ok {
				colorSpace(dict["/ColorSpace"])
			}
		}
	}
	return uses
}

// inheritedResources is what one form takes from those drawing it.
type inheritedResources struct {
	values   map[resourceUse]pdfValue
	hasOwn   bool // it has /Resources, and they lack some name
	conflict bool // two callers give one name different values
}

// formsInheritingResources walks what the pages draw and returns each form that
// takes a name from its caller.
func (d *Document) formsInheritingResources() map[*pdfStream]*inheritedResources {
	objects := d.objects
	out := map[*pdfStream]*inheritedResources{}
	type visit struct {
		form *pdfStream
		res  uintptr
	}
	seen := map[visit]bool{}
	lookup := func(res pdfDict, u resourceUse) pdfValue {
		dict, _ := resolveRefToDict(objects, res[u.category])
		return dict[u.name]
	}
	var walkContent func(data []byte, res pdfDict, depth int)
	drawForm := func(form *pdfStream, caller pdfDict, depth int) {
		if form == nil || depth > maxFormDepth || dictGetName(form.Dict, "/Subtype") == "/Image" {
			return
		}
		key := visit{form, reflect.ValueOf(caller).Pointer()}
		if seen[key] {
			return
		}
		seen[key] = true
		data := decodedStreamData(form)
		ops, err := parseContentStream(data)
		if err != nil {
			return
		}
		own, hasOwn := resolveRefToDict(objects, form.Dict["/Resources"])
		for _, u := range contentResourceUses(ops) {
			if lookup(own, u) != nil {
				continue
			}
			v := lookup(caller, u)
			if v == nil {
				continue
			}
			inh := out[form]
			if inh == nil {
				inh = &inheritedResources{values: map[resourceUse]pdfValue{}, hasOwn: hasOwn}
				out[form] = inh
			}
			if old, known := inh.values[u]; known && !reflect.DeepEqual(old, v) {
				inh.conflict = true
			}
			inh.values[u] = v
		}
		if hasOwn {
			walkContent(data, own, depth+1)
		} else {
			walkContent(data, caller, depth+1)
		}
	}
	walkContent = func(data []byte, res pdfDict, depth int) {
		ops, err := parseContentStream(data)
		if err != nil {
			return
		}
		for _, op := range ops {
			switch op.Operator {
			case "Do":
				if n, ok := at(op, 0).(pdfName); ok {
					form, _ := resolveRef(objects, lookup(res, resourceUse{"/XObject", string(n)})).(*pdfStream)
					drawForm(form, res, depth)
				}
			case "gs":
				if n, ok := at(op, 0).(pdfName); ok {
					gs, _ := resolveRefToDict(objects, lookup(res, resourceUse{"/ExtGState", string(n)}))
					sm, _ := resolveRefToDict(objects, gs["/SMask"])
					form, _ := resolveRef(objects, sm["/G"]).(*pdfStream)
					drawForm(form, res, depth)
				}
			}
		}
	}
	for _, p := range d.Pages() {
		if data, err := p.contentStreams(); err == nil {
			walkContent(data, p.pageResources(), 0)
		}
		dict := p.pageDict()
		if dict == nil {
			continue
		}
		annots, _ := resolveRefToArray(objects, dict["/Annots"])
		for _, v := range annots {
			ad, _ := resolveRefToDict(objects, v)
			ap, _ := resolveRefToDict(objects, ad["/AP"])
			switch n := resolveRef(objects, ap["/N"]).(type) {
			case *pdfStream:
				drawForm(n, nil, 0)
			case pdfDict:
				for _, state := range n {
					form, _ := resolveRef(objects, state).(*pdfStream)
					drawForm(form, nil, 0)
				}
			}
		}
	}
	return out
}

// fixInheritedResources gives a form with no /Resources the names it takes from its
// callers, which draws exactly what it drew. A form that has resources and lacks a
// name is left, as a reader may not look further and adding it could draw more; so
// is one whose callers disagree, which would need a copy per caller. Both stay
// reported.
func (d *Document) fixInheritedResources() {
	for form, inh := range d.formsInheritingResources() {
		if inh.hasOwn || inh.conflict {
			continue
		}
		res := pdfDict{}
		for u, v := range inh.values {
			dict, _ := res[u.category].(pdfDict)
			if dict == nil {
				dict = pdfDict{}
				res[u.category] = dict
			}
			dict[u.name] = v
		}
		form.Dict["/Resources"] = res
	}
}

// pdfaCheckInheritedResources reports a form taking names from its callers.
func (d *Document) pdfaCheckInheritedResources(format PDFAFormat, r *PDFAValidationReport) {
	if format.part() == 1 {
		return
	}
	var names []string
	for _, inh := range d.formsInheritingResources() {
		for u := range maps.Keys(inh.values) {
			names = append(names, strings.TrimPrefix(u.name, "/"))
		}
	}
	if len(names) > 0 {
		slices.Sort(names)
		r.add("INHERITED_RESOURCES", fmt.Sprintf("a form takes %s from the resources of what draws it; PDF/A requires it to have its own (ISO 19005-2 §6.2.2)", strings.Join(slices.Compact(names), ", ")))
	}
}
