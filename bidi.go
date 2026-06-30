// SPDX-License-Identifier: MIT

package asposepdf

// Unicode Bidirectional Algorithm (UAX #9), pragmatic single-paragraph
// implementation — RTL text support, phase 1 (epic pdf-go-emak / pdf-go-ohyp).
//
// This reorders a logical-order string into visual (display) order so that the
// left-to-right glyph emission in renderTextInBuilder draws Hebrew/Arabic runs
// right-to-left and keeps embedded Latin/number runs left-to-right. It runs
// entirely in the text-layout layer; the encoder and renderer are untouched.
//
// Scope (phase 1): the resolution rules W1–W7, N1–N2 and I1–I2 over a single
// isolating run sequence (the whole line, with sor/eor = the paragraph base
// direction), the L2 reordering, and L4 mirroring of paired punctuation.
// Explicit embedding/override/isolate formatting codes (LRE/RLE/LRI/…), the
// bracket-pair rule N0, and the L1 trailing-whitespace reset are out of scope —
// they are rare in the kind of content this library generates and can be added
// later. Arabic contextual shaping is phase 2 (it precedes reordering).

// bidiCls is a (reduced) Unicode bidirectional character class.
type bidiCls uint8

const (
	clsL   bidiCls = iota // Left-to-Right (strong)
	clsR                  // Right-to-Left (strong, Hebrew)
	clsAL                 // Right-to-Left Arabic (strong)
	clsEN                 // European Number
	clsES                 // European Separator (+ -)
	clsET                 // European Terminator (% $ … °)
	clsAN                 // Arabic Number
	clsCS                 // Common Separator (, . / :)
	clsNSM                // Nonspacing Mark
	clsWS                 // Whitespace
	clsON                 // Other Neutral
)

// bidiClass returns the bidi class of r. The table is range-based, covering the
// blocks this library realistically renders (Latin, Hebrew, Arabic, the common
// separators and numbers); anything unlisted defaults to clsL.
func bidiClass(r rune) bidiCls {
	switch {
	case r == 0x0020 || r == 0x0009 || r == 0x000B || r == 0x000C || r == 0x2028:
		return clsWS
	case r >= 0x0030 && r <= 0x0039: // ASCII digits
		return clsEN
	case r == '+' || r == '-':
		return clsES
	case r == '#' || r == '$' || r == '%' || r == 0x00A2 || r == 0x00A3 ||
		r == 0x00A4 || r == 0x00A5 || r == 0x00B0 || r == 0x00B1 || r == 0x2030:
		return clsET
	case r == ',' || r == '.' || r == '/' || r == ':' || r == 0x00A0:
		return clsCS
	case r >= 0x0041 && r <= 0x005A, r >= 0x0061 && r <= 0x007A: // ASCII letters
		return clsL
	// Hebrew block + Hebrew presentation forms.
	case r >= 0x0590 && r <= 0x05FF, r >= 0xFB1D && r <= 0xFB4F:
		if r >= 0x0591 && r <= 0x05BD || r == 0x05BF || r == 0x05C1 || r == 0x05C2 ||
			r == 0x05C4 || r == 0x05C5 || r == 0x05C7 {
			return clsNSM // Hebrew points
		}
		return clsR
	// Arabic numbers.
	case r >= 0x0660 && r <= 0x0669, r >= 0x06F0 && r <= 0x06F9:
		return clsAN
	// Arabic combining marks (harakat) and similar.
	case r >= 0x064B && r <= 0x065F, r == 0x0670, r >= 0x06D6 && r <= 0x06DC,
		r >= 0x06DF && r <= 0x06E4, r >= 0x06E7 && r <= 0x06E8, r >= 0x06EA && r <= 0x06ED:
		return clsNSM
	// Arabic letters: main block, supplement, extended-A, presentation forms A/B.
	case r >= 0x0600 && r <= 0x06FF, r >= 0x0750 && r <= 0x077F,
		r >= 0x08A0 && r <= 0x08FF, r >= 0xFB50 && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF:
		return clsAL
	// General-punctuation neutrals and common symbol ranges.
	case r >= 0x0021 && r <= 0x002F, r >= 0x003A && r <= 0x0040,
		r >= 0x005B && r <= 0x0060, r >= 0x007B && r <= 0x007E,
		r >= 0x2010 && r <= 0x2027, r >= 0x2030 && r <= 0x205E:
		return clsON
	default:
		return clsL
	}
}

// bidiHasStrongRTL reports whether s contains any strong right-to-left character
// (Hebrew or Arabic). Used to skip BiDi work for plain left-to-right text.
func bidiHasStrongRTL(s string) bool {
	for _, r := range s {
		switch bidiClass(r) {
		case clsR, clsAL:
			return true
		}
	}
	return false
}

// bidiBaseLevel returns the paragraph embedding level: 1 when explicitRTL, else
// the auto level per rules P2/P3 (the first strong character's direction; 0 if
// none).
func bidiBaseLevel(s string, explicitRTL bool) int {
	if explicitRTL {
		return 1
	}
	for _, r := range s {
		switch bidiClass(r) {
		case clsL:
			return 0
		case clsR, clsAL:
			return 1
		}
	}
	return 0
}

// dirFromLevel returns the strong direction implied by an embedding level.
func dirFromLevel(level int) bidiCls {
	if level%2 == 0 {
		return clsL
	}
	return clsR
}

