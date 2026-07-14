// SPDX-License-Identifier: MIT

package asposepdf

// Markdown AST (epic pdf-go-fh4l). Unexported: the public surface is
// MarkdownToDocument / Flow.AddMarkdown / Page.AddMarkdown.
//
// The parser follows the two-phase strategy described in the CommonMark spec
// (blocks first, inlines second) and is validated against the official
// spec.json test set (testdata/commonmark_spec.json).

type mdBlockKind int

const (
	mdDocument mdBlockKind = iota
	mdParagraph
	mdHeading // Level 1-6
	mdBlockQuote
	mdList     // container of mdListItem
	mdListItem // container
	mdCodeBlock
	mdThematicBreak
	mdHTMLBlock // recognized per spec; skipped by the PDF renderer
	mdTable     // GFM extension
)

// mdAlign is a GFM table column alignment (from the delimiter row).
type mdAlign int

const (
	mdAlignNone mdAlign = iota
	mdAlignLeft
	mdAlignCenter
	mdAlignRight
)

// mdListData describes a list or list-item marker; two markers belong to the
// same list when Type/Delim/BulletChar match.
type mdListData struct {
	ordered      bool
	bulletChar   byte // '-', '+', '*'
	delim        byte // '.' or ')'
	start        int  // ordered start number
	tight        bool
	padding      int // marker width + following spaces (content column)
	markerOffset int // indentation of the marker itself
}

// mdBlock is one node of the block tree.
type mdBlock struct {
	kind     mdBlockKind
	parent   *mdBlock
	children []*mdBlock

	// Accumulated raw content lines (paragraph/heading text before inline
	// parsing, code/html literal lines, table rows).
	content []string

	level   int    // heading level
	literal string // finalized code/html literal
	info    string // fence info string (first word = language)

	list mdListData // mdList and mdListItem

	// GFM table: raw cell texts (inline-parsed in phase 2).
	aligns      []mdAlign
	headerCells []string
	rows        [][]string

	// Inline tree (phase 2), set for paragraph/heading/table cells' owner.
	inlines []*mdInline

	// Block-parser state.
	open            bool
	lastLineBlank   bool
	fenced          bool
	fenceChar       byte
	fenceLength     int
	fenceOffset     int
	htmlType        int // 1..7 per spec §4.6
	startLine       int
	spanningHeader  bool // internal: table currently collecting rows
	refsOnly        bool // paragraph consumed entirely by link reference definitions
	blankAfterEmpty bool // list item began with a blank-after-marker line
}

// mdInlineKind — inline node kinds (parsed in phase 2, epic pdf-go-fh4l.2).
type mdInlineKind int

const (
	mdText mdInlineKind = iota
	mdSoftBreak
	mdHardBreak
	mdCodeSpan
	mdEmph
	mdStrong
	mdStrike // GFM
	mdLink
	mdImage
	mdHTMLInline // raw inline HTML; skipped by the PDF renderer (except <br>)
)

type mdInline struct {
	kind     mdInlineKind
	text     string // mdText/mdCodeSpan/mdHTMLInline literal
	dest     string // mdLink/mdImage destination
	title    string
	children []*mdInline
}

// mdLinkRef is a link reference definition ([label]: dest "title").
type mdLinkRef struct {
	dest  string
	title string
}

func (b *mdBlock) appendChild(child *mdBlock) {
	child.parent = b
	b.children = append(b.children, child)
}

func (b *mdBlock) lastChild() *mdBlock {
	if len(b.children) == 0 {
		return nil
	}
	return b.children[len(b.children)-1]
}

// unlink removes b from its parent's children.
func (b *mdBlock) unlink() {
	if b.parent == nil {
		return
	}
	siblings := b.parent.children
	for i, c := range siblings {
		if c == b {
			b.parent.children = append(siblings[:i], siblings[i+1:]...)
			break
		}
	}
	b.parent = nil
}

// canContain reports whether a block of kind parent may hold a child of kind k
// (CommonMark: lists hold only items; items/quotes/document hold anything but
// items; leaves hold nothing).
func canContain(parent, k mdBlockKind) bool {
	switch parent {
	case mdDocument, mdBlockQuote, mdListItem:
		return k != mdListItem
	case mdList:
		return k == mdListItem
	default:
		return false
	}
}

// acceptsLines reports whether the block collects raw text lines.
func acceptsLines(k mdBlockKind) bool {
	switch k {
	case mdParagraph, mdCodeBlock, mdHTMLBlock, mdTable:
		return true
	}
	return false
}
