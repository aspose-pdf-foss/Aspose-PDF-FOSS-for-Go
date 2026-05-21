// SPDX-License-Identifier: MIT

package asposepdf

import "sync"

// applyDifferences overlays /Differences entries onto a base encoding.
// diffs is a pdfArray of the form: code₁ /name₁ /name₂ … code₂ /name₃ …
// Each integer starts a run at that code; each name maps glyphToRune[name].
func applyDifferences(base [256]rune, diffs pdfArray) [256]rune {
	enc := base
	code := 0
	for _, v := range diffs {
		switch val := v.(type) {
		case int:
			code = val
		case pdfName:
			name := string(val)
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
			if r, ok := glyphToRune[name]; ok && code < 256 {
				enc[code] = r
			}
			code++
		}
	}
	return enc
}

// WinAnsiEncoding — the most common encoding in PDF (Windows code page 1252).
// Positions 0-31 and 127 are undefined (U+FFFD). 32-126 match ASCII.
// 128-159 have special mappings. 160-255 match Latin-1 Supplement.
var winAnsiEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32-127: ASCII
	' ', '!', '"', '#', '$', '%', '&', '\'',
	'(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7',
	'8', '9', ':', ';', '<', '=', '>', '?',
	'@', 'A', 'B', 'C', 'D', 'E', 'F', 'G',
	'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W',
	'X', 'Y', 'Z', '[', '\\', ']', '^', '_',
	'`', 'a', 'b', 'c', 'd', 'e', 'f', 'g',
	'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w',
	'x', 'y', 'z', '{', '|', '}', '~', '\uFFFD',
	// 128-159: Windows-1252 special characters
	'\u20AC', '\uFFFD', '\u201A', '\u0192', '\u201E', '\u2026', '\u2020', '\u2021', // 128-135
	'\u02C6', '\u2030', '\u0160', '\u2039', '\u0152', '\uFFFD', '\u017D', '\uFFFD', // 136-143
	'\uFFFD', '\u2018', '\u2019', '\u201C', '\u201D', '\u2022', '\u2013', '\u2014', // 144-151
	'\u02DC', '\u2122', '\u0161', '\u203A', '\u0153', '\uFFFD', '\u017E', '\u0178', // 152-159
	// 160-255: Latin-1 Supplement
	'\u00A0', '\u00A1', '\u00A2', '\u00A3', '\u00A4', '\u00A5', '\u00A6', '\u00A7',
	'\u00A8', '\u00A9', '\u00AA', '\u00AB', '\u00AC', '\u00AD', '\u00AE', '\u00AF',
	'\u00B0', '\u00B1', '\u00B2', '\u00B3', '\u00B4', '\u00B5', '\u00B6', '\u00B7',
	'\u00B8', '\u00B9', '\u00BA', '\u00BB', '\u00BC', '\u00BD', '\u00BE', '\u00BF',
	'\u00C0', '\u00C1', '\u00C2', '\u00C3', '\u00C4', '\u00C5', '\u00C6', '\u00C7',
	'\u00C8', '\u00C9', '\u00CA', '\u00CB', '\u00CC', '\u00CD', '\u00CE', '\u00CF',
	'\u00D0', '\u00D1', '\u00D2', '\u00D3', '\u00D4', '\u00D5', '\u00D6', '\u00D7',
	'\u00D8', '\u00D9', '\u00DA', '\u00DB', '\u00DC', '\u00DD', '\u00DE', '\u00DF',
	'\u00E0', '\u00E1', '\u00E2', '\u00E3', '\u00E4', '\u00E5', '\u00E6', '\u00E7',
	'\u00E8', '\u00E9', '\u00EA', '\u00EB', '\u00EC', '\u00ED', '\u00EE', '\u00EF',
	'\u00F0', '\u00F1', '\u00F2', '\u00F3', '\u00F4', '\u00F5', '\u00F6', '\u00F7',
	'\u00F8', '\u00F9', '\u00FA', '\u00FB', '\u00FC', '\u00FD', '\u00FE', '\u00FF',
}

