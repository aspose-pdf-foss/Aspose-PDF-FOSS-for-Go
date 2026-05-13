# AES-256 Encryption Design Spec (Subepic 2 of `pdf-go-ccl`)

**Date:** 2026-05-13
**Issue:** `pdf-go-ccl` — AES-128 and AES-256 encryption (Standard Security Handler V4/V5/V6)
**Subepic 2 scope:** AES-256, V=5 R=6, /CFM /AESV3 per ISO 32000-2 — read + write
**Subepic 1 (already shipped):** AES-128 V=4 R=4 /CFM /AESV2 — read + write
**Not in scope:** V=5 R=5 (deprecated, Adobe Acrobat 9-10 era)

## Goals

- Read PDFs encrypted with AES-256 (V=5 R=6, /CFM /AESV3, single StdCF crypt filter).
- Write PDFs encrypted with AES-256 via `(*Document).SetEncryption(opts)` with `Algorithm: EncryptionAlgAES256`.
- AES-128 (`EncryptionAlgAES128`) **remains** the zero-value default. AES-256 is explicit opt-in.
- AES-256 output bumps PDF header to `%PDF-2.0` per ISO 32000-2.
- RC4-128 V=2 R=3 and AES-128 V=4 R=4 continue to work without regressions.

## Non-Goals

- V=5 R=5 (deprecated AES-256 with flawed validation algorithm).
- Public-key encryption (`/Filter /PubSec`).
- Per-stream `/Crypt` filter overrides — only single document-wide StdCF.
- SASLprep / Unicode password normalization (RFC 4013) — we treat passwords as byte-level UTF-8.
- Surfacing `/EncryptMetadata` to the public API (we always write `true`).
- Changing the public-API surface of `EncryptionOptions` beyond adding `EncryptionAlgAES256` to the existing enum.

## Architecture

### Differences from V=4 R=4 (Subepic 1)

| Aspect | V=4 R=4 (Subepic 1) | V=5 R=6 (this Subepic) |
|---|---|---|
| Hash | MD5 | SHA-256 (+ SHA-384/SHA-512 in iterations) |
| Key length | 128-bit (16 bytes) | 256-bit (32 bytes) |
| Per-object key | Algorithm 1.A (`MD5(docKey \|\| objNum \|\| gen \|\| "sAlT")`) | **None** — use FEK directly |
| Password derivation | Algorithm 2 (padded password+O+P+fileID through MD5 rounds) | Algorithm 2.B (iterated SHA-256/384/512 hash chain, ≥64 rounds) |
| /U entry | 32 bytes (16-byte hash + 16-byte fileID-MD5) | 48 bytes (32-byte hash + 8-byte validation salt + 8-byte key salt) |
| /O entry | 32 bytes (RC4 chain over user password) | 48 bytes (similar to /U but salt also incorporates /U) |
| File encryption key | Derived deterministically from password+O+P+fileID | **Random 256-bit**, encrypted into /UE and /OE |
| /UE, /OE | Not present | Each 32 bytes — AES-256-CBC(FEK) under user/owner derived key |
| /Perms | Not present | 16 bytes — AES-256-ECB(perms_block) under FEK; tamper-detection |
| /EncryptMetadata | Supported but optional | Required entry (always `true` on write) |
| PDF header | Same as base document (1.4-1.7) | Bumped to `%PDF-2.0` |
| /CF/StdCF /CFM | /AESV2 | /AESV3 |
| /Length | 128 | 256 |
| `fileID` participation | Used in Algorithm 2 key derivation | Not used in key derivation |

### Reused from Subepic 1 infrastructure (unchanged)

- `EncryptionAlgorithm` enum — extended with `EncryptionAlgAES256` constant.
- `EncryptionOptions` struct — `Algorithm` field already present from Subepic 1.
- `addPKCS7` / `stripPKCS7` — AES-256-CBC also uses PKCS#7 padding.
- `encryptState` struct — extended with new fields (`userKeyEntry`, `ownerKeyEntry`, `permsEntry`).
- Dispatcher pattern in `encryptBytes`/`decryptObject` — gains a third case for AES-256.
- Writer's `encFn func([]byte) ([]byte, error)` mechanism — already returns error since Subepic 1.

