// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"math"
	"os"
)

// ImageFormat describes the output format of an extracted image.
type ImageFormat int

const (
	ImageFormatPNG ImageFormat = iota
	ImageFormatJPEG
)

// ImageColorSpace describes the original color space of the image in the PDF.
type ImageColorSpace int

const (
	ColorSpaceDeviceRGB ImageColorSpace = iota
	ColorSpaceDeviceGray
	ColorSpaceDeviceCMYK
	ColorSpaceIndexed
	ColorSpaceICCBased
)

// Image holds an extracted image with its encoded data and metadata.
type Image struct {
	Data       []byte          // encoded image bytes (PNG or JPEG)
	Format     ImageFormat     // output format
	Width      int             // pixel width
	Height     int             // pixel height
	BPC        int             // bits per component (original)
	ColorSpace ImageColorSpace // original PDF color space
	X, Y       float64         // position on page (lower-left, in points)
	PageWidth  float64         // display width on page (in points)
	PageHeight float64         // display height on page (in points)
	Inline     bool            // true if from inline image (BI/ID/EI)
}

// ImageInfo holds metadata about an image found on a page without decoding pixel data.
// Call Extract() to perform the actual decoding and get the full Image.
type ImageInfo struct {
	Width      int             // pixel width
	Height     int             // pixel height
	BPC        int             // bits per component (original)
	ColorSpace ImageColorSpace // original PDF color space
	Format     ImageFormat     // output format (PNG or JPEG)
	X, Y       float64         // position on page (lower-left, in points)
	PageWidth  float64         // display width on page (in points)
	PageHeight float64         // display height on page (in points)
	Inline     bool            // true if from inline image (BI/ID/EI)
	Name       string          // XObject name (e.g. "/Im0"); empty for inline

	// private — for deferred extraction
	objects map[int]*pdfObject
	stream  *pdfStream
	formVal pdfValue
	dict    pdfDict // inline: normalized dict
	rawData []byte  // inline: raw image bytes
	ctm     [6]float64
	page    *Page // page this image belongs to (for Replace/Remove)
}

// Save writes the image data to a file.
func (img *Image) Save(path string) error {
	return os.WriteFile(path, img.Data, 0o644)
}

// WriteTo writes the image data to w.
func (img *Image) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(img.Data)
	return int64(n), err
}

// ExtractImages returns images from all pages (one slice per page).
func (d *Document) ExtractImages() ([][]Image, error) {
	pages := d.Pages()
	result := make([][]Image, len(pages))
	for i, p := range pages {
		images, err := p.ExtractImages()
		if err != nil {
			return nil, err
		}
		result[i] = images
	}
	return result, nil
}

// ExtractImages returns all images found on the page.
func (p *Page) ExtractImages() ([]Image, error) {
	infos, err := p.ImageInfos()
	if err != nil {
		return nil, err
	}
	var images []Image
	for i := range infos {
		img, err := infos[i].Extract()
		if err != nil {
			continue // skip undecodable images, same as current behavior
		}
		images = append(images, *img)
	}
	return images, nil
}

// Extract decodes the image and returns the full Image with pixel data.
func (info *ImageInfo) Extract() (*Image, error) {
	img := &Image{
		Width:      info.Width,
		Height:     info.Height,
		BPC:        info.BPC,
		ColorSpace: info.ColorSpace,
		Format:     info.Format,
		X:          info.X,
		Y:          info.Y,
		PageWidth:  info.PageWidth,
		PageHeight: info.PageHeight,
		Inline:     info.Inline,
	}

	if info.Inline {
		return extractInlineImageData(img, info.dict, info.rawData)
	}
	return extractXObjectImageData(img, info.objects, info.stream, info.formVal)
}

