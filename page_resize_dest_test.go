// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math"
	"testing"
)

func destNear(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// Destinations pointing at a position on a resized page move with its
// content, wherever they are stored: outline items (a /Dest or a GoTo action),
// named destinations, link annotations and the open action. A destination on
// another page, and a name reference, stay as they are.
func TestResizeMovesDestinations(t *testing.T) {
	doc := NewDocument(400, 800)
	if err := doc.AddBlankPage(400, 800); err != nil {
		t.Fatal(err)
	}
	p1, _ := doc.Page(1)
	p2, _ := doc.Page(2)

	item := func(d Destination, a Action) *OutlineItemCollection {
		it := NewOutlineItemCollection(doc)
		it.SetTitle("x")
		if d != nil {
			it.SetDestination(d)
		}
		if a != nil {
			it.SetAction(a)
		}
		if err := doc.Outlines().Add(it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	item(NewDestinationXYZ(p1, 100, 600, 2), nil)
	item(NewDestinationFitH(p1, 500), nil)
	item(NewDestinationFitV(p1, 40), nil)
	item(NewDestinationFitR(p1, 10, 20, 30, 40), nil)
	item(NewDestinationFitBH(p1, 300), nil)
	item(NewDestinationFitBV(p1, 60), nil)
	item(NewDestinationXYZ(p2, 100, 600, 1), nil)
	item(NewDestinationXYZUnchanged(p1, 100, true, 0, false, 0, false), nil)
	item(nil, NewGoToAction(1, 700))
	item(nil, NewGoToAction(2, 700))

	if err := doc.NamedDestinations().Add("spot", NewDestinationXYZ(p1, 200, 400, 1)); err != nil {
		t.Fatal(err)
	}
	item(NewNamedDestination(doc, "spot"), nil)

	link := NewLinkAnnotation(p2, Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 30})
	if err := p2.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	linkDict := link.annotationBaseRef().dict
	linkDict["/Dest"] = encodeDestination(NewDestinationXYZ(p1, 80, 160, 1))

	linkA := NewLinkAnnotation(p2, Rectangle{LLX: 60, LLY: 10, URX: 90, URY: 30})
	if err := p2.Annotations().Add(linkA); err != nil {
		t.Fatal(err)
	}
	// Two annotations sharing one action object must move it only once.
	actNum := doc.nextID
	doc.nextID++
	doc.objects[actNum] = &pdfObject{Num: actNum, Value: pdfDict{"/S": pdfName("/GoTo"), "/D": encodeDestination(NewDestinationFitH(p1, 200))}}
	linkA.annotationBaseRef().dict["/A"] = pdfRef{Num: actNum}
	linkB := NewLinkAnnotation(p2, Rectangle{LLX: 100, LLY: 10, URX: 130, URY: 30})
	if err := p2.Annotations().Add(linkB); err != nil {
		t.Fatal(err)
	}
	linkB.annotationBaseRef().dict["/A"] = pdfRef{Num: actNum}

	if doc.catalog == nil {
		doc.catalog = pdfDict{}
	}
	doc.catalog["/OpenAction"] = encodeDestination(NewDestinationXYZ(p1, 20, 80, 1))

	// Half size, plus margins' offset: x' = 0.5x + 10, y' = 0.25y + 20.
	params := ContentsResizeParameters{
		ContentsWidth: ResizePercents(50), ContentsHeight: ResizePercents(25),
		LeftMargin: ResizeUnits(10), BottomMargin: ResizeUnits(20),
	}
	if err := doc.ResizeContents(params, 1); err != nil {
		t.Fatal(err)
	}
	mx := func(x float64) float64 { return 0.5*x + 10 }
	my := func(y float64) float64 { return 0.25*y + 20 }

	check := func(t *testing.T, d *Document) {
		t.Helper()
		items := d.Outlines().All()
		if got := items[0].Destination().(*DestinationXYZ); !destNear(got.Left(), mx(100)) || !destNear(got.Top(), my(600)) || !destNear(got.Zoom(), 2) {
			t.Errorf("XYZ = %v %v zoom %v", got.Left(), got.Top(), got.Zoom())
		}
		if got := items[1].Destination().(*DestinationFitH); !destNear(got.top, my(500)) {
			t.Errorf("FitH top = %v", got.top)
		}
		if got := items[2].Destination().(*DestinationFitV); !destNear(got.left, mx(40)) {
			t.Errorf("FitV left = %v", got.left)
		}
		if got := items[3].Destination().(*DestinationFitR); !destNear(got.left, mx(10)) || !destNear(got.bottom, my(20)) || !destNear(got.right, mx(30)) || !destNear(got.top, my(40)) {
			t.Errorf("FitR = %+v", *got)
		}
		if got := items[4].Destination().(*DestinationFitBH); !destNear(got.top, my(300)) {
			t.Errorf("FitBH top = %v", got.top)
		}
		if got := items[5].Destination().(*DestinationFitBV); !destNear(got.left, mx(60)) {
			t.Errorf("FitBV left = %v", got.left)
		}
		if got := items[6].Destination().(*DestinationXYZ); !destNear(got.Left(), 100) || !destNear(got.Top(), 600) {
			t.Errorf("destination on the untouched page moved: %v %v", got.Left(), got.Top())
		}
		if got := items[7].Destination().(*DestinationXYZ); !destNear(got.Left(), mx(100)) || got.HasTop() {
			t.Errorf("unchanged-top XYZ = left %v, hasTop %v", got.Left(), got.HasTop())
		}
		if got := items[8].Action().(*GoToAction); !destNear(got.Top(), my(700)) {
			t.Errorf("GoTo top = %v", got.Top())
		}
		if got := items[9].Action().(*GoToAction); !destNear(got.Top(), 700) {
			t.Errorf("GoTo on the untouched page moved: %v", got.Top())
		}
		if _, isName := items[10].Destination().(*NamedDestination); !isName {
			t.Errorf("name reference became %T", items[10].Destination())
		}
		if got := d.NamedDestinations().Get("spot").(*DestinationXYZ); !destNear(got.Left(), mx(200)) || !destNear(got.Top(), my(400)) {
			t.Errorf("named destination = %v %v", got.Left(), got.Top())
		}
	}
	check(t, doc)

	if got := parseDestination(doc, linkDict["/Dest"]).(*DestinationXYZ); !destNear(got.Left(), mx(80)) || !destNear(got.Top(), my(160)) {
		t.Errorf("link /Dest = %v %v", got.Left(), got.Top())
	}
	act, _ := resolveRefToDict(doc.objects, pdfRef{Num: actNum})
	if got := parseDestination(doc, act["/D"]).(*DestinationFitH); !destNear(got.top, my(200)) {
		t.Errorf("shared GoTo action top = %v, want %v (moved once)", got.top, my(200))
	}
	if got := parseDestination(doc, doc.catalog["/OpenAction"]).(*DestinationXYZ); !destNear(got.Left(), mx(20)) || !destNear(got.Top(), my(80)) {
		t.Errorf("open action = %v %v", got.Left(), got.Top())
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	check(t, re)
}

// An item read from a file keeps its destination's kind: a /FitH action does
// not become /XYZ on the way through, and a second resize moves it again.
func TestResizeMovesParsedOutlineAction(t *testing.T) {
	doc := NewDocument(400, 800)
	p1, _ := doc.Page(1)
	it := NewOutlineItemCollection(doc)
	it.SetTitle("x")
	it.SetDestination(NewDestinationFitH(p1, 500))
	if err := doc.Outlines().Add(it); err != nil {
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
	// Turn the /Dest into a GoTo action carrying the /FitH.
	parsed := re.Outlines().All()[0]
	parsed.dict["/A"] = pdfDict{"/S": pdfName("/GoTo"), "/D": parsed.dict["/Dest"]}
	delete(parsed.dict, "/Dest")

	withDest := NewOutlineItemCollection(re)
	withDest.SetTitle("y")
	withDest.SetDestination(NewDestinationFitH(p1Of(re), 400))
	if err := re.Outlines().Add(withDest); err != nil {
		t.Fatal(err)
	}
	// As if it had been read from the file too: dict-backed, not detached.
	withDest.dict = pdfDict{"/Title": "y", "/Dest": encodeDestination(withDest.destination)}

	half := ContentsResizeParameters{ContentsHeight: ResizePercents(50), BottomMargin: ResizeUnits(0)}
	for range 2 {
		if err := re.ResizeContents(half, 1); err != nil {
			t.Fatal(err)
		}
	}
	buf.Reset()
	if _, err := re.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	final, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := resolveRefToDict(final.objects, final.Outlines().All()[0].dict["/A"])
	got, ok := parseDestination(final, a["/D"]).(*DestinationFitH)
	if !ok {
		t.Fatalf("action destination became %T", parseDestination(final, a["/D"]))
	}
	if !destNear(got.top, 125) {
		t.Errorf("top = %v, want 125", got.top)
	}
	if d, ok := final.Outlines().All()[1].Destination().(*DestinationFitH); !ok || !destNear(d.top, 100) {
		t.Errorf("parsed /Dest = %+v, want FitH top 100", final.Outlines().All()[1].Destination())
	}
}

// Reading the outline of a file must not change what its items do on save:
// a /FitH action stays /FitH and an action no Action type models survives.
func TestOutlineKeepsParsedActionOnSave(t *testing.T) {
	doc := NewDocument(400, 800)
	p1, _ := doc.Page(1)
	for range 2 {
		it := NewOutlineItemCollection(doc)
		it.SetTitle("x")
		it.SetDestination(NewDestinationFit(p1))
		if err := doc.Outlines().Add(it); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	items := re.Outlines().All()
	items[0].dict["/A"] = pdfDict{"/S": pdfName("/GoTo"), "/D": encodeDestination(NewDestinationFitH(p1Of(re), 300))}
	delete(items[0].dict, "/Dest")
	items[1].dict["/A"] = pdfDict{"/S": pdfName("/GoToR"), "/F": "other.pdf", "/D": pdfArray{0, pdfName("/Fit")}}
	delete(items[1].dict, "/Dest")

	buf.Reset()
	if _, err := re.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	final, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	items = final.Outlines().All()
	a0, _ := resolveRefToDict(final.objects, items[0].dict["/A"])
	if got, ok := parseDestination(final, a0["/D"]).(*DestinationFitH); !ok || !destNear(got.top, 300) {
		t.Errorf("GoTo /FitH came back as %v", a0["/D"])
	}
	a1, _ := resolveRefToDict(final.objects, items[1].dict["/A"])
	if dictGetName(a1, "/S") != "/GoToR" {
		t.Errorf("GoToR action came back as %v", a1)
	}
}

func p1Of(d *Document) *Page {
	p, _ := d.Page(1)
	return p
}

// The single-page method moves destinations too.
func TestPageResizeMovesDestination(t *testing.T) {
	doc := NewDocument(400, 800)
	p1, _ := doc.Page(1)
	it := NewOutlineItemCollection(doc)
	it.SetTitle("x")
	it.SetDestination(NewDestinationFitH(p1, 400))
	if err := doc.Outlines().Add(it); err != nil {
		t.Fatal(err)
	}
	if err := p1.ResizeContents(ContentsResizeParameters{ContentsHeight: ResizePercents(50), BottomMargin: ResizeUnits(0)}); err != nil {
		t.Fatal(err)
	}
	if got := it.Destination().(*DestinationFitH); !destNear(got.top, 200) {
		t.Errorf("top = %v, want 200", got.top)
	}
}
