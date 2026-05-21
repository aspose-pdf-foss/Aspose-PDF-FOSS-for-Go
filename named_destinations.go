// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
)

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

// NamedDestinations is a name-to-destination map per ISO 32000-1 §12.3.2.3.
// Backed at PDF level by the modern /Catalog/Names/Dests name tree
// (PDF 1.2+); on read it also absorbs legacy /Catalog/Dests for
// backward compatibility. On write, only /Names/Dests is emitted.
//
// Mirrors Aspose.PDF for .NET's NamedDestinations collection.
type NamedDestinations struct {
	doc     *Document
	entries map[string]Destination
}

// Document returns the document this collection is bound to.
func (n *NamedDestinations) Document() *Document { return n.doc }

// Count returns the number of registered entries.
func (n *NamedDestinations) Count() int { return len(n.entries) }

// Has reports whether name is registered.
func (n *NamedDestinations) Has(name string) bool {
	_, ok := n.entries[name]
	return ok
}

// Get returns the destination registered under name, or nil if absent.
// Never returns a *NamedDestination (no recursive lookups).
func (n *NamedDestinations) Get(name string) Destination {
	return n.entries[name]
}

// Add registers dest under name. Errors on:
//   - empty name
//   - nil dest
//   - dest is itself a *NamedDestination (would create a name→name loop)
// If name was already present, the previous value is replaced silently.
func (n *NamedDestinations) Add(name string, dest Destination) error {
	if name == "" {
		return fmt.Errorf("NamedDestinations.Add: empty name")
	}
	if dest == nil {
		return fmt.Errorf("NamedDestinations.Add(%q): nil destination", name)
	}
	if _, ok := dest.(*NamedDestination); ok {
		return fmt.Errorf("NamedDestinations.Add(%q): value cannot itself be a NamedDestination (would loop)", name)
	}
	if n.entries == nil {
		n.entries = map[string]Destination{}
	}
	n.entries[name] = dest
	return nil
}

// Remove deletes the entry; returns true if it existed.
func (n *NamedDestinations) Remove(name string) bool {
	if _, ok := n.entries[name]; !ok {
		return false
	}
	delete(n.entries, name)
	return true
}

// Names returns a snapshot slice of all registered names in lex order.
func (n *NamedDestinations) Names() []string {
	out := make([]string, 0, len(n.entries))
	for k := range n.entries {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// All returns a snapshot map of name → destination.
func (n *NamedDestinations) All() map[string]Destination {
	out := make(map[string]Destination, len(n.entries))
	for k, v := range n.entries {
		out[k] = v
	}
	return out
}

// Clear removes every entry.
func (n *NamedDestinations) Clear() {
	n.entries = nil
}

// NamedDestinations returns the document's named-destination collection.
// Always non-nil. Lazy-initialized on first call from /Catalog/Names/Dests
// (modern PDF 1.2+ name tree) merged with legacy /Catalog/Dests; on
// collision the /Names/Dests entry wins.
//
// Mirrors Aspose.PDF for .NET's Document.NamedDestinations property.
func (d *Document) NamedDestinations() *NamedDestinations {
	if d.namedDests == nil {
		d.namedDests = parseNamedDestinations(d)
	}
	return d.namedDests
}
