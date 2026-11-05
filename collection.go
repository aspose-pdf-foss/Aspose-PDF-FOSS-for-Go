// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strings"
	"time"
)

// PDF portfolios / collections (ISO 32000-1 §7.11.6 + §12.3.5): a portfolio
// presents the document's embedded files as a navigable set with a viewer-side
// table of custom columns, rather than as a plain attachment list. The
// container is the catalog's /Collection dictionary; the columns are a
// /Schema of collection fields; each attachment carries the values for those
// fields in its file specification's /CI collection item.
//
//	col := doc.Collection()
//	col.SetView(pdf.CollectionViewDetails)
//	col.Schema().Add("Invoice", "Invoice #", pdf.CollectionFieldText)
//	col.Schema().Add("Amount", "Total", pdf.CollectionFieldNumber)
//	f, _ := doc.EmbeddedFiles().Add("invoice-042.pdf")
//	f.CollectionItem().SetText("Invoice", "INV-042")
//	f.CollectionItem().SetNumber("Amount", 1499.50)
//	col.SetSort("Amount", false)
//
// Mirrors Aspose.PDF for .NET's Document.Collection / CollectionSchema /
// CollectionField. The document stays an ordinary PDF: viewers that do not
// implement portfolios show the cover page and the attachments as usual.

// CollectionView selects how a viewer presents the portfolio's files
// (the /Collection /View entry).
type CollectionView int

const (
	// CollectionViewDetails lists the files in a table of schema columns (/D).
	CollectionViewDetails CollectionView = iota
	// CollectionViewTiles shows one tile per file (/T).
	CollectionViewTiles
	// CollectionViewHidden hides the portfolio pane, showing the cover
	// page only (/H).
	CollectionViewHidden
)

func (v CollectionView) pdfName() pdfName {
	switch v {
	case CollectionViewTiles:
		return "/T"
	case CollectionViewHidden:
		return "/H"
	default:
		return "/D"
	}
}

func collectionViewFromName(n string) CollectionView {
	switch n {
	case "/T":
		return CollectionViewTiles
	case "/H":
		return CollectionViewHidden
	default:
		return CollectionViewDetails
	}
}

// CollectionFieldType is a schema column's data type (the collection field's
// /Subtype). The last five are derived fields: their values come from the file
// specification itself, so they need no per-file collection-item entry.
type CollectionFieldType int

const (
	// CollectionFieldText holds a text value (/S).
	CollectionFieldText CollectionFieldType = iota
	// CollectionFieldDate holds a date value (/D).
	CollectionFieldDate
	// CollectionFieldNumber holds a numeric value (/N).
	CollectionFieldNumber
	// CollectionFieldFilename shows the attachment's file name (/F).
	CollectionFieldFilename
	// CollectionFieldDescription shows the attachment's description (/Desc).
	CollectionFieldDescription
	// CollectionFieldModDate shows the embedded file's modification date
	// (/ModDate).
	CollectionFieldModDate
	// CollectionFieldCreationDate shows the embedded file's creation date
	// (/CreationDate).
	CollectionFieldCreationDate
	// CollectionFieldSize shows the embedded file's size in bytes (/Size).
	CollectionFieldSize
)

var collectionFieldNames = map[CollectionFieldType]string{
	CollectionFieldText:         "/S",
	CollectionFieldDate:         "/D",
	CollectionFieldNumber:       "/N",
	CollectionFieldFilename:     "/F",
	CollectionFieldDescription:  "/Desc",
	CollectionFieldModDate:      "/ModDate",
	CollectionFieldCreationDate: "/CreationDate",
	CollectionFieldSize:         "/Size",
}

func collectionFieldTypeFromName(n string) CollectionFieldType {
	for t, name := range collectionFieldNames {
		if name == n {
			return t
		}
	}
	return CollectionFieldText
}

// derived reports whether the field's value comes from the file specification
// rather than from a collection item.
func (t CollectionFieldType) derived() bool {
	switch t {
	case CollectionFieldFilename, CollectionFieldDescription,
		CollectionFieldModDate, CollectionFieldCreationDate, CollectionFieldSize:
		return true
	}
	return false
}

// Collection is the document's portfolio view over its embedded files.
// Always non-nil; IsPortfolio reports whether the document actually carries a
// /Collection dictionary. Mutating calls create it.
type Collection struct {
	doc *Document
}

