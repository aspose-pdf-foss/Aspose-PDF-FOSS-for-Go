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

// defaultColorSpaceUses are the default colour spaces (ISO 32000-1 §8.6.5.6)
// a form's device colours take from its resources.
var defaultColorSpaceUses = []resourceUse{
	{"/ColorSpace", "/DefaultGray"}, {"/ColorSpace", "/DefaultRGB"}, {"/ColorSpace", "/DefaultCMYK"},
}

// inheritedResources is what one form takes from those drawing it.
type inheritedResources struct {
	values   map[resourceUse]pdfValue
	hasOwn   bool // it has /Resources, and they lack some name
	conflict bool // two callers give one name different values
	// absent holds the default colour spaces a caller does not have:
	// absence means device colour there, so a caller with one and a caller
	// without one disagree.
	absent map[resourceUse]bool
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
	// A form is decoded and parsed once, and the names it uses listed once,
	// however many resource contexts draw it; one with its own resources is
	// walked into once. It used to be parsed again for every caller: one
	// large form drawn from a hundred small ones took 20 s and 19 GB
	// (GHSA-rfwg-hxcj-7pp5 point 7).
	type parsedForm struct {
		ops  []contentOp
		uses []resourceUse
		ok   bool
	}
	parsed := map[*pdfStream]*parsedForm{}
	parse := func(form *pdfStream) *parsedForm {
		if pf, ok := parsed[form]; ok {
			return pf
		}
		pf := &parsedForm{}
		parsed[form] = pf
		ops, err := parseContentStream(decodedStreamData(form))
		if err != nil {
			return pf
		}
		pf.ops, pf.ok = ops, true
		seenUse := map[resourceUse]bool{}
		for _, u := range contentResourceUses(ops) {
			if !seenUse[u] {
				seenUse[u] = true
				pf.uses = append(pf.uses, u)
			}
		}
		return pf
	}
	walkedOwn := map[*pdfStream]bool{}
	var walkOps func(ops []contentOp, res pdfDict, depth int)
	drawForm := func(form *pdfStream, caller pdfDict, depth int) {
		if form == nil || depth > maxFormDepth || dictGetName(form.Dict, "/Subtype") == "/Image" {
			return
		}
		key := visit{form, reflect.ValueOf(caller).Pointer()}
		if seen[key] {
			return
		}
		seen[key] = true
		pf := parse(form)
		if !pf.ok {
			return
		}
		own, hasOwn := resolveRefToDict(objects, form.Dict["/Resources"])
		// The caller's default colour spaces apply to whatever a form with
		// no resources of its own paints in a device space, the initial
		// black included, so they go with it too (GitHub issue #26 point
		// 6); a caller without them adds nothing.
		uses := pf.uses
		if !hasOwn {
			uses = append(slices.Clip(uses), defaultColorSpaceUses...)
		}
		for _, u := range uses {
			if lookup(own, u) != nil {
				continue
			}
			v := lookup(caller, u)
			isDefault := slices.Contains(defaultColorSpaceUses, u)
			if v == nil && !isDefault {
				continue
			}
			inh := out[form]
			if inh == nil {
				inh = &inheritedResources{values: map[resourceUse]pdfValue{}, absent: map[resourceUse]bool{}, hasOwn: hasOwn}
				out[form] = inh
			}
			if v == nil {
				if _, known := inh.values[u]; known {
					inh.conflict = true
				}
				inh.absent[u] = true
				continue
			}
			if old, known := inh.values[u]; (known && !reflect.DeepEqual(old, v)) || inh.absent[u] {
				inh.conflict = true
			}
			inh.values[u] = v
		}
		if hasOwn {
			if !walkedOwn[form] {
				walkedOwn[form] = true
				walkOps(pf.ops, own, depth+1)
			}
		} else {
			walkOps(pf.ops, caller, depth+1)
		}
	}
	walkOps = func(ops []contentOp, res pdfDict, depth int) {
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
			if ops, err := parseContentStream(data); err == nil {
				walkOps(ops, p.pageResources(), 0)
			}
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
		if inh.hasOwn || inh.conflict || len(inh.values) == 0 {
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
			// Taken implicitly, not by name: not what the rule is about.
			if !slices.Contains(defaultColorSpaceUses, u) {
				names = append(names, strings.TrimPrefix(u.name, "/"))
			}
		}
	}
	if len(names) > 0 {
		slices.Sort(names)
		r.add("INHERITED_RESOURCES", fmt.Sprintf("a form takes %s from the resources of what draws it; PDF/A requires it to have its own (ISO 19005-2 §6.2.2)", strings.Join(slices.Compact(names), ", ")))
	}
}
