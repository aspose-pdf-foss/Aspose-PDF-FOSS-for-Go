// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// maxFormRedactDepth bounds Form-XObject recursion while redacting into
// one, the same way maxFormDepth (text.go) bounds it during text
// extraction. Unlike text extraction's cap, hitting this one while a
// region genuinely intersects the Do at that depth is treated as unsafe
// (an error), not a silent skip: by construction, recursion only
// happens for a Do already found to intersect a region, so reaching the
// cap means there IS unredacted, intersecting content this pass is
// refusing to examine — the same "refuse rather than leave incomplete"
// principle as a non-invertible matrix or an unresolvable /BBox (code
// review of this fix: an earlier draft skipped silently here, which is
// exactly the class of silent, incomplete "success" this whole feature
// exists to close).
const maxFormRedactDepth = 32

// maxFormRedactWork bounds the TOTAL number of Form XObject placements
// redacted into across every nested level within ONE redacted PAGE — a
// depth cap alone does not bound the WORK done at a given depth: a form
// that places two region-intersecting copies of a child, each placing two
// more, doubles the work every level, so maxFormRedactDepth's generous 32
// levels alone could still mean an astronomical number of
// redactFormXObject calls (and cloned objects) for an
// adversarially-constructed file. Once this budget is exhausted,
// ApplyRedactions refuses (an error) rather than continuing to burn
// CPU/memory on a file shaped to maximize Form-XObject fan-out —
// confirmed reachable (not just theoretical) by an independent review of
// this fix's first draft, which had no such bound at all.
//
// Scoped per page, not per whole ApplyRedactions call: a fresh
// formRedactBudget is constructed inside rewriteImageOperatorsInStream,
// which applyRedactionsToPage calls once per redacted page. A document
// with many redacted pages, each individually under this budget, can
// still do more total work than maxFormRedactWork across the call as a
// whole — an earlier revision of this comment (and of ApplyRedactions'
// own doc comment) claimed a single call-wide total, which an
// independent review found was not what the code actually does. This is
// an intentional, not merely overlooked, scope: the property this budget
// actually guards against is EXPONENTIAL fan-out WITHIN one page/one
// nesting tree, which stays bounded regardless of page count; a per-call
// total would instead degrade into a page-count limit on large, entirely
// ordinary documents (many pages, each with a few ordinarily-nested
// forms) — ApplyRedactions should not start refusing a 10,000-page
// document purely for being long.
const maxFormRedactWork = 10000

// formRedactBudget is a work counter, shared (by pointer) across every
// level of Form XObject recursion within the ONE page it was constructed
// for — see maxFormRedactWork for why the scope is per-page, not
// per-ApplyRedactions-call. It also memoizes clones (pdf-go-wuv7) and
// remembers the next free "/FmN" resource index, both for the same
// reason: every level of recursion, and both callers that create clones
// (redactFormDo for page content, redactOneAnnotationAppearance for
// annotation appearances), share this one instance per page.
type formRedactBudget struct {
	spent int

	// clones memoizes an already-produced redacted clone's object number,
	// keyed by what fully determines its content (formCloneKey): the
	// source Form XObject stream, the CTM composing with its own
	// /Matrix, and the inherited text state (pdf-go-w7ye) seeding its
	// nested text rewrite, if any. Without this, several byte-identical
	// placements of the same form — a logo stamped on every page, or
	// several widgets sharing one appearance stream — each got their own
	// full duplicate clone object, inflating the saved file far beyond
	// what the content actually needs; maxFormRedactWork bounds the
	// COUNT of placements redacted into, but not, before this, their
	// aggregate SIZE.
	clones map[formCloneKey]int

	// fmCursor remembers the next "/FmN" index nextFormXObjectResourceName
	// should try first — without it, every call rescanned from "/Fm0",
	// making total resource-naming cost quadratic in the number of
	// clones created on one page.
	fmCursor int
}

func (b *formRedactBudget) consume() error {
	if b == nil {
		return nil // unit tests that construct a bare rewriteXObjectDoOperatorsInStream call pass no budget; unbounded is fine for a single hand-built op list
	}
	b.spent++
	if b.spent > maxFormRedactWork {
		return fmt.Errorf("asposepdf: ApplyRedactions: exceeded the Form XObject redaction work budget (%d); refusing rather than continuing on what looks like a pathologically-nested or fan-out-heavy file", maxFormRedactWork)
	}
	return nil
}

// formCloneKey identifies one Form XObject placement's redacted output
// uniquely enough to memoize it (pdf-go-wuv7): redactFormXObject's output
// depends only on the source stream, the CTM in effect (composed with the
// form's own /Matrix, which is intrinsic to form and so need not be
// included separately), the redaction regions (constant within one page —
// collected once in applyRedactionsToPage — so not part of the key), and
// the inherited text state seeding its nested text rewrite, if any. Two
// placements agreeing on all of these produce byte-identical redacted
// content. form is compared by pointer identity, which correctly
// distinguishes different source objects — resolveRef always returns the
// same *pdfStream for the same object number within one Document, and
// nothing in this redaction pass ever mutates an original (unredacted)
// form's stream value in place (clone-not-mutate throughout).
type formCloneKey struct {
	form *pdfStream
	ctm  [6]float64

	// The inherited-text-state fields, flattened rather than embedding
	// *textGState directly: textGState holds a *fontInfo, whose identity
	// belongs to one rewriter's font interner — two passes over the same
	// resources resolve the same font to different pointers. fontName (the resource name "/F1" etc. the state was resolved
	// under) is a faithful proxy for font identity within the one
	// resources dict a given redaction pass works from.
	hasInherited bool
	fontName     string
	fontSize     float64
	charSpace    float64
	wordSpace    float64
	horizScale   float64
	leading      float64
	rise         float64 // Ts (pdf-go-lx1f) — two placements differing only in inherited rise can redact different glyphs near a region boundary, so it must be part of what distinguishes a memoized clone
}

