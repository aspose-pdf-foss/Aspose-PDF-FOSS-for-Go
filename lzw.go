// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// lzwDecode decodes LZWDecode filter data (ISO 32000-1 §7.4.4, the TIFF LZW
// variant): MSB-first bit packing, 9→12-bit variable-width codes, 256 = clear
// table, 257 = end of data, new entries from 258. earlyChange selects whether
// the code width grows one code early (1, the PDF default) or exactly when the
// table fills (0, per /DecodeParms /EarlyChange). Go's compress/lzw cannot be
// used: it only implements the late-change convention.
//
// A truncated stream returns the bytes decoded so far without error (matching
// the tolerance of flateDecode); an impossible code errors.
func lzwDecode(data []byte, earlyChange int) ([]byte, error) {
	const (
		clearCode = 256
		eodCode   = 257
		maxWidth  = 12
	)
	if earlyChange != 0 {
		earlyChange = 1
	}

	table := make([][]byte, 258, 4096)
	resetTable := func() {
		table = table[:258]
		for i := 0; i < 256; i++ {
			table[i] = []byte{byte(i)}
		}
	}
	resetTable()

	var out []byte
	var prev []byte
	width := 9
	bitPos := 0
	totalBits := len(data) * 8

	readCode := func() (int, bool) {
		if bitPos+width > totalBits {
			return 0, false
		}
		v := 0
		for i := 0; i < width; i++ {
			v = v<<1 | int(data[bitPos>>3]>>uint(7-bitPos&7))&1
			bitPos++
		}
		return v, true
	}

	for {
		code, ok := readCode()
		if !ok {
			return out, nil // truncated stream — keep what decoded
		}
		switch {
		case code == clearCode:
			resetTable()
			width = 9
			prev = nil
		case code == eodCode:
			return out, nil
		default:
			var entry []byte
			switch {
			case code < 256 || (code >= 258 && code < len(table)):
				entry = table[code]
			case code == len(table) && prev != nil:
				// KwKwK: the code being defined right now.
				entry = append(append(make([]byte, 0, len(prev)+1), prev...), prev[0])
			default:
				return out, fmt.Errorf("lzw: invalid code %d (table size %d)", code, len(table))
			}
			out = append(out, entry...)
			if prev != nil && len(table) < 1<<maxWidth {
				ne := append(append(make([]byte, 0, len(prev)+1), prev...), entry[0])
				table = append(table, ne)
			}
			prev = entry
			if len(table)+earlyChange >= 1<<width && width < maxWidth {
				width++
			}
		}
	}
}

// lzwEarlyChange reads /EarlyChange from a DecodeParms dict (default 1).
func lzwEarlyChange(params pdfDict) int {
	if params != nil {
		if _, has := params["/EarlyChange"]; has {
			return dictGetInt(params, "/EarlyChange")
		}
	}
	return 1
}
