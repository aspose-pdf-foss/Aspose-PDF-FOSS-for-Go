// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

// jpegWithComment encodes a tiny solid-color JPEG and splices a COM marker
// (0xFF 0xFE, ISO/IEC 10918-1) carrying marker right after the SOI, so the
// resulting bytes are still a valid JPEG (parseJPEGHeader skips markers it
// doesn't recognize by their declared length, same as any real JPEG
// decoder) but also carry a literal, grep-able payload: a forensic stand-in
// for "this image's actual pixel data", since DCTDecode images are stored
// verbatim (image_add.go's createJPEGXObject, Decoded: false) rather than
// re-encoded, so whatever bytes go in come back out byte-for-byte in the
// saved file unless the XObject itself is actually removed.
func jpegWithComment(t *testing.T, marker string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Fatal("encoded image is not a JPEG")
	}
	seg := []byte(marker)
	length := len(seg) + 2 // includes the 2 length bytes themselves
	com := []byte{0xFF, 0xFE, byte(length >> 8), byte(length)}
	com = append(com, seg...)
	out := make([]byte, 0, len(raw)+len(com))
	out = append(out, raw[:2]...) // SOI
	out = append(out, com...)
	out = append(out, raw[2:]...)
	return out
}

// TestApplyRedactionsRemovesFullyCoveredImageBytes pins pdf-go-ye1z: an
// Image XObject whose placement falls entirely inside a redaction region
// has its Do invocation dropped from the content stream
// (rewriteXObjectDoOperatorsInStream's dropDo path), but until this was
// verified, the page's /Resources/XObject entry for that image was never
// confirmed to actually go away -- it stayed graph-reachable
// (page -> Resources -> XObject -> image stream) and RemoveUnusedObjects'
// ordinary reachability walk would never collect it, so the image's pixel
// data survived in the saved file even though nothing in the content
// stream invokes it any more.
//
// In practice this already closes on its own: rewriteXObjectDoOperatorsInStream's
// resource-pruning pass (neededXObjectNames, added for the unrelated
// pdf-go-w5hl Form-XObject fix) computes which /XObject names the FINAL
// rewritten content still invokes via ANY Do operator -- Image or Form
// alike, it does not distinguish -- and deletes every other name from the
// page's own (already-privatized) /Resources/XObject dict. A dropped
// Image Do leaves no surviving Do for that name, so it is pruned the same
// way a redirected Form XObject's now-stale original name is; once pruned,
// ApplyRedactions' own trailing RemoveUnusedObjects call reclaims the now
// cleanly-unreachable image stream object. This test exists to pin that
// behavior with a direct, forensic (byte-level, not ExtractText-trusting)
// check specific to image pixel data, and to catch a regression if the
// pruning logic above is ever narrowed to Form XObjects only.
func TestApplyRedactionsRemovesFullyCoveredImageBytes(t *testing.T) {
	const marker = "YE1Z-IMAGE-MARKER-9f3c"
	jpg := jpegWithComment(t, marker)

	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	imgRect := Rectangle{LLX: 50, LLY: 650, URX: 150, URY: 750}
	if err := p.AddImageFromStream(bytes.NewReader(jpg), imgRect); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("visible", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 620}); err != nil {
		t.Fatal(err)
	}

	// Redact region strictly covers the image's placement rect.
	ra := NewRedactAnnotation(p, Rectangle{LLX: 40, LLY: 640, URX: 160, URY: 760})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	infosBefore, err := p.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infosBefore) != 1 {
		t.Fatalf("before redaction: got %d images, want 1", len(infosBefore))
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	if found, n := documentContainsBytes(d, marker); found {
		t.Errorf("image marker %q still present in %d in-memory object(s) after ApplyRedactions", marker, n)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(reopened, marker); found {
		t.Errorf("image marker %q recoverable from %d object(s) in the saved+reopened file", marker, n)
	}

	p2, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	infosAfter, err := p2.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infosAfter) != 0 {
		t.Errorf("after redaction+reopen: got %d images on the page, want 0", len(infosAfter))
	}
	txt, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "visible") {
		t.Errorf("unrelated text was lost: got %q", txt)
	}
}
