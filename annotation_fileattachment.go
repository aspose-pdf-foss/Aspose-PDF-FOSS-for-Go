// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// FileAttachmentIcon names per ISO 32000-1 §12.5.6.15 Table 178.
type FileAttachmentIcon int

const (
	FileAttachmentIconUnknown FileAttachmentIcon = iota
	FileAttachmentIconGraph
	FileAttachmentIconPaperclip // PDF default
	FileAttachmentIconPushPin
	FileAttachmentIconTag
)

// FileAttachmentAnnotation embeds a file in the document and shows an
// icon at the annotation's /Rect. Per ISO 32000-1 §12.5.6.15. No /AP
// is generated — viewers render the icon themselves based on /Name.
type FileAttachmentAnnotation struct {
	annotationBase
}

func (a *FileAttachmentAnnotation) AnnotationType() AnnotationType {
	return AnnotationTypeFileAttachment
}

// NewFileAttachmentAnnotation builds an unbound file-attachment
// annotation. Page must be non-nil. The /Rect is auto-computed as a
// 24×24 pt square anchored at position (Acrobat icon convention).
//
// Call SetFile or SetFileFromStream to embed file data; without that,
// the annotation has no /FS entry and viewers render an empty icon.
func NewFileAttachmentAnnotation(page *Page, position Point) *FileAttachmentAnnotation {
	if page == nil {
		panic("NewFileAttachmentAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/FileAttachment"),
		"/Rect":    pdfArray{position.X, position.Y, position.X + 24, position.Y + 24},
		"/Name":    pdfName("/Paperclip"),
	}
	return &FileAttachmentAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
}

// Icon returns the /Name entry mapped to a FileAttachmentIcon.
// Returns FileAttachmentIconPaperclip (the spec default) if absent.
func (a *FileAttachmentAnnotation) Icon() FileAttachmentIcon {
	n, ok := a.dict["/Name"].(pdfName)
	if !ok {
		return FileAttachmentIconPaperclip
	}
	switch n {
	case "/Graph":
		return FileAttachmentIconGraph
	case "/Paperclip":
		return FileAttachmentIconPaperclip
	case "/PushPin":
		return FileAttachmentIconPushPin
	case "/Tag":
		return FileAttachmentIconTag
	}
	return FileAttachmentIconUnknown
}

// SetIcon writes the /Name entry. Unknown is encoded as /Paperclip.
func (a *FileAttachmentAnnotation) SetIcon(i FileAttachmentIcon) {
	var name pdfName
	switch i {
	case FileAttachmentIconGraph:
		name = "/Graph"
	case FileAttachmentIconPushPin:
		name = "/PushPin"
	case FileAttachmentIconTag:
		name = "/Tag"
	default: // Paperclip + Unknown
		name = "/Paperclip"
	}
	a.dict["/Name"] = name
}

// HasFile returns true if SetFile or SetFileFromStream has been called
// successfully and not subsequently cleared. Stub for now — full
// implementation in Task 3.
func (a *FileAttachmentAnnotation) HasFile() bool {
	return a.dict["/FS"] != nil
}

// RegenerateAppearance is a no-op for FileAttachmentAnnotation (no /AP
// — viewers render the icon themselves).
func (a *FileAttachmentAnnotation) RegenerateAppearance() {}

// parseFileAttachmentAnnotation builds a FileAttachmentAnnotation from
// a parsed dict.
func parseFileAttachmentAnnotation(base annotationBase) *FileAttachmentAnnotation {
	return &FileAttachmentAnnotation{annotationBase: base}
}

// SetFile embeds the file at path as the annotation's attachment. The
// file's MIME type is auto-detected from the extension via
// mime.TypeByExtension; falls back to "application/octet-stream" for
// unknown extensions. Returns error on file-open or read failures.
func (a *FileAttachmentAnnotation) SetFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("FileAttachmentAnnotation.SetFile: %w", err)
	}
	name := filepath.Base(path)
	mt := detectMIMEType(name)
	return a.embedFileBytes(data, name, mt)
}

// SetFileFromStream is the io.Reader variant of SetFile. The caller
// supplies the displayed filename (used for /F entry and for MIME
// detection from extension).
func (a *FileAttachmentAnnotation) SetFileFromStream(r io.Reader, name string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("FileAttachmentAnnotation.SetFileFromStream: %w", err)
	}
	return a.embedFileBytes(data, name, detectMIMEType(name))
}

// embedFileBytes builds the /EmbeddedFile stream + /Filespec dict (shared with
// the document-level EmbeddedFiles collection) and wires the annotation's /FS.
func (a *FileAttachmentAnnotation) embedFileBytes(data []byte, name, mimeType string) error {
	fsID := a.doc.buildEmbeddedFilespec(data, name, mimeType, "")
	a.dict["/FS"] = pdfRef{Num: fsID}
	return nil
}

