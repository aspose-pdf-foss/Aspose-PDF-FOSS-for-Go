// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EmbeddedFiles is the document's collection of attached (embedded) files —
// the /Catalog/Names/EmbeddedFiles name tree (ISO 32000-1 §7.11.4). Each entry
// is a /Filespec whose /EF points at an /EmbeddedFile stream, letting a PDF
// carry arbitrary companion files (source code, datasets, the original of a
// scan). Always non-nil; a document with no attachments yields an empty
// collection. Mirrors Aspose.PDF for .NET's Document.EmbeddedFiles.
//
// This is the document-level counterpart to FileAttachmentAnnotation (a file
// pinned to a page); both share the same /EmbeddedFile + /Filespec machinery.
type EmbeddedFiles struct {
	doc *Document
}

// EmbeddedFiles returns the document-level embedded-file collection.
func (d *Document) EmbeddedFiles() *EmbeddedFiles { return &EmbeddedFiles{doc: d} }

// EmbeddedFile is one attachment: a /Filespec dictionary with an embedded
// stream. Read its metadata and content, or set its description.
type EmbeddedFile struct {
	doc      *Document
	name     string
	filespec pdfDict
}

// Names returns the attachment names (name-tree keys), lexicographically sorted.
func (e *EmbeddedFiles) Names() []string {
	raw := e.raw()
	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Count returns the number of embedded files.
func (e *EmbeddedFiles) Count() int { return len(e.raw()) }

// Has reports whether an attachment with the given name exists.
func (e *EmbeddedFiles) Has(name string) bool {
	_, ok := e.raw()[name]
	return ok
}

// Get returns the named attachment, or nil if absent.
func (e *EmbeddedFiles) Get(name string) *EmbeddedFile {
	val, ok := e.raw()[name]
	if !ok {
		return nil
	}
	fs, ok := resolveRefToDict(e.doc.objects, val)
	if !ok {
		return nil
	}
	return &EmbeddedFile{doc: e.doc, name: name, filespec: fs}
}

// All returns every attachment, in sorted-name order.
func (e *EmbeddedFiles) All() []*EmbeddedFile {
	names := e.Names()
	out := make([]*EmbeddedFile, 0, len(names))
	for _, n := range names {
		if f := e.Get(n); f != nil {
			out = append(out, f)
		}
	}
	return out
}

// Add embeds the file at path under its base name, detecting the MIME type
// from its extension, and returns the new attachment. An existing attachment
// with the same name is replaced.
func (e *EmbeddedFiles) Add(path string) (*EmbeddedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("EmbeddedFiles.Add: %w", err)
	}
	return e.addBytes(filepath.Base(path), data)
}

// AddFromStream embeds the bytes read from r under the given name (the MIME
// type is detected from the name's extension), and returns the new attachment.
func (e *EmbeddedFiles) AddFromStream(name string, r io.Reader) (*EmbeddedFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("EmbeddedFiles.AddFromStream: %w", err)
	}
	return e.addBytes(name, data)
}

func (e *EmbeddedFiles) addBytes(name string, data []byte) (*EmbeddedFile, error) {
	if name == "" {
		return nil, fmt.Errorf("EmbeddedFiles: attachment name must not be empty")
	}
	fsID := e.doc.buildEmbeddedFilespec(data, name, detectMIMEType(name), "")
	raw := e.raw()
	raw[name] = pdfRef{Num: fsID}
	e.writeBack(raw)
	return e.Get(name), nil
}

// Remove deletes the named attachment, returning whether it existed. The
// orphaned objects are reclaimed by RemoveUnusedObjects.
func (e *EmbeddedFiles) Remove(name string) bool {
	raw := e.raw()
	if _, ok := raw[name]; !ok {
		return false
	}
	delete(raw, name)
	e.writeBack(raw)
	return true
}

// Clear removes every attachment.
func (e *EmbeddedFiles) Clear() { e.writeBack(map[string]pdfValue{}) }

// raw walks /Catalog/Names/EmbeddedFiles into a name → raw /Filespec value map,
// preserving the existing object references.
func (e *EmbeddedFiles) raw() map[string]pdfValue {
	m := map[string]pdfValue{}
	root := e.root()
	if root == nil {
		return m
	}
	walkNameTree(e.doc, root, func(name string, val pdfValue) {
		if _, dup := m[name]; !dup {
			m[name] = val
		}
	})
	return m
}

// root returns the /Catalog/Names/EmbeddedFiles node, or nil if absent.
func (e *EmbeddedFiles) root() pdfValue {
	nd, ok := resolveRefToDict(e.doc.objects, e.doc.catalog["/Names"])
	if !ok {
		return nil
	}
	return nd["/EmbeddedFiles"]
}

