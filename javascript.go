// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
)

// JavaScriptCollection is the document-level JavaScript store, backed by
// the /Catalog/Names/JavaScript name tree (ISO 32000-1 §7.7.4 / §8.5.1).
// Each entry is a named script that the viewer executes when the document
// opens (after any /OpenAction). Always non-nil; empty for documents
// without document-level JavaScript. Lazy-parsed on first call.
//
// Mirrors Aspose.PDF for .NET's Document.JavaScript (a named-script
// collection). Note this is distinct from action-level JavaScriptAction,
// which runs on a specific annotation/field event.
//
// SECURITY WARNING: document-level JavaScript executes in the recipient's
// viewer when the file is opened. Embed only scripts you authored or
// audited; viewers commonly disable JavaScript by default.
type JavaScriptCollection struct {
	doc     *Document
	scripts map[string]string
	order   []string
}

// JavaScript returns the document-level JavaScript collection. Always
// non-nil. Mirrors Aspose.PDF for .NET's Document.JavaScript.
func (d *Document) JavaScript() *JavaScriptCollection {
	if d.js == nil {
		d.js = parseDocumentJavaScript(d)
	}
	return d.js
}

// parseDocumentJavaScript reads /Catalog/Names/JavaScript into a
// collection. Always returns a non-nil collection.
func parseDocumentJavaScript(d *Document) *JavaScriptCollection {
	c := &JavaScriptCollection{doc: d, scripts: map[string]string{}}
	if d.catalog == nil {
		return c
	}
	namesRaw, ok := d.catalog["/Names"]
	if !ok {
		return c
	}
	namesDict, ok := resolveRefToDict(d.objects, namesRaw)
	if !ok {
		return c
	}
	jsRaw, ok := namesDict["/JavaScript"]
	if !ok {
		return c
	}
	walkNameTree(d, jsRaw, func(name string, val pdfValue) {
		if _, exists := c.scripts[name]; !exists {
			c.order = append(c.order, name)
		}
		c.scripts[name] = jsScriptFromValue(d, val)
	})
	return c
}

// jsScriptFromValue resolves a name-tree value (a /JavaScript action
// dict, possibly behind an indirect ref) to its script text. The /JS
// entry may be a literal string or a stream per ISO 32000-1 §7.9.2.
func jsScriptFromValue(d *Document, raw pdfValue) string {
	switch v := raw.(type) {
	case pdfRef:
		if obj, ok := d.objects[v.Num]; ok {
			return jsScriptFromValue(d, obj.Value)
		}
	case pdfDict:
		return jsStringFromJS(d, v["/JS"])
	}
	return ""
}

// jsStringFromJS decodes a /JS value (string, hex string, stream, or
// indirect ref to a stream) into plain script text.
func jsStringFromJS(d *Document, raw pdfValue) string {
	switch v := raw.(type) {
	case string:
		return decodeFormString(v)
	case pdfHexString:
		return decodeFormString(v)
	case *pdfStream:
		return string(v.Data)
	case pdfRef:
		if obj, ok := d.objects[v.Num]; ok {
			return jsStringFromJS(d, obj.Value)
		}
	}
	return ""
}

// Count reports how many named scripts are in the collection.
func (c *JavaScriptCollection) Count() int { return len(c.order) }

// Has reports whether a script with the given name exists.
func (c *JavaScriptCollection) Has(name string) bool {
	_, ok := c.scripts[name]
	return ok
}

// Get returns the script registered under name, or "" if absent.
func (c *JavaScriptCollection) Get(name string) string { return c.scripts[name] }

// Names returns the script names in lexical order (the order they are
// written to the name tree).
func (c *JavaScriptCollection) Names() []string {
	out := make([]string, len(c.order))
	copy(out, c.order)
	sort.Strings(out)
	return out
}

// Add registers (or replaces) a named script and writes it through to the
// /Catalog/Names/JavaScript name tree. An empty name is rejected.
func (c *JavaScriptCollection) Add(name, script string) error {
	if name == "" {
		return fmt.Errorf("JavaScript.Add: empty name")
	}
	if _, exists := c.scripts[name]; !exists {
		c.order = append(c.order, name)
	}
	c.scripts[name] = script
	c.writeBack()
	return nil
}

// Remove deletes the named script. Returns true if it existed.
func (c *JavaScriptCollection) Remove(name string) bool {
	if _, ok := c.scripts[name]; !ok {
		return false
	}
	delete(c.scripts, name)
	for i, n := range c.order {
		if n == name {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.writeBack()
	return true
}

// Clear removes every named script.
func (c *JavaScriptCollection) Clear() {
	c.scripts = map[string]string{}
	c.order = nil
	c.writeBack()
}

// writeBack rebuilds /Catalog/Names/JavaScript as a single flat name-tree
// leaf (valid for any size per ISO 32000-1 §7.9.6) with entries sorted by
// name. When the collection is empty the /JavaScript subentry is removed.
func (c *JavaScriptCollection) writeBack() {
	nd := c.doc.namesDict()
	if len(c.order) == 0 {
		delete(nd, "/JavaScript")
		return
	}
	names := make([]string, len(c.order))
	copy(names, c.order)
	sort.Strings(names)
	arr := make(pdfArray, 0, len(names)*2)
	for _, name := range names {
		arr = append(arr, name, pdfDict{
			"/Type": pdfName("/Action"),
			"/S":    pdfName("/JavaScript"),
			"/JS":   c.scripts[name],
		})
	}
	nd["/JavaScript"] = pdfDict{"/Names": arr}
}

// namesDict returns the catalog's /Names dict as a live, mutable pdfDict,
// creating it (as a direct dict) when absent. When /Names is an indirect
// reference the referenced object's dict is returned and mutated in place
// (the writer carries it through); when direct, the catalog's own dict is
// returned. Either way mutations persist on Save.
func (d *Document) namesDict() pdfDict {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	switch v := d.catalog["/Names"].(type) {
	case pdfDict:
		return v
	case pdfRef:
		if obj, ok := d.objects[v.Num]; ok {
			if dict, ok := obj.Value.(pdfDict); ok {
				return dict
			}
		}
	}
	nd := pdfDict{}
	d.catalog["/Names"] = nd
	return nd
}
