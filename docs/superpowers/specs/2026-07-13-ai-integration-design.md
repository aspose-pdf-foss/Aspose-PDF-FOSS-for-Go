# AI integration (`ai` subpackage) — design

**Date:** 2026-07-13 · **Status:** approved (brainstorm with Andrey) · **Epic:** pdf-go (beads ids assigned at filing)

## Motivation

Add AI-powered document operations — summarization, OCR of scanned pages, Q&A, image
description — mirroring **Aspose.PDF for .NET's `Aspose.Pdf.AI` namespace** (the modern
"copilot" surface; there is also the older `Aspose.Pdf.Plugins.PdfChatGpt` plugin, which we
do not mirror). Everything stays **pure Go, stdlib only**: the LLM client is `net/http` +
`encoding/json` against an OpenAI-compatible endpoint.

Key competitive finding: Aspose's `OpenAIOcrCopilot` **only extracts text**
(`GetTextRecognitionResultAsync` → `TextRecognitionResult.OcrDetails[].ExtractedText`).
It does not produce a searchable PDF. We own the whole PDF stack (renderer for page
images, writer for an invisible text layer), so **`MakeSearchable` — scanned PDF in,
selectable/searchable PDF out — is our differentiator**, the second "beyond Aspose"
capability after interactive HTML forms.

## Decisions (from brainstorm)

1. **API shape: copilots, mirroring Aspose.Pdf.AI.** `ai.NewSummaryCopilot(client, opts)`
   → `GetSummary(ctx)` / `SaveSummary(ctx, path)`; recognizable to Aspose.PDF for .NET
   users, adapted to Go idioms (ctx + error returns instead of async/exceptions, options
   structs instead of `WithX()` builders).
2. **Providers: one OpenAI-compatible client behind an interface.** `ai.AIClient` is the
   contract; `ai.NewOpenAIClient` is the bundled implementation (works with OpenAI,
   LiteLLM, Ollama, OpenRouter, Azure-compatible gateways — same choice as the AI-editor
   project). Anthropic/native providers reachable via a gateway or a user-supplied
   `AIClient`.
3. **Phase 1 scope: SummaryCopilot + OCR/MakeSearchable.** ChatCopilot and
   ImageDescriptionCopilot (+ auto-`/Alt` for tagged PDFs) are phases 2–3.
4. **OCR box problem: hybrid.** Vision LLMs return text reliably but bounding boxes
   poorly. The LLM engine asks for **line-level** text; the invisible layer is laid out
   line-by-line with an even-spacing heuristic over the scanned region — Ctrl+F and
   copy/paste work, selection alignment is approximate. An **`OCREngine` interface** lets
   users plug engines with real word boxes (Tesseract, cloud OCR) for exact alignment.

## Package layout

New subpackage `github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/ai`, same module.

- The **root package stays free of AI and network code**; Go only compiles imported
  packages, so the zero-dependency story of the core is untouched.
- `ai` imports the root package (for `*asposepdf.Document`, `RenderPNG`, `AddText`,
  `NewDocument`, …) and **only its public API** — any capability OCR needs from the core
  is added to the core as a small public feature (see "Core additions").
- Files: `ai/client.go` (interface + types), `ai/openai.go` (HTTP client),
  `ai/summary.go`, `ai/ocr.go` (engine interface + LLM engine), `ai/searchable.go`
  (MakeSearchable), later `ai/chat.go`, `ai/imagedesc.go`.

## Client

```go
// The contract every copilot consumes. One call = one chat completion.
type AIClient interface {
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

type CompletionRequest struct {
    Messages    []Message // roles: system / user / assistant
    Temperature *float64  // nil = provider default
    MaxTokens   int       // 0 = provider default
}

type Message struct {
    Role   string  // "system" | "user" | "assistant"
    Text   string
    Images []MessageImage // vision: base64 data-URL parts (PNG/JPEG)
}

type MessageImage struct{ MIME string; Data []byte }

type CompletionResponse struct {
    Text  string
    Usage Usage // prompt/completion token counts when the provider reports them
}
```

