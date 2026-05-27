// Full library showcase — a small "annual report"-style document that
// exercises every major Aspose.PDF for Go feature inside a single narrative.
//
// Document layout:
//
//	page i  — Cover (Aspose logo + title + version + date)
//	page ii — Table of Contents (clickable, backed by /Catalog/Names/Dests)
//	page 1  — Text capabilities (Standard 14, embedded TTF Unicode, decorations)
//	page 2  — JPEG image, scaled and centred
//	page 3  — AcroForm with every supported field type
//	page 4  — Annotation showcase (markup, drawing, text, FreeText, Stamp, FileAttachment)
//	page 5  — Redaction demo (destructive: ApplyRedactions runs at save time)
//	page 6  — Restaurant bill (single-page Table with ColSpan summary rows)
//	page 7+ — Multi-page Sales Report (overflow, repeating headers, RowSpan/ColSpan)
//	page N  — Landscape wide chart (uses pdf.PageFormatA4.Landscape())
//	page N+1 — Vector graphics showcase (every Draw* method, inline SVG)
//
// Cross-cutting features:
//   - Page labels: roman (i, ii) for front matter, decimal restarting at 1 for body
//   - Hierarchical outline (bookmarks) with sub-bookmarks for sales-report categories
//   - Named destinations powering the TOC link annotations
//   - Document metadata (Title/Author/Subject/Keywords/Creator/CreationDate)
//   - Aspose SVG logo stamped on every body page (top-right corner)
//   - Diagonal "WATERMARK" behind content on body pages (cover/TOC stay clean)
//   - Unified footer on every body page with the formatted page label
//   - AES-256 encryption (PDF 2.0) with user password "password"
//
// Output: result_files/full_scenario.pdf
//
// Run from the repo root: `go run ./_examples/full_scenario`
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

const (
	outputPath = "result_files/full_scenario.pdf"
	docTitle   = "Aspose.PDF for Go — Feature Showcase"
	docAuthor  = "Aspose"
	docVersion = "v0.2.0"
)

// Named destination keys used by the TOC, outlines, and section anchors.
const (
	destText      = "section.text"
	destImage     = "section.image"
	destForm      = "section.form"
	destAnnot     = "section.annotations"
	destRedact    = "section.redaction"
	destBill      = "section.bill"
	destSales     = "section.sales"
	destLandscape = "section.landscape"
	destVector    = "section.vector"
)

// section is one TOC/outline entry. The page is filled in once the body has
// been laid out and the actual *pdf.Page references are known.
type section struct {
	dest    string
	title   string
	subtype string // colour-coded category for the outline
	page    *pdf.Page
}

