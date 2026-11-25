// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// repeatedContents is a page whose /Contents names object 5 n times; stream
// is the body of object 5.
func repeatedContents(n int, stream string) []byte {
	objs := pageWith("", "", "<< >>", "")
	objs[2] = strings.Replace(objs[2], "/Contents 4 0 R", "/Contents ["+refs(5, n)+"]", 1)
	objs = append(objs, stream)
	return hardeningPDF(objs...)
}

// A page's content streams together are held to the limit of one stream: an
// array naming one large stream many times is not concatenated without end.
func TestPageContentsBudget(t *testing.T) {
	saved := maxDecodedStream
	defer func() { maxDecodedStream = saved }()
	maxDecodedStream = 1 << 20

	stream := "<< /Length 0 >> stream\n" + strings.Repeat("0 0 m\n", (64<<10)/6) + "\nendstream"
	d := openHardening(t, repeatedContents(200, stream))
	if _, err := mustPage(t, d).contentStreams(); !errors.Is(err, errDecodedTooLarge) {
		t.Errorf("200 mentions of a 64 KB stream: %v, want errDecodedTooLarge", err)
	}
	d = openHardening(t, repeatedContents(10, stream))
	data, err := mustPage(t, d).contentStreams()
	if err != nil {
		t.Fatalf("10 mentions within the limit: %v", err)
	}
	if want := 10 * (len(stream) - len("<< /Length 0 >> stream\n\nendstream") + 1); len(data) != want {
		t.Errorf("content is %d bytes, want %d", len(data), want)
	}
}

// A stream named many times is decoded once: one that will not decode costs
// its decoding limit at most once, not at every mention.
func TestPageContentsDecodedOnce(t *testing.T) {
	saved := maxDecodedStream
	defer func() { maxDecodedStream = saved }()
	maxDecodedStream = 1 << 20

	// Flate twice over 100 MB of zeros: a few hundred bytes in the file,
	// decoding past the limit.
	var once, twice bytes.Buffer
	zw := zlib.NewWriter(&once)
	zeros := make([]byte, 1<<20)
	for i := 0; i < 100; i++ {
		zw.Write(zeros)
	}
	zw.Close()
	zw = zlib.NewWriter(&twice)
	zw.Write(once.Bytes())
	zw.Close()
	stream := fmt.Sprintf("<< /Filter [/FlateDecode /FlateDecode] /Length %d >> stream\n%s\nendstream", twice.Len(), twice.String())

	d := openHardening(t, repeatedContents(500, stream))
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := mustPage(t, d).contentStreams(); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 100<<20 {
		t.Errorf("500 mentions of a stream that will not decode allocated %d MB: it is decoded per mention", grew>>20)
	}
}
