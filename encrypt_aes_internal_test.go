package asposepdf

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"testing"
)

func TestAddPKCS7_VariousLengths(t *testing.T) {
	cases := []struct {
		in       []byte
		wantPad  int
		wantTail []byte // last few bytes (pad value, repeated)
	}{
		{[]byte{}, 16, bytes.Repeat([]byte{16}, 16)},
		{[]byte{0x01}, 15, bytes.Repeat([]byte{15}, 15)},
		{[]byte{0x01, 0x02, 0x03}, 13, bytes.Repeat([]byte{13}, 13)},
		{bytes.Repeat([]byte{0x42}, 15), 1, []byte{1}},
		{bytes.Repeat([]byte{0x42}, 16), 16, bytes.Repeat([]byte{16}, 16)},
		{bytes.Repeat([]byte{0x42}, 17), 15, bytes.Repeat([]byte{15}, 15)},
	}
	for _, tc := range cases {
		got := addPKCS7(tc.in, aes.BlockSize)
		if len(got)%aes.BlockSize != 0 {
			t.Errorf("len(addPKCS7(%d-byte input)) = %d, not block-multiple", len(tc.in), len(got))
		}
		if len(got)-len(tc.in) != tc.wantPad {
			t.Errorf("addPKCS7(%d-byte input) added %d pad bytes, want %d",
				len(tc.in), len(got)-len(tc.in), tc.wantPad)
		}
		tail := got[len(got)-len(tc.wantTail):]
		if !bytes.Equal(tail, tc.wantTail) {
			t.Errorf("addPKCS7 trailing bytes = %v, want %v", tail, tc.wantTail)
		}
	}
}

func TestStripPKCS7_RoundTrip(t *testing.T) {
	inputs := [][]byte{
		{},
		{0x01},
		{0x01, 0x02, 0x03},
		bytes.Repeat([]byte{0x42}, 15),
		bytes.Repeat([]byte{0x42}, 16),
		bytes.Repeat([]byte{0x42}, 100),
	}
	for _, in := range inputs {
		padded := addPKCS7(in, aes.BlockSize)
		out, err := stripPKCS7(padded)
		if err != nil {
			t.Errorf("stripPKCS7 on padded %d-byte input: %v", len(in), err)
			continue
		}
		if !bytes.Equal(in, out) {
			t.Errorf("roundtrip differs for %d-byte input: got %v, want %v", len(in), out, in)
		}
	}
}

func TestStripPKCS7_InvalidPadding(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"not block-aligned", bytes.Repeat([]byte{0x01}, 15)},
		{"pad byte zero", append(bytes.Repeat([]byte{0x42}, 15), 0)},
		{"pad byte too large", append(bytes.Repeat([]byte{0x42}, 15), 17)},
		{"mismatched pad bytes", append(bytes.Repeat([]byte{0x42}, 13), 3, 3, 4)},
	}
	for _, tc := range cases {
		if _, err := stripPKCS7(tc.data); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestObjectKeyAES128_KnownVector(t *testing.T) {
	// Reference vector verified offline:
	//   docKey = 16 bytes of 0xAB
	//   objNum = 0x010203, gen = 0x0405
	//   suffix = "sAlT" (literal, per ISO 32000-1 §7.6.2)
	//   key = MD5(docKey || objNum_LE_3 || gen_LE_2 || "sAlT")
	// Offline computation:
	//   import hashlib
	//   buf = bytes([0xAB]*16) + bytes([0x03, 0x02, 0x01, 0x05, 0x04]) + b"sAlT"
	//   print(hashlib.md5(buf).hexdigest())
	// → "517f71c032e35e41161763d66b87fcc9"
	docKey := bytes.Repeat([]byte{0xAB}, 16)
	got := objectKeyAES128(docKey, 0x010203, 0x0405)
	want, _ := hex.DecodeString("517f71c032e35e41161763d66b87fcc9")
	if !bytes.Equal(got, want) {
		t.Errorf("objectKeyAES128 = %x, want %x", got, want)
	}
	if len(got) != 16 {
		t.Errorf("objectKeyAES128 length = %d, want 16 for AES-128", len(got))
	}
}

func TestObjectKeyAES128_DiffersFromRC4Key(t *testing.T) {
	// AES key derivation appends "sAlT" before MD5 — must produce a
	// different output than the RC4 path for the same docKey/objNum/gen.
	docKey := bytes.Repeat([]byte{0x55}, 16)
	state := &encryptState{key: docKey}
	rc4Key := state.objectKey(42)
	aesKey := objectKeyAES128(docKey, 42, 0)
	if bytes.Equal(rc4Key, aesKey[:len(rc4Key)]) {
		t.Errorf("AES and RC4 keys must differ for the same input")
	}
}

func TestEncryptBytesAES128_RoundTrip(t *testing.T) {
	state := &encryptState{
		algorithm: EncryptionAlgAES128,
		key:       bytes.Repeat([]byte{0xCD}, 16),
	}
	inputs := [][]byte{
		[]byte("Hello world"),
		[]byte("a"),
		[]byte(""),
		bytes.Repeat([]byte{0x42}, 1024),
	}
	for _, plain := range inputs {
		cipher, err := encryptBytesAES128(state, 42, 0, plain)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		// Ciphertext must be at least one block of IV + at least one block of body.
		if len(cipher) < 2*aes.BlockSize {
			t.Errorf("ciphertext length %d < 32 (IV + min body)", len(cipher))
		}
		if len(cipher)%aes.BlockSize != 0 {
			t.Errorf("ciphertext length %d not block-aligned", len(cipher))
		}
		// Decrypt and confirm it matches.
		got, err := decryptObjectAES128(state, 42, 0, cipher)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, plain) {
			t.Errorf("roundtrip differs: got %v, want %v", got, plain)
		}
	}
}

