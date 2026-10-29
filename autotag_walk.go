// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// Content-stream side of auto-tagging (autotag.go): walk a page's top-level
// operators, record what each painting operator draws and where, and — once
// every glyph, image and form has an owner — write the operators back with
// marked-content sections around them. See
// docs/superpowers/specs/2026-10-29-auto-tagging-design.md.

// Owners of painted content. Element owners are indices (>= 0) into the
// page's leaf list.
const (
	atOwnerNone     = -2 // paints nothing, or lies inside an existing /Artifact
	atOwnerArtifact = -1
)

type atKind int

const (
	atState   atKind = iota // paints nothing; rides inside an open section
	atBarrier               // closes any open section: BT ET q Q BDC BMC EMC
	atText                  // Tj TJ ' "
	atImage                 // image Do, BI
	atForm                  // Form XObject Do
	atVector                // a painted path or sh
)

// atTok is one element of a text-showing operator: a glyph (its encoded
// bytes and the page-space point it is tested at) or a TJ kerning number.
type atTok struct {
	glyph bool
	raw   string
	kern  pdfValue
	x, y  float64
}

// atItem is one unit of the rewritten stream.
type atItem struct {
	kind atKind
	ops  []contentOp // the original operator(s): a path's construction + paint ops, or one op

	// atText
	textOp string
	toks   []atTok
	pre    []contentOp // line move for ' and ", emitted ahead of split pieces
	// kernUnit is how far, in page units along the baseline, a TJ number
	// of -1 moves the glyphs after it (fontSize·Th·|baseline scale|/1000);
	// horizontal reports a baseline along the page's x axis. Used by
	// ReplaceText's ShiftRestOfLine to move glyphs with kerning numbers.
	kernUnit   float64
	horizontal bool

	// atImage / atForm: page-space placement
	minX, minY, maxX, maxY float64
	formHasImage           bool

	inArtifact bool // inside an existing /Artifact section: left untouched

	// Assigned by the caller before emission.
	owner  int   // non-text items
	owners []int // atText: one per glyph token, in order
}

