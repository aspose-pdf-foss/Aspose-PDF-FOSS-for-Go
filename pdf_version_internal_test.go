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

func TestVersionFromCatalog(t *testing.T) {
	d := NewDocument(200, 200)
	d.catalog = pdfDict{"/Version": pdfName("/2.0")}
	if v := d.Version(); v != "2.0" {
		t.Errorf("Version with catalog /Version 2.0 = %s", v)
	}
	d.catalog["/Version"] = pdfName("/1.3") // lower than the header: the header wins
	d.version = 17
	if v := d.Version(); v != "1.7" {
		t.Errorf("Version = %s, want 1.7", v)
	}
	if err := d.SetVersion("1.5"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.catalog["/Version"]; ok {
		t.Error("SetVersion left the catalog /Version")
	}
}

func TestDocumentIDHex(t *testing.T) {
	d := NewDocument(200, 200)
	d.docID = pdfArray{pdfHexString{0x31, 0xc7, 0xa8}, "\x00\xff"}
	id, ok := d.ID()
	if !ok || id.Original != "31C7A8" || id.Modified != "00FF" {
		t.Errorf("ID = %+v, %v", id, ok)
	}
	d.docID = pdfArray{pdfHexString{1}, 5}
	if _, ok := d.ID(); ok {
		t.Error("a malformed /ID was reported")
	}
}
