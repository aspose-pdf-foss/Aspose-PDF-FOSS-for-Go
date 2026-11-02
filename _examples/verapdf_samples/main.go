// SPDX-License-Identifier: MIT

// Generates the files tools/validate_verapdf.py has veraPDF check.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

var out = "result_files/verapdf"

func autotag(src, name string) {
	d, err := asposepdf.Open(src)
	if err != nil {
		log.Printf("%s: %v", src, err)
		return
	}
	res, err := d.AutoTag(asposepdf.DefaultAutoTaggingSettings())
	if err != nil {
		log.Printf("%s: %v", src, err)
		return
	}
	if _, err := d.ConvertToPDFUA(); err != nil {
		log.Fatal(err)
	}
	if err := d.Save(filepath.Join(out, "ua_"+name+".pdf")); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ua_%s: %+v\n", name, *res)
}

func pdfa(src, name string, f asposepdf.PDFAFormat, label string, tag bool) {
	d, err := asposepdf.Open(src)
	if err != nil {
		log.Printf("%s: %v", src, err)
		return
	}
	if tag {
		if _, err := d.AutoTag(asposepdf.DefaultAutoTaggingSettings()); err != nil {
			log.Printf("%s: %v", src, err)
			return
		}
	}
	rep, err := d.ConvertToPDFA(f)
	if err != nil {
		log.Printf("%s: %v", src, err)
		return
	}
	if tag {
		if _, err := d.ConvertToPDFUA(); err != nil {
			log.Fatal(err)
		}
	}
	if err := d.Save(filepath.Join(out, "a"+label+"_"+name+".pdf")); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("a%s_%s: conformant=%v issues=%d\n", label, name, rep.Conformant, len(rep.Issues))
}

// annotated builds an untagged document with a link, a form field, a stamp and a note.
func annotated() *asposepdf.Document {
	d := asposepdf.NewDocument(612, 792)
	d.SetInfo(asposepdf.DocumentInfo{Title: "Annotated sample"})
	p, _ := d.Page(1)
	style := asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12}
	if err := p.AddText("Read the manual before you start working.", style, asposepdf.Rectangle{LLX: 72, LLY: 680, URX: 540, URY: 720}); err != nil {
		log.Fatal(err)
	}
	if err := p.AddText("Second paragraph far below the first one.", style, asposepdf.Rectangle{LLX: 72, LLY: 300, URX: 540, URY: 340}); err != nil {
		log.Fatal(err)
	}
	link := asposepdf.NewLinkAnnotation(p, asposepdf.Rectangle{LLX: 72, LLY: 700, URX: 200, URY: 720})
	link.SetAction(asposepdf.NewGoToURIAction("https://example.com/manual"))
	stamp := asposepdf.NewStampAnnotation(p, asposepdf.Rectangle{LLX: 72, LLY: 400, URX: 200, URY: 440}, asposepdf.StampNameDraft)
	note := asposepdf.NewHighlightAnnotation(p, asposepdf.Rectangle{LLX: 72, LLY: 300, URX: 200, URY: 340})
	for _, a := range []asposepdf.Annotation{link, stamp, note} {
		if err := p.Annotations().Add(a); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := d.Form().AddTextField(1, asposepdf.Rectangle{LLX: 72, LLY: 500, URX: 300, URY: 520}, "email"); err != nil {
		log.Fatal(err)
	}
	return d
}

func annotatedSamples() {
	d := annotated()
	if _, err := d.AutoTag(asposepdf.DefaultAutoTaggingSettings()); err != nil {
		log.Fatal(err)
	}
	if _, err := d.ConvertToPDFUA(); err != nil {
		log.Fatal(err)
	}
	if err := d.Save(filepath.Join(out, "ua_annotated.pdf")); err != nil {
		log.Fatal(err)
	}
	d = annotated()
	if _, err := d.AutoTag(asposepdf.DefaultAutoTaggingSettings()); err != nil {
		log.Fatal(err)
	}
	rep, err := d.ConvertToPDFA(asposepdf.PDFA2A)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := d.ConvertToPDFUA(); err != nil {
		log.Fatal(err)
	}
	if err := d.Save(filepath.Join(out, "a2a_annotated.pdf")); err != nil {
		log.Fatal(err)
	}
	fmt.Println("a2a_annotated:", rep.Conformant, rep.Issues)
}

func main() {
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, n := range []string{"4pages", "Binder1", "PdfWithLinks", "PdfWithTable", "alfa", "marketing", "PdfWithAcroForm"} {
		src := filepath.Join("testdata", n+".pdf")
		autotag(src, n)
		pdfa(src, n, asposepdf.PDFA2B, "2b", false)
		pdfa(src, n, asposepdf.PDFA1B, "1b", false)
		pdfa(src, n, asposepdf.PDFA2A, "2a", true)
	}
	annotatedSamples()
}
