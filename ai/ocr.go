// SPDX-License-Identifier: MIT

package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// OCREngine recognizes text on one page image. The bundled implementation is
// [LLMOCREngine] (vision model, line-level text, no coordinates); adapters
// over engines that report word boxes (Tesseract, cloud OCR services) plug in
// through the same interface and yield precisely positioned hidden text in
// MakeSearchable.
type OCREngine interface {
	Recognize(ctx context.Context, img image.Image) (*OCRResult, error)
}

// OCRBox is a rectangle in image pixel space: origin at the top-left corner,
// Y increasing downward (the usual raster convention — distinct from PDF user
// space on purpose).
type OCRBox struct {
	Left, Top, Right, Bottom float64
}

// OCRResult is the recognized content of one page image.
type OCRResult struct {
	Lines []OCRLine
}

// OCRLine is one physical text line. Box and Words are optional — nil/empty
// from engines that don't report coordinates (like the LLM engine).
type OCRLine struct {
	Text  string
	Box   *OCRBox
	Words []OCRWord
}

// OCRWord is word-level detail within a line, for engines that provide it.
type OCRWord struct {
	Text string
	Box  OCRBox
}

// Text joins the recognized lines with newlines.
func (r *OCRResult) Text() string {
	lines := make([]string, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = l.Text
	}
	return strings.Join(lines, "\n")
}

// LLMOCROptions configures the vision-model OCR engine.
type LLMOCROptions struct {
	// Language of the document, as a hint for the model (e.g. "Russian").
	// Empty = let the model detect it.
	Language string
}

// LLMOCREngine recognizes text by sending the page image to a vision-capable
// chat model. It returns line-level text in reading order without
// coordinates (vision LLMs transcribe reliably but localize poorly).
type LLMOCREngine struct {
	client AIClient
	opts   LLMOCROptions
}

// NewLLMOCREngine returns an OCREngine backed by a vision-capable AI model.
func NewLLMOCREngine(client AIClient, opts LLMOCROptions) *LLMOCREngine {
	return &LLMOCREngine{client: client, opts: opts}
}

// Recognize sends the image to the model and parses its line-per-line
// transcription.
func (e *LLMOCREngine) Recognize(ctx context.Context, img image.Image) (*OCRResult, error) {
	if e.client == nil {
		return nil, errors.New("ai: LLMOCREngine has no client")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("ai: encode page image: %w", err)
	}

	sys := "You are an OCR engine. Transcribe ALL text visible in the image exactly, line by line, in natural reading order. Output plain text only: one physical line of the image per output line. Preserve original spelling, numbers, punctuation and case. Do not add commentary, labels, or markdown fences. If the image contains no text, output nothing."
	if e.opts.Language != "" {
		sys += " The text is in " + e.opts.Language + "."
	}
	resp, err := e.client.Complete(ctx, CompletionRequest{
		Messages: []Message{
			{Role: RoleSystem, Text: sys},
			{Role: RoleUser, Text: "Transcribe the text in this image.", Images: []MessageImage{{MIME: "image/png", Data: buf.Bytes()}}},
		},
	})
	if err != nil {
		return nil, err
	}

	result := &OCRResult{}
	for _, line := range strings.Split(strings.TrimSpace(resp.Text), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		result.Lines = append(result.Lines, OCRLine{Text: line})
	}
	return result, nil
}

// OcrOptions configures an OcrCopilot.
type OcrOptions struct {
	// Document to process. Required.
	Document *pdf.Document
	// Pages is a 1-based page subset. Nil = every page that looks scanned
	// (no extractable text + a dominant full-page image).
	Pages []int
	// All forces processing of every page (or every listed page) even when
	// it already carries extractable text.
	All bool
	// DPI used to render pages for recognition. 0 = 300.
	DPI float64
	// Font for the hidden text layer written by MakeSearchable. Nil =
	// Helvetica (WinAnsi — Latin only; load a Unicode font via
	// Document.LoadFont for Cyrillic, Greek, etc.).
	Font pdf.Font
}

// TextRecognitionResult is the recognized text of one processed page.
// Mirrors Aspose.PDF for .NET's TextRecognitionResult/OcrDetail.
type TextRecognitionResult struct {
	PageNumber int // 1-based
	Text       string
	Lines      []OCRLine
}