// MacRomanEncoding — used in PDFs generated on macOS.
var macRomanEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32-127: ASCII (same as WinAnsi)
	' ', '!', '"', '#', '$', '%', '&', '\'',
	'(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7',
	'8', '9', ':', ';', '<', '=', '>', '?',
	'@', 'A', 'B', 'C', 'D', 'E', 'F', 'G',
	'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W',
	'X', 'Y', 'Z', '[', '\\', ']', '^', '_',
	'`', 'a', 'b', 'c', 'd', 'e', 'f', 'g',
	'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w',
	'x', 'y', 'z', '{', '|', '}', '~', '\uFFFD',
	// 128-255: Mac OS Roman
	'\u00C4', '\u00C5', '\u00C7', '\u00C9', '\u00D1', '\u00D6', '\u00DC', '\u00E1', // 128-135
	'\u00E0', '\u00E2', '\u00E4', '\u00E3', '\u00E5', '\u00E7', '\u00E9', '\u00E8', // 136-143
	'\u00EA', '\u00EB', '\u00ED', '\u00EC', '\u00EE', '\u00EF', '\u00F1', '\u00F3', // 144-151
	'\u00F2', '\u00F4', '\u00F6', '\u00F5', '\u00FA', '\u00F9', '\u00FB', '\u00FC', // 152-159
	'\u2020', '\u00B0', '\u00A2', '\u00A3', '\u00A7', '\u2022', '\u00B6', '\u00DF', // 160-167
	'\u00AE', '\u00A9', '\u2122', '\u00B4', '\u00A8', '\u2260', '\u00C6', '\u00D8', // 168-175
	'\u221E', '\u00B1', '\u2264', '\u2265', '\u00A5', '\u00B5', '\u2202', '\u2211', // 176-183
	'\u220F', '\u03C0', '\u222B', '\u00AA', '\u00BA', '\u03A9', '\u00E6', '\u00F8', // 184-191
	'\u00BF', '\u00A1', '\u00AC', '\u221A', '\u0192', '\u2248', '\u2206', '\u00AB', // 192-199
	'\u00BB', '\u2026', '\u00A0', '\u00C0', '\u00C3', '\u00D5', '\u0152', '\u0153', // 200-207
	'\u2013', '\u2014', '\u201C', '\u201D', '\u2018', '\u2019', '\u00F7', '\u25CA', // 208-215
	'\u00FF', '\u0178', '\u2044', '\u20AC', '\u2039', '\u203A', '\uFB01', '\uFB02', // 216-223
	'\u2021', '\u00B7', '\u201A', '\u201E', '\u2030', '\u00C2', '\u00CA', '\u00C1', // 224-231
	'\u00CB', '\u00C8', '\u00CD', '\u00CE', '\u00CF', '\u00CC', '\u00D3', '\u00D4', // 232-239
	'\uF8FF', '\u00D2', '\u00DA', '\u00DB', '\u00D9', '\u0131', '\u02C6', '\u02DC', // 240-247
	'\u00AF', '\u02D8', '\u02D9', '\u02DA', '\u00B8', '\u02DD', '\u02DB', '\u02C7', // 248-255
}

// StandardEncoding — the default PostScript encoding for Type 1 fonts.
var standardEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32-127
	' ', '!', '"', '#', '$', '%', '&', '\u2019', // 32-39 (39 = quoteright)
	'(', ')', '*', '+', ',', '-', '.', '/', // 40-47
	'0', '1', '2', '3', '4', '5', '6', '7', // 48-55
	'8', '9', ':', ';', '<', '=', '>', '?', // 56-63
	'@', 'A', 'B', 'C', 'D', 'E', 'F', 'G', // 64-71
	'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', // 72-79
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', // 80-87
	'X', 'Y', 'Z', '[', '\\', ']', '^', '_', // 88-95
	'\u2018', 'a', 'b', 'c', 'd', 'e', 'f', 'g', // 96-103 (96 = quoteleft)
	'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', // 104-111
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w', // 112-119
	'x', 'y', 'z', '{', '|', '}', '~', '\uFFFD', // 120-127
	// 128-160: mostly undefined in StandardEncoding
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD',                                                                       // 160
	'\u00A1',                                                                       // 161 exclamdown
	'\u00A2',                                                                       // 162 cent
	'\u00A3',                                                                       // 163 sterling
	'\u2044',                                                                       // 164 fraction
	'\u00A5',                                                                       // 165 yen
	'\u0192',                                                                       // 166 florin
	'\u00A7',                                                                       // 167 section
	'\u00A4',                                                                       // 168 currency
	'\u0027',                                                                       // 169 quotesingle
	'\u201C',                                                                       // 170 quotedblleft
	'\u00AB',                                                                       // 171 guillemotleft
	'\u2039',                                                                       // 172 guilsinglleft
	'\u203A',                                                                       // 173 guilsinglright
	'\uFB01',                                                                       // 174 fi
	'\uFB02',                                                                       // 175 fl
	'\uFFFD',                                                                       // 176
	'\u2013',                                                                       // 177 endash
	'\u2020',                                                                       // 178 dagger
	'\u2021',                                                                       // 179 daggerdbl
	'\u00B7',                                                                       // 180 periodcentered
	'\uFFFD',                                                                       // 181
	'\u00B6',                                                                       // 182 paragraph
	'\u2022',                                                                       // 183 bullet
	'\u201A',                                                                       // 184 quotesinglbase
	'\u201E',                                                                       // 185 quotedblbase
	'\u201D',                                                                       // 186 quotedblright
	'\u00BB',                                                                       // 187 guillemotright
	'\u2026',                                                                       // 188 ellipsis
	'\u2030',                                                                       // 189 perthousand
	'\uFFFD',                                                                       // 190
	'\u00BF',                                                                       // 191 questiondown
	'\uFFFD',                                                                       // 192
	'\u0060',                                                                       // 193 grave
	'\u00B4',                                                                       // 194 acute
	'\u02C6',                                                                       // 195 circumflex
	'\u02DC',                                                                       // 196 tilde
	'\u00AF',                                                                       // 197 macron
	'\u02D8',                                                                       // 198 breve
	'\u02D9',                                                                       // 199 dotaccent
	'\u00A8',                                                                       // 200 dieresis
	'\uFFFD',                                                                       // 201
	'\u02DA',                                                                       // 202 ring
	'\u00B8',                                                                       // 203 cedilla
	'\uFFFD',                                                                       // 204
	'\u02DD',                                                                       // 205 hungarumlaut
	'\u02DB',                                                                       // 206 ogonek
	'\u02C7',                                                                       // 207 caron
	'\u2014',                                                                       // 208 emdash
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', // 209-216
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', // 217-224
	'\u00C6',                               // 225 AE
	'\uFFFD',                               // 226
	'\u00AA',                               // 227 ordfeminine
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', // 228-231
	'\u0141',                               // 232 Lslash
	'\u00D8',                               // 233 Oslash
	'\u0152',                               // 234 OE
	'\u00BA',                               // 235 ordmasculine
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', // 236-239
	'\uFFFD',           // 240
	'\u00E6',           // 241 ae
	'\uFFFD',           // 242
	'\uFFFD',           // 243
	'\uFFFD',           // 244
	'\u0131',           // 245 dotlessi
	'\uFFFD', '\uFFFD', // 246-247
	'\u0142',                               // 248 lslash
	'\u00F8',                               // 249 oslash
	'\u0153',                               // 250 oe
	'\u00DF',                               // 251 germandbls
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', // 252-255
}