// extractXObjectImageData decodes an XObject image stream into the provided Image.
func extractXObjectImageData(img *Image, objects map[int]*pdfObject, stream *pdfStream, formVal pdfValue) (*Image, error) {
	filter := primaryFilter(stream.Dict)

	if filter == "/DCTDecode" {
		jpegData := stream.Data
		if stream.Decoded {
			jpegData = getRawStreamData(objects, formVal)
			if jpegData == nil {
				return nil, fmt.Errorf("cannot read raw JPEG data")
			}
		}

		var alphaMask []byte
		if smaskVal, ok := stream.Dict["/SMask"]; ok {
			alphaMask = decodeSoftMask(objects, smaskVal)
		}

		// CMYK JPEGs decode wrong through Go's CMYK→RGB (Adobe files store
		// inverted ink), and a soft mask needs an RGBA PNG anyway: decode to RGB
		// (decodeJPEGToPixels handles the Adobe inversion) and re-encode as PNG.
		// Plain RGB/Gray JPEGs with no mask pass through untouched.
		if img.ColorSpace == ColorSpaceDeviceCMYK || alphaMask != nil {
			pixels, _, _, err := decodeJPEGToPixels(jpegData)
			if err != nil {
				return nil, err
			}
			pngData, err := encodePNG(pixels, img.Width, img.Height, 8, 3, alphaMask)
			if err != nil {
				return nil, err
			}
			img.Data = pngData
			img.Format = ImageFormatPNG
			return img, nil
		}

		img.Data = jpegData
		img.Format = ImageFormatJPEG
		return img, nil
	}

	if filter == "/JPXDecode" || filter == "/JPX" {
		jpxData := stream.Data
		if stream.Decoded {
			jpxData = getRawStreamData(objects, formVal)
			if jpxData == nil {
				return nil, fmt.Errorf("cannot read raw JPEG2000 data")
			}
		}
		pixels, comps, w, h, err := jpxDecode(jpxData)
		if err != nil {
			return nil, err
		}
		if w > 0 {
			img.Width = w
		}
		if h > 0 {
			img.Height = h
		}
		var alphaMask []byte
		if smaskVal, ok := stream.Dict["/SMask"]; ok {
			alphaMask = decodeSoftMask(objects, smaskVal)
		}
		// MRC scans put a high-res bilevel /Mask stencil over a low-res colour
		// foreground. Composite at the mask's resolution so text stays sharp.
		if maskVal, ok := stream.Dict["/Mask"]; ok {
			if mAlpha, mw, mh, ok := decodeStencilMask(objects, maskVal); ok {
				pixels, alphaMask, img.Width, img.Height = applyStencilMask(pixels, comps, img.Width, img.Height, mAlpha, mw, mh)
			}
		}
		pngData, err := encodePNG(pixels, img.Width, img.Height, 8, comps, alphaMask)
		if err != nil {
			return nil, err
		}
		img.Data = pngData
		img.Format = ImageFormatPNG
		img.BPC = 8
		switch comps {
		case 1:
			img.ColorSpace = ColorSpaceDeviceGray
		case 4:
			img.ColorSpace = ColorSpaceDeviceCMYK
		default:
			img.ColorSpace = ColorSpaceDeviceRGB
		}
		return img, nil
	}

	if filter == "/JBIG2Decode" || filter == "/JBIG2" {
		decoded, err := jbig2Decode(stream.Data, jbig2GlobalsData(objects, stream.Dict), img.Width, img.Height)
		if err != nil {
			return nil, err
		}
		var alphaMask []byte
		if smaskVal, ok := stream.Dict["/SMask"]; ok {
			alphaMask = decodeSoftMask(objects, smaskVal)
		}
		// JBIG2 output is 1-bpp DeviceGray (0 = black, as packed by jbig2Decode).
		pngData, err := encodePNG(decoded, img.Width, img.Height, 1, 1, alphaMask)
		if err != nil {
			return nil, err
		}
		img.Data = pngData
		img.Format = ImageFormatPNG
		return img, nil
	}

	var rawPixels []byte
	if stream.Decoded {
		rawPixels = stream.Data
	} else {
		var err error
		rawPixels, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return nil, err
		}
	}

	bpc := img.BPC
	components := colorSpaceComponents(objects, stream.Dict, img.ColorSpace)
	if bpc == 0 {
		bpc = 8
	}

	if img.ColorSpace == ColorSpaceIndexed {
		palette, baseComponents := resolveIndexedPalette(objects, stream.Dict)
		rawPixels = expandIndexed(rawPixels, palette, baseComponents)
		components = baseComponents
	}

	var alphaMask []byte
	if smaskVal, ok := stream.Dict["/SMask"]; ok {
		alphaMask = decodeSoftMask(objects, smaskVal)
	}

	pngData, err := encodePNG(rawPixels, img.Width, img.Height, bpc, components, alphaMask)
	if err != nil {
		return nil, err
	}

	img.Data = pngData
	img.Format = ImageFormatPNG
	return img, nil
}

