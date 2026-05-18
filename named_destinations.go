package asposepdf

// NamedDestination wraps a name reference into the document's
// NamedDestinations collection. Implements Destination so it can be
// used wherever an explicit destination is accepted (outline entries,
// future link annotation /Dest values, GoToAction).
//
// Resolution is lazy: Page() and Resolve() look up the name in
// doc.NamedDestinations() at call time. This allows constructing a
// NamedDestination before adding the entry to the collection
// (forward reference).
//
// Per ISO 32000-1 §12.3.2.3.
type NamedDestination struct {
	doc  *Document
	name string
}

// NewNamedDestination builds a name-reference destination. The name
// need not be registered yet — resolution is lazy at Page() and
// write time.
//
// Aspose .NET: new NamedDestination(name)
// Go:          pdf.NewNamedDestination(doc, name)
func NewNamedDestination(doc *Document, name string) *NamedDestination {
	return &NamedDestination{doc: doc, name: name}
}

// DestinationType returns DestinationTypeNamed.
func (n *NamedDestination) DestinationType() DestinationType { return DestinationTypeNamed }

// Name returns the registered name this destination references.
func (n *NamedDestination) Name() string { return n.name }

// Page resolves the underlying destination's page via the document's
// NamedDestinations collection. Returns nil if the name is not
// registered or the underlying destination has no Page.
func (n *NamedDestination) Page() *Page {
	inner := n.Resolve()
	if inner == nil {
		return nil
	}
	return inner.Page()
}

// Resolve returns the underlying explicit destination registered under
// this name, or nil if absent. Useful when you need the typed
// concrete (e.g. *DestinationXYZ to read coordinates).
func (n *NamedDestination) Resolve() Destination {
	if n.doc == nil || n.name == "" {
		return nil
	}
	return n.doc.NamedDestinations().Get(n.name)
}

// --- Task 1 stubs — replaced by Task 2's full implementation. ---

// NamedDestinations stub — Task 2 fills in the full collection API.
type NamedDestinations struct {
	doc *Document
}

// Get stub — Task 2 replaces with the real map lookup.
func (n *NamedDestinations) Get(name string) Destination { return nil }

// NamedDestinations returns the document's named-destination collection.
// Always non-nil. Lazy-initialized on first call.
//
// Mirrors Aspose.PDF for .NET's Document.NamedDestinations property.
// (Task 7 replaces the stub initialization with parseNamedDestinations.)
func (d *Document) NamedDestinations() *NamedDestinations {
	if d.namedDests == nil {
		d.namedDests = &NamedDestinations{doc: d}
	}
	return d.namedDests
}
