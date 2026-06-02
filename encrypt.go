// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/md5"
	cryptorand "crypto/rand"
	"crypto/rc4"
	"encoding/binary"
	"fmt"
	"io"
)

// encKeyLen is the document encryption key length in bytes (128-bit RC4).
const encKeyLen = 16

// passwordPadBytes is the 32-byte padding string defined in PDF spec Appendix B.
var passwordPadBytes = [32]byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// encryptPermissionsAllowAll grants every operation. Per ISO 32000-1
// Table 22: bits 1-2 reserved 0, bits 3-12 all permissions set, bits
// 7-8/13-32 reserved set = 0xFFFFFFFC (signed -4).
const encryptPermissionsAllowAll int32 = -4

// Permissions controls what a viewer allows on an encrypted PDF. The zero
// value denies every operation. Permissions only take effect when the
// document is also encrypted (SetPassword); viewers enforce the bits —
// the library itself is not a DRM enforcer.
//
// Per ISO 32000-1 §7.6.3.2 Table 22, bit meanings for R=3/R=4:
//
//	AllowPrint         — bit 3  (low-resolution print)
//	AllowModify        — bit 4  (modify page contents)
//	AllowCopy          — bit 5  (copy text / extract graphics)
//	AllowAnnotations   — bit 6  (add/modify text annotations)
//	AllowFormFill      — bit 9  (fill in existing form fields)
//	AllowAccessibility — bit 10 (extract text/graphics for accessibility)
//	AllowAssembly      — bit 11 (insert/rotate/delete pages)
//	AllowPrintHighRes  — bit 12 (print in high resolution)
type Permissions struct {
	AllowPrint         bool
	AllowModify        bool
	AllowCopy          bool
	AllowAnnotations   bool
	AllowFormFill      bool
	AllowAccessibility bool
	AllowAssembly      bool
	AllowPrintHighRes  bool
}

// permissionsFromPDFBits is the inverse of toPDFBits: decode the /P value
// back into a Permissions struct. Reserved bits are ignored — only the
// eight boolean permission bits are extracted.
func permissionsFromPDFBits(bits int32) Permissions {
	u := uint32(bits)
	return Permissions{
		AllowPrint:         u&(1<<2) != 0,
		AllowModify:        u&(1<<3) != 0,
		AllowCopy:          u&(1<<4) != 0,
		AllowAnnotations:   u&(1<<5) != 0,
		AllowFormFill:      u&(1<<8) != 0,
		AllowAccessibility: u&(1<<9) != 0,
		AllowAssembly:      u&(1<<10) != 0,
		AllowPrintHighRes:  u&(1<<11) != 0,
	}
}

// toPDFBits returns the /P value encoded per ISO 32000-1 Table 22 with
// the Adobe convention: reserved bits 1-2 cleared, reserved bits 7-8 and
// 13-32 set high (shall-be-1 per spec for R>=3).
func (p Permissions) toPDFBits() int32 {
	// Reserved-high baseline: bits 7-8 (0x00C0) + bits 13-32 (0xFFFFF000).
	bits := uint32(0xFFFFF0C0)
	if p.AllowPrint {
		bits |= 1 << 2 // bit 3
	}
	if p.AllowModify {
		bits |= 1 << 3 // bit 4
	}
	if p.AllowCopy {
		bits |= 1 << 4 // bit 5
	}
	if p.AllowAnnotations {
		bits |= 1 << 5 // bit 6
	}
	if p.AllowFormFill {
		bits |= 1 << 8 // bit 9
	}
	if p.AllowAccessibility {
		bits |= 1 << 9 // bit 10
	}
	if p.AllowAssembly {
		bits |= 1 << 10 // bit 11
	}
	if p.AllowPrintHighRes {
		bits |= 1 << 11 // bit 12
	}
	return int32(bits)
}

// EncryptionAlgorithm selects the cipher and security-handler revision
// used by (*Document).SetEncryption. The zero value is AES-128.
type EncryptionAlgorithm int

