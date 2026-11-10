// SPDX-License-Identifier: MIT

package asposepdf

// PageMode is how a viewer presents the document when it opens: which panel,
// if any, is shown next to the pages (the catalog /PageMode, ISO 32000-1
// Table 28). Mirrors Aspose.PDF for .NET's PageMode.
type PageMode int

const (
	PageModeUseNone        PageMode = iota // neither outline nor thumbnails (the default)
	PageModeUseOutlines                    // the bookmarks panel
	PageModeUseThumbs                      // the page thumbnails panel
	PageModeFullScreen                     // full-screen mode, no menu bar or controls
	PageModeUseOC                          // the optional content (layers) panel
	PageModeUseAttachments                 // the attachments panel
)

var pageModeNames = [...]string{"/UseNone", "/UseOutlines", "/UseThumbs", "/FullScreen", "/UseOC", "/UseAttachments"}

// PageMode returns how a viewer shows the document when it opens;
// PageModeUseNone when the document does not say, or names a mode PDF does not
// define. Mirrors Aspose.PDF for .NET's Document.PageMode.
func (d *Document) PageMode() PageMode {
	n, _ := d.catalog["/PageMode"].(pdfName)
	for i, s := range pageModeNames {
		if string(n) == s {
			return PageMode(i)
		}
	}
	return PageModeUseNone
}

// SetPageMode sets how a viewer shows the document when it opens.
// PageModeUseNone, the default, removes the entry; a value outside the
// constants is treated as PageModeUseNone.
func (d *Document) SetPageMode(m PageMode) {
	if m <= PageModeUseNone || int(m) >= len(pageModeNames) {
		delete(d.catalog, "/PageMode")
		return
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	d.catalog["/PageMode"] = pdfName(pageModeNames[m])
}
