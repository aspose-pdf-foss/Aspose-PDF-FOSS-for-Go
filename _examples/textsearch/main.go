// textsearch opens a real PDF, searches it for a query string, and prints the
// matches (text + page + bounding rectangle) as JSON.
//
// Usage:
//
//	go run ./_examples/textsearch                 # default: "Marketing" in testdata/marketing.pdf
//	go run ./_examples/textsearch <query>         # custom query, default document
//	go run ./_examples/textsearch <query> <file>  # custom query and document
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

type rect struct {
	LLX float64 `json:"llx"`
	LLY float64 `json:"lly"`
	URX float64 `json:"urx"`
	URY float64 `json:"ury"`
}

type match struct {
	Page int    `json:"page"`
	Text string `json:"text"`
	Rect rect   `json:"rect"`
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func main() {
	query := "Marketing"
	path := "testdata/marketing.pdf"
	if len(os.Args) > 1 {
		query = os.Args[1]
	}
	if len(os.Args) > 2 {
		path = os.Args[2]
	}

	doc, err := pdf.Open(path)
	if err != nil {
		log.Fatalf("open %q: %v", path, err)
	}

	// Case-insensitive so "Marketing" and "marketing" both surface.
	found, err := doc.SearchText(query, pdf.SearchOptions{CaseInsensitive: true})
	if err != nil {
		log.Fatalf("search %q: %v", query, err)
	}

	matches := make([]match, 0, len(found))
	for _, m := range found {
		matches = append(matches, match{
			Page: m.PageNumber,
			Text: m.Text,
			Rect: rect{round1(m.Rect.LLX), round1(m.Rect.LLY), round1(m.Rect.URX), round1(m.Rect.URY)},
		})
	}

	result := struct {
		File    string  `json:"file"`
		Query   string  `json:"query"`
		Count   int     `json:"count"`
		Matches []match `json:"matches"`
	}{File: path, Query: query, Count: len(matches), Matches: matches}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	fmt.Println(string(out))
}
