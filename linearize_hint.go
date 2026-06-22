// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
)

// bitWriter writes a big-endian bit stream, MSB first within each byte.
type bitWriter struct {
	buf   []byte
	nbits int
}

func (w *bitWriter) writeBits(v uint64, n int) {
	for k := n - 1; k >= 0; k-- {
		if w.nbits%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if (v>>uint(k))&1 == 1 {
			w.buf[len(w.buf)-1] |= 1 << uint(7-(w.nbits%8))
		}
		w.nbits++
	}
}

// align rounds up to a byte boundary (the partial byte is already in buf).
func (w *bitWriter) align() { w.nbits = (w.nbits + 7) &^ 7 }

func (w *bitWriter) bytes() []byte { return w.buf }

// bitsFor returns the number of bits needed to represent values 0..n.
func bitsFor(n int) int {
	b := 0
	for n > 0 {
		b++
		n >>= 1
	}
	return b
}

func maxInt(xs []int) int {
	m := 0
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

func minInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}

// hintBuilder computes the primary hint stream (page-offset + shared-object
// hint tables) per ISO 32000-1 §F.4, in the field layout used by qpdf.
type hintBuilder struct {
	pageObjCount []int   // objects per page
	pageLen      []int   // byte length per page
	contentLen   []int   // content-stream byte length per page
	sharedLen    []int   // byte length per shared object
	nShFirstPage int     // shared objects used by the first page
	pageShared   [][]int // per page (>=1): shared indices referenced
	firstShObj   int     // lin number of the first shared object
}

// build returns (hintStreamContent, sOffset) where sOffset is the byte offset
// of the shared-object table within the content. firstPageOff and firstShOff
// are file offsets filled into the two header location fields.
func (h *hintBuilder) build(firstPageOff, firstShOff int) ([]byte, int) {
	n := len(h.pageObjCount)

	minObjs := minInt(h.pageObjCount)
	dObjBits := bitsFor(maxInt(h.pageObjCount) - minObjs)
	minPageLen := minInt(h.pageLen)
	dPageLenBits := bitsFor(maxInt(h.pageLen) - minPageLen)
	minContLen := minInt(h.contentLen)
	dContLenBits := bitsFor(maxInt(h.contentLen) - minContLen)

	maxShPerPage := 0
	for _, refs := range h.pageShared {
		if len(refs) > maxShPerPage {
			maxShPerPage = len(refs)
		}
	}
	nSharedBits := bitsFor(maxShPerPage)
	sharedIdBits := bitsFor(maxInt0(len(h.sharedLen) - 1))

	var pw bitWriter
	pw.writeBits(uint64(minObjs), 32)
	pw.writeBits(uint64(firstPageOff), 32)
	pw.writeBits(uint64(dObjBits), 16)
	pw.writeBits(uint64(minPageLen), 32)
	pw.writeBits(uint64(dPageLenBits), 16)
	pw.writeBits(0, 32) // min content-stream offset
	pw.writeBits(0, 16) // delta content-stream offset bits
	pw.writeBits(uint64(minContLen), 32)
	pw.writeBits(uint64(dContLenBits), 16)
	pw.writeBits(uint64(nSharedBits), 16)
	pw.writeBits(uint64(sharedIdBits), 16)
	pw.writeBits(0, 16) // numerator bits
	pw.writeBits(4, 16) // denominator

	for i := 0; i < n; i++ {
		pw.writeBits(uint64(h.pageObjCount[i]-minObjs), dObjBits)
	}
	for i := 0; i < n; i++ {
		pw.writeBits(uint64(h.pageLen[i]-minPageLen), dPageLenBits)
	}
	for i := 0; i < n; i++ {
		pw.writeBits(uint64(len(h.pageShared[i])), nSharedBits)
	}
	for i := 0; i < n; i++ {
		for _, id := range h.pageShared[i] {
			pw.writeBits(uint64(id), sharedIdBits)
		}
	}
	// numerator bits = 0, nothing
	// delta content-stream offset bits = 0, nothing
	for i := 0; i < n; i++ {
		pw.writeBits(uint64(h.contentLen[i]-minContLen), dContLenBits)
	}
	pw.align()
	pageTable := pw.bytes()
	sOffset := len(pageTable)

	// Shared-object hint table.
	minShLen := minInt(h.sharedLen)
	dShLenBits := bitsFor(maxInt(h.sharedLen) - minShLen)
	var sw bitWriter
	sw.writeBits(uint64(h.firstShObj), 32)
	sw.writeBits(uint64(firstShOff), 32)
	sw.writeBits(uint64(h.nShFirstPage), 32)
	sw.writeBits(uint64(len(h.sharedLen)), 32)
	sw.writeBits(0, 16) // bits for greatest object count in a group
	sw.writeBits(uint64(minShLen), 32)
	sw.writeBits(uint64(dShLenBits), 16)
	// Column-major: all delta lengths, then all signature flags (1 bit each;
	// no MD5 signature data is emitted, so each flag is 0).
	for _, ln := range h.sharedLen {
		sw.writeBits(uint64(ln-minShLen), dShLenBits)
	}
	for range h.sharedLen {
		sw.writeBits(0, 1)
	}
	sw.align()

	// qpdf's hint-table reader consumes a few bits past the last encoded entry
	// (final-entry alignment); a small run of trailing zero bytes keeps it from
	// overrunning the stream. Extra bytes in the hint stream are ignored by the
	// reader and harmless.
	out := append(pageTable, sw.bytes()...)
	out = append(out, 0, 0, 0, 0)
	return out, sOffset
}

