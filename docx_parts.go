// SPDX-License-Identifier: MIT

package asposepdf

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"
)

// Static/templated OPC parts of the DOCX package (docx_write.go holds the
// document.xml serializer). All XML is emitted with the transitional
// (schemas.openxmlformats.org 2006) namespaces — what Word itself writes and
// every consumer accepts — via string templating + xmlEscape (the xmp.go
// pattern; encoding/xml marshalling fights OOXML's namespace prefixes).

const docxXMLHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const docxNSw = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const docxNSr = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

// docxContentTypes covers every part the writer can emit. Uncovered parts are
// the #1 Word-repair trigger, so image extensions are declared even when no
// image is present (harmless).
const docxContentTypes = docxXMLHeader +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Default Extension="png" ContentType="image/png"/>` +
	`<Default Extension="jpg" ContentType="image/jpeg"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
	`</Types>`

const docxRootRels = docxXMLHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

// docxStyles is the minimal styles part: document defaults, Normal, the six
// built-in heading styles (named styles carrying w:outlineLvl — required for
// Word's navigation pane and TOC fields; direct formatting is invisible
// there), and the Hyperlink character style.
func docxStyles(bodyHalfPoints, spacingAfter int) string {
	var b strings.Builder
	b.WriteString(docxXMLHeader)
	fmt.Fprintf(&b, `<w:styles xmlns:w="%s">`, docxNSw)
	fmt.Fprintf(&b, `<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="%d"/></w:pPr></w:pPrDefault></w:docDefaults>`,
		bodyHalfPoints, bodyHalfPoints, spacingAfter)
	b.WriteString(`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>`)
	// Heading sizes scale off the body size like the exporters' thresholds.
	ratios := []float64{1.7, 1.35, 1.14, 1.05, 1.0, 1.0}
	for i := 1; i <= 6; i++ {
		sz := int(float64(bodyHalfPoints)*ratios[i-1]+0.5) &^ 1 // even half-points
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`,
			i, i, i-1, sz, sz)
	}
	b.WriteString(`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr></w:style>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

// docxNumbering emits one bullet abstract (id 0) and one decimal abstract
// (id 1), each with all 9 levels, plus one w:num per list instance the
// document serializer allocated. Each w:num is one list — two ordered lists
// sharing a numId would continue numbering across each other, so every list
// instance gets a fresh numId.
func docxNumbering(instances []bool) string {
	var b strings.Builder
	b.WriteString(docxXMLHeader)
	fmt.Fprintf(&b, `<w:numbering xmlns:w="%s">`, docxNSw)
	for abs := 0; abs < 2; abs++ {
		fmt.Fprintf(&b, `<w:abstractNum w:abstractNumId="%d"><w:multiLevelType w:val="hybridMultilevel"/>`, abs)
		for lvl := 0; lvl < 9; lvl++ {
			indent := 720 + 720*lvl // twips: 0.5" per level
			if abs == 0 {
				// A literal U+2022 bullet in the default font renders the same
				// everywhere (the classic Symbol-font bullet needs a private-use
				// character that only the Symbol face covers).
				fmt.Fprintf(&b, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="&#8226;"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`,
					lvl, indent)
			} else {
				fmt.Fprintf(&b, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%%%d."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`,
					lvl, lvl+1, indent)
			}
		}
		b.WriteString(`</w:abstractNum>`)
	}
	for i, ordered := range instances {
		abs := 0
		if ordered {
			abs = 1
		}
		fmt.Fprintf(&b, `<w:num w:numId="%d"><w:abstractNumId w:val="%d"/></w:num>`, i+1, abs)
	}
	b.WriteString(`</w:numbering>`)
	return b.String()
}

// docxRel is one entry of word/_rels/document.xml.rels.
type docxRel struct {
	id       string
	relType  string
	target   string
	external bool
}

func docxDocumentRels(rels []docxRel) string {
	var b strings.Builder
	b.WriteString(docxXMLHeader)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rels {
		mode := ""
		if r.external {
			// Omitting TargetMode="External" makes Word treat the URL as an
			// internal part reference → repair prompt.
			mode = ` TargetMode="External"`
		}
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"%s/>`,
			r.id, r.relType, xmlEscape(r.target), mode)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// docxPart is one file of the OPC package.
type docxPart struct {
	name string
	data []byte
}

// writeDocxZip assembles the package. Part names use forward slashes and
// exact case; [Content_Types].xml goes first by convention.
func writeDocxZip(w io.Writer, parts []docxPart) error {
	zw := zip.NewWriter(w)
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return fmt.Errorf("docx: create %s: %w", p.name, err)
		}
		if _, err := f.Write(p.data); err != nil {
			return fmt.Errorf("docx: write %s: %w", p.name, err)
		}
	}
	return zw.Close()
}
