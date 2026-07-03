// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/md5"
	"errors"
	"fmt"
)

// ErrEncrypted is returned by Open / OpenStream when the input PDF carries
// an /Encrypt dictionary. Callers should retry with OpenWithPassword or
// OpenStreamWithPassword to supply a user or owner password.
var ErrEncrypted = errors.New("PDF is encrypted; use OpenWithPassword")

// buildDecryptState parses an /Encrypt dict and returns the per-document
// encryption state for decryption. Dispatches by /V and /R: V=2 R=3 →
// RC4-128 Standard Security Handler; V=4 R=4 → AES-128 via /CFM /AESV2.
func buildDecryptState(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
	filter := dictGetName(encDict, "/Filter")
	if filter != "/Standard" {
		return nil, fmt.Errorf("unsupported /Filter %q (only /Standard is implemented)", filter)
	}
	v := dictGetInt(encDict, "/V")
	r := dictGetInt(encDict, "/R")
	switch {
	case (v == 1 || v == 0) && r == 2:
		// Original Standard Security Handler: 40-bit RC4 (5-byte key).
		return buildDecryptStateRC4(encDict, trailer, password, 2, 5)
	case v == 2 && r == 3:
		// RC4 with a key length given by /Length (default 128-bit).
		keyLen := dictGetInt(encDict, "/Length") / 8
		if keyLen <= 0 || keyLen > 16 {
			keyLen = 16
		}
		return buildDecryptStateRC4(encDict, trailer, password, 3, keyLen)
	case v == 4 && r == 4:
		return buildDecryptStateV4R4(encDict, trailer, password)
	case v == 5 && r == 6:
		return buildDecryptStateV5R6(encDict, password) // trailer/ID not used for V=5 R=6
	default:
		return nil, fmt.Errorf("unsupported security handler V=%d R=%d", v, r)
	}
}

// buildDecryptStateV2R3 is retained for the AES-128 (V=4 R=4) password
// path, which reuses the RC4 R=3 / 16-byte-key Algorithms 2/5/7.
func buildDecryptStateV2R3(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
	return buildDecryptStateRC4(encDict, trailer, password, 3, 16)
}

