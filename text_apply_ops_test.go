// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// An in-place edit of segment 0's colour changes segment 0 only: the
// fragment's state does not share the segment's colour.
func TestApplyColourInPlaceSegmentOnly(t *testing.T) {
	d, _ := absorberDoc(t)
	f := absorbOne(t, d, "plain bold tail")
	f.Segments[0].TextState.ForegroundColor.R = 1
	if f.TextState.ForegroundColor.R != 0 {
		t.Fatal("fragment state shares the segment's colour")
	}
	pl, err := d.planTextEdit(f, newReplaceFonts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if pl == nil || pl.whole || len(pl.segs) != 1 || pl.segs[0] != 0 {
		t.Fatalf("plan %+v", pl)
	}
	r := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, r, "plain bold tail")
	if len(g.Segments) != 3 || g.Segments[0].TextState.ForegroundColor.R < 0.99 ||
		g.Segments[1].TextState.ForegroundColor.R > 0.01 || g.Segments[2].TextState.ForegroundColor.R > 0.01 {
		t.Fatal("colour edit reached other segments")
	}
	// After the commit the fragment, its segments and the snapshot still
	// share nothing.
	f.Segments[0].TextState.ForegroundColor.G = 1
	if f.TextState.ForegroundColor.G != 0 || f.bind.snap.segments[0].state.ForegroundColor.G != 0 {
		t.Fatal("colour shared after commit")
	}
}

