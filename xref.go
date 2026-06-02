// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// xrefTable holds all cross-reference entries for a document.
type xrefTable struct {
	entries map[int]xrefEntry
}

func newXrefTable() *xrefTable {
	return &xrefTable{entries: make(map[int]xrefEntry)}
}

// parseXRef reads the cross-reference table starting from startxref offset.
// It handles both traditional xref tables and cross-reference streams (PDF 1.5+).
func parseXRef(data []byte, startOffset int64) (*xrefTable, pdfDict, error) {
	table := newXrefTable()
	var trailer pdfDict

	// May be chained via /Prev; process all.
	offsets := []int64{startOffset}
	visited := map[int64]bool{}

	for len(offsets) > 0 {
		off := offsets[len(offsets)-1]
		offsets = offsets[:len(offsets)-1]

		if visited[off] {
			continue
		}
		visited[off] = true

		var t pdfDict
		var err error

		if isXRefStream(data, off) {
			t, err = parseXRefStream(data, off, table)
		} else {
			t, err = parseXRefTable(data, off, table)
		}
		if err != nil {
			return nil, nil, err
		}

		if trailer == nil {
			trailer = t
		}

		if prev, ok := t["/Prev"]; ok {
			var prevOff int64
			switch v := prev.(type) {
			case int:
				prevOff = int64(v)
			case float64:
				prevOff = int64(v)
			}
			if prevOff > 0 {
				offsets = append(offsets, prevOff)
			}
		}
	}

	return table, trailer, nil
}

// isXRefStream checks if the data at offset starts with an indirect object
// (not an "xref" keyword), meaning it's a cross-reference stream.
func isXRefStream(data []byte, offset int64) bool {
	l := newLexerAt(data, int(offset))
	l.skipWS()
	tok, _ := l.Next()
	if tok.kind == tokKeyword && string(tok.raw) == "xref" {
		return false
	}
	return true
}

