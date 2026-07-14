// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CommonMark inline parser (epic pdf-go-fh4l.2), a port of the reference
// algorithm (commonmark.js / cmark): a single left-to-right pass emitting
// nodes into a doubly-linked working list, a delimiter stack for the
// emphasis algorithm (spec §6.2 rules 1–17, including the "multiple of 3"
// rule), and a bracket stack for links/images (inline, full/collapsed/
// shortcut reference). GFM strikethrough (~~) rides the same delimiter
// machinery.

// inlNode is the working doubly-linked node; converted to *mdInline at the
// end (the linked shape is required by the emphasis/link algorithms, which
// splice ranges of siblings into new parents).
type inlNode struct {
	kind                  mdInlineKind
	text                  string
	dest, title           string
	parent                *inlNode
	prev, next            *inlNode
	firstChild, lastChild *inlNode
}

func (n *inlNode) appendChild(c *inlNode) {
	c.parent = n
	c.prev = n.lastChild
	c.next = nil
	if n.lastChild != nil {
		n.lastChild.next = c
	} else {
		n.firstChild = c
	}
	n.lastChild = c
}

// insertAfter inserts sibling after n within n's parent's child list.
func (n *inlNode) insertAfter(sibling *inlNode) {
	sibling.parent = n.parent
	sibling.next = n.next
	if sibling.next != nil {
		sibling.next.prev = sibling
	} else if n.parent != nil {
		n.parent.lastChild = sibling
	}
	sibling.prev = n
	n.next = sibling
}

