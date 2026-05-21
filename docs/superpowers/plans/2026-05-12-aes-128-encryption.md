# AES-128 Encryption Implementation Plan (Subepic 1 of `pdf-go-ccl`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship AES-128 (Standard Security Handler V=4 R=4, `/CFM /AESV2`) — read + write — alongside the existing RC4-128 V=2 R=3 path, with AES-128 becoming the new default for `(*Document).SetEncryption`.

**Architecture:** Reuse PDF Algorithms 2/3/5/7 from RC4 verbatim. Add Algorithm 1.A (`"sAlT"`-suffixed per-object key) plus AES-128-CBC with PKCS#7 padding. `EncryptionAlgorithm` enum on `EncryptionOptions` selects the cipher; encrypt/decrypt paths dispatch on it. `/Encrypt` dict gains `/V=4 /R=4 /CF/StdCF/CFM=/AESV2 /StmF=/StdCF /StrF=/StdCF` for AES output.

**Tech Stack:** Go 1.24, standard library only (`crypto/aes`, `crypto/cipher`, `crypto/md5`, `crypto/rand`). pypdf 6.x for cross-tool verification (Task 15 only).

**Reference:** [docs/superpowers/specs/2026-05-12-aes-128-encryption-design.md](../specs/2026-05-12-aes-128-encryption-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `encrypt.go` (modify) | Add `EncryptionAlgorithm` enum, extend `EncryptionOptions`, extend `encryptConfig`/`encryptState`, dispatcher in `encryptBytes` |
| `encrypt_aes.go` (new) | `encryptBytesAES128`, `objectKeyAES128`, `addPKCS7` |
| `encrypt_aes_internal_test.go` (new) | Internal tests: PKCS7, object key vectors, encrypt roundtrip, IV randomness |
| `decrypt.go` (modify) | `buildDecryptState` dispatcher by `/V`; `decryptObject` dispatcher in `getObject` |
| `decrypt_aes.go` (new) | `buildDecryptStateV4R4`, `decryptObjectAES128`, `stripPKCS7` |
| `decrypt_aes_internal_test.go` (new) | Internal tests: stripPKCS7 edges, decrypt edges, /CF validation |
| `writer.go` (modify) | `encFn` signature `func([]byte) ([]byte, error)`; write `/Encrypt` V=4 dict for AES path |
| `encrypt_aes_test.go` (new) | External end-to-end tests: AES roundtrip, defaults, RC4-explicit, permissions, wrong password, owner recovery |
| `encrypt_aes_integration_test.go` (new) | FileAttachment + AcroForm coexistence under AES |
| `encrypt_aes_pypdf_test.go` (new) | Cross-tool tests against pypdf 6.x |
| `CLAUDE.md`, `README.md` (modify, Task 16) | Public API docs + new-default note |

---

## Task 1: EncryptionAlgorithm enum + EncryptionOptions field

**Files:**
- Modify: `encrypt.go`
- Create: `encryption_algorithm_test.go`

- [ ] **Step 1: Write the failing test**

Create `encryption_algorithm_test.go`:
```go
package asposepdf_test

import (
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestEncryptionAlgorithmConstants(t *testing.T) {
    if int(pdf.EncryptionAlgAES128) != 0 {
        t.Errorf("EncryptionAlgAES128 = %d, want 0 (zero value default)", int(pdf.EncryptionAlgAES128))
    }
    if int(pdf.EncryptionAlgRC4_128) != 1 {
        t.Errorf("EncryptionAlgRC4_128 = %d, want 1", int(pdf.EncryptionAlgRC4_128))
    }
}

func TestEncryptionOptionsHasAlgorithmField(t *testing.T) {
    opts := pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgAES128,
    }
    if opts.Algorithm != pdf.EncryptionAlgAES128 {
        t.Errorf("Algorithm = %v", opts.Algorithm)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```powershell
go test -run TestEncryptionAlgorithm -v ./...
```
Expected: build failure (undefined types).

- [ ] **Step 3: Add the type + field**

In `encrypt.go`, after the `Permissions` constants block (around line 102), add:
```go
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
)
```

Update `EncryptionOptions` (encrypt.go:128) to add the field:
```go
type EncryptionOptions struct {
    UserPassword  string
    OwnerPassword string
    Permissions   *Permissions
    Algorithm     EncryptionAlgorithm // zero value = EncryptionAlgAES128
}
```

The field is added but not yet consumed by `SetEncryption` — that comes in Task 9.

- [ ] **Step 4: Run tests + commit**

```powershell
go test -run TestEncryptionAlgorithm -v ./...
go test ./...
git add encrypt.go encryption_algorithm_test.go
git commit -m "feat: EncryptionAlgorithm enum + Algorithm field on EncryptionOptions"
```

Expected: all green; no behaviour change yet — RC4 path unchanged.

---

## Task 2: PKCS#7 helpers (addPKCS7 + stripPKCS7)

**Files:**
- Create: `encrypt_aes.go`
- Create: `decrypt_aes.go`
- Create: `encrypt_aes_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Create `encrypt_aes_internal_test.go`:
```go
package asposepdf

import (
    "bytes"
    "crypto/aes"
    "testing"
)

func TestAddPKCS7_VariousLengths(t *testing.T) {
    cases := []struct {
        in       []byte
        wantPad  int
        wantTail []byte // last few bytes (pad value, repeated)
    }{
        {[]byte{}, 16, bytes.Repeat([]byte{16}, 16)},
        {[]byte{0x01}, 15, bytes.Repeat([]byte{15}, 15)},
        {[]byte{0x01, 0x02, 0x03}, 13, bytes.Repeat([]byte{13}, 13)},
        {bytes.Repeat([]byte{0x42}, 15), 1, []byte{1}},
        {bytes.Repeat([]byte{0x42}, 16), 16, bytes.Repeat([]byte{16}, 16)},
        {bytes.Repeat([]byte{0x42}, 17), 15, bytes.Repeat([]byte{15}, 15)},
    }
    for _, tc := range cases {
        got := addPKCS7(tc.in, aes.BlockSize)
        if len(got)%aes.BlockSize != 0 {
            t.Errorf("len(addPKCS7(%d-byte input)) = %d, not block-multiple", len(tc.in), len(got))
        }
        if len(got)-len(tc.in) != tc.wantPad {
            t.Errorf("addPKCS7(%d-byte input) added %d pad bytes, want %d",
                len(tc.in), len(got)-len(tc.in), tc.wantPad)
        }
        tail := got[len(got)-len(tc.wantTail):]
        if !bytes.Equal(tail, tc.wantTail) {
            t.Errorf("addPKCS7 trailing bytes = %v, want %v", tail, tc.wantTail)
        }
    }
}

func TestStripPKCS7_RoundTrip(t *testing.T) {
    inputs := [][]byte{
        {},
        {0x01},
        {0x01, 0x02, 0x03},
        bytes.Repeat([]byte{0x42}, 15),
        bytes.Repeat([]byte{0x42}, 16),
        bytes.Repeat([]byte{0x42}, 100),
    }
    for _, in := range inputs {
        padded := addPKCS7(in, aes.BlockSize)
        out, err := stripPKCS7(padded)
        if err != nil {
            t.Errorf("stripPKCS7 on padded %d-byte input: %v", len(in), err)
            continue
        }
        if !bytes.Equal(in, out) {
            t.Errorf("roundtrip differs for %d-byte input: got %v, want %v", len(in), out, in)
        }
    }
}

func TestStripPKCS7_InvalidPadding(t *testing.T) {
    cases := []struct {
        name string
        data []byte
    }{
        {"empty", []byte{}},
        {"not block-aligned", bytes.Repeat([]byte{0x01}, 15)},
        {"pad byte zero", append(bytes.Repeat([]byte{0x42}, 15), 0)},
        {"pad byte too large", append(bytes.Repeat([]byte{0x42}, 15), 17)},
        {"mismatched pad bytes", append(bytes.Repeat([]byte{0x42}, 13), 3, 3, 4)},
    }
    for _, tc := range cases {
        if _, err := stripPKCS7(tc.data); err == nil {
            t.Errorf("%s: expected error, got nil", tc.name)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```powershell
