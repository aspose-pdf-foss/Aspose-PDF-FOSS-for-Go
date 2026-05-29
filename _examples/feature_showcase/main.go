// Feature Showcase — a small "annual report"-style document that
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
//   - AES-256 encryption (PDF 2.0) with an empty user password so any
//     viewer opens without a prompt; the owner password still protects
//     modify-document and other permission-gated operations.
//
// Output: docs/feature_showcase.pdf — committed to the repository and linked
// from README.md. Regenerate after meaningful example changes, but avoid
// re-committing on every minor tweak to keep git history lean.
//
// Run from the repo root: `go run ./_examples/feature_showcase`
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

const (
	outputPath  = "docs/feature_showcase.pdf"
	productName = "Aspose.PDF FOSS for Go"
	docTitle    = productName + " — Feature Showcase"
	docAuthor   = "Aspose"
	docVersion  = "v0.2.0"
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
	addRedactionDemo(doc, redactPage)
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
		{destText, "Text Capabilities Showcase", "text", textPage},
		{destImage, "Image Embedding", "image", imagePage},
		{destForm, "AcroForm Fields", "form", formPage},
		{destAnnot, "Annotation Gallery", "annotations", annotPage},
		{destRedact, "Redactions", "redaction", redactPage},
		{destBill, "Restaurant Bill", "bill", billPage},
		{destSales, "Multi-Page Sales Report", "sales", salesPage},
		{destLandscape, "Annual Sales — 12 Month Trend", "landscape", landscapePage},
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

	// Note: ApplyRedactions is called inside addRedactionDemo so it can
	// run on a SUBSET of that page's /Redact annotations — the demo
	// preserves two annots in mark-mode by adding them after the call.

	// --- Per-page furniture: footer, logo stamp, watermark ----------
	// Pre-load the SVG logo once for stamping. Done after content so
	// stampAsposeLogoOnEveryPage sees the final page set.
	if err := stampAsposeLogoOnEveryPage(doc, coverPage, tocPage); err != nil {
		log.Fatalf("svg logo stamp: %v", err)
	}
	// Watermark — applied only on the Text Capabilities page, where it
	// doubles as an example of AddText's `Behind: true` mode (text drawn
	// under the page content) and the AddTextWatermark API surface. Other
	// body pages are watermark-free so their own content reads cleanly.
	if err := addCenteredWatermark(textPage, "WATERMARK"); err != nil {
		log.Fatalf("watermark: %v", err)
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
		Subject:      "End-to-end showcase of " + productName + " capabilities",
		Keywords:     "aspose,pdf,go,golang,acroform,annotations,svg,redaction,encryption",
		Creator:      productName + " " + docVersion,
		Producer:     productName + " " + docVersion,
		CreationDate: now,
		ModDate:      now,
		Custom: map[string]string{
			"AsposeProduct": productName,
		},
	})

	// --- Encryption — AES-256 (PDF 2.0) -----------------------------
	// Empty user password lets any viewer open the file without a prompt,
	// while the owner password still gates modify/extract operations under
	// the configured Permissions. This is the same pattern used by many
	// commercial PDFs that are "encrypted but freely readable".
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "",
		OwnerPassword: "owner",
		Permissions: &pdf.Permissions{
			AllowPrint:         true,
			AllowPrintHighRes:  true,
			AllowCopy:          true,
			AllowAccessibility: true,
			AllowFormFill:      true, // fields stay editable for any viewer
			AllowAnnotations:   true, // sticky-notes / markup stay editable
		},
		Algorithm: pdf.EncryptionAlgAES256,
	})

	if err := doc.Save(outputPath); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Printf("wrote %s", outputPath)
}

func mustAddPage(err error) {
	if err != nil {
		log.Fatalf("add page: %v", err)
	}
}

// sectionHeader paints a section's title (and optional subtitle) in the
// shared body-page style — 26pt navy Helvetica-Bold, centred near the top
// of the page, with an italic grey subtitle below it. Used by every body
// section so headings stay typographically consistent.
func sectionHeader(page *pdf.Page, title, subtitle string) {
	size, _ := page.Size()
	mustText(page.AddText(title, pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   26,
		Color:  &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: size.Height - 90, URX: size.Width - 50, URY: size.Height - 55}))
	if subtitle != "" {
		mustText(page.AddText(subtitle, pdf.TextStyle{
			Font:   pdf.FontHelveticaOblique,
			Size:   11,
			Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
			HAlign: pdf.HAlignCenter,
		}, pdf.Rectangle{LLX: 50, LLY: size.Height - 113, URX: size.Width - 50, URY: size.Height - 98}))
	}
}

// ---------------------------------------------------------------------
// Page 1 — text
// ---------------------------------------------------------------------

