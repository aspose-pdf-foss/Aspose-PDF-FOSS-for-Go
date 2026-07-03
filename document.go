// SPDX-License-Identifier: MIT

// Package asposepdf is a pure-Go, zero-dependency PDF library: read, create,
// edit, render, sign, encrypt, and validate PDF documents. Its public API is
// shaped to mirror Aspose.PDF for .NET (Document, Page, TextFragment, Table,
// Form, OutlineItemCollection, ...), adapted to Go idioms.
package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// Document is a PDF document. Operations directly mutate the receiver.
type Document struct {
	objects      map[int]*pdfObject     // all PDF objects by ID
	pages        []*pdfObject           // ordered /Page objects
	pageCache    []*Page                // cached live views by index, lazy-allocated
	catalog      pdfDict                // /Catalog dict
	info         pdfDict                // /Info dict; nil = no metadata
	encrypt      *encryptConfig         // nil = no encryption
	preserved    *encryptState          // captured verbatim at OpenWithPassword time; nil after any explicit mutation
	nextID       int                    // next available object ID
	outlinesRoot *OutlineItemCollection // nil until first Outlines() call
	namedDests   *NamedDestinations     // nil until first NamedDestinations() call
	js           *JavaScriptCollection  // nil until first JavaScript() call
	tagged       *TaggedContent         // nil until first TaggedContent() call
	sign         *signConfig            // nil unless Sign() configured a digital signature
	source       []byte                 // raw bytes the document was opened from; nil for built docs (used by VerifySignatures for /ByteRange)

	// Captured at open time from the trailer, used by incremental save
	// (signing an existing PDF without rewriting it). Zero for built docs.
	catalogNum    int      // original /Root object number
	docID         pdfArray // original trailer /ID (carried into the incremental trailer)
	origSize      int      // original trailer /Size (new object numbers start here)
	encryptObjNum int      // original /Encrypt object number (0 if unencrypted)

	// embeddedFonts lists every TTF loaded via LoadFont, in load order, so
	// (*Document).SubsetFonts can walk them and shrink each /FontFile2 to
	// only the glyphs that were actually drawn.
	embeddedFonts []*embeddedFont

	// formFonts maps an /AcroForm/DR/Font resource name (e.g. "Helv") to
	// the Go Font registered under it, so widget /AP generators can render
	// a field's styled text with the exact font the caller chose — including
	// embedded TTFs that FindFont can't reconstruct from a /BaseFont alone.
	// Populated by (*Form).ensureFont; nil until the first field is created
	// or styled. Not persisted: after a Save+Open round-trip it is empty and
	// generators fall back to resolving Standard 14 fonts from /DR/Font.
	formFonts map[string]Font

	// Phase 3b: SVG text font resolution callback (nil = heuristic only).
	svgFontResolver SVGFontResolver
}

// Open opens a PDF file and returns a Document.
//
// Example:
//
//	doc, err := asposepdf.Open("input.pdf")
func Open(path string) (*Document, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	return OpenStream(bytes.NewReader(data))
}

// OpenStream reads a PDF from r and returns a Document. Returns
// ErrEncrypted if the input is password-protected; in that case retry
// with OpenStreamWithPassword.
//
// Example:
//
//	doc, err := asposepdf.OpenStream(file)
func OpenStream(r io.Reader) (*Document, error) {
	return openStreamCore(r, nil)
}

// OpenWithPassword opens a password-protected PDF file. Use Open for
// unencrypted files. The password is tried as both user and owner
// password; either unlocks the document for editing.
//
// Edit-in-place: the original /O, /U, /P, and /ID bytes from the file
// are preserved on the returned Document so a subsequent Save reuses them
// verbatim — BOTH the original user and owner passwords continue to work
// after re-save, and permissions are preserved bit-for-bit.  If you call
// SetPassword, SetPermissions, SetEncryption, or RemoveEncryption after
// open, the preserved state is discarded and the document is re-encrypted
// from the new configuration.
//
// Example:
//
//	doc, err := asposepdf.OpenWithPassword("locked.pdf", "secret")
func OpenWithPassword(path, password string) (*Document, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	return OpenStreamWithPassword(bytes.NewReader(data), password)
}

// OpenStreamWithPassword reads a password-protected PDF from r. Plain
// (unencrypted) PDFs are also accepted — the password is silently
// ignored — so this method is a safe drop-in for code that doesn't know
// up front whether the input is encrypted.
//
// See OpenWithPassword for the edit-in-place preservation semantics.
func OpenStreamWithPassword(r io.Reader, password string) (*Document, error) {
	return openStreamCore(r, &password)
}

