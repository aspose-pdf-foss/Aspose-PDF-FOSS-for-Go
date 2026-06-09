// SPDX-License-Identifier: MIT

package asposepdf

// JBIG2 MMR-coded regions (ITU-T T.88 §6.2.6) reuse the CCITT Group 4 decoder
// (ccitt.go). JBIG2 uses the convention that a 1 bit is foreground (black), which
// is exactly CCITT's /BlackIs1 mode, so the decoded packed rows map straight onto
// a jbig2Bitmap.
func jbig2MMRDecode(data []byte, width, height int) jbig2Bitmap {
	bm := newJBIG2Bitmap(width, height)
	if len(bm) != height || width <= 0 {
		return bm
	}
	packed, _ := ccittDecode(data, ccittParams{k: -1, columns: width, rows: height, blackIs1: true})
	rowBytes := (width + 7) / 8
	for y := 0; y < height; y++ {
		base := y * rowBytes
		if base+rowBytes > len(packed) {
			break
		}
		row := bm[y]
		for x := 0; x < width; x++ {
			row[x] = (packed[base+x/8] >> uint(7-x%8)) & 1
		}
	}
	return bm
}
