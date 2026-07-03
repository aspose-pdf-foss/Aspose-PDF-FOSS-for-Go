// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/md5"
	"testing"
)

// TestDecodeLiteralStringOctal verifies that PDF literal-string octal
// escapes (\ddd), named escapes, and backslash-EOL line continuations are
// decoded to the correct raw bytes per ISO 32000-1 §7.3.4.2. This matters
// for binary strings such as the /O and /U encryption entries, which many
// producers write as literal strings full of octal escapes.
func TestDecodeLiteralStringOctal(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		{`(\232\021A)`, []byte{0x9A, 0x11, 'A'}}, // 3- and 3-digit octal
		{`(\0\7\10\377)`, []byte{0, 7, 8, 0xFF}}, // 1/1/2/3-digit octal
		{`(a\)b\(c)`, []byte("a)b(c")},           // escaped parens
		{`(x\\y)`, []byte{'x', '\\', 'y'}},       // escaped backslash
		{"(a\\\nb)", []byte("ab")},               // backslash-LF continuation
		{"(a\\\r\nb)", []byte("ab")},             // backslash-CRLF continuation
		{`(plain)`, []byte("plain")},             // no escapes
		{`(\101\102\103)`, []byte("ABC")},        // octal letters
	}
	for _, c := range cases {
		got := decodeLiteralString([]byte(c.in))
		if got != string(c.want) {
			t.Errorf("decodeLiteralString(%q) = % x, want % x", c.in, got, c.want)
		}
	}
}

// TestStandardSecurityR2 exercises the 40-bit RC4 (V=1 R=2) password
// algorithms end to end with synthetic /O and /U entries: a correct user
// password and a correct owner password both authenticate, and a wrong
// password is rejected.
func TestStandardSecurityR2(t *testing.T) {
	const keyLen, r = 5, 2
	id := []byte("0123456789ABCDEF")
	var perms int32 = -3904
	userPwd, ownerPwd := "alice", "bob"

	// /O per Algorithm 3, revision 2: owner key = MD5(pad(owner))[:5];
	// /O = RC4(ownerKey, pad(user)).
	ownerSum := md5.Sum(padPassword(ownerPwd))
	ownerKey := ownerSum[:keyLen]
	O := make([]byte, 32)
	copy(O, padPassword(userPwd))
	applyRC4(O, ownerKey)

	// /U per Algorithm 4, revision 2: key from Algorithm 2, then
	// RC4(key, 32-byte pad).
	key := computeEncKeyR(userPwd, O, perms, id, r, keyLen)
	U := computeUserEntryR(key, id, r)
	if len(U) != 32 {
		t.Fatalf("R=2 /U length = %d, want 32", len(U))
	}

	if !verifyUserPasswordR(userPwd, O, U, id, perms, r, keyLen) {
		t.Error("correct user password failed to authenticate")
	}
	rec, ok := recoverUserPasswordFromOwnerR(ownerPwd, O, r, keyLen)
	if !ok || !verifyUserPasswordR(rec, O, U, id, perms, r, keyLen) {
		t.Error("correct owner password failed to authenticate")
	}
	if verifyUserPasswordR("wrong", O, U, id, perms, r, keyLen) {
		t.Error("wrong password was accepted")
	}

	// The per-object key for a 5-byte document key must be 10 bytes
	// (keyLen + 5), not the 16 used for 128-bit documents.
	st := &encryptState{algorithm: EncryptionAlgRC4_128, key: key}
	if got := len(st.objectKey(7)); got != keyLen+5 {
		t.Errorf("objectKey length = %d, want %d", got, keyLen+5)
	}
}

// TestDecodeLiteralStringBinaryRoundsTrip is a sanity check that a 32-byte
// binary blob written as a literal string with octal escapes decodes back
// to the exact bytes.
func TestDecodeLiteralStringBinaryRoundsTrip(t *testing.T) {
	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i*7 + 3)
	}
	var lit bytes.Buffer
	lit.WriteByte('(')
	for _, b := range want {
		switch b {
		case '(', ')', '\\':
			lit.WriteByte('\\')
			lit.WriteByte(b)
		default:
			// Always octal-escape so the test exercises that path.
			lit.WriteByte('\\')
			lit.WriteByte('0' + (b>>6)&7)
			lit.WriteByte('0' + (b>>3)&7)
			lit.WriteByte('0' + b&7)
		}
	}
	lit.WriteByte(')')

	got := decodeLiteralString(lit.Bytes())
	if got != string(want) {
		t.Errorf("round-trip mismatch:\n got % x\nwant % x", got, want)
	}
}
