// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// documentContainsBytes reports whether any object in d.objects -- decoded
// stream data, or a literal string value -- contains want, and how many
// objects do. Mirrors the verification method pdf-go-2qkq's reporter used
// (decode every stream object in the saved file and search for the
// redacted string) rather than trusting that the visible page text no
// longer shows it, which is exactly what hid this bug in the first place.
func documentContainsBytes(d *Document, want string) (bool, int) {
	wb := []byte(want)
	found := 0
	for _, obj := range d.objects {
		switch v := obj.Value.(type) {
		case *pdfStream:
			if bytes.Contains(decodedStreamData(v), wb) {
				found++
			}
			if dictContainsString(v.Dict, want) {
				found++
			}
		case pdfDict:
			if dictContainsString(v, want) {
				found++
			}
		case pdfArray:
			if arrayContainsString(v, want) {
				found++
			}
		case pdfHexString, pdfRaw:
			if valueContainsString(v, want) {
				found++
			}
		}
	}
	return found > 0, found
}

// valueContainsString checks the handful of pdfValue shapes that can carry
// a literal byte sequence directly (not nested in a dict/array): plain Go
// strings, and the two []byte-based named types (pdfHexString, pdfRaw).
func valueContainsString(v pdfValue, want string) bool {
	switch vv := v.(type) {
	case string:
		return strings.Contains(vv, want)
	case pdfHexString:
		return bytes.Contains([]byte(vv), []byte(want))
	case pdfRaw:
		return bytes.Contains([]byte(vv), []byte(want))
	}
	return false
}

func dictContainsString(d pdfDict, want string) bool {
	for _, v := range d {
		if valueContainsString(v, want) {
			return true
		}
		switch vv := v.(type) {
		case pdfDict:
			if dictContainsString(vv, want) {
				return true
			}
		case pdfArray:
			if arrayContainsString(vv, want) {
				return true
			}
		}
	}
	return false
}

func arrayContainsString(a pdfArray, want string) bool {
	for _, v := range a {
		if valueContainsString(v, want) {
			return true
		}
		switch vv := v.(type) {
		case pdfDict:
			if dictContainsString(vv, want) {
				return true
			}
		case pdfArray:
			if arrayContainsString(vv, want) {
				return true
			}
		}
	}
	return false
}

// TestApplyRedactionsLeavesNoRecoverableBytes is Reproduction 1 from
// pdf-go-2qkq's private report: several AddText calls before redaction
// leave dead content-stream copies (appendToContentStream, pdf-go-qgrn)
// that still carry the secret in full, in addition to the stream
// replacePageContents itself abandons when it points /Contents at the
// rewritten (already-redacted) stream. Before the fix, both kinds of
// object survived into the saved file, fully recoverable; this asserts
// neither does, by decoding every object rather than trusting
// ExtractText.
func TestApplyRedactionsLeavesNoRecoverableBytes(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "SECRET4711"
	if err := p.AddText(secret, TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("visible", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 620}); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(p, Rectangle{LLX: 40, LLY: 690, URX: 310, URY: 725})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	if found, n := documentContainsBytes(d, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions", secret, n)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(reopened, secret); found {
		t.Errorf("%q recoverable from %d object(s) in the saved+reopened file", secret, n)
	}

	p2, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt, secret) {
		t.Errorf("page text still contains %q", secret)
	}
	if !strings.Contains(txt, "visible") {
		t.Errorf("unrelated text was lost: got %q", txt)
	}
}

// TestApplyRedactionsOnOpenedDocumentLeavesNoRecoverableBytes is
// Reproduction 2 from pdf-go-2qkq's private report: an existing,
// already-clean (no dead objects) PDF opened from disk, redacted via a
// SearchText match rather than a hand-built rectangle. The original
// content stream replacePageContents abandons is the only leak path
// here (there are no pre-existing dead AddText copies), but it is the
// core of the vulnerability: the page's *original* /Contents object,
// still holding the secret verbatim, must not survive into the saved
// file.
func TestApplyRedactionsOnOpenedDocumentLeavesNoRecoverableBytes(t *testing.T) {
	const secret = "SECRET4711"
	src := NewDocumentFromFormat(PageFormatA4)
	p, err := src.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText(secret+" visible", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720}); err != nil {
		t.Fatal(err)
	}
	src.RemoveUnusedObjects() // start from a clean file, as the report does

	var srcBuf bytes.Buffer
	if _, err := src.WriteTo(&srcBuf); err != nil {
		t.Fatal(err)
	}

	d, err := OpenStream(bytes.NewReader(srcBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := p2.SearchText(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText(%q) = %d matches, want 1", secret, len(matches))
	}
	ra := NewRedactAnnotation(p2, matches[0].Rect)
	if err := p2.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	if found, n := documentContainsBytes(d, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions", secret, n)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(reopened, secret); found {
		t.Errorf("%q recoverable from %d object(s) in the saved+reopened file", secret, n)
	}
}
