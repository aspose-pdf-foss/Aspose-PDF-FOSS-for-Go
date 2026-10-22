// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// pdf-go-tnsb point 2: a non-embedded simple font named by a common alias
// of a Standard-14 face (/Arial, /TimesNewRoman, /CourierNew, …) with no
// /Encoding and no /ToUnicode used to resolve as an unknown font — only an
// exact Standard-14 PostScript name was recognised — so ApplyRedactions and
// ReplaceText refused the whole page (pdf-go-oq9i) and text extraction
// returned U+FFFD for every character, even though the renderer and the
// width tables already treat these names as their Standard-14 equivalents.

// aliasFontPage builds a one-page document whose font resource is a
// non-embedded /TrueType font with the given /BaseFont and neither
// /Encoding nor /ToUnicode, and returns the page and the resource name.
func aliasFontPage(t *testing.T, baseFont string) (*Document, *Page, string) {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	res, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	fonts, _ := resolveRef(doc.objects, page.pageResources()["/Font"]).(pdfDict)
	fd, ok := resolveRefToDict(doc.objects, fonts[res])
	if !ok {
		t.Fatal("font resource did not resolve")
	}
	fd["/Subtype"] = pdfName("/TrueType")
	fd["/BaseFont"] = pdfName(baseFont)
	delete(fd, "/Encoding")
	delete(fd, "/ToUnicode")
	return doc, page, res
}

func TestApplyRedactionsHandlesAliasedStandardFont(t *testing.T) {
	for _, base := range []string{"/Arial", "/Arial,Bold", "/TimesNewRoman", "/CourierNew"} {
		t.Run(base, func(t *testing.T) {
			doc, page, res := aliasFontPage(t, base)
			const secret = "ALIASSECRET"
			content := "BT\n" + res + " 12 Tf\n100 700 Td\n(" + secret + ") Tj\n" +
				"0 -100 Td\n(KEEPME) Tj\nET\n"
			if err := page.appendToContentStream([]byte(content)); err != nil {
				t.Fatal(err)
			}
			ra := NewRedactAnnotation(page, Rectangle{LLX: 95, LLY: 695, URX: 300, URY: 715})
			if err := page.Annotations().Add(ra); err != nil {
				t.Fatal(err)
			}
			if err := doc.ApplyRedactions(); err != nil {
				t.Fatalf("ApplyRedactions refused an aliased Standard-14 font: %v", err)
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if found, n := documentContainsBytes(reopened, secret); found {
				t.Errorf("%q still recoverable from %d object(s) after ApplyRedactions", secret, n)
			}
			p1, err := reopened.Page(1)
			if err != nil {
				t.Fatal(err)
			}
			text, err := p1.ExtractText()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "KEEPME") {
				t.Errorf("text outside the region did not survive / extract: %q", text)
			}
		})
	}
}

// An embedded font program carries its own built-in encoding, which the
// name says nothing about, so an embedded font named "Arial" with no
// /Encoding must stay unknown: guessing StandardEncoding there would
// change how the renderer picks its glyphs (render_text.go consults the
// PDF encoding before the program's own cmap once a font is "known").
func TestResolveFontEmbeddedAliasStaysUnknown(t *testing.T) {
	doc, page, res := aliasFontPage(t, "/ABCDEF+Arial")
	fonts, _ := resolveRef(doc.objects, page.pageResources()["/Font"]).(pdfDict)
	fd, _ := resolveRefToDict(doc.objects, fonts[res])
	fileNum := doc.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte{0}, Decoded: true})
	descNum := doc.addObject(pdfDict{
		"/Type":      pdfName("/FontDescriptor"),
		"/FontName":  pdfName("/ABCDEF+Arial"),
		"/FontFile2": pdfRef{Num: fileNum},
	})
	fd["/FontDescriptor"] = pdfRef{Num: descNum}
	if fi := resolveFont(doc.objects, fd); fi.known {
		t.Error("embedded font named Arial with no /Encoding resolved as known")
	}
	delete(fd, "/FontDescriptor")
	if fi := resolveFont(doc.objects, fd); !fi.known {
		t.Error("non-embedded font named Arial with no /Encoding resolved as unknown")
	}
}