// symbolEncoding — encoding for the Symbol font.
var symbolEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32-127
	' ', '!', '\u2200', '#', '\u2203', '%', '&', '\u220B', // 32-39
	'(', ')', '\u2217', '+', ',', '\u2212', '.', '/', // 40-47
	'0', '1', '2', '3', '4', '5', '6', '7', // 48-55
	'8', '9', ':', ';', '<', '=', '>', '?', // 56-63
	'\u2245', '\u0391', '\u0392', '\u03A7', '\u0394', '\u0395', '\u03A6', '\u0393', // 64-71
	'\u0397', '\u0399', '\u03D1', '\u039A', '\u039B', '\u039C', '\u039D', '\u039F', // 72-79
	'\u03A0', '\u0398', '\u03A1', '\u03A3', '\u03A4', '\u03A5', '\u03C2', '\u03A9', // 80-87
	'\u039E', '\u03A8', '\u0396', '[', '\u2234', ']', '\u22A5', '_', // 88-95
	'\uF8E5', '\u03B1', '\u03B2', '\u03C7', '\u03B4', '\u03B5', '\u03C6', '\u03B3', // 96-103
	'\u03B7', '\u03B9', '\u03D5', '\u03BA', '\u03BB', '\u03BC', '\u03BD', '\u03BF', // 104-111
	'\u03C0', '\u03B8', '\u03C1', '\u03C3', '\u03C4', '\u03C5', '\u03D6', '\u03C9', // 112-119
	'\u03BE', '\u03C8', '\u03B6', '{', '|', '}', '\u223C', '\uFFFD', // 120-127
	// 128-159: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 160-255
	'\u20AC', '\u03D2', '\u2032', '\u2264', '\u2044', '\u221E', '\u0192', '\u2663', // 160-167
	'\u2666', '\u2665', '\u2660', '\u2194', '\u2190', '\u2191', '\u2192', '\u2193', // 168-175
	'\u00B0', '\u00B1', '\u2033', '\u2265', '\u00D7', '\u221D', '\u2202', '\u2022', // 176-183
	'\u00F7', '\u2260', '\u2261', '\u2248', '\u2026', '\u23AF', '\u23D0', '\u21B5', // 184-191
	'\u2135', '\u2111', '\u211C', '\u2118', '\u2297', '\u2295', '\u2205', '\u2229', // 192-199
	'\u222A', '\u2283', '\u2287', '\u2284', '\u2282', '\u2286', '\u2208', '\u2209', // 200-207
	'\u2220', '\u2207', '\u00AE', '\u00A9', '\u2122', '\u220F', '\u221A', '\u22C5', // 208-215
	'\u00AC', '\u2227', '\u2228', '\u21D4', '\u21D0', '\u21D1', '\u21D2', '\u21D3', // 216-223
	'\u25CA', '\u2329', '\uFFFD', '\uFFFD', '\uFFFD', '\u2211', '\u239B', '\u239C', // 224-231
	'\u239D', '\u23A1', '\u23A2', '\u23A3', '\u23A7', '\u23A8', '\u23A9', '\u23AA', // 232-239
	'\uFFFD', '\u232A', '\u222B', '\u2320', '\u23AE', '\u2321', '\u239E', '\u239F', // 240-247
	'\u23A0', '\u23A4', '\u23A5', '\u23A6', '\u23AB', '\u23AC', '\u23AD', '\uFFFD', // 248-255
}

