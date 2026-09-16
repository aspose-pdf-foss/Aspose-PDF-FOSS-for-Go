// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// twoColumnPage lays three rows of two columns at fixed x positions and
// returns the reopened document, so extraction sees a written file.
func twoColumnPage(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}
	rows := []struct{ left, right string }{
		{"Apples", "12"},
		{"Pears", "7"},
		{"Plums", "153"},
	}
	for i, r := range rows {
		y := 150.0 - float64(i)*20
		if err := page.AddText(r.left, style, pdf.Rectangle{LLX: 20, LLY: y, URX: 180, URY: y + 16}); err != nil {
			t.Fatal(err)
		}
		if err := page.AddText(r.right, style, pdf.Rectangle{LLX: 240, LLY: y, URX: 380, URY: y + 16}); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return reopened
}

// Reading order collapses the gap between columns to one space; the flattened
// mode keeps it, which is the whole point of the mode.
func TestExtractTextFlattenKeepsHorizontalGaps(t *testing.T) {
	doc := twoColumnPage(t)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	reading, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reading, "Apples 12") {
		t.Fatalf("reading order = %q, want the columns joined by one space", reading)
	}

	flat, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractFlatten})
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(flat, "\n", 2)[0]
	if !strings.HasPrefix(first, "Apples") || !strings.HasSuffix(strings.TrimRight(first, " "), "12") {
		t.Fatalf("flattened first line = %q, want it to start with Apples and end with 12", first)
	}
	if n := strings.Count(first, " "); n < 10 {
		t.Errorf("flattened first line = %q, want the column gap preserved as spaces (got %d)", first, n)
	}
}

// The columns have to line up: that is what makes the output readable in a
// monospace font and diffable between revisions.
func TestExtractTextFlattenAlignsColumns(t *testing.T) {
	doc := twoColumnPage(t)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractFlatten})
	if err != nil {
		t.Fatal(err)
	}

	var cols []int
	for _, line := range strings.Split(flat, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		for _, want := range []string{"12", "7", "153"} {
			if i := strings.Index(line, want); i > 0 {
				cols = append(cols, i)
				break
			}
		}
	}
	if len(cols) != 3 {
		t.Fatalf("found %d value columns in %q, want 3", len(cols), flat)
	}
	for i, c := range cols {
		if c != cols[0] {
			t.Errorf("row %d starts its value at column %d, row 0 at %d:\n%s", i, c, cols[0], flat)
		}
	}
}

// The mode must not disturb the other two.
func TestExtractTextModesStayDistinct(t *testing.T) {
	doc := twoColumnPage(t)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	reading, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractReading})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractRaw})
	if err != nil {
		t.Fatal(err)
	}
	flat, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractFlatten})
	if err != nil {
		t.Fatal(err)
	}
	if flat == reading {
		t.Error("flattened output is identical to reading order")
	}
	// Every mode must carry the same words, whatever the spacing.
	for _, word := range []string{"Apples", "Pears", "Plums", "12", "7", "153"} {
		for name, text := range map[string]string{"reading": reading, "raw": raw, "flatten": flat} {
			if !strings.Contains(text, word) {
				t.Errorf("%s mode lost %q", name, word)
			}
		}
	}
}

// An empty page yields an empty string rather than a grid of spaces.
func TestExtractTextFlattenEmptyPage(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := page.ExtractText(pdf.TextExtractOptions{Mode: pdf.TextExtractFlatten})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(flat) != "" {
		t.Errorf("empty page flattened to %q", flat)
	}
}
