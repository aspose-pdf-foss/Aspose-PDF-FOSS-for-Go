// Builds a single-page A4 PDF with a title, a paragraph of body text,
// and an AcroForm containing one field of each supported type.
//
// Output: result_files/acroform_build.pdf
package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		log.Fatalf("page: %v", err)
	}

	// Title.
	titleStyle := pdf.TextStyle{
		Font:   pdf.FontHelveticaBold,
		Size:   24,
		HAlign: pdf.HAlignCenter,
	}
	if err := page.AddText("Subscription Application",
		titleStyle,
		pdf.Rectangle{LLX: 50, LLY: 770, URX: 545, URY: 810}); err != nil {
		log.Fatalf("title: %v", err)
	}

	// Body paragraph below the title.
	bodyStyle := pdf.TextStyle{
		Font:        pdf.FontHelvetica,
		Size:        11,
		LineSpacing: 1.4,
	}
	const intro = "Please complete the form below. Your information will be " +
		"used solely for processing your subscription. Required fields " +
		"are marked with an asterisk (*)."
	if err := page.AddText(intro, bodyStyle,
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 760}); err != nil {
		log.Fatalf("body: %v", err)
	}

	// Section labels next to each field.
	labelStyle := pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 11}
	smallStyle := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}

	// ---- Form fields ----

	form := doc.Form()

	// Text input: full name.
	page.AddText("Full name *", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 655, URX: 200, URY: 670})
	name, err := form.AddTextField(1,
		pdf.Rectangle{LLX: 200, LLY: 650, URX: 545, URY: 675}, "name")
	if err != nil {
		log.Fatalf("name field: %v", err)
	}
	name.SetMaxLen(80)
	name.SetRequired(true)

	// Text input: email.
	page.AddText("Email *", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 615, URX: 200, URY: 630})
	email, err := form.AddTextField(1,
		pdf.Rectangle{LLX: 200, LLY: 610, URX: 545, URY: 635}, "email")
	if err != nil {
		log.Fatalf("email field: %v", err)
	}
	email.SetMaxLen(120)
	email.SetRequired(true)

	// Text input: notes (multi-line).
	page.AddText("Notes", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 575, URX: 200, URY: 590})
	notes, err := form.AddTextField(1,
		pdf.Rectangle{LLX: 200, LLY: 540, URX: 545, URY: 595}, "notes")
	if err != nil {
		log.Fatalf("notes field: %v", err)
	}
	notes.SetMultiline(true)

	// Checkbox: subscribe to newsletter.
	page.AddText("Subscribe to newsletter", smallStyle,
		pdf.Rectangle{LLX: 80, LLY: 510, URX: 300, URY: 525})
	subscribe, err := form.AddCheckbox(1,
		pdf.Rectangle{LLX: 50, LLY: 510, URX: 70, URY: 525}, "subscribe")
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	subscribe.SetChecked(true)

	// Radio group: plan.
	page.AddText("Plan", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 480, URX: 200, URY: 495})
	plan, err := form.AddRadioGroup("plan", []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 200, LLY: 480, URX: 215, URY: 495}, Export: "basic"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 290, LLY: 480, URX: 305, URY: 495}, Export: "premium"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 395, LLY: 480, URX: 410, URY: 495}, Export: "enterprise"},
	})
	if err != nil {
		log.Fatalf("plan: %v", err)
	}
	page.AddText("Basic", smallStyle, pdf.Rectangle{LLX: 220, LLY: 480, URX: 280, URY: 495})
	page.AddText("Premium", smallStyle, pdf.Rectangle{LLX: 310, LLY: 480, URX: 380, URY: 495})
	page.AddText("Enterprise", smallStyle, pdf.Rectangle{LLX: 415, LLY: 480, URX: 530, URY: 495})
	plan.Options()[0].SetSelected(true)

	// Combo box: country.
	page.AddText("Country", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 440, URX: 200, URY: 455})
	country, err := form.AddComboBox(1,
		pdf.Rectangle{LLX: 200, LLY: 435, URX: 545, URY: 460}, "country",
		[]pdf.ChoiceOption{
			{Value: "United States", Export: "US"},
			{Value: "Canada", Export: "CA"},
			{Value: "United Kingdom", Export: "GB"},
			{Value: "Germany", Export: "DE"},
			{Value: "Japan", Export: "JP"},
		})
	if err != nil {
		log.Fatalf("country: %v", err)
	}
	if err := country.SetSelected(0); err != nil {
		log.Fatalf("country select: %v", err)
	}

	// List box: interests (multi-select).
	page.AddText("Interests", labelStyle,
		pdf.Rectangle{LLX: 50, LLY: 380, URX: 200, URY: 395})
	interests, err := form.AddListBox(1,
		pdf.Rectangle{LLX: 200, LLY: 320, URX: 545, URY: 405}, "interests",
		[]pdf.ChoiceOption{
			{Value: "Product updates"},
			{Value: "Engineering blog"},
			{Value: "Webinars"},
			{Value: "Beta program"},
		})
	if err != nil {
		log.Fatalf("interests: %v", err)
	}
	interests.SetMultiSelect(true)
	if err := interests.SetSelected(0, 2); err != nil {
		log.Fatalf("interests select: %v", err)
	}

	// Push button: submit (action stub — wired only on the viewer side).
	if _, err := form.AddPushButton(1,
		pdf.Rectangle{LLX: 200, LLY: 270, URX: 320, URY: 300}, "submit",
		"Submit"); err != nil {
		log.Fatalf("submit: %v", err)
	}

	const out = "result_files/acroform_build.pdf"
	if err := doc.Save(out); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Printf("saved %s — %d form fields", out, len(form.Fields()))
}
