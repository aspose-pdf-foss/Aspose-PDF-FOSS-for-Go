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
// revisions. The trailer is taken from the file's last `trailer`
// dictionary or, failing that, its last /XRef stream; /Root otherwise comes
// from the first object whose /Type is /Catalog.
//
// Limitation: objects stored inside compressed object streams (ObjStm)
// have no top-level header and are not recovered, so a file that both
// uses object streams and has a broken xref may still fail to open.
func reconstructXRef(data []byte, trailerSynthesised *bool) (*xrefTable, pdfDict, error) {
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

	trailer, synthesised := reconstructTrailer(data, table)
	if synthesised {
		*trailerSynthesised = true
	}
	if trailer == nil {
		return nil, nil, fmt.Errorf("reconstruct xref: no /Root catalog found")
	}
	return table, trailer, nil
}

// reconstructTrailer recovers a trailer dictionary. It prefers the file's
// last `trailer` dict (which carries /Root, and possibly /Encrypt, /ID,
// /Info) since that is usually intact even when the xref offsets are not;
// a file written with only a cross-reference stream has no `trailer`
// keyword, so the dictionary of its last /Type /XRef stream stands in (it
// carries the same trailer entries, ISO 32000-1 §7.5.8.2). Failing both, it
// synthesises a trailer pointing /Root at the first /Catalog object found
// via the reconstructed table.
// The second result reports that the trailer had to be synthesised from a
// catalog object because the file carried none usable.
//
// Whatever the source, a trailer without /Encrypt adopts a security-handler
// dictionary found among the scanned objects: dropping it would open an
// encrypted file as if it were plain, without checking a password. Without
// /ID the RC4/AES-128 handlers then fail, which is the right outcome for a
// file this damaged.
func reconstructTrailer(data []byte, table *xrefTable) (pdfDict, bool) {
	raw := newRawDocument(data, table, pdfDict{})
	// Deterministic scan order so the result is stable.
	maxNum := 0
	for num := range table.entries {
		if num > maxNum {
			maxNum = num
		}
	}
	var catalog, encrypt int
	var xrefDict pdfDict
	var xrefOff int64 = -1
	for num := 0; num <= maxNum; num++ {
		entry, ok := table.entries[num]
		if !ok {
			continue
		}
		obj, err := raw.getObject(num)
		if err != nil {
			continue
		}
		var d pdfDict
		switch v := obj.Value.(type) {
		case pdfDict:
			d = v
		case *pdfStream:
			if dictGetName(v.Dict, "/Type") == "/XRef" && entry.Offset > xrefOff {
				xrefDict, xrefOff = v.Dict, entry.Offset
			}
			continue
		default:
			continue
		}
		if catalog == 0 && dictGetName(d, "/Type") == "/Catalog" {
			catalog = num
		}
		if encrypt == 0 && isSecurityHandlerDict(d) {
			encrypt = num
		}
	}

	trailer, synthesised := lastTrailerDict(data), false
	if _, ok := trailer["/Root"]; !ok {
		trailer = nil
		if _, ok := xrefDict["/Root"]; ok {
			trailer = pdfDict{}
			for _, k := range []string{"/Root", "/Encrypt", "/ID", "/Info"} {
				if v, ok := xrefDict[k]; ok {
					trailer[k] = v
				}
			}
		}
	}
	if trailer == nil {
		if catalog == 0 {
			return nil, false
		}
		trailer, synthesised = pdfDict{"/Root": pdfRef{Num: catalog}}, true
	}
	if _, ok := trailer["/Encrypt"]; !ok && encrypt != 0 {
		trailer["/Encrypt"] = pdfRef{Num: encrypt}
	}
	return trailer, synthesised
}

// isSecurityHandlerDict reports whether d looks like an /Encrypt
// dictionary: a Standard handler with /O and /U, or a public-key handler
// with /Recipients (directly or in a crypt filter).
func isSecurityHandlerDict(d pdfDict) bool {
	switch dictGetName(d, "/Filter") {
	case "/Standard":
		_, o := d["/O"]
		_, u := d["/U"]
		return o && u
	case "/Adobe.PubSec":
		_, r := d["/Recipients"]
		_, cf := d["/CF"]
		return r || cf
	}
	return false
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
