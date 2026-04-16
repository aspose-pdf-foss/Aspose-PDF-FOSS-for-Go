package asposepdf

import (
	"strings"
)

// wrapText splits text into lines that fit within maxWidth points.
// It breaks at spaces; words longer than maxWidth are broken by character.
// Explicit newlines in the input force a line break.
func wrapText(text string, widths [256]float64, fontSize, maxWidth float64) []string {
	if text == "" {
		return nil
	}

	var result []string
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var line string
		var lineWidth float64

		for _, word := range words {
			wordWidth := measureString(word, widths, fontSize)

			if lineWidth == 0 {
				// First word on line.
				if wordWidth <= maxWidth {
					line = word
					lineWidth = wordWidth
				} else {
					// Word too long — break by character.
					broken := breakWord(word, widths, fontSize, maxWidth)
					for i, part := range broken {
						if i < len(broken)-1 {
							result = append(result, part)
						} else {
							line = part
							lineWidth = measureString(part, widths, fontSize)
						}
					}
				}
			} else {
				spaceWidth := widths[' '] / 1000.0 * fontSize
				if lineWidth+spaceWidth+wordWidth <= maxWidth {
					line += " " + word
					lineWidth += spaceWidth + wordWidth
				} else {
					result = append(result, line)
					if wordWidth <= maxWidth {
						line = word
						lineWidth = wordWidth
					} else {
						broken := breakWord(word, widths, fontSize, maxWidth)
						for i, part := range broken {
							if i < len(broken)-1 {
								result = append(result, part)
							} else {
								line = part
								lineWidth = measureString(part, widths, fontSize)
							}
						}
					}
				}
			}
		}
		if line != "" || lineWidth == 0 {
			result = append(result, line)
		}
	}

	return result
}

// measureString returns the width of a string in points.
func measureString(s string, widths [256]float64, fontSize float64) float64 {
	var w float64
	for i := 0; i < len(s); i++ {
		w += widths[s[i]] / 1000.0 * fontSize
	}
	return w
}

// breakWord breaks a single word into parts that each fit within maxWidth.
func breakWord(word string, widths [256]float64, fontSize, maxWidth float64) []string {
	var parts []string
	start := 0
	var w float64
	for i := 0; i < len(word); i++ {
		cw := widths[word[i]] / 1000.0 * fontSize
		if w+cw > maxWidth && i > start {
			parts = append(parts, word[start:i])
			start = i
			w = 0
		}
		w += cw
	}
	if start < len(word) {
		parts = append(parts, word[start:])
	}
	return parts
}

// escapeStringPDF escapes special characters for a PDF literal string.
func escapeStringPDF(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			b.WriteString("\\(")
		case ')':
			b.WriteString("\\)")
		case '\\':
			b.WriteString("\\\\")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
