// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// rawStream adds an uncompressed (no-filter, Decoded=false) stream object and
// returns its object number.
func rawStream(d *Document, dict pdfDict, data []byte) int {
	if dict == nil {
		dict = pdfDict{}
	}
	return d.addObject(&pdfStream{Dict: dict, Data: data, Decoded: false})
}

func TestCompressUncompressedStreams(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)

	payload := bytes.Repeat([]byte("compress me please "), 20) // > compressMinBytes
	plain := rawStream(d, pdfDict{}, payload)
	tiny := rawStream(d, pdfDict{}, []byte("x"))                                      // below threshold
	meta := rawStream(d, pdfDict{"/Type": pdfName("/Metadata")}, payload)             // must stay raw
	img := rawStream(d, pdfDict{"/Subtype": pdfName("/Image")}, payload)              // images excluded
	already := d.addObject(&pdfStream{Dict: pdfDict{}, Data: payload, Decoded: true}) // already decoded

	n := d.compressUncompressedStreams()
	if n != 1 {
		t.Fatalf("compressed %d streams; want 1", n)
	}
	if !d.objects[plain].Value.(*pdfStream).Decoded {
		t.Error("plain stream not marked decoded")
	}
	if d.objects[tiny].Value.(*pdfStream).Decoded {
		t.Error("tiny stream should be left uncompressed")
	}
	if d.objects[meta].Value.(*pdfStream).Decoded {
		t.Error("metadata stream must stay raw")
	}
	if d.objects[img].Value.(*pdfStream).Decoded {
		t.Error("image stream must be left to OptimizeImages")
	}
	_ = already
}

func TestRemoveDuplicateStreams(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)

	content := bytes.Repeat([]byte("shared resource data "), 10)
	a := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject")}, Data: content, Decoded: true})
	b := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject")}, Data: append([]byte(nil), content...), Decoded: true})
	c := d.addObject(&pdfStream{Dict: pdfDict{"/Type": pdfName("/XObject")}, Data: []byte("different"), Decoded: true})

	// A holder dict referencing all three (so refs must be repointed).
	holder := d.addObject(pdfDict{
		"/A": pdfRef{Num: a},
		"/B": pdfRef{Num: b},
		"/C": pdfRef{Num: c},
	})

	removed := d.removeDuplicateStreams()
	if removed != 1 {
		t.Fatalf("removed %d streams; want 1 (b duplicates a)", removed)
	}
	// The lower-numbered a is the keeper; b is gone.
	if _, ok := d.objects[b]; ok {
		t.Error("duplicate b was not deleted")
	}
	if _, ok := d.objects[a]; !ok {
		t.Error("keeper a was deleted")
	}
	if _, ok := d.objects[c]; !ok {
		t.Error("distinct c was deleted")
	}
	// The holder's /B ref now points at the keeper a.
	hd := d.objects[holder].Value.(pdfDict)
	if ref, ok := hd["/B"].(pdfRef); !ok || ref.Num != a {
		t.Errorf("holder /B = %v; want ref to keeper %d", hd["/B"], a)
	}
	if ref, ok := hd["/A"].(pdfRef); !ok || ref.Num != a {
		t.Errorf("holder /A = %v; want ref to keeper %d", hd["/A"], a)
	}
}

// TestOptimizeRemovedObjectsCountsBothPasses pins GitHub issue #9's fix: the
// pre-pass Optimize now runs at its start (to avoid RemoveDuplicateStreams
// and CompressStreams hashing/scanning dead content-stream copies
// appendToContentStream leaves behind, see Optimize's doc comment) must have
// its removed-count ADDED to the final pass's, not overwritten by it. With
// only RemoveUnusedObjects enabled here, the pre-pass removes every dead
// copy and the final pass then finds nothing left — a `res.RemovedObjects =
// d.RemoveUnusedObjects()` on the final call (instead of +=) would silently
// report 0 removed even though real garbage was cleared.
func TestOptimizeRemovedObjectsCountsBothPasses(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	// Each call replaces /Contents with a brand-new object; the previous one
	// is left dead. 6 calls -> 5 dead copies for the pre-pass to clear.
	for i := 0; i < 6; i++ {
		if err := p.AddText(fmt.Sprintf("line %d", i), TextStyle{Size: 10},
			Rectangle{LLX: 50, LLY: float64(700 - i*20), URX: 300, URY: float64(720 - i*20)}); err != nil {
			t.Fatal(err)
		}
	}

	before := len(doc.objects)
	res, err := doc.Optimize(OptimizationOptions{RemoveUnusedObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	after := len(doc.objects)

	if got, want := before-after, res.RemovedObjects; got != want {
		t.Errorf("Optimize actually removed %d objects (before=%d after=%d) but reported RemovedObjects=%d", got, before, after, want)
	}
	if res.RemovedObjects < 5 {
		t.Errorf("RemovedObjects = %d, want at least 5 (the dead content-stream copies)", res.RemovedObjects)
	}
}

func TestRemoveDuplicateStreamsRewritesContentRefs(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	content := bytes.Repeat([]byte("data "), 20)
	a := d.addObject(&pdfStream{Dict: pdfDict{}, Data: content, Decoded: true})
	b := d.addObject(&pdfStream{Dict: pdfDict{}, Data: append([]byte(nil), content...), Decoded: true})

	// A content stream that references the duplicate by "b 0 R" in its bytes.
	body := []byte("/X " + itoa(b) + " 0 R Do")
	holder := d.addObject(&pdfStream{Dict: pdfDict{}, Data: body, Decoded: true})

	if got := d.removeDuplicateStreams(); got != 1 {
		t.Fatalf("removed %d; want 1", got)
	}
	newBody := string(d.objects[holder].Value.(*pdfStream).Data)
	want := "/X " + itoa(a) + " 0 R Do"
	if newBody != want {
		t.Errorf("content ref not rewritten: got %q want %q", newBody, want)
	}
}
