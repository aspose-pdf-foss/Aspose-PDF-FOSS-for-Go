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