// atWalk parses a page's content and returns its items.
func atWalk(data []byte, objects map[int]*pdfObject, resources pdfDict) ([]atItem, error) {
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}
	fonts := newFontInterner(resolveFontResources(objects, resources))

	var (
		ctm   = identityMatrix()
		tm    = identityMatrix()
		lm    = identityMatrix()
		gs    textGState
		stack []textGState
		// existing marked content, one entry per open section
		mc   []atMC
		path []contentOp
	)
	gs.font = emptyFontInfo
	gs.horizScale = 1
	gs.ctm = ctm

	inArtifact := func() bool { return len(mc) > 0 && mc[len(mc)-1].artifact }
	var items []atItem
	add := func(it atItem) {
		it.inArtifact = inArtifact()
		items = append(items, it)
	}
	state := func(op contentOp) { add(atItem{kind: atState, ops: []contentOp{op}}) }

	showText := func(op contentOp, str pdfValue, isTJ bool) atItem {
		it := atItem{kind: atText, textOp: op.Operator, ops: []contentOp{op}}
		trm := matMul(tm, ctm)
		it.kernUnit = gs.fontSize * gs.horizScale * math.Hypot(trm[0], trm[1]) / 1000
		it.horizontal = math.Abs(trm[1]) < 1e-9 && trm[0] > 0
		place := func(s string) {
			it.toks = append(it.toks, atTextGlyphs(s, &gs, &tm)...)
		}
		if isTJ {
			arr, _ := str.(pdfArray)
			for _, el := range arr {
				switch v := el.(type) {
				case string:
					place(v)
				default:
					n := operandFloat(v)
					tm = matMul(translateMatrix(-n/1000*gs.fontSize*gs.horizScale, 0), tm)
					it.toks = append(it.toks, atTok{kern: el})
				}
			}
		} else if s, ok := str.(string); ok {
			place(s)
		}
		return it
	}

	for _, op := range ops {
		switch op.Operator {
		case "BT":
			tm, lm = identityMatrix(), identityMatrix()
			add(atItem{kind: atBarrier, ops: []contentOp{op}})
		case "ET":
			add(atItem{kind: atBarrier, ops: []contentOp{op}})
		case "q":
			g := gs
			g.ctm = ctm
			stack = append(stack, g)
			add(atItem{kind: atBarrier, ops: []contentOp{op}})
		case "Q":
			if n := len(stack); n > 0 {
				gs = stack[n-1]
				ctm = gs.ctm
				stack = stack[:n-1]
			}
			add(atItem{kind: atBarrier, ops: []contentOp{op}})
		case "BDC", "BMC":
			art := inArtifact()
			if len(op.Operands) > 0 && operandName(op.Operands[0]) == "/Artifact" {
				art = true
			}
			drop := false
			if op.Operator == "BDC" {
				op, drop = atStripMCID(op)
			}
			mc = append(mc, atMC{artifact: art, dropped: drop})
			if !drop {
				add(atItem{kind: atBarrier, ops: []contentOp{op}})
			}
		case "EMC":
			n := len(mc)
			if n == 0 || !mc[n-1].dropped {
				add(atItem{kind: atBarrier, ops: []contentOp{op}})
			}
			if n > 0 {
				mc = mc[:n-1]
			}
		case "cm":
			if m, ok := readCMMatrix(op.Operands); ok {
				ctm = matMul(m, ctm)
			}
			state(op)
		case "Tf":
			if len(op.Operands) >= 2 {
				gs.fontName = operandName(op.Operands[0])
				gs.font = fonts.get(gs.fontName)
				gs.fontSize = operandFloat(op.Operands[1])
			}
			state(op)
		case "Tc":
			if len(op.Operands) >= 1 {
				gs.charSpace = operandFloat(op.Operands[0])
			}
			state(op)
		case "Tw":
			if len(op.Operands) >= 1 {
				gs.wordSpace = operandFloat(op.Operands[0])
			}
			state(op)
		case "Tz":
			if len(op.Operands) >= 1 {
				gs.horizScale = operandFloat(op.Operands[0]) / 100
			}
			state(op)
		case "TL":
			if len(op.Operands) >= 1 {
				gs.leading = operandFloat(op.Operands[0])
			}
			state(op)
		case "Ts":
			if len(op.Operands) >= 1 {
				gs.rise = operandFloat(op.Operands[0])
			}
			state(op)
		case "Td", "TD":
			if len(op.Operands) >= 2 {
				tx, ty := operandFloat(op.Operands[0]), operandFloat(op.Operands[1])
				if op.Operator == "TD" {
					gs.leading = -ty
				}
				lm = matMul(translateMatrix(tx, ty), lm)
				tm = lm
			}
			state(op)
		case "T*":
			lm = matMul(translateMatrix(0, -gs.leading), lm)
			tm = lm
			state(op)
		case "Tm":
			if len(op.Operands) >= 6 {
				for i := 0; i < 6; i++ {
					tm[i] = operandFloat(op.Operands[i])
				}
				lm = tm
			}
			state(op)
		case "Tj", "TJ":
			if len(op.Operands) < 1 {
				state(op)
				continue
			}
			gs.ctm = ctm
			add(showText(op, op.Operands[0], op.Operator == "TJ"))
		case "'":
			lm = matMul(translateMatrix(0, -gs.leading), lm)
			tm = lm
			if len(op.Operands) < 1 {
				state(op)
				continue
			}
			gs.ctm = ctm
			it := showText(op, op.Operands[0], false)
			it.pre = []contentOp{{Operator: "T*"}}
			add(it)
		case "\"":
			if len(op.Operands) < 3 {
				state(op)
				continue
			}
			gs.wordSpace = operandFloat(op.Operands[0])
			gs.charSpace = operandFloat(op.Operands[1])
			lm = matMul(translateMatrix(0, -gs.leading), lm)
			tm = lm
			gs.ctm = ctm
			it := showText(op, op.Operands[2], false)
			it.pre = []contentOp{
				{Operator: "Tw", Operands: []pdfValue{op.Operands[0]}},
				{Operator: "Tc", Operands: []pdfValue{op.Operands[1]}},
				{Operator: "T*"},
			}
			add(it)
		case "m", "l", "c", "v", "y", "h", "re", "W", "W*":
			path = append(path, op)
		case "n":
			path = append(path, op)
			add(atItem{kind: atState, ops: path})
			path = nil
		case "f", "F", "f*", "S", "s", "B", "B*", "b", "b*":
			path = append(path, op)
			add(atItem{kind: atVector, ops: path})
			path = nil
		case "sh":
			add(atItem{kind: atVector, ops: []contentOp{op}})
		case "BI":
			it := atItem{kind: atImage, ops: []contentOp{op}}
			it.minX, it.minY, it.maxX, it.maxY = transformedBBox(ctm, 0, 0, 1, 1)
			add(it)
		case "Do":
			name := ""
			if len(op.Operands) > 0 {
				name = operandName(op.Operands[0])
			}
			st := resolveXObjectStream(objects, resources, name)
			if st == nil {
				state(op)
				continue
			}
			if dictGetName(st.Dict, "/Subtype") == "/Form" {
				it := atItem{kind: atForm, ops: []contentOp{op}}
				if x0, y0, x1, y1, ok := formPlacementBbox(objects, st, ctm); ok {
					it.minX, it.minY, it.maxX, it.maxY = x0, y0, x1, y1
				}
				it.formHasImage = atFormHasImage(objects, st)
				add(it)
				continue
			}
			it := atItem{kind: atImage, ops: []contentOp{op}}
			it.minX, it.minY, it.maxX, it.maxY = transformedBBox(ctm, 0, 0, 1, 1)
			add(it)
		default:
			state(op)
		}
	}
	if len(path) > 0 {
		add(atItem{kind: atState, ops: path})
	}
	return items, nil
}

