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

func skipIfNoPypdfForOutline(t *testing.T) {
	t.Helper()
	if err := exec.Command("python", "-c", "import pypdf").Run(); err != nil {
		t.Skip("pypdf not available — skipping cross-tool test")
	}
}

func TestOutlines_ReadableByPypdf(t *testing.T) {
	skipIfNoPypdfForOutline(t)
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("Cross-tool Bookmark")
	item.SetDestination(pdf.NewDestinationFit(page))
	doc.Outlines().Add(item)

	tmp, _ := os.CreateTemp("", "outline-cross-*.pdf")
	defer os.Remove(tmp.Name())
	doc.WriteTo(tmp)
	tmp.Close()

	script := `
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
def walk(items, out):
    for it in items:
        if isinstance(it, list):
            walk(it, out)
        else:
            out.append(it.title)
out = []
walk(r.outline, out)
print('|'.join(out))
`
	res, err := exec.Command("python", "-c", script).Output()
	if err != nil {
		t.Fatalf("pypdf outline read failed: %v", err)
	}
	if !strings.Contains(string(res), "Cross-tool Bookmark") {
		t.Errorf("pypdf didn't find our bookmark: %q", res)
	}
}

func TestOutlines_ReadsPypdfOutlines(t *testing.T) {
	skipIfNoPypdfForOutline(t)
	tmp, _ := os.CreateTemp("", "outline-from-pypdf-*.pdf")
	defer os.Remove(tmp.Name())
	tmp.Close()

	script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.add_outline_item("From pypdf", 0)
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
	if err := exec.Command("python", "-c", script).Run(); err != nil {
		t.Fatalf("pypdf write failed: %v", err)
	}
	raw, _ := os.ReadFile(tmp.Name())
	doc, err := pdf.OpenStream(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Outlines().Count() != 1 {
		t.Fatalf("our outline count after pypdf-built = %d", doc.Outlines().Count())
	}
	got := doc.Outlines().At(0).Title()
	if got != "From pypdf" {
		t.Errorf("Title = %q, want \"From pypdf\"", got)
	}
}