// extractInlineImageData decodes an inline image into the provided Image.
func extractInlineImageData(img *Image, dict pdfDict, rawData []byte) (*Image, error) {
	filter := primaryFilter(dict)
	data := rawData

	if filter == "/DCTDecode" {
		img.Data = data
		img.Format = ImageFormatJPEG
		return img, nil
	}

	if filter != "" {
		var err error
		data, err = applyFilter(filter, data)
		if err != nil {
			return nil, err
		}
	}

	components := componentsByCS(img.ColorSpace)
	bpc := img.BPC
	if bpc == 0 {
		bpc = 8
	}

	pngData, err := encodePNG(data, img.Width, img.Height, bpc, components, nil)
	if err != nil {
		return nil, err
	}
	img.Data = pngData
	img.Format = ImageFormatPNG
	return img, nil
}

// decodeStencilMask decodes an explicit /Mask stencil (an /ImageMask stream,
// possibly JBIG2-coded) into a per-pixel alpha map (255 = paint, 0 = masked
// out), honoring /Decode. Returns the mask's own pixel dimensions. ISO 32000-1
// §8.9.6.3: a sample value of 1 masks the point out (default /Decode [0 1]).
func decodeStencilMask(objects map[int]*pdfObject, maskVal pdfValue) (alpha []byte, w, h int, ok bool) {
	stream, isStream := resolveRef(objects, maskVal).(*pdfStream)
	if !isStream {
		return nil, 0, 0, false
	}
	if b, _ := stream.Dict["/ImageMask"].(bool); !b {
		return nil, 0, 0, false
	}
	w = int(operandFloat(resolveRef(objects, stream.Dict["/Width"])))
	h = int(operandFloat(resolveRef(objects, stream.Dict["/Height"])))
	if w <= 0 || h <= 0 {
		return nil, 0, 0, false
	}

	var bits []byte
	switch primaryFilter(stream.Dict) {
	case "/JBIG2Decode", "/JBIG2":
		var err error
		bits, err = jbig2Decode(stream.Data, jbig2GlobalsData(objects, stream.Dict), w, h)
		if err != nil {
			return nil, 0, 0, false
		}
	default:
		if stream.Decoded {
			bits = stream.Data
		} else {
			var err error
			bits, err = decodeStream(stream.Dict, stream.Data)
			if err != nil {
				return nil, 0, 0, false
			}
		}
	}

	// Default /Decode [0 1]: sample 0 = paint. JBIG2 packs foreground as 0, so
	// text paints. /Decode [1 0] inverts which sample value paints.
	paintWhenZero := true
	if dec, okk := stream.Dict["/Decode"].(pdfArray); okk && len(dec) >= 2 {
		if operandFloat(dec[0]) == 1 {
			paintWhenZero = false
		}
	}

	rowBytes := (w + 7) / 8
	alpha = make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bi := y*rowBytes + x/8
			var sample int
			if bi < len(bits) {
				sample = int(bits[bi]>>(7-uint(x%8))) & 1
			}
			paint := (sample == 0) == paintWhenZero
			if paint {
				alpha[y*w+x] = 255
			}
		}
	}
	return alpha, w, h, true
}

// applyStencilMask composites a colour image (rgb, comps channels, iw×iw) with a
// stencil alpha map (mw×mh) at the mask's resolution, nearest-neighbour sampling
// the colour. MRC scans pair a low-res colour foreground with a high-res mask;
// emitting at mask resolution keeps text edges sharp.
func applyStencilMask(rgb []byte, comps, iw, ih int, alpha []byte, mw, mh int) (outRGB, outAlpha []byte, ow, oh int) {
	if iw == mw && ih == mh {
		return rgb, alpha, iw, ih
	}
	ow, oh = mw, mh
	outRGB = make([]byte, ow*oh*comps)
	for y := 0; y < oh; y++ {
		sy := y * ih / oh
		for x := 0; x < ow; x++ {
			sx := x * iw / ow
			copy(outRGB[(y*ow+x)*comps:(y*ow+x)*comps+comps], rgb[(sy*iw+sx)*comps:(sy*iw+sx)*comps+comps])
		}
	}
	return outRGB, alpha, ow, oh
}