// zapfDingbatsEncoding — encoding for the ZapfDingbats font.
var zapfDingbatsEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32-127: Dingbats characters
	' ', '\u2701', '\u2702', '\u2703', '\u2704', '\u260E', '\u2706', '\u2707', // 32-39
	'\u2708', '\u2709', '\u261B', '\u261E', '\u270C', '\u270D', '\u270E', '\u270F', // 40-47
	'\u2710', '\u2711', '\u2712', '\u2713', '\u2714', '\u2715', '\u2716', '\u2717', // 48-55
	'\u2718', '\u2719', '\u271A', '\u271B', '\u271C', '\u271D', '\u271E', '\u271F', // 56-63
	'\u2720', '\u2721', '\u2722', '\u2723', '\u2724', '\u2725', '\u2726', '\u2727', // 64-71
	'\u2605', '\u2729', '\u272A', '\u272B', '\u272C', '\u272D', '\u272E', '\u272F', // 72-79
	'\u2730', '\u2731', '\u2732', '\u2733', '\u2734', '\u2735', '\u2736', '\u2737', // 80-87
	'\u2738', '\u2739', '\u273A', '\u273B', '\u273C', '\u273D', '\u273E', '\u273F', // 88-95
	'\u2740', '\u2741', '\u2742', '\u2743', '\u2744', '\u2745', '\u2746', '\u2747', // 96-103
	'\u2748', '\u2749', '\u274A', '\u274B', '\u25CF', '\u274D', '\u25A0', '\u274F', // 104-111
	'\u2750', '\u2751', '\u2752', '\u25B2', '\u25BC', '\u25C6', '\u2756', '\u25D7', // 112-119
	'\u2758', '\u2759', '\u275A', '\u275B', '\u275C', '\u275D', '\u275E', '\uFFFD', // 120-127
	// 128-160: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', // 160
	// 161-255
	'\u2761', '\u2762', '\u2763', '\u2764', '\u2765', '\u2766', '\u2767', // 161-167
	'\u2663', '\u2666', '\u2665', '\u2660', '\u2460', '\u2461', '\u2462', '\u2463', // 168-175
	'\u2464', '\u2465', '\u2466', '\u2467', '\u2468', '\u2469', '\u2776', '\u2777', // 176-183
	'\u2778', '\u2779', '\u277A', '\u277B', '\u277C', '\u277D', '\u277E', '\u277F', // 184-191
	'\u2780', '\u2781', '\u2782', '\u2783', '\u2784', '\u2785', '\u2786', '\u2787', // 192-199
	'\u2788', '\u2789', '\u278A', '\u278B', '\u278C', '\u278D', '\u278E', '\u278F', // 200-207
	'\u2790', '\u2791', '\u2792', '\u2793', '\u2794', '\u2192', '\u2194', '\u2195', // 208-215
	'\u2798', '\u2799', '\u279A', '\u279B', '\u279C', '\u279D', '\u279E', '\u279F', // 216-223
	'\u27A0', '\u27A1', '\u27A2', '\u27A3', '\u27A4', '\u27A5', '\u27A6', '\u27A7', // 224-231
	'\u27A8', '\u27A9', '\u27AA', '\u27AB', '\u27AC', '\u27AD', '\u27AE', '\u27AF', // 232-239
	'\uFFFD', '\u27B1', '\u27B2', '\u27B3', '\u27B4', '\u27B5', '\u27B6', '\u27B7', // 240-247
	'\u27B8', '\u27B9', '\u27BA', '\u27BB', '\u27BC', '\u27BD', '\u27BE', '\uFFFD', // 248-255
}

