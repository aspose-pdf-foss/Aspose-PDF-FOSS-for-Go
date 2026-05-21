// SPDX-License-Identifier: MIT

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

func TestBuildDecryptStateV4R4_MissingCF(t *testing.T) {
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      4,
		"/R":      4,
		"/Length": 128,
		"/P":      -4,
		"/O":      string(bytes.Repeat([]byte{0x01}, 32)),
		"/U":      string(bytes.Repeat([]byte{0x02}, 32)),
		// /CF intentionally missing
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	trailer := pdfDict{"/ID": pdfArray{string(bytes.Repeat([]byte{0xCD}, 16))}}
	if _, err := buildDecryptStateV4R4(encDict, trailer, "x"); err == nil {
		t.Error("expected error for missing /CF")
	}
}

func TestBuildDecryptStateV4R4_WrongCFM(t *testing.T) {
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      4, "/R": 4, "/Length": 128, "/P": -4,
		"/O": string(bytes.Repeat([]byte{0x01}, 32)),
		"/U": string(bytes.Repeat([]byte{0x02}, 32)),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/V2"), // wrong — should be /AESV2
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	trailer := pdfDict{"/ID": pdfArray{string(bytes.Repeat([]byte{0xCD}, 16))}}
	if _, err := buildDecryptStateV4R4(encDict, trailer, "x"); err == nil {
		t.Error("expected error for /CFM /V2 in V=4 dict")
	}
}

func TestBuildDecryptStateDispatcher(t *testing.T) {
	// Unsupported V/R → error mentioning unsupported.
	encDict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5,
		"/R":      6,
	}
	trailer := pdfDict{}
	_, err := buildDecryptState(encDict, trailer, "x")
	if err == nil {
		t.Error("expected error for V=5 R=6 (not yet supported)")
	}
}
