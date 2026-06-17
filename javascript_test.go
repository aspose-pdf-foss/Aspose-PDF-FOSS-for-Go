// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestDocumentJavaScriptAddGet(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	js := doc.JavaScript()
	if js.Count() != 0 {
		t.Fatalf("fresh doc Count = %d, want 0", js.Count())
	}
	if err := js.Add("welcome", "app.alert('hi');"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := js.Add("init", "console.println('init');"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if js.Count() != 2 {
		t.Errorf("Count = %d, want 2", js.Count())
	}
	if !js.Has("welcome") {
		t.Error("Has(welcome) = false")
	}
	if got := js.Get("welcome"); got != "app.alert('hi');" {
		t.Errorf("Get(welcome) = %q", got)
	}
	if err := js.Add("", "x"); err == nil {
		t.Error("Add with empty name = nil error, want error")
	}
}

func TestDocumentJavaScriptRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	js := doc.JavaScript()
	mustNoErr(t, js.Add("b_script", "var b = 2;"))
	mustNoErr(t, js.Add("a_script", "var a = 1;"))

	doc2 := reopen(t, doc)
	js2 := doc2.JavaScript()
	if js2.Count() != 2 {
		t.Fatalf("after reopen Count = %d, want 2", js2.Count())
	}
	// Names come back lex-sorted (name-tree order).
	names := js2.Names()
	if len(names) != 2 || names[0] != "a_script" || names[1] != "b_script" {
		t.Errorf("Names = %v, want [a_script b_script]", names)
	}
	if got := js2.Get("a_script"); got != "var a = 1;" {
		t.Errorf("Get(a_script) = %q", got)
	}
	if got := js2.Get("b_script"); got != "var b = 2;" {
		t.Errorf("Get(b_script) = %q", got)
	}
}

func TestDocumentJavaScriptRemoveClear(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	js := doc.JavaScript()
	mustNoErr(t, js.Add("one", "1;"))
	mustNoErr(t, js.Add("two", "2;"))

	if !js.Remove("one") {
		t.Error("Remove(one) = false")
	}
	if js.Remove("nope") {
		t.Error("Remove(nope) = true, want false")
	}
	if js.Count() != 1 || js.Has("one") {
		t.Errorf("after Remove Count=%d Has(one)=%v", js.Count(), js.Has("one"))
	}

	// Removal survives a round-trip (the /JavaScript subentry is rewritten).
	doc2 := reopen(t, doc)
	if doc2.JavaScript().Has("one") {
		t.Error("removed script reappeared after reopen")
	}
	if !doc2.JavaScript().Has("two") {
		t.Error("kept script lost after reopen")
	}

	js.Clear()
	if js.Count() != 0 {
		t.Errorf("after Clear Count = %d, want 0", js.Count())
	}
	doc3 := reopen(t, doc)
	if doc3.JavaScript().Count() != 0 {
		t.Errorf("after Clear+reopen Count = %d, want 0", doc3.JavaScript().Count())
	}
}

func TestDocumentJavaScriptCoexistsWithNamedDest(t *testing.T) {
	// Named destinations and document JavaScript both live under
	// /Catalog/Names — verify they don't clobber each other on Save.
	doc := pdf.NewDocument(400, 400)
	mustNoErr(t, doc.AddBlankPage(400, 400))
	p2, _ := doc.Page(2)
	mustNoErr(t, doc.NamedDestinations().Add("chapter2", pdf.NewDestinationFit(p2)))
	mustNoErr(t, doc.JavaScript().Add("greet", "app.alert('x');"))

	doc2 := reopen(t, doc)
	if !doc2.JavaScript().Has("greet") {
		t.Error("JavaScript lost after coexisting with named dest")
	}
	if !doc2.NamedDestinations().Has("chapter2") {
		t.Error("named destination lost after coexisting with JavaScript")
	}
}

func TestOpenActionGoTo(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	mustNoErr(t, doc.AddBlankPage(400, 400))
	mustNoErr(t, doc.AddBlankPage(400, 400))

	doc.SetOpenAction(pdf.NewGoToAction(3, 750))

	doc2 := reopen(t, doc)
	act := doc2.OpenAction()
	if act == nil {
		t.Fatal("OpenAction = nil after reopen")
	}
	if act.ActionType() != pdf.ActionTypeGoTo {
		t.Fatalf("ActionType = %v, want GoTo", act.ActionType())
	}
	gt := act.(*pdf.GoToAction)
	if gt.PageNum() != 3 {
		t.Errorf("PageNum = %d, want 3 (page ref must resolve)", gt.PageNum())
	}
}

func TestOpenActionJavaScript(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	doc.SetOpenAction(pdf.NewJavaScriptAction("app.alert('opened');"))

	doc2 := reopen(t, doc)
	act := doc2.OpenAction()
	if act == nil || act.ActionType() != pdf.ActionTypeJavaScript {
		t.Fatalf("OpenAction = %v, want JavaScript action", act)
	}
	if got := act.(*pdf.JavaScriptAction).Script(); got != "app.alert('opened');" {
		t.Errorf("Script = %q", got)
	}
}

func TestOpenActionClear(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	doc.SetOpenAction(pdf.NewNamedAction(pdf.NamedActionPrint))
	if doc.OpenAction() == nil {
		t.Fatal("OpenAction = nil right after set")
	}
	doc.SetOpenAction(nil)
	if doc.OpenAction() != nil {
		t.Error("OpenAction not nil after SetOpenAction(nil)")
	}
	doc2 := reopen(t, doc)
	if doc2.OpenAction() != nil {
		t.Error("OpenAction reappeared after clear + reopen")
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