// macExpertEncoding — used by expert character sets in Type 1 fonts.
// Contains old-style figures, small caps, fractions, and other typographic extras.
// Code→glyph-name mappings from PDF Reference Table D.4 (verified against Apache PDFBox).
// Small caps map to their full-size letter for text extraction usability.
// Old-style figures map to standard digits. PUA codepoints are avoided.
var macExpertEncoding = [256]rune{
	// 0-31: undefined
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 32: space, 33: exclamsmall, 34: Hungarumlautsmall, 35: undef
	// 36: centoldstyle, 37: dollaroldstyle, 38: dollarsuperior, 39: ampersandsmall
	' ', '!', '\u02DD', '\uFFFD', '\u00A2', '$', '$', '&',
	// 40: Acutesmall, 41: parenleftsuperior, 42: parenrightsuperior, 43: twodotenleader
	// 44: onedotenleader, 45: comma, 46: hyphen, 47: period
	'\u00B4', '\u207D', '\u207E', '\u2025', '\u2024', ',', '-', '.',
	// 48: fraction, 49-58: zerooldstyle..nineoldstyle, 59: colon, 60: semicolon
	'\u2044', '0', '1', '2', '3', '4', '5', '6',
	'7', '8', ':', ';', '\uFFFD', '\uFFFD', '\uFFFD', '?',
	// 64-71: undef, undef, undef, undef, Ethsmall(67→Ð), undef..
	'\uFFFD', '\uFFFD', '\uFFFD', '\u00D0', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 72: onehalf, 73: onequarter, 74: threequarters
	// 75: oneeighth, 76: threeeighths, 77: fiveeighths, 78: seveneighths, 79: onethird
	'\u00BD', '\u00BC', '\u00BE', '\u215B', '\u215C', '\u215D', '\u215E', '\u2153',
	// 80: twothirds, 81: zerosuperior, 82-87: foursuperior..ninesuperior
	'\u2154', '\u2070', '\u2074', '\u2075', '\u2076', '\u2077', '\u2078', '\u2079',
	// 88-97: zeroinferior..nineinferior
	'\u2080', '\u2081', '\u2082', '\u2083', '\u2084', '\u2085', '\u2086', '\u2087',
	'\u2088', '\u2089',
	// 98: centinferior, 99: dollarinferior, 100: periodinferior, 101: commainferior
	'\u00A2', '$', '.', ',',
	// 102-107: Agravesmall..Aringsmall → small caps → uppercase letters
	'\u00C0', '\u00C1', '\u00C2', '\u00C3', '\u00C4', '\u00C5',
	// 108: AEsmall, 109: Ccedillasmall
	'\u00C6', '\u00C7',
	// 110-117: Egravesmall..Idieresissmall
	'\u00C8', '\u00C9', '\u00CA', '\u00CB', '\u00CC', '\u00CD', '\u00CE', '\u00CF',
	// 118: Engsmall(→Ŋ), 119: undef(Ntildesmall→Ñ)
	'\u014A', '\u00D1',
	// 120-123: Oacutesmall..Odieresissmall, 124: OEsmall, 125: Oslashsmall
	'\u00D3', '\u00D4', '\u00D5', '\u00D6', '\u0152', '\u00D8',
	// 126: Ugravesmall, 127: Uacutesmall
	'\u00D9', '\u00DA',
	// 128: Ucircumflexsmall, 129: Udieresissmall
	'\u00DB', '\u00DC',
	// 130: undef, 131: Yacutesmall, 132: Thornsmall, 133: Ydieresissmall
	'\uFFFD', '\u00DD', '\u00DE', '\u0178',
	// 134: undef(osabornemedieval), 135: Aacutesmall(dup→Á)
	'\uFFFD', '\u00C1',
	// 136: undef(Acircumflexsmall dup), 137: undef, 138: undef, 139: undef
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 140: undef, 141: undef, 142: undef, 143: undef
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 144: exclamdownsmall
	'\u00A1',
	// 145-148: undef
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 149: cent
	'\u00A2',
	// 150-151: undef
	'\uFFFD', '\uFFFD',
	// 152: Lslashsmall(→Ł), 153-154: undef, 155: Scaronsmall(→Š), 156: Zcaronsmall(→Ž)
	'\u0141', '\uFFFD', '\uFFFD', '\u0160', '\u017D',
	// 157: Dieresissmall(→¨), 158: Brevesmall(→˘), 159: Caronsmall(→ˇ)
	'\u00A8', '\u02D8', '\u02C7',
	// 160: Dotaccentsmall(→˙), 161: Macronsmall(→¯), 162: figuredash(→‒)
	'\u02D9', '\u00AF', '\u2012',
	// 163: hypheninferior(→-), 164: Ogoneksmall(→˛), 165: Ringsmall(→˚)
	'-', '\u02DB', '\u02DA',
	// 166: Cedillasmall(→¸), 167: questiondownsmall(→¿)
	'\u00B8', '\u00BF',
	// 168: undef, 169: onesuperior(→¹), 170: twosuperior(→²), 171: threesuperior(→³)
	'\uFFFD', '\u00B9', '\u00B2', '\u00B3',
	// 172: centsuperior(→¢), 173-174: undef
	'\u00A2', '\uFFFD', '\uFFFD',
	// 175: parenleftinferior(→₍), 176: parenrightinferior(→₎)
	'\u208D', '\u208E',
	// 177-182: undef
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 183-196: Asmall..Nsmall → A..N
	'A', 'B', '\uFFFD', 'C', 'D', 'E', 'F', 'G', 'H',
	'I', 'J', 'K', 'L', 'M', 'N',
	// 198: undef, 199-210: Osmall..Zsmall → O..Z
	'\uFFFD', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
	// 211: colonmonetary(→₡), 212: onefitted(→1), 213: rupiah
	'\u20A1', '1', '\uFFFD',
	// 214: Tildesmall(→˜), 215: undef
	'\u02DC', '\uFFFD',
	// 216: Ydieresissmall(dup→Ÿ), 217-218: undef
	'\u0178', '\uFFFD', '\uFFFD',
	// 219-225: undef
	'\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	// 226: lslash(→ł), 227-228: undef
	'\u0142', '\uFFFD', '\uFFFD',
	// 229: ff, 230: fi, 231: fl, 232: ffi, 233: ffl
	'\uFB00', '\uFB01', '\uFB02', '\uFB03', '\uFB04',
	// 234: parenleftbt(→⁽), 235: parenrightbt(→⁾)
	'\u207D', '\u207E',
	// 236: Circumflexsmall(→ˆ), 237: hyphensuperior(→-)
	'\u02C6', '-',
	// 238: Gravesmall(→`), 239-247: undef
	'`', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD', '\uFFFD',
	'\uFFFD', '\uFFFD',
	// 248: periodcentered(→·), 249-251: undef
	'\u00B7', '\uFFFD', '\uFFFD', '\uFFFD',
	// 252: periodsuperior(→.), 253-255: undef
	'.', '\uFFFD', '\uFFFD', '\uFFFD',
}

