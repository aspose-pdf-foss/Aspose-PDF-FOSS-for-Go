// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// makeTextDoc builds a one-page document with a single line of text.
func makeTextDoc(t *testing.T, text string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	p, _ := doc.Page(1)
	if err := p.AddText(text, pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 20, LLY: 150, URX: 380, URY: 185}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	return doc
}

func extractFirst(t *testing.T, doc *pdf.Document) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	txt, err := out.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	return txt[0]
}

func TestReplaceTextBasic(t *testing.T) {
	doc := makeTextDoc(t, "Invoice 2023 Total")
	n, err := doc.ReplaceText("2023", "2026")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 1 {
		t.Errorf("replaced %d, want 1", n)
	}
	got := extractFirst(t, doc)
	if !strings.Contains(got, "2026") {
		t.Errorf("extracted %q, want it to contain 2026", got)
	}
	if strings.Contains(got, "2023") {
		t.Errorf("extracted %q, still contains the old 2023", got)
	}
	// Visual order preserved: replacement sits between the surrounding words.
	if i, j := strings.Index(got, "Invoice"), strings.Index(got, "2026"); i < 0 || j < 0 || i > j {
		t.Errorf("extracted %q, want Invoice before 2026 (order preserved)", got)
	}
}

func TestReplaceTextCount(t *testing.T) {
	doc := makeTextDoc(t, "a X b X c X")
	n, err := doc.ReplaceText("X", "Y")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 3 {
		t.Errorf("replaced %d, want 3", n)
	}
	got := extractFirst(t, doc)
	if strings.Contains(got, "X") {
		t.Errorf("extracted %q, still contains X", got)
	}
	if c := strings.Count(got, "Y"); c != 3 {
		t.Errorf("extracted %q has %d Ys, want 3", got, c)
	}
}

func TestReplaceTextCaseInsensitive(t *testing.T) {
	doc := makeTextDoc(t, "Hello WORLD")
	n, err := doc.ReplaceText("world", "Earth", pdf.ReplaceOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 1 {
		t.Errorf("replaced %d, want 1", n)
	}
	if got := extractFirst(t, doc); !strings.Contains(got, "Earth") {
		t.Errorf("extracted %q, want Earth", got)
	}
}

func TestReplaceTextRegex(t *testing.T) {
	doc := makeTextDoc(t, "ID 12345 ref")
	n, err := doc.ReplaceText(`\d+`, "#####", pdf.ReplaceOptions{Regex: true})
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 1 {
		t.Errorf("replaced %d, want 1", n)
	}
	got := extractFirst(t, doc)
	if strings.Contains(got, "12345") {
		t.Errorf("extracted %q, still contains the digits", got)
	}
	if !strings.Contains(got, "#####") {
		t.Errorf("extracted %q, want #####", got)
	}
}

func TestReplaceTextDelete(t *testing.T) {
	doc := makeTextDoc(t, "keep DELETE keep")
	n, err := doc.ReplaceText("DELETE", "")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 1 {
		t.Errorf("replaced %d, want 1", n)
	}
	if got := extractFirst(t, doc); strings.Contains(got, "DELETE") {
		t.Errorf("extracted %q, DELETE was not removed", got)
	}
}

func TestReplaceTextNotFound(t *testing.T) {
	doc := makeTextDoc(t, "nothing to see")
	n, err := doc.ReplaceText("absent", "x")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if n != 0 {
		t.Errorf("replaced %d, want 0", n)
	}
}

func TestReplaceTextBadRegex(t *testing.T) {
	doc := makeTextDoc(t, "text")
	if _, err := doc.ReplaceText("(", "x", pdf.ReplaceOptions{Regex: true}); err == nil {
		t.Error("ReplaceText with invalid regex = nil error, want an error")
	}
}
