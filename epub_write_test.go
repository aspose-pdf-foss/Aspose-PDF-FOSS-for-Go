// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func epubParts(t *testing.T, data []byte) (map[string][]byte, []*zip.File) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a zip: %v", err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		parts[f.Name] = b
	}
	return parts, zr.File
}

func TestWriteEpubContainerInvariants(t *testing.T) {
	doc := buildDocxSource(t)
	var buf bytes.Buffer
	if err := doc.WriteEpub(&buf, pdf.EpubSaveOptions{Title: "Test Book"}); err != nil {
		t.Fatalf("WriteEpub: %v", err)
	}
	parts, files := epubParts(t, buf.Bytes())

	// OCF: mimetype first and stored.
	if len(files) == 0 || files[0].Name != "mimetype" {
		t.Fatal("mimetype is not the first zip entry")
	}
	if files[0].Method != zip.Store {
		t.Error("mimetype entry is compressed; must be stored")
	}
	if string(parts["mimetype"]) != "application/epub+zip" {
		t.Errorf("mimetype content = %q", parts["mimetype"])
	}
	if !strings.Contains(string(parts["META-INF/container.xml"]), "OEBPS/content.opf") {
		t.Error("container.xml does not point at the package document")
	}

	opf := string(parts["OEBPS/content.opf"])
	if !strings.Contains(opf, "<dc:title>Test Book</dc:title>") {
		t.Error("dc:title missing")
	}
	if !strings.Contains(opf, `properties="nav"`) {
		t.Error("nav item missing")
	}
	// Every manifest href resolves to a zip part; every spine idref resolves.
	for _, m := range regexp.MustCompile(`href="([^"]+)"`).FindAllStringSubmatch(opf, -1) {
		if parts["OEBPS/"+m[1]] == nil {
			t.Errorf("manifest href %s missing from package", m[1])
		}
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`<item id="([^"]+)"`).FindAllStringSubmatch(opf, -1) {
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`<itemref idref="([^"]+)"`).FindAllStringSubmatch(opf, -1) {
		if !ids[m[1]] {
			t.Errorf("spine idref %s not in manifest", m[1])
		}
	}

	// Every XML part is well-formed.
	for name, data := range parts {
		if !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".opf") && !strings.HasSuffix(name, ".xml") {
			continue
		}
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s: malformed XML: %v", name, err)
				break
			}
		}
	}
}

func TestWriteEpubContent(t *testing.T) {
	doc := buildDocxSource(t)
	var buf bytes.Buffer
	if err := doc.WriteEpub(&buf); err != nil {
		t.Fatal(err)
	}
	parts, _ := epubParts(t, buf.Bytes())
	var all strings.Builder
	for name, data := range parts {
		if strings.HasPrefix(name, "OEBPS/text/") {
			all.Write(data)
		}
	}
	x := all.String()
	for _, want := range []string{"Annual Report", "quarterly revenue", "Key highlights",
		"Revenue up", "First step", "code_sample()"} {
		if !strings.Contains(x, want) {
			t.Errorf("chapter text missing %q", want)
		}
	}
	if !strings.Contains(x, "<h1") {
		t.Error("no h1 heading")
	}
	if !strings.Contains(x, "<ul>") || !strings.Contains(x, "<ol>") {
		t.Error("lists not reconstructed")
	}
	if !strings.Contains(x, "<pre>") {
		t.Error("code block not reconstructed")
	}
	if !strings.Contains(x, `<a href="https://example.com/">`) {
		t.Error("hyperlink not reconstructed")
	}
	// The title from /Info is absent here; nav must still be present and the
	// h1 must appear in it.
	nav := string(parts["OEBPS/nav.xhtml"])
	if !strings.Contains(nav, "Annual Report") {
		t.Errorf("nav missing the level-1 heading: %s", nav)
	}
}
