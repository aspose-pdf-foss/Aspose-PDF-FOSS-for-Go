// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// BarcodeField (barcode.go / barcode_code128.go / barcode_qr.go /
// barcode_qr_gf256.go / barcode_qr_tables.go) — a text field (/FT /Tx) whose
// value is drawn as a barcode symbol instead of text. Mirrors Aspose.PDF for
// .NET's BarcodeField (Symbology / CodeText), adapted to this library's
// form-field pattern: it embeds TextBoxField like NumberField/DateField
// (form_fields_extra.go), so Value()/SetValue()/SetReadOnly/Flatten/JSON
// export all just work, and a private (non-spec, ignored by every other
// reader) dict entry records which symbology to draw and lets
// fieldFromNode reclassify the type on reopen — the same trick
// NumberField/DateField play with their format JavaScript action.
//
// v1 supports two symbologies (BarcodeCode128, BarcodeQR); PDF417 is
// tracked separately (see CLAUDE.md/beads) because a correct encoder needs
// a large (2787-entry) codeword-to-bar-pattern lookup table this library
// cannot safely hand-transcribe without a reference decoder to check it
// against.

// BarcodeSymbology identifies which barcode encoding a BarcodeField draws.
type BarcodeSymbology int

const (
	BarcodeSymbologyUnknown BarcodeSymbology = iota
	// BarcodeCode128 encodes printable ASCII (0x20-0x7E) via Code Set B
	// (ISO/IEC 15417). Non-ASCII text is rejected — use BarcodeQR instead.
	BarcodeCode128
	// BarcodeQR encodes arbitrary text as its UTF-8 bytes in QR byte mode
	// (ISO/IEC 18004), error-correction level M, version chosen
	// automatically (1-40).
	BarcodeQR
)

func barcodeSymbologyName(s BarcodeSymbology) pdfName {
	switch s {
	case BarcodeCode128:
		return "/Code128"
	case BarcodeQR:
		return "/QR"
	}
	return "/Unknown"
}

func barcodeSymbologyFromName(n pdfName) BarcodeSymbology {
	switch n {
	case "/Code128":
		return BarcodeCode128
	case "/QR":
		return BarcodeQR
	}
	return BarcodeSymbologyUnknown
}

// barcodeMarkerKey is a private, non-spec dict entry on the field (and
// widget, since text fields are combined-pattern) recording the chosen
// symbology. Unknown dict keys are ignored by every conforming reader, so
// this is safe to carry in the file; it exists purely so this library can
// tell a BarcodeField apart from a plain TextBoxField on reopen.
const barcodeMarkerKey = "/AsposeBC"

func barcodeSymbologyFromDict(dict pdfDict) BarcodeSymbology {
	marker, ok := dict[barcodeMarkerKey].(pdfDict)
	if !ok {
		return BarcodeSymbologyUnknown
	}
	name, ok := marker["/Symb"].(pdfName)
	if !ok {
		return BarcodeSymbologyUnknown
	}
	return barcodeSymbologyFromName(name)
}

// barcodeModules is a rectangular grid of ink/no-ink barcode modules,
// row-major top-to-bottom, quiet zone already baked in — the common
// representation renderBarcodeModules returns regardless of symbology.
type barcodeModules struct {
	Cols, Rows int
	Bits       []bool // len == Cols*Rows
	// Square marks a 2-D symbology (QR): the appearance generator scales it
	// uniformly and centres it. A linear symbology (Code128, Rows==1)
	// instead stretches to the widget's full height.
	Square bool
}

// renderBarcodeModules encodes value under the given symbology. Returns an
// error for an unknown/unset symbology or a value that symbology cannot
// encode (checked before any field mutation by AddBarcodeField/SetValue/
// SetSymbology, so a rejected call leaves the field unchanged).
func renderBarcodeModules(s BarcodeSymbology, value string) (barcodeModules, error) {
	switch s {
	case BarcodeCode128:
		return encodeCode128Modules(value)
	case BarcodeQR:
		return encodeQRModules(value)
	default:
		return barcodeModules{}, fmt.Errorf("asposepdf: unknown barcode symbology %d", s)
	}
}

// BarcodeField draws its value as a barcode symbol. Value()/SetValue()
// (inherited from TextBoxField) get/set the encoded text; every value
// change re-renders the widget's /AP. Bar colour and background reuse
// FieldStyle.TextColor / BackgroundColor via SetStyle — there is no
// separate "ForegroundColor" on this type. Unlike other field types,
// BarcodeField draws no default border or background fill (a barcode's
// quiet zone must stay open), so both stay transparent unless SetStyle
// sets them explicitly.
type BarcodeField struct{ TextBoxField }

// AddBarcodeField adds a barcode field: value is the text encoded into the
// symbol (a URL or short text for QR; printable ASCII for Code128), and
// symbology selects the encoding. Errors (without creating the field) if
// value cannot be encoded under symbology.
func (f *Form) AddBarcodeField(pageNum int, rect Rectangle, name string, symbology BarcodeSymbology, value string) (*BarcodeField, error) {
	if _, err := renderBarcodeModules(symbology, value); err != nil {
		return nil, err
	}
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict[barcodeMarkerKey] = pdfDict{"/Symb": barcodeSymbologyName(symbology)}
		// Unlike the sibling extra field types (form_fields_extra.go), which
		// start with an empty /V and expect a separate SetValue call, this
		// sets /V directly: a barcode field is useless without an initial
		// value, and it's already validated above.
		dict["/V"] = encodeFormString(value)
	})
	if err != nil {
		return nil, err
	}
	return fld.(*BarcodeField), nil
}

// Symbology returns the field's barcode encoding.
func (bc *BarcodeField) Symbology() BarcodeSymbology {
	if bc.node == nil {
		return BarcodeSymbologyUnknown
	}
	return barcodeSymbologyFromDict(bc.node.dict)
}

// SetSymbology changes the barcode encoding and re-renders the appearance.
// Errors (leaving the field unchanged) if the field's current value cannot
// be encoded under the new symbology.
func (bc *BarcodeField) SetSymbology(s BarcodeSymbology) error {
	if bc.node == nil {
		return errFieldDetached
	}
	if _, err := renderBarcodeModules(s, bc.Value()); err != nil {
		return err
	}
	bc.node.dict[barcodeMarkerKey] = pdfDict{"/Symb": barcodeSymbologyName(s)}
	noteFormMutated(bc.node)
	return nil
}

// SetValue validates that value encodes under the field's current
// symbology before writing it (leaving the field unchanged on error), then
// re-renders the barcode appearance.
func (bc *BarcodeField) SetValue(value string) error {
	if bc.node == nil {
		return errFieldDetached
	}
	if _, err := renderBarcodeModules(bc.Symbology(), value); err != nil {
		return err
	}
	return bc.TextBoxField.SetValue(value)
}
