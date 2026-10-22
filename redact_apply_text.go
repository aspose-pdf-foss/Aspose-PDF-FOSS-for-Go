// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// rewriteTextOperatorsInStream removes glyphs whose test point falls
// inside any region from text-rendering operators (Tj, TJ, ', "),
// preserving the position of surviving glyphs via TJ kerning gaps. The
// test point is the glyph's horizontal center at its baseline, OFFSET
// BY TEXT RISE (y=rise in text space — Ts is tracked the same way
// charSpace/wordSpace/etc. are, pdf-go-lx1f, closed) — "center" in the
// rest of this file's comments means this, not the glyph's true 2D
// bounding-box center; a region whose y-range excludes both the
// baseline AND any active rise (e.g. one hand-drawn tightly over just
// the upper half of a line of ordinary, non-risen text) can still miss
// a glyph that is visibly inside it. The documented, intended workflow —
// build a RedactAnnotation from a SearchText match's own Rect — is
// unaffected either way: SearchText's own Rect always spans
// descender-to-ascender and so always contains the baseline (and, for
// the common case of a modest rise on an inline superscript/subscript,
// the risen position too) regardless of Ts.
//
// Ts itself is intentionally NOT applied to the EMC-time /ActualText
// position check below (case "EMC") — that check deliberately mirrors
// textExtractor.currentPos (text.go), which has no Ts case of its own
// either (consistent, not merely unfixed: fixing the extractor's own
// Ts-unawareness, so SearchText itself reports a risen glyph's true
// visual position, is a separate, deliberately out-of-scope change —
// the "ideally to SearchText too" note on pdf-go-lx1f — since it would
// also move every already-correct, Ts-unaware position SearchText and
// ExtractTextWithLayout currently report for existing documents, a
// wider blast radius than this file's own glyph-filtering fix).
//
// Form XObjects (Do op), inline images, and non-text operators are passed
// through unchanged at THIS level — a Form XObject's own content is
// redacted separately, by redact_apply_image.go's recursion into it, via
// its own call to this same function on just that form's content. Per
// ISO 32000-1 §8.10.1 a form inherits the FULL graphics state — including
// the text state — in effect at the Do that invoked it (except the CTM,
// handled separately: regions are already mapped into the form's own
// local content space by the time this function sees them, so the form's
// content always starts at ctm = identity regardless of what the page's
// CTM was at the Do). initial, when non-nil, supplies that inherited text
// state (font/fontName/fontSize/charSpace/wordSpace/horizScale/leading —
// its ctm field is never read here) as the walker's starting point instead
// of the usual zero/unknown state; redact_apply_image.go's
// rewriteXObjectDoOperatorsInStream captures it by tracking the SAME
// text-state operators (Tf/Tc/Tw/Tz/TL, saved/restored by q/Q) while it
// walks the CALLING content stream, and hands it to redactFormXObject,
// which passes it through to this call (pdf-go-w7ye, closed): a form
// whose own content draws text with no Tf of its own, relying on that
// inherited font, previously had its text ALWAYS treated as unknown-font
// (see below) here, which — since pdf-go-oq9i started refusing rather
// than silently passing unknown-font text through — made ApplyRedactions
// refuse outright on such a document instead of leaving the text
// unredacted; both the silent-leak and the outright-refusal failure modes
// are closed by resolving the font this function actually inherited.
// Every top-level call (the page's own content, ReplaceText) passes nil:
// a page has nothing to inherit from, and ReplaceText never recurses into
// a Form XObject in the first place.
//
// A glyph's page-space position is the text matrix (tm) composed with
// the graphics-state CTM — ISO 32000-1 §9.4.4's text rendering matrix is
// Trm = [font/rise submatrix] × Tm × CTM (the font-size/rise submatrix
// is deliberately omitted here: font size is already baked into the
// text-space advances glyphAdvance/glyphAdvanceCID compute, and text
// rise is the documented gap above), and the regions this function
// compares glyph centers against are always given in page space (the
// same space every other ApplyRedactions entry point uses). The CTM
// half of that composition was missing entirely until pdf-go-njbg: this
// function tracked tm (via BT/Td/TD/Tm/T*) but never ctm (via q/Q/cm),
// so any text drawn after a non-identity cm — e.g. the page-flipping
// "1 0 0 -1 0 H cm" many producers (Chrome/Skia among them) emit at the
// very start of a page's content to work in a top-left-origin coordinate
// system — was compared against regions at the WRONG position and so
// never matched, even though SearchText (which IS CTM-aware) reported
// the correct position and a RedactAnnotation built from that match
// looked, to the caller, like it should obviously work. Confirmed by
// direct reproduction, not just by reading the code: a page built with
// exactly that Y-flip cm reported ApplyRedactions success while the
// secret text stayed fully present and extractable afterward. The path
// rewriter (redact_apply_path.go) and the Form-XObject/image rewriter
// (redact_apply_image.go) both already tracked q/Q/cm correctly; only
// this file was missing it.
//
// A Tj/TJ/'/" drawn with a font this function cannot classify
// (fontMap has no usable entry — font.known == false) now REFUSES
// (returns an error) rather than passing it through unredacted, the
// same "refuse rather than guess" principle this feature already
// applies to a non-invertible Form-XObject matrix or an unresolvable
// /BBox (pdf-go-oq9i). Passing it through used to report success while
// several real cases left the text fully visible when rendered AND
// extractable via ExtractText/SearchText on the "redacted" file — not
// merely forensically recoverable from raw bytes, but readable through
// this library's own ordinary read APIs — because the renderer and the
// extractor's own fallback logic can resolve some fonts this
// redaction-specific flag calls unknown (an undeclared font name
// substitutes Helvetica on both read paths; a simple font with no
// usable /Encoding/ToUnicode still renders from its program's built-in
// encoding). objects/resources (both may be nil, in which case BDC
// properties referenced by name cannot be resolved and are left alone)
// are used ONLY for the /ActualText scrub below — this function never
// mutates resources, so no private copy is required.
//
// Marked-content /ActualText scrubbing (pdf-go-1ha8): a BDC's
// /ActualText property (routine in accessibility-tagged output from
// Word, InDesign, etc.) is a SEPARATE textual representation of
// whatever is drawn between that BDC and its matching EMC — glyphs
// inside are redacted normally by the Tj/TJ handling below, but
// /ActualText itself was never examined, so ExtractText/SearchText on
// an already-"redacted" page could still return the original secret
// text verbatim via that replacement mechanism, even with the
// underlying glyphs correctly gone. Fixed by tracking BDC/EMC nesting:
// if ANY glyph drawn inside a BDC/EMC span (at any nesting depth
// beneath it) was dropped, that BDC's properties are rewritten on EMC
// to drop /ActualText — always as a fresh INLINE dict, never by
// mutating a /Properties-resource-referenced dict in place (which can
// be shared by OTHER, untouched spans using the same name — the same
// sharing hazard class as pdf-go-w5hl's Form-XObject /Resources, just
// for marked-content properties instead).
//
// Two more scrub triggers, beyond "a glyph was dropped inside this
// span", were added after an independent review found the first draft
// (glyph-drop only) still missed real cases of exactly the leak this
// mechanism exists to close:
//
//   - The EMC-time ActualText-POSITION check (case "EMC" below): the
//     extractor (text.go) emits an /ActualText span's replacement text
//     AT EMC, positioned wherever tm∘ctm has moved TO by then —
//     advanceGlyph still runs for every one of the span's own
//     (suppressed) glyphs, but the position-recording half of emitRune
//     is exactly what /ActualText suppresses, so the reported position
//     does not overlap where those glyphs are actually drawn. The
//     documented workflow — find the secret via SearchText, build a
//     RedactAnnotation from THAT match's own Rect — therefore builds a
//     region around the EMC-time position, not around the span's own
//     glyphs, which the glyph-drop check alone can completely miss
//     (reproduced directly: filler glyphs nowhere near the region, the
//     /ActualText text itself never dropped by anything).
//   - The "Do" case below: this pass and the separate Form-XObject/
//     image rewriter (redact_apply_image.go) are two SEQUENTIAL passes
//     over the same content, each with its own notion of "dropped" —
//     a Form XObject placed via Do inside an /ActualText span is
//     redacted entirely by the LATER pass, which this one has no
//     visibility into. Conservatively marking the span dropped
//     whenever the Do's OWN placement could possibly overlap a region
//     (without knowing whether the later pass will actually drop
//     anything inside it — this pass has no way to ask) closes that
//     gap; reproduced directly with a Form XObject's secret text fully
//     inside the region while the enclosing span's own direct glyphs
//     were unrelated filler outside it.
//
// Two gaps the same review round found were closed later (pdf-go-tnsb):
// a BDC left unterminated at end-of-stream (a malformed or truncated
// stream) is now closed as if by EMC, so its scrub decision is still
// made; and a non-embedded simple font named by a Standard-14 alias
// (/Arial, /TimesNewRoman, …) with no /Encoding or /ToUnicode now
// resolves as known (font.go's resolveFont), instead of being refused
// by the unknown-font check below.
func rewriteTextOperatorsInStream(data []byte, regions []QuadPoint, fontMap map[string]fontInfo, objects map[int]*pdfObject, resources pdfDict, initial *textGState) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	// Walker state mirrors textExtractor but outputs contentOps instead of fragments.
	fonts := newFontInterner(fontMap)
	var (
		font       = emptyFontInfo
		fontName   string // current resource name like "/F1"
		fontSize   float64
		charSpace  float64
		wordSpace  float64
		horizScale = 1.0 // Tz / 100
		leading    float64
		rise       float64 // Ts, text space units, applied directly (ISO 32000-1 §9.4.4 — not scaled by fontSize, since this file already omits that submatrix; see this function's own doc comment)
		tm         = identityMatrix()
		lm         = identityMatrix()
		ctm        = identityMatrix()
		gStack     []textGState
		mcStack    []*mcSpan
	)
	if initial != nil {
		// Seed the text-state portion of the inherited graphics state
		// (pdf-go-w7ye) — never ctm, which always starts at identity here
		// regardless of what the caller's CTM was: regions have already
		// been mapped into this content's own local space by the time
		// they reach this function (redactFormXObject's inverse-matrix
		// transform), so comparing against tm∘identity is what's
		// already correct for every OTHER call into this function too.
		font, fontName = initial.font, initial.fontName
		fontSize, charSpace, wordSpace = initial.fontSize, initial.charSpace, initial.wordSpace
		horizScale, leading = initial.horizScale, initial.leading
		rise = initial.rise
	}

	out := make([]contentOp, 0, len(ops))

	// markDropped flags every currently-open marked-content span (at
	// every nesting depth) as having had something redacted inside it —
	// called whenever a Tj/TJ/'/" actually drops at least one glyph.
	markDropped := func() {
		for _, span := range mcStack {
			span.dropped = true
		}
	}

	// closeSpan applies a just-popped span's scrub decision: a span with
	// something redacted inside it loses its /ActualText, and the
	// enclosing span (now the top of mcStack) inherits "dropped", since
	// its own /ActualText is stale too.
	closeSpan := func(span *mcSpan) {
		if span.dropped && span.props != nil {
			if _, hasActualText := span.props["/ActualText"]; hasActualText {
				scrubbed := make(pdfDict, len(span.props))
				for k, v := range span.props {
					if k == "/ActualText" {
						continue
					}
					scrubbed[k] = v
				}
				// Always a FRESH inline dict — even if the original BDC
				// referenced /Properties by name — so a /Properties entry
				// shared by other, untouched spans is never mutated.
				out[span.outIdx] = contentOp{Operator: "BDC", Operands: []pdfValue{span.tagOperand, scrubbed}}
			}
		}
		if span.dropped && len(mcStack) > 0 {
			mcStack[len(mcStack)-1].dropped = true
		}
	}

	for _, op := range ops {
		switch op.Operator {

		case "BT":
			tm = identityMatrix()
			lm = identityMatrix()
			out = append(out, op)

		case "ET":
			out = append(out, op)

		case "BDC":
			span := &mcSpan{outIdx: len(out)}
			if len(op.Operands) >= 2 {
				span.tagOperand = op.Operands[0]
				if props, ok := resolveBDCProperties(objects, resources, op.Operands[1]); ok {
					span.props = make(pdfDict, len(props))
					for k, v := range props {
						span.props[k] = v
					}
				}
			}
			mcStack = append(mcStack, span)
			out = append(out, op)

		case "BMC":
			// BMC has no properties operand, so it can never itself carry
			// /ActualText — still pushed (empty) so EMC pops the right
			// nesting level and "dropped" propagates through it correctly.
			mcStack = append(mcStack, &mcSpan{outIdx: len(out)})
			out = append(out, op)

		case "EMC":
			out = append(out, op)
			if n := len(mcStack); n > 0 {
				span := mcStack[n-1]
				mcStack = mcStack[:n-1]

				// The extractor emits an /ActualText span's replacement
				// text AT EMC, positioned at the CURRENT tm∘ctm origin
				// (textExtractor.currentPos, text.go) — NOT overlapping
				// the span's own (suppressed) glyphs: advanceGlyph still
				// runs for every one of them, but the emitRune call that
				// would record their positions is the one /ActualText
				// specifically suppresses, so by EMC the text matrix has
				// already moved past wherever those glyphs actually sit.
				// A caller who finds the secret via SearchText — the
				// documented workflow — and builds a region from THAT
				// match is building it around THIS point, not around the
				// span's underlying glyphs, so the glyph-drop tracking
				// below (which this span's own Tj/TJ calls feed via
				// markDropped) can miss exactly the common case
				// SearchText→redact exists to catch: an ActualText span
				// whose drawn glyphs (filler, a different cipher, …)
				// never themselves fall in any region. Treating this
				// point the same way the extractor does closes that gap.
				// Found reachable by an independent review of this
				// mechanism's first draft (a region built from
				// SearchText's own reported match for the ActualText
				// string left the span, and its glyphs, completely
				// untouched).
				if ax, ay := matApplyPoint(matMul(tm, ctm), 0, 0); pointInAnyQuad(ax, ay, regions) {
					span.dropped = true
				}
				closeSpan(span)
			}

		case "q":
			// q saves the whole graphics state, not just the CTM — ISO
			// 32000-1 Table 52 lists character spacing, word spacing,
			// horizontal scaling, leading, text font and text font size
			// as graphics-state (not text-object-state) parameters, so
			// they are exactly as q/Q-scoped as the CTM is. tm/lm are
			// NOT part of the graphics state (only BT resets them) and
			// so are deliberately not saved here — this mirrors
			// textExtractor's own q/Q handling (text.go) exactly, field
			// for field except for the colour state this rewriter
			// doesn't track.
			gStack = append(gStack, textGState{
				ctm: ctm, font: font, fontName: fontName, fontSize: fontSize,
				charSpace: charSpace, wordSpace: wordSpace, horizScale: horizScale,
				leading: leading, rise: rise,
			})
			out = append(out, op)

		case "Q":
			if n := len(gStack); n > 0 {
				s := gStack[n-1]
				gStack = gStack[:n-1]
				ctm, font, fontName, fontSize = s.ctm, s.font, s.fontName, s.fontSize
				charSpace, wordSpace, horizScale, leading = s.charSpace, s.wordSpace, s.horizScale, s.leading
				rise = s.rise
			}
			out = append(out, op)

		case "cm":
			// cm operands: a b c d e f — new CTM = matrix_from_operands × old_CTM
			// (ISO 32000-1 §8.4.4), same convention redact_apply_path.go /
			// redact_apply_image.go already use.
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
			out = append(out, op)

		case "Tf":
			if len(op.Operands) >= 2 {
				fontName = operandName(op.Operands[0])
				// An unknown font resolves to emptyFontInfo, so known==false
				// is preserved for the refusal below.
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

		case "Td":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				lm = matMul(translateMatrix(tx, ty), lm)
				tm = lm
			}
			out = append(out, op)

		case "TD":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				leading = -ty
				lm = matMul(translateMatrix(tx, ty), lm)
				tm = lm
			}
			out = append(out, op)

		case "T*":
			lm = matMul(translateMatrix(0, -leading), lm)
			tm = lm
			out = append(out, op)

		case "Tm":
			if len(op.Operands) >= 6 {
				for i := 0; i < 6; i++ {
					tm[i] = operandFloat(op.Operands[i])
				}
				lm = tm
			}
			out = append(out, op)

		case "Tj":
			if len(op.Operands) >= 1 {
				state := glyphFilterState{
					font:       font,
					fontName:   fontName,
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
					ctm:        ctm,
					rise:       rise,
				}
				newOps, totalAdvance, dropped, err := filterTjString(op.Operands[0], state, regions)
				if err != nil {
					return nil, err
				}
				// Advance the text matrix by total advance regardless of what survived.
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				if dropped {
					markDropped()
				}
				out = append(out, newOps...)
			} else {
				out = append(out, op)
			}

		case "TJ":
			if len(op.Operands) >= 1 {
				state := glyphFilterState{
					font:       font,
					fontName:   fontName,
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
					ctm:        ctm,
					rise:       rise,
				}
				newOps, totalAdvance, dropped, err := filterTJArray(op.Operands[0], state, regions)
				if err != nil {
					return nil, err
				}
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				if dropped {
					markDropped()
				}
				out = append(out, newOps...)
			} else {
				out = append(out, op)
			}

		case "'":
			// Equivalent to T* + Tj.
			lm = matMul(translateMatrix(0, -leading), lm)
			tm = lm
			// Emit a T* op to preserve the line advance.
			out = append(out, contentOp{Operator: "T*"})
			if len(op.Operands) >= 1 {
				state := glyphFilterState{
					font:       font,
					fontName:   fontName,
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
					ctm:        ctm,
					rise:       rise,
				}
				newOps, totalAdvance, dropped, err := filterTjString(op.Operands[0], state, regions)
				if err != nil {
					return nil, err
				}
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				if dropped {
					markDropped()
				}
				out = append(out, newOps...)
			}

		case "\"":
			// Equivalent to Tw a, Tc b, T*, Tj(string).
			if len(op.Operands) >= 3 {
				wordSpace = operandFloat(op.Operands[0])
				charSpace = operandFloat(op.Operands[1])
				lm = matMul(translateMatrix(0, -leading), lm)
				tm = lm
				// Emit Tw, Tc, T* ops to preserve spacing state.
				out = append(out, contentOp{Operator: "Tw", Operands: []pdfValue{op.Operands[0]}})
				out = append(out, contentOp{Operator: "Tc", Operands: []pdfValue{op.Operands[1]}})
				out = append(out, contentOp{Operator: "T*"})
				state := glyphFilterState{
					font:       font,
					fontName:   fontName,
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
					ctm:        ctm,
					rise:       rise,
				}
				newOps, totalAdvance, dropped, err := filterTjString(op.Operands[2], state, regions)
				if err != nil {
					return nil, err
				}
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				if dropped {
					markDropped()
				}
				out = append(out, newOps...)
			} else {
				out = append(out, op)
			}

		case "Do":
			// This pass never drops/clips a Do itself — that is the
			// SEPARATE image/Do rewriter's job, run as its own pass
			// AFTER this one. But a Do whose placement could overlap a
			// region, drawn while an /ActualText span is open, still
			// needs THAT span's dropped flag set here and now: these
			// two rewriters never share state otherwise, so if this
			// pass doesn't flag it, nothing ever will, and the later
			// pass dropping the Do's own content (an image, or a Form
			// XObject's text) leaves /ActualText — a separate textual
			// representation of the same span — stale and unscrubbed.
			// Conservative by design: ANY possible overlap counts,
			// without knowing whether the later pass will actually
			// end up dropping something inside it, since this pass has
			// no way to ask. Confirmed reachable by an independent
			// review: a redaction region covering a Form XObject's
			// secret text, placed inside a BDC/ActualText span with
			// unrelated filler glyphs of its own, used to leave that
			// span's /ActualText completely untouched.
			if len(op.Operands) >= 1 {
				if name := operandName(op.Operands[0]); name != "" {
					if st := resolveXObjectStream(objects, resources, name); st != nil {
						var kind doActionKind
						if dictGetName(st.Dict, "/Subtype") == "/Form" {
							if minX, minY, maxX, maxY, ok := formPlacementBbox(objects, st, ctm); ok {
								kind = classifyBbox(minX, minY, maxX, maxY, regions)
							} else {
								kind = dropDo // unresolvable /BBox: can't rule it out, assume the worst
							}
						} else {
							kind = classifyDoBbox(ctm, regions).kind
						}
						if kind != keepDo {
							markDropped()
						}
					}
				}
			}
			out = append(out, op)

		default:
			out = append(out, op)
		}
	}

	// A malformed or truncated stream can end with spans still open (a
	// BDC with no EMC). Close them innermost-first exactly as EMC would,
	// so a span with redacted content inside still loses its /ActualText
	// instead of carrying the original text out in the saved bytes
	// (pdf-go-tnsb point 1). The EMC-position check is skipped: with no
	// EMC, the extractor never emits the span's text at all.
	for len(mcStack) > 0 {
		span := mcStack[len(mcStack)-1]
		mcStack = mcStack[:len(mcStack)-1]
		closeSpan(span)
	}

	return serializeContentOps(out), nil
}

