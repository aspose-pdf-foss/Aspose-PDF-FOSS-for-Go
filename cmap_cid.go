// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strconv"
	"strings"
)

// cidCMap maps character codes to CIDs for a composite (Type0) font, per the
// CMap text format (ISO 32000-1 §9.7.5 / Adobe TN #5099). It handles a
// variable-width codespace (1–4 bytes), so a single byte string mixes 1-byte
// Latin codes and 2-byte CJK codes (e.g. GBK-EUC-H). The same parser serves the
// predefined Adobe CMaps (embedded, gzip'd) and a PDF's own embedded /Encoding
// CMap stream.
type cidCMap struct {
	name      string
	wmode     int // 0 = horizontal, 1 = vertical
	codespace []cmapCodespace
	singles   map[uint32]uint16 // cidchar: code → CID
	ranges    []cmapCIDRange    // cidrange: sorted by lo for binary search
}

type cmapCodespace struct {
	nbytes    int
	low, high [4]byte
}

type cmapCIDRange struct {
	lo, hi uint32
	cid    uint16
}

// cidForCode returns the CID for a character code, or 0 (.notdef) if unmapped.
func (c *cidCMap) cidForCode(code uint32) uint16 {
	if c.singles != nil {
		if cid, ok := c.singles[code]; ok {
			return cid
		}
	}
	// Binary search the sorted ranges.
	i := sort.Search(len(c.ranges), func(i int) bool { return c.ranges[i].hi >= code })
	if i < len(c.ranges) && code >= c.ranges[i].lo && code <= c.ranges[i].hi {
		r := c.ranges[i]
		return r.cid + uint16(code-r.lo)
	}
	return 0
}

// next consumes one character code from s, returning the code value, the CID,
// and the number of bytes consumed. Byte length is determined by the codespace
// (byte-wise containment per §9.7.6.2). A byte sequence matching no codespace
// consumes a single byte (best-effort resync).
func (c *cidCMap) next(s []byte) (code uint32, cid uint16, n int) {
	for length := 1; length <= 4 && length <= len(s); length++ {
		if !c.hasCodespaceLen(length) {
			continue
		}
		if c.inCodespace(s, length) {
			var v uint32
			for i := 0; i < length; i++ {
				v = v<<8 | uint32(s[i])
			}
			return v, c.cidForCode(v), length
		}
	}
	// No codespace matched: consume the shortest defined codespace length's
	// worth of bytes (or 1) so decoding makes progress.
	n = c.minCodespaceLen()
	if n == 0 || n > len(s) {
		n = 1
	}
	var v uint32
	for i := 0; i < n && i < len(s); i++ {
		v = v<<8 | uint32(s[i])
	}
	return v, c.cidForCode(v), n
}

func (c *cidCMap) hasCodespaceLen(n int) bool {
	for _, cs := range c.codespace {
		if cs.nbytes == n {
			return true
		}
	}
	return false
}

func (c *cidCMap) minCodespaceLen() int {
	m := 0
	for _, cs := range c.codespace {
		if m == 0 || cs.nbytes < m {
			m = cs.nbytes
		}
	}
	return m
}

