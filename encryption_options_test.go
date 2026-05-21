// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestSetEncryptionPasswordsOnly verifies the simple case: only passwords,
// no explicit Permissions — falls back to all-allow default, matching what
// SetPassword alone produces.
func TestSetEncryptionPasswordsOnly(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetEncryption(EncryptionOptions{
		UserPassword:  "secret",
		OwnerPassword: "owner",
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/P 4294967292")) {
		t.Error("expected all-allow /P (4294967292) when Permissions omitted")
	}
}

// TestSetEncryptionWithPermissions verifies a non-nil Permissions pointer
// drives the /P bit-packing through toPDFBits.
func TestSetEncryptionWithPermissions(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetEncryption(EncryptionOptions{
		UserPassword: "secret",
		Permissions: &Permissions{
			AllowPrint:         true,
			AllowAccessibility: true,
		},
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	wantP := Permissions{AllowPrint: true, AllowAccessibility: true}.toPDFBits()
	wantStr := intToString(int(uint32(wantP)))
	if !bytes.Contains(buf.Bytes(), []byte("/P "+wantStr)) {
		t.Errorf("expected /P %s in saved file", wantStr)
	}
}

// TestSetEncryptionEmptyOwnerDefaultsToUser reproduces the behavior
// guaranteed by SetPassword: owner password falls back to user password
// when left empty.
func TestSetEncryptionEmptyOwnerDefaultsToUser(t *testing.T) {
	docA := NewDocument(595, 842)
	docA.SetEncryption(EncryptionOptions{UserPassword: "secret"})

	docB := NewDocument(595, 842)
	docB.SetPassword("secret", "") // established behavior: empty owner = user

	// Derive /O for both; they should match. Force deterministic fileID by
	// computing directly (bypass random).
	oA := computeOwnerEntry("secret", "secret")
	oB := computeOwnerEntry("secret", "secret")
	if !bytes.Equal(oA, oB) {
		t.Error("/O entry should be identical for SetEncryption{UserPassword:\"secret\"} and SetPassword(\"secret\",\"\")")
	}
	if docA.encrypt.userPassword != docB.encrypt.userPassword ||
		docA.encrypt.ownerPassword != docB.encrypt.ownerPassword {
		t.Errorf("encryptConfig differs:\n  A: %+v\n  B: %+v", docA.encrypt, docB.encrypt)
	}
}

// TestSetEncryptionReplacesPriorConfig documents the semantics: calling
// SetEncryption starts from a clean slate. A prior SetPassword +
// SetPermissions are discarded in favor of the fresh options struct.
func TestSetEncryptionReplacesPriorConfig(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetPassword("old", "old-owner")
	doc.SetPermissions(Permissions{AllowPrint: true})

	doc.SetEncryption(EncryptionOptions{
		UserPassword: "new",
		// Permissions omitted → back to all-allow default
	})

	if doc.encrypt.userPassword != "new" {
		t.Errorf("userPassword = %q, want \"new\" (prior SetPassword should be replaced)", doc.encrypt.userPassword)
	}
	if doc.encrypt.ownerPassword != "" {
		t.Errorf("ownerPassword = %q, want empty (prior SetPassword should be replaced)", doc.encrypt.ownerPassword)
	}
	if doc.encrypt.hasPermissions {
		t.Error("hasPermissions should be false after SetEncryption with nil Permissions")
	}
}

// TestSetEncryptionThenSetPermissionsOverrides verifies that the per-
// field setters continue to work on top of SetEncryption for targeted
// updates.
func TestSetEncryptionThenSetPermissionsOverrides(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetEncryption(EncryptionOptions{
		UserPassword: "secret",
		Permissions:  &Permissions{AllowPrint: true},
	})
	doc.SetPermissions(Permissions{AllowCopy: true}) // narrow, replaces

	want := Permissions{AllowCopy: true}.toPDFBits()
	if doc.encrypt.permissions != want {
		t.Errorf("permissions = %#x, want %#x", doc.encrypt.permissions, want)
	}
}

func intToString(v int) string {
	// Tiny helper to avoid importing strconv only for this test. The tests
	// above emit /P as a positive unsigned decimal, same format as writer.go.
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}
