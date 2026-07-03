// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"testing"
)

func TestHashV5R6_KnownVector(t *testing.T) {
	// Reference vector computed offline via the Python equivalent of
	// Algorithm 2.B (ISO 32000-2 §7.6.4.3.4):
	//   password = b"pw"
	//   salt = bytes([0xAB] * 8)
	//   extra = b""
	//   hashV5R6 → first 32 bytes of K after iteration terminates.
	// Frozen reference (pre-verified, 76 rounds in Python):
	want, _ := hex.DecodeString("b2f65b9d1faca5ed0dfa849a3c641a6b41b16613dcdd74ef6e6a6d1f7e3b9177")
	got := hashV5R6([]byte("pw"), bytes.Repeat([]byte{0xAB}, 8), nil)
	if !bytes.Equal(got, want) {
		t.Errorf("hashV5R6 mismatch:\n got: %x\nwant: %x", got, want)
	}
	if len(got) != 32 {
		t.Errorf("hashV5R6 length = %d, want 32", len(got))
	}
}

func TestHashV5R6_ExtraAffectsOutput(t *testing.T) {
	a := hashV5R6([]byte("pw"), bytes.Repeat([]byte{0xAB}, 8), nil)
	b := hashV5R6([]byte("pw"), bytes.Repeat([]byte{0xAB}, 8), []byte("extra"))
	if bytes.Equal(a, b) {
		t.Error("hashV5R6 should differ when extra changes")
	}
}

func TestHashV5R6_PasswordAffectsOutput(t *testing.T) {
	a := hashV5R6([]byte("pw1"), bytes.Repeat([]byte{0xAB}, 8), nil)
	b := hashV5R6([]byte("pw2"), bytes.Repeat([]byte{0xAB}, 8), nil)
	if bytes.Equal(a, b) {
		t.Error("hashV5R6 should differ when password changes")
	}
}

func TestHashV5R6_SaltAffectsOutput(t *testing.T) {
	a := hashV5R6([]byte("pw"), bytes.Repeat([]byte{0xAB}, 8), nil)
	b := hashV5R6([]byte("pw"), bytes.Repeat([]byte{0xCD}, 8), nil)
	if bytes.Equal(a, b) {
		t.Error("hashV5R6 should differ when salt changes")
	}
}

func TestEncryptBytesAES256_RoundTrip(t *testing.T) {
	state := &encryptState{
		algorithm: EncryptionAlgAES256,
		key:       bytes.Repeat([]byte{0xCD}, 32), // 32-byte FEK
	}
	inputs := [][]byte{
		[]byte("Hello world"),
		[]byte("a"),
		[]byte(""),
		bytes.Repeat([]byte{0x42}, 1024),
		bytes.Repeat([]byte{0x42}, 16), // exact block boundary
		bytes.Repeat([]byte{0x42}, 32), // 2 blocks exactly
	}
	for _, plain := range inputs {
		cipher, err := encryptBytesAES256(state, plain)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if len(cipher) < 2*aes.BlockSize {
			t.Errorf("ciphertext length %d < 32 (IV + min body)", len(cipher))
		}
		if len(cipher)%aes.BlockSize != 0 {
			t.Errorf("ciphertext length %d not block-aligned", len(cipher))
		}
		got, err := decryptObjectAES256(state, cipher)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, plain) {
			t.Errorf("roundtrip differs for %d-byte input", len(plain))
		}
	}
}

func TestEncryptBytesAES256_IVRandomness(t *testing.T) {
	state := &encryptState{
		algorithm: EncryptionAlgAES256,
		key:       bytes.Repeat([]byte{0xCD}, 32),
	}
	plain := []byte("identical input")
	c1, _ := encryptBytesAES256(state, plain)
	c2, _ := encryptBytesAES256(state, plain)
	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of identical input produced identical output — IV not random")
	}
}

func TestEncryptBytesAES256_NeedsAES256Key(t *testing.T) {
	state := &encryptState{
		algorithm: EncryptionAlgAES256,
		key:       bytes.Repeat([]byte{0xCD}, 16), // wrong size (AES-128 length)
	}
	if _, err := encryptBytesAES256(state, []byte("hi")); err == nil {
		t.Error("encryptBytesAES256 should fail with 16-byte key")
	}
}

func TestBuildPermsBlock_Layout(t *testing.T) {
	block := buildPermsBlock(-4, true)
	if len(block) != 16 {
		t.Fatalf("perms block length = %d, want 16", len(block))
	}
	// /P = -4 → uint32 0xFFFFFFFC → LE bytes 0xFC 0xFF 0xFF 0xFF
	if block[0] != 0xFC || block[1] != 0xFF || block[2] != 0xFF || block[3] != 0xFF {
		t.Errorf("P bytes wrong: %x %x %x %x", block[0], block[1], block[2], block[3])
	}
	// Bytes 4-7: 0xFF 0xFF 0xFF 0xFF
	for i := 4; i < 8; i++ {
		if block[i] != 0xFF {
			t.Errorf("byte %d = %x, want 0xFF", i, block[i])
		}
	}
	// Byte 8: 'T' for EncryptMetadata=true
	if block[8] != 'T' {
		t.Errorf("byte 8 = %q, want 'T'", block[8])
	}
	// Bytes 9-11: 'a','d','b'
	if block[9] != 'a' || block[10] != 'd' || block[11] != 'b' {
		t.Errorf("marker bytes wrong: %q%q%q", block[9], block[10], block[11])
	}
}

func TestBuildPermsBlock_EncryptMetadataFalse(t *testing.T) {
	block := buildPermsBlock(-1, false)
	if block[8] != 'F' {
		t.Errorf("byte 8 with EncryptMetadata=false = %q, want 'F'", block[8])
	}
}

