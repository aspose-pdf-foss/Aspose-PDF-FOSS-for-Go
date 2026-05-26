// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

type cssSelectorKind int

const (
	cssSelClass   cssSelectorKind = iota // .foo
	cssSelID                              // #foo
	cssSelElement                         // foo
)

type cssSelector struct {
	kind cssSelectorKind
	name string
}

type cssRule struct {
	selectors  []cssSelector
	properties map[string]string
}

// parseSVGCSS parses an SVG <style> block body into a list of rules.
// Best-effort: malformed rules are silently dropped. Comments (/* ... */) are stripped.
func parseSVGCSS(s string) []cssRule {
	var rules []cssRule
	// Strip /* ... */ comments
	for {
		start := strings.Index(s, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "*/")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+end+2:]
	}
	for len(s) > 0 {
		open := strings.IndexByte(s, '{')
		closeIdx := strings.IndexByte(s, '}')
		if open < 0 || closeIdx < 0 || open >= closeIdx {
			break
		}
		selectorList := strings.TrimSpace(s[:open])
		body := s[open+1 : closeIdx]
		s = s[closeIdx+1:]
		var sels []cssSelector
		for _, sel := range strings.Split(selectorList, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			switch {
			case strings.HasPrefix(sel, "."):
				sels = append(sels, cssSelector{cssSelClass, sel[1:]})
			case strings.HasPrefix(sel, "#"):
				sels = append(sels, cssSelector{cssSelID, sel[1:]})
			default:
				sels = append(sels, cssSelector{cssSelElement, sel})
			}
		}
		if len(sels) == 0 {
			continue
		}
		props := map[string]string{}
		for _, decl := range strings.Split(body, ";") {
			kv := strings.SplitN(decl, ":", 2)
			if len(kv) != 2 {
				continue
			}
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k != "" {
				props[k] = v
			}
		}
		rules = append(rules, cssRule{selectors: sels, properties: props})
	}
	return rules
}
