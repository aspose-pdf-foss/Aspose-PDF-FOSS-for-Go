// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func newObj(d *Document, v pdfValue) pdfRef {
	id := d.nextID
	d.nextID++
	d.objects[id] = &pdfObject{Num: id, Value: v}
	return pdfRef{Num: id}
}

func TestConvertToPDF20RemovesDeprecated(t *testing.T) {
	d := NewDocument(200, 200)
	d.catalog = pdfDict{}
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/ProcSet": pdfArray{pdfName("/PDF"), pdfName("/Text")}}
	fd := newObj(d, pdfDict{"/Type": pdfName("/FontDescriptor"), "/CharSet": "(/a/b)", "/CIDSet": pdfRef{Num: 999}, "/Flags": 4})
	sha1Sig := newObj(d, pdfDict{"/Type": pdfName("/Sig"), "/SubFilter": pdfName("/adbe.pkcs7.sha1")})
	rsaSig := newObj(d, pdfDict{"/Type": pdfName("/Sig"), "/SubFilter": pdfName("/adbe.x509.rsa_sha1")})
	goodSig := newObj(d, pdfDict{"/Type": pdfName("/Sig"), "/SubFilter": pdfName("/adbe.pkcs7.detached")})
	f1 := newObj(d, pdfDict{"/FT": pdfName("/Sig"), "/T": "s1", "/V": sha1Sig})
	f2 := newObj(d, pdfDict{"/FT": pdfName("/Sig"), "/T": "s2", "/V": rsaSig})
	f3 := newObj(d, pdfDict{"/FT": pdfName("/Sig"), "/T": "s3", "/V": goodSig})
	d.catalog["/MarkInfo"] = pdfDict{"/Marked": true, "/Suspects": true}
	d.catalog["/Perms"] = pdfDict{"/UR3": goodSig, "/UR": goodSig, "/DocMDP": sha1Sig}

	if err := d.ConvertToPDF20(); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.pageDict()["/Resources"].(pdfDict)["/ProcSet"]; ok {
		t.Error("/ProcSet kept")
	}
	fdd := d.objects[fd.Num].Value.(pdfDict)
	if _, ok := fdd["/CharSet"]; ok {
		t.Error("/CharSet kept")
	}
	if _, ok := fdd["/CIDSet"]; ok {
		t.Error("/CIDSet kept")
	}
	if fdd["/Flags"] != 4 {
		t.Error("the rest of the font descriptor changed")
	}
	for _, f := range []pdfRef{f1, f2} {
		if _, ok := d.objects[f.Num].Value.(pdfDict)["/V"]; ok {
			t.Errorf("SHA-1 signature in field %d kept", f.Num)
		}
	}
	if _, ok := d.objects[f3.Num].Value.(pdfDict)["/V"]; !ok {
		t.Error("a SHA-256 signature was removed")
	}
	mark := d.catalog["/MarkInfo"].(pdfDict)
	if _, ok := mark["/Suspects"]; ok || mark["/Marked"] != true {
		t.Errorf("/MarkInfo = %v", mark)
	}
	if _, ok := d.catalog["/Perms"]; ok {
		t.Errorf("/Perms = %v, want it gone (usage rights and a DocMDP naming a removed signature)", d.catalog["/Perms"])
	}
	if d.Version() != "2.0" {
		t.Errorf("Version = %s", d.Version())
	}
}

func TestConvertToPDF20KeepsDocMDPOfAKeptSignature(t *testing.T) {
	d := NewDocument(200, 200)
	sig := newObj(d, pdfDict{"/Type": pdfName("/Sig"), "/SubFilter": pdfName("/ETSI.CAdES.detached")})
	newObj(d, pdfDict{"/FT": pdfName("/Sig"), "/V": sig})
	d.catalog = pdfDict{"/Perms": pdfDict{"/DocMDP": sig, "/UR3": sig}, "/Version": pdfName("/1.7")}
	if err := d.ConvertToPDF20(); err != nil {
		t.Fatal(err)
	}
	perms, _ := d.catalog["/Perms"].(pdfDict)
	if perms["/DocMDP"] != sig {
		t.Errorf("/Perms = %v, want the DocMDP kept", perms)
	}
	if _, ok := perms["/UR3"]; ok {
		t.Error("/UR3 kept")
	}
	if _, ok := d.catalog["/Version"]; ok {
		t.Error("catalog /Version kept")
	}
}

func TestConvertToPDF20KeepsExtensionSchemas(t *testing.T) {
	d := NewDocument(200, 200)
	if err := d.SetXMP(XMPMetadata{Title: "T"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := d.XMPRaw()
	block := `<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/"><pdfaExtension:schemas><rdf:Bag/></pdfaExtension:schemas></rdf:Description>`
	if err := d.SetXMPRaw([]byte(insertXMPDescriptions(string(raw), []string{block}))); err != nil {
		t.Fatal(err)
	}
	d.SetInfo(DocumentInfo{Author: "A"})
	if err := d.ConvertToPDF20(); err != nil {
		t.Fatal(err)
	}
	out, _ := d.XMPRaw()
	if !strings.Contains(string(out), "pdfaExtension:schemas") {
		t.Error("extension schema lost")
	}
	if x, _ := d.XMP(); x.Title != "T" || len(x.Authors) != 1 {
		t.Errorf("XMP = %+v", x)
	}
}

func TestConvertToPDF20RefusesRevision5(t *testing.T) {
	d := NewDocument(200, 200)
	d.preserved = &encryptState{algorithm: EncryptionAlgAES256, revision: 5}
	if err := d.ConvertToPDF20(); !errors.Is(err, ErrDeprecatedFeature) {
		t.Errorf("err = %v", err)
	}
	d.preserved = &encryptState{algorithm: EncryptionAlgRC4_128}
	if err := d.ConvertToPDF20(); !errors.Is(err, ErrDeprecatedFeature) {
		t.Errorf("err = %v", err)
	}
	d.preserved = &encryptState{algorithm: EncryptionAlgAES256}
	if err := d.ConvertToPDF20(); err != nil {
		t.Errorf("AES-256 revision 6 refused: %v", err)
	}
	if !bytes.HasPrefix([]byte(d.outputVersion(d.preserved).header()), []byte("%PDF-2.0")) {
		t.Error("not written as 2.0")
	}
}
