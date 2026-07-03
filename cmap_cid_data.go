// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/binary"
	"io"
	"sync"
)

// cjkCMapFS holds the predefined Adobe CJK CMaps (code→CID, gzip'd text) and the
// per-ordering CID→Unicode tables (gzip'd binary), generated from Adobe's
// BSD-licensed cmap-resources. They back non-embedded composite (Type0) CJK
// fonts: a PDF naming /Encoding /GBK-EUC-H carries no glyph data, so we decode
// codes to CIDs here and map CIDs to Unicode for extraction and glyph lookup.
//
//go:embed cjk_cmaps
var cjkCMapFS embed.FS

var (
	cjkCMapMu    sync.Mutex
	cjkCMapCache = map[string]*cidCMap{}
	cjkUniMu     sync.Mutex
	cjkUniCache  = map[string]map[uint16]rune{}
)

// predefinedCMap returns the named Adobe predefined CMap (e.g. "GBK-EUC-H"),
// or nil if it is not bundled. Results are cached; usecmap references resolve
// against the same bundle.
func predefinedCMap(name string) *cidCMap {
	cjkCMapMu.Lock()
	defer cjkCMapMu.Unlock()
	return loadPredefinedCMapLocked(name)
}

func loadPredefinedCMapLocked(name string) *cidCMap {
	if c, ok := cjkCMapCache[name]; ok {
		return c
	}
	// Mark in-progress as nil to break usecmap cycles.
	cjkCMapCache[name] = nil
	data, err := readGzipEmbed("cjk_cmaps/" + name + ".cmap.gz")
	if err != nil {
		return nil
	}
	c := parseCIDCMap(data, func(parent string) *cidCMap {
		return loadPredefinedCMapLocked(parent)
	})
	c.name = name
	cjkCMapCache[name] = c
	return c
}

// cidToUnicodeForOrdering returns the CID→Unicode map for an Adobe ordering
// ("GB1", "CNS1", "Japan1", "Korea1", "KR"), or nil if not bundled.
func cidToUnicodeForOrdering(ordering string) map[uint16]rune {
	cjkUniMu.Lock()
	defer cjkUniMu.Unlock()
	if m, ok := cjkUniCache[ordering]; ok {
		return m
	}
	cjkUniCache[ordering] = nil
	data, err := readGzipEmbed("cjk_cmaps/" + ordering + ".cid2uni.gz")
	if err != nil {
		return nil
	}
	m := decodeCID2Uni(data)
	cjkUniCache[ordering] = m
	return m
}

// decodeCID2Uni reads the compact CID→Unicode binary: a uint32 count, then
// that many (rune uint32, cid uint16) little-endian records.
func decodeCID2Uni(data []byte) map[uint16]rune {
	if len(data) < 4 {
		return nil
	}
	count := binary.LittleEndian.Uint32(data)
	m := make(map[uint16]rune, count)
	off := 4
	for i := uint32(0); i < count; i++ {
		if off+6 > len(data) {
			break
		}
		cp := binary.LittleEndian.Uint32(data[off:])
		cid := binary.LittleEndian.Uint16(data[off+4:])
		off += 6
		m[cid] = rune(cp)
	}
	return m
}

func readGzipEmbed(path string) ([]byte, error) {
	f, err := cjkCMapFS.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, gz); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
