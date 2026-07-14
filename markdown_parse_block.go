// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"strconv"
	"strings"
)

// CommonMark block parser (epic pdf-go-fh4l.1). A faithful port of the
// parsing strategy in the CommonMark spec appendix (and its reference
// implementation commonmark.js): a tree of open blocks; every input line
// (1) matches the continuation conditions of the open blocks, (2) tries to
// start new blocks, (3) adds its remainder as text to the deepest open block.
// Tabs are handled as 4-column tab stops without rewriting content
// (partially consumed tabs expand to spaces). GFM tables are recognized by
// the header-line + delimiter-row transformation on open paragraphs.

type mdParser struct {
	doc                  *mdBlock
	tip                  *mdBlock // deepest open block
	oldtip               *mdBlock
	currentLine          string
	lineNumber           int
	offset               int // byte offset into currentLine
	column               int // visual column (tabs = 4-column stops)
	nextNonspace         int
	nextNonspaceColumn   int
	indent               int
	indented             bool
	blank                bool
	partiallyConsumedTab bool
	allClosed            bool
	lastMatchedContainer *mdBlock
	refmap               map[string]mdLinkRef
}

// parseMarkdownBlocks runs the block phase over src and returns the document
// root plus the collected link-reference definitions.
func parseMarkdownBlocks(src string) (*mdBlock, map[string]mdLinkRef) {
	doc := &mdBlock{kind: mdDocument, open: true, startLine: 1}
	p := &mdParser{doc: doc, tip: doc, oldtip: doc, lastMatchedContainer: doc, refmap: map[string]mdLinkRef{}}

	// Normalize line endings; replace insecure NUL per spec §2.3; drop a BOM.
	src = strings.TrimPrefix(src, "\uFEFF")
	src = strings.ReplaceAll(src, "\x00", "�")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	// A trailing newline yields one phantom empty final line — drop it.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, ln := range lines {
		p.incorporateLine(ln)
	}
	for p.tip != nil {
		p.finalize(p.tip)
	}
	return doc, p.refmap
}

// --- low-level cursor helpers ------------------------------------------------

func (p *mdParser) peek(pos int) byte {
	if pos < len(p.currentLine) {
		return p.currentLine[pos]
	}
	return 0
}

func (p *mdParser) findNextNonspace() {
	i := p.offset
	col := p.column
	for i < len(p.currentLine) {
		c := p.currentLine[i]
		if c == ' ' {
			i++
			col++
		} else if c == '\t' {
			i++
			col += 4 - (col % 4)
		} else {
			break
		}
	}
	p.blank = i >= len(p.currentLine)
	p.nextNonspace = i
	p.nextNonspaceColumn = col
	p.indent = col - p.column
	p.indented = p.indent >= 4
}