// mcSpan tracks one open BDC/BMC marked-content span while this
// function walks a content stream, for the /ActualText scrub
// (pdf-go-1ha8): outIdx is where its BDC/BMC op landed in out (so EMC
// can replace it in place once it knows whether anything inside was
// dropped), tagOperand is the BDC's original tag operand (reused
// verbatim if the op is rewritten), props is a PRIVATE, owned copy of
// its resolved properties dict (nil if it has none, or none could be
// resolved — e.g. a /Properties name with no resources to resolve it
// against), and dropped records whether any glyph drawn anywhere
// inside this span (at any nesting depth beneath it) was redacted.
type mcSpan struct {
	outIdx     int
	tagOperand pdfValue
	props      pdfDict
	dropped    bool
}

// resolveBDCProperties resolves a BDC op's second operand — either an
// inline dict or a pdfName looked up in resources' /Properties
// sub-dict (ISO 32000-1 §14.6.2) — to the properties dict it names,
// mirroring textExtractor.resolveActualText's resolution logic exactly
// (text.go) but returning the WHOLE dict rather than just /ActualText,
// since the scrub step needs every OTHER property preserved too. ok is
// false when nothing could be resolved (no resources, unknown name, or
// an operand of neither shape) — callers must treat that as "unknown",
// not "no properties/no ActualText".
func resolveBDCProperties(objects map[int]*pdfObject, resources pdfDict, operand pdfValue) (pdfDict, bool) {
	switch v := operand.(type) {
	case pdfDict:
		return v, true
	case pdfName:
		if resources == nil {
			return nil, false
		}
		propsVal, ok := resources["/Properties"]
		if !ok {
			return nil, false
		}
		propsDict, ok := resolveRefToDict(objects, propsVal)
		if !ok {
			return nil, false
		}
		entryVal, ok := propsDict[string(v)]
		if !ok {
			return nil, false
		}
		d, ok := resolveRefToDict(objects, entryVal)
		return d, ok
	default:
		return nil, false
	}
}