func newFormCloneKey(st *pdfStream, ctm [6]float64, inherited *textGState) formCloneKey {
	k := formCloneKey{form: st, ctm: ctm}
	if inherited != nil {
		k.hasInherited = true
		k.fontName = inherited.fontName
		k.fontSize = inherited.fontSize
		k.charSpace = inherited.charSpace
		k.wordSpace = inherited.wordSpace
		k.horizScale = inherited.horizScale
		k.leading = inherited.leading
		k.rise = inherited.rise
	}
	return k
}

// lookupClone/storeClone are the memoization table's read/write sides; a
// nil budget (bare unit-test calls with no budget) always misses, so
// those callers see no behavior change at all.
func (b *formRedactBudget) lookupClone(key formCloneKey) (int, bool) {
	if b == nil || b.clones == nil {
		return 0, false
	}
	id, ok := b.clones[key]
	return id, ok
}

func (b *formRedactBudget) storeClone(key formCloneKey, id int) {
	if b == nil {
		return
	}
	if b.clones == nil {
		b.clones = map[formCloneKey]int{}
	}
	b.clones[key] = id
}

// privateXObjectResources returns a resources dict that is SAFE for a
// redaction pass to add/remove entries in: a fresh shallow copy of
// resources itself, with its OWN fresh copies of the /XObject and /Font
// sub-dicts (the two sub-dicts a redaction pass can actually write to —
// /XObject for a redirected clone, /Font because renderRedactOverlay's
// AddText call registers a Standard-14 font resource for the overlay
// text). Every other sub-dict (/ColorSpace, /ExtGState, …) is carried
// over by reference, unchanged, since nothing here mutates those.
//
// Without this, redactFormDo's redirect-this-one-placement-only logic,
// rewriteXObjectDoOperatorsInStream's resource pruning, and the overlay
// font registration were all mutating whatever dict resources["/XObject"]
// or resources["/Font"] resolved to DIRECTLY — but that dict is not
// necessarily private to the one page or one Form XObject placement
// being processed: a Form XObject's own /Resources is shared by EVERY
// placement of that same form object (not just the one a redaction
// region happens to intersect), and a page's /Resources can itself be
// shared across pages via an indirect reference (a common PDF producer
// optimization). Mutating it in place for one placement's sake corrupted
// every OTHER placement/page sharing it — confirmed reachable (an
// untouched page sharing a redacted form's own resources lost that
// form's nested content entirely) by an independent review of this
// fix's first draft. Installing a private copy — via this helper,
// called once per page in applyRedactionsToPage and once per
// placement-needing-a-clone in redactFormDo/redactFormXObject — decouples
// each one, so only the dict belonging to the ONE page or the ONE
// private clone this call is building ever changes.
func privateXObjectResources(objects map[int]*pdfObject, resources pdfDict) pdfDict {
	fresh := make(pdfDict, len(resources)+1)
	for k, v := range resources {
		fresh[k] = v
	}
	for _, key := range []string{"/XObject", "/Font"} {
		original, _ := resolveRefToDict(objects, resources[key])
		cp := make(pdfDict, len(original))
		for k, v := range original {
			cp[k] = v
		}
		fresh[key] = cp
	}
	return fresh
}

// rewriteImageOperatorsInStream removes Image XObject invocations (Do)
// whose painted bbox falls entirely inside any redact region, wraps
// partially-overlapping invocations in a q...Q with an even-odd clip
// path that masks out the redacted area, and — this is the function's
// other half, despite the name kept for minimal API churn — recurses a
// redaction pass into a Form XObject's own content when its placement
// intersects a region at all (see rewriteXObjectDoOperatorsInStream and
// redactFormXObject for how; pdf-go-w5hl, the real fix for a v0.11.0
// safety stopgap that used to refuse outright here instead).
//
// Fully-outside Do operators are passed through unchanged. The CTM
// state machine tracks q/Q nesting and cm pre-multiplications.
// Operators other than q/Q/cm/Do are passed through unchanged.
func rewriteImageOperatorsInStream(data []byte, regions []QuadPoint, doc *Document, resources pdfDict) ([]byte, error) {
	// One budget per call — in practice, one per page (applyRedactionsToPage
	// calls this once per redacted page) — bounds the total Form XObject
	// redaction work this call can do; see maxFormRedactWork. initial=nil:
	// a page's own content has nothing to inherit a text state from.
	return rewriteXObjectDoOperatorsInStream(data, regions, doc, resources, 0, &formRedactBudget{}, nil, nil)
}

