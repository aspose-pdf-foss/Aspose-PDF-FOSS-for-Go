// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestHeaderVersion(t *testing.T) {
	for in, want := range map[string]pdfVersion{
		"%PDF-1.4\n":            14,
		"%PDF-2.0\r\n":          20,
		"%PDF-1.0 ":             10,
		"junk\n%PDF-1.7\n":      17, // a reader looks in the first 1024 bytes
		"%PDF-1.10\n":           0,
		"%PDF-1.8\n":            0,
		"%PDF-3.0\n":            0,
		"%PDF-":                 0,
		"no header at all here": 0,
	} {
		if got := headerVersion([]byte(in)); got != want {
			t.Errorf("headerVersion(%q) = %d, want %d", in, got, want)
		}
	}
	far := make([]byte, 1100)
	copy(far[1090:], "%PDF-1.7\n")
	if v := headerVersion(far); v != 0 {
		t.Errorf("header past 1024 bytes read as %d", v)
	}
	if h := pdfVersion(20).header(); h != "%PDF-2.0\n" {
		t.Errorf("header() = %q", h)
	}
}
