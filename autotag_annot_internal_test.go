// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"testing"
)

// atStructFor returns the element that references annotation object num
// through an /OBJR, walking the structure tree, and its parent's type.
func atStructFor(d *Document, num int) (pdfDict, string) {
	root, _ := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	isObjr := func(v pdfValue) (bool, bool) {
		kd, ok := resolveRef(d.objects, v).(pdfDict)
		if !ok || dictGetName(kd, "/Type") != "/OBJR" {
			return false, false
		}
		r, ok := kd["/Obj"].(pdfRef)
		return true, ok && r.Num == num
	}
	var walk func(v pdfValue, parentType string) (pdfDict, string)
	walk = func(v pdfValue, parentType string) (pdfDict, string) {
		if arr, ok := resolveRef(d.objects, v).(pdfArray); ok {
			for _, k := range arr {
				if e, p := walk(k, parentType); e != nil {
					return e, p
				}
			}
			return nil, ""
		}
		dict, ok := resolveRefToDict(d.objects, v)
		if !ok {
			return nil, ""
		}
		self := dictGetName(dict, "/S")
		kids, isArr := resolveRef(d.objects, dict["/K"]).(pdfArray)
		if !isArr {
			kids = pdfArray{dict["/K"]}
		}
		for _, kid := range kids {
			if objr, match := isObjr(kid); objr {
				if match {
					return dict, parentType
				}
				continue
			}
			if e, p := walk(kid, self); e != nil {
				return e, p
			}
		}
		return nil, ""
	}
	return walk(root["/K"], "")
}