// removeNode unlinks n from its parent's child list.
func removeNode(n *inlNode) {
	if n.prev != nil {
		n.prev.next = n.next
	} else if n.parent != nil {
		n.parent.firstChild = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else if n.parent != nil {
		n.parent.lastChild = n.prev
	}
	n.parent, n.prev, n.next = nil, nil, nil
}

type mdDelim struct {
	cc         byte // '*', '_' or '~'
	numdelims  int
	origdelims int
	node       *inlNode
	prev, next *mdDelim
	canOpen    bool
	canClose   bool
}

type mdBracket struct {
	node         *inlNode
	prev         *mdBracket
	prevDelim    *mdDelim
	index        int // subject position after the bracket
	image        bool
	active       bool
	bracketAfter bool
}

type mdInlineParser struct {
	subject    string
	pos        int
	root       *inlNode // container whose child list we append to
	refmap     map[string]mdLinkRef
	delimiters *mdDelim
	brackets   *mdBracket
	gfmLinkify bool // GFM autolink extension (bare URLs/emails in text)
}

// parseInlineContent parses one leaf block's raw content into an inline tree.
func parseInlineContent(content string, refmap map[string]mdLinkRef, gfmLinkify bool) []*mdInline {
	p := &mdInlineParser{
		subject:    strings.Trim(content, " \t\n"),
		refmap:     refmap,
		root:       &inlNode{},
		gfmLinkify: gfmLinkify,
	}
	for p.pos < len(p.subject) {
		p.parseInline()
	}
	p.processEmphasis(nil)
	return convertInlines(p.root.firstChild)
}

// resolveInlines walks the block tree parsing inline content of every leaf
// that carries text.
func resolveInlines(b *mdBlock, refmap map[string]mdLinkRef, gfmLinkify bool) {
	switch b.kind {
	case mdParagraph, mdHeading:
		b.inlines = parseInlineContent(strings.Join(b.content, "\n"), refmap, gfmLinkify)
	}
	for _, c := range b.children {
		resolveInlines(c, refmap, gfmLinkify)
	}
}

// parseMarkdown is the full front end: blocks, refmap, then inlines — in the
// library's dialect (CommonMark + the GFM extensions).
func parseMarkdown(src string) *mdBlock {
	doc, refmap := parseMarkdownBlocks(src)
	markTaskItems(doc)
	resolveInlines(doc, refmap, true)
	return doc
}

// markTaskItems detects GFM task-list markers ("[ ] "/"[x] " opening a list
// item's first paragraph) before inline parsing (afterwards the brackets are
// scattered across inline nodes) and strips them from the source text.
func markTaskItems(b *mdBlock) {
	if b.kind == mdListItem && len(b.children) > 0 {
		para := b.children[0]
		if para.kind == mdParagraph && len(para.content) > 0 {
			t := para.content[0]
			switch {
			case strings.HasPrefix(t, "[ ] "), t == "[ ]":
				b.task = true
				para.content[0] = strings.TrimPrefix(strings.TrimPrefix(t, "[ ] "), "[ ]")
			case strings.HasPrefix(t, "[x] "), strings.HasPrefix(t, "[X] "), t == "[x]", t == "[X]":
				b.task, b.taskChecked = true, true
				para.content[0] = strings.TrimLeft(t[3:], " ")
			}
		}
	}
	for _, c := range b.children {
		markTaskItems(c)
	}
}

func convertInlines(first *inlNode) []*mdInline {
	var out []*mdInline
	for n := first; n != nil; n = n.next {
		m := &mdInline{kind: n.kind, text: n.text, dest: n.dest, title: n.title}
		m.children = convertInlines(n.firstChild)
		out = append(out, m)
	}
	return out
}

// --- main dispatch ------------------------------------------------------------

func (p *mdInlineParser) appendNode(n *inlNode) *inlNode {
	p.root.appendChild(n)
	return n
}

func (p *mdInlineParser) appendText(s string) *inlNode {
	return p.appendNode(&inlNode{kind: mdText, text: s})
}

func (p *mdInlineParser) peekByte() byte {
	if p.pos < len(p.subject) {
		return p.subject[p.pos]
	}
	return 0
}

func (p *mdInlineParser) parseInline() {
	c := p.peekByte()
	switch c {
	case '\n':
		p.parseNewline()
	case '\\':
		p.parseBackslash()
	case '`':
		p.parseBackticks()
	case '*', '_', '~':
		p.handleDelim(c)
	case '[':
		p.pos++
		node := p.appendText("[")
		p.addBracket(node, false)
	case '!':
		if p.pos+1 < len(p.subject) && p.subject[p.pos+1] == '[' {
			p.pos += 2
			node := p.appendText("![")
			p.addBracket(node, true)
		} else {
			p.pos++
			p.appendText("!")
		}
	case ']':
		p.parseCloseBracket()
	case '<':
		if !p.parseAutolink() && !p.parseHTMLTag() {
			p.pos++
			p.appendText("<")
		}
	case '&':
		p.parseEntity()
	default:
		p.parseString()
	}
}

// parseString consumes plain text up to the next special character, turning
// GFM bare autolinks (http://…, www.…, email) into links along the way.
func (p *mdInlineParser) parseString() {
	start := p.pos
	for p.pos < len(p.subject) && !mdInlineSpecial(p.subject[p.pos]) {
		p.pos++
	}
	if p.pos == start { // defensive: never loop forever
		p.pos++
	}
	text := p.subject[start:p.pos]
	// No linkification inside an open bracket (link text must not nest links).
	if p.gfmLinkify && p.brackets == nil {
		p.appendLinkified(text)
		return
	}
	p.appendText(text)
}

// GFM autolink extension: bare URLs, www. domains and emails in plain text.
var (
	mdBareURLRe   = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<]+`)
	mdBareEmailRe = regexp.MustCompile(`[A-Za-z0-9._+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9_-]+)+`)
)

// appendLinkified splits text into text/link nodes per the GFM autolink
// extension (word-boundary start, trailing-punctuation and paren trimming).
func (p *mdInlineParser) appendLinkified(text string) {
	for text != "" {
		loc := mdBareURLRe.FindStringIndex(text)
		email := false
		if eloc := mdBareEmailRe.FindStringIndex(text); eloc != nil && (loc == nil || eloc[0] < loc[0]) {
			loc = eloc
			email = true
		}
		if loc == nil {
			break
		}
		// The match must start a word (start of text or after whitespace/(/*_~).
		if loc[0] > 0 {
			c := text[loc[0]-1]
			if c != ' ' && c != '\t' && c != '\n' && c != '(' && c != '*' && c != '_' && c != '~' {
				p.appendText(text[:loc[1]])
				text = text[loc[1]:]
				continue
			}
		}
		match := text[loc[0]:loc[1]]
		if email {
			// Local part must not be preceded by more address chars; final
			// char of the domain must be alphanumeric.
			for len(match) > 0 && !isAlnumByte(match[len(match)-1]) {
				match = match[:len(match)-1]
			}
			if !strings.Contains(match, "@") {
				p.appendText(text[:loc[1]])
				text = text[loc[1]:]
				continue
			}
		} else {
			match = mdTrimAutolinkTail(match)
		}
		if match == "" {
			p.appendText(text[:loc[1]])
			text = text[loc[1]:]
			continue
		}
		p.appendText(text[:loc[0]])
		dest := match
		switch {
		case email:
			dest = "mailto:" + match
		case strings.HasPrefix(strings.ToLower(match), "www."):
			dest = "http://" + match
		}
		link := &inlNode{kind: mdLink, dest: dest}
		link.appendChild(&inlNode{kind: mdText, text: match})
		p.appendNode(link)
		text = text[loc[0]+len(match):]
	}
	if text != "" {
		p.appendText(text)
	}
}

func isAlnumByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// mdTrimAutolinkTail strips trailing punctuation and unbalanced closing
// parens from a bare autolink, per the GFM extension rules.
func mdTrimAutolinkTail(s string) string {
	for s != "" {
		c := s[len(s)-1]
		if strings.IndexByte("?!.,:*_~'\";", c) >= 0 {
			s = s[:len(s)-1]
			continue
		}
		if c == ')' {
			if strings.Count(s, ")") > strings.Count(s, "(") {
				s = s[:len(s)-1]
				continue
			}
		}
		break
	}
	return s
}

func mdInlineSpecial(c byte) bool {
	switch c {
	case '\n', '\\', '`', '*', '_', '~', '[', ']', '!', '<', '&':
		return true
	}
	return false
}

