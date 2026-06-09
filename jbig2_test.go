// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"testing"
)

func TestJBIG2Log2(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 512: 9, 513: 10, 580: 10, 1024: 10, 1025: 11}
	for in, want := range cases {
		if got := jbig2Log2(in); got != want {
			t.Errorf("jbig2Log2(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestJBIG2Pack checks the foreground→sample inversion: JBIG2 foreground (1=black)
// must pack as sample bit 0 so a 1-bpc DeviceGray image renders it black, and
// background packs as bit 1 (white). Rows are byte-aligned MSB-first.
func TestJBIG2Pack(t *testing.T) {
	// 10px wide, 2 rows; mark a few foreground pixels.
	page := newJBIG2Bitmap(10, 2)
	page[0][0] = 1 // black at (0,0)
	page[0][9] = 1 // black at (9,0)
	packed := jbig2Pack(page, 10, 2)
	rowBytes := (10 + 7) / 8 // 2
	if len(packed) != rowBytes*2 {
		t.Fatalf("packed len = %d, want %d", len(packed), rowBytes*2)
	}
	bit := func(x, y int) int { return int(packed[y*rowBytes+x/8]>>uint(7-x%8)) & 1 }
	if bit(0, 0) != 0 || bit(9, 0) != 0 {
		t.Error("foreground pixels should pack as sample bit 0 (black)")
	}
	if bit(1, 0) != 1 || bit(5, 1) != 1 {
		t.Error("background pixels should pack as sample bit 1 (white)")
	}
}

// TestJBIG2ParseSegments builds a two-segment embedded stream (page info + a
// stub) and verifies the header parser extracts numbers, types, page assoc and
// data slices, including the short referred-to-segment form.
func TestJBIG2ParseSegments(t *testing.T) {
	var b bytes.Buffer
	writeSeg := func(num uint32, typ byte, page byte, data []byte) {
		binary.Write(&b, binary.BigEndian, num)
		b.WriteByte(typ)  // flags: type, 1-byte page assoc
		b.WriteByte(0x00) // ref-count/retention: 0 referred segments
		b.WriteByte(page) // page association (1 byte)
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.Write(data)
	}
	writeSeg(1, 48, 1, []byte("PAGEINFOxxxxxxxxxxx")) // type 48 page info
	writeSeg(2, 38, 1, []byte("REGIONDATA"))          // type 38 generic region

	segs, err := jbig2ParseSegments(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	if segs[0].number != 1 || segs[0].typ != 48 || segs[0].page != 1 {
		t.Errorf("seg0 = %+v", segs[0])
	}
	if segs[1].number != 2 || segs[1].typ != 38 || string(segs[1].data) != "REGIONDATA" {
		t.Errorf("seg1 = num%d type%d data=%q", segs[1].number, segs[1].typ, segs[1].data)
	}
}

// TestJBIG2DecodeFile is an end-to-end pixel check against a real JBIG2 scan that
// uses the symbol-dictionary + refinement-aggregate + text-region path. It is
// skipped when the corpus file is absent (it lives outside the repo). The
// expected black-pixel count was verified byte-identical to jbig2dec / PyMuPDF.
func TestJBIG2DecodeFile(t *testing.T) {
	path := os.Getenv("JBIG2_TESTFILE")
	if path == "" {
		path = `D:/aspose/claude/external_testdata/jbig2-6.pdf`
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("JBIG2 corpus file not present; set JBIG2_TESTFILE to run")
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	all, err := d.ExtractImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 || len(all[0]) == 0 {
		t.Fatal("no images extracted")
	}
	im := all[0][0]
	if im.Width != 1508 || im.Height != 1981 {
		t.Fatalf("image size = %dx%d, want 1508x1981", im.Width, im.Height)
	}
	m, err := png.Decode(bytes.NewReader(im.Data))
	if err != nil {
		t.Fatal("decode extracted PNG:", err)
	}
	bnd := m.Bounds()
	black := 0
	for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
		for x := bnd.Min.X; x < bnd.Max.X; x++ {
			if r, _, _, _ := m.At(x, y).RGBA(); r>>8 < 128 {
				black++
			}
		}
	}
	if black != 327716 {
		t.Errorf("black pixels = %d, want 327716 (byte-exact reference)", black)
	}
}