func (p *mdParser) advanceNextNonspace() {
	p.offset = p.nextNonspace
	p.column = p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

// advanceOffset advances by count characters (columns=false) or visual
// columns (columns=true), splitting tabs when needed.
func (p *mdParser) advanceOffset(count int, columns bool) {
	for count > 0 && p.offset < len(p.currentLine) {
		c := p.currentLine[p.offset]
		if c == '\t' {
			charsToTab := 4 - (p.column % 4)
			if columns {
				p.partiallyConsumedTab = charsToTab > count
				charsToAdvance := charsToTab
				if charsToTab > count {
					charsToAdvance = count
				}
				p.column += charsToAdvance
				if !p.partiallyConsumedTab {
					p.offset++
				}
				count -= charsToAdvance
			} else {
				p.partiallyConsumedTab = false
				p.column += charsToTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

// restLine returns the remainder of the current line from the cursor,
// expanding a partially consumed tab into spaces. Pure — the cursor is not
// moved (the parser advances to the next line after consuming it).
func (p *mdParser) restLine() string {
	if p.partiallyConsumedTab {
		charsToTab := 4 - (p.column % 4)
		return strings.Repeat(" ", charsToTab) + p.currentLine[p.offset+1:]
	}
	return p.currentLine[p.offset:]
}

func (p *mdParser) addLine() {
	p.tip.content = append(p.tip.content, p.restLine())
}

// addChild closes blocks that cannot contain kind, then appends a new open
// block of that kind as the new tip.
func (p *mdParser) addChild(kind mdBlockKind) *mdBlock {
	for !canContain(p.tip.kind, kind) {
		p.finalize(p.tip)
	}
	child := &mdBlock{kind: kind, open: true, startLine: p.lineNumber}
	p.tip.appendChild(child)
	p.tip = child
	return child
}

func (p *mdParser) closeUnmatchedBlocks() {
	if p.allClosed {
		return
	}
	for p.oldtip != p.lastMatchedContainer {
		parent := p.oldtip.parent
		p.finalize(p.oldtip)
		p.oldtip = parent
	}
	p.allClosed = true
}

// --- main loop ----------------------------------------------------------------

func (p *mdParser) incorporateLine(ln string) {
	p.currentLine = ln
	p.lineNumber++
	p.offset = 0
	p.column = 0
	p.blank = false
	p.partiallyConsumedTab = false
	p.oldtip = p.tip

	// 1. Match continuation conditions of the open blocks.
	container := p.doc
	for {
		last := container.lastChild()
		if last == nil || !last.open {
			break
		}
		container = last
		p.findNextNonspace()
		switch p.blockContinue(container) {
		case 0: // matched
		case 1: // not matched
			container = container.parent
			goto doneMatching
		case 2: // line fully consumed (fence close)
			return
		}
	}
doneMatching:
	p.allClosed = container == p.oldtip
	p.lastMatchedContainer = container

	matchedLeaf := container.kind != mdParagraph && acceptsLines(container.kind)

	// 2. Try to start new blocks.
	for !matchedLeaf {
		p.findNextNonspace()
		// Fast path: nothing that can start a block here.
		if !p.indented && !mdMaybeSpecial(p.peek(p.nextNonspace)) {
			p.advanceNextNonspace()
			break
		}
		res := p.tryBlockStarts(container)
		if res == 0 {
			p.advanceNextNonspace()
			break
		}
		container = p.tip
		if res == 2 {
			matchedLeaf = true
		}
	}

	// 3. The remainder is text for the tip.
	if !p.allClosed && !p.blank && p.tip.kind == mdParagraph {
		// Lazy continuation.
		p.addLine()
		return
	}
	p.closeUnmatchedBlocks()

	t := container.kind
	// A blank line marks the just-closed last child (list looseness signal),
	// mirroring cmark's S_set_last_line_blank bookkeeping.
	if p.blank {
		if lc := container.lastChild(); lc != nil {
			lc.lastLineBlank = true
		}
	}
	// Blank lines don't count inside block quotes / fenced code, or on the
	// line that opens an empty list item.
	container.lastLineBlank = p.blank && !(t == mdBlockQuote || t == mdHeading || t == mdThematicBreak ||
		(t == mdCodeBlock && container.fenced) ||
		(t == mdListItem && len(container.children) == 0 && container.startLine == p.lineNumber))

	switch {
	case acceptsLines(t):
		if t == mdParagraph && p.maybeStartTable(container) {
			return // the delimiter row is consumed by the transformation
		}
		p.addLine()
		if t == mdHTMLBlock && container.htmlType >= 1 && container.htmlType <= 5 &&
			mdHTMLEndRe[container.htmlType].MatchString(p.currentLine[p.offset:]) {
			p.finalize(container)
		}
	case p.offset < len(p.currentLine) && !p.blank:
		p.addChild(mdParagraph)
		p.advanceNextNonspace()
		p.addLine()
	}
}

// mdMaybeSpecial reports whether c can begin a block construct.
func mdMaybeSpecial(c byte) bool {
	switch c {
	case '#', '`', '~', '*', '+', '_', '=', '<', '>', '-', '|', ':':
		return true
	}
	return c >= '0' && c <= '9'
}

// --- continuation conditions ---------------------------------------------------

// blockContinue: 0 = matched, 1 = failed, 2 = line consumed entirely.
func (p *mdParser) blockContinue(b *mdBlock) int {
	switch b.kind {
	case mdDocument, mdList:
		return 0
	case mdBlockQuote:
		if !p.indented && p.peek(p.nextNonspace) == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if c := p.peek(p.offset); c == ' ' || c == '\t' {
				p.advanceOffset(1, true)
			}
			return 0
		}
		return 1
	case mdListItem:
		if p.blank {
			if len(b.children) == 0 {
				return 1 // blank line after empty item ends it
			}
			p.advanceNextNonspace()
			return 0
		}
		if p.indent >= b.list.markerOffset+b.list.padding {
			p.advanceOffset(b.list.markerOffset+b.list.padding, true)
			return 0
		}
		return 1
	case mdCodeBlock:
		if b.fenced {
			// Closing fence?
			if p.indent <= 3 && p.peek(p.nextNonspace) == b.fenceChar {
				rest := p.currentLine[p.nextNonspace:]
				if m := mdClosingFenceRe.FindString(rest); m != "" &&
					m[0] == b.fenceChar && len(strings.TrimRight(m, " \t")) >= b.fenceLength {
					p.finalize(b)
					return 2
				}
			}
			// Skip up to fenceOffset columns of indentation.
			for i := b.fenceOffset; i > 0; i-- {
				if c := p.peek(p.offset); c != ' ' && c != '\t' {
					break
				}
				p.advanceOffset(1, true)
			}
			return 0
		}
		// Indented code.
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return 0
		}
		if p.blank {
			p.advanceNextNonspace()
			return 0
		}
		return 1
	case mdHeading, mdThematicBreak:
		return 1
	case mdHTMLBlock:
		if p.blank && (b.htmlType == 6 || b.htmlType == 7) {
			return 1
		}
		return 0
	case mdTable:
		if p.blank {
			return 1
		}
		return 0
	case mdParagraph:
		if p.blank {
			return 1
		}
		return 0
	}
	return 1
}

// --- block starts ----------------------------------------------------------------

var (
	mdATXRe          = regexp.MustCompile(`^#{1,6}(?:[ \t]+|$)`)
	mdATXTrailRe     = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
	mdATXOnlyHashRe  = regexp.MustCompile(`^[ \t]*#+[ \t]*$`)
	mdOpenFenceRe    = regexp.MustCompile("^`{3,}[^`]*$|^~{3,}")
	mdClosingFenceRe = regexp.MustCompile("^(?:`{3,}|~{3,})[ \t]*$")
	mdSetextRe       = regexp.MustCompile(`^(?:=+|-+)[ \t]*$`)
	mdThematicRe     = regexp.MustCompile(`^(?:(?:\*[ \t]*){3,}|(?:_[ \t]*){3,}|(?:-[ \t]*){3,})$`)
	mdBulletRe       = regexp.MustCompile(`^[*+-]`)
	mdOrderedRe      = regexp.MustCompile(`^(\d{1,9})([.)])`)
)

// HTML block open conditions per spec §4.6 (types 1..7).
var mdHTMLOpenRe = [8]*regexp.Regexp{
	nil,
	regexp.MustCompile(`(?i)^<(?:script|pre|textarea|style)(?:[ \t>]|$)`),
	regexp.MustCompile(`^<!--`),
	regexp.MustCompile(`^<\?`),
	regexp.MustCompile(`^<![a-zA-Z]`),
	regexp.MustCompile(`^<!\[CDATA\[`),
	regexp.MustCompile(`(?i)^</?(?:address|article|aside|base|basefont|blockquote|body|caption|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption|figure|footer|form|frame|frameset|h1|h2|h3|h4|h5|h6|head|header|hr|html|iframe|legend|li|link|main|menu|menuitem|nav|noframes|ol|optgroup|option|p|param|search|section|summary|table|tbody|td|tfoot|th|thead|title|tr|track|ul)(?:[ \t]|/?>|$)`),
	regexp.MustCompile(`(?i)^(?:<[a-z][a-z0-9-]*(?:[ \t]+[a-z_:][a-z0-9_.:-]*(?:[ \t]*=[ \t]*(?:[^ \t"'=<>` + "`" + `]+|'[^']*'|"[^"]*"))?)*[ \t]*/?>|</[a-z][a-z0-9-]*[ \t]*>)[ \t]*$`),
}

// HTML block end conditions for types 1..5 (6/7 end on a blank line).
var mdHTMLEndRe = [6]*regexp.Regexp{
	nil,
	regexp.MustCompile(`(?i)</(?:script|pre|textarea|style)>`),
	regexp.MustCompile(`-->`),
	regexp.MustCompile(`\?>`),
	regexp.MustCompile(`>`),
	regexp.MustCompile(`\]\]>`),
}

// tryBlockStarts: 0 = no match, 1 = container start, 2 = leaf start.
func (p *mdParser) tryBlockStarts(container *mdBlock) int {
	// Block quote.
	if !p.indented && p.peek(p.nextNonspace) == '>' {
		p.advanceNextNonspace()
		p.advanceOffset(1, false)
		if c := p.peek(p.offset); c == ' ' || c == '\t' {
			p.advanceOffset(1, true)
		}
		p.closeUnmatchedBlocks()
		p.addChild(mdBlockQuote)
		return 1
	}

	// ATX heading.
	if !p.indented {
		if m := mdATXRe.FindString(p.currentLine[p.nextNonspace:]); m != "" {
			p.advanceNextNonspace()
			p.advanceOffset(len(m), false)
			p.closeUnmatchedBlocks()
			h := p.addChild(mdHeading)
			h.level = len(strings.TrimRight(m, " \t"))
			rest := p.currentLine[p.offset:]
			if mdATXOnlyHashRe.MatchString(rest) {
				rest = ""
			} else {
				rest = mdATXTrailRe.ReplaceAllString(rest, "")
			}
			h.content = append(h.content, strings.Trim(rest, " \t"))
			p.advanceOffset(len(p.currentLine)-p.offset, false)
			return 2
		}
	}

	// Fenced code block.
	if !p.indented {
		if m := mdOpenFenceRe.FindString(p.currentLine[p.nextNonspace:]); m != "" {
			fenceLen := 0
			for fenceLen < len(m) && m[fenceLen] == m[0] {
				fenceLen++
			}
			p.closeUnmatchedBlocks()
			b := p.addChild(mdCodeBlock)
			b.fenced = true
			b.fenceChar = m[0]
			b.fenceLength = fenceLen
			b.fenceOffset = p.indent
			p.advanceNextNonspace()
			p.advanceOffset(fenceLen, false)
			return 2
		}
	}

	// HTML block.
	if !p.indented && p.peek(p.nextNonspace) == '<' {
		rest := p.currentLine[p.nextNonspace:]
		for t := 1; t <= 7; t++ {
			if t == 7 && container.kind == mdParagraph {
				continue // type 7 cannot interrupt a paragraph
			}
			if mdHTMLOpenRe[t].MatchString(rest) {
				p.closeUnmatchedBlocks()
				b := p.addChild(mdHTMLBlock)
				b.htmlType = t
				// Don't advance: the whole line (from offset) is content.
				return 2
			}
		}
	}

	// Setext heading (transforms the open paragraph).
	if !p.indented && container.kind == mdParagraph {
		if m := mdSetextRe.FindString(p.currentLine[p.nextNonspace:]); m != "" {
			p.closeUnmatchedBlocks()
			// Resolve leading link reference definitions first.
			p.extractRefDefs(container)
			if len(container.content) > 0 {
				container.kind = mdHeading
				if m[0] == '=' {
					container.level = 1
				} else {
					container.level = 2
				}
				// Trim per spec (final spaces/tabs of the content line).
				for i, l := range container.content {
					container.content[i] = strings.Trim(l, " \t")
				}
				p.advanceOffset(len(p.currentLine)-p.offset, false)
				return 2
			}
			return 0
		}
	}

	// Thematic break.
	if !p.indented && mdThematicRe.MatchString(p.currentLine[p.nextNonspace:]) {
		p.closeUnmatchedBlocks()
		p.addChild(mdThematicBreak)
		p.advanceOffset(len(p.currentLine)-p.offset, false)
		return 2
	}

	// List item.
	if !p.indented || container.kind == mdList {
		if data, ok := p.parseListMarker(container); ok {
			p.closeUnmatchedBlocks()
			if p.tip.kind != mdList || !listsMatch(container.list, data) {
				l := p.addChild(mdList)
				l.list = data
			}
			item := p.addChild(mdListItem)
			item.list = data
			return 1
		}
	}

	// Indented code block.
	if p.indented && p.tip.kind != mdParagraph && !p.blank {
		p.advanceOffset(4, true)
		p.closeUnmatchedBlocks()
		p.addChild(mdCodeBlock)
		return 2
	}

	return 0
}

// parseListMarker recognizes a bullet or ordered list marker at nextNonspace
// and, when found, advances the cursor past it computing content padding.
func (p *mdParser) parseListMarker(container *mdBlock) (mdListData, bool) {
	var data mdListData
	if p.indent >= 4 {
		return data, false
	}
	rest := p.currentLine[p.nextNonspace:]
	markerLength := 0

	if mdBulletRe.MatchString(rest) {
		// Not a bullet if it's really a thematic break (e.g. "* * *").
		if mdThematicRe.MatchString(rest) {
			return data, false
		}
		data.ordered = false
		data.bulletChar = rest[0]
		markerLength = 1
	} else if m := mdOrderedRe.FindStringSubmatch(rest); m != nil {
		n, _ := strconv.Atoi(m[1])
		if container.kind == mdParagraph && n != 1 {
			return data, false // only "1." can interrupt a paragraph
		}
		data.ordered = true
		data.start = n
		data.delim = m[2][0]
		markerLength = len(m[1]) + 1
	} else {
		return data, false
	}

	// The character after the marker must be space, tab, or end of line.
	if markerLength < len(rest) {
		c := rest[markerLength]
		if c != ' ' && c != '\t' {
			return data, false
		}
	}
	// An empty item cannot interrupt a paragraph.
	if container.kind == mdParagraph && strings.Trim(rest[markerLength:], " \t") == "" {
		return data, false
	}

	data.markerOffset = p.indent
	p.advanceNextNonspace()
	p.advanceOffset(markerLength, true)
	spacesStartCol := p.column
	spacesStartOffset := p.offset
	spacesStartTab := p.partiallyConsumedTab
	for {
		p.advanceOffset(1, true)
		c := p.peek(p.offset)
		if p.column-spacesStartCol >= 5 || (c != ' ' && c != '\t') {
			break
		}
	}
	blankItem := p.offset >= len(p.currentLine)
	spacesAfterMarker := p.column - spacesStartCol
	if spacesAfterMarker >= 5 || spacesAfterMarker < 1 || blankItem {
		data.padding = markerLength + 1
		p.column = spacesStartCol
		p.offset = spacesStartOffset
		p.partiallyConsumedTab = spacesStartTab
		if c := p.peek(p.offset); c == ' ' || c == '\t' {
			p.advanceOffset(1, true)
		}
	} else {
		data.padding = markerLength + spacesAfterMarker
	}
	return data, true
}

func listsMatch(a, b mdListData) bool {
	return a.ordered == b.ordered && a.delim == b.delim && a.bulletChar == b.bulletChar
}

// --- GFM tables -------------------------------------------------------------------

var mdTableDelimCellRe = regexp.MustCompile(`^:?-+:?$`)

// maybeStartTable checks whether the current line is a GFM delimiter row
// matching the open paragraph's last line (the header row); if so the
// paragraph (or its last line) becomes an mdTable and the line is consumed.
func (p *mdParser) maybeStartTable(para *mdBlock) bool {
	if p.indented || len(para.content) == 0 {
		return false
	}
	line := strings.Trim(p.restLine(), " \t")
	if !strings.Contains(line, "|") {
		return false // require a pipe in the delimiter row (excludes setext "---")
	}
	delims := splitTableRow(line)
	if len(delims) == 0 {
		return false
	}
	aligns := make([]mdAlign, len(delims))
	for i, d := range delims {
		d = strings.TrimSpace(d)
		if !mdTableDelimCellRe.MatchString(d) {
			return false
		}
		switch {
		case strings.HasPrefix(d, ":") && strings.HasSuffix(d, ":"):
			aligns[i] = mdAlignCenter
		case strings.HasSuffix(d, ":"):
			aligns[i] = mdAlignRight
		case strings.HasPrefix(d, ":"):
			aligns[i] = mdAlignLeft
		}
	}
	header := para.content[len(para.content)-1]
	headerCells := splitTableRow(strings.Trim(header, " \t"))
	if len(headerCells) != len(delims) {
		return false
	}

	// Transform: the paragraph's last line becomes the table header; earlier
	// lines stay as a paragraph before it.
	if len(para.content) == 1 {
		para.kind = mdTable
		para.content = nil
	} else {
		para.content = para.content[:len(para.content)-1]
		p.finalize(para)
		tbl := p.addChild(mdTable)
		para = tbl
	}
	para.aligns = aligns
	para.headerCells = headerCells
	p.tip = para
	// Consume the delimiter row line.
	p.advanceOffset(len(p.currentLine)-p.offset, false)
	return true
}

// splitTableRow splits a row on unescaped pipes, honoring a leading/trailing
// pipe and backslash-escaped \|.
func splitTableRow(s string) []string {
	s = strings.Trim(s, " \t")
	if s == "" {
		return nil
	}
	var cells []string
	var cur strings.Builder
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			if c != '|' {
				cur.WriteByte('\\')
			}
			cur.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if escaped {
		cur.WriteByte('\\')
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	// A leading/trailing pipe contributes empty edge cells — drop them.
	if len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// --- finalization ----------------------------------------------------------------

func (p *mdParser) finalize(b *mdBlock) {
	parent := b.parent
	b.open = false

	switch b.kind {
	case mdParagraph:
		p.extractRefDefs(b)
		if len(b.content) == 0 {
			b.unlink()
		}
	case mdCodeBlock:
		if b.fenced {
			// First content line is the info string.
			if len(b.content) > 0 {
				b.info = mdUnescapeString(strings.TrimSpace(b.content[0]))
				b.literal = strings.Join(b.content[1:], "\n")
				if len(b.content) > 1 {
					b.literal += "\n"
				}
			}
		} else {
			// Strip trailing blank lines.
			lines := b.content
			for len(lines) > 0 && strings.Trim(lines[len(lines)-1], " \t") == "" {
				lines = lines[:len(lines)-1]
			}
			b.literal = strings.Join(lines, "\n")
			if len(lines) > 0 {
				b.literal += "\n"
			}
		}
		b.content = nil
	case mdHTMLBlock:
		b.literal = strings.Join(b.content, "\n")
		b.content = nil
	case mdTable:
		for _, row := range b.content {
			cells := splitTableRow(strings.Trim(row, " \t"))
			// Normalize to the header width.
			for len(cells) < len(b.headerCells) {
				cells = append(cells, "")
			}
			if len(cells) > len(b.headerCells) {
				cells = cells[:len(b.headerCells)]
			}
			b.rows = append(b.rows, cells)
		}
		b.content = nil
	case mdList:
		b.list.tight = true
		for i, item := range b.children {
			if mdEndsWithBlankLine(item) && i < len(b.children)-1 {
				b.list.tight = false
				break
			}
			for j, sub := range item.children {
				if mdEndsWithBlankLine(sub) && (i < len(b.children)-1 || j < len(item.children)-1) {
					b.list.tight = false
					break
				}
			}
			if !b.list.tight {
				break
			}
		}
		for _, item := range b.children {
			item.list.tight = b.list.tight
		}
	}

	if parent != nil {
		p.tip = parent
	} else {
		p.tip = nil
	}
}

func mdEndsWithBlankLine(b *mdBlock) bool {
	for b != nil {
		if b.lastLineBlank {
			return true
		}
		if b.kind == mdList || b.kind == mdListItem {
			b = b.lastChild()
		} else {
			break
		}
	}
	return false
}