// rewriteXObjectDoOperatorsInStream is rewriteImageOperatorsInStream with
// an explicit recursion depth, so redactFormXObject can call back into it
// for a Form XObject's own content at depth+1 (nested forms) while the
// top-level, page-content call starts at depth 0 — an explicit work
// budget, shared (by pointer) across every level of that recursion — and
// preserve, a set of resource names this call must never prune from
// resources["/XObject"] even if nothing in THIS content stream references
// them anymore, because some OTHER consumer this function has no
// visibility into (an annotation appearance stream with no own
// /Resources, for the page-level call — see pagePreservedXObjectNames)
// still resolves its own Do names through this exact dict. nil for every
// recursive (Form-XObject-level) call: nothing outside a freshly cloned
// form's own content ever looks at that clone's /Resources, so there is
// nothing else to preserve there.
//
// initial seeds the text-state portion (font/fontSize/charSpace/
// wordSpace/horizScale/leading/rise — never ctm) of this walker's OWN
// running state, the same way rewriteTextOperatorsInStream's own initial
// parameter does — needed so a NESTED Form XObject (one placed by a Do
// inside another form's content, itself reached only because the OUTER
// form inherited a text state at ITS Do) continues the inheritance chain
// correctly instead of restarting at zero one level down. nil for the
// page-level call (rewriteImageOperatorsInStream): a page has nothing to
// inherit from. This walker tracks Tf/Tc/Tw/Tz/TL/Ts (saved/restored by
// q/Q, same as rewriteTextOperatorsInStream's own gStack) purely to
// capture the state in effect AT EACH Do, to hand to redactFormDo/
// redactFormXObject for that specific placement's nested text rewrite —
// it has no text-rendering operators of its own to apply that state to
// (pdf-go-w7ye).
func rewriteXObjectDoOperatorsInStream(data []byte, regions []QuadPoint, doc *Document, resources pdfDict, depth int, budget *formRedactBudget, preserve map[string]bool, initial *textGState) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}
	fonts := newFontInterner(resolveFontResources(objects, resources))

	ctm := identityMatrix()
	var (
		font       = emptyFontInfo
		fontName   string
		fontSize   float64
		charSpace  float64
		wordSpace  float64
		horizScale = 1.0
		leading    float64
		rise       float64 // Ts (pdf-go-lx1f)
	)
	if initial != nil {
		font, fontName = initial.font, initial.fontName
		fontSize, charSpace, wordSpace = initial.fontSize, initial.charSpace, initial.wordSpace
		horizScale, leading = initial.horizScale, initial.leading
		rise = initial.rise
	}
	var stack []textGState

	out := make([]contentOp, 0, len(ops))
	for _, op := range ops {
		switch op.Operator {
		case "q":
			stack = append(stack, textGState{
				ctm: ctm, font: font, fontName: fontName, fontSize: fontSize,
				charSpace: charSpace, wordSpace: wordSpace, horizScale: horizScale,
				leading: leading, rise: rise,
			})
			out = append(out, op)
		case "Q":
			if n := len(stack); n > 0 {
				s := stack[n-1]
				stack = stack[:n-1]
				ctm, font, fontName, fontSize = s.ctm, s.font, s.fontName, s.fontSize
				charSpace, wordSpace, horizScale, leading = s.charSpace, s.wordSpace, s.horizScale, s.leading
				rise = s.rise
			}
			out = append(out, op)
		case "cm":
			// cm operands: a b c d e f — set CTM = [a b c d e f] ∘ current_CTM
			// Per ISO 32000-1 §8.4.4: the new CTM = matrix_from_operands × old_CTM
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
			out = append(out, op)
		case "Tf":
			if len(op.Operands) >= 2 {
				fontName = operandName(op.Operands[0])
				font = fonts.get(fontName)
				fontSize = operandFloat(op.Operands[1])
			}
			out = append(out, op)
		case "Tc":
			if len(op.Operands) >= 1 {
				charSpace = operandFloat(op.Operands[0])
			}
			out = append(out, op)
		case "Tw":
			if len(op.Operands) >= 1 {
				wordSpace = operandFloat(op.Operands[0])
			}
			out = append(out, op)
		case "Tz":
			if len(op.Operands) >= 1 {
				horizScale = operandFloat(op.Operands[0]) / 100.0
			}
			out = append(out, op)
		case "TL":
			if len(op.Operands) >= 1 {
				leading = operandFloat(op.Operands[0])
			}
			out = append(out, op)
		case "Ts":
			if len(op.Operands) >= 1 {
				rise = operandFloat(op.Operands[0])
			}
			out = append(out, op)
		case "Do":
			if len(op.Operands) == 0 {
				out = append(out, op)
				continue
			}
			name := operandName(op.Operands[0])
			st := resolveXObjectStream(objects, resources, name)
			if st != nil && dictGetName(st.Dict, "/Subtype") == "/Form" {
				// The text state in effect AT THIS Do — what the form
				// actually inherits per ISO 32000-1 §8.10.1, ctm handled
				// separately as always (see redactFormXObject).
				inherited := &textGState{
					font: font, fontName: fontName, fontSize: fontSize,
					charSpace: charSpace, wordSpace: wordSpace,
					horizScale: horizScale, leading: leading, rise: rise,
				}
				newOp, err := redactFormDo(doc, st, ctm, regions, resources, depth, op, budget, inherited)
				if err != nil {
					return nil, err
				}
				out = append(out, newOp)
				continue
			}

			out = emitClassifiedImageOp(out, op, ctm, regions)
		case "BI":
			// An inline image (BI ... ID ... EI, ISO 32000-1 §8.9.7) paints
			// into the unit square under the current CTM exactly like an
			// Image XObject's Do (image_inline_write.go emits the same
			// preceding "q <w> 0 0 <h> <x> <y> cm" positioning convention —
			// see render.go's own "BI" case, which paints it through the
			// identical classifyDoBbox unit-square assumption). Already
			// parsed as a single contentOp (parseContentStream's own BI
			// handling consumes through ID/EI internally), so the same
			// drop/clip/keep classification used for a non-Form Do applies
			// verbatim — closes pdf-go-xp2u's inline-image half: before
			// this, BI fell through to the default case and passed through
			// a redaction region completely untouched, the same class of
			// leak as pdf-go-ye1z but for pixel data that never became a
			// separate XObject in the first place.
			out = emitClassifiedImageOp(out, op, ctm, regions)
		default:
			out = append(out, op)
		}
	}

	finalData := serializeContentOps(out)

	// Prune resources["/XObject"] down to exactly the names something
	// reachable from the FINAL rewritten content — directly, or
	// transitively through a nested Form XObject that has no own
	// /Resources and so resolves ITS Do names through this same dict —
	// still needs, plus whatever the caller says must survive regardless
	// (preserve). This replaces an earlier, reactive "only remove a name
	// if every Do invocation of it in this stream got redirected to a
	// clone" heuristic, which an independent review found both
	// under-pruned (a cloned form's OWN private resources — built as a
	// full copy of whatever it inherited — kept forwarding a reference to
	// the very original object the clone exists to replace, through an
	// entry the clone's content never uses at all, so the original
	// (unredacted) form stayed reachable and un-collectable by
	// RemoveUnusedObjects — findings this specific heuristic could never
	// catch, since it only ever reacted to names it had itself redirected)
	// and over-pruned (removing a name a sibling Form XObject sharing this
	// same resources dict still calls, even though nothing in THIS
	// content stream does anymore). Computing "needed" directly from the
	// final content, transitively, fixes both: an unused inherited entry
	// is correctly dropped (nothing in the rewritten content, or anything
	// it calls, ever reaches it), and a name a sibling still needs is
	// correctly found (the transitive scan walks into that sibling too,
	// since it also has no own /Resources) and kept.
	if xobjDict, ok := resolveRefToDict(objects, resources["/XObject"]); ok {
		needed := map[string]bool{}
		neededXObjectNames(finalData, objects, resources, depth, budget, needed, map[*pdfStream]bool{})
		for name := range xobjDict {
			if needed[name] || preserve[name] {
				continue
			}
			delete(xobjDict, name)
		}
	}

	return finalData, nil
}

