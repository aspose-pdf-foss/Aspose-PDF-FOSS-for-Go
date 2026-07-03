// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// FontRepository resolves fonts for rendering from caller-registered folders
// and files, and optionally the operating system's font directories. It lets a
// document render Standard-14 (and other non-embedded) text with exact fonts
// instead of the bundled metric-compatible substitutes. Mirrors Aspose.PDF for
// .NET's FontRepository / FontRepository.Sources.
//
// Use the package-level AddFontFolder / AddFontFile / AddSystemFonts. The
// registry is process-global (fonts are a machine resource) and indexed lazily
// on first use. Only TrueType (.ttf) outline fonts are used by the renderer.
type fontRepository struct {
	mu      sync.Mutex
	folders []string
	files   []string
	indexed bool

	// index: "family|style" and exact PostScript name (both lowercased) → the
	// specific sub-font that satisfies them. A path may hold several fonts (a
	// .ttc collection), so the value is a (path, index) reference, not a path.
	byFamily map[string]fontRef
	byName   map[string]fontRef
	parsed   map[string][]*ttfFont // all sub-fonts of a file, parsed once

	// Separate, lazily-built index of the OS font directories, consulted only
	// for non-embedded CJK fonts (findCJK). Kept apart from the user-registered
	// index so Latin substitution stays opt-in (a document must not silently
	// switch to a system Arial just because it was installed), while CJK — which
	// has no bundled substitute — still resolves out of the box.
	sysIndexed   bool
	sysByFamily  map[string]fontRef
	sysByName    map[string]fontRef
	sysByCompact map[string]fontRef // separator-stripped PS name / family → ref
}

// fontRef points at one sub-font within a font file (index 0 for a plain .ttf).
type fontRef struct {
	path  string
	index int
}

var fontRepo = &fontRepository{}

// AddFontFolder registers a directory searched (recursively) for fonts when
// rendering. Mirrors adding a FolderFontSource in Aspose.PDF for .NET.
func AddFontFolder(path string) {
	fontRepo.mu.Lock()
	defer fontRepo.mu.Unlock()
	fontRepo.folders = append(fontRepo.folders, path)
	fontRepo.indexed = false
}

// AddFontFile registers a single font file. Mirrors a FileFontSource.
func AddFontFile(path string) {
	fontRepo.mu.Lock()
	defer fontRepo.mu.Unlock()
	fontRepo.files = append(fontRepo.files, path)
	fontRepo.indexed = false
}

// AddSystemFonts registers the operating system's standard font directories.
// Mirrors adding a SystemFontSource. Not enabled by default — call it to let
// the renderer use installed fonts (e.g. real Arial / Times New Roman).
func AddSystemFonts() {
	for _, d := range systemFontDirs() {
		AddFontFolder(d)
	}
}

// ClearFontSources removes all registered folders and files.
func ClearFontSources() {
	fontRepo.mu.Lock()
	defer fontRepo.mu.Unlock()
	fontRepo.folders, fontRepo.files, fontRepo.indexed = nil, nil, false
}

// find returns a registered font matching fi, or nil if none is registered.
// Resolution order: exact PostScript name, then the requested family (plus
// Standard-14 aliases) at the requested style, then that family's regular
// weight. Matching is by the font's real name-table family — not a coarse
// serif/mono/sans guess — so a request for Helvetica never resolves to an
// unrelated serif or CJK face; when nothing matches we return nil and the
// caller falls back to the bundled metric-compatible substitute.
func (r *fontRepository) find(fi fontInfo) *ttfFont {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.folders) == 0 && len(r.files) == 0 {
		return nil
	}
	r.ensureIndexed()

	if ref, ok := r.byName[normalizeFontName(fi.name)]; ok {
		return r.load(ref)
	}
	style := styleKey(fi.bold, fi.italic, strings.ToLower(fi.name))
	families := candidateFamilies(fi.name)
	for _, fam := range families {
		if ref, ok := r.byFamily[fam+"|"+style]; ok {
			return r.load(ref)
		}
	}
	// Same family, regular weight, when the exact style isn't installed.
	for _, fam := range families {
		if ref, ok := r.byFamily[fam+"|regular"]; ok {
			return r.load(ref)
		}
	}
	return nil
}