// Collection returns the document's portfolio facade.
func (d *Document) Collection() *Collection { return &Collection{doc: d} }

// dict returns the catalog's /Collection dictionary, or nil when the document
// is not a portfolio.
func (c *Collection) dict() pdfDict {
	if c.doc == nil || c.doc.catalog == nil {
		return nil
	}
	d, ok := resolveRefToDict(c.doc.objects, c.doc.catalog["/Collection"])
	if !ok {
		return nil
	}
	return d
}

// ensure returns the /Collection dictionary, creating it (as a details-view
// portfolio) when the document does not have one yet. The catalog itself is
// materialized on demand, as elsewhere (a freshly built document has none
// until a catalog-level feature is used).
func (c *Collection) ensure() pdfDict {
	if d := c.dict(); d != nil {
		return d
	}
	if c.doc.catalog == nil {
		c.doc.catalog = pdfDict{}
	}
	d := pdfDict{"/Type": pdfName("/Collection")}
	c.doc.catalog["/Collection"] = d
	return d
}

// IsPortfolio reports whether the document presents its attachments as a
// portfolio.
func (c *Collection) IsPortfolio() bool { return c.dict() != nil }

// Files returns the portfolio's attachments — the document's embedded files,
// which a portfolio is a presentation layer over.
func (c *Collection) Files() []*EmbeddedFile { return c.doc.EmbeddedFiles().All() }

// View returns the presentation mode; CollectionViewDetails when unset.
func (c *Collection) View() CollectionView {
	d := c.dict()
	if d == nil {
		return CollectionViewDetails
	}
	return collectionViewFromName(dictGetName(d, "/View"))
}

// SetView sets the presentation mode, turning the document into a portfolio
// if it is not one already.
func (c *Collection) SetView(v CollectionView) {
	c.ensure()["/View"] = v.pdfName()
}

// InitialFile returns the name of the attachment a viewer opens first, or ""
// when unset.
func (c *Collection) InitialFile() string {
	d := c.dict()
	if d == nil {
		return ""
	}
	return decodeFormString(d["/D"])
}

// SetInitialFile names the attachment a viewer should present first; "" clears
// the entry. The name must match an embedded file's name-tree key.
func (c *Collection) SetInitialFile(name string) {
	d := c.ensure()
	if name == "" {
		delete(d, "/D")
		return
	}
	d["/D"] = encodeFormString(name)
}

// Sort returns the field the viewer sorts by and its direction; ok is false
// when no sort is configured.
func (c *Collection) Sort() (field string, ascending bool, ok bool) {
	d := c.dict()
	if d == nil {
		return "", false, false
	}
	sd, isDict := resolveRefToDict(c.doc.objects, d["/Sort"])
	if !isDict {
		return "", false, false
	}
	key := ""
	switch s := resolveRef(c.doc.objects, sd["/S"]).(type) {
	case pdfName:
		key = collectionKeyFromName(string(s))
	case pdfArray:
		if len(s) > 0 {
			if n, isName := resolveRef(c.doc.objects, s[0]).(pdfName); isName {
				key = collectionKeyFromName(string(n))
			}
		}
	}
	if key == "" {
		return "", false, false
	}
	asc := true
	switch a := resolveRef(c.doc.objects, sd["/A"]).(type) {
	case bool:
		asc = a
	case pdfArray:
		if len(a) > 0 {
			if b, isBool := resolveRef(c.doc.objects, a[0]).(bool); isBool {
				asc = b
			}
		}
	}
	return key, asc, true
}

// SetSort makes the viewer sort the file list by the named schema field;
// an empty field clears the sort.
func (c *Collection) SetSort(field string, ascending bool) {
	d := c.ensure()
	if field == "" {
		delete(d, "/Sort")
		return
	}
	d["/Sort"] = pdfDict{
		"/S": pdfName(collectionFieldKey(field)),
		"/A": ascending,
	}
}

// Schema returns the portfolio's column schema.
func (c *Collection) Schema() *CollectionSchema { return &CollectionSchema{col: c} }

// Remove drops the /Collection dictionary: the attachments stay, presented as
// an ordinary attachment list again.
func (c *Collection) Remove() {
	if c.doc != nil && c.doc.catalog != nil {
		delete(c.doc.catalog, "/Collection")
	}
}