// textGState is the subset of this walker's running state that "q"
// saves and "Q" restores — the graphics-state (not text-object-state)
// parameters of ISO 32000-1 Table 52, the same fields textExtractor's
// own gsStack (text.go) saves/restores.
//
// font is a pointer into a shared, never-mutated fontInfo (see
// fontInterner), not a copy: a fontInfo is ~3 KB (two 256-entry arrays),
// so snapshotting it by value let a flood of unmatched "q" operators —
// two bytes each, highly compressible — cost ~3 KB of live memory apiece
// and ~17 KB of allocation once the stack's growth copies are counted
// (pdf-go-ifpy point 3; ~900 bytes per q now). text.go's extractorGState
// had the identical pattern and holds a pointer too.
type textGState struct {
	ctm        [6]float64
	font       *fontInfo
	fontName   string
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizScale float64
	leading    float64
	rise       float64 // Ts (pdf-go-lx1f)
}

// emptyFontInfo is the shared state for "no font selected" or "the named
// font did not resolve": known == false. Shared and never mutated, like
// every *fontInfo the walkers hand around.
var emptyFontInfo = &fontInfo{}

// fontInterner hands out one shared *fontInfo per font resource name, so
// a content-stream walker and every q/Q snapshot it takes point at the
// same fontInfo rather than each holding a ~3 KB copy. Scoped to one font
// map: a walker whose map changes (a Form XObject with its own
// /Resources) uses a fresh interner for it.
type fontInterner struct {
	src  map[string]fontInfo
	ptrs map[string]*fontInfo
}

