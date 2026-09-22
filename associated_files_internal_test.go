// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

func catalogAFNums(d *Document) []int {
	var nums []int
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok {
			nums = append(nums, r.Num)
		}
	}
	return nums
}

func TestAFRelationshipSetAndList(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("data.csv", strings.NewReader("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.AFRelationship(); got != AFUnspecified {
		t.Errorf("new attachment relationship = %v, want AFUnspecified", got)
	}
	if len(catalogAFNums(doc)) != 0 {
		t.Error("an attachment with no relationship was listed in /AF")
	}

	f.SetAFRelationship(AFData)
	f = doc.EmbeddedFiles().Get("data.csv")
	if got := f.AFRelationship(); got != AFData {
		t.Errorf("relationship = %v, want AFData", got)
	}
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != f.ref.Num {
		t.Fatalf("/AF = %v, want exactly the file specification %d", n, f.ref.Num)
	}

	// Changing the relationship must not list the file twice.
	f.SetAFRelationship(AFSource)
	if n := catalogAFNums(doc); len(n) != 1 {
		t.Errorf("/AF has %d entries after a second SetAFRelationship, want 1", len(n))
	}
	if name, _ := f.filespec["/AFRelationship"].(pdfName); name != "/Source" {
		t.Errorf("/AFRelationship = %v, want /Source", f.filespec["/AFRelationship"])
	}
}

func TestAFRelationshipRoundTrip(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("source.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	f.SetAFRelationship(AFAlternative)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	g := back.EmbeddedFiles().Get("source.txt")
	if g == nil {
		t.Fatal("attachment lost in the round trip")
	}
	if got := g.AFRelationship(); got != AFAlternative {
		t.Errorf("relationship after round trip = %v, want AFAlternative", got)
	}
	if !back.isAssociatedFile(g.ref) {
		t.Error("the attachment is not listed in /AF after the round trip")
	}
}

func TestRemovingAttachmentRemovesItFromAF(t *testing.T) {
	doc := NewDocument(200, 200)
	a, _ := doc.EmbeddedFiles().AddFromStream("a.txt", strings.NewReader("a"))
	b, _ := doc.EmbeddedFiles().AddFromStream("b.txt", strings.NewReader("b"))
	a.SetAFRelationship(AFData)
	b.SetAFRelationship(AFData)
	doc.EmbeddedFiles().Remove("a.txt")
	if n := catalogAFNums(doc); len(n) != 1 || n[0] != b.ref.Num {
		t.Errorf("/AF after removing a.txt = %v, want only b.txt's %d", n, b.ref.Num)
	}
	doc.EmbeddedFiles().Clear()
	if _, ok := doc.catalog["/AF"]; ok {
		t.Error("/AF survived Clear")
	}
}

// PDF/A-3 requires /ModDate in an embedded file's parameters.
func TestEmbeddedFileHasModDate(t *testing.T) {
	doc := NewDocument(200, 200)
	f, err := doc.EmbeddedFiles().AddFromStream("x.txt", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := f.stream().Dict["/Params"].(pdfDict)
	md, _ := params["/ModDate"].(string)
	if !strings.HasPrefix(md, "D:") {
		t.Errorf("/Params /ModDate = %q, want a PDF date", md)
	}
}