### New for AES-256 (Subepic 2)

- `hashV5R6` — Algorithm 2.B (iterated SHA-256/384/512 hash chain).
- `/U`, `/O`, `/UE`, `/OE`, `/Perms` entry construction (write) and parsing+validation (read).
- AES-256-CBC encryption/decryption (32-byte key vs 16 for AES-128).
- AES-256-ECB single-block encrypt for `/Perms`.
- Per-object encryption with FEK directly (no per-object key derivation).
- PDF header version bump branch in writer (`%PDF-1.4` → `%PDF-2.0` only for AES-256 output).
- `/Encrypt` V=5 R=6 dict serialization.
- Cross-tool round-trip with pypdf 6.x.

### File organization

| File | Role |
|---|---|
| `encrypt.go` (modify) | Add `EncryptionAlgAES256` constant; extend `encryptBytes` dispatcher with AES-256 branch |
| `encrypt_aes256.go` (new) | `hashV5R6`, `newEncryptStateV5R6`, `encryptBytesAES256`, `aes256CBCNoPadding`, `aes256ECBSingleBlock`, `buildPermsBlock` |
| `encrypt_aes256_internal_test.go` (new) | Hash vector, /U/O/UE/OE/Perms shapes, roundtrip, dispatcher |
| `decrypt.go` (modify) | Extend `buildDecryptState` switch with V=5 R=6; extend `decryptObject` with AES-256 branch |
| `decrypt_aes256.go` (new) | `buildDecryptStateV5R6`, `tryUserPasswordV5R6`, `tryOwnerPasswordV5R6`, `decryptObjectAES256`, `decryptObjectTreeAES256`, `verifyPermsV5R6` |
| `decrypt_aes256_internal_test.go` (new) | Edge cases on bad /CF, missing /UE, password validation paths |
| `writer.go` (modify) | PDF header bump branch; extend `buildEncryptDict` switch with V=5 R=6 case |
| `encrypt_aes256_test.go` (new) | External end-to-end tests (roundtrip, defaults, RC4/AES-128 regression, wrong password, owner recovery, permissions, /Perms tamper detection) |
| `encrypt_aes256_integration_test.go` (new) | FileAttachment + AcroForm + multi-page under AES-256 |
| `encrypt_aes256_pypdf_test.go` (new) | pypdf cross-tool round-trip both directions |
| `CLAUDE.md`, `README.md` (modify) | Public API doc update + PDF 2.0 compatibility note |

## Public API

```go
type EncryptionAlgorithm int

const (
    EncryptionAlgAES128 EncryptionAlgorithm = iota // existing default
    EncryptionAlgRC4_128                           // existing legacy
    EncryptionAlgAES256                            // NEW: AES-256 V=5 R=6 per ISO 32000-2
)

// EncryptionOptions struct — Algorithm field already present from Subepic 1; no changes.
```

Usage:

```go
doc := pdf.NewDocument(595, 842)
doc.SetEncryption(pdf.EncryptionOptions{
    UserPassword:  "user",
    OwnerPassword: "owner",
    Permissions:   &pdf.Permissions{AllowPrint: true, AllowCopy: true},
    Algorithm:     pdf.EncryptionAlgAES256,
})
doc.Save("strong.pdf") // %PDF-2.0 header + V=5 R=6 /CFM /AESV3
```

Behaviour:

- `SetEncryption(EncryptionOptions{UserPassword: "x"})` with zero `Algorithm` → AES-128 (unchanged from Subepic 1).
- `Document.SetPassword(user, owner)` → AES-128 default (unchanged).
- `pdf.Encrypt(in, out, userPwd, ownerPwd)` top-level helper → RC4-128 (unchanged).
- Read-side `OpenWithPassword` / `OpenStreamWithPassword` — same signature; internal dispatcher gains V=5 R=6 case.
- `(*Document).Permissions()` — unchanged; returns Permissions parsed from /P. Tamper-detection via /Perms happens during Open (a tampered /Perms entry returns an error at Open time).
- `(*Document).RemoveEncryption()` — unchanged.

