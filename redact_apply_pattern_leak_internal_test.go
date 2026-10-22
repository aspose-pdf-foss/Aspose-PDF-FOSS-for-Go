// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// pdf-go-ifik: a tiling pattern's cell is a content stream of its own,
// reached from the page only through /Resources/Pattern. When every shape
// filled with it is redacted away, the pattern must go too — otherwise its
// cell content (here, text) stays in the saved file.

// patternPage builds a page with a tiling pattern whose cell draws text,
// and returns the document, page and pattern.
func patternPage(t *testing.T, cellText string) (*Document, *Page, *TilingPattern) {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	tp := doc.CreateTilingPattern(200, 20)
	if err := tp.Canvas().AddText(cellText, TextStyle{Size: 10}, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 20}); err != nil {
		t.Fatal(err)
	}
	return doc, page, tp
}

// redactAndReopen applies one redaction region and returns the saved and
// reopened document.
func redactAndReopen(t *testing.T, doc *Document, page *Page, region Rectangle) *Document {
	t.Helper()
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}
	if err := doc.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return re
}

func TestApplyRedactionsDropsPatternOfFullyRedactedFill(t *testing.T) {
	const secret = "PATTERNSECRET"
	doc, page, tp := patternPage(t, secret)
	if err := page.DrawRectangle(Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700}, ShapeStyle{FillTiling: tp}); err != nil {
		t.Fatal(err)
	}
	re := redactAndReopen(t, doc, page, Rectangle{LLX: 50, LLY: 550, URX: 350, URY: 750})
	if found, n := documentContainsBytes(re, secret); found {
		t.Errorf("pattern cell text still recoverable from %d object(s) after its only fill was redacted", n)
	}
}

// A second shape filled with the same pattern, outside the region, still
// paints with it: the pattern must stay and keep rendering.
func TestApplyRedactionsKeepsPatternStillUsedOutsideRegion(t *testing.T) {
	const cell = "SHAREDCELL"
	doc, page, tp := patternPage(t, cell)
	for _, r := range []Rectangle{
		{LLX: 100, LLY: 600, URX: 300, URY: 700}, // redacted
		{LLX: 100, LLY: 100, URX: 300, URY: 200}, // untouched
	} {
		if err := page.DrawRectangle(r, ShapeStyle{FillTiling: tp}); err != nil {
			t.Fatal(err)
		}
	}
	re := redactAndReopen(t, doc, page, Rectangle{LLX: 50, LLY: 550, URX: 350, URY: 750})
	if found, _ := documentContainsBytes(re, cell); !found {
		t.Fatal("pattern still used by an unredacted fill was removed")
	}
	p1, err := re.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	pats, _ := resolveRefToDict(re.objects, p1.pageResources()["/Pattern"])
	if len(pats) != 1 {
		t.Errorf("page /Pattern has %d entries, want the 1 still-used pattern", len(pats))
	}
}

// The scan must count a pattern as used when the painting operator comes
// from a Form XObject that inherits the page's resources and paints with
// the colour selected before its Do — the form's own content names no
// pattern at all.
func TestNeededPatternNamesFollowsInheritedColourIntoForm(t *testing.T) {
	objects := map[int]*pdfObject{
		7: {Num: 7, Value: &pdfStream{
			Dict:    pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form")},
			Data:    []byte("0 0 10 10 re f"),
			Decoded: true,
		}},
	}
	resources := pdfDict{"/XObject": pdfDict{"/Fm0": pdfRef{Num: 7}}}
	needed := map[string]bool{}
	ok := neededPatternNames([]byte("/Pattern cs /P0 scn /Fm0 Do"), objects, resources,
		patternColourState{}, 0, &formRedactBudget{}, needed, map[*pdfStream]bool{})
	if !ok || !needed["/P0"] {
		t.Errorf("pattern painted through an inheriting form not counted as needed (ok=%v, needed=%v)", ok, needed)
	}

	// A form that selects one of the page's patterns itself.
	objects[7].Value.(*pdfStream).Data = []byte("/Pattern cs /P2 scn 0 0 10 10 re f")
	needed = map[string]bool{}
	ok = neededPatternNames([]byte("/Fm0 Do"), objects, resources,
		patternColourState{}, 0, &formRedactBudget{}, needed, map[*pdfStream]bool{})
	if !ok || !needed["/P2"] {
		t.Errorf("pattern selected inside an inheriting form not counted as needed (ok=%v, needed=%v)", ok, needed)
	}
}

// Selecting a pattern and then switching back to a device colour before
// painting does not use it; q/Q restores the selection.
func TestNeededPatternNamesTracksColourState(t *testing.T) {
	cases := []struct {
		content string
		want    []string
	}{
		{"/Pattern cs /P0 scn 0 0 1 1 re f", []string{"/P0"}},
		{"/Pattern cs /P0 scn 1 0 0 rg 0 0 1 1 re f", nil},
		{"/Pattern cs /P0 scn q 0 g Q 0 0 1 1 re f", []string{"/P0"}},
		{"/Pattern CS /P1 SCN 0 0 m 1 1 l S", []string{"/P1"}},
		{"/Pattern cs /P0 scn", nil}, // selected, never painted
	}
	for _, c := range cases {
		needed := map[string]bool{}
		neededPatternNames([]byte(c.content), nil, pdfDict{}, patternColourState{}, 0, &formRedactBudget{}, needed, map[*pdfStream]bool{})
		var got []string
		for n := range needed {
			got = append(got, n)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: needed %v, want %v", c.content, got, c.want)
		}
	}
}

// The page's /Pattern dict can be shared with another page (here, the
// same Go map, as an indirect dict would be after parsing). Pruning it
// for the redacted page must not take the pattern away from the other.
func TestApplyRedactionsPatternPruneDoesNotTouchSharedDict(t *testing.T) {
	const cell = "OTHERPAGECELL"
	doc, page, tp := patternPage(t, cell)
	if err := doc.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	page2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*Page{page, page2} {
		if err := p.DrawRectangle(Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700}, ShapeStyle{FillTiling: tp}); err != nil {
			t.Fatal(err)
		}
	}
	page2.pageResources()["/Pattern"] = page.pageResources()["/Pattern"]
	re := redactAndReopen(t, doc, page, Rectangle{LLX: 50, LLY: 550, URX: 350, URY: 750})
	p2, err := re.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	if pats, _ := resolveRefToDict(re.objects, p2.pageResources()["/Pattern"]); len(pats) != 1 {
		t.Errorf("unredacted page 2 lost its pattern: /Pattern has %d entries", len(pats))
	}
}

// The same inheriting form drawn twice must not stop the scan (and with it
// the pruning) on its second Do.
func TestNeededPatternNamesSameFormTwice(t *testing.T) {
	objects := map[int]*pdfObject{
		7: {Num: 7, Value: &pdfStream{
			Dict:    pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form")},
			Data:    []byte("0 0 10 10 re S"),
			Decoded: true,
		}},
	}
	resources := pdfDict{"/XObject": pdfDict{"/Fm0": pdfRef{Num: 7}}}
	needed := map[string]bool{}
	ok := neededPatternNames([]byte("/Fm0 Do /Fm0 Do"), objects, resources,
		patternColourState{}, 0, &formRedactBudget{}, needed, map[*pdfStream]bool{})
	if !ok {
		t.Error("drawing the same form twice aborted the pattern scan")
	}
}