// parseNewline: a soft break, or a hard break when the preceding text ends
// with two or more spaces.
func (p *mdInlineParser) parseNewline() {
	p.pos++
	last := p.root.lastChild
	if last != nil && last.kind == mdText && strings.HasSuffix(last.text, " ") {
		hard := strings.HasSuffix(last.text, "  ")
		last.text = strings.TrimRight(last.text, " ")
		kind := mdSoftBreak
		if hard {
			kind = mdHardBreak
		}
		if last.text == "" {
			// Text was only spaces: replace it.
			last.kind = kind
		} else {
			p.appendNode(&inlNode{kind: kind})
		}
	} else {
		p.appendNode(&inlNode{kind: mdSoftBreak})
	}
	// Leading spaces of the next line are gobbled.
	for p.pos < len(p.subject) && p.subject[p.pos] == ' ' {
		p.pos++
	}
}

func (p *mdInlineParser) parseBackslash() {
	p.pos++
	switch {
	case p.peekByte() == '\n':
		p.pos++
		p.appendNode(&inlNode{kind: mdHardBreak})
		for p.pos < len(p.subject) && p.subject[p.pos] == ' ' {
			p.pos++
		}
	case p.pos < len(p.subject) && isASCIIPunct(p.subject[p.pos]):
		p.appendText(string(p.subject[p.pos]))
		p.pos++
	default:
		p.appendText("\\")
	}
}

// parseBackticks: a code span, or the literal run when unmatched.
func (p *mdInlineParser) parseBackticks() {
	start := p.pos
	for p.pos < len(p.subject) && p.subject[p.pos] == '`' {
		p.pos++
	}
	openLen := p.pos - start
	afterOpen := p.pos

	// Find a backtick run of exactly openLen.
	i := afterOpen
	for i < len(p.subject) {
		j := strings.IndexByte(p.subject[i:], '`')
		if j < 0 {
			break
		}
		runStart := i + j
		runEnd := runStart
		for runEnd < len(p.subject) && p.subject[runEnd] == '`' {
			runEnd++
		}
		if runEnd-runStart == openLen {
			content := p.subject[afterOpen:runStart]
			content = strings.ReplaceAll(content, "\n", " ")
			if len(content) > 2 && content[0] == ' ' && content[len(content)-1] == ' ' &&
				strings.Trim(content, " ") != "" {
				content = content[1 : len(content)-1]
			}
			p.appendNode(&inlNode{kind: mdCodeSpan, text: content})
			p.pos = runEnd
			return
		}
		i = runEnd
	}
	// No matching closer: literal backticks.
	p.pos = afterOpen
	p.appendText(p.subject[start:afterOpen])
}

// --- emphasis delimiters ----------------------------------------------------------

func mdIsUniSpace(r rune) bool {
	return r == 0 || unicode.IsSpace(r)
}

func mdIsUniPunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// handleDelim scans a delimiter run, computes flanking, emits its text node
// and pushes a delimiter stack entry.
func (p *mdInlineParser) handleDelim(cc byte) {
	before, _ := utf8.DecodeLastRuneInString(p.subject[:p.pos])
	if p.pos == 0 {
		before = 0
	}
	start := p.pos
	for p.pos < len(p.subject) && p.subject[p.pos] == cc {
		p.pos++
	}
	num := p.pos - start
	after, _ := utf8.DecodeRuneInString(p.subject[p.pos:])
	if p.pos >= len(p.subject) {
		after = 0
	}

	afterIsWS := mdIsUniSpace(after)
	afterIsPunct := mdIsUniPunct(after)
	beforeIsWS := mdIsUniSpace(before)
	beforeIsPunct := mdIsUniPunct(before)

	leftFlanking := !afterIsWS && (!afterIsPunct || beforeIsWS || beforeIsPunct)
	rightFlanking := !beforeIsWS && (!beforeIsPunct || afterIsWS || afterIsPunct)

	var canOpen, canClose bool
	switch cc {
	case '_':
		canOpen = leftFlanking && (!rightFlanking || beforeIsPunct)
		canClose = rightFlanking && (!leftFlanking || afterIsPunct)
	default: // '*', '~'
		canOpen = leftFlanking
		canClose = rightFlanking
	}
	if cc == '~' && num > 2 {
		// GFM: runs of more than two tildes are not strikethrough.
		canOpen, canClose = false, false
	}

	node := p.appendText(p.subject[start:p.pos])
	if canOpen || canClose {
		p.delimiters = &mdDelim{
			cc: cc, numdelims: num, origdelims: num,
			node: node, prev: p.delimiters,
			canOpen: canOpen, canClose: canClose,
		}
		if p.delimiters.prev != nil {
			p.delimiters.prev.next = p.delimiters
		}
	}
}

func (p *mdInlineParser) removeDelimiter(d *mdDelim) {
	if d.prev != nil {
		d.prev.next = d.next
	}
	if d.next == nil {
		p.delimiters = d.prev
	} else {
		d.next.prev = d.prev
	}
}

// processEmphasis implements spec §6.2's "look for link or emphasis" phase
// over the delimiter stack, down to stackBottom (exclusive).
func (p *mdInlineParser) processEmphasis(stackBottom *mdDelim) {
	var openersBottom [3]map[byte]*mdDelim
	for i := range openersBottom {
		openersBottom[i] = map[byte]*mdDelim{
			'_': stackBottom, '*': stackBottom, '~': stackBottom,
		}
	}

	// Find the first delimiter above stackBottom.
	closer := p.delimiters
	for closer != nil && closer.prev != stackBottom {
		closer = closer.prev
	}
	for closer != nil {
		if !closer.canClose {
			closer = closer.next
			continue
		}
		cc := closer.cc
		// Look back for a matching opener.
		opener := closer.prev
		openerFound := false
		bottom := openersBottom[closer.origdelims%3][cc]
		for opener != nil && opener != stackBottom && opener != bottom {
			if opener.cc == cc && opener.canOpen {
				oddMatch := (closer.canOpen || opener.canClose) &&
					closer.origdelims%3 != 0 &&
					(opener.origdelims+closer.origdelims)%3 == 0
				if cc == '~' && opener.numdelims != closer.numdelims {
					// GFM strikethrough requires equal-length runs.
				} else if !oddMatch {
					openerFound = true
					break
				}
			}
			opener = opener.prev
		}
		oldCloser := closer

		if !openerFound {
			closer = closer.next
		} else {
			var newNode *inlNode
			if cc == '~' {
				newNode = &inlNode{kind: mdStrike}
				closer.numdelims = 0
				opener.numdelims = 0
				opener.node.text = ""
				closer.node.text = ""
			} else {
				useDelims := 1
				if closer.numdelims >= 2 && opener.numdelims >= 2 {
					useDelims = 2
				}
				kind := mdEmph
				if useDelims == 2 {
					kind = mdStrong
				}
				newNode = &inlNode{kind: kind}
				opener.numdelims -= useDelims
				closer.numdelims -= useDelims
				opener.node.text = opener.node.text[:opener.numdelims]
				closer.node.text = closer.node.text[:closer.numdelims]
			}

			// Move nodes between opener.node and closer.node into newNode.
			for n := opener.node.next; n != nil && n != closer.node; {
				next := n.next
				removeNode(n)
				newNode.appendChild(n)
				n = next
			}
			opener.node.insertAfter(newNode)

			// Remove delimiters between opener and closer.
			for d := closer.prev; d != nil && d != opener; {
				prev := d.prev
				p.removeDelimiter(d)
				d = prev
			}
			if opener.numdelims == 0 {
				removeNode(opener.node)
				p.removeDelimiter(opener)
			}
			if closer.numdelims == 0 {
				removeNode(closer.node)
				next := closer.next
				p.removeDelimiter(closer)
				closer = next
			}
		}

		if !openerFound {
			// Set a lower bound for future searches.
			openersBottom[oldCloser.origdelims%3][cc] = oldCloser.prev
			if !oldCloser.canOpen {
				p.removeDelimiter(oldCloser)
			}
		}
	}
	// Remove all remaining delimiters above stackBottom.
	for p.delimiters != nil && p.delimiters != stackBottom {
		p.removeDelimiter(p.delimiters)
	}
}

