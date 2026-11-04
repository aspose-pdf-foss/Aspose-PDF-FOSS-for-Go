// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"maps"
	"math"
	"slices"
)

// ConvertDeviceCMYKToRGB recolours everything in DeviceCMYK to DeviceRGB through
// the same conversion the renderer uses, so a page looks the same on screen. Call
// it before ConvertToPDFA when a document must conform but has CMYK colours: the
// output intent ConvertToPDFA adds is sRGB, which does not cover DeviceCMYK, and
// ValidatePDFA reports it as COLOR_INTENT_MISMATCH.
//
// It is a separate step because it cannot be undone: the CMYK values a print
// workflow relies on, such as a pure black text ink, are gone. ICCBased CMYK has a
// profile of its own, conforms as it is and stays. A CMYK JPEG or JPEG 2000 image
// is stored losslessly afterwards, so the file can grow. A signature in the
// document no longer verifies.
//
// Covered: content operators, inline images and images, shadings with a function,
// Indexed, Separation and DeviceN spaces resting on DeviceCMYK, Pattern underlying
// spaces, transparency groups, and the colours of annotations and form fields, in
// pages, forms, patterns, Type3 glyphs and annotation appearances. Left as they
// were, and still reported by ValidatePDFA: mesh shadings carrying colours in their
// data, a DeviceN space of more than 8 colorants, a JPEG with a colour-key mask,
// and any image or function that cannot be decoded.
//
// A document whose output intent is CMYK needs none of this and would stop
// conforming, so it is refused with an error.
func (d *Document) ConvertDeviceCMYKToRGB() error {
	if d.outputIntentComponents() == 4 {
		return errors.New("ConvertDeviceCMYKToRGB: the output intent is CMYK, which covers DeviceCMYK and not DeviceRGB")
	}
	cv := &cmykConverter{d: d, done: map[*pdfStream]bool{}, sampled: map[[2]int]pdfRef{}}
	// Content first: reading it takes the colour spaces as they were.
	var replaced []int
	for i := range d.pages {
		page, err := d.Page(i + 1)
		if err != nil {
			return err
		}
		replaced = append(replaced, cv.page(page)...)
	}
	cv.remainingContent()
	cv.colorObjects()
	d.dropUnreachable(replaced)
	return nil
}

type cmykConverter struct {
	d       *Document
	done    map[*pdfStream]bool
	sampled map[[2]int]pdfRef // function object and inputs -> its RGB form
}

// cmykKind says how sc/scn operands relate to the current colour space.
type cmykKind int

const (
	cmykNone    cmykKind = iota
	cmykDevice           // four components to convert
	cmykPattern          // four components and a pattern name
)

type cmykColorState struct{ fill, stroke cmykKind }

// maxColorSpaceDepth bounds following a colour space through its base spaces: the
// format nests two levels, and a damaged file can refer to itself.
const maxColorSpaceDepth = 8

func isDeviceCMYK(objects map[int]*pdfObject, v pdfValue) bool {
	n, ok := resolveRef(objects, v).(pdfName)
	return ok && (n == "/DeviceCMYK" || n == "/CMYK")
}

// rgbOperands converts four numeric operands to three, with enough digits to land
// on the same 8-bit value.
func rgbOperands(cmyk []pdfValue) []pdfValue {
	r, g, b := adobeCMYKToRGB(clamp01(opFloat(cmyk[0])), clamp01(opFloat(cmyk[1])), clamp01(opFloat(cmyk[2])), clamp01(opFloat(cmyk[3])))
	unit := func(v uint8) pdfValue { return math.Round(float64(v)/255*10000) / 10000 }
	return []pdfValue{unit(r), unit(g), unit(b)}
}