// primaryFilter returns the first filter name, or "" if none.
func primaryFilter(d pdfDict) string {
	filterVal, ok := d["/Filter"]
	if !ok {
		return ""
	}
	if n, ok := filterVal.(pdfName); ok {
		return string(n)
	}
	if arr, ok := filterVal.(pdfArray); ok && len(arr) > 0 {
		if n, ok := arr[0].(pdfName); ok {
			return string(n)
		}
	}
	return ""
}

// resolveColorSpace determines the ImageColorSpace from a stream dict.
func resolveColorSpace(objects map[int]*pdfObject, d pdfDict) ImageColorSpace {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return ColorSpaceDeviceRGB
	}
	csVal = resolveRef(objects, csVal)

	switch v := csVal.(type) {
	case pdfName:
		return colorSpaceFromName(string(v))
	case pdfArray:
		if len(v) > 0 {
			if n, ok := v[0].(pdfName); ok {
				name := string(n)
				if name == "/ICCBased" {
					return ColorSpaceICCBased
				}
				if name == "/Indexed" {
					return ColorSpaceIndexed
				}
				return colorSpaceFromName(name)
			}
		}
	}
	return ColorSpaceDeviceRGB
}

func colorSpaceFromName(name string) ImageColorSpace {
	switch name {
	case "/DeviceRGB":
		return ColorSpaceDeviceRGB
	case "/DeviceGray":
		return ColorSpaceDeviceGray
	case "/DeviceCMYK":
		return ColorSpaceDeviceCMYK
	default:
		return ColorSpaceDeviceRGB
	}
}

// getRawStreamData re-reads raw (un-decoded) stream bytes for an object.
func getRawStreamData(objects map[int]*pdfObject, val pdfValue) []byte {
	ref, ok := val.(pdfRef)
	if !ok {
		return nil
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return nil
	}
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		return nil
	}
	if !stream.Decoded {
		return stream.Data
	}
	return nil
}

// colorSpaceComponents returns the number of color components for the image's color space.
func colorSpaceComponents(objects map[int]*pdfObject, d pdfDict, cs ImageColorSpace) int {
	switch cs {
	case ColorSpaceDeviceGray:
		return 1
	case ColorSpaceDeviceRGB:
		return 3
	case ColorSpaceDeviceCMYK:
		return 4
	case ColorSpaceICCBased:
		return iccBasedComponents(objects, d)
	case ColorSpaceIndexed:
		return 1
	default:
		return 3
	}
}

// iccBasedComponents reads /N from the ICCBased color space stream.
func iccBasedComponents(objects map[int]*pdfObject, d pdfDict) int {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return 3
	}
	csVal = resolveRef(objects, csVal)
	arr, ok := csVal.(pdfArray)
	if !ok || len(arr) < 2 {
		return 3
	}
	iccStream := resolveRef(objects, arr[1])
	if s, ok := iccStream.(*pdfStream); ok {
		n := dictGetInt(s.Dict, "/N")
		if n > 0 {
			return n
		}
	}
	return 3
}

// decodeSoftMask decodes a soft mask image XObject to raw grayscale bytes.
func decodeSoftMask(objects map[int]*pdfObject, smaskVal pdfValue) []byte {
	resolved := resolveRef(objects, smaskVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return nil
	}
	if stream.Decoded {
		return stream.Data
	}
	data, err := decodeStream(stream.Dict, stream.Data)
	if err != nil {
		return nil
	}
	return data
}

