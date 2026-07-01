// SPDX-License-Identifier: MIT

package asposepdf

// Arabic contextual shaping — RTL text support, phase 2 (epic pdf-go-emak).
//
// Arabic letters take up to four contextual forms (isolated / initial / medial
// / final) depending on whether they join to their neighbours. This maps each
// letter in a logical-order string to the corresponding Unicode Arabic
// Presentation Forms-B glyph (U+FE70–U+FEFF), and forms the mandatory lam-alef
// ligatures. It runs before BiDi reordering (shapeArabic is called on the
// logical string in renderTextInBuilder), so a font that covers Presentation
// Forms-B renders connected Arabic without any change to the encoder or
// renderer. Vowel marks (harakat) are transparent to joining and pass through.
//
// Scope: the basic Arabic block letters + lam-alef ligatures. Not covered:
// Presentation Forms-A ligatures, mark repositioning (GPOS), and Urdu/Farsi
// extended letters whose initial/medial forms live outside Forms-B — those need
// the OpenType shaper (phase 3).

// arabicForms holds the contextual forms of one Arabic letter (0 = the form does
// not exist for this letter) plus its joining behaviour.
type arabicForms struct {
	iso, fin, ini, med rune
	joinsLeft          bool // can connect to the following letter (its left side)
	joinsRight         bool // can connect to the preceding letter (its right side)
}

func dualForm(iso, fin, ini, med rune) arabicForms {
	return arabicForms{iso, fin, ini, med, true, true}
}

func rightForm(iso, fin rune) arabicForms {
	return arabicForms{iso: iso, fin: fin, joinsRight: true}
}

func noJoinForm(iso rune) arabicForms {
	return arabicForms{iso: iso}
}

// arabicShapeTable maps each base Arabic letter to its contextual forms.
var arabicShapeTable = map[rune]arabicForms{
	0x0621: noJoinForm(0xFE80),                           // HAMZA
	0x0622: rightForm(0xFE81, 0xFE82),                    // ALEF MADDA
	0x0623: rightForm(0xFE83, 0xFE84),                    // ALEF HAMZA ABOVE
	0x0624: rightForm(0xFE85, 0xFE86),                    // WAW HAMZA
	0x0625: rightForm(0xFE87, 0xFE88),                    // ALEF HAMZA BELOW
	0x0626: dualForm(0xFE89, 0xFE8A, 0xFE8B, 0xFE8C),     // YEH HAMZA
	0x0627: rightForm(0xFE8D, 0xFE8E),                    // ALEF
	0x0628: dualForm(0xFE8F, 0xFE90, 0xFE91, 0xFE92),     // BEH
	0x0629: rightForm(0xFE93, 0xFE94),                    // TEH MARBUTA
	0x062A: dualForm(0xFE95, 0xFE96, 0xFE97, 0xFE98),     // TEH
	0x062B: dualForm(0xFE99, 0xFE9A, 0xFE9B, 0xFE9C),     // THEH
	0x062C: dualForm(0xFE9D, 0xFE9E, 0xFE9F, 0xFEA0),     // JEEM
	0x062D: dualForm(0xFEA1, 0xFEA2, 0xFEA3, 0xFEA4),     // HAH
	0x062E: dualForm(0xFEA5, 0xFEA6, 0xFEA7, 0xFEA8),     // KHAH
	0x062F: rightForm(0xFEA9, 0xFEAA),                    // DAL
	0x0630: rightForm(0xFEAB, 0xFEAC),                    // THAL
	0x0631: rightForm(0xFEAD, 0xFEAE),                    // REH
	0x0632: rightForm(0xFEAF, 0xFEB0),                    // ZAIN
	0x0633: dualForm(0xFEB1, 0xFEB2, 0xFEB3, 0xFEB4),     // SEEN
	0x0634: dualForm(0xFEB5, 0xFEB6, 0xFEB7, 0xFEB8),     // SHEEN
	0x0635: dualForm(0xFEB9, 0xFEBA, 0xFEBB, 0xFEBC),     // SAD
	0x0636: dualForm(0xFEBD, 0xFEBE, 0xFEBF, 0xFEC0),     // DAD
	0x0637: dualForm(0xFEC1, 0xFEC2, 0xFEC3, 0xFEC4),     // TAH
	0x0638: dualForm(0xFEC5, 0xFEC6, 0xFEC7, 0xFEC8),     // ZAH
	0x0639: dualForm(0xFEC9, 0xFECA, 0xFECB, 0xFECC),     // AIN
	0x063A: dualForm(0xFECD, 0xFECE, 0xFECF, 0xFED0),     // GHAIN
	0x0641: dualForm(0xFED1, 0xFED2, 0xFED3, 0xFED4),     // FEH
	0x0642: dualForm(0xFED5, 0xFED6, 0xFED7, 0xFED8),     // QAF
	0x0643: dualForm(0xFED9, 0xFEDA, 0xFEDB, 0xFEDC),     // KAF
	0x0644: dualForm(0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0),     // LAM
	0x0645: dualForm(0xFEE1, 0xFEE2, 0xFEE3, 0xFEE4),     // MEEM
	0x0646: dualForm(0xFEE5, 0xFEE6, 0xFEE7, 0xFEE8),     // NOON
	0x0647: dualForm(0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC),     // HEH
	0x0648: rightForm(0xFEED, 0xFEEE),                    // WAW
	0x0649: rightForm(0xFEEF, 0xFEF0),                    // ALEF MAKSURA (treated right-joining)
	0x064A: dualForm(0xFEF1, 0xFEF2, 0xFEF3, 0xFEF4),     // YEH
	0x0640: {0x0640, 0x0640, 0x0640, 0x0640, true, true}, // TATWEEL (join-causing)
}