// emitClassifiedImageOp appends op (a non-Form Do, or a BI inline image) to
// out, classified against regions under the unit-square convention every
// raster-painting operator shares (ISO 32000-1 §8.9.5.1 for an Image
// XObject, §8.9.7 for an inline image — both paint into [0,1]×[0,1] under
// whatever CTM is in effect, so classifyDoBbox applies to either
// identically): dropped entirely when fully inside a region, wrapped in a
// local clip when partially overlapping, passed through unchanged
// otherwise.
func emitClassifiedImageOp(out []contentOp, op contentOp, ctm [6]float64, regions []QuadPoint) []contentOp {
	action := classifyDoBbox(ctm, regions)
	switch action.kind {
	case keepDo:
		out = append(out, op)
	case dropDo:
		// skip emission — op and its visual contribution are suppressed
	case clipDo:
		// Wrap op in a local q...Q with an even-odd clip path that masks
		// out the redacted region(s) while keeping the unredacted area.
		out = append(out, contentOp{Operator: "q"})
		out = append(out, buildImageClipPath(action.bbox, regions)...)
		out = append(out, op)
		out = append(out, contentOp{Operator: "Q"})
	}
	return out
}

// neededXObjectNames scans data for every /XObject resource name actually
// invoked by a Do operator, then — for any such name resolving to a Form
// XObject with NO own /Resources, so it resolves ITS OWN Do names through
// this exact same resources dict (ISO 32000-1 §7.8.3's inheritance rule)
// — recurses into that form's content too, since those names are just as
// "needed" by resources as anything the top-level content calls directly.
// Results accumulate into needed (so a caller can seed it, or share one
// map across several top-level scans against the same resources dict).
//
// Depth- and work-budget-capped the same way the real redaction recursion
// is (maxFormRedactDepth/maxFormRedactWork) — but conservatively: hitting
// either cap, or a cycle (visited), simply stops recursing into THAT one
// branch (the name that led to it is already marked needed either way)
// rather than erroring, since this function must never UNDER-report what
// is needed — a false "still needed" only costs a little unreclaimed
// cleanup, while a false "no longer needed" could delete an entry
// something else still resolves through.
func neededXObjectNames(data []byte, objects map[int]*pdfObject, resources pdfDict, depth int, budget *formRedactBudget, needed map[string]bool, visited map[*pdfStream]bool) {
	ops, err := parseContentStream(data)
	if err != nil {
		return
	}
	for _, op := range ops {
		if op.Operator != "Do" || len(op.Operands) == 0 {
			continue
		}
		name := operandName(op.Operands[0])
		needed[name] = true
		st := resolveXObjectStream(objects, resources, name)
		if st == nil || dictGetName(st.Dict, "/Subtype") != "/Form" {
			continue
		}
		if _, hasOwn := st.Dict["/Resources"]; hasOwn {
			continue // own /Resources — does not resolve names through THIS dict
		}
		if visited[st] || depth+1 >= maxFormRedactDepth || budget.consume() != nil {
			continue
		}
		visited[st] = true
		neededXObjectNames(decodedStreamData(st), objects, resources, depth+1, budget, needed, visited)
	}
}

