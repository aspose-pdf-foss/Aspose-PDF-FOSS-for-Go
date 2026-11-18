// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// helloWorldDoc is a page with "Hello world" (Helvetica 12) at (100, 600)
// and "other line" at (100, 500), reopened so the text is found text.
func helloWorldDoc(t *testing.T) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for _, l := range []struct {
		s string
		y float64
	}{{"Hello world", 600}, {"other line", 500}} {
		f := NewTextFragment(l.s)
		f.Position = Position{XIndent: 100, YIndent: l.y}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	return reopenDoc(t, d)
}

func firstFound(t *testing.T, a *TextFragmentAbsorber, d *Document) *TextFragment {
	t.Helper()
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) == 0 {
		t.Fatal("nothing found")
	}
	return fs[0]
}

func savedText(t *testing.T, d *Document) string {
	t.Helper()
	txt, err := mustPage(t, reopenDoc(t, d)).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	return txt
}

// The same words found twice and changed twice: the second change would
// draw over the first. Refused before anything changes.
func TestApplyRefusesOverlappingEdits(t *testing.T) {
	d := helloWorldDoc(t)
	before := writeBytes(t, d)
	a1, a2 := NewTextFragmentAbsorber("Hello"), NewTextFragmentAbsorber("Hello")
	f1, f2 := firstFound(t, a1, d), firstFound(t, a2, d)
	f1.Text, f2.Text = "Bye", "Hi"
	_, err := d.WriteTo(&strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "overlaps") || !strings.Contains(err.Error(), `new text "Bye"`) || !strings.Contains(err.Error(), `new text "Hi"`) {
		t.Fatalf("got %v", err)
	}
	f2.Text = "Hello"
	if txt := savedText(t, d); !strings.Contains(txt, "Bye world") || strings.Contains(txt, "Hi") {
		t.Fatalf("text %q", txt)
	}
	// Refusal changed nothing: undoing both edits writes the original.
	d = openBytes(t, before)
	f1, f2 = firstFound(t, NewTextFragmentAbsorber("Hello"), d), firstFound(t, NewTextFragmentAbsorber("Hello"), d)
	f1.Text, f2.Text = "Bye", "Hi"
	if err := d.ApplyTextChanges(); err == nil {
		t.Fatal("expected refusal")
	}
	f1.Text, f2.Text = "Hello", "Hello"
	if string(writeBytes(t, d)) != string(before) {
		t.Fatal("refused apply changed the document")
	}
}

// A visit applies pending edits first, so searching again after an edit
// finds the page as it will be saved (as in Aspose, where edits are
// immediate).
func TestVisitAppliesPendingEdits(t *testing.T) {
	d := helloWorldDoc(t)
	firstFound(t, NewTextFragmentAbsorber("Hello"), d).Text = "Bye"
	a := NewTextFragmentAbsorber("Hello")
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	if len(a.TextFragments()) != 0 {
		t.Fatal("found text replaced before the visit")
	}
	if got := firstFound(t, NewTextFragmentAbsorber("Bye"), d).Text; got != "Bye" {
		t.Fatalf("found %q", got)
	}
	if txt := savedText(t, d); !strings.Contains(txt, "Bye world") {
		t.Fatalf("text %q", txt)
	}
}

// A visit returns the error of the pending edits it applies.
func TestVisitReturnsApplyError(t *testing.T) {
	d := helloWorldDoc(t)
	f1 := firstFound(t, NewTextFragmentAbsorber("Hello"), d)
	f2 := firstFound(t, NewTextFragmentAbsorber("Hello"), d)
	f1.Text, f2.Text = "Bye", "Hi"
	p := mustPage(t, d)
	if err := NewTextFragmentAbsorber("world").Visit(p); err == nil {
		t.Fatal("Visit ignored the apply error")
	}
	if err := NewTextFragmentAbsorber("world").VisitDocument(d); err == nil {
		t.Fatal("VisitDocument ignored the apply error")
	}
}

// A line found before a word in it was edited no longer describes the page:
// changing it is refused instead of redrawing the old words.
func TestStaleLineFragmentRefused(t *testing.T) {
	d := helloWorldDoc(t)
	all := NewTextFragmentAbsorber("")
	if err := all.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	other := all.TextFragments()[1]
	if other.Text != "other line" {
		t.Fatalf("second line %q", other.Text)
	}
	firstFound(t, NewTextFragmentAbsorber("Hello"), d).Text = "Bye"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	line := all.TextFragments()[0]
	line.TextState.ForegroundColor = &Color{R: 1, A: 1}
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Fatalf("got %v", err)
	}
	line.TextState.ForegroundColor = &Color{A: 1}
	// A fragment the edit did not touch stays editable.
	other.Text = "edited line"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatalf("untouched fragment refused: %v", err)
	}
	txt := savedText(t, d)
	if !strings.Contains(txt, "Bye world") || strings.Contains(txt, "Hello") || !strings.Contains(txt, "edited line") {
		t.Fatalf("text %q", txt)
	}
}

func TestAbsorberResultsReplaceAndReset(t *testing.T) {
	d := helloWorldDoc(t)
	a := NewTextFragmentAbsorber("l")
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	n := len(a.TextFragments())
	if n == 0 {
		t.Fatal("nothing found")
	}
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	if len(a.TextFragments()) != n {
		t.Fatalf("visit accumulated: %d then %d", n, len(a.TextFragments()))
	}
	fs := a.TextFragments()
	fs[0] = nil
	if a.TextFragments()[0] == nil {
		t.Fatal("TextFragments returned the absorber's own slice")
	}
	a.Reset()
	if len(a.TextFragments()) != 0 {
		t.Fatal("Reset kept fragments")
	}
}
