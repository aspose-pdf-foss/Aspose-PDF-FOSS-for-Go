package asposepdf

// rewriteTextOperatorsInStream removes glyphs whose center falls inside
// any region from text-rendering operators (Tj, TJ, ', "), preserving
// the position of surviving glyphs via TJ kerning gaps.
//
// Form XObjects (Do op), inline images, and non-text operators are passed
// through unchanged. Unknown fonts (font.known == false) cause the
// affected Tj/TJ op to be passed through unchanged.
func rewriteTextOperatorsInStream(data []byte, regions []QuadPoint, fontMap map[string]fontInfo) ([]byte, error) {
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
	)

	out := make([]contentOp, 0, len(ops))

	for _, op := range ops {
		switch op.Operator {

		case "BT":
			tm = identityMatrix()
			lm = identityMatrix()
			out = append(out, op)

		case "ET":
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
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
				}
				newOps, totalAdvance := filterTjString(op.Operands[0], state, regions)
				// Advance the text matrix by total advance regardless of what survived.
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				out = append(out, newOps...)
			} else {
				out = append(out, op)
			}

		case "TJ":
			if len(op.Operands) >= 1 {
				state := glyphFilterState{
					font:       font,
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
				}
				newOps, totalAdvance := filterTJArray(op.Operands[0], state, regions)
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
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
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
				}
				newOps, totalAdvance := filterTjString(op.Operands[0], state, regions)
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
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
					fontSize:   fontSize,
					charSpace:  charSpace,
					wordSpace:  wordSpace,
					horizScale: horizScale,
					tm:         tm,
				}
				newOps, totalAdvance := filterTjString(op.Operands[2], state, regions)
				tm = matMul(translateMatrix(totalAdvance, 0), tm)
				out = append(out, newOps...)
			} else {
				out = append(out, op)
			}

		default:
			out = append(out, op)
		}
	}

	return serializeContentOps(out), nil
}

// glyphFilterState holds the current text rendering state for glyph filtering.
type glyphFilterState struct {
	font       fontInfo
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizScale float64
	tm         [6]float64
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
// that fall inside any region. Returns replacement ops and total text advance.
//
// If font.known == false, passes the op through unchanged with computed advance=0.
// If all glyphs are kept, returns the original op.
// If all glyphs are dropped, returns no ops.
// Otherwise returns a TJ op with kerning gaps for dropped glyphs.
func filterTjString(operand pdfValue, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64) {
	s, ok := operand.(string)
	if !ok {
		// Not a string operand — pass through as Tj.
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{operand}}}, 0
	}

	// Unknown font: pass through unchanged, advance unknown (return 0 so tm isn't moved).
	if !state.font.known {
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{operand}}}, 0
	}

	if state.font.isType0 {
		return filterTjStringType0(s, state, regions)
	}
	return filterTjStringSingleByte(s, state, regions)
}

// filterTjStringSingleByte handles single-byte fonts.
func filterTjStringSingleByte(s string, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64) {
	// Collect per-glyph info and decide which to drop.
	runningX := 0.0
	glyphs := make([]singleByteGlyph, len(s))
	totalAdvance := 0.0

	for i := 0; i < len(s); i++ {
		code := s[i]
		adv := glyphAdvance(state.font, code, state.fontSize, state.charSpace, state.wordSpace, state.horizScale)

		// Glyph center in text space: (runningX + adv/2, 0).
		cx := runningX + adv/2.0
		ux, uy := matApplyPoint(state.tm, cx, 0)
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
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{s}}}, totalAdvance
	}

	if droppedCount == len(glyphs) {
		// All dropped: emit nothing.
		return nil, totalAdvance
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
		return nil, totalAdvance
	}
	return []contentOp{{Operator: "TJ", Operands: []pdfValue{arr}}}, totalAdvance
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
func filterTjStringType0(s string, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64) {
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
		ux, uy := matApplyPoint(state.tm, cx, 0)
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
		return []contentOp{{Operator: "Tj", Operands: []pdfValue{s}}}, totalAdvance
	}
	if droppedCount == len(glyphs) {
		return nil, totalAdvance
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
		return nil, totalAdvance
	}
	return []contentOp{{Operator: "TJ", Operands: []pdfValue{arr}}}, totalAdvance
}

// filterTJArray processes a TJ operand (array of strings and kerning numbers).
// Returns replacement ops and total text advance accumulated across all elements.
//
// Strategy: walk the TJ array, tracking the current tm internally.
// Strings get per-glyph filtering; numeric entries are accumulated into the output
// and also applied to the running position.
func filterTJArray(operand pdfValue, state glyphFilterState, regions []QuadPoint) ([]contentOp, float64) {
	arr, ok := operand.(pdfArray)
	if !ok {
		return []contentOp{{Operator: "TJ", Operands: []pdfValue{operand}}}, 0
	}

	// Unknown font: pass through unchanged.
	if !state.font.known {
		return []contentOp{{Operator: "TJ", Operands: []pdfValue{operand}}}, 0
	}

	// We'll build a new TJ array, tracking position as we go.
	// The state.tm is the tm at the start; we maintain a running advance.
	runningAdvance := 0.0 // text-space advance so far in this TJ
	totalAdvance := 0.0   // total advance including numeric kerning

	var outArr pdfArray

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
					ux, uy := matApplyPoint(localTM, cx, 0)
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
					ux, uy := matApplyPoint(localTM, cx, 0)
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
			// TJ kerning: displacement = -N/1000 * fontSize (text space before horizScale)
			displacement := -float64(v) / 1000.0 * state.fontSize
			runningAdvance += displacement
			totalAdvance += displacement
			outArr = append(outArr, elem)

		case float64:
			displacement := -v / 1000.0 * state.fontSize
			runningAdvance += displacement
			totalAdvance += displacement
			outArr = append(outArr, elem)
		}
	}

	if len(outArr) == 0 {
		return nil, totalAdvance
	}

	// Compact adjacent strings in outArr.
	outArr = compactTJArray(outArr)

	if len(outArr) == 0 {
		return nil, totalAdvance
	}

	return []contentOp{{Operator: "TJ", Operands: []pdfValue{outArr}}}, totalAdvance
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
