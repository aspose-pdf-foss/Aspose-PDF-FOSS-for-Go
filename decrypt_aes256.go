package asposepdf

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// decryptObjectAES256 is the inverse of encryptBytesAES256. The first
// 16 bytes of ciphertext are the IV; the remainder is AES-256-CBC
// ciphertext of PKCS#7-padded plaintext under the FEK.
func decryptObjectAES256(s *encryptState, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("AES-256 ciphertext shorter than IV (%d bytes)", len(ciphertext))
	}
	iv := ciphertext[:aes.BlockSize]
	body := ciphertext[aes.BlockSize:]
	if len(body)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("AES-256 body not block-aligned (%d bytes)", len(body))
	}
	block, err := aes.NewCipher(s.key) // 32-byte key → AES-256
	if err != nil {
		return nil, fmt.Errorf("AES-256 NewCipher: %w", err)
	}
	plain := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, body)
	return stripPKCS7(plain)
}

// verifyPermsV5R6 decrypts the /Perms block and checks tamper-detection
// invariants: the 'adb' marker at bytes 9-11 must be present (proves
// the decrypt produced spec-shaped output), and the embedded /P must
// match the /P declared in the /Encrypt dict (defends against a /P
// modification by a third party). Per ISO 32000-2 §7.6.4.6.2.
//
// The /EncryptMetadata byte (8) is not strictly cross-checked here —
// producers (notably pypdf) may write inconsistent values; lenient
// read avoids spurious failures.
func verifyPermsV5R6(fek, permsEnc []byte, declaredP int32) error {
	if len(permsEnc) != 16 {
		return fmt.Errorf("/Perms length = %d, want 16", len(permsEnc))
	}
	block, err := aes.NewCipher(fek)
	if err != nil {
		return fmt.Errorf("/Perms decrypt NewCipher: %w", err)
	}
	decoded := make([]byte, 16)
	block.Decrypt(decoded, permsEnc) // single-block ECB

	if decoded[9] != 'a' || decoded[10] != 'd' || decoded[11] != 'b' {
		return fmt.Errorf("/Perms tampered: marker %q%q%q",
			decoded[9], decoded[10], decoded[11])
	}
	permsP := int32(uint32(decoded[0]) | uint32(decoded[1])<<8 |
		uint32(decoded[2])<<16 | uint32(decoded[3])<<24)
	if permsP != declaredP {
		return fmt.Errorf("/Perms tampered: P=%d in block vs %d in dict", permsP, declaredP)
	}
	return nil
}
