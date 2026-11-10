// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"errors"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestConvertToPDF20MovesInfoToXMP(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	d.SetInfo(pdf.DocumentInfo{
		Title: "Title", Author: "Author", Subject: "Subject", Keywords: "k1, k2",
		Creator: "Creator", Producer: "Producer",
		CreationDate: "D:20261110120000Z", ModDate: "D:20261111120000Z",
		Custom: map[string]string{"CustomKey": "CustomValue"},
	})
	if err := d.ConvertToPDF20(); err != nil {
		t.Fatal(err)
	}
	data := savedBytes(t, d)
	if headerOf(data) != "%PDF-2.0" {
		t.Errorf("header %s", headerOf(data))
	}
	re, err := pdf.OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	info, _ := re.Info()
	if info.Title != "" || info.Author != "" || info.Subject != "" || info.Keywords != "" ||
		info.Creator != "" || info.Producer != "" || len(info.Custom) != 0 {
		t.Errorf("/Info keeps %+v", info)
	}
	if info.CreationDate != "D:20261110120000Z" || info.ModDate != "D:20261111120000Z" {
		t.Errorf("dates in /Info: %q %q", info.CreationDate, info.ModDate)
	}
	x, err := re.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if x.Title != "Title" || len(x.Authors) != 1 || x.Authors[0] != "Author" || x.Description != "Subject" ||
		len(x.Keywords) != 2 || x.CreatorTool != "Creator" || x.Producer != "Producer" {
		t.Errorf("XMP = %+v", x)
	}
	found := false
	for _, p := range x.Custom {
		if p.Name == "CustomKey" && p.Value == "CustomValue" {
			found = true
		}
	}
	if !found {
		t.Errorf("custom entry not in XMP: %+v", x.Custom)
	}
}

func TestConvertToPDF20AddsNoMetadataToABareDocument(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	if err := d.ConvertToPDF20(); err != nil {
		t.Fatal(err)
	}
	if raw, _ := d.XMPRaw(); len(raw) != 0 {
		t.Error("an XMP packet was added to a document with no metadata")
	}
	if d.Version() != "2.0" {
		t.Errorf("Version = %s", d.Version())
	}
}

func TestConvertToPDF20RefusesDeprecatedEncryption(t *testing.T) {
	for _, alg := range []pdf.EncryptionAlgorithm{pdf.EncryptionAlgRC4_40, pdf.EncryptionAlgRC4_128, pdf.EncryptionAlgAES128} {
		d := pdf.NewDocument(200, 200)
		d.SetInfo(pdf.DocumentInfo{Title: "T"})
		d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: alg})
		err := d.ConvertToPDF20()
		if !errors.Is(err, pdf.ErrDeprecatedFeature) {
			t.Errorf("algorithm %d: err = %v", alg, err)
		}
		if info, _ := d.Info(); info.Title != "T" || d.Version() != "1.4" {
			t.Errorf("algorithm %d: a refused conversion changed the document", alg)
		}
	}
	// An opened RC4 file too.
	d := pdf.NewDocument(200, 200)
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: pdf.EncryptionAlgRC4_128})
	opened, err := pdf.OpenStreamWithPassword(bytes.NewReader(savedBytes(t, d)), "u")
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.ConvertToPDF20(); !errors.Is(err, pdf.ErrDeprecatedFeature) {
		t.Errorf("opened RC4 file: err = %v", err)
	}
	// AES-256 is what PDF 2.0 keeps.
	d = pdf.NewDocument(200, 200)
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: pdf.EncryptionAlgAES256})
	if err := d.ConvertToPDF20(); err != nil {
		t.Errorf("AES-256: %v", err)
	}
}

func TestConvertToPDF20RefusesPDFA123(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	if _, err := d.ConvertToPDFA(pdf.PDFA2B); err != nil {
		t.Fatal(err)
	}
	if err := d.ConvertToPDF20(); err == nil {
		t.Error("a PDF/A-2 document was converted to PDF 2.0")
	}
	d = pdf.NewDocument(200, 200)
	if _, err := d.ConvertToPDFA(pdf.PDFA4); err != nil {
		t.Fatal(err)
	}
	if err := d.ConvertToPDF20(); err != nil {
		t.Errorf("PDF/A-4 refused: %v", err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(savedBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	if r := re.ValidatePDFA(pdf.PDFA4); !r.Conformant {
		t.Errorf("PDF/A-4 lost by the conversion: %+v", r.Issues)
	}
}