func newFontInterner(src map[string]fontInfo) *fontInterner {
	return &fontInterner{src: src, ptrs: make(map[string]*fontInfo)}
}

// get returns the shared fontInfo for name, or emptyFontInfo when the map
// has no such font.
func (fi *fontInterner) get(name string) *fontInfo {
	if p, ok := fi.ptrs[name]; ok {
		return p
	}
	v, ok := fi.src[name]
	if !ok {
		return emptyFontInfo
	}
	p := &v
	fi.ptrs[name] = p
	return p
}

// glyphFilterState holds the current text rendering state for glyph filtering.
type glyphFilterState struct {
	font *fontInfo
	// fontName is the resource name (e.g. "/F1") font was resolved
	// from — carried only so an unknown-font refusal (pdf-go-oq9i) can
	// name which font resource is the problem.
	fontName   string
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizScale float64
	tm         [6]float64
	// ctm is the graphics-state CTM in effect for this Tj/TJ/'/" — a
	// glyph's page-space position is tm composed with ctm (see
	// rewriteTextOperatorsInStream's own doc comment, pdf-go-njbg), never
	// tm alone.
	ctm [6]float64
	// rise is the active text rise (Ts) in text-space units, applied
	// directly as the glyph test point's y coordinate before tm/ctm
	// (pdf-go-lx1f) — see rewriteTextOperatorsInStream's own doc comment
	// for why it needs no further scaling.
	rise float64
}