// atTextGlyphs splits one shown string into glyph tokens, each with the
// page-space point redaction tests a glyph at (the centre of its advance on
// the baseline), and advances tm past them. A font whose widths are unknown
// yields one token for the whole string, placed at its start.
func atTextGlyphs(s string, gs *textGState, tm *[6]float64) []atTok {
	trm := func(x float64) (float64, float64) {
		return matApplyPoint(matMul(*tm, gs.ctm), x, gs.rise)
	}
	if !gs.font.known || len(s) == 0 {
		x, y := trm(0)
		adv := 0.0
		if gs.font.isType0 {
			adv = float64(len(s)/2) * (500.0/1000*gs.fontSize + gs.charSpace) * gs.horizScale
		} else {
			for i := 0; i < len(s); i++ {
				adv += glyphAdvance(gs.font, s[i], gs.fontSize, gs.charSpace, gs.wordSpace, gs.horizScale)
			}
		}
		*tm = matMul(translateMatrix(adv, 0), *tm)
		if len(s) == 0 {
			return nil
		}
		return []atTok{{glyph: true, raw: s, x: x, y: y}}
	}
	var toks []atTok
	run := 0.0
	emit := func(raw string, adv float64) {
		x, y := trm(run + adv/2)
		toks = append(toks, atTok{glyph: true, raw: raw, x: x, y: y})
		run += adv
	}
	if gs.font.isType0 {
		b := []byte(s)
		if cm := gs.font.cidCMap; cm != nil {
			for len(b) > 0 {
				_, cid, n := cm.next(b)
				if n <= 0 {
					n = 1
				}
				emit(string(b[:n]), glyphAdvanceCID(gs.font, cid, n, gs.fontSize, gs.charSpace, gs.wordSpace, gs.horizScale))
				b = b[n:]
			}
		} else {
			for i := 0; i+1 < len(b); i += 2 {
				cid := uint16(b[i])<<8 | uint16(b[i+1])
				emit(string(b[i:i+2]), glyphAdvanceCID(gs.font, cid, 2, gs.fontSize, gs.charSpace, gs.wordSpace, gs.horizScale))
			}
		}
	} else {
		for i := 0; i < len(s); i++ {
			emit(s[i:i+1], glyphAdvance(gs.font, s[i], gs.fontSize, gs.charSpace, gs.wordSpace, gs.horizScale))
		}
	}
	*tm = matMul(translateMatrix(run, 0), *tm)
	return toks
}

// atMC is one open marked-content section of the original stream.
type atMC struct {
	artifact bool // an /Artifact section, or inside one
	dropped  bool // a stale MCID wrapper removed together with its EMC
}

// atStripMCID removes a stale /MCID from an existing BDC's inline property
// list: the document has no structure tree (AutoTag refuses one that does),
// so such an MCID points nowhere, and keeping it would nest one MCID inside
// another. A wrapper left with no properties is dropped (drop = true; its
// EMC must go too) — it carried nothing but the dead MCID.
func atStripMCID(op contentOp) (out contentOp, drop bool) {
	if len(op.Operands) < 2 {
		return op, false
	}
	props, ok := op.Operands[1].(pdfDict)
	if !ok {
		return op, false
	}
	if _, has := props["/MCID"]; !has {
		return op, false
	}
	rest := make(pdfDict, len(props))
	for k, v := range props {
		if k != "/MCID" {
			rest[k] = v
		}
	}
	if len(rest) == 0 {
		return op, true
	}
	return contentOp{Operator: "BDC", Operands: []pdfValue{op.Operands[0], rest}}, false
}

// atFormHasImage reports whether a Form XObject's own resources name an
// Image XObject (a shallow check: a logo placed as a form).
func atFormHasImage(objects map[int]*pdfObject, st *pdfStream) bool {
	res, ok := resolveRefToDict(objects, st.Dict["/Resources"])
	if !ok {
		return false
	}
	xobjs, ok := resolveRefToDict(objects, res["/XObject"])
	if !ok {
		return false
	}
	for _, v := range xobjs {
		if s, ok := resolveRef(objects, v).(*pdfStream); ok && dictGetName(s.Dict, "/Subtype") == "/Image" {
			return true
		}
	}
	return false
}

