// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// qpdfR4RC4 is the /Encrypt of a file qpdf wrote with a revision-4 handler,
// an RC4 crypt filter (/CFM /V2), /EncryptMetadata false, an empty user
// password and owner password "owner"; key is the file key qpdf derived.
var qpdfR4RC4 = struct{ o, u, id, key string }{
	o:   "566fa873ee33c797cd3b904fdadf814afa34df9a38f6ed41b984e2c6da2aa6f5",
	u:   "f856a6f07e7d55d5919d1d48b035bd840021446990b9e4114071a4d9104984c1",
	id:  "31415926535897932384626433832795",
	key: "103e4ddd6f927e1359d9d24e2c588189",
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func qpdfR4RC4Dict(t *testing.T) (pdfDict, pdfDict) {
	enc := pdfDict{
		"/Filter": pdfName("/Standard"), "/V": 4, "/R": 4, "/Length": 128, "/P": -1028,
		"/O": string(unhex(t, qpdfR4RC4.o)), "/U": string(unhex(t, qpdfR4RC4.u)),
		"/CF": pdfDict{"/StdCF": pdfDict{"/AuthEvent": pdfName("/DocOpen"), "/CFM": pdfName("/V2"), "/Length": 16}},
		"/StmF": pdfName("/StdCF"), "/StrF": pdfName("/StdCF"), "/EncryptMetadata": false,
	}
	trailer := pdfDict{"/ID": pdfArray{string(unhex(t, qpdfR4RC4.id)), string(unhex(t, qpdfR4RC4.id))}}
	return enc, trailer
}

// TestV4RC4CryptFilterKey: a V=4 handler with an RC4 crypt filter opens
// (pdf-go-r140), and with /EncryptMetadata false the key includes Algorithm 2
// step (f) — checked against the key qpdf derived, with either password.
func TestV4RC4CryptFilterKey(t *testing.T) {
	for _, pw := range []string{"", "owner"} {
		enc, trailer := qpdfR4RC4Dict(t)
		state, err := buildDecryptStateV4R4(enc, trailer, pw)
		if err != nil {
			t.Fatalf("password %q: %v", pw, err)
		}
		if got := hex.EncodeToString(state.key); got != qpdfR4RC4.key {
			t.Errorf("password %q: key %s, want %s", pw, got, qpdfR4RC4.key)
		}
		if state.algorithm != EncryptionAlgRC4_128 || !state.keyPlainMeta {
			t.Errorf("password %q: algorithm %v, keyPlainMeta %v", pw, state.algorithm, state.keyPlainMeta)
		}
	}
	enc, trailer := qpdfR4RC4Dict(t)
	if _, err := buildDecryptStateV4R4(enc, trailer, "wrong"); err == nil {
		t.Error("a wrong password was accepted")
	}
}

// TestV4RC4PlainMetadataResave: a document whose key needs /EncryptMetadata
// false is written back as revision 4 with that entry and its /Metadata stream
// in the clear, so it reopens with the same key; other streams stay encrypted.
func TestV4RC4PlainMetadataResave(t *testing.T) {
	enc, trailer := qpdfR4RC4Dict(t)
	state, err := buildDecryptStateV4R4(enc, trailer, "")
	if err != nil {
		t.Fatal(err)
	}
	d := NewDocument(300, 100)
	p, _ := d.Page(1)
	if err := p.AddText("secret body", TextStyle{}, Rectangle{LLX: 10, LLY: 40, URX: 290, URY: 90}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetXMPRaw([]byte(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><!-- plain marker --></x:xmpmeta><?xpacket end="w"?>`)); err != nil {
		t.Fatal(err)
	}
	d.preserved = state
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.Contains(out, []byte("plain marker")) {
		t.Error("the /Metadata stream was encrypted")
	}
	if !bytes.HasPrefix(out, []byte("%PDF-1.5")) {
		t.Errorf("header %q, want %%PDF-1.5", out[:8])
	}
	for _, want := range []string{"/EncryptMetadata false", "/V2"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("no %s in the /Encrypt dictionary", want)
		}
	}

	re, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	texts, err := re.ExtractText()
	if err != nil || !strings.Contains(texts[0], "secret body") {
		t.Errorf("text after reopen: %q, %v", texts, err)
	}
	if hex.EncodeToString(re.preserved.key) != qpdfR4RC4.key {
		t.Errorf("key after reopen %x", re.preserved.key)
	}
}
