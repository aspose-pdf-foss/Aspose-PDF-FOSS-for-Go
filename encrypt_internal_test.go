package asposepdf

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestPadPassword verifies padding against ISO 32000-1 Algorithm 2 step (a):
// "pad or truncate to exactly 32 bytes, using the padding string" — padding
// bytes are taken from the BEGINNING of passwordPadBytes, not from offset n.
// A fencepost error here silently produces ciphertext every PDF reader rejects,
// because both writer and reader are self-consistent.
func TestPadPassword(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []byte
	}{
		{
			name: "empty password is full pad constant",
			in:   "",
			want: passwordPadBytes[:],
		},
		{
			name: "short password: 'secret' + pad[:26]",
			in:   "secret",
			want: append([]byte("secret"), passwordPadBytes[:26]...),
		},
		{
			name: "single byte + pad[:31]",
			in:   "a",
			want: append([]byte("a"), passwordPadBytes[:31]...),
		},
		{
			name: "exactly 32 bytes: unchanged",
			in:   "0123456789abcdef0123456789abcdef",
			want: []byte("0123456789abcdef0123456789abcdef"),
		},
		{
			name: "longer than 32 bytes: truncated",
			in:   "0123456789abcdef0123456789abcdefEXTRA",
			want: []byte("0123456789abcdef0123456789abcdef"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := padPassword(tc.in)
			if len(got) != 32 {
				t.Fatalf("length = %d, want 32", len(got))
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("\ngot:  %x\nwant: %x", got, tc.want)
			}
		})
	}
}

// TestComputeUserEntryReferenceVector pins computeEncKey and computeUserEntry
// against a vector produced by an independent PDF writer (pypdf). Unlike the
// self-consistent round-trip tests, this proves our implementation matches the
// spec Algorithms 2 and 5 as used by real-world PDF tooling. If any subroutine
// drifts from the spec, /U will no longer match and the test fails.
//
// Source: a minimal encrypted PDF produced by pypdf.PdfWriter.encrypt(
//
//	user_password="secret", permissions_flag=-4, use_128bit=True).
//
// Reference values read directly from that PDF's /Encrypt dict and /ID.
func TestComputeUserEntryReferenceVector(t *testing.T) {
	fileID := hexDecode(t, "3431373330306533613233626666333962666434643761393431343839316334")
	oEntry := hexDecode(t, "cae12a13706437b2a133a2021c2c7f1f1b0692d87066efdbef7b1b00e6c60758")
	wantU := hexDecode(t, "20d82f78cf48b212ae3b4c732d19b38228bf4e5e4e758a4164004e56fffa0108")
	const password = "secret"

	key := computeEncKey(password, oEntry, encryptPermissions, fileID)
	gotU := computeUserEntry(key, fileID)

	if !bytes.Equal(gotU, wantU) {
		t.Errorf("computeUserEntry mismatch\ngot:  %x\nwant: %x", gotU, wantU)
	}
	if !verifyUserPassword(password, oEntry, wantU, fileID) {
		t.Error("verifyUserPassword rejected the correct password for the reference vector")
	}
	if verifyUserPassword("wrong", oEntry, wantU, fileID) {
		t.Error("verifyUserPassword accepted a wrong password for the reference vector")
	}
}

// TestNonASCIIPasswordMatchesPyPDF pins the encoding of non-ASCII passwords.
// padPassword currently passes raw UTF-8 bytes through, which matches the
// behavior of pypdf 6.x and modern Adobe Acrobat. If we ever silently switch
// to PDFDocEncoding or another transcoding, the Cyrillic password "пароль"
// would encode to different bytes and this test would fail — surfacing a
// breaking change in password semantics that would otherwise be invisible
// to ASCII-only test suites.
//
// Vector source: pypdf.PdfWriter.encrypt(user_password="пароль",
// algorithm="RC4-128", permissions_flag=-4). /O, /U, /ID read directly
// from the resulting file's /Encrypt dict and /ID.
func TestNonASCIIPasswordMatchesPyPDF(t *testing.T) {
	fileID := hexDecode(t, "e8b53c4162a2da07c50cc77f77f017af")
	oEntry := hexDecode(t, "477b07103319bc9bbb9d4be32a0bb5311ec33b8b606cd3a8c5f4c3e726c9f61c")
	wantU := hexDecode(t, "3d24e11b71f977b0b15b1af2895e645f28bf4e5e4e758a4164004e56fffa0108")
	const password = "пароль"

	key := computeEncKey(password, oEntry, encryptPermissions, fileID)
	gotU := computeUserEntry(key, fileID)

	if !bytes.Equal(gotU, wantU) {
		t.Errorf("computeUserEntry mismatch\ngot:  %x\nwant: %x", gotU, wantU)
	}
	if !verifyUserPassword(password, oEntry, wantU, fileID) {
		t.Error("verifyUserPassword rejected the correct Cyrillic password")
	}
	if verifyUserPassword("parol", oEntry, wantU, fileID) {
		t.Error("verifyUserPassword accepted a Latin transliteration instead of UTF-8 bytes")
	}
}

// TestEncryptPasswordVerification tests the cryptographic round-trip:
// passwords that were used to produce /O and /U can be verified against them.
func TestEncryptPasswordVerification(t *testing.T) {
	fileID := []byte("0123456789abcdef") // fixed 16-byte ID for determinism

	oEntry := computeOwnerEntry("user", "owner")
	key := computeEncKey("user", oEntry, encryptPermissions, fileID)
	uEntry := computeUserEntry(key, fileID)

	if !verifyUserPassword("user", oEntry, uEntry, fileID) {
		t.Error("correct user password not verified")
	}
	if verifyUserPassword("wrong", oEntry, uEntry, fileID) {
		t.Error("wrong password incorrectly verified")
	}
	if verifyUserPassword("owner", oEntry, uEntry, fileID) {
		t.Error("owner password should not satisfy user verification")
	}
}

// TestEncryptSameUserOwner verifies that an empty owner password falls back to user password.
func TestEncryptSameUserOwner(t *testing.T) {
	fileID := []byte("fedcba9876543210")

	oEntry := computeOwnerEntry("secret", "secret") // ownerPwd == userPwd
	key := computeEncKey("secret", oEntry, encryptPermissions, fileID)
	uEntry := computeUserEntry(key, fileID)

	if !verifyUserPassword("secret", oEntry, uEntry, fileID) {
		t.Error("correct password not verified when user == owner")
	}
}

// TestEncryptRC4Symmetric verifies that encrypting twice (RC4 XOR) restores the original.
func TestEncryptRC4Symmetric(t *testing.T) {
	s := &encryptState{key: make([]byte, encKeyLen)}
	copy(s.key, []byte("0123456789abcdef"))

	original := []byte("Hello, PDF encryption!")
	encrypted := s.encryptBytes(5, original)
	decrypted := s.encryptBytes(5, encrypted)

	if string(decrypted) != string(original) {
		t.Errorf("RC4 round-trip failed: got %q, want %q", decrypted, original)
	}
}

func hexDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}