// glyphAdvance computes the text-space advance for a single-byte glyph code.
// Returns advance in text space units (same space as the text matrix translation).
func glyphAdvance(fi *fontInfo, code byte, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	w := fi.widths[code]
	if w == 0 {
		w = 500 // fallback per spec
	}
	adv := (w/1000.0*fontSize + charSpace) * horizScale
	if code == 0x20 {
		adv += wordSpace * horizScale
	}
	return adv
}

// glyphAdvanceCID computes the text-space advance for one Type0 glyph. cid
// is the RESOLVED CID (what /W is keyed by, ISO 32000-1 §9.7.4.3) — never
// the raw character code, which for a non-Identity CMap can differ from the
// CID entirely; nbytes is how many bytes of the content-stream string this
// glyph's code occupied (1 for a 1-byte code in a mixed-width CMap, 2 for
// Identity-H/V or a 2-byte CMap code). Word spacing (Tw) applies only when
// a SINGLE-BYTE code maps to CID 32 — ISO 32000-1 §9.3.3 restricts Tw to the
// single-byte code 32 and never to a multi-byte one, and this mirrors
// text.go's own extractor (advanceGlyphCID) exactly rather than
// re-deriving the condition independently (pdf-go-eys1/pdf-go-ifpy point 2,
// both closed by this same nbytes/cid-aware rewrite — see
// filterTjStringType0's own doc comment).
func glyphAdvanceCID(fi *fontInfo, cid uint16, nbytes int, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	w := fi.defaultW
	if cw, ok := fi.cidWidths[cid]; ok {
		w = cw
	}
	if w == 0 {
		w = fi.defaultW
		if w == 0 {
			w = 500
		}
	}
	adv := (w/1000.0*fontSize + charSpace) * horizScale
	if nbytes == 1 && cid == 32 {
		adv += wordSpace * horizScale
	}
	return adv
}

// matApplyPoint applies a PDF text/CTM matrix to a point (x, y) and returns user-space coords.
// The PDF matrix [a b c d e f] transforms (x,y) as:
//
//	x' = a*x + c*y + e
//	y' = b*x + d*y + f
func matApplyPoint(m [6]float64, x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// filterTjString processes a single Tj operand (a string), filtering glyphs
// that fall inside any region. Returns replacement ops, total text advance,
// and whether anything was actually dropped.
//
// If all glyphs are kept, returns the original op.
// If all glyphs are dropped, returns no ops.
// Otherwise returns a TJ op with kerning gaps for dropped glyphs.
//
// Returns an error, rather than passing the op through, when
// font.known == false (pdf-go-oq9i) — see rewriteTextOperatorsInStream's
// own doc comment for why passthrough is unsafe here, not merely
// imprecise: this redaction-pipeline-specific flag can be false even
// when the renderer or the extractor's own fallback can still resolve
// and display/extract the exact same text.
func filterTjString(operand pdfValue, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64, bool, error) {
	s, ok := operand.(string)
	if !ok {
		// Not a string operand — pass through as Tj.
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{operand}}}, 0, false, nil
	}

	if !state.font.known {
		// Generic wording, not "ApplyRedactions" specifically: this
		// function is shared by ApplyRedactions, Form-XObject
		// redaction, AND ReplaceText (replace.go reuses it to remove
		// the OLD text before drawing a replacement) — an error naming
		// the wrong caller would mislead whoever reads it.
		return nil, 0, false, fmt.Errorf("asposepdf: cannot safely determine the position of text drawn with font %q; refusing rather than silently leaving it unmodified", state.fontName)
	}

	if state.font.isType0 {
		ops, adv, dropped := filterTjStringType0(s, state, regions)
		return ops, adv, dropped, nil
	}
	ops, adv, dropped := filterTjStringSingleByte(s, state, regions)
	return ops, adv, dropped, nil
}