// --- brackets: links and images -------------------------------------------------

func (p *mdInlineParser) addBracket(node *inlNode, image bool) {
	if p.brackets != nil {
		p.brackets.bracketAfter = true
	}
	p.brackets = &mdBracket{
		node: node, prev: p.brackets, prevDelim: p.delimiters,
		index: p.pos, image: image, active: true,
	}
}

func (p *mdInlineParser) removeBracket() {
	p.brackets = p.brackets.prev
}

func (p *mdInlineParser) parseCloseBracket() {
	p.pos++
	startPos := p.pos

	opener := p.brackets
	if opener == nil {
		p.appendText("]")
		return
	}
	if !opener.active {
		p.removeBracket()
		p.appendText("]")
		return
	}

	var dest, title string
	matched := false

	// Inline link: (dest "title")
	if p.peekByte() == '(' {
		savePos := p.pos
		p.pos++
		p.spnl()
		if d, ok := p.parseLinkDestination(); ok {
			dest = d
			n := p.pos
			p.spnl()
			if p.pos != n { // whitespace before a title is required
				if t, ok := p.parseLinkTitle(); ok {
					title = t
					p.spnl()
				}
			}
			if p.peekByte() == ')' {
				p.pos++
				matched = true
			}
		}
		if !matched {
			p.pos = savePos
		}
	}

	// Reference link: [label], [] or shortcut.
	if !matched {
		beforeLabel := p.pos
		n := p.parseLinkLabel()
		var refLabel string
		if n > 2 {
			refLabel = p.subject[beforeLabel+1 : beforeLabel+n-1]
		} else if !opener.bracketAfter {
			// Collapsed [] or shortcut: the label is the bracketed text.
			refLabel = p.subject[opener.index : startPos-1]
		}
		if n == 0 {
			p.pos = startPos
		}
		if refLabel != "" {
			if ref, ok := p.refmap[mdNormalizeLabel(refLabel)]; ok {
				dest = ref.dest
				title = ref.title
				matched = true
			} else {
				p.pos = startPos
			}
		}
	}

	if !matched {
		p.removeBracket()
		p.pos = startPos
		p.appendText("]")
		return
	}

	kind := mdLink
	if opener.image {
		kind = mdImage
	}
	link := &inlNode{kind: kind, dest: dest, title: title}
	// Move everything after the opener's bracket node into the link.
	for n := opener.node.next; n != nil; {
		next := n.next
		removeNode(n)
		link.appendChild(n)
		n = next
	}
	p.appendNode(link)
	p.processEmphasis(opener.prevDelim)
	removeNode(opener.node)
	p.removeBracket()

	// Links cannot contain links: deactivate earlier link openers.
	if !opener.image {
		for b := p.brackets; b != nil; b = b.prev {
			if !b.image {
				b.active = false
			}
		}
	}
}

// parseLinkLabel matches [label] at pos; returns the total length or 0.
func (p *mdInlineParser) parseLinkLabel() int {
	if p.peekByte() != '[' {
		return 0
	}
	i := p.pos + 1
	hasContent := false
	for i < len(p.subject) {
		c := p.subject[i]
		if c == '\\' && i+1 < len(p.subject) {
			hasContent = true
			i += 2
			continue
		}
		if c == ']' {
			n := i - p.pos + 1
			if n > 1001 {
				return 0
			}
			// [] is allowed as a collapsed marker (length 2).
			if n == 2 || hasContent {
				p.pos = i + 1
				return n
			}
			return 0
		}
		if c == '[' {
			return 0
		}
		if c != ' ' && c != '\t' && c != '\n' {
			hasContent = true
		}
		i++
		if i-p.pos > 1001 {
			return 0
		}
	}
	return 0
}

