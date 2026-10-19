// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// documentFileID returns the two /ID elements for a fresh (non-incremental)
// trailer: the original file's /ID when the document was opened from one
// (ISO 32000-1 §14.4's "permanent identifier" is kept across a re-save that
// way), otherwise a deterministic pair derived by hashing body — the bytes
// already written for this save — so repeated saves of the same in-memory
// content get the same /ID rather than omitting one, as PDF/A requires
// (ISO 19005-2 §6.1.3). Shared with incrementalID, which passes the
// original source bytes instead of a freshly-built body.
func (d *Document) documentFileID(body []byte) ([]byte, []byte) {
	get := func(v pdfValue) []byte {
		switch s := v.(type) {
		case pdfHexString:
			return []byte(s)
		case string:
			return []byte(s)
		}
		return nil
	}
	if len(d.docID) == 2 {
		id0, id1 := get(d.docID[0]), get(d.docID[1])
		if id0 != nil && id1 != nil {
			return id0, id1
		}
	}
	sum := md5.Sum(body)
	return sum[:], sum[:]
}

// buildDocumentPDF serializes d to a PDF byte slice.
func buildDocumentPDF(d *Document) ([]byte, error) {
	// A document whose source already carries an appended revision built in
	// memory (AddValidationInfo) is written back verbatim: rebuilding it would
	// drop the revision, and with it the /DSS.
	if d.revisionAppended {
		return d.source, nil
	}

	// SignOptions.LTV: sign first, then append the validation material as a
	// further revision. It can only be tied to a signature that exists.
	if d.sign != nil && d.sign.ltv {
		return buildSignedWithLTV(d)
	}

	// Incremental signature: append a new revision to the original bytes
	// rather than rewriting, so any earlier signature stays valid.
	if d.sign != nil && d.sign.incremental {
		return buildIncrementalSignedPDF(d)
	}

	// Signing a freshly-built (no source) encrypted document: build the
	// encrypted PDF first, then sign it incrementally — the only approach that
	// stays interoperable, since the signature objects must be encrypted and a
	// signed revision must sit atop the finished encrypted bytes.
	if d.sign != nil && !d.sign.incremental && len(d.source) == 0 && (d.encrypt != nil || d.preserved != nil) {
		return buildFreshEncryptedSignedPDF(d)
	}

	asm, err := d.assemble()
	if err != nil {
		return nil, err
	}
	encState := asm.encState
	contentIDs := asm.contentIDs
	remap := asm.remap
	pagesObjID := asm.pagesObjID
	catalogObjID := asm.catalogObjID
	infoObjID := asm.infoObjID
	encryptObjID := asm.encryptObjID
	totalObjects := asm.totalObjects
	remapFn := asm.remapFn()

	// PDF/A-1 (PDF 1.4-based) forbids object and cross-reference streams; check
	// the document's own XMP here rather than trusting compressObjects alone,
	// so the guard holds regardless of Optimize/ConvertToPDFA call order and
	// for a document opened from an existing PDF/A-1 file (see isPDFA1).
	if d.compressObjects && !d.isPDFA1() {
		out, err := buildObjectStreamPDF(d, asm)
		if err != nil {
			return nil, err
		}
		if d.sign != nil {
			return d.applySignature(out)
		}
		return out, nil
	}

	var buf bytes.Buffer
	buf.WriteString(asm.header)
	buf.WriteString("%\xe2\xe3\xcf\xd3\n") // binary marker

	offsets := make(map[int]int64, totalObjects)

	// Write content objects.
	for _, oldID := range contentIDs {
		obj := d.objects[oldID]
		outID := remap[oldID]
		offsets[outID] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(outID, 0, b) }
		}
		if err := writeObject(&buf, outID, obj.Value, remapFn, encFn); err != nil {
			return nil, err
		}
	}

	// Write /Pages node.
	offsets[pagesObjID] = int64(buf.Len())
	writePageTreeNode(&buf, pagesObjID, d.pages, remapFn)

	// Write /Catalog. Preserve every field from the original catalog
	// (/Outlines, /AcroForm, /Names, /PageLabels, /Metadata, etc.) so
	// a Save+Reopen roundtrip is lossless. /Pages is replaced with the
	// writer-built node; other refs are remapped by writeValue. Deep-copy
	// so that building the output catalog never aliases d.catalog.
	offsets[catalogObjID] = int64(buf.Len())
	catOut := asm.catalog
	var catalogEncFn func([]byte) ([]byte, error)
	if encState != nil {
		catalogEncFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(catalogObjID, 0, b) }
	}
	if err := writeObject(&buf, catalogObjID, pdfValue(catOut), remapFn, catalogEncFn); err != nil {
		return nil, err
	}

	// Write /Info if present.
	if infoObjID != 0 {
		offsets[infoObjID] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(infoObjID, 0, b) }
		}
		if err := writeObject(&buf, infoObjID, pdfValue(d.info), remapFn, encFn); err != nil {
			return nil, err
		}
	}

	// Write /Encrypt if present.
	if encryptObjID != 0 {
		offsets[encryptObjID] = int64(buf.Len())
		encDict := buildEncryptDict(encState)
		if err := writeObject(&buf, encryptObjID, pdfValue(encDict), func(n int) int { return n }, nil); err != nil {
			return nil, err
		}
	}

	// Write xref table.
	xrefOffset := int64(buf.Len())
	fmt.Fprintf(&buf, "xref\n0 %d\n", totalObjects)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i < totalObjects; i++ {
		off, ok := offsets[i]
		if !ok {
			fmt.Fprintf(&buf, "0000000000 00000 f \n")
		} else {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		}
	}

	// Write trailer.
	buf.WriteString("trailer\n<<\n")
	fmt.Fprintf(&buf, "/Size %d\n", totalObjects)
	fmt.Fprintf(&buf, "/Root %d 0 R\n", catalogObjID)
	if infoObjID != 0 {
		fmt.Fprintf(&buf, "/Info %d 0 R\n", infoObjID)
	}
	if encState != nil {
		fmt.Fprintf(&buf, "/Encrypt %d 0 R\n", encryptObjID)
		buf.WriteString("/ID [")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString(" ")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString("]\n")
	} else {
		id0, id1 := d.documentFileID(buf.Bytes())
		buf.WriteString("/ID [")
		writeHexBytes(&buf, id0)
		buf.WriteString(" ")
		writeHexBytes(&buf, id1)
		buf.WriteString("]\n")
	}
	buf.WriteString(">>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	if d.sign != nil {
		return d.applySignature(buf.Bytes())
	}
	return buf.Bytes(), nil
}