// filterTjStringSingleByte handles single-byte fonts.
func filterTjStringSingleByte(s string, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64, bool) {
	// Collect per-glyph info and decide which to drop.
	runningX := 0.0
	glyphs := make([]singleByteGlyph, len(s))
	totalAdvance := 0.0

	for i := 0; i < len(s); i++ {
		code := s[i]
		adv := glyphAdvance(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)

		// Glyph center in text space: (runningX + adv/2, 0).
		cx := runningX + adv/2.0
		ux, uy := matApplyPoint(matMul(state.tm, state.ctm), cx, state.rise)
		dropped := pointInAnyQuad(ux, uy, regions)

		glyphs[i] = singleByteGlyph{code: code, advance: adv, dropped: dropped}
		runningX += adv
		totalAdvance += adv
	}

	// Check if all kept or all dropped.
	droppedCount := 0
	for _, g := range glyphs {
		if g.dropped {
			droppedCount++
		}
	}

	if droppedCount == 0 {
		// All kept: return original Tj op unchanged.
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{s}}}, totalAdvance, false
	}

	if droppedCount == len(glyphs) {
		// All dropped: emit nothing.
		return nil, totalAdvance, true
	}

	// Mixed: build TJ array with kerning gaps.
	// TJ kerning: a negative number N shifts the next glyph right by -N/1000 * fontSize text-space units.
	// To account for a dropped glyph of width W thousandths, insert -(W_thousandths) where W_thousandths = w/horizScale * 1000/fontSize.
	// Actually: advance = (w/1000 * fontSize + charSpace + wordSpace) * horizScale
	// To encode this advance as a TJ kerning (which is applied as -k/1000 * fontSize, WITHOUT horizScale in TJ):
	// We need: -k/1000 * fontSize = advance / horizScale  (undo horizScale that TJ doesn't apply)
	// Wait — TJ kerning is applied differently. Let's be precise:
	//   TJ numeric entry N: displacement = -N/1000 * fontSize  (text space, before horizScale)
	//   The actual user-space shift = displacement * horizScale = -N/1000 * fontSize * horizScale
	// We want to shift by the dropped glyph advance (already includes horizScale):
	//   droppedAdvance = (w/1000 * fontSize + charSpace + [wordSpace if space]) * horizScale
	// So: -N/1000 * fontSize * horizScale = droppedAdvance
	//   N = -droppedAdvance * 1000 / (fontSize * horizScale)
	// However, charSpace and wordSpace are also added to TJ strings — TJ doesn't add them for numeric entries.
	// For simplicity in the kerning gap (which represents dropped glyphs), we use the full advance
	// converted back to TJ units: N = -advance * 1000 / (fontSize * horizScale)
	// Note: horizScale of 1.0 (common case) simplifies this to -advance * 1000 / fontSize.

	arr := buildTJArraySingleByte(glyphs, state)
	if len(arr) == 0 {
		return nil, totalAdvance, true
	}
	return []contentOp{{Operator: "TJ", Operands: []pdfValue{arr}}}, totalAdvance, true
}

type singleByteGlyph struct {
	code    byte
	advance float64
	dropped bool
}

// buildTJArraySingleByte builds a pdfArray for a TJ operator from a slice of glyph decisions.
// Kept glyphs go into string segments; dropped glyphs become negative kerning numbers.
func buildTJArraySingleByte(glyphs []singleByteGlyph, state glyphFilterState) pdfArray {
	var arr pdfArray
	var kept []byte
	var pendingKernAdv float64 // accumulated advance of dropped glyphs waiting to become kerning

	flush := func() {
		if pendingKernAdv != 0 {
			// Convert advance back to TJ kerning units.
			// TJ kerning N shifts by -N/1000 * fontSize (text-space, before horizScale).
			// We want to compensate for pendingKernAdv (which is already in text-space post-horizScale).
			// So N = -pendingKernAdv * 1000 / (fontSize * horizScale)
			fs := state.fontSize
			hs := state.horizScale
			if fs == 0 || hs == 0 {
				fs = 1
				hs = 1
			}
			kern := -pendingKernAdv * 1000.0 / (fs * hs)
			arr = append(arr, kern)
			pendingKernAdv = 0
		}
		if len(kept) > 0 {
			arr = append(arr, string(kept))
			kept = kept[:0]
		}
	}

	for _, g := range glyphs {
		if g.dropped {
			// Flush any accumulated kept glyphs first, then accumulate kerning.
			if len(kept) > 0 {
				arr = append(arr, string(kept))
				kept = kept[:0]
			}
			pendingKernAdv += g.advance
		} else {
			// Flush pending kerning first, then accumulate glyph.
			if pendingKernAdv != 0 {
				fs := state.fontSize
				hs := state.horizScale
				if fs == 0 || hs == 0 {
					fs = 1
					hs = 1
				}
				kern := -pendingKernAdv * 1000.0 / (fs * hs)
				arr = append(arr, kern)
				pendingKernAdv = 0
			}
			kept = append(kept, g.code)
		}
	}
	flush()

	return arr
}

// cidGlyph is one decoded Type0 glyph in a Tj/TJ string: raw is the exact
// content-stream bytes this glyph's code occupied (2 bytes for
// Identity-H/V, 1-4 for a font with its own CMap — pdf-go-eys1), kept
// verbatim so a surviving glyph's bytes are re-emitted unchanged.
type cidGlyph struct {
	raw     []byte
	advance float64
	dropped bool
}

