// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// The tests below pin GHSA-rfwg-hxcj-7pp5: small crafted files that stopped
// OpenStream (stack overflow, endless loops, gigabytes allocated, panics).
// Each must open (or fail) quickly and without a panic.

// openCrafted opens data, failing the test on a panic or when it does not
// return in time; an error from OpenStream is fine.
func openCrafted(t *testing.T, data []byte) (*Document, error) {
	t.Helper()
	var d *Document
	var err error
	withinDeadline(t, hardeningDeadline, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic: %v", r)
			}
		}()
		d, err = OpenStream(bytes.NewReader(data))
	})
	return d, err
}

// xrefStreamPDF writes a file whose objects are found through a
// cross-reference stream. plain are ordinary objects by number; packed are
// put into object stream stm (an ordinary object) in the order given;
// where overrides the cross-reference entry of a number as "stream, index"
// (to list an object as packed somewhere it is not). The catalog is 1.
func xrefStreamPDF(plain map[int]string, stm int, packed []int, packedBody map[int]string, where map[int][2]int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	var hdr, body strings.Builder
	for _, n := range packed {
		fmt.Fprintf(&hdr, "%d %d ", n, body.Len())
		body.WriteString(packedBody[n])
		body.WriteString("\n")
	}
	if stm > 0 {
		first := hdr.Len()
		data := hdr.String() + body.String()
		plain[stm] = fmt.Sprintf("<< /Type /ObjStm /N %d /First %d /Length %d >> stream\n%s\nendstream", len(packed), first, len(data), data)
	}
	maxNum := 0
	for n := range plain {
		maxNum = max(maxNum, n)
	}
	for _, n := range packed {
		maxNum = max(maxNum, n)
	}
	for n := 1; n <= maxNum; n++ {
		if body, ok := plain[n]; ok {
			offsets[n] = buf.Len()
			fmt.Fprintf(&buf, "%d 0 obj %s endobj\n", n, body)
		}
	}
	xrefNum := maxNum + 1
	var rows bytes.Buffer
	row := func(typ byte, f2 uint32, f3 uint16) {
		rows.WriteByte(typ)
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], f2)
		rows.Write(b[:])
		binary.BigEndian.PutUint16(b[:2], f3)
		rows.Write(b[:2])
	}
	index := map[int]int{}
	for i, n := range packed {
		index[n] = i
	}
	xrefOff := buf.Len()
	for n := 0; n <= xrefNum; n++ {
		switch {
		case n == 0:
			row(0, 0, 65535)
		case where[n] != [2]int{}:
			row(2, uint32(where[n][0]), uint16(where[n][1]))
		case n == xrefNum:
			row(1, uint32(xrefOff), 0)
		default:
			if off, ok := offsets[n]; ok {
				row(1, uint32(off), 0)
			} else if i, ok := index[n]; ok {
				row(2, uint32(stm), uint16(i))
			} else {
				row(0, 0, 0)
			}
		}
	}
	fmt.Fprintf(&buf, "%d 0 obj << /Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Length %d >> stream\n", xrefNum, xrefNum+1, rows.Len())
	buf.Write(rows.Bytes())
	fmt.Fprintf(&buf, "\nendstream endobj\nstartxref\n%d\n%%%%EOF", xrefOff)
	return buf.Bytes()
}

// Point 1: arrays and dictionaries nested far beyond anything real are
// refused, not followed until the stack runs out; nesting within the bound
// parses.
func TestDeepNestingRefused(t *testing.T) {
	deep := strings.Repeat("[", 200000) + strings.Repeat("]", 200000)
	objs := pageWith("", "", "<< /X "+deep+" >>", "BT ET")
	if _, err := openCrafted(t, hardeningPDF(objs...)); err != nil {
		t.Logf("refused: %v", err)
	}
	dict := strings.Repeat("<< /A ", 200000) + strings.Repeat(">> ", 200000)
	objs = pageWith("", "", "<< /X "+dict+" >>", "BT ET")
	openCrafted(t, hardeningPDF(objs...))

	ok := strings.Repeat("[", 300) + "1" + strings.Repeat("]", 300)
	d, err := openCrafted(t, hardeningPDF(pageWith("", "", "<< /X "+ok+" >>", "BT ET")...))
	if err != nil {
		t.Fatalf("300 levels refused: %v", err)
	}
	v := resolveRef(d.objects, mustPage(t, d).pageResources()["/X"])
	for i := 0; i < 300; i++ {
		arr, isArr := v.(pdfArray)
		if !isArr || len(arr) != 1 {
			t.Fatalf("level %d: %#v", i, v)
		}
		v = arr[0]
	}
}

