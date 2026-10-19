// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// rewriteTextOperatorsInStream removes glyphs whose test point falls
// inside any region from text-rendering operators (Tj, TJ, ', "),
// preserving the position of surviving glyphs via TJ kerning gaps. The
// test point is the glyph's horizontal center at its BASELINE (y=0 in
// text space, i.e. ignoring text rise — Ts has no case in this file at
// all, tracked as pdf-go-lx1f) — "center" in the rest of this file's
// comments means this,
// not the glyph's true 2D bounding-box center; a region whose y-range
// excludes the baseline (e.g. one hand-drawn tightly over just the upper
// half of a line of text) can miss a glyph that is visibly inside it.
// The documented, intended workflow — build a RedactAnnotation from a
// SearchText match's own Rect — is unaffected: SearchText's own
// Rect always spans descender-to-ascender and so always contains the
// baseline too.
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
// Known gaps in this mechanism, found by the same review round and
// deliberately deferred (bundled as pdf-go-tnsb, both low-severity):
// an unterminated BDC (a malformed/truncated stream with no matching
// EMC before end-of-stream) leaves that span's mcStack entry simply
// discarded when this function returns — its scrub decision is never
// made, but this needs a malformed input to begin with, and the rest
// of the document is unaffected; and the unknown-font refusal below
// (pdf-go-oq9i) checks fontInfo.known, which font.go's resolveFont
// sets via an exact PostScript-name match — a simple font using a
// common alias like /Arial (not literally one of the Standard-14
// names) with no /Encoding/ToUnicode is refused even though
// render_std14.go's own standard14WidthAlias table would recognize it
// as Helvetica-equivalent, so this over-refuses on a wider set of
// real documents than strictly necessary (always the safe direction —
// over-refusing, never under-redacting — but a usability regression
// worth revisiting by widening font.go's own resolution path).
func rewriteTextOperatorsInStream(data []byte, regions []QuadPoint, fontMap map[string]fontInfo, objects map[int]*pdfObject, resources pdfDict, initial *textGState) ([]byte, error) {
	if len(regions) == 0 {
		return data, nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	// Walker state mirrors textExtractor but outputs contentOps instead of fragments.
	var (
		font       fontInfo
		fontName   string // current resource name like "/F1"
		fontSize   float64
		charSpace  float64
		wordSpace  float64
		horizScale = 1.0 // Tz / 100
		leading    float64
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

				if span.dropped && span.props != nil {
					if _, hasActualText := span.props["/ActualText"]; hasActualText {
						scrubbed := make(pdfDict, len(span.props))
						for k, v := range span.props {
							if k == "/ActualText" {
								continue
							}
							scrubbed[k] = v
						}
						// Always a FRESH inline dict — even if the
						// original BDC referenced /Properties by name —
						// so a /Properties entry shared by other,
						// untouched spans is never mutated.
						out[span.outIdx] = contentOp{Operator: "BDC", Operands: []pdfValue{span.tagOperand, scrubbed}}
					}
				}
				// A span's content (including anything nested inside it)
				// being redacted means the ENCLOSING span's own
				// /ActualText, if any, is now stale too — propagate up.
				if span.dropped && len(mcStack) > 0 {
					mcStack[len(mcStack)-1].dropped = true
				}
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
				leading: leading,
			})
			out = append(out, op)

		case "Q":
			if n := len(gStack); n > 0 {
				s := gStack[n-1]
				gStack = gStack[:n-1]
				ctm, font, fontName, fontSize = s.ctm, s.font, s.fontName, s.fontSize
				charSpace, wordSpace, horizScale, leading = s.charSpace, s.wordSpace, s.horizScale, s.leading
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
				if fi, ok := fontMap[fontName]; ok {
					font = fi
				} else {
					// Unknown font: zero out so known==false is preserved.
					font = fontInfo{}
				}
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
type textGState struct {
	ctm        [6]float64
	font       fontInfo
	fontName   string
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizScale float64
	leading    float64
}

// glyphFilterState holds the current text rendering state for glyph filtering.
type glyphFilterState struct {
	font fontInfo
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
}

// glyphAdvance computes the text-space advance for a single-byte glyph code.
// Returns advance in text space units (same space as the text matrix translation).
func glyphAdvance(fi fontInfo, code byte, fontSize, charSpace, wordSpace, horizScale float64) float64 {
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

// glyphAdvanceCID computes the text-space advance for a CID glyph.
func glyphAdvanceCID(fi fontInfo, code uint16, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	w := fi.defaultW
	if cw, ok := fi.cidWidths[code]; ok {
		w = cw
	}
	if w == 0 {
		w = fi.defaultW
		if w == 0 {
			w = 500
		}
	}
	adv := (w/1000.0*fontSize + charSpace) * horizScale
	if code == 0x0020 {
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
		ux, uy := matApplyPoint(matMul(state.tm, state.ctm), cx, 0)
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

// filterTjStringType0 handles Type0 (CID) fonts where glyphs are 2 bytes each.
func filterTjStringType0(s string, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64, bool) {
	type cidGlyph struct {
		hi, lo  byte
		code    uint16
		advance float64
		dropped bool
	}

	runningX := 0.0
	var glyphs []cidGlyph
	totalAdvance := 0.0

	for i := 0; i+1 < len(s); i += 2 {
		hi := s[i]
		lo := s[i+1]
		code := uint16(hi)<<8 | uint16(lo)
		adv := glyphAdvanceCID(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)

		cx := runningX + adv/2.0
		ux, uy := matApplyPoint(matMul(state.tm, state.ctm), cx, 0)
		dropped := pointInAnyQuad(ux, uy, regions)

		glyphs = append(glyphs, cidGlyph{hi: hi, lo: lo, code: code, advance: adv, dropped: dropped})
		runningX += adv
		totalAdvance += adv
	}

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
			keptBytes = append(keptBytes, g.hi, g.lo)
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
			var cidGlyphs []struct {
				hi, lo  byte
				code    uint16
				advance float64
				dropped bool
			}

			var stringAdvance float64

			if state.font.isType0 {
				lx := 0.0
				for i := 0; i+1 < len(v); i += 2 {
					hi := v[i]
					lo := v[i+1]
					code := uint16(hi)<<8 | uint16(lo)
					adv := glyphAdvanceCID(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)
					cx := lx + adv/2.0
					ux, uy := matApplyPoint(matMul(localTM, state.ctm), cx, 0)
					dropped := pointInAnyQuad(ux, uy, regions)
					cidGlyphs = append(cidGlyphs, struct {
						hi, lo  byte
						code    uint16
						advance float64
						dropped bool
					}{hi, lo, code, adv, dropped})
					lx += adv
					stringAdvance += adv
				}
			} else {
				lx := 0.0
				for i := 0; i < len(v); i++ {
					code := v[i]
					adv := glyphAdvance(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)
					cx := lx + adv/2.0
					ux, uy := matApplyPoint(matMul(localTM, state.ctm), cx, 0)
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
						outArr = append(outArr, string([]byte{g.hi, g.lo}))
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