// buildDecryptStateRC4 parses the /Encrypt dict and the trailer's /ID array,
// verifies the supplied password against /U (user) or /O (owner) using the
// revision-r algorithms with a keyLen-byte key, and returns an encryptState
// whose document key is ready to derive per-object keys for decryption.
// r is 2 (original 40-bit handler) or 3 (RC4 with /Length-bit key).
func buildDecryptStateRC4(encDict pdfDict, trailer pdfDict, password string, r, keyLen int) (*encryptState, error) {
	filter := dictGetName(encDict, "/Filter")
	if filter != "/Standard" {
		return nil, fmt.Errorf("unsupported /Filter %q (only /Standard is implemented)", filter)
	}

	oVal, ok := encDict["/O"]
	if !ok {
		return nil, fmt.Errorf("/Encrypt dict missing /O")
	}
	uVal, ok := encDict["/U"]
	if !ok {
		return nil, fmt.Errorf("/Encrypt dict missing /U")
	}
	pVal, ok := encDict["/P"]
	if !ok {
		return nil, fmt.Errorf("/Encrypt dict missing /P")
	}

	oBytes, err := pdfStringBytes(oVal)
	if err != nil {
		return nil, fmt.Errorf("/O: %w", err)
	}
	uBytes, err := pdfStringBytes(uVal)
	if err != nil {
		return nil, fmt.Errorf("/U: %w", err)
	}
	permissions := int32(uint32(toInt(pVal)))

	idVal, ok := trailer["/ID"]
	if !ok {
		return nil, fmt.Errorf("trailer missing /ID (required for encryption)")
	}
	idArr, ok := idVal.(pdfArray)
	if !ok || len(idArr) == 0 {
		return nil, fmt.Errorf("trailer /ID is not a non-empty array")
	}
	fileID, err := pdfStringBytes(idArr[0])
	if err != nil {
		return nil, fmt.Errorf("/ID[0]: %w", err)
	}

	// Try user password first; fall back to owner password.
	if verifyUserPasswordR(password, oBytes, uBytes, fileID, permissions, r, keyLen) {
		key := computeEncKeyR(password, oBytes, permissions, fileID, r, keyLen)
		return &encryptState{
			algorithm:   EncryptionAlgRC4_128,
			key:         key,
			fileID:      fileID,
			ownerEntry:  oBytes,
			userEntry:   uBytes,
			permissions: permissions,
		}, nil
	}

	if userPwd, ok := recoverUserPasswordFromOwnerR(password, oBytes, r, keyLen); ok {
		if verifyUserPasswordR(userPwd, oBytes, uBytes, fileID, permissions, r, keyLen) {
			key := computeEncKeyR(userPwd, oBytes, permissions, fileID, r, keyLen)
			return &encryptState{
				algorithm:   EncryptionAlgRC4_128,
				key:         key,
				fileID:      fileID,
				ownerEntry:  oBytes,
				userEntry:   uBytes,
				permissions: permissions,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid password")
}

// recoverUserPasswordFromOwnerR is the revision-aware form of
// recoverUserPasswordFromOwner. For r==2 the owner key is a single MD5 of
// the padded owner password (keyLen bytes) and the padded user password is
// recovered with a single RC4 pass; for r>=3 the owner key takes 50 extra
// MD5 rounds and the 20 XOR'd RC4 rounds are inverted.
func recoverUserPasswordFromOwnerR(ownerPwd string, oEntry []byte, r, keyLen int) (string, bool) {
	if len(oEntry) != 32 {
		return "", false
	}
	sum := md5.Sum(padPassword(ownerPwd))
	key := sum[:]
	if r >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(key[:keyLen])
			key = s[:]
		}
	}
	ownerKey := key[:keyLen]

	result := make([]byte, len(oEntry))
	copy(result, oEntry)
	if r == 2 {
		applyRC4(result, ownerKey)
	} else {
		// Invert the 20 RC4 rounds applied to the padded user password.
		for i := 19; i >= 1; i-- {
			applyRC4(result, xorKey(ownerKey, byte(i)))
		}
		applyRC4(result, ownerKey)
	}
	// `result` is now the padded user password. Strip trailing pad bytes by
	// finding the first byte from the end of the password padding constant
	// that doesn't match — but since verifyUserPassword re-pads the input,
	// we need the original user password (or any prefix that re-pads to the
	// same 32 bytes). Returning `string(result)` after stripping pad works
	// for ASCII; for full fidelity we just return the 32-byte padded form
	// — verifyUserPassword's padPassword on a 32-byte string truncates to
	// the same 32 bytes, so the comparison succeeds.
	return string(result), true
}

// pdfStringBytes returns the raw bytes of a PDF string value parsed from
// the file. The parser stores both literal-string and hex-string forms as
// Go strings holding the decoded bytes verbatim.
func pdfStringBytes(v pdfValue) ([]byte, error) {
	switch s := v.(type) {
	case string:
		return []byte(s), nil
	case pdfHexString:
		return []byte(s), nil
	}
	return nil, fmt.Errorf("expected PDF string, got %T", v)
}

// decryptObject mutates obj's value tree in place: every string is
// decrypted via the appropriate per-object cipher (RC4 for V=2 R=3;
// AES-128-CBC for V=4 R=4; AES-256-CBC for V=5 R=6), and every stream's raw Data is decrypted
// then decoded via the /Filter chain. The /Encrypt dict itself is
// never decrypted by this function — callers must skip it.
func decryptObject(obj *pdfObject, state *encryptState) error {
	switch state.algorithm {
	case EncryptionAlgRC4_128:
		key := state.objectKey(obj.Num)
		obj.Value = decryptValue(obj.Value, key)
		return nil
	case EncryptionAlgAES128:
		return decryptObjectTreeAES128(obj, state)
	case EncryptionAlgAES256:
		return decryptObjectTreeAES256(obj, state)
	}
	return fmt.Errorf("decryptObject: unknown algorithm %d", state.algorithm)
}

func decryptValue(v pdfValue, key []byte) pdfValue {
	switch val := v.(type) {
	case string:
		return decryptString(val, key)
	case pdfHexString:
		return pdfHexString(rc4Decrypted([]byte(val), key))
	case pdfDict:
		sig := isSignatureDict(val)
		for k, vv := range val {
			if sig && k == "/Contents" {
				continue // signature /Contents is never encrypted (ISO 32000-1 §7.6.2)
			}
			val[k] = decryptValue(vv, key)
		}
		return val
	case pdfArray:
		for i, vv := range val {
			val[i] = decryptValue(vv, key)
		}
		return val
	case *pdfStream:
		decryptStreamInPlace(val, key)
		return val
	}
	return v
}

// isSignatureDict reports whether d is a signature (or document-timestamp)
// dictionary, whose /Contents value is exempt from encryption per ISO
// 32000-1 §7.6.2. A /ByteRange alongside /Contents is the reliable marker.
func isSignatureDict(d pdfDict) bool {
	if _, ok := d["/ByteRange"]; !ok {
		return false
	}
	_, ok := d["/Contents"]
	return ok
}

func decryptString(s string, key []byte) string {
	return string(rc4Decrypted([]byte(s), key))
}

func rc4Decrypted(in, key []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	applyRC4(out, key)
	return out
}

// decryptStreamInPlace decrypts a stream's Data and re-runs the /Filter
// decode chain. The parser tries to decode at parse time; on encrypted
// data this almost always fails (RC4 output is not a valid zlib/ASCIIHex/
// ASCII85 stream) and the parser preserves raw bytes with Decoded=false.
// We rely on that path: decrypt the raw bytes, then decode.
//
// If a stream came back already Decoded=true, that means decode succeeded
// on encrypted bytes — extraordinarily unlikely in practice. We treat
// Data as already-clean and return it untouched, since we no longer have
// the original encrypted bytes to recover from.
func decryptStreamInPlace(s *pdfStream, key []byte) {
	if s.Decoded {
		return
	}
	s.Data = rc4Decrypted(s.Data, key)
	if decoded, err := decodeStream(s.Dict, s.Data); err == nil {
		s.Data = decoded
		s.Decoded = true
	}
}
