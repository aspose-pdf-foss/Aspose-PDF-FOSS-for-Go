# Object Streams (CompressObjects) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pack non-stream objects into object streams and write a cross-reference stream when `OptimizationOptions.CompressObjects` is set, so saved files shrink losslessly.

**Architecture:** `assemble()` stays the single source of output numbering; `buildDocumentPDF` branches to a new layout function in `objstm_write.go` that writes ineligible objects as ordinary indirect objects, eligible ones in batches of 100 inside `/ObjStm` streams, and a `/Type /XRef` stream in place of the classic table. `appendRevision` writes a cross-reference stream section when the file's last section is one.

**Tech Stack:** Go 1.24, standard library only (`bytes`, `compress/zlib` via the existing stream writer). pikepdf 10.9.1 (qpdf) as the independent check, used from a throwaway script only.

**Spec:** `docs/superpowers/specs/2026-09-17-compress-objects-design.md`

## Global Constraints

- Every new `.go` file starts with `// SPDX-License-Identifier: MIT`, a blank line, then `package asposepdf`.
- Pure Go, standard library only. No new module dependency.
- No new entries in `testdata/testfiles.json`, no new files in `testdata/`; fixtures are built in memory.
- Batch size: 100 objects per object stream (`objStmCapacity`).
- Header: at least `%PDF-1.5`; a higher header (`%PDF-2.0` for AES-256) is kept.
- Never packed: streams, the encryption dictionary, signature dictionaries (`isSignatureDict`), object streams, the cross-reference stream.
- Objects inside an object stream are written with no per-object encryption; the object stream is encrypted as a stream under its own number. The cross-reference stream is never encrypted.
- `SaveLinearized`/`WriteToLinearized` ignore the flag.
- `gofmt -l` clean on touched files, `go vet ./...` silent, `go test ./...` passes, `golangci-lint run` → `0 issues`.
- Commits end with a blank line then `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`. Do not push, do not tag.

## File Structure

| File | Responsibility |
|---|---|
| `objstm_write.go` (create) | Object-stream body, cross-reference stream row encoding, eligibility, the compressed layout writer |
| `objstm_write_internal_test.go` (create) | Encoder unit tests and document round trips |
| `writer.go` (modify) | Branch to `buildObjectStreamPDF` when `d.compressObjects` |
| `document.go` (modify) | `compressObjects bool` field |
| `optimize.go` (modify) | `CompressObjects` option, `CompressedObjects` result, default preset |
| `sign_incremental.go` (modify) | `appendRevision` emits a cross-reference stream after a cross-reference stream |
| `CLAUDE.md`, `CHANGELOG.md` (modify) | Documentation |

Existing code this plan relies on (verified):

- `type assembled struct { encState *encryptState; contentIDs []int; remap map[int]int; pagesObjID, catalogObjID, infoObjID, encryptObjID, totalObjects int; catalog pdfDict; header string }` and `(*assembled).remapFn() func(int) int` — `linearize_assemble.go`. Output IDs are `1..totalObjects-1`.
- `writeObject(buf *bytes.Buffer, id int, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error` and `writeValue(...)` — `writer.go`. A `*pdfStream` with `Decoded: true` is Flate-compressed by `writeValue`, then `encFn` is applied to the data; the stream dict itself is never encrypted.
- `buildEncryptDict(s *encryptState) pdfDict`, `encState.encryptBytes(num, gen int, b []byte) ([]byte, error)`, `encState.fileID []byte`.
- `isSignatureDict(d pdfDict) bool` — `decrypt.go`.
- `pdfDirectRef{Num}` (never remapped), `pdfHexString`.
- `isXRefStream(data []byte, offset int64) bool` — `xref.go`; the reader handles `/W`, `/Index`, type-2 entries, and mixed `/Prev` chains, and does not decrypt objects taken from an object stream (`validate.go` `getObject`).
- `appendRevision` in `sign_incremental.go` builds `rows []xrefRow{num, gen int; off int64}` sorted by number and a `size`, then writes a classic table and trailer; `prevXref` and `id0, id1 := d.incrementalID()` are in scope.

---

### Task 1: Object-stream and cross-reference-stream encoders

**Files:**
- Create: `objstm_write.go`
- Test: `objstm_write_internal_test.go`

**Interfaces:**
- Produces:
  - `const objStmCapacity = 100`
  - `type packedObj struct { num int; body []byte }`
  - `func buildObjectStream(objs []packedObj) (data []byte, first int)`
  - `type xrefStreamEntry struct { typ int; f2 int64; f3 int }`
  - `func encodeXRefEntries(rows []xrefStreamEntry) (data []byte, w pdfArray)`

- [ ] **Step 1: Write the failing test**

