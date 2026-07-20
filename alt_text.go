// SPDX-License-Identifier: MIT

package asposepdf

// Alternate-text authoring for tagged PDFs (epic pdf-go-yrnd phase 3, the
// PDF-side half of ai.ImageDescriptionCopilot.FillAltTexts). Walks the
// structure tree for /Figure elements that lack alternate text, resolves the
// image each one brackets (via its marked-content MCID on its page), and lets
// a caller set the /Alt — turning ValidatePDFUA's UA_FIGURE_NO_ALT findings
// into a one-call fix.

// FigureAlt is a /Figure structure element that has no alternate text, paired
// with the image it brackets (when resolvable). Set the text with SetAltText.
type FigureAlt struct {
	doc  *Document
	elem pdfDict // the /Figure structure-element dict (mutated by SetAltText)
	img  *Image  // resolved image, or nil when it could not be located
}

// Image returns the figure's image and whether one was resolved. The image is
// decoded (Data holds PNG/JPEG bytes).
func (f *FigureAlt) Image() (*Image, bool) {
	return f.img, f.img != nil
}

// SetAltText sets the figure's /Alt (alternate text). Empty input is ignored.
func (f *FigureAlt) SetAltText(alt string) {
	if alt == "" {
		return
	}
	f.elem["/Alt"] = alt
}

// FiguresNeedingAltText walks the document's logical structure tree and
// returns every /Figure element that lacks alternate text (/Alt or
// /ActualText), each paired with the image it brackets when that image can be
// located. Returns nil for an untagged document. Mirrors the discovery half
// of Aspose.PDF for .NET's accessibility tooling; the description side lives
// in the ai subpackage (ImageDescriptionCopilot.FillAltTexts).
func (d *Document) FiguresNeedingAltText() ([]*FigureAlt, error) {
	root, ok := resolveRefToDict(d.objects, d.catalog["/StructTreeRoot"])
	if !ok {
		return nil, nil
	}
	roleMap, _ := resolveRefToDict(d.objects, root["/RoleMap"])

	// MCID→image maps are built per page on demand and cached.
	pageImages := map[int]map[int]*Image{}

	var figs []*FigureAlt
	var walk func(elem pdfDict)
	walk = func(elem pdfDict) {
		if resolveStructType(roleMap, dictGetName(elem, "/S")) == "/Figure" && !hasAltText(d.objects, elem) {
			fa := &FigureAlt{doc: d, elem: elem}
			fa.img = d.figureImage(elem, pageImages)
			figs = append(figs, fa)
		}
		for _, kid := range d.structElemKids(elem) {
			walk(kid)
		}
	}
	for _, kid := range d.structElemKids(root) {
		walk(kid)
	}
	return figs, nil
}

// figureImage resolves the image a /Figure element brackets, via its page
// (/Pg) and marked-content MCID (/K).
func (d *Document) figureImage(elem pdfDict, cache map[int]map[int]*Image) *Image {
	pageNum := d.pageNumberOfRef(elem["/Pg"])
	if pageNum == 0 {
		return nil
	}
	mcids := mcidsOf(elem["/K"])
	if len(mcids) == 0 {
		return nil
	}
	m, ok := cache[pageNum]
	if !ok {
		m = d.mcidImages(pageNum)
		cache[pageNum] = m
	}
	for _, id := range mcids {
		if img := m[id]; img != nil {
			return img
		}
	}
	return nil
}

// pageNumberOfRef returns the 1-based page number a /Pg reference points at,
// or 0 when it does not match a page.
func (d *Document) pageNumberOfRef(v pdfValue) int {
	ref, ok := v.(pdfRef)
	if !ok {
		return 0
	}
	for i, po := range d.pages {
		if po != nil && po.Num == ref.Num {
			return i + 1
		}
	}
	return 0
}

// mcidsOf extracts the MCID integers directly referenced by a structure
// element's /K (a bare int, an array of ints, or /MCR dicts carrying /MCID).
func mcidsOf(k pdfValue) []int {
	var out []int
	var add func(v pdfValue)
	add = func(v pdfValue) {
		switch x := v.(type) {
		case int:
			out = append(out, x)
		case pdfArray:
			for _, e := range x {
				add(e)
			}
		case pdfDict:
			if dictGetName(x, "/Type") == "/MCR" {
				if id, ok := x["/MCID"].(int); ok {
					out = append(out, id)
				}
			}
		}
	}
	add(k)
	return out
}

// mcidImages parses a page's content stream and maps each marked-content MCID
// to the image drawn inside it (the last image wins when several share one
// MCID). Images are matched to the page's ImageInfos by XObject name and
// decoded.
func (d *Document) mcidImages(pageNum int) map[int]*Image {
	out := map[int]*Image{}
	page, err := d.Page(pageNum)
	if err != nil {
		return out
	}
	content, err := page.contentStreams()
	if err != nil {
		return out
	}
	ops, err := parseContentStream(content)
	if err != nil {
		return out
	}
	resources := page.pageResources()

	// Map image XObject name → decoded image (once per page).
	infos, err := page.ImageInfos()
	if err != nil {
		return out
	}
	imgByName := map[string]*Image{}
	for i := range infos {
		if infos[i].Name == "" {
			continue
		}
		if img, err := infos[i].Extract(); err == nil {
			imgByName[infos[i].Name] = img
		}
	}
	if len(imgByName) == 0 {
		return out
	}

	// Walk the content tracking the marked-content MCID stack; attribute
	// each image Do to the innermost active MCID.
	var mcidStack []int
	for _, op := range ops {
		switch op.Operator {
		case "BDC":
			mcidStack = append(mcidStack, bdcMCID(d.objects, resources, op.Operands))
		case "BMC":
			mcidStack = append(mcidStack, -1)
		case "EMC":
			if len(mcidStack) > 0 {
				mcidStack = mcidStack[:len(mcidStack)-1]
			}
		case "Do":
			if len(op.Operands) < 1 {
				continue
			}
			cur := -1
			for i := len(mcidStack) - 1; i >= 0; i-- {
				if mcidStack[i] >= 0 {
					cur = mcidStack[i]
					break
				}
			}
			if cur < 0 {
				continue
			}
			if img := imgByName[operandName(op.Operands[0])]; img != nil {
				out[cur] = img
			}
		}
	}
	return out
}

// bdcMCID reads the /MCID from a BDC operator's property list (an inline dict,
// or a name resolved through /Resources/Properties); returns -1 when absent.
func bdcMCID(objects map[int]*pdfObject, resources pdfDict, operands []pdfValue) int {
	if len(operands) < 2 {
		return -1
	}
	var props pdfDict
	switch p := operands[1].(type) {
	case pdfDict:
		props = p
	case pdfName:
		if resources != nil {
			if pd, ok := resolveRefToDict(objects, resources["/Properties"]); ok {
				props, _ = resolveRefToDict(objects, pd[string(p)])
			}
		}
	}
	if props == nil {
		return -1
	}
	if id, ok := resolveRef(objects, props["/MCID"]).(int); ok {
		return id
	}
	return -1
}
