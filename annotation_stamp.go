// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"os"
)

// StampName names per ISO 32000-1 §12.5.6.13 Table 184. Used in
// /Subtype /Stamp annotations' /Name entry. Unknown handles non-spec
// custom names (round-tripped via RawName).
type StampName int

const (
	StampNameUnknown StampName = iota
	StampNameApproved
	StampNameAsIs
	StampNameConfidential
	StampNameDepartmental
	StampNameDraft         // PDF default
	StampNameExperimental
	StampNameExpired
	StampNameFinal
	StampNameForComment
	StampNameForPublicRelease
	StampNameNotApproved
	StampNameNotForPublicRelease
	StampNameSold
	StampNameTopSecret
)

// String returns the spec name (e.g. "Approved") for diagnostics.
func (n StampName) String() string {
	s := string(stampNameToPDF(n))
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

// StampAnnotation is a rubber-stamp annotation. Renders one of 14
// predefined visuals (Approved, Confidential, Draft, etc.) or a custom
// image. Per ISO 32000-1 §12.5.6.13.
type StampAnnotation struct {
	drawingAnnotationBase
	customImageObjID int // 0 = no custom image
}

func (a *StampAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStamp }

// NewStampAnnotation builds an unbound stamp annotation. Page must be
// non-nil. /Name defaults to the supplied name (use StampNameDraft if
// uncertain).
func NewStampAnnotation(page *Page, rect Rectangle, name StampName) *StampAnnotation {
	if page == nil {
		panic("NewStampAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Stamp"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
		"/Name":    stampNameToPDF(name),
	}
	a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// Name returns the StampName decoded from /Name. Returns
// StampNameUnknown for non-spec custom names.
func (a *StampAnnotation) Name() StampName {
	n, _ := a.dict["/Name"].(pdfName)
	return pdfNameToStampName(n)
}

// SetName writes the /Name entry from a typed StampName.
func (a *StampAnnotation) SetName(n StampName) {
	a.dict["/Name"] = stampNameToPDF(n)
	a.regenerateAP()
}

// RawName returns the /Name entry as a raw string ("/Approved", custom).
func (a *StampAnnotation) RawName() string {
	n, _ := a.dict["/Name"].(pdfName)
	return string(n)
}

// SetRawName writes the /Name entry from a raw string. Used for
// non-spec custom names. Calling SetRawName with a value not matching
// any spec name will cause Name() to return StampNameUnknown.
func (a *StampAnnotation) SetRawName(s string) {
	a.dict["/Name"] = pdfName(s)
	a.regenerateAP()
}

// HasCustomImage returns true if SetCustomImage / SetCustomImageFromStream
// has been called and not subsequently cleared. Stub for now — full
// custom-image support in Task 8.
func (a *StampAnnotation) HasCustomImage() bool {
	return a.customImageObjID != 0
}

// regenerateAP rebuilds /AP/N. Stub for now — full impl in Task 7.
func (a *StampAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateStampAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current state.
func (a *StampAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// SetCustomImage embeds the image at path as the stamp's /AP/N visual,
// overriding the predefined-name template. Format auto-detected from
// magic bytes (JPEG, PNG).
func (a *StampAnnotation) SetCustomImage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("StampAnnotation.SetCustomImage: %w", err)
	}
	return a.setCustomImageBytes(data)
}

// SetCustomImageFromStream is the io.Reader variant of SetCustomImage.
func (a *StampAnnotation) SetCustomImageFromStream(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("StampAnnotation.SetCustomImageFromStream: %w", err)
	}
	return a.setCustomImageBytes(data)
}

// setCustomImageBytes is the common implementation: detect format,
// build Image XObject, register in doc.objects, store objID.
func (a *StampAnnotation) setCustomImageBytes(data []byte) error {
	format, err := detectImageFormat(data)
	if err != nil {
		return fmt.Errorf("StampAnnotation.SetCustomImage: %w", err)
	}
	imgStream, smaskStream, err := createImageXObject(data, format)
	if err != nil {
		return fmt.Errorf("StampAnnotation.SetCustomImage: %w", err)
	}
	// If PNG with alpha, embed SMask first and link from main image.
	if smaskStream != nil {
		smaskID := a.doc.nextID
		a.doc.nextID++
		a.doc.objects[smaskID] = &pdfObject{Num: smaskID, Value: smaskStream}
		imgStream.Dict["/SMask"] = pdfRef{Num: smaskID}
	}
	imgID := a.doc.nextID
	a.doc.nextID++
	a.doc.objects[imgID] = &pdfObject{Num: imgID, Value: imgStream}

	a.customImageObjID = imgID
	a.regenerateAP()
	return nil
}

// ClearCustomImage reverts /AP/N to the predefined-name template visual.
// The previously-attached image XObject becomes orphan and can be
// reclaimed via doc.RemoveUnusedObjects().
func (a *StampAnnotation) ClearCustomImage() {
	a.customImageObjID = 0
	a.regenerateAP()
}

// parseStampAnnotation builds a StampAnnotation from a parsed dict.
// If /AP/N/Resources/XObject/Im0 is present, treats this as a
// custom-image stamp and re-derives customImageObjID accordingly.
func parseStampAnnotation(base annotationBase) *StampAnnotation {
	a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP

	// Detect custom image from /AP/N/Resources/XObject/Im0.
	if apDict, ok := base.dict["/AP"].(pdfDict); ok {
		if nVal, ok := apDict["/N"]; ok {
			// /N may be a pdfRef or inline pdfStream.
			if ref, ok := nVal.(pdfRef); ok {
				if obj, exists := base.doc.objects[ref.Num]; exists {
					if stream, ok := obj.Value.(*pdfStream); ok {
						if res, ok := stream.Dict["/Resources"].(pdfDict); ok {
							if xobj, ok := res["/XObject"].(pdfDict); ok {
								if imRef, ok := xobj["/Im0"].(pdfRef); ok {
									a.customImageObjID = imRef.Num
								}
							}
						}
					}
				}
			}
		}
	}
	return a
}
