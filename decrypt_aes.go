// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// stripPKCS7 removes PKCS#7 padding from data and returns the unpadded
// bytes. data must be a positive multiple of aes.BlockSize. The final
// byte indicates the pad length (1..16); all pad bytes must equal that
// value or an error is returned.
func stripPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("PKCS#7: bad length %d", len(data))
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil, fmt.Errorf("PKCS#7: invalid pad byte %d", pad)
	}
	for i := len(data) - pad; i < len(data); i++ {
		if data[i] != byte(pad) {
			return nil, fmt.Errorf("PKCS#7: malformed padding at offset %d", i)
		}
	}
	return data[:len(data)-pad], nil
}

// decryptObjectAES128 is the inverse of encryptBytesAES128. The first
// 16 bytes of ciphertext are the IV; the remainder is AES-128-CBC
// ciphertext of PKCS#7-padded plaintext under the per-object key.
func decryptObjectAES128(s *encryptState, objNum, gen int, ciphertext []byte) ([]byte, error) {
	key := objectKeyAES128(s.key, objNum, gen)
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("AES ciphertext shorter than IV (%d bytes)", len(ciphertext))
	}
	iv := ciphertext[:aes.BlockSize]
	body := ciphertext[aes.BlockSize:]
	if len(body)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("AES ciphertext body not block-aligned (%d bytes)", len(body))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, body)
	return stripPKCS7(plain)
}

// buildDecryptStateV4R4 parses a /V=4 /R=4 /Encrypt dict and validates
// that the crypt filter referenced by /StmF and /StrF uses /CFM /AESV2.
// Per ISO 32000-1 §7.6.3.2 / §7.6.5. Passwords are verified via the
// same Algorithms 2/5/7 as V=2 R=3 (delegated to buildDecryptStateV2R3
// after we swap V to 2 for the recursion — see implementation note).
func buildDecryptStateV4R4(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
	cfRaw, ok := encDict["/CF"].(pdfDict)
	if !ok {
		return nil, fmt.Errorf("V=4 R=4: /CF dict missing")
	}
	stmName, _ := encDict["/StmF"].(pdfName)
	if stmName == "" {
		return nil, fmt.Errorf("V=4 R=4: /StmF missing")
	}
	if strName, _ := encDict["/StrF"].(pdfName); strName != "" && strName != stmName {
		return nil, fmt.Errorf("V=4 R=4: /StmF and /StrF differ — unsupported")
	}
	cfEntry, ok := cfRaw[string(stmName)].(pdfDict)
	if !ok {
		return nil, fmt.Errorf("V=4 R=4: /CF/%s entry missing or wrong type", stmName)
	}
	cfm, _ := cfEntry["/CFM"].(pdfName)
	if cfm != "/AESV2" {
		return nil, fmt.Errorf("V=4 R=4: unsupported /CFM %q (want /AESV2)", cfm)
	}

	// Algorithms 2/5/7 are identical between V=2 R=3 and V=4 R=4.
	// Make a shallow copy of encDict with /V=2 /R=3 so the existing
	// V2R3 implementation can verify the password without rejecting V=4.
	fake := make(pdfDict, len(encDict))
	for k, v := range encDict {
		fake[k] = v
	}
	fake["/V"] = 2
	fake["/R"] = 3
	state, err := buildDecryptStateV2R3(fake, trailer, password)
	if err != nil {
		return nil, err
	}
	state.algorithm = EncryptionAlgAES128
	return state, nil
}

// decryptObjectTreeAES128 walks obj's value tree, AES-decrypting every
// string and stream payload with the per-object AES-128 key derived
// from obj.Num and obj.Gen.
func decryptObjectTreeAES128(obj *pdfObject, state *encryptState) error {
	decrypt := func(b []byte) ([]byte, error) {
		return decryptObjectAES128(state, obj.Num, obj.Gen, b)
	}
	newVal, err := decryptValueAES128(obj.Value, decrypt)
	if err != nil {
		return err
	}
	obj.Value = newVal
	return nil
}

func decryptValueAES128(v pdfValue, decrypt func([]byte) ([]byte, error)) (pdfValue, error) {
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
		for k, vv := range val {
			nv, err := decryptValueAES128(vv, decrypt)
			if err != nil {
				return nil, err
			}
			val[k] = nv
		}
		return val, nil
	case pdfArray:
		for i, vv := range val {
			nv, err := decryptValueAES128(vv, decrypt)
			if err != nil {
				return nil, err
			}
			val[i] = nv
		}
		return val, nil
	case *pdfStream:
		if err := decryptStreamAES128(val, decrypt); err != nil {
			return nil, err
		}
		return val, nil
	}
	return v, nil
}

func decryptStreamAES128(s *pdfStream, decrypt func([]byte) ([]byte, error)) error {
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
