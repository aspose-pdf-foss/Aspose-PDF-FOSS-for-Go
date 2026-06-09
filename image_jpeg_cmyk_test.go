// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestJpegHasAdobeMarker checks the APP14 "Adobe" marker walk used to decide
// whether a CMYK JPEG's ink channels are inverted.
func TestJpegHasAdobeMarker(t *testing.T) {
	// SOI, APP14 "Adobe" (len 14), then SOS.
	withMarker := []byte{
		0xFF, 0xD8,
		0xFF, 0xEE, 0x00, 0x0E, 'A', 'd', 'o', 'b', 'e', 0, 0, 0, 0, 0, 0, 0,
		0xFF, 0xDA,
	}
	if !jpegHasAdobeMarker(withMarker) {
		t.Error("APP14 Adobe marker not detected")
	}

	// SOI, APP0 "JFIF" (no Adobe), then SOS.
	noMarker := []byte{
		0xFF, 0xD8,
		0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0,
		0xFF, 0xDA,
	}
	if jpegHasAdobeMarker(noMarker) {
		t.Error("false positive: no Adobe marker present")
	}

	// "Adobe" bytes inside entropy-coded data (after SOS) must not match.
	inScanData := []byte{0xFF, 0xD8, 0xFF, 0xDA, 'A', 'd', 'o', 'b', 'e'}
	if jpegHasAdobeMarker(inScanData) {
		t.Error("false positive: matched Adobe inside scan data")
	}

	if jpegHasAdobeMarker([]byte{0x00, 0x01}) {
		t.Error("non-JPEG should not match")
	}
}
