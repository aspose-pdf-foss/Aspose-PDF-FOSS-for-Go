// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
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

// buildDecryptStateV5R6 parses a /V=5 /R=6 /Encrypt dict, validates the
// /CF/StdCF/CFM=/AESV3 structure, validates the password against /U
// (user) or /O (owner), recovers the FEK from /UE or /OE, and verifies
// the /Perms tamper-detection block. Per ISO 32000-2 §7.6.4.
func buildDecryptStateV5R6(encDict pdfDict, password string) (*encryptState, error) {
	// 1. Validate /CF/StdCF/CFM == /AESV3.
	cfRaw, ok := encDict["/CF"].(pdfDict)
	if !ok {
		return nil, fmt.Errorf("V=5 R=6: /CF dict missing")
	}
	stmName, _ := encDict["/StmF"].(pdfName)
	if stmName == "" {
		return nil, fmt.Errorf("V=5 R=6: /StmF missing")
	}
	if strName, _ := encDict["/StrF"].(pdfName); strName != "" && strName != stmName {
		return nil, fmt.Errorf("V=5 R=6: /StmF and /StrF differ — unsupported")
	}
	cfEntry, ok := cfRaw[string(stmName)].(pdfDict)
	if !ok {
		return nil, fmt.Errorf("V=5 R=6: /CF/%s missing", stmName)
	}
	cfm, _ := cfEntry["/CFM"].(pdfName)
	if cfm != "/AESV3" {
		return nil, fmt.Errorf("V=5 R=6: unsupported /CFM %q (want /AESV3)", cfm)
	}

	// 2. Read /U, /O, /UE, /OE, /Perms — all required with exact lengths.
	U, err := readBytesEntryExact(encDict, "/U", 48)
	if err != nil {
		return nil, err
	}
	O, err := readBytesEntryExact(encDict, "/O", 48)
	if err != nil {
		return nil, err
	}
	UE, err := readBytesEntryExact(encDict, "/UE", 32)
	if err != nil {
		return nil, err
	}
	OE, err := readBytesEntryExact(encDict, "/OE", 32)
	if err != nil {
		return nil, err
	}
	permsEnc, err := readBytesEntryExact(encDict, "/Perms", 16)
	if err != nil {
		return nil, err
	}

	pVal, ok := encDict["/P"]
	if !ok {
		return nil, fmt.Errorf("V=5 R=6: /P missing")
	}
	permissions := int32(uint32(toInt(pVal)))

	// 3. Try user password.
	pwBytes := []byte(password)
	fek, ok := tryUserPasswordV5R6(pwBytes, U, UE)
	if !ok {
		// 4. Try owner password.
		fek, ok = tryOwnerPasswordV5R6(pwBytes, U, O, OE)
	}
	if !ok {
		return nil, fmt.Errorf("invalid password")
	}

	// 5. Verify /Perms tamper-detection.
	if err := verifyPermsV5R6(fek, permsEnc, permissions); err != nil {
		return nil, err
	}

	return &encryptState{
		algorithm:     EncryptionAlgAES256,
		key:           fek,
		userEntry:     U,
		ownerEntry:    O,
		userKeyEntry:  UE,
		ownerKeyEntry: OE,
		permsEntry:    permsEnc,
		permissions:   permissions,
	}, nil
}

// tryUserPasswordV5R6 validates pwd against /U and decrypts /UE to
// recover the FEK on success. Returns (fek, true) if matched.
func tryUserPasswordV5R6(pwd, U, UE []byte) ([]byte, bool) {
	storedHash := U[0:32]
	validSalt := U[32:40]
	keySalt := U[40:48]
	if !bytes.Equal(hashV5R6(pwd, validSalt, nil), storedHash) {
		return nil, false
	}
	return unwrapFEK(hashV5R6(pwd, keySalt, nil), UE)
}

// tryOwnerPasswordV5R6 validates pwd against /O (which incorporates the
// full /U entry) and decrypts /OE to recover the FEK.
func tryOwnerPasswordV5R6(pwd, U, O, OE []byte) ([]byte, bool) {
	storedHash := O[0:32]
	validSalt := O[32:40]
	keySalt := O[40:48]
	if !bytes.Equal(hashV5R6(pwd, validSalt, U), storedHash) {
		return nil, false
	}
	return unwrapFEK(hashV5R6(pwd, keySalt, U), OE)
}

// unwrapFEK decrypts a 32-byte AES-256-CBC ciphertext (no padding,
// 16-byte zero IV) under wrappingKey to recover the FEK.
func unwrapFEK(wrappingKey, ciphertext []byte) ([]byte, bool) {
	if len(ciphertext) != 32 {
		return nil, false
	}
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, false
	}
	fek := make([]byte, 32)
	iv := make([]byte, 16) // 16 zero bytes per spec
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(fek, ciphertext)
	return fek, true
}

// readBytesEntryExact reads a /Encrypt dict entry as raw bytes and
// requires the length to match exactly.
func readBytesEntryExact(dict pdfDict, key string, wantLen int) ([]byte, error) {
	v, ok := dict[key]
	if !ok {
		return nil, fmt.Errorf("V=5 R=6: %s missing", key)
	}
	b, err := pdfStringBytes(v)
	if err != nil {
		return nil, fmt.Errorf("V=5 R=6: %s: %w", key, err)
	}
	if len(b) != wantLen {
		return nil, fmt.Errorf("V=5 R=6: %s length = %d, want %d", key, len(b), wantLen)
	}
	return b, nil
}

// decryptObjectTreeAES256 walks obj's value tree, AES-256-decrypting
// every string and stream payload with the FEK held in state.key.
// V=5 R=6 has no per-object key derivation — object number and gen
// are not used.
func decryptObjectTreeAES256(obj *pdfObject, state *encryptState) error {
	decrypt := func(b []byte) ([]byte, error) {
		return decryptObjectAES256(state, b)
	}
	newVal, err := decryptValueAES256(obj.Value, decrypt)
	if err != nil {
		return err
	}
	obj.Value = newVal
	return nil
}

func decryptValueAES256(v pdfValue, decrypt func([]byte) ([]byte, error)) (pdfValue, error) {
	switch val := v.(type) {
	case string:
		plain, err := decrypt([]byte(val))
		if err != nil {
			return nil, err
		}
		return string(plain), nil
	case pdfHexString:
		plain, err := decrypt([]byte(val))
		if err != nil {
			return nil, err
		}
		return pdfHexString(plain), nil
	case pdfDict:
		sig := isSignatureDict(val)
		for k, vv := range val {
			if sig && k == "/Contents" {
				continue // signature /Contents is never encrypted (ISO 32000-1 §7.6.2)
			}
			nv, err := decryptValueAES256(vv, decrypt)
			if err != nil {
				return nil, err
			}
			val[k] = nv
		}
		return val, nil
	case pdfArray:
		for i, vv := range val {
			nv, err := decryptValueAES256(vv, decrypt)
			if err != nil {
				return nil, err
			}
			val[i] = nv
		}
		return val, nil
	case *pdfStream:
		if err := decryptStreamAES256(val, decrypt); err != nil {
			return nil, err
		}
		return val, nil
	}
	return v, nil
}

func decryptStreamAES256(s *pdfStream, decrypt func([]byte) ([]byte, error)) error {
	if s.Decoded {
		return nil
	}
	plain, err := decrypt(s.Data)
	if err != nil {
		return err
	}
	s.Data = plain
	if decoded, derr := decodeStream(s.Dict, s.Data); derr == nil {
		s.Data = decoded
		s.Decoded = true
	}
	return nil
}
