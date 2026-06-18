// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"os"
)

// LoadFontByName resolves a font by family name (with an optional bold / italic
// style) through the FontRepository — the folders and files registered with
// AddFontFolder / AddFontFile first, then the operating system's font
// directories — embeds the matched face into the document, and returns a Font
// usable in TextStyle.Font. It is the by-name counterpart to LoadFont(path):
// callers that know a family ("Calibri", "Verdana", "Arial" bold) need not
// locate the file themselves. A sub-font of a .ttc collection is re-wrapped as a
// standalone sfnt before embedding. Standard-14 family names resolve to their
// installed metric-equivalents (Helvetica → Arial, Times → Times New Roman, …);
// for a non-embedded Standard-14 font use the package-level vars (FontHelvetica,
// …) instead. Returns an error if no matching font is registered or installed.
// Mirrors the intent of Aspose.PDF for .NET's FontRepository.FindFont.
func (d *Document) LoadFontByName(family string, bold, italic bool) (Font, error) {
	ref, ok := fontRepo.resolveNamedRef(family, bold, italic)
	if !ok {
		return nil, fmt.Errorf("load font %q: no matching font is registered or installed", family)
	}
	data, err := fontRepo.sfntBytes(ref)
	if err != nil {
		return nil, fmt.Errorf("load font %q: %w", family, err)
	}
	return d.loadFontFromBytes(data)
}

// resolveNamedRef resolves a family name + style to a specific font file (and
// sub-font index) through the registered sources first, then the OS font
// directories. Unlike the render-time substitution helpers — which keep system
// fonts opt-in so a document never silently re-faces — this is an explicit
// by-name request, so the operating-system fonts are always consulted.
func (r *fontRepository) resolveNamedRef(name string, bold, italic bool) (fontRef, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureIndexed()

	norm := normalizeFontName(name)
	style := styleKey(bold, italic, norm)
	// Try the literal requested family first, then the Standard-14 / bundled
	// expansions. candidateFamilies maps anything containing "sans" to the
	// Arial family, so without the literal entry "DejaVu Sans" would wrongly
	// resolve to Arial; an explicit by-name request must prefer the real font.
	fams := append([]string{strippedFamily(norm)}, candidateFamilies(name)...)

	// Registered sources: exact PostScript name, then family at the requested
	// style, then the family's regular weight.
	if ref, ok := r.byName[norm]; ok {
		return ref, true
	}
	for _, fam := range fams {
		if ref, ok := r.byFamily[fam+"|"+style]; ok {
			return ref, true
		}
	}
	for _, fam := range fams {
		if ref, ok := r.byFamily[fam+"|regular"]; ok {
			return ref, true
		}
	}

	// Operating-system fonts.
	r.ensureSystemIndexed()
	if ref, ok := r.sysByName[norm]; ok {
		return ref, true
	}
	if ck := compactFontName(strippedFamily(norm)); ck != "" {
		if ref, ok := r.sysByCompact[ck]; ok {
			return ref, true
		}
	}
	for _, fam := range fams {
		if ref, ok := r.sysByFamily[fam+"|"+style]; ok {
			return ref, true
		}
	}
	for _, fam := range fams {
		if ref, ok := r.sysByFamily[fam+"|regular"]; ok {
			return ref, true
		}
	}
	return fontRef{}, false
}

// sfntBytes returns a standalone sfnt program for the font a ref points at: the
// file bytes verbatim for a single-font .ttf/.otf, or — for one sub-font of a
// .ttc collection — that face re-wrapped as a standalone sfnt (a collection
// cannot be embedded as a single /FontFile2).
func (r *fontRepository) sfntBytes(ref fontRef) ([]byte, error) {
	r.mu.Lock()
	fonts := r.loadAll(ref.path)
	r.mu.Unlock()
	if ref.index < 0 || ref.index >= len(fonts) {
		return nil, fmt.Errorf("sub-font index %d out of range", ref.index)
	}
	f := fonts[ref.index]
	if len(fonts) == 1 {
		// Single-font file: f.data is already a standalone sfnt.
		if len(f.data) > 0 {
			return f.data, nil
		}
		return os.ReadFile(ref.path)
	}
	return standaloneSFNT(f)
}

// standaloneSFNT re-wraps one TrueType sub-font of a collection as a standalone
// sfnt, copying its table directory into a fresh single-font header (a .ttc's
// sub-fonts share table data through file-absolute offsets, so they cannot be
// embedded directly). OpenType-CFF sub-fonts (no glyf table) are not handled —
// a single-font .otf still embeds via the normal /FontFile3 path.
func standaloneSFNT(f *ttfFont) ([]byte, error) {
	if _, ok := f.tables["glyf"]; !ok {
		return nil, fmt.Errorf("only TrueType (glyf) collection sub-fonts can be embedded")
	}
	tbls := map[string][]byte{}
	for tag := range f.tables {
		if b := tableSlice(f.data, f.tables, tag); len(b) > 0 {
			tbls[tag] = b
		}
	}
	if len(tbls) == 0 {
		return nil, fmt.Errorf("sub-font table directory is empty")
	}
	return assembleSFNT(tbls), nil
}
