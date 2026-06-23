// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
)

// ConvertToGrayscale converts the document to grayscale in place: every
// device colour set in a content stream (text, vector graphics), every raster
// image, shading and annotation colour is mapped to its luminance grey. The
// geometry, text and layout are untouched. Mirrors the intent of Aspose.PDF for
// .NET's grayscale conversion.
//
// Coverage: DeviceRGB/DeviceGray/DeviceCMYK and ICCBased colours in content
// streams and via cs/sc/scn; RGB/CMYK/Indexed/ICCBased raster images (re-encoded
// as DeviceGray); axial/radial (type 2/3) shadings; and annotation /C, /IC and
// appearance streams — recursing through Form XObjects. Best-effort / left as-is
// (documented): Separation/DeviceN tints, Pattern fills, and type 0/4 shading
// functions, whose colours pass through unconverted.
func (d *Document) ConvertToGrayscale() error {
	cv := &grayConverter{doc: d, visited: map[int]bool{}, streams: map[*pdfStream]bool{}}
	for i := range d.pages {
		page, err := d.Page(i + 1)
		if err != nil {
			return err
		}
		if err := cv.page(page); err != nil {
			return err
		}
	}
	return nil
}

type grayConverter struct {
	doc     *Document
	visited map[int]bool        // colorspace/shading/pattern objects already done
	streams map[*pdfStream]bool // content/image streams already done
}

func rgbToGray(r, g, b float64) float64 { return 0.3*r + 0.59*g + 0.11*b }

func cmykToGray(c, m, y, k float64) float64 {
	return rgbToGray((1-c)*(1-k), (1-m)*(1-k), (1-y)*(1-k))
}

func opFloat(v pdfValue) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// page converts a page's content, resources and annotations.
func (cv *grayConverter) page(p *Page) error {
	pageDict := p.pageDict()
	if pageDict == nil {
		return nil
	}
	res := p.pageResources()
	if err := cv.rewritePageContent(p, res); err != nil {
		return err
	}
	cv.images(p)
	cv.resources(res)
	cv.annotations(p, pageDict)
	return nil
}

// rewritePageContent rewrites the page's /Contents colour operators and replaces
// it with a single grey content stream.
func (cv *grayConverter) rewritePageContent(p *Page, res pdfDict) error {
	data, err := p.contentStreams()
	if err != nil || data == nil {
		return nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil // leave unparseable content untouched
	}
	newData := serializeContentOps(cv.grayOps(ops, res))
	id := cv.doc.addObject(&pdfStream{Dict: pdfDict{}, Data: newData, Decoded: true})
	p.pageDict()["/Contents"] = pdfRef{Num: id}
	return nil
}

// grayOps rewrites the colour-setting operators in a content-stream op list.
func (cv *grayConverter) grayOps(ops []contentOp, res pdfDict) []contentOp {
	out := make([]contentOp, 0, len(ops))
	var fillCS, strokeCS csKind
	for _, op := range ops {
		switch op.Operator {
		case "rg":
			op = grayFill(rgbToGray(opFloat(at(op, 0)), opFloat(at(op, 1)), opFloat(at(op, 2))), false)
		case "RG":
			op = grayFill(rgbToGray(opFloat(at(op, 0)), opFloat(at(op, 1)), opFloat(at(op, 2))), true)
		case "k":
			op = grayFill(cmykToGray(opFloat(at(op, 0)), opFloat(at(op, 1)), opFloat(at(op, 2)), opFloat(at(op, 3))), false)
		case "K":
			op = grayFill(cmykToGray(opFloat(at(op, 0)), opFloat(at(op, 1)), opFloat(at(op, 2)), opFloat(at(op, 3))), true)
		case "cs":
			fillCS = cv.classifyCS(op, res)
		case "CS":
			strokeCS = cv.classifyCS(op, res)
		case "sc", "scn":
			op = grayscaleSCN(op, fillCS)
		case "SC", "SCN":
			op = grayscaleSCN(op, strokeCS)
		}
		out = append(out, op)
	}
	return out
}

func at(op contentOp, i int) pdfValue {
	if i < len(op.Operands) {
		return op.Operands[i]
	}
	return 0
}

func grayFill(g float64, stroke bool) contentOp {
	o := "g"
	if stroke {
		o = "G"
	}
	return contentOp{Operator: o, Operands: []pdfValue{clamp01(g)}}
}

// csKind classifies the active colour space for sc/scn conversion.
type csKind int