// spnl skips spaces/tabs and at most one newline.
func (p *mdInlineParser) spnl() {
	nl := false
	for p.pos < len(p.subject) {
		switch p.subject[p.pos] {
		case ' ', '\t':
			p.pos++
		case '\n':
			if nl {
				return
			}
			nl = true
			p.pos++
		default:
			return
		}
	}
}

// parseLinkDestination at pos: <...> or a bare run with balanced parens.
func (p *mdInlineParser) parseLinkDestination() (string, bool) {
	s := p.subject
	i := p.pos
	if i < len(s) && s[i] == '<' {
		i++
		start := i
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if c == '>' {
				p.pos = i + 1
				return mdUnescapeString(s[start:i]), true
			}
			if c == '<' || c == '\n' {
				return "", false
			}
			i++
		}
		return "", false
	}
	start := i
	depth := 0
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			i += 2
			continue
		}
		if c == '(' {
			depth++
			if depth > 32 {
				return "", false
			}
		} else if c == ')' {
			if depth == 0 {
				break
			}
			depth--
		} else if c <= ' ' {
			break
		}
		i++
	}
	if i == start && (i >= len(s) || s[i] != ')') {
		return "", false
	}
	if depth != 0 {
		return "", false
	}
	p.pos = i
	return mdUnescapeString(s[start:i]), true
}

// parseLinkTitle at pos: "...", '...' or (...).
func (p *mdInlineParser) parseLinkTitle() (string, bool) {
	s := p.subject
	i := p.pos
	if i >= len(s) {
		return "", false
	}
	opener := s[i]
	if opener != '"' && opener != '\'' && opener != '(' {
		return "", false
	}
	closer := opener
	if opener == '(' {
		closer = ')'
	}
	i++
	start := i
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if c == closer {
			p.pos = i + 1
			return mdUnescapeString(s[start:i]), true
		}
		if opener == '(' && c == '(' {
			return "", false
		}
		i++
	}
	return "", false
}

// --- autolinks and raw HTML -----------------------------------------------------

var (
	mdAutolinkURIRe   = regexp.MustCompile(`^<[A-Za-z][A-Za-z0-9.+-]{1,31}:[^<>\x00-\x20]*>`)
	mdAutolinkEmailRe = regexp.MustCompile(`^<(?:[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)>`)
)

func (p *mdInlineParser) parseAutolink() bool {
	rest := p.subject[p.pos:]
	if m := mdAutolinkEmailRe.FindString(rest); m != "" {
		addr := m[1 : len(m)-1]
		link := &inlNode{kind: mdLink, dest: "mailto:" + addr}
		link.appendChild(&inlNode{kind: mdText, text: addr})
		p.appendNode(link)
		p.pos += len(m)
		return true
	}
	if m := mdAutolinkURIRe.FindString(rest); m != "" {
		uri := m[1 : len(m)-1]
		link := &inlNode{kind: mdLink, dest: uri}
		link.appendChild(&inlNode{kind: mdText, text: uri})
		p.appendNode(link)
		p.pos += len(m)
		return true
	}
	return false
}

// Raw inline HTML per spec §6.4 (open/close tag, comment, PI, declaration,
// CDATA).
var mdHTMLTagRe = regexp.MustCompile(`(?s)^(?:` +
	`<[A-Za-z][A-Za-z0-9-]*(?:[ \t\n]+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:[ \t\n]*=[ \t\n]*(?:[^ \t\n"'=<>` + "`" + `]+|'[^']*'|"[^"]*"))?)*[ \t\n]*/?>` + // open tag
	`|</[A-Za-z][A-Za-z0-9-]*[ \t\n]*>` + // closing tag
	`|<!-->|<!--->|<!--.*?-->` + // comment
	`|<\?.*?\?>` + // processing instruction
	`|<![A-Za-z][^>]*>` + // declaration
	`|<!\[CDATA\[.*?\]\]>` + // CDATA
	`)`)

func (p *mdInlineParser) parseHTMLTag() bool {
	m := mdHTMLTagRe.FindString(p.subject[p.pos:])
	if m == "" {
		return false
	}
	p.appendNode(&inlNode{kind: mdHTMLInline, text: m})
	p.pos += len(m)
	return true
}

func (p *mdInlineParser) parseEntity() {
	if m := mdEntityRe.FindString(p.subject[p.pos:]); m != "" {
		if u := mdUnescapeString(m); u != m {
			p.appendText(u)
			p.pos += len(m)
			return
		}
	}
	p.pos++
	p.appendText("&")
}
