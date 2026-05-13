package asposepdf

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/sha512"
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