// decodeCIDGlyphs decodes s into its Type0 glyphs, computing each one's
// advance/dropped status against localTM (already positioned at this
// string's own start) and regions. Two decode paths: when state.font.
// cidCMap is nil (Identity-H/V, ISO 32000-1 §9.7.6.2 Table 118's special
// predefined encodings — code IS the CID, always exactly 2 bytes), the
// fixed-stride loop this file always used; otherwise (a predefined CJK
// CMap like 90ms-RKSJ-H, or the PDF's own embedded /Encoding CMap stream)
// decodes through the font's actual codespace via cidCMap.next, which can
// mix 1- and multi-byte codes in one string — font.go's resolveType0Encoding
// already builds this cidCMap, and the text EXTRACTOR (text.go's
// showStringCMap) and the RENDERER (render_text.go) already decode through
// it the same way; this redaction rewriter used to be the one place in the
// codebase that still assumed every Type0 string is fixed-2-byte
// regardless, which — for any font using such a CMap — could decode glyph
// boundaries, CIDs, and therefore widths and positions, from the wrong
// bytes entirely: dropping the wrong chunk of text, keeping a secret that
// should have been redacted, or garbling unrelated survivor text,
// depending on the specific bytes. Width lookup is keyed by the RESOLVED
// CID either way (glyphAdvanceCID), not the raw code — the two coincide
// for Identity-H/V by definition, but differ for a real CMap, where the
// OLD code here looked widths up by the wrong key even before pdf-go-eys1
// mattered at all.
func decodeCIDGlyphs(s string, state glyphFilterState, localTM [6]float64, regions []QuadPoint) ([]cidGlyph, float64) {
	b := []byte(s)
	var glyphs []cidGlyph
	runningX := 0.0
	totalAdvance := 0.0

	if state.font.cidCMap != nil {
		for len(b) > 0 {
			_, cid, n := state.font.cidCMap.next(b)
			adv := glyphAdvanceCID(state.font, cid, n, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)
			cx := runningX + adv/2.0
			ux, uy := matApplyPoint(matMul(localTM, state.ctm), cx, state.rise)
			dropped := pointInAnyQuad(ux, uy, regions)
			glyphs = append(glyphs, cidGlyph{raw: b[:n], advance: adv, dropped: dropped})
			runningX += adv
			totalAdvance += adv
			b = b[n:]
		}
		return glyphs, totalAdvance
	}

	for i := 0; i+1 < len(b); i += 2 {
		cid := uint16(b[i])<<8 | uint16(b[i+1])
		adv := glyphAdvanceCID(state.font, cid, 2, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)
		cx := runningX + adv/2.0
		ux, uy := matApplyPoint(matMul(localTM, state.ctm), cx, state.rise)
		dropped := pointInAnyQuad(ux, uy, regions)
		glyphs = append(glyphs, cidGlyph{raw: b[i : i+2], advance: adv, dropped: dropped})
		runningX += adv
		totalAdvance += adv
	}
	return glyphs, totalAdvance
}

// filterTjStringType0 handles Type0 (CID) fonts — see decodeCIDGlyphs for
// how glyph boundaries are decoded.
func filterTjStringType0(s string, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64, bool) {
	glyphs, totalAdvance := decodeCIDGlyphs(s, state, state.tm, regions)

	droppedCount := 0
	for _, g := range glyphs {
		if g.dropped {
			droppedCount++
		}
	}

	if droppedCount == 0 {
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{s}}}, totalAdvance, false
	}
	if droppedCount == len(glyphs) {
		return nil, totalAdvance, true
	}

	// Build TJ array for CID font.
	var arr pdfArray
	var keptBytes []byte
	var pendingKernAdv float64

	for _, g := range glyphs {
		if g.dropped {
			if len(keptBytes) > 0 {
				arr = append(arr, string(keptBytes))
				keptBytes = keptBytes[:0]
			}
			pendingKernAdv += g.advance
		} else {
			if pendingKernAdv != 0 {
				fs := state.fontSize
				hs := state.horizScale
				if fs == 0 || hs == 0 {
					fs = 1
					hs = 1
				}
				kern := -pendingKernAdv * 1000.0 / (fs * hs)
				arr = append(arr, kern)
				pendingKernAdv = 0
			}
			keptBytes = append(keptBytes, g.raw...)
		}
	}
	// Flush trailing.
	if pendingKernAdv != 0 {
		fs := state.fontSize
		hs := state.horizScale
		if fs == 0 || hs == 0 {
			fs = 1
			hs = 1
		}
		kern := -pendingKernAdv * 1000.0 / (fs * hs)
		arr = append(arr, kern)
	}
	if len(keptBytes) > 0 {
		arr = append(arr, string(keptBytes))
	}

	if len(arr) == 0 {
		return nil, totalAdvance, true
	}
	return []contentOp{{Operator: "TJ", Operands: []pdfValue{arr}}}, totalAdvance, true
}

