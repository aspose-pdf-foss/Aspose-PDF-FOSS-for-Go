// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

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

// Generate a table of contents from the document outline. The TOC is
// inserted as new page(s) at the front; entries link to their targets.
func ExampleDocument_GenerateTOC() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	body, _ := doc.Page(1)

	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetTitle("Chapter 1")
	chapter.SetDestination(pdf.NewDestinationFit(body))
	if err := doc.Outlines().Add(chapter); err != nil {
		log.Fatal(err)
	}

	added, err := doc.GenerateTOC(pdf.TOCOptions{Title: "Table of Contents"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("TOC pages added:", added, "| total pages:", doc.PageCount())
	// Output: TOC pages added: 1 | total pages: 3
}

// Search for text on a page and get the bounding box of each match.
func ExamplePage_SearchText() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	_ = page.AddText("The quick brown fox jumps over the lazy dog.",
		pdf.TextStyle{Size: 14}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})

	matches, err := page.SearchText("brown fox")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d match: %q\n", len(matches), matches[0].Text)
	// Output: 1 match: "brown fox"
}

// Find and replace every occurrence of a string across the document.
func ExampleDocument_ReplaceText() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	_ = page.AddText("Draft version. Draft only.",
		pdf.TextStyle{Size: 14}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})

	n, err := doc.ReplaceText("Draft", "Final")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("replaced:", n)
	// Output: replaced: 2
}

// Create a text form field, fill it, and read the value back.
func ExampleForm_AddTextField() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	field, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 725}, "customer")
	if err != nil {
		log.Fatal(err)
	}
	_ = field.SetValue("ACME Corp")

	fmt.Println(form.Field("customer").Value())
	// Output: ACME Corp
}

// Generate a paginated document with the flow layout: headings, paragraphs
// and tables are laid out top-to-bottom with automatic page breaks.
func ExampleDocument_NewFlow() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{})
	flow.AddHeading(1, "Quarterly Report", pdf.TextStyle{})
	flow.AddParagraph("Revenue grew in every region this quarter.", pdf.TextStyle{Size: 11})
	flow.AddList([]string{"North: +12%", "South: +8%"}, false, pdf.TextStyle{Size: 11})

	pages, err := flow.Render()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("pages:", pages)
	// Output: pages: 1
}

// Sign a document with an ECDSA key and verify the signature. Certificates
// are ordinarily loaded from a key store; here one is generated in memory.
func ExampleDocument_Sign() {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Jane Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, _ := x509.CreateCertificate(cryptorand.Reader, tmpl, tmpl, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(der)

	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Reason: "Approval"}); err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}

	signed, _ := pdf.OpenStream(&buf)
	sigs, err := signed.VerifySignatures()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("signatures: %d, valid: %v, reason: %s\n", len(sigs), sigs[0].Valid, sigs[0].Reason)
	// Output: signatures: 1, valid: true, reason: Approval
}

// Convert Markdown (CommonMark + GFM) into a paginated PDF.
func ExampleMarkdownToDocumentFromStream() {
	md := "# Hello\n\nA paragraph with **bold** text.\n"
	doc, err := pdf.MarkdownToDocumentFromStream(strings.NewReader(md))
	if err != nil {
		log.Fatal(err)
	}
	pages, _ := doc.ExtractText()
	fmt.Println(strings.Split(pages[0], "\n")[0])
	// Output: Hello
}

// Export a document as GFM Markdown (the reverse of the Markdown renderer).
func ExampleDocument_WriteMarkdown() {
	doc, err := pdf.MarkdownToDocumentFromStream(strings.NewReader("# Title\n\nBody text.\n"))
	if err != nil {
		log.Fatal(err)
	}
	var out strings.Builder
	if err := doc.WriteMarkdown(&out); err != nil {
		log.Fatal(err)
	}
	fmt.Println(strings.Split(out.String(), "\n")[0])
	// Output: # Title
}

// Export a page as a standalone true-vector SVG file.
func ExamplePage_WriteSVG() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	_ = page.DrawCircle(pdf.Point{X: 200, Y: 600}, 50,
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, G: 0.8, A: 1}})

	var svg strings.Builder
	if err := page.WriteSVG(&svg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("vector output:", strings.Contains(svg.String(), "<svg"))
	// Output: vector output: true
}

// Export the document as self-contained HTML with a selectable text layer.
func ExampleDocument_WriteHTML() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	_ = page.AddText("Hello HTML", pdf.TextStyle{Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})

	var html bytes.Buffer
	if err := doc.WriteHTML(&html, pdf.HTMLSaveOptions{Mode: pdf.HTMLModeText}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("has text spans:", bytes.Contains(html.Bytes(), []byte("<span")))
	// Output: has text spans: true
}

// Render a page to a raster image with the built-in dependency-free renderer.
func ExampleDocument_RenderImage() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	_ = page.AddText("Preview me", pdf.TextStyle{Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})

	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("rendered:", img.Bounds().Dx() > 0 && img.Bounds().Dy() > 0)
	// Output: rendered: true
}

// Shrink a document with the unified lossless optimizer (unused objects,
// font subsetting, stream compression, duplicate-stream dedup).
func ExampleDocument_Optimize() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		log.Fatal(err)
	}
	page, _ := doc.Page(1)
	_ = page.AddText("Привет, мир!", pdf.TextStyle{Font: font, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})

	res, err := doc.Optimize(pdf.DefaultOptimizationOptions())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("fonts subset:", res.SubsettedFonts)
	// Output: fonts subset: 1
}

// Build a bookmark (outline) tree with clickable destinations.
func ExampleDocument_Outlines() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)

	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Chapter 1")
	item.SetDestination(pdf.NewDestinationFit(page))
	if err := doc.Outlines().Add(item); err != nil {
		log.Fatal(err)
	}

	fmt.Println(doc.Outlines().Count(), doc.Outlines().At(0).Title())
	// Output: 1 Chapter 1
}

// Author logical page labels (front matter i, ii, then body 1, 2, …).
func ExampleDocument_SetPageLabels() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	_ = doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	_ = doc.AddBlankPageFromFormat(pdf.PageFormatA4)

	err := doc.SetPageLabels([]pdf.PageLabelRange{
		{StartPage: 1, Style: pdf.PageLabelRomanLower},
		{StartPage: 3, Style: pdf.PageLabelDecimal},
	})
	if err != nil {
		log.Fatal(err)
	}
	p2, _ := doc.Page(2)
	p3, _ := doc.Page(3)
	fmt.Println(p2.Label(), p3.Label())
	// Output: ii 1
}