// findCJK resolves a non-embedded composite CJK font to a real outline font:
// first any caller-registered font (respecting the opt-in registry), then the
// operating system's fonts matched by the requested family (e.g. SimSun →
// simsun.ttc) or, failing that, a well-known default family for the Adobe
// ordering. Returns nil if no covering font is installed.
func (r *fontRepository) findCJK(fi fontInfo, ordering string) *ttfFont {
	// Prefer the document's own font when installed (exact name / compact match),
	// so a Yu Gothic Medium resolves to that exact face — not just any Yu Gothic —
	// giving the right glyph variant and units-per-em.
	if f := r.findSystemExact(fi); f != nil {
		return f
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureSystemIndexed()

	style := styleKey(fi.bold, fi.italic, strings.ToLower(fi.name))
	fams := append(candidateFamilies(fi.name), cjkOrderingFamilies(ordering)...)
	for _, fam := range fams {
		if ref, ok := r.sysByFamily[fam+"|"+style]; ok {
			return r.load(ref)
		}
	}
	for _, fam := range fams {
		if ref, ok := r.sysByFamily[fam+"|regular"]; ok {
			return r.load(ref)
		}
	}
	// Fuzzy pass: a candidate family may be installed under a longer name —
	// "Microsoft JhengHei" ships as "Microsoft JhengHei UI" on modern Windows.
	// Accept an indexed family that has the candidate as a word-boundary prefix.
	// This also sidesteps the "-ExtB" extension faces (mingliub.ttc), whose
	// family names use a '-' separator and which carry only rare CJK-Extension-B
	// glyphs — matching one would render common characters blank.
	for _, want := range []string{style, "regular"} {
		for _, fam := range fams {
			if ref, ok := r.sysFamilyPrefix(fam, want); ok {
				return r.load(ref)
			}
		}
	}
	return nil
}

// sysFamilyPrefix finds an indexed system font whose family name equals fam or
// begins with "fam " (a word-boundary prefix) and whose style matches. Families
// carrying a CJK extension block ("ext-a"/"ext-b"/…) are skipped — they hold
// only rare supplementary ideographs, not the common ones. Caller holds r.mu.
func (r *fontRepository) sysFamilyPrefix(fam, style string) (fontRef, bool) {
	suffix := "|" + style
	for key, ref := range r.sysByFamily {
		name, ok := strings.CutSuffix(key, suffix)
		if !ok || !strings.HasPrefix(name, fam+" ") {
			continue
		}
		if strings.Contains(name, "ext-") || strings.Contains(name, "extb") ||
			strings.Contains(name, "exta") || strings.Contains(name, "extg") {
			continue
		}
		return ref, true
	}
	return fontRef{}, false
}

// findSystemStd14 resolves a Standard-14-family font (Courier/Helvetica/Times
// and their styles) to the installed metric-equivalent face — Courier New,
// Arial, Times New Roman — the same substitutes Acrobat uses. Only consulted
// for Standard-14 aliases, so arbitrary document fonts still go through the
// opt-in registry and the bundled metric-compatible substitutes.
func (r *fontRepository) findSystemStd14(fi fontInfo) *ttfFont {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureSystemIndexed()
	style := styleKey(fi.bold, fi.italic, strings.ToLower(fi.name))
	for _, fam := range candidateFamilies(fi.name) {
		if ref, ok := r.sysByFamily[fam+"|"+style]; ok {
			return r.load(ref)
		}
	}
	return nil
}

// isStd14Alias reports whether a font name belongs to one of the three
// Latin Standard-14 families (by the usual aliases).
func isStd14Alias(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"courier", "helvetica", "arial", "times"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// findSystemExact resolves a font to an installed face by an exact name match —
// PostScript name, then a separator-stripped ("compact") PostScript-or-family
// match (so a PDF BaseFont "YuGothicMedium" finds the system "YuGothic-Medium" /
// "Yu Gothic Medium"). It never falls back to a coarse family/substitute, so it
// is safe for non-embedded Type0/Identity-H fonts: the document's CIDs are the
// original font's glyph IDs, so the real installed font renders them correctly
// (Identity GID = CID), while an unrelated face is never substituted in.
func (r *fontRepository) findSystemExact(fi fontInfo) *ttfFont {
	if f := r.find(fi); f != nil {
		return f
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureSystemIndexed()
	if ref, ok := r.sysByName[normalizeFontName(fi.name)]; ok {
		return r.load(ref)
	}
	if ck := compactFontName(strippedFamily(normalizeFontName(fi.name))); ck != "" {
		if ref, ok := r.sysByCompact[ck]; ok {
			return r.load(ref)
		}
	}
	return nil
}

// compactFontName lowercases and strips every non-alphanumeric character, so
// "YuGothic-Medium", "Yu Gothic Medium" and "YuGothicMedium" all collapse to
// the same key.
func compactFontName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ensureSystemIndexed indexes the OS font directories into the sys* maps once.
func (r *fontRepository) ensureSystemIndexed() {
	if r.sysIndexed {
		return
	}
	r.sysByFamily = map[string]fontRef{}
	r.sysByName = map[string]fontRef{}
	r.sysByCompact = map[string]fontRef{}
	if r.parsed == nil {
		r.parsed = map[string][]*ttfFont{}
	}
	var files []string
	for _, d := range systemFontDirs() {
		_ = filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && isFontFile(p) {
				files = append(files, p)
			}
			return nil
		})
	}
	for _, p := range files {
		for i, f := range r.loadAll(p) {
			ref := fontRef{path: p, index: i}
			if ps := strings.ToLower(f.postScriptName); ps != "" {
				if _, exists := r.sysByName[ps]; !exists {
					r.sysByName[ps] = ref
				}
				if ck := compactFontName(ps); ck != "" {
					if _, exists := r.sysByCompact[ck]; !exists {
						r.sysByCompact[ck] = ref
					}
				}
			}
			fam := strings.ToLower(strings.TrimSpace(f.family))
			if fam == "" {
				continue
			}
			sub := strings.ToLower(f.subfamily)
			style := styleKey(f.flagsBold || strings.Contains(sub, "bold"),
				f.flagsItalic || strings.Contains(sub, "italic") || strings.Contains(sub, "oblique"), "")
			if _, exists := r.sysByFamily[fam+"|"+style]; !exists {
				r.sysByFamily[fam+"|"+style] = ref
			}
			// Compact family+subfamily key (e.g. "yu gothic"+"medium" →
			// "yugothicmedium") so a PDF BaseFont "YuGothicMedium" resolves even
			// when the PostScript name differs.
			if ck := compactFontName(fam + sub); ck != "" {
				if _, exists := r.sysByCompact[ck]; !exists {
					r.sysByCompact[ck] = ref
				}
			}
		}
	}
	r.sysIndexed = true
}

// cjkOrderingFamilies returns well-known installed font families covering an
// Adobe CID ordering, tried when the document's own BaseFont isn't installed.
func cjkOrderingFamilies(ordering string) []string {
	switch ordering {
	case "GB1": // Simplified Chinese
		return []string{"simsun", "nsimsun", "microsoft yahei", "simhei", "kaiti", "fangsong", "dengxian"}
	case "CNS1": // Traditional Chinese
		return []string{"pmingliu", "mingliu", "microsoft jhenghei", "dfkai-sb", "kaiti tc"}
	case "Japan1": // Japanese
		return []string{"ms mincho", "ms pmincho", "ms gothic", "ms pgothic", "yu mincho", "yu gothic", "meiryo"}
	case "Korea1", "KR": // Korean
		return []string{"batang", "gulim", "malgun gothic", "dotum", "gungsuh"}
	}
	return nil
}

// isCJKFamily reports whether a font's BaseFont names a well-known CJK family.
// Used to route a non-embedded *simple* font (e.g. a SimSun WinAnsi face) to a
// system CJK font for its Latin glyphs too, instead of a bundled Latin
// substitute — so it matches its composite (Type0) sibling of the same font.
func isCJKFamily(name string) bool {
	n := strippedFamily(normalizeFontName(name))
	for _, ord := range []string{"GB1", "CNS1", "Japan1", "Korea1"} {
		for _, f := range cjkOrderingFamilies(ord) {
			if n == f {
				return true
			}
		}
	}
	// Unambiguous CJK tokens (also catch suffixed names like "SimSun-ExtB").
	for _, k := range []string{"simsun", "nsimsun", "simhei", "kaiti", "fangsong", "mincho", "mingliu", "batang", "gulim", "gungsuh", "dotum", "malgun", "meiryo", "yahei", "jhenghei", "songti", "heiti", "dfkai", "msgothic", "ms gothic", "pgothic"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

func (r *fontRepository) ensureIndexed() {
	if r.indexed {
		return
	}
	r.byFamily = map[string]fontRef{}
	r.byName = map[string]fontRef{}
	if r.parsed == nil {
		r.parsed = map[string][]*ttfFont{}
	}

	var files []string
	for _, d := range r.folders {
		_ = filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && isFontFile(p) {
				files = append(files, p)
			}
			return nil
		})
	}
	files = append(files, r.files...)

	for _, p := range files {
		for i, f := range r.loadAll(p) {
			ref := fontRef{path: p, index: i}
			if ps := strings.ToLower(f.postScriptName); ps != "" {
				if _, exists := r.byName[ps]; !exists {
					r.byName[ps] = ref
				}
			}
			fam := strings.ToLower(strings.TrimSpace(f.family))
			if fam == "" {
				continue
			}
			sub := strings.ToLower(f.subfamily)
			style := styleKey(f.flagsBold || strings.Contains(sub, "bold"),
				f.flagsItalic || strings.Contains(sub, "italic") || strings.Contains(sub, "oblique"), "")
			key := fam + "|" + style
			if _, exists := r.byFamily[key]; !exists {
				r.byFamily[key] = ref
			}
		}
	}
	r.indexed = true
}

// isFontFile reports whether path has a font extension the renderer can use.
func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".ttc", ".otf":
		return true
	}
	return false
}

