// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// t1Encrypt is the inverse of t1Decrypt: it prepends `skip` zero bytes and
// encrypts, so t1Decrypt(…, skip) recovers plain.
func t1Encrypt(plain []byte, r uint16, skip int) []byte {
	const c1, c2 = 52845, 22719
	full := append(make([]byte, skip), plain...)
	out := make([]byte, len(full))
	for i, p := range full {
		c := p ^ byte(r>>8)
		out[i] = c
		r = (uint16(c)+r)*c1 + c2
	}
	return out
}

// buildMinimalType1 assembles a tiny but structurally complete Type1 program
// with one glyph "A" (a triangle), used to exercise the parser, the eexec /
// charstring decryption, and the charstring interpreter end to end.
func buildMinimalType1() []byte {
	// Charstring for "A": hsbw 0 200; rmoveto 50 0; rlineto 50 100;
	// rlineto -100 0; closepath; endchar. Numbers use the Type1 encoding.
	cs := []byte{
		139, 247, 92, 13, // hsbw 0 200
		189, 139, 21, // rmoveto 50 0
		189, 239, 5, // rlineto 50 100
		39, 139, 5, // rlineto -100 0
		9,  // closepath
		14, // endchar
	}
	notdef := []byte{139, 139, 13, 14} // hsbw 0 0; endchar

	encCS := t1Encrypt(cs, 4330, 4)
	encNotdef := t1Encrypt(notdef, 4330, 4)

	var priv bytes.Buffer
	priv.WriteString("dup /Private 1 dict dup begin\n/lenIV 4 def\n")
	priv.WriteString("2 index /CharStrings 2 dict dup begin\n")
	priv.WriteString("/.notdef ")
	priv.WriteString(itoa(len(encNotdef)))
	priv.WriteString(" RD ")
	priv.Write(encNotdef)
	priv.WriteString(" ND\n/A ")
	priv.WriteString(itoa(len(encCS)))
	priv.WriteString(" RD ")
	priv.Write(encCS)
	priv.WriteString(" ND\nend\nend\n")

	eexec := t1Encrypt(priv.Bytes(), 55665, 4)

	var out bytes.Buffer
	out.WriteString("%!PS-AdobeFont-1.0: Test 1.0\n")
	out.WriteString("/FontMatrix [0.001 0 0 0.001 0 0] readonly def\n")
	out.WriteString("/Encoding StandardEncoding def\n")
	out.WriteString("currentfile eexec\n")
	out.Write(eexec)
	out.WriteString("\n0000000000000000\ncleartomark\n")
	return out.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestParseType1MinimalGlyph(t *testing.T) {
	f := parseType1(buildMinimalType1(), 0, 0)
	if f == nil {
		t.Fatal("parseType1 returned nil")
	}
	if f.unitsPerEm != 1000 {
		t.Errorf("unitsPerEm = %v, want 1000", f.unitsPerEm)
	}
	gid, ok := f.nameToGID["A"]
	if !ok {
		t.Fatalf("glyph A not found; names = %v", f.glyphNames)
	}
	// StandardEncoding code for "A" is 65; built-in encoding should map it.
	if f.builtinEnc[65] != "A" {
		t.Errorf("builtinEnc[65] = %q, want A", f.builtinEnc[65])
	}
	contours := f.glyphContours(gid)
	if len(contours) != 1 {
		t.Fatalf("glyph A contours = %d, want 1", len(contours))
	}
	// Triangle vertices: (50,0) (100,100) (0,100).
	pts := contours[0]
	if len(pts) != 3 {
		t.Fatalf("triangle points = %d, want 3", len(pts))
	}
	want := [][2]float64{{50, 0}, {100, 100}, {0, 100}}
	for i, w := range want {
		if pts[i].x != w[0] || pts[i].y != w[1] {
			t.Errorf("point %d = (%v,%v), want (%v,%v)", i, pts[i].x, pts[i].y, w[0], w[1])
		}
	}
}

// TestType1DecryptRoundTrip checks the eexec cipher round-trips.
func TestType1DecryptRoundTrip(t *testing.T) {
	plain := []byte("hello type1 charstring data")
	enc := t1Encrypt(plain, 55665, 4)
	got := t1Decrypt(enc, 55665, 4)
	if !bytes.Equal(got, plain) {
		t.Errorf("round trip = %q, want %q", got, plain)
	}
}
