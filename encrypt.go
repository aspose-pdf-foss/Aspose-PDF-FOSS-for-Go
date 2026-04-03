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

// encryptPermissions grants all user operations (print, copy, modify, annotations).
// PDF spec: bits 1-2 reserved 0, all permission bits set = -4 (0xFFFFFFFC).
const encryptPermissions int32 = -4

// encryptConfig holds the password settings for encrypting a document.
type encryptConfig struct {
	userPassword  string
	ownerPassword string // if empty, treated the same as userPassword
}

// encryptState holds the computed values needed to encrypt a single PDF write.
type encryptState struct {
	key        []byte // 16-byte document encryption key
	fileID     []byte // 16-byte random file identifier
	ownerEntry []byte // 32-byte /O value for the /Encrypt dict
	userEntry  []byte // 32-byte /U value for the /Encrypt dict
}

// newEncryptState derives all encryption parameters from cfg.
func newEncryptState(cfg *encryptConfig) (*encryptState, error) {
	fileID := make([]byte, 16)
	if _, err := io.ReadFull(cryptorand.Reader, fileID); err != nil {
		return nil, fmt.Errorf("generate file ID: %w", err)
	}

	ownerPwd := cfg.ownerPassword
	if ownerPwd == "" {
		ownerPwd = cfg.userPassword
	}

	oEntry := computeOwnerEntry(cfg.userPassword, ownerPwd)
	key := computeEncKey(cfg.userPassword, oEntry, encryptPermissions, fileID)
	uEntry := computeUserEntry(key, fileID)

	return &encryptState{
		key:        key,
		fileID:     fileID,
		ownerEntry: oEntry,
		userEntry:  uEntry,
	}, nil
}

// padPassword pads or truncates s to exactly 32 bytes using the PDF spec padding string.
func padPassword(s string) []byte {
	out := make([]byte, 32)
	n := copy(out, s)
	copy(out[n:], passwordPadBytes[n:])
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
	h := md5.New()
	h.Write(padPassword(userPwd))
	h.Write(ownerEntry)
	var p [4]byte
	binary.LittleEndian.PutUint32(p[:], uint32(permissions))
	h.Write(p[:])
	h.Write(fileID)
	key := h.Sum(nil)[:encKeyLen]
	// For R=3: 50 additional MD5 passes.
	for i := 0; i < 50; i++ {
		s := md5.Sum(key)
		key = s[:encKeyLen]
	}
	return key
}

// computeUserEntry computes the /U encryption dict entry per PDF Algorithm 5 (R=3).
func computeUserEntry(encKey, fileID []byte) []byte {
	h := md5.New()
	h.Write(passwordPadBytes[:])
	h.Write(fileID)
	result := h.Sum(nil) // 16 bytes
	applyRC4(result, encKey)
	for i := 1; i <= 19; i++ {
		applyRC4(result, xorKey(encKey, byte(i)))
	}
	return append(result, make([]byte, 16)...) // pad to 32 bytes
}

// encryptBytes encrypts (or decrypts — RC4 is symmetric) data for the given object number.
func (s *encryptState) encryptBytes(newObjNum int, data []byte) []byte {
	key := s.objectKey(newObjNum)
	result := make([]byte, len(data))
	copy(result, data)
	applyRC4(result, key)
	return result
}

// objectKey derives the per-object RC4 key per PDF Algorithm 1.
// Generation number is always 0 in our writer.
func (s *encryptState) objectKey(objNum int) []byte {
	h := md5.New()
	h.Write(s.key)
	h.Write([]byte{byte(objNum), byte(objNum >> 8), byte(objNum >> 16), 0, 0})
	out := h.Sum(nil)
	keyLen := encKeyLen + 5
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
func verifyUserPassword(password string, ownerEntry, storedU, fileID []byte) bool {
	key := computeEncKey(password, ownerEntry, encryptPermissions, fileID)
	computed := computeUserEntry(key, fileID)
	if len(computed) < 16 || len(storedU) < 16 {
		return false
	}
	for i := 0; i < 16; i++ {
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