// loadAll parses (once, cached) every sub-font of a file. A plain .ttf yields
// one element; a .ttc collection yields one per TrueType sub-font.
func (r *fontRepository) loadAll(path string) []*ttfFont {
	if c, ok := r.parsed[path]; ok {
		return c
	}
	var fonts []*ttfFont
	if data, err := os.ReadFile(path); err == nil {
		if fs, err := parseFontCollection(data); err == nil {
			fonts = fs
		}
	}
	r.parsed[path] = fonts // cache nil/empty too, so a bad file isn't re-read
	return fonts
}

// load returns the specific sub-font a fontRef points at, or nil.
func (r *fontRepository) load(ref fontRef) *ttfFont {
	c := r.loadAll(ref.path)
	if ref.index >= 0 && ref.index < len(c) {
		return c[ref.index]
	}
	return nil
}

// normalizeFontName strips the leading slash and subset prefix and lowercases.
func normalizeFontName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if len(name) > 7 && name[6] == '+' {
		name = name[7:]
	}
	return strings.ToLower(name)
}

// candidateFamilies maps a requested font name to the family names (lowercased,
// as they appear in a font's name table) that may satisfy it, most-specific
// first. For the Standard-14 families it expands to the real installed faces and
// the bundled substitutes (e.g. Helvetica → Arial, Helvetica, Arimo). For any
// other name it returns the requested family stripped of its style suffix, so a
// custom registered font still matches by its own name.
func candidateFamilies(name string) []string {
	n := normalizeFontName(name)
	switch {
	case strings.Contains(n, "times") || strings.Contains(n, "georgia") || (strings.Contains(n, "roman") && !strings.Contains(n, "cascadia")) || strings.Contains(n, "serif") && !strings.Contains(n, "sans"):
		return []string{"times new roman", "times", "tinos", "liberation serif"}
	case strings.Contains(n, "courier") || strings.Contains(n, "mono") || strings.Contains(n, "consol"):
		return []string{"courier new", "courier", "cousine", "liberation mono", "consolas"}
	case strings.Contains(n, "helvetica") || strings.Contains(n, "arial") || strings.Contains(n, "sans"):
		return []string{"arial", "helvetica", "arimo", "liberation sans"}
	default:
		return []string{strippedFamily(n)}
	}
}

