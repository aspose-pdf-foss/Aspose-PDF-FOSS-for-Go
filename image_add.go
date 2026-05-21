// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strconv"
)

// createImageXObject builds a pdfStream for the image.
// Returns the image stream and an optional SMask stream (for PNG with alpha).
func createImageXObject(data []byte, format ImageFormat) (*pdfStream, *pdfStream, error) {
	switch format {
	case ImageFormatJPEG:
		return createJPEGXObject(data)
	case ImageFormatPNG:
		return createPNGXObject(data)
	default:
		return nil, nil, fmt.Errorf("unsupported format")
	}
}

func createJPEGXObject(data []byte) (*pdfStream, *pdfStream, error) {
	info, err := parseJPEGHeader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode image: %w", err)
	}

	cs := pdfName("/DeviceRGB")
	if info.components == 1 {
		cs = pdfName("/DeviceGray")
	} else if info.components == 4 {
		cs = pdfName("/DeviceCMYK")
	}

	stream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            info.width,
			"/Height":           info.height,
			"/BitsPerComponent": 8,
			"/ColorSpace":       cs,
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    data,
		Decoded: false,
	}
	return stream, nil, nil
}

func createPNGXObject(data []byte) (*pdfStream, *pdfStream, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode image: %w", err)
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	var pixels []byte
	var alpha []byte
	hasAlpha := false
	cs := pdfName("/DeviceRGB")

	switch src := img.(type) {
	case *image.NRGBA:
		pixels = make([]byte, w*h*3)
		alpha = make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := src.NRGBAAt(x+bounds.Min.X, y+bounds.Min.Y)
				off := (y*w + x) * 3
				pixels[off] = c.R
				pixels[off+1] = c.G
				pixels[off+2] = c.B
				alpha[y*w+x] = c.A
			}
		}
		hasAlpha = true
	case *image.RGBA:
		pixels = make([]byte, w*h*3)
		alpha = make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := src.RGBAAt(x+bounds.Min.X, y+bounds.Min.Y)
				off := (y*w + x) * 3
				// Un-premultiply alpha: RGBA stores premultiplied values,
				// but PDF expects straight (non-premultiplied) color + separate SMask.
				if c.A > 0 {
					pixels[off] = uint8(uint16(c.R) * 255 / uint16(c.A))
					pixels[off+1] = uint8(uint16(c.G) * 255 / uint16(c.A))
					pixels[off+2] = uint8(uint16(c.B) * 255 / uint16(c.A))
				}
				alpha[y*w+x] = c.A
			}
		}
		hasAlpha = true
	case *image.Gray:
		cs = pdfName("/DeviceGray")
		pixels = make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := src.GrayAt(x+bounds.Min.X, y+bounds.Min.Y)
				pixels[y*w+x] = c.Y
			}
		}
	default:
		// Generic fallback: convert to RGB.
		pixels = make([]byte, w*h*3)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
				off := (y*w + x) * 3
				pixels[off] = uint8(r >> 8)
				pixels[off+1] = uint8(g >> 8)
				pixels[off+2] = uint8(b >> 8)
			}
		}
	}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            w,
			"/Height":           h,
			"/BitsPerComponent": 8,
			"/ColorSpace":       cs,
		},
		Data:    pixels,
		Decoded: true,
	}

	// Check if all alpha values are 0xFF — skip SMask if fully opaque.
	if hasAlpha {
		allOpaque := true
		for _, a := range alpha {
			if a != 0xFF {
				allOpaque = false
				break
			}
		}
		if allOpaque {
			hasAlpha = false
		}
	}

	if !hasAlpha {
		return imgStream, nil, nil
	}

	smaskStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            w,
			"/Height":           h,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceGray"),
		},
		Data:    alpha,
		Decoded: true,
	}

	return imgStream, smaskStream, nil
}

// detectImageFormat identifies the image format from the first bytes.
func detectImageFormat(data []byte) (ImageFormat, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("unsupported image format: data too short")
	}
	if data[0] == 0xFF && data[1] == 0xD8 {
		return ImageFormatJPEG, nil
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return ImageFormatPNG, nil
	}
	return 0, fmt.Errorf("unsupported image format")
}

// jpegHeaderInfo holds dimensions and component count from a JPEG header.
type jpegHeaderInfo struct {
	width      int
	height     int
	components int // 1=gray, 3=RGB, 4=CMYK
}

