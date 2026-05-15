package asposepdf

import "fmt"

// OutlineItemCollection represents an outline entry and the collection
// of its children. The recursive structure mirrors Aspose.PDF for .NET:
// each entry is both a tree node (with Title, Color, Action,
// Destination, etc.) and a collection (Add/At/Remove/Count for
// children). The root collection — returned by Document.Outlines() —
// has no parent and an empty Title; only its children are visible as
// top-level bookmarks.
//
// Per ISO 32000-1 §12.3.3.
type OutlineItemCollection struct {
	doc      *Document
	parent   *OutlineItemCollection
	children []*OutlineItemCollection

	// In-memory state for items not yet (or never) backed by a dict.
	title       string
	bold        bool
	italic      bool
	color       *Color
	isExpanded  bool
	action      Action
	destination Destination

	// Set when this item was parsed from an existing PDF; nil for
	// newly-created items. Currently unused (read path is Task 10);
	// kept here so the struct is final-shaped from Task 2.
	dict   pdfDict
	objNum int
}

// NewOutlineItemCollection builds an unattached outline entry bound to
// the given document. Add it to a parent via Document.Outlines().Add(...)
// or via another entry's Add(...) — until added it has no effect on
// the saved PDF.
//
// Aspose .NET: new OutlineItemCollection(doc.Outlines)
// Go:          pdf.NewOutlineItemCollection(doc)
func NewOutlineItemCollection(doc *Document) *OutlineItemCollection {
	return &OutlineItemCollection{
		doc:        doc,
		isExpanded: true, // matches Aspose .NET default
	}
}

// Document returns the document this collection is bound to.
func (o *OutlineItemCollection) Document() *Document { return o.doc }

// Parent returns the parent entry, or nil for the root collection.
func (o *OutlineItemCollection) Parent() *OutlineItemCollection { return o.parent }

// Count returns the number of direct children (placeholder until
// Task 5 adds the rest of the collection API).
func (o *OutlineItemCollection) Count() int { return len(o.children) }

// Title returns the bookmark text.
func (o *OutlineItemCollection) Title() string {
	if o.dict != nil {
		return decodeFormString(o.dict["/Title"])
	}
	return o.title
}

// SetTitle replaces the bookmark text.
func (o *OutlineItemCollection) SetTitle(s string) {
	o.detachFromDict()
	o.title = s
}

// Bold corresponds to /F bit 2 in the outline item dict. Default false.
func (o *OutlineItemCollection) Bold() bool {
	if o.dict != nil {
		return outlineDictFlags(o.dict)&2 != 0
	}
	return o.bold
}

func (o *OutlineItemCollection) SetBold(b bool) {
	o.detachFromDict()
	o.bold = b
}

// Italic corresponds to /F bit 1. Default false.
func (o *OutlineItemCollection) Italic() bool {
	if o.dict != nil {
		return outlineDictFlags(o.dict)&1 != 0
	}
	return o.italic
}

func (o *OutlineItemCollection) SetItalic(b bool) {
	o.detachFromDict()
	o.italic = b
}

// Color returns the RGB label color, or nil if /C is absent (default
// black). SetColor(nil) clears /C.
func (o *OutlineItemCollection) Color() *Color {
	if o.dict != nil {
		return readDictColor(o.dict)
	}
	return o.color
}

func (o *OutlineItemCollection) SetColor(c *Color) {
	o.detachFromDict()
	o.color = c
}

// IsExpanded controls the viewer's initial expand/collapse state.
// Encoded via the sign of /Count. Default true.
func (o *OutlineItemCollection) IsExpanded() bool {
	if o.dict != nil {
		return readDictIsExpanded(o.dict)
	}
	return o.isExpanded
}

func (o *OutlineItemCollection) SetIsExpanded(b bool) {
	o.detachFromDict()
	o.isExpanded = b
}

// Action returns the action attached via /A. Reuses the Action
// interface defined for annotations.
func (o *OutlineItemCollection) Action() Action {
	if o.dict != nil {
		return parseDictAction(o.doc, o.dict["/A"])
	}
	return o.action
}

// SetAction sets the /A action. Pass nil to clear.
func (o *OutlineItemCollection) SetAction(a Action) {
	o.detachFromDict()
	o.action = a
}

