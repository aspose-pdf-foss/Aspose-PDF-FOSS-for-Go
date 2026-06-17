// Regenerates docs/feature_showcase-preview.png — the clickable preview shown
// in the README — as a 2×2 montage of representative pages from
// docs/feature_showcase.pdf. Every page is rasterized by this library's own
// pure-Go renderer (Document.RenderImage); only the grid assembly uses the
// standard library's image package.
//
// Run AFTER regenerating the showcase itself:
//
//	go run ./_examples/feature_showcase
//	go run ./_examples/showcase_preview
//
// Output: docs/feature_showcase-preview.png
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc, err := pdf.Open("docs/feature_showcase.pdf")
	if err != nil {
		log.Fatalf("open showcase: %v", err)
	}

	// Representative portrait pages: cover, Annotation Gallery, Multi-Page
	// Sales Report, Vector Graphics. (Same selection as the original preview.)
	pages := []int{1, 6, 9, 12}
	imgs := make([]image.Image, len(pages))
	for i, n := range pages {
		im, err := doc.RenderImage(n, pdf.RenderOptions{DPI: 64})
		if err != nil {
			log.Fatalf("render page %d: %v", n, err)
		}
		imgs[i] = im
	}

	cw := imgs[0].Bounds().Dx()
	ch := imgs[0].Bounds().Dy()
	const gap, margin = 18, 18
	w := margin*2 + cw*2 + gap
	h := margin*2 + ch*2 + gap

	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	// Row-major 2×2: (col, row).
	cells := [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	frame := color.RGBA{R: 200, G: 200, B: 205, A: 255}
	for i, im := range imgs {
		col, row := cells[i][0], cells[i][1]
		x := margin + col*(cw+gap)
		y := margin + row*(ch+gap)
		r := image.Rect(x, y, x+cw, y+ch)
		draw.Draw(canvas, r, im, im.Bounds().Min, draw.Src)
		drawBorder(canvas, r, frame)
	}

	out, err := os.Create("docs/feature_showcase-preview.png")
	if err != nil {
		log.Fatalf("create preview: %v", err)
	}
	defer out.Close()
	if err := png.Encode(out, canvas); err != nil {
		log.Fatalf("encode preview: %v", err)
	}
	log.Printf("wrote docs/feature_showcase-preview.png (%d×%d)", w, h)
}

// drawBorder strokes a 1px rectangle outline.
func drawBorder(img *image.RGBA, r image.Rectangle, c color.Color) {
	for x := r.Min.X; x < r.Max.X; x++ {
		img.Set(x, r.Min.Y, c)
		img.Set(x, r.Max.Y-1, c)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		img.Set(r.Min.X, y, c)
		img.Set(r.Max.X-1, y, c)
	}
}
