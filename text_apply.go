// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// ApplyTextChanges draws the edits made to text fragments onto their pages:
// fragments found by a TextFragmentAbsorber or drawn by a TextBuilder whose
// Text, Position, TextState or segments changed since. The old glyphs are
// removed by position (as ReplaceText removes a match) and the fragment is
// drawn again from its current state: the whole fragment when its text,
// position, own state or segments changed, else only the segments whose
// state changed, each at its place. Unchanged fragments cost nothing.
//
// Every writer (WriteTo, Save, WriteToLinearized, WriteIncremental, the
// exporters and the renderers) calls it first, so edits reach the output
// without calling it; call it to read the page back before saving. Applying
// is idempotent. Mirrors the effect of setting TextFragment.Text or
// TextFragment.TextState in Aspose.PDF for .NET, which changes the page at
// once.
//
// Setting Text on a found fragment replaces its segments with one in the
// first segment's state; a change to the fragment's TextState applies field
// by field to every segment. A changed fragment that cannot be edited (on a
// rotated baseline, a match across pages, a line of a paragraph, text on a
// form canvas) makes the call fail, naming it, before any page changes; so
// does one whose old glyphs the content rewriter cannot place (a font with
// unknown widths), in which case the pages are restored as they were.
func (d *Document) ApplyTextChanges() error {
	if len(d.boundFragments) == 0 {
		return nil
	}
	fonts := newReplaceFonts(nil)
	var plans []*textEditPlan
	for _, f := range d.boundFragments {
		pl, err := d.planTextEdit(f, fonts)
		if err != nil {
			return err
		}
		if pl != nil {
			plans = append(plans, pl)
		}
	}
	if len(plans) == 0 {
		return nil
	}

	// Group by page, keeping the order fragments were bound in.
	var order []int
	byPage := map[int][]*textEditPlan{}
	for _, pl := range plans {
		if _, ok := byPage[pl.page]; !ok {
			order = append(order, pl.page)
		}
		byPage[pl.page] = append(byPage[pl.page], pl)
	}

	tx := d.beginTextEdit(order)
	var old []int
	for _, idx := range order {
		p, err := d.Page(idx + 1)
		if err != nil {
			tx.rollback()
			return fmt.Errorf("apply text changes: %w", err)
		}
		var regions []QuadPoint
		for _, pl := range byPage[idx] {
			regions = append(regions, pl.regions...)
		}
		old = append(old, contentStreamIDs(p.pageDict()["/Contents"])...)
		old = append(old, xobjectRefs(d.objects, p.pageResources())...)
		// Every region on the page is stripped before anything is drawn,
		// so one fragment's new text is not removed with another's old.
		data, err := stripTextInRegions(p, regions)
		if err == nil {
			err = replacePageContents(p, data)
		}
		if err != nil {
			tx.rollback()
			return fmt.Errorf("apply text changes: page %d: %w", idx+1, err)
		}
		for _, pl := range byPage[idx] {
			if err := pl.draw(p); err != nil {
				tx.rollback()
				return fmt.Errorf("apply text changes: fragment %q: %w", pl.f.Text, err)
			}
		}
	}
	for _, pl := range plans {
		pl.commit()
	}
	d.dropUnreachable(old)
	return nil
}

// xobjectRefs returns the objects a resources dictionary's /XObject entries
// name: forms the strip may clone, left unreachable when every placement was.
func xobjectRefs(objects map[int]*pdfObject, res pdfDict) []int {
	xo, ok := resolveRefToDict(objects, res["/XObject"])
	if !ok {
		return nil
	}
	var ids []int
	for _, v := range xo {
		if r, ok := v.(pdfRef); ok {
			ids = append(ids, r.Num)
		}
	}
	return ids
}

// textEditTx remembers what drawing changes on the pages an edit touches,
// so a failure part way through leaves the document as it was.
type textEditTx struct {
	d      *Document
	nextID int
	pages  []savedPageKeys
}

type savedPageKeys struct {
	dict pdfDict
	vals map[string]pdfValue // absent key: nil value with present=false
	has  map[string]bool
}

var textEditPageKeys = []string{"/Contents", "/Resources", "/Annots"}

func (d *Document) beginTextEdit(pages []int) *textEditTx {
	tx := &textEditTx{d: d, nextID: d.nextID}
	for _, idx := range pages {
		pd, ok := d.pages[idx].Value.(pdfDict)
		if !ok {
			continue
		}
		s := savedPageKeys{dict: pd, vals: map[string]pdfValue{}, has: map[string]bool{}}
		for _, k := range textEditPageKeys {
			v, ok := pd[k]
			s.vals[k], s.has[k] = v, ok
		}
		tx.pages = append(tx.pages, s)
	}
	return tx
}

// rollback restores the page keys, deletes every object created since and
// forgets the cached page views (their content and annotation caches).
func (tx *textEditTx) rollback() {
	for _, s := range tx.pages {
		for _, k := range textEditPageKeys {
			if s.has[k] {
				s.dict[k] = s.vals[k]
			} else {
				delete(s.dict, k)
			}
		}
	}
	for id := range tx.d.objects {
		if id >= tx.nextID {
			delete(tx.d.objects, id)
		}
	}
	tx.d.nextID = tx.nextID
	for _, p := range tx.d.pageCache {
		if p != nil {
			p.contentsBounded = false
			p.annotations = nil
		}
	}
}

