// SPDX-License-Identifier: MIT

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

func TestBuildDecryptStateV5R6_MissingCF(t *testing.T) {
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 6, "/Length": 256, "/P": -4,
		"/O":     string(bytes.Repeat([]byte{0x01}, 48)),
		"/U":     string(bytes.Repeat([]byte{0x02}, 48)),
		"/UE":    string(bytes.Repeat([]byte{0x03}, 32)),
		"/OE":    string(bytes.Repeat([]byte{0x04}, 32)),
		"/Perms": string(bytes.Repeat([]byte{0x05}, 16)),
		// /CF intentionally missing
	}
	if _, err := buildDecryptStateV5R6(encDict, "x"); err == nil {
		t.Error("expected error for missing /CF")
	}
}

func TestBuildDecryptStateV5R6_WrongCFM(t *testing.T) {
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 6, "/Length": 256, "/P": -4,
		"/O":     string(bytes.Repeat([]byte{0x01}, 48)),
		"/U":     string(bytes.Repeat([]byte{0x02}, 48)),
		"/UE":    string(bytes.Repeat([]byte{0x03}, 32)),
		"/OE":    string(bytes.Repeat([]byte{0x04}, 32)),
		"/Perms": string(bytes.Repeat([]byte{0x05}, 16)),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/AESV2"), // wrong — should be /AESV3
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	if _, err := buildDecryptStateV5R6(encDict, "x"); err == nil {
		t.Error("expected error for /CFM /AESV2 in V=5 dict")
	}
}

func TestBuildDecryptStateV5R6_MissingUE(t *testing.T) {
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 6, "/Length": 256, "/P": -4,
		"/O": string(bytes.Repeat([]byte{0x01}, 48)),
		"/U": string(bytes.Repeat([]byte{0x02}, 48)),
		// /UE missing
		"/OE":    string(bytes.Repeat([]byte{0x04}, 32)),
		"/Perms": string(bytes.Repeat([]byte{0x05}, 16)),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/AESV3"),
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	if _, err := buildDecryptStateV5R6(encDict, "x"); err == nil {
		t.Error("expected error for missing /UE")
	}
}

func TestBuildDecryptStateV5R6_WrongPassword(t *testing.T) {
	// Build a real V=5 R=6 state via newEncryptStateV5R6, then attempt
	// to recover with a different password.
	cfg := &encryptConfig{algorithm: EncryptionAlgAES256, userPassword: "correct"}
	state, _ := newEncryptStateV5R6(cfg)
	// Construct the /Encrypt dict from this state.
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 6, "/Length": 256, "/P": int(uint32(state.permissions)),
		"/O":     string(state.ownerEntry),
		"/U":     string(state.userEntry),
		"/UE":    string(state.userKeyEntry),
		"/OE":    string(state.ownerKeyEntry),
		"/Perms": string(state.permsEntry),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/AESV3"),
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	if _, err := buildDecryptStateV5R6(encDict, "wrong"); err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestBuildDecryptStateV5R6_CorrectPassword(t *testing.T) {
	cfg := &encryptConfig{algorithm: EncryptionAlgAES256, userPassword: "correct"}
	state, _ := newEncryptStateV5R6(cfg)
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 6, "/Length": 256, "/P": int(uint32(state.permissions)),
		"/O":     string(state.ownerEntry),
		"/U":     string(state.userEntry),
		"/UE":    string(state.userKeyEntry),
		"/OE":    string(state.ownerKeyEntry),
		"/Perms": string(state.permsEntry),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/AESV3"),
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	recovered, err := buildDecryptStateV5R6(encDict, "correct")
	if err != nil {
		t.Fatalf("buildDecryptStateV5R6: %v", err)
	}
	if !bytes.Equal(recovered.key, state.key) {
		t.Error("recovered FEK differs from original")
	}
}
