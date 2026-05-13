package asposepdf

import (
	"bytes"
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
