// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func TestRemoveImage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 image, got %d", len(infos))
	}

	err := infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify /XObject dict no longer has /Im0.
	resources := page.pageResources()
	xobjVal := resolveRef(page.doc.objects, resources["/XObject"])
	xobjDict, _ := xobjVal.(pdfDict)
	if xobjDict != nil {
		if _, exists := xobjDict["/Im0"]; exists {
			t.Error("/Im0 should be removed from XObject resources")
		}
	}

	// Verify content stream no longer has Do.
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "Do") {
		t.Error("content stream should not contain Do after removal")
	}
}

func TestRemoveImageNestedQ(t *testing.T) {
	// Content stream with nested q/Q: outer text block + inner image block.
	contentData := "q\nBT\n/F1 12 Tf\n100 700 Td\n(Hello) Tj\nET\nQ\nq\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"

	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    []byte{0xFF, 0xD8, 0xFF, 0xD9},
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
			"/Font": pdfDict{
				"/F1": pdfRef{Num: 99},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 image, got %d", len(infos))
	}

	err := infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Content stream should still have the text block but not the image Do.
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "Do") {
		t.Error("content stream should not contain Do after removal")
	}
	if !strings.Contains(content, "Tj") {
		t.Error("content stream should still contain Tj (text operator)")
	}
}

func TestRemoveImageInvalidInfo(t *testing.T) {
	info := &ImageInfo{}
	err := info.Remove()
	if err == nil {
		t.Fatal("expected error for nil page/stream")
	}
}

func TestSerializeContentOps(t *testing.T) {
	// Build a simple content stream, parse it, serialize it, parse again.
	original := "q\n10 0 0 20 50.5 100 cm\n/Im0 Do\nQ\n"
	ops, err := parseContentStream([]byte(original))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	serialized := serializeContentOps(ops)
	result := string(serialized)

	// Re-parse and verify structural equivalence.
	ops2, err := parseContentStream(serialized)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(ops2) != len(ops) {
		t.Fatalf("op count: got %d, want %d", len(ops2), len(ops))
	}

	for i, op := range ops {
		if ops2[i].Operator != op.Operator {
			t.Errorf("op[%d]: got %q, want %q", i, ops2[i].Operator, op.Operator)
		}
		if len(ops2[i].Operands) != len(op.Operands) {
			t.Errorf("op[%d] operands: got %d, want %d", i, len(ops2[i].Operands), len(op.Operands))
		}
	}

	// Verify key operators are present in output.
	if !strings.Contains(result, "cm") {
		t.Error("serialized should contain cm")
	}
	if !strings.Contains(result, "Do") {
		t.Error("serialized should contain Do")
	}
	if !strings.Contains(result, "/Im0") {
		t.Error("serialized should contain /Im0")
	}
}

func TestSerializeContentOpsWithText(t *testing.T) {
	// Content stream with text operators and TJ array.
	original := "BT\n/F1 12 Tf\n100 700 Td\n[(Hello) -50 (World)] TJ\nET\n"
	ops, err := parseContentStream([]byte(original))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	serialized := serializeContentOps(ops)
	ops2, err := parseContentStream(serialized)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(ops2) != len(ops) {
		t.Fatalf("op count: got %d, want %d", len(ops2), len(ops))
	}

	// Verify TJ operator preserved.
	found := false
	for _, op := range ops2 {
		if op.Operator == "TJ" {
			found = true
			if len(op.Operands) != 1 {
				t.Errorf("TJ operands: got %d, want 1", len(op.Operands))
			}
		}
	}
	if !found {
		t.Error("TJ operator not found in re-parsed output")
	}
}