const (
	// EncryptionAlgAES128 — AES-128, Standard Security Handler V=4 R=4,
	// /CFM /AESV2. ISO 32000-1 §7.6.3.2. The default (zero value).
	EncryptionAlgAES128 EncryptionAlgorithm = iota

	// EncryptionAlgRC4_128 — RC4-128, Standard Security Handler V=2 R=3.
	// Legacy compatibility only — AES-128 is preferred for new documents.
	EncryptionAlgRC4_128

	// EncryptionAlgAES256 — AES-256, Standard Security Handler V=5 R=6,
	// /CFM /AESV3. ISO 32000-2 §7.6.4. Output PDF header is bumped to
	// %PDF-2.0; viewers older than Adobe Acrobat DC (~2015) may not
	// support PDF 2.0 documents.
	EncryptionAlgAES256
)

// EncryptionOptions bundles every knob that controls how a document is
// encrypted when saved. It is the unified structured input for
// (*Document).SetEncryption; the shorter SetPassword / SetPermissions
// methods remain available for the common case of one-line updates.
//
// Zero values:
//
//	UserPassword  — no password (empty string is a valid "open without
//	                password" user-password under R<=4; the PDF still
//	                carries permissions).
//	OwnerPassword — defaults to UserPassword, matching Adobe behavior.
//	Permissions   — nil means "grant all" (the historical default);
//	                pass a non-nil pointer to restrict. Permissions{}
//	                (value, not pointer) deliberately denies everything,
//	                so the pointer distinguishes "omitted" from "deny".
//	Algorithm     — EncryptionAlgAES128 (zero value).
//
// Example:
//
//	doc.SetEncryption(asposepdf.EncryptionOptions{
//	    UserPassword:  "user",
//	    OwnerPassword: "owner",
//	    Permissions:   &asposepdf.Permissions{AllowPrint: true, AllowCopy: true},
//	})
//	doc.Save("restricted.pdf")
type EncryptionOptions struct {
	UserPassword  string
	OwnerPassword string
	Permissions   *Permissions
	Algorithm     EncryptionAlgorithm
}

// encryptConfig holds password and permission settings for encrypting a document.
type encryptConfig struct {
	algorithm      EncryptionAlgorithm
	userPassword   string
	ownerPassword  string // if empty, treated the same as userPassword
	permissions    int32  // /P value; used if hasPermissions is true
	hasPermissions bool   // false → fall back to encryptPermissionsAllowAll
}

// effectivePermissions returns the /P value to use, honoring an explicit
// SetPermissions call or falling back to the all-allow default.
func (c *encryptConfig) effectivePermissions() int32 {
	if c.hasPermissions {
		return c.permissions
	}
	return encryptPermissionsAllowAll
}

// encryptState holds the computed values needed to encrypt a single PDF write.
type encryptState struct {
	algorithm     EncryptionAlgorithm // used by Task 9's dispatcher
	key           []byte              // RC4/AES-128: 16 bytes; AES-256: 32 bytes (FEK)
	fileID        []byte              // 16 bytes; not used for V=5 R=6 key derivation
	ownerEntry    []byte              // RC4/AES-128: 32 bytes; AES-256: 48 bytes
	userEntry     []byte              // RC4/AES-128: 32 bytes; AES-256: 48 bytes
	userKeyEntry  []byte              // AES-256 only: 32 bytes (/UE); zero for others
	ownerKeyEntry []byte              // AES-256 only: 32 bytes (/OE); zero for others
	permsEntry    []byte              // AES-256 only: 16 bytes (/Perms); zero for others
	permissions   int32               // /P value propagated to /Encrypt dict
}

// newEncryptState derives all encryption parameters from cfg.
func newEncryptState(cfg *encryptConfig) (*encryptState, error) {
	if cfg.algorithm == EncryptionAlgAES256 {
		return newEncryptStateV5R6(cfg)
	}

	fileID := make([]byte, 16)
	if _, err := io.ReadFull(cryptorand.Reader, fileID); err != nil {
		return nil, fmt.Errorf("generate file ID: %w", err)
	}

	ownerPwd := cfg.ownerPassword
	if ownerPwd == "" {
		ownerPwd = cfg.userPassword
	}

	perms := cfg.effectivePermissions()
	oEntry := computeOwnerEntry(cfg.userPassword, ownerPwd)
	key := computeEncKey(cfg.userPassword, oEntry, perms, fileID)
	uEntry := computeUserEntry(key, fileID)

	return &encryptState{
		algorithm:   cfg.algorithm, // zero value = EncryptionAlgAES128 (new default)
		key:         key,
		fileID:      fileID,
		ownerEntry:  oEntry,
		userEntry:   uEntry,
		permissions: perms,
	}, nil
}