func main() {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	// --- Document scaffolding ---------------------------------------
	// 8 portrait A4 pages up front: cover, TOC, then 6 body sections
	// (text, image, form, annotations, redaction, restaurant bill).
	// Sales report, landscape chart, and vector showcase are appended
	// later so they land *after* any sales-report continuation pages.
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i := 0; i < 7; i++ {
		mustAddPage(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	}

	coverPage, _ := doc.Page(1)
	tocPage, _ := doc.Page(2)
	textPage, _ := doc.Page(3)
	imagePage, _ := doc.Page(4)
	formPage, _ := doc.Page(5)
	annotPage, _ := doc.Page(6)
	redactPage, _ := doc.Page(7)
	billPage, _ := doc.Page(8)

	// --- Body content -----------------------------------------------
	addCoverPage(doc, coverPage)
	// TOC is filled last — once every section is anchored we know
	// the destination names exist.
	addPageText(doc, textPage)
	addPageImage(imagePage)
	addFormFields(doc, formPage)
	addAnnotations(annotPage)
	addRedactionDemo(redactPage)
	addRestaurantBill(billPage)

	// Sales report — append a fresh page, then let the table grow.
	mustAddPage(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	salesPage, _ := doc.Page(doc.PageCount())
	addSalesReport(doc, salesPage)

	// Landscape wide chart — physically wider page via Landscape().
	mustAddPage(doc.AddBlankPage(pdf.PageFormatA4.Landscape().Width, pdf.PageFormatA4.Landscape().Height))
	landscapePage, _ := doc.Page(doc.PageCount())
	addLandscapeChart(landscapePage)

	// Vector graphics showcase — last content page.
	mustAddPage(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	vectorPage, _ := doc.Page(doc.PageCount())
	addVectorShowcase(doc, vectorPage)

	// --- Named destinations -----------------------------------------
	// The TOC links and the outline both target these. Forward
	// references through NewNamedDestination work because the named
	// destination resolves at view time (and writers serialise
	// /Catalog/Names/Dests at the end).
	sections := []section{
		{destText, "Text Capabilities", "text", textPage},
		{destImage, "Image Embedding", "image", imagePage},
		{destForm, "AcroForm Fields", "form", formPage},
		{destAnnot, "Annotations", "annotations", annotPage},
		{destRedact, "Redactions", "redaction", redactPage},
		{destBill, "Restaurant Bill", "bill", billPage},
		{destSales, "Sales Report", "sales", salesPage},
		{destLandscape, "Landscape Chart", "landscape", landscapePage},
		{destVector, "Vector Graphics", "vector", vectorPage},
	}
	named := doc.NamedDestinations()
	for _, s := range sections {
		if err := named.Add(s.dest, pdf.NewDestinationFit(s.page)); err != nil {
			log.Fatalf("named dest %q: %v", s.dest, err)
		}
	}

	// --- Table of Contents (depends on named destinations) ----------
	addTOC(doc, tocPage, sections)

	// --- Destructive transforms BEFORE encryption and stamping ------
	// ApplyRedactions rewrites content streams in place — must happen
	// before the watermark / logo content streams are appended so the
	// redactor doesn't see them as paths to clip.
	if err := doc.ApplyRedactions(); err != nil {
		log.Fatalf("apply redactions: %v", err)
	}

	// --- Per-page furniture: footer, logo stamp, watermark ----------
	// Pre-load the SVG logo once for stamping. Done after content so
	// stampAsposeLogoOnEveryPage sees the final page set.
	if err := stampAsposeLogoOnEveryPage(doc, coverPage, tocPage); err != nil {
		log.Fatalf("svg logo stamp: %v", err)
	}
	for _, p := range doc.Pages() {
		// Cover/TOC are deliberately watermark-free.
		if p == coverPage || p == tocPage {
			continue
		}
		if err := addCenteredWatermark(p, "WATERMARK"); err != nil {
			log.Fatalf("watermark: %v", err)
		}
	}
	// Footer — uses the page label so "1 / 10", "ii / x" stay coherent.
	for i, p := range doc.Pages() {
		addUnifiedFooter(p, i+1, doc.PageCount())
	}

	// --- Outline (bookmarks) ----------------------------------------
	// Nested: top-level entries point at each section; the Sales Report
	// entry has child entries for each category (Pasta, Pizza, …).
	addBookmarks(doc, sections)

	// --- Page labels ------------------------------------------------
	// Cover + TOC use lowercase roman; body restarts at decimal 1.
	if err := doc.SetPageLabels([]pdf.PageLabelRange{
		{StartPage: 1, Style: pdf.PageLabelRomanLower},
		{StartPage: 3, Style: pdf.PageLabelDecimal, StartNum: 1},
	}); err != nil {
		log.Fatalf("page labels: %v", err)
	}

	// --- Metadata ---------------------------------------------------
	now := time.Now().UTC().Format("D:20060102150405Z")
	doc.SetMetadata(pdf.Metadata{
		Title:        docTitle,
		Author:       docAuthor,
		Subject:      "End-to-end showcase of Aspose.PDF for Go capabilities",
		Keywords:     "aspose,pdf,go,golang,acroform,annotations,svg,redaction,encryption",
		Creator:      "Aspose.PDF for Go " + docVersion,
		Producer:     "Aspose.PDF for Go " + docVersion,
		CreationDate: now,
		ModDate:      now,
		Custom: map[string]string{
			"AsposeProduct": "Aspose.PDF for Go (FOSS)",
		},
	})

	// --- Encryption — AES-256 (PDF 2.0) -----------------------------
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "password",
		OwnerPassword: "owner-password",
		Permissions:   &pdf.Permissions{AllowPrint: true, AllowCopy: true, AllowAccessibility: true},
		Algorithm:     pdf.EncryptionAlgAES256,
	})

	if err := doc.Save(outputPath); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Printf("wrote %s (open with password \"password\")", outputPath)
}

func mustAddPage(err error) {
	if err != nil {
		log.Fatalf("add page: %v", err)
	}
}

// ---------------------------------------------------------------------
// Page 1 — text
// ---------------------------------------------------------------------

func addPageText(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	// ===== Title =====
	titleStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   26,
		Color:  &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Text Capabilities Showcase", titleStyle, pdf.Rectangle{
		LLX: 50, LLY: size.Height - 90, URX: size.Width - 50, URY: size.Height - 55,
	}))

	// ===== Subtitle =====
	subStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   11,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText(
		"Standard 14 fonts  •  embedded TTF & Unicode  •  decorations  •  colors  •  word-wrap",
		subStyle, pdf.Rectangle{
			LLX: 50, LLY: size.Height - 113, URX: size.Width - 50, URY: size.Height - 98,
		}))

	// Section-header helper.
	sectionStyle := pdf.TextStyle{
		Font:  pdf.FontHelveticaBold,
		Size:  13,
		Color: &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1},
	}
	section := func(label string, top float64) {
		mustText(page.AddText(label, sectionStyle, pdf.Rectangle{
			LLX: 50, LLY: top - 16, URX: size.Width - 50, URY: top,
		}))
	}

	// ===== Section 1: Standard 14 PDF Fonts =====
	section("Standard 14 PDF Fonts", size.Height-140)

	sample := "The quick brown fox jumps over 42 lazy dogs."
	labelStyle := pdf.TextStyle{
		Font:  pdf.FontCourier,
		Size:  8,
		Color: &pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1},
	}
	fonts := []struct {
		font  pdf.Font
		label string
	}{
		{pdf.FontHelvetica, "Helvetica"},
		{pdf.FontHelveticaBold, "Helvetica-Bold"},
		{pdf.FontHelveticaOblique, "Helvetica-Oblique"},
		{pdf.FontHelveticaBoldOblique, "Helvetica-BoldOblique"},
		{pdf.FontTimesRoman, "Times-Roman"},
		{pdf.FontTimesBold, "Times-Bold"},
		{pdf.FontTimesItalic, "Times-Italic"},
		{pdf.FontTimesBoldItalic, "Times-BoldItalic"},
		{pdf.FontCourier, "Courier"},
		{pdf.FontCourierBold, "Courier-Bold"},
		{pdf.FontCourierOblique, "Courier-Oblique"},
		{pdf.FontCourierBoldOblique, "Courier-BoldOblique"},
	}
	y := size.Height - 170
	for _, f := range fonts {
		mustText(page.AddText(f.label, labelStyle, pdf.Rectangle{
			LLX: 50, LLY: y - 11, URX: 185, URY: y + 1,
		}))
		s := pdf.TextStyle{Font: f.font, Size: 11}
		mustText(page.AddText(sample, s, pdf.Rectangle{
			LLX: 190, LLY: y - 12, URX: size.Width - 50, URY: y + 2,
		}))
		y -= 12
	}

	// ===== Section 2: Embedded TTF — Unicode =====
	y -= 14
	section("Embedded TTF (DejaVu Sans) — Unicode", y)
	y -= 22

	deja, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		log.Fatalf("load DejaVu Sans: %v", err)
	}
	unicodeLines := []string{
		"Русский: Здравствуй, мир!",
		"Ελληνικά: Γειά σου, κόσμε!",
		"Deutsch: Schöne Grüße aus München",
		"Français: Bonjour à tous, ça va?",
		"Symbols: → ← ★ ♥ ☎ € § ¶ ¥ £ © ®",
	}
	unicodeStyle := pdf.TextStyle{Font: deja, Size: 11}
	for _, line := range unicodeLines {
		mustText(page.AddText(line, unicodeStyle, pdf.Rectangle{
			LLX: 60, LLY: y - 14, URX: size.Width - 50, URY: y + 1,
		}))
		y -= 15
	}

	// ===== Section 3: Decorations =====
	y -= 12
	section("Decorations", y)
	y -= 22

	body := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11}

	// Underline + Strikethrough side-by-side.
	uStyle := body
	uStyle.Underline = true
	mustText(page.AddText("This text is underlined.", uStyle, pdf.Rectangle{
		LLX: 60, LLY: y - 14, URX: 295, URY: y + 1,
	}))
	sStyle := body
	sStyle.Strikethrough = true
	mustText(page.AddText("This text is struck through.", sStyle, pdf.Rectangle{
		LLX: 310, LLY: y - 14, URX: 545, URY: y + 1,
	}))
	y -= 18

	// Background highlight + 35% opacity.
	bgStyle := body
	bgStyle.Background = &pdf.Color{R: 1, G: 0.95, B: 0.4, A: 1}
	mustText(page.AddText("Yellow highlight background.", bgStyle, pdf.Rectangle{
		LLX: 60, LLY: y - 14, URX: 295, URY: y + 2,
	}))
	opStyle := body
	opStyle.Color = &pdf.Color{R: 0, G: 0, B: 0, A: 0.35}
	mustText(page.AddText("35% opacity text (faded).", opStyle, pdf.Rectangle{
		LLX: 310, LLY: y - 14, URX: 545, URY: y + 1,
	}))
	y -= 22

	// ===== Section 4: Color palette =====
	section("Color palette", y)
	y -= 22

	colors := []struct {
		col   pdf.Color
		label string
	}{
		{pdf.Color{R: 0.85, G: 0.10, B: 0.10, A: 1}, "crimson"},
		{pdf.Color{R: 0.10, G: 0.60, B: 0.20, A: 1}, "forest"},
		{pdf.Color{R: 0.10, G: 0.20, B: 0.80, A: 1}, "azure"},
		{pdf.Color{R: 0.60, G: 0.30, B: 0.70, A: 1}, "violet"},
		{pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1}, "amber"},
		{pdf.Color{R: 0.05, G: 0.55, B: 0.55, A: 1}, "teal"},
	}
	colW := (size.Width - 100) / float64(len(colors))
	for i, c := range colors {
		col := c.col // copy for a stable pointer address
		st := pdf.TextStyle{
			Font:   pdf.FontHelveticaBold,
			Size:   13,
			Color:  &col,
			HAlign: pdf.HAlignCenter,
		}
		mustText(page.AddText(c.label, st, pdf.Rectangle{
			LLX: 50 + float64(i)*colW, LLY: y - 16, URX: 50 + float64(i+1)*colW, URY: y + 2,
		}))
	}
	y -= 28

	// ===== Section 5: Word wrap & line spacing =====
	section("Word wrap & line spacing", y)
	y -= 22

	paragraph := pdf.TextStyle{
		Font:        pdf.FontTimesRoman,
		Size:        11,
		LineSpacing: 1.4,
	}
	mustText(page.AddText(
		"This paragraph demonstrates automatic word wrapping at the right edge of the bounding "+
			"rectangle. Words break on whitespace; line spacing is 1.4× the font size. AddText "+
			"handles alignment, clipping at the rectangle boundary, and font-aware glyph-width "+
			"measurement, so all these features carry through into table cells and free-text annotations.",
		paragraph,
		pdf.Rectangle{LLX: 60, LLY: 80, URX: size.Width - 50, URY: y + 2}))

	// ===== Footer =====
	// (Unified page footer is added later in main() — addUnifiedFooter.)
}

// ---------------------------------------------------------------------
// Page 2 — image
// ---------------------------------------------------------------------

func addPageImage(page *pdf.Page) {
	size, _ := page.Size()

	// Caption above the image.
	caption := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   16,
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Page 2 — Image", caption,
		pdf.Rectangle{LLX: 50, LLY: size.Height - 80, URX: size.Width - 50, URY: size.Height - 50}))

	// Koala.jpg is 1024x768. Scale to ~60% of page width preserving aspect ratio.
	imgW := size.Width * 0.6
	imgH := imgW * 768.0 / 1024.0
	x := (size.Width - imgW) / 2
	y := (size.Height - imgH) / 2
	if err := page.AddImage("testdata/Koala.jpg", pdf.Rectangle{
		LLX: x, LLY: y, URX: x + imgW, URY: y + imgH,
	}); err != nil {
		log.Fatalf("add image: %v", err)
	}
}

// ---------------------------------------------------------------------
// Page 3 — AcroForm with every supported field type
// ---------------------------------------------------------------------