// resolveIndexedPalette extracts the palette bytes and base component count
// from an Indexed color space array: [/Indexed base hival lookup].
func resolveIndexedPalette(objects map[int]*pdfObject, d pdfDict) ([]byte, int) {
	csVal, ok := d["/ColorSpace"]
	if !ok {
		return nil, 3
	}
	csVal = resolveRef(objects, csVal)
	arr, ok := csVal.(pdfArray)
	if !ok || len(arr) < 4 {
		return nil, 3
	}

	// Base color space (arr[1]).
	baseComponents := 3
	switch v := resolveRef(objects, arr[1]).(type) {
	case pdfName:
		baseComponents = componentsByCS(colorSpaceFromName(string(v)))
	case pdfArray:
		if len(v) > 0 {
			if n, ok := v[0].(pdfName); ok && string(n) == "/ICCBased" && len(v) > 1 {
				if s, ok := resolveRef(objects, v[1]).(*pdfStream); ok {
					baseComponents = dictGetInt(s.Dict, "/N")
					if baseComponents == 0 {
						baseComponents = 3
					}
				}
			}
		}
	}

	// Lookup table (arr[3]) — string or stream.
	var palette []byte
	switch v := resolveRef(objects, arr[3]).(type) {
	case string:
		palette = []byte(v)
	case *pdfStream:
		if v.Decoded {
			palette = v.Data
		} else {
			decoded, err := decodeStream(v.Dict, v.Data)
			if err == nil {
				palette = decoded
			}
		}
	}

	return palette, baseComponents
}

func componentsByCS(cs ImageColorSpace) int {
	switch cs {
	case ColorSpaceDeviceGray:
		return 1
	case ColorSpaceDeviceCMYK:
		return 4
	default:
		return 3
	}
}

// resolveColorSpaceInline resolves color space from an inline image dict.
func resolveColorSpaceInline(dict pdfDict) ImageColorSpace {
	csVal, ok := dict["/ColorSpace"]
	if !ok {
		return ColorSpaceDeviceGray
	}
	if n, ok := csVal.(pdfName); ok {
		return colorSpaceFromName(string(n))
	}
	return ColorSpaceDeviceRGB
}

// collectImageInfos walks content stream ops, tracking CTM, and collects image metadata
// without decoding pixel data.
func collectImageInfos(objects map[int]*pdfObject, ops []contentOp, resources pdfDict) []ImageInfo {
	var infos []ImageInfo
	ctm := identityMatrix()
	var ctmStack [][6]float64

	for _, op := range ops {
		switch op.Operator {
		case "cm":
			if len(op.Operands) >= 6 {
				var m [6]float64
				for i := 0; i < 6; i++ {
					m[i] = operandFloat(op.Operands[i])
				}
				ctm = matMul(m, ctm)
			}
		case "q":
			ctmStack = append(ctmStack, ctm)
		case "Q":
			if len(ctmStack) > 0 {
				ctm = ctmStack[len(ctmStack)-1]
				ctmStack = ctmStack[:len(ctmStack)-1]
			}
		case "Do":
			if len(op.Operands) >= 1 {
				name := operandName(op.Operands[0])
				if info, ok := xobjectImageInfo(objects, resources, name, ctm); ok {
					infos = append(infos, info)
				} else {
					formInfos := formXObjectImageInfos(objects, resources, name, ctm)
					infos = append(infos, formInfos...)
				}
			}
		case "BI":
			if len(op.Operands) >= 2 {
				if info, ok := inlineImageInfo(op.Operands[0], op.Operands[1], ctm); ok {
					infos = append(infos, info)
				}
			}
		}
	}
	return infos
}

// xobjectImageInfo collects metadata for an XObject image without decoding pixels.
func xobjectImageInfo(objects map[int]*pdfObject, resources pdfDict, name string, ctm [6]float64) (ImageInfo, bool) {
	if name == "" || resources == nil {
		return ImageInfo{}, false
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return ImageInfo{}, false
	}
	xobjDict, ok := resolveRefToDict(objects, xobjVal)
	if !ok {
		return ImageInfo{}, false
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return ImageInfo{}, false
	}
	resolved := resolveRef(objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return ImageInfo{}, false
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Image" {
		return ImageInfo{}, false
	}

	width := dictGetInt(stream.Dict, "/Width")
	height := dictGetInt(stream.Dict, "/Height")
	bpc := dictGetInt(stream.Dict, "/BitsPerComponent")
	if width <= 0 || height <= 0 {
		return ImageInfo{}, false
	}

	cs := resolveColorSpace(objects, stream.Dict)
	filter := primaryFilter(stream.Dict)

	// Determine output format.
	format := ImageFormatPNG
	if filter == "/DCTDecode" {
		format = ImageFormatJPEG
		// JPEG with soft mask must be re-encoded as PNG.
		if _, hasSMask := stream.Dict["/SMask"]; hasSMask {
			format = ImageFormatPNG
		}
	}

	return ImageInfo{
		Width:      width,
		Height:     height,
		BPC:        bpc,
		ColorSpace: cs,
		Format:     format,
		X:          ctm[4],
		Y:          ctm[5],
		PageWidth:  math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1]),
		PageHeight: math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3]),
		Name:       name,
		objects:    objects,
		stream:     stream,
		formVal:    formVal,
		ctm:        ctm,
	}, true
}

