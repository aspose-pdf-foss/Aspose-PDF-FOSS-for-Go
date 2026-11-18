// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// rewriterDoc is a page with "Hello world" at the top, "other line" lower
// down, a small table and an image, reopened so the text is found text.
func rewriterDoc(t *testing.T) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for _, l := range []struct {
		s string
		y float64
	}{{"Hello world", 750}, {"other line", 650}} {
		f := NewTextFragment(l.s)
		f.Position = Position{XIndent: 100, YIndent: l.y}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	tb := NewTable().SetColumnWidths([]float64{80, 80}).SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 1})
	tb.AddRows([][]string{{"a1", "b1"}, {"a2", "b2"}})
	if _, err := p.AddTable(tb, Rectangle{LLX: 100, LLY: 400, URX: 260, URY: 500}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddImage("testdata/aspose-logo.png", Rectangle{LLX: 100, LLY: 200, URX: 200, URY: 260}); err != nil {
		t.Fatal(err)
	}
	return reopenDoc(t, d)
}

// Every operation rewriting page content draws pending text edits first, so
// it does not leave them stale (which made Save fail).
func TestContentRewritersApplyPendingEdits(t *testing.T) {
	ops := []struct {
		name string
		// prep runs before the edit (finding what the operation needs).
		prep func(t *testing.T, d *Document) func() error
	}{
		{"Document.ReplaceText", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := d.ReplaceText("other", "another"); return err }
		}},
		{"Page.ReplaceText", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := mustPage(t, d).ReplaceText("other", "another"); return err }
		}},
		{"ApplyRedactions", func(t *testing.T, d *Document) func() error {
			return d.ApplyRedactions
		}},
		{"ConvertToGrayscale", func(t *testing.T, d *Document) func() error { return d.ConvertToGrayscale }},
		{"ConvertDeviceCMYKToRGB", func(t *testing.T, d *Document) func() error { return d.ConvertDeviceCMYKToRGB }},
		{"FlattenTransparency", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := d.FlattenTransparency(); return err }
		}},
		{"AutoTag", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := d.AutoTag(AutoTaggingSettings{}); return err }
		}},
		{"Document.RemoveAllText", func(t *testing.T, d *Document) func() error { return d.RemoveAllText }},
		{"Page.RemoveAllText", func(t *testing.T, d *Document) func() error { return mustPage(t, d).RemoveAllText }},
		{"MarkupParagraph.SetText", func(t *testing.T, d *Document) func() error {
			pm, err := mustPage(t, d).Paragraphs()
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range pm.Sections {
				for _, par := range s.Paragraphs {
					if strings.Contains(par.Text, "other line") {
						return func() error { return par.SetText("new line") }
					}
				}
			}
			t.Fatal("paragraph not found")
			return nil
		}},
		{"AbsorbedTextParagraph.SetHorizontalAlignment", func(t *testing.T, d *Document) func() error {
			a := NewTextParagraphAbsorber(Rectangle{LLX: 90, LLY: 640, URX: 400, URY: 670})
			if err := a.Visit(mustPage(t, d)); err != nil {
				t.Fatal(err)
			}
			tp := a.TextParagraphs()
			if len(tp) == 0 {
				t.Fatal("paragraph not found")
			}
			return func() error { return tp[0].SetHorizontalAlignment(HorizontalAlignmentRight) }
		}},
		{"ImageInfo.Remove", func(t *testing.T, d *Document) func() error {
			infos, err := mustPage(t, d).ImageInfos()
			if err != nil || len(infos) == 0 {
				t.Fatalf("images %d %v", len(infos), err)
			}
			return infos[0].Remove
		}},
		{"Document.ResizeContents", func(t *testing.T, d *Document) func() error {
			return func() error { return d.ResizeContents(ResizeMargins(10, 10, 10, 10)) }
		}},
		{"Page.ResizeContents", func(t *testing.T, d *Document) func() error {
			return func() error { return mustPage(t, d).ResizeContents(ResizeMargins(10, 10, 10, 10)) }
		}},
		{"Sanitize", func(t *testing.T, d *Document) func() error {
			return func() error {
				_, err := d.Sanitize(HiddenDataSanitizationOptions{ConvertPagesToImages: true})
				return err
			}
		}},
		{"ConvertToPDFA", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := d.ConvertToPDFA(PDFA1B); return err }
		}},
		{"ConvertToPDFUA", func(t *testing.T, d *Document) func() error {
			return func() error { _, err := d.ConvertToPDFUA(); return err }
		}},
		{"TableAbsorber.Remove", func(t *testing.T, d *Document) func() error {
			ta := NewTableAbsorber()
			if err := ta.Visit(mustPage(t, d)); err != nil || len(ta.TableList()) == 0 {
				t.Fatalf("tables %v", err)
			}
			return func() error { return ta.Remove(ta.TableList()[0]) }
		}},
		{"TableAbsorber.Replace", func(t *testing.T, d *Document) func() error {
			ta := NewTableAbsorber()
			if err := ta.Visit(mustPage(t, d)); err != nil || len(ta.TableList()) == 0 {
				t.Fatalf("tables %v", err)
			}
			nt := NewTable().SetColumnWidths([]float64{80, 80})
			nt.AddRows([][]string{{"x", "y"}, {"z", "w"}})
			return func() error { return ta.Replace(mustPage(t, d), ta.TableList()[0], nt) }
		}},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			d := rewriterDoc(t)
			run := op.prep(t, d)
			firstFound(t, NewTextFragmentAbsorber("Hello"), d).Text = "Bye"
			if err := run(); err != nil {
				t.Fatal(err)
			}
			if d.textChangesPending() {
				t.Fatal("edit still pending after the operation")
			}
			if _, err := d.WriteTo(&strings.Builder{}); err != nil {
				t.Fatalf("save: %v", err)
			}
		})
	}
}

// The reviewer's scenario: an edit, then ReplaceText on another word, then
// Save: both changes are on the page.
func TestEditThenReplaceTextSaves(t *testing.T) {
	d := helloWorldDoc(t)
	firstFound(t, NewTextFragmentAbsorber("Hello"), d).Text = "Bye"
	if n, err := d.ReplaceText("other", "another"); err != nil || n != 1 {
		t.Fatalf("replace %d %v", n, err)
	}
	txt := savedText(t, d)
	if !strings.Contains(txt, "Bye world") || strings.Contains(txt, "Hello") || !strings.Contains(txt, "another line") {
		t.Fatalf("text %q", txt)
	}
}
