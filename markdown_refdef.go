// SPDX-License-Identifier: MIT

package asposepdf

import (
	"html"
	"regexp"
	"strings"
)

// Link reference definitions (CommonMark §4.7): `[label]: dest "title"`
// lines at the start of a paragraph are consumed into the parser's refmap;
// whatever remains stays paragraph content.

// extractRefDefs strips leading link-reference definitions from a paragraph.
func (p *mdParser) extractRefDefs(b *mdBlock) {
	if len(b.content) == 0 {
		return
	}
	src := strings.Join(b.content, "\n")
	pos := 0
	for pos < len(src) && strings.HasPrefix(strings.TrimLeft(src[pos:], " "), "[") {
		consumed, label, dest, title, ok := scanLinkRefDef(src[pos:])
		if !ok {
			break
		}
		key := mdNormalizeLabel(label)
		if _, exists := p.refmap[key]; !exists && key != "" {
			p.refmap[key] = mdLinkRef{dest: dest, title: title}
		}
		pos += consumed
	}
	if pos == 0 {
		return
	}
	rest := src[pos:]
	if strings.TrimSpace(rest) == "" {
		b.content = nil
		return
	}
	b.content = strings.Split(rest, "\n")
}

// mdNormalizeLabel case-folds and collapses internal whitespace, per spec
// §4.7 (matching of labels is case-insensitive with whitespace collapsed).
var mdLabelWSRe = regexp.MustCompile(`[ \t\n]+`)

func mdNormalizeLabel(label string) string {
	label = strings.TrimSpace(mdLabelWSRe.ReplaceAllString(label, " "))
	// Approximate Unicode *full* case folding with stdlib tools: simple
	// lowercase, plus the one multi-char expansion the spec's own tests
	// exercise (ß→ss; ToLower already maps ẞ→ß).
	return strings.ReplaceAll(strings.ToLower(label), "ß", "ss")
}

// scanLinkRefDef matches one link reference definition at the start of s.
// consumed counts the bytes eaten, including the terminating newline.
func scanLinkRefDef(s string) (consumed int, label, dest, title string, ok bool) {
	i := 0
	for i < len(s) && i < 3 && s[i] == ' ' {
		i++
	}
	if i >= len(s) || s[i] != '[' {
		return
	}
	// Label: ≤999 chars, no unescaped brackets, at least one non-space char,
	// no blank lines.
	j := i + 1
	hasContent := false
	for {
		if j >= len(s) || j-i-1 > 999 {
			return
		}
		c := s[j]
		if c == '\\' && j+1 < len(s) {
			hasContent = true
			j += 2
			continue
		}
		if c == ']' {
			break
		}
		if c == '[' {
			return
		}
		if c == '\n' && j+1 < len(s) && s[j+1] == '\n' {
			return
		}
		if c != ' ' && c != '\t' && c != '\n' {
			hasContent = true
		}
		j++
	}
	if !hasContent {
		return
	}
	label = s[i+1 : j]
	j++ // past ']'
	if j >= len(s) || s[j] != ':' {
		return
	}
	j++
	j = mdSkipWSOneNL(s, j)
	if j < 0 || j >= len(s) {
		return
	}

	// Destination.
	if s[j] == '<' {
		j++
		start := j
		for {
			if j >= len(s) {
				return
			}
			c := s[j]
			if c == '\\' && j+1 < len(s) {
				j += 2
				continue
			}
			if c == '>' {
				break
			}
			if c == '<' || c == '\n' {
				return
			}
			j++
		}
		dest = mdUnescapeString(s[start:j])
		j++
	} else {
		start := j
		depth := 0
		for j < len(s) {
			c := s[j]
			if c == '\\' && j+1 < len(s) && isASCIIPunct(s[j+1]) {
				j += 2
				continue
			}
			if c == '(' {
				depth++
			} else if c == ')' {
				if depth == 0 {
					break
				}
				depth--
			} else if c <= ' ' {
				break
			}
			j++
		}
		if j == start || depth != 0 {
			return
		}
		dest = mdUnescapeString(s[start:j])
	}

	// After the destination: optional title, separated by whitespace.
	k := j
	for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
		k++
	}
	sawWS := k > j
	sawNL := false
	destLineEnd := 0
	if k >= len(s) {
		return k, label, dest, "", true
	}
	if s[k] == '\n' {
		sawNL = true
		sawWS = true
		destLineEnd = k + 1
		k++
		for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
			k++
		}
		if k >= len(s) {
			return destLineEnd, label, dest, "", true
		}
	}

	if sawWS && (s[k] == '"' || s[k] == '\'' || s[k] == '(') {
		opener := s[k]
		closer := opener
		if opener == '(' {
			closer = ')'
		}
		m := k + 1
		valid := false
		for m < len(s) {
			c := s[m]
			if c == '\\' && m+1 < len(s) {
				m += 2
				continue
			}
			if c == closer {
				valid = true
				break
			}
			if opener == '(' && c == '(' {
				break
			}
			if c == '\n' && m+1 < len(s) && s[m+1] == '\n' {
				break
			}
			m++
		}
		if valid {
			t := mdUnescapeString(s[k+1 : m])
			m++ // past closer
			e := m
			for e < len(s) && (s[e] == ' ' || s[e] == '\t') {
				e++
			}
			if e >= len(s) {
				return e, label, dest, t, true
			}
			if s[e] == '\n' {
				return e + 1, label, dest, t, true
			}
		}
		// Title didn't work out; the definition is still valid without it
		// when the destination's own line ended cleanly.
		if sawNL {
			return destLineEnd, label, dest, "", true
		}
		return 0, "", "", "", false
	}

	if sawNL {
		return destLineEnd, label, dest, "", true
	}
	return 0, "", "", "", false
}

// mdSkipWSOneNL skips spaces/tabs and at most one newline; a second newline
// (blank line) returns -1.
func mdSkipWSOneNL(s string, i int) int {
	nl := false
	for i < len(s) {
		switch s[i] {
		case ' ', '\t':
			i++
		case '\n':
			if nl {
				return -1
			}
			nl = true
			i++
		default:
			return i
		}
	}
	return i
}

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}

// mdEntityRe matches an HTML entity or numeric character reference with the
// terminating semicolon CommonMark requires (unlike HTML's legacy set).
var mdEntityRe = regexp.MustCompile(`^&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[a-zA-Z][a-zA-Z0-9]{0,47});`)

// mdUnescapeString resolves backslash escapes of ASCII punctuation and
// entity/numeric character references (used for code-fence info strings,
// link destinations and titles).
func mdUnescapeString(s string) string {
	if !strings.ContainsAny(s, "\\&") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			b.WriteByte(s[i+1])
			i++
		case c == '&':
			if m := mdEntityRe.FindString(s[i:]); m != "" {
				if u := html.UnescapeString(m); u != m {
					if u == "\x00" {
						u = "�"
					}
					b.WriteString(u)
					i += len(m) - 1
					continue
				}
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