// Destination returns the explicit view destination via /Dest, or nil
// if absent. If both Destination and Action are set, /Dest takes
// priority per ISO 32000-1 §12.3.3.
func (o *OutlineItemCollection) Destination() Destination {
	if o.dict != nil {
		return parseDestination(o.doc, o.dict["/Dest"])
	}
	return o.destination
}

// SetDestination sets the /Dest entry. Pass nil to clear.
func (o *OutlineItemCollection) SetDestination(d Destination) {
	o.detachFromDict()
	o.destination = d
}

// detachFromDict pulls all dict-backed values into struct fields and
// clears the dict reference. After this call, the item is no longer
// tied to the original PDF object — subsequent SetXxx work on struct
// fields directly. Idempotent: no-op if dict is already nil.
func (o *OutlineItemCollection) detachFromDict() {
	if o.dict == nil {
		return
	}
	o.title = decodeFormString(o.dict["/Title"])
	flags := outlineDictFlags(o.dict)
	o.bold = flags&2 != 0
	o.italic = flags&1 != 0
	o.color = readDictColor(o.dict)
	o.isExpanded = readDictIsExpanded(o.dict)
	o.destination = parseDestination(o.doc, o.dict["/Dest"])
	o.action = parseDictAction(o.doc, o.dict["/A"])
	o.dict = nil
}

// Outlines returns the document's root outline collection. Always
// non-nil — an empty collection is returned for documents without
// outline content. Mirrors Aspose.PDF for .NET's Document.Outlines.
func (d *Document) Outlines() *OutlineItemCollection {
	if d.outlinesRoot == nil {
		d.outlinesRoot = parseOutlines(d)
	}
	return d.outlinesRoot
}

// Add appends child as the last child of this entry. Errors on nil,
// cross-document, cycle, or already-attached child.
func (o *OutlineItemCollection) Add(child *OutlineItemCollection) error {
	if err := o.validateAddCandidate(child); err != nil {
		return err
	}
	o.children = append(o.children, child)
	child.parent = o
	return nil
}

// Insert inserts child at the given 0-based index among this entry's children.
func (o *OutlineItemCollection) Insert(index int, child *OutlineItemCollection) error {
	if err := o.validateAddCandidate(child); err != nil {
		return err
	}
	if index < 0 || index > len(o.children) {
		return fmt.Errorf("OutlineItemCollection.Insert: index %d out of range [0,%d]", index, len(o.children))
	}
	o.children = append(o.children, nil)
	copy(o.children[index+1:], o.children[index:])
	o.children[index] = child
	child.parent = o
	return nil
}

// Remove detaches child if it's a direct child. Returns true on hit.
func (o *OutlineItemCollection) Remove(child *OutlineItemCollection) bool {
	for i, c := range o.children {
		if c == child {
			o.children = append(o.children[:i], o.children[i+1:]...)
			child.parent = nil
			return true
		}
	}
	return false
}

// RemoveAt detaches the child at the given index.
func (o *OutlineItemCollection) RemoveAt(index int) error {
	if index < 0 || index >= len(o.children) {
		return fmt.Errorf("OutlineItemCollection.RemoveAt: index %d out of range [0,%d)", index, len(o.children))
	}
	child := o.children[index]
	o.children = append(o.children[:index], o.children[index+1:]...)
	child.parent = nil
	return nil
}

// At returns the child at the given 0-based index, or nil if out-of-range.
func (o *OutlineItemCollection) At(index int) *OutlineItemCollection {
	if index < 0 || index >= len(o.children) {
		return nil
	}
	return o.children[index]
}

// All returns a snapshot slice of direct children.
func (o *OutlineItemCollection) All() []*OutlineItemCollection {
	out := make([]*OutlineItemCollection, len(o.children))
	copy(out, o.children)
	return out
}

// validateAddCandidate enforces invariants common to Add and Insert.
func (o *OutlineItemCollection) validateAddCandidate(child *OutlineItemCollection) error {
	if child == nil {
		return fmt.Errorf("OutlineItemCollection: nil child")
	}
	if child.doc != o.doc {
		return fmt.Errorf("OutlineItemCollection: child belongs to a different Document")
	}
	if child.parent != nil {
		return fmt.Errorf("OutlineItemCollection: child is already attached to a parent")
	}
	// Cycle check: walk up o's parent chain and reject if we encounter child.
	for cur := o; cur != nil; cur = cur.parent {
		if cur == child {
			return fmt.Errorf("OutlineItemCollection: cycle detected — child is an ancestor of o")
		}
	}
	return nil
}
