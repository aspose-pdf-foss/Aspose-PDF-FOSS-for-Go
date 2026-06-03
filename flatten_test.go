// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func pageText(t *testing.T, doc *asposepdf.Document, page int) string {
	t.Helper()
	texts, err := doc.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if page < 1 || page > len(texts) {
		t.Fatalf("page %d out of range (%d pages)", page, len(texts))
	}
	return texts[page-1]
}

func TestFormFlattenRemovesFieldsAndBakesValue(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	field, err := doc.Form().AddTextField(1, asposepdf.Rectangle{LLX: 50, LLY: 200, URX: 300, URY: 230}, "name")
	if err != nil {
		t.Fatalf("AddTextField: %v", err)
	}
	if err := field.SetValue("Hello"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if got := len(doc.Form().Fields()); got != 1 {
		t.Fatalf("before flatten: %d fields, want 1", got)
	}

	if err := doc.Flatten(); err != nil {
		t.Fatalf("Flatten: %v", err)
	}

	if got := len(doc.Form().Fields()); got != 0 {
		t.Errorf("after flatten: %d fields, want 0", got)
	}
	if doc.Form().HasField("name") {
		t.Error("HasField(\"name\") true after flatten")
	}

	// Save + reopen: no form survives, value is baked into page content.
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := asposepdf.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := len(re.Form().Fields()); got != 0 {
		t.Errorf("reloaded: %d fields, want 0", got)
	}
	if txt := pageText(t, re, 1); !strings.Contains(txt, "Hello") {
		t.Errorf("baked field value not found in reloaded page text: %q", txt)
	}
}

func TestDocumentFlattenNoForm(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	if err := doc.Flatten(); err != nil {
		t.Errorf("Flatten on a document without a form should be a no-op: %v", err)
	}
}

func TestFieldFlattenLeavesOtherFields(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	fa, err := doc.Form().AddTextField(1, asposepdf.Rectangle{LLX: 50, LLY: 220, URX: 300, URY: 250}, "a")
	if err != nil {
		t.Fatalf("AddTextField a: %v", err)
	}
	if _, err := doc.Form().AddTextField(1, asposepdf.Rectangle{LLX: 50, LLY: 170, URX: 300, URY: 200}, "b"); err != nil {
		t.Fatalf("AddTextField b: %v", err)
	}
	if err := fa.SetValue("AVAL"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if err := fa.Flatten(); err != nil {
		t.Fatalf("Field.Flatten: %v", err)
	}

	form := doc.Form()
	if form.HasField("a") {
		t.Error(`field "a" should be gone after Field.Flatten`)
	}
	if !form.HasField("b") {
		t.Error(`field "b" must survive flattening only "a"`)
	}
	if got := len(form.Fields()); got != 1 {
		t.Errorf("fields after flatten: %d, want 1", got)
	}
	if txt := pageText(t, doc, 1); !strings.Contains(txt, "AVAL") {
		t.Errorf("flattened field value not baked: %q", txt)
	}
}

func TestAnnotationFlattenBakesAndRemoves(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	page, _ := doc.Page(1)
	ft := asposepdf.NewFreeTextAnnotation(page,
		asposepdf.Rectangle{LLX: 50, LLY: 150, URX: 320, URY: 190},
		"Stamped",
		asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := page.Annotations().Count(); got != 1 {
		t.Fatalf("before flatten: %d annotations, want 1", got)
	}

	if err := ft.Flatten(); err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if got := page.Annotations().Count(); got != 0 {
		t.Errorf("after flatten: %d annotations, want 0", got)
	}

	out := filepath.Join(t.TempDir(), "annot.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := asposepdf.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rp, _ := re.Page(1)
	if got := rp.Annotations().Count(); got != 0 {
		t.Errorf("reloaded: %d annotations, want 0", got)
	}
	if txt := pageText(t, re, 1); !strings.Contains(txt, "Stamped") {
		t.Errorf("baked free-text not found in reloaded page text: %q", txt)
	}
}

func TestAnnotationFlattenNotAttached(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	page, _ := doc.Page(1)
	ft := asposepdf.NewFreeTextAnnotation(page,
		asposepdf.Rectangle{LLX: 50, LLY: 150, URX: 320, URY: 190},
		"x", asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12})
	// Not added to the page yet.
	if err := ft.Flatten(); err == nil {
		t.Error("Flatten on an unattached annotation should error")
	}
}

func TestAnnotationCollectionFlattenSkipsWidgets(t *testing.T) {
	doc := asposepdf.NewDocument(400, 300)
	field, err := doc.Form().AddTextField(1, asposepdf.Rectangle{LLX: 50, LLY: 220, URX: 300, URY: 250}, "f1")
	if err != nil {
		t.Fatalf("AddTextField: %v", err)
	}
	if err := field.SetValue("FieldVal"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	page, _ := doc.Page(1)
	ft := asposepdf.NewFreeTextAnnotation(page,
		asposepdf.Rectangle{LLX: 50, LLY: 150, URX: 320, URY: 190},
		"FreeVal", asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12})
	if err := page.Annotations().Add(ft); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Page now carries the field's widget plus the free-text annotation.
	if got := page.Annotations().Count(); got != 2 {
		t.Fatalf("before: %d annotations, want 2 (widget + free text)", got)
	}

	if err := page.Annotations().Flatten(); err != nil {
		t.Fatalf("Flatten: %v", err)
	}

	// The free text is flattened away; the form widget is left for Form.Flatten.
	if got := page.Annotations().Count(); got != 1 {
		t.Errorf("after: %d annotations, want 1 (widget remains)", got)
	}
	if !doc.Form().HasField("f1") {
		t.Error("form field must survive AnnotationCollection.Flatten")
	}
}
