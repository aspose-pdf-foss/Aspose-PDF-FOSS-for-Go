# E-Invoices (ZUGFeRD / Factur-X) — Design

Date: 2026-09-22 · Epic: pdf-go-t6s9 (ZUGFeRD/Factur-X increment) · Status: approved

## Goal

Package a structured invoice into a PDF the way ZUGFeRD 2.x / Factur-X 1.0
require, and get it back out of incoming PDFs. A hybrid e-invoice is a
PDF/A-3 document with the invoice's XML (UN/CEFACT Cross Industry Invoice,
CII) embedded as an associated file and described in the XMP metadata.
Mirrors the intent of Aspose.PDF for .NET's `PdfFormat.ZUGFeRD` conversion and
`FileSpecification.AFRelationship`.

## Decisions taken before design

- **Write and read, not generate.** The caller supplies the CII XML (their
  invoicing system produces it); the library packages it correctly and
  extracts it reliably. Generating CII from a Go model of EN 16931 is a
  separate domain and out of scope.
- **Two layers.** A general PDF/A-3 associated-files mechanism on embedded
  files, which PDF/A-3 needs in its own right, and a one-call invoice API on
  top of it that is hard to get wrong.

## Public API

### Associated files

```go
type AFRelationship int

const (
    AFUnspecified AFRelationship = iota // /Unspecified
    AFSource                            // /Source
    AFData                              // /Data
    AFAlternative                       // /Alternative
    AFSupplement                        // /Supplement
)

func (f *EmbeddedFile) AFRelationship() AFRelationship
func (f *EmbeddedFile) SetAFRelationship(r AFRelationship)
```

Setting a relationship writes `/AFRelationship` on the file specification and
adds it to the catalog's `/AF` array (once); removing the attachment removes
it from `/AF` too. `AFRelationship()` reports `AFUnspecified` for a file with
no entry. Mirrors Aspose's `FileSpecification.AFRelationship`.

Every embedded file stream written by the library gains `/Params /ModDate`
(PDF/A-3 requires it); `/Size` is already there.

### Invoice — write

```go
func (d *Document) AttachInvoice(xml []byte, opts ...InvoiceOptions) (*PDFAValidationReport, error)

type InvoiceOptions struct {
    FileName string         // default "factur-x.xml"; "xrechnung.xml" for an XRechnung profile
    Profile  InvoiceProfile // default: detected from the XML
    Format   PDFAFormat     // PDFA3B (default) or PDFA3A; any other value is an error
}
```

Steps:

1. Check the XML is well-formed and its root is `CrossIndustryInvoice` in
   namespace `urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100`.
2. Determine the profile (below), unless `Profile` is set.
3. Embed the XML under `FileName` with MIME `text/xml` and description
   "Factur-X invoice" — replacing an existing attachment of that name — and
   relationship `AFAlternative` for BASIC and above, `AFData` for MINIMUM and
   BASIC WL (the Factur-X rule: the lower profiles are not a full
   representation of the invoice).
4. `ConvertToPDFA(Format)`.
5. Write the Factur-X XMP properties and the PDF/A extension schema that
   declares them.
6. Return the PDF/A report.

The default file name follows the profile: `xrechnung.xml` for XRechnung,
`factur-x.xml` otherwise. Calling `AttachInvoice` again replaces the invoice
and the XMP block rather than adding a second one.

### Invoice — read

```go
func (d *Document) Invoice() (*Invoice, error) // (nil, nil) when the document is not an e-invoice

type Invoice struct {
    XML      []byte
    FileName string
    Profile  InvoiceProfile
    Standard string // "Factur-X 1.0 / ZUGFeRD 2.1+", "ZUGFeRD 2.0", "ZUGFeRD 1.0"
    Version  string // fx:Version from the XMP, "" when absent
}

type InvoiceProfile int // InvoiceProfileUnknown, Minimum, BasicWL, Basic, EN16931, Extended, XRechnung
```

The invoice is found by attachment name — `factur-x.xml`, `xrechnung.xml`,
`zugferd-invoice.xml` (ZUGFeRD 2.0), `ZUGFeRD-invoice.xml` (ZUGFeRD 1.0),
case-insensitively — falling back to the name the XMP's `DocumentFileName`
gives. The profile comes from the XMP `ConformanceLevel` when present, else
from the XML. `Standard` is decided by the XMP namespace (below), else by the
file name.

