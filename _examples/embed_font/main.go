package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)

	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		log.Fatalf("load font: %v", err)
	}

	page, err := doc.Page(1)
	if err != nil {
		log.Fatalf("page: %v", err)
	}
	size, _ := page.Size()

	title := pdf.TextStyle{
		Font:  font,
		Size:  24,
		Color: &pdf.Color{R: 0.1, G: 0.2, B: 0.5, A: 1},
	}
	body := pdf.TextStyle{
		Font:        font,
		Size:        14,
		LineSpacing: 1.4,
	}
	caption := pdf.TextStyle{
		Font:   pdf.FontHelveticaOblique,
		Size:   10,
		Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
		HAlign: pdf.HAlignCenter,
	}

	margin := 50.0
	rect := func(top, height float64) pdf.Rectangle {
		return pdf.Rectangle{
			LLX: margin,
			LLY: size.Height - top - height,
			URX: size.Width - margin,
			URY: size.Height - top,
		}
	}

	if err := page.AddText("Multilingual PDF with embedded TTF",
		title, rect(margin, 40)); err != nil {
		log.Fatalf("title: %v", err)
	}

	samples := []struct {
		label string
		text  string
	}{
		{"English", "The quick brown fox jumps over the lazy dog."},
		{"Russian", "Съешь ещё этих мягких французских булок, да выпей чаю."},
		{"Greek", "Γειά σου κόσμε! Η γρήγορη καφέ αλεπού."},
		{"German", "Falsches Üben von Xylophonmusik quält jeden größeren Zwerg."},
		{"French", "Portez ce vieux whisky au juge blond qui fume."},
		{"Polish", "Pchnąć w tę łódź jeża lub ośm skrzyń fig."},
	}

	top := margin + 60.0
	for _, s := range samples {
		labelRect := pdf.Rectangle{
			LLX: margin, LLY: size.Height - top - 20,
			URX: margin + 80, URY: size.Height - top,
		}
		_ = page.AddText(s.label+":", pdf.TextStyle{
			Font: font, Size: 12,
			Color: &pdf.Color{R: 0.5, G: 0.1, B: 0.1, A: 1},
		}, labelRect)

		textRect := pdf.Rectangle{
			LLX: margin + 90, LLY: size.Height - top - 20,
			URX: size.Width - margin, URY: size.Height - top,
		}
		if err := page.AddText(s.text, body, textRect); err != nil {
			log.Fatalf("sample %s: %v", s.label, err)
		}
		top += 35
	}

	footerRect := pdf.Rectangle{
		LLX: 0, LLY: 30,
		URX: size.Width, URY: 60,
	}
	_ = page.AddText("Generated with aspose.pdf-for-go-foss — DejaVuSans embedded",
		caption, footerRect)

	if err := doc.Save("result_files/embed_font.pdf"); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Println("saved result_files/embed_font.pdf")
}