// strippedFamily removes common style suffixes from an already-normalized font
// name so a custom font like "Calibri-Bold" matches the family "calibri".
func strippedFamily(n string) string {
	if i := strings.IndexByte(n, ','); i >= 0 {
		n = n[:i]
	}
	for _, tok := range []string{"-boldoblique", "-boldobl", "-bolditalic", "-boldital", "-bold", "-oblique", "-italic", "-ital", "-regular", "-roman", "boldoblique", "bolditalic", "psmt"} {
		n = strings.ReplaceAll(n, tok, "")
	}
	return strings.TrimRight(strings.TrimSpace(n), "-_ ")
}

func styleKey(bold, italic bool, name string) string {
	bold = bold || strings.Contains(name, "bold")
	italic = italic || strings.Contains(name, "italic") || strings.Contains(name, "oblique")
	switch {
	case bold && italic:
		return "bolditalic"
	case bold:
		return "bold"
	case italic:
		return "italic"
	default:
		return "regular"
	}
}

// systemFontDirs returns the OS font directories for the current platform.
func systemFontDirs() []string {
	switch runtime.GOOS {
	case "windows":
		var dirs []string
		if w := os.Getenv("WINDIR"); w != "" {
			dirs = append(dirs, filepath.Join(w, "Fonts"))
		}
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			dirs = append(dirs, filepath.Join(la, "Microsoft", "Windows", "Fonts"))
		}
		return dirs
	case "darwin":
		home, _ := os.UserHomeDir()
		return []string{"/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library", "Fonts")}
	default: // linux and other unix
		home, _ := os.UserHomeDir()
		return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".fonts"), filepath.Join(home, ".local", "share", "fonts")}
	}
}
