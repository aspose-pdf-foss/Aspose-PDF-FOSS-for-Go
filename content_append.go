// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// contentAppendMarkerKey is a private, non-spec dict entry (same
// convention as barcodeMarkerKey, barcode.go) written on the tiny "Q"
// stream appendToContentStream inserts the one time it needs to protect
// newly-appended content from whatever graphics state the page's
// PRE-EXISTING content might leave open (pdf-go-p1yc). Its presence
// anywhere in a page's /Contents array means that protective wrap has
// already happened — on this document instance, or (since the marker is
// an ordinary dict entry, so it round-trips through Save/Open like any
// other) in an earlier session — so a later append can just add one more
// array entry instead of wrapping again. Unknown dict keys are ignored by
// every conforming reader, so this is safe to carry in the file.
const contentAppendMarkerKey = "/AsposeCB"

// contentsEntries normalizes a page's current /Contents value (absent,
// null, a single stream, or an array — ISO 32000-1 §7.8.2 allows all
// four read-side, and page.contentStreams() already concatenates an
// array when reading) into a flat list of entries ready to be rebuilt as
// a fresh array, and reports whether any entry already carries
// contentAppendMarkerKey.
//
// The returned slice is always a fresh copy, never an alias into
// whatever resolved it (a resolved pdfArray value could in principle be
// shared, e.g. an indirect /Contents reference reused by more than one
// page in an unusual third-party file) — callers go on to mutate it
// (append/prepend an entry), and mutating a shared backing array in
// place would corrupt that OTHER page's content too.
func contentsEntries(objects map[int]*pdfObject, contentsVal pdfValue) (entries []pdfValue, bounded bool) {
	if contentsVal == nil {
		return nil, false
	}
	resolved := resolveRef(objects, contentsVal)
	switch rv := resolved.(type) {
	case pdfArray:
		entries = make([]pdfValue, len(rv))
		copy(entries, rv)
		for _, e := range entries {
			if st, ok := resolveRef(objects, e).(*pdfStream); ok {
				if b, _ := st.Dict[contentAppendMarkerKey].(bool); b {
					bounded = true
				}
			}
		}
		return entries, bounded
	case *pdfStream:
		// Keep the original reference form (contentsVal, not a freshly
		// built one) — it may itself be an indirect reference the rest
		// of the document still expects to resolve the same way.
		return []pdfValue{contentsVal}, false
	default:
		return nil, false // absent, null, or unresolvable
	}
}

// newContentStream allocates a fresh, raw (Decoded: true — the writer
// Flate-compresses on output, same convention as replacePageContents and
// every other content-stream constructor in this codebase) stream object
// holding data, merges extraDict into its own dict, and returns a pdfRef
// to it.
func newContentStream(doc *Document, data []byte, extraDict pdfDict) pdfRef {
	dict := make(pdfDict, len(extraDict))
	for k, v := range extraDict {
		dict[k] = v
	}
	id := doc.nextID
	doc.nextID++
	doc.objects[id] = &pdfObject{Num: id, Value: &pdfStream{
		Dict:    dict,
		Data:    data,
		Decoded: true,
	}}
	return pdfRef{Num: id}
}

