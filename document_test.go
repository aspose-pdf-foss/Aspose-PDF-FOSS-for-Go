package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestDocumentOpen(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if doc.PageCount() != marketingPages {
		t.Fatalf("expected %d pages, got %d", marketingPages, doc.PageCount())
	}
}

func TestDocumentSave(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_save.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != marketingPages {
		t.Fatalf("expected %d pages after save, got %d", marketingPages, n)
	}
}

func TestDocumentRotate(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 90"))
	if count != marketingPages {
		t.Errorf("expected /Rotate 90 on %d pages, found %d", marketingPages, count)
	}
}

func TestDocumentRotateSpecificPage(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate180, 1); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_specific_page.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 180"))
	if count != 1 {
		t.Errorf("expected /Rotate 180 exactly once (page 1 only), got %d", count)
	}
}

func TestDocumentRotateAccumulates(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
		t.Fatalf("second Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_accumulates.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 180"))
	if count != marketingPages {
		t.Errorf("expected /Rotate 180 on %d pages (accumulated 90+90), found %d", marketingPages, count)
	}
}

func TestDocumentRotateDuplicatePageNums(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90, 1, 1); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_duplicate_page_nums.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	// Duplicate page 1 must be deduplicated — rotation is 90°, not 180°.
	if count := bytes.Count(data, []byte("/Rotate 90")); count != 1 {
		t.Errorf("expected /Rotate 90 exactly once, got %d", count)
	}
}

func TestDocumentExtractPages(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	extracted, err := doc.Extract(asposepdf.PageRange{From: 1, To: 1})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if extracted.PageCount() != 1 {
		t.Fatalf("expected 1 page, got %d", extracted.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_extract_pages.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := extracted.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != 1 {
		t.Fatalf("expected 1 page in saved file, got %d", n)
	}
}

func TestDocumentReorder(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Reorder([]int{2, 1}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if doc.PageCount() != marketingPages {
		t.Fatalf("expected %d pages after Reorder, got %d", marketingPages, doc.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_reorder.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != marketingPages {
		t.Fatalf("expected %d pages in saved file, got %d", marketingPages, n)
	}
}

func TestDocumentWriteTo(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var buf bytes.Buffer
	n, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if n == 0 {
		t.Fatal("WriteTo wrote 0 bytes")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("output does not start with PDF header")
	}

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "document_write_to.pdf")
	if err := os.WriteFile(outputPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got := pageCountFromFile(t, outputPath); got != fourPagesCount {
		t.Fatalf("expected %d pages, got %d", fourPagesCount, got)
	}
}

func TestDocumentSetRotation(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// First rotate page 1 to 180°, then set it absolutely to 90°.
	if err := doc.Rotate(asposepdf.Rotate180, 1); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err := doc.SetRotation(asposepdf.Rotate90, 1); err != nil {
		t.Fatalf("SetRotation: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_set_rotation.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	// Must be 90°, not 270° (180+90).
	if count := bytes.Count(data, []byte("/Rotate 90")); count != 1 {
		t.Errorf("expected /Rotate 90 exactly once, got %d", count)
	}
	if bytes.Contains(data, []byte("/Rotate 270")) {
		t.Error("found /Rotate 270 — SetRotation must not accumulate")
	}
}

func TestDocumentInvalidRotateAngle(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.RotationAngle(45)); err == nil {
		t.Fatal("expected error for angle=45")
	}
	if err := doc.SetRotation(asposepdf.RotationAngle(45)); err == nil {
		t.Fatal("SetRotation: expected error for angle=45")
	}
}

func TestDocumentRotateZeroIsNoop(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	before := doc.PageCount()
	if err := doc.Rotate(asposepdf.Rotate0); err != nil {
		t.Fatalf("Rotate(Rotate0): %v", err)
	}
	if doc.PageCount() != before {
		t.Errorf("expected %d pages, got %d", before, doc.PageCount())
	}
}

func TestDocumentInvalidReorderPageNum(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Reorder([]int{1, 5}); err == nil {
		t.Fatal("expected error for page 5 in a 2-page document")
	}
}

func TestDocumentInvalidExtract(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := doc.Extract(); err == nil {
		t.Fatal("expected error for empty ranges")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 0, To: 1}); err == nil {
		t.Fatal("expected error for from=0")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 1, To: 999}); err == nil {
		t.Fatal("expected error for to out of bounds")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 2, To: 1}); err == nil {
		t.Fatal("expected error for from > to")
	}
}

func TestRemoveUnusedObjectsRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}

	// Remove all images from page 1.
	for _, info := range infos {
		if err := info.Remove(); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}

	removed := doc.RemoveUnusedObjects()
	t.Logf("removed %d unused objects", removed)
	if removed < 1 {
		t.Error("expected at least 1 object removed after image removal")
	}

	outDir := filepath.Join("result_files", "TestRemoveUnusedObjectsRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate the output.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Verify file size decreased compared to saving without cleanup.
	docNoCleanup, _ := asposepdf.Open("testdata/PdfWithImages.pdf")
	page2, _ := docNoCleanup.Page(1)
	infos2, _ := page2.ImageInfos()
	for _, info := range infos2 {
		info.Remove()
	}
	noCleanupPath := filepath.Join(outDir, "no_cleanup.pdf")
	docNoCleanup.Save(noCleanupPath)

	cleanupInfo, _ := os.Stat(outPath)
	noCleanupInfo, _ := os.Stat(noCleanupPath)
	t.Logf("with cleanup: %d bytes, without: %d bytes", cleanupInfo.Size(), noCleanupInfo.Size())
	if cleanupInfo.Size() >= noCleanupInfo.Size() {
		t.Error("expected smaller file size after RemoveUnusedObjects")
	}
}
