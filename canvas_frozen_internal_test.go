// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestXFormCanvasAddImageAfterFreezeDoesNotMutateResources is the
// internal-package companion to TestXFormCanvasAddImageAfterFreezeErrors
// (canvas_frozen_test.go): it directly inspects the already-built form's
// /Resources dict — which is the literal same pdfDict Go map as the
// canvas's own /Resources (makeFormXObjectWithResources stores it by
// reference, not by copy) — before and after a rejected post-freeze
// AddImageFromStream call.
//
// This property is NOT observable through RemoveUnusedObjects: unlike the
// orphaned-but-unreachable content-stream objects point 1 of pdf-go-nvpo
// is about, an image registered into that SHARED resources dict is
// genuinely reachable from the page (page -> Resources/XObject/<form> ->
// form -> its own Resources/XObject/<image>), so it is never flagged as
// unused even though nothing ever draws it — it is pointless, not
// unreachable. The only way to confirm addImageFromBytes's own
// checkCanvasNotFrozen guard (image_add.go) is doing its job — rejecting
// the draw BEFORE the image object is created and wired into that shared
// dict, not merely failing later at the content-stream append the way
// every other draw primitive does — is to check the dict's own entries
// directly.
func TestXFormCanvasAddImageAfterFreezeDoesNotMutateResources(t *testing.T) {
	doc := NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	form := doc.CreateForm(100, 50)
	if err := form.Canvas().AddText("Early", TextStyle{Size: 10},
		Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(form, Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatalf("AddForm: %v", err)
	}

	formStream, ok := doc.objects[form.formID].Value.(*pdfStream)
	if !ok {
		t.Fatal("built form object is not a stream")
	}
	resources, ok := formStream.Dict["/Resources"].(pdfDict)
	if !ok {
		t.Fatal("built form has no inline /Resources dict")
	}
	xobjCountBefore := xobjectEntryCount(resources)

	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	if err := form.Canvas().AddImageFromStream(bytes.NewReader(pngBuf.Bytes()),
		Rectangle{URX: 10, URY: 10}); err == nil {
		t.Fatal("AddImageFromStream on a frozen canvas: want error, got nil")
	}

	xobjCountAfter := xobjectEntryCount(resources)
	if xobjCountAfter != xobjCountBefore {
		t.Errorf("/Resources/XObject entry count = %d after the rejected draw, want unchanged %d — "+
			"the image object must never be created/registered when the canvas is frozen",
			xobjCountAfter, xobjCountBefore)
	}
}

// xobjectEntryCount returns the number of entries in resources's
// /XObject sub-dict (0 if absent).
func xobjectEntryCount(resources pdfDict) int {
	xobj, ok := resources["/XObject"].(pdfDict)
	if !ok {
		return 0
	}
	return len(xobj)
}

// fontEntryCount returns the number of entries in resources's /Font
// sub-dict (0 if absent).
func fontEntryCount(resources pdfDict) int {
	f, ok := resources["/Font"].(pdfDict)
	if !ok {
		return 0
	}
	return len(f)
}

// extGStateEntryCount returns the number of entries in resources's
// /ExtGState sub-dict (0 if absent).
func extGStateEntryCount(resources pdfDict) int {
	gs, ok := resources["/ExtGState"].(pdfDict)
	if !ok {
		return 0
	}
	return len(gs)
}

// TestXFormCanvasTextAfterFreezeDoesNotMutateResourcesOrFont reproduces
// GitHub issue #20 point 2 (shadowhunter1470-svg) exactly: resolveFontForPage/
// ensureStandardFontResource/ensureExtGState (and, via the embedded font's
// own encode closure, useGlyph — which marks a glyph used in the FONT
// OBJECT ITSELF, a document-wide resource) all used to run before the
// frozen check inside appendToContentStream fired, so a rejected
// AddTextAt/AddText/DrawRectangle call on a frozen canvas still left
// traces in the saved file: new /ToUnicode-tracked glyphs on the
// embedded font, a new /GS0 ExtGState, and a new /Times-Roman font
// resource — exactly the three calls and three symptoms the issue
// reports. checkCanvasNotFrozen is now the FIRST thing each of these
// methods does, before any of that runs.
func TestXFormCanvasTextAfterFreezeDoesNotMutateResourcesOrFont(t *testing.T) {
	data, err := readFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	doc := NewDocument(595, 842)
	f, err := doc.LoadFontFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	ef, ok := f.(*embeddedFont)
	if !ok {
		t.Fatal("LoadFontFromStream did not return *embeddedFont")
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	form := doc.CreateForm(100, 50)
	canvas := form.Canvas()
	if err := canvas.AddTextAt("abc", TextStyle{Font: f, Size: 10}, 5, 20); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(form, Rectangle{LLX: 10, LLY: 10, URX: 110, URY: 60}); err != nil {
		t.Fatalf("AddForm: %v", err)
	}

	formStream, ok := doc.objects[form.formID].Value.(*pdfStream)
	if !ok {
		t.Fatal("built form object is not a stream")
	}
	resources, ok := formStream.Dict["/Resources"].(pdfDict)
	if !ok {
		t.Fatal("built form has no inline /Resources dict")
	}
	fontCountBefore := fontEntryCount(resources)
	gsCountBefore := extGStateEntryCount(resources)
	usedGlyphsBefore := len(ef.usedGlyphs)

	// 1. AddTextAt with new glyphs the frozen canvas's font has never
	// drawn before (Ж, Я, Ю — none appear in "abc").
	if err := canvas.AddTextAt("ЖЯЮ", TextStyle{Font: f, Size: 10}, 5, 5); err == nil {
		t.Error("AddTextAt on a frozen canvas: want error, got nil")
	}
	if got := len(ef.usedGlyphs); got != usedGlyphsBefore {
		t.Errorf("embeddedFont.usedGlyphs count = %d after the rejected AddTextAt, want unchanged %d — "+
			"the rejected glyphs must never be marked used", got, usedGlyphsBefore)
	}

	// 2. AddText with a different (Standard-14) font never used on this
	// canvas before.
	if err := canvas.AddText("w", TextStyle{Font: FontTimesRoman, Size: 10}, Rectangle{URX: 100, URY: 50}); err == nil {
		t.Error("AddText on a frozen canvas: want error, got nil")
	}
	if got := fontEntryCount(resources); got != fontCountBefore {
		t.Errorf("/Resources/Font entry count = %d after the rejected AddText, want unchanged %d — "+
			"a new font resource must never be registered when the canvas is frozen", got, fontCountBefore)
	}

	// 3. DrawRectangle with a translucent fill, which registers an
	// ExtGState for the alpha.
	if err := canvas.DrawRectangle(Rectangle{LLX: 1, LLY: 1, URX: 9, URY: 9}, ShapeStyle{FillColor: &Color{R: 1, A: 0.5}}); err == nil {
		t.Error("DrawRectangle on a frozen canvas: want error, got nil")
	}
	if got := extGStateEntryCount(resources); got != gsCountBefore {
		t.Errorf("/Resources/ExtGState entry count = %d after the rejected DrawRectangle, want unchanged %d — "+
			"a new ExtGState must never be registered when the canvas is frozen", got, gsCountBefore)
	}
}