// writeBack rebuilds /Catalog/Names/EmbeddedFiles as one flat name-tree leaf
// (valid for any size per ISO 32000-1 §7.9.6), entries sorted by name. An empty
// collection drops the /EmbeddedFiles subentry. Sibling /Names subentries
// (Dests, JavaScript) are preserved by the writer's name-tree handling.
func (e *EmbeddedFiles) writeBack(raw map[string]pdfValue) {
	nd := e.doc.namesDict()
	if len(raw) == 0 {
		delete(nd, "/EmbeddedFiles")
		return
	}
	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	arr := make(pdfArray, 0, len(names)*2)
	for _, n := range names {
		arr = append(arr, n, raw[n])
	}
	nd["/EmbeddedFiles"] = pdfDict{"/Names": arr}
}

// --- EmbeddedFile accessors ---

// Name returns the attachment's name (its name-tree key).
func (f *EmbeddedFile) Name() string { return f.name }

// Description returns the /Filespec /Desc text, or "" if none.
func (f *EmbeddedFile) Description() string { return decodeFormString(f.filespec["/Desc"]) }

// SetDescription sets (or, for "", removes) the /Filespec /Desc text.
func (f *EmbeddedFile) SetDescription(s string) {
	if s == "" {
		delete(f.filespec, "/Desc")
		return
	}
	f.filespec["/Desc"] = encodeFormString(s)
}

// MIMEType returns the embedded stream's /Subtype as a MIME type (e.g.
// "application/pdf"), or "" if absent.
func (f *EmbeddedFile) MIMEType() string {
	st := f.stream()
	if st == nil {
		return ""
	}
	n, ok := st.Dict["/Subtype"].(pdfName)
	if !ok {
		return ""
	}
	return unescapePDFName(strings.TrimPrefix(string(n), "/"))
}

// Size returns the attachment's size in bytes (0 if no content).
func (f *EmbeddedFile) Size() int {
	st := f.stream()
	if st == nil {
		return 0
	}
	return len(decodedStreamData(st))
}

// Data returns the attachment's decoded content bytes.
func (f *EmbeddedFile) Data() ([]byte, error) {
	st := f.stream()
	if st == nil {
		return nil, fmt.Errorf("EmbeddedFile %q: no content stream", f.name)
	}
	src := decodedStreamData(st)
	out := make([]byte, len(src))
	copy(out, src)
	return out, nil
}

// WriteTo writes the attachment's content to w (implements io.WriterTo).
func (f *EmbeddedFile) WriteTo(w io.Writer) (int64, error) {
	data, err := f.Data()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// Save writes the attachment's content to a file.
func (f *EmbeddedFile) Save(path string) error {
	data, err := f.Data()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// stream follows /EF/F (then /UF) to the /EmbeddedFile stream, or nil.
func (f *EmbeddedFile) stream() *pdfStream {
	ef, ok := resolveRefToDict(f.doc.objects, f.filespec["/EF"])
	if !ok {
		return nil
	}
	for _, k := range []string{"/F", "/UF"} {
		if st, ok := resolveRef(f.doc.objects, ef[k]).(*pdfStream); ok {
			return st
		}
	}
	return nil
}

// buildEmbeddedFilespec creates an /EmbeddedFile stream plus a /Filespec dict
// for data, and returns the /Filespec object ID. Shared by the document-level
// EmbeddedFiles collection and FileAttachmentAnnotation.
func (d *Document) buildEmbeddedFilespec(data []byte, name, mimeType, description string) int {
	embedded := &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/EmbeddedFile"),
			"/Subtype": pdfName("/" + escapePDFName(mimeType)),
			"/Params":  pdfDict{"/Size": len(data)},
			"/Length":  len(data),
		},
		Data:    data,
		Decoded: false, // store raw — the writer leaves it uncompressed
	}
	embedID := d.nextID
	d.nextID++
	d.objects[embedID] = &pdfObject{Num: embedID, Value: embedded}

	filespec := pdfDict{
		"/Type": pdfName("/Filespec"),
		"/F":    name,
		"/UF":   name,
		"/EF": pdfDict{
			"/F":  pdfRef{Num: embedID},
			"/UF": pdfRef{Num: embedID},
		},
	}
	if description != "" {
		filespec["/Desc"] = encodeFormString(description)
	}
	fsID := d.nextID
	d.nextID++
	d.objects[fsID] = &pdfObject{Num: fsID, Value: filespec}
	return fsID
}
