// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// TextSegment is a run of text with one TextState inside a TextFragment.
// Mirrors Aspose.PDF for .NET's TextSegment. Position and Rectangle are filled
// for segments that were drawn or found on a page.
//
// A Position set on a segment (other than the first, which starts at the
// fragment's Position) places it there, an absolute baseline start like the
// fragment's; the segments after it without one follow on from its end. On a
// drawn or found fragment, changing a segment's Position moves that segment
// when the change is applied (the first one moves the fragment's start). In
// Aspose.PDF for .NET a segment's position moves with the fragment's; here
// it stays where it was set.
type TextSegment struct {
	Text      string
	TextState TextState
	Hyperlink string
	Position  Position
	Rectangle Rectangle

	// glyphX, on a found segment, is where each character of Text starts
	// on the page (NaN for a space extraction inserted), so a redraw that
	// keeps the text and its metrics puts every glyph back in its place.
	glyphX []float64
}

// NewTextSegment returns a segment holding text.
func NewTextSegment(text string) *TextSegment { return &TextSegment{Text: text} }

// NewTextFragment returns a fragment with one segment holding text. Mirrors
// Aspose.PDF for .NET's new TextFragment(text).
func NewTextFragment(text string) *TextFragment {
	return &TextFragment{Text: text, Segments: []*TextSegment{NewTextSegment(text)}}
}

// segmentsToDraw returns the segments to draw: Segments when set, else one
// segment made from Text. Each carries its state inheriting the fragment's.
func (f *TextFragment) segmentsToDraw() []*TextSegment {
	src := f.Segments
	if len(src) == 0 {
		src = []*TextSegment{{Text: f.Text}}
	}
	out := make([]*TextSegment, len(src))
	for i, s := range src {
		c := *s
		c.TextState = s.TextState.inherit(f.TextState)
		out[i] = &c
	}
	return out
}

// TextBuilder appends text to a page. Mirrors Aspose.PDF for .NET's
// TextBuilder.
type TextBuilder struct{ page *Page }

// NewTextBuilder returns a builder drawing on p.
func NewTextBuilder(p *Page) *TextBuilder { return &TextBuilder{page: p} }

// fragmentBinding ties a fragment to the page it was drawn on or found on,
// with a snapshot of how it looked then, so later edits can be detected.
type fragmentBinding struct {
	page  *Page
	drawn []QuadPoint
	snap  fragmentSnapshot
	// noEdit, when set, is why edits to this fragment must be refused (a
	// found fragment on a rotated baseline).
	noEdit string
	// found marks a fragment the absorber read from the page: its segments
	// carry their own explicit state, which does not inherit the fragment's.
	found bool
	// obj is the page dictionary the fragment is on, so the page is found
	// again after pages were reordered.
	obj *pdfObject
	// gen is the page's content generation when the fragment was bound.
	gen int
	// placed[i] reports whether segment i was drawn at a Position given for
	// it rather than following on from the segment before it, so a recorded
	// position is told from a chosen one when the fragment is redrawn.
	placed []bool
	// owned, for a fragment drawn by AppendText (one entry per drawn
	// quad), is what was drawn with each segment besides its glyphs: the
	// background box and the link. A redraw removes them. A found
	// fragment owns nothing: its background and links are the page's.
	owned []ownedPiece
}

// ownedPiece is what a drawn segment drew besides its glyphs: its background
// box (the id of its marked-content section, 0 for none) and its link.
type ownedPiece struct {
	bg   int
	link *LinkAnnotation
}

type segmentSnapshot struct {
	text  string
	link  string
	state TextState
	pos   Position
}

// fragmentSnapshot records a fragment's text, position, own state and each
// segment's state (resolved for authored fragments, as found for found
// ones). Colours are copied by value, fonts kept by identity.
type fragmentSnapshot struct {
	text     string
	position Position
	state    TextState
	segments []segmentSnapshot
}

func copyColor(c *Color) *Color {
	if c == nil {
		return nil
	}
	v := *c
	return &v
}