// openStreamCore is the shared implementation. password == nil means "no
// password supplied"; an encrypted file then returns ErrEncrypted.
func openStreamCore(r io.Reader, password *string) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}

	// Primary path: locate and parse the cross-reference table/stream, then
	// build the document. If anything in that chain fails — a missing or
	// corrupt xref, or a catalog/page tree that doesn't resolve (e.g. an
	// off-by-one xref subsection) — fall back to reconstructing the xref by
	// scanning the file for object headers and retry. ErrEncrypted is not a
	// failure to recover from: the file parsed fine, it just needs a password.
	var firstErr error
	if startOff, err := findStartXRef(data); err == nil {
		if xref, trailer, perr := parseXRef(data, startOff); perr == nil {
			doc, derr := buildFromXRef(data, xref, trailer, password)
			if derr == nil {
				doc.source = data
				return doc, nil
			}
			if errors.Is(derr, ErrEncrypted) {
				return nil, derr
			}
			firstErr = derr
		} else {
			firstErr = perr
		}
	} else {
		firstErr = err
	}

	xref, trailer, rerr := reconstructXRef(data)
	if rerr != nil {
		return nil, fmt.Errorf("parse PDF: %w", coalesceErr(firstErr, rerr))
	}
	doc, derr := buildFromXRef(data, xref, trailer, password)
	if derr != nil {
		if errors.Is(derr, ErrEncrypted) {
			return nil, derr
		}
		return nil, fmt.Errorf("parse PDF: %w", coalesceErr(firstErr, derr))
	}
	doc.source = data
	return doc, nil
}

// coalesceErr returns the first non-nil error, preferring first.
func coalesceErr(first, second error) error {
	if first != nil {
		return first
	}
	return second
}

// buildFromXRef assembles a Document from a parsed (or reconstructed)
// cross-reference table and trailer. Returns ErrEncrypted (unwrapped) when
// the file is encrypted and no password was supplied.
func buildFromXRef(data []byte, xref *xrefTable, trailer pdfDict, password *string) (*Document, error) {
	raw := newRawDocument(data, xref, trailer)

	// pendingEncrypt is set when the file was opened with a password so
	// that the resulting Document re-encrypts on Save by default.
	var pendingEncrypt *encryptConfig
	var pendingPreserved *encryptState

	if encVal, ok := trailer["/Encrypt"]; ok {
		if password == nil {
			return nil, ErrEncrypted
		}
		encRef, ok := encVal.(pdfRef)
		if !ok {
			return nil, fmt.Errorf("/Encrypt is not an indirect ref")
		}
		// The /Encrypt object itself is never encrypted, so we can fetch
		// it via getObject before configuring decryption on raw.
		encObj, err := raw.getObject(encRef.Num)
		if err != nil {
			return nil, fmt.Errorf("read /Encrypt: %w", err)
		}
		encDict, ok := encObj.Value.(pdfDict)
		if !ok {
			return nil, fmt.Errorf("/Encrypt is not a dict")
		}
		state, err := buildDecryptState(encDict, trailer, *password)
		if err != nil {
			return nil, err
		}
		raw.encState = state
		raw.encryptObjNum = encRef.Num
		// Capture the parsed encryptState verbatim so re-Save can reuse the
		// original /O, /U, /P, and /ID bytes without re-deriving from a single
		// password.  Both original passwords (user and owner) continue to work
		// because neither hash has changed.
		pendingPreserved = state
		// Also build a minimal encryptConfig so that callers who immediately
		// query Permissions() get a correct answer, and so the encrypt!=nil
		// sentinel works. The supplied password is stored as both slots —
		// it's only consulted if the user explicitly calls SetPassword et al.
		// and thereby clears pendingPreserved.
		pendingEncrypt = &encryptConfig{
			userPassword:   *password,
			ownerPassword:  *password,
			permissions:    state.permissions,
			hasPermissions: true,
		}
	}

	objects, err := parseAllObjectsFrom(raw)
	if err != nil {
		return nil, err
	}

	resolveIndirectStreamFilters(objects)

	// /Encrypt object — drop it from the working set; the writer rebuilds
	// /Encrypt from d.encrypt on save (or omits it for plain saves).
	if raw.encState != nil {
		delete(objects, raw.encryptObjNum)
	}

	catalog, err := extractCatalog(objects, trailer)
	if err != nil {
		return nil, err
	}

	pages, err := resolvePageTree(objects, catalog)
	if err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}

	// Remove structural page-tree and catalog objects — the writer rebuilds them.
	// Keeping them would produce orphaned /Pages nodes that fail validation.
	for id, obj := range objects {
		if d, ok := obj.Value.(pdfDict); ok {
			switch dictGetName(d, "/Type") {
			case "/Pages", "/Catalog":
				delete(objects, id)
			}
		}
	}

	doc := &Document{
		objects:   objects,
		pages:     pages,
		catalog:   catalog,
		info:      extractInfo(objects, trailer),
		nextID:    maxObjectID(objects) + 1,
		encrypt:   pendingEncrypt,
		preserved: pendingPreserved,
	}
	// Capture trailer facts needed for incremental save (Document.Sign on an
	// existing PDF appends a revision rather than rewriting).
	if r, ok := trailer["/Root"].(pdfRef); ok {
		doc.catalogNum = r.Num
	}
	if id, ok := trailer["/ID"].(pdfArray); ok {
		doc.docID = id
	}
	if sz := dictGetInt(trailer, "/Size"); sz > 0 {
		doc.origSize = sz
	}
	if doc.origSize < doc.nextID {
		doc.origSize = doc.nextID
	}
	doc.encryptObjNum = raw.encryptObjNum
	return doc, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}