// redactFormDo decides what to do with one Do invocation of a Form
// XObject: pass it through unchanged when its placement (formPlacementBbox,
// using the form's own /BBox+/Matrix, not an Image XObject's unit square)
// doesn't intersect any region at all, refuse (error) when the placement
// can't be trusted (bad/missing /BBox) or its matrix can't be inverted to
// map regions into the form's content, or redirect it to a freshly cloned,
// redacted copy of the form when redactFormXObject actually changed
// something.
//
// A clone, never an in-place mutation of the original form object OR
// its /Resources dict: the same Form XObject can be Do-invoked from
// several placements — on this page or another, the same stream shared
// via indirect reference, think a logo stamped on every page — and only
// some of those placements may overlap a redaction region. Rewriting
// the shared content stream in place would blank it everywhere;
// redirecting only THIS Do operand to a private copy (a fresh object
// number, a fresh resource name scoped to this one invocation) leaves
// every other reference to the original object — this stream, on this
// page or any other — completely untouched. Confirmed necessary by an
// independent review of this function's first draft, which redacted the
// shared stream directly. A SECOND, easy-to-miss sharing hazard the same
// review caught: a Form XObject's own /Resources dict is ALSO shared by
// every placement of that form, not just the one being redacted — so
// redactFormXObject never resolves or mutates it directly either, only
// a private copy (see privateXObjectResources), which THIS function
// installs as the clone's own /Resources below.
//
// resources is the CALLER's (parent content stream's) resources dict —
// already a private, safe-to-mutate copy by the time it reaches here
// (see privateXObjectResources): this function only ever adds ONE new
// entry to it (the clone's fresh name), it never resolves or mutates the
// form's OWN resources itself — that happens inside redactFormXObject.
// The caller no longer needs to know whether this Do operand was
// redirected to a clone or passed through unchanged: resources["/XObject"]
// pruning now works from the final rewritten content's actual Do targets
// (see neededXObjectNames), not from a per-name replaced/survived tally.
//
// inherited is the text state (font/fontSize/charSpace/wordSpace/
// horizScale/leading/rise) in effect at THIS specific Do, captured by the
// caller's walker — passed straight through to redactFormXObject, which
// uses it to seed the form's own text rewrite instead of starting at a
// zeroed/unknown state (pdf-go-w7ye): a form whose own content draws
// text with no Tf of its own relies entirely on this.
func redactFormDo(doc *Document, st *pdfStream, ctm [6]float64, regions []QuadPoint, resources pdfDict, depth int, original contentOp, budget *formRedactBudget, inherited *textGState) (contentOp, error) {
	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}
	minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, ctm)
	if !ok {
		name := ""
		if len(original.Operands) > 0 {
			name = operandName(original.Operands[0])
		}
		return contentOp{}, fmt.Errorf("asposepdf: ApplyRedactions: Form XObject %q has no resolvable /BBox; cannot safely determine whether a redaction region intersects it", name)
	}
	if classifyBbox(minX, minY, maxX, maxY, regions) == keepDo {
		return original, nil
	}

	// depth+1 is the depth the NESTED recursion below would run at — if
	// that would exceed the cap, refuse rather than silently leaving this
	// (already confirmed intersecting) Form XObject unexamined, same
	// "refuse rather than leave incomplete" principle as a non-invertible
	// matrix or an unresolvable /BBox (code review of an earlier draft,
	// which skipped silently at the cap instead).
	if depth+1 >= maxFormRedactDepth {
		name := ""
		if len(original.Operands) > 0 {
			name = operandName(original.Operands[0])
		}
		return contentOp{}, fmt.Errorf("asposepdf: ApplyRedactions: Form XObject %q nesting exceeds the redaction depth limit (%d); refusing rather than leaving an intersecting, unexamined Form XObject", name, maxFormRedactDepth)
	}
	if err := budget.consume(); err != nil {
		return contentOp{}, err
	}

	// A byte-identical earlier placement (same form, same composed CTM,
	// same inherited text state) already produced a clone this page —
	// reuse it instead of re-running the three rewriters and allocating
	// a full duplicate stream object (pdf-go-wuv7). Only the resource
	// NAME is placement-specific (this Do's resources dict might not be
	// the same dict an earlier placement registered into — a nested
	// form's own private resources, say); the underlying object is
	// shared.
	key := newFormCloneKey(st, ctm, inherited)
	if cachedID, ok := budget.lookupClone(key); ok {
		xobjDict, ok := resolveRefToDict(doc.objects, resources["/XObject"])
		if !ok {
			return contentOp{}, fmt.Errorf("asposepdf: ApplyRedactions: internal error: Form XObject resolved but its /XObject resources dict did not")
		}
		newName := nextFormXObjectResourceName(xobjDict, budget)
		xobjDict[newName] = pdfRef{Num: cachedID}
		return contentOp{Operator: "Do", Operands: []pdfValue{pdfName(newName)}}, nil
	}

	newData, newResources, changed, err := redactFormXObject(doc, st, resources, ctm, regions, depth+1, budget, inherited)
	if err != nil {
		return contentOp{}, err
	}
	if !changed {
		return original, nil
	}

	newID := doc.nextID
	doc.nextID++
	newDict := make(pdfDict, len(st.Dict))
	for k, v := range st.Dict {
		newDict[k] = v
	}
	// newResources is redactFormXObject's own PRIVATE resources copy, not
	// st.Dict's original /Resources reference: if redacting the form's
	// content redirected a NESTED Do to a clone, that clone's name was
	// registered into newResources, not into the (possibly shared with
	// other placements of this same form) original — using the original
	// here would leave the clone's content referencing a name its own
	// /Resources never got.
	if newResources != nil {
		newDict["/Resources"] = newResources
	}
	newDict["/Length"] = len(newData)
	// /Filter + /DecodeParms described the OLD (possibly compressed)
	// bytes; newData is raw/decoded, same convention as
	// replacePageContents — the writer re-Flates on output.
	delete(newDict, "/Filter")
	delete(newDict, "/DecodeParms")
	doc.objects[newID] = &pdfObject{Num: newID, Value: &pdfStream{
		Dict:    newDict,
		Data:    newData,
		Decoded: true,
	}}

	budget.storeClone(key, newID)

	xobjDict, ok := resolveRefToDict(doc.objects, resources["/XObject"])
	if !ok {
		return contentOp{}, fmt.Errorf("asposepdf: ApplyRedactions: internal error: Form XObject resolved but its /XObject resources dict did not")
	}
	newName := nextFormXObjectResourceName(xobjDict, budget)
	xobjDict[newName] = pdfRef{Num: newID}

	return contentOp{Operator: "Do", Operands: []pdfValue{pdfName(newName)}}, nil
}

// nextFormXObjectResourceName returns the next available /Fm* resource
// name in xobjDict — the same convention flatten.go's registerFormXObject
// uses for freshly-created Form XObjects. budget's fmCursor (nil-safe; a
// bare unit-test call with no budget just scans from 0 every time, as
// before) remembers where the LAST call left off across every clone
// created on this page, so a long run of clones doesn't rescan "/Fm0"
// upward from zero each time — nextFormXObjectResourceName used to make
// total naming cost quadratic in the number of clones (pdf-go-wuv7).
// Still verifies non-existence in THIS specific xobjDict before returning
// a name (never assumes the cursor alone is enough): a third-party file's
// own pre-existing "/FmN" entries, or a different resources dict than the
// one the cursor last advanced against, could still collide.
func nextFormXObjectResourceName(xobjDict pdfDict, budget *formRedactBudget) string {
	start := 0
	if budget != nil {
		start = budget.fmCursor
	}
	for i := start; ; i++ {
		name := fmt.Sprintf("/Fm%d", i)
		if _, exists := xobjDict[name]; !exists {
			if budget != nil {
				budget.fmCursor = i + 1
			}
			return name
		}
	}
}

