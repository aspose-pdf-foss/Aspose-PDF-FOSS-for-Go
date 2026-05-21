// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
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

	startOff, err := findStartXRef(data)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xref, trailer, err := parseXRef(data, startOff)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

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
			return nil, fmt.Errorf("parse PDF: /Encrypt is not an indirect ref")
		}
		// The /Encrypt object itself is never encrypted, so we can fetch
		// it via getObject before configuring decryption on raw.
		encObj, err := raw.getObject(encRef.Num)
		if err != nil {
			return nil, fmt.Errorf("parse PDF: read /Encrypt: %w", err)
		}
		encDict, ok := encObj.Value.(pdfDict)
		if !ok {
			return nil, fmt.Errorf("parse PDF: /Encrypt is not a dict")
		}
		state, err := buildDecryptState(encDict, trailer, *password)
		if err != nil {
			return nil, fmt.Errorf("parse PDF: %w", err)
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
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	// /Encrypt object — drop it from the working set; the writer rebuilds
	// /Encrypt from d.encrypt on save (or omits it for plain saves).
	if raw.encState != nil {
		delete(objects, raw.encryptObjNum)
	}

	catalog, err := extractCatalog(objects, trailer)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
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

	return &Document{
		objects:   objects,
		pages:     pages,
		catalog:   catalog,
		info:      extractInfo(objects, trailer),
		nextID:    maxObjectID(objects) + 1,
		encrypt:   pendingEncrypt,
		preserved: pendingPreserved,
	}, nil
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
	defer f.Close()
	_, err = d.WriteTo(f)
	return err
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
