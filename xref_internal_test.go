// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// sectionIsXRefStream must fully confirm the object at the offset is really
// a /Type /XRef stream, unlike isXRefStream's cheap "not the xref keyword"
// negative test.
func TestSectionIsXRefStream(t *testing.T) {
	xrefStreamObj := []byte("5 0 obj\n<< /Type /XRef /Size 1 /W [1 1 1] >>\nstream\nABC\nendstream\nendobj\n")
	ordinaryObj := []byte("5 0 obj\n<< /Type /Catalog /Pages 1 0 R >>\nendobj\n")
	classicXref := []byte("xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 1 >>\n")

	if !sectionIsXRefStream(xrefStreamObj, 0) {
		t.Error("a real /Type /XRef stream object should report true")
	}
	if sectionIsXRefStream(classicXref, 0) {
		t.Error("the \"xref\" keyword should report false")
	}
	if sectionIsXRefStream(ordinaryObj, 0) {
		t.Error("an ordinary non-XRef object should report false")
	}
	if sectionIsXRefStream(xrefStreamObj, int64(len(xrefStreamObj))+100) {
		t.Error("an out-of-range offset should report false")
	}
	if sectionIsXRefStream(xrefStreamObj, -1) {
		t.Error("a negative offset should report false")
	}
}