// textEditPlan is one changed fragment: what to remove and what to draw.
type textEditPlan struct {
	f       *TextFragment
	page    int // index in d.pages
	regions []QuadPoint

	// whole redraws the fragment from tmp; else segs lists the segments
	// redrawn, each from its own one-segment fragment.
	whole bool
	tmp   *TextFragment
	segs  []int
	one   []*TextFragment

	drawnWhole []QuadPoint
	drawnSeg   [][]QuadPoint
}

// planTextEdit returns the plan for f, nil when f is unchanged, or an error
// when f changed and cannot be edited. Nothing is changed on the page.
func (d *Document) planTextEdit(f *TextFragment, fonts *replaceFonts) (*textEditPlan, error) {
	b := f.bind
	if b == nil || b.page == nil {
		return nil, nil
	}
	snap := b.snap
	found := b.found
	cur := snapshotOf(f)
	textChanged := f.Text != snap.text
	whole := textChanged || f.Position != snap.position || !sameState(f.TextState, snap.state) ||
		len(cur.segments) != len(snap.segments)
	if !whole {
		for i := range cur.segments {
			if cur.segments[i].text != snap.segments[i].text || cur.segments[i].link != snap.segments[i].link {
				whole = true
				break
			}
		}
	}
	var changedSegs []int
	if !whole {
		for i := range cur.segments {
			if !sameState(cur.segments[i].state, snap.segments[i].state) {
				changedSegs = append(changedSegs, i)
			}
		}
		if len(changedSegs) == 0 {
			return nil, nil
		}
	}

	if b.noEdit != "" {
		return nil, fmt.Errorf("apply text changes: fragment %q cannot be edited: %s", snap.text, b.noEdit)
	}
	if b.page.obj != nil {
		return nil, fmt.Errorf("apply text changes: fragment %q is on a form canvas, not a page", snap.text)
	}
	idx := -1
	for i, o := range d.pages {
		if o == b.obj {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil // its page was removed from the document
	}

	pl := &textEditPlan{f: f, page: idx, whole: whole}
	perSegment := len(b.drawn) == len(snap.segments)
	if !whole && !perSegment {
		pl.whole = true
	}

	// segState is segment i's state with the edits applied: for a found
	// fragment its own explicit state plus every field of the fragment's
	// state that changed; for an authored one its own (inheriting) state.
	segState := func(s *TextSegment) TextState {
		if found {
			return applyStateDiff(s.TextState, snap.state, f.TextState)
		}
		return s.TextState
	}

	if pl.whole {
		pl.regions = append(pl.regions, b.drawn...)
		tmp := &TextFragment{Position: f.Position}
		if !found {
			tmp.TextState = f.TextState
		}
		switch {
		case textChanged && len(f.Segments) > 0:
			tmp.Segments = []*TextSegment{{Text: f.Text, TextState: segState(f.Segments[0]), Hyperlink: f.Segments[0].Hyperlink}}
		case textChanged || len(f.Segments) == 0:
			if found {
				// No segments left (an emptied fragment): the fragment's
				// own state is its first segment's.
				tmp.Segments = []*TextSegment{{Text: f.Text, TextState: f.TextState}}
			} else {
				tmp.Text = f.Text
			}
		default:
			for _, s := range f.Segments {
				tmp.Segments = append(tmp.Segments, &TextSegment{Text: s.Text, TextState: segState(s), Hyperlink: s.Hyperlink})
			}
		}
		if err := d.prepareDrawing(tmp, found, fonts); err != nil {
			return nil, fmt.Errorf("apply text changes: fragment %q: %w", snap.text, err)
		}
		pl.tmp = tmp
		return pl, nil
	}

	for _, i := range changedSegs {
		s := f.Segments[i]
		pos := snap.segments[i].pos
		pl.regions = append(pl.regions, b.drawn[i])
		one := &TextFragment{Position: pos,
			Segments: []*TextSegment{{Text: s.Text, TextState: segState(s), Hyperlink: s.Hyperlink}}}
		if !found {
			one.TextState = f.TextState
		}
		if err := d.prepareDrawing(one, found, fonts); err != nil {
			return nil, fmt.Errorf("apply text changes: fragment %q: %w", snap.text, err)
		}
		pl.segs = append(pl.segs, i)
		pl.one = append(pl.one, one)
	}
	return pl, nil
}

// prepareDrawing resolves the fonts tmp's segments are drawn in. Fonts found
// on the page (documentFont) are not drawable as they are; drawingFont picks
// one.
func (d *Document) prepareDrawing(tmp *TextFragment, found bool, fonts *replaceFonts) error {
	if len(tmp.Segments) == 0 {
		f, err := d.drawingFont(tmp.TextState, tmp.Text, fonts)
		if err != nil {
			return err
		}
		tmp.TextState.Font = f
		return nil
	}
	if !found {
		f, err := d.drawingFont(tmp.TextState, "", fonts)
		if err != nil {
			return err
		}
		tmp.TextState.Font = f
	}
	for _, s := range tmp.Segments {
		st := s.TextState
		if !found {
			st = st.inherit(tmp.TextState)
		}
		f, err := d.drawingFont(st, s.Text, fonts)
		if err != nil {
			return err
		}
		if s.TextState.Font != nil || found {
			s.TextState.Font = f
		}
	}
	return nil
}

// drawingFont is the font text in state st is drawn in when an edited
// fragment is redrawn: a font found on the page is replaced by the closest
// Standard-14 face of its family and style, or that family by name when the
// face lacks a character of text (as ReplaceText chooses); any other font is
// used as it is. The single place the drawing font of an edit is chosen.
func (d *Document) drawingFont(st TextState, text string, fonts *replaceFonts) (Font, error) {
	df, ok := st.Font.(documentFont)
	if !ok {
		return st.Font, nil
	}
	bold, italic := st.FontStyle&FontStyleBold != 0, st.FontStyle&FontStyleItalic != 0
	m := replaceMatch{font: standard14ForFragment(df.name, bold, italic), fontName: df.name, bold: bold, italic: italic}
	if text == "" {
		return m.font, nil
	}
	return fonts.fontFor(d, m, text)
}

// applyStateDiff returns st with every field in which now differs from was
// set to now's value.
func applyStateDiff(st, was, now TextState) TextState {
	if now.Font != was.Font {
		st.Font = now.Font
	}
	if now.FontSize != was.FontSize {
		st.FontSize = now.FontSize
	}
	if now.FontStyle != was.FontStyle {
		st.FontStyle = now.FontStyle
	}
	if !sameColor(now.ForegroundColor, was.ForegroundColor) {
		st.ForegroundColor = copyColor(now.ForegroundColor)
	}
	if !sameColor(now.BackgroundColor, was.BackgroundColor) {
		st.BackgroundColor = copyColor(now.BackgroundColor)
	}
	if !sameColor(now.StrokingColor, was.StrokingColor) {
		st.StrokingColor = copyColor(now.StrokingColor)
	}
	if now.RenderingMode != was.RenderingMode {
		st.RenderingMode = now.RenderingMode
	}
	if now.Underline != was.Underline {
		st.Underline = now.Underline
	}
	if now.StrikeOut != was.StrikeOut {
		st.StrikeOut = now.StrikeOut
	}
	if now.Invisible != was.Invisible {
		st.Invisible = now.Invisible
	}
	if now.CharacterSpacing != was.CharacterSpacing {
		st.CharacterSpacing = now.CharacterSpacing
	}
	if now.WordSpacing != was.WordSpacing {
		st.WordSpacing = now.WordSpacing
	}
	if now.HorizontalScaling != was.HorizontalScaling {
		st.HorizontalScaling = now.HorizontalScaling
	}
	if now.LineSpacing != was.LineSpacing {
		st.LineSpacing = now.LineSpacing
	}
	if now.Rotation != was.Rotation {
		st.Rotation = now.Rotation
	}
	return st
}

// draw draws the plan's text on p (the page's old glyphs already removed).
// Segments with no text draw nothing: an emptied fragment is a deletion.
func (pl *textEditPlan) draw(p *Page) error {
	if pl.whole {
		dropEmptySegments(pl.tmp)
		if pl.tmp.Text == "" && len(pl.tmp.Segments) == 0 {
			return nil
		}
		drawn, err := drawFragment(p, pl.tmp)
		if err != nil {
			return err
		}
		pl.drawnWhole = drawn
		return nil
	}
	for _, one := range pl.one {
		if one.Segments[0].Text == "" {
			pl.drawnSeg = append(pl.drawnSeg, nil)
			continue
		}
		drawn, err := drawFragment(p, one)
		if err != nil {
			return err
		}
		pl.drawnSeg = append(pl.drawnSeg, drawn)
	}
	return nil
}

func dropEmptySegments(f *TextFragment) {
	if len(f.Segments) == 0 {
		return
	}
	kept := f.Segments[:0]
	for _, s := range f.Segments {
		if s.Text != "" {
			kept = append(kept, s)
		}
	}
	f.Segments = kept
}

// commit records the drawn result on the fragment and refreshes its
// snapshot, so a later apply finds nothing changed.
func (pl *textEditPlan) commit() {
	f, b := pl.f, pl.f.bind
	if pl.whole {
		f.Segments = pl.tmp.Segments
		if pl.drawnWhole != nil {
			f.Text = pl.tmp.Text
			f.Position = pl.tmp.Position
		}
		b.drawn = pl.drawnWhole
	} else {
		for k, i := range pl.segs {
			s := f.Segments[i]
			one := pl.one[k].Segments[0]
			if b.found {
				s.TextState = one.TextState
			}
			s.Position, s.Rectangle = one.Position, one.Rectangle
			if q := pl.drawnSeg[k]; len(q) == 1 {
				b.drawn[i] = q[0]
			}
		}
	}
	b.snap = snapshotOf(f)
}
