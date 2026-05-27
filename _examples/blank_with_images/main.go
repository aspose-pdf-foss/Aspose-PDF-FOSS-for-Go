package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	// Create a blank A4 landscape document.
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4.Landscape())

	// Page 1: add Koala centered.
	page1, err := doc.Page(1)
	if err != nil {
		log.Fatalf("page 1: %v", err)
	}
	size1, _ := page1.Size()

	// Koala.jpg is 1024x768 pixels. Scale to fit nicely on the page (use 500pt width).
	imgW := 500.0
	imgH := imgW * 768.0 / 1024.0 // preserve aspect ratio = 375

	x := (size1.Width - imgW) / 2
	y := (size1.Height - imgH) / 2
	err = page1.AddImage("testdata/Koala.jpg", pdf.Rectangle{
		LLX: x,
		LLY: y,
		URX: x + imgW,
		URY: y + imgH,
	})
	if err != nil {
		log.Fatalf("add koala: %v", err)
	}

	// Add a second blank page (same A4 landscape).
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4.Landscape()); err != nil {
		log.Fatalf("add blank page: %v", err)
	}

	// Page 2: add Penguins in the top-left corner with 30pt margins.
	page2, err := doc.Page(2)
	if err != nil {
		log.Fatalf("page 2: %v", err)
	}
	size2, _ := page2.Size()

	// Penguins.jpg is 410x307 pixels. Use native size in points.
	penW := 410.0
	penH := 307.0
	margin := 30.0

	err = page2.AddImage("testdata/Penguins.jpg", pdf.Rectangle{
		LLX: margin,
		LLY: size2.Height - margin - penH,
		URX: margin + penW,
		URY: size2.Height - margin,
	})
	if err != nil {
		log.Fatalf("add penguins: %v", err)
	}

	if err := doc.Save("result_files/blank_with_images.pdf"); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Println("saved result_files/blank_with_images.pdf")
}