// parseJPEGHeader reads SOF marker to extract dimensions and component count.
func parseJPEGHeader(r io.Reader) (jpegHeaderInfo, error) {
	var buf [2]byte
	// Read SOI marker.
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return jpegHeaderInfo{}, fmt.Errorf("failed to read JPEG SOI: %w", err)
	}
	if buf[0] != 0xFF || buf[1] != 0xD8 {
		return jpegHeaderInfo{}, fmt.Errorf("not a JPEG file")
	}

	// Scan markers to find SOF.
	for {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return jpegHeaderInfo{}, fmt.Errorf("failed to read JPEG marker: %w", err)
		}
		if buf[0] != 0xFF {
			return jpegHeaderInfo{}, fmt.Errorf("invalid JPEG marker")
		}
		marker := buf[1]
		// Skip fill bytes (0xFF).
		for marker == 0xFF {
			if _, err := io.ReadFull(r, buf[1:]); err != nil {
				return jpegHeaderInfo{}, fmt.Errorf("failed to read JPEG marker: %w", err)
			}
			marker = buf[1]
		}
		// SOF markers: C0 (baseline), C1 (extended), C2 (progressive).
		if marker >= 0xC0 && marker <= 0xC2 {
			var sof [6]byte
			var lenBuf [2]byte
			if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
				return jpegHeaderInfo{}, fmt.Errorf("failed to read SOF length: %w", err)
			}
			if _, err := io.ReadFull(r, sof[:]); err != nil {
				return jpegHeaderInfo{}, fmt.Errorf("failed to read SOF data: %w", err)
			}
			return jpegHeaderInfo{
				width:      int(binary.BigEndian.Uint16(sof[3:5])),
				height:     int(binary.BigEndian.Uint16(sof[1:3])),
				components: int(sof[5]),
			}, nil
		}
		// Skip non-SOF marker segment.
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return jpegHeaderInfo{}, fmt.Errorf("failed to read marker length: %w", err)
		}
		segLen := int(binary.BigEndian.Uint16(buf[:])) - 2
		if segLen < 0 {
			return jpegHeaderInfo{}, fmt.Errorf("invalid JPEG segment length")
		}
		if _, err := io.CopyN(io.Discard, r, int64(segLen)); err != nil {
			return jpegHeaderInfo{}, fmt.Errorf("failed to skip JPEG segment: %w", err)
		}
	}
}

// AddImage adds an image from a file to the page within the given rectangle.
func (p *Page) AddImage(path string, rect Rectangle) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("add image: %w", err)
	}
	return p.addImageFromBytes(data, rect)
}

// AddImageFromStream adds an image from a reader to the page within the given rectangle.
func (p *Page) AddImageFromStream(r io.Reader, rect Rectangle) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("add image: %w", err)
	}
	return p.addImageFromBytes(data, rect)
}

func (p *Page) addImageFromBytes(data []byte, rect Rectangle) error {
	if err := rect.validate(); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("add image: empty data")
	}

	format, err := detectImageFormat(data)
	if err != nil {
		return err
	}

	imgStream, smaskStream, err := createImageXObject(data, format)
	if err != nil {
		return err
	}

	// Register SMask if present.
	if smaskStream != nil {
		smaskID := p.doc.nextID
		p.doc.nextID++
		p.doc.objects[smaskID] = &pdfObject{Num: smaskID, Value: smaskStream}
		imgStream.Dict["/SMask"] = pdfRef{Num: smaskID}
	}

	// Register image XObject.
	imgID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[imgID] = &pdfObject{Num: imgID, Value: imgStream}

	// Add to page resources.
	pageDict := p.pageDict()
	if pageDict == nil {
		return fmt.Errorf("add image: page has no dict")
	}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}
	xobjVal := resolveRef(p.doc.objects, resources["/XObject"])
	xobjDict, _ := xobjVal.(pdfDict)
	if xobjDict == nil {
		xobjDict = pdfDict{}
		resources["/XObject"] = xobjDict
	}

	name := nextXObjectName(xobjDict)
	xobjDict[name] = pdfRef{Num: imgID}

	// Append drawing operators to content stream.
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	ops := fmt.Sprintf("\nq\n%s 0 0 %s %s %s cm\n%s Do\nQ\n",
		formatFloat(w), formatFloat(h), formatFloat(rect.LLX), formatFloat(rect.LLY), name)

	return p.appendToContentStream([]byte(ops))
}

func nextXObjectName(xobjDict pdfDict) string {
	for i := 0; ; i++ {
		name := "/Im" + strconv.Itoa(i)
		if _, exists := xobjDict[name]; !exists {
			return name
		}
	}
}

func (p *Page) appendToContentStream(data []byte) error {
	existing, err := p.contentStreams()
	if err != nil {
		return err
	}

	newData := append(existing, data...)
	newStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    newData,
		Decoded: true,
	}

	newID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[newID] = &pdfObject{Num: newID, Value: newStream}

	pageDict := p.pageDict()
	pageDict["/Contents"] = pdfRef{Num: newID}
	return nil
}