func addFormFields(doc *pdf.Document, page *pdf.Page) {
	form := doc.Form()
	const labelW = 130

	pageNum := page.Number()
	addLabel := func(text string, y float64) {
		style := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11}
		mustText(page.AddText(text, style, pdf.Rectangle{
			LLX: 50, LLY: y, URX: 50 + labelW, URY: y + 18,
		}))
	}

	// Header.
	mustText(page.AddText("AcroForm Fields",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 16, HAlign: pdf.HAlignCenter},
		pdf.Rectangle{LLX: 50, LLY: 770, URX: 545, URY: 800}))

	// Row 1: text field.
	addLabel("Full name:", 720)
	tb, err := form.AddTextField(pageNum, pdf.Rectangle{LLX: 200, LLY: 720, URX: 450, URY: 740}, "FullName")
	if err != nil {
		log.Fatalf("text field: %v", err)
	}
	tb.SetValue("Alice Sample")

	// Row 2: checkbox.
	addLabel("Subscribe:", 680)
	cb, err := form.AddCheckbox(pageNum, pdf.Rectangle{LLX: 200, LLY: 680, URX: 218, URY: 698}, "Subscribe")
	if err != nil {
		log.Fatalf("checkbox: %v", err)
	}
	cb.SetValue("Yes")

	// Row 3: radio group (3 options arranged horizontally).
	addLabel("Plan:", 640)
	rb, err := form.AddRadioGroup("Plan", []pdf.RadioItem{
		{PageNum: pageNum, Rect: pdf.Rectangle{LLX: 200, LLY: 640, URX: 218, URY: 658}, Export: "Basic"},
		{PageNum: pageNum, Rect: pdf.Rectangle{LLX: 290, LLY: 640, URX: 308, URY: 658}, Export: "Pro"},
		{PageNum: pageNum, Rect: pdf.Rectangle{LLX: 380, LLY: 640, URX: 398, URY: 658}, Export: "Enterprise"},
	})
	if err != nil {
		log.Fatalf("radio group: %v", err)
	}
	rb.SetValue("Pro")

	// Inline labels for the radio options.
	radioLabel := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}
	mustText(page.AddText("Basic", radioLabel, pdf.Rectangle{LLX: 222, LLY: 642, URX: 280, URY: 658}))
	mustText(page.AddText("Pro", radioLabel, pdf.Rectangle{LLX: 312, LLY: 642, URX: 370, URY: 658}))
	mustText(page.AddText("Enterprise", radioLabel, pdf.Rectangle{LLX: 402, LLY: 642, URX: 480, URY: 658}))

	// Row 4: combo box.
	addLabel("Country:", 600)
	combo, err := form.AddComboBox(pageNum, pdf.Rectangle{LLX: 200, LLY: 600, URX: 350, URY: 620}, "Country",
		[]pdf.ChoiceOption{
			{Value: "United States", Export: "US"},
			{Value: "United Kingdom", Export: "UK"},
			{Value: "Germany", Export: "DE"},
			{Value: "Japan", Export: "JP"},
		})
	if err != nil {
		log.Fatalf("combo box: %v", err)
	}
	combo.SetValue("United States")

	// Row 5: list box.
	addLabel("Interests:", 540)
	lb, err := form.AddListBox(pageNum, pdf.Rectangle{LLX: 200, LLY: 460, URX: 350, URY: 560}, "Interests",
		[]pdf.ChoiceOption{
			{Value: "PDF Engineering", Export: "pdf"},
			{Value: "Cryptography", Export: "crypto"},
			{Value: "Typography", Export: "type"},
			{Value: "Color Science", Export: "color"},
		})
	if err != nil {
		log.Fatalf("list box: %v", err)
	}
	lb.SetMultiSelect(true)
	lb.SetValue("PDF Engineering")

	// Row 6: push button.
	addLabel("Submit:", 420)
	if _, err := form.AddPushButton(pageNum,
		pdf.Rectangle{LLX: 200, LLY: 415, URX: 320, URY: 445}, "SubmitBtn", "Submit Form"); err != nil {
		log.Fatalf("push button: %v", err)
	}
}

// ---------------------------------------------------------------------
// Page 4 — every supported annotation
// ---------------------------------------------------------------------

func addAnnotations(page *pdf.Page) {
	// Heading.
	mustText(page.AddText("Page 4 — Annotations",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 16, HAlign: pdf.HAlignCenter},
		pdf.Rectangle{LLX: 50, LLY: 770, URX: 545, URY: 800}))

	col := page.Annotations()

	// --- Markup annotations sit on top of underlying text so they're visible. ---
	mustText(page.AddText("Highlight this sentence and underline this phrase. Squiggle me and strike me through.",
		pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 720, URX: 545, URY: 745}))

	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 720, URX: 200, URY: 745})
	hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	hl.SetContents("Yellow highlight")
	mustAnnot(col.Add(hl))

	un := pdf.NewUnderlineAnnotation(page, pdf.Rectangle{LLX: 210, LLY: 720, URX: 320, URY: 745})
	un.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	un.SetContents("Underline")
	mustAnnot(col.Add(un))

	sq := pdf.NewSquigglyAnnotation(page, pdf.Rectangle{LLX: 330, LLY: 720, URX: 420, URY: 745})
	sq.SetColor(&pdf.Color{R: 1, G: 0.5, B: 0, A: 1})
	sq.SetContents("Squiggly")
	mustAnnot(col.Add(sq))

	st := pdf.NewStrikeOutAnnotation(page, pdf.Rectangle{LLX: 430, LLY: 720, URX: 545, URY: 745})
	st.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	st.SetContents("Strike-through")
	mustAnnot(col.Add(st))

	// --- Link with URI action. ---
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 680, URX: 250, URY: 700})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	mustAnnot(col.Add(link))
	mustText(page.AddText("→ Open example.com",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11, Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1}},
		pdf.Rectangle{LLX: 50, LLY: 682, URX: 250, URY: 698}))

	// --- Drawing primitives: Square, Circle, Line, Ink. ---
	square := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 580, URX: 150, URY: 650})
	square.SetColor(&pdf.Color{R: 0.8, G: 0, B: 0, A: 1})
	square.SetBorderWidth(2)
	square.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0.5, A: 1})
	mustAnnot(col.Add(square))

	circle := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 170, LLY: 580, URX: 270, URY: 650})
	circle.SetColor(&pdf.Color{R: 0, G: 0.5, B: 0, A: 1})
	circle.SetBorderStyle(pdf.BorderDashed)
	circle.SetDashPattern([]float64{4, 2})
	circle.SetBorderWidth(2)
	mustAnnot(col.Add(circle))

	line := pdf.NewLineAnnotation(page, pdf.Point{X: 290, Y: 580}, pdf.Point{X: 390, Y: 650})
	line.SetColor(&pdf.Color{R: 0, G: 0, B: 0.7, A: 1})
	line.SetBorderWidth(2)
	line.SetStartLineEnding(pdf.LineEndingOpenArrow)
	line.SetEndLineEnding(pdf.LineEndingClosedArrow)
	mustAnnot(col.Add(line))

	ink := pdf.NewInkAnnotation(page, [][]pdf.Point{{
		{X: 410, Y: 595}, {X: 425, Y: 615}, {X: 445, Y: 605}, {X: 465, Y: 625}, {X: 485, Y: 615},
		{X: 505, Y: 635}, {X: 525, Y: 620},
	}})
	ink.SetColor(&pdf.Color{R: 0.6, G: 0, B: 0.6, A: 1})
	ink.SetBorderWidth(2)
	mustAnnot(col.Add(ink))

	// --- Text-bearing annotations: Text (sticky note), FreeText, Stamp. ---
	note := pdf.NewTextAnnotation(page, pdf.Point{X: 60, Y: 510})
	note.SetIcon(pdf.TextIconNote)
	note.SetTitle("Reviewer")
	note.SetContents("This is a sticky-note annotation. Click the icon to read.")
	mustAnnot(col.Add(note))

	freeText := pdf.NewFreeTextAnnotation(page, pdf.Rectangle{LLX: 110, LLY: 480, URX: 300, URY: 540},
		"FreeText: rendered text\ndrawn directly on page",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11,
			Color:      &pdf.Color{R: 0, G: 0, B: 0, A: 1},
			Background: &pdf.Color{R: 1, G: 1, B: 0.8, A: 1},
		})
	freeText.SetBorderWidth(1)
	mustAnnot(col.Add(freeText))

	stamp := pdf.NewStampAnnotation(page,
		pdf.Rectangle{LLX: 320, LLY: 480, URX: 530, URY: 540},
		pdf.StampNameApproved)
	mustAnnot(col.Add(stamp))

	// --- FileAttachment: embeds a small text file. ---
	att := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 60, Y: 420})
	att.SetIcon(pdf.FileAttachmentIconPaperclip)
	att.SetTitle("Reviewer")
	att.SetContents("Quarterly report — see attachment")
	if err := att.SetFileFromStream(
		strings.NewReader("Confidential report contents (demonstration only)."),
		"q3-report.txt"); err != nil {
		log.Fatalf("attach file: %v", err)
	}
	att.SetFileDescription("Q3 financial summary")
	mustAnnot(col.Add(att))
	mustText(page.AddText("← Embedded file attachment (open the paperclip icon)",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10, Color: &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1}},
		pdf.Rectangle{LLX: 90, LLY: 415, URX: 545, URY: 430}))

	// (The /Redact annotation type has its own dedicated demo page —
	// see addRedactionDemo. Splitting it out lets ApplyRedactions run on
	// just the demo content without affecting the rest of this showcase.)
}

// ---------------------------------------------------------------------
// Page 5 — restaurant bill rendered as a Table
// ---------------------------------------------------------------------