const (
	csOther csKind = iota // Separation/DeviceN/Indexed/Pattern — leave as-is
	csGray
	csRGB
	csCMYK
)

// classifyCS resolves the colour-space operand of a cs/CS operator.
func (cv *grayConverter) classifyCS(op contentOp, res pdfDict) csKind {
	name, ok := at(op, 0).(pdfName)
	if !ok {
		return csOther
	}
	switch string(name) {
	case "/DeviceGray", "/CalGray", "/G":
		return csGray
	case "/DeviceRGB", "/CalRGB", "/Lab", "/RGB":
		return csRGB
	case "/DeviceCMYK", "/CMYK":
		return csCMYK
	}
	// Named colour space resolved from /Resources/ColorSpace.
	csDict, _ := resolveRefToDict(cv.doc.objects, res["/ColorSpace"])
	if csDict == nil {
		return csOther
	}
	return cv.classifyCSValue(csDict[string(name)])
}

func (cv *grayConverter) classifyCSValue(v pdfValue) csKind {
	switch t := resolveRef(cv.doc.objects, v).(type) {
	case pdfName:
		switch string(t) {
		case "/DeviceGray", "/CalGray":
			return csGray
		case "/DeviceRGB", "/CalRGB", "/Lab":
			return csRGB
		case "/DeviceCMYK":
			return csCMYK
		}
	case pdfArray:
		if len(t) == 0 {
			return csOther
		}
		head, _ := t[0].(pdfName)
		switch string(head) {
		case "/ICCBased":
			if s, ok := resolveRef(cv.doc.objects, t[1]).(*pdfStream); ok {
				switch toInt(s.Dict["/N"]) {
				case 1:
					return csGray
				case 3:
					return csRGB
				case 4:
					return csCMYK
				}
			}
		case "/CalGray":
			return csGray
		case "/CalRGB", "/Lab":
			return csRGB
		}
	}
	return csOther
}

// grayscaleSCN converts an sc/scn operand list when the active space is device
// RGB/Gray/CMYK; a trailing pattern name or a non-device space is left as-is.
func grayscaleSCN(op contentOp, kind csKind) contentOp {
	n := len(op.Operands)
	if n == 0 {
		return op
	}
	if _, isPattern := op.Operands[n-1].(pdfName); isPattern {
		return op // pattern colour — converted via the pattern object
	}
	switch kind {
	case csGray:
		return op
	case csRGB:
		if n >= 3 {
			g := clamp01(rgbToGray(opFloat(op.Operands[0]), opFloat(op.Operands[1]), opFloat(op.Operands[2])))
			op.Operands = []pdfValue{g, g, g}
		}
	case csCMYK:
		if n >= 4 {
			g := cmykToGray(opFloat(op.Operands[0]), opFloat(op.Operands[1]), opFloat(op.Operands[2]), opFloat(op.Operands[3]))
			op.Operands = []pdfValue{0.0, 0.0, 0.0, clamp01(1 - g)}
		}
	}
	return op
}

// images converts every raster image on the page (including those inside Form
// XObjects, which ImageInfos already recurses into) to DeviceGray.
func (cv *grayConverter) images(p *Page) {
	infos, err := p.ImageInfos()
	if err != nil {
		return
	}
	for i := range infos {
		info := infos[i]
		if info.Inline || info.stream == nil || cv.streams[info.stream] {
			continue
		}
		cv.streams[info.stream] = true
		if isTrue(info.stream.Dict["/ImageMask"]) || isTrue(info.stream.Dict["/IM"]) {
			continue // stencil mask painted with the (already-greyed) fill colour
		}
		if dictGetName(info.stream.Dict, "/ColorSpace") == "/DeviceGray" {
			continue
		}
		cv.convertImage(&info)
	}
}

func (cv *grayConverter) convertImage(info *ImageInfo) {
	img, err := info.Extract()
	if err != nil {
		return
	}
	decoded, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return
	}
	b := decoded.Bounds()
	w, h := b.Dx(), b.Dy()
	samples := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := decoded.At(b.Min.X+x, b.Min.Y+y).RGBA()
			samples[y*w+x] = byte(clamp01(rgbToGray(float64(r)/65535, float64(g)/65535, float64(bl)/65535))*255 + 0.5)
		}
	}
	nd := pdfDict{
		"/Type":             pdfName("/XObject"),
		"/Subtype":          pdfName("/Image"),
		"/Width":            w,
		"/Height":           h,
		"/ColorSpace":       pdfName("/DeviceGray"),
		"/BitsPerComponent": 8,
	}
	if sm, ok := info.stream.Dict["/SMask"]; ok {
		nd["/SMask"] = sm // soft mask (alpha) is unchanged by greying
	}
	info.stream.Dict = nd
	info.stream.Data = samples
	info.stream.Decoded = true
}