// appendToContentStream adds data as a new entry at the END of the
// page's content, after everything already there, without rewriting (or
// leaving dead in the file) a single byte of the existing content —
// closing pdf-go-jbiu: the previous implementation decoded the whole
// existing content, concatenated data onto it, and wrote the result as
// one brand-new stream, abandoning the old one in d.objects; the writer
// has no reachability pass of its own, so every such call left one more
// dead copy in the saved file, and file size grew roughly quadratically
// with the number of calls on a page (GitHub issue #16 point 1,
// shadowhunter1470-svg — the root cause this report traced GitHub issue
// #9/pdf-go-qgrn and the pdf-go-2qkq redaction advisory back to; both of
// those only ever swept up the SYMPTOM with a RemoveUnusedObjects call
// after the fact, an ordinary document that never calls
// Optimize/ApplyRedactions still accumulated the dead copies this closes
// at the source).
//
// Every one of this library's own drawing calls (AddText, Draw*,
// AddImage, …) already wraps its own emitted operators in q…Q, so the
// NEW entry never needs protecting from itself. What it DOES need
// protecting from is whatever graphics state the page's EXISTING
// content — on a page opened from a real-world, third-party-produced
// file, not guaranteed self-balanced at its own end: an unmatched `cm`,
// an active clip, a lingering `3 Tr` — leaves open, since per ISO
// 32000-1 §7.8.2 the graphics state is NOT reset between the streams
// making up one /Contents (array or otherwise) (pdf-go-p1yc; GitHub
// issue #16 point 2, same reporter, concrete repro: a page ending in an
// open `3 Tr`+half-scale `cm` rendered newly-added text invisible and a
// newly-added rectangle half-size/mispositioned). Fixed by wrapping the
// existing content in a one-time `q`…`Q` pair — by the time `Q` runs,
// the state is back to whatever it was before that content started
// (the default state, on the very first append) — the MOMENT this page's
// content transitions from "whatever was there before" into this
// method's own managed array form; contentAppendMarkerKey records that
// the transition has already happened so it is never redone. If there
// was no existing content at all, there is nothing to protect against
// and nothing to wrap — the new stream simply becomes /Contents outright,
// the same minimal single-stream shape a brand-new page always had.
func (p *Page) appendToContentStream(data []byte) error {
	pageDict := p.pageDict()
	if pageDict == nil {
		return fmt.Errorf("page has no dict")
	}
	entries, bounded := contentsEntries(p.doc.objects, pageDict["/Contents"])
	newRef := newContentStream(p.doc, data, nil)

	switch {
	case len(entries) == 0:
		pageDict["/Contents"] = newRef
	case bounded:
		pageDict["/Contents"] = pdfArray(append(entries, newRef))
	default:
		qRef := newContentStream(p.doc, []byte("q\n"), nil)
		endRef := newContentStream(p.doc, []byte("Q\n"), pdfDict{contentAppendMarkerKey: true})
		wrapped := make(pdfArray, 0, len(entries)+3)
		wrapped = append(wrapped, qRef)
		wrapped = append(wrapped, entries...)
		wrapped = append(wrapped, endRef)
		wrapped = append(wrapped, newRef)
		pageDict["/Contents"] = wrapped
	}
	return nil
}

// prependToContentStream inserts data BEFORE the page's existing
// content, closing the same pdf-go-jbiu dead-copy bug for the "draw
// behind everything" path (stamp.go's Background option, stamp_pdfpage.go,
// and TextStyle.Behind in text_add.go): the new entry becomes a new
// leading array entry instead of the whole page being decoded,
// re-concatenated, and rewritten as one brand-new stream. No q…Q
// protection is needed here the way appendToContentStream needs it for
// the opposite direction: the new content runs FIRST, before anything
// else, so it can only ever inherit the page's own default initial
// graphics state (never anything a prior drawing call left open), and
// — like every other drawing call in this codebase — it already wraps
// its own emitted operators in q…Q, so it cannot itself leak state
// forward into the content now running after it either.
func (p *Page) prependToContentStream(data []byte) error {
	pageDict := p.pageDict()
	if pageDict == nil {
		return fmt.Errorf("page has no dict")
	}
	entries, _ := contentsEntries(p.doc.objects, pageDict["/Contents"])
	newRef := newContentStream(p.doc, data, nil)

	if len(entries) == 0 {
		pageDict["/Contents"] = newRef
		return nil
	}
	wrapped := make(pdfArray, 0, len(entries)+1)
	wrapped = append(wrapped, newRef)
	wrapped = append(wrapped, entries...)
	pageDict["/Contents"] = wrapped
	return nil
}