func addRestaurantBill(page *pdf.Page) {
	size, _ := page.Size()

	// Restaurant name.
	titleStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   24,
		Color:  &pdf.Color{R: 0.6, G: 0.3, B: 0.1, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Trattoria da Marco", titleStyle, pdf.Rectangle{
		LLX: 50, LLY: size.Height - 90, URX: size.Width - 50, URY: size.Height - 55,
	}))

	// Tagline.
	taglineStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   12,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Authentic Italian Cuisine — Receipt", taglineStyle, pdf.Rectangle{
		LLX: 50, LLY: size.Height - 115, URX: size.Width - 50, URY: size.Height - 95,
	}))

	// Order info line.
	infoStyle := pdf.TextStyle{
		Font:   pdf.FontHelvetica,
		Size:   10,
		Color:  &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Date: 2026-05-19    Table: 7    Server: Marco    Receipt #: 4218",
		infoStyle, pdf.Rectangle{
			LLX: 50, LLY: size.Height - 140, URX: size.Width - 50, URY: size.Height - 122,
		}))

	// Table: 4 columns Item / Qty / Unit Price / Total.
	table := pdf.NewTable().
		SetColumnWidths([]float64{260, 50, 75, 75}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1,
			Color: &pdf.Color{R: 0.6, G: 0.3, B: 0.1, A: 1}}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.4,
			Color: &pdf.Color{R: 0.75, G: 0.75, B: 0.75, A: 1}}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 5, Right: 8, Bottom: 5, Left: 8}).
		SetDefaultCellStyle(pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})

	// Header row.
	headerBG := &pdf.Color{R: 0.6, G: 0.3, B: 0.1, A: 1}
	headerStyle := pdf.TextStyle{
		Font:  pdf.FontHelveticaBold,
		Size:  11,
		Color: &pdf.Color{R: 1, G: 1, B: 1, A: 1},
	}
	header := table.AddRow()
	for i, t := range []string{"Item", "Qty", "Unit Price", "Total"} {
		c := header.AddCell(t)
		c.SetBackground(headerBG)
		c.SetTextStyle(headerStyle)
		switch i {
		case 0:
			c.SetHAlign(pdf.HAlignLeft)
		case 1:
			c.SetHAlign(pdf.HAlignCenter)
		default:
			c.SetHAlign(pdf.HAlignRight)
		}
	}

	// Menu items.
	items := []struct {
		name        string
		qty         int
		unit, total float64
	}{
		{"Bruschetta al Pomodoro", 2, 8.50, 17.00},
		{"Insalata Caprese", 1, 12.00, 12.00},
		{"Spaghetti alla Carbonara", 2, 16.50, 33.00},
		{"Pizza Margherita", 1, 14.00, 14.00},
		{"Tiramisu", 2, 7.50, 15.00},
		{"House Red Wine (bottle)", 1, 28.00, 28.00},
		{"Espresso", 4, 3.50, 14.00},
	}
	var subtotal float64
	for _, it := range items {
		subtotal += it.total
		row := table.AddRow()
		row.AddCell(it.name).SetHAlign(pdf.HAlignLeft)
		row.AddCell(fmt.Sprintf("%d", it.qty)).SetHAlign(pdf.HAlignCenter)
		row.AddCell(fmt.Sprintf("€%.2f", it.unit)).SetHAlign(pdf.HAlignRight)
		row.AddCell(fmt.Sprintf("€%.2f", it.total)).SetHAlign(pdf.HAlignRight)
	}

	// Summary rows (label spans cells 1-3 visually via right-alignment; the
	// MVP Table API has no rowspan/colspan, so we fill empty cells for cols
	// 1-2 and use a right-aligned label in col 3.)
	addSummary := func(label string, amount float64, bold bool, bg *pdf.Color) {
		labelStyle := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11}
		amountStyle := labelStyle
		if bold {
			labelStyle.Font = pdf.FontHelveticaBold
			amountStyle.Font = pdf.FontHelveticaBold
			labelStyle.Size = 12
			amountStyle.Size = 12
		}
		row := table.AddRow()
		if bg != nil {
			row.SetBackground(bg) // Phase 3 — row-level background, no per-cell setup
		}
		// One label cell spans the first 3 columns (Item / Qty / Unit Price),
		// then the amount cell on the right.
		row.AddCell(label).SetColSpan(3).SetTextStyle(labelStyle).SetHAlign(pdf.HAlignRight)
		row.AddCell(fmt.Sprintf("€%.2f", amount)).SetTextStyle(amountStyle).SetHAlign(pdf.HAlignRight)
	}
	tax := subtotal * 0.10
	service := subtotal * 0.15
	total := subtotal + tax + service
	addSummary("Subtotal:", subtotal, false, nil)
	addSummary("Tax (10%):", tax, false, nil)
	addSummary("Service (15%):", service, false, nil)
	addSummary("TOTAL:", total, true, &pdf.Color{R: 0.97, G: 0.93, B: 0.85, A: 1})

	// Render the table — width 460pt centered on A4 (595 - 460 = 135 → 67.5 margin).
	const tableLLX, tableURX = 67.5, 527.5
	if _, err := page.AddTable(table, pdf.Rectangle{
		LLX: tableLLX, LLY: 200, URX: tableURX, URY: size.Height - 165,
	}); err != nil {
		log.Fatalf("add table: %v", err)
	}

	// Thank-you line below the table.
	thanksStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   14,
		Color:  &pdf.Color{R: 0.6, G: 0.3, B: 0.1, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Grazie mille e a presto!", thanksStyle, pdf.Rectangle{
		LLX: 50, LLY: 140, URX: size.Width - 50, URY: 175,
	}))

	// (Unified page footer is added later in main() — addUnifiedFooter.)
}

// ---------------------------------------------------------------------
// Page 6+ — multi-page Sales Report Table
//
// Exercises every Table feature shipped through Phase 3:
//   - Image cell in header (Cell.SetImage in a ColSpan'd cell)
//   - ColSpan for header bar, section dividers, and TOTAL row
//   - Repeating header rows (Table.SetRepeatingRowsCount) — 3 rows repeat on each page
//   - Multi-page overflow (Table.SetOverflowMargins + auto-append continuation pages)
//   - Row-level styling (Row.SetBackground, Row.SetTextStyle, Row.SetHeight)
//   - Batch body construction (Table.AddRows([][]string))
//   - Per-cell HAlign/VAlign overrides on top of row defaults
//   - Custom cell border + table outer border with edge de-duplication
// ---------------------------------------------------------------------

