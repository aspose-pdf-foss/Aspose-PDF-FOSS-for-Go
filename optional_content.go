// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Optional Content / layers (ISO 32000-1 §8.11). A Layer is an Optional Content
// Group (OCG) — a named group of content that a viewer can switch on or off.
// Author a layer with Document.AddLayer, bracket the page content that belongs
// to it with Page.BeginLayer / Page.EndLayer, and set its default visibility
// with Layer.SetVisible. The built-in renderer honors the default configuration
// (content of a hidden layer is not drawn), and viewers expose the layers in
// their Layers panel. Mirrors the intent of Aspose.PDF for .NET's
// Document.Layers / OptionalContentGroup.

// Layer is one Optional Content Group (OCG) in the document.
type Layer struct {
	doc *Document
	num int // OCG object number
}

// AddLayer creates a new layer (OCG) named name and registers it in the
// document's optional-content properties. The layer is visible by default;
// call SetVisible(false) to hide it. Returns the layer for assigning content
// (Page.BeginLayer) and tuning visibility.
func (d *Document) AddLayer(name string) *Layer {
	ocg := pdfDict{
		"/Type": pdfName("/OCG"),
		"/Name": encodeFormString(name),
	}
	num := d.nextID
	d.nextID++
	d.objects[num] = &pdfObject{Num: num, Value: ocg}

	ocp := d.ocPropertiesDict()
	ref := pdfRef{Num: num}
	ocp["/OCGs"] = appendToArrayValue(d, ocp["/OCGs"], ref)
	if dcfg, ok := resolveRefToDict(d.objects, ocp["/D"]); ok {
		dcfg["/Order"] = appendToArrayValue(d, dcfg["/Order"], ref)
	}
	return &Layer{doc: d, num: num}
}

// Layers returns the document's layers (OCGs), in /OCProperties/OCGs order.
// Empty for a document with no optional content.
func (d *Document) Layers() []*Layer {
	ocp, ok := resolveRefToDict(d.objects, d.catalog["/OCProperties"])
	if !ok {
		return nil
	}
	arr, _ := resolveRefToArray(d.objects, ocp["/OCGs"])
	var out []*Layer
	for _, e := range arr {
		ref, ok := e.(pdfRef)
		if !ok {
			continue
		}
		if obj, ok := d.objects[ref.Num]; ok {
			if dd, ok := obj.Value.(pdfDict); ok && dictGetName(dd, "/Type") == "/OCG" {
				out = append(out, &Layer{doc: d, num: ref.Num})
			}
		}
	}
	return out
}

// Name returns the layer's display name (its /Name).
func (l *Layer) Name() string {
	if d, ok := l.ocg(); ok {
		return decodeFormString(d["/Name"])
	}
	return ""
}

// SetName sets the layer's display name.
func (l *Layer) SetName(name string) {
	if d, ok := l.ocg(); ok {
		d["/Name"] = encodeFormString(name)
	}
}

// IsVisible reports whether the layer is shown by default (not listed in the
// default configuration's /OFF set).
func (l *Layer) IsVisible() bool {
	dcfg, ok := l.doc.ocDConfig(false)
	if !ok {
		return true
	}
	if off, ok := resolveRefToArray(l.doc.objects, dcfg["/OFF"]); ok {
		for _, e := range off {
			if ref, ok := e.(pdfRef); ok && ref.Num == l.num {
				return false
			}
		}
	}
	return true
}

// SetVisible sets the layer's default visibility (its membership in the default
// configuration's /OFF set). Viewers can still toggle it interactively.
func (l *Layer) SetVisible(visible bool) {
	dcfg, _ := l.doc.ocDConfig(true)
	if visible {
		dcfg["/OFF"] = removeRefFromArrayValue(l.doc, dcfg["/OFF"], l.num)
	} else {
		dcfg["/OFF"] = appendToArrayValue(l.doc, dcfg["/OFF"], pdfRef{Num: l.num})
	}
}

func (l *Layer) ocg() (pdfDict, bool) {
	if obj, ok := l.doc.objects[l.num]; ok {
		if d, ok := obj.Value.(pdfDict); ok {
			return d, true
		}
	}
	return nil, false
}

