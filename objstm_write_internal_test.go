// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"
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

// formDocument builds a one-page document with n text fields — many small
// dictionaries, which is what object streams are for.
func formDocument(t *testing.T, n int) *Document {
	t.Helper()
	doc := NewDocument(612, 792)
	for i := 0; i < n; i++ {
		row, col := i/10, i%10
		rect := Rectangle{LLX: 20 + float64(col)*58, LLY: 740 - float64(row)*14, URX: 74 + float64(col)*58, URY: 752 - float64(row)*14}
		f, err := doc.Form().AddTextField(1, rect, fmt.Sprintf("field%03d", i))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetValue(fmt.Sprintf("value %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

func saveCompressed(t *testing.T, doc *Document) []byte {
	t.Helper()
	if _, err := doc.Optimize(OptimizationOptions{CompressObjects: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A compressed save must reopen to the same document, carry object streams
// and a cross-reference stream, and be smaller than the classic save.
func TestCompressObjectsRoundTrip(t *testing.T) {
	var plain bytes.Buffer
	if _, err := formDocument(t, 150).WriteTo(&plain); err != nil {
		t.Fatal(err)
	}
	out := saveCompressed(t, formDocument(t, 150))

	if !bytes.HasPrefix(out, []byte("%PDF-1.5")) {
		t.Errorf("header = %q, want %%PDF-1.5", out[:8])
	}
	if !bytes.Contains(out, []byte("/ObjStm")) {
		t.Error("no object stream in the output")
	}
	if !bytes.Contains(out, []byte("/XRef")) {
		t.Error("no cross-reference stream in the output")
	}
	if bytes.Contains(out, []byte("\nxref\n")) {
		t.Error("a classic xref table was still written")
	}
	if len(out) >= plain.Len() {
		t.Errorf("compressed %d bytes, classic %d — no saving", len(out), plain.Len())
	}

	doc, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("page count = %d, want 1", doc.PageCount())
	}
	f := doc.Form().Field("field149")
	if f == nil || f.Value() != "value 149" {
		t.Errorf("field149 did not survive: %#v", f)
	}
	if n := len(doc.Form().Fields()); n != 150 {
		t.Errorf("%d fields after reopen, want 150", n)
	}
}

// Encrypted objects inside an object stream are protected by the stream's
// encryption only; every algorithm must reopen with the password. The
// "value 7 readable in the clear" bytes.Contains check this test used to run
// proves nothing: Flate already hides the literal text inside the object
// stream regardless of encryption, so it can never fail. Assert something
// that actually distinguishes encrypted from unencrypted output instead:
// /ObjStm is present (objects still got packed) and the header matches the
// algorithm (AES-256 bumps to %PDF-2.0 per ISO 32000-2; the others bump the
// usual %PDF-1.4 -> %PDF-1.5 for object streams).
func TestCompressObjectsEncrypted(t *testing.T) {
	for _, alg := range []EncryptionAlgorithm{EncryptionAlgRC4_40, EncryptionAlgRC4_128, EncryptionAlgAES128, EncryptionAlgAES256} {
		t.Run(fmt.Sprint(alg), func(t *testing.T) {
			doc := formDocument(t, 120)
			doc.SetEncryption(EncryptionOptions{UserPassword: "u", OwnerPassword: "o", Algorithm: alg})
			out := saveCompressed(t, doc)
			if !bytes.Contains(out, []byte("/ObjStm")) {
				t.Error("no object stream in the encrypted output")
			}
			wantHeader := "%PDF-1.5"
			if alg == EncryptionAlgAES256 {
				wantHeader = "%PDF-2.0"
			}
			if !bytes.HasPrefix(out, []byte(wantHeader)) {
				t.Errorf("header = %q, want %s", out[:8], wantHeader)
			}
			reopened, err := OpenStreamWithPassword(bytes.NewReader(out), "u")
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			f := reopened.Form().Field("field007")
			if f == nil || f.Value() != "value 7" {
				t.Errorf("field007 = %#v, want value 7", f)
			}
		})
	}
}

// A document signed on a compressed save keeps its signature dictionary out of
// the object streams, so the placeholders are patched and the signature holds.
func TestCompressObjectsSigned(t *testing.T) {
	cert, key := newTestSigner(t, "ObjStm Signer")

	doc := formDocument(t, 120)
	if err := doc.Sign(SignOptions{Certificate: cert, PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	out := saveCompressed(t, doc)

	reopened, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || !sigs[0].Valid || !sigs[0].CoversWholeDocument {
		t.Fatalf("signature on a compressed save does not hold: %+v", sigs)
	}
}

// newTestSigner returns a fresh self-signed certificate + key pair for
// signing tests, named cn.
func newTestSigner(t *testing.T, cn string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() % 1_000_000),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

// A revision appended to a compressed file keeps the file's kind of cross
// reference: a stream after a stream. The earlier signature stays valid and
// the later one covers the whole file.
func TestIncrementalSignatureOnCompressedFile(t *testing.T) {
	cert1, key1 := newTestSigner(t, "First")
	doc := formDocument(t, 120)
	if err := doc.Sign(SignOptions{Certificate: cert1, PrivateKey: key1}); err != nil {
		t.Fatal(err)
	}
	first := saveCompressed(t, doc)

	reopened, err := OpenStream(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	cert2, key2 := newTestSigner(t, "Second")
	if err := reopened.Sign(SignOptions{Certificate: cert2, PrivateKey: key2}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := reopened.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	second := buf.Bytes()

	appended := second[len(first):]
	if bytes.Contains(appended, []byte("\nxref\n")) {
		t.Error("a classic xref section was appended to a file using cross-reference streams")
	}
	if !bytes.Contains(appended, []byte("/XRef")) {
		t.Error("the appended revision carries no cross-reference stream")
	}

	final, err := OpenStream(bytes.NewReader(second))
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := final.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signatures, want 2", len(sigs))
	}
	for _, s := range sigs {
		if !s.Valid {
			t.Errorf("signature %s is not valid: %v", s.FieldName, s.Err)
		}
	}
}

// An encrypted document signed on a compressed save is signed incrementally
// on top of its own encrypted, compressed bytes; that revision must use a
// cross-reference stream too, and the signature must hold.
func TestSignEncryptedCompressedAppendsXRefStream(t *testing.T) {
	cert, key := newTestSigner(t, "Encrypted Signer")
	doc := formDocument(t, 120)
	doc.SetEncryption(EncryptionOptions{UserPassword: "u", OwnerPassword: "o", Algorithm: EncryptionAlgAES256})
	if err := doc.Sign(SignOptions{Certificate: cert, PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	out := saveCompressed(t, doc)
	if bytes.Contains(out, []byte("\nxref\n")) {
		t.Error("a classic xref section appears in an encrypted, compressed, signed file")
	}
	if !bytes.Contains(out, []byte("/XRef")) {
		t.Error("no cross-reference stream in an encrypted, compressed, signed file")
	}
	reopened, err := OpenStreamWithPassword(bytes.NewReader(out), "u")
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("signature does not hold: %+v", sigs)
	}
}

// More than objStmCapacity (100) packable objects must spill into a second
// /ObjStm, and every object must still be reachable through it on reopen.
func TestCompressObjectsMultipleObjectStreams(t *testing.T) {
	out := saveCompressed(t, formDocument(t, 101))

	if n := bytes.Count(out, []byte("/Type /ObjStm")); n < 2 {
		t.Errorf("%d /ObjStm streams in the output, want at least 2", n)
	}

	doc, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if n := len(doc.Form().Fields()); n != 101 {
		t.Errorf("%d fields after reopen, want 101", n)
	}
	for i := 0; i < 101; i++ {
		name := fmt.Sprintf("field%03d", i)
		f := doc.Form().Field(name)
		want := fmt.Sprintf("value %d", i)
		if f == nil || f.Value() != want {
			t.Errorf("%s = %#v, want %q", name, f, want)
		}
	}
}

func TestOptimizeReportsPackableObjects(t *testing.T) {
	doc := formDocument(t, 30)
	res, err := doc.Optimize(OptimizationOptions{CompressObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.CompressedObjects < 30 {
		t.Errorf("CompressedObjects = %d, want at least 30", res.CompressedObjects)
	}
	if !DefaultOptimizationOptions().CompressObjects {
		t.Error("CompressObjects is not in the default preset")
	}
}

// PDF/A-1 is built on PDF 1.4, which has no object streams: converting to it
// must undo a CompressObjects set earlier, or the file cannot conform.
func TestConvertToPDFA1ClearsCompressObjects(t *testing.T) {
	doc := formDocument(t, 5)
	if _, err := doc.Optimize(OptimizationOptions{CompressObjects: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("/ObjStm")) {
		t.Error("a PDF/A-1 conversion was written with object streams")
	}
}

// The PDF/A-1 guard must hold in either call order. ConvertToPDFA clears
// compressObjects as a convenience, but a later Optimize call can set it
// again; the writer must still refuse object streams because it checks the
// document's own XMP (isPDFA1), not just the flag Optimize last touched.
func TestConvertToPDFA1ThenOptimizeStaysUncompressed(t *testing.T) {
	doc := formDocument(t, 5)
	if _, err := doc.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Optimize(DefaultOptimizationOptions()); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if bytes.Contains(out, []byte("/ObjStm")) {
		t.Error("a PDF/A-1 document was written with object streams after a later Optimize call")
	}
	if bytes.Contains(out, []byte("/XRef")) {
		t.Error("a PDF/A-1 document was written with a cross-reference stream after a later Optimize call")
	}
}

// The guard also protects a document that was never itself passed to
// ConvertToPDFA: a document opened from an existing PDF/A-1 file already
// carries the pdfaid:part=1 XMP, and Optimize must still refuse to compress
// it, purely from that XMP — the compressObjects-clearing convenience in
// ConvertToPDFA never ran on this *Document instance at all.
func TestOpenedPDFA1DocumentStaysUncompressed(t *testing.T) {
	doc := formDocument(t, 5)
	if _, err := doc.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	var pdfa1 bytes.Buffer
	if _, err := doc.WriteTo(&pdfa1); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStream(bytes.NewReader(pdfa1.Bytes()))
	if err != nil {
		t.Fatalf("reopen the PDF/A-1 file: %v", err)
	}
	if _, err := reopened.Optimize(DefaultOptimizationOptions()); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := reopened.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("/ObjStm")) {
		t.Error("a document opened from a PDF/A-1 file was written with object streams after Optimize")
	}
	if bytes.Contains(out.Bytes(), []byte("/XRef")) {
		t.Error("a document opened from a PDF/A-1 file was written with a cross-reference stream after Optimize")
	}
}

// PDF/A-2 is PDF 1.7-based and permits object streams, so the same
// Optimize-after-convert sequence must still compress.
func TestConvertToPDFA2ThenOptimizeCompresses(t *testing.T) {
	doc := formDocument(t, 150)
	if _, err := doc.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Optimize(DefaultOptimizationOptions()); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.Contains(out, []byte("/ObjStm")) {
		t.Error("a PDF/A-2 document should still pack into object streams after ConvertToPDFA+Optimize")
	}
	if !bytes.Contains(out, []byte("/XRef")) {
		t.Error("a PDF/A-2 document should still write a cross-reference stream after ConvertToPDFA+Optimize")
	}
}
