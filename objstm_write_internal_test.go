// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strconv"
	"strings"
	"testing"
)

// The object stream must read back through the same parsing the reader uses:
// a header of "num offset" pairs, then each body at /First + offset.
func TestBuildObjectStream(t *testing.T) {
	objs := []packedObj{
		{num: 3, body: []byte("<</Type /Annot>>")},
		{num: 7, body: []byte("[1 2 3]")},
		{num: 9, body: []byte("(hello)")},
	}
	data, first := buildObjectStream(objs)

	header := strings.Fields(string(data[:first]))
	if len(header) != 6 {
		t.Fatalf("header has %d fields, want 6: %q", len(header), data[:first])
	}
	for i, o := range objs {
		num, _ := strconv.Atoi(header[2*i])
		off, _ := strconv.Atoi(header[2*i+1])
		if num != o.num {
			t.Errorf("entry %d names object %d, want %d", i, num, o.num)
		}
		v, err := parseValue(newLexer(data[first+off:]))
		if err != nil {
			t.Fatalf("object %d does not parse at its offset: %v", o.num, err)
		}
		switch o.num {
		case 3:
			d, ok := v.(pdfDict)
			if !ok || d["/Type"] != pdfName("/Annot") {
				t.Errorf("object 3 parsed as %#v", v)
			}
		case 7:
			if a, ok := v.(pdfArray); !ok || len(a) != 3 {
				t.Errorf("object 7 parsed as %#v", v)
			}
		case 9:
			if str, ok := v.(string); !ok || str != "hello" {
				t.Errorf("object 9 parsed as %#v", v)
			}
		}
	}
}

// Rows are big-endian with widths [1 w2 w3], each width the smallest that
// holds the largest value in its column.
func TestEncodeXRefEntries(t *testing.T) {
	rows := []xrefStreamEntry{
		{typ: 0, f2: 0, f3: 65535},
		{typ: 1, f2: 70000, f3: 0},
		{typ: 2, f2: 12, f3: 5},
	}
	data, w := encodeXRefEntries(rows)
	if len(w) != 3 || w[0] != 1 || w[1] != 3 || w[2] != 2 {
		t.Fatalf("/W = %v, want [1 3 2]", w)
	}
	if len(data) != 3*(1+3+2) {
		t.Fatalf("%d bytes, want %d", len(data), 3*6)
	}
	for i, r := range rows {
		row := data[i*6:]
		if int(row[0]) != r.typ {
			t.Errorf("row %d type = %d, want %d", i, row[0], r.typ)
		}
		if got := readField(row, 1, 3); int64(got) != r.f2 {
			t.Errorf("row %d field 2 = %d, want %d", i, got, r.f2)
		}
		if got := readField(row, 4, 2); got != r.f3 {
			t.Errorf("row %d field 3 = %d, want %d", i, got, r.f3)
		}
	}
}