// Point 2: an object stream listed as stored in itself does not recurse.
func TestObjectStreamInsideItself(t *testing.T) {
	plain := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
	}
	data := xrefStreamPDF(plain, 5, []int{1}, map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"},
		map[int][2]int{5: {5, 1}})
	openCrafted(t, data)
	_, err := (&rawDocument{data: data, xref: &xrefTable{entries: map[int]xrefEntry{
		5: {Compressed: true, StreamObjNum: 5},
	}}, cache: map[int]*pdfObject{}, objStreams: map[int]map[int]*pdfObject{}}).getObject(5)
	if err == nil {
		t.Error("an object stream stored in itself resolved")
	}
}

// Point 10: object stream members are found by number, so a stream of many
// members opens in linear time.
func TestManyObjectStreamMembersStayLinear(t *testing.T) {
	build := func(n int) []byte {
		plain := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /Extra 4 0 R >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		}
		packed := make([]int, n)
		bodies := map[int]string{}
		var refs strings.Builder
		for i := range packed {
			num := 10 + i
			packed[i] = num
			bodies[num] = fmt.Sprintf("<< /I %d >>", i)
			fmt.Fprintf(&refs, "%d 0 R ", num)
		}
		plain[4] = "[" + refs.String() + "]"
		return xrefStreamPDF(plain, 5, packed, bodies, nil)
	}
	small, large := build(5000), build(20000)
	d, err := openCrafted(t, small)
	if err != nil || d.PageCount() != 1 {
		t.Fatalf("open: %v", err)
	}
	if o := d.objects[10+4999]; o == nil {
		t.Fatal("last member not loaded")
	}
	assertLinearGrowth(t, "GHSA-rfwg point 10", 5000, 20000, func(n int) {
		data := small
		if n == 20000 {
			data = large
		}
		if _, err := OpenStream(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	})
}

// Point 3: a damaged cross-reference table with an object numbered near
// MaxInt64 is rebuilt without counting up to that number.
func TestReconstructHugeObjectNumber(t *testing.T) {
	for _, num := range []string{"2000000000", "9223372036854775807"} {
		data := []byte("%PDF-1.4\n" +
			"1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
			"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n" +
			"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >> endobj\n" +
			num + " 0 obj << /X 1 >> endobj\n" +
			"xref\nbroken\ntrailer << /Root 1 0 R >>\nstartxref\n9\n%%EOF")
		if d, err := openCrafted(t, data); err == nil && d.PageCount() != 1 {
			t.Errorf("%s: %d pages", num, d.PageCount())
		}
	}
}

