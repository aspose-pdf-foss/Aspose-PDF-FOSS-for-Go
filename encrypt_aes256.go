package asposepdf

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"io"
)

// hashV5R6 computes the Algorithm 2.B hash per ISO 32000-2 §7.6.4.3.4.
// The result is always the first 32 bytes of K after the iteration
// terminates. extra is empty (nil) for /U hash computation, and the
// 48-byte /U entry for /O hash computation. password is the user's
// or owner's password as raw UTF-8 bytes (no SASLprep).
func hashV5R6(password, salt, extra []byte) []byte {
	// Step 1: initial K = SHA-256(password || salt || extra)
	h := sha256.New()
	h.Write(password)
	h.Write(salt)
	h.Write(extra)
	K := h.Sum(nil) // 32 bytes

	for round := 0; ; round++ {
		// Step a: K1 = 64 × (password || K || extra)
		blockLen := len(password) + len(K) + len(extra)
		K1 := make([]byte, 64*blockLen)
		for i := 0; i < 64; i++ {
			off := i * blockLen
			copy(K1[off:], password)
			copy(K1[off+len(password):], K)
			copy(K1[off+len(password)+len(K):], extra)
		}

		// Step b: AES-128-CBC encrypt K1 using K[0:16] as key, K[16:32] as IV.
		// K is always 32+ bytes from the SHA-2 family.
		block, _ := aes.NewCipher(K[0:16])
		E := make([]byte, len(K1))
		cipher.NewCBCEncrypter(block, K[16:32]).CryptBlocks(E, K1)

		// Step c: val = sum(E[0:16]) mod 3.
		// Math: 256 ≡ 1 (mod 3), so big-endian-int(E[0:16]) mod 3 ==
		// sum(E[0:16]) mod 3. Documented in the design spec.
		var sum uint32
		for _, b := range E[0:16] {
			sum += uint32(b)
		}
		val := sum % 3

		// Step d: K = SHA-{256,384,512}(E)
		switch val {
		case 0:
			sum256 := sha256.Sum256(E)
			K = sum256[:]
		case 1:
			sum384 := sha512.Sum384(E)
			K = sum384[:]
		case 2:
			sum512 := sha512.Sum512(E)
			K = sum512[:]
		}

		// Step e+f: termination — minimum 64 rounds, then E_last <= round - 32.
		if round >= 64 && int(E[len(E)-1]) <= round-32 {
			break
		}
	}
	return K[0:32]
}

// encryptBytesAES256 encrypts plaintext under the document's File
// Encryption Key (FEK) using AES-256-CBC with PKCS#7 padding and a
// random 16-byte IV prepended. V=5 R=6 has no per-object key derivation
// (unlike V≤4 Algorithm 1/1.A) — every string and stream uses the FEK
// directly. ISO 32000-2 §7.6.4.6.
func encryptBytesAES256(s *encryptState, plaintext []byte) ([]byte, error) {
	if len(s.key) != 32 {
		return nil, fmt.Errorf("AES-256 requires 32-byte key, got %d bytes", len(s.key))
	}
	padded := addPKCS7(plaintext, aes.BlockSize)
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(cryptorand.Reader, iv); err != nil {
		return nil, fmt.Errorf("AES-256 IV: %w", err)
	}
	block, err := aes.NewCipher(s.key) // 32-byte key → AES-256
	if err != nil {
		return nil, fmt.Errorf("AES-256 NewCipher: %w", err)
	}
	body := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(body, padded)
	out := make([]byte, len(iv)+len(body))
	copy(out, iv)
	copy(out[len(iv):], body)
	return out, nil
}

// buildPermsBlock constructs the 16-byte permissions block per ISO
// 32000-2 §7.6.4.6.2. The block is later AES-256-ECB encrypted under
// the FEK and stored as /Perms for tamper-detection.
//
// Layout:
//   bytes 0-3:   /P (little-endian, signed-32 cast to unsigned-32 bytes)
//   bytes 4-7:   0xFF 0xFF 0xFF 0xFF (high 32 bits of permissions, all 1s)
//   byte  8:     'T' if encryptMetadata; 'F' otherwise
//   bytes 9-11:  'a', 'd', 'b' (marker proving decrypt produced valid output)
//   bytes 12-15: 4 random bytes (entropy/padding)
func buildPermsBlock(permissions int32, encryptMetadata bool) []byte {
	out := make([]byte, 16)
	p := uint32(permissions)
	out[0] = byte(p)
	out[1] = byte(p >> 8)
	out[2] = byte(p >> 16)
	out[3] = byte(p >> 24)
	out[4], out[5], out[6], out[7] = 0xFF, 0xFF, 0xFF, 0xFF
	if encryptMetadata {
		out[8] = 'T'
	} else {
		out[8] = 'F'
	}
	out[9], out[10], out[11] = 'a', 'd', 'b'
	// Best-effort random bytes for the last 4. crypto/rand failure is
	// exceedingly unlikely; falling through with zero bytes is acceptable.
	_, _ = io.ReadFull(cryptorand.Reader, out[12:16])
	return out
}