Aspose.PDF for .NET parity:

- .NET: `doc.Encrypt(userPwd, ownerPwd, privilege, CryptoAlgorithm.AESx256)`
- Our: `doc.SetEncryption(EncryptionOptions{UserPassword, OwnerPassword, Permissions, Algorithm: EncryptionAlgAES256})`

PDF compatibility note (will be in README): AES-256 output produces `%PDF-2.0` per ISO 32000-2. Viewers older than Adobe Acrobat DC (~2015) may not support PDF 2.0 documents. Users prioritizing universal viewer compatibility should stay on AES-128 (the default).

## Algorithm 2.B (`hashV5R6`)

Per ISO 32000-2 §7.6.4.3.4.

Signature:

```go
// hashV5R6 computes the Algorithm 2.B hash. The result is always 32 bytes
// (the first 32 bytes of K after the iteration terminates).
//
// extra is empty (nil) for /U hash computation, and the 48-byte /U entry
// for /O hash computation.
func hashV5R6(password []byte, salt []byte, extra []byte) []byte
```

Algorithm:

```
1. K = SHA-256(password || salt || extra)             // initial 32-byte K
2. For round = 0; ; round++:
     a. K1 = concat(64 copies of (password || K || extra))
     b. E = AES-128-CBC-encrypt(
              key = K[0:16],
              iv  = K[16:32],
              data = K1
            )
     c. val = sum(E[0:16]) mod 3
     d. switch val:
          case 0: K = SHA-256(E)        // 32 bytes
          case 1: K = SHA-384(E)        // 48 bytes
          case 2: K = SHA-512(E)        // 64 bytes
     e. lastByte = last byte of E
     f. if round >= 64 && lastByte <= round - 32 {
          break
        }
3. Return K[0:32]
```

The `mod 3` shortcut (sum of bytes ≡ big-endian-int mod 3) is mathematically exact because 256 ≡ 1 (mod 3), so each digit in base-256 contributes its value directly. We use byte-sum.

Where it's called:

- **Write:**
  - Compute /U hash: `hashV5R6(passwordUTF8, validationSaltU, nil)`
  - Compute /O hash: `hashV5R6(ownerPasswordUTF8, validationSaltO, U_48_bytes)`
  - Derive user wrapping key (for /UE): `hashV5R6(passwordUTF8, keySaltU, nil)`
  - Derive owner wrapping key (for /OE): `hashV5R6(ownerPasswordUTF8, keySaltO, U_48_bytes)`

- **Read:**
  - Validate user password: compare `hashV5R6(password, /U[32:40], nil)` with `/U[0:32]`
  - Validate owner password: compare `hashV5R6(password, /O[32:40], U_48_bytes)` with `/O[0:32]`
  - Derive wrapping key for FEK decryption: same formula as write

Verification:

- Static test vector via pypdf 6.x — hardcode one known (password, salt, extra) → expected 32-byte hash output (computed offline once, then frozen in the test).
- Functional roundtrip — for any password, write then read with the same password recovers content.

## Write Side

### Overall pipeline

```
1. Generate random File Encryption Key (FEK): 32 bytes from crypto/rand
2. Generate 4 random salts (8 bytes each): validSaltU, keySaltU, validSaltO, keySaltO
3. Compute /U (48 bytes): hashV5R6(pwUserUTF8, validSaltU, nil) || validSaltU || keySaltU
4. Compute /O (48 bytes): hashV5R6(pwOwnerUTF8, validSaltO, U) || validSaltO || keySaltO
5. Compute /UE (32 bytes): AES-256-CBC encrypt FEK with key = hashV5R6(pwUser, keySaltU, nil), IV = 16 zero bytes
6. Compute /OE (32 bytes): AES-256-CBC encrypt FEK with key = hashV5R6(pwOwner, keySaltO, U), IV = 16 zero bytes
7. Compute /Perms (16 bytes): AES-256-ECB encrypt the perms-block under FEK
8. Serialize /Encrypt dict
9. Per-object: AES-256-CBC encrypt with FEK directly
```

