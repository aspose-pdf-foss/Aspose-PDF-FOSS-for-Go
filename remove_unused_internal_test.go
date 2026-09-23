// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// outlinedDocBytes saves a one-page document with three bookmarks whose
// titles carry a searchable marker.
func outlinedDocBytes(t *testing.T) []byte {
	t.Helper()
	doc := NewDocument(300, 300)
	for i := 0; i < 3; i++ {
		item := NewOutlineItemCollection(doc)
		item.SetTitle(fmt.Sprintf("OLDTITLE-%d", i))
		if err := doc.Outlines().Add(item); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reopen(t *testing.T, data []byte) *Document {
	t.Helper()
	doc, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The catalog's /Pages names an object number that is free on a reopened
// file (the page tree is rebuilt on save), and the next new object takes it.
// Following /Pages from the catalog kept that object alive forever.
func TestRemoveUnusedObjectsIgnoresStalePagesNumber(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	stale, ok := doc.catalog["/Pages"].(pdfRef)
	if !ok {
		t.Fatal("the reopened catalog has no /Pages reference")
	}
	// An orphan created now lands on the old /Pages number when the numbers
	// line up — force it so the test does not depend on the layout.
	doc.objects[stale.Num] = &pdfObject{Num: stale.Num, Value: pdfDict{"/Orphan": true}}
	doc.RemoveUnusedObjects()
	if _, kept := doc.objects[stale.Num]; kept {
		t.Error("an unreferenced object was kept because it reused the old /Pages number")
	}
}

// Once bookmarks are loaded and edited, the writer rebuilds the tree; the
// parsed one must not survive RemoveUnusedObjects as dead weight.
func TestRemoveUnusedObjectsDropsReplacedOutlineTree(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	for _, item := range doc.Outlines().All() {
		item.SetTitle("renamed")
	}
	doc.RemoveUnusedObjects()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("OLDTITLE")) {
		t.Error("the replaced outline tree is still in the output")
	}
	back := reopen(t, buf.Bytes())
	if n := back.Outlines().Count(); n != 3 {
		t.Fatalf("%d bookmarks after the round trip, want 3", n)
	}
	if got := back.Outlines().At(0).Title(); got != "renamed" {
		t.Errorf("bookmark title = %q, want renamed", got)
	}
}

// Untouched bookmarks are only reachable from the catalog; they must survive.
func TestRemoveUnusedObjectsKeepsUntouchedOutlines(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	doc.RemoveUnusedObjects()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if n := reopen(t, buf.Bytes()).Outlines().Count(); n != 3 {
		t.Errorf("%d bookmarks after RemoveUnusedObjects, want 3", n)
	}
}

// Removing every bookmark must remove them from the saved file; the catalog
// used to keep pointing at the parsed tree, so they all came back.
func TestRemovingAllOutlinesRemovesThem(t *testing.T) {
	doc := reopen(t, outlinedDocBytes(t))
	for doc.Outlines().Count() > 0 {
		if err := doc.Outlines().RemoveAt(0); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if n := reopen(t, buf.Bytes()).Outlines().Count(); n != 0 {
		t.Errorf("%d bookmarks came back after removing all of them", n)
	}
}