// writePageTreeNode writes the /Pages node with kids pointing to pages.
func writePageTreeNode(buf *bytes.Buffer, pagesObjID int, pages []*pdfObject, remapFn func(int) int) {
	fmt.Fprintf(buf, "%d 0 obj\n<<\n/Type /Pages\n/Count %d\n/Kids [", pagesObjID, len(pages))
	for i, page := range pages {
		if i > 0 {
			buf.WriteString(" ")
		}
		fmt.Fprintf(buf, "%d 0 R", remapFn(page.Num))
	}
	buf.WriteString("]\n>>\nendobj\n")
}

// writeObject writes "N 0 obj\n...\nendobj\n" for the given value.
func writeObject(buf *bytes.Buffer, id int, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
	fmt.Fprintf(buf, "%d 0 obj\n", id)
	if err := writeValue(buf, v, remapFn, encFn); err != nil {
		return err
	}
	buf.WriteString("\nendobj\n")
	return nil
}

// sortedObjectIDs returns the object IDs from objects in ascending order.
func sortedObjectIDs(objects map[int]*pdfObject) []int {
	ids := make([]int, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// writeFloatValue formats a float64 dict/array value per ISO 32000-1
// §7.3.3: an optional sign, digits, and an optional decimal point — no
// exponent form at all, unlike Go's 'g' verb, which switches to it for
// small- or large-magnitude numbers (e.g. a tiny floating-point rounding
// remainder from a computed layout, written as "-1e-13"). Not a merely
// theoretical strictness concern: this library's OWN parser/lexer, which
// correctly implements only the spec's no-exponent grammar, could not
// read "-1e-13" back at all — a document saved with such a value and
// reopened with this same library lost the annotation carrying it
// entirely (confirmed directly by reverting this fix and re-running
// TestWriteValueNeverUsesExponentNotation, writer_float_internal_test.go,
// whose round-trip assertion failed with 0 annotations found instead of
// 1). GitHub issue #16 point 4, shadowhunter1470-svg. Content-stream
// numbers already go through a
// different, correct helper (appearance_builder.go's formatFloat:
// fixed-notation, 6 decimal places, trailing zeros/point trimmed) — only
// this dictionary/array value path was affected.
//
// 'f' with precision -1 gives the same "shortest string that parses back
// to the exact same float64" guarantee 'g' does (strconv's doc: the
// special precision applies to 'f' too, not only 'e'/'g'), just without
// ever switching notation — so this changes representation, not
// precision. NaN/±Inf (which 'f'/'g' both render as non-numeric text no
// PDF reader can parse at all) are refused outright rather than written
// as garbage; reaching this from the public API means something upstream
// already computed a non-finite coordinate, which this library has no
// sensible default for.
func writeFloatValue(buf *bytes.Buffer, val float64) error {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return fmt.Errorf("asposepdf: cannot write non-finite number %v to a PDF dictionary/array (ISO 32000-1 §7.3.3 defines no such value)", val)
	}
	buf.WriteString(strconv.FormatFloat(val, 'f', -1, 64))
	return nil
}

// writeNameValue writes a PDF name per ISO 32000-1 §7.3.5: every byte
// outside the printable range 0x21-0x7E, plus "#" itself and the eight
// delimiter characters ( ) < > [ ] { } / %, is written as "#xx" (two
// uppercase hex digits) — everywhere in this codebase, including every
// caller-facing API that writes one (pdf-go-zg1d, reported privately
// by shadowhunter1470-svg), a pdfName value was written VERBATIM,
// untouched by this escaping. Where the name came from caller data —
// a Document.Info custom key, a font's /BaseFont read from a loaded
// font file, SetRawName, an AcroForm radio button's export name — a
// delimiter or whitespace byte in it ended the name early and the rest
// became further PDF syntax the caller never intended to write: the
// reproduction in the report turns one Info custom entry
// (`"Dept /Injected (yes) /X": "v"`) into two actual dictionary
// entries. Escaping centrally here, rather than at each of those call
// sites, means every current AND future caller-facing name path is
// covered by construction, not by remembering to escape at the edge.
//
// The leading "/" (byte 0, by this codebase's convention: every
// pdfName value, including every literal in this file's own source,
// already carries it — see e.g. the SetRawName/RawName doc comments)
// is written literally; it is the name's own introducer, not part of
// its value, and is not itself escaped even though it also appears in
// the delimiter list above (a bare, unescaped "/" is exactly what
// starts every name token, including this one). A name with no leading
// "/" at all (malformed input from a caller who forgot it, a
// pre-existing, narrower issue this fix does not change) still gets
// byte 0 written as-is and everything after it escaped, rather than
// this function guessing a slash that was never there.
//
// The reader decodes the same "#xx" escapes back (decodeNameEscapes,
// lexer.go) so a name written this way reads back as the exact same
// name, per spec ("/A#20B" and "A B" are the same name) — without
// that, this fix alone would make this library's own round-trip of a
// name needing escaping come back different than what was written.
func writeNameValue(buf *bytes.Buffer, name pdfName) error {
	b := []byte(name)
	if len(b) == 0 {
		return fmt.Errorf("asposepdf: cannot write an empty PDF name")
	}
	buf.WriteByte(b[0])
	for _, c := range b[1:] {
		if nameByteNeedsEscape(c) {
			fmt.Fprintf(buf, "#%02X", c)
		} else {
			buf.WriteByte(c)
		}
	}
	return nil
}

// nameByteNeedsEscape reports whether c must be written as "#xx" when
// it appears (after the name's own leading "/") in a PDF name: outside
// the regular/printable range 0x21-0x7E, or "#" itself, or one of the
// eight delimiter characters ISO 32000-1 §7.2.3 Table 1 defines.
func nameByteNeedsEscape(c byte) bool {
	if c < 0x21 || c > 0x7E {
		return true
	}
	switch c {
	case '#', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// writeValue serialises a PDF value to buf, remapping object reference numbers via remapFn.
// encFn, if non-nil, is called to encrypt string and stream data for the current object.
func writeValue(buf *bytes.Buffer, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
	switch val := v.(type) {
	case pdfNull:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(val))
	case float64:
		if err := writeFloatValue(buf, val); err != nil {
			return err
		}
	case pdfName:
		if err := writeNameValue(buf, val); err != nil {
			return err
		}
	case pdfRaw:
		// Verbatim — used for signature /Contents and /ByteRange placeholders.
		buf.Write(val)
	case string:
		if encFn != nil {
			enc, err := encFn([]byte(val))
			if err != nil {
				return err
			}
			writeHexBytes(buf, enc)
		} else {
			buf.WriteString("(")
			buf.WriteString(escapeLiteral(val))
			buf.WriteString(")")
		}
	case pdfHexString:
		if encFn != nil {
			enc, err := encFn([]byte(val))
			if err != nil {
				return err
			}
			writeHexBytes(buf, enc)
		} else {
			writeHexBytes(buf, []byte(val))
		}
	case pdfRef:
		buf.WriteString(strconv.Itoa(remapFn(val.Num)))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDirectRef:
		// Already in new object space — write as-is, no remapping.
		buf.WriteString(strconv.Itoa(val.Num))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDict:
		buf.WriteString("<<")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("\n")
			// A dict key is itself a PDF name and needs the exact same
			// escaping a name VALUE does (writeNameValue) — this is the
			// actual line the report's reproduction exploits: a
			// Document.Info custom key written here verbatim let a
			// caller-supplied "Dept /Injected (yes) /X" turn into two
			// real dictionary entries instead of one oddly-named one.
			if err := writeNameValue(buf, pdfName(k)); err != nil {
				return err
			}
			buf.WriteString(" ")
			if err := writeValue(buf, val[k], remapFn, encFn); err != nil {
				return err
			}
		}
		buf.WriteString("\n>>")
	case pdfArray:
		buf.WriteString("[")
		for i, item := range val {
			if i > 0 {
				buf.WriteString(" ")
			}
			if err := writeValue(buf, item, remapFn, encFn); err != nil {
				return err
			}
		}
		buf.WriteString("]")
	case *pdfStream:
		d := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			// Remove encoding filters — we re-compress decoded data below.
			// If Decoded==false the raw compressed bytes are preserved as-is,
			// so the original /Filter must stay in the dict.
			if val.Decoded && (k == "/Filter" || k == "/DecodeParms" || k == "/FFilter" || k == "/FDecodeParms") {
				continue
			}
			d[k] = dv
		}
		data := val.Data
		if val.Decoded {
			var zbuf bytes.Buffer
			w := zlib.NewWriter(&zbuf)
			_, _ = w.Write(data) // in-memory buffer: cannot fail
			_ = w.Close()
			data = zbuf.Bytes()
			d["/Filter"] = pdfName("/FlateDecode")
		}
		if encFn != nil {
			enc, err := encFn(data)
			if err != nil {
				return err
			}
			data = enc
		}
		d["/Length"] = len(data)
		if err := writeValue(buf, d, remapFn, nil); err != nil { // dict keys are not encrypted
			return err
		}
		buf.WriteString("\nstream\n")
		buf.Write(data)
		buf.WriteString("\nendstream")
	}
	return nil
}