// glyphToRune maps Adobe glyph names to Unicode rune values.
// Used by applyDifferences to resolve /Differences entries.
// Derived from the Adobe Glyph List (AGL).
var glyphToRune = map[string]rune{
	"A": 'A', "AE": '\u00C6', "Aacute": '\u00C1', "Abreve": '\u0102',
	"Acircumflex": '\u00C2', "Adieresis": '\u00C4', "Agrave": '\u00C0',
	"Alpha": '\u0391', "Amacron": '\u0100', "Aogonek": '\u0104',
	"Aring": '\u00C5', "Atilde": '\u00C3',
	"B": 'B', "Beta": '\u0392',
	"C": 'C', "Cacute": '\u0106', "Ccaron": '\u010C', "Ccedilla": '\u00C7',
	"Chi": '\u03A7',
	"D":   'D', "Dcaron": '\u010E', "Dcroat": '\u0110', "Delta": '\u0394',
	"E": 'E', "Eacute": '\u00C9', "Ecaron": '\u011A', "Ecircumflex": '\u00CA',
	"Edieresis": '\u00CB', "Egrave": '\u00C8', "Emacron": '\u0112',
	"Eogonek": '\u0118', "Epsilon": '\u0395', "Eta": '\u0397', "Eth": '\u00D0',
	"Euro": '\u20AC',
	"F":    'F',
	"G":    'G', "Gamma": '\u0393', "Gbreve": '\u011E', "Gcommaaccent": '\u0122',
	"H": 'H',
	"I": 'I', "Iacute": '\u00CD', "Icircumflex": '\u00CE', "Idieresis": '\u00CF',
	"Idotaccent": '\u0130', "Igrave": '\u00CC', "Imacron": '\u012A',
	"Iogonek": '\u012E', "Iota": '\u0399',
	"J": 'J',
	"K": 'K', "Kappa": '\u039A', "Kcommaaccent": '\u0136',
	"L": 'L', "Lacute": '\u0139', "Lambda": '\u039B', "Lcaron": '\u013D',
	"Lcommaaccent": '\u013B', "Lslash": '\u0141',
	"M": 'M', "Mu": '\u039C',
	"N": 'N', "Nacute": '\u0143', "Ncaron": '\u0147', "Ncommaaccent": '\u0145',
	"Ntilde": '\u00D1', "Nu": '\u039D',
	"O": 'O', "OE": '\u0152', "Oacute": '\u00D3', "Ocircumflex": '\u00D4',
	"Odieresis": '\u00D6', "Ograve": '\u00D2', "Ohungarumlaut": '\u0150',
	"Omacron": '\u014C', "Omega": '\u03A9', "Omicron": '\u039F',
	"Oslash": '\u00D8', "Otilde": '\u00D5',
	"P": 'P', "Phi": '\u03A6', "Pi": '\u03A0', "Psi": '\u03A8',
	"Q": 'Q',
	"R": 'R', "Racute": '\u0154', "Rcaron": '\u0158', "Rcommaaccent": '\u0156',
	"Rho": '\u03A1',
	"S":   'S', "Sacute": '\u015A', "Scaron": '\u0160', "Scedilla": '\u015E',
	"Scommaaccent": '\u0218', "Sigma": '\u03A3',
	"T": 'T', "Tau": '\u03A4', "Tcaron": '\u0164', "Tcommaaccent": '\u021A',
	"Theta": '\u0398', "Thorn": '\u00DE',
	"U": 'U', "Uacute": '\u00DA', "Ucircumflex": '\u00DB', "Udieresis": '\u00DC',
	"Ugrave": '\u00D9', "Uhungarumlaut": '\u0170', "Umacron": '\u016A',
	"Uogonek": '\u0172', "Upsilon": '\u03A5', "Uring": '\u016E',
	"V": 'V',
	"W": 'W',
	"X": 'X', "Xi": '\u039E',
	"Y": 'Y', "Yacute": '\u00DD', "Ydieresis": '\u0178',
	"Z": 'Z', "Zacute": '\u0179', "Zcaron": '\u017D', "Zdotaccent": '\u017B',
	"Zeta": '\u0396',
	"a":    'a', "aacute": '\u00E1', "abreve": '\u0103', "acircumflex": '\u00E2',
	"acute": '\u00B4', "adieresis": '\u00E4', "ae": '\u00E6',
	"agrave": '\u00E0', "alpha": '\u03B1', "amacron": '\u0101',
	"ampersand": '&', "aogonek": '\u0105', "aring": '\u00E5',
	"asciicircum": '^', "asciitilde": '~', "asterisk": '*',
	"at": '@', "atilde": '\u00E3',
	"b": 'b', "backslash": '\\', "bar": '|', "beta": '\u03B2',
	"braceleft": '{', "braceright": '}', "bracketleft": '[', "bracketright": ']',
	"breve": '\u02D8', "brokenbar": '\u00A6', "bullet": '\u2022',
	"c": 'c', "cacute": '\u0107', "caron": '\u02C7', "ccaron": '\u010D',
	"ccedilla": '\u00E7', "cedilla": '\u00B8', "cent": '\u00A2',
	"chi": '\u03C7', "circumflex": '\u02C6', "colon": ':', "comma": ',',
	"copyright": '\u00A9', "currency": '\u00A4',
	"d": 'd', "dagger": '\u2020', "daggerdbl": '\u2021', "dcaron": '\u010F',
	"dcroat": '\u0111', "degree": '\u00B0', "delta": '\u03B4',
	"dieresis": '\u00A8', "divide": '\u00F7', "dollar": '$',
	"dotaccent": '\u02D9', "dotlessi": '\u0131',
	"e": 'e', "eacute": '\u00E9', "ecaron": '\u011B', "ecircumflex": '\u00EA',
	"edieresis": '\u00EB', "egrave": '\u00E8', "eight": '8',
	"ellipsis": '\u2026', "emacron": '\u0113', "emdash": '\u2014',
	"endash": '\u2013', "eogonek": '\u0119', "epsilon": '\u03B5',
	"equal": '=', "eta": '\u03B7', "eth": '\u00F0', "exclam": '!',
	"exclamdown": '\u00A1',
	"f":          'f', "fi": '\uFB01', "five": '5', "fl": '\uFB02', "florin": '\u0192',
	"four": '4', "fraction": '\u2044',
	"g": 'g', "gamma": '\u03B3', "gbreve": '\u011F', "gcommaaccent": '\u0123',
	"germandbls": '\u00DF', "grave": '`', "greater": '>', "guillemotleft": '\u00AB',
	"guillemotright": '\u00BB', "guilsinglleft": '\u2039', "guilsinglright": '\u203A',
	"h": 'h', "hungarumlaut": '\u02DD', "hyphen": '-',
	"i": 'i', "iacute": '\u00ED', "icircumflex": '\u00EE', "idieresis": '\u00EF',
	"igrave": '\u00EC', "imacron": '\u012B', "iogonek": '\u012F', "iota": '\u03B9',
	"j": 'j',
	"k": 'k', "kappa": '\u03BA', "kcommaaccent": '\u0137',
	"l": 'l', "lacute": '\u013A', "lambda": '\u03BB', "lcaron": '\u013E',
	"lcommaaccent": '\u013C', "less": '<', "logicalnot": '\u00AC',
	"lozenge": '\u25CA', "lslash": '\u0142',
	"m": 'm', "macron": '\u00AF', "minus": '\u2212', "mu": '\u03BC',
	"multiply": '\u00D7',
	"n":        'n', "nacute": '\u0144', "ncaron": '\u0148', "ncommaaccent": '\u0146',
	"nine": '9', "ntilde": '\u00F1', "nu": '\u03BD', "numbersign": '#',
	"o": 'o', "oacute": '\u00F3', "ocircumflex": '\u00F4', "odieresis": '\u00F6',
	"oe": '\u0153', "ogonek": '\u02DB', "ograve": '\u00F2',
	"ohungarumlaut": '\u0151', "omacron": '\u014D', "omega": '\u03C9',
	"omicron": '\u03BF', "one": '1', "onehalf": '\u00BD', "onequarter": '\u00BC',
	"onesuperior": '\u00B9', "ordfeminine": '\u00AA', "ordmasculine": '\u00BA',
	"oslash": '\u00F8', "otilde": '\u00F5',
	"p": 'p', "paragraph": '\u00B6', "parenleft": '(', "parenright": ')',
	"percent": '%', "period": '.', "periodcentered": '\u00B7',
	"perthousand": '\u2030', "phi": '\u03C6', "pi": '\u03C0', "plus": '+',
	"plusminus": '\u00B1', "psi": '\u03C8',
	"q": 'q', "question": '?', "questiondown": '\u00BF', "quotedbl": '"',
	"quotedblbase": '\u201E', "quotedblleft": '\u201C', "quotedblright": '\u201D',
	"quoteleft": '\u2018', "quoteright": '\u2019', "quotesinglbase": '\u201A',
	"quotesingle": '\'',
	"r":           'r', "racute": '\u0155', "radical": '\u221A', "rcaron": '\u0159',
	"rcommaaccent": '\u0157', "registered": '\u00AE', "rho": '\u03C1',
	"ring": '\u02DA',
	"s":    's', "sacute": '\u015B', "scaron": '\u0161', "scedilla": '\u015F',
	"scommaaccent": '\u0219', "section": '\u00A7', "semicolon": ';',
	"seven": '7', "sigma": '\u03C3', "six": '6', "slash": '/',
	"space": ' ', "sterling": '\u00A3',
	"t": 't', "tau": '\u03C4', "tcaron": '\u0165', "tcommaaccent": '\u021B',
	"theta": '\u03B8', "thorn": '\u00FE', "three": '3',
	"threequarters": '\u00BE', "threesuperior": '\u00B3', "tilde": '\u02DC',
	"trademark": '\u2122', "two": '2', "twosuperior": '\u00B2',
	"u": 'u', "uacute": '\u00FA', "ucircumflex": '\u00FB', "udieresis": '\u00FC',
	"ugrave": '\u00F9', "uhungarumlaut": '\u0171', "umacron": '\u016B',
	"underscore": '_', "uogonek": '\u0173', "upsilon": '\u03C5',
	"uring": '\u016F',
	"v":     'v',
	"w":     'w',
	"x":     'x', "xi": '\u03BE',
	"y": 'y', "yacute": '\u00FD', "ydieresis": '\u00FF', "yen": '\u00A5',
	"z": 'z', "zacute": '\u017A', "zcaron": '\u017E', "zdotaccent": '\u017C',
	"zero": '0', "zeta": '\u03B6',
}