// FileName returns the displayed filename from /Filespec/F. Empty if no file.
func (a *FileAttachmentAnnotation) FileName() string {
	fs := a.resolveFilespec()
	if fs == nil {
		return ""
	}
	if name, ok := fs["/UF"].(string); ok && name != "" {
		return name
	}
	if name, ok := fs["/F"].(string); ok {
		return name
	}
	return ""
}

// FileMIMEType returns the /Subtype on /EmbeddedFile (e.g. "application/pdf").
// Empty if no file or /Subtype missing.
func (a *FileAttachmentAnnotation) FileMIMEType() string {
	stream := a.resolveEmbeddedFile()
	if stream == nil {
		return ""
	}
	n, ok := stream.Dict["/Subtype"].(pdfName)
	if !ok {
		return ""
	}
	s := string(n)
	if len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return unescapePDFName(s)
}

// FileSize returns the size of the embedded file in bytes. Zero if no file.
func (a *FileAttachmentAnnotation) FileSize() int {
	stream := a.resolveEmbeddedFile()
	if stream == nil {
		return 0
	}
	return len(stream.Data)
}

// FileBytes returns a defensive copy of the embedded file's raw bytes.
// Nil if no file.
func (a *FileAttachmentAnnotation) FileBytes() []byte {
	stream := a.resolveEmbeddedFile()
	if stream == nil {
		return nil
	}
	out := make([]byte, len(stream.Data))
	copy(out, stream.Data)
	return out
}

// FileDescription returns /Filespec/Desc. Empty if no description set.
func (a *FileAttachmentAnnotation) FileDescription() string {
	fs := a.resolveFilespec()
	if fs == nil {
		return ""
	}
	return decodeFormString(fs["/Desc"])
}

// SetFileDescription writes /Filespec/Desc.
func (a *FileAttachmentAnnotation) SetFileDescription(s string) {
	fsRef, ok := a.dict["/FS"].(pdfRef)
	if !ok {
		return
	}
	obj, ok := a.doc.objects[fsRef.Num]
	if !ok {
		return
	}
	fs, ok := obj.Value.(pdfDict)
	if !ok {
		return
	}
	if s == "" {
		delete(fs, "/Desc")
	} else {
		fs["/Desc"] = encodeFormString(s)
	}
}

// resolveFilespec returns the /Filespec dict referenced by /FS, or nil.
func (a *FileAttachmentAnnotation) resolveFilespec() pdfDict {
	ref, ok := a.dict["/FS"].(pdfRef)
	if !ok {
		return nil
	}
	obj, ok := a.doc.objects[ref.Num]
	if !ok {
		return nil
	}
	fs, ok := obj.Value.(pdfDict)
	if !ok {
		return nil
	}
	return fs
}

// resolveEmbeddedFile follows /FS/EF/F to the /EmbeddedFile stream, or nil.
func (a *FileAttachmentAnnotation) resolveEmbeddedFile() *pdfStream {
	fs := a.resolveFilespec()
	if fs == nil {
		return nil
	}
	ef, ok := fs["/EF"].(pdfDict)
	if !ok {
		return nil
	}
	ref, ok := ef["/F"].(pdfRef)
	if !ok {
		return nil
	}
	obj, ok := a.doc.objects[ref.Num]
	if !ok {
		return nil
	}
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		return nil
	}
	return stream
}

// detectMIMEType looks up the MIME type from the file extension via
// mime.TypeByExtension, with fallbacks for common types.
func detectMIMEType(name string) string {
	ext := filepath.Ext(name)
	if mt := mime.TypeByExtension(ext); mt != "" {
		// Strip charset suffix (e.g. "text/plain; charset=utf-8" → "text/plain").
		if i := strings.Index(mt, ";"); i >= 0 {
			mt = mt[:i]
		}
		return strings.TrimSpace(mt)
	}
	// Fallbacks for common extensions not always in mime registry.
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".zip":
		return "application/zip"
	}
	return "application/octet-stream"
}

// escapePDFName escapes characters not allowed in a PDF name. The "/"
// in MIME types is the most common case (e.g. "application/pdf" →
// "application#2Fpdf").
func escapePDFName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '#' || c < 0x21 || c > 0x7E {
			b.WriteString(fmt.Sprintf("#%02X", c))
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unescapePDFName reverses escapePDFName.
func unescapePDFName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i+2 < len(s) {
			var v int
			_, _ = fmt.Sscanf(s[i+1:i+3], "%X", &v) // bad hex → 0
			b.WriteByte(byte(v))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