If `ownerPassword == ""`, fall back to `ownerPassword = userPassword` (matches RC4 + AES-128 behavior). The resulting /O entry is still distinct from /U because of different salts and the `U_bytes` extra in the hash.

### /U entry (48 bytes per ISO 32000-2 §7.6.4.4)

```
+----------+----------------+----------+
| hash(32) | validSalt(8)   | keySalt(8) |
+----------+----------------+----------+
```

`hash = hashV5R6(passwordUTF8, validSalt, nil)`. Validates user password; consumed during open to derive the wrapping key for `/UE`.

### /O entry (48 bytes)

Same shape as /U but the hash incorporates the /U bytes:

```
+----------+----------------+----------+
| hash(32) | validSalt(8)   | keySalt(8) |
+----------+----------------+----------+
```

`hash = hashV5R6(ownerPasswordUTF8, validSalt, U_48_bytes)`. /U must be computed first (the hash depends on its full 48 bytes).

### /UE entry (32 bytes) — encrypted FEK under user password

```go
wrappingKeyU := hashV5R6(userPasswordUTF8, U[40:48] /* keySalt */, nil) // 32 bytes
block, _ := aes.NewCipher(wrappingKeyU)
iv := make([]byte, 16) // 16 zero bytes — fixed IV per spec
UE := make([]byte, 32)
cipher.NewCBCEncrypter(block, iv).CryptBlocks(UE, FEK)
// FEK is exactly 32 bytes = 2 AES blocks, no PKCS#7 padding
```

### /OE entry (32 bytes) — encrypted FEK under owner password

```go
wrappingKeyO := hashV5R6(ownerPasswordUTF8, O[40:48] /* keySalt */, U_48_bytes)
block, _ := aes.NewCipher(wrappingKeyO)
iv := make([]byte, 16)
OE := make([]byte, 32)
cipher.NewCBCEncrypter(block, iv).CryptBlocks(OE, FEK)
```

### /Perms (16 bytes) — tamper-resistant permissions

Per ISO 32000-2 §7.6.4.6.2:

```
Byte 0-3:   /P value, 4-byte little-endian (signed 32-bit interpretation)
Byte 4-7:   0xFF 0xFF 0xFF 0xFF
Byte 8:     'T' if EncryptMetadata=true; 'F' otherwise (we always write 'T')
Byte 9-11:  'a', 'd', 'b'
Byte 12-15: 4 random bytes (entropy/padding)
```

Then:

```go
block, _ := aes.NewCipher(FEK) // AES-256 key
encryptedPerms := make([]byte, 16)
block.Encrypt(encryptedPerms, permsBlock) // single-block ECB, no IV, no padding
```

### Per-object encryption

```go
func encryptBytesAES256(s *encryptState, plaintext []byte) ([]byte, error) {
    // s.key holds the FEK (32 bytes)
    padded := addPKCS7(plaintext, aes.BlockSize)
    iv := make([]byte, aes.BlockSize)
    if _, err := io.ReadFull(cryptorand.Reader, iv); err != nil {
        return nil, fmt.Errorf("AES-256 IV: %w", err)
    }
    block, err := aes.NewCipher(s.key) // 32-byte key → AES-256
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

Signature is `(s *encryptState, plaintext []byte)` — `objNum`/`gen` not needed (no per-object key derivation). Dispatcher in `encryptBytes`:

```go
func (s *encryptState) encryptBytes(objNum, gen int, data []byte) ([]byte, error) {
    switch s.algorithm {
    case EncryptionAlgRC4_128:
        return encryptBytesRC4(s, objNum, data), nil
    case EncryptionAlgAES128:
        return encryptBytesAES128(s, objNum, gen, data)
    case EncryptionAlgAES256:
        return encryptBytesAES256(s, data) // ignores objNum, gen
    }
    return nil, fmt.Errorf("encryptBytes: unknown algorithm %d", s.algorithm)
}
```

### `/Encrypt` dict for V=5 R=6

```
<< /Filter /Standard
   /V 5 /R 6
   /Length 256
   /P n
   /O <96-hex chars = 48 bytes>
   /U <96-hex chars = 48 bytes>
   /OE <64-hex chars = 32 bytes>
   /UE <64-hex chars = 32 bytes>
   /Perms <32-hex chars = 16 bytes>
   /EncryptMetadata true
   /CF << /StdCF << /Type /CryptFilter /CFM /AESV3 /AuthEvent /DocOpen /Length 32 >> >>
   /StmF /StdCF
   /StrF /StdCF
