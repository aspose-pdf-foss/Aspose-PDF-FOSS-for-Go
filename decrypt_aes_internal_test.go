package asposepdf

import (
	"bytes"
	"testing"
)

func TestDecryptObjectAES128_ShortCiphertext(t *testing.T) {
	state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
	// Less than IV length.
	if _, err := decryptObjectAES128(state, 1, 0, []byte{0x01, 0x02, 0x03}); err == nil {
		t.Error("expected error for short ciphertext, got nil")
	}
}

func TestDecryptObjectAES128_UnalignedBody(t *testing.T) {
	state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
	// 16-byte IV + 17 bytes (not block-aligned body).
	bad := make([]byte, 16+17)
	if _, err := decryptObjectAES128(state, 1, 0, bad); err == nil {
		t.Error("expected error for unaligned body, got nil")
	}
}

func TestDecryptObjectAES128_GarbledCiphertextBadPadding(t *testing.T) {
	// Random IV + random body of correct length but garbled — decryption
	// will produce noise, PKCS#7 strip will fail.
	state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
	bad := bytes.Repeat([]byte{0xFF}, 32) // 16-byte IV + 16-byte body
	if _, err := decryptObjectAES128(state, 1, 0, bad); err == nil {
		t.Error("expected PKCS#7 error on garbled ciphertext, got nil")
	}
}