// classify reads the operand of cs/CS against the colour spaces as they were.
func (cv *cmykConverter) classify(v pdfValue, res pdfDict) cmykKind {
	objects := cv.d.objects
	if isDeviceCMYK(objects, v) {
		return cmykDevice
	}
	name, ok := v.(pdfName)
	if !ok {
		return cmykNone
	}
	csd, _ := resolveRefToDict(objects, res["/ColorSpace"])
	if isDeviceCMYK(objects, csd[string(name)]) {
		return cmykDevice
	}
	if arr, ok := resolveRefToArray(objects, csd[string(name)]); ok && len(arr) == 2 && operandName(arr[0]) == "/Pattern" && isDeviceCMYK(objects, arr[1]) {
		return cmykPattern
	}
	return cmykNone
}

// rewriteOps converts a content stream's colours, tracking the current colour
// space through q/Q as it is part of the graphics state. It reports whether
// anything changed, so a stream with no CMYK is left byte for byte.
func (cv *cmykConverter) rewriteOps(ops []contentOp, res pdfDict) bool {
	var state cmykColorState
	var stack []cmykColorState
	changed := false
	setColor := func(op *contentOp, kind cmykKind) {
		n := len(op.Operands)
		switch {
		case kind == cmykDevice && n == 4:
			op.Operands = rgbOperands(op.Operands)
			changed = true
		case kind == cmykPattern && n == 5:
			op.Operands = append(rgbOperands(op.Operands), op.Operands[4])
			changed = true
		}
	}
	for i := range ops {
		op := &ops[i]
		switch op.Operator {
		case "q":
			stack = append(stack, state)
		case "Q":
			if len(stack) > 0 {
				state = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "k":
			state.fill = cmykNone
			if len(op.Operands) == 4 {
				op.Operator, op.Operands = "rg", rgbOperands(op.Operands)
				changed = true
			}
		case "K":
			state.stroke = cmykNone
			if len(op.Operands) == 4 {
				op.Operator, op.Operands = "RG", rgbOperands(op.Operands)
				changed = true
			}
		case "g", "rg":
			state.fill = cmykNone
		case "G", "RG":
			state.stroke = cmykNone
		case "cs", "CS":
			kind := cv.classify(at(*op, 0), res)
			if kind == cmykDevice {
				op.Operands = []pdfValue{pdfName("/DeviceRGB")}
				changed = true
			}
			if op.Operator == "cs" {
				state.fill = kind
			} else {
				state.stroke = kind
			}
		case "sc", "scn":
			setColor(op, state.fill)
		case "SC", "SCN":
			setColor(op, state.stroke)
		case "BI":
			if cv.inlineImage(op, res) {
				changed = true
			}
		}
	}
	return changed
}

// page rewrites a page's content, returning the streams it replaced.
func (cv *cmykConverter) page(p *Page) []int {
	dict := p.pageDict()
	if dict == nil {
		return nil
	}
	res := p.pageResources()
	var replaced []int
	if data, err := p.contentStreams(); err == nil && data != nil {
		if ops, err := parseContentStream(data); err == nil && cv.rewriteOps(ops, res) {
			replaced = cv.contentIDs(dict["/Contents"])
			dict["/Contents"] = pdfRef{Num: cv.d.addObject(&pdfStream{Dict: pdfDict{}, Data: serializeContentOps(ops), Decoded: true})}
		}
	}
	cv.resources(res)
	annots, _ := resolveRefToArray(cv.d.objects, dict["/Annots"])
	for _, v := range annots {
		if ad, ok := resolveRefToDict(cv.d.objects, v); ok {
			cv.appearance(ad["/AP"], 0)
		}
	}
	return replaced
}

func (cv *cmykConverter) contentIDs(v pdfValue) []int {
	var ids []int
	if ref, ok := v.(pdfRef); ok {
		ids = append(ids, ref.Num)
	}
	if arr, ok := resolveRef(cv.d.objects, v).(pdfArray); ok {
		for _, e := range arr {
			if ref, ok := e.(pdfRef); ok {
				ids = append(ids, ref.Num)
			}
		}
	}
	return ids
}

// appearance follows /AP through its appearance and state dictionaries, which nest
// two levels; a damaged file can refer to itself.
func (cv *cmykConverter) appearance(v pdfValue, depth int) {
	if depth > 2 {
		return
	}
	switch t := resolveRef(cv.d.objects, v).(type) {
	case *pdfStream:
		cv.contentStream(t, nil)
	case pdfDict:
		for _, sub := range t {
			cv.appearance(sub, depth+1)
		}
	}
}

// resources follows what a resource dictionary holds to the content streams in it.
func (cv *cmykConverter) resources(res pdfDict) {
	objects := cv.d.objects
	if res == nil {
		return
	}
	each := func(key string, fn func(v pdfValue)) {
		if dict, ok := resolveRefToDict(objects, res[key]); ok {
			for _, v := range dict {
				fn(v)
			}
		}
	}
	each("/XObject", func(v pdfValue) {
		if s, ok := resolveRef(objects, v).(*pdfStream); ok && dictGetName(s.Dict, "/Subtype") == "/Form" {
			cv.contentStream(s, res)
		}
	})
	each("/Pattern", func(v pdfValue) {
		if s, ok := resolveRef(objects, v).(*pdfStream); ok {
			cv.contentStream(s, res)
		}
	})
	each("/ExtGState", func(v pdfValue) {
		gs, _ := resolveRefToDict(objects, v)
		sm, _ := resolveRefToDict(objects, gs["/SMask"])
		if s, ok := resolveRef(objects, sm["/G"]).(*pdfStream); ok {
			cv.contentStream(s, res)
		}
	})
	each("/Font", func(v pdfValue) {
		font, _ := resolveRefToDict(objects, v)
		if dictGetName(font, "/Subtype") != "/Type3" {
			return
		}
		fontRes := res
		if own, ok := resolveRefToDict(objects, font["/Resources"]); ok {
			fontRes = own
		}
		procs, _ := resolveRefToDict(objects, font["/CharProcs"])
		for _, p := range procs {
			if s, ok := resolveRef(objects, p).(*pdfStream); ok {
				cv.contentStream(s, fontRes)
			}
		}
	})
}

// contentStream rewrites a form, a tiling cell or a glyph procedure once. One with
// no /Resources of its own uses its caller's.
func (cv *cmykConverter) contentStream(s *pdfStream, parent pdfDict) {
	if cv.done[s] {
		return
	}
	cv.done[s] = true
	res := parent
	if own, ok := resolveRefToDict(cv.d.objects, s.Dict["/Resources"]); ok {
		res = own
	}
	if ops, err := parseContentStream(decodedStreamData(s)); err == nil && cv.rewriteOps(ops, res) {
		s.Data = serializeContentOps(ops)
		s.Decoded = true
		delete(s.Dict, "/Filter")
		delete(s.Dict, "/DecodeParms")
	}
	cv.resources(res)
}

// remainingContent catches forms and tiling cells no page's resources led to.
func (cv *cmykConverter) remainingContent() {
	for _, id := range slices.Sorted(maps.Keys(cv.d.objects)) {
		s, ok := cv.d.objects[id].Value.(*pdfStream)
		if !ok {
			continue
		}
		if dictGetName(s.Dict, "/Subtype") == "/Form" || toInt(s.Dict["/PatternType"]) == 1 {
			cv.contentStream(s, nil)
		}
	}
}

// colorObjects converts every colour held outside content streams. It runs after
// the content, which needs to read the spaces as they were, and takes images
// first, as their samples need the same.
func (cv *cmykConverter) colorObjects() {
	ids := slices.Sorted(maps.Keys(cv.d.objects))
	for _, id := range ids {
		if s, ok := cv.d.objects[id].Value.(*pdfStream); ok {
			cv.image(s)
		}
	}
	for _, id := range ids {
		switch t := cv.d.objects[id].Value.(type) {
		case *pdfStream:
			cv.colorDict(t.Dict)
		case pdfDict:
			cv.colorDict(t)
		case pdfArray:
			cv.colorArray(t)
		}
	}
	for _, page := range cv.d.pages {
		if dict, ok := page.Value.(pdfDict); ok {
			cv.colorDict(dict)
		}
	}
	cv.colorDict(cv.d.catalog)
}

func (cv *cmykConverter) colorArray(arr pdfArray) {
	for _, e := range arr {
		switch t := e.(type) {
		case pdfDict:
			cv.colorDict(t)
		case pdfArray:
			cv.colorArray(t)
		}
	}
}

// colorDict converts the colours in a dictionary and in those nested directly in it.
func (cv *cmykConverter) colorDict(dict pdfDict) {
	objects := cv.d.objects
	_, isShading := dict["/ShadingType"]
	switch {
	case isShading:
		cv.shading(dict)
	case dictGetName(dict, "/Subtype") == "/Image":
		// Done by image; one it could not convert stays as it was.
	default:
		if named, ok := resolveRefToDict(objects, dict["/ColorSpace"]); ok {
			for key, v := range named {
				if nv, changed := cv.colorSpace(v, 0); changed {
					named[key] = nv
				}
			}
		} else if nv, changed := cv.colorSpace(dict["/ColorSpace"], 0); changed {
			dict["/ColorSpace"] = nv
		}
	}
	// A soft mask's backdrop is in its group's colour space, which stays when it has
	// a profile of its own.
	softMaskICC := false
	if g, ok := resolveRef(objects, dict["/G"]).(*pdfStream); ok {
		group, _ := resolveRefToDict(objects, g.Dict["/Group"])
		_, softMaskICC = resolveRefToArray(objects, group["/CS"])
	}
	if dictGetName(dict, "/S") == "/Transparency" {
		if nv, changed := cv.colorSpace(dict["/CS"], 0); changed {
			dict["/CS"] = nv
		}
	}
	for _, key := range []string{"/C", "/IC", "/BC", "/BG"} {
		if key == "/BC" && softMaskICC {
			continue
		}
		if arr, ok := dict[key].(pdfArray); ok && len(arr) == 4 && allNumbers(arr) {
			dict[key] = pdfArray(rgbOperands(arr))
		}
	}
	if da, ok := dict["/DA"].(string); ok {
		if ops, err := parseContentStream([]byte(da)); err == nil && cv.rewriteOps(ops, nil) {
			dict["/DA"] = string(bytes.TrimSpace(serializeContentOps(ops)))
		}
	}
	for _, v := range dict {
		switch t := v.(type) {
		case pdfDict:
			cv.colorDict(t)
		case pdfArray:
			cv.colorArray(t)
		}
	}
}

func allNumbers(arr pdfArray) bool {
	for _, v := range arr {
		switch v.(type) {
		case int, float64:
		default:
			return false
		}
	}
	return true
}

// colorSpace returns the RGB form of a colour-space value and whether it differs.
// An array behind a reference is replaced where it is stored, for everything
// sharing it; a name is not, as an image that could not be converted may share it.
func (cv *cmykConverter) colorSpace(v pdfValue, depth int) (pdfValue, bool) {
	objects := cv.d.objects
	if depth > maxColorSpaceDepth {
		return v, false
	}
	if isDeviceCMYK(objects, v) {
		return pdfName("/DeviceRGB"), true
	}
	if ref, ok := v.(pdfRef); ok {
		obj := objects[ref.Num]
		if obj == nil {
			return v, false
		}
		nv, changed := cv.colorSpace(obj.Value, depth+1)
		if changed {
			obj.Value = nv
		}
		return v, changed
	}
	arr, ok := v.(pdfArray)
	if !ok || len(arr) < 2 {
		return v, false
	}
	replace := func(i int, nv pdfValue) pdfArray {
		out := slices.Clone(arr)
		out[i] = nv
		return out
	}
	switch operandName(arr[0]) {
	case "/Pattern":
		if nv, changed := cv.colorSpace(arr[1], depth+1); changed {
			return replace(1, nv), true
		}
	case "/Indexed", "/I":
		if len(arr) < 4 {
			return v, false
		}
		if !isDeviceCMYK(objects, arr[1]) {
			if nv, changed := cv.colorSpace(arr[1], depth+1); changed {
				return replace(1, nv), true
			}
			return v, false
		}
		out := replace(1, pdfName("/DeviceRGB"))
		out[3] = cv.indexedLookup(arr[3])
		return out, true
	case "/Separation", "/DeviceN":
		if len(arr) < 4 || !isDeviceCMYK(objects, arr[2]) {
			return v, false
		}
		inputs := 1
		if names, ok := resolveRefToArray(objects, arr[1]); ok {
			inputs = len(names)
		}
		fn, ok := cv.sampledRGB(arr[3], inputs)
		if !ok {
			return v, false
		}
		out := replace(2, pdfName("/DeviceRGB"))
		out[3] = fn
		return out, true
	}
	return v, false
}

// indexedLookup converts an Indexed table of CMYK entries to RGB ones.
func (cv *cmykConverter) indexedLookup(v pdfValue) string {
	var table []byte
	switch t := resolveRef(cv.d.objects, v).(type) {
	case string:
		table = []byte(t)
	case *pdfStream:
		table = decodedStreamData(t)
	}
	return string(cmykToRGB(table, len(table)/4))
}

// sampledGrid gives the samples per input of an RGB function standing in for one
// with that many inputs, or 0 when the table would be too large to write.
func sampledGrid(inputs int) int {
	switch inputs {
	case 1:
		return 256
	case 2:
		return 33
	case 3:
		return 17
	case 4:
		return 9
	case 5:
		return 5
	case 6, 7, 8:
		return 3
	}
	return 0
}

// functionKnown reports whether parseFunction evaluates this function rather than
// falling back to a constant.
func (cv *cmykConverter) functionKnown(v pdfValue, depth int) bool {
	objects := cv.d.objects
	if depth > maxColorSpaceDepth {
		return false
	}
	if arr, ok := resolveRef(objects, v).(pdfArray); ok {
		for _, e := range arr {
			if !cv.functionKnown(e, depth+1) {
				return false
			}
		}
		return len(arr) > 0
	}
	dict, stream := asDictStream(resolveRef(objects, v))
	if dict == nil {
		return false
	}
	switch toInt(resolveRef(objects, dict["/FunctionType"])) {
	case 2:
		return true
	case 3:
		sub, ok := resolveRefToArray(objects, dict["/Functions"])
		if !ok || len(sub) == 0 {
			return false
		}
		for _, e := range sub {
			if !cv.functionKnown(e, depth+1) {
				return false
			}
		}
		return true
	case 0:
		return stream != nil && len(shFloats(objects, dict["/Size"])) <= maxSampledInputs
	case 4:
		return stream != nil && parsePSProgram(decodedStreamData(stream)) != nil
	}
	return false
}

// functionDomain reads the domain of a function or of the first in an array of them.
func (cv *cmykConverter) functionDomain(v pdfValue) []float64 {
	first := resolveRef(cv.d.objects, v)
	if arr, ok := first.(pdfArray); ok && len(arr) > 0 {
		first = resolveRef(cv.d.objects, arr[0])
	}
	dict, _ := asDictStream(first)
	if dict == nil {
		return nil
	}
	return shFloats(cv.d.objects, dict["/Domain"])
}

// sampledRGB tabulates a function giving CMYK as a sampled function giving RGB
// over the same domain. A table keeps the same colours in between too, which a
// function of the same type interpolating in RGB would not.
func (cv *cmykConverter) sampledRGB(fnVal pdfValue, inputs int) (pdfRef, bool) {
	objects := cv.d.objects
	grid := sampledGrid(inputs)
	if grid == 0 || !cv.functionKnown(fnVal, 0) {
		return pdfRef{}, false
	}
	key := [2]int{-1, inputs}
	if ref, ok := fnVal.(pdfRef); ok {
		key[0] = ref.Num
		if done, ok := cv.sampled[key]; ok {
			return done, true
		}
	}
	domain := cv.functionDomain(fnVal)
	if len(domain) != 2*inputs {
		return pdfRef{}, false
	}
	fn := parseFunction(objects, fnVal)
	in := make([]float64, inputs)
	for i := range in {
		in[i] = domain[2*i]
	}
	if len(fn.eval(in)) != 4 {
		return pdfRef{}, false
	}
	total := 1
	sizes := make(pdfArray, inputs)
	domainOut := make(pdfArray, 2*inputs)
	for i := range inputs {
		total *= grid
		sizes[i] = grid
		domainOut[2*i], domainOut[2*i+1] = domain[2*i], domain[2*i+1]
	}
	data := make([]byte, 0, total*3)
	for n := range total {
		rest := n
		for i := range inputs {
			step := float64(rest%grid) / float64(grid-1)
			in[i] = domain[2*i] + step*(domain[2*i+1]-domain[2*i])
			rest /= grid
		}
		out := fn.eval(in)
		r, g, b := adobeCMYKToRGB(clamp01(out[0]), clamp01(out[1]), clamp01(out[2]), clamp01(out[3]))
		data = append(data, r, g, b)
	}
	ref := pdfRef{Num: cv.d.addObject(&pdfStream{Dict: pdfDict{
		"/FunctionType":  0,
		"/Domain":        domainOut,
		"/Range":         pdfArray{0, 1, 0, 1, 0, 1},
		"/Size":          sizes,
		"/BitsPerSample": 8,
	}, Data: data, Decoded: true})}
	if key[0] >= 0 {
		cv.sampled[key] = ref
	}
	return ref, true
}

// shading converts a shading in DeviceCMYK through its function; its /Background is
// in the same space. One with its colours in its mesh data has no function and
// stays. Other spaces need only the colour-space conversion, as their components
// keep their meaning.
func (cv *cmykConverter) shading(dict pdfDict) {
	objects := cv.d.objects
	if !isDeviceCMYK(objects, dict["/ColorSpace"]) {
		if nv, changed := cv.colorSpace(dict["/ColorSpace"], 0); changed {
			dict["/ColorSpace"] = nv
		}
		return
	}
	fnVal, has := dict["/Function"]
	if !has {
		return
	}
	ref, ok := cv.sampledRGB(fnVal, len(cv.functionDomain(fnVal))/2)
	if !ok {
		return
	}
	dict["/Function"] = ref
	dict["/ColorSpace"] = pdfName("/DeviceRGB")
	if bg, ok := resolveRefToArray(objects, dict["/Background"]); ok && len(bg) == 4 && allNumbers(bg) {
		dict["/Background"] = pdfArray(rgbOperands(bg))
	}
}

// image converts an image XObject, whose samples depend on its colour space.
func (cv *cmykConverter) image(s *pdfStream) {
	if dictGetName(s.Dict, "/Subtype") != "/Image" || isTrue(s.Dict["/ImageMask"]) {
		return
	}
	cs := s.Dict["/ColorSpace"]
	if !isDeviceCMYK(cv.d.objects, cs) {
		if nv, changed := cv.colorSpace(cs, 0); changed {
			s.Dict["/ColorSpace"] = nv
		}
		return
	}
	_, colourKey := resolveRefToArray(cv.d.objects, s.Dict["/Mask"])
	img, ok := cv.decodeImage(s.Dict, s.Data, s.Decoded, colourKey)
	if !ok {
		return
	}
	if colourKey {
		cv.colourKeyStencil(s, img)
	}
	if sm, ok := resolveRef(cv.d.objects, s.Dict["/SMask"]).(*pdfStream); ok {
		if matte, ok := resolveRefToArray(cv.d.objects, sm.Dict["/Matte"]); ok && len(matte) == 4 && allNumbers(matte) {
			sm.Dict["/Matte"] = pdfArray(rgbOperands(matte))
		}
	}
	s.Dict["/ColorSpace"] = pdfName("/DeviceRGB")
	s.Dict["/BitsPerComponent"] = 8
	s.Dict["/Width"], s.Dict["/Height"] = img.w, img.h
	delete(s.Dict, "/Decode")
	delete(s.Dict, "/Filter")
	delete(s.Dict, "/DecodeParms")
	s.Data = img.rgb
	s.Decoded = true
}

type cmykImage struct {
	rgb     []byte
	samples []byte // the CMYK samples, when they were read directly
	w, h    int
	bpc     int
}

// decodeImage reads an image's samples: data is what the stream holds, decoded or
// with every filter still on it.
func (cv *cmykConverter) decodeImage(dict pdfDict, data []byte, decoded, colourKey bool) (cmykImage, bool) {
	objects := cv.d.objects
	chain := filterChain(dict)
	last := ""
	if len(chain) > 0 {
		last = chain[len(chain)-1]
	}
	if last == "/DCTDecode" || last == "/DCT" || last == "/JPXDecode" {
		if decoded || colourKey {
			return cmykImage{}, false
		}
		encoded := data
		for _, f := range chain[:len(chain)-1] {
			out, err := applyFilter(f, encoded)
			if err != nil {
				return cmykImage{}, false
			}
			encoded = out
		}
		if last == "/JPXDecode" {
			pixels, comps, w, h, err := jpxDecode(encoded)
			if err != nil || (comps != 3 && comps != 4) {
				return cmykImage{}, false
			}
			if comps == 4 {
				pixels = cmykToRGB(pixels, w*h)
			}
			return cmykImage{rgb: pixels, w: w, h: h}, true
		}
		// The same decoding extraction and the renderer use, Adobe inversion included.
		pixels, w, h, err := decodeJPEGToPixels(encoded)
		if err != nil {
			return cmykImage{}, false
		}
		return cmykImage{rgb: pixels, w: w, h: h}, true
	}
	samples := data
	if !decoded {
		var err error
		if samples, err = decodeStream(dict, data); err != nil {
			return cmykImage{}, false
		}
	}
	w, h := toInt(resolveRef(objects, dict["/Width"])), toInt(resolveRef(objects, dict["/Height"]))
	bpc := toInt(resolveRef(objects, dict["/BitsPerComponent"]))
	if bpc == 0 {
		bpc = 8
	}
	rgb, ok := cmykSamplesToRGB(samples, w, h, bpc, shFloats(objects, dict["/Decode"]))
	if !ok {
		return cmykImage{}, false
	}
	return cmykImage{rgb: rgb, samples: samples, w: w, h: h, bpc: bpc}, true
}

// cmykSamplesToRGB converts packed samples of 1 to 16 bits per component, rows
// padded to a byte, applying /Decode. A stream too short for its dimensions is
// refused rather than guessed at.
func cmykSamplesToRGB(samples []byte, w, h, bpc int, decode []float64) ([]byte, bool) {
	switch bpc {
	case 1, 2, 4, 8, 16:
	default:
		return nil, false
	}
	if w <= 0 || h <= 0 || w > math.MaxInt32/h/3 {
		return nil, false
	}
	rowBytes := (w*4*bpc + 7) / 8
	if len(samples) < rowBytes*h {
		return nil, false
	}
	if len(decode) < 8 {
		decode = []float64{0, 1, 0, 1, 0, 1, 0, 1}
	}
	maxV := float64(uint64(1)<<uint(bpc) - 1)
	rgb := make([]byte, w*h*3)
	var last [4]uint32
	var lr, lg, lb uint8
	haveLast := false
	for y := range h {
		row := samples[y*rowBytes:]
		for x := range w {
			var cur [4]uint32
			for c := range 4 {
				bit := (x*4 + c) * bpc
				switch bpc {
				case 8:
					cur[c] = uint32(row[bit/8])
				case 16:
					cur[c] = uint32(row[bit/8])<<8 | uint32(row[bit/8+1])
				default:
					cur[c] = uint32(row[bit/8]>>uint(8-bpc-bit%8)) & (1<<uint(bpc) - 1)
				}
			}
			if !haveLast || cur != last {
				var v [4]float64
				for c := range 4 {
					v[c] = clamp01(decode[2*c] + float64(cur[c])/maxV*(decode[2*c+1]-decode[2*c]))
				}
				lr, lg, lb = adobeCMYKToRGB(v[0], v[1], v[2], v[3])
				last, haveLast = cur, true
			}
			off := (y*w + x) * 3
			rgb[off], rgb[off+1], rgb[off+2] = lr, lg, lb
		}
	}
	return rgb, true
}

// colourKeyStencil replaces a colour-key mask, whose ranges are in the old samples,
// with the stencil it amounts to (ISO 32000-1 §8.9.6.3): a 1 masks a pixel out.
func (cv *cmykConverter) colourKeyStencil(s *pdfStream, img cmykImage) {
	ranges, _ := resolveRefToArray(cv.d.objects, s.Dict["/Mask"])
	flat := make(pdfArray, len(ranges))
	for i, v := range ranges {
		flat[i] = resolveRef(cv.d.objects, v)
	}
	alpha := colourKeyAlpha(img.samples, img.w, img.h, img.bpc, 4, flat)
	if alpha == nil {
		delete(s.Dict, "/Mask")
		return
	}
	rowBytes := (img.w + 7) / 8
	bits := make([]byte, rowBytes*img.h)
	for y := range img.h {
		for x := range img.w {
			if alpha[y*img.w+x] == 0 {
				bits[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	s.Dict["/Mask"] = pdfRef{Num: cv.d.addObject(&pdfStream{Dict: pdfDict{
		"/Type":             pdfName("/XObject"),
		"/Subtype":          pdfName("/Image"),
		"/ImageMask":        true,
		"/Width":            img.w,
		"/Height":           img.h,
		"/BitsPerComponent": 1,
	}, Data: bits, Decoded: true})}
}

// inlineImage converts a BI operator in DeviceCMYK, or in a space resting on it.
// Its samples are written in hex, as binary data could hold what reads as an EI.
func (cv *cmykConverter) inlineImage(op *contentOp, res pdfDict) bool {
	dict, ok := at(*op, 0).(pdfDict)
	data, isData := at(*op, 1).(string)
	if !ok || !isData {
		return false
	}
	objects := cv.d.objects
	cs := dict["/ColorSpace"]
	if name, ok := cs.(pdfName); ok && !isDeviceCMYK(objects, name) {
		csd, _ := resolveRefToDict(objects, res["/ColorSpace"])
		if isDeviceCMYK(objects, csd[string(name)]) {
			cs = pdfName("/DeviceCMYK")
		}
	}
	nd := make(pdfDict, len(dict))
	maps.Copy(nd, dict)
	if !isDeviceCMYK(objects, cs) {
		if _, isArray := cs.(pdfArray); !isArray {
			return false // a name in the resources is converted there
		}
		nv, changed := cv.colorSpace(cs, 0)
		if !changed {
			return false
		}
		nd["/ColorSpace"] = nv
		op.Operands = []pdfValue{nd, data}
		return true
	}
	img, ok := cv.decodeImage(dict, []byte(data), false, false)
	if !ok {
		return false
	}
	nd["/ColorSpace"] = pdfName("/DeviceRGB")
	nd["/BitsPerComponent"] = 8
	nd["/Width"], nd["/Height"] = img.w, img.h
	nd["/Filter"] = pdfName("/ASCIIHexDecode")
	delete(nd, "/Decode")
	delete(nd, "/DecodeParms")
	op.Operands = []pdfValue{nd, hex.EncodeToString(img.rgb) + ">"}
	return true
}