`NewOpenAIClient(opts OpenAIClientOptions) *OpenAIClient` — options struct (Go idiom
instead of Aspose's builder): `BaseURL` (default `https://api.openai.com/v1`), `APIKey`,
`Model` (required), `HTTPClient *http.Client` (nil → sensible default with timeout),
`MaxRetries int` (429/5xx exponential backoff, default 2). POSTs
`{base}/chat/completions`; text and vision (`image_url` with `data:` URLs) content parts;
no streaming in v1. Errors carry HTTP status + provider error message
(`*ai.APIError{Status, Code, Message}`).

## SummaryCopilot (phase 1a)

Mirrors `OpenAISummaryCopilot` (`GetSummaryAsync` / `GetSummaryDocumentAsync` /
`SaveSummaryAsync`):

```go
func NewSummaryCopilot(client AIClient, opts SummaryOptions) *SummaryCopilot

type SummaryOptions struct {
    Document  *asposepdf.Document   // exactly one of Document / Documents / Texts
    Documents []*asposepdf.Document // multi-doc summary (Aspose DocumentCollection)
    Texts     []string              // raw text inputs (Aspose TextDocument)
    Language  string                // "" = same language as the document
    MaxWords  int                   // 0 = model default; passed as instruction
    Prompt    string                // extra instruction appended to the system prompt
}

func (c *SummaryCopilot) GetSummary(ctx context.Context) (string, error)
func (c *SummaryCopilot) GetSummaryDocument(ctx context.Context, format ...asposepdf.PageFormat) (*asposepdf.Document, error)
func (c *SummaryCopilot) SaveSummary(ctx context.Context, path string) error
```

Pipeline: `Document.ExtractText()` per page → chunking → LLM.

- **Chunking (map-reduce):** pages are concatenated into chunks of ~`chunkRunes`
  (≈24 000 runes ≈ 6–8k tokens, conservative for small-context local models). One chunk →
  single summarization call. Multiple chunks → summarize each ("map"), then summarize the
  summaries ("reduce"). Page boundaries are never split mid-page.
- **Scanned pages** (no extractable text): skipped in v1 with a note in the prompt; the
  README points at running `MakeSearchable` first. (Vision fallback = later polish.)
- `GetSummaryDocument` renders the summary via the existing flow layout (`NewFlow` +
  heading + paragraphs) — reuses, not reimplements, the generator.

## OCR (phase 1b)

### Engine interface

```go
// OCREngine recognizes text on one page image. Implementations: the bundled
// LLM engine (line-level, approximate boxes) or user adapters over Tesseract /
// cloud OCR services (word-level, exact boxes).
type OCREngine interface {
    Recognize(ctx context.Context, img image.Image) (*OCRResult, error)
}

type OCRResult struct{ Lines []OCRLine }

type OCRLine struct {
    Text  string
    Box   *asposepdf.Rectangle // in image pixel space, Y down; nil = unknown
    Words []OCRWord            // optional word-level detail (empty from the LLM engine)
}

type OCRWord struct {
    Text string
    Box  asposepdf.Rectangle
}
```

`NewLLMOCREngine(client AIClient, opts LLMOCROptions) *LLMOCREngine` — sends the page PNG
to the vision model, asks for a strict line-per-line transcription (system prompt pins
format: plain text, one physical line per line, no commentary, preserve reading order).
`LLMOCROptions{Language string, DPI float64 /* render DPI, default 300 */}`. Boxes are nil.

### OcrCopilot — Aspose-compatible text extraction

```go
func NewOcrCopilot(engine OCREngine, opts OcrOptions) *OcrCopilot

type OcrOptions struct {
    Document *asposepdf.Document
    Pages    []int // 1-based subset; nil = pages that need OCR (see detection)
    All      bool  // force-OCR every listed page even if it already has text
}

// Mirrors GetTextRecognitionResultAsync: recognized text per processed page.
func (c *OcrCopilot) GetTextRecognition(ctx context.Context) ([]TextRecognitionResult, error)

type TextRecognitionResult struct {
    PageNumber int
    Text       string // lines joined with \n
    Lines      []OCRLine
}
```

### MakeSearchable — the differentiator

```go
// MakeSearchable OCRs the document's scanned pages and injects an invisible
// text layer (text rendering mode 3) over each, so the PDF becomes
// selectable, copyable and Ctrl+F-searchable in any viewer. The visual
// appearance is unchanged. Returns the number of pages processed.
func (c *OcrCopilot) MakeSearchable(ctx context.Context) (int, error)
```

- **Scanned-page detection** (`needsOCR`): page has no extractable text (or < 3 glyphs —
  tolerate producer noise) **and** at least one image covering ≥ 70 % of the page area
  (via existing `ImageInfos` + page size). `Pages`/`All` override.
- **Placement with real boxes** (engine provided them): image-pixel boxes are mapped to
  PDF user space through the render scale (`72/DPI`, Y flip); each line (or word) is drawn
  invisibly at its box with font size = box height and per-line horizontal scaling to
  match the box width (same fitting idea as the HTML text layer).
- **Placement heuristic (LLM engine, no boxes):** the scanned region = the dominant
  image's rect. N recognized lines are laid out top-to-bottom on an even grid inside that
  rect (line height = rect height / N, capped to a sane font-size range); each line is
  width-fitted with horizontal scaling. Search and copy work; selection is roughly
  aligned. Documented honestly as approximate.
- The layer is drawn with a Standard-14 face (WinAnsi text) or the DejaVu-style embedded
  font when the text needs Unicode — reusing `AddText`'s font machinery; `SubsetFonts`
  applies as usual.

### Core additions (root package, public)

- `TextStyle.Invisible bool` — emit `3 Tr` around the shown text (ISO 32000-1 §9.3.6).
  Generally useful (watermark-under-content tricks, testing), tiny change in
  `text_add.go`; the renderer already honors mode 3. This keeps `ai` on public API only.

## Phase 2 — ChatCopilot (later)

`NewChatCopilot(client, ChatOptions{Documents, SystemPrompt})` →
`Ask(ctx, q) (string, error)`, `History()`, `Reset()`. Context = extracted text (chunked,
truncated to a budget with a "context window" note). No embeddings/vector store in v1 —
Aspose uses OpenAI Assistants + vector stores; we deliberately start with plain stuffing
and add retrieval only if real documents demand it.

## Phase 3 — ImageDescriptionCopilot + auto-Alt (later)

Mirrors `OpenAIImageDescriptionCopilot`: describe extracted images. Our twist:
`FillAltTexts(ctx, doc)` walks a tagged document's structure tree and fills missing
`/Alt` on `/Figure` elements — turns `ValidatePDFUA` `UA_FIGURE_NO_ALT` findings into a
one-call fix.

## Testing

- **No network in CI.** All copilot/engine tests run against `httptest.Server` serving
  canned OpenAI-style JSON; assert request shape (model, messages, data-URL images) and
  pipeline behavior (chunk splits, map-reduce, retry on 429).
- `MakeSearchable` end-to-end with a **fake engine** returning known lines/boxes over a
  scanned-style fixture (rendered page re-embedded as full-page image): after OCR,
  `ExtractText` finds the words, `SearchText` locates them near expected coordinates, and
  a render before/after pixel-diff proves invisibility.
- One gated live-API example under `_examples/ai_summary/` (env `OPENAI_*`), never run by
  `go test`.

## Privacy & docs

README + doc comments must state plainly: these features **send document content
(text and/or page images) to the configured AI endpoint**. Nothing is sent unless an
`ai` copilot is explicitly invoked; for sensitive documents point users at local
endpoints (Ollama/LiteLLM) which the same client supports.

## Out of scope (v1)

Streaming responses; embeddings/vector stores; fine-tuning and the Assistants API
bindings (Aspose's 89-class binding layer is an implementation detail, not a public
capability); vision fallback inside SummaryCopilot; word-exact boxes from the LLM engine;
bundled Tesseract bindings (cgo — never).