// parseXRefTable parses a traditional xref table and returns the trailer dict.
func parseXRefTable(data []byte, offset int64, table *xrefTable) (pdfDict, error) {
	l := newLexerAt(data, int(offset))

	tok, err := l.Next()
	if err != nil || string(tok.raw) != "xref" {
		return nil, fmt.Errorf("expected 'xref' at offset %d", offset)
	}

	for {
		l.skipWS()
		// peek: "trailer" or a subsection start
		kw := l.peekKeyword()
		if kw == "trailer" {
			break
		}

		// subsection: "startObj count"
		tok1, _ := l.Next()
		tok2, _ := l.Next()
		if tok1.kind != tokInt || tok2.kind != tokInt {
			return nil, fmt.Errorf("invalid xref subsection header")
		}
		startObj, _ := strconv.Atoi(string(tok1.raw))
		count, _ := strconv.Atoi(string(tok2.raw))

		// Skip past the newline that ends the subsection header line.
		l.skipLine()

		for i := 0; i < count; i++ {
			// Each entry is exactly 20 bytes per spec.
			// Be lenient: read up to the next end-of-line so 20- or 21-byte
			// variants and any of the three line-ending conventions (LF, CR,
			// or CRLF — classic-Mac CR-only xrefs occur in the wild) work.
			lineStart := l.pos
			for l.pos < len(data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
			entry := string(data[lineStart:l.pos])
			if l.pos < len(data) && l.data[l.pos] == '\r' {
				l.pos++ // skip CR
			}
			if l.pos < len(data) && l.data[l.pos] == '\n' {
				l.pos++ // skip LF (handles both CRLF and lone LF)
			}

			parts := strings.Fields(entry)
			if len(parts) < 3 {
				continue
			}
			offset, _ := strconv.ParseInt(parts[0], 10, 64)
			// gen, _ := strconv.Atoi(parts[1])
			flag := parts[2]
			objNum := startObj + i

			if _, exists := table.entries[objNum]; exists {
				continue // later xref tables take precedence; skip older
			}

			if flag == "f" {
				table.entries[objNum] = xrefEntry{Free: true}
			} else {
				table.entries[objNum] = xrefEntry{Offset: offset}
			}
		}
	}

	// Read trailer dict
	tok, err = l.Next() // consume "trailer"
	if err != nil || string(tok.raw) != "trailer" {
		return nil, fmt.Errorf("expected 'trailer'")
	}
	l.skipWS()
	val, err := parseValue(l)
	if err != nil {
		return nil, fmt.Errorf("trailer dict: %w", err)
	}
	d, ok := val.(pdfDict)
	if !ok {
		return nil, fmt.Errorf("trailer is not a dict")
	}
	return d, nil
}

// parseXRefStream parses a cross-reference stream object (PDF 1.5+).
func parseXRefStream(data []byte, offset int64, table *xrefTable) (pdfDict, error) {
	obj, err := parseIndirectObject(data, offset)
	if err != nil {
		return nil, fmt.Errorf("xref stream object: %w", err)
	}
	s, ok := obj.Value.(*pdfStream)
	if !ok {
		return nil, fmt.Errorf("xref stream is not a stream")
	}

	d := s.Dict

	// Parse /W field: [w1 w2 w3] — widths of each field in bytes
	wVal, ok := d["/W"]
	if !ok {
		return nil, fmt.Errorf("xref stream missing /W")
	}
	wArr, ok := wVal.(pdfArray)
	if !ok || len(wArr) != 3 {
		return nil, fmt.Errorf("xref stream /W must be array of 3")
	}
	w := [3]int{toInt(wArr[0]), toInt(wArr[1]), toInt(wArr[2])}
	entrySize := w[0] + w[1] + w[2]
	if entrySize == 0 {
		return d, nil
	}

	// Parse /Index field: [start count start count ...]; default [0 /Size]
	size := dictGetInt(d, "/Size")
	var indexes []int
	if idxVal, ok := d["/Index"]; ok {
		if arr, ok := idxVal.(pdfArray); ok {
			for _, v := range arr {
				indexes = append(indexes, toInt(v))
			}
		}
	}
	if len(indexes) == 0 {
		indexes = []int{0, size}
	}

	streamData := s.Data
	pos := 0

	for i := 0; i+1 < len(indexes); i += 2 {
		startObj := indexes[i]
		count := indexes[i+1]

		for j := 0; j < count; j++ {
			if pos+entrySize > len(streamData) {
				return nil, fmt.Errorf("xref stream data truncated")
			}
			raw := streamData[pos : pos+entrySize]
			pos += entrySize

			objNum := startObj + j
			if _, exists := table.entries[objNum]; exists {
				continue
			}

			f1 := readField(raw, 0, w[0])
			f2 := readField(raw, w[0], w[1])
			f3 := readField(raw, w[0]+w[1], w[2])

			entryType := 1 // default when w[0]==0
			if w[0] > 0 {
				entryType = f1
			}

			switch entryType {
			case 0: // free
				table.entries[objNum] = xrefEntry{Free: true}
			case 1: // uncompressed
				table.entries[objNum] = xrefEntry{Offset: int64(f2)}
			case 2: // compressed in object stream
				table.entries[objNum] = xrefEntry{
					Compressed:   true,
					StreamObjNum: f2,
					StreamIndex:  f3,
				}
			}
		}
	}

	return d, nil
}

// readField reads an integer of `width` bytes from data starting at offset.
func readField(data []byte, offset, width int) int {
	if width == 0 {
		return 0
	}
	var buf [8]byte
	copy(buf[8-width:], data[offset:offset+width])
	return int(binary.BigEndian.Uint64(buf[:]))
}

func toInt(v pdfValue) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// findStartXRef searches backward from end of file for "startxref" and returns the offset.
func findStartXRef(data []byte) (int64, error) {
	// Search last 1024 bytes (PDF spec requires it to be near the end).
	searchArea := data
	if len(data) > 1024 {
		searchArea = data[len(data)-1024:]
	}

	idx := bytes.LastIndex(searchArea, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("startxref not found")
	}

	// Adjust index to full data
	if len(data) > 1024 {
		idx += len(data) - 1024
	}

	l := newLexerAt(data, idx+len("startxref"))
	tok, err := l.Next()
	if err != nil || tok.kind != tokInt {
		return 0, fmt.Errorf("invalid startxref value")
	}
	off, _ := strconv.ParseInt(string(tok.raw), 10, 64)
	if off < 0 || off >= int64(len(data)) {
		return 0, fmt.Errorf("startxref offset %d out of range", off)
	}
	return off, nil
}
