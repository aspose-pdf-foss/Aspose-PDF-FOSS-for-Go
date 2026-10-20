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

// TestOptimizeRemovedObjectsCountsBothPasses pins GitHub issue #9's fix:
// the pre-pass Optimize runs at its start (to avoid RemoveDuplicateStreams
// and CompressStreams hashing/scanning dead objects for nothing, see
// Optimize's doc comment) must have its removed-count ADDED to the final
// pass's, not overwritten by it — a `res.RemovedObjects =
// d.RemoveUnusedObjects()` on the final call (instead of +=) would
// silently report only whatever the SECOND call found, hiding whatever
// the pre-pass already removed.
//
// Originally exercised via appendToContentStream's dead content-stream
// copies (repeated AddText calls on one page each left the previous
// /Contents stream dead) — that root cause is now closed at the source
// (content_append.go, pdf-go-jbiu/GitHub issue #16: an append adds one
// /Contents array entry instead of rewriting a combined stream, so
// nothing is ever abandoned), so this test instead injects synthetic
// unreachable objects directly, independent of any particular drawing
// call's behavior.
func TestOptimizeRemovedObjectsCountsBothPasses(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	if _, err := doc.Page(1); err != nil {
		t.Fatal(err)
	}
	// Six objects reachable from nothing -- generic stand-ins for "dead
	// garbage", the same shape the pre-pass exists to clear regardless of
	// how it was produced (a third-party file's own pre-existing orphans,
	// or this library's full-content-replace call sites like
	// ApplyRedactions/ConvertToGrayscale, which still abandon whatever
	// /Contents pointed at before).
	for i := 0; i < 6; i++ {
		doc.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte(fmt.Sprintf("dead %d", i)), Decoded: true})
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
	if res.RemovedObjects < 6 {
		t.Errorf("RemovedObjects = %d, want at least 6 (the injected dead objects)", res.RemovedObjects)
	}
}

// TestRemoveDuplicateStreamsRewritesStructuralRefsNotStreamBytes pins the
// corrected behavior after pdf-go-2apc / GHSA-696v-26w4-39vc:
// removeDuplicateStreams repoints a merged-away duplicate's references
// through the STRUCTURAL object graph (a dict/array pdfRef — here, the
// page's own /Resources/XObject entry) via rewriteRefs, but must never
// touch a stream's own DATA bytes, even when they happen to contain a
// digit sequence shaped exactly like a reference to the duplicate. A
// real content stream never names a resource this way (Do operands are
// always resource NAMES resolved through /Resources, never a raw
// "N G R" embedded in the operator stream) — rewriteRefs used to scan
// and rewrite stream bytes anyway, which both corrupted any ordinary
// page text that happened to contain such a sequence AND was never
// needed for the structural case this function actually exists to
// handle, which the first half of this test confirms still works.
func TestRemoveDuplicateStreamsRewritesStructuralRefsNotStreamBytes(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	content := bytes.Repeat([]byte("data "), 20)
	a := d.addObject(&pdfStream{Dict: pdfDict{}, Data: content, Decoded: true})
	b := d.addObject(&pdfStream{Dict: pdfDict{}, Data: append([]byte(nil), content...), Decoded: true})

	// The STRUCTURAL way a page actually references an XObject: a
	// /Resources/XObject dict entry, a real pdfRef — not anything
	// embedded in stream bytes.
	page, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	resources := page.pageResources()
	xobj := pdfDict{"/X": pdfRef{Num: b}}
	resources["/XObject"] = xobj

	// A content stream whose own bytes happen to contain a digit
	// sequence shaped exactly like a reference to the duplicate (b) —
	// pure coincidence, same as ordinary page text could produce (e.g.
	// "see item 3 0 R" in a price list) — which must survive byte-for-byte.
	coincidence := []byte("/X Do % see item " + itoa(b) + " 0 R for details")
	holder := d.addObject(&pdfStream{Dict: pdfDict{}, Data: coincidence, Decoded: true})

	if got := d.removeDuplicateStreams(); got != 1 {
		t.Fatalf("removed %d; want 1", got)
	}

	// Structural repoint: the page's /Resources/XObject entry must now
	// point at the keeper (a), not the removed duplicate (b). rewriteRefs
	// returns a fresh deep copy rather than mutating in place, so this
	// must be re-read from d.objects (obj.Value was reassigned), not from
	// the stale local xobj map built before the call.
	newPageDict, ok := d.objects[page.pageObj().Num].Value.(pdfDict)
	if !ok {
		t.Fatal("page object is no longer a dict")
	}
	newResources, ok := newPageDict["/Resources"].(pdfDict)
	if !ok {
		t.Fatal("page /Resources is no longer a dict")
	}
	newXObj, ok := newResources["/XObject"].(pdfDict)
	if !ok {
		t.Fatal("/Resources/XObject is no longer a dict")
	}
	if ref, ok := newXObj["/X"].(pdfRef); !ok || ref.Num != a {
		t.Errorf("/Resources/XObject/X = %v, want a plain ref to the keeper object %d", newXObj["/X"], a)
	}

	// Stream bytes: untouched, even though they contain the duplicate's
	// own (now-stale, but irrelevant) number in "N 0 R" shape.
	gotBody := string(d.objects[holder].Value.(*pdfStream).Data)
	if gotBody != string(coincidence) {
		t.Errorf("content stream bytes were rewritten: got %q, want the original %q unchanged", gotBody, coincidence)
	}
}