// Pages returns a live view of all pages in the document.
func (d *Document) Pages() []*Page {
	pages := make([]*Page, len(d.pages))
	for i := range d.pages {
		p, _ := d.Page(i + 1) // uses cache
		pages[i] = p
	}
	return pages
}

// Page returns a live view of the page at the given 1-based number.
func (d *Document) Page(n int) (*Page, error) {
	if n < 1 || n > len(d.pages) {
		return nil, fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
	}
	index := n - 1
	// Lazily allocate and grow the page cache. Pages added after the cache
	// was first populated (via AddBlankPage etc.) extend len(d.pages) past the
	// cache length, so we must resize before indexing.
	if len(d.pageCache) < len(d.pages) {
		grown := make([]*Page, len(d.pages))
		copy(grown, d.pageCache)
		d.pageCache = grown
	}
	if d.pageCache[index] == nil {
		d.pageCache[index] = &Page{doc: d, index: index}
	}
	return d.pageCache[index], nil
}

// Append adds all pages from others to this document, merging their objects.
// Nil arguments are silently skipped.
//
// Example:
//
//	doc1, _ := asposepdf.Open("part1.pdf")
//	doc2, _ := asposepdf.Open("part2.pdf")
//	doc1.Append(doc2)
//	doc1.Save("combined.pdf")
func (d *Document) Append(others ...*Document) {
	for _, other := range others {
		if other == nil {
			continue
		}
		// Build ID mapping: other's object IDs → new IDs in d.
		idMap := make(map[int]int, len(other.objects))
		for oldID := range other.objects {
			idMap[oldID] = d.nextID
			d.nextID++
		}
		// Copy objects with rewritten refs.
		for oldID, obj := range other.objects {
			newID := idMap[oldID]
			d.objects[newID] = &pdfObject{
				Num:   newID,
				Gen:   obj.Gen,
				Value: rewriteRefs(obj.Value, idMap),
			}
		}
		// Add pages (using new IDs).
		for _, page := range other.pages {
			d.pages = append(d.pages, d.objects[idMap[page.Num]])
		}
	}
}

// RemoveUnusedObjects removes objects from the document that are not
// reachable from any page. Returns the number of objects removed.
func (d *Document) RemoveUnusedObjects() int {
	reachable := collectReachableIDs(d.objects, d.pages)

	removed := 0
	for id := range d.objects {
		if !reachable[id] {
			delete(d.objects, id)
			removed++
		}
	}
	return removed
}

// SetPassword configures the document to be encrypted when saved.
// userPassword is required to open; ownerPassword controls permissions.
// If ownerPassword is empty, it defaults to userPassword.
//
// Password encoding: The password bytes are passed unchanged to the PDF
// Standard Security Handler (RC4-128, V=2 R=3). For ASCII passwords this
// matches both the PDF specification and every major PDF viewer. For
// non-ASCII passwords the raw UTF-8 bytes are used, which is compatible
// with pypdf 6.x and Adobe Acrobat DC but may not be accepted by strictly
// legacy PDFDocEncoding-only viewers (e.g. Adobe Reader 9 and older).
// For international passwords with guaranteed interop, AES-256 (R=6) is
// the only complete solution — not yet supported by this library.
//
// Example:
//
//	doc.SetPassword("secret", "")
//	doc.Save("encrypted.pdf")
func (d *Document) SetPassword(userPassword, ownerPassword string) {
	d.preserved = nil // explicit mutation overrides preserved state
	if d.encrypt == nil {
		d.encrypt = &encryptConfig{}
	}
	d.encrypt.userPassword = userPassword
	d.encrypt.ownerPassword = ownerPassword
}

