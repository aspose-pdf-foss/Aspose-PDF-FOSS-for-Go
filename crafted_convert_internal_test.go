// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"sync"
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
// its ink once. Two hundred bars sharing it must cost about what two hundred
// bars with small appearances of their own do, not two hundred reads of it.
func TestColorBarInkReadOnce(t *testing.T) {
	const bars = 200
	build := func(shared bool) *Document {
		var annots strings.Builder
		for i := 0; i < bars; i++ {
			fmt.Fprintf(&annots, "%d 0 R ", 6+i)
		}
		objs := pageWith("", "/Annots ["+annots.String()+"] /TrimBox [20 20 180 180]", "<< >>", "")
		big := "0 0 0 1 k " + strings.Repeat("0 0 1 1 re f ", 60000)
		objs = append(objs, flated("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", big))
		for i := 0; i < bars; i++ {
			ap := 5
			if !shared {
				ap = 6 + bars + i
			}
			objs = append(objs, fmt.Sprintf("<< /Type /Annot /Subtype /PrinterMark /MN /ColorBar /Rect [0 0 10 10] /AP << /N %d 0 R >> >>", ap))
		}
		if !shared {
			for i := 0; i < bars; i++ {
				objs = append(objs, flated("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", "0 0 0 1 k 0 0 1 1 re f"))
			}
		}
		d, err := OpenStream(bytes.NewReader(hardeningPDF(objs...)))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	timeList := func(shared bool) time.Duration {
		best := time.Duration(1 << 62)
		for i := 0; i < 3; i++ {
			d := build(shared)
			start := time.Now()
			mustPage(t, d).Annotations().All()
			best = min(best, time.Since(start))
		}
		return best
	}
	own, shared := timeList(false), timeList(true)
	if shared > 10*own+200*time.Millisecond {
		t.Errorf("%d bars sharing one appearance took %v, with their own %v: the shared appearance is read per bar", bars, shared, own)
	}
	d := build(true)
	if a, ok := mustPage(t, d).Annotations().At(0).(*ColorBarAnnotation); !ok || a.ColorOfCMYK() != ColorsOfCMYKBlack {
		t.Errorf("ink read as %+v", mustPage(t, d).Annotations().At(0))
	}
}

// Listing the annotations of different pages from several goroutines reads
// colour-bar inks through one document-wide cache; run under -race.
func TestColorBarInkConcurrentPages(t *testing.T) {
	const pages = 8
	var kids strings.Builder
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", ""}
	for i := 0; i < pages; i++ {
		page, ap, annot := 3+3*i, 4+3*i, 5+3*i
		fmt.Fprintf(&kids, "%d 0 R ", page)
		objs = append(objs,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /TrimBox [20 20 180 180] /Annots [%d 0 R] >>", annot),
			"<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >> stream\n0 0 0 1 k 0 0 1 1 re f\nendstream",
			fmt.Sprintf("<< /Type /Annot /Subtype /PrinterMark /MN /ColorBar /Rect [0 0 10 10] /AP << /N %d 0 R >> >>", ap))
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pages)
	d, err := OpenStream(bytes.NewReader(hardeningPDF(objs...)))
	if err != nil {
		t.Fatal(err)
	}
	ps := d.Pages()
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func(p *Page) {
			defer wg.Done()
			if a, ok := p.Annotations().At(0).(*ColorBarAnnotation); !ok || a.ColorOfCMYK() != ColorsOfCMYKBlack {
				t.Errorf("page %d: ink read as %+v", p.Number(), p.Annotations().At(0))
			}
		}(p)
	}
	wg.Wait()
}
