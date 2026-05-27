// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// Open a PDF file and inspect its page count.
func ExampleOpen() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(doc.PageCount(), "pages")
	// Output: 4 pages
}

// Open an encrypted PDF by trying the password as both user and owner password.
func ExampleOpenWithPassword() {
	// Build an encrypted document in memory so the example is self-contained.
	src := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	src.SetEncryption(pdf.EncryptionOptions{UserPassword: "secret"})
	var buf bytes.Buffer
	if _, err := src.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}

	doc, err := pdf.OpenStreamWithPassword(&buf, "secret")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("opened:", doc.PageCount(), "pages")
	// Output: opened: 1 pages
}

// Create a blank A4 document in memory.
func ExampleNewDocumentFromFormat() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	size, _ := page.Size()
	fmt.Printf("%.0fx%.0f pt\n", size.Width, size.Height)
	// Output: 595x842 pt
}

// Draw "Hello, world!" centered on a blank A4 page and write the
// resulting PDF to an in-memory buffer.
func ExamplePage_AddText() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	size, _ := page.Size()

	style := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   24,
		HAlign: pdf.HAlignCenter,
		VAlign: pdf.VAlignMiddle,
	}
	rect := pdf.Rectangle{LLX: 0, LLY: 0, URX: size.Width, URY: size.Height}
	if err := page.AddText("Hello, world!", style, rect); err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote:", buf.Len() > 0)
	// Output: wrote: true
}

// Split a multi-page document into single-page documents.
func ExampleDocument_Split() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	parts, err := doc.Split()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("parts:", len(parts))
	// Output: parts: 4
}

// Extract selected page ranges into a new document.
func ExampleDocument_Extract() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	out, err := doc.Extract(
		pdf.PageRange{From: 1, To: 2},
		pdf.PageRange{From: 4, To: 4},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("extracted:", out.PageCount(), "pages")
	// Output: extracted: 3 pages
}

// Merge two documents by appending all pages of one into another.
func ExampleDocument_Append() {
	a, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	b, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	a.Append(b)
	fmt.Println("merged:", a.PageCount(), "pages")
	// Output: merged: 8 pages
}

// Rotate selected pages by 90 degrees clockwise. Subsequent calls accumulate.
func ExampleDocument_Rotate() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	if err := doc.Rotate(pdf.Rotate90, 1, 3); err != nil {
		log.Fatal(err)
	}
	p1, _ := doc.Page(1)
	p2, _ := doc.Page(2)
	fmt.Println("page1:", p1.Rotation(), "page2:", p2.Rotation())
	// Output: page1: 90 page2: 0
}

// Place a JPEG image on a blank page.
func ExamplePage_AddImage() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 50, LLY: 50, URX: 300, URY: 300}
	if err := page.AddImage("testdata/Koala.jpg", rect); err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}
	fmt.Println("ok")
	// Output: ok
}

// Convert a standalone image into a single-page PDF.
func ExampleImageToDocument() {
	doc, err := pdf.ImageToDocument("testdata/Koala.jpg")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("pages:", doc.PageCount())
	// Output: pages: 1
}

// Extract text from every page in visual reading order.
func ExampleDocument_ExtractText() {
	doc, err := pdf.Open("testdata/Hello world.pdf")
	if err != nil {
		log.Fatal(err)
	}
	pages, err := doc.ExtractText()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(strings.TrimSpace(pages[0]))
	// Output: Hello, world!
}

// Stamp an SVG logo as a watermark on every page.
func ExampleDocument_AddSVGWatermark() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	if err := doc.AddSVGWatermark("testdata/aspose-logo.svg"); err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}
	fmt.Println("watermarked:", buf.Len() > 0)
	// Output: watermarked: true
}

// Apply a diagonal "DRAFT" text watermark behind the content of every page.
func ExampleDocument_AddTextWatermark() {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	style := pdf.TextStyle{
		Font:     pdf.FontHelveticaBold,
		Size:     72,
		Color:    &pdf.Color{R: 0.85, G: 0.85, B: 0.85, A: 0.5},
		Rotation: 45,
		HAlign:   pdf.HAlignCenter,
		VAlign:   pdf.VAlignMiddle,
		Behind:   true,
	}
	if err := doc.AddTextWatermark("DRAFT", style); err != nil {
		log.Fatal(err)
	}
	fmt.Println("ok")
	// Output: ok
}

// Encrypt a document with AES-128 and a user password.
func ExampleDocument_SetEncryption() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "secret",
		OwnerPassword: "owner-secret",
		Permissions:   &pdf.Permissions{AllowPrint: true, AllowCopy: true},
		Algorithm:     pdf.EncryptionAlgAES128,
	})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}

	// The file is now encrypted; Open returns ErrEncrypted.
	if _, err := pdf.OpenStream(&buf); err != nil {
		fmt.Println("encrypted")
	}
	// Output: encrypted
}

// Validate the structural integrity of a PDF file.
func ExampleValidate() {
	report, err := pdf.Validate("testdata/4pages.pdf")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("valid:", report.Valid, "issues:", len(report.Issues))
	// Output: valid: true issues: 0
}

// Build a tiny invoice-style table and render it onto a blank page.
func ExampleNewTable() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{300, 100}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.5}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 6, Bottom: 4, Left: 6})

	header := table.AddRow()
	header.AddCell("Item").SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11})
	header.AddCell("Price").SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11}).
		SetHAlign(pdf.HAlignRight)

	table.AddRows([][]string{
		{"Espresso", "€3.50"},
		{"Cappuccino", "€4.50"},
		{"Tiramisu", "€7.50"},
	})

	rect := pdf.Rectangle{LLX: 50, LLY: 500, URX: 450, URY: 750}
	if _, err := page.AddTable(table, rect); err != nil {
		log.Fatal(err)
	}
	fmt.Println("rows:", table.RowCount())
	// Output: rows: 4
}

// Draw a filled rectangle with a stroked border on a blank page.
func ExamplePage_DrawRectangle() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)

	style := pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{
			Color: &pdf.Color{R: 0.1, G: 0.3, B: 0.6, A: 1},
			Width: 2,
		},
		FillColor: &pdf.Color{R: 0.85, G: 0.92, B: 1, A: 1},
	}
	rect := pdf.Rectangle{LLX: 100, LLY: 600, URX: 400, URY: 750}
	if err := page.DrawRectangle(rect, style); err != nil {
		log.Fatal(err)
	}
	fmt.Println("ok")
	// Output: ok
}

// Add a clickable link annotation that opens an external URL.
func ExampleNewLinkAnnotation() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)

	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://pkg.go.dev/github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"))
	if err := page.Annotations().Add(link); err != nil {
		log.Fatal(err)
	}
	fmt.Println("annotations:", page.Annotations().Count())
	// Output: annotations: 1
}