func TestNewEncryptStateV5R6_FieldLengths(t *testing.T) {
	cfg := &encryptConfig{
		algorithm:      EncryptionAlgAES256,
		userPassword:   "user",
		ownerPassword:  "owner",
		permissions:    -4,
		hasPermissions: true,
	}
	state, err := newEncryptStateV5R6(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if state.algorithm != EncryptionAlgAES256 {
		t.Errorf("algorithm = %v", state.algorithm)
	}
	if len(state.key) != 32 {
		t.Errorf("FEK length = %d, want 32", len(state.key))
	}
	if len(state.userEntry) != 48 {
		t.Errorf("/U length = %d, want 48", len(state.userEntry))
	}
	if len(state.ownerEntry) != 48 {
		t.Errorf("/O length = %d, want 48", len(state.ownerEntry))
	}
	if len(state.userKeyEntry) != 32 {
		t.Errorf("/UE length = %d, want 32", len(state.userKeyEntry))
	}
	if len(state.ownerKeyEntry) != 32 {
		t.Errorf("/OE length = %d, want 32", len(state.ownerKeyEntry))
	}
	if len(state.permsEntry) != 16 {
		t.Errorf("/Perms length = %d, want 16", len(state.permsEntry))
	}
	if state.permissions != -4 {
		t.Errorf("permissions = %d", state.permissions)
	}
}

func TestNewEncryptStateV5R6_OwnerDefaultsToUser(t *testing.T) {
	cfg := &encryptConfig{
		algorithm:    EncryptionAlgAES256,
		userPassword: "shared",
		// OwnerPassword empty
		hasPermissions: false,
	}
	state, err := newEncryptStateV5R6(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Owner-entry hash should still differ from user-entry hash because
	// of distinct salts and the U_bytes-extra parameter.
	if bytes.Equal(state.userEntry[0:32], state.ownerEntry[0:32]) {
		t.Error("/U hash equals /O hash even though salts and extra differ")
	}
}

func TestNewEncryptStateV5R6_RandomFEKEachCall(t *testing.T) {
	cfg := &encryptConfig{
		algorithm:    EncryptionAlgAES256,
		userPassword: "x",
	}
	s1, _ := newEncryptStateV5R6(cfg)
	s2, _ := newEncryptStateV5R6(cfg)
	if bytes.Equal(s1.key, s2.key) {
		t.Error("FEK should be random per newEncryptStateV5R6 call")
	}
}

func TestEncryptBytesDispatcher_AES256(t *testing.T) {
	plain := []byte("dispatcher routes correctly")
	aes256State := &encryptState{
		algorithm: EncryptionAlgAES256,
		key:       bytes.Repeat([]byte{0xCD}, 32),
	}
	out, err := aes256State.encryptBytes(1, 0, plain)
	if err != nil {
		t.Fatalf("dispatcher: %v", err)
	}
	if len(out) < 2*aes.BlockSize {
		t.Errorf("output length %d < 32 (IV + min body)", len(out))
	}
	// Roundtrip via decrypt path.
	got, err := decryptObjectAES256(aes256State, out)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("dispatcher roundtrip failed")
	}
}

func TestBuildEncryptDict_V5R6(t *testing.T) {
	cfg := &encryptConfig{algorithm: EncryptionAlgAES256, userPassword: "x"}
	state, _ := newEncryptStateV5R6(cfg)
	dict := buildEncryptDict(state)

	if v := dict["/V"]; v != 5 {
		t.Errorf("/V = %v, want 5", v)
	}
	if r := dict["/R"]; r != 6 {
		t.Errorf("/R = %v, want 6", r)
	}
	if l := dict["/Length"]; l != 256 {
		t.Errorf("/Length = %v, want 256", l)
	}
	for _, k := range []string{"/O", "/U", "/OE", "/UE", "/Perms"} {
		if _, ok := dict[k]; !ok {
			t.Errorf("dict missing %s", k)
		}
	}
	if em := dict["/EncryptMetadata"]; em != true {
		t.Errorf("/EncryptMetadata = %v, want true", em)
	}
	cf, ok := dict["/CF"].(pdfDict)
	if !ok {
		t.Fatal("/CF missing or wrong type")
	}
	stdCF, ok := cf["/StdCF"].(pdfDict)
	if !ok {
		t.Fatal("/CF/StdCF missing")
	}
	if cfm, _ := stdCF["/CFM"].(pdfName); cfm != "/AESV3" {
		t.Errorf("/CF/StdCF/CFM = %v, want /AESV3", cfm)
	}
	if stmf, _ := dict["/StmF"].(pdfName); stmf != "/StdCF" {
		t.Errorf("/StmF = %v, want /StdCF", stmf)
	}
}

func TestBuildEncryptDict_AES128Unchanged(t *testing.T) {
	state := &encryptState{
		algorithm:   EncryptionAlgAES128,
		key:         bytes.Repeat([]byte{0xAB}, 16),
		ownerEntry:  bytes.Repeat([]byte{0x01}, 32),
		userEntry:   bytes.Repeat([]byte{0x02}, 32),
		permissions: -4,
	}
	dict := buildEncryptDict(state)
	if v := dict["/V"]; v != 4 {
		t.Errorf("AES-128 /V = %v, want 4 (no regression)", v)
	}
	if _, exists := dict["/UE"]; exists {
		t.Error("AES-128 dict should not contain /UE")
	}
	if _, exists := dict["/Perms"]; exists {
		t.Error("AES-128 dict should not contain /Perms")
	}
}