func maxInt0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// makeHintObject serializes the primary hint stream object (uncompressed).
func makeHintObject(linNum int, content []byte, sOffset int) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%d 0 obj\n<< /Length %d /S %d >>\nstream\n", linNum, len(content), sOffset)
	b.Write(content)
	b.WriteString("\nendstream\nendobj\n")
	return b.Bytes()
}

// pad left-justifies n and pads with trailing spaces to width w, keeping a
// containing structure a constant byte length regardless of the value.
func pad(n, w int) string {
	s := strconv.Itoa(n)
	for len(s) < w {
		s += " "
	}
	return s
}

// classicXref builds a single-subsection classic cross-reference table. A
// negative offset denotes the free object 0 entry.
func classicXref(startObj int, offsets []int) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "xref\n%d %d\n", startObj, len(offsets))
	for _, off := range offsets {
		if off < 0 {
			b.WriteString("0000000000 65535 f \n")
		} else {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		}
	}
	return b.String()
}

// classicXrefLen returns the byte length of classicXref(startObj, count entries).
func classicXrefLen(startObj, count int) int {
	return len("xref\n") + len(fmt.Sprintf("%d %d\n", startObj, count)) + 20*count
}

// linID returns a deterministic 16-byte file identifier (hex) so the layout is
// stable and the file carries a /ID without needing a random source.
func linID(size, pages int) string {
	sum := md5.Sum([]byte(fmt.Sprintf("aspose-linearized-%d-%d", size, pages)))
	return hex.EncodeToString(sum[:])
}

func buildFirstTrailer(size, root, info, prev, pages int) string {
	id := linID(size, pages)
	var b bytes.Buffer
	b.WriteString("trailer\n<< /Size ")
	b.WriteString(pad(size, 8))
	b.WriteString(" /Root ")
	b.WriteString(pad(root, 8))
	b.WriteString(" 0 R")
	if info != 0 {
		b.WriteString(" /Info ")
		b.WriteString(pad(info, 8))
		b.WriteString(" 0 R")
	}
	b.WriteString(" /Prev ")
	b.WriteString(pad(prev, 10))
	fmt.Fprintf(&b, " /ID [<%s><%s>] >>\n", id, id)
	return b.String()
}

func buildMainTrailer(size, root, info, pages int) string {
	id := linID(size, pages)
	var b bytes.Buffer
	b.WriteString("trailer\n<< /Size ")
	b.WriteString(pad(size, 8))
	b.WriteString(" /Root ")
	b.WriteString(pad(root, 8))
	b.WriteString(" 0 R")
	if info != 0 {
		b.WriteString(" /Info ")
		b.WriteString(pad(info, 8))
		b.WriteString(" 0 R")
	}
	fmt.Fprintf(&b, " /ID [<%s><%s>] >>\n", id, id)
	return b.String()
}
