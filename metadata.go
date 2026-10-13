// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// DocumentInfo contains document information from the PDF Info dictionary.
// Fields not present in the source PDF are empty strings.
//
// Mirrors Aspose.PDF for .NET's DocumentInfo (Document.Info). Note the
// cross-API naming: in Aspose.PDF for .NET, Document.Info is this Info
// dictionary while Document.Metadata is the XMP store — which here is
// (*Document).XMP, not this type.
type DocumentInfo struct {
	Title        string
	Author       string
	Subject      string
	Keywords     string
	Creator      string
	Producer     string
	CreationDate string
	ModDate      string
	Custom       map[string]string // arbitrary Info dict entries
}

// Info returns the Info-dictionary metadata from this document.
// Mirrors Aspose.PDF for .NET's Document.Info; the XMP store is XMP().
func (d *Document) Info() (DocumentInfo, error) {
	if d.info == nil {
		return DocumentInfo{}, nil
	}
	return readInfoFromDict(d.info), nil
}

// readInfoFromDict extracts a DocumentInfo value from a pdfDict. Text
// fields (everything but the two dates, which are always plain ASCII) are
// decoded with decodeFormString, matching how buildInfoDict now encodes
// them (ISO 32000-1 §7.9.2.2 text strings: PDFDocEncoding or UTF-16BE with
// a byte-order mark) — GitHub issue #13. A string without a BOM is
// returned exactly as stored, so a value an earlier version of this
// library wrote as raw UTF-8 still reads back unchanged.
func readInfoFromDict(infoDict pdfDict) DocumentInfo {
	standardKeys := map[string]bool{
		"/Title": true, "/Author": true, "/Subject": true, "/Keywords": true,
		"/Creator": true, "/Producer": true, "/CreationDate": true, "/ModDate": true,
	}
	var custom map[string]string
	for k, v := range infoDict {
		if standardKeys[k] {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			if custom == nil {
				custom = make(map[string]string)
			}
			custom[strings.TrimPrefix(k, "/")] = decodeFormString(s)
		}
	}
	return DocumentInfo{
		Title:        infoTextString(infoDict, "/Title"),
		Author:       infoTextString(infoDict, "/Author"),
		Subject:      infoTextString(infoDict, "/Subject"),
		Keywords:     infoTextString(infoDict, "/Keywords"),
		Creator:      infoTextString(infoDict, "/Creator"),
		Producer:     infoTextString(infoDict, "/Producer"),
		CreationDate: infoString(infoDict, "/CreationDate"), // always plain ASCII; not a §7.9.2.2 text string
		ModDate:      infoString(infoDict, "/ModDate"),
		Custom:       custom,
	}
}

// SetInfo replaces the document's Info dictionary with the given metadata.
// Empty string fields are omitted. This is a full replacement.
// Mirrors assigning Aspose.PDF for .NET's Document.Info.
func (d *Document) SetInfo(info DocumentInfo) {
	d.info = buildInfoDict(info)
}

// ClearInfo removes the Info dictionary entirely.
func (d *Document) ClearInfo() {
	d.info = nil
}

// buildInfoDict converts a DocumentInfo value into a pdfDict for the Info
// object. Fields with empty string values are omitted. Custom keys are
// prefixed with "/". Custom keys that duplicate standard field names are
// ignored.
//
// Text fields (everything but the two dates) are encoded with
// encodeFormString (ISO 32000-1 §7.9.2.2: PDFDocEncoding for ASCII,
// UTF-16BE with a byte-order mark otherwise) — a plain Go string's UTF-8
// bytes used to be written verbatim, which readers decode as
// PDFDocEncoding/UTF-16BE and so show garbled for any non-ASCII character
// (e.g. "Meldergruppenauslösung" became "MeldergruppenauslÃ¶sung" in
// pdfinfo, though the XMP title was unaffected). The XMP store (xmp.go)
// already used correct ISO 8859-1/UTF-8 handling and needed no change.
// GitHub issue #13.
func buildInfoDict(meta DocumentInfo) pdfDict {
	d := make(pdfDict)
	pairs := [][2]string{
		{"/Title", meta.Title},
		{"/Author", meta.Author},
		{"/Subject", meta.Subject},
		{"/Keywords", meta.Keywords},
		{"/Creator", meta.Creator},
		{"/Producer", meta.Producer},
	}
	for _, kv := range pairs {
		if kv[1] != "" {
			d[kv[0]] = encodeFormString(kv[1])
		}
	}
	// Dates are always plain ASCII (D:YYYYMMDDHHmmSSOHH'mm') -- not a
	// §7.9.2.2 text string, so they're stored as given, unencoded.
	if meta.CreationDate != "" {
		d["/CreationDate"] = meta.CreationDate
	}
	if meta.ModDate != "" {
		d["/ModDate"] = meta.ModDate
	}
	standardNames := map[string]bool{
		"Title": true, "Author": true, "Subject": true, "Keywords": true,
		"Creator": true, "Producer": true, "CreationDate": true, "ModDate": true,
	}
	for k, v := range meta.Custom {
		if v != "" && !standardNames[k] {
			d["/"+k] = encodeFormString(v)
		}
	}
	return d
}

// infoString returns a string field from the Info dictionary as stored
// (no text-string decoding), or "" if absent. Used for the two dates,
// which are never §7.9.2.2 text strings.
func infoString(d pdfDict, key string) string {
	v, ok := d[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// infoTextString returns a §7.9.2.2 text-string field from the Info
// dictionary, decoded with decodeFormString. A value with no byte-order
// mark comes back exactly as stored, so a file an earlier version of this
// library wrote (raw UTF-8 bytes) still reads back unchanged.
func infoTextString(d pdfDict, key string) string {
	return decodeFormString(d[key])
}
