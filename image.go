package asposepdf

import (
	"io"
	"math"
	"os"
)

// ImageFormat describes the output format of an extracted image.
type ImageFormat int

const (
	ImageFormatPNG  ImageFormat = iota
	ImageFormatJPEG
)

// ImageColorSpace describes the original color space of the image in the PDF.
type ImageColorSpace int

const (
	ColorSpaceDeviceRGB  ImageColorSpace = iota
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
	return extractImagesFromOps(p.doc.objects, ops, resources)
}

// extractImagesFromOps walks content stream ops, tracking CTM, and extracts images.
func extractImagesFromOps(objects map[int]*pdfObject, ops []contentOp, resources pdfDict) ([]Image, error) {
	var images []Image
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
				if img, ok := extractXObjectImage(objects, resources, name, ctm); ok {
					images = append(images, img)
				}
			}
		}
	}
	return images, nil
}

// extractXObjectImage extracts an image from an XObject reference.
// Returns false if the XObject is not an image or can't be decoded.
func extractXObjectImage(objects map[int]*pdfObject, resources pdfDict, name string, ctm [6]float64) (Image, bool) {
	if name == "" || resources == nil {
		return Image{}, false
	}
	xobjVal, ok := resources["/XObject"]
	if !ok {
		return Image{}, false
	}
	xobjDict, ok := resolveRefToDict(objects, xobjVal)
	if !ok {
		return Image{}, false
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return Image{}, false
	}
	resolved := resolveRef(objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return Image{}, false
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Image" {
		return Image{}, false
	}

	width := dictGetInt(stream.Dict, "/Width")
	height := dictGetInt(stream.Dict, "/Height")
	bpc := dictGetInt(stream.Dict, "/BitsPerComponent")
	if width <= 0 || height <= 0 {
		return Image{}, false
	}

	cs := resolveColorSpace(objects, stream.Dict)
	filter := primaryFilter(stream.Dict)

	img := Image{
		Width:      width,
		Height:     height,
		BPC:        bpc,
		ColorSpace: cs,
		X:          ctm[4],
		Y:          ctm[5],
		PageWidth:  math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1]),
		PageHeight: math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3]),
	}

	if filter == "/DCTDecode" {
		// JPEG passthrough — use raw stream bytes (before decoding).
		img.Data = stream.Data
		if stream.Decoded {
			img.Data = getRawStreamData(objects, formVal)
			if img.Data == nil {
				return Image{}, false
			}
		}
		img.Format = ImageFormatJPEG
		return img, true
	}

	// Decode pixels and encode as PNG.
	var rawPixels []byte
	if stream.Decoded {
		rawPixels = stream.Data
	} else {
		var err error
		rawPixels, err = decodeStream(stream.Dict, stream.Data)
		if err != nil {
			return Image{}, false
		}
	}

	components := colorSpaceComponents(objects, stream.Dict, cs)
	if bpc == 0 {
		bpc = 8
	}

	// Resolve soft mask for alpha channel.
	var alphaMask []byte
	if smaskVal, ok := stream.Dict["/SMask"]; ok {
		alphaMask = decodeSoftMask(objects, smaskVal)
	}

	pngData, err := encodePNG(rawPixels, width, height, bpc, components, alphaMask)
	if err != nil {
		return Image{}, false
	}

	img.Data = pngData
	img.Format = ImageFormatPNG
	return img, true
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

