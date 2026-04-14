package asposepdf

import (
	"testing"
)

func TestImageInfoHasPage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image info")
	}
	if infos[0].page == nil {
		t.Error("ImageInfo.page should be set")
	}
}

// createDocWithImage builds a Document with one page containing a JPEG XObject.
func createDocWithImage() *Document {
	jpegData := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B, 0x08,
		0x00, 0x0A, 0x00, 0x0A, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xD9,
	}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}
}
