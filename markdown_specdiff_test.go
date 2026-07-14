// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"strings"
	"testing"
)

// TestCommonMarkSpecDiff prints got/want diffs for failing examples of the
// sections in MD_SPEC_SECTIONS (comma-separated). Debug aid; skipped unless
// the env var is set.
func TestCommonMarkSpecDiff(t *testing.T) {
	sectionsEnv := os.Getenv("MD_SPEC_SECTIONS")
	if sectionsEnv == "" {
		t.Skip("set MD_SPEC_SECTIONS to run")
	}
	want := map[string]bool{}
	for _, s := range strings.Split(sectionsEnv, ",") {
		want[strings.TrimSpace(s)] = true
	}
	max := 12
	for _, ex := range loadSpecExamples(t) {
		if !want[ex.Section] {
			continue
		}
		got := mdTestHTML(parseMarkdownCore(ex.Markdown))
		if got == ex.HTML {
			continue
		}
		if max--; max < 0 {
			t.Log("... more failures suppressed")
			break
		}
		t.Logf("== example %d (%s) ==\nMARKDOWN: %q\nWANT: %q\nGOT:  %q", ex.Example, ex.Section, ex.Markdown, ex.HTML, got)
	}
}