`InvoiceProfile` has a `String()` giving the Factur-X spelling: "MINIMUM",
"BASIC WL", "BASIC", "EN 16931", "EXTENDED", "XRECHNUNG".

### Validator and converter

- `ValidatePDFA` for PDF/A-3 formats adds rule `EMBEDDED_FILE_NOT_ASSOCIATED`:
  every embedded file's specification must carry `/AFRelationship` and be
  listed in the catalog's `/AF` (ISO 19005-3 §6.8).
- `ConvertToPDFA` for PDF/A-3 formats gives every attachment without a
  relationship `AFUnspecified` and lists it in `/AF`, so conversion produces
  a conforming file.

## Standards data

Profile identifiers (`rsm:ExchangedDocumentContext/
ram:GuidelineSpecifiedDocumentContextParameter/ram:ID`):

| URN | Profile |
|---|---|
| `urn:factur-x.eu:1p0:minimum` | MINIMUM |
| `urn:factur-x.eu:1p0:basicwl` | BASIC WL |
| `urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic` | BASIC |
| `urn:cen.eu:en16931:2017` | EN 16931 |
| `urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended` | EXTENDED |
| `urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_` + version | XRECHNUNG |

ZUGFeRD 2.0 used `urn:zugferd.de:2p0:*` for its profiles; those map to the
same values (`minimum`, `basicwl`, `basic`, `en16931`, `extended`). An unknown
URN is an error on write unless `Profile` is given, and `InvoiceProfileUnknown`
on read.

XMP namespaces:

| Standard | Namespace | Prefix |
|---|---|---|
| Factur-X 1.0 / ZUGFeRD 2.1+ (written) | `urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#` | `fx` |
| ZUGFeRD 2.0 (read) | `urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#` | `fx` |
| ZUGFeRD 1.0 (read) | `urn:ferd:pdfa:CrossIndustryDocument:invoice:1p0#` | `zf` |

Written properties: `fx:DocumentType` = `INVOICE`, `fx:DocumentFileName`,
`fx:Version` = `1.0`, `fx:ConformanceLevel` = the profile spelling.

## Internals

| File | Responsibility |
|---|---|
| `associated_files.go` | `AFRelationship`, reading/writing `/AFRelationship`, the catalog `/AF` array, `/Params /ModDate` |
| `einvoice.go` | `AttachInvoice`, `Invoice`, profile detection, file names and namespaces |
| `einvoice_xmp.go` | The `pdfaExtension:schemas` description and the `fx:*` properties in the XMP packet |
| `validate_pdfa.go`, `pdfa_convert.go` | The PDF/A-3 rule and the automatic relationship on conversion |

The `fx:*` properties travel through the existing XMP machinery:
`ConvertToPDFA` rewrites the packet but keeps simple custom properties. The
extension schema is a nested structure (`pdfaExtension:schemas` → bag of
`pdfaSchema` with a sequence of `pdfaProperty`), so it is inserted into the
finished packet as its own `rdf:Description`; a second call replaces it.

Profile detection reads the XML with `encoding/xml` only as far as the
guideline ID, so a large invoice is not held twice in parsed form.

Errors: malformed XML, root other than CII, unknown profile without an
explicit one, `Format` not a PDF/A-3 level. An encrypted document is
decrypted by the conversion (PDF/A forbids encryption) — documented, not
silent.

## Validation

- Unit tests: relationship round trip and `/AF` membership (set, change,
  remove attachment); `/Params /ModDate` present; profile detection for every
  URN; `AttachInvoice` → save → reopen → `Invoice()` for every profile; a
  second `AttachInvoice` replaces rather than duplicates; reading hand-built
  ZUGFeRD 1.0 and 2.0 files; the new PDF/A-3 rule and the conversion that
  satisfies it; errors for malformed XML, non-CII root, non-PDF/A-3 format.
- Independent check: the Python **factur-x** library (Akretion, BSD),
  installed into a throwaway virtualenv under `result_files/` — never into the
  system Python. It extracts the XML from our output, checks the Factur-X XMP
  fields, and validates the XML against the official XSD of its profile.
- Honest limit: full PDF/A-3 conformance still needs veraPDF (a JRE, absent
  here); our own `ValidatePDFA` is the check available locally.

## Out of scope

- Generating CII XML from invoice data; EN 16931 Schematron business rules.
- Writing ZUGFeRD 1.0 or 2.0 (read only).
- UBL invoices (XRechnung-UBL is a pure-XML format, not a hybrid PDF).
- Order-X and other Cross Industry Document types.
