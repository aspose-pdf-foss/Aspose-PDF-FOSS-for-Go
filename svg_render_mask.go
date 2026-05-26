// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// buildMaskFormXObject renders the mask's children into a PDF Form XObject
// with /Type /XObject /Subtype /Form /Group <</Type /Group /S /Transparency /CS /DeviceGray>>.
// Returns a pdfRef to the indirect Form XObject.
//
// bbox is the bounding box for the mask in user-space units (typically the bbox of the
// shape being masked, OR the page MediaBox for userSpaceOnUse masks).
//
// The mask's color space is DeviceGray (luminance-based per SVG spec).
// White (1.0) = fully visible, black (0.0) = fully hidden.
func buildMaskFormXObject(p *Page, svg *SVG, mask *svgMask, bbox Rectangle) (pdfRef, error) {
	if mask == nil || len(mask.children) == 0 {
		return pdfRef{}, nil // caller treats Num==0 as "no mask"
	}

	// Render mask children into a content stream buffer.
	var buf bytes.Buffer
	renderSVGNodes(&buf, p, svg, mask.children, defaultSVGStyle())

	// Build the /Resources entry: share page resources so the mask content can
	// reference fonts, patterns, and XObjects already registered on the page.
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
	}

	// Build Form XObject dict per ISO 32000-1 §8.10.2 and §11.6.5.2.
	dict := pdfDict{
		"/Type":     pdfName("/XObject"),
		"/Subtype":  pdfName("/Form"),
		"/FormType": 1,
		"/BBox":     pdfArray{bbox.LLX, bbox.LLY, bbox.URX, bbox.URY},
		// Transparency group: /CS /DeviceGray encodes the mask as luminance.
		// Per SVG spec §15.6: mask luminance = 0.2125·R + 0.7154·G + 0.0721·B,
		// but since mask content already targets gray-channel output, DeviceGray
		// is the correct PDF color space here.
		"/Group": pdfDict{
			"/Type": pdfName("/Group"),
			"/S":    pdfName("/Transparency"),
			"/CS":   pdfName("/DeviceGray"),
		},
		"/Resources": resources,
	}

	// Register the Form XObject as an indirect object.
	streamObj := &pdfStream{
		Dict:    dict,
		Data:    buf.Bytes(),
		Decoded: true,
	}
	objID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[objID] = &pdfObject{Num: objID, Value: streamObj}

	return pdfRef{Num: objID}, nil
}

// applyMask, when style.mask is set, builds a Form XObject of the mask's children,
// registers an ExtGState with /SMask referencing the Form XObject, and emits
// /GS<n> gs into buf. The mask remains active until the caller emits Q.
//
// Best-effort: missing or invalid mask ref → render unmasked.
func applyMask(buf *bytes.Buffer, p *Page, svg *SVG, style svgStyle, shape svgNode) {
	if style.mask == "" || svg == nil {
		return
	}
	mask, ok := svg.defs[style.mask].(*svgMask)
	if !ok || mask == nil {
		return
	}

	// Compute bbox for the mask (use shape's bounding box if available).
	x0, y0, x1, y1 := svgShapeBBox(shape)
	if x1-x0 <= 0 || y1-y0 <= 0 {
		// Fallback: use page MediaBox-ish default
		x0, y0, x1, y1 = 0, 0, 1000, 1000
	}
	bbox := Rectangle{LLX: x0, LLY: y0, URX: x1, URY: y1}

	formRef, err := buildMaskFormXObject(p, svg, mask, bbox)
	if err != nil || formRef.Num == 0 {
		return
	}

	// Create the SMask dict and an ExtGState wrapping it.
	smaskDict := pdfDict{
		"/Type": pdfName("/Mask"),
		"/S":    pdfName("/Luminosity"),
		"/G":    formRef,
	}
	gsDict := pdfDict{
		"/Type":  pdfName("/ExtGState"),
		"/SMask": smaskDict,
	}
	gsObj := &pdfObject{Num: p.doc.nextID, Value: gsDict}
	p.doc.nextID++
	p.doc.objects[gsObj.Num] = gsObj

	// Register in page's /Resources/ExtGState.
	name := registerSMaskExtGState(p, gsObj.Num)
	fmt.Fprintf(buf, "%s gs\n", name)
}

// registerSMaskExtGState adds a reference to an indirect ExtGState object in the
// page's /Resources/ExtGState dict and returns the resource name (e.g. "/GS3").
//
// Reuses the same naming scheme as ensureExtGState but doesn't try to dedupe —
// each mask gets its own entry (the dict is different from the alpha-only dicts).
func registerSMaskExtGState(p *Page, gsObjNum int) string {
	resources := p.pageResources()
	if resources == nil {
		return "/GS0" // shouldn't happen
	}
	gsVal := resolveRef(p.doc.objects, resources["/ExtGState"])
	gsDict, _ := gsVal.(pdfDict)
	if gsDict == nil {
		gsDict = pdfDict{}
		resources["/ExtGState"] = gsDict
	}
	name := nextGSName(gsDict)
	gsDict[name] = pdfRef{Num: gsObjNum}
	return name
}
