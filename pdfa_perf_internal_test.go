// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// manyRunsDocument draws one embedded composite font on many lines, each
// AddTextAt call selecting it with its own Tf.
func manyRunsDocument(tb testing.TB, pages, lines int) *Document {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "DejaVuSans.ttf"))
	if err != nil {
		tb.Fatal(err)
	}
	d := NewDocument(595, 842)
	font, err := d.LoadFontFromStream(bytes.NewReader(data))
	if err != nil {
		tb.Fatal(err)
	}
	for i := 1; i <= pages; i++ {
		if i > 1 {
			if err := d.AddBlankPage(595, 842); err != nil {
				tb.Fatal(err)
			}
		}
		p, err := d.Page(i)
		if err != nil {
			tb.Fatal(err)
		}
		for j := 0; j < lines; j++ {
			text := fmt.Sprintf("Line %d on page %d: the quick brown fox", j+1, i)
			if err := p.AddTextAt(text, TextStyle{Font: font, Size: 10}, 50, 800-float64(j)*14); err != nil {
				tb.Fatal(err)
			}
		}
	}
	return d
}

// TestPDFAUsedCIDsResolvesEachFontOnce pins GitHub #22: the walk resolves a
// font once, however many Tf operators select it.
func TestPDFAUsedCIDsResolvesEachFontOnce(t *testing.T) {
	d := manyRunsDocument(t, 3, 20)
	calls := 0
	saved := resolveFontHook
	resolveFontHook = func(objects map[int]*pdfObject, fd pdfDict) fontInfo {
		calls++
		return saved(objects, fd)
	}
	t.Cleanup(func() { resolveFontHook = saved })

	used := d.pdfaUsedCIDs()
	if calls != 1 {
		t.Errorf("resolveFont called %d times for one font selected 60 times, want 1", calls)
	}
	if len(used) != 1 {
		t.Fatalf("used fonts = %d, want 1", len(used))
	}
	for _, cids := range used {
		if len(cids) < 20 {
			t.Errorf("only %d CIDs recorded, want the glyphs of the drawn text", len(cids))
		}
	}
}

// TestWriteToRepeatable pins the pooled deflate writers: writing a document
// twice gives the same bytes, so a reused writer starts as a new one would.
func TestWriteToRepeatable(t *testing.T) {
	d := manyRunsDocument(t, 2, 10)
	var a, b bytes.Buffer
	if _, err := d.WriteTo(&a); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("two writes of the same document differ")
	}
	if _, err := OpenStream(bytes.NewReader(a.Bytes())); err != nil {
		t.Fatalf("written file does not reopen: %v", err)
	}
}

func BenchmarkConvertToPDFAManyTextRuns(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		d := manyRunsDocument(b, 10, 50)
		b.StartTimer()
		if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteToManyStreams(b *testing.B) {
	d := manyRunsDocument(b, 10, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.WriteTo(&bytes.Buffer{}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestZlibWriterPoolMatchesFreshWriter checks a reused writer compresses each
// input exactly as a new writer does, after it has compressed something else.
func TestZlibWriterPoolMatchesFreshWriter(t *testing.T) {
	inputs := [][]byte{
		bytes.Repeat([]byte("BT /F1 10 Tf 50 800 Td (abc) Tj ET\n"), 200),
		[]byte("q 1 0 0 1 0 0 cm Q"),
		{},
		bytes.Repeat([]byte{0, 1, 2, 3, 250, 251}, 5000),
	}
	for round := 0; round < 2; round++ {
		for i, in := range inputs {
			var fresh, pooled bytes.Buffer
			fw := zlib.NewWriter(&fresh)
			_, _ = fw.Write(in)
			_ = fw.Close()
			w := zlibWriterPool.Get().(*zlib.Writer)
			w.Reset(&pooled)
			_, _ = w.Write(in)
			_ = w.Close()
			zlibWriterPool.Put(w)
			if !bytes.Equal(fresh.Bytes(), pooled.Bytes()) {
				t.Errorf("round %d input %d: pooled writer output differs from a new writer", round, i)
			}
		}
	}
}