func addPageText(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	sectionHeader(page,
		"Text Capabilities Showcase",
		"Standard 14 fonts  •  embedded TTF & Unicode  •  decorations  •  colors  •  word-wrap")

	// Honest secondary note: what the library can do for text beyond what
	// is rendered visually on this page.
	mustText(page.AddText(
		"Also available  ·  ExtractText / ExtractTextWithLayout (font, color, position, sub/superscript)  ·  AddTextWatermark on selected pages  ·  destructive removal via ApplyRedactions",
		pdf.TextStyle{
			Font:        pdf.FontHelvetica,
			Size:        9,
			Color:       &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign:      pdf.HAlignCenter,
			LineSpacing: 1.3,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 145, URX: size.Width - 50, URY: size.Height - 120}))

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
	section("Standard 14 PDF Fonts", size.Height-170)

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
	y := size.Height - 200
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

	sectionHeader(page,
		"Image Embedding",
		"JPEG / PNG raster images placed with pixel-precise Page.AddImage rectangles")

	// Honest secondary note: what the library can also do with images.
	mustText(page.AddText(
		"Also available  ·  ExtractImages (format, colour space, position)  ·  ImageInfo metadata-only inspection  ·  in-place Replace / Remove  ·  ImageToDocument  ·  OptimizeImages",
		pdf.TextStyle{
			Font:        pdf.FontHelvetica,
			Size:        9,
			Color:       &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign:      pdf.HAlignCenter,
			LineSpacing: 1.3,
		},
		pdf.Rectangle{LLX: 40, LLY: size.Height - 150, URX: size.Width - 40, URY: size.Height - 120}))

	// Van Gogh's "The Starry Night" (1889) — public domain. The source file
	// is 1280×1014; we scale to ~60% of page width preserving aspect ratio.
	const (
		srcW = 1280.0
		srcH = 1014.0
	)
	imgW := size.Width * 0.6
	imgH := imgW * srcH / srcW
	x := (size.Width - imgW) / 2
	y := (size.Height - imgH) / 2
	if err := page.AddImage("testdata/starry-night.jpg", pdf.Rectangle{
		LLX: x, LLY: y, URX: x + imgW, URY: y + imgH,
	}); err != nil {
		log.Fatalf("add image: %v", err)
	}

	// Caption below the painting — tasteful attribution for the artwork.
	mustText(page.AddText("Vincent van Gogh, The Starry Night (1889) — public domain",
		pdf.TextStyle{
			Font:   pdf.FontHelveticaOblique,
			Size:   10,
			Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.45, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: y - 22, URX: size.Width - 50, URY: y - 6}))
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

	sectionHeader(page,
		"AcroForm Fields",
		"Text  •  checkbox  •  radio group  •  combo box  •  list box  •  styled via Field.SetStyle")

	// Honest secondary note: what the library can also do for forms.
	size, _ := page.Size()
	mustText(page.AddText(
		"Fields below are styled with Field.SetStyle (border, background, text colour, font, alignment)  ·  Also available  ·  read / write any value  ·  Required & ReadOnly flags  ·  MaxLen, Multiline, Password (text)  ·  MultiSelect (list)  ·  Add / Remove options  ·  RemoveField",
		pdf.TextStyle{
			Font:        pdf.FontHelvetica,
			Size:        9,
			Color:       &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign:      pdf.HAlignCenter,
			LineSpacing: 1.3,
		},
		pdf.Rectangle{LLX: 30, LLY: size.Height - 145, URX: size.Width - 30, URY: size.Height - 115}))

	// Row 1: text field. All rows shifted ~20pt down from the previous
	// layout so the "Also available" note above has 2 lines of breathing room.
	// Widget chrome (border, fill, dropdown chevron, selection highlight)
	// now ships in each field's pre-generated /AP/N appearance stream, so
	// no extra content-stream decorations are needed here.
	addLabel("Full name:", 670)
	tfRect := pdf.Rectangle{LLX: 200, LLY: 670, URX: 450, URY: 690}
	tb, err := form.AddTextField(pageNum, tfRect, "FullName")
	if err != nil {
		log.Fatalf("text field: %v", err)
	}
	tb.SetValue("Alice Sample")

	// Brand palette for field styling (Field.SetStyle → /MK, /BS, /DA, /Q).
	navy := &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1}
	tint := &pdf.Color{R: 0.95, G: 0.96, B: 1.0, A: 1}
	green := &pdf.Color{R: 0.10, G: 0.55, B: 0.25, A: 1}

	// Style the text field: navy border, faint tint fill, navy text.
	mustErr(tb.SetStyle(pdf.FieldStyle{
		BorderColor:     navy,
		BackgroundColor: tint,
		TextColor:       navy,
		BorderWidth:     1,
		TextFont:        pdf.FontHelvetica,
		TextSize:        12,
	}))

	// Row 2: checkbox — navy box, green check.
	addLabel("Subscribe:", 630)
	cbRect := pdf.Rectangle{LLX: 200, LLY: 630, URX: 218, URY: 648}
	cb, err := form.AddCheckbox(pageNum, cbRect, "Subscribe")
	if err != nil {
		log.Fatalf("checkbox: %v", err)
	}
	mustErr(cb.SetStyle(pdf.FieldStyle{BorderColor: navy, BorderWidth: 1, TextColor: green}))
	cb.SetValue("Yes")

	// Row 3: radio group (3 options arranged horizontally).
	addLabel("Plan:", 590)
	rbRects := []pdf.Rectangle{
		{LLX: 200, LLY: 590, URX: 218, URY: 608},
		{LLX: 290, LLY: 590, URX: 308, URY: 608},
		{LLX: 380, LLY: 590, URX: 398, URY: 608},
	}
	rb, err := form.AddRadioGroup("Plan", []pdf.RadioItem{
		{PageNum: pageNum, Rect: rbRects[0], Export: "Basic"},
		{PageNum: pageNum, Rect: rbRects[1], Export: "Pro"},
		{PageNum: pageNum, Rect: rbRects[2], Export: "Enterprise"},
	})
	if err != nil {
		log.Fatalf("radio group: %v", err)
	}
	mustErr(rb.SetStyle(pdf.FieldStyle{BorderColor: navy, BorderWidth: 1, TextColor: navy}))
	rb.SetValue("Pro")

	// Inline labels for the radio options.
	radioLabel := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}
	mustText(page.AddText("Basic", radioLabel, pdf.Rectangle{LLX: 222, LLY: 592, URX: 280, URY: 608}))
	mustText(page.AddText("Pro", radioLabel, pdf.Rectangle{LLX: 312, LLY: 592, URX: 370, URY: 608}))
	mustText(page.AddText("Enterprise", radioLabel, pdf.Rectangle{LLX: 402, LLY: 592, URX: 480, URY: 608}))

	// Row 4: combo box. The widget's /AP/N draws its own chevron.
	addLabel("Country:", 550)
	cbxRect := pdf.Rectangle{LLX: 200, LLY: 550, URX: 350, URY: 570}
	combo, err := form.AddComboBox(pageNum, cbxRect, "Country",
		[]pdf.ChoiceOption{
			{Value: "United States", Export: "US"},
			{Value: "United Kingdom", Export: "UK"},
			{Value: "Germany", Export: "DE"},
			{Value: "Japan", Export: "JP"},
		})
	if err != nil {
		log.Fatalf("combo box: %v", err)
	}
	mustErr(combo.SetStyle(pdf.FieldStyle{BorderColor: navy, BackgroundColor: tint, TextColor: navy, BorderWidth: 1}))
	combo.SetValue("United States")

	// Row 5: list box.
	addLabel("Interests:", 490)
	lbRect := pdf.Rectangle{LLX: 200, LLY: 410, URX: 350, URY: 510}
	lb, err := form.AddListBox(pageNum, lbRect, "Interests",
		[]pdf.ChoiceOption{
			{Value: "PDF Engineering", Export: "pdf"},
			{Value: "Cryptography", Export: "crypto"},
			{Value: "Typography", Export: "type"},
			{Value: "Color Science", Export: "color"},
		})
	if err != nil {
		log.Fatalf("list box: %v", err)
	}
	mustErr(lb.SetStyle(pdf.FieldStyle{BorderColor: navy, BackgroundColor: tint, TextColor: navy, BorderWidth: 1}))
	lb.SetMultiSelect(true)
	lb.SetValue("PDF Engineering")
}

// ---------------------------------------------------------------------
// Page 4 — every supported annotation
// ---------------------------------------------------------------------