Create `objstm_write_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strconv"
	"strings"
	"testing"
)

// The object stream must read back through the same parsing the reader uses:
// a header of "num offset" pairs, then each body at /First + offset.
func TestBuildObjectStream(t *testing.T) {
	objs := []packedObj{
		{num: 3, body: []byte("<</Type /Annot>>")},
		{num: 7, body: []byte("[1 2 3]")},
		{num: 9, body: []byte("(hello)")},
	}
	data, first := buildObjectStream(objs)

	header := strings.Fields(string(data[:first]))
	if len(header) != 6 {
		t.Fatalf("header has %d fields, want 6: %q", len(header), data[:first])
	}
	for i, o := range objs {
		num, _ := strconv.Atoi(header[2*i])
		off, _ := strconv.Atoi(header[2*i+1])
		if num != o.num {
			t.Errorf("entry %d names object %d, want %d", i, num, o.num)
		}
		v, err := parseValue(newLexer(data[first+off:]))
		if err != nil {
			t.Fatalf("object %d does not parse at its offset: %v", o.num, err)
		}
		switch o.num {
		case 3:
			d, ok := v.(pdfDict)
			if !ok || d["/Type"] != pdfName("/Annot") {
				t.Errorf("object 3 parsed as %#v", v)
			}
		case 7:
			if a, ok := v.(pdfArray); !ok || len(a) != 3 {
				t.Errorf("object 7 parsed as %#v", v)
			}
		case 9:
			if str, ok := v.(string); !ok || str != "hello" {
				t.Errorf("object 9 parsed as %#v", v)
			}
		}
	}
}
```

Append the row-encoding test:

```go
// Rows are big-endian with widths [1 w2 w3], each width the smallest that
// holds the largest value in its column.
func TestEncodeXRefEntries(t *testing.T) {
	rows := []xrefStreamEntry{
		{typ: 0, f2: 0, f3: 65535},
		{typ: 1, f2: 70000, f3: 0},
		{typ: 2, f2: 12, f3: 5},
	}
	data, w := encodeXRefEntries(rows)
	if len(w) != 3 || w[0] != 1 || w[1] != 3 || w[2] != 2 {
		t.Fatalf("/W = %v, want [1 3 2]", w)
	}
	if len(data) != 3*(1+3+2) {
		t.Fatalf("%d bytes, want %d", len(data), 3*6)
	}
	for i, r := range rows {
		row := data[i*6:]
		if int(row[0]) != r.typ {
			t.Errorf("row %d type = %d, want %d", i, row[0], r.typ)
		}
		if got := readField(row, 1, 3); int64(got) != r.f2 {
			t.Errorf("row %d field 2 = %d, want %d", i, got, r.f2)
		}
		if got := readField(row, 4, 2); got != r.f3 {
			t.Errorf("row %d field 3 = %d, want %d", i, got, r.f3)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestBuildObjectStream|TestEncodeXRefEntries" .`
Expected: FAIL — `undefined: packedObj`, `undefined: buildObjectStream`, `undefined: xrefStreamEntry`.

- [ ] **Step 3: Write the implementation**

