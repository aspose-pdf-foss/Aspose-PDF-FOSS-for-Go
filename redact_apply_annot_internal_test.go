// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestApplyRedactionsRedactsWidgetAppearance pins the highest-value case of
// pdf-go-xp2u: a form field's own value, drawn via its widget's /AP
// appearance stream (not the page's /Contents), sitting under a
// redaction region -- think a text field showing an SSN. Before this,
// ApplyRedactions never looked at any annotation's /AP at all, so the
// widget's rendered value survived completely untouched: visible on
// render, and sitting in the appearance stream's own content bytes for
// anyone who decodes the saved file directly.
//
// Checked at the level that actually matters here: the appearance
// stream's own content bytes, not the field's /V value (which this
// method deliberately leaves alone, same as it leaves a FreeText
// annotation's /Contents metadata alone -- /V is the field's data model,
// a separate and legitimate thing from what got drawn and rendered).
func TestApplyRedactionsRedactsWidgetAppearance(t *testing.T) {
	const secret = "123-45-6789"

	d := NewDocumentFromFormat(PageFormatA4)
	rect := Rectangle{LLX: 50, LLY: 650, URX: 250, URY: 680}
	field, err := d.Form().AddTextField(1, rect, "ssn")
	if err != nil {
		t.Fatal(err)
	}
	if err := field.SetValue(secret); err != nil {
		t.Fatal(err)
	}

	apBefore, ok := resolveRefToDict(d.objects, field.node.dict["/AP"])
	if !ok {
		t.Fatal("widget has no /AP after SetValue")
	}
	stBefore, ok := resolveRef(d.objects, apBefore["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("widget /AP/N did not resolve to a stream")
	}
	if !bytes.Contains(decodedStreamData(stBefore), []byte(secret)) {
		t.Fatal("sanity check failed: widget appearance does not draw the secret before redaction")
	}

	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(p, rect)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	// Re-resolve: the widget dict itself is untouched (AddTextField put
	// the field+widget in one combined dict for a single-widget field),
	// but its /AP should now be a fresh dict pointing at a cloned,
	// redacted stream.
	apAfter, ok := resolveRefToDict(d.objects, field.node.dict["/AP"])
	if !ok {
		t.Fatal("widget lost its /AP entirely")
	}
	stAfter, ok := resolveRef(d.objects, apAfter["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("widget /AP/N no longer resolves to a stream")
	}
	if bytes.Contains(decodedStreamData(stAfter), []byte(secret)) {
		t.Error("widget appearance stream still draws the secret after ApplyRedactions")
	}

	// The clone must be a NEW object, not the original mutated in place
	// (which would corrupt any other reference to the same stream, per
	// the whole redactFormDo/redactFormXObject sharing discipline this
	// reuses).
	if stAfter == stBefore {
		t.Error("appearance stream was mutated in place instead of cloned")
	}

	// Round-trips: the saved+reopened file's widget appearance stream
	// still doesn't draw the secret (/V legitimately still carries it,
	// out of scope for this fix — same as a FreeText annotation's own
	// /Contents metadata — so a whole-document byte scan is not the
	// right check here; the appearance stream specifically is).
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	field2 := reopened.Form().Field("ssn")
	if field2 == nil {
		t.Fatal("field did not survive round-trip")
	}
	tb2 := field2.(*TextBoxField)
	apReopened, ok := resolveRefToDict(reopened.objects, tb2.node.dict["/AP"])
	if !ok {
		t.Fatal("reopened widget has no /AP")
	}
	stReopened, ok := resolveRef(reopened.objects, apReopened["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("reopened widget /AP/N did not resolve to a stream")
	}
	if bytes.Contains(decodedStreamData(stReopened), []byte(secret)) {
		t.Error("secret recoverable from the widget appearance stream in the saved+reopened file")
	}
}

// TestApplyRedactionsLeavesNonOverlappingAnnotationAppearanceUntouched
// confirms the converse of the widget test above: an annotation whose
// /Rect does not overlap any redaction region must come out of
// ApplyRedactions with its /AP completely unchanged -- not merely
// visually equivalent, but the SAME stream object, never resolved,
// cloned, or redirected at all. redactAnnotationAppearances' coarse
// /Rect-vs-region pre-check exists specifically to guarantee this: an
// annotation nowhere near any region should cost this pass nothing more
// than one cheap bbox comparison.
func TestApplyRedactionsLeavesNonOverlappingAnnotationAppearanceUntouched(t *testing.T) {
	const farText = "unrelated note, far from any redaction"

	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	farRect := Rectangle{LLX: 50, LLY: 50, URX: 250, URY: 90}
	fta := NewFreeTextAnnotation(p, farRect, farText, TextStyle{Size: 12})
	if err := p.Annotations().Add(fta); err != nil {
		t.Fatal(err)
	}
	stBefore, ok := resolveRef(d.objects, fta.annotationBaseRef().dict["/AP"]).(pdfDict)
	if !ok {
		t.Fatal("FreeText annotation has no /AP dict")
	}
	nBefore, ok := resolveRef(d.objects, stBefore["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("FreeText /AP/N did not resolve to a stream")
	}

	// A redaction region nowhere near farRect.
	secretRect := Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 730}
	if err := p.AddText("TOP-SECRET-ELSEWHERE", TextStyle{Size: 12}, secretRect); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(p, secretRect)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	apAfterVal := fta.annotationBaseRef().dict["/AP"]
	apAfter, ok := resolveRef(d.objects, apAfterVal).(pdfDict)
	if !ok {
		t.Fatal("FreeText lost its /AP dict")
	}
	nAfter, ok := resolveRef(d.objects, apAfter["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("FreeText /AP/N no longer resolves to a stream")
	}
	if nAfter != nBefore {
		t.Error("an annotation nowhere near any redaction region had its appearance cloned/replaced")
	}
	if !bytes.Contains(decodedStreamData(nAfter), []byte("unrelated")) {
		t.Error("unrelated annotation's visible text was lost")
	}
}

// TestValidateRedactionsFlagsUnsafeAnnotationAppearance confirms
// ValidateRedactions' pre-flight check catches a malformed/third-party
// annotation appearance (no resolvable /BBox, which ISO 32000-1 §8.10.2
// requires on every conformant Form XObject) overlapping a redaction
// region -- the same "refuse rather than guess" stance
// unsafeFormXObjectName already takes for a page-content Form XObject,
// extended to annotation /AP streams. ApplyRedactions itself must refuse
// the same way, not silently skip it.
func TestValidateRedactionsFlagsUnsafeAnnotationAppearance(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := Rectangle{LLX: 50, LLY: 650, URX: 250, URY: 680}
	fta := NewFreeTextAnnotation(p, rect, "note", TextStyle{Size: 12})
	if err := p.Annotations().Add(fta); err != nil {
		t.Fatal(err)
	}

	apDict, ok := resolveRefToDict(d.objects, fta.annotationBaseRef().dict["/AP"])
	if !ok {
		t.Fatal("FreeText annotation has no /AP dict")
	}
	nStream, ok := resolveRef(d.objects, apDict["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("FreeText /AP/N did not resolve to a stream")
	}
	delete(nStream.Dict, "/BBox") // simulate a malformed/third-party appearance stream

	ra := NewRedactAnnotation(p, rect)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ValidateRedactions(); err == nil {
		t.Error("ValidateRedactions did not flag the malformed annotation appearance")
	}
	if err := d.ApplyRedactions(); err == nil {
		t.Error("ApplyRedactions did not refuse on the malformed annotation appearance")
	}
}