// collectionFieldKey turns a user-facing field key into a PDF name (with the
// leading slash), accepting either form.
func collectionFieldKey(key string) string {
	return "/" + strings.TrimPrefix(key, "/") // the writer escapes what a name needs
}

// collectionKeyFromName is the inverse: a PDF name to the user-facing key.
func collectionKeyFromName(name string) string {
	return unescapePDFName(strings.TrimPrefix(name, "/"))
}

// CollectionSchema is the portfolio's set of columns.
type CollectionSchema struct {
	col *Collection
}

// dict returns the /Collection /Schema dictionary, or nil when absent.
func (s *CollectionSchema) dict() pdfDict {
	cd := s.col.dict()
	if cd == nil {
		return nil
	}
	d, ok := resolveRefToDict(s.col.doc.objects, cd["/Schema"])
	if !ok {
		return nil
	}
	return d
}

func (s *CollectionSchema) ensure() pdfDict {
	if d := s.dict(); d != nil {
		return d
	}
	d := pdfDict{"/Type": pdfName("/CollectionSchema")}
	s.col.ensure()["/Schema"] = d
	return d
}

// Add defines a column: key identifies the field in collection items,
// displayName is the header a viewer shows, and typ is the value type.
// Re-adding an existing key replaces it. The new field is visible, not
// editable, and ordered after the existing ones.
func (s *CollectionSchema) Add(key, displayName string, typ CollectionFieldType) *CollectionField {
	sd := s.ensure()
	order := 0
	for _, f := range s.Fields() {
		if o := f.Order(); o >= order {
			order = o + 1
		}
	}
	fd := pdfDict{
		"/Type":    pdfName("/CollectionField"),
		"/Subtype": pdfName(collectionFieldNames[typ]),
		"/N":       encodeFormString(displayName),
		"/O":       float64(order),
		"/V":       true,
	}
	sd[collectionFieldKey(key)] = fd
	return &CollectionField{key: strings.TrimPrefix(key, "/"), dict: fd}
}

// Field returns the column with the given key, or nil.
func (s *CollectionSchema) Field(key string) *CollectionField {
	sd := s.dict()
	if sd == nil {
		return nil
	}
	fd, ok := resolveRefToDict(s.col.doc.objects, sd[collectionFieldKey(key)])
	if !ok {
		return nil
	}
	return &CollectionField{key: strings.TrimPrefix(key, "/"), dict: fd}
}

// Fields returns the columns in display order (by /O, then by key).
func (s *CollectionSchema) Fields() []*CollectionField {
	sd := s.dict()
	if sd == nil {
		return nil
	}
	var out []*CollectionField
	for name, v := range sd {
		if name == "/Type" {
			continue
		}
		fd, ok := resolveRefToDict(s.col.doc.objects, v)
		if !ok {
			continue
		}
		out = append(out, &CollectionField{key: collectionKeyFromName(name), dict: fd})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if oi, oj := out[i].Order(), out[j].Order(); oi != oj {
			return oi < oj
		}
		return out[i].key < out[j].key
	})
	return out
}

// Keys returns the column keys in display order.
func (s *CollectionSchema) Keys() []string {
	fields := s.Fields()
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.key)
	}
	return out
}

// Count returns the number of columns.
func (s *CollectionSchema) Count() int { return len(s.Fields()) }

// Remove deletes a column; it reports whether one was removed. Values already
// stored in collection items are left alone (harmless, and they reappear if
// the column is added back).
func (s *CollectionSchema) Remove(key string) bool {
	sd := s.dict()
	if sd == nil {
		return false
	}
	name := collectionFieldKey(key)
	if _, ok := sd[name]; !ok {
		return false
	}
	delete(sd, name)
	return true
}

// CollectionField is one portfolio column.
type CollectionField struct {
	key  string
	dict pdfDict
}

// Key returns the field key used by collection items.
func (f *CollectionField) Key() string { return f.key }

// DisplayName returns the column header text.
func (f *CollectionField) DisplayName() string { return decodeFormString(f.dict["/N"]) }

// SetDisplayName sets the column header text.
func (f *CollectionField) SetDisplayName(s string) { f.dict["/N"] = encodeFormString(s) }

// Type returns the column's value type.
func (f *CollectionField) Type() CollectionFieldType {
	return collectionFieldTypeFromName(dictGetName(f.dict, "/Subtype"))
}