func addSalesReport(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	// Title and feature list above the table.
	titleStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   22,
		Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Multi-Page Sales Report", titleStyle, pdf.Rectangle{
		LLX: 50, LLY: size.Height - 88, URX: size.Width - 50, URY: size.Height - 55,
	}))
	subStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   10,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText(
		"image header  •  repeating headers  •  ColSpan  •  Row.SetBackground  •  AddRows batch  •  overflow",
		subStyle, pdf.Rectangle{
			LLX: 50, LLY: size.Height - 110, URX: size.Width - 50, URY: size.Height - 95,
		}))

	// Palette.
	navy := &pdf.Color{R: 0.10, G: 0.15, B: 0.40, A: 1}
	white := &pdf.Color{R: 1, G: 1, B: 1, A: 1}
	titleBG := &pdf.Color{R: 0.94, G: 0.95, B: 0.99, A: 1}
	sectionBG := &pdf.Color{R: 0.85, G: 0.88, B: 0.95, A: 1}
	zebraBG := &pdf.Color{R: 0.97, G: 0.97, B: 0.97, A: 1}
	totalBG := &pdf.Color{R: 0.97, G: 0.93, B: 0.85, A: 1}

	// Build the table.
	table := pdf.NewTable().
		SetColumnWidths([]float64{260, 60, 80, 80}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1, Color: navy}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.4,
			Color: &pdf.Color{R: 0.78, G: 0.78, B: 0.78, A: 1}}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 6, Bottom: 4, Left: 6}).
		SetDefaultCellStyle(pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}).
		SetRepeatingRowsCount(3).
		SetOverflowMargins(60, 60)

	// ---- Header rows (3 rows, all marked as repeating) ----

	// Row 0: logo (image cell, ColSpan 4). Row.SetHeight constrains the image
	// to a banner-style strip; the image scales to fit while preserving aspect.
	logoRow := table.AddRow().SetHeight(54).SetBackground(navy)
	logoRow.AddCell("").
		SetColSpan(4).
		SetImage("testdata/Koala.jpg").
		SetHAlign(pdf.HAlignCenter).
		SetVAlign(pdf.VAlignMiddle)

	// Row 1: title text (ColSpan 4) with a soft tinted background.
	titleRow := table.AddRow().SetHeight(28).SetBackground(titleBG)
	titleRow.AddCell("Trattoria da Marco  —  Quarterly Sales Report  (Q3 2026)").
		SetColSpan(4).
		SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: navy}).
		SetHAlign(pdf.HAlignCenter).
		SetVAlign(pdf.VAlignMiddle)

	// Row 2: column headers — row-level bg + text style propagate to all cells,
	// per-cell HAlign overrides.
	colHeader := table.AddRow().SetHeight(22).SetBackground(navy).
		SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11, Color: white})
	colHeader.AddCell("Item").SetHAlign(pdf.HAlignLeft).SetVAlign(pdf.VAlignMiddle)
	colHeader.AddCell("Qty").SetHAlign(pdf.HAlignCenter).SetVAlign(pdf.VAlignMiddle)
	colHeader.AddCell("Unit Price").SetHAlign(pdf.HAlignRight).SetVAlign(pdf.VAlignMiddle)
	colHeader.AddCell("Revenue").SetHAlign(pdf.HAlignRight).SetVAlign(pdf.VAlignMiddle)

	// ---- Body sections ----

	// Helper: category divider — single ColSpan(4) cell with accent background.
	addCategoryDivider := func(label string) {
		row := table.AddRow().SetBackground(sectionBG)
		row.AddCell(label).
			SetColSpan(4).
			SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11, Color: navy}).
			SetHAlign(pdf.HAlignLeft)
	}

	// Helper: bulk-add product rows via AddRows, apply alternating zebra striping,
	// fix per-cell alignment (qty centered, prices right-aligned), and sum revenue.
	var grandTotal float64
	zebraIdx := 0
	addItems := func(items [][]string) {
		rows := table.AddRows(items)
		for i, row := range rows {
			if (i+zebraIdx)%2 == 1 {
				row.SetBackground(zebraBG)
			}
			cells := row.Cells()
			cells[1].SetHAlign(pdf.HAlignCenter)
			cells[2].SetHAlign(pdf.HAlignRight)
			cells[3].SetHAlign(pdf.HAlignRight)
			rev, _ := strconv.ParseFloat(items[i][3], 64)
			grandTotal += rev
		}
		zebraIdx += len(items)
	}

	// Pasta dishes.
	addCategoryDivider("Pasta Dishes")
	addItems([][]string{
		{"Spaghetti alla Carbonara", "47", "16.50", "775.50"},
		{"Tagliatelle al Ragu Bolognese", "38", "17.00", "646.00"},
		{"Lasagna alla Forno", "29", "18.50", "536.50"},
		{"Fettuccine Alfredo", "24", "16.00", "384.00"},
		{"Penne all'Arrabbiata", "31", "15.00", "465.00"},
		{"Linguine al Pesto Genovese", "26", "16.50", "429.00"},
		{"Ravioli di Spinaci e Ricotta", "22", "17.50", "385.00"},
		{"Gnocchi ai Quattro Formaggi", "19", "17.00", "323.00"},
	})

	// Pizza selection.
	addCategoryDivider("Pizza Selection")
	addItems([][]string{
		{"Pizza Margherita", "62", "12.00", "744.00"},
		{"Pizza Quattro Formaggi", "41", "14.50", "594.50"},
		{"Pizza Capricciosa", "35", "15.00", "525.00"},
		{"Pizza Diavola", "33", "14.00", "462.00"},
		{"Pizza Marinara", "28", "11.00", "308.00"},
		{"Pizza Napoletana", "39", "13.50", "526.50"},
		{"Pizza Prosciutto e Funghi", "37", "15.50", "573.50"},
		{"Pizza Quattro Stagioni", "30", "16.00", "480.00"},
	})

	// Antipasti.
	addCategoryDivider("Antipasti")
	addItems([][]string{
		{"Bruschetta al Pomodoro", "54", "8.50", "459.00"},
		{"Carpaccio di Manzo", "21", "14.00", "294.00"},
		{"Insalata Caprese", "33", "12.00", "396.00"},
		{"Vitello Tonnato", "18", "16.50", "297.00"},
	})

	// Desserts.
	addCategoryDivider("Desserts")
	addItems([][]string{
		{"Tiramisu Classico", "67", "7.50", "502.50"},
		{"Panna Cotta ai Frutti di Bosco", "44", "7.00", "308.00"},
		{"Cannoli Siciliani", "32", "6.50", "208.00"},
		{"Gelato Misto (3 scoops)", "58", "6.00", "348.00"},
		{"Sfogliatella Napoletana", "27", "7.50", "202.50"},
	})

	// Beverages.
	addCategoryDivider("Beverages")
	addItems([][]string{
		{"House Red Wine (Chianti, bottle)", "42", "28.00", "1176.00"},
		{"House White Wine (Pinot Grigio, bottle)", "36", "26.00", "936.00"},
		{"Sparkling Water (Acqua Frizzante, 1L)", "89", "4.50", "400.50"},
		{"Espresso", "215", "3.50", "752.50"},
		{"Cappuccino", "127", "4.50", "571.50"},
		{"Limoncello (glass)", "53", "8.00", "424.00"},
	})

	// ---- TOTAL row: ColSpan(3) label + grand total, row-level bg + custom margin ----
	totalRow := table.AddRow().
		SetHeight(32).
		SetBackground(totalBG).
		SetMargin(pdf.MarginInfo{Top: 6, Right: 8, Bottom: 6, Left: 8})
	totalRow.AddCell("GRAND TOTAL").
		SetColSpan(3).
		SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 13, Color: navy}).
		SetHAlign(pdf.HAlignRight).
		SetVAlign(pdf.VAlignMiddle)
	totalRow.AddCell(fmt.Sprintf("€%.2f", grandTotal)).
		SetTextStyle(pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: navy}).
		SetHAlign(pdf.HAlignRight).
		SetVAlign(pdf.VAlignMiddle)

	// Render — overflow logic auto-appends continuation pages with repeated headers.
	pagesAdded, err := page.AddTable(table, pdf.Rectangle{
		LLX: 50, LLY: 70, URX: size.Width - 50, URY: size.Height - 130,
	})
	if err != nil {
		log.Fatalf("add sales table: %v", err)
	}
	log.Printf("sales report: %d continuation pages auto-appended", pagesAdded)

	// (Unified page footer is added later in main() — addUnifiedFooter.)
}

// ---------------------------------------------------------------------
// Page 7 — vector graphics showcase
//
// Exercises every (*Page).Draw* method shipped in Vector Phase 1:
//   - DrawLine (axis lines, with dash pattern + round cap)
//   - DrawRectangle (bar fills + a semi-transparent overlay)
//   - DrawRoundedRectangle (callout box)
//   - DrawCircle (highlight marker on the peak bar)
//   - DrawEllipse (decorative shape)
//   - DrawPolyline (trend line through bar tops)
//   - DrawPolygon (triangular alert marker)
//   - DrawPath with MoveTo / LineTo / CurveTo / Arc / Close (pie slice + smile)
// ---------------------------------------------------------------------