go test -run 'TestAddPKCS7|TestStripPKCS7' -v ./...
```
Expected: build failure.

- [ ] **Step 3: Implement addPKCS7 in `encrypt_aes.go`**

Create `encrypt_aes.go`:
```go
package asposepdf

// addPKCS7 appends PKCS#7 padding to data. The padding length is always
// in 1..blockSize (even when len(data) is already a multiple of blockSize,
// a full block of padding is appended) per RFC 5652 §6.3.
func addPKCS7(data []byte, blockSize int) []byte {
    pad := blockSize - (len(data) % blockSize)
    out := make([]byte, len(data)+pad)
    copy(out, data)
    for i := len(data); i < len(out); i++ {
        out[i] = byte(pad)
    }
    return out
}
```

- [ ] **Step 4: Implement stripPKCS7 in `decrypt_aes.go`**

Create `decrypt_aes.go`:
```go
package asposepdf

import (
    "crypto/aes"
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
```

- [ ] **Step 5: Run tests + commit**

```powershell
go test -run 'TestAddPKCS7|TestStripPKCS7' -v ./...
go test ./...
git add encrypt_aes.go decrypt_aes.go encrypt_aes_internal_test.go
git commit -m "feat: PKCS#7 padding helpers (addPKCS7 + stripPKCS7)"
```

---

## Task 3: objectKeyAES128 (Algorithm 1.A)

**Files:**
- Modify: `encrypt_aes.go`
- Modify: `encrypt_aes_internal_test.go`

- [ ] **Step 1: Write the failing test (with known reference vector)**

Append to `encrypt_aes_internal_test.go`:
```go
import "encoding/hex"

func TestObjectKeyAES128_KnownVector(t *testing.T) {
    // Reference vector computed offline:
    //   docKey = 16 bytes of 0xAB
    //   objNum = 0x010203, gen = 0x0405
    //   suffix = "sAlT" (literal, per ISO 32000-1 §7.6.2)
    //   key = MD5(docKey || objNum_LE_3 || gen_LE_2 || "sAlT")
    // Offline computation:
    //   import hashlib
    //   buf = bytes([0xAB]*16) + bytes([0x03, 0x02, 0x01, 0x05, 0x04]) + b"sAlT"
    //   print(hashlib.md5(buf).hexdigest())
    // → "5b1d8d09b73bda35e88066fb6824ac8e"
    docKey := bytes.Repeat([]byte{0xAB}, 16)
    got := objectKeyAES128(docKey, 0x010203, 0x0405)
    want, _ := hex.DecodeString("5b1d8d09b73bda35e88066fb6824ac8e")
    if !bytes.Equal(got, want) {
        t.Errorf("objectKeyAES128 = %x, want %x", got, want)
    }
    if len(got) != 16 {
        t.Errorf("objectKeyAES128 length = %d, want 16 for AES-128", len(got))
    }
}

func TestObjectKeyAES128_DiffersFromRC4Key(t *testing.T) {
    // AES key derivation appends "sAlT" before MD5 — must produce a
    // different output than the RC4 path for the same docKey/objNum/gen.
    docKey := bytes.Repeat([]byte{0x55}, 16)
    state := &encryptState{key: docKey}
    rc4Key := state.objectKey(42)
    aesKey := objectKeyAES128(docKey, 42, 0)
    if bytes.Equal(rc4Key, aesKey[:len(rc4Key)]) {
        t.Errorf("AES and RC4 keys must differ for the same input")
    }
}
```

(Note: this references `encryptState.objectKey` which is the existing RC4 method.)

- [ ] **Step 2: Run tests to verify they fail**

```powershell
go test -run TestObjectKeyAES128 -v ./...
```
Expected: build failure (undefined).

- [ ] **Step 3: Verify the reference vector before implementing**

Run a quick Python check to confirm the hardcoded hex in the test matches reality (one-off; document the command in the task notes):

```bash
python -c "import hashlib; print(hashlib.md5(bytes([0xAB]*16) + bytes([0x03,0x02,0x01,0x05,0x04]) + b'sAlT').hexdigest())"
```

The output must be `5b1d8d09b73bda35e88066fb6824ac8e`. If not, fix the test vector first.

- [ ] **Step 4: Implement objectKeyAES128**

Append to `encrypt_aes.go`. Add imports `"crypto/md5"`:
```go
// objectKeyAES128 derives the per-object AES-128 key per PDF Algorithm 1.A
// (ISO 32000-1 §7.6.2). The literal 4-byte "sAlT" suffix differentiates
// the key from the RC4 Algorithm 1 computation on the same document key.
func objectKeyAES128(docKey []byte, objNum, gen int) []byte {
    buf := make([]byte, 0, len(docKey)+5+4)
    buf = append(buf, docKey...)
    buf = append(buf,
        byte(objNum), byte(objNum>>8), byte(objNum>>16),
        byte(gen), byte(gen>>8),
        's', 'A', 'l', 'T')
    sum := md5.Sum(buf)
    return sum[:16] // full MD5 output for AES-128
}
```

- [ ] **Step 5: Run tests + commit**

```powershell
go test -run TestObjectKeyAES128 -v ./...
go test ./...
git add encrypt_aes.go encrypt_aes_internal_test.go
git commit -m "feat: objectKeyAES128 — per-object AES key derivation (PDF Algorithm 1.A)"
```

---

## Task 4: encryptBytesAES128

**Files:**
- Modify: `encrypt_aes.go`
- Modify: `encrypt_aes_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `encrypt_aes_internal_test.go`:
```go
func TestEncryptBytesAES128_RoundTrip(t *testing.T) {
    state := &encryptState{
        algorithm: EncryptionAlgAES128,
        key:       bytes.Repeat([]byte{0xCD}, 16),
    }
    inputs := [][]byte{
        []byte("Hello world"),
        []byte("a"),
        []byte(""),
        bytes.Repeat([]byte{0x42}, 1024),
    }
    for _, plain := range inputs {
        cipher, err := encryptBytesAES128(state, 42, 0, plain)
        if err != nil {
            t.Fatalf("encrypt: %v", err)
        }
        // Ciphertext must be at least one block of IV + at least one block of body.
        if len(cipher) < 2*aes.BlockSize {
            t.Errorf("ciphertext length %d < 32 (IV + min body)", len(cipher))
        }
        if len(cipher)%aes.BlockSize != 0 {
            t.Errorf("ciphertext length %d not block-aligned", len(cipher))
        }
        // Decrypt and confirm it matches.
        got, err := decryptObjectAES128(state, 42, 0, cipher)
        if err != nil {
            t.Fatalf("decrypt: %v", err)
        }
        if !bytes.Equal(got, plain) {
            t.Errorf("roundtrip differs: got %v, want %v", got, plain)
        }
    }
}

func TestEncryptBytesAES128_IVRandomness(t *testing.T) {
    state := &encryptState{
        algorithm: EncryptionAlgAES128,
        key:       bytes.Repeat([]byte{0xCD}, 16),
    }
    plain := []byte("identical input")
    c1, _ := encryptBytesAES128(state, 1, 0, plain)
    c2, _ := encryptBytesAES128(state, 1, 0, plain)
    if bytes.Equal(c1, c2) {
        t.Error("two encryptions of identical input produced identical output — IV not random")
    }
}
```

Also add stub fields to `encryptState` to make compilation work (the full extension comes in Task 9, but we need `algorithm` now):
```go
type encryptState struct {
    algorithm   EncryptionAlgorithm // new — but not yet used by encryptBytes dispatcher
    key         []byte
    fileID      []byte
    ownerEntry  []byte
    userEntry   []byte
    permissions int32
}
```

- [ ] **Step 2: Run tests to verify they fail**

```powershell
go test -run TestEncryptBytesAES128 -v ./...
```
Expected: build failure (undefined `encryptBytesAES128`, `decryptObjectAES128`).

- [ ] **Step 3: Implement encryptBytesAES128**

Append to `encrypt_aes.go`. Add imports `"crypto/aes"`, `"crypto/cipher"`, `cryptorand "crypto/rand"`, `"fmt"`, `"io"`:
```go
// encryptBytesAES128 encrypts plaintext under the per-object AES-128 key
// derived from state.key, objNum, and gen. The output is a 16-byte
// random IV followed by AES-128-CBC ciphertext of plaintext with
// PKCS#7 padding. ISO 32000-1 §7.6.2 / §7.6.3.4.
func encryptBytesAES128(s *encryptState, objNum, gen int, plaintext []byte) ([]byte, error) {
    key := objectKeyAES128(s.key, objNum, gen)
    padded := addPKCS7(plaintext, aes.BlockSize)
    iv := make([]byte, aes.BlockSize)
    if _, err := io.ReadFull(cryptorand.Reader, iv); err != nil {
        return nil, fmt.Errorf("AES IV: %w", err)
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    body := make([]byte, len(padded))
    cipher.NewCBCEncrypter(block, iv).CryptBlocks(body, padded)
    out := make([]byte, len(iv)+len(body))
    copy(out, iv)
    copy(out[len(iv):], body)
    return out, nil
}
```

- [ ] **Step 4: Implement decryptObjectAES128 (stub for now — test needs it)**

Append to `decrypt_aes.go`. Add imports `"crypto/aes"`, `"crypto/cipher"`, `"fmt"`:
```go
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
```

- [ ] **Step 5: Run tests + commit**

```powershell
go test -run TestEncryptBytesAES128 -v ./...
go test ./...
git add encrypt.go encrypt_aes.go decrypt_aes.go encrypt_aes_internal_test.go
git commit -m "feat: encryptBytesAES128 + decryptObjectAES128 (AES-128-CBC + PKCS#7)"
```

---

## Task 5: decryptObjectAES128 edge-case tests

**Files:**
- Create: `decrypt_aes_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Create `decrypt_aes_internal_test.go`:
```go
package asposepdf

import (
    "bytes"
    "testing"
)

func TestDecryptObjectAES128_ShortCiphertext(t *testing.T) {
    state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
    // Less than IV length.
    if _, err := decryptObjectAES128(state, 1, 0, []byte{0x01, 0x02, 0x03}); err == nil {
        t.Error("expected error for short ciphertext, got nil")
    }
}

func TestDecryptObjectAES128_UnalignedBody(t *testing.T) {
    state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
    // 16-byte IV + 17 bytes (not block-aligned body).
    bad := make([]byte, 16+17)
    if _, err := decryptObjectAES128(state, 1, 0, bad); err == nil {
        t.Error("expected error for unaligned body, got nil")
    }
}

func TestDecryptObjectAES128_GarbledCiphertextBadPadding(t *testing.T) {
    // Random IV + random body of correct length but garbled — decryption
    // will produce noise, PKCS#7 strip will fail.
    state := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}
    bad := bytes.Repeat([]byte{0xFF}, 32) // 16-byte IV + 16-byte body
    if _, err := decryptObjectAES128(state, 1, 0, bad); err == nil {
        t.Error("expected PKCS#7 error on garbled ciphertext, got nil")
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run TestDecryptObjectAES128 -v ./...
go test ./...
git add decrypt_aes_internal_test.go
git commit -m "test: decryptObjectAES128 edge cases (short, unaligned, garbled)"
```

---

## Task 6: Refactor — extract encryptBytesRC4 from existing encryptBytes

**Files:**
- Modify: `encrypt.go`

- [ ] **Step 1: Verify all RC4 tests pass before refactor**

```powershell
go test -run 'TestEncrypt|TestDecrypt|TestSetPassword|TestSetEncryption|TestPermissions' -v ./...
```
Expected: all green. Record the count; it should be the same after refactor.

- [ ] **Step 2: Rename the existing encryptBytes method to encryptBytesRC4 (named function, not method)**

In `encrypt.go`, replace lines 252-258 (`func (s *encryptState) encryptBytes(...)`) with:

```go
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
```

Then add a new dispatcher method (which Task 9 will fill in with the AES branch):
```go
// encryptBytes is the per-object encryption dispatcher. Task 9 adds the
// AES-128 branch — for now it forwards to encryptBytesRC4.
func (s *encryptState) encryptBytes(objNum, gen int, data []byte) ([]byte, error) {
    return encryptBytesRC4(s, objNum, data), nil
}
```

Note the new method signature: `(objNum, gen int, data []byte) ([]byte, error)`. This is the future shape — `gen` ignored for RC4 today, but Task 7 changes the writer call sites to pass it, and Task 9 adds the AES branch.

- [ ] **Step 3: Update all call sites in writer.go**

In `writer.go`, find every `encState.encryptBytes(outID, b)` style call (writer.go:80, 106, 115). They all need to:
- Pass `gen` as `0` (writer always emits gen-0 objects).
- Handle the error return.

The change touches the `encFn` closures. **DO NOT change the `encFn` signature yet** — that's Task 7. For now, just panic on error inside the closure (this should never trigger for RC4):

```go
// Before:
encFn = func(b []byte) []byte { return encState.encryptBytes(outID, b) }

// After:
encFn = func(b []byte) []byte {
    out, err := encState.encryptBytes(outID, 0, b)
    if err != nil {
        panic(fmt.Sprintf("RC4 encryption failed (should be unreachable): %v", err))
    }
    return out
}
```

This intentionally keeps the `encFn` signature stable so Task 6 stays a pure refactor.

- [ ] **Step 4: Run full RC4 test suite**

```powershell
go test ./...
```
Expected: all green. No behaviour change.

- [ ] **Step 5: Commit**

```powershell
git add encrypt.go writer.go
git commit -m "refactor: extract encryptBytesRC4 + add encryptBytes dispatcher signature"
```

---

## Task 7: encFn signature `func([]byte) ([]byte, error)`

**Files:**
- Modify: `writer.go`

This task changes `encFn` to return an error. Touch sites: writer.go:78, 80, 106, 113, 115, 174 (signature), 192 (signature), 209-210, 217-218, 277.

- [ ] **Step 1: Update `writeObject` and `writeValue` signatures**

writer.go:174:
```go
func writeObject(buf *bytes.Buffer, id int, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
    buf.WriteString(fmt.Sprintf("%d 0 obj\n", id))
    if err := writeValue(buf, v, remapFn, encFn); err != nil {
        return err
    }
    buf.WriteString("\nendobj\n")
    return nil
}
```

writer.go:192:
```go
func writeValue(buf *bytes.Buffer, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
    // … existing switch …
}
```

Inside `writeValue`, every recursive call propagates errors. Every `encFn(...)` call returns `([]byte, error)` — error path returns from `writeValue`. Examples:

writer.go:209-210 (literal string with encryption):
```go
if encFn != nil {
    enc, err := encFn([]byte(val))
    if err != nil {
        return err
    }
    writeHexBytes(buf, enc)
}
```

writer.go:217-218 (hex string with encryption): same pattern.

writer.go:277 (stream data):
```go
if encFn != nil {
    enc, err := encFn(data)
    if err != nil {
        return err
    }
    data = enc
}
```

For places that recursively call `writeValue(...)` (dict iteration writer.go:244, array writer.go:253), wrap with error return.

- [ ] **Step 2: Update all encFn closures in writer.go to drop the panic and return the real error**

writer.go:78-82 (main object writer loop):
```go
var encFn func([]byte) ([]byte, error)
if encState != nil {
    encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(outID, 0, b) }
}
if err := writeObject(&buf, outID, obj.Value, remapFn, encFn); err != nil {
    return nil, err
}
```

writer.go:106 (catalog) and writer.go:113-115 (info dict): same pattern. Catalog encFn:
```go
catalogEncFn := func(b []byte) ([]byte, error) { return encState.encryptBytes(catalogObjID, 0, b) }
```

Info encFn similarly.

The top-level `buildDocumentPDF` already returns an error — propagate any new error sites naturally.

- [ ] **Step 3: Run tests**

```powershell
go test ./...
```
Expected: all green. RC4 path unchanged behaviourally; error returns are propagated but never trigger.

- [ ] **Step 4: Commit**

```powershell
git add writer.go
git commit -m "refactor: encFn returns error (preps for AES IV generation failure path)"
```

---

## Task 8: Writer — serialize `/Encrypt` V=4 R=4 dict

**Files:**
- Modify: `writer.go`

- [ ] **Step 1: Find where `/Encrypt` is written today**

```powershell
grep -n '/Encrypt\|/Filter./Standard\|/V . 2\|writeEncryptDict' writer.go
```

Locate the function/section that writes the V=2 R=3 dict. Call it `writeEncryptDict` if not already named.

- [ ] **Step 2: Branch on encState.algorithm**

```go
func writeEncryptDict(buf *bytes.Buffer, encState *encryptState) {
    buf.WriteString("<<\n/Filter /Standard\n")
    switch encState.algorithm {
    case EncryptionAlgRC4_128:
        buf.WriteString("/V 2\n/R 3\n/Length 128\n")
    case EncryptionAlgAES128:
        buf.WriteString("/V 4\n/R 4\n/Length 128\n")
    }
    // /P, /O, /U common to both
    fmt.Fprintf(buf, "/P %d\n", encState.permissions)
    buf.WriteString("/O ")
    writeHexBytes(buf, encState.ownerEntry)
    buf.WriteString("\n/U ")
    writeHexBytes(buf, encState.userEntry)
    buf.WriteByte('\n')
    if encState.algorithm == EncryptionAlgAES128 {
        // Crypt filter section
        buf.WriteString("/CF << /StdCF << /Type /CryptFilter /CFM /AESV2 /AuthEvent /DocOpen /Length 16 >> >>\n")
        buf.WriteString("/StmF /StdCF\n")
        buf.WriteString("/StrF /StdCF\n")
    }
    buf.WriteString(">>")
}
```

(Adapt the existing function — the exact code shape may differ.)

- [ ] **Step 3: Add internal test**

Append to `encrypt_aes_internal_test.go`:
```go
func TestWriteEncryptDictAES128(t *testing.T) {
    state := &encryptState{
        algorithm:   EncryptionAlgAES128,
        key:         bytes.Repeat([]byte{0xAB}, 16),
        fileID:      bytes.Repeat([]byte{0xCD}, 16),
        ownerEntry:  bytes.Repeat([]byte{0x01}, 32),
        userEntry:   bytes.Repeat([]byte{0x02}, 32),
        permissions: -4,
    }
    var buf bytes.Buffer
    writeEncryptDict(&buf, state)
    s := buf.String()
    for _, want := range []string{"/V 4", "/R 4", "/Length 128", "/CF", "/StdCF", "/CFM /AESV2", "/StmF /StdCF", "/StrF /StdCF"} {
        if !strings.Contains(s, want) {
            t.Errorf("encrypt dict missing %q in:\n%s", want, s)
        }
    }
    if strings.Contains(s, "/V 2") || strings.Contains(s, "/R 3") {
        t.Errorf("AES dict has V=2/R=3 leftovers:\n%s", s)
    }
}
```

Add `"strings"` to imports if missing.

- [ ] **Step 4: Run tests + commit**

```powershell
go test -run TestWriteEncryptDictAES128 -v ./...
go test ./...
git add writer.go encrypt_aes_internal_test.go
git commit -m "feat: writer serializes /Encrypt V=4 R=4 for AES-128 (with /CF/StmF/StrF)"
```

---

## Task 9: Encrypt-side dispatcher in encryptBytes + encryptConfig/encryptState extensions

**Files:**
- Modify: `encrypt.go`

- [ ] **Step 1: Extend encryptConfig + encryptState**

In `encrypt.go`, around line 135 and 152:
```go
type encryptConfig struct {
    algorithm      EncryptionAlgorithm
    userPassword   string
    ownerPassword  string
    permissions    int32
    hasPermissions bool
}

type encryptState struct {
    algorithm   EncryptionAlgorithm
    key         []byte
    fileID      []byte
    ownerEntry  []byte
    userEntry   []byte
    permissions int32
}
```

(`algorithm` may already have been added in Task 4's stub on `encryptState`; just ensure it's there.)

- [ ] **Step 2: Pipe algorithm from EncryptionOptions into config**

Find `(*Document).SetEncryption` (somewhere in `document.go` or `encrypt.go`). Update it to:
```go
func (d *Document) SetEncryption(opts EncryptionOptions) {
    d.encryptCfg = &encryptConfig{
        algorithm:      opts.Algorithm, // zero value = EncryptionAlgAES128
        userPassword:   opts.UserPassword,
        ownerPassword:  opts.OwnerPassword,
        // … existing fields …
    }
    if opts.Permissions != nil {
        d.encryptCfg.permissions = opts.Permissions.toPDFBits()
        d.encryptCfg.hasPermissions = true
    }
}
```

(Adapt to actual code shape.) Then in `newEncryptState(cfg)`:
```go
state := &encryptState{
    algorithm:   cfg.algorithm,
    key:         key,
    fileID:      fileID,
    ownerEntry:  oEntry,
    userEntry:   uEntry,
    permissions: perms,
}
```

- [ ] **Step 3: Fill in the AES branch of encryptBytes**

Replace the Task 6 stub `encryptBytes` with the real dispatcher:
```go
// encryptBytes is the per-object encryption dispatcher. gen is the
// object generation; for newly-written objects it is always 0 but the
// parameter exists because Algorithm 1.A uses it.
func (s *encryptState) encryptBytes(objNum, gen int, data []byte) ([]byte, error) {
    switch s.algorithm {
    case EncryptionAlgRC4_128:
        return encryptBytesRC4(s, objNum, data), nil
    case EncryptionAlgAES128:
        return encryptBytesAES128(s, objNum, gen, data)
    }
    return nil, fmt.Errorf("encryptBytes: unknown algorithm %d", s.algorithm)
}
```

- [ ] **Step 4: Write internal test for dispatcher**

Append to `encrypt_aes_internal_test.go`:
```go
func TestEncryptBytesDispatcher(t *testing.T) {
    plain := []byte("hello dispatcher")
    rc4State := &encryptState{algorithm: EncryptionAlgRC4_128, key: bytes.Repeat([]byte{0xAB}, 16)}
    aesState := &encryptState{algorithm: EncryptionAlgAES128, key: bytes.Repeat([]byte{0xAB}, 16)}

    rc4Out, err := rc4State.encryptBytes(1, 0, plain)
    if err != nil {
        t.Fatal(err)
    }
    aesOut, err := aesState.encryptBytes(1, 0, plain)
    if err != nil {
        t.Fatal(err)
    }
    // AES output starts with 16-byte IV + ciphertext; minimum 32 bytes.
    if len(aesOut) < 32 {
        t.Errorf("AES output too short: %d", len(aesOut))
    }
    // RC4 output is same length as input (stream cipher, no padding/IV).
    if len(rc4Out) != len(plain) {
        t.Errorf("RC4 output length = %d, want %d", len(rc4Out), len(plain))
    }
    // They must differ.
    if bytes.Equal(rc4Out, aesOut[:len(rc4Out)]) {
        t.Error("dispatcher returned identical bytes for RC4 vs AES — wrong algorithm selected")
    }
}
```

- [ ] **Step 5: Run + commit**

```powershell
go test -run TestEncryptBytesDispatcher -v ./...
go test ./...
git add encrypt.go encrypt_aes_internal_test.go
git commit -m "feat: encryptBytes dispatcher routes by EncryptionAlgorithm + extends config/state"
```

After this commit, all `Document.SetEncryption(EncryptionOptions{...})` calls with explicit `Algorithm: EncryptionAlgRC4_128` produce RC4 PDFs and calls without explicit Algorithm produce AES-128. End-to-end tests come in Tasks 12+.

---

## Task 10: buildDecryptState dispatcher + buildDecryptStateV4R4

**Files:**
- Modify: `decrypt.go`
- Modify: `decrypt_aes.go`
- Modify: `decrypt_aes_internal_test.go`

- [ ] **Step 1: Write failing tests**

Append to `decrypt_aes_internal_test.go`:
```go
import "testing"

func TestBuildDecryptStateV4R4_MissingCF(t *testing.T) {
    encDict := pdfDict{
        "/Filter": pdfName("/Standard"),
        "/V":      4,
        "/R":      4,
        "/Length": 128,
        "/P":      -4,
        "/O":      string(bytes.Repeat([]byte{0x01}, 32)),
        "/U":      string(bytes.Repeat([]byte{0x02}, 32)),
        // /CF missing
        "/StmF": pdfName("/StdCF"),
        "/StrF": pdfName("/StdCF"),
    }
    trailer := pdfDict{"/ID": pdfArray{string(bytes.Repeat([]byte{0xCD}, 16))}}
    if _, err := buildDecryptStateV4R4(encDict, trailer, "x"); err == nil {
        t.Error("expected error for missing /CF")
    }
}

func TestBuildDecryptStateV4R4_WrongCFM(t *testing.T) {
    encDict := pdfDict{
        "/Filter": pdfName("/Standard"),
        "/V":      4, "/R": 4, "/Length": 128, "/P": -4,
        "/O": string(bytes.Repeat([]byte{0x01}, 32)),
        "/U": string(bytes.Repeat([]byte{0x02}, 32)),
        "/CF": pdfDict{
            "/StdCF": pdfDict{
                "/Type": pdfName("/CryptFilter"),
                "/CFM":  pdfName("/V2"), // wrong — should be /AESV2
            },
        },
        "/StmF": pdfName("/StdCF"),
        "/StrF": pdfName("/StdCF"),
    }
    trailer := pdfDict{"/ID": pdfArray{string(bytes.Repeat([]byte{0xCD}, 16))}}
    if _, err := buildDecryptStateV4R4(encDict, trailer, "x"); err == nil {
        t.Error("expected error for /CFM /V2 in V=4 dict")
    }
}
```

- [ ] **Step 2: Implement buildDecryptStateV4R4 in `decrypt_aes.go`**

Append to `decrypt_aes.go`:
```go
// buildDecryptStateV4R4 parses a /V=4 /R=4 /Encrypt dict and validates
// that the crypt filter referenced by /StmF and /StrF uses /CFM /AESV2.
// Per ISO 32000-1 §7.6.3.2 / §7.6.5. Passwords are verified via the
// same Algorithms 2/5/7 as V=2 R=3.
func buildDecryptStateV4R4(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
    cfRaw, ok := encDict["/CF"].(pdfDict)
    if !ok {
        return nil, fmt.Errorf("V=4 R=4: /CF dict missing")
    }
    stmName, _ := encDict["/StmF"].(pdfName)
    strName, _ := encDict["/StrF"].(pdfName)
    if stmName == "" {
        return nil, fmt.Errorf("V=4 R=4: /StmF missing")
    }
    if strName != "" && strName != stmName {
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

    // Run the V=2 R=3 password machinery — Algorithms 2/5/7 are identical.
    state, err := buildDecryptStateV2R3(encDict, trailer, password)
    if err != nil {
        return nil, err
    }
    state.algorithm = EncryptionAlgAES128
    return state, nil
}
```

(Note: this assumes `buildDecryptStateV2R3` exists. If the current code is named `buildDecryptState` and is the V=2 R=3 path, rename it to `buildDecryptStateV2R3` in this task, and introduce the dispatcher `buildDecryptState` per next bullet.)

- [ ] **Step 3: Add the dispatcher**

In `decrypt.go`, rename existing `buildDecryptState` body to `buildDecryptStateV2R3` and add a new top-level dispatcher:
```go
// buildDecryptState parses an /Encrypt dict and returns the per-document
// encryption state for decryption. Dispatches by /V and /R.
func buildDecryptState(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
    v := dictGetInt(encDict, "/V")
    r := dictGetInt(encDict, "/R")
    switch {
    case v == 2 && r == 3:
        state, err := buildDecryptStateV2R3(encDict, trailer, password)
        if err == nil && state != nil {
            state.algorithm = EncryptionAlgRC4_128
        }
        return state, err
    case v == 4 && r == 4:
        return buildDecryptStateV4R4(encDict, trailer, password)
    default:
        return nil, fmt.Errorf("unsupported /V=%d /R=%d", v, r)
    }
}
```

- [ ] **Step 4: Run tests + commit**

```powershell
go test -run TestBuildDecryptStateV4R4 -v ./...
go test ./...
git add decrypt.go decrypt_aes.go decrypt_aes_internal_test.go
git commit -m "feat: buildDecryptState dispatcher + V=4 R=4 (AES-128) parsing"
```

Expected: all green; existing RC4 read tests pass via the renamed V2R3 path.

---

## Task 11: getObject decryption dispatcher

**Files:**
- Modify: the file that contains `rawDocument.getObject` (likely `doc.go`)

- [ ] **Step 1: Find the existing RC4 decrypt call in getObject**

```powershell
grep -n 'applyRC4\|getObject' doc.go
```

Locate the line where the existing code decrypts an object's bytes after parsing.

- [ ] **Step 2: Replace direct applyRC4 call with dispatcher**

Add a helper `decryptObject` to `decrypt.go`:
```go
// decryptObject is the per-object decryption dispatcher mirroring
// encryptBytes on the write side. gen is the generation read from the
// xref entry.
func decryptObject(state *encryptState, objNum, gen int, data []byte) ([]byte, error) {
    switch state.algorithm {
    case EncryptionAlgRC4_128:
        // RC4 is in-place; produce a copy for caller safety.
        key := state.objectKey(objNum)
        out := make([]byte, len(data))
        copy(out, data)
        applyRC4(out, key)
        return out, nil
    case EncryptionAlgAES128:
        return decryptObjectAES128(state, objNum, gen, data)
    }
    return nil, fmt.Errorf("decryptObject: unknown algorithm %d", state.algorithm)
}
```

In `getObject`, replace the current `applyRC4(...)` block with a `decryptObject(state, objNum, gen, data)` call. Error path propagates up — already returns error in getObject's existing signature (verify).

- [ ] **Step 3: Run full suite**

```powershell
go test ./...
```
Expected: all green. Existing RC4 reads go through the new dispatcher path.

- [ ] **Step 4: Commit**

```powershell
git add doc.go decrypt.go
git commit -m "refactor: getObject calls decryptObject dispatcher (RC4 + AES branches)"
```

---

## Task 12: End-to-end AES roundtrip + RC4 regression

**Files:**
- Create: `encrypt_aes_test.go`

- [ ] **Step 1: Write the failing tests**

Create `encrypt_aes_test.go`:
```go
package asposepdf_test

import (
    "bytes"
    "errors"
    "strings"
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestSetEncryptionAES128_RoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    page.AddText("Secret content", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "pwd",
        Algorithm:    pdf.EncryptionAlgAES128,
    })
    var buf bytes.Buffer
    if _, err := doc.WriteTo(&buf); err != nil {
        t.Fatal(err)
    }
    // Encrypted — Open without password fails.
    if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); !errors.Is(err, pdf.ErrEncrypted) {
        t.Errorf("OpenStream without password: err=%v, want ErrEncrypted", err)
    }
    // OpenWithPassword succeeds.
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "pwd")
    if err != nil {
        t.Fatalf("OpenStreamWithPassword: %v", err)
    }
    text, err := doc2.ExtractText()
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(strings.Join(text, "\n"), "Secret content") {
        t.Errorf("text not recovered: %q", text)
    }
}

