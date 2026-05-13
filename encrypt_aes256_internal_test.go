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
