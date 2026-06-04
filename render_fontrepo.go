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

	// index: family bucket ("sans"/"serif"/"mono") + style → path, and exact
	// PostScript name (lowercased) → path.
	byBucket map[string]string
	byName   map[string]string
	parsed   map[string]*ttfFont
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

// find returns a registered font matching fi (by exact PostScript name, then by
// family bucket + style), or nil if none is registered.
func (r *fontRepository) find(fi fontInfo) *ttfFont {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.folders) == 0 && len(r.files) == 0 {
		return nil
	}
	r.ensureIndexed()

	name := normalizeFontName(fi.name)
	if p, ok := r.byName[name]; ok {
		return r.load(p)
	}
	key := familyBucket(fi.name) + ":" + styleKey(fi.bold, fi.italic, name)
	if p, ok := r.byBucket[key]; ok {
		return r.load(p)
	}
	// Fall back to the regular weight of the bucket.
	if p, ok := r.byBucket[familyBucket(fi.name)+":regular"]; ok {
		return r.load(p)
	}
	return nil
}

func (r *fontRepository) ensureIndexed() {
	if r.indexed {
		return
	}
	r.byBucket = map[string]string{}
	r.byName = map[string]string{}
	if r.parsed == nil {
		r.parsed = map[string]*ttfFont{}
	}

	var files []string
	for _, d := range r.folders {
		_ = filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.EqualFold(filepath.Ext(p), ".ttf") {
				files = append(files, p)
			}
			return nil
		})
	}
	files = append(files, r.files...)

	for _, p := range files {
		f := r.load(p)
		if f == nil {
			continue
		}
		ps := strings.ToLower(f.postScriptName)
		if ps != "" {
			if _, exists := r.byName[ps]; !exists {
				r.byName[ps] = p
			}
		}
		key := bucketOfFont(f) + ":" + styleKey(f.flagsBold, f.flagsItalic, ps)
		if _, exists := r.byBucket[key]; !exists {
			r.byBucket[key] = p
		}
	}
	r.indexed = true
}

func (r *fontRepository) load(path string) *ttfFont {
	if f, ok := r.parsed[path]; ok {
		return f
	}
	var parsed *ttfFont
	if data, err := os.ReadFile(path); err == nil {
		if f, err := parseTTF(data); err == nil {
			parsed = f
		}
	}
	r.parsed[path] = parsed
	return parsed
}

// normalizeFontName strips the leading slash and subset prefix and lowercases.
func normalizeFontName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if len(name) > 7 && name[6] == '+' {
		name = name[7:]
	}
	return strings.ToLower(name)
}

// familyBucket classifies a font name as serif / mono / sans.
func familyBucket(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "times") || strings.Contains(n, "serif") || strings.Contains(n, "roman") || strings.Contains(n, "georgia") || strings.Contains(n, "tinos"):
		return "serif"
	case strings.Contains(n, "courier") || strings.Contains(n, "mono") || strings.Contains(n, "consol") || strings.Contains(n, "cousine"):
		return "mono"
	default:
		return "sans"
	}
}

func bucketOfFont(f *ttfFont) string {
	b := familyBucket(f.postScriptName)
	if b == "sans" && f.isFixedPitch {
		return "mono"
	}
	return b
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