// transformedBBox maps the rectangle (x0,y0)-(x1,y1) through m and returns
// the bounding box of the result.
func transformedBBox(m [6]float64, x0, y0, x1, y1 float64) (minX, minY, maxX, maxY float64) {
	pts := [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}}
	for i, p := range pts {
		x, y := matApplyPoint(m, p[0], p[1])
		if i == 0 || x < minX {
			minX = x
		}
		if i == 0 || x > maxX {
			maxX = x
		}
		if i == 0 || y < minY {
			minY = y
		}
		if i == 0 || y > maxY {
			maxY = y
		}
	}
	return
}

// atPiece is one rewritten operator with its owner.
type atPiece struct {
	owner int
	op    contentOp
}

// atPieces turns an item, with its owner(s) assigned, into owned operators —
// splitting a text operator at owner boundaries. Splitting is exact: a
// show operator advances the text matrix by exactly the glyphs (and TJ
// numbers) it shows, so consecutive pieces draw what the whole did.
func atPieces(it atItem) []atPiece {
	single := func(owner int) []atPiece {
		out := make([]atPiece, 0, len(it.ops))
		for _, op := range it.ops {
			out = append(out, atPiece{owner, op})
		}
		return out
	}
	switch {
	case it.inArtifact, it.kind == atState, it.kind == atBarrier:
		return single(atOwnerNone)
	case it.kind != atText:
		return single(it.owner)
	}

	// Text: group tokens into runs of one owner. A kerning number belongs
	// to the run of the glyph before it (a leading one to the first run).
	type run struct {
		owner int
		toks  []atTok
	}
	var runs []run
	var pending []atTok
	gi := 0
	for _, t := range it.toks {
		if !t.glyph {
			if len(runs) == 0 {
				pending = append(pending, t)
			} else {
				runs[len(runs)-1].toks = append(runs[len(runs)-1].toks, t)
			}
			continue
		}
		o := it.owners[gi]
		gi++
		if len(runs) == 0 || runs[len(runs)-1].owner != o {
			runs = append(runs, run{owner: o})
			if len(runs) == 1 {
				runs[0].toks = append(runs[0].toks, pending...)
				pending = nil
			}
		}
		runs[len(runs)-1].toks = append(runs[len(runs)-1].toks, t)
	}
	switch len(runs) {
	case 0:
		return single(atOwnerNone) // shows nothing (empty string, numbers only)
	case 1:
		return single(runs[0].owner)
	}

	var out []atPiece
	for _, op := range it.pre {
		out = append(out, atPiece{atOwnerNone, op})
	}
	for _, r := range runs {
		if it.textOp == "TJ" {
			var arr pdfArray
			for _, t := range r.toks {
				if !t.glyph {
					arr = append(arr, t.kern)
					continue
				}
				if n := len(arr); n > 0 {
					if s, ok := arr[n-1].(string); ok {
						arr[n-1] = s + t.raw
						continue
					}
				}
				arr = append(arr, t.raw)
			}
			out = append(out, atPiece{r.owner, contentOp{Operator: "TJ", Operands: []pdfValue{arr}}})
			continue
		}
		s := ""
		for _, t := range r.toks {
			s += t.raw
		}
		out = append(out, atPiece{r.owner, contentOp{Operator: "Tj", Operands: []pdfValue{s}}})
	}
	return out
}

// atEmit writes the owned operators back with marked-content sections.
// Consecutive operators of one owner share a section; a section is closed
// before any barrier operator (BT ET q Q BDC BMC EMC), so the new sections
// always nest correctly with text objects, saved graphics states and the
// stream's existing marked content. Operators that paint nothing travel
// inside whatever section is open. mcid is called once per new element
// section and returns that section's MCID and structure type.
func atEmit(items []atItem, mcid func(owner int) (int, StructType)) []contentOp {
	var out []contentOp
	open := atOwnerNone
	closeOpen := func() {
		if open != atOwnerNone {
			out = append(out, contentOp{Operator: "EMC"})
			open = atOwnerNone
		}
	}
	for _, it := range items {
		if it.kind == atBarrier {
			closeOpen()
			out = append(out, it.ops...)
			continue
		}
		for _, p := range atPieces(it) {
			if p.owner == atOwnerNone {
				out = append(out, p.op)
				continue
			}
			if p.owner != open {
				closeOpen()
				if p.owner == atOwnerArtifact {
					out = append(out, contentOp{Operator: "BMC", Operands: []pdfValue{pdfName("/Artifact")}})
				} else {
					id, typ := mcid(p.owner)
					out = append(out, contentOp{Operator: "BDC", Operands: []pdfValue{pdfName(string(typ)), pdfDict{"/MCID": id}}})
				}
				open = p.owner
			}
			out = append(out, p.op)
		}
	}
	closeOpen()
	return out
}