var (
	winAnsiReverseOnce sync.Once
	winAnsiReverse     map[rune]byte
)

// winAnsiEncodeRune returns the WinAnsi byte code for the given rune.
// Returns (0, false) if the rune is not representable in WinAnsiEncoding.
func winAnsiEncodeRune(r rune) (byte, bool) {
	winAnsiReverseOnce.Do(func() {
		winAnsiReverse = make(map[rune]byte, 256)
		for code, ch := range winAnsiEncoding {
			if ch == '\uFFFD' {
				continue
			}
			// First occurrence wins (WinAnsi has no duplicates anyway).
			if _, exists := winAnsiReverse[ch]; !exists {
				winAnsiReverse[ch] = byte(code)
			}
		}
	})
	c, ok := winAnsiReverse[r]
	return c, ok
}

var (
	symbolReverseOnce sync.Once
	symbolReverse     map[rune]byte

	zapfDingbatsReverseOnce sync.Once
	zapfDingbatsReverse     map[rune]byte
)

// symbolEncodeRune returns the Symbol-font byte code for the given rune.
// Returns (0, false) if the rune is not representable in symbolEncoding.
func symbolEncodeRune(r rune) (byte, bool) {
	symbolReverseOnce.Do(func() {
		symbolReverse = buildReverseEncoding(symbolEncoding)
	})
	c, ok := symbolReverse[r]
	return c, ok
}