>>
```

Writer `buildEncryptDict` gets the third switch arm.

### PDF header bump

In writer, where the PDF prolog is emitted:

```go
header := "%PDF-1.4\n"
if d.encrypt != nil && d.encrypt.algorithm == EncryptionAlgAES256 {
    header = "%PDF-2.0\n"
}
buf.WriteString(header)
buf.WriteString(binaryComment) // existing 4-byte non-ASCII marker
```

### `encryptState` extensions

```go
type encryptState struct {
    algorithm     EncryptionAlgorithm
    key           []byte // RC4: 16 bytes; AES-128: 16 bytes; AES-256: 32 bytes (FEK)
    fileID        []byte // 16 bytes; not used for V=5 R=6 key derivation
    ownerEntry    []byte // RC4/AES-128: 32 bytes; AES-256: 48 bytes
    userEntry     []byte // RC4/AES-128: 32 bytes; AES-256: 48 bytes
    userKeyEntry  []byte // AES-256 only: 32 bytes (/UE)        NEW
    ownerKeyEntry []byte // AES-256 only: 32 bytes (/OE)        NEW
    permsEntry    []byte // AES-256 only: 16 bytes (/Perms)     NEW
    permissions   int32
}
```

New fields are zero for RC4 and AES-128 (no behavior change for those algorithms).

## Read Side

### Dispatcher extension

```go
func buildDecryptState(encDict pdfDict, trailer pdfDict, password string) (*encryptState, error) {
    filter := dictGetName(encDict, "/Filter")
    if filter != "/Standard" {
        return nil, fmt.Errorf("unsupported /Filter %q", filter)
    }
    v := dictGetInt(encDict, "/V")
    r := dictGetInt(encDict, "/R")
    switch {
    case v == 2 && r == 3:
        return buildDecryptStateV2R3(encDict, trailer, password)
    case v == 4 && r == 4:
        return buildDecryptStateV4R4(encDict, trailer, password)
    case v == 5 && r == 6:
        return buildDecryptStateV5R6(encDict, password) // trailer/ID not needed
    default:
        return nil, fmt.Errorf("unsupported security handler V=%d R=%d", v, r)
    }
}
```

### `buildDecryptStateV5R6`

1. Validate /CF/StdCF/CFM == /AESV3.
2. Validate /StmF and /StrF both point to /StdCF (divergent → unsupported error).
3. Read /U (48), /O (48), /UE (32), /OE (32), /Perms (16) — all required, exact byte lengths.
4. Read /P.
5. Try user password: `hashV5R6(pwd, /U[32:40], nil)` ?== `/U[0:32]`. If match, decrypt /UE → FEK.
6. Else try owner password: `hashV5R6(pwd, /O[32:40], /U)` ?== `/O[0:32]`. If match, decrypt /OE → FEK.
7. Else return "invalid password".
8. Verify /Perms tamper-detection: AES-256-ECB decrypt /Perms under FEK; check marker "adb" at bytes 9-11; check P matches `/P` dict value.
9. Construct `encryptState{algorithm: AES256, key: FEK, ...}`.

### Per-object decryption

```go
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
        return nil, err
    }
    plain := make([]byte, len(body))
    cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, body)
    return stripPKCS7(plain)
}
```

Dispatcher in `decryptObject`:

```go
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
```

`decryptObjectTreeAES256` mirrors `decryptObjectTreeAES128` but uses `decryptObjectAES256(state, b)` (no objNum/gen needed). Optionally factor both into a single tree-walker parameterized by a decrypt closure for DRY.

### `/Perms` tamper-detection

```go
func verifyPermsV5R6(fek, permsEnc []byte, declaredP int32) error {
    block, err := aes.NewCipher(fek)
    if err != nil {
        return err
    }
    decoded := make([]byte, 16)
    block.Decrypt(decoded, permsEnc) // single-block ECB

    if decoded[9] != 'a' || decoded[10] != 'd' || decoded[11] != 'b' {
        return fmt.Errorf("/Perms tampered: bad marker")
    }
    permsP := int32(uint32(decoded[0]) | uint32(decoded[1])<<8 |
        uint32(decoded[2])<<16 | uint32(decoded[3])<<24)
    if permsP != declaredP {
        return fmt.Errorf("/Perms tampered: P=%d in block vs %d in dict", permsP, declaredP)
    }
    return nil
}
```

`buildDecryptStateV5R6` calls this after FEK recovery; mismatch returns error from `OpenWithPassword`.

## Testing Strategy

### Internal tests (`package asposepdf`)

`encrypt_aes256_internal_test.go`:

- `TestHashV5R6_KnownVector` — static reference vector hardcoded from pypdf computation.
- `TestHashV5R6_TerminationCondition` — function always returns 32 bytes for varied inputs.
- `TestHashV5R6_ExtraAffectsOutput` — extra parameter changes the result.
- `TestNewEncryptStateV5R6_Lengths` — /U=48, /O=48, /UE=32, /OE=32, /Perms=16, key=32.
- `TestEncryptBytesAES256_RoundTrip` — random plaintext → encrypt → decrypt = identity (empty, 1 byte, 1024 bytes, multiples of 16).
- `TestEncryptBytesAES256_IVRandomness` — same input encrypted twice produces different output.
- `TestEncryptBytesDispatcher_AES256` — `state.encryptBytes` routes AES-256 to `encryptBytesAES256`.
- `TestPermsBlockRoundTrip` — buildPermsBlock → encrypt → decrypt → verify == nil.
- `TestPermsBlockTamperDetection` — flip byte → verify returns error.

`decrypt_aes256_internal_test.go`:

- `TestBuildDecryptStateV5R6_MissingCF` → error.
- `TestBuildDecryptStateV5R6_WrongCFM` (/AESV2 in V=5 dict) → error.
- `TestBuildDecryptStateV5R6_MissingUE` → error.
- `TestBuildDecryptStateV5R6_PermsLengthError` (not 16 bytes) → error.
- `TestTryUserPasswordV5R6_Wrong` → returns (nil, false).
- `TestDecryptObjectAES256_ShortCiphertext` → error.
- `TestDecryptObjectAES256_UnalignedBody` → error.

### External tests

`encrypt_aes256_test.go`:

- `TestSetEncryptionAES256_RoundTrip` — Save → OpenWithPassword → ExtractText returns original.
- `TestSetEncryptionAES256_DefaultIsStillAES128` — zero `Algorithm` produces /V 4, not /V 5.
- `TestSetEncryptionAES256_OutputV5R6Shape` — explicit AES-256 → output contains /V 5, /R 6, /Length 256, /CFM /AESV3, /EncryptMetadata, /UE, /OE, /Perms.
- `TestSetEncryptionAES256_HeaderIsPDF20` — output starts with `%PDF-2.0`.
- `TestSetEncryptionAES256_RC4Unchanged` — explicit RC4 still produces /V 2 with %PDF-1.4 header.
- `TestSetEncryptionAES256_AES128Unchanged` — explicit AES-128 still produces /V 4 with %PDF-1.4 header.
- `TestSetEncryptionAES256_OwnerPasswordRecovery` — owner password opens the document.
- `TestSetEncryptionAES256_WrongPassword` → ErrEncrypted.
- `TestSetEncryptionAES256_PermissionsRoundTrip` — Permissions{AllowPrint: true} survives.
- `TestSetEncryptionAES256_PermsTamperDetection` — mutating output's /P byte → Open fails.

### Integration tests

`encrypt_aes256_integration_test.go`:

- `TestSetEncryptionAES256_WithFileAttachment` — embedded file roundtrip survives.
- `TestSetEncryptionAES256_WithAcroForm` — field values readable after roundtrip.
- `TestSetEncryptionAES256_MultiPage` — 3-page document roundtrips.

### Cross-tool tests

`encrypt_aes256_pypdf_test.go`:

- `TestAES256_ReadableByPypdf` — our output → pypdf decrypts + extracts text.
- `TestAES256_ReadsPypdfOutput` — pypdf-built AES-256 PDF (`algorithm="AES-256"` for R=6 specifically) → our `OpenStreamWithPassword` reads.

Both skip automatically if pypdf is unavailable. Same pattern as Subepic 1.

### Regression baseline

- All Subepic-0 (RC4) and Subepic-1 (AES-128) tests continue to pass without modification.
- The writer's PDF header bump branch applies only when `algorithm == EncryptionAlgAES256`; AES-128/RC4 output uses the pre-existing header.

## Risks

1. **Algorithm 2.B implementation drift.** Iterative hash chain is the most fragile piece. Edge cases in termination condition or mod 3 branch easy to get wrong.
   - **Mitigation:** Static test vector from pypdf is mandatory. Cross-tool tests serve as second-line interop verification.

2. **Big-endian mod 3 trick.** Using `sum(bytes) mod 3` instead of `bigInt(bytes) mod 3`. Math: 256 ≡ 1 (mod 3), so each byte contributes its value modulo 3. Equivalent and faster.
   - **Mitigation:** Documented in spec; safety net is the static test vector — if the math is wrong, hash won't match pypdf reference.

3. **`/Perms` tamper-detection false positives.** If a user externally tampers with /P, Open fails with "tampered" error.
   - **Mitigation:** This is correct per-spec behavior. Documented in README. Legitimate /P changes flow through `(*Document).SetPermissions` + Save, which recomputes /Perms.

4. **PDF header bump compatibility.** Pre-Acrobat-DC (~2015) viewers may not read `%PDF-2.0`.
   - **Mitigation:** AES-256 is opt-in. Default AES-128 keeps `%PDF-1.4`. Users explicitly choosing AES-256 acknowledge the tradeoff. README documents this.

5. **Owner password == user password case.** If `OwnerPassword == ""`, falls back to `userPassword`. /O entry still distinct from /U due to different salts and /U-bytes-extra in hash.
   - **Mitigation:** Verified by test.

6. **`/UE` / `/OE` IV is 16 zero bytes per spec.** Cryptographic concern: IV reuse normally weakens CBC. Here the data (FEK) is itself uniformly random — no plaintext-correlation attack possible.
   - **Mitigation:** None needed — spec requirement.

7. **`/EncryptMetadata` handling.** We always write `true`. pypdf-generated PDFs may have `false`; we should still decrypt them. The /Perms block's byte-8 marker (`T` or `F`) must match the dict entry on read for tamper-detection, but if a producer omits the consistency check (pypdf doesn't enforce it), we can be lenient on read (only check `'adb'` marker + P match, ignore byte 8).
   - **Mitigation:** Lenient on read (byte 8 not strictly checked); strict on write (always `T`).

## Aspose.PDF for .NET fidelity

Aspose .NET exposes `CryptoAlgorithm.AESx256` which ships V=5 R=6 (R=5 deprecated, mirrors our choice). Mapping:

- .NET: `doc.Encrypt(userPwd, ownerPwd, privilege, CryptoAlgorithm.AESx256)`
- Go (this library): `doc.SetEncryption(EncryptionOptions{UserPassword, OwnerPassword, Permissions, Algorithm: EncryptionAlgAES256})`

Semantically equivalent; Go options-struct idiom vs .NET positional args.

## Open Questions

None — all design decisions agreed during brainstorming.

## References

- ISO 32000-2:2020 §7.6.4 — encryption (Standard Security Handler V=5 R=6)
- ISO 32000-2:2020 §7.6.4.3.4 — Algorithm 2.B
- ISO 32000-2:2020 §7.6.4.4 — /U, /O, /UE, /OE construction
- ISO 32000-2:2020 §7.6.4.6 — /Perms entry
- Adobe extension levels reference for V=5 R=6 (informational)
- Subepic 1 spec: [2026-05-12-aes-128-encryption-design.md](2026-05-12-aes-128-encryption-design.md)
