// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// objHeaderRE matches an indirect-object header "N G obj".
var objHeaderRE = regexp.MustCompile(`(\d+)[ \t]+(\d+)[ \t]+obj\b`)

// isAlphaNum reports whether b is an ASCII letter or digit.
func isAlphaNum(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// reconstructXRef rebuilds a cross-reference table by scanning the raw
// file for indirect-object headers ("N G obj"), used as a recovery path
// when the file's own xref is missing, corrupt, or inconsistent (e.g. an
// off-by-one subsection start). The latest occurrence of each object
// number wins, matching how incremental updates supersede earlier
// revisions. The trailer's /Root is taken from the file's last `trailer`
// dictionary when present, otherwise from the first object whose /Type is
// /Catalog.
//
// Limitation: objects stored inside compressed object streams (ObjStm)
// have no top-level header and are not recovered, so a file that both
// uses object streams and has a broken xref may still fail to open.
func reconstructXRef(data []byte) (*xrefTable, pdfDict, error) {
	table := &xrefTable{entries: map[int]xrefEntry{}}
	for _, loc := range objHeaderRE.FindAllSubmatchIndex(data, -1) {
		start := loc[0]
		// Skip a digit run that is part of a longer token (e.g. inside a
		// word or a bigger number in binary stream data): a genuine object
		// header is never preceded by an alphanumeric byte. Delimiters
		// (>, ], ), }), whitespace, EOL, or start-of-file are all accepted,
		// which tolerates files that drop the EOL before "N G obj" or wedge
		// a little garbage (e.g. "…endobj\nGS>4 0 obj") ahead of it.
		if start > 0 && isAlphaNum(data[start-1]) {
			continue
		}
		num, err := strconv.Atoi(string(data[loc[2]:loc[3]]))
		if err != nil {
			continue
		}
		table.entries[num] = xrefEntry{Offset: int64(start)} // last wins
	}
	if len(table.entries) == 0 {
		return nil, nil, fmt.Errorf("reconstruct xref: no objects found")
	}

	trailer := reconstructTrailer(data, table)
	if trailer == nil {
		return nil, nil, fmt.Errorf("reconstruct xref: no /Root catalog found")
	}
	return table, trailer, nil
}

// reconstructTrailer recovers a trailer dictionary. It prefers the file's
// last `trailer` dict (which carries /Root, and possibly /Encrypt, /ID,
// /Info) since that is usually intact even when the xref offsets are not;
// failing that, it synthesises a trailer pointing /Root at the first
// /Catalog object found via the reconstructed table.
func reconstructTrailer(data []byte, table *xrefTable) pdfDict {
	if t := lastTrailerDict(data); t != nil {
		if _, ok := t["/Root"]; ok {
			return t
		}
	}
	raw := newRawDocument(data, table, pdfDict{})
	// Deterministic scan order so the result is stable.
	maxNum := 0
	for num := range table.entries {
		if num > maxNum {
			maxNum = num
		}
	}
	for num := 0; num <= maxNum; num++ {
		if _, ok := table.entries[num]; !ok {
			continue
		}
		obj, err := raw.getObject(num)
		if err != nil {
			continue
		}
		if d, ok := obj.Value.(pdfDict); ok && dictGetName(d, "/Type") == "/Catalog" {
			return pdfDict{"/Root": pdfRef{Num: num}}
		}
	}
	return nil
}

// lastTrailerDict parses the dictionary following the file's last
// `trailer` keyword, or returns nil if absent/unparseable.
func lastTrailerDict(data []byte) pdfDict {
	idx := bytes.LastIndex(data, []byte("trailer"))
	if idx < 0 {
		return nil
	}
	l := newLexerAt(data, idx+len("trailer"))
	v, err := parseValue(l)
	if err != nil {
		return nil
	}
	d, _ := v.(pdfDict)
	return d
}
