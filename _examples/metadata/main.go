package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// parsePDFDate parses a PDF date string (D:YYYYMMDDHHmmSSOHH'mm')
// and returns an ISO 8601 string, or the original value on parse failure.
func parsePDFDate(s string) string {
	s = strings.TrimPrefix(s, "D:")
	s = strings.ReplaceAll(s, "'", "")
	layouts := []string{
		"20060102150405-0700",
		"20060102150405Z",
		"20060102150405",
		"200601021504",
		"2006010215",
		"20060102",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return s
}

type readableMetadata struct {
	Title        string            `json:"Title,omitempty"`
	Author       string            `json:"Author,omitempty"`
	Subject      string            `json:"Subject,omitempty"`
	Keywords     string            `json:"Keywords,omitempty"`
	Creator      string            `json:"Creator,omitempty"`
	Producer     string            `json:"Producer,omitempty"`
	CreationDate string            `json:"CreationDate,omitempty"`
	ModDate      string            `json:"ModDate,omitempty"`
	Custom       map[string]string `json:"Custom,omitempty"`
}

func printMetadata(doc *pdf.Document) {
	meta, err := doc.Metadata()
	if err != nil {
		log.Fatalf("metadata: %v", err)
	}
	out, err := json.MarshalIndent(readableMetadata{
		Title:        meta.Title,
		Author:       meta.Author,
		Subject:      meta.Subject,
		Keywords:     meta.Keywords,
		Creator:      meta.Creator,
		Producer:     meta.Producer,
		CreationDate: parsePDFDate(meta.CreationDate),
		ModDate:      parsePDFDate(meta.ModDate),
		Custom:       meta.Custom,
	}, "", "  ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	fmt.Println(string(out))
}

func main() {
	doc, err := pdf.Open("testdata/split/4pages.pdf")
	if err != nil {
		log.Fatalf("open: %v", err)
	}

	doc.ClearMetadata()

	printMetadata(doc)
}