func TestSetEncryptionAES128_DefaultsToAES(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x"}) // no Algorithm — zero value
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    s := buf.String()
    if !strings.Contains(s, "/V 4") {
        t.Errorf("expected /V 4 (AES default), got:\n%s", s[:200])
    }
    if !strings.Contains(s, "/CFM /AESV2") {
        t.Errorf("expected /CFM /AESV2, missing")
    }
}

func TestSetEncryptionRC4Explicit_Unchanged(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgRC4_128,
    })
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    s := buf.String()
    if !strings.Contains(s, "/V 2") {
        t.Errorf("expected /V 2 for explicit RC4, missing")
    }
    if strings.Contains(s, "/CFM") {
        t.Errorf("RC4 dict should not contain /CFM, found")
    }
    // Confirm RC4 roundtrip still works.
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatalf("RC4 OpenStreamWithPassword: %v", err)
    }
    _ = doc2
}

func TestSetEncryptionAES128_WrongPassword(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "right", Algorithm: pdf.EncryptionAlgAES128})
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "wrong"); err == nil {
        t.Error("expected error for wrong password, got nil")
    }
}

func TestSetEncryptionAES128_OwnerPasswordRecovery(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword:  "user",
        OwnerPassword: "owner",
        Algorithm:     pdf.EncryptionAlgAES128,
    })
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    // Owner password should also open the document.
    if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "owner"); err != nil {
        t.Errorf("owner password open: %v", err)
    }
}

