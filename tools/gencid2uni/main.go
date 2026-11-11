//go:build ignore

// SPDX-License-Identifier: MIT

// gencid2uni adds to a bundled CID→Unicode table (cjk_cmaps/<ordering>.cid2uni.gz)
// the CIDs it lacks, taken from the UTF-32 columns of Adobe's cid2code.txt for
// that ordering (github.com/adobe-type-tools/cmap-resources, BSD 3-Clause).
// The UTF-16 column the tables were first built from leaves some CIDs out
// that the UTF-32 columns map — Japan1's alternate glyphs for JIS X 0213 and
// JIS2004, for instance — so text in those glyphs extracted as U+FFFD
// (pdf-go-q2nq). Entries already in the table are kept as they are.
//
//	go run tools/gencid2uni/main.go <ordering> <path-to-cid2code.txt>
//
// Of several values in a cell, the first that is neither a CJK compatibility
// ideograph nor a radical is taken, else the first; a trailing "v" (vertical
// form) is dropped, as the table holds the character a glyph stands for.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gencid2uni <ordering> <cid2code.txt>")
		os.Exit(2)
	}
	ordering, src := os.Args[1], os.Args[2]
	path := filepath.Join("cjk_cmaps", ordering+".cid2uni.gz")
	raw, table, err := readTable(path)
	if err != nil {
		fail(err)
	}
	added, err := supplement(table, src)
	if err != nil {
		fail(err)
	}
	if len(added) == 0 {
		fmt.Printf("%s: nothing to add\n", ordering)
		return
	}
	if err := writeTable(path, raw, added); err != nil {
		fail(err)
	}
	fmt.Printf("%s: %d CIDs added, %d in all\n", ordering, len(added), len(table))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// readTable returns the table's decompressed bytes and its contents.
func readTable(path string) ([]byte, map[uint16]rune, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, nil, err
	}
	if len(data) < 4 {
		return nil, nil, fmt.Errorf("%s: too short", path)
	}
	n := binary.LittleEndian.Uint32(data)
	m := make(map[uint16]rune, n)
	for i, off := uint32(0), 4; i < n && off+6 <= len(data); i, off = i+1, off+6 {
		m[binary.LittleEndian.Uint16(data[off+4:])] = rune(binary.LittleEndian.Uint32(data[off:]))
	}
	return data, m, nil
}

// writeTable writes the table in the loader's format (decodeCID2Uni): a uint32
// count, then (rune uint32, cid uint16) little-endian records. The existing
// records keep their order and the added ones follow, by CID.
func writeTable(path string, raw []byte, added map[uint16]rune) error {
	cids := make([]int, 0, len(added))
	for c := range added {
		cids = append(cids, int(c))
	}
	sort.Ints(cids)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, binary.LittleEndian.Uint32(raw)+uint32(len(cids)))
	buf.Write(raw[4:])
	for _, c := range cids {
		_ = binary.Write(&buf, binary.LittleEndian, uint32(added[uint16(c)]))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(c))
	}
	var out bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if _, err := zw.Write(buf.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// supplement adds to m the CIDs it lacks and returns them.
func supplement(m map[uint16]rune, src string) (map[uint16]rune, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cols []int
	added := map[uint16]rune{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if fields[0] == "CID" {
			for i, h := range fields {
				if strings.HasSuffix(h, "-UTF32") {
					cols = append(cols, i)
				}
			}
			continue
		}
		cid, err := strconv.Atoi(fields[0])
		if err != nil || cid > 0xFFFF {
			continue
		}
		if _, ok := m[uint16(cid)]; ok {
			continue
		}
		for _, c := range cols {
			if c < len(fields) {
				if r := pick(fields[c]); r != 0 {
					m[uint16(cid)] = r
					added[uint16(cid)] = r
					break
				}
			}
		}
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("%s: no UTF-32 columns", src)
	}
	return added, sc.Err()
}

func pick(cell string) rune {
	if cell == "*" {
		return 0
	}
	var first rune
	for _, v := range strings.Split(cell, ",") {
		v = strings.TrimSuffix(v, "v")
		n, err := strconv.ParseUint(v, 16, 32)
		if err != nil || n == 0 {
			continue
		}
		r := rune(n)
		if first == 0 {
			first = r
		}
		if !compatOrRadical(r) {
			return r
		}
	}
	return first
}

func compatOrRadical(r rune) bool {
	return (r >= 0x2E80 && r <= 0x2FDF) || (r >= 0xF900 && r <= 0xFAFF) || (r >= 0x2F800 && r <= 0x2FA1F)
}