func addVectorShowcase(doc *pdf.Document, page *pdf.Page) {
	_ = doc // kept for signature symmetry with addPageText / addSalesReport

	size, _ := page.Size()

	// Title.
	mustText(page.AddText("Vector Graphics Showcase",
		pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 22,
			Color:  &pdf.Color{R: 0.1, G: 0.5, B: 0.3, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 88, URX: size.Width - 50, URY: size.Height - 55},
	))
	mustText(page.AddText(
		"DrawLine  •  DrawRectangle  •  DrawCircle  •  DrawEllipse  •  DrawPolyline  •  DrawPolygon  •  DrawPath  •  RoundedRectangle  •  Arc",
		pdf.TextStyle{
			Font: pdf.FontHelveticaOblique, Size: 10,
			Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 112, URX: size.Width - 50, URY: size.Height - 96},
	))

	// === Bar chart ===
	chartHeader := pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 12,
		Color:  &pdf.Color{R: 0.1, G: 0.5, B: 0.3, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	mustText(page.AddText("Monthly Sales Trend (€ thousands)", chartHeader,
		pdf.Rectangle{LLX: 50, LLY: 720, URX: size.Width - 50, URY: 738}))

	const (
		chartLeft   = 90.0
		chartRight  = 530.0
		chartBottom = 500.0
		chartTop    = 700.0
	)
	// Y-axis (dashed).
	if err := page.DrawLine(
		pdf.Point{X: chartLeft, Y: chartBottom},
		pdf.Point{X: chartLeft, Y: chartTop},
		pdf.LineStyle{
			Color:       &pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1},
			Width:       0.75,
			DashPattern: []float64{3, 2},
		},
	); err != nil {
		log.Fatalf("y-axis: %v", err)
	}
	// X-axis (solid).
	if err := page.DrawLine(
		pdf.Point{X: chartLeft, Y: chartBottom},
		pdf.Point{X: chartRight, Y: chartBottom},
		pdf.LineStyle{
			Color: &pdf.Color{R: 0.2, G: 0.2, B: 0.2, A: 1},
			Width: 1.5,
			Cap:   pdf.LineCapRound,
		},
	); err != nil {
		log.Fatalf("x-axis: %v", err)
	}

	// 7 monthly bars.
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul"}
	values := []float64{22, 28, 34, 27, 31, 25, 29} // €k
	barWidth := (chartRight - chartLeft - 30) / float64(len(months))
	const maxBar = 40.0                                     // scale: 40k = full chart height
	barColor := &pdf.Color{R: 0.3, G: 0.6, B: 0.9, A: 0.85} // slight transparency
	barTops := make([]pdf.Point, len(months))
	bestIdx := 0
	for i, v := range values {
		if v > values[bestIdx] {
			bestIdx = i
		}
		barH := (v / maxBar) * (chartTop - chartBottom - 20)
		x := chartLeft + 15 + float64(i)*barWidth
		barTop := chartBottom + barH
		if err := page.DrawRectangle(
			pdf.Rectangle{LLX: x, LLY: chartBottom, URX: x + barWidth - 8, URY: barTop},
			pdf.ShapeStyle{
				LineStyle: pdf.LineStyle{Width: 0.5, Color: &pdf.Color{R: 0.1, G: 0.3, B: 0.6, A: 1}},
				FillColor: barColor,
			},
		); err != nil {
			log.Fatalf("bar %d: %v", i, err)
		}
		mustText(page.AddText(months[i],
			pdf.TextStyle{
				Font: pdf.FontHelvetica, Size: 9,
				Color:  &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1},
				HAlign: pdf.HAlignCenter,
			},
			pdf.Rectangle{LLX: x, LLY: chartBottom - 14, URX: x + barWidth - 8, URY: chartBottom - 2},
		))
		// Track center-top of each bar for the trend polyline.
		barTops[i] = pdf.Point{X: x + (barWidth-8)/2, Y: barTop}
	}

	// Trend polyline (DrawPolyline).
	if err := page.DrawPolyline(barTops, pdf.LineStyle{
		Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1},
		Width: 1.5,
		Cap:   pdf.LineCapRound,
		Join:  pdf.LineJoinRound,
	}); err != nil {
		log.Fatalf("trend line: %v", err)
	}

	// Highlight circle on the best month.
	if err := page.DrawCircle(barTops[bestIdx], 6, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.5, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1}},
		FillColor: &pdf.Color{R: 1, G: 1, B: 1, A: 1},
	}); err != nil {
		log.Fatalf("highlight circle: %v", err)
	}

	// === Decorations row at y ≈ 420..460 ===
	// Rounded-rectangle callout for the peak month.
	calloutRect := pdf.Rectangle{LLX: 90, LLY: 420, URX: 280, URY: 460}
	if err := page.DrawRoundedRectangle(calloutRect, 8, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1}},
		FillColor: &pdf.Color{R: 1, G: 0.97, B: 0.85, A: 1},
	}); err != nil {
		log.Fatalf("callout: %v", err)
	}
	mustText(page.AddText(
		fmt.Sprintf("Peak: %s — €%.0fk", months[bestIdx], values[bestIdx]),
		pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 12,
			Color:  &pdf.Color{R: 0.6, G: 0.35, B: 0.05, A: 1},
			HAlign: pdf.HAlignCenter,
			VAlign: pdf.VAlignMiddle,
		},
		calloutRect,
	))

	// Triangle alert marker.
	if err := page.DrawPolygon([]pdf.Point{
		{X: 310, Y: 430}, {X: 350, Y: 430}, {X: 330, Y: 458},
	}, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0.85, G: 0.10, B: 0.10, A: 1}},
		FillColor: &pdf.Color{R: 1, G: 0.9, B: 0.4, A: 1},
	}); err != nil {
		log.Fatalf("triangle: %v", err)
	}
	mustText(page.AddText("!",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 16,
			Color: &pdf.Color{R: 0.85, G: 0.10, B: 0.10, A: 1}, HAlign: pdf.HAlignCenter},
		pdf.Rectangle{LLX: 315, LLY: 438, URX: 345, URY: 456}))

	// Decorative ellipse.
	if err := page.DrawEllipse(pdf.Point{X: 410, Y: 440}, 30, 16, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0.1, G: 0.5, B: 0.3, A: 1}},
		FillColor: &pdf.Color{R: 0.85, G: 0.95, B: 0.85, A: 0.7},
	}); err != nil {
		log.Fatalf("ellipse: %v", err)
	}

	// Pie slice using Path with Arc.
	piePath := pdf.NewPath().
		MoveTo(490, 440).
		LineTo(530, 440).
		Arc(490, 440, 40, 0, 1.0472). // 60° slice
		Close()
	if err := page.DrawPath(piePath, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0.6, G: 0.3, B: 0.7, A: 1}},
		FillColor: &pdf.Color{R: 0.85, G: 0.75, B: 0.95, A: 1},
	}); err != nil {
		log.Fatalf("pie slice: %v", err)
	}

	// === Path showcase: smile-shaped curve at the bottom ===
	smile := pdf.NewPath().
		MoveTo(200, 200).
		CurveTo(220, 170, 280, 170, 300, 200).
		MoveTo(170, 240).
		LineTo(170, 260).
		MoveTo(330, 240).
		LineTo(330, 260)
	if err := page.DrawPath(smile, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{
			Width: 3, Cap: pdf.LineCapRound, Join: pdf.LineJoinRound,
			Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1},
		},
	}); err != nil {
		log.Fatalf("smile: %v", err)
	}

	// Semi-transparent watermark-like overlay rectangle (demos alpha).
	if err := page.DrawRectangle(
		pdf.Rectangle{LLX: 50, LLY: 120, URX: size.Width - 50, URY: 175},
		pdf.ShapeStyle{
			FillColor: &pdf.Color{R: 0.1, G: 0.5, B: 0.3, A: 0.18},
		},
	); err != nil {
		log.Fatalf("alpha rect: %v", err)
	}
	mustText(page.AddText(
		"Every primitive above uses vector ops emitted by the new (*Page).DrawX API.",
		pdf.TextStyle{
			Font: pdf.FontHelveticaOblique, Size: 11,
			Color:  &pdf.Color{R: 0.1, G: 0.4, B: 0.25, A: 1},
			HAlign: pdf.HAlignCenter, VAlign: pdf.VAlignMiddle,
		},
		pdf.Rectangle{LLX: 60, LLY: 125, URX: size.Width - 60, URY: 170},
	))

	// (Unified page footer is added later in main() — addUnifiedFooter.)
}

// ---------------------------------------------------------------------
// Outlines (bookmarks)
// ---------------------------------------------------------------------

// addBookmarks builds a two-level outline tree: one top-level entry per
// section pointing at a named destination, plus per-category sub-bookmarks
// nested under "Sales Report". Named destinations are forward references
// (resolved at view time), so they keep working even after redaction
// rewrites content streams.
func addBookmarks(doc *pdf.Document, sections []section) {
	root := doc.Outlines()

	colors := map[string]*pdf.Color{
		"text":        {R: 0.15, G: 0.20, B: 0.55, A: 1},
		"image":       nil,
		"form":        nil,
		"annotations": {R: 0.6, G: 0, B: 0.6, A: 1},
		"redaction":   {R: 0.0, G: 0.0, B: 0.0, A: 1},
		"bill":        {R: 0.6, G: 0.3, B: 0.1, A: 1},
		"sales":       {R: 0.1, G: 0.15, B: 0.4, A: 1},
		"landscape":   {R: 0.4, G: 0.3, B: 0.6, A: 1},
		"vector":      {R: 0.1, G: 0.5, B: 0.3, A: 1},
	}

	for _, s := range sections {
		entry := pdf.NewOutlineItemCollection(doc)
		entry.SetTitle(s.title)
		entry.SetDestination(pdf.NewNamedDestination(doc, s.dest))
		if c := colors[s.subtype]; c != nil {
			entry.SetColor(c)
		}
		entry.SetBold(s.subtype == "text" || s.subtype == "bill" ||
			s.subtype == "sales" || s.subtype == "vector")
		mustAddOutline(root.Add(entry))

		if s.subtype == "sales" {
			entry.SetIsExpanded(true)
			for _, cat := range []string{"Pasta", "Pizza", "Antipasti", "Desserts", "Beverages"} {
				sub := pdf.NewOutlineItemCollection(doc)
				sub.SetTitle(cat)
				sub.SetDestination(pdf.NewNamedDestination(doc, s.dest))
				mustAddOutline(entry.Add(sub))
			}
		}
	}
}

// ---------------------------------------------------------------------
// Cover page
// ---------------------------------------------------------------------

