// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/json"
	"io"
)

// JSONDiffOutputGenerator renders comparison differences as JSON. Mirrors
// Aspose.PDF for .NET's JsonDiffOutputGenerator.
type JSONDiffOutputGenerator struct{}

// NewJSONDiffOutputGenerator creates a generator. It takes no options — the
// operation data speaks for itself in JSON, unlike HTML/PDF which need colors
// to mark differences visually.
func NewJSONDiffOutputGenerator() *JSONDiffOutputGenerator {
	return &JSONDiffOutputGenerator{}
}

var _ StringDiffOutputGenerator = (*JSONDiffOutputGenerator)(nil)
var _ FileDiffOutputGenerator = (*JSONDiffOutputGenerator)(nil)

// diffRectJSON is Rectangle's JSON shape: lowerCamelCase keys, matching the
// rest of this generator's schema (Rectangle itself carries no json tags,
// since it is a shared type used well beyond JSON output).
type diffRectJSON struct {
	LLX float64 `json:"llx"`
	LLY float64 `json:"lly"`
	URX float64 `json:"urx"`
	URY float64 `json:"ury"`
}

func rectsJSON(rects []Rectangle) []diffRectJSON {
	if len(rects) == 0 {
		return nil
	}
	out := make([]diffRectJSON, len(rects))
	for i, r := range rects {
		out[i] = diffRectJSON(r)
	}
	return out
}

// diffOperationJSON is one DiffOperation's JSON shape.
type diffOperationJSON struct {
	Operation   string         `json:"operation"`
	Text        string         `json:"text"`
	SourcePage  int            `json:"sourcePage"`
	DestPage    int            `json:"destPage"`
	SourceRects []diffRectJSON `json:"sourceRects,omitempty"`
	DestRects   []diffRectJSON `json:"destRects,omitempty"`
}

func opJSON(op DiffOperation) diffOperationJSON {
	return diffOperationJSON{
		Operation:   op.Operation.String(),
		Text:        op.Text,
		SourcePage:  op.SourcePage,
		DestPage:    op.DestPage,
		SourceRects: rectsJSON(op.SourceRects),
		DestRects:   rectsJSON(op.DestRects),
	}
}

// diffPageJSON is one page's JSON shape in the per-page output.
type diffPageJSON struct {
	Page       int                 `json:"page"`
	Operations []diffOperationJSON `json:"operations"`
}

// GenerateOutput renders ops as a flat JSON array of operations.
func (g *JSONDiffOutputGenerator) GenerateOutput(ops []DiffOperation) (string, error) {
	out := make([]diffOperationJSON, len(ops))
	for i, op := range ops {
		out[i] = opJSON(op)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GenerateOutputPages renders pages as a JSON array of {"page","operations"}
// objects, one per page.
func (g *JSONDiffOutputGenerator) GenerateOutputPages(pages [][]DiffOperation) (string, error) {
	out := make([]diffPageJSON, len(pages))
	for i, ops := range pages {
		opsJSON := make([]diffOperationJSON, len(ops))
		for j, op := range ops {
			opsJSON[j] = opJSON(op)
		}
		out[i] = diffPageJSON{Page: i + 1, Operations: opsJSON}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteOutput writes GenerateOutput's JSON to w.
func (g *JSONDiffOutputGenerator) WriteOutput(w io.Writer, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// WriteOutputPages writes GenerateOutputPages's JSON to w.
func (g *JSONDiffOutputGenerator) WriteOutputPages(w io.Writer, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// SaveOutput writes GenerateOutput's JSON to path.
func (g *JSONDiffOutputGenerator) SaveOutput(path string, ops []DiffOperation) error {
	s, err := g.GenerateOutput(ops)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}

// SaveOutputPages writes GenerateOutputPages's JSON to path.
func (g *JSONDiffOutputGenerator) SaveOutputPages(path string, pages [][]DiffOperation) error {
	s, err := g.GenerateOutputPages(pages)
	if err != nil {
		return err
	}
	return writeFile(path, []byte(s))
}
