// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// Metadata contains document information from the PDF Info dictionary.
// Fields not present in the source PDF are empty strings.
type Metadata struct {
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

// Metadata returns the Info metadata from this document.
func (d *Document) Metadata() (Metadata, error) {
	if d.info == nil {
		return Metadata{}, nil
	}
	return readMetadataFromDict(d.info), nil
}

// readMetadataFromDict extracts a Metadata value from a pdfDict.
func readMetadataFromDict(infoDict pdfDict) Metadata {
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
			custom[strings.TrimPrefix(k, "/")] = s
		}
	}
	return Metadata{
		Title:        infoString(infoDict, "/Title"),
		Author:       infoString(infoDict, "/Author"),
		Subject:      infoString(infoDict, "/Subject"),
		Keywords:     infoString(infoDict, "/Keywords"),
		Creator:      infoString(infoDict, "/Creator"),
		Producer:     infoString(infoDict, "/Producer"),
		CreationDate: infoString(infoDict, "/CreationDate"),
		ModDate:      infoString(infoDict, "/ModDate"),
		Custom:       custom,
	}
}

// SetMetadata replaces the document's Info dictionary with the given metadata.
// Empty string fields are omitted. This is a full replacement.
func (d *Document) SetMetadata(meta Metadata) {
	d.info = buildInfoDict(meta)
}

// ClearMetadata removes the Info dictionary entirely.
func (d *Document) ClearMetadata() {
	d.info = nil
}

// buildInfoDict converts a Metadata value into a pdfDict for the Info object.
// Fields with empty string values are omitted. Custom keys are prefixed with "/".
// Custom keys that duplicate standard field names are ignored.
func buildInfoDict(meta Metadata) pdfDict {
	d := make(pdfDict)
	pairs := [][2]string{
		{"/Title", meta.Title},
		{"/Author", meta.Author},
		{"/Subject", meta.Subject},
		{"/Keywords", meta.Keywords},
		{"/Creator", meta.Creator},
		{"/Producer", meta.Producer},
		{"/CreationDate", meta.CreationDate},
		{"/ModDate", meta.ModDate},
	}
	for _, kv := range pairs {
		if kv[1] != "" {
			d[kv[0]] = kv[1]
		}
	}
	standardNames := map[string]bool{
		"Title": true, "Author": true, "Subject": true, "Keywords": true,
		"Creator": true, "Producer": true, "CreationDate": true, "ModDate": true,
	}
	for k, v := range meta.Custom {
		if v != "" && !standardNames[k] {
			d["/"+k] = v
		}
	}
	return d
}

// infoString returns a string field from the Info dictionary, or "" if absent.
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