func TestSetEncryptionAES128_PermissionsRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgAES128,
        Permissions: &pdf.Permissions{
            AllowPrint: true,
            AllowCopy:  true,
        },
    })
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    perms, encrypted := doc2.Permissions()
    if !encrypted {
        t.Fatal("Permissions() reports not encrypted")
    }
    if !perms.AllowPrint || !perms.AllowCopy {
        t.Errorf("permissions lost: %+v", perms)
    }
    if perms.AllowModify {
        t.Errorf("AllowModify should be false: %+v", perms)
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run TestSetEncryptionAES128 -v ./...
go test -run TestSetEncryptionRC4Explicit_Unchanged -v ./...
go test ./...
git add encrypt_aes_test.go
git commit -m "test: end-to-end AES-128 roundtrip + RC4-explicit regression"
```

Expected: all six new tests pass + full suite green.

---

## Task 13: Migrate existing encryption tests (RC4 zero-value tests → explicit)

**Files:**
- Modify: existing encryption test files

The default change (zero `EncryptionOptions` now yields AES-128) means tests that explicitly assert RC4 output bytes must opt back into RC4.

- [ ] **Step 1: Identify affected tests**

```powershell
grep -rn 'EncryptionOptions{' --include='*_test.go'
grep -rn 'SetPassword\|pdf.Encrypt' --include='*_test.go'
```

For each test that:
- Asserts `/V 2` or `/R 3` in output → add `Algorithm: pdf.EncryptionAlgRC4_128` explicitly.
- Tests roundtrip without checking which algorithm — should still pass under AES default; leave as-is.
- Tests `pdf.Encrypt()` (top-level helper, RC4-only) — unchanged.
- Tests `SetPassword(user, owner)` (helper that calls SetEncryption with zero options) — will now produce AES output. Update to expect AES, OR change SetPassword to keep producing RC4 (NOT recommended — let it migrate).

Audit pass: open each affected test, decide:
- "Test checks roundtrip works" → leave alone (AES roundtrip works fine).
- "Test checks specific bytes/dict shape" → make algorithm explicit.

- [ ] **Step 2: Update each as required**

Common updates:

```go
// Before:
doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x"})
// (and the test then asserted /V 2 or /R 3)

// After:
doc.SetEncryption(pdf.EncryptionOptions{
    UserPassword: "x",
    Algorithm:    pdf.EncryptionAlgRC4_128,
})
```

- [ ] **Step 3: Run full suite**

```powershell
go test ./...
```
Expected: all green.

- [ ] **Step 4: Commit**

```powershell
git add <affected test files>
git commit -m "test: migrate RC4-shape assertions to explicit EncryptionAlgRC4_128"
```

---

## Task 14: Cross-cutting integration tests (FileAttachment + AcroForm under AES)

**Files:**
- Create: `encrypt_aes_integration_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package asposepdf_test

import (
    "bytes"
    "strings"
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestSetEncryptionAES128_WithFileAttachment(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
    fa.SetIcon(pdf.FileAttachmentIconPushPin)
    if err := fa.SetFileFromStream(strings.NewReader("attached data"), "data.txt"); err != nil {
        t.Fatal(err)
    }
    page.Annotations().Add(fa)
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES128})
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    page2 := doc2.Pages()[0]
    fa2 := page2.Annotations().At(0).(*pdf.FileAttachmentAnnotation)
    if got := string(fa2.FileBytes()); got != "attached data" {
        t.Errorf("file bytes after AES roundtrip = %q, want %q", got, "attached data")
    }
}

