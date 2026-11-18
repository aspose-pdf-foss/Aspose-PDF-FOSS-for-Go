// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// ApplyTextChanges draws the edits made to text fragments onto their pages:
// fragments found by a TextFragmentAbsorber or drawn by a TextBuilder whose
// Text, Position, TextState or segments changed since. The old glyphs are
// removed by position (as ReplaceText removes a match) and the fragment is
// drawn again from its current state: the whole fragment when its text,
// position, own state or segments changed, else only the segments whose
// state changed, each at its place. Unchanged fragments cost nothing: they
// are compared in place, without allocating.
//
// Every writer (WriteTo, Save, WriteToLinearized, WriteIncremental, the
// exporters, the renderers, and the operations building documents from
// pages: Split, Extract, Append, NUp, Booklet, NewPdfPageStamp, ImportForm)
// calls it first, so edits reach the output without calling it; call it to
// read the page back before saving, and before rendering pages from several
// goroutines (applying changes the document). Applying is idempotent. An
// incremental signature (an already signed document, SignOptions.Incremental)
// cannot carry pending edits: Save refuses then. Mirrors the effect of
// setting TextFragment.Text or TextFragment.TextState in Aspose.PDF for .NET,
// which changes the page at once.
//
// Setting Text on a found fragment replaces its segments with one in the
// first segment's state; a change to the fragment's TextState applies field
// by field to every segment. A changed fragment that cannot be edited (on a
// rotated baseline, a match across pages, a line of a paragraph, text on a
// form canvas, a page whose content was rewritten as a whole after the
// fragment was found) makes the call fail, naming its page and text, before
// any page changes; so does one whose old glyphs the content rewriter cannot
// place (a font with unknown glyph positions on the page) or whose new text
// no font covers: a font found on the page is replaced by the Standard-14
// face of its family, else that family by name, as ReplaceText chooses.
// Edited right-to-left text keeps its right edge unless Position changed.
// A fragment drawn by a TextBuilder takes the background box and link it
// was drawn with along when redrawn; a found fragment's background and links
// are the page's and stay. A document carrying a revision appended in memory
// (AddValidationInfo) is written as it is, so edits to it are refused too.
func (d *Document) ApplyTextChanges() error {
	var plans []*textEditPlan
	var fonts *replaceFonts
	// Planning may embed a fallback font; a refusal takes it out again, so
	// a refused call leaves the document as it was.
	planNext, planFonts := d.nextID, len(d.embeddedFonts)
	refuse := func(err error) error {
		for id := range d.objects {
			if id >= planNext {
				delete(d.objects, id)
			}
		}
		d.nextID = planNext
		d.embeddedFonts = d.embeddedFonts[:planFonts]
		return err
	}
	for _, f := range d.boundFragments {
		if b := f.bind; b == nil || b.page == nil || b.snap.matches(f, b.found) {
			continue
		}
		if fonts == nil {
			fonts = newReplaceFonts(nil)
		}
		pl, err := d.planTextEdit(f, fonts)
		if err != nil {
			return refuse(err)
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
	pageNames := func(idx int) string {
		names := make([]string, len(byPage[idx]))
		for i, pl := range byPage[idx] {
			names[i] = pl.name
		}
		return strings.Join(names, ", ")
	}

	// Text the content rewriter cannot place (a font with unknown glyph
	// positions) is refused here, before any page changes: the page-level
	// rewrite is run on the page's content without keeping the result.
	pageRegions := map[int][]QuadPoint{}
	for _, idx := range order {
		var regions []QuadPoint
		for _, pl := range byPage[idx] {
			regions = append(regions, pl.regions...)
		}
		pageRegions[idx] = regions
		p, err := d.Page(idx + 1)
		if err != nil {
			return refuse(fmt.Errorf("apply text changes: page %d: %w", idx+1, err))
		}
		if err := checkTextRemovable(p, regions); err != nil {
			return refuse(fmt.Errorf("apply text changes: page %d: %s: cannot be edited: %w", idx+1, pageNames(idx), err))
		}
	}

	tx := d.beginTextEdit(order)
	// A rollback also takes out what planning loaded (a fallback font).
	tx.nextID, tx.fonts = planNext, planFonts
	var old []int
	for _, idx := range order {
		p, err := d.Page(idx + 1)
		if err != nil {
			tx.rollback()
			return fmt.Errorf("apply text changes: page %d: %w", idx+1, err)
		}
		bgs := map[int]bool{}
		for _, pl := range byPage[idx] {
			pl.p = p
			for _, own := range pl.removed {
				if own.bg != 0 {
					bgs[own.bg] = true
				}
			}
		}
		old = append(old, contentStreamIDs(p.pageDict()["/Contents"])...)
		old = append(old, formGraphRefs(d.objects, p.pageResources())...)
		// Every region on the page is stripped before anything is drawn,
		// so one fragment's new text is not removed with another's old.
		data, err := stripTextInRegions(p, pageRegions[idx])
		if err == nil {
			data, err = removeTextBackgrounds(data, bgs)
		}
		if err == nil {
			err = replacePageContents(p, data)
		}
		if err != nil {
			tx.rollback()
			return fmt.Errorf("apply text changes: page %d: %s: remove the old text: %w", idx+1, pageNames(idx), err)
		}
		for _, pl := range byPage[idx] {
			if err := pl.draw(p); err != nil {
				tx.rollback()
				return pl.errorf("draw: %w", err)
			}
		}
	}
	for _, pl := range plans {
		pl.commit()
	}
	// The other fragments bound on an edited page keep valid regions: only
	// the edited fragments' glyphs moved. Carry them to the new generation.
	for _, s := range tx.pages {
		now := s.obj.contentGen
		for _, f := range d.boundFragments {
			if b := f.bind; b != nil && b.obj == s.obj && b.gen == s.gen {
				b.gen = now
			}
		}
	}
	// A form a live XForm handle names may be placed again: never deleted.
	keep := d.liveFormGraph()
	kept := old[:0]
	for _, id := range old {
		if !keep[id] {
			kept = append(kept, id)
		}
	}
	d.dropUnreachable(kept)
	return nil
}

// textChangesPending reports whether a bound fragment changed since it was
// drawn, found or last applied.
func (d *Document) textChangesPending() bool {
	for _, f := range d.boundFragments {
		if b := f.bind; b != nil && b.page != nil && !b.snap.matches(f, b.found) {
			return true
		}
	}
	return false
}

// checkTextRemovable reports an error when the content rewriter refuses the
// page's text (a font whose glyph positions are unknown), without changing
// the page.
func checkTextRemovable(p *Page, regions []QuadPoint) error {
	data, err := p.contentStreams()
	if err != nil {
		return err
	}
	res := p.pageResources()
	fontMap := resolveFontResources(p.doc.objects, res)
	if _, err := rewriteTextOperatorsInStream(data, regions, fontMap, p.doc.objects, res, nil); err != nil {
		return fmt.Errorf("the positions of the page's glyphs are unknown: %w", err)
	}
	return nil
}

// textEditTx remembers what drawing changes on the pages an edit touches,
// so a failure part way through leaves the document as it was.
type textEditTx struct {
	d      *Document
	nextID int
	fonts  int // len(d.embeddedFonts) to restore
	pages  []savedPageKeys
}

type savedPageKeys struct {
	obj  *pdfObject
	gen  int
	dict pdfDict
	vals map[string]pdfValue
	has  map[string]bool
}

var textEditPageKeys = []string{"/Contents", "/Resources", "/Annots"}

func (d *Document) beginTextEdit(pages []int) *textEditTx {
	tx := &textEditTx{d: d, nextID: d.nextID, fonts: len(d.embeddedFonts)}
	for _, idx := range pages {
		o := d.pages[idx]
		pd, ok := o.Value.(pdfDict)
		if !ok {
			continue
		}
		s := savedPageKeys{obj: o, gen: o.contentGen, dict: pd, vals: map[string]pdfValue{}, has: map[string]bool{}}
		for _, k := range textEditPageKeys {
			v, ok := pd[k]
			s.vals[k], s.has[k] = v, ok
		}
		tx.pages = append(tx.pages, s)
	}
	return tx
}

// rollback restores the page keys and content generations, deletes every
// object created since and forgets the cached page views (their content and
// annotation caches).
func (tx *textEditTx) rollback() {
	for _, s := range tx.pages {
		for _, k := range textEditPageKeys {
			if s.has[k] {
				s.dict[k] = s.vals[k]
			} else {
				delete(s.dict, k)
			}
		}
		s.obj.contentGen = s.gen
	}
	for id := range tx.d.objects {
		if id >= tx.nextID {
			delete(tx.d.objects, id)
		}
	}
	tx.d.nextID = tx.nextID
	tx.d.embeddedFonts = tx.d.embeddedFonts[:tx.fonts]
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
	page    int    // index in d.pages
	name    string // `page N: fragment "old" (new text "new")`, for errors
	regions []QuadPoint

	// whole redraws the fragment from tmp; else segs lists the segments
	// redrawn, each from its own one-segment fragment.
	whole bool
	tmp   *TextFragment
	segs  []int
	one   []*TextFragment

	drawnWhole []QuadPoint
	drawnSeg   [][]QuadPoint
	ownWhole   []ownedPiece
	ownSeg     [][]ownedPiece

	// removed is what the old drawing owned besides its glyphs (background
	// boxes, links), removed with it; p is the page drawn on.
	removed []ownedPiece
	p       *Page
}

func (pl *textEditPlan) errorf(format string, args ...any) error {
	return fmt.Errorf("apply text changes: page %d: %s: %w", pl.page+1, pl.name, fmt.Errorf(format, args...))
}

// fragmentName names a fragment in errors by the text it was found or drawn
// with, and its new text when that changed.
func fragmentName(f *TextFragment) string {
	if old := f.bind.snap.text; f.Text != old {
		return fmt.Sprintf("fragment %q (new text %q)", old, f.Text)
	}
	return fmt.Sprintf("fragment %q", f.bind.snap.text)
}

// planTextEdit returns the plan for a changed fragment f, nil when f is
// unchanged or its page is gone, or an error when f cannot be edited.
// Nothing is changed on the page.
func (d *Document) planTextEdit(f *TextFragment, fonts *replaceFonts) (*textEditPlan, error) {
	b := f.bind
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

	idx := -1
	for i, o := range d.pages {
		if o == b.obj {
			idx = i
			break
		}
	}
	pl := &textEditPlan{f: f, page: idx, whole: whole, name: fragmentName(f)}
	switch {
	case b.page.obj != nil:
		return nil, fmt.Errorf("apply text changes: %s: drawn on a form canvas, not a page", pl.name)
	case idx < 0:
		return nil, nil // its page was removed from the document
	case b.noEdit != "":
		return nil, pl.errorf("cannot be edited: %s", b.noEdit)
	case b.gen != b.obj.contentGen:
		return nil, pl.errorf("cannot be edited: page content was rewritten after the fragment was found")
	case d.revisionAppended:
		return nil, pl.errorf("cannot be edited: the document carries a revision appended in memory (AddValidationInfo) and is written as it is; save it, reopen and edit")
	}

	perSegment := len(b.drawn) == len(snap.segments)
	if !whole && !perSegment {
		pl.whole = true
	}

	// segState is segment i's state with the edits applied: for a found
	// fragment its own explicit state plus every field of the fragment's
	// state that changed; for an authored one its own (inheriting) state.
	// Always a copy: no colour is shared with the caller's fragment.
	segState := func(s *TextSegment) TextState {
		if found {
			return copyState(applyStateDiff(s.TextState, snap.state, f.TextState))
		}
		return copyState(s.TextState)
	}

	owned := len(b.owned) == len(b.drawn)
	if pl.whole {
		pl.regions = append(pl.regions, b.drawn...)
		if owned {
			pl.removed = append(pl.removed, b.owned...)
		}
		tmp := &TextFragment{Position: f.Position}
		if !found {
			tmp.TextState = copyState(f.TextState)
		}
		switch {
		case textChanged && len(f.Segments) > 0:
			tmp.Segments = []*TextSegment{{Text: f.Text, TextState: segState(f.Segments[0]), Hyperlink: f.Segments[0].Hyperlink}}
		case textChanged || len(f.Segments) == 0:
			if found {
				// No segments left (an emptied fragment): the fragment's
				// own state is its first segment's.
				tmp.Segments = []*TextSegment{{Text: f.Text, TextState: copyState(f.TextState)}}
			} else {
				tmp.Text = f.Text
			}
		default:
			for _, s := range f.Segments {
				tmp.Segments = append(tmp.Segments, &TextSegment{Text: s.Text, TextState: segState(s), Hyperlink: s.Hyperlink})
			}
		}
		if err := d.prepareDrawing(tmp, found, fonts); err != nil {
			return nil, pl.errorf("%w", err)
		}
		if found && f.Position == snap.position && bidiBaseLevel(snap.text, false) == 1 {
			// Right-to-left text keeps its right edge on its line.
			if err := keepRightEdge(tmp, b.drawn, snap); err != nil {
				return nil, pl.errorf("%w", err)
			}
		}
		pl.tmp = tmp
		return pl, nil
	}

	for _, i := range changedSegs {
		s := f.Segments[i]
		pos := snap.segments[i].pos
		pl.regions = append(pl.regions, b.drawn[i])
		if owned {
			pl.removed = append(pl.removed, b.owned[i])
		}
		one := &TextFragment{Position: pos,
			Segments: []*TextSegment{{Text: s.Text, TextState: segState(s), Hyperlink: s.Hyperlink}}}
		if !found {
			one.TextState = copyState(f.TextState)
		}
		if err := d.prepareDrawing(one, found, fonts); err != nil {
			return nil, pl.errorf("%w", err)
		}
		pl.segs = append(pl.segs, i)
		pl.one = append(pl.one, one)
	}
	return pl, nil
}

// keepRightEdge moves tmp's start so its text ends where the old text's
// first line ended (the right edge of the quads drawn on the first baseline).
func keepRightEdge(tmp *TextFragment, drawn []QuadPoint, snap fragmentSnapshot) error {
	right, ok := 0.0, false
	for i, q := range drawn {
		if i < len(snap.segments) && snap.segments[i].pos.YIndent != snap.position.YIndent {
			continue
		}
		if r := quadBounds(q).URX; !ok || r > right {
			right, ok = r, true
		}
	}
	if !ok {
		return nil
	}
	w := 0.0
	for _, s := range tmp.segmentsToDraw() {
		sw, err := s.TextState.MeasureString(s.Text)
		if err != nil {
			return err
		}
		w += sw
	}
	tmp.Position.XIndent = right - w
	return nil
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
		drawn, owned, err := drawFragment(p, pl.tmp)
		if err != nil {
			return err
		}
		pl.drawnWhole, pl.ownWhole = drawn, owned
		return nil
	}
	for _, one := range pl.one {
		if one.Segments[0].Text == "" {
			pl.drawnSeg = append(pl.drawnSeg, nil)
			pl.ownSeg = append(pl.ownSeg, nil)
			continue
		}
		drawn, owned, err := drawFragment(p, one)
		if err != nil {
			return err
		}
		pl.drawnSeg = append(pl.drawnSeg, drawn)
		pl.ownSeg = append(pl.ownSeg, owned)
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

// commit records the drawn result on the fragment, removes the links the
// old drawing made and refreshes the snapshot, so a later apply finds nothing
// changed.
func (pl *textEditPlan) commit() {
	f, b := pl.f, pl.f.bind
	for _, own := range pl.removed {
		if own.link != nil && pl.p != nil {
			pl.p.Annotations().Delete(own.link)
		}
	}
	if pl.whole {
		f.Segments = pl.tmp.Segments
		if pl.drawnWhole != nil {
			f.Text = pl.tmp.Text
			f.Position = pl.tmp.Position
		}
		b.drawn = pl.drawnWhole
		if !b.found {
			b.owned = pl.ownWhole
		}
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
			if !b.found && len(b.owned) == len(b.drawn) {
				b.owned[i] = ownedPiece{}
				if o := pl.ownSeg[k]; len(o) == 1 {
					b.owned[i] = o[0]
				}
			}
		}
	}
	if b.found {
		refreshValueFields(f)
	}
	b.snap = snapshotOf(f)
}

// refreshValueFields sets a found fragment's value fields (those
// ExtractTextWithLayout fills) to what was drawn: its first segment's
// font, size and colour, and the extent of its first line.
func refreshValueFields(f *TextFragment) {
	if len(f.Segments) == 0 {
		f.Text, f.Width = "", 0
		return
	}
	var all strings.Builder
	for _, s := range f.Segments {
		all.WriteString(s.Text)
	}
	f.Text = all.String()
	s0 := f.Segments[0]
	st := s0.TextState.resolved()
	f.Y = s0.Position.YIndent
	f.FontSize = st.FontSize
	if st.Font != nil {
		f.FontName = styledFont(st.Font, st.FontStyle).BaseFont()
	}
	if c := st.ForegroundColor; c != nil {
		f.Color = *c
	}
	f.Bold, f.Italic = st.FontStyle&FontStyleBold != 0, st.FontStyle&FontStyleItalic != 0
	left, right := s0.Rectangle.LLX, s0.Rectangle.URX
	for _, s := range f.Segments[1:] {
		if s.Position.YIndent != s0.Position.YIndent {
			continue
		}
		left, right = math.Min(left, s.Rectangle.LLX), math.Max(right, s.Rectangle.URX)
	}
	f.X, f.Width = left, right-left
}