func addAnnotations(page *pdf.Page) {
	sectionHeader(page,
		"Annotation Gallery",
		"13 of 15 supported types  ·  Redact has its own page; Widget is shown via AcroForm")

	// Honest secondary note: what the library can also do with annotations.
	size, _ := page.Size()
	mustText(page.AddText(
		"Also available  ·  read existing annotations via Annotations().All() / At(i)  ·  type-asserted setters (SetColor / SetContents / SetRect / per-type props)  ·  Delete / DeleteAt  ·  /AP auto-regenerated on every setter  ·  round-trip safe under AES encryption",
		pdf.TextStyle{
			Font:        pdf.FontHelvetica,
			Size:        9,
			Color:       &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign:      pdf.HAlignCenter,
			LineSpacing: 1.3,
		},
		pdf.Rectangle{LLX: 30, LLY: size.Height - 148, URX: size.Width - 30, URY: size.Height - 115}))

	annots := page.Annotations()

	// Gallery laid out as a 2-column grid of labelled cards: each card
	// has the annotation name at the top and a small rendering of the
	// annotation in the body. 7 rows × 2 cols = 14 slots (last slot blank).
	const (
		cardW   = 235.0
		cardH   = 75.0
		labelH  = 14.0
		topY    = 685.0 // top of the first row, below the section header + note
		gapX    = 25.0
		leftX   = 50.0
	)
	rightX := leftX + cardW + gapX

	labelStyle := pdf.TextStyle{
		Font:  pdf.FontHelveticaBold,
		Size:  11,
		Color: &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1},
	}
	captionStyle := pdf.TextStyle{
		Font:  pdf.FontHelveticaOblique,
		Size:  8,
		Color: &pdf.Color{R: 0.55, G: 0.55, B: 0.6, A: 1},
	}

	// renderMarkup is a helper for the four highlight/underline/squiggly/
	// strikeout markup annots. The annotation itself relies on the viewer
	// regenerating /AP from /Subtype + /QuadPoints + /C (which Acrobat
	// does, but MuPDF/PyMuPDF and some others don't), so we ALSO draw the
	// markup decoration manually in the content stream — the page shows
	// the right thing in any viewer. `decorate` paints over the sample
	// text rect with the visual the spec describes.
	renderMarkup := func(body pdf.Rectangle, sample string, color *pdf.Color, mk func(rect pdf.Rectangle) pdf.Annotation, decorate func(rect pdf.Rectangle)) {
		yMid := (body.LLY+body.URY)/2 - 6
		textRect := pdf.Rectangle{LLX: body.LLX + 4, LLY: yMid, URX: body.URX - 4, URY: yMid + 16}
		// Decoration is drawn FIRST so the text renders on top (matters for
		// the yellow highlight box especially — text needs to stay readable).
		decorate(textRect)
		mustText(page.AddText(sample, pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 12}, textRect))
		_ = color
		mustAnnot(annots.Add(mk(textRect)))
	}

	// Each cell: a renderer that draws into its `body` rect.
	type cell struct {
		name    string
		caption string // small italic line under the label, optional
		render  func(body pdf.Rectangle)
	}

	cells := []cell{
		// --- Markup annotations ---
		{"Highlight", "yellow background over text", func(body pdf.Rectangle) {
			yellow := &pdf.Color{R: 1, G: 1, B: 0, A: 1}
			renderMarkup(body, "Highlight this phrase", yellow,
				func(r pdf.Rectangle) pdf.Annotation {
					a := pdf.NewHighlightAnnotation(page, r)
					a.SetColor(yellow)
					a.SetContents("Yellow highlight")
					return a
				},
				func(r pdf.Rectangle) {
					mustVector(page.DrawRectangle(r, pdf.ShapeStyle{FillColor: yellow}))
				})
		}},
		{"Underline", "single line under the text", func(body pdf.Rectangle) {
			blue := &pdf.Color{R: 0, G: 0, B: 1, A: 1}
			renderMarkup(body, "Underline this phrase", blue,
				func(r pdf.Rectangle) pdf.Annotation {
					a := pdf.NewUnderlineAnnotation(page, r)
					a.SetColor(blue)
					return a
				},
				func(r pdf.Rectangle) {
					mustVector(page.DrawLine(
						pdf.Point{X: r.LLX, Y: r.LLY + 1},
						pdf.Point{X: r.URX, Y: r.LLY + 1},
						pdf.LineStyle{Color: blue, Width: 0.8},
					))
				})
		}},
		{"Squiggly", "wavy underline for proofreaders", func(body pdf.Rectangle) {
			orange := &pdf.Color{R: 1, G: 0.5, B: 0, A: 1}
			renderMarkup(body, "Squiggle this phrase", orange,
				func(r pdf.Rectangle) pdf.Annotation {
					a := pdf.NewSquigglyAnnotation(page, r)
					a.SetColor(orange)
					return a
				},
				func(r pdf.Rectangle) {
					// Wavy line below the text — short zig-zag segments.
					y := r.LLY + 1
					var points []pdf.Point
					for x := r.LLX; x <= r.URX; x += 2 {
						dy := 1.0
						if int((x-r.LLX)/2)%2 == 1 {
							dy = -1
						}
						points = append(points, pdf.Point{X: x, Y: y + dy})
					}
					mustVector(page.DrawPolyline(points, pdf.LineStyle{
						Color: orange, Width: 0.7, Cap: pdf.LineCapRound, Join: pdf.LineJoinRound,
					}))
				})
		}},
		{"StrikeOut", "line through the text", func(body pdf.Rectangle) {
			red := &pdf.Color{R: 1, G: 0, B: 0, A: 1}
			renderMarkup(body, "Strike this phrase out", red,
				func(r pdf.Rectangle) pdf.Annotation {
					a := pdf.NewStrikeOutAnnotation(page, r)
					a.SetColor(red)
					return a
				},
				func(r pdf.Rectangle) {
					midY := (r.LLY + r.URY) / 2
					mustVector(page.DrawLine(
						pdf.Point{X: r.LLX, Y: midY},
						pdf.Point{X: r.URX, Y: midY},
						pdf.LineStyle{Color: red, Width: 0.8},
					))
				})
		}},
		// --- Link with URI ---
		{"Link", "clickable URL with GoToURI action", func(body pdf.Rectangle) {
			yMid := (body.LLY+body.URY)/2 - 6
			rect := pdf.Rectangle{LLX: body.LLX + 4, LLY: yMid, URX: body.URX - 4, URY: yMid + 16}
			mustText(page.AddText("Open example.com",
				pdf.TextStyle{
					Font: pdf.FontHelveticaBold, Size: 11,
					Color:     &pdf.Color{R: 0.1, G: 0.3, B: 0.7, A: 1},
					Underline: true,
				}, rect))
			lnk := pdf.NewLinkAnnotation(page, rect)
			lnk.SetAction(pdf.NewGoToURIAction("https://example.com"))
			lnk.SetBorderWidth(0)
			mustAnnot(annots.Add(lnk))
		}},
		// --- Text (sticky-note) ---
		{"Text — sticky note", "click the icon to read the comment", func(body pdf.Rectangle) {
			iconX := body.LLX + 12
			iconY := body.LLY + (body.URY-body.LLY)/2 - 4
			a := pdf.NewTextAnnotation(page, pdf.Point{X: iconX, Y: iconY})
			a.SetIcon(pdf.TextIconNote)
			a.SetTitle("Reviewer")
			a.SetContents("This is a sticky-note annotation.")
			mustAnnot(annots.Add(a))
		}},
		// --- FreeText ---
		{"FreeText", "text drawn directly on the page", func(body pdf.Rectangle) {
			rect := pdf.Rectangle{LLX: body.LLX + 30, LLY: body.LLY + 4, URX: body.URX - 30, URY: body.URY - 4}
			a := pdf.NewFreeTextAnnotation(page, rect, "FreeText sample",
				pdf.TextStyle{
					Font: pdf.FontHelveticaBold, Size: 10,
					Color:      &pdf.Color{R: 0, G: 0, B: 0, A: 1},
					Background: &pdf.Color{R: 1, G: 1, B: 0.8, A: 1},
					HAlign:     pdf.HAlignCenter, VAlign: pdf.VAlignMiddle,
				})
			a.SetBorderWidth(1)
			mustAnnot(annots.Add(a))
		}},
		// --- Square ---
		{"Square", "filled rectangle with border", func(body pdf.Rectangle) {
			rect := centeredRect(body, 80, 35)
			a := pdf.NewSquareAnnotation(page, rect)
			a.SetColor(&pdf.Color{R: 0.8, G: 0, B: 0, A: 1})
			a.SetBorderWidth(2)
			a.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0.5, A: 1})
			mustAnnot(annots.Add(a))
		}},
		// --- Circle ---
		{"Circle", "dashed border, no fill", func(body pdf.Rectangle) {
			rect := centeredRect(body, 80, 35)
			a := pdf.NewCircleAnnotation(page, rect)
			a.SetColor(&pdf.Color{R: 0, G: 0.5, B: 0, A: 1})
			a.SetBorderStyle(pdf.BorderDashed)
			a.SetDashPattern([]float64{4, 2})
			a.SetBorderWidth(2)
			mustAnnot(annots.Add(a))
		}},
		// --- Line ---
		{"Line", "line with start/end arrow endings", func(body pdf.Rectangle) {
			midY := (body.LLY + body.URY) / 2
			a := pdf.NewLineAnnotation(page,
				pdf.Point{X: body.LLX + 25, Y: midY},
				pdf.Point{X: body.URX - 25, Y: midY},
			)
			a.SetColor(&pdf.Color{R: 0, G: 0, B: 0.7, A: 1})
			a.SetBorderWidth(2)
			a.SetStartLineEnding(pdf.LineEndingOpenArrow)
			a.SetEndLineEnding(pdf.LineEndingClosedArrow)
			mustAnnot(annots.Add(a))
		}},
		// --- Ink ---
		{"Ink", "free-hand pen strokes (Catmull-Rom smoothed)", func(body pdf.Rectangle) {
			midY := (body.LLY + body.URY) / 2
			width := body.URX - body.LLX - 30
			step := width / 6
			x0 := body.LLX + 15
			a := pdf.NewInkAnnotation(page, [][]pdf.Point{{
				{X: x0 + 0*step, Y: midY - 8},
				{X: x0 + 1*step, Y: midY + 6},
				{X: x0 + 2*step, Y: midY - 4},
				{X: x0 + 3*step, Y: midY + 10},
				{X: x0 + 4*step, Y: midY - 2},
				{X: x0 + 5*step, Y: midY + 8},
				{X: x0 + 6*step, Y: midY - 6},
			}})
			a.SetColor(&pdf.Color{R: 0.6, G: 0, B: 0.6, A: 1})
			a.SetBorderWidth(2)
			mustAnnot(annots.Add(a))
		}},
		// --- Stamp ---
		{"Stamp", "predefined or custom-image stamp", func(body pdf.Rectangle) {
			rect := centeredRect(body, 110, 35)
			a := pdf.NewStampAnnotation(page, rect, pdf.StampNameApproved)
			mustAnnot(annots.Add(a))
		}},
		// --- FileAttachment ---
		{"FileAttachment", "embedded file behind a paperclip icon", func(body pdf.Rectangle) {
			iconX := body.LLX + 12
			iconY := body.LLY + (body.URY-body.LLY)/2 - 4
			a := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: iconX, Y: iconY})
			a.SetIcon(pdf.FileAttachmentIconPaperclip)
			a.SetTitle("Reviewer")
			a.SetContents("Quarterly report — see attachment")
			if err := a.SetFileFromStream(
				strings.NewReader("Confidential report contents (demonstration only)."),
				"q3-report.txt"); err != nil {
				log.Fatalf("attach file: %v", err)
			}
			a.SetFileDescription("Q3 financial summary")
			mustAnnot(annots.Add(a))
		}},
	}

	// Lay out as 2-column grid filled column-by-column (left column first
	// gets cells 0..6, right column gets 7..12).
	const rowsPerCol = 7
	for i, c := range cells {
		colIdx := i / rowsPerCol
		rowIdx := i % rowsPerCol
		cardX := leftX
		if colIdx == 1 {
			cardX = rightX
		}
		cardTop := topY - float64(rowIdx)*cardH
		cardBot := cardTop - cardH

		// Thin separator line between rows (skip above the first row).
		if rowIdx > 0 {
			mustVector(page.DrawLine(
				pdf.Point{X: cardX + 4, Y: cardTop - 1},
				pdf.Point{X: cardX + cardW - 4, Y: cardTop - 1},
				pdf.LineStyle{Color: &pdf.Color{R: 0.9, G: 0.9, B: 0.93, A: 1}, Width: 0.5},
			))
		}

		// Name label.
		mustText(page.AddText(c.name, labelStyle, pdf.Rectangle{
			LLX: cardX, LLY: cardTop - labelH, URX: cardX + cardW, URY: cardTop - 2,
		}))

		// Caption — short italic note under the label.
		if c.caption != "" {
			mustText(page.AddText(c.caption, captionStyle, pdf.Rectangle{
				LLX: cardX, LLY: cardTop - labelH - 11, URX: cardX + cardW, URY: cardTop - labelH - 1,
			}))
		}

		// Body — where the annotation renders.
		body := pdf.Rectangle{
			LLX: cardX + 4,
			LLY: cardBot + 4,
			URX: cardX + cardW - 4,
			URY: cardTop - labelH - 14,
		}
		c.render(body)
	}
}