func TestSetEncryptionAES128_WithAcroForm(t *testing.T) {
    // Use an existing AcroForm fixture; e.g. PdfWithAcroForm.pdf via testFile helper.
    // If the helper isn't applicable here, build a minimal form programmatically.
    doc := pdf.NewDocument(595, 842)
    form := doc.Form()
    tb, err := form.AddTextField("Name", 1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
    if err != nil {
        t.Fatal(err)
    }
    tb.SetValue("Alice")
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES128})
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    field := doc2.Form().Field("Name")
    if field == nil {
        t.Fatal("field Name not found after roundtrip")
    }
    if v := field.Value(); v != "Alice" {
        t.Errorf("Name value after AES roundtrip = %q, want %q", v, "Alice")
    }
}

func TestSetEncryptionAES128_MultiPage(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    doc.AddBlankPage(595, 842)
    doc.AddBlankPage(595, 842)
    for n := 1; n <= 3; n++ {
        page, _ := doc.Page(n)
        page.AddText("Page "+string(rune('0'+n)), pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
            pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
    }
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES128})
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    if doc2.PageCount() != 3 {
        t.Errorf("PageCount = %d, want 3", doc2.PageCount())
    }
    text, _ := doc2.ExtractText()
    for n, pageText := range text {
        wantSubstr := "Page " + string(rune('0'+n+1))
        if !strings.Contains(pageText, wantSubstr) {
            t.Errorf("page %d missing %q: %q", n+1, wantSubstr, pageText)
        }
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run TestSetEncryptionAES128_With -v ./...
go test -run TestSetEncryptionAES128_MultiPage -v ./...
go test ./...
git add encrypt_aes_integration_test.go
git commit -m "test: AES-128 cross-cutting (FileAttachment + AcroForm + multi-page)"
```

---

## Task 15: pypdf cross-tool tests

**Files:**
- Create: `encrypt_aes_pypdf_test.go`

- [ ] **Step 1: Skip-if-no-pypdf shape**

```go
package asposepdf_test

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"

    pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func skipIfNoPypdf(t *testing.T) {
    t.Helper()
    cmd := exec.Command("python", "-c", "import pypdf")
    if err := cmd.Run(); err != nil {
        t.Skip("pypdf not available — skipping cross-tool test")
    }
}

func TestAES128_ReadableByPypdf(t *testing.T) {
    skipIfNoPypdf(t)
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    page.AddText("Cross tool content", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
    doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES128})

    tmp, err := os.CreateTemp("", "aes-readable-*.pdf")
    if err != nil {
        t.Fatal(err)
    }
    defer os.Remove(tmp.Name())
    if _, err := doc.WriteTo(tmp); err != nil {
        t.Fatal(err)
    }
    tmp.Close()

    script := `
import sys
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
if r.is_encrypted:
    r.decrypt("x")
print(r.pages[0].extract_text())
`
    out, err := exec.Command("python", "-c", script).Output()
    if err != nil {
        t.Fatalf("pypdf failed to read our AES output: %v", err)
    }
    if !strings.Contains(string(out), "Cross tool") {
        t.Errorf("pypdf extracted text missing expected content: %q", out)
    }
}

