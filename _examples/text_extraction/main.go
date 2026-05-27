package main

import (
	"fmt"
	"log"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc, err := pdf.Open("testdata/binder1.pdf")
	if err != nil {
		log.Fatalf("open: %v", err)
	}

	// Extract text from each page individually.
	fmt.Println("=== Per-page text extraction ===")
	for i := 1; i <= doc.PageCount(); i++ {
		page, err := doc.Page(i)
		if err != nil {
			log.Fatalf("page %d: %v", i, err)
		}
		text, err := page.ExtractText()
		if err != nil {
			log.Fatalf("extract page %d: %v", i, err)
		}
		fmt.Printf("--- Page %d ---\n", i)
		if strings.TrimSpace(text) == "" {
			fmt.Println("(empty)")
		} else {
			fmt.Println(text)
		}
		fmt.Println()
	}

	// Extract text from the entire document at once.
	fmt.Println("=== Full document text extraction ===")
	texts, err := doc.ExtractText()
	if err != nil {
		log.Fatalf("extract all: %v", err)
	}
	for i, text := range texts {
		fmt.Printf("--- Page %d ---\n", i+1)
		if strings.TrimSpace(text) == "" {
			fmt.Println("(empty)")
		} else {
			fmt.Println(text)
		}
		fmt.Println()
	}
}