// zapfDingbatsEncodeRune returns the ZapfDingbats byte code for the given rune.
// Returns (0, false) if the rune is not representable in zapfDingbatsEncoding.
func zapfDingbatsEncodeRune(r rune) (byte, bool) {
	zapfDingbatsReverseOnce.Do(func() {
		zapfDingbatsReverse = buildReverseEncoding(zapfDingbatsEncoding)
	})
	c, ok := zapfDingbatsReverse[r]
	return c, ok
}

// buildReverseEncoding inverts a 256-slot encoding table into a rune→byte
// map. U+FFFD slots (marker for undefined codes) are skipped; the lowest
// code wins on duplicates.
func buildReverseEncoding(enc [256]rune) map[rune]byte {
	rev := make(map[rune]byte, 256)
	for code, ch := range enc {
		if ch == '�' {
			continue
		}
		if _, exists := rev[ch]; !exists {
			rev[ch] = byte(code)
		}
	}
	return rev
}

// encodeRuneForStandardFont dispatches rune→byte encoding to the right
// table for a Standard 14 PDF font. Symbol and ZapfDingbats use their
// built-in encodings; every other Standard 14 font uses WinAnsi, which
// AddText pins via /Encoding /WinAnsiEncoding on the font resource.
func encodeRuneForStandardFont(pdfFontName string, r rune) (byte, bool) {
	switch pdfFontName {
	case "/Symbol":
		return symbolEncodeRune(r)
	case "/ZapfDingbats":
		return zapfDingbatsEncodeRune(r)
	default:
		return winAnsiEncodeRune(r)
	}
}
