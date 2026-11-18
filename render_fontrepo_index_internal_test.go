// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSystemIndexDoesNotRetainFaces pins GitHub #21: indexing the OS font
// directories must keep only the names, not every parsed face. A face is parsed
// again when a document draws with it, a file that is not a font keeps its
// negative entry, and a file parsed before indexing stays cached.
func TestSystemIndexDoesNotRetainFaces(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	font := filepath.Join(dir, "DejaVuSans.ttf")
	early := filepath.Join(dir, "Early.ttf")
	bad := filepath.Join(dir, "NotAFont.ttf")
	for p, b := range map[string][]byte{font: data, early: data, bad: []byte("not a font")} {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	saved := systemFontDirs
	systemFontDirs = func() []string { return []string{dir} }
	t.Cleanup(func() { systemFontDirs = saved })

	r := &fontRepository{parsed: map[string][]*ttfFont{}}
	held := r.loadAll(early) // parsed before indexing, e.g. drawn with already
	if len(held) == 0 {
		t.Fatal("test font did not parse")
	}
	r.ensureSystemIndexed()

	if _, ok := r.parsed[font]; ok {
		t.Error("indexing kept the parsed faces of a font file")
	}
	if c, ok := r.parsed[early]; !ok || len(c) == 0 || c[0] != held[0] {
		t.Error("indexing dropped a face parsed before it")
	}
	if c, ok := r.parsed[bad]; !ok || len(c) != 0 {
		t.Error("indexing dropped the negative entry of a file that is not a font")
	}

	ref, ok := r.sysByFamily["dejavu sans|regular"]
	if !ok {
		t.Fatal("DejaVu Sans missing from the system index")
	}
	f := r.load(ref)
	if f == nil || f.family != held[0].family {
		t.Fatalf("load(%v) = %v, want the DejaVu Sans face parsed again", ref, f)
	}
	if again := r.load(ref); again != f {
		t.Error("a face loaded after indexing is not cached")
	}
}