// centeredRect returns a rectangle of given width/height centred inside outer.
func centeredRect(outer pdf.Rectangle, w, h float64) pdf.Rectangle {
	cx := (outer.LLX + outer.URX) / 2
	cy := (outer.LLY + outer.URY) / 2
	return pdf.Rectangle{LLX: cx - w/2, LLY: cy - h/2, URX: cx + w/2, URY: cy + h/2}
}

// ---------------------------------------------------------------------
// Page 5 — restaurant bill rendered as a Table
// ---------------------------------------------------------------------

func addRestaurantBill(page *pdf.Page) {
	size, _ := page.Size()

	sectionHeader(page,
		"Restaurant Bill",
		"Trattoria da Marco — single-page Table with ColSpan summary rows")

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

	sectionHeader(page,
		"Multi-Page Sales Report",
		"image header  •  repeating headers  •  ColSpan  •  Row.SetBackground  •  AddRows batch  •  overflow")

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
		SetImage("testdata/sales-banner.jpg").
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

	sectionHeader(page,
		"Vector Graphics",
		"DrawLine  •  Rectangle  •  Circle  •  Ellipse  •  Polyline  •  Polygon  •  Path with Arc")

	// === 2×3 gallery of labeled primitive demos ===
	// Each card shows the API name as a label and a representative figure
	// drawn with that primitive. Cards share a common frame so the page
	// reads as one consistent gallery rather than scattered demos.
	const (
		colCount    = 2
		rowCount    = 3
		gridLeft    = 50.0
		gridRight   = 545.0
		gridTop     = 705.0
		gridBottom  = 105.0
		gapX        = 14.0
		gapY        = 14.0
		labelInset  = 12.0
		labelHeight = 22.0
	)
	cardW := (gridRight - gridLeft - gapX) / float64(colCount)
	cardH := (gridTop - gridBottom - gapY*float64(rowCount-1)) / float64(rowCount)

	cardBG := &pdf.Color{R: 0.985, G: 0.985, B: 0.995, A: 1}
	cardBorder := &pdf.Color{R: 0.83, G: 0.85, B: 0.92, A: 1}
	labelColor := &pdf.Color{R: 0.15, G: 0.20, B: 0.55, A: 1}

	// drawCard paints a card's frame and label, returns the inner drawing
	// rectangle the caller should target for its primitive.
	drawCard := func(col, row int, label string) pdf.Rectangle {
		x := gridLeft + float64(col)*(cardW+gapX)
		y := gridTop - float64(row+1)*cardH - float64(row)*gapY
		outer := pdf.Rectangle{LLX: x, LLY: y, URX: x + cardW, URY: y + cardH}
		mustVector(page.DrawRoundedRectangle(outer, 6, pdf.ShapeStyle{
			FillColor: cardBG,
			LineStyle: pdf.LineStyle{Width: 0.5, Color: cardBorder},
		}))
		mustText(page.AddText(label, pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 11, Color: labelColor,
		}, pdf.Rectangle{
			LLX: outer.LLX + labelInset, LLY: outer.URY - labelHeight - 2,
			URX: outer.URX - labelInset, URY: outer.URY - 4,
		}))
		// Inner rect leaves room for the label at the top and a small
		// breathing margin around the figure.
		return pdf.Rectangle{
			LLX: outer.LLX + labelInset, LLY: outer.LLY + 10,
			URX: outer.URX - labelInset, URY: outer.URY - labelHeight - 6,
		}
	}

	mid := func(r pdf.Rectangle) (float64, float64) {
		return (r.LLX + r.URX) / 2, (r.LLY + r.URY) / 2
	}

	// --- Card 1: DrawLine — three stroke variants -----------------------
	inner := drawCard(0, 0, "DrawLine")
	_, ymid := mid(inner)
	mustVector(page.DrawLine(
		pdf.Point{X: inner.LLX + 8, Y: ymid + 28},
		pdf.Point{X: inner.URX - 8, Y: ymid + 28},
		pdf.LineStyle{Width: 2.5, Color: &pdf.Color{R: 0.20, G: 0.30, B: 0.70, A: 1}, Cap: pdf.LineCapRound},
	))
	mustVector(page.DrawLine(
		pdf.Point{X: inner.LLX + 8, Y: ymid},
		pdf.Point{X: inner.URX - 8, Y: ymid},
		pdf.LineStyle{Width: 2, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1}, DashPattern: []float64{8, 5}},
	))
	mustVector(page.DrawLine(
		pdf.Point{X: inner.LLX + 8, Y: ymid - 28},
		pdf.Point{X: inner.URX - 8, Y: ymid - 28},
		pdf.LineStyle{
			Width: 3, Color: &pdf.Color{R: 0.10, G: 0.50, B: 0.30, A: 1},
			Cap: pdf.LineCapRound, DashPattern: []float64{0.5, 6},
		},
	))

	// --- Card 2: Rectangle + RoundedRectangle ---------------------------
	inner = drawCard(1, 0, "Rectangle  •  Rounded")
	gap := 14.0
	half := (inner.URX - inner.LLX - gap) / 2
	rectA := pdf.Rectangle{LLX: inner.LLX, LLY: inner.LLY + 6, URX: inner.LLX + half, URY: inner.URY - 6}
	rectB := pdf.Rectangle{LLX: rectA.URX + gap, LLY: rectA.LLY, URX: inner.URX, URY: rectA.URY}
	mustVector(page.DrawRectangle(rectA, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.2, Color: &pdf.Color{R: 0.20, G: 0.30, B: 0.70, A: 1}},
		FillColor: &pdf.Color{R: 0.82, G: 0.88, B: 1.00, A: 1},
	}))
	mustVector(page.DrawRoundedRectangle(rectB, 14, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.2, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1}},
		FillColor: &pdf.Color{R: 1.00, G: 0.92, B: 0.78, A: 1},
	}))

	// --- Card 3: Circle + Ellipse ---------------------------------------
	inner = drawCard(0, 1, "Circle  •  Ellipse")
	xmid, ymid := mid(inner)
	cR := 28.0
	mustVector(page.DrawCircle(
		pdf.Point{X: inner.LLX + cR + 12, Y: ymid},
		cR,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1.4, Color: &pdf.Color{R: 0.55, G: 0.25, B: 0.70, A: 1}},
			FillColor: &pdf.Color{R: 0.92, G: 0.85, B: 0.97, A: 0.92},
		},
	))
	_ = xmid
	mustVector(page.DrawEllipse(
		pdf.Point{X: inner.URX - 50, Y: ymid}, 44, 24,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1.4, Color: &pdf.Color{R: 0.10, G: 0.50, B: 0.30, A: 1}},
			FillColor: &pdf.Color{R: 0.85, G: 0.95, B: 0.85, A: 0.92},
		},
	))

	// --- Card 4: Polyline — open zigzag ---------------------------------
	inner = drawCard(1, 1, "Polyline")
	pts := make([]pdf.Point, 0, 9)
	steps := 8
	stride := (inner.URX - inner.LLX - 16) / float64(steps)
	low := inner.LLY + 14
	high := inner.URY - 14
	for i := 0; i <= steps; i++ {
		x := inner.LLX + 8 + float64(i)*stride
		y := low
		if i%2 == 1 {
			y = high
		}
		pts = append(pts, pdf.Point{X: x, Y: y})
	}
	mustVector(page.DrawPolyline(pts, pdf.LineStyle{
		Width: 2.5, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1},
		Cap: pdf.LineCapRound, Join: pdf.LineJoinRound,
	}))

	// --- Card 5: Polygon — five-point star ------------------------------
	inner = drawCard(0, 2, "Polygon")
	xmid, ymid = mid(inner)
	outerR := 36.0
	innerR := 15.0
	star := make([]pdf.Point, 0, 10)
	for i := 0; i < 10; i++ {
		ang := math.Pi/2 - float64(i)*math.Pi/5
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		star = append(star, pdf.Point{X: xmid + r*math.Cos(ang), Y: ymid + r*math.Sin(ang)})
	}
	mustVector(page.DrawPolygon(star, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.3, Color: &pdf.Color{R: 0.78, G: 0.55, B: 0.06, A: 1}, Join: pdf.LineJoinMiter, MiterLimit: 4},
		FillColor: &pdf.Color{R: 1.00, G: 0.84, B: 0.36, A: 1},
	}))

	// --- Card 6: Path with Arc — pie slice + bezier wave ----------------
	inner = drawCard(1, 2, "Path with Arc")
	// Pie slice on the left half.
	pcx := inner.LLX + 38
	pcy := (inner.LLY + inner.URY) / 2
	pieR := 34.0
	pie := pdf.NewPath().
		MoveTo(pcx, pcy).
		LineTo(pcx+pieR, pcy).
		Arc(pcx, pcy, pieR, 0, 2.0944). // 120° sweep
		Close()
	mustVector(page.DrawPath(pie, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.3, Color: &pdf.Color{R: 0.20, G: 0.45, B: 0.78, A: 1}},
		FillColor: &pdf.Color{R: 0.78, G: 0.88, B: 1.00, A: 1},
	}))
	// Cubic Bezier wave on the right half.
	wx0 := inner.LLX + 88
	wx1 := inner.URX - 4
	wave := pdf.NewPath().
		MoveTo(wx0, pcy).
		CurveTo(wx0+14, pcy+30, wx0+30, pcy-30, wx0+44, pcy).
		CurveTo(wx0+58, pcy+30, wx1-14, pcy-30, wx1, pcy)
	mustVector(page.DrawPath(wave, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{
			Width: 2.2, Color: &pdf.Color{R: 0.95, G: 0.55, B: 0.05, A: 1},
			Cap: pdf.LineCapRound, Join: pdf.LineJoinRound,
		},
	}))

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

