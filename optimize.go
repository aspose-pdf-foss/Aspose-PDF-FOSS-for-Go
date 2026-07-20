// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// OptimizationOptions selects which reductions Document.Optimize applies.
// The zero value does nothing; use DefaultOptimizationOptions for the safe
// lossless preset. Mirrors Aspose.PDF for .NET's OptimizationOptions consumed
// by Document.OptimizeResources.
type OptimizationOptions struct {
	// RemoveUnusedObjects drops objects unreachable from any page.
	RemoveUnusedObjects bool
	// SubsetFonts rebuilds embedded TrueType fonts to the glyphs used.
	SubsetFonts bool
	// CompressStreams Flate-compresses source streams stored uncompressed
	// (lossless).
	CompressStreams bool
	// RemoveDuplicateStreams merges byte-identical stream objects into one
	// and repoints references (lossless).
	RemoveDuplicateStreams bool
	// Images, when non-nil, downscales/recodes images per the options
	// (potentially lossy — opt-in).
	Images *OptimizeImageOptions
}

// OptimizationResult reports what Document.Optimize changed.
type OptimizationResult struct {
	RemovedObjects      int
	SubsettedFonts      int
	OptimizedImages     int
	CompressedStreams   int
	DeduplicatedStreams int
}

// DefaultOptimizationOptions returns the safe, lossless preset: remove unused
// objects, subset fonts, compress uncompressed streams, and dedupe identical
// streams. Image recompression is left off (opt-in via Images because it can
// be lossy).
func DefaultOptimizationOptions() OptimizationOptions {
	return OptimizationOptions{
		RemoveUnusedObjects:    true,
		SubsetFonts:            true,
		CompressStreams:        true,
		RemoveDuplicateStreams: true,
	}
}

// Optimize reduces the document's size in place per opts and returns a report
// of what changed. Call before Save/WriteTo. Mirrors Aspose.PDF for .NET's
// Document.OptimizeResources(OptimizationOptions).
//
// Order: images → fonts → compress streams → dedupe streams → remove unused
// objects (so anything a transform orphans is reclaimed last).
func (d *Document) Optimize(opts OptimizationOptions) (OptimizationResult, error) {
	var res OptimizationResult
	if opts.Images != nil {
		n, err := d.OptimizeImages(*opts.Images)
		if err != nil {
			return res, fmt.Errorf("optimize: %w", err)
		}
		res.OptimizedImages = n
	}
	if opts.SubsetFonts {
		n, err := d.SubsetFonts()
		if err != nil {
			return res, fmt.Errorf("optimize: %w", err)
		}
		res.SubsettedFonts = n
	}
	if opts.CompressStreams {
		res.CompressedStreams = d.compressUncompressedStreams()
	}
	if opts.RemoveDuplicateStreams {
		res.DeduplicatedStreams = d.removeDuplicateStreams()
	}
	if opts.RemoveUnusedObjects {
		res.RemovedObjects = d.RemoveUnusedObjects()
	}
	return res, nil
}

// compressMinBytes: streams smaller than this are left alone — Flate framing
// overhead can make tiny streams larger.
const compressMinBytes = 48

// compressUncompressedStreams marks Flate-compressible source streams that are
// currently stored without a filter as decoded, so the writer Flate-compresses
// them on Save. Returns the count marked. Lossless.
func (d *Document) compressUncompressedStreams() int {
	n := 0
	for _, obj := range d.objects {
		st, ok := obj.Value.(*pdfStream)
		if !ok || st.Decoded {
			continue // decoded streams are already (re)compressed by the writer
		}
		if _, hasFilter := st.Dict["/Filter"]; hasFilter {
			continue // already filtered/compressed
		}
		if len(st.Data) < compressMinBytes || !compressibleStream(st.Dict) {
			continue
		}
		// No /Filter + Decoded==false → Data is raw uncompressed bytes.
		// Marking it decoded makes the writer add /FlateDecode.
		st.Decoded = true
		n++
	}
	return n
}

// compressibleStream reports whether a stream is safe to Flate-compress.
func compressibleStream(dict pdfDict) bool {
	switch dictGetName(dict, "/Type") {
	case "/Metadata", "/XRef":
		// XMP metadata must stay uncompressed for PDF/A; xref streams are
		// rebuilt by the writer.
		return false
	}
	if dictGetName(dict, "/Subtype") == "/Image" {
		return false // images belong to OptimizeImages
	}
	return true
}

// removeDuplicateStreams merges byte-identical stream objects (same normalized
// dict + same decoded content) into the lowest-numbered instance, repoints all
// references (including those embedded in content-stream bytes) to it, and
// deletes the duplicates. Returns the number of streams removed. Lossless.
//
// Limitation: two streams count as duplicates only when their dicts are
// literally equal (same referenced object numbers), so a producer that cloned
// a whole resource subtree with fresh numbers is not deduped.
func (d *Document) removeDuplicateStreams() int {
	// Deterministic order → the lowest object number is the canonical keeper.
	nums := make([]int, 0, len(d.objects))
	for num := range d.objects {
		nums = append(nums, num)
	}
	sort.Ints(nums)

	seen := map[string]int{} // signature → keeper object number
	idMap := map[int]int{}   // duplicate → keeper
	for _, num := range nums {
		st, ok := d.objects[num].Value.(*pdfStream)
		if !ok {
			continue
		}
		sig := streamSignature(st)
		if keep, ok := seen[sig]; ok {
			idMap[num] = keep
		} else {
			seen[sig] = num
		}
	}
	if len(idMap) == 0 {
		return 0
	}
	for _, obj := range d.objects {
		obj.Value = rewriteRefs(obj.Value, idMap)
	}
	for dup := range idMap {
		delete(d.objects, dup)
	}
	return len(idMap)
}

// streamSignature is a content hash of a stream: its decoded bytes plus a
// canonical serialization of its dict (excluding /Length, which varies with
// the stored encoding). Byte-identical streams share a signature.
func streamSignature(st *pdfStream) string {
	h := sha256.New()
	h.Write([]byte(canonicalDict(st.Dict)))
	h.Write([]byte{0})
	h.Write(decodedStreamData(st))
	return string(h.Sum(nil))
}

// canonicalDict serializes a dict deterministically (sorted keys), skipping
// /Length, for use in a stream signature.
func canonicalDict(dict pdfDict) string {
	keys := make([]string, 0, len(dict))
	for k := range dict {
		if k == "/Length" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('<')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(' ')
		b.WriteString(canonicalValue(dict[k]))
		b.WriteByte(';')
	}
	b.WriteByte('>')
	return b.String()
}

// canonicalValue serializes a value deterministically for signatures.
func canonicalValue(v pdfValue) string {
	switch val := v.(type) {
	case pdfDict:
		return canonicalDict(val)
	case pdfArray:
		var b strings.Builder
		b.WriteByte('[')
		for _, e := range val {
			b.WriteString(canonicalValue(e))
			b.WriteByte(',')
		}
		b.WriteByte(']')
		return b.String()
	case pdfRef:
		return fmt.Sprintf("R%d", val.Num)
	case pdfName:
		return string(val)
	case *pdfStream:
		return "S" + canonicalDict(val.Dict)
	default:
		return fmt.Sprintf("%v", val)
	}
}