// Point 4: a /W or /W2 range is clamped to the CIDs there are.
func TestCIDWidthRangeClamped(t *testing.T) {
	for _, w := range []string{
		"/W [0 9223372036854775807 500]",
		"/W [-9223372036854775807 10 500]",
		"/W2 [-9223372036854775807 10 -1000 500 880]",
	} {
		objs := pageWith("", "", "<< /Font << /F1 5 0 R >> >>", "BT /F1 12 Tf 10 10 Td <0001> Tj ET")
		objs = append(objs,
			"<< /Type /Font /Subtype /Type0 /BaseFont /X /Encoding /Identity-H /DescendantFonts [6 0 R] >>",
			"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /X /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+w+" >>")
		d, err := openCrafted(t, hardeningPDF(objs...))
		if err != nil {
			t.Fatal(err)
		}
		withinDeadline(t, hardeningDeadline, func() {
			if _, err := mustPage(t, d).ExtractText(); err != nil {
				t.Error(err)
			}
		})
	}
	if lo, hi, ok := cidRange(-5, 3); !ok || lo != 0 || hi != 3 {
		t.Errorf("cidRange(-5, 3) = %d %d %v", lo, hi, ok)
	}
	if _, _, ok := cidRange(70000, 80000); ok {
		t.Error("a range above 0xFFFF is kept")
	}
}

// Found alongside point 4: CMap ranges are expanded within a bound — a
// vertical CMap's range ending at 0xFFFFFFFF looped forever, and repeated
// full bfrange lines cost billions of steps.
func TestCMapRangeExpansionBounded(t *testing.T) {
	cm := parseCIDCMap([]byte("1 begincodespacerange <00000000> <FFFFFFFF> endcodespacerange\n"+
		"1 begincidrange <00000000> <FFFFFFFF> 1 endcidrange\n"),
		nil)
	cm.parent = predefinedCMap("UniJIS-UCS2-H")
	if cm.parent == nil {
		t.Fatal("no predefined parent CMap")
	}
	withinDeadline(t, hardeningDeadline, func() { cm.verticalBases() })

	var b strings.Builder
	b.WriteString("begincmap\n20000 beginbfrange\n")
	for i := 0; i < 20000; i++ {
		b.WriteString("<0000> <FFFF> <0041>\n")
	}
	b.WriteString("endbfrange\nendcmap\n")
	withinDeadline(t, hardeningDeadline, func() {
		if u := parseUnicodeMap([]byte(b.String())); u.runes[0x10] != 'A'+0x10 {
			t.Errorf("mapping lost: %q", u.runes[0x10])
		}
	})
}

// Point 5: a stream that inflates past maxEagerDecode is not decoded while
// the file is opened, and decoding stops at maxDecodedStream.
func TestDecompressionBounded(t *testing.T) {
	var once, twice bytes.Buffer
	zw := zlib.NewWriter(&once)
	zeros := make([]byte, 1<<20)
	for i := 0; i < 160; i++ { // 160 MB of zeros
		zw.Write(zeros)
	}
	zw.Close()
	zw = zlib.NewWriter(&twice)
	zw.Write(once.Bytes())
	zw.Close()
	objs := pageWith("", "", "<< >>", "BT ET")
	objs = append(objs, fmt.Sprintf("<< /Filter [/FlateDecode /FlateDecode] /Length %d >> stream\n%s\nendstream", twice.Len(), twice.String()))
	data := hardeningPDF(objs...)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	d, err := openCrafted(t, data)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 400<<20 {
		t.Errorf("open allocated %d MB for an unreferenced stream", grew>>20)
	}
	if st, ok := d.objects[5].Value.(*pdfStream); !ok || st.Decoded {
		t.Error("a stream over the eager limit was decoded at open")
	}
	if _, err := flateDecodeLimit(once.Bytes(), 1<<20); !errors.Is(err, errDecodedTooLarge) {
		t.Errorf("limit not applied: %v", err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
}

// Point 6: numbers a file can make overflow or out of range no longer panic.
func TestUncheckedNumbersRefused(t *testing.T) {
	t.Run("stream length", func(t *testing.T) {
		objs := pageWith("", "", "<< >>", "BT ET")
		objs = append(objs, "<< /Length 9223372036854775800 >> stream\nabc\nendstream")
		openCrafted(t, hardeningPDF(objs...))
	})
	t.Run("predictor columns", func(t *testing.T) {
		for _, cols := range []string{"-2", "-1", "0", "9223372036854775807"} {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			zw.Write([]byte{0, 1, 2, 3, 4, 5})
			zw.Close()
			objs := pageWith("", "", "<< >>", "BT ET")
			objs = append(objs, fmt.Sprintf("<< /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns %s >> /Length %d >> stream\n%s\nendstream", cols, z.Len(), z.String()))
			openCrafted(t, hardeningPDF(objs...))
		}
	})
	t.Run("xref stream /W", func(t *testing.T) {
		data := xrefStreamPDF(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		}, 0, nil, nil, nil)
		data = bytes.Replace(data, []byte("/W [1 4 2]"), []byte("/W [1 9 1]"), 1)
		openCrafted(t, data)
	})
	t.Run("type1 lengths", func(t *testing.T) {
		objs := pageWith("", "", "<< /Font << /F1 5 0 R >> >>", "BT /F1 12 Tf 10 10 Td (a) Tj ET")
		objs = append(objs,
			"<< /Type /Font /Subtype /Type1 /BaseFont /X /FontDescriptor 6 0 R >>",
			"<< /Type /FontDescriptor /FontName /X /Flags 4 /FontFile 7 0 R >>",
			"<< /Length1 4611686018427387904 /Length2 4611686018427387904 /Length 0 >> stream\n%!PS-AdobeFont-1.0: X\n/Encoding StandardEncoding def\neexec\nendstream")
		d, err := openCrafted(t, hardeningPDF(objs...))
		if err != nil {
			t.Fatal(err)
		}
		withinDeadline(t, hardeningDeadline, func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic: %v", r)
				}
			}()
			if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
				t.Log(err)
			}
		})
	})
}
