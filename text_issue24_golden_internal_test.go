// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

// TestAddTextOrdinaryTextGolden pins the content stream AddText and
// AddTextAt write for plain ASCII text with zero-value options, so the
// line-breaking, invisible-character and validation changes of GitHub
// issue #24 provably leave ordinary text byte-identical.
func TestAddTextOrdinaryTextGolden(t *testing.T) {
	d := NewDocument(595, 842)
	b, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skip(err)
	}
	f, err := d.LoadFontFromStream(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	r := Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 800}
	if err := p.AddText("The quick brown fox jumps over the lazy dog. Pack my box\nwith five dozen liquor jugs.", TextStyle{Size: 11}, r); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Kerning AV To ffi office Wave, the quick brown fox.", TextStyle{Font: f, Size: 10, HAlign: HAlignCenter, Underline: true}, r); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Hello, world", TextStyle{Size: 9}, 60, 500); err != nil {
		t.Fatal(err)
	}
	got, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	const want = "q\n\n\nQ\n\n\nq\nq\n50 600 150 200 re W n\nBT\n/F0 11 Tf\n0 0 0 rg\n50 791.2 Td\n(The quick brown fox jumps) Tj\n0 -13.2 Td\n(over the lazy dog. Pack my) Tj\n0 -13.2 Td\n(box) Tj\n0 -13.2 Td\n(with five dozen liquor jugs.) Tj\nET\nQ\nQ\n\n\nq\nq\n50 600 150 200 re W n\nBT\n/F1 10 Tf\n0 0 0 rg\n52.429199 790.717773 Td\n[<002E> 50.042969 <00480055> 17.210938 <0051004C0051004A00030024> 64.835938 <003900030037> 170.128906 <0052000313B40003005213B4> 1.03125 <004600480003003A> 64.285156 <004400590048000F>] TJ\n21.569824 -12 Td\n[<0057004B0048000300540058004C0046004E00030045> 1.085938 <0055> 21.839844 <0052005A0051000300490052> 31.546875 <005B0011>] TJ\nET\n0 0 0 rg\n52.429199 789.717773 145.141602 0.5 re f\n73.999023 777.717773 102.001953 0.5 re f\nQ\nQ\n\n\nq\nBT\n/F0 9 Tf\n0 0 0 rg\n60 500 Td\n(Hello, world) Tj\nET\nQ\n\n"
	if string(got) != want {
		t.Errorf("content stream changed:\n got %q\nwant %q", got, want)
	}
}