// BeginLayer starts a marked-content section on the page assigned to layer:
// content drawn after this call (AddText, Draw*, AddImage, …) belongs to the
// layer until EndLayer. Pair every BeginLayer with an EndLayer.
func (p *Page) BeginLayer(layer *Layer) error {
	if layer == nil {
		return fmt.Errorf("BeginLayer: nil layer")
	}
	if layer.doc != p.doc {
		return fmt.Errorf("BeginLayer: layer belongs to a different document")
	}
	name := p.registerOCGProperty(layer.num)
	return p.appendToContentStream([]byte("/OC " + name + " BDC\n"))
}

// EndLayer closes the marked-content section opened by BeginLayer.
func (p *Page) EndLayer() error {
	return p.appendToContentStream([]byte("EMC\n"))
}

// registerOCGProperty ensures the page /Resources/Properties maps a name to the
// OCG and returns that name (reusing an existing mapping when present).
func (p *Page) registerOCGProperty(ocgNum int) string {
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	props, ok := resolveRef(p.doc.objects, resources["/Properties"]).(pdfDict)
	if !ok {
		props = pdfDict{}
		resources["/Properties"] = props
	}
	for k, v := range props {
		if r, ok := v.(pdfRef); ok && r.Num == ocgNum {
			return k
		}
	}
	name := ""
	for i := 0; ; i++ {
		name = fmt.Sprintf("/oc%d", i)
		if _, exists := props[name]; !exists {
			break
		}
	}
	props[name] = pdfRef{Num: ocgNum}
	return name
}

// ocPropertiesDict returns the catalog's /OCProperties dict, creating it (with
// an empty /OCGs and a default configuration /D) when absent.
func (d *Document) ocPropertiesDict() pdfDict {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	if ocp, ok := resolveRefToDict(d.objects, d.catalog["/OCProperties"]); ok {
		if _, ok := ocp["/OCGs"]; !ok {
			ocp["/OCGs"] = pdfArray{}
		}
		if _, ok := ocp["/D"]; !ok {
			ocp["/D"] = pdfDict{"/Order": pdfArray{}}
		}
		return ocp
	}
	ocp := pdfDict{"/OCGs": pdfArray{}, "/D": pdfDict{"/Order": pdfArray{}}}
	d.catalog["/OCProperties"] = ocp
	return ocp
}

// ocDConfig returns the default configuration (/OCProperties/D) dict. When
// create is false and there is no optional content, returns (nil, false).
func (d *Document) ocDConfig(create bool) (pdfDict, bool) {
	if !create {
		ocp, ok := resolveRefToDict(d.objects, d.catalog["/OCProperties"])
		if !ok {
			return nil, false
		}
		dcfg, ok := resolveRefToDict(d.objects, ocp["/D"])
		return dcfg, ok
	}
	ocp := d.ocPropertiesDict()
	dcfg, _ := resolveRefToDict(d.objects, ocp["/D"])
	return dcfg, true
}

// appendToArrayValue appends item to an array value that may be inline or an
// indirect reference, returning the (possibly unchanged) value to store back.
func appendToArrayValue(d *Document, v pdfValue, item pdfValue) pdfValue {
	switch a := v.(type) {
	case pdfArray:
		return append(a, item)
	case pdfRef:
		if obj, ok := d.objects[a.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				obj.Value = append(arr, item)
				return a
			}
		}
	}
	return pdfArray{item}
}

// removeRefFromArrayValue removes every reference to object num from an array
// value (inline or indirect), returning the value to store back.
func removeRefFromArrayValue(d *Document, v pdfValue, num int) pdfValue {
	filter := func(arr pdfArray) pdfArray {
		out := arr[:0:0]
		for _, e := range arr {
			if ref, ok := e.(pdfRef); ok && ref.Num == num {
				continue
			}
			out = append(out, e)
		}
		return out
	}
	switch a := v.(type) {
	case pdfArray:
		return filter(a)
	case pdfRef:
		if obj, ok := d.objects[a.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				obj.Value = filter(arr)
				return a
			}
		}
	}
	return v
}