// Order returns the column's position (/O); 0 when unset.
func (f *CollectionField) Order() int { return dictGetInt(f.dict, "/O") }

// SetOrder sets the column's position among the others.
func (f *CollectionField) SetOrder(n int) { f.dict["/O"] = float64(n) }

// IsVisible reports whether the viewer shows the column (/V, default true).
func (f *CollectionField) IsVisible() bool {
	if v, ok := f.dict["/V"].(bool); ok {
		return v
	}
	return true
}

// SetVisible shows or hides the column.
func (f *CollectionField) SetVisible(v bool) { f.dict["/V"] = v }

// IsDerived reports whether the column's values come from the attachment's
// file specification (file name, description, dates, size) rather than from
// its collection item — such a column needs no per-file value.
func (f *CollectionField) IsDerived() bool { return f.Type().derived() }

// IsEditable reports whether a viewer may edit the values (/E, default false).
func (f *CollectionField) IsEditable() bool {
	v, _ := f.dict["/E"].(bool)
	return v
}

// SetEditable allows or forbids viewer-side editing of the column's values.
func (f *CollectionField) SetEditable(v bool) { f.dict["/E"] = v }

// CollectionItem holds one attachment's values for the portfolio's schema
// fields (the file specification's /CI dictionary).
type CollectionItem struct {
	file *EmbeddedFile
}

// CollectionItem returns the attachment's portfolio field values.
func (f *EmbeddedFile) CollectionItem() *CollectionItem { return &CollectionItem{file: f} }

func (i *CollectionItem) dict() pdfDict {
	if i.file == nil || i.file.filespec == nil {
		return nil
	}
	d, ok := resolveRefToDict(i.file.doc.objects, i.file.filespec["/CI"])
	if !ok {
		return nil
	}
	return d
}

func (i *CollectionItem) ensure() pdfDict {
	if d := i.dict(); d != nil {
		return d
	}
	d := pdfDict{"/Type": pdfName("/CollectionItem")}
	i.file.filespec["/CI"] = d
	return d
}

// value resolves a field's stored value, following a /CollectionSubitem
// wrapper when a producer used one.
func (i *CollectionItem) value(field string) pdfValue {
	d := i.dict()
	if d == nil {
		return nil
	}
	v := resolveRef(i.file.doc.objects, d[collectionFieldKey(field)])
	if sub, ok := v.(pdfDict); ok {
		if dictGetName(sub, "/Type") == "/CollectionSubitem" {
			return resolveRef(i.file.doc.objects, sub["/D"])
		}
	}
	return v
}

// SetText stores a text value for the field.
func (i *CollectionItem) SetText(field, value string) {
	i.ensure()[collectionFieldKey(field)] = encodeFormString(value)
}

// SetNumber stores a numeric value for the field.
func (i *CollectionItem) SetNumber(field string, value float64) {
	i.ensure()[collectionFieldKey(field)] = value
}

// SetDate stores a date value for the field (written as a PDF date string).
func (i *CollectionItem) SetDate(field string, t time.Time) {
	i.ensure()[collectionFieldKey(field)] = pdfDateString(t)
}

// Text returns the field's value as text, or "" when unset.
func (i *CollectionItem) Text(field string) string {
	return decodeFormString(i.value(field))
}

// Number returns the field's numeric value; ok is false when the field is
// unset or holds another type.
func (i *CollectionItem) Number(field string) (value float64, ok bool) {
	switch v := i.value(field).(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// Date returns the field's date value; ok is false when the field is unset or
// does not parse as a PDF date.
func (i *CollectionItem) Date(field string) (value time.Time, ok bool) {
	s := decodeFormString(i.value(field))
	if s == "" {
		return time.Time{}, false
	}
	return parsePDFDate(s)
}

// Fields returns the keys this item carries values for, sorted.
func (i *CollectionItem) Fields() []string {
	d := i.dict()
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d))
	for name := range d {
		if name == "/Type" {
			continue
		}
		out = append(out, collectionKeyFromName(name))
	}
	sort.Strings(out)
	return out
}

// Remove deletes the item's value for a field; it reports whether one was
// removed.
func (i *CollectionItem) Remove(field string) bool {
	d := i.dict()
	if d == nil {
		return false
	}
	name := collectionFieldKey(field)
	if _, ok := d[name]; !ok {
		return false
	}
	delete(d, name)
	return true
}
