// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// assembled holds the output object set and structural metadata produced by
// (*Document).assemble. Both the normal sequential writer (buildDocumentPDF)
// and the linearized writer (buildLinearizedPDF) consume it, so object IDs,
// the /Pages node, /Catalog, encryption state and the file header are computed
// in exactly one place.
type assembled struct {
	encState     *encryptState
	contentIDs   []int       // old object IDs, ascending
	remap        map[int]int // old ID -> output ID
	pagesObjID   int
	catalogObjID int
	infoObjID    int // 0 if no /Info
	encryptObjID int // 0 if not encrypted
	totalObjects int // exclusive upper bound (output IDs are 1..totalObjects-1)
	catalog      pdfDict
	header       string
}

// remapFn returns a reference-remapping function over the assembled id map.
func (a *assembled) remapFn() func(int) int {
	return func(n int) int {
		if out, ok := a.remap[n]; ok {
			return out
		}
		return n
	}
}

// assemble builds the full output object set: it injects signature, outline and
// named-destination objects, assigns sequential output IDs to every content
// object plus the structural /Pages, /Catalog, /Info and /Encrypt objects,
// patches each page's /Parent, and constructs the output /Catalog. It mutates
// d.objects (adding writer-built objects) exactly as the previous inline code
// in buildDocumentPDF did.
func (d *Document) assemble() (*assembled, error) {
	var encState *encryptState
	if d.preserved != nil {
		encState = d.preserved
	} else if d.encrypt != nil {
		var err error
		encState, err = newEncryptState(d.encrypt)
		if err != nil {
			return nil, fmt.Errorf("encrypt: %w", err)
		}
	}

	if d.sign != nil {
		d.buildSignatureObjects()
	}

	outlinesRef, outlineObjs := buildOutlineObjects(d)
	for _, obj := range outlineObjs {
		d.objects[obj.Num] = obj
	}

	ndTreeRef, ndNamesDictRef, ndObjs := buildNamedDestTree(d)
	if ndTreeRef.Num != 0 {
		var preserved pdfDict
		switch v := d.catalog["/Names"].(type) {
		case pdfRef:
			if obj, ok := d.objects[v.Num]; ok {
				if dict, ok := obj.Value.(pdfDict); ok {
					preserved = pdfDict{}
					for k, val := range dict {
						if k != "/Dests" {
							preserved[k] = deepCopyValue(val)
						}
					}
				}
			}
		case pdfDict:
			preserved = pdfDict{}
			for k, val := range v {
				if k != "/Dests" {
					preserved[k] = deepCopyValue(val)
				}
			}
		}
		if len(preserved) > 0 {
			namesObj := ndObjs[1]
			if dict, ok := namesObj.Value.(pdfDict); ok {
				for k, val := range preserved {
					dict[k] = val
				}
			}
		}
	}
	for _, obj := range ndObjs {
		d.objects[obj.Num] = obj
	}

	contentIDs := sortedObjectIDs(d.objects)
	remap := make(map[int]int, len(contentIDs))
	nextOut := 1
	for _, id := range contentIDs {
		remap[id] = nextOut
		nextOut++
	}
	pagesObjID := nextOut
	nextOut++
	catalogObjID := nextOut
	nextOut++
	var infoObjID int
	if d.info != nil {
		infoObjID = nextOut
		nextOut++
	}
	var encryptObjID int
	if encState != nil {
		encryptObjID = nextOut
		nextOut++
	}
	totalObjects := nextOut

	for _, page := range d.pages {
		if dict, ok := page.Value.(pdfDict); ok {
			dict["/Parent"] = pdfDirectRef{Num: pagesObjID}
		}
	}

	header := "%PDF-1.4\n"
	if d.encrypt != nil && d.encrypt.algorithm == EncryptionAlgAES256 {
		header = "%PDF-2.0\n"
	}

	catOut := make(pdfDict, len(d.catalog)+2)
	for k, v := range d.catalog {
		if k == "/Pages" {
			continue
		}
		catOut[k] = deepCopyValue(v)
	}
	catOut["/Type"] = pdfName("/Catalog")
	catOut["/Pages"] = pdfDirectRef{Num: pagesObjID}
	if outlinesRef.Num != 0 {
		catOut["/Outlines"] = outlinesRef
	}
	if ndTreeRef.Num != 0 {
		catOut["/Names"] = ndNamesDictRef
	}

	return &assembled{
		encState:     encState,
		contentIDs:   contentIDs,
		remap:        remap,
		pagesObjID:   pagesObjID,
		catalogObjID: catalogObjID,
		infoObjID:    infoObjID,
		encryptObjID: encryptObjID,
		totalObjects: totalObjects,
		catalog:      catOut,
		header:       header,
	}, nil
}