// filterTJArray processes a TJ operand (array of strings and kerning numbers).
// Returns replacement ops and total text advance accumulated across all elements.
//
// Strategy: walk the TJ array, tracking the current tm internally.
// Strings get per-glyph filtering; numeric entries are accumulated into the output
// and also applied to the running position.
func filterTJArray(operand pdfValue, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64, bool, error) {
	arr, ok := operand.(pdfArray)
	if !ok {
		return []contentOp{{Operator: "TJ", Operands: []pdfValue{operand}}}, 0, false, nil
	}

	if !state.font.known {
		// Generic wording, not "ApplyRedactions" specifically: this
		// function is shared by ApplyRedactions, Form-XObject
		// redaction, AND ReplaceText (replace.go reuses it to remove
		// the OLD text before drawing a replacement) — an error naming
		// the wrong caller would mislead whoever reads it.
		return nil, 0, false, fmt.Errorf("asposepdf: cannot safely determine the position of text drawn with font %q; refusing rather than silently leaving it unmodified", state.fontName)
	}

	// We'll build a new TJ array, tracking position as we go.
	// The state.tm is the tm at the start; we maintain a running advance.
	runningAdvance := 0.0 // text-space advance so far in this TJ
	totalAdvance := 0.0   // total advance including numeric kerning

	var outArr pdfArray
	// anyDropped tracks whether ANY glyph in this whole TJ array was
	// actually redacted. Without it, filterTJArray always rebuilt the
	// array (through compactTJArray below) even when NOTHING was
	// dropped — compactTJArray strips a leading/trailing kerning-only
	// entry unconditionally, which is correct for one left over after a
	// drop emptied the glyphs around it, but wrong for one the ORIGINAL
	// document author put there intentionally (e.g. letter-spacing
	// before the first glyph in the array) on a TJ this redaction pass
	// never actually touched — silently shifting unrelated, untouched
	// text sideways on any page that merely HAS a redaction region
	// somewhere, found by an independent review. filterTjStringSingleByte/
	// filterTjStringType0 already have this "nothing dropped -> return
	// the original op verbatim" guard; this gives filterTJArray the same
	// one.
	anyDropped := false

	for _, elem := range arr {
		switch v := elem.(type) {
		case string:
			// Filter the string glyphs given the current running tm.
			localTM := matMul(translateMatrix(runningAdvance, 0), state.tm)
			localState := state
			localState.tm = localTM

			var glyphs []singleByteGlyph
			var cidGlyphs []cidGlyph

			var stringAdvance float64

			if state.font.isType0 {
				cidGlyphs, stringAdvance = decodeCIDGlyphs(v, state, localTM, regions)
			} else {
				lx := 0.0
				for i := 0; i < len(v); i++ {
					code := v[i]
					adv := glyphAdvance(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)
					cx := lx + adv/2.0
					ux, uy := matApplyPoint(matMul(localTM, state.ctm), cx, state.rise)
					dropped := pointInAnyQuad(ux, uy, regions)
					glyphs = append(glyphs, singleByteGlyph{code: code, advance: adv, dropped: dropped})
					lx += adv
					stringAdvance += adv
				}
			}

			// Now append to outArr.
			if state.font.isType0 {
				for _, g := range cidGlyphs {
					if !g.dropped {
						// Append kept bytes as a string fragment.
						outArr = append(outArr, string(g.raw))
					} else {
						anyDropped = true
						// Append kerning gap.
						fs := state.fontSize
						hs := state.horizScale
						if fs == 0 || hs == 0 {
							fs = 1
							hs = 1
						}
						kern := -g.advance * 1000.0 / (fs * hs)
						outArr = append(outArr, kern)
					}
				}
			} else {
				for _, g := range glyphs {
					if !g.dropped {
						outArr = append(outArr, string([]byte{g.code}))
					} else {
						anyDropped = true
						fs := state.fontSize
						hs := state.horizScale
						if fs == 0 || hs == 0 {
							fs = 1
							hs = 1
						}
						kern := -g.advance * 1000.0 / (fs * hs)
						outArr = append(outArr, kern)
					}
				}
			}

			runningAdvance += stringAdvance
			totalAdvance += stringAdvance

		case int:
			// TJ kerning per ISO 32000-1 §9.4.3: the numeric adjustment is
			// -N/1000 * fontSize in UNSCALED text space, and like every
			// other term in the tx/ty formula (§9.4.4) it is THEN scaled
			// by horizScale — the same scaling glyphAdvance/glyphAdvanceCID
			// already bake into stringAdvance above, and the same scaling
			// this file's own kerning-number WRITERS (buildTJArraySingleByte
			// and the inline equivalents in filterTjStringType0/
			// filterTJArray) already divide out when they go the other
			// direction (advance -> TJ number). Reading a kerning number
			// without this scaling drifted runningAdvance (and therefore
			// every later glyph's computed page-space position) out of
			// sync with the glyph-advance side whenever Tz != 100 and the
			// TJ array already contained kerning — reachable on nearly any
			// real producer's output, since TJ kerning for ordinary letter
			// spacing is extremely common even when Tz is never touched by
			// this SPECIFIC document (the bug only needs Tz != 100 to be in
			// effect, not to be set inside this same TJ op). Found during
			// an independent review of pdf-go-njbg's cm-tracking fix.
			displacement := -float64(v) / 1000.0 * state.fontSize * state.horizScale
			runningAdvance += displacement
			totalAdvance += displacement
			outArr = append(outArr, elem)

		case float64:
			displacement := -v / 1000.0 * state.fontSize * state.horizScale
			runningAdvance += displacement
			totalAdvance += displacement
			outArr = append(outArr, elem)
		}
	}

	if !anyDropped {
		// Nothing in this TJ was actually redacted: return the ORIGINAL
		// operand verbatim rather than outArr (which is semantically
		// equivalent but not necessarily byte-identical, and — the bug
		// this guards against — would otherwise still go through
		// compactTJArray's leading/trailing-kerning strip below).
		return []contentOp{{Operator: "TJ", Operands: []pdfValue{operand}}}, totalAdvance, false, nil
	}

	if len(outArr) == 0 {
		return nil, totalAdvance, true, nil
	}

	// Compact adjacent strings in outArr.
	outArr = compactTJArray(outArr)

	if len(outArr) == 0 {
		return nil, totalAdvance, true, nil
	}

	return []contentOp{{Operator: "TJ", Operands: []pdfValue{outArr}}}, totalAdvance, true, nil
}

// compactTJArray merges adjacent string elements and removes zero-advance kerning entries
// at the start or end of the array to produce a cleaner output.
func compactTJArray(arr pdfArray) pdfArray {
	if len(arr) == 0 {
		return arr
	}

	var result pdfArray
	i := 0
	for i < len(arr) {
		switch v := arr[i].(type) {
		case string:
			// Collect consecutive strings.
			combined := v
			i++
			for i < len(arr) {
				if s2, ok := arr[i].(string); ok {
					combined += s2
					i++
				} else {
					break
				}
			}
			if combined != "" {
				result = append(result, combined)
			}
		case float64:
			// Only add non-zero kerning.
			if v != 0 {
				result = append(result, v)
			}
			i++
		case int:
			if v != 0 {
				result = append(result, v)
			}
			i++
		default:
			result = append(result, arr[i])
			i++
		}
	}

	// Remove leading/trailing kerning-only entries (they shift nothing visible).
	for len(result) > 0 {
		switch result[0].(type) {
		case float64, int:
			result = result[1:]
		default:
			goto doneLeading
		}
	}
doneLeading:
	for len(result) > 0 {
		switch result[len(result)-1].(type) {
		case float64, int:
			result = result[:len(result)-1]
		default:
			goto doneTrailing
		}
	}
doneTrailing:

	return result
}

// pointInAnyQuad reports whether (x, y) lies inside the axis-aligned
// bounding box of any quad in regions. Acceptable for the MVP since
// redact quads are typically rectangular.
func pointInAnyQuad(x, y float64, regions []QuadPoint) bool {
	for _, q := range regions {
		minX := minF(minF(q.X1, q.X2), minF(q.X3, q.X4))
		maxX := maxF(maxF(q.X1, q.X2), maxF(q.X3, q.X4))
		minY := minF(minF(q.Y1, q.Y2), minF(q.Y3, q.Y4))
		maxY := maxF(maxF(q.Y1, q.Y2), maxF(q.Y3, q.Y4))
		if x >= minX && x <= maxX && y >= minY && y <= maxY {
			return true
		}
	}
	return false
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