// SetPermissions configures what operations a viewer allows on the
// encrypted document (printing, copying, modifying, etc.). Permissions
// only take effect if the document is also encrypted — call SetPassword
// to set the user and owner passwords. If SetPermissions is never called,
// all operations are allowed by default, matching the historical behavior.
//
// Example:
//
//	doc.SetPassword("secret", "owner-secret")
//	doc.SetPermissions(asposepdf.Permissions{AllowPrint: true, AllowCopy: true})
//	doc.Save("restricted.pdf")
func (d *Document) SetPermissions(p Permissions) {
	d.preserved = nil // explicit mutation overrides preserved state
	if d.encrypt == nil {
		d.encrypt = &encryptConfig{}
	}
	d.encrypt.permissions = p.toPDFBits()
	d.encrypt.hasPermissions = true
}

// Permissions returns the viewer-permission settings currently configured
// on this document, plus a boolean indicating whether the document is
// configured for encryption at all. For a document opened via
// OpenWithPassword, the permissions reflect the /P value read from the
// original file. Returns the zero Permissions and false for unencrypted
// documents.
func (d *Document) Permissions() (Permissions, bool) {
	if d.encrypt == nil {
		return Permissions{}, false
	}
	bits := d.encrypt.effectivePermissions()
	return permissionsFromPDFBits(bits), true
}

// RemoveEncryption clears any previously configured encryption (passwords
// and permissions) so the next Save produces a plaintext PDF. This is the
// way to "decrypt" an encrypted file via the public API: open with a
// password, call RemoveEncryption, save.
//
// Example:
//
//	doc, _ := asposepdf.OpenWithPassword("locked.pdf", "secret")
//	doc.RemoveEncryption()
//	doc.Save("plain.pdf")
func (d *Document) RemoveEncryption() {
	d.preserved = nil // explicit mutation overrides preserved state
	d.encrypt = nil
}

// ChangePassword re-encrypts the document with new passwords on the next Save,
// keeping the current encryption algorithm and permissions. The document must
// already be open and decrypted (via OpenWithPassword / OpenStreamWithPassword)
// — the password that unlocked it is the "old" password. If newOwnerPassword is
// empty it defaults to newUserPassword. Returns an error if the document is not
// encrypted; to encrypt a plaintext document use SetPassword / SetEncryption.
// Mirrors Aspose.PDF for .NET's Document.ChangePasswords.
//
// Example:
//
//	doc, _ := asposepdf.OpenWithPassword("locked.pdf", "old")
//	doc.ChangePassword("new", "")
//	doc.Save("locked.pdf")
func (d *Document) ChangePassword(newUserPassword, newOwnerPassword string) error {
	var (
		alg      EncryptionAlgorithm
		permBits int32
		hasPerms bool
	)
	switch {
	case d.preserved != nil:
		// The original /Encrypt parsed at open time carries the true algorithm
		// and /P bits — preserve both so only the passwords change.
		alg, permBits, hasPerms = d.preserved.algorithm, d.preserved.permissions, true
	case d.encrypt != nil:
		alg, permBits, hasPerms = d.encrypt.algorithm, d.encrypt.permissions, d.encrypt.hasPermissions
	default:
		return fmt.Errorf("ChangePassword: document is not encrypted (use SetPassword or SetEncryption)")
	}
	if newOwnerPassword == "" {
		newOwnerPassword = newUserPassword
	}
	d.preserved = nil // re-encrypt from the new config rather than reuse originals
	d.encrypt = &encryptConfig{
		algorithm:      alg,
		userPassword:   newUserPassword,
		ownerPassword:  newOwnerPassword,
		permissions:    permBits,
		hasPermissions: hasPerms,
	}
	return nil
}

