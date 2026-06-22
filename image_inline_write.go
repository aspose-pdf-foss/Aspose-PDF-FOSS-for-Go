// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
)

// AddInlineImage draws a small image directly into the page content stream as
// an inline image (BI … ID … EI, ISO 32000-1 §8.9.7) rather than an Image
// XObject. Inline images suit tiny, one-off pictures (icons, rules); for
// anything larger, AddImage (an XObject) is the better choice and the
// recommended default. PNG and JPEG inputs are supported; PNG transparency is
// dropped (inline images cannot carry a soft mask). The image is scaled to fill
// rect. Mirrors the inline-image idiom of ISO 32000-1 §8.9.7.
func (p *Page) AddInlineImage(path string, rect Rectangle) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("AddInlineImage: %w", err)
	}
	return p.addInlineImage(data, rect)
}

// AddInlineImageFromStream is the io.Reader variant of AddInlineImage.
func (p *Page) AddInlineImageFromStream(r io.Reader, rect Rectangle) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("AddInlineImageFromStream: %w", err)
	}
	return p.addInlineImage(data, rect)
}

func (p *Page) addInlineImage(data []byte, rect Rectangle) error {
	if rect.URX <= rect.LLX || rect.URY <= rect.LLY {
		return fmt.Errorf("AddInlineImage: rect must be non-empty")
	}
	format, err := detectImageFormat(data)
	if err != nil {
		return fmt.Errorf("AddInlineImage: %w", err)
	}

	var (
		w, h         int
		csAbbr, filt string
		body         []byte
	)
	switch format {
	case ImageFormatJPEG:
		// Decode to samples and re-store as Flate (not a DCTDecode passthrough):
		// inline images are tiny, and a single colour/filter form keeps the
		// reader simple and the EI boundary unambiguous.
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("AddInlineImage: %w", err)
		}
		var pixels []byte
		var comps int
		pixels, w, h, comps = rgbSamplesFromImage(img)
		if comps == 1 {
			csAbbr = "/G"
		} else {
			csAbbr = "/RGB"
		}
		filt = "[/AHx /Fl]"
		body = asciiHexPDF(flateEncode(pixels))
	case ImageFormatPNG:
		imgStream, smask, err := createPNGXObject(data)
		if err != nil {
			return fmt.Errorf("AddInlineImage: %w", err)
		}
		w, h = toInt(imgStream.Dict["/Width"]), toInt(imgStream.Dict["/Height"])
		csAbbr = csAbbrevForName(imgStream.Dict["/ColorSpace"])
		pixels := imgStream.Data
		// Inline images cannot carry a soft mask, so flatten any alpha over a
		// white background rather than leaving transparent areas black.
		if smask != nil {
			flattenOverWhite(pixels, smask.Data, csComponentCount(csAbbr))
		}
		filt = "[/AHx /Fl]" // Flate then ASCIIHex — decode is ASCIIHex then Flate
		body = asciiHexPDF(flateEncode(pixels))
	default:
		return fmt.Errorf("AddInlineImage: unsupported image format")
	}

	// ASCIIHex data ends with the '>' EOD and contains no '>' before it, so EI
	// is unambiguous (unlike a binary or ASCII85 stream, where '>' can recur).
	rw, rh := rect.URX-rect.LLX, rect.URY-rect.LLY
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "q\n%s 0 0 %s %s %s cm\n", formNum(rw), formNum(rh), formNum(rect.LLX), formNum(rect.LLY))
	fmt.Fprintf(&buf, "BI\n/W %d /H %d /CS %s /BPC 8 /F %s\nID\n", w, h, csAbbr, filt)
	buf.Write(body)
	buf.WriteString("\nEI\nQ\n")
	return p.appendToContentStream(buf.Bytes())
}

// rgbSamplesFromImage extracts 8-bpc samples from a decoded image: gray for
// *image.Gray, RGB otherwise. Used for JPEG inputs (which carry no alpha).
func rgbSamplesFromImage(img image.Image) (pix []byte, w, h, comps int) {
	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	if g, ok := img.(*image.Gray); ok {
		pix = make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				pix[y*w+x] = g.GrayAt(b.Min.X+x, b.Min.Y+y).Y
			}
		}
		return pix, w, h, 1
	}
	pix = make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, gg, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			o := (y*w + x) * 3
			pix[o], pix[o+1], pix[o+2] = byte(r>>8), byte(gg>>8), byte(bb>>8)
		}
	}
	return pix, w, h, 3
}

// csAbbrevForName maps a full device colour-space name to its inline abbreviation.
func csAbbrevForName(v pdfValue) string {
	if n, ok := v.(pdfName); ok {
		switch string(n) {
		case "/DeviceGray":
			return "/G"
		case "/DeviceCMYK":
			return "/CMYK"
		}
	}
	return "/RGB"
}

// flateEncode zlib-compresses data (FlateDecode on read).
func flateEncode(data []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

// asciiHexPDF ASCIIHex-encodes data and appends the '>' end-of-data marker. The
// output contains only [0-9A-F] and the final '>', so the inline-image EI
// terminator is unambiguous.
func asciiHexPDF(data []byte) []byte {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, len(data)*2+1)
	for _, b := range data {
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return append(out, '>')
}

// csComponentCount returns the samples-per-pixel for an inline colour-space
// abbreviation.
func csComponentCount(csAbbr string) int {
	switch csAbbr {
	case "/G":
		return 1
	case "/CMYK":
		return 4
	default:
		return 3
	}
}

// flattenOverWhite composites comps-component pixels over a white background
// using a per-pixel alpha, so transparent areas become white rather than black.
func flattenOverWhite(pix, alpha []byte, comps int) {
	for i := 0; i < len(alpha); i++ {
		a := int(alpha[i])
		if a == 255 {
			continue
		}
		base := i * comps
		if base+comps > len(pix) {
			break
		}
		for c := 0; c < comps; c++ {
			pix[base+c] = byte((int(pix[base+c])*a + 255*(255-a)) / 255)
		}
	}
}
