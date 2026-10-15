// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestReplaceTextLeavesNoRecoverableBytes pins a pre-existing leak found
// during an independent review of pdf-go-oq9i/pdf-go-1ha8: ReplaceText
// removes the OLD matched text via the same content-stream rewriter
// ApplyRedactions uses, then calls replacePageContents to point /Contents
// at a BRAND NEW stream — but, unlike ApplyRedactions, never called
// RemoveUnusedObjects afterward, so the OLD stream object (still holding
// the full, un-redacted original text) stayed in d.objects, unreferenced
// but still written to the saved file — exactly pdf-go-2qkq's class of
// leak, just reached through ReplaceText instead of ApplyRedactions.
func TestReplaceTextLeavesNoRecoverableBytes(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "REPLACE-LEAK-SECRET-998877"
	if err := page.AddText(secret, TextStyle{Font: FontHelvetica, Size: 12},
		Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 720}); err != nil {
		t.Fatal(err)
	}

	n, err := doc.ReplaceText(secret, "REDACTED")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 1 {
		t.Fatalf("ReplaceText: replaced %d, want 1", n)
	}

	if found, cnt := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ReplaceText", secret, cnt)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, cnt := documentContainsBytes(reopened, secret); found {
		t.Errorf("%q recoverable from %d object(s) in the saved+reopened file", secret, cnt)
	}
}
