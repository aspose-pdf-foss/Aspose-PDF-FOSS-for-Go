// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"testing"
)

// TestHashV5R5_KnownVector pins the revision-5 hash to SHA-256 over
// password || salt || extra, with expected values from Python's hashlib.
func TestHashV5R5_KnownVector(t *testing.T) {
	salt := bytes.Repeat([]byte{0xAB}, 8)
	for _, tc := range []struct {
		extra []byte
		want  string
	}{
		{nil, "4d8f68b3a08f73a1c5ff92954e687bd49728112873a505784986cacd77e62965"},
		{[]byte("extra"), "c37bcbe41b24a4e3ca98f34dd9dfe070b7b0e342b1cd647b2f37f686222c2a29"},
	} {
		if got := hex.EncodeToString(hashV5R5([]byte("pw"), salt, tc.extra)); got != tc.want {
			t.Errorf("hashV5R5(extra=%q) = %s, want %s", tc.extra, got, tc.want)
		}
	}
}

// buildEncryptDictV5R5 builds a revision-5 /Encrypt dict the way Acrobat 9
// does — the R=6 layout with every hash taken by hashV5R5 — and returns it
// with the FEK. withPerms=false omits /Perms, as some R=5 producers do.
func buildEncryptDictV5R5(t *testing.T, userPwd, ownerPwd string, perms int32, withPerms bool) (pdfDict, []byte) {
	t.Helper()
	fek := bytes.Repeat([]byte{0x5A}, 32)
	vsU, ksU := bytes.Repeat([]byte{1}, 8), bytes.Repeat([]byte{2}, 8)
	vsO, ksO := bytes.Repeat([]byte{3}, 8), bytes.Repeat([]byte{4}, 8)

	U := append(append(hashV5R5([]byte(userPwd), vsU, nil), vsU...), ksU...)
	O := append(append(hashV5R5([]byte(ownerPwd), vsO, U), vsO...), ksO...)
	UE, err := aes256CBCNoPadding(hashV5R5([]byte(userPwd), ksU, nil), fek)
	if err != nil {
		t.Fatal(err)
	}
	OE, err := aes256CBCNoPadding(hashV5R5([]byte(ownerPwd), ksO, U), fek)
	if err != nil {
		t.Fatal(err)
	}
	dict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/V":      5, "/R": 5, "/Length": 256, "/P": int(uint32(perms)),
		"/O":  string(O),
		"/U":  string(U),
		"/UE": string(UE),
		"/OE": string(OE),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/Type": pdfName("/CryptFilter"),
				"/CFM":  pdfName("/AESV3"),
			},
		},
		"/StmF": pdfName("/StdCF"),
		"/StrF": pdfName("/StdCF"),
	}
	if withPerms {
		block, _ := aes.NewCipher(fek)
		enc := make([]byte, 16)
		block.Encrypt(enc, buildPermsBlock(perms, true))
		dict["/Perms"] = string(enc)
	}
	return dict, fek
}

// TestBuildDecryptStateV5R5 exercises the revision-5 password algorithms
// end to end with synthetic entries: the user and owner passwords both
// recover the FEK, with or without /Perms, and a wrong password is rejected
// with ErrInvalidPassword.
func TestBuildDecryptStateV5R5(t *testing.T) {
	for _, withPerms := range []bool{true, false} {
		dict, fek := buildEncryptDictV5R5(t, "user", "owner", -4, withPerms)
		for _, pw := range []string{"user", "owner"} {
			state, err := buildDecryptStateFor(dict, pdfDict{}, &openCredentials{password: &pw})
			if err != nil {
				t.Fatalf("perms=%v password %q: %v", withPerms, pw, err)
			}
			if !bytes.Equal(state.key, fek) {
				t.Errorf("perms=%v password %q: recovered FEK differs", withPerms, pw)
			}
			if state.revision != 5 || state.algorithm != EncryptionAlgAES256 {
				t.Errorf("perms=%v: revision=%d algorithm=%v, want 5 AES-256", withPerms, state.revision, state.algorithm)
			}
		}
		wrong := "wrong"
		if _, err := buildDecryptStateFor(dict, pdfDict{}, &openCredentials{password: &wrong}); err != ErrInvalidPassword {
			t.Errorf("perms=%v wrong password: err = %v, want ErrInvalidPassword", withPerms, err)
		}
	}
}

// TestBuildDecryptStateV5R5_TamperedP: a /P that no longer matches the
// encrypted /Perms block is rejected, as for revision 6.
func TestBuildDecryptStateV5R5_TamperedP(t *testing.T) {
	dict, _ := buildEncryptDictV5R5(t, "user", "owner", -4, true)
	dict["/P"] = -8
	if _, err := buildDecryptStateV5(dict, "user", 5); err == nil {
		t.Error("tampered /P accepted")
	}
}

// TestBuildEncryptDict_PreservedV5R5 re-emits a parsed revision-5 state, as
// a re-save of a document opened with its password does. The dict must keep
// /R 5 — the preserved /U and /O hashes are only valid under revision 5 —
// and must not invent a /Perms the file never had.
func TestBuildEncryptDict_PreservedV5R5(t *testing.T) {
	for _, withPerms := range []bool{true, false} {
		dict, _ := buildEncryptDictV5R5(t, "user", "owner", -4, withPerms)
		state, err := buildDecryptStateV5(dict, "user", 5)
		if err != nil {
			t.Fatal(err)
		}
		out := buildEncryptDict(state)
		if out["/V"] != 5 || out["/R"] != 5 {
			t.Errorf("perms=%v: /V=%v /R=%v, want 5 5", withPerms, out["/V"], out["/R"])
		}
		if _, has := out["/Perms"]; has != withPerms {
			t.Errorf("perms=%v: /Perms present = %v", withPerms, has)
		}
	}
}