// padPassword pads or truncates s to exactly 32 bytes using the PDF spec padding string.
// Per ISO 32000-1 Algorithm 2 step (a): append bytes from the BEGINNING of the
// padding string until the result is 32 bytes long.
func padPassword(s string) []byte {
	out := make([]byte, 32)
	n := copy(out, s)
	copy(out[n:], passwordPadBytes[:])
	return out
}

// computeOwnerEntry computes the /O encryption dict entry per PDF Algorithm 3.
func computeOwnerEntry(userPwd, ownerPwd string) []byte {
	// MD5 of padded owner password, then 50 extra rounds (R=3).
	sum := md5.Sum(padPassword(ownerPwd))
	key := sum[:]
	for i := 0; i < 50; i++ {
		s := md5.Sum(key[:encKeyLen])
		key = s[:]
	}
	ownerKey := key[:encKeyLen]

	// RC4-encrypt padded user password with owner key, then 19 XOR'd iterations (R=3).
	result := padPassword(userPwd)
	applyRC4(result, ownerKey)
	for i := 1; i <= 19; i++ {
		applyRC4(result, xorKey(ownerKey, byte(i)))
	}
	return result
}

// computeEncKey computes the document encryption key per PDF Algorithm 2.
func computeEncKey(userPwd string, ownerEntry []byte, permissions int32, fileID []byte) []byte {
	return computeEncKeyR(userPwd, ownerEntry, permissions, fileID, 3, encKeyLen)
}

// computeEncKeyR is the revision/key-length-aware form of computeEncKey
// (PDF Algorithm 2). r==2 stops after the initial MD5 (40-bit handler);
// r>=3 applies the 50 extra MD5 passes. keyLen is the key length in bytes
// (5 for 40-bit, 16 for 128-bit).
func computeEncKeyR(userPwd string, ownerEntry []byte, permissions int32, fileID []byte, r, keyLen int) []byte {
	h := md5.New()
	h.Write(padPassword(userPwd))
	h.Write(ownerEntry)
	var p [4]byte
	binary.LittleEndian.PutUint32(p[:], uint32(permissions))
	h.Write(p[:])
	h.Write(fileID)
	key := h.Sum(nil)[:keyLen]
	if r >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(key)
			key = s[:keyLen]
		}
	}
	return key
}

// computeUserEntry computes the /U encryption dict entry per PDF Algorithm 5 (R=3).
func computeUserEntry(encKey, fileID []byte) []byte {
	return computeUserEntryR(encKey, fileID, 3)
}

// computeUserEntryR computes /U per Algorithm 4 (r==2) or Algorithm 5
// (r>=3). For r==2, /U is simply the 32-byte password padding string
// RC4-encrypted with the key. For r>=3 it is MD5(pad||ID) RC4-encrypted
// with 19 extra XOR'd rounds, then padded back to 32 bytes.
func computeUserEntryR(encKey, fileID []byte, r int) []byte {
	if r == 2 {
		out := make([]byte, 32)
		copy(out, passwordPadBytes[:])
		applyRC4(out, encKey)
		return out
	}
	h := md5.New()
	h.Write(passwordPadBytes[:])
	h.Write(fileID)
	result := h.Sum(nil) // 16 bytes
	applyRC4(result, encKey)
	for i := 1; i <= 19; i++ {
		applyRC4(result, xorKey(encKey, byte(i)))
	}
	// Algorithm 5 step 7: "append 16 bytes of arbitrary padding". The spec says
	// only the first 16 bytes are checked, but in practice Adobe and poppler
	// reject /U entries that are not padded with the first 16 bytes of the
	// password padding constant. Matching that convention maximises interop.
	return append(result, passwordPadBytes[:16]...)
}