// addCoverPage paints the front cover: large Aspose SVG mark, project title,
// subtitle, version + creation date. Uses no watermark / logo stamp — the
// cover stays visually clean.
func addCoverPage(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	// Soft background tint band across the centre of the page so the title
	// sits on a colour stripe rather than empty white.
	bandRect := pdf.Rectangle{LLX: 0, LLY: size.Height/2 - 80, URX: size.Width, URY: size.Height/2 + 90}
	mustVector(page.DrawRectangle(bandRect, pdf.ShapeStyle{
		FillColor: &pdf.Color{R: 0.95, G: 0.96, B: 0.99, A: 1},
	}))

	// Big Aspose logo, centred horizontally, upper third of the band.
	if svg, err := doc.LoadSVG("testdata/aspose-logo.svg"); err == nil {
		const logoW = 360.0
		logoH := logoW * 100.0 / 314.0 // viewBox aspect 3.14:1
		x := (size.Width - logoW) / 2
		y := size.Height/2 + 10
		mustErr(page.AddSVGObject(svg, pdf.Rectangle{LLX: x, LLY: y, URX: x + logoW, URY: y + logoH}))
	}

	// Title — wide rect so AutoText doesn't wrap, generous vertical room
	// so descenders aren't clipped.
	mustText(page.AddText(docTitle, pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   24,
		Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 30, LLY: size.Height/2 - 35, URX: size.Width - 30, URY: size.Height/2 - 5}))

	// Subtitle.
	mustText(page.AddText("An end-to-end tour of every feature in one document.", pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   13,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.45, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: size.Height/2 - 65, URX: size.Width - 50, URY: size.Height/2 - 40}))

	// Footer: version + date, bottom-centre.
	dateStr := time.Now().Format("January 2, 2006")
	mustText(page.AddText(docVersion+"  ·  "+dateStr, pdf.TextStyle{
		Font:   pdf.FontHelvetica,
		Size:   11,
		Color:  &pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: 80, URX: size.Width - 50, URY: 100}))
}

// ---------------------------------------------------------------------
// Table of Contents
// ---------------------------------------------------------------------

// addTOC renders the table of contents and overlays a LinkAnnotation on each
// row so clicking jumps to the corresponding section via its named
// destination. Uses a leader-dot row style ("Title.....1") and respects the
// page label of the destination page (so the body section labelled "1" is
// shown as "1", not its absolute index).
func addTOC(doc *pdf.Document, page *pdf.Page, sections []section) {
	size, _ := page.Size()

	// Heading.
	mustText(page.AddText("Contents", pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   30,
		Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: size.Height - 110, URX: size.Width - 50, URY: size.Height - 70}))

	// Horizontal rule under the title.
	mustVector(page.DrawLine(
		pdf.Point{X: 80, Y: size.Height - 125},
		pdf.Point{X: size.Width - 80, Y: size.Height - 125},
		pdf.LineStyle{Color: &pdf.Color{R: 0.85, G: 0.85, B: 0.9, A: 1}, Width: 1},
	))

	annot := page.Annotations()
	rowH := 32.0
	yTop := size.Height - 160.0
	for i, s := range sections {
		y := yTop - float64(i)*rowH

		// Page label of the destination — uses /PageLabels at view time, but
		// because we haven't called SetPageLabels yet at this point it falls
		// back to absolute index. SetPageLabels runs later in main(); the TOC
		// page-number text reflects the *expected* label here.
		var label string
		if i == 0 {
			label = "1"
		} else {
			label = strconv.Itoa(i + 1)
		}

		// Title on the left.
		mustText(page.AddText(s.title, pdf.TextStyle{
			Font:  pdf.FontHelvetica,
			Size:  13,
			Color: &pdf.Color{R: 0.1, G: 0.1, B: 0.15, A: 1},
		}, pdf.Rectangle{LLX: 100, LLY: y, URX: 380, URY: y + 20}))

		// Page number on the right.
		mustText(page.AddText(label, pdf.TextStyle{
			Font:   pdf.FontHelveticaBold,
			Size:   13,
			Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
			HAlign: pdf.HAlignRight,
		}, pdf.Rectangle{LLX: size.Width - 130, LLY: y, URX: size.Width - 100, URY: y + 20}))

		// Leader dots between title and page number.
		mustText(page.AddText(strings.Repeat(". ", 80), pdf.TextStyle{
			Font:  pdf.FontHelvetica,
			Size:  11,
			Color: &pdf.Color{R: 0.7, G: 0.7, B: 0.75, A: 1},
		}, pdf.Rectangle{LLX: 250, LLY: y + 2, URX: size.Width - 135, URY: y + 18}))

		// Clickable link over the whole row → GoTo the section's page.
		// (LinkAnnotation currently only supports /A action, not /Dest, so we
		// jump by absolute page number rather than via the named destination.
		// The named destinations are still emitted into the catalog — the
		// outline (bookmarks) tree resolves through them.)
		link := pdf.NewLinkAnnotation(page, pdf.Rectangle{
			LLX: 95, LLY: y, URX: size.Width - 95, URY: y + 22,
		})
		topY, _ := s.page.Size()
		link.SetAction(pdf.NewGoToAction(s.page.Number(), topY.Height))
		link.SetHighlight(pdf.LinkHighlightOutline)
		mustAnnot(annot.Add(link))
	}

	// Hint below the list.
	mustText(page.AddText("Click any title above to jump to the section.", pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   10,
		Color:  &pdf.Color{R: 0.55, G: 0.55, B: 0.6, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: 120, URX: size.Width - 50, URY: 140}))
}

// ---------------------------------------------------------------------
// Redaction demo
// ---------------------------------------------------------------------

// addRedactionDemo lays out a memo whose sensitive fields each sit on their
// own line with known coordinates, so the /Redact annotations can target
// each exactly. Document.ApplyRedactions() (called from main before save)
// destroys the underlying text glyphs inside the quad regions and stamps the
// overlay text — so by the time the PDF reaches the viewer the redacted
// version is the *only* version present.
func addRedactionDemo(page *pdf.Page) {
	size, _ := page.Size()

	mustText(page.AddText("Redactions", pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 26,
		Color:  &pdf.Color{R: 0, G: 0, B: 0, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: size.Height - 100, URX: size.Width - 50, URY: size.Height - 65}))

	mustText(page.AddText("Document.ApplyRedactions destructively removes text glyphs, image XObjects, and path operators inside every /Redact annotation. Each sensitive field below is on its own line so the redaction rectangles can target it precisely.",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11, Color: &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1}, LineSpacing: 1.4},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 175, URX: size.Width - 50, URY: size.Height - 115}))

	// Header line.
	mustText(page.AddText("Internal memo — Q3 personnel changes",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}},
		pdf.Rectangle{LLX: 60, LLY: size.Height - 215, URX: size.Width - 60, URY: size.Height - 195}))

	// Each row: a label on the left, a value on the right. We give the value
	// rect a fixed Y range so the redact rectangle below maps to it cleanly.
	const (
		valueLLX = 220.0
		valueURX = 500.0
	)
	labelStyle := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 12, Color: &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1}}
	valueStyle := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 12, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}}

	rows := []struct {
		label, value string
		y            float64
	}{
		{"Employee:", "Maria Castellano (ID 47821)", 580},
		{"Retention bonus:", "$185,000.00", 550},
		{"Direct phone:", "+1 (415) 555-0182", 520},
		{"Bank routing:", "026009593", 490},
		{"Account number:", "4421-9087-7733-2104", 460},
		{"Effective date:", "2026-04-15", 430},
	}
	for _, r := range rows {
		mustText(page.AddText(r.label, labelStyle, pdf.Rectangle{
			LLX: 60, LLY: r.y, URX: 215, URY: r.y + 16,
		}))
		mustText(page.AddText(r.value, valueStyle, pdf.Rectangle{
			LLX: valueLLX, LLY: r.y, URX: valueURX, URY: r.y + 16,
		}))
	}

	// Redact the four sensitive rows (bonus, phone, routing, account). We
	// over-extend the rect slightly on each side so all glyphs are covered.
	redactRows := []float64{550, 520, 490, 460}
	annots := page.Annotations()
	overlayStyle := pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 9,
		Color:  &pdf.Color{R: 1, G: 1, B: 1, A: 1},
		HAlign: pdf.HAlignCenter, VAlign: pdf.VAlignMiddle,
	}
	for _, y := range redactRows {
		rect := pdf.Rectangle{LLX: valueLLX - 4, LLY: y - 2, URX: valueURX, URY: y + 18}
		red := pdf.NewRedactAnnotation(page, rect)
		red.SetInteriorColor(&pdf.Color{R: 0, G: 0, B: 0, A: 1})
		red.SetOverlayText("[REDACTED]")
		red.SetOverlayTextStyle(overlayStyle)
		mustAnnot(annots.Add(red))
	}

	// Caption below the rows.
	mustText(page.AddText("Document.ApplyRedactions() runs before save — the original glyphs are not present in the output PDF.",
		pdf.TextStyle{
			Font: pdf.FontHelveticaOblique, Size: 10,
			Color:  &pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: 380, URX: size.Width - 50, URY: 400}))
}

