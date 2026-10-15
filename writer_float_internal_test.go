// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math"
	"regexp"
	"strings"
	"testing"
)

// exponentNumberRe matches a number written in exponent notation, e.g.
// "-1e-13" or "2.5E+07" -- distinct from an ordinary word like "Rect"
// merely containing the letter e.
var exponentNumberRe = regexp.MustCompile(`[0-9][eE][-+]?[0-9]`)

// TestWriteValueNeverUsesExponentNotation pins pdf-go-mxhk (GitHub issue
// #16 point 4, shadowhunter1470-svg): writeValue used to format dict/array
// float64 values with strconv.FormatFloat(val, 'g', -1, 64), which
// switches to exponent notation for small-magnitude numbers -- a tiny
// floating-point rounding remainder from a computed layout, easy to
// produce by accident, is written as e.g. "-1e-13". ISO 32000-1 §7.3.3
// defines a PDF "real" number as an optional sign, digits, and an
// optional decimal point -- no exponent form at all, so a strict reader
// may reject or misparse such a value. The reporter's own repro:
// 56.7 - 56.7000000000001 as a link annotation's /Rect LLX.
func TestWriteValueNeverUsesExponentNotation(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	x := 56.7 - 56.7000000000001 // -1e-13
	if math.Abs(x) >= 1e-9 {
		t.Fatalf("sanity check failed: x = %v is not the expected tiny rounding remainder", x)
	}
	link := NewLinkAnnotation(p, Rectangle{LLX: x, LLY: 10, URX: 50, URY: 30})
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "/Rect") && exponentNumberRe.MatchString(line) {
			t.Errorf("dict value written in exponent notation: %q", strings.TrimSpace(line))
		}
	}

	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	coll := p2.Annotations()
	if coll.Count() != 1 {
		t.Fatalf("got %d annotations after round-trip, want 1", coll.Count())
	}
	got := coll.At(0).Rect()
	if math.Abs(got.LLX-x) > 1e-6 {
		t.Errorf("LLX round-tripped as %v, want ~%v", got.LLX, x)
	}
}

// TestWriteValueRefusesNonFiniteFloat pins the NaN/±Inf half of
// pdf-go-mxhk: strconv.FormatFloat renders both as non-numeric text
// ("NaN", "+Inf") that no PDF reader can parse as a number at all, so
// writeValue must refuse rather than write garbage.
func TestWriteValueRefusesNonFiniteFloat(t *testing.T) {
	var buf bytes.Buffer
	if err := writeValue(&buf, math.NaN(), nil, nil); err == nil {
		t.Error("writeValue accepted NaN")
	}
	if err := writeValue(&buf, math.Inf(1), nil, nil); err == nil {
		t.Error("writeValue accepted +Inf")
	}
	if err := writeValue(&buf, math.Inf(-1), nil, nil); err == nil {
		t.Error("writeValue accepted -Inf")
	}
}