// encryptBytesRC4 encrypts data under the per-object RC4 key derived
// from state.key and objNum (PDF Algorithm 1). Generation is always 0
// in our writer. This implements the V=2 R=3 path verbatim — the
// dispatcher in encryptBytes selects this for EncryptionAlgRC4_128.
func encryptBytesRC4(s *encryptState, objNum int, data []byte) []byte {
	key := s.objectKey(objNum)
	result := make([]byte, len(data))
	copy(result, data)
	applyRC4(result, key)
	return result
}

// encryptBytes is the per-object encryption dispatcher. gen is the
// object generation; for newly-written objects it is always 0 but the
// parameter exists because Algorithm 1.A (AES) reads it.
func (s *encryptState) encryptBytes(objNum, gen int, data []byte) ([]byte, error) {
	switch s.algorithm {
	case EncryptionAlgRC4_128:
		return encryptBytesRC4(s, objNum, data), nil
	case EncryptionAlgAES128:
		return encryptBytesAES128(s, objNum, gen, data)
	case EncryptionAlgAES256:
		return encryptBytesAES256(s, data) // ignores objNum, gen (no per-object key)
	}
	return nil, fmt.Errorf("encryptBytes: unknown algorithm %d", s.algorithm)
}

// objectKey derives the per-object RC4 key per PDF Algorithm 1.
// Generation number is always 0 in our writer.
func (s *encryptState) objectKey(objNum int) []byte {
	h := md5.New()
	h.Write(s.key)
	h.Write([]byte{byte(objNum), byte(objNum >> 8), byte(objNum >> 16), 0, 0})
	out := h.Sum(nil)
	// Per Algorithm 1, the per-object key length is the document key length
	// plus 5, capped at 16. Use the actual key length so 40-bit (5-byte)
	// documents get a 10-byte per-object key, not 16.
	keyLen := len(s.key) + 5
	if keyLen > 16 {
		keyLen = 16
	}
	return out[:keyLen]
}

// applyRC4 applies RC4 to data in-place with the given key.
func applyRC4(data, key []byte) {
	c, _ := rc4.NewCipher(key)
	c.XORKeyStream(data, data)
}

// xorKey returns a copy of key with each byte XOR'd with x.
func xorKey(key []byte, x byte) []byte {
	out := make([]byte, len(key))
	for i, b := range key {
		out[i] = b ^ x
	}
	return out
}

// verifyUserPassword returns true if password matches the encryption parameters.
// It recomputes /U and compares the first 16 bytes (per PDF spec for R=3).
// permissions must be the /P value read from the /Encrypt dict of the file
// being verified — the derivation is /P-dependent.
func verifyUserPassword(password string, ownerEntry, storedU, fileID []byte, permissions int32) bool {
	return verifyUserPasswordR(password, ownerEntry, storedU, fileID, permissions, 3, encKeyLen)
}

// verifyUserPasswordR is the revision/key-length-aware form. It recomputes
// /U and compares the first 32 bytes for r==2 (Algorithm 6) or the first
// 16 bytes for r>=3.
func verifyUserPasswordR(password string, ownerEntry, storedU, fileID []byte, permissions int32, r, keyLen int) bool {
	key := computeEncKeyR(password, ownerEntry, permissions, fileID, r, keyLen)
	computed := computeUserEntryR(key, fileID, r)
	n := 16
	if r == 2 {
		n = 32
	}
	if len(computed) < n || len(storedU) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if computed[i] != storedU[i] {
			return false
		}
	}
	return true
}

// Encrypt writes a password-protected copy of inputPath to outputPath.
// The document is encrypted using RC4-128 (PDF 1.4 Standard Security Handler, revision 3).
// userPassword is required to open the document; ownerPassword controls permission settings.
// If ownerPassword is empty, it defaults to userPassword.
//
// Example:
//
//	err := asposepdf.Encrypt("input.pdf", "output.pdf", "secret", "")
func Encrypt(inputPath, outputPath, userPassword, ownerPassword string) error {
	doc, err := Open(inputPath)
	if err != nil {
		return err
	}
	doc.SetPassword(userPassword, ownerPassword)
	return doc.Save(outputPath)
}