// ---------------------------------------------------------------------
// Landscape wide chart
// ---------------------------------------------------------------------

// addLandscapeChart fills a landscape-oriented A4 page with a wide 12-month
// bar chart. Exercises pdf.PageFormatA4.Landscape() for non-portrait pages,
// plus Path / Line / Rectangle drawing primitives across a much wider canvas
// than any of the portrait sections.
func addLandscapeChart(page *pdf.Page) {
	size, _ := page.Size() // 842 x 595 for A4 landscape

	mustText(page.AddText("Annual Sales — 12 Month Trend", pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 24,
		Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: size.Height - 80, URX: size.Width - 50, URY: size.Height - 50}))

	mustText(page.AddText("Wide-format chart on a landscape A4 (pdf.PageFormatA4.Landscape())",
		pdf.TextStyle{
			Font: pdf.FontHelveticaOblique, Size: 11,
			Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.45, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 105, URX: size.Width - 50, URY: size.Height - 85}))

	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	values := []float64{42, 51, 49, 58, 67, 72, 79, 81, 74, 68, 55, 62}
	const (
		chartLeft   = 80.0
		chartBottom = 110.0
		chartTop    = 440.0
	)
	chartRight := size.Width - 60
	barSlot := (chartRight - chartLeft) / float64(len(months))
	barWidth := barSlot * 0.62

	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	scaleY := (chartTop - chartBottom) / (maxVal * 1.15)

	// Y-axis grid lines.
	for i := 1; i <= 5; i++ {
		y := chartBottom + float64(i)*(chartTop-chartBottom)/5
		mustVector(page.DrawLine(
			pdf.Point{X: chartLeft, Y: y}, pdf.Point{X: chartRight, Y: y},
			pdf.LineStyle{
				Color:       &pdf.Color{R: 0.9, G: 0.9, B: 0.93, A: 1},
				Width:       0.5,
				DashPattern: []float64{2, 3},
			},
		))
		mustText(page.AddText(fmt.Sprintf("€%.0fk", maxVal*float64(i)/5*1.15), pdf.TextStyle{
			Font: pdf.FontHelvetica, Size: 8,
			Color:  &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign: pdf.HAlignRight,
		}, pdf.Rectangle{LLX: chartLeft - 38, LLY: y - 5, URX: chartLeft - 4, URY: y + 5}))
	}

	// X-axis baseline.
	mustVector(page.DrawLine(
		pdf.Point{X: chartLeft, Y: chartBottom}, pdf.Point{X: chartRight, Y: chartBottom},
		pdf.LineStyle{Color: &pdf.Color{R: 0.2, G: 0.2, B: 0.2, A: 1}, Width: 1.2, Cap: pdf.LineCapRound},
	))

	// Bars + month labels + value labels.
	tops := make([]pdf.Point, len(values))
	for i, v := range values {
		x := chartLeft + float64(i)*barSlot + (barSlot-barWidth)/2
		barTop := chartBottom + v*scaleY
		mustVector(page.DrawRectangle(
			pdf.Rectangle{LLX: x, LLY: chartBottom, URX: x + barWidth, URY: barTop},
			pdf.ShapeStyle{
				LineStyle: pdf.LineStyle{Width: 0.6, Color: &pdf.Color{R: 0.1, G: 0.3, B: 0.6, A: 1}},
				FillColor: &pdf.Color{R: 0.3, G: 0.55, B: 0.85, A: 0.92},
			},
		))
		mustText(page.AddText(months[i], pdf.TextStyle{
			Font: pdf.FontHelvetica, Size: 10,
			Color:  &pdf.Color{R: 0.3, G: 0.3, B: 0.35, A: 1},
			HAlign: pdf.HAlignCenter,
		}, pdf.Rectangle{LLX: x - 5, LLY: chartBottom - 18, URX: x + barWidth + 5, URY: chartBottom - 5}))
		mustText(page.AddText(fmt.Sprintf("%.0f", v), pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 9,
			Color:  &pdf.Color{R: 0.1, G: 0.3, B: 0.6, A: 1},
			HAlign: pdf.HAlignCenter,
		}, pdf.Rectangle{LLX: x - 5, LLY: barTop + 2, URX: x + barWidth + 5, URY: barTop + 14}))
		tops[i] = pdf.Point{X: x + barWidth/2, Y: barTop}
	}

	// Trend polyline through bar tops.
	mustVector(page.DrawPolyline(tops, pdf.LineStyle{
		Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1},
		Width: 1.8,
		Cap:   pdf.LineCapRound,
		Join:  pdf.LineJoinRound,
	}))
}

// ---------------------------------------------------------------------
// Per-page footer (unified across the whole document)
// ---------------------------------------------------------------------

// addUnifiedFooter draws a thin separator + centred "<index> / <total>" page
// position on every page. Using indices (not labels) here keeps the
// implementation simple — readers who need symbolic labels read them from
// /PageLabels via (*Page).Label().
func addUnifiedFooter(page *pdf.Page, index, total int) {
	size, _ := page.Size()
	mustVector(page.DrawLine(
		pdf.Point{X: 50, Y: 40}, pdf.Point{X: size.Width - 50, Y: 40},
		pdf.LineStyle{Color: &pdf.Color{R: 0.85, G: 0.85, B: 0.9, A: 1}, Width: 0.5},
	))
	mustText(page.AddText(fmt.Sprintf("%s   ·   %d / %d", docTitle, index, total), pdf.TextStyle{
		Font: pdf.FontHelvetica, Size: 8,
		Color:  &pdf.Color{R: 0.55, G: 0.55, B: 0.6, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: 22, URX: size.Width - 50, URY: 36}))
}

func mustVector(err error) {
	if err != nil {
		log.Fatalf("vector draw: %v", err)
	}
}

func mustErr(err error) {
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

// stampAsposeLogoOnEveryPage loads testdata/aspose-logo.svg once and renders
// it into the top-right corner of every page except the supplied skip list
// (cover + TOC, which carry their own branding). The logo's viewBox is
// 314×100 (aspect 3.14:1); the stamp rect preserves that aspect via
// preserveAspectRatio.
func stampAsposeLogoOnEveryPage(doc *pdf.Document, skip ...*pdf.Page) error {
	svg, err := doc.LoadSVG("testdata/aspose-logo.svg")
	if err != nil {
		return err
	}
	skipSet := make(map[*pdf.Page]bool, len(skip))
	for _, sp := range skip {
		skipSet[sp] = true
	}
	const (
		stampW = 120.0
		stampH = 38.0 // matches viewBox aspect (314/100 * 38 ≈ 119.3)
		margin = 25.0
	)
	for _, p := range doc.Pages() {
		if skipSet[p] {
			continue
		}
		size, err := p.Size()
		if err != nil {
			return err
		}
		urx := size.Width - margin
		ury := size.Height - margin
		rect := pdf.Rectangle{
			LLX: urx - stampW, LLY: ury - stampH,
			URX: urx, URY: ury,
		}
		if err := p.AddSVGObject(svg, rect); err != nil {
			return err
		}
	}
	return nil
}

// addCenteredWatermark places "WATERMARK" geometrically at the page
// center, rotated 45°. Page.AddText rotates around the rect's
// bottom-left corner, so we pre-compute a rect whose post-rotation
// center lands at (pageW/2, pageH/2).
func addCenteredWatermark(page *pdf.Page, text string) error {
	size, err := page.Size()
	if err != nil {
		return err
	}
	const (
		fontSize = 48.0
		rectW    = 340.0 // "WATERMARK" at 48pt bold needs ~315pt — leave margin to avoid wrap
		rectH    = 60.0  // ≈ fontSize + padding
		cos45    = 0.70710678
		sin45    = 0.70710678
	)
	// Solve for rect.LLX / LLY so the text center (rect center, since
	// HAlignCenter+VAlignMiddle) maps to the page center after a 45°
	// rotation around (rect.LLX, rect.LLY).
	llx := size.Width/2 - (rectW/2)*cos45 + (rectH/2)*sin45
	lly := size.Height/2 - (rectW/2)*sin45 - (rectH/2)*cos45

	return page.AddText(text, pdf.TextStyle{
		Font:     pdf.FontHelveticaBold,
		Size:     fontSize,
		Color:    &pdf.Color{R: 0.85, G: 0.85, B: 0.85, A: 0.4},
		Rotation: 45,
		HAlign:   pdf.HAlignCenter,
		VAlign:   pdf.VAlignMiddle,
		Behind:   true,
	}, pdf.Rectangle{LLX: llx, LLY: lly, URX: llx + rectW, URY: lly + rectH})
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func mustText(err error) {
	if err != nil {
		log.Fatalf("add text: %v", err)
	}
}

func mustAnnot(err error) {
	if err != nil {
		log.Fatalf("add annotation: %v", err)
	}
}

func mustAddOutline(err error) {
	if err != nil {
		log.Fatalf("add outline: %v", err)
	}
}