// SetEncryption configures every encryption-related setting at once from
// an EncryptionOptions struct. It replaces any prior configuration set by
// SetPassword or SetPermissions. See EncryptionOptions for field-level
// semantics; nil Permissions means "grant all".
//
// SetPassword and SetPermissions remain convenient for one-liner updates
// and compose cleanly either before or after SetEncryption.
//
// Example:
//
//	doc.SetEncryption(asposepdf.EncryptionOptions{
//	    UserPassword:  "user",
//	    OwnerPassword: "owner",
//	    Permissions:   &asposepdf.Permissions{AllowPrint: true, AllowCopy: true},
//	})
//	doc.Save("restricted.pdf")
func (d *Document) SetEncryption(opts EncryptionOptions) {
	d.preserved = nil // explicit mutation overrides preserved state
	cfg := &encryptConfig{
		algorithm:     opts.Algorithm, // zero value = EncryptionAlgAES128
		userPassword:  opts.UserPassword,
		ownerPassword: opts.OwnerPassword,
	}
	if opts.Permissions != nil {
		cfg.permissions = opts.Permissions.toPDFBits()
		cfg.hasPermissions = true
	}
	d.encrypt = cfg
}

// WriteTo writes the document to w. It implements io.WriterTo.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if len(d.pages) == 0 {
		return 0, fmt.Errorf("document has no pages")
	}
	data, err := buildDocumentPDF(d)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// Save writes the document to outputPath.
func (d *Document) Save(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	if _, err := d.WriteTo(f); err != nil {
		_ = f.Close() // best-effort; the write error takes precedence
		return err
	}
	// The file was written for output: a failed Close can mean lost data,
	// so its error is returned rather than ignored.
	return f.Close()
}

// validateRange validates from/to against [1, total].
func validateRange(from, to, total int) (int, int, error) {
	if from < 1 || from > total {
		return 0, 0, fmt.Errorf("page range from=%d out of bounds (1..%d)", from, total)
	}
	if to < 1 || to > total {
		return 0, 0, fmt.Errorf("page range to=%d out of bounds (1..%d)", to, total)
	}
	if from > to {
		return 0, 0, fmt.Errorf("invalid page range: from=%d > to=%d", from, to)
	}
	return from, to, nil
}

// resolvePageIndices converts 1-based page numbers to 0-based indices.
// If pageNums is empty, returns all indices. Duplicates are silently removed.
func resolvePageIndices(total int, pageNums []int) ([]int, error) {
	if len(pageNums) == 0 {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices, nil
	}
	seen := make(map[int]bool, len(pageNums))
	indices := make([]int, 0, len(pageNums))
	for _, n := range pageNums {
		if n < 1 || n > total {
			return nil, fmt.Errorf("page number %d out of range (1..%d)", n, total)
		}
		if !seen[n] {
			seen[n] = true
			indices = append(indices, n-1)
		}
	}
	return indices, nil
}

// resolveIndirectStreamFilters materialises indirect /Filter and /DecodeParms
// references in stream dictionaries (ISO 32000-1 §7.3.8.2 allows them, e.g.
// /Filter 11 0 R → [/ASCII85Decode /RunLengthDecode]) and decodes streams the
// parse-time pass had to leave raw because the filter chain wasn't visible
// while the referenced object was still unparsed.
func resolveIndirectStreamFilters(objects map[int]*pdfObject) {
	for _, obj := range objects {
		st, ok := obj.Value.(*pdfStream)
		if !ok || st.Decoded {
			continue
		}
		changed := false
		for _, key := range []string{"/Filter", "/DecodeParms", "/DP"} {
			v, ok := st.Dict[key]
			if !ok {
				continue
			}
			if rv, didChange := resolveValueShallow(objects, v); didChange {
				st.Dict[key] = rv
				changed = true
			}
		}
		if changed {
			if decoded, err := decodeStream(st.Dict, st.Data); err == nil {
				st.Data, st.Decoded = decoded, true
			}
		}
	}
}

// resolveValueShallow resolves an indirect reference (and, for arrays, each
// element) one level deep, reporting whether anything changed.
func resolveValueShallow(objects map[int]*pdfObject, v pdfValue) (pdfValue, bool) {
	changed := false
	if ref, ok := v.(pdfRef); ok {
		if obj, exists := objects[ref.Num]; exists {
			v = obj.Value
			changed = true
		} else {
			return v, false
		}
	}
	if arr, ok := v.(pdfArray); ok {
		out := make(pdfArray, len(arr))
		for i, el := range arr {
			out[i] = el
			if ref, ok := el.(pdfRef); ok {
				if obj, exists := objects[ref.Num]; exists {
					out[i] = obj.Value
					changed = true
				}
			}
		}
		return out, changed
	}
	return v, changed
}
