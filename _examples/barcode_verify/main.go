// SPDX-License-Identifier: MIT

// barcode_verify renders BarcodeField symbols of every symbology to PNGs (through
// the real field/appearance/rasterizer pipeline) plus a manifest.json of what
// each should decode to, for tools/validate_barcodes.py to check with a
// third-party decoder. Usage: go run ./_examples/barcode_verify [outdir]
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

type item struct {
	File   string `json:"file"`
	Format string `json:"format"`
	Value  string `json:"value"`
}

func main() {
	out := "result_files/barcodes"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	var ascii strings.Builder
	for r := 0x20; r <= 0x7e; r++ {
		ascii.WriteRune(rune(r))
	}
	type c struct {
		sym   pdf.BarcodeSymbology
		name  string
		value string
		rect  pdf.Rectangle
	}
	wide := pdf.Rectangle{LLX: 40, LLY: 600, URX: 555, URY: 700}
	square := pdf.Rectangle{LLX: 100, LLY: 400, URX: 500, URY: 800}
	tall := pdf.Rectangle{LLX: 200, LLY: 300, URX: 340, URY: 800}
	cases := []c{
		{pdf.BarcodeCode128, "Code128", "HELLO", wide},
		{pdf.BarcodeCode128, "Code128", "Hello, World! 123", wide},
		{pdf.BarcodeCode128, "Code128", "0123456789", wide},
		{pdf.BarcodeCode128, "Code128", ascii.String(), wide},
		{pdf.BarcodeCode128, "Code128", strings.Repeat("Ab1-", 15), wide},
		{pdf.BarcodeQR, "QRCode", "HELLO", square},
		{pdf.BarcodeQR, "QRCode", "https://example.com/sku/123?x=1&y=2", square},
		{pdf.BarcodeQR, "QRCode", "Привет, мир — héllo 日本語", square},
		{pdf.BarcodeQR, "QRCode", strings.Repeat("QR multi-block stress test. ", 12), square}, // version 10+
		{pdf.BarcodeQR, "QRCode", strings.Repeat("v7+ version info block ", 9), square},       // version >= 7
		{pdf.BarcodeQR, "QRCode", strings.Repeat("x", 1000), square},
		{pdf.BarcodePDF417, "PDF417", "HELLO", wide},
		{pdf.BarcodePDF417, "PDF417", "ABCDEF", wide}, // multiple of 6: latch 924
		{pdf.BarcodePDF417, "PDF417", "https://example.com/invoice/2026-09-24?total=133.00", wide},
		{pdf.BarcodePDF417, "PDF417", "Привет, мир — héllo 日本語", square},
		{pdf.BarcodePDF417, "PDF417", strings.Repeat("PDF417 stress test data. ", 30), tall},
		{pdf.BarcodePDF417, "PDF417", strings.Repeat("a", 1108), square}, // capacity edge
	}

	var manifest []item
	for i, tc := range cases {
		doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
		if _, err := doc.Form().AddBarcodeField(1, tc.rect, "bc", tc.sym, tc.value); err != nil {
			log.Fatalf("case %d (%s): %v", i, tc.name, err)
		}
		page, err := doc.Page(1)
		if err != nil {
			log.Fatal(err)
		}
		file := fmt.Sprintf("%02d_%s.png", i, strings.ToLower(tc.name))
		f, err := os.Create(filepath.Join(out, file))
		if err != nil {
			log.Fatal(err)
		}
		if err := page.RenderPNG(f, pdf.RenderOptions{DPI: 300}); err != nil {
			log.Fatal(err)
		}
		f.Close()
		manifest = append(manifest, item{file, tc.name, tc.value})
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), data, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d symbols to %s\n", len(manifest), out)
}
