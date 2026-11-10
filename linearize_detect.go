// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"regexp"
)

var reFirstObject = regexp.MustCompile(`(?:^|[\r\n\s])(\d+)[ \t\r\n]+(\d+)[ \t\r\n]+obj\b`)

// fileIsLinearized reports whether data is a linearized file (ISO 32000-1
// Annex F): its first indirect object, within the first 1024 bytes, is a
// linearization parameter dictionary whose /L equals the file's length. A
// file that has been appended to since (an incremental update) no longer
// matches /L and, as Annex F says, is no longer to be treated as linearized.
func fileIsLinearized(data []byte) bool {
	head := data[:min(len(data), 1024)]
	h := bytes.Index(head, []byte("%PDF-"))
	if h < 0 {
		return false
	}
	m := reFirstObject.FindSubmatchIndex(head[h:])
	if m == nil {
		return false
	}
	obj, err := parseIndirectObject(data, int64(h+m[2]))
	if err != nil {
		return false
	}
	dict, _ := obj.Value.(pdfDict) // anything else has no /Linearized
	if _, ok := dict["/Linearized"]; !ok {
		return false
	}
	l, ok := dict["/L"].(int)
	return ok && l == len(data)
}

// IsLinearized reports whether the file the document was opened from is
// linearized ("fast web view", ISO 32000-1 Annex F); false for a document built
// in memory. Write a linearized file with SaveLinearized or WriteToLinearized.
// Mirrors reading Aspose.PDF for .NET's Document.IsLinearized.
func (d *Document) IsLinearized() bool { return d.linearized }