// resources walks a /Resources dict: Form XObjects (recurse content), shadings
// and shading/tiling patterns.
func (cv *grayConverter) resources(res pdfDict) {
	if res == nil {
		return
	}
	if xobjs, ok := resolveRefToDict(cv.doc.objects, res["/XObject"]); ok {
		for _, v := range xobjs {
			if s, ok := resolveRef(cv.doc.objects, v).(*pdfStream); ok {
				cv.formXObject(s)
			}
		}
	}
	if shadings, ok := resolveRefToDict(cv.doc.objects, res["/Shading"]); ok {
		for _, v := range shadings {
			cv.shading(v)
		}
	}
	if patterns, ok := resolveRefToDict(cv.doc.objects, res["/Pattern"]); ok {
		for _, v := range patterns {
			cv.pattern(v)
		}
	}
}

// formXObject rewrites a Form XObject's own content and recurses its resources.
func (cv *grayConverter) formXObject(s *pdfStream) {
	if s == nil || cv.streams[s] || dictGetName(s.Dict, "/Subtype") != "/Form" {
		return
	}
	cv.streams[s] = true
	res, _ := resolveRefToDict(cv.doc.objects, s.Dict["/Resources"])
	ops, err := parseContentStream(decodedStreamData(s))
	if err == nil {
		s.Data = serializeContentOps(cv.grayOps(ops, res))
		s.Decoded = true
		delete(s.Dict, "/Filter")
		delete(s.Dict, "/DecodeParms")
	}
	cv.resources(res)
}

func (cv *grayConverter) pattern(v pdfValue) {
	s, isStream := resolveRef(cv.doc.objects, v).(*pdfStream)
	if isStream {
		// Tiling pattern: a content stream with its own resources.
		if cv.streams[s] {
			return
		}
		cv.streams[s] = true
		res, _ := resolveRefToDict(cv.doc.objects, s.Dict["/Resources"])
		if ops, err := parseContentStream(decodedStreamData(s)); err == nil {
			s.Data = serializeContentOps(cv.grayOps(ops, res))
			s.Decoded = true
			delete(s.Dict, "/Filter")
			delete(s.Dict, "/DecodeParms")
		}
		cv.resources(res)
		return
	}
	if pd, ok := resolveRefToDict(cv.doc.objects, v); ok {
		// Shading pattern (PatternType 2).
		cv.shading(pd["/Shading"])
	}
}

// annotations converts annotation colours and appearance streams.
func (cv *grayConverter) annotations(p *Page, pageDict pdfDict) {
	annots, ok := resolveRefToArray(cv.doc.objects, pageDict["/Annots"])
	if !ok {
		return
	}
	for _, a := range annots {
		ad, ok := resolveRefToDict(cv.doc.objects, a)
		if !ok {
			continue
		}
		grayColorArray(ad, "/C")
		grayColorArray(ad, "/IC")
		if mk, ok := resolveRefToDict(cv.doc.objects, ad["/MK"]); ok {
			grayColorArray(mk, "/BC")
			grayColorArray(mk, "/BG")
		}
		cv.appearanceStreams(ad["/AP"])
	}
}

func (cv *grayConverter) appearanceStreams(ap pdfValue) {
	apDict, ok := resolveRefToDict(cv.doc.objects, ap)
	if !ok {
		return
	}
	var walk func(v pdfValue)
	walk = func(v pdfValue) {
		switch t := resolveRef(cv.doc.objects, v).(type) {
		case *pdfStream:
			cv.formXObject(t)
		case pdfDict:
			for _, sub := range t {
				walk(sub)
			}
		}
	}
	for _, v := range apDict {
		walk(v)
	}
}

// grayColorArray converts a 1/3/4-component colour array in place.
func grayColorArray(dict pdfDict, key string) {
	arr, ok := dict[key].(pdfArray)
	if !ok {
		return
	}
	switch len(arr) {
	case 3:
		g := clamp01(rgbToGray(opFloat(arr[0]), opFloat(arr[1]), opFloat(arr[2])))
		dict[key] = pdfArray{g}
	case 4:
		g := clamp01(cmykToGray(opFloat(arr[0]), opFloat(arr[1]), opFloat(arr[2]), opFloat(arr[3])))
		dict[key] = pdfArray{g}
	}
}