// addCoverPage paints the front cover: a large pinwheel-only mark from the
// brand SVG (the wordmark is omitted — the product name beneath the pinwheel
// is the only place "Aspose" appears), then the product title, then a
// subtitle. Cover stays watermark-free and logo-stamp-free in main().
func addCoverPage(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	// Big pinwheel — square, centred, upper-middle of the page.
	const logoSize = 220.0
	logoX := (size.Width - logoSize) / 2
	logoY := size.Height - 100 - logoSize // 100pt margin from the top
	if svg, err := doc.LoadSVG("testdata/aspose-pinwheel.svg"); err == nil {
		mustErr(page.AddSVGObject(svg, pdf.Rectangle{
			LLX: logoX, LLY: logoY, URX: logoX + logoSize, URY: logoY + logoSize,
		}))
	}

	// Product title — sits below the pinwheel with breathing room.
	titleY := logoY - 80
	mustText(page.AddText(productName, pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   30,
		Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 30, LLY: titleY, URX: size.Width - 30, URY: titleY + 40}))

	// Subtitle.
	mustText(page.AddText("Feature Showcase", pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   18,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.45, A: 1},
		HAlign: pdf.HAlignCenter,
	}, pdf.Rectangle{LLX: 50, LLY: titleY - 38, URX: size.Width - 50, URY: titleY - 8}))

	// One-liner below the subtitle.
	mustText(page.AddText("An end-to-end tour of every library capability in one document.",
		pdf.TextStyle{
			Font:   pdf.FontHelvetica,
			Size:   12,
			Color:  &pdf.Color{R: 0.5, G: 0.5, B: 0.55, A: 1},
			HAlign: pdf.HAlignCenter,
		}, pdf.Rectangle{LLX: 50, LLY: titleY - 70, URX: size.Width - 50, URY: titleY - 48}))

	// CTA links — one row near the bottom of the page, two clickable groups
	// separated by a centred bold bullet:
	//   [github mark] Source code   •   [Go logo] API reference
	// The bullet sits exactly at the horizontal centre of the page and the
	// rest of the row is laid out symmetrically around it.
	const (
		iconH     = 12.0          // shared icon height; widths follow each viewBox aspect
		githubAR  = 1.0           // GitHub mark viewBox 16×16
		goAR      = 207.0 / 78.0  // Go logo viewBox 207×78
		textGap   = 5.0           // gap between an icon and its label
		bulletGap = 12.0          // gap between a label and the centre bullet
		linkY     = 110.0         // y-baseline of the row (well above the footer at y=40)
		srcW      = 74.0          // approx width of "Source code" at Helvetica-Bold 12pt
		apiW      = 88.0          // approx width of "API reference"
		bulletHW  = 5.0           // half-width of the bullet glyph
	)
	center := size.Width / 2
	linkStyle := pdf.TextStyle{
		Font:      pdf.FontHelveticaBold,
		Size:      12,
		Color:     &pdf.Color{R: 0.1, G: 0.3, B: 0.7, A: 1},
		Underline: true,
	}
	bulletStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   14,
		Color:  &pdf.Color{R: 0.3, G: 0.3, B: 0.35, A: 1},
		HAlign: pdf.HAlignCenter,
	}
	annots := page.Annotations()

	// --- Left group: GitHub mark + "Source code" ---
	githubW := iconH * githubAR
	srcEnd := center - bulletHW - bulletGap
	srcStart := srcEnd - srcW
	ghEnd := srcStart - textGap
	ghStart := ghEnd - githubW
	if svg, err := doc.LoadSVG("testdata/github-mark.svg"); err == nil {
		mustErr(page.AddSVGObject(svg, pdf.Rectangle{
			LLX: ghStart, LLY: linkY, URX: ghEnd, URY: linkY + iconH,
		}))
	}
	mustText(page.AddText("Source code", linkStyle, pdf.Rectangle{
		LLX: srcStart, LLY: linkY - 1, URX: srcEnd, URY: linkY + iconH,
	}))
	leftLink := pdf.NewLinkAnnotation(page, pdf.Rectangle{
		LLX: ghStart - 2, LLY: linkY - 3, URX: srcEnd + 2, URY: linkY + iconH + 3,
	})
	leftLink.SetAction(pdf.NewGoToURIAction("https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"))
	leftLink.SetBorderWidth(0)
	mustAnnot(annots.Add(leftLink))

	// --- Centre bullet (horizontal page centre) ---
	mustText(page.AddText("•", bulletStyle, pdf.Rectangle{
		LLX: center - bulletHW, LLY: linkY - 2, URX: center + bulletHW, URY: linkY + iconH + 2,
	}))

	// --- Right group: Go logo + "API reference" ---
	goW := iconH * goAR
	goStart := center + bulletHW + bulletGap
	goEnd := goStart + goW
	apiStart := goEnd + textGap
	apiEnd := apiStart + apiW
	if svg, err := doc.LoadSVG("testdata/go-logo.svg"); err == nil {
		mustErr(page.AddSVGObject(svg, pdf.Rectangle{
			LLX: goStart, LLY: linkY, URX: goEnd, URY: linkY + iconH,
		}))
	}
	mustText(page.AddText("API reference", linkStyle, pdf.Rectangle{
		LLX: apiStart, LLY: linkY - 1, URX: apiEnd, URY: linkY + iconH,
	}))
	rightLink := pdf.NewLinkAnnotation(page, pdf.Rectangle{
		LLX: goStart - 2, LLY: linkY - 3, URX: apiEnd + 2, URY: linkY + iconH + 3,
	})
	rightLink.SetAction(pdf.NewGoToURIAction("https://pkg.go.dev/github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"))
	rightLink.SetBorderWidth(0)
	mustAnnot(annots.Add(rightLink))
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

		// Each section's page label matches its actual /PageLabels assignment
		// (set in main: cover/TOC use roman, body restarts at decimal 1). So
		// the label of body page P is P-2. This stays correct when the sales
		// report inserts continuation pages between sections — the entries
		// after sales skip a number, which is what the viewer also shows.
		label := strconv.Itoa(s.page.Number() - 2)

		// Item number — sequential 1-based index, decoupled from the
		// destination page label so continuation pages don't create gaps.
		mustText(page.AddText(fmt.Sprintf("%d.", i+1), pdf.TextStyle{
			Font:   pdf.FontHelveticaBold,
			Size:   13,
			Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
			HAlign: pdf.HAlignRight,
		}, pdf.Rectangle{LLX: 72, LLY: y, URX: 95, URY: y + 20}))

		// Title on the left.
		mustText(page.AddText(s.title, pdf.TextStyle{
			Font:  pdf.FontHelvetica,
			Size:  13,
			Color: &pdf.Color{R: 0.1, G: 0.1, B: 0.15, A: 1},
		}, pdf.Rectangle{LLX: 105, LLY: y, URX: 380, URY: y + 20}))

		// Page number on the right.
		mustText(page.AddText(label, pdf.TextStyle{
			Font:   pdf.FontHelveticaBold,
			Size:   13,
			Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
			HAlign: pdf.HAlignRight,
		}, pdf.Rectangle{LLX: size.Width - 130, LLY: y, URX: size.Width - 100, URY: y + 20}))

		// Leader dots between title and page number. Start past the widest
		// title in this list ("Annual Sales — 12 Month Trend" ≈ x=300) so the
		// dots never overlap title text. URY/LLY centred on title baseline.
		mustText(page.AddText(strings.Repeat(". ", 80), pdf.TextStyle{
			Font:  pdf.FontHelvetica,
			Size:  11,
			Color: &pdf.Color{R: 0.7, G: 0.7, B: 0.75, A: 1},
		}, pdf.Rectangle{LLX: 320, LLY: y + 2, URX: size.Width - 135, URY: y + 18}))

		// Clickable link over the whole row → GoTo the section's page.
		// /Border [0 0 0] suppresses the default 1pt black rectangle Acrobat
		// and other viewers draw around link annotations.
		link := pdf.NewLinkAnnotation(page, pdf.Rectangle{
			LLX: 95, LLY: y, URX: size.Width - 95, URY: y + 22,
		})
		topY, _ := s.page.Size()
		link.SetAction(pdf.NewGoToAction(s.page.Number(), topY.Height))
		link.SetBorderWidth(0)
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

