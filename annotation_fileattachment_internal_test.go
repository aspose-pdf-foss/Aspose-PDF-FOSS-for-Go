// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// TestFileAttachmentAnnotationDeleteDropsAF covers pdf-go-tdx5's cleanup
// requirement: a FileAttachmentAnnotation's filespec listed in the catalog
// /AF (SetAFRelationship) is itself a GC root, so deleting only the
// annotation and leaving the /AF entry would keep the filespec — and its
// EmbeddedFile stream — reachable and orphaned in the saved file forever.
func TestFileAttachmentAnnotationDeleteDropsAF(t *testing.T) {
	doc := NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fa := NewFileAttachmentAnnotation(p, Point{X: 10, Y: 10})
	if err := fa.SetFileFromStream(strings.NewReader("x"), "x.txt"); err != nil {
		t.Fatal(err)
	}
	if err := p.Annotations().Add(fa); err != nil {
		t.Fatal(err)
	}
	fa.SetAFRelationship(AFData)

	ref, _ := fa.resolveFilespecRef()
	if ref.Num == 0 || !doc.isAssociatedFile(ref) {
		t.Fatal("setup: filespec not listed in /AF")
	}

	if !p.Annotations().Delete(fa) {
		t.Fatal("Delete returned false")
	}
	if doc.isAssociatedFile(ref) {
		t.Error("filespec still listed in catalog /AF after annotation delete")
	}
}