func isTrue(v pdfValue) bool { b, ok := v.(bool); return ok && b }

// componentsToGray reduces an n-component colour array to a 1-component grey
// array, interpreting it per the source colour-space kind.
func componentsToGray(arr pdfArray, kind csKind) pdfArray {
	switch kind {
	case csRGB:
		if len(arr) >= 3 {
			return pdfArray{clamp01(rgbToGray(opFloat(arr[0]), opFloat(arr[1]), opFloat(arr[2])))}
		}
	case csCMYK:
		if len(arr) >= 4 {
			return pdfArray{clamp01(cmykToGray(opFloat(arr[0]), opFloat(arr[1]), opFloat(arr[2]), opFloat(arr[3])))}
		}
	case csGray:
		if len(arr) >= 1 {
			return pdfArray{clamp01(opFloat(arr[0]))}
		}
	}
	return arr
}

// shading converts an axial/radial (type 2/3) shading to DeviceGray by greying
// its colour functions and /Background; shadings whose colour space cannot be
// interpreted or whose functions are not type 2/3 (e.g. sampled/PostScript) are
// left unchanged.
func (cv *grayConverter) shading(v pdfValue) {
	if num, ok := v.(pdfRef); ok {
		if cv.visited[num.Num] {
			return
		}
		cv.visited[num.Num] = true
	}
	resolved := resolveRef(cv.doc.objects, v)
	var sh pdfDict
	switch t := resolved.(type) {
	case pdfDict:
		sh = t
	case *pdfStream:
		sh = t.Dict
	default:
		return
	}
	kind := cv.classifyCSValue(sh["/ColorSpace"])
	if kind == csOther {
		return
	}
	fns := cv.collectFunctions(sh["/Function"])
	for _, fn := range fns {
		if !cv.functionConvertible(fn) {
			return // a non-type-2/3 function: leave the whole shading
		}
	}
	for _, fn := range fns {
		cv.convertFunction(fn, kind)
	}
	sh["/ColorSpace"] = pdfName("/DeviceGray")
	if bg, ok := sh["/Background"].(pdfArray); ok {
		sh["/Background"] = componentsToGray(bg, kind)
	}
}

// collectFunctions resolves /Function (a single function or an array) into a
// flat list of resolved function dicts.
func (cv *grayConverter) collectFunctions(v pdfValue) []pdfValue {
	if v == nil {
		return nil
	}
	if arr, ok := resolveRef(cv.doc.objects, v).(pdfArray); ok {
		out := make([]pdfValue, 0, len(arr))
		for _, e := range arr {
			out = append(out, e)
		}
		return out
	}
	return []pdfValue{v}
}

func (cv *grayConverter) functionDict(v pdfValue) pdfDict {
	switch t := resolveRef(cv.doc.objects, v).(type) {
	case pdfDict:
		return t
	case *pdfStream:
		return t.Dict
	}
	return nil
}

// functionConvertible reports whether a function is a type 2/3 (and, for 3, all
// of its sub-functions) so its output can be greyed.
func (cv *grayConverter) functionConvertible(v pdfValue) bool {
	d := cv.functionDict(v)
	if d == nil {
		return false
	}
	switch toInt(d["/FunctionType"]) {
	case 2:
		return true
	case 3:
		for _, sub := range cv.collectFunctions(d["/Functions"]) {
			if !cv.functionConvertible(sub) {
				return false
			}
		}
		return true
	}
	return false
}

func (cv *grayConverter) convertFunction(v pdfValue, kind csKind) {
	d := cv.functionDict(v)
	if d == nil {
		return
	}
	switch toInt(d["/FunctionType"]) {
	case 2:
		if c0, ok := d["/C0"].(pdfArray); ok {
			d["/C0"] = componentsToGray(c0, kind)
		} else {
			d["/C0"] = pdfArray{0.0}
		}
		if c1, ok := d["/C1"].(pdfArray); ok {
			d["/C1"] = componentsToGray(c1, kind)
		} else {
			d["/C1"] = pdfArray{1.0}
		}
		d["/Range"] = pdfArray{0.0, 1.0}
	case 3:
		for _, sub := range cv.collectFunctions(d["/Functions"]) {
			cv.convertFunction(sub, kind)
		}
		d["/Range"] = pdfArray{0.0, 1.0}
	}
}