// redactFormXObject recurses a redaction pass into a Form XObject's own
// content stream for the region(s) that intersect it. Regions are given
// in page space (the same space every other redaction entry point uses);
// this maps them into the form's LOCAL content-stream coordinate space by
// applying the INVERSE of the placement matrix in effect for this specific
// Do invocation — the form's own /Matrix composed with ctm, the same
// composition formPlacementBbox uses to go the other direction. Once the
// regions are in the form's own space, the form's content is just another
// content stream: the same three rewriters process it (this function
// recurses into rewriteXObjectDoOperatorsInStream at depth+1 for a nested
// Form XObject, so one level of nesting — or more, up to
// maxFormRedactDepth — is handled, not only the top one).
//
// Returns the redacted bytes and changed=true only if they actually
// differ from the original; the caller (redactFormDo) skips cloning
// entirely when nothing changed. In practice this rarely saves a clone:
// changed is a byte-for-byte comparison against the ORIGINAL (possibly
// hand-written or differently-formatted) stream bytes, and the three
// rewriters always re-tokenize and re-serialize via serializeContentOps
// once regions is non-empty, which normalizes whitespace/operand
// formatting even when nothing was actually filtered — so changed is
// effectively always true once the coarse placement bbox intersects a
// region at all, not only when something the form draws was inside it
// once mapped precisely (an earlier version of this comment overclaimed
// the latter; a real "did redaction actually remove anything" check
// would need to compare at the content-operator level, not raw bytes —
// not done, since over-cloning is a (documented) performance cost, not a
// correctness one, now that every clone's /Resources is both private and
// pruned to only what it needs — see neededXObjectNames). newResources is
// the PRIVATE resources dict this call used to resolve/redirect anything
// inside the form's own content (see privateXObjectResources) — nil when
// changed is false, otherwise the caller must install it as the clone's
// own /Resources, NOT reuse the form's original (possibly
// shared-across-placements) one: if a nested Do got redirected to ITS
// OWN clone, that clone's fresh resource name was registered into THIS
// private copy, not the original.
//
// The depth cap (maxFormRedactDepth) and the work budget
// (maxFormRedactWork) are both enforced by the caller (redactFormDo),
// before it ever calls this function — see redactFormDo's own doc
// comment for why depth is checked there and not here.
//
// inherited is the text state (font/fontSize/charSpace/wordSpace/
// horizScale/leading/rise) in effect at the Do that invoked this form,
// captured by the caller — per ISO 32000-1 §8.10.1 this form's own
// content inherits exactly that state (never ctm, which this form's
// content always starts at identity for, since localRegions above is
// already in the form's own local space). Used as the starting state
// for rewriteTextOperatorsInStream below, and threaded through to the
// nested rewriteXObjectDoOperatorsInStream call so a NESTED Form XObject
// one level further down continues the same inheritance chain
// (pdf-go-w7ye) rather than restarting at zero/unknown.
func redactFormXObject(doc *Document, st *pdfStream, parentResources pdfDict, ctm [6]float64, regions []QuadPoint, depth int, budget *formRedactBudget, inherited *textGState) (newData []byte, newResources pdfDict, changed bool, err error) {
	var objects map[int]*pdfObject
	if doc != nil {
		objects = doc.objects
	}

	formMatrix := identityMatrix()
	if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
		formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
	}
	composed := matMul(formMatrix, ctm)
	inv, ok := invertMatrix(composed)
	if !ok {
		return nil, nil, false, fmt.Errorf("asposepdf: ApplyRedactions: a Form XObject's placement matrix is not invertible; cannot map a redaction region into its content")
	}

	localRegions := make([]QuadPoint, len(regions))
	for i, q := range regions {
		localRegions[i] = transformQuad(q, inv)
	}

	formOwnResources := parentResources
	if resVal, ok := st.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(objects, resVal); ok {
			formOwnResources = rd
		}
	}
	// A FRESH, private copy — never the form's own (or inherited parent's)
	// resources dict directly. That dict can be shared by every OTHER
	// placement of this same form object (or, for the inherited-parent
	// case, by every other page/form that itself shares the parent's
	// resources); mutating it in place here — adding a nested clone's
	// name, or pruning a stale one — would corrupt every one of those
	// other, untouched placements. Confirmed reachable (an unrelated,
	// non-redacted page sharing this form lost its content entirely) by
	// an independent review of an earlier draft that resolved
	// formOwnResources directly and handed it straight to the rewriters.
	formResources := privateXObjectResources(objects, formOwnResources)

	original := decodedStreamData(st)
	fontMap := resolveFontResources(objects, formResources)

	data, err := rewriteTextOperatorsInStream(original, localRegions, fontMap, objects, formResources, inherited)
	if err != nil {
		return nil, nil, false, err
	}
	// preserve=nil: nothing outside this freshly-cloned form's own
	// content will ever look at formResources, so there is nothing else
	// that needs its /XObject entries kept beyond what this form's own
	// (rewritten) content actually calls — see neededXObjectNames.
	// initial=inherited: a NESTED Form XObject placed somewhere in THIS
	// form's own content inherits from where it's actually invoked
	// below, but if this form's content itself draws no Tj of its own
	// before reaching that nested Do, the chain traces back to the SAME
	// inherited state this form itself started from.
	data, err = rewriteXObjectDoOperatorsInStream(data, localRegions, doc, formResources, depth, budget, nil, inherited)
	if err != nil {
		return nil, nil, false, err
	}
	data, err = rewritePathOperatorsInStream(data, localRegions)
	if err != nil {
		return nil, nil, false, err
	}

	if string(data) == string(original) {
		return nil, nil, false, nil
	}
	return data, formResources, true, nil
}

// transformQuad maps every corner of q through the affine matrix m.
func transformQuad(q QuadPoint, m [6]float64) QuadPoint {
	x1, y1 := matApplyPoint(m, q.X1, q.Y1)
	x2, y2 := matApplyPoint(m, q.X2, q.Y2)
	x3, y3 := matApplyPoint(m, q.X3, q.Y3)
	x4, y4 := matApplyPoint(m, q.X4, q.Y4)
	return QuadPoint{X1: x1, Y1: y1, X2: x2, Y2: y2, X3: x3, Y3: y3, X4: x4, Y4: y4}
}