// Untouched bound fragments cost no allocation per ApplyTextChanges call.
func TestApplyUntouchedAllocatesNothing(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for i := range 1000 {
		f := NewTextFragment("x")
		f.Position = Position{XIndent: 50 + float64(i%20)*25, YIndent: 800 - float64(i/20)*15}
		f.TextState.ForegroundColor = &Color{0, 0, 1, 1}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	if n := testing.AllocsPerRun(10, func() { _ = d.ApplyTextChanges() }); n != 0 {
		t.Fatalf("authored: %v allocations", n)
	}
	r := reopenDoc(t, d)
	a := NewTextFragmentAbsorber("x")
	if err := a.VisitDocument(r); err != nil {
		t.Fatal(err)
	}
	if len(r.boundFragments) < 1000 {
		t.Fatalf("found %d", len(r.boundFragments))
	}
	if n := testing.AllocsPerRun(10, func() { _ = r.ApplyTextChanges() }); n != 0 {
		t.Fatalf("found: %v allocations", n)
	}
}

func TestApplyRefusedOnIncrementalSign(t *testing.T) {
	cert, key := newTestSigner(t, "edit")
	src := applyDocBytes(t)

	d := openBytes(t, src)
	absorbOne(t, d, "Figure").Text = "Picture"
	if err := d.Sign(SignOptions{Certificate: cert, PrivateKey: key, Incremental: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err == nil || !strings.Contains(err.Error(), "text changes pending") {
		t.Fatalf("err %v", err)
	}
	if buf.Len() != 0 {
		t.Fatal("bytes written")
	}

	// Saved, reopened, then signed: fine.
	d = openBytes(t, src)
	absorbOne(t, d, "Figure").Text = "Picture"
	d = openBytes(t, writeBytes(t, d))
	absorbOne(t, d, "Picture") // found but untouched: no obstacle
	if err := d.Sign(SignOptions{Certificate: cert, PrivateKey: key, Incremental: true}); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	if absorbCount(t, r, "Picture") != 1 {
		t.Fatal("signed copy lost the edit")
	}
}

// Operations building documents from pages apply pending edits first.
func TestApplyBeforePageOperations(t *testing.T) {
	type op struct {
		name string
		run  func(t *testing.T, d *Document) *Document
	}
	for _, o := range []op{
		{"split", func(t *testing.T, d *Document) *Document {
			parts, err := d.Split()
			if err != nil {
				t.Fatal(err)
			}
			return parts[0]
		}},
		{"extract", func(t *testing.T, d *Document) *Document {
			x, err := d.Extract(PageRange{From: 1, To: 1})
			if err != nil {
				t.Fatal(err)
			}
			return x
		}},
		{"append-source", func(t *testing.T, d *Document) *Document {
			dst := NewDocumentFromFormat(PageFormatA4)
			if err := dst.Append(d); err != nil {
				t.Fatal(err)
			}
			x, err := dst.Extract(PageRange{From: 2, To: 2})
			if err != nil {
				t.Fatal(err)
			}
			return x
		}},
		{"append-receiver", func(t *testing.T, d *Document) *Document {
			if err := d.Append(NewDocumentFromFormat(PageFormatA4)); err != nil {
				t.Fatal(err)
			}
			if d.boundFragments[0].bind.snap.text != "Picture" {
				t.Fatal("receiver edits not applied by Append")
			}
			return d
		}},
		{"nup", func(t *testing.T, d *Document) *Document {
			x, err := d.NUp(NUpOptions{Rows: 1, Cols: 1})
			if err != nil {
				t.Fatal(err)
			}
			return x
		}},
		{"booklet", func(t *testing.T, d *Document) *Document {
			x, err := d.Booklet(BookletOptions{})
			if err != nil {
				t.Fatal(err)
			}
			return x
		}},
		{"pdfpagestamp", func(t *testing.T, d *Document) *Document {
			dst := NewDocumentFromFormat(PageFormatA4)
			s, err := NewPdfPageStamp(d, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := mustPage(t, dst).AddStamp(s); err != nil {
				t.Fatal(err)
			}
			return dst
		}},
	} {
		t.Run(o.name, func(t *testing.T) {
			d := openBytes(t, applyDocBytes(t))
			absorbOne(t, d, "Figure").Text = "Picture"
			out := o.run(t, d)
			if o.name != "append-receiver" {
				// The result is built before any later save of d: the
				// edit must already be in it.
				if d.boundFragments[0].bind.snap.text != "Picture" {
					t.Fatal("source edits not applied")
				}
			}
			r := reopenDoc(t, out)
			if absorbCount(t, r, "Picture") != 1 || absorbCount(t, r, "Figure") != 0 {
				t.Fatal("pending edit not in the result")
			}
		})
	}
	t.Run("importform", func(t *testing.T) {
		src := openBytes(t, applyDocBytes(t))
		f := absorbOne(t, src, "Figure")
		f.Text = "Picture"
		form := src.CreateForm(50, 50)
		if _, err := NewDocumentFromFormat(PageFormatA4).ImportForm(form); err != nil {
			t.Fatal(err)
		}
		if f.bind.snap.text != "Picture" {
			t.Fatal("source edits not applied")
		}
	})
}

// A page rewritten as a whole after a fragment was found makes an edit to it
// fail; an untouched stale fragment is no obstacle; an applied edit keeps
// the other fragments on its page editable.
func TestApplyStaleBinding(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	if _, err := d.ReplaceText("second", "2nd"); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	f.Text = "Picture"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "rewritten") || !strings.Contains(err.Error(), "page 1") ||
		!strings.Contains(err.Error(), `"Figure"`) || !strings.Contains(err.Error(), `"Picture"`) {
		t.Fatalf("err %v", err)
	}

	d = openBytes(t, applyDocBytes(t))
	f = absorbOne(t, d, "Figure")
	g := absorbOne(t, d, "second line")
	f.Text = "Picture"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	g.Text = "2nd line"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	r := openBytes(t, writeBytes(t, d))
	if absorbCount(t, r, "Picture") != 1 || absorbCount(t, r, "2nd line") != 1 || absorbCount(t, r, "second") != 0 {
		t.Fatal("sibling edit")
	}
}

// Other whole-content rewrites mark bindings stale too.
func TestApplyStaleAfterGrayscale(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	if err := d.ConvertToGrayscale(); err != nil {
		t.Fatal(err)
	}
	f.Text = "Picture"
	if err := d.ApplyTextChanges(); err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Fatalf("err %v", err)
	}
}

// A fragment found after a rewrite records the page's current generation.
func TestApplyBoundAfterRewrite(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	if _, err := d.ReplaceText("second", "2nd"); err != nil {
		t.Fatal(err)
	}
	absorbOne(t, d, "Figure").Text = "Picture"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
}

// A segment's own state change on an authored fragment is applied to that
// segment.
func TestApplyAuthoredSegmentState(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	f := NewTextFragment("one ")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.Segments = append(f.Segments, NewTextSegment("two"))
	if err := NewTextBuilder(mustPage(t, d)).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.Segments[1].TextState.ForegroundColor = &Color{1, 0, 0, 1}
	r := reopenDoc(t, d)
	g := absorbOne(t, r, "one two")
	if len(g.Segments) != 2 || g.Segments[1].TextState.ForegroundColor.R < 0.99 || g.Segments[0].TextState.ForegroundColor.R > 0.01 {
		t.Fatal("segment state not applied")
	}
}

func TestApplyOnSplitHTML(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "Figure")
	f.Text = "Picture"
	if err := d.SaveHTML(filepath.Join(t.TempDir(), "x.html"), HTMLSaveOptions{SplitPages: true}); err != nil {
		t.Fatal(err)
	}
	if f.bind.snap.text != "Picture" {
		t.Fatal("split HTML did not apply the edit")
	}
}

// A failure removing the old text (an unknown font on page 2) after page 1
// was edited leaves both pages as they were, and names page and fragment.
func TestApplyRollbackAfterStripFailure(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	p1 := mustPage(t, d)
	p2, _ := d.Page(2)
	a := NewTextFragment("alpha")
	a.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p1).AppendText(a); err != nil {
		t.Fatal(err)
	}
	b := NewTextFragment("beta")
	b.Position = Position{XIndent: 100, YIndent: 600}
	if err := NewTextBuilder(p2).AppendText(b); err != nil {
		t.Fatal(err)
	}
	if err := p2.appendToContentStream([]byte("BT /Fmissing 12 Tf 100 300 Td (zzz) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	before := writeBytes(t, d)
	a.Text = "ALPHA"
	b.Text = "BETA"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "page 2") || !strings.Contains(err.Error(), `"beta"`) ||
		!strings.Contains(err.Error(), `"BETA"`) {
		t.Fatalf("err %v", err)
	}
	a.Text, b.Text = "alpha", "beta"
	if !bytes.Equal(writeBytes(t, d), before) {
		t.Fatal("failed apply left changes")
	}
}

// Every refusal names the page and the fragment's text.
func TestApplyErrorsNamePageAndText(t *testing.T) {
	d := openBytes(t, rotatedDocBytes(t))
	absorbOne(t, d, "tilted").Text = "upright"
	err := d.ApplyTextChanges()
	if err == nil || !strings.Contains(err.Error(), "page 1") || !strings.Contains(err.Error(), `"tilted"`) ||
		!strings.Contains(err.Error(), `"upright"`) {
		t.Fatalf("err %v", err)
	}
}