// inlineImageInfo collects metadata for an inline image without decoding pixels.
func inlineImageInfo(dictVal, dataVal pdfValue, ctm [6]float64) (ImageInfo, bool) {
	dict, ok := dictVal.(pdfDict)
	if !ok {
		return ImageInfo{}, false
	}
	rawData, ok := dataVal.(string)
	if !ok {
		return ImageInfo{}, false
	}

	width := dictGetInt(dict, "/Width")
	height := dictGetInt(dict, "/Height")
	bpc := dictGetInt(dict, "/BitsPerComponent")
	if width <= 0 || height <= 0 {
		return ImageInfo{}, false
	}
	if bpc == 0 {
		bpc = 8
	}

	cs := resolveColorSpaceInline(dict)
	filter := primaryFilter(dict)

	format := ImageFormatPNG
	if filter == "/DCTDecode" {
		format = ImageFormatJPEG
	}

	return ImageInfo{
		Width:      width,
		Height:     height,
		BPC:        bpc,
		ColorSpace: cs,
		Format:     format,
		X:          ctm[4],
		Y:          ctm[5],
		PageWidth:  math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1]),
		PageHeight: math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3]),
		Inline:     true,
		dict:       dict,
		rawData:    []byte(rawData),
		ctm:        ctm,
	}, true
}

// formXObjectImageInfos collects image metadata from a Form XObject's content stream.
func formXObjectImageInfos(objects map[int]*pdfObject, resources pdfDict, name string, ctm [6]float64) []ImageInfo {
	if name == "" || resources == nil {
		return nil
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return nil
	}
	xobjDict, ok := resolveRefToDict(objects, xobjVal)
	if !ok {
		return nil
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return nil
	}
	resolved := resolveRef(objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return nil
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Form" {
		return nil
	}

	var data []byte
	if stream.Decoded {
		data = stream.Data
	} else {
		var err error
		data, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return nil
		}
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil
	}

	formCTM := ctm
	if matVal, ok := stream.Dict["/Matrix"]; ok {
		if arr, ok := matVal.(pdfArray); ok && len(arr) == 6 {
			var fm [6]float64
			for i := 0; i < 6; i++ {
				fm[i] = operandFloat(arr[i])
			}
			formCTM = matMul(fm, ctm)
		}
	}

	formResources := resources
	if resVal, ok := stream.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(objects, resVal); ok {
			formResources = rd
		}
	}

	infos := collectImageInfos(objects, ops, formResources)
	for i := range infos {
		infos[i].X += formCTM[4]
		infos[i].Y += formCTM[5]
	}
	return infos
}

// ImageInfos returns metadata for all images found on the page without decoding pixel data.
func (p *Page) ImageInfos() ([]ImageInfo, error) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	resources := p.pageResources()
	infos := collectImageInfos(p.doc.objects, ops, resources)
	for i := range infos {
		infos[i].page = p
	}
	return infos, nil
}

// ImageInfos returns image metadata for all pages (one slice per page) without decoding pixel data.
func (d *Document) ImageInfos() ([][]ImageInfo, error) {
	pages := d.Pages()
	result := make([][]ImageInfo, len(pages))
	for i, p := range pages {
		infos, err := p.ImageInfos()
		if err != nil {
			return nil, err
		}
		result[i] = infos
	}
	return result, nil
}