// doActionKind categorises what to do with a Do operator.
type doActionKind int

const (
	keepDo doActionKind = iota // no intersection — pass through
	dropDo                     // fully inside at least one region — suppress
	clipDo                     // partial overlap — clip the painting
)

// doAction is the classification result for a single Do operator.
type doAction struct {
	kind doActionKind
	bbox Rectangle // image bbox in page space (used for clipDo)
}

// classifyDoBbox computes the axis-aligned bbox of the unit-square Image
// XObject under the given CTM, then classifies its relationship to the
// redact regions. Image space is always the unit square [0,1]×[0,1] (ISO
// 32000-1 §8.9.5.1) — this must NOT be reused for a Form XObject, whose
// real painted extent is its own /BBox + /Matrix, not the unit square; see
// formPlacementBbox.
func classifyDoBbox(ctm [6]float64, regions []QuadPoint) doAction {
	x0, y0 := matApplyPoint(ctm, 0, 0)
	x1, y1 := matApplyPoint(ctm, 1, 0)
	x2, y2 := matApplyPoint(ctm, 0, 1)
	x3, y3 := matApplyPoint(ctm, 1, 1)
	minX := minF(minF(x0, x1), minF(x2, x3))
	maxX := maxF(maxF(x0, x1), maxF(x2, x3))
	minY := minF(minF(y0, y1), minF(y2, y3))
	maxY := maxF(maxF(y0, y1), maxF(y2, y3))
	return doAction{
		kind: classifyBbox(minX, minY, maxX, maxY, regions),
		bbox: Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY},
	}
}

// formPlacementBbox computes the real page-space axis-aligned bbox of a
// Form XObject placed with the given CTM: its own /BBox (ISO 32000-1
// §8.10.2 — any rectangle, NOT necessarily the unit square an Image
// XObject always uses) composed with its own /Matrix (§8.10.1, default
// identity, applied in form space before ctm). Reusing classifyDoBbox's
// unit-square assumption for a Form XObject is wrong and was shown, by an
// independent review, to silently defeat unsafeFormXObjectName for the
// common case of a non-trivial placement scale or a redaction region that
// doesn't happen to cover the form's CTM-origin corner (pdf-go-w5hl).
// Falls back to the unit square only if /BBox is missing or malformed —
// conservatively wrong in the same direction classifyDoBbox always was,
// not a new gap.
// ok is false when /BBox can't be resolved to at least 4 numbers — ISO
// 32000-1 §8.10.2 requires /BBox on every Form XObject, so its absence
// means non-conformant or adversarially-malformed input, exactly what a
// redaction-safety check should trust least. An earlier draft guessed the
// unit square in that case, which an independent review flagged as its
// own false-negative risk (the same "guess a safe-looking value instead
// of refusing" mistake that produced the two bugs before it in this
// chain) — callers must treat ok==false as unsafe outright, not fall back
// to any estimate.
func formPlacementBbox(objects map[int]*pdfObject, st *pdfStream, ctm [6]float64) (minX, minY, maxX, maxY float64, ok bool) {
	bb := shFloats(objects, st.Dict["/BBox"])
	if len(bb) < 4 {
		return 0, 0, 0, 0, false
	}
	bbox := Rectangle{LLX: bb[0], LLY: bb[1], URX: bb[2], URY: bb[3]}

	formMatrix := identityMatrix()
	if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
		formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
	}
	effective := matMul(formMatrix, ctm)

	x0, y0 := matApplyPoint(effective, bbox.LLX, bbox.LLY)
	x1, y1 := matApplyPoint(effective, bbox.URX, bbox.LLY)
	x2, y2 := matApplyPoint(effective, bbox.LLX, bbox.URY)
	x3, y3 := matApplyPoint(effective, bbox.URX, bbox.URY)
	minX = minF(minF(x0, x1), minF(x2, x3))
	maxX = maxF(maxF(x0, x1), maxF(x2, x3))
	minY = minF(minF(y0, y1), minF(y2, y3))
	maxY = maxF(maxF(y0, y1), maxF(y2, y3))
	return minX, minY, maxX, maxY, true
}

// buildImageClipPath produces content ops for an even-odd clip path that
// preserves the image area EXCEPT where it overlaps with a redact region.
//
// Algorithm (ISO 32000-1 §8.5.3.3.2 even-odd rule):
//   - Outer rectangle = full image bbox
//   - For each overlapping region: add a rectangle for the intersection
//   - Under the even-odd rule, the inner rectangles are "holes" inside
//     the outer bbox — paint occurs in bbox minus the holes.
//
// The clip is followed by "n" (no-op path paint) to apply it without marking.
func buildImageClipPath(bbox Rectangle, regions []QuadPoint) []contentOp {
	var ops []contentOp

	// Outer rectangle: the full image bbox.
	ops = append(ops, reOp(bbox.LLX, bbox.LLY, bbox.URX-bbox.LLX, bbox.URY-bbox.LLY))

	// Inner rectangles: intersections of image bbox with each redact region.
	for _, q := range regions {
		rMinX, rMinY, rMaxX, rMaxY := boundsOfQuad(q)

		// Compute intersection of image bbox and region aabb.
		iMinX := maxF(bbox.LLX, rMinX)
		iMinY := maxF(bbox.LLY, rMinY)
		iMaxX := minF(bbox.URX, rMaxX)
		iMaxY := minF(bbox.URY, rMaxY)

		if iMaxX > iMinX && iMaxY > iMinY {
			ops = append(ops, reOp(iMinX, iMinY, iMaxX-iMinX, iMaxY-iMinY))
		}
	}

	// Even-odd clip + no-paint path operator.
	ops = append(ops, contentOp{Operator: "W*"})
	ops = append(ops, contentOp{Operator: "n"})
	return ops
}