// lamAlefLigature maps the alef that follows a lam to the {isolated, final}
// lam-alef ligature glyphs.
var lamAlefLigature = map[rune][2]rune{
	0x0622: {0xFEF5, 0xFEF6}, // LAM + ALEF MADDA
	0x0623: {0xFEF7, 0xFEF8}, // LAM + ALEF HAMZA ABOVE
	0x0625: {0xFEF9, 0xFEFA}, // LAM + ALEF HAMZA BELOW
	0x0627: {0xFEFB, 0xFEFC}, // LAM + ALEF
}

// bidiHasArabic reports whether s contains any Arabic letter that shaping
// applies to.
func bidiHasArabic(s string) bool {
	for _, r := range s {
		if bidiClass(r) == clsAL {
			return true
		}
	}
	return false
}

// shapeArabic replaces the Arabic letters in s (logical order) with their
// contextual Presentation Forms-B glyphs and forms lam-alef ligatures. Non-Arabic
// runs and combining marks pass through unchanged.
func shapeArabic(s string) string {
	if !bidiHasArabic(s) {
		return s
	}
	runes := []rune(s)
	n := len(runes)
	skip := make([]bool, n)
	out := make([]rune, 0, n)

	for i := 0; i < n; i++ {
		if skip[i] {
			continue
		}
		f, ok := arabicShapeTable[runes[i]]
		if !ok {
			out = append(out, runes[i])
			continue
		}

		// Lam-alef ligature: a LAM immediately followed (past any marks) by an
		// alef variant is one glyph.
		if runes[i] == 0x0644 {
			if nj := nextLetterIndex(runes, i); nj >= 0 {
				if lig, isAlef := lamAlefLigature[runes[nj]]; isAlef {
					pf, pok := prevLetter(runes, i)
					prevJoin := pok && pf.joinsLeft
					glyph := lig[0] // isolated
					if prevJoin {
						glyph = lig[1] // final
					}
					out = append(out, glyph)
					skip[nj] = true
					continue
				}
			}
		}

		pf, pok := prevLetter(runes, i)
		nf, nok := nextLetter(runes, i)
		prevJoin := pok && pf.joinsLeft && f.joinsRight
		nextJoin := nok && nf.joinsRight && f.joinsLeft

		var glyph rune
		switch {
		case prevJoin && nextJoin:
			glyph = pickForm(f.med, f.fin, f.iso)
		case prevJoin:
			glyph = pickForm(f.fin, f.iso)
		case nextJoin:
			glyph = pickForm(f.ini, f.iso)
		default:
			glyph = f.iso
		}
		out = append(out, glyph)
	}
	return string(out)
}

// pickForm returns the first non-zero form (the fallbacks make a missing
// medial fall back to final, and a missing initial to isolated).
func pickForm(forms ...rune) rune {
	for _, f := range forms {
		if f != 0 {
			return f
		}
	}
	return 0
}

// prevLetter returns the joining info of the nearest preceding shapeable letter
// (skipping transparent marks); ok is false if the neighbour is not a shapeable
// letter (e.g. a space or Latin), which breaks the join.
func prevLetter(runes []rune, i int) (arabicForms, bool) {
	for j := i - 1; j >= 0; j-- {
		if bidiClass(runes[j]) == clsNSM {
			continue
		}
		f, ok := arabicShapeTable[runes[j]]
		return f, ok
	}
	return arabicForms{}, false
}

// nextLetter is prevLetter's forward counterpart.
func nextLetter(runes []rune, i int) (arabicForms, bool) {
	for j := i + 1; j < len(runes); j++ {
		if bidiClass(runes[j]) == clsNSM {
			continue
		}
		f, ok := arabicShapeTable[runes[j]]
		return f, ok
	}
	return arabicForms{}, false
}

// nextLetterIndex returns the index of the nearest following non-mark rune, or
// -1 if none.
func nextLetterIndex(runes []rune, i int) int {
	for j := i + 1; j < len(runes); j++ {
		if bidiClass(runes[j]) == clsNSM {
			continue
		}
		return j
	}
	return -1
}
