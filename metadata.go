package asposepdf

import (
	"fmt"
	"strings"
)

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


// Metadata returns the Info metadata from the primary source document.
// For documents assembled from multiple sources, metadata from the first
// source document is returned.
func (d *Document) Metadata() (Metadata, error) {
	if len(d.pages) == 0 {
		return Metadata{}, fmt.Errorf("document has no pages")
	}
	return readMetadata(d.pages[0].src)
}

// readMetadata extracts the Info dictionary from a parsed document.
func readMetadata(doc *rawDocument) (Metadata, error) {
	infoRef, ok := doc.trailer["/Info"]
	if !ok {
		return Metadata{}, nil
	}
	infoDict, err := doc.resolveDict(infoRef)
	if err != nil {
		return Metadata{}, fmt.Errorf("read Info dict: %w", err)
	}

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
	}, nil
}

// metadataConfig holds the metadata override to apply when saving a Document.
type metadataConfig struct {
	meta  Metadata
	clear bool // if true, omit the Info dictionary entirely
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

// SetMetadata returns a new Document configured to write meta as the PDF Info
// dictionary when saved. Empty string fields are omitted. This is a full
// replacement: any metadata from the source document is discarded on save.
//
// To update a single field, read the current metadata, modify it, and call
// SetMetadata with the updated struct:
//
//	meta, _ := doc.Metadata()
//	meta.Title = "New Title"
//	doc = doc.SetMetadata(meta)
func (d *Document) SetMetadata(meta Metadata) *Document {
	result := d.withCopiedPatches()
	result.metadataConfig = &metadataConfig{meta: meta}
	return result
}

// ClearMetadata returns a new Document configured to omit the Info dictionary
// entirely when saved. Use this to strip all metadata before publishing.
func (d *Document) ClearMetadata() *Document {
	result := d.withCopiedPatches()
	result.metadataConfig = &metadataConfig{clear: true}
	return result
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