// reOp builds a rectangle path operator: x y w h re
func reOp(x, y, w, h float64) contentOp {
	return contentOp{
		Operator: "re",
		Operands: []pdfValue{
			pdfValue(x),
			pdfValue(y),
			pdfValue(w),
			pdfValue(h),
		},
	}
}

// readCMMatrix parses the 6 operands of a cm operator into a [6]float64 matrix.
// Returns (matrix, true) on success, (zero, false) if operands are missing/invalid.
func readCMMatrix(operands []pdfValue) ([6]float64, bool) {
	if len(operands) < 6 {
		return [6]float64{}, false
	}
	var m [6]float64
	for i := 0; i < 6; i++ {
		switch v := operands[i].(type) {
		case float64:
			m[i] = v
		case int:
			m[i] = float64(v)
		default:
			return [6]float64{}, false
		}
	}
	return m, true
}

// boundsOfQuad returns the axis-aligned bounding box of a QuadPoint.
func boundsOfQuad(q QuadPoint) (minX, minY, maxX, maxY float64) {
	minX = minF(minF(q.X1, q.X2), minF(q.X3, q.X4))
	maxX = maxF(maxF(q.X1, q.X2), maxF(q.X3, q.X4))
	minY = minF(minF(q.Y1, q.Y2), minF(q.Y3, q.Y4))
	maxY = maxF(maxF(q.Y1, q.Y2), maxF(q.Y3, q.Y4))
	return
}

// resolveXObjectStream resolves name within resources' /XObject dict to
// its stream object, or nil if it can't be resolved.
func resolveXObjectStream(objects map[int]*pdfObject, resources pdfDict, name string) *pdfStream {
	if resources == nil {
		return nil
	}
	xobjDict, ok := resolveRefToDict(objects, resources["/XObject"])
	if !ok {
		return nil
	}
	ref, ok := xobjDict[name]
	if !ok {
		return nil
	}
	st, _ := resolveRef(objects, ref).(*pdfStream)
	return st
}

// unsafeFormXObjectName is ValidateRedactions's read-only pre-flight
// counterpart to redactFormDo/redactFormXObject: it reports a Form
// XObject placement that intersects a redaction region AND cannot be
// safely redacted into — a missing/malformed /BBox (formPlacementBbox's
// own ok=false), or a placement matrix that composes to something
// non-invertible (so a region can't be mapped into the form's local
// content-stream space at all). A placement that intersects but CAN be
// redacted into — the common case, since pdf-go-w5hl — is no longer
// reported; only before that fix did any intersection count as unsafe.
//
// Recurses into a safely-resolvable form's own content at the SAME
// depth, depth cap, and work budget redactFormXObject would use (see
// maxFormRedactDepth/maxFormRedactWork), so a problem several levels of
// nesting down (a nested Form XObject with its own bad /BBox, say) is
// caught here too, before ApplyRedactions attempts any mutation —
// mirroring redactFormXObject's own recursion exactly (same
// formPlacementBbox/invertMatrix checks, same
// region-transform-via-inverse-matrix) is deliberate: ApplyRedactions's
// own mutation path is clone-based and never corrupts anything in place
// even if an unvalidated deeper level later fails and surfaces as its
// own error, so this check's only job is to give the caller advance
// warning, not to gate correctness.
func unsafeFormXObjectName(ops []contentOp, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict) (string, bool) {
	if len(regions) == 0 {
		return "", false
	}
	return unsafeFormXObjectNameDepth(ops, regions, objects, resources, 0, &formRedactBudget{})
}

func unsafeFormXObjectNameDepth(ops []contentOp, regions []QuadPoint, objects map[int]*pdfObject, resources pdfDict, depth int, budget *formRedactBudget) (string, bool) {
	ctm := identityMatrix()
	var stack [][6]float64
	for _, op := range ops {
		switch op.Operator {
		case "q":
			stack = append(stack, ctm)
		case "Q":
			if len(stack) > 0 {
				ctm = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "cm":
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
		case "Do":
			if len(op.Operands) == 0 {
				continue
			}
			name := operandName(op.Operands[0])
			st := resolveXObjectStream(objects, resources, name)
			if st == nil || dictGetName(st.Dict, "/Subtype") != "/Form" {
				continue
			}
			minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, ctm)
			if !ok {
				return name, true
			}
			if classifyBbox(minX, minY, maxX, maxY, regions) == keepDo {
				continue
			}

			// Recursing one level deeper, from here, would run at
			// depth+1 — if that reaches the cap, or the work budget is
			// exhausted, treat this placement as unsafe rather than
			// silently reporting it safe: redactFormXObject would refuse
			// at exactly this same point (code review of an earlier
			// draft, which stopped silently here instead — a validator
			// that says "safe" right where the real pass would error is
			// worse than one that is merely conservative).
			if depth+1 >= maxFormRedactDepth {
				return name, true
			}
			if budget.consume() != nil {
				return name, true
			}

			formMatrix := identityMatrix()
			if mv := shFloats(objects, st.Dict["/Matrix"]); len(mv) == 6 {
				formMatrix = [6]float64{mv[0], mv[1], mv[2], mv[3], mv[4], mv[5]}
			}
			inv, invOK := invertMatrix(matMul(formMatrix, ctm))
			if !invOK {
				return name, true
			}

			localRegions := make([]QuadPoint, len(regions))
			for i, q := range regions {
				localRegions[i] = transformQuad(q, inv)
			}
			formResources := resources
			if resVal, ok := st.Dict["/Resources"]; ok {
				if rd, ok := resolveRefToDict(objects, resVal); ok {
					formResources = rd
				}
			}
			innerOps, err := parseContentStream(decodedStreamData(st))
			if err != nil {
				return name, true
			}
			if innerName, unsafe := unsafeFormXObjectNameDepth(innerOps, localRegions, objects, formResources, depth+1, budget); unsafe {
				return innerName, true
			}
		}
	}
	return "", false
}