// Annotations join the structure tree as PDF/UA asks: a link in a /Link
// element under the text it covers, a form field in /Form, anything else in
// /Annot. A popup and a hidden annotation stay out, the page gets /Tabs /S,
// and the page still renders exactly as before.
func TestAutoTagAnnotations(t *testing.T) {
	doc := NewDocument(612, 792)
	p, _ := doc.Page(1)
	style := TextStyle{Font: FontHelvetica, Size: 12}
	if err := p.AddText("Read the manual before you start working.", style, Rectangle{LLX: 72, LLY: 680, URX: 540, URY: 720}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Second paragraph far below the first one.", style, Rectangle{LLX: 72, LLY: 300, URX: 540, URY: 340}); err != nil {
		t.Fatal(err)
	}

	link := NewLinkAnnotation(p, Rectangle{LLX: 72, LLY: 700, URX: 200, URY: 720})
	link.SetAction(NewGoToURIAction("https://example.com/manual"))
	high := NewHighlightAnnotation(p, Rectangle{LLX: 72, LLY: 300, URX: 200, URY: 340})
	note := NewTextAnnotation(p, Point{X: 500, Y: 100}) // over no text, near the bottom
	note.SetContents("a note")
	hidden := NewSquareAnnotation(p, Rectangle{LLX: 10, LLY: 10, URX: 20, URY: 20})
	popup := NewSquareAnnotation(p, Rectangle{LLX: 30, LLY: 10, URX: 40, URY: 20})
	for _, a := range []Annotation{link, high, note, hidden, popup} {
		if err := p.Annotations().Add(a); err != nil {
			t.Fatal(err)
		}
	}
	hidden.annotationBaseRef().dict["/F"] = 2
	popup.annotationBaseRef().dict["/Subtype"] = pdfName("/Popup")

	if _, err := doc.Form().AddTextField(1, Rectangle{LLX: 72, LLY: 500, URX: 300, URY: 520}, "email"); err != nil {
		t.Fatal(err)
	}

	before, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}

	res, err := doc.AutoTag(DefaultAutoTaggingSettings())
	if err != nil {
		t.Fatal(err)
	}
	if res.Annotations != 4 {
		t.Errorf("Annotations = %d, want 4 (link, highlight, note, field)", res.Annotations)
	}

	check := func(t *testing.T, d *Document) {
		t.Helper()
		pg, _ := d.Page(1)
		pd := pg.pageDict()
		if dictGetName(pd, "/Tabs") != "/S" {
			t.Errorf("/Tabs = %v", pd["/Tabs"])
		}
		annots, _ := resolveRef(d.objects, pd["/Annots"]).(pdfArray)
		found := map[string]pdfDict{}
		parents := map[string]string{}
		var linkDict pdfDict
		for _, a := range annots {
			r := a.(pdfRef)
			ad, _ := resolveRefToDict(d.objects, r)
			sub := dictGetName(ad, "/Subtype")
			el, parent := atStructFor(d, r.Num)
			if sub == "/Popup" || sub == "/Square" {
				if el != nil {
					t.Errorf("%s annotation is in the tree", sub)
				}
				if _, has := ad["/StructParent"]; has {
					t.Errorf("%s annotation has /StructParent", sub)
				}
				continue
			}
			if el == nil {
				t.Errorf("%s annotation is not in the tree", sub)
				continue
			}
			if _, has := ad["/StructParent"]; !has {
				t.Errorf("%s annotation has no /StructParent", sub)
			}
			found[sub] = el
			parents[sub] = parent
			if sub == "/Link" {
				linkDict = ad
			}
		}
		for sub, want := range map[string]string{"/Link": "/Link", "/Widget": "/Form", "/Highlight": "/Annot", "/Text": "/Annot"} {
			if el := found[sub]; el == nil || dictGetName(el, "/S") != want {
				t.Errorf("%s annotation is in %v, want %s", sub, el, want)
			}
		}
		if parents["/Link"] != "/P" && parents["/Link"] != "/H1" {
			t.Errorf("link's parent = %q, want the text element it covers", parents["/Link"])
		}
		if parents["/Text"] != "/Document" {
			t.Errorf("note over no text has parent %q, want /Document", parents["/Text"])
		}
		if linkDict != nil {
			if c, _ := linkDict["/Contents"].(string); c == "" {
				t.Errorf("link has no /Contents")
			}
		}
		if _, hasAlt := found["/Widget"]["/Alt"]; !hasAlt {
			t.Errorf("field with no /TU got no /Alt on its /Form")
		}
		if alt, _ := found["/Highlight"]["/Alt"].(string); alt == "" {
			t.Errorf("highlight with no /Contents got no /Alt")
		}
		if _, hasAlt := found["/Text"]["/Alt"]; hasAlt {
			t.Errorf("note with /Contents should not need /Alt")
		}
		// Every /StructParent resolves through the parent tree to its element.
		root, _ := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
		tree, _ := resolveRefToDict(d.objects, root["/ParentTree"])
		nums, _ := resolveRef(d.objects, tree["/Nums"]).(pdfArray)
		for _, a := range annots {
			ad, _ := resolveRefToDict(d.objects, a)
			sp, has := ad["/StructParent"]
			if !has {
				continue
			}
			ok := false
			for i := 0; i+1 < len(nums); i += 2 {
				if nums[i] == sp {
					el, _ := atStructFor(d, a.(pdfRef).Num)
					got, _ := resolveRefToDict(d.objects, nums[i+1])
					ok = dictGetName(got, "/S") == dictGetName(el, "/S")
				}
			}
			if !ok {
				t.Errorf("%s: /StructParent %v not in the parent tree", dictGetName(ad, "/Subtype"), sp)
			}
		}
	}
	check(t, doc)

	after, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.(*image.RGBA).Pix, after.(*image.RGBA).Pix) {
		t.Errorf("tagging annotations changed the render")
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

// A page with annotations and no text at all still gets them tagged.
func TestAutoTagAnnotationOnPageWithoutText(t *testing.T) {
	doc := NewDocument(612, 792)
	p, _ := doc.Page(1)
	link := NewLinkAnnotation(p, Rectangle{LLX: 72, LLY: 700, URX: 200, URY: 720})
	link.SetAction(NewGoToURIAction("https://example.com/"))
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	num := link.annotationBaseRef().objID
	el, _ := atStructFor(doc, num)
	if el == nil || dictGetName(el, "/S") != "/Link" {
		t.Fatalf("link element = %v", el)
	}
	if c, _ := link.annotationBaseRef().dict["/Contents"].(string); c != "https://example.com/" {
		t.Errorf("/Contents = %q, want the URI", c)
	}
	if _, has := el["/Pg"]; !has {
		t.Errorf("/Link has no /Pg")
	}
}

// A direct annotation dictionary in /Annots has no object number to
// reference, so it is promoted to an indirect object.
func TestAutoTagPromotesDirectAnnotation(t *testing.T) {
	doc := NewDocument(612, 792)
	p, _ := doc.Page(1)
	p.pageDict()["/Annots"] = pdfArray{pdfDict{
		"/Type": pdfName("/Annot"), "/Subtype": pdfName("/Stamp"), "/Contents": "direct",
		"/Rect": pdfArray{10.0, 10.0, 50.0, 50.0},
	}}
	res, err := doc.AutoTag(DefaultAutoTaggingSettings())
	if err != nil {
		t.Fatal(err)
	}
	if res.Annotations != 1 {
		t.Fatalf("Annotations = %d", res.Annotations)
	}
	r, ok := p.pageDict()["/Annots"].(pdfArray)[0].(pdfRef)
	if !ok {
		t.Fatalf("entry = %v, want a reference", p.pageDict()["/Annots"])
	}
	if el, _ := atStructFor(doc, r.Num); el == nil || dictGetName(el, "/S") != "/Annot" {
		t.Errorf("element = %v", el)
	}
}

// An annotation over no text takes its place in the reading order by position.
func TestAutoTagAnnotationReadingOrder(t *testing.T) {
	doc := NewDocument(612, 792)
	p, _ := doc.Page(1)
	style := TextStyle{Font: FontHelvetica, Size: 12}
	for _, r := range []Rectangle{{LLX: 72, LLY: 680, URX: 540, URY: 720}, {LLX: 72, LLY: 300, URX: 540, URY: 340}} {
		if err := p.AddText("A paragraph of ordinary body text.", style, r); err != nil {
			t.Fatal(err)
		}
	}
	stamp := NewStampAnnotation(p, Rectangle{LLX: 72, LLY: 480, URX: 200, URY: 520}, StampNameDraft)
	if err := p.Annotations().Add(stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.AutoTag(DefaultAutoTaggingSettings()); err != nil {
		t.Fatal(err)
	}
	kids, _ := doc.tagged.root.dict["/K"].(pdfArray)
	var order []string
	for _, k := range kids {
		kd, _ := resolveRefToDict(doc.objects, k)
		order = append(order, dictGetName(kd, "/S"))
	}
	if len(order) != 3 || order[1] != "/Annot" {
		t.Errorf("order = %v, want the stamp between the two paragraphs", order)
	}
}