// addRedactionDemo lays out a memo that shows BOTH redaction phases on
// the same page:
//
//   - phase 1 (applied): two rows get /Redact annotations, then
//     Document.ApplyRedactions runs — the underlying glyphs are destroyed
//     and the annotations are removed from /Annots. In the saved PDF these
//     rows show only a solid black rectangle with the "[REDACTED]" overlay;
//     the original value is gone from the content stream.
//
//   - phase 2 (mark-mode): two more rows get /Redact annotations added
//     AFTER the apply call, so they remain as live annotations. The
//     original value text is intact underneath; the annotation draws a
//     semi-transparent yellow tint with a "MARK" overlay so the value
//     reads through. In Acrobat these annots show up in the Comments
//     panel and can be edited or removed by the user; selecting the text
//     under them copies the original value.
//
// Two extra rows have no /Redact at all to act as a control.
func addRedactionDemo(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	sectionHeader(page,
		"Redactions",
		"Mark-mode  vs  applied — both phases side by side on the same page")

	mustText(page.AddText("Document.ApplyRedactions destructively rewrites the content stream — glyphs inside every targeted /Redact annotation are gone for good. Annotations added after the call stay in mark-mode: the value reads through and is still copy-selectable.",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10.5, Color: &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1}, LineSpacing: 1.4},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 180, URX: size.Width - 50, URY: size.Height - 130}))

	// Header line.
	mustText(page.AddText("Internal memo — Q3 personnel changes",
		pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}},
		pdf.Rectangle{LLX: 60, LLY: size.Height - 220, URX: size.Width - 60, URY: size.Height - 200}))

	// Each row: label on the left, value on the right, plus a phase tag on
	// the far right so the reader can connect each rect to the right phase.
	const (
		valueLLX = 220.0
		valueURX = 460.0
	)
	labelStyle := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 12, Color: &pdf.Color{R: 0.3, G: 0.3, B: 0.3, A: 1}}
	valueStyle := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 12, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}}

	type phase int
	const (
		phaseNone phase = iota // no redact at all (control)
		phaseApply             // redact + ApplyRedactions; content destroyed
		phaseMark              // redact only, no apply; content intact under tint
	)
	rows := []struct {
		label, value string
		y            float64
		ph           phase
	}{
		{"Employee:", "Maria Castellano (ID 47821)", 580, phaseNone},
		{"Retention bonus:", "$185,000.00", 550, phaseApply},
		{"Direct phone:", "+1 (415) 555-0182", 520, phaseApply},
		{"Bank routing:", "026009593", 490, phaseMark},
		{"Account number:", "4421-9087-7733-2104", 460, phaseMark},
		{"Effective date:", "2026-04-15", 430, phaseNone},
	}
	tagStyle := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 8, HAlign: pdf.HAlignLeft}
	for _, r := range rows {
		mustText(page.AddText(r.label, labelStyle, pdf.Rectangle{
			LLX: 60, LLY: r.y, URX: 215, URY: r.y + 16,
		}))
		mustText(page.AddText(r.value, valueStyle, pdf.Rectangle{
			LLX: valueLLX, LLY: r.y, URX: valueURX, URY: r.y + 16,
		}))
		// Right-side phase tag.
		var tagText string
		var tagColor *pdf.Color
		switch r.ph {
		case phaseApply:
			tagText = "applied"
			tagColor = &pdf.Color{R: 0.7, G: 0.1, B: 0.1, A: 1}
		case phaseMark:
			tagText = "mark-mode"
			tagColor = &pdf.Color{R: 0.5, G: 0.4, B: 0.0, A: 1}
		}
		if tagText != "" {
			style := tagStyle
			style.Color = tagColor
			mustText(page.AddText(tagText, style, pdf.Rectangle{
				LLX: 475, LLY: r.y + 1, URX: size.Width - 50, URY: r.y + 15,
			}))
		}
	}

	annots := page.Annotations()
	overlayApplied := pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 9,
		Color:  &pdf.Color{R: 1, G: 1, B: 1, A: 1},
		HAlign: pdf.HAlignCenter, VAlign: pdf.VAlignMiddle,
	}
	overlayMark := pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 9,
		Color:  &pdf.Color{R: 0.4, G: 0.3, B: 0, A: 1},
		HAlign: pdf.HAlignCenter, VAlign: pdf.VAlignMiddle,
	}
	rectFor := func(y float64) pdf.Rectangle {
		return pdf.Rectangle{LLX: valueLLX - 4, LLY: y - 2, URX: valueURX, URY: y + 18}
	}

	// Phase 1: add /Redact annotations for rows tagged phaseApply, then run
	// Document.ApplyRedactions. That rewrites the content stream (destroys
	// the value glyphs inside each quad) and removes those /Redact annots
	// from /Annots — by the time the PDF is saved, those rows hold a plain
	// filled rectangle (no annotation, no recoverable text).
	for _, r := range rows {
		if r.ph != phaseApply {
			continue
		}
		red := pdf.NewRedactAnnotation(page, rectFor(r.y))
		red.SetInteriorColor(&pdf.Color{R: 0, G: 0, B: 0, A: 1})
		red.SetOverlayText("[REDACTED]")
		red.SetOverlayTextStyle(overlayApplied)
		mustAnnot(annots.Add(red))
	}
	if err := doc.ApplyRedactions(); err != nil {
		log.Fatalf("apply redactions: %v", err)
	}

	// Phase 2: add /Redact annotations AFTER the apply call. These stay
	// in mark-mode for ever — the annotation is part of the PDF, the
	// content under it is still alive (just visually covered).
	//
	// We use solid yellow /IC (the library's redact appearance always
	// paints an opaque fill; transparency through /IC is not implemented).
	// Colour is the discriminator: black + "[REDACTED]" = applied (gone),
	// yellow + "MARK" = mark-mode (annotation, value preserved beneath).
	// In Acrobat the user can verify by opening the Comments panel — the
	// mark-mode rows appear there as Redact annotations; the applied rows
	// don't, because their annotations were removed by ApplyRedactions.
	for _, r := range rows {
		if r.ph != phaseMark {
			continue
		}
		red := pdf.NewRedactAnnotation(page, rectFor(r.y))
		red.SetInteriorColor(&pdf.Color{R: 0.98, G: 0.82, B: 0.18, A: 1})
		red.SetOverlayText("MARK — annotation, not applied")
		red.SetOverlayTextStyle(overlayMark)
		mustAnnot(annots.Add(red))
	}

	// Caption below the rows explaining how to verify the difference.
	mustText(page.AddText("Try copying the values: applied rows yield nothing (the glyphs aren't there), mark-mode rows still copy the original text.",
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

	sectionHeader(page,
		"Annual Sales — 12 Month Trend",
		"PageFormatA4.Landscape  •  DrawRectangle (alpha fill)  •  DashPattern grid  •  DrawPolyline trend line  •  AddText labels")

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
	// Round the Y-axis upper bound up to a nice value so gridline labels read
	// €20/€40/€60/€80/€100 instead of €19/€37/€56/€75/€93. Pick the smallest
	// step from a standard set such that step×5 covers maxVal×1.15 headroom.
	target := maxVal * 1.15
	yStep := 1.0
	for _, s := range []float64{1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000} {
		if s*5 >= target {
			yStep = s
			break
		}
	}
	yMax := yStep * 5
	scaleY := (chartTop - chartBottom) / yMax

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
		mustText(page.AddText(fmt.Sprintf("€%.0fk", yStep*float64(i)), pdf.TextStyle{
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
		mustText(page.AddText(fmt.Sprintf("€%.0fk", v), pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 9,
			Color:  &pdf.Color{R: 0.1, G: 0.3, B: 0.6, A: 1},
			HAlign: pdf.HAlignCenter,
		}, pdf.Rectangle{LLX: x - 10, LLY: barTop + 2, URX: x + barWidth + 10, URY: barTop + 14}))
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
