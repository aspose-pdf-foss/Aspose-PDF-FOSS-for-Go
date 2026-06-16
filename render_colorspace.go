// SPDX-License-Identifier: MIT

package asposepdf

// Separation and DeviceN colour spaces (ISO 32000-1 §8.6.6.4) name one or more
// colorants whose tint values are mapped, through a tint-transform function,
// into an alternate device space. The renderer resolves the space set by cs/CS
// and, on sc/scn, runs the tint operands through the transform to an RGB colour.

// tintFunc converts a set of tint values to an 8-bit RGB colour.
type tintFunc func(tints []float64) (uint8, uint8, uint8)

// namedColorSpace resolves a colour-space operand: a device space name is
// returned as-is; any other name is looked up in /Resources/ColorSpace.
func (rd *renderer) namedColorSpace(name string) pdfValue {
	switch name {
	case "/DeviceRGB", "/DeviceGray", "/DeviceCMYK", "/Pattern", "/G", "/RGB", "/CMYK":
		return pdfName(name)
	}
	if csd, ok := resolveRefToDict(rd.page.doc.objects, rd.res["/ColorSpace"]); ok {
		return csd[name]
	}
	return nil
}

// tintConverter builds a tintFunc for a /Separation or /DeviceN colour space, or
// nil for any other space (the caller then uses device colour by operand count).
func (rd *renderer) tintConverter(csVal pdfValue) tintFunc {
	return csTintConverter(rd.page.doc.objects, csVal)
}

// csTintConverter builds the tintFunc for an array-defined /Separation or
// /DeviceN colour space (the only forms that carry a tint transform), or nil for
// any other space. Standalone (no /Resources lookup) so non-renderer callers —
// e.g. shadings — can resolve a DeviceN colour model too.
func csTintConverter(objects map[int]*pdfObject, csVal pdfValue) tintFunc {
	arr, ok := resolveRefToArray(objects, csVal)
	if !ok || len(arr) < 4 {
		return nil
	}
	switch operandName(arr[0]) {
	case "/Separation", "/DeviceN":
	default:
		return nil
	}
	fn := parseFunction(objects, arr[3]) // tint transform (index 3)
	if fn == nil {
		return nil
	}
	return func(tints []float64) (uint8, uint8, uint8) {
		return compsToRGB(fn.eval(tints))
	}
}

// compsToRGB converts alternate-space components to RGB by their count
// (1=gray, 3=RGB, 4=CMYK), mirroring the shading colour model inference.
func compsToRGB(c []float64) (uint8, uint8, uint8) {
	switch len(c) {
	case 1:
		return gray8(c[0])
	case 4:
		return cmykToRGB8(c[0], c[1], c[2], c[3])
	default:
		if len(c) >= 3 {
			return clamp8(c[0]), clamp8(c[1]), clamp8(c[2])
		}
		return 0, 0, 0
	}
}