func TestEncryptBytesAES128_IVRandomness(t *testing.T) {
	state := &encryptState{
		algorithm: EncryptionAlgAES128,
		key:       bytes.Repeat([]byte{0xCD}, 16),
	}
	plain := []byte("identical input")
	c1, _ := encryptBytesAES128(state, 1, 0, plain)
	c2, _ := encryptBytesAES128(state, 1, 0, plain)
	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of identical input produced identical output — IV not random")
	}
}

func TestBuildEncryptDictAES128(t *testing.T) {
	state := &encryptState{
		algorithm:   EncryptionAlgAES128,
		key:         bytes.Repeat([]byte{0xAB}, 16),
		fileID:      bytes.Repeat([]byte{0xCD}, 16),
		ownerEntry:  bytes.Repeat([]byte{0x01}, 32),
		userEntry:   bytes.Repeat([]byte{0x02}, 32),
		permissions: -4,
	}
	dict := buildEncryptDict(state)
	// Verify V=4 R=4 shape.
	if v, _ := dict["/V"]; v != 4 {
		t.Errorf("/V = %v, want 4", v)
	}
	if r, _ := dict["/R"]; r != 4 {
		t.Errorf("/R = %v, want 4", r)
	}
	if l, _ := dict["/Length"]; l != 128 {
		t.Errorf("/Length = %v, want 128", l)
	}
	// /CF presence — must be a nested pdfDict.
	cf, ok := dict["/CF"].(pdfDict)
	if !ok {
		t.Fatalf("/CF missing or wrong type: %T", dict["/CF"])
	}
	stdCF, ok := cf["/StdCF"].(pdfDict)
	if !ok {
		t.Fatalf("/CF/StdCF missing or wrong type: %T", cf["/StdCF"])
	}
	if cfm, _ := stdCF["/CFM"].(pdfName); cfm != "/AESV2" {
		t.Errorf("/CF/StdCF/CFM = %v, want /AESV2", cfm)
	}
	if stmf, _ := dict["/StmF"].(pdfName); stmf != "/StdCF" {
		t.Errorf("/StmF = %v, want /StdCF", stmf)
	}
	if strF, _ := dict["/StrF"].(pdfName); strF != "/StdCF" {
		t.Errorf("/StrF = %v, want /StdCF", strF)
	}
}

func TestEncryptBytesDispatcher(t *testing.T) {
	plain := []byte("hello dispatcher")
	rc4State := &encryptState{algorithm: EncryptionAlgRC4_128, key: bytes.Repeat([]byte{0xAB}, 16)}
	aesState := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}

	rc4Out, err := rc4State.encryptBytes(1, 0, plain)
	if err != nil {
		t.Fatal(err)
	}
	aesOut, err := aesState.encryptBytes(1, 0, plain)
	if err != nil {
		t.Fatal(err)
	}
	// AES output starts with 16-byte IV + ciphertext; minimum 32 bytes.
	if len(aesOut) < 32 {
		t.Errorf("AES output too short: %d", len(aesOut))
	}
	// RC4 output is same length as input (stream cipher, no padding/IV).
	if len(rc4Out) != len(plain) {
		t.Errorf("RC4 output length = %d, want %d", len(rc4Out), len(plain))
	}
	// They must differ.
	if bytes.Equal(rc4Out, aesOut[:len(rc4Out)]) {
		t.Error("dispatcher returned identical bytes for RC4 vs AES — wrong algorithm selected")
	}
}

func TestBuildEncryptDictRC4Unchanged(t *testing.T) {
	state := &encryptState{
		algorithm:   EncryptionAlgRC4_128,
		key:         bytes.Repeat([]byte{0xAB}, 16),
		fileID:      bytes.Repeat([]byte{0xCD}, 16),
		ownerEntry:  bytes.Repeat([]byte{0x01}, 32),
		userEntry:   bytes.Repeat([]byte{0x02}, 32),
		permissions: -4,
	}
	dict := buildEncryptDict(state)
	if v, _ := dict["/V"]; v != 2 {
		t.Errorf("/V = %v, want 2 for RC4", v)
	}
	if r, _ := dict["/R"]; r != 3 {
		t.Errorf("/R = %v, want 3 for RC4", r)
	}
	if _, exists := dict["/CF"]; exists {
		t.Error("/CF must not be present in RC4 dict")
	}
	if _, exists := dict["/StmF"]; exists {
		t.Error("/StmF must not be present in RC4 dict")
	}
	if _, exists := dict["/StrF"]; exists {
		t.Error("/StrF must not be present in RC4 dict")
	}
}
