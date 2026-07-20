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

// ImageDescriptionOptions configures an ImageDescriptionCopilot.
type ImageDescriptionOptions struct {
	// Language of the description; "" = the model's default (usually English).
	Language string
	// MaxWords caps the description length; 0 → a short caption (the default
	// prompt asks for one concise sentence).
	MaxWords int
	// Prompt appends an extra instruction to the system prompt (e.g. domain
	// context: "these are engineering diagrams").
	Prompt string
}

// ImageDescriptionCopilot describes images with a vision model. Mirrors
// Aspose.PDF for .NET's OpenAIImageDescriptionCopilot; FillAltTexts extends it
// with a one-call accessibility fix for tagged PDFs.
//
// Privacy: the images are sent to the configured AI endpoint.
type ImageDescriptionCopilot struct {
	client AIClient
	opts   ImageDescriptionOptions
}

// NewImageDescriptionCopilot returns a copilot that describes images via
// client.
func NewImageDescriptionCopilot(client AIClient, opts ImageDescriptionOptions) *ImageDescriptionCopilot {
	return &ImageDescriptionCopilot{client: client, opts: opts}
}

// Describe returns a short natural-language description of img, suitable as
// alternate text.
func (c *ImageDescriptionCopilot) Describe(ctx context.Context, img image.Image) (string, error) {
	if img == nil {
		return "", errors.New("ai: nil image")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("ai: encode image: %w", err)
	}
	return c.describeBytes(ctx, buf.Bytes(), "image/png")
}

// describeBytes describes already-encoded image bytes (PNG or JPEG).
func (c *ImageDescriptionCopilot) describeBytes(ctx context.Context, data []byte, mime string) (string, error) {
	if c.client == nil {
		return "", errors.New("ai: ImageDescriptionCopilot has no client")
	}
	var sys strings.Builder
	sys.WriteString("You describe images to serve as alternate text for accessibility. ")
	sys.WriteString("Respond with only the description — no preamble, no 'image of', no markdown. ")
	if c.opts.MaxWords > 0 {
		fmt.Fprintf(&sys, "Keep it under %d words. ", c.opts.MaxWords)
	} else {
		sys.WriteString("Keep it to one concise sentence. ")
	}
	if c.opts.Language != "" {
		fmt.Fprintf(&sys, "Write the description in %s. ", c.opts.Language)
	}
	if c.opts.Prompt != "" {
		sys.WriteString(c.opts.Prompt)
	}

	resp, err := c.client.Complete(ctx, CompletionRequest{
		Messages: []Message{
			{Role: RoleSystem, Text: strings.TrimSpace(sys.String())},
			{Role: RoleUser, Text: "Describe this image.", Images: []MessageImage{{MIME: mime, Data: data}}},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// FillAltTexts finds every /Figure in a tagged document that lacks alternate
// text, describes its image with the vision model, and sets the description as
// the figure's /Alt — turning ValidatePDFUA's UA_FIGURE_NO_ALT findings into a
// one-call fix. Returns the number of figures filled. Figures whose image
// cannot be located are skipped (reported via the count). The document is
// modified in place; call Save/WriteTo to persist.
func (c *ImageDescriptionCopilot) FillAltTexts(ctx context.Context, doc *pdf.Document) (int, error) {
	if doc == nil {
		return 0, errors.New("ai: nil document")
	}
	figures, err := doc.FiguresNeedingAltText()
	if err != nil {
		return 0, err
	}
	filled := 0
	for _, fig := range figures {
		img, ok := fig.Image()
		if !ok {
			continue
		}
		desc, err := c.describeBytes(ctx, img.Data, mimeOf(img.Format))
		if err != nil {
			return filled, err
		}
		if desc == "" {
			continue
		}
		fig.SetAltText(desc)
		filled++
	}
	return filled, nil
}

func mimeOf(f pdf.ImageFormat) string {
	if f == pdf.ImageFormatJPEG {
		return "image/jpeg"
	}
	return "image/png"
}