func TestAES128_ReadsPypdfOutput(t *testing.T) {
    skipIfNoPypdf(t)
    // Have pypdf build an AES-128 PDF for us.
    tmp, err := os.CreateTemp("", "aes-from-pypdf-*.pdf")
    if err != nil {
        t.Fatal(err)
    }
    defer os.Remove(tmp.Name())
    tmp.Close()

    script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.encrypt(user_password="x", owner_password="o", algorithm="AES-128")
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
    if err := exec.Command("python", "-c", script).Run(); err != nil {
        t.Fatalf("pypdf failed to build AES-128 PDF: %v", err)
    }
    raw, _ := os.ReadFile(tmp.Name())
    if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(raw), "x"); err != nil {
        t.Errorf("our OpenStreamWithPassword on pypdf AES-128 output: %v", err)
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run TestAES128_ReadableByPypdf -v ./...
go test -run TestAES128_ReadsPypdfOutput -v ./...
go test ./...
git add encrypt_aes_pypdf_test.go
git commit -m "test: pypdf cross-tool round-trip for AES-128"
```

If pypdf is missing, the tests are skipped automatically — full suite still passes.

---

## Task 16: Docs + close Subepic 1

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md**

In the encryption section (search for `Encrypt(inputPath, outputPath`):

Update `EncryptionOptions` description:
```
- `EncryptionOptions` struct — unified encryption configuration: UserPassword, OwnerPassword (empty → defaults to UserPassword), Permissions *Permissions (nil → grant all), Algorithm EncryptionAlgorithm (zero value → AES-128). Consumed by `(*Document).SetEncryption`
- `EncryptionAlgorithm` enum — `EncryptionAlgAES128` (default, AES-128 V=4 R=4 /CFM /AESV2 per ISO 32000-1 §7.6.3.2), `EncryptionAlgRC4_128` (legacy V=2 R=3)
```

Update the `Encrypt` top-level helper docstring to note it's RC4-only:
```
- `Encrypt(inputPath, outputPath, userPassword, ownerPassword)` — top-level helper writes RC4-128 (PDF 1.4 V=2 R=3) protected PDF. For AES, use `(*Document).SetEncryption(EncryptionOptions{...})`
```

In the `Decryption pipeline` block, note the dispatcher:
```
- Decryption pipeline: `OpenWithPassword`/`OpenStreamWithPassword` parse `/Encrypt`, dispatch by /V (V=2 R=3 → RC4 path; V=4 R=4 → AES-128 path). Both reuse Algorithms 2/5/7 for password handling; per-object decryption uses Algorithm 1 (RC4) or 1.A (AES, with "sAlT" literal suffix). Stream `/Filter` chains are re-applied after decryption per PDF spec ordering (encrypt-after-filter)
```

- [ ] **Step 2: Update README.md**

In the encryption example section, add an AES variant:

```markdown
### Encryption

```go
doc, _ := pdf.Open("input.pdf")
doc.SetEncryption(pdf.EncryptionOptions{
    UserPassword:  "user",
    OwnerPassword: "owner",
    Permissions:   &pdf.Permissions{AllowPrint: true, AllowCopy: true},
    // Algorithm: pdf.EncryptionAlgAES128 — default. Explicit pdf.EncryptionAlgRC4_128 for legacy RC4.
})
doc.Save("encrypted.pdf")
```

Default encryption is AES-128 (Standard Security Handler V=4 R=4, ISO 32000-1 §7.6.3.2). Pass
`Algorithm: pdf.EncryptionAlgRC4_128` for the legacy RC4-128 path. The top-level `pdf.Encrypt()`
helper remains RC4-only for backwards compatibility.
```

Add to the Features bullet list (or wherever encryption is mentioned):
> Encryption — AES-128 (default, V=4 R=4 /AESV2) and RC4-128 (legacy V=2 R=3); Standard Security Handler; user + owner passwords; granular permissions; round-trip preserves AcroForm fields, annotations, embedded files.

- [ ] **Step 3: Run full suite**

```powershell
go test ./...
go vet ./...
```

- [ ] **Step 4: Commit docs**

```powershell
git add CLAUDE.md README.md
git commit -m "docs: AES-128 encryption (Subepic 1 of pdf-go-ccl) in CLAUDE.md and README"
```

- [ ] **Step 5: Update bd issue**

```bash
bd update pdf-go-ccl --append-notes "Subepic 1 (AES-128 V=4 R=4 /CFM /AESV2 read+write, new default for SetEncryption) shipped 2026-05-XX. Public API: EncryptionAlgorithm enum + Algorithm field on EncryptionOptions. RC4-128 V=2 R=3 stays alongside, explicit opt-in. pypdf cross-tool round-trip passes both directions. Subepic 2 (AES-256 V=5 R=6) remains open under this umbrella."
```

Keep `pdf-go-ccl` open — Subepic 2 (AES-256) is the remaining work.

---

## Self-review

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| EncryptionAlgorithm enum + EncryptionOptions field | 1 |
| PKCS#7 padding helpers | 2 |
| objectKeyAES128 (Algorithm 1.A) | 3 |
| encryptBytesAES128 + decryptObjectAES128 | 4, 5 |
| RC4 refactor (encryptBytesRC4) | 6 |
| encFn signature change | 7 |
| /Encrypt V=4 R=4 dict serialization | 8 |
| Encrypt-side dispatcher | 9 |
| Decrypt-side dispatcher + V4R4 parsing | 10, 11 |
| End-to-end roundtrip + RC4 regression | 12 |
| Existing-test migration | 13 |
| FileAttachment + AcroForm coexistence | 14 |
| pypdf cross-tool | 15 |
| Docs + close Subepic 1 | 16 |

**Placeholder scan:** Every task has full code or precise pointer to existing code. The only "adapt to current code shape" notes are in Task 8 (writeEncryptDict) and Task 9 (SetEncryption call site) — these are minor and the implementer reads ~10 lines to find the shape.

**Type consistency:** `encryptBytes(objNum, gen int, data []byte) ([]byte, error)` introduced in Task 6 as a stub; the real dispatcher in Task 9 retains the same signature. `encFn func([]byte) ([]byte, error)` introduced in Task 7 and used consistently afterwards.

---

## Execution Handoff

After saving this plan, two execution options:

**1. Subagent-Driven** — fresh subagent per task, two-stage review (spec + quality).
**2. Inline Execution** — execute in this session via executing-plans.