// OcrCopilot recognizes text on scanned pages. GetTextRecognition mirrors
// Aspose.PDF for .NET's OpenAIOcrCopilot; MakeSearchable goes further and
// writes the recognized text back into the PDF as an invisible layer.
//
// Privacy: rendered page images are sent to the engine's AI endpoint.
type OcrCopilot struct {
	engine OCREngine
	opts   OcrOptions
}

// NewOcrCopilot returns a copilot recognizing opts.Document's pages with
// engine.
func NewOcrCopilot(engine OCREngine, opts OcrOptions) *OcrCopilot {
	if opts.DPI <= 0 {
		opts.DPI = 300
	}
	return &OcrCopilot{engine: engine, opts: opts}
}

// GetTextRecognition renders each target page, runs the OCR engine on it, and
// returns the recognized text per page. The document is not modified.
func (c *OcrCopilot) GetTextRecognition(ctx context.Context) ([]TextRecognitionResult, error) {
	targets, err := c.targetPages()
	if err != nil {
		return nil, err
	}
	var results []TextRecognitionResult
	for _, num := range targets {
		res, err := c.recognizePage(ctx, num)
		if err != nil {
			return nil, fmt.Errorf("ai: OCR page %d: %w", num, err)
		}
		results = append(results, TextRecognitionResult{PageNumber: num, Text: res.Text(), Lines: res.Lines})
	}
	return results, nil
}

// recognizePage renders one page at the configured DPI and recognizes it.
func (c *OcrCopilot) recognizePage(ctx context.Context, pageNum int) (*OCRResult, error) {
	if c.engine == nil {
		return nil, errors.New("ai: OcrCopilot has no engine")
	}
	img, err := c.opts.Document.RenderImage(pageNum, pdf.RenderOptions{DPI: c.opts.DPI})
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return c.engine.Recognize(ctx, img)
}

// targetPages resolves the pages to process: the explicit subset, or every
// page that needs OCR (detected), honoring All.
func (c *OcrCopilot) targetPages() ([]int, error) {
	doc := c.opts.Document
	if doc == nil {
		return nil, errors.New("ai: OcrOptions.Document is required")
	}
	if c.opts.Pages != nil {
		total := doc.PageCount()
		for _, n := range c.opts.Pages {
			if n < 1 || n > total {
				return nil, fmt.Errorf("ai: page %d out of range 1..%d", n, total)
			}
		}
		if c.opts.All {
			return append([]int(nil), c.opts.Pages...), nil
		}
		var out []int
		for _, n := range c.opts.Pages {
			p, err := doc.Page(n)
			if err != nil {
				return nil, err
			}
			need, _, err := pageNeedsOCR(p)
			if err != nil {
				return nil, err
			}
			if need {
				out = append(out, n)
			}
		}
		return out, nil
	}
	var out []int
	for i := 1; i <= doc.PageCount(); i++ {
		p, err := doc.Page(i)
		if err != nil {
			return nil, err
		}
		if c.opts.All {
			out = append(out, i)
			continue
		}
		need, _, err := pageNeedsOCR(p)
		if err != nil {
			return nil, err
		}
		if need {
			out = append(out, i)
		}
	}
	return out, nil
}

// pageNeedsOCR reports whether a page looks scanned: fewer than 3 extractable
// characters (tolerating producer noise) and at least one image covering
// ≥ 70 % of the page area. The dominant image's display rect is returned for
// the heuristic text placement.
func pageNeedsOCR(p *pdf.Page) (bool, *pdf.Rectangle, error) {
	text, err := p.ExtractText()
	if err != nil {
		return false, nil, err
	}
	if len(strings.TrimSpace(text)) >= 3 {
		return false, nil, nil
	}
	size, err := p.Size()
	if err != nil {
		return false, nil, err
	}
	pageArea := size.Width * size.Height
	if pageArea <= 0 {
		return false, nil, nil
	}
	infos, err := p.ImageInfos()
	if err != nil {
		return false, nil, err
	}
	var best *pdf.Rectangle
	var bestArea float64
	for _, info := range infos {
		area := info.PageWidth * info.PageHeight
		if area > bestArea {
			bestArea = area
			best = &pdf.Rectangle{LLX: info.X, LLY: info.Y, URX: info.X + info.PageWidth, URY: info.Y + info.PageHeight}
		}
	}
	if best == nil || bestArea < 0.7*pageArea {
		return false, nil, nil
	}
	return true, best, nil
}
