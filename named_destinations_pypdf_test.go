package asposepdf_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func skipIfNoPypdfForNamedDest(t *testing.T) {
	t.Helper()
	if err := exec.Command("python", "-c", "import pypdf").Run(); err != nil {
		t.Skip("pypdf not available — skipping cross-tool test")
	}
}

func TestNamedDestinations_ReadableByPypdf(t *testing.T) {
	skipIfNoPypdfForNamedDest(t)
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("intro", pdf.NewDestinationFit(page)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tmp, err := os.CreateTemp("", "nd-cross-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := doc.WriteTo(tmp); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	script := `
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
nd = r.named_destinations
print("|".join(sorted(nd.keys())))
`
	out, err := exec.Command("python", "-c", script).Output()
	if err != nil {
		t.Fatalf("pypdf named_destinations read failed: %v (output: %q)", err, out)
	}
	if !strings.Contains(string(out), "intro") {
		t.Errorf("pypdf missing 'intro' in named destinations: %q", out)
	}
}

func TestNamedDestinations_ReadsPypdfOutput(t *testing.T) {
	skipIfNoPypdfForNamedDest(t)
	tmp, err := os.CreateTemp("", "nd-from-pypdf-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.add_named_destination("byPypdf", 0)
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
	if err := exec.Command("python", "-c", script).Run(); err != nil {
		t.Fatalf("pypdf write failed: %v", err)
	}
	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if doc.NamedDestinations().Get("byPypdf") == nil {
		t.Error("our parser didn't find 'byPypdf' in pypdf-built PDF")
	}
}
