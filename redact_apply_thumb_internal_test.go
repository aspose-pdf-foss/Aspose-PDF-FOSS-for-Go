// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// pdf-go-ekp4: a page's /Thumb entry (ISO 32000-1 §7.7.3.3) is an
// independent miniature rendering of the page, reached through its own
// page-dict key — never through /Contents, so none of ApplyRedactions'
// content-stream rewriters ever see it. If it happens to be a thumbnail
// of the PRE-redaction page (the common case for a producer that
// generates one when the page is first authored), it could otherwise
// keep showing the redacted content after everything else was correctly
// rewritten — the same class of leak as pdf-go-2qkq, reached through a
// different, independent object nothing else in this pipeline touches.
// ApplyRedactions now drops the reference outright (applyRedactionsToPage
// step 5a) rather than regenerating it, since a thumbnail is purely
// decorative and this library's own read path never consults one.

// TestApplyRedactionsDropsPageThumb confirms the /Thumb key is removed
// from a redacted page's dict, and that the thumbnail stream's own bytes
// are unrecoverable both in memory and in the saved+reopened file — the
// forensic check this whole fix chain uses throughout, since a key
// merely becoming unresolvable is not the same as the bytes actually
// being gone (RemoveUnusedObjects, which ApplyRedactions already calls,
// is what actually removes them once nothing references the object).
func TestApplyRedactionsDropsPageThumb(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	const marker = "THUMBNAIL-PRE-REDACTION-MARKER"
	thumbID := d.nextID
	d.nextID++
	d.objects[thumbID] = &pdfObject{Num: thumbID, Value: &pdfStream{
		Dict: pdfDict{
			"/Width":            10.0,
			"/Height":           10.0,
			"/ColorSpace":       pdfName("/DeviceGray"),
			"/BitsPerComponent": 8.0,
		},
		Data:    []byte(marker),
		Decoded: true,
	}}
	p.pageDict()["/Thumb"] = pdfRef{Num: thumbID}

	ra := NewRedactAnnotation(p, Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if _, present := p.pageDict()["/Thumb"]; present {
		t.Error("page dict still has a /Thumb entry after ApplyRedactions")
	}
	if found, n := documentContainsBytes(d, marker); found {
		t.Errorf("thumbnail marker still present in %d in-memory object(s) after ApplyRedactions", n)
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
		t.Errorf("thumbnail marker recoverable from %d object(s) in the saved+reopened file", n)
	}
}

// TestApplyRedactionsDropsPageThumbEvenWithoutOwnRegionOverlap confirms
// the /Thumb removal is unconditional once a page has ANY redact
// region — a thumbnail renders the WHOLE page, so there is no
// "region doesn't overlap the thumbnail" case to spare it from, unlike
// every other content path this feature touches.
func TestApplyRedactionsDropsPageThumbEvenWithoutOwnRegionOverlap(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	thumbID := d.nextID
	d.nextID++
	d.objects[thumbID] = &pdfObject{Num: thumbID, Value: &pdfStream{
		Dict: pdfDict{
			"/Width": 4.0, "/Height": 4.0,
			"/ColorSpace": pdfName("/DeviceGray"), "/BitsPerComponent": 8.0,
		},
		Data:    []byte{0, 0, 0, 0},
		Decoded: true,
	}}
	p.pageDict()["/Thumb"] = pdfRef{Num: thumbID}

	// A region tucked in a corner far from anything meaningful — the
	// point is only that the page HAS a redact region at all.
	ra := NewRedactAnnotation(p, Rectangle{LLX: 1, LLY: 1, URX: 5, URY: 5})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if _, present := p.pageDict()["/Thumb"]; present {
		t.Error("page dict still has a /Thumb entry after ApplyRedactions")
	}
}
