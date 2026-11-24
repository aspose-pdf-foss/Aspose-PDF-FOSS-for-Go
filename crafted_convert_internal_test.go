// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
	"time"
)

// flated returns a stream object body holding data Flate-compressed.
func flated(dict, data string) string {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write([]byte(data))
	zw.Close()
	return fmt.Sprintf("<< %s /Filter /FlateDecode /Length %d >> stream\n%s\nendstream", dict, z.Len(), z.String())
}

// GHSA-rfwg-hxcj-7pp5 point 7: one large form drawn from many resource
// contexts is parsed once per conversion, not once per context.
func TestPDFAFormParsedOncePerConversion(t *testing.T) {
	build := func(callers int) []byte {
		var drawCallers strings.Builder
		var callerRes strings.Builder
		for i := 0; i < callers; i++ {
			fmt.Fprintf(&drawCallers, "/C%d Do ", i)
			fmt.Fprintf(&callerRes, "/C%d %d 0 R ", i, 6+i)
		}
		objs := pageWith("", "", "<< /XObject << "+callerRes.String()+">> >>", drawCallers.String())
		big := strings.Repeat("0 0 m 1 1 l S\n", 20000)
		objs = append(objs, flated("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", big))
		for i := 0; i < callers; i++ {
			objs = append(objs, "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject << /Big 5 0 R >> >> /Length 0 >> stream\n/Big Do\nendstream")
		}
		return hardeningPDF(objs...)
	}
	// The large form is the work; a hundred callers of it must cost about
	// what one does, not a hundred times as much.
	timeOf := func(callers int) time.Duration {
		d, err := OpenStream(bytes.NewReader(build(callers)))
		if err != nil {
			t.Fatal(err)
		}
		best := time.Duration(1 << 62)
		for i := 0; i < 3; i++ {
			start := time.Now()
			d.formsInheritingResources()
			best = min(best, time.Since(start))
		}
		return best
	}
	t1, t100 := timeOf(1), timeOf(100)
	if t100 > 10*t1+20*time.Millisecond {
		t.Errorf("100 callers took %v, one caller %v: the form is parsed per caller", t100, t1)
	}
}

// unbalancedMarked has n unclosed BMCs followed by n show operators.
func unbalancedMarked(n int) []byte {
	// Every glyph at the same place, so a region over it covers them all.
	content := strings.Repeat("/P BMC\n", n) + "BT /F0 12 Tf\n" + strings.Repeat("1 0 0 1 10 10 Tm (a) Tj\n", n) + "ET\n"
	objs := pageWith("", "", "<< /Font << /F0 5 0 R >> >>", "")
	objs[3] = flated("", content)
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	return hardeningPDF(objs...)
}

// GHSA-rfwg-hxcj-7pp5 point 8: unbalanced BMCs do not make the glyph check,
// text extraction or redaction quadratic.
func TestUnbalancedMarkedContentStaysLinear(t *testing.T) {
	const small, large = 10000, 40000
	files := map[int][]byte{small: unbalancedMarked(small), large: unbalancedMarked(large)}
	open := func(n int) *Document {
		d, err := OpenStream(bytes.NewReader(files[n]))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	assertLinearGrowth(t, "glyph check", small, large, func(n int) {
		d := open(n)
		d.glyphFontsInUse()
	})
	assertLinearGrowth(t, "text extraction", small, large, func(n int) {
		if _, err := mustPage(t, open(n)).ExtractText(); err != nil {
			t.Fatal(err)
		}
	})
	// Redaction drops a glyph per show operator and used to walk the stack
	// for each.
	assertLinearGrowth(t, "redaction", small, large, func(n int) {
		d := open(n)
		p := mustPage(t, d)
		if err := p.Annotations().Add(NewRedactAnnotation(p, Rectangle{0, 0, 200, 200})); err != nil {
			t.Fatal(err)
		}
		if err := d.ApplyRedactions(); err != nil {
			t.Fatal(err)
		}
	})
}

// GHSA-rfwg-hxcj-7pp5 point 9: colour bars sharing one large appearance read
// its ink once.
func TestColorBarInkReadOnce(t *testing.T) {
	build := func(bars int) *Document {
		var annots strings.Builder
		for i := 0; i < bars; i++ {
			fmt.Fprintf(&annots, "%d 0 R ", 6+i)
		}
		objs := pageWith("", "/Annots ["+annots.String()+"] /TrimBox [20 20 180 180]", "<< >>", "")
		big := "0 0 0 1 k\n" + strings.Repeat("0 0 1 1 re f\n", 60000)
		objs = append(objs, flated("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", big))
		for i := 0; i < bars; i++ {
			objs = append(objs, "<< /Type /Annot /Subtype /PrinterMark /MN /ColorBar /Rect [0 0 10 10] /AP << /N 5 0 R >> >>")
		}
		d, err := OpenStream(bytes.NewReader(hardeningPDF(objs...)))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	one, many := build(1), build(200)
	timeList := func(d *Document) time.Duration {
		start := time.Now()
		for i := 0; i < 3; i++ {
			mustPage(t, d).annotations = nil
			mustPage(t, d).Annotations().All()
		}
		return time.Since(start)
	}
	timeList(one) // warm
	t1, t200 := timeList(one), timeList(many)
	if t200 > 20*t1+50*time.Millisecond {
		t.Errorf("200 bars took %v, one bar %v: the shared appearance is parsed per bar", t200, t1)
	}
	if a, ok := mustPage(t, many).Annotations().At(0).(*ColorBarAnnotation); !ok || a.ColorOfCMYK() != ColorsOfCMYKBlack {
		t.Errorf("ink read as %+v", mustPage(t, many).Annotations().At(0))
	}
}