Create `objstm_write.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strconv"
)

// Object streams and cross-reference streams (ISO 32000-1 §7.5.7, §7.5.8),
// written when OptimizationOptions.CompressObjects is set. Non-stream objects
// are packed a hundred at a time into Flate-compressed /ObjStm streams, which
// compress far better together than as separate indirect objects, and the
// classic xref table becomes a compressed /XRef stream.

// objStmCapacity is how many objects one object stream holds. Large enough to
// compress well, small enough that a reader resolving one object does not
// inflate a huge stream.
const objStmCapacity = 100

// packedObj is one object destined for an object stream: its output number
// and its serialized value, without the "obj"/"endobj" wrapper.
type packedObj struct {
	num  int
	body []byte
}

// buildObjectStream returns the decoded data of an object stream holding objs
// and the /First offset: a header of "number offset" pairs, then the bodies,
// each offset counted from /First.
func buildObjectStream(objs []packedObj) (data []byte, first int) {
	var header, bodies bytes.Buffer
	for i, o := range objs {
		if i > 0 {
			header.WriteByte(' ')
		}
		header.WriteString(strconv.Itoa(o.num))
		header.WriteByte(' ')
		header.WriteString(strconv.Itoa(bodies.Len()))
		bodies.Write(o.body)
		bodies.WriteByte('\n')
	}
	header.WriteByte('\n')
	first = header.Len()
	return append(header.Bytes(), bodies.Bytes()...), first
}

// xrefStreamEntry is one row of a cross-reference stream (ISO 32000-1 §7.5.8.3
// Table 18): type 0 free (f2 next free object, f3 generation), type 1 in use
// (f2 byte offset, f3 generation), type 2 compressed (f2 object-stream number,
// f3 index within it).
type xrefStreamEntry struct {
	typ int
	f2  int64
	f3  int
}

// encodeXRefEntries packs rows big-endian with widths [1 w2 w3], each the
// smallest that holds the largest value in its column, and returns the /W
// array alongside.
func encodeXRefEntries(rows []xrefStreamEntry) (data []byte, w pdfArray) {
	var max2 int64
	max3 := 0
	for _, r := range rows {
		if r.f2 > max2 {
			max2 = r.f2
		}
		if r.f3 > max3 {
			max3 = r.f3
		}
	}
	w2, w3 := byteWidth(max2), byteWidth(int64(max3))
	data = make([]byte, 0, len(rows)*(1+w2+w3))
	for _, r := range rows {
		data = append(data, byte(r.typ))
		data = appendBigEndian(data, r.f2, w2)
		data = appendBigEndian(data, int64(r.f3), w3)
	}
	return data, pdfArray{1, w2, w3}
}

// byteWidth is the number of bytes needed to hold v, at least one.
func byteWidth(v int64) int {
	w := 1
	for v > 0xFF {
		v >>= 8
		w++
	}
	return w
}

// appendBigEndian appends the low width bytes of v, most significant first.
func appendBigEndian(dst []byte, v int64, width int) []byte {
	for i := width - 1; i >= 0; i-- {
		dst = append(dst, byte(v>>(8*uint(i))))
	}
	return dst
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -run "TestBuildObjectStream|TestEncodeXRefEntries" .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add objstm_write.go objstm_write_internal_test.go
git commit -m "$(cat <<'EOF'
feat: object-stream and cross-reference-stream encoders (pdf-go-o0wy)

The two building blocks of compressed output: an object stream's body (a
header of number/offset pairs, then the objects, offsets counted from
/First) and the rows of a cross-reference stream, big-endian with each column
as narrow as its largest value allows. Both are checked by reading them back
the way the reader does.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: The compressed layout, the option, and round trips

**Files:**
- Modify: `objstm_write.go` (append), `writer.go`, `document.go`, `optimize.go`
- Test: `objstm_write_internal_test.go` (append)

**Interfaces:**
- Consumes: `objStmCapacity`, `packedObj`, `buildObjectStream`, `xrefStreamEntry`, `encodeXRefEntries` (Task 1).
- Produces:
  - `Document.compressObjects bool`
  - `func objectPackable(id int, v pdfValue, encryptObjID int) bool`
  - `func buildObjectStreamPDF(d *Document, asm *assembled) ([]byte, error)`
  - `OptimizationOptions.CompressObjects bool`, `OptimizationResult.CompressedObjects int`
  - `func (d *Document) countPackableObjects() int`

- [ ] **Step 1: Write the failing tests**

Append to `objstm_write_internal_test.go` (add imports `"bytes"`, `"crypto/rand"`, `"crypto/rsa"`, `"crypto/x509"`, `"crypto/x509/pkix"`, `"math/big"`, `"time"`, `"fmt"`):

```go
// formDocument builds a one-page document with n text fields — many small
// dictionaries, which is what object streams are for.
func formDocument(t *testing.T, n int) *Document {
	t.Helper()
	doc := NewDocument(612, 792)
	for i := 0; i < n; i++ {
		row, col := i/10, i%10
		rect := Rectangle{LLX: 20 + float64(col)*58, LLY: 740 - float64(row)*14, URX: 74 + float64(col)*58, URY: 752 - float64(row)*14}
		f, err := doc.Form().AddTextField(1, rect, fmt.Sprintf("field%03d", i))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetValue(fmt.Sprintf("value %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

func saveCompressed(t *testing.T, doc *Document) []byte {
	t.Helper()
	if _, err := doc.Optimize(OptimizationOptions{CompressObjects: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A compressed save must reopen to the same document, carry object streams
// and a cross-reference stream, and be smaller than the classic save.
func TestCompressObjectsRoundTrip(t *testing.T) {
	var plain bytes.Buffer
	if _, err := formDocument(t, 150).WriteTo(&plain); err != nil {
		t.Fatal(err)
	}
	out := saveCompressed(t, formDocument(t, 150))

	if !bytes.HasPrefix(out, []byte("%PDF-1.5")) {
		t.Errorf("header = %q, want %%PDF-1.5", out[:8])
	}
	if !bytes.Contains(out, []byte("/ObjStm")) {
		t.Error("no object stream in the output")
	}
	if !bytes.Contains(out, []byte("/XRef")) {
		t.Error("no cross-reference stream in the output")
	}
	if bytes.Contains(out, []byte("\nxref\n")) {
		t.Error("a classic xref table was still written")
	}
	if len(out) >= plain.Len() {
		t.Errorf("compressed %d bytes, classic %d — no saving", len(out), plain.Len())
	}

	doc, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("page count = %d, want 1", doc.PageCount())
	}
	f := doc.Form().Field("field149")
	if f == nil || f.Value() != "value 149" {
		t.Errorf("field149 did not survive: %#v", f)
	}
	if n := len(doc.Form().Fields()); n != 150 {
		t.Errorf("%d fields after reopen, want 150", n)
	}
}

// Encrypted objects inside an object stream are protected by the stream's
// encryption only; every algorithm must reopen with the password.
func TestCompressObjectsEncrypted(t *testing.T) {
	for _, alg := range []EncryptionAlgorithm{EncryptionAlgRC4_128, EncryptionAlgAES128, EncryptionAlgAES256} {
		t.Run(fmt.Sprint(alg), func(t *testing.T) {
			doc := formDocument(t, 120)
			doc.SetEncryption(EncryptionOptions{UserPassword: "u", OwnerPassword: "o", Algorithm: alg})
			out := saveCompressed(t, doc)
			if bytes.Contains(out, []byte("value 7")) {
				t.Error("a field value is readable in the clear")
			}
			reopened, err := OpenStreamWithPassword(bytes.NewReader(out), "u")
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			f := reopened.Form().Field("field007")
			if f == nil || f.Value() != "value 7" {
				t.Errorf("field007 = %#v, want value 7", f)
			}
		})
	}
}

// A document signed on a compressed save keeps its signature dictionary out of
// the object streams, so the placeholders are patched and the signature holds.
func TestCompressObjectsSigned(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "ObjStm Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	doc := formDocument(t, 120)
	if err := doc.Sign(SignOptions{Certificate: cert, PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	out := saveCompressed(t, doc)

	reopened, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || !sigs[0].Valid || !sigs[0].CoversWholeDocument {
		t.Fatalf("signature on a compressed save does not hold: %+v", sigs)
	}
}

func TestOptimizeReportsPackableObjects(t *testing.T) {
	doc := formDocument(t, 30)
	res, err := doc.Optimize(OptimizationOptions{CompressObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.CompressedObjects < 30 {
		t.Errorf("CompressedObjects = %d, want at least 30", res.CompressedObjects)
	}
	if !DefaultOptimizationOptions().CompressObjects {
		t.Error("CompressObjects is not in the default preset")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestCompressObjects|TestOptimizeReportsPackableObjects" .`
Expected: FAIL — `unknown field CompressObjects in struct literal`.

- [ ] **Step 3: Add the option and the flag**

`document.go` — add to the `Document` struct next to `revisionAppended`:

```go
	// compressObjects makes Save/WriteTo pack non-stream objects into object
	// streams and write a cross-reference stream (OptimizationOptions.CompressObjects).
	compressObjects bool
```

`optimize.go` — add to `OptimizationOptions` after `RemoveDuplicateStreams`:

```go
	// CompressObjects packs non-stream objects into object streams and writes
	// a cross-reference stream on Save/WriteTo (lossless; the file becomes
	// PDF 1.5 or later). Ignored by SaveLinearized.
	CompressObjects bool
```

to `OptimizationResult`:

```go
	// CompressedObjects is the number of objects eligible for packing when
	// Optimize ran; the packing itself happens on Save/WriteTo.
	CompressedObjects int
```

to `DefaultOptimizationOptions`:

```go
		CompressObjects:        true,
```

and at the end of `Optimize`, after the `RemoveUnusedObjects` block (so the count reflects what survives):

```go
	if opts.CompressObjects {
		d.compressObjects = true
		res.CompressedObjects = d.countPackableObjects()
	}
```

Also extend the doc comment of `DefaultOptimizationOptions` to name object-stream packing among the lossless steps.

- [ ] **Step 4: Write the layout writer**

Append to `objstm_write.go` (add `"fmt"` and `"sort"` to the imports):

```go
// objectPackable reports whether an object may live inside an object stream:
// not a stream, not the encryption dictionary, and not a signature dictionary,
// whose /Contents and /ByteRange are patched in place by byte offset.
func objectPackable(id int, v pdfValue, encryptObjID int) bool {
	if id == encryptObjID && encryptObjID != 0 {
		return false
	}
	switch t := v.(type) {
	case *pdfStream:
		return false
	case pdfDict:
		return !isSignatureDict(t)
	}
	return true
}

// countPackableObjects counts the document's objects eligible for packing.
func (d *Document) countPackableObjects() int {
	n := 0
	for num, obj := range d.objects {
		if objectPackable(num, obj.Value, 0) {
			n++
		}
	}
	return n
}

// buildObjectStreamPDF lays out an assembled document with eligible objects
// packed into object streams and a cross-reference stream in place of the
// classic table. Numbering comes from assemble; object streams and the
// cross-reference stream take the numbers after it.
func buildObjectStreamPDF(d *Document, asm *assembled) ([]byte, error) {
	encState := asm.encState
	remapFn := asm.remapFn()
	identity := func(n int) int { return n }

	type item struct {
		id      int
		val     pdfValue
		remap   func(int) int
		encrypt bool
	}
	items := make([]item, 0, len(asm.contentIDs)+4)
	for _, oldID := range asm.contentIDs {
		items = append(items, item{asm.remap[oldID], d.objects[oldID].Value, remapFn, true})
	}
	kids := make(pdfArray, len(d.pages))
	for i, p := range d.pages {
		kids[i] = pdfDirectRef{Num: remapFn(p.Num)}
	}
	items = append(items, item{asm.pagesObjID,
		pdfDict{"/Type": pdfName("/Pages"), "/Count": len(d.pages), "/Kids": kids}, identity, false})
	items = append(items, item{asm.catalogObjID, pdfValue(asm.catalog), remapFn, true})
	if asm.infoObjID != 0 {
		items = append(items, item{asm.infoObjID, pdfValue(d.info), remapFn, true})
	}
	if asm.encryptObjID != 0 {
		items = append(items, item{asm.encryptObjID, pdfValue(buildEncryptDict(encState)), identity, false})
	}

	header := asm.header
	if header == "%PDF-1.4\n" {
		header = "%PDF-1.5\n"
	}
	var buf bytes.Buffer
	buf.WriteString(header)
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	encFor := func(num int) func([]byte) ([]byte, error) {
		if encState == nil {
			return nil
		}
		return func(b []byte) ([]byte, error) { return encState.encryptBytes(num, 0, b) }
	}

	offsets := make(map[int]int64, len(items))
	var packed []packedObj
	for _, it := range items {
		if objectPackable(it.id, it.val, asm.encryptObjID) {
			// Written without per-object encryption: the object stream that
			// holds it is encrypted as a whole.
			var body bytes.Buffer
			if err := writeValue(&body, it.val, it.remap, nil); err != nil {
				return nil, err
			}
			packed = append(packed, packedObj{num: it.id, body: body.Bytes()})
			continue
		}
		offsets[it.id] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if it.encrypt {
			encFn = encFor(it.id)
		}
		if err := writeObject(&buf, it.id, it.val, it.remap, encFn); err != nil {
			return nil, err
		}
	}
	sort.Slice(packed, func(i, j int) bool { return packed[i].num < packed[j].num })

	type location struct{ stream, index int }
	where := make(map[int]location, len(packed))
	nextID := asm.totalObjects
	for start := 0; start < len(packed); start += objStmCapacity {
		end := start + objStmCapacity
		if end > len(packed) {
			end = len(packed)
		}
		batch := packed[start:end]
		data, first := buildObjectStream(batch)
		num := nextID
		nextID++
		for i, o := range batch {
			where[o.num] = location{num, i}
		}
		offsets[num] = int64(buf.Len())
		st := &pdfStream{
			Dict:    pdfDict{"/Type": pdfName("/ObjStm"), "/N": len(batch), "/First": first},
			Data:    data,
			Decoded: true,
		}
		if err := writeObject(&buf, num, st, identity, encFor(num)); err != nil {
			return nil, err
		}
	}

	xrefNum := nextID
	size := xrefNum + 1
	xrefOff := int64(buf.Len())
	rows := make([]xrefStreamEntry, size)
	rows[0] = xrefStreamEntry{typ: 0, f2: 0, f3: 65535}
	for n := 1; n < size; n++ {
		if l, ok := where[n]; ok {
			rows[n] = xrefStreamEntry{typ: 2, f2: int64(l.stream), f3: l.index}
		} else if off, ok := offsets[n]; ok {
			rows[n] = xrefStreamEntry{typ: 1, f2: off}
		}
	}
	rows[xrefNum] = xrefStreamEntry{typ: 1, f2: xrefOff}
	data, w := encodeXRefEntries(rows)

	dict := pdfDict{
		"/Type": pdfName("/XRef"),
		"/Size": size,
		"/W":    w,
		"/Root": pdfDirectRef{Num: asm.catalogObjID},
	}
	if asm.infoObjID != 0 {
		dict["/Info"] = pdfDirectRef{Num: asm.infoObjID}
	}
	if encState != nil {
		dict["/Encrypt"] = pdfDirectRef{Num: asm.encryptObjID}
		dict["/ID"] = pdfArray{pdfHexString(encState.fileID), pdfHexString(encState.fileID)}
	}
	// The cross-reference stream is never encrypted (ISO 32000-1 §7.5.8.2).
	if err := writeObject(&buf, xrefNum, &pdfStream{Dict: dict, Data: data, Decoded: true}, identity, nil); err != nil {
		return nil, err
	}
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes(), nil
}
```

- [ ] **Step 5: Branch in the writer**

`writer.go`, in `buildDocumentPDF`, directly after `remapFn := asm.remapFn()` — before `var buf bytes.Buffer`:

```go
	if d.compressObjects {
		out, err := buildObjectStreamPDF(d, asm)
		if err != nil {
			return nil, err
		}
		if d.sign != nil {
			return d.applySignature(out)
		}
		return out, nil
	}
```

If `remapFn` or another local becomes unused only on this path, the compiler will not complain since the classic path still uses them.

- [ ] **Step 6: Run the tests**

Run: `go test -run "TestCompressObjects|TestOptimizeReportsPackableObjects|TestOptimize|TestBuildObjectStream|TestEncodeXRefEntries" .`
Expected: PASS. Then the whole package: `go test .` — PASS. If an existing `Optimize` test asserts exact output bytes or a `%PDF-1.4` header after `DefaultOptimizationOptions`, update its expectation to the compressed form and say so in the commit message.

- [ ] **Step 7: Commit**

```bash
git add objstm_write.go objstm_write_internal_test.go writer.go document.go optimize.go
git commit -m "$(cat <<'EOF'
feat: CompressObjects packs objects into object streams (pdf-go-o0wy)

With OptimizationOptions.CompressObjects — now part of the default preset —
Save and WriteTo put every non-stream object into Flate-compressed object
streams a hundred at a time and replace the classic xref table with a
cross-reference stream. Streams, the encryption dictionary and signature
dictionaries stay ordinary indirect objects; a packed object is encrypted
only through the object stream that holds it, and the cross-reference stream
is never encrypted. Round trips hold for plain, RC4/AES-128/AES-256 and
signed documents. Mirrors Aspose.PDF for .NET's CompressObjects.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Incremental revisions after a cross-reference stream

**Files:**
- Modify: `sign_incremental.go` (`appendRevision`), `objstm_write.go` (append helper)
- Test: `objstm_write_internal_test.go` (append)

**Interfaces:**
- Consumes: `encodeXRefEntries`, `xrefStreamEntry` (Task 1); `saveCompressed`, `formDocument` (Task 2 tests).
- Produces: `func encodeRevisionXRef(rows []xrefRow) (data []byte, w, index pdfArray)`

- [ ] **Step 1: Write the failing test**

Append to `objstm_write_internal_test.go`:

```go
// A revision appended to a compressed file keeps the file's kind of cross
// reference: a stream after a stream. The earlier signature stays valid and
// the later one covers the whole file.
func TestIncrementalSignatureOnCompressedFile(t *testing.T) {
	newSigner := func(cn string) (*x509.Certificate, *rsa.PrivateKey) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano() % 1_000_000),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().AddDate(1, 0, 0),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}
		der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return cert, key
	}

	cert1, key1 := newSigner("First")
	doc := formDocument(t, 120)
	if err := doc.Sign(SignOptions{Certificate: cert1, PrivateKey: key1}); err != nil {
		t.Fatal(err)
	}
	first := saveCompressed(t, doc)

	reopened, err := OpenStream(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	cert2, key2 := newSigner("Second")
	if err := reopened.Sign(SignOptions{Certificate: cert2, PrivateKey: key2}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := reopened.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	second := buf.Bytes()

	appended := second[len(first):]
	if bytes.Contains(appended, []byte("\nxref\n")) {
		t.Error("a classic xref section was appended to a file using cross-reference streams")
	}
	if !bytes.Contains(appended, []byte("/XRef")) {
		t.Error("the appended revision carries no cross-reference stream")
	}

	final, err := OpenStream(bytes.NewReader(second))
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := final.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signatures, want 2", len(sigs))
	}
	for _, s := range sigs {
		if !s.Valid {
			t.Errorf("signature %s is not valid: %v", s.FieldName, s.Err)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestIncrementalSignatureOnCompressedFile .`
Expected: FAIL — "a classic xref section was appended to a file using cross-reference streams".

- [ ] **Step 3: Add the revision encoder**

Append to `objstm_write.go`:

```go
// encodeRevisionXRef encodes the rows of an appended revision (sorted by
// number, all in use) as a cross-reference stream, grouping consecutive
// numbers into /Index subsections.
func encodeRevisionXRef(rows []xrefRow) (data []byte, w, index pdfArray) {
	entries := make([]xrefStreamEntry, len(rows))
	for i, r := range rows {
		entries[i] = xrefStreamEntry{typ: 1, f2: r.off, f3: r.gen}
	}
	for i := 0; i < len(rows); {
		j := i
		for j+1 < len(rows) && rows[j+1].num == rows[j].num+1 {
			j++
		}
		index = append(index, rows[i].num, j-i+1)
		i = j + 1
	}
	data, w = encodeXRefEntries(entries)
	return data, w, index
}
```

- [ ] **Step 4: Branch in appendRevision**

In `sign_incremental.go`, `appendRevision`: the block after `rows` is filled currently reads

```go
	xrefOff := int64(buf.Len())
	writeIncrementalXref(&buf, rows)

	id0, id1 := d.incrementalID()
```

Insert, immediately before `xrefOff := int64(buf.Len())`:

```go
	// A file whose last section is a cross-reference stream gets one too:
	// readers generally accept a classic table appended after a stream, strict
	// validators do not.
	if isXRefStream(d.source, prevXref) {
		xrefNum := size
		size++
		xrefOff := int64(buf.Len())
		rows = append(rows, xrefRow{num: xrefNum, off: xrefOff})
		data, w, index := encodeRevisionXRef(rows)
		id0, id1 := d.incrementalID()
		dict := pdfDict{
			"/Type":  pdfName("/XRef"),
			"/Size":  size,
			"/W":     w,
			"/Index": index,
			"/Root":  pdfDirectRef{Num: d.catalogNum},
			"/Prev":  int(prevXref),
			"/ID":    pdfArray{pdfHexString(id0), pdfHexString(id1)},
		}
		if encState != nil && d.encryptObjNum > 0 {
			dict["/Encrypt"] = pdfDirectRef{Num: d.encryptObjNum}
		}
		if err := writeObject(&buf, xrefNum, &pdfStream{Dict: dict, Data: data, Decoded: true}, identity, nil); err != nil {
			return nil, err
		}
		fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)
		return buf.Bytes(), nil
	}

```

`identity`, `size`, `rows`, `prevXref`, `encState` are already in scope in `appendRevision`.

- [ ] **Step 5: Run the tests**

Run: `go test -run "TestIncrementalSignatureOnCompressedFile|TestSign|TestVerify|TestAddValidationInfo|TestOfflineVerification" .`
Expected: PASS — the existing classic-file incremental tests are unaffected.

- [ ] **Step 6: Commit**

```bash
git add objstm_write.go objstm_write_internal_test.go sign_incremental.go
git commit -m "$(cat <<'EOF'
feat: append cross-reference streams to compressed files (pdf-go-o0wy)

An incremental revision — a further signature, AddValidationInfo — now uses
the same kind of cross reference as the file it extends: a stream with
/Index subsections and /Prev after a cross-reference stream, a classic table
after a classic one. Readers mostly tolerate the mix; strict validators do
not. A second signature on a compressed file verifies alongside the first.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Corpus check with qpdf, documentation

**Files:**
- Throwaway (not committed): `result_files/objstm/sweep_test.go` copied into the root as `zz_objstm_sweep_test.go` while running, and `result_files/objstm/check.py`
- Modify: `CLAUDE.md`, `CHANGELOG.md`

- [ ] **Step 1: Resave the corpus compressed**

Create `zz_objstm_sweep_test.go` in the repository root (deleted in Step 3):

```go
package asposepdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestObjStmCorpusSweep(t *testing.T) {
	dir := os.Getenv("OBJSTM_CORPUS")
	if dir == "" {
		t.Skip("OBJSTM_CORPUS not set")
	}
	out := filepath.Join("result_files", "objstm", "out")
	_ = os.MkdirAll(out, 0o755)
	files, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
	var before, after int64
	ok, failed := 0, 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		doc, err := OpenStream(bytes.NewReader(src))
		if err != nil {
			continue // encrypted or unreadable sources are out of scope
		}
		var plain bytes.Buffer
		if _, err := doc.WriteTo(&plain); err != nil {
			continue
		}
		doc2, err := OpenStream(bytes.NewReader(src))
		if err != nil {
			continue
		}
		doc2.compressObjects = true
		var comp bytes.Buffer
		if _, err := doc2.WriteTo(&comp); err != nil {
			failed++
			fmt.Printf("WRITE %s: %v\n", filepath.Base(f), err)
			continue
		}
		back, err := OpenStream(bytes.NewReader(comp.Bytes()))
		if err != nil || back.PageCount() != doc.PageCount() {
			failed++
			fmt.Printf("REOPEN %s: %v\n", filepath.Base(f), err)
			continue
		}
		before += int64(plain.Len())
		after += int64(comp.Len())
		_ = os.WriteFile(filepath.Join(out, filepath.Base(f)), comp.Bytes(), 0o644)
		ok++
	}
	fmt.Printf("ok=%d failed=%d classic=%d compressed=%d saving=%.1f%%\n",
		ok, failed, before, after, 100*float64(before-after)/float64(before))
}
```

Run: `OBJSTM_CORPUS=D:/aspose/claude/external_testdata go test -run TestObjStmCorpusSweep -timeout 60m .`
Expected: `failed=0`; record the `saving=` figure for the commit message and CHANGELOG.

- [ ] **Step 2: Check the output with qpdf**

Create `result_files/objstm/check.py`:

```python
import pathlib, sys
import pikepdf

out = pathlib.Path("result_files/objstm/out")
bad = 0
total = 0
for p in sorted(out.glob("*.pdf")):
    total += 1
    try:
        with pikepdf.open(p) as pdf:
            problems = [str(x) for x in pdf.check()]
            len(pdf.pages)
        if problems:
            bad += 1
            print(p.name, problems[:3])
    except Exception as e:
        bad += 1
        print(p.name, "OPEN", e)
print(f"checked={total} with_problems={bad}")
sys.exit(1 if bad else 0)
```

Run: `python result_files/objstm/check.py`
Expected: `with_problems=0`. For any document listed, check whether qpdf reports the same problem on the *classic* save of that document (write it with `compressObjects` false and run `pdf.check()` on it); a problem present in both is inherited from the source and not caused by this work. Fix any problem that appears only in the compressed output before continuing.

- [ ] **Step 3: Remove the throwaway test**

Run: `rm zz_objstm_sweep_test.go` — it must not be committed (`result_files/` is gitignored).

- [ ] **Step 4: Documentation**

`CLAUDE.md` — in the `Optimize` bullet (`optimize.go`), replace the sentence beginning "Object-stream packing (`CompressObjects`/ObjStm), `UnembedFonts`, and RC4-40 are deferred follow-ups (`pdf-go-qf2g`)" with:

```markdown
**`CompressObjects`** (in the default preset, `objstm_write.go`, `pdf-go-o0wy`): `Optimize` sets `Document.compressObjects`, and `buildDocumentPDF` branches to `buildObjectStreamPDF`, which writes ineligible objects as ordinary indirect objects, every other object in Flate-compressed `/ObjStm` streams of up to 100 (`objStmCapacity`), and a `/Type /XRef` cross-reference stream instead of the classic table (header raised to at least `%PDF-1.5`). Never packed (`objectPackable`): streams, the encryption dictionary, signature dictionaries (their placeholders are patched by byte offset). A packed object is written without per-object encryption — the object stream is encrypted as a stream under its own number — and the cross-reference stream is never encrypted. `appendRevision` writes a cross-reference stream section (`encodeRevisionXRef`, `/Index` subsections + `/Prev`) when the file's last section is one, so incremental signing and `AddValidationInfo` keep the file's kind. The flag lives on the in-memory document only (reopening a compressed file and saving writes classic unless `Optimize` runs again) and `SaveLinearized` ignores it. Corpus: <ok> documents resaved compressed, qpdf `check()` clean, <saving>% smaller than the classic save. `UnembedFonts` is the remaining follow-up
```

(fill `<ok>` and `<saving>` from Step 1).

`CHANGELOG.md` — first bullet under `## [Unreleased]` → `### Added`:

```markdown
- **Object streams** — `OptimizationOptions.CompressObjects`, now part of `DefaultOptimizationOptions()`, packs every non-stream object into Flate-compressed object streams and writes a cross-reference stream instead of the classic xref table: lossless, <saving>% smaller than a classic save across the test corpus, and checked clean by qpdf. Streams, the encryption dictionary and signature dictionaries stay ordinary objects, encrypted and signed documents round-trip, and a later incremental revision keeps the file's cross-reference kind so earlier signatures stay valid. Mirrors Aspose.PDF for .NET's `CompressObjects`. (`pdf-go-o0wy`)
```

- [ ] **Step 5: Full gate**

Run: `gofmt -l objstm_write.go objstm_write_internal_test.go writer.go document.go optimize.go sign_incremental.go; go vet ./... && go test ./... && golangci-lint run`
Expected: gofmt lists nothing, vet silent, tests pass, `0 issues`.

- [ ] **Step 6: Commit and track the follow-up**

```bash
git add CLAUDE.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs: object streams (pdf-go-o0wy)

Corpus: <ok> documents resaved compressed, qpdf check() clean, <saving>%
smaller than the classic save.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
bd create "Optimize: UnembedFonts" --body "Phase 2 of pdf-go-o0wy: drop an embedded font program when a metric-compatible standard/system face covers it, rewriting the font dict (lossy for exotic faces, opt-in). Mirrors Aspose.PDF for .NET's OptimizationOptions.UnembedFonts."
bd close pdf-go-o0wy
```

---

## Self-Review

**Spec coverage.** Public API (`CompressObjects`, `CompressedObjects`, default preset, flag honoured on later saves, linearization ignores it) — Task 2 Step 3 and Task 4 docs. Layout order and object-stream/cross-reference-stream encoding — Tasks 1 and 2. Eligibility — `objectPackable`, Task 2. Header floor — Task 2. Encryption semantics — Task 2 writer + `TestCompressObjectsEncrypted` (all three algorithms). Full-rewrite signing — `TestCompressObjectsSigned`. Incremental revisions keep the cross-reference kind — Task 3. Validation list: round trip, encryption, signatures, structure unit tests, qpdf corpus check — Tasks 1–4. Out-of-scope items are not implemented; `UnembedFonts` becomes a tracked issue in Task 4.

**Placeholder scan.** The only fill-ins are the corpus figures `<ok>`/`<saving>` in Task 4, which come from Step 1's output by construction. 

**Type consistency.** `packedObj{num, body}`, `buildObjectStream`, `xrefStreamEntry{typ, f2, f3}`, `encodeXRefEntries` are defined in Task 1 and used unchanged in Tasks 2–3. `objectPackable(id, v, encryptObjID)` is used by `countPackableObjects` and `buildObjectStreamPDF`. `encodeRevisionXRef` consumes the existing `xrefRow{num, gen, off}`. Test helpers `formDocument`/`saveCompressed` are defined in Task 2 and reused in Task 3.