// writeHexBytes writes b as a PDF hex string <AABB...>.
func writeHexBytes(buf *bytes.Buffer, b []byte) {
	const hex = "0123456789abcdef"
	buf.WriteByte('<')
	for _, c := range b {
		buf.WriteByte(hex[c>>4])
		buf.WriteByte(hex[c&0xf])
	}
	buf.WriteByte('>')
}

// escapeLiteral escapes special characters in a PDF literal string.
func escapeLiteral(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// buildEncryptDict builds the /Encrypt dictionary for the standard
// security handler. Branches on s.algorithm:
//   - EncryptionAlgRC4_128 → V=2 R=3, no /CF (default crypt filter is
//     implicit RC4 for V<4).
//   - EncryptionAlgRC4_40 → V=1 R=2, /Length 40, likewise no /CF.
//   - EncryptionAlgAES128 → V=4 R=4 with /CF/StdCF/CFM /AESV2 and
//     /StmF and /StrF pointing to /StdCF. ISO 32000-1 §7.6.3.2.
//
// /O and /U are raw 32-byte binary values emitted as hex strings to
// avoid literal-string parsing ambiguities (embedded NULs, etc.).
func buildEncryptDict(s *encryptState) pdfDict {
	if s.pubSec {
		return buildPubSecEncryptDict(s)
	}
	dict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/Length": 128,
		// /P is a 32-bit bitfield. Emit as unsigned so it reads as a positive
		// integer (e.g. 4294967292 for grant-all), matching Adobe/pypdf
		// convention and avoiding signed-int interop pitfalls.
		"/P": int(uint32(s.permissions)),
		"/O": pdfHexString(s.ownerEntry),
		"/U": pdfHexString(s.userEntry),
	}
	switch s.algorithm {
	case EncryptionAlgAES128:
		dict["/V"] = 4
		dict["/R"] = 4
		dict["/CF"] = pdfDict{
			"/StdCF": pdfDict{
				"/Type":      pdfName("/CryptFilter"),
				"/CFM":       pdfName("/AESV2"),
				"/AuthEvent": pdfName("/DocOpen"),
				"/Length":    16,
			},
		}
		dict["/StmF"] = pdfName("/StdCF")
		dict["/StrF"] = pdfName("/StdCF")
	case EncryptionAlgAES256:
		// A revision-5 file re-saved with its preserved state keeps /R 5:
		// its /U and /O hashes are only valid under that revision.
		dict["/V"] = 5
		dict["/R"] = 6
		if s.revision == 5 {
			dict["/R"] = 5
		}
		dict["/Length"] = 256
		dict["/OE"] = pdfHexString(s.ownerKeyEntry)
		dict["/UE"] = pdfHexString(s.userKeyEntry)
		if s.permsEntry != nil {
			dict["/Perms"] = pdfHexString(s.permsEntry)
		}
		dict["/EncryptMetadata"] = true
		dict["/CF"] = pdfDict{
			"/StdCF": pdfDict{
				"/Type":      pdfName("/CryptFilter"),
				"/CFM":       pdfName("/AESV3"),
				"/AuthEvent": pdfName("/DocOpen"),
				"/Length":    32,
			},
		}
		dict["/StmF"] = pdfName("/StdCF")
		dict["/StrF"] = pdfName("/StdCF")
	case EncryptionAlgRC4_40:
		// The original handler: no /V at all is legal, but writing 1 is
		// clearer and equally accepted.
		dict["/V"] = 1
		dict["/R"] = 2
		dict["/Length"] = 40
	default: // EncryptionAlgRC4_128
		dict["/V"] = 2
		dict["/R"] = 3
		// Revision 3 allows any key length from 40 to 128 bits; a document
		// opened from such a file keeps its key, so say how long it is.
		if n := len(s.key); n > 0 && n < 16 {
			dict["/Length"] = n * 8
		}
	}
	return dict
}