func (c *cidCMap) inCodespace(s []byte, length int) bool {
	for _, cs := range c.codespace {
		if cs.nbytes != length {
			continue
		}
		ok := true
		for i := 0; i < length; i++ {
			if s[i] < cs.low[i] || s[i] > cs.high[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (c *cidCMap) finalize() {
	sort.Slice(c.ranges, func(i, j int) bool { return c.ranges[i].lo < c.ranges[j].lo })
}

// parseCIDCMap parses CMap text into a cidCMap. usecmapResolver, when non-nil,
// supplies a parent CMap by name for the `usecmap` operator (predefined chaining).
func parseCIDCMap(data []byte, usecmapResolver func(name string) *cidCMap) *cidCMap {
	c := &cidCMap{singles: map[uint32]uint16{}}
	toks := cmapTokenize(data)
	i := 0
	for i < len(toks) {
		t := toks[i]
		switch {
		case t == "usecmap" && usecmapResolver != nil:
			// Previous token is the /Name of the parent CMap.
			if i > 0 && strings.HasPrefix(toks[i-1], "/") {
				if parent := usecmapResolver(toks[i-1][1:]); parent != nil {
					c.inheritFrom(parent)
				}
			}
			i++
		case t == "/WMode":
			if i+1 < len(toks) {
				c.wmode, _ = strconv.Atoi(toks[i+1])
			}
			i += 2
		case t == "begincodespacerange":
			i++
			for i+1 < len(toks) && toks[i] != "endcodespacerange" {
				lo, ln, ok1 := cmapHex(toks[i])
				hi, _, ok2 := cmapHex(toks[i+1])
				if ok1 && ok2 {
					c.codespace = append(c.codespace, makeCodespace(lo, hi, ln))
				}
				i += 2
			}
			i++ // skip endcodespacerange
		case strings.HasSuffix(t, "begincidrange") || strings.HasSuffix(t, "beginbfrange"):
			i++
			for i+2 < len(toks) && !strings.HasSuffix(toks[i], "endcidrange") && !strings.HasSuffix(toks[i], "endbfrange") {
				lo, _, ok1 := cmapHex(toks[i])
				hi, _, ok2 := cmapHex(toks[i+1])
				cid, ok3 := cmapCID(toks[i+2])
				if ok1 && ok2 && ok3 {
					c.ranges = append(c.ranges, cmapCIDRange{lo: lo, hi: hi, cid: cid})
				}
				i += 3
			}
			i++ // skip end*range
		case strings.HasSuffix(t, "begincidchar") || strings.HasSuffix(t, "beginbfchar"):
			i++
			for i+1 < len(toks) && !strings.HasSuffix(toks[i], "endcidchar") && !strings.HasSuffix(toks[i], "endbfchar") {
				code, _, ok1 := cmapHex(toks[i])
				cid, ok2 := cmapCID(toks[i+1])
				if ok1 && ok2 {
					c.singles[code] = cid
				}
				i += 2
			}
			i++ // skip end*char
		default:
			i++
		}
	}
	c.finalize()
	return c
}

func (c *cidCMap) inheritFrom(parent *cidCMap) {
	if len(c.codespace) == 0 {
		c.codespace = append(c.codespace, parent.codespace...)
	}
	for k, v := range parent.singles {
		if _, ok := c.singles[k]; !ok {
			c.singles[k] = v
		}
	}
	c.ranges = append(c.ranges, parent.ranges...)
	if c.wmode == 0 {
		c.wmode = parent.wmode
	}
}

func makeCodespace(lo, hi uint32, nbytes int) cmapCodespace {
	cs := cmapCodespace{nbytes: nbytes}
	for i := 0; i < nbytes; i++ {
		shift := uint(8 * (nbytes - 1 - i))
		cs.low[i] = byte(lo >> shift)
		cs.high[i] = byte(hi >> shift)
	}
	return cs
}

// cmapTokenize splits CMap text into tokens: hex strings keep their angle
// brackets, names keep their slash, and operators/numbers are bare words.
// Comment lines (starting with %) are skipped.
func cmapTokenize(data []byte) []string {
	var toks []string
	i, n := 0, len(data)
	for i < n {
		ch := data[i]
		switch {
		case ch == '%':
			for i < n && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case ch <= ' ':
			i++
		case ch == '<':
			j := i + 1
			for j < n && data[j] != '>' {
				j++
			}
			toks = append(toks, string(data[i:min(j+1, n)]))
			i = j + 1
		case ch == '[' || ch == ']' || ch == '{' || ch == '}':
			i++ // bfrange array brackets — ignored (we only read range form)
		default:
			j := i
			for j < n && data[j] > ' ' && data[j] != '<' && data[j] != '%' && data[j] != '[' && data[j] != ']' {
				j++
			}
			toks = append(toks, string(data[i:j]))
			i = j
		}
	}
	return toks
}

// cmapHex parses a "<hhhh>" token, returning the value and byte count.
func cmapHex(tok string) (uint32, int, bool) {
	if len(tok) < 2 || tok[0] != '<' || tok[len(tok)-1] != '>' {
		return 0, 0, false
	}
	body := tok[1 : len(tok)-1]
	if len(body) == 0 || len(body)%2 != 0 || len(body) > 8 {
		return 0, 0, false
	}
	v, err := strconv.ParseUint(body, 16, 64)
	if err != nil {
		return 0, 0, false
	}
	return uint32(v), len(body) / 2, true
}

// cmapCID parses the destination of a cidrange/cidchar entry, which is either a
// decimal CID ("814") or, for bf-style maps reused here, a "<hhhh>" hex value.
func cmapCID(tok string) (uint16, bool) {
	if len(tok) >= 2 && tok[0] == '<' && tok[len(tok)-1] == '>' {
		v, _, ok := cmapHex(tok)
		return uint16(v), ok
	}
	v, err := strconv.Atoi(tok)
	if err != nil || v < 0 {
		return 0, false
	}
	return uint16(v), true
}
