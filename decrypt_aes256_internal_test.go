package asposepdf

import (
	"bytes"
	"crypto/aes"
	"testing"
)

func TestVerifyPermsV5R6_Valid(t *testing.T) {
	fek := bytes.Repeat([]byte{0xAB}, 32)
	block := buildPermsBlock(-4, true)
	enc := make([]byte, 16)
	cipher, _ := aes.NewCipher(fek)
	cipher.Encrypt(enc, block)
	if err := verifyPermsV5R6(fek, enc, -4); err != nil {
		t.Errorf("verify should pass: %v", err)
	}
}

func TestVerifyPermsV5R6_TamperedP(t *testing.T) {
	fek := bytes.Repeat([]byte{0xAB}, 32)
	block := buildPermsBlock(-4, true)
	enc := make([]byte, 16)
	cipher, _ := aes.NewCipher(fek)
	cipher.Encrypt(enc, block)
	// Verify with WRONG declared P.
	if err := verifyPermsV5R6(fek, enc, -8); err == nil {
		t.Error("verify should reject mismatched P")
	}
}

func TestVerifyPermsV5R6_TamperedBlock(t *testing.T) {
	fek := bytes.Repeat([]byte{0xAB}, 32)
	block := buildPermsBlock(-4, true)
	enc := make([]byte, 16)
	cipher, _ := aes.NewCipher(fek)
	cipher.Encrypt(enc, block)
	enc[0] ^= 0xFF // flip a byte
	if err := verifyPermsV5R6(fek, enc, -4); err == nil {
		t.Error("verify should reject byte-flipped ciphertext")
	}
}