// bidiResolve computes the per-rune embedding level for one line (treated as a
// single isolating run sequence whose boundaries take the base direction).
func bidiResolve(runes []rune, baseLevel int) []int {
	n := len(runes)
	cls := make([]bidiCls, n)
	for i, r := range runes {
		cls[i] = bidiClass(r)
	}
	sor := dirFromLevel(baseLevel)

	// W1: NSM takes the class of the previous character (sor at the start).
	prev := sor
	for i := 0; i < n; i++ {
		if cls[i] == clsNSM {
			cls[i] = prev
		}
		prev = cls[i]
	}
	// W2: EN → AN when the last strong type is AL.
	lastStrong := sor
	for i := 0; i < n; i++ {
		switch cls[i] {
		case clsL, clsR, clsAL:
			lastStrong = cls[i]
		case clsEN:
			if lastStrong == clsAL {
				cls[i] = clsAN
			}
		}
	}
	// W3: AL → R.
	for i := 0; i < n; i++ {
		if cls[i] == clsAL {
			cls[i] = clsR
		}
	}
	// W4: a single ES between two EN → EN; a single CS between two numbers of
	// the same kind → that kind.
	for i := 1; i < n-1; i++ {
		switch cls[i] {
		case clsES:
			if cls[i-1] == clsEN && cls[i+1] == clsEN {
				cls[i] = clsEN
			}
		case clsCS:
			if cls[i-1] == clsEN && cls[i+1] == clsEN {
				cls[i] = clsEN
			} else if cls[i-1] == clsAN && cls[i+1] == clsAN {
				cls[i] = clsAN
			}
		}
	}
	// W5: a sequence of ET adjacent to EN → EN.
	for i := 0; i < n; i++ {
		if cls[i] != clsET {
			continue
		}
		j := i
		for j < n && cls[j] == clsET {
			j++
		}
		adjEN := (i > 0 && cls[i-1] == clsEN) || (j < n && cls[j] == clsEN)
		if adjEN {
			for k := i; k < j; k++ {
				cls[k] = clsEN
			}
		}
		i = j - 1
	}
	// W6: remaining ES, ET, CS → ON.
	for i := 0; i < n; i++ {
		switch cls[i] {
		case clsES, clsET, clsCS:
			cls[i] = clsON
		}
	}
	// W7: EN → L when the last strong type is L.
	lastStrong = sor
	for i := 0; i < n; i++ {
		switch cls[i] {
		case clsL, clsR:
			lastStrong = cls[i]
		case clsEN:
			if lastStrong == clsL {
				cls[i] = clsL
			}
		}
	}

	// N1/N2: resolve neutrals (ON, WS). EN and AN count as R for this purpose.
	neutralDir := func(c bidiCls) bidiCls {
		switch c {
		case clsL:
			return clsL
		case clsR, clsEN, clsAN:
			return clsR
		}
		return clsON // not a strong-ish boundary type
	}
	embedDir := dirFromLevel(baseLevel)
	for i := 0; i < n; i++ {
		if cls[i] != clsON && cls[i] != clsWS {
			continue
		}
		j := i
		for j < n && (cls[j] == clsON || cls[j] == clsWS) {
			j++
		}
		before := embedDir
		if i > 0 {
			before = neutralDir(cls[i-1])
		} else {
			before = neutralDir(sor)
		}
		after := embedDir
		if j < n {
			after = neutralDir(cls[j])
		} else {
			after = neutralDir(sor)
		}
		var resolved bidiCls
		if before == after && (before == clsL || before == clsR) {
			resolved = before // N1
		} else {
			resolved = embedDir // N2
		}
		for k := i; k < j; k++ {
			cls[k] = resolved
		}
		i = j - 1
	}

	// I1/I2: implicit levels. After the rules above every class is L, R, EN or AN.
	levels := make([]int, n)
	for i := 0; i < n; i++ {
		lvl := baseLevel
		if lvl%2 == 0 { // even (LTR) embedding
			switch cls[i] {
			case clsR:
				lvl++
			case clsEN, clsAN:
				lvl += 2
			}
		} else { // odd (RTL) embedding
			switch cls[i] {
			case clsL, clsEN, clsAN:
				lvl++
			}
		}
		levels[i] = lvl
	}
	return levels
}

// bidiReorderIndices returns the visual order of indices for the given levels
// (rule L2): from the highest level down to 1, reverse every contiguous run of
// positions whose level is at least that value.
func bidiReorderIndices(levels []int) []int {
	n := len(levels)
	order := make([]int, n)
	maxLevel := 0
	for i := 0; i < n; i++ {
		order[i] = i
		if levels[i] > maxLevel {
			maxLevel = levels[i]
		}
	}
	for lvl := maxLevel; lvl >= 1; lvl-- {
		i := 0
		for i < n {
			if levels[order[i]] < lvl {
				i++
				continue
			}
			j := i
			for j < n && levels[order[j]] >= lvl {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
	return order
}

// bidiVisualString reorders s (one line, logical order) into visual order for a
// paragraph whose base level is baseLevel, mirroring paired punctuation that
// ends up at an odd (RTL) level (rule L4).
func bidiVisualString(s string, baseLevel int) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	levels := bidiResolve(runes, baseLevel)
	order := bidiReorderIndices(levels)
	out := make([]rune, len(runes))
	for i, idx := range order {
		r := runes[idx]
		if levels[idx]%2 == 1 {
			r = bidiMirror(r)
		}
		out[i] = r
	}
	return string(out)
}

// bidiMirror returns the mirror image of a paired punctuation character (for
// characters resolved to an RTL level), or r unchanged.
func bidiMirror(r rune) rune {
	switch r {
	case '(':
		return ')'
	case ')':
		return '('
	case '[':
		return ']'
	case ']':
		return '['
	case '{':
		return '}'
	case '}':
		return '{'
	case '<':
		return '>'
	case '>':
		return '<'
	case 0x00AB: // «
		return 0x00BB
	case 0x00BB: // »
		return 0x00AB
	}
	return r
}