// copyState returns st with its colours copied, so later edits through the
// fragment's colour pointers are seen as changes.
func copyState(st TextState) TextState {
	st.ForegroundColor = copyColor(st.ForegroundColor)
	st.BackgroundColor = copyColor(st.BackgroundColor)
	st.StrokingColor = copyColor(st.StrokingColor)
	return st
}

func snapshotOf(f *TextFragment) fragmentSnapshot {
	snap := fragmentSnapshot{text: f.Text, position: f.Position, state: copyState(f.TextState)}
	if f.bind != nil && f.bind.found {
		for _, s := range f.Segments {
			snap.segments = append(snap.segments, segmentSnapshot{text: s.Text, link: s.Hyperlink,
				state: copyState(s.TextState), pos: s.Position})
		}
		return snap
	}
	for i, s := range f.segmentsToDraw() {
		seg := segmentSnapshot{text: s.Text, link: s.Hyperlink, state: copyState(s.TextState.resolved())}
		if i < len(f.Segments) {
			seg.pos = f.Segments[i].Position
		}
		snap.segments = append(snap.segments, seg)
	}
	return snap
}

func sameColor(a, b *Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameState(a, b TextState) bool {
	return a.Font == b.Font && a.FontSize == b.FontSize && a.FontStyle == b.FontStyle &&
		sameColor(a.ForegroundColor, b.ForegroundColor) &&
		sameColor(a.BackgroundColor, b.BackgroundColor) &&
		sameColor(a.StrokingColor, b.StrokingColor) &&
		a.RenderingMode == b.RenderingMode && a.Underline == b.Underline &&
		a.StrikeOut == b.StrikeOut && a.Invisible == b.Invisible &&
		a.CharacterSpacing == b.CharacterSpacing && a.WordSpacing == b.WordSpacing &&
		a.HorizontalScaling == b.HorizontalScaling && a.LineSpacing == b.LineSpacing &&
		a.Rotation == b.Rotation
}

// compareBlack is the default colour resolvedForCompare uses: read only.
var compareBlack = Color{0, 0, 0, 1}

// resolvedForCompare is resolved() without allocating, for comparison only:
// the result shares compareBlack and must not be stored.
func resolvedForCompare(s TextState) TextState {
	return s.inherit(TextState{Font: FontHelvetica, FontSize: 12, ForegroundColor: &compareBlack, HorizontalScaling: 100})
}

// matches reports, without allocating, whether f still looks as s recorded
// it (snapshotOf(f) would equal s). found selects how segments are recorded.
func (s *fragmentSnapshot) matches(f *TextFragment, found bool) bool {
	if f.Text != s.text || f.Position != s.position || !sameState(f.TextState, s.state) {
		return false
	}
	if found {
		if len(f.Segments) != len(s.segments) {
			return false
		}
		for i, seg := range f.Segments {
			sn := &s.segments[i]
			if seg.Text != sn.text || seg.Hyperlink != sn.link || seg.Position != sn.pos || !sameState(seg.TextState, sn.state) {
				return false
			}
		}
		return true
	}
	if len(f.Segments) == 0 {
		return len(s.segments) == 1 && s.segments[0].text == f.Text && s.segments[0].link == "" &&
			sameState(resolvedForCompare(f.TextState), s.segments[0].state)
	}
	if len(f.Segments) != len(s.segments) {
		return false
	}
	for i, seg := range f.Segments {
		sn := &s.segments[i]
		if seg.Text != sn.text || seg.Hyperlink != sn.link || seg.Position != sn.pos ||
			!sameState(resolvedForCompare(seg.TextState.inherit(f.TextState)), sn.state) {
			return false
		}
	}
	return true
}

// equal reports whether two snapshots describe the same fragment.
func (s fragmentSnapshot) equal(o fragmentSnapshot) bool {
	if s.text != o.text || s.position != o.position || len(s.segments) != len(o.segments) ||
		!sameState(s.state, o.state) {
		return false
	}
	for i := range s.segments {
		a, b := s.segments[i], o.segments[i]
		if a.text != b.text || a.link != b.link || a.pos != b.pos || !sameState(a.state, b.state) {
			return false
		}
	}
	return true
}

// bindFragment remembers f so edits to it can be applied on save.
// Binding the same fragment again (its text appended twice) keeps one entry.
func (d *Document) bindFragment(f *TextFragment) {
	if f.bind != nil && f.bind.page != nil {
		f.bind.obj = f.bind.page.pageObj()
		if f.bind.obj != nil {
			f.bind.gen = f.bind.obj.contentGen
		}
	}
	if _, ok := d.boundSet[f]; ok {
		return
	}
	if d.boundSet == nil {
		d.boundSet = map[*TextFragment]struct{}{}
	}
	d.boundSet[f] = struct{}{}
	d.boundFragments = append(d.boundFragments, f)
}

// textBox returns how far the text box of font at size reaches above and below
// the baseline (descent <= 0), consistent with the boxes SearchText reports.
func textBox(font Font, size float64) (asc, desc float64, err error) {
	if font == nil {
		font = FontHelvetica
	}
	if !font.IsEmbedded() {
		if a, d, ok := standard14VerticalMetrics("/" + font.BaseFont()); ok {
			return a * size / 1000, d * size / 1000, nil
		}
	}
	m, err := GetFontMetrics(font, size)
	if err != nil {
		return 0, 0, err
	}
	d := m.Descent
	if d == 0 {
		d = -0.2 * size
	}
	return m.Ascent, d, nil
}

// rotateQuad rotates q by deg degrees counter-clockwise about (cx, cy).
func rotateQuad(q QuadPoint, cx, cy, deg float64) QuadPoint {
	sin, cos := math.Sincos(deg * math.Pi / 180)
	rot := func(px, py float64) (float64, float64) {
		dx, dy := px-cx, py-cy
		return cx + dx*cos - dy*sin, cy + dx*sin + dy*cos
	}
	q.X1, q.Y1 = rot(q.X1, q.Y1)
	q.X2, q.Y2 = rot(q.X2, q.Y2)
	q.X3, q.Y3 = rot(q.X3, q.Y3)
	q.X4, q.Y4 = rot(q.X4, q.Y4)
	return q
}

// quadBounds returns the bounding rectangle of q.
func quadBounds(q QuadPoint) Rectangle {
	xs := []float64{q.X1, q.X2, q.X3, q.X4}
	ys := []float64{q.Y1, q.Y2, q.Y3, q.Y4}
	r := Rectangle{LLX: xs[0], URX: xs[0], LLY: ys[0], URY: ys[0]}
	for i := 1; i < 4; i++ {
		r.LLX, r.URX = math.Min(r.LLX, xs[i]), math.Max(r.URX, xs[i])
		r.LLY, r.URY = math.Min(r.LLY, ys[i]), math.Max(r.URY, ys[i])
	}
	return r
}

// drawTextPiece draws text at baseline (x, y): its background box, the text
// itself and, when hyperlink is set, a link over the box. It returns the box,
// its quad (rotated with the state's Rotation about (x, y)) and what it drew
// besides the glyphs. Shared by AppendText and AppendParagraph so both bind
// what they draw alike.
func drawTextPiece(p *Page, st TextState, style TextStyle, hyperlink, text string, x, y, width, asc, desc float64) (Rectangle, QuadPoint, ownedPiece, error) {
	var own ownedPiece
	box := Rectangle{LLX: x, LLY: y + desc, URX: x + width, URY: y + asc}
	quad := QuadPoint{X1: box.LLX, Y1: box.URY, X2: box.URX, Y2: box.URY, X3: box.LLX, Y3: box.LLY, X4: box.URX, Y4: box.LLY}
	if st.Rotation != 0 {
		quad = rotateQuad(quad, x, y, st.Rotation)
		box = quadBounds(quad)
	}
	if st.BackgroundColor != nil && text != "" && st.Rotation == 0 {
		id, err := p.drawTextBackground(box, st.BackgroundColor)
		if err != nil {
			return box, quad, own, err
		}
		own.bg = id
	}
	style.Background = nil // drawn above, as the piece's text box
	if err := p.AddTextAt(text, style, x, y); err != nil {
		return box, quad, own, err
	}
	if hyperlink != "" && text != "" {
		link := NewLinkAnnotation(p, box)
		link.SetAction(NewGoToURIAction(hyperlink))
		if err := p.Annotations().Add(link); err != nil {
			return box, quad, own, err
		}
		own.link = link
	}
	return box, quad, own, nil
}

type resolvedSegment struct {
	st        TextState
	style     TextStyle
	asc, desc float64
	width     float64
}

// AppendText draws the fragment's segments one after another on a baseline
// starting at the fragment's Position (the page's top-left margin when unset),
// with no wrapping. Each segment inherits the fragment's TextState.
//
// The fragment is bound to the document so a later change to it can be
// applied (ApplyTextChanges, or any save); bindings live as long as the
// document does. AppendText is not atomic: every segment is measured before
// drawing, but should drawing a segment fail (a link that cannot be added),
// the segments before it stay on the page and the fragment is not bound.
func (b *TextBuilder) AppendText(f *TextFragment) error {
	if b == nil || b.page == nil {
		return errors.New("text builder: nil page")
	}
	if f == nil {
		return errors.New("text builder: nil fragment")
	}
	place := f.explicitPositions()
	if f.Position == (Position{}) && len(place) > 0 && place[0] != (Position{}) {
		f.Position = place[0] // the first segment's Position starts the fragment
	}
	drawn, owned, err := drawFragment(b.page, f, place)
	if err != nil {
		return err
	}
	f.bind = &fragmentBinding{page: b.page, drawn: drawn, owned: owned, placed: placedFlags(place)}
	f.bind.snap = snapshotOf(f)
	b.page.doc.bindFragment(f)
	return nil
}

// explicitPositions returns, per segment of f, the Position it is to be drawn
// at, zero for one following on from the segment before it: a segment's
// Position, unless it is only the position recorded when the fragment was
// last drawn or found there (not one given for it).
func (f *TextFragment) explicitPositions() []Position {
	out := make([]Position, len(f.Segments))
	b := f.bind
	for i, s := range f.Segments {
		if s.Position == (Position{}) {
			continue
		}
		if b != nil && i < len(b.snap.segments) && s.Position == b.snap.segments[i].pos &&
			(i >= len(b.placed) || !b.placed[i]) {
			continue
		}
		out[i] = s.Position
	}
	return out
}

// placedFlags reports which segments place gives a position (the first one
// never counts: it starts the fragment).
func placedFlags(place []Position) []bool {
	flags := make([]bool, len(place))
	for i := 1; i < len(place); i++ {
		flags[i] = place[i] != (Position{})
	}
	return flags
}

// drawFragment draws f on p as AppendText does, recording the text, start
// position and segment boxes on f, and returns the quad of each segment drawn
// and what was drawn with it besides its glyphs. place[i], when set (i > 0),
// is where segment i starts instead of the end of the segment before it. It
// does not bind f (ApplyTextChanges redraws through it).
func drawFragment(p *Page, f *TextFragment, place []Position) ([]QuadPoint, []ownedPiece, error) {
	segs := f.segmentsToDraw()
	if len(segs) > 1 {
		for _, s := range segs {
			if s.TextState.Rotation != 0 {
				return nil, nil, errors.New("text builder: rotated multi-segment fragments are not supported")
			}
		}
	}

	rs := make([]resolvedSegment, len(segs))
	maxAsc := 0.0
	for i, s := range segs {
		st := s.TextState.resolved()
		style := st.style()
		asc, desc, err := textBox(style.Font, st.FontSize)
		if err != nil {
			return nil, nil, fmt.Errorf("text builder: %w", err)
		}
		w, err := st.MeasureString(s.Text)
		if err != nil {
			return nil, nil, fmt.Errorf("text builder: %w", err)
		}
		if first, last, ok := glyphSpan(s.glyphX, s.Text); ok {
			// Every glyph goes back where it was, with the character
			// spacing the glyphs were set with (so the text reads as it
			// did, without gaps taken for spaces); the segment spans from
			// its first glyph to the end of its last.
			if st.CharacterSpacing == 0 {
				st.CharacterSpacing = inferCharSpacing(st, s.Text, s.glyphX)
				style.CharSpacing = st.CharacterSpacing
			}
			tail, err := st.MeasureString(string([]rune(s.Text)[last:]))
			if err != nil {
				return nil, nil, fmt.Errorf("text builder: %w", err)
			}
			w = s.glyphX[last] - s.glyphX[first] + tail
			style.glyphX = s.glyphX
		}
		rs[i] = resolvedSegment{st, style, asc, desc, w}
		if asc > maxAsc {
			maxAsc = asc
		}
	}

	x, y := f.Position.XIndent, f.Position.YIndent
	if f.Position == (Position{}) {
		crop, err := p.CropBox()
		if err != nil {
			return nil, nil, fmt.Errorf("text builder: %w", err)
		}
		x, y = crop.LLX+72, crop.URY-72-maxAsc
	}
	startX, startY := x, y

	var drawn []QuadPoint
	var owned []ownedPiece
	var all strings.Builder
	for i, s := range segs {
		r := rs[i]
		if i > 0 && i < len(place) && place[i] != (Position{}) {
			x, y = place[i].XIndent, place[i].YIndent
		}
		if first, _, ok := glyphSpan(s.glyphX, s.Text); ok {
			x = s.glyphX[first]
		}
		box, quad, own, err := drawTextPiece(p, r.st, r.style, s.Hyperlink, s.Text, x, y, r.width, r.asc, r.desc)
		if err != nil {
			return nil, nil, fmt.Errorf("text builder: %w", err)
		}
		owned = append(owned, own)
		if i < len(f.Segments) {
			f.Segments[i].Position = Position{XIndent: x, YIndent: y}
			f.Segments[i].Rectangle = box
		}
		drawn = append(drawn, quad)
		all.WriteString(s.Text)
		x += r.width
	}

	f.Text = all.String()
	f.Position = Position{XIndent: startX, YIndent: startY}
	return drawn, owned, nil
}

// glyphSpan returns the first and last characters of text with a known
// glyph position in xs, when xs gives one position per character of text.
func glyphSpan(xs []float64, text string) (first, last int, ok bool) {
	if len(xs) == 0 || len(xs) != utf8.RuneCountInString(text) {
		return 0, 0, false
	}
	first, last = -1, -1
	for i, x := range xs {
		if math.IsNaN(x) {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	return first, last, first >= 0
}

// inferCharSpacing returns the character spacing (Tc, in unscaled text space
// units) the glyphs at xs were set with: the median, over neighbouring
// characters other than spaces, of how far the next glyph starts past the
// end of this one's advance. Zero when it is under a hundredth of a point.
func inferCharSpacing(st TextState, text string, xs []float64) float64 {
	plain := st
	plain.CharacterSpacing, plain.WordSpacing = 0, 0
	th := st.HorizontalScaling / 100
	if th == 0 {
		th = 1
	}
	runes := []rune(text)
	var gaps []float64
	for i := 0; i+1 < len(runes) && i+1 < len(xs); i++ {
		if runes[i] == ' ' || math.IsNaN(xs[i]) || math.IsNaN(xs[i+1]) {
			continue
		}
		w, err := plain.MeasureString(string(runes[i]))
		if err != nil {
			return 0
		}
		gaps = append(gaps, xs[i+1]-xs[i]-w)
	}
	if len(gaps) == 0 {
		return 0
	}
	sort.Float64s(gaps)
	g := gaps[len(gaps)/2]
	if math.Abs(g) < 0.01 {
		return 0
	}
	return g / th
}
