package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

// TestEncryptDecryptRoundTripFiles is the headline integration test for
// the encryption epic: take a real testdata PDF, capture its per-page
// extracted text, encrypt with both user and owner passwords, save to a
// real file, reopen via OpenWithPassword, capture per-page text again,
// and assert byte-for-byte equality. Runs across the full set of text-
// bearing testdata fixtures including Cyrillic content (alfa.pdf).
//
// Anything broken in /Encrypt parsing, password verification, key
// derivation, per-object decryption, ObjStm decryption, stream re-decode,
// or string decryption fails here.
func TestEncryptDecryptRoundTripFiles(t *testing.T) {
	files := testFiles(t)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}

	for _, src := range files {
		t.Run(stem(src), func(t *testing.T) {
			// Snapshot the plain content.
			plain, err := pdf.Open(src)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			origPages, err := plain.ExtractText()
			if err != nil {
				t.Fatalf("ExtractText (plain): %v", err)
			}

			// Encrypt and save.
			plain.SetPassword("user-secret", "owner-secret")
			encryptedPath := filepath.Join(resultDir, stem(src)+"_encrypted.pdf")
			if err := plain.Save(encryptedPath); err != nil {
				t.Fatalf("Save encrypted: %v", err)
			}

			// Round-trip with user password.
			t.Run("user password", func(t *testing.T) {
				doc, err := pdf.OpenWithPassword(encryptedPath, "user-secret")
				if err != nil {
					t.Fatalf("OpenWithPassword(user): %v", err)
				}
				gotPages, err := doc.ExtractText()
				if err != nil {
					t.Fatalf("ExtractText: %v", err)
				}
				assertPagesEqual(t, gotPages, origPages)
			})

			// Round-trip with owner password.
			t.Run("owner password", func(t *testing.T) {
				doc, err := pdf.OpenWithPassword(encryptedPath, "owner-secret")
				if err != nil {
					t.Fatalf("OpenWithPassword(owner): %v", err)
				}
				gotPages, err := doc.ExtractText()
				if err != nil {
					t.Fatalf("ExtractText: %v", err)
				}
				assertPagesEqual(t, gotPages, origPages)
			})
		})
	}
}

// TestEditInPlaceEncrypted exercises the full edit-in-place workflow that
// motivated password-aware Open: open an encrypted file, mutate it, save
// with encryption preserved, and verify that BOTH the original content
// and the new content come back on the next open. After OpenWithPassword,
// the supplied password is preserved on Document.encrypt so a plain Save
// re-encrypts automatically — this test pins that behavior.
func TestEditInPlaceEncrypted(t *testing.T) {
	src := testFile(t)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}

	// Step 1: encrypt the source.
	plain, err := pdf.Open(src)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	origPages, err := plain.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText (plain): %v", err)
	}
	plain.SetPassword("secret", "")
	encryptedPath := filepath.Join(resultDir, "edit_in_place_step1.pdf")
	if err := plain.Save(encryptedPath); err != nil {
		t.Fatalf("Save encrypted: %v", err)
	}

	// Step 2: open encrypted, add a watermark, save (should remain encrypted).
	const watermark = "EDITED IN PLACE"
	doc, err := pdf.OpenWithPassword(encryptedPath, "secret")
	if err != nil {
		t.Fatalf("OpenWithPassword: %v", err)
	}
	if err := doc.AddTextWatermark(watermark, pdf.TextStyle{
		Font: pdf.FontHelveticaBold, Size: 36,
	}); err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}
	editedPath := filepath.Join(resultDir, "edit_in_place_step2.pdf")
	if err := doc.Save(editedPath); err != nil {
		t.Fatalf("Save edited: %v", err)
	}

	// Step 3: plain Open of the saved file must fail (still encrypted).
	if _, err := pdf.Open(editedPath); err == nil {
		t.Error("plain Open succeeded on edited file — encryption was lost across edit-in-place")
	}

	// Step 4: OpenWithPassword should return both original and watermark text.
	doc2, err := pdf.OpenWithPassword(editedPath, "secret")
	if err != nil {
		t.Fatalf("reopen edited: %v", err)
	}
	pagesAfter, err := doc2.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText (edited): %v", err)
	}
	if len(pagesAfter) != len(origPages) {
		t.Fatalf("page count changed: got %d, want %d", len(pagesAfter), len(origPages))
	}
	// Every page must contain the watermark; original content must still be there.
	for i, page := range pagesAfter {
		if !strings.Contains(page, watermark) {
			t.Errorf("page %d missing watermark %q after edit-in-place roundtrip", i+1, watermark)
		}
		if !strings.Contains(page, origPages[i]) && origPages[i] != "" {
			// Watermark may interleave with original text via line groupings;
			// require the longest non-trivial original line to survive.
			if longest := longestLine(origPages[i]); longest != "" && !strings.Contains(page, longest) {
				t.Errorf("page %d lost original content after edit; longest original line %q not found", i+1, longest)
			}
		}
	}
}

// TestPermissionsSurviveRoundTrip pins the new Permissions() getter and
// permissions preservation across Save+OpenWithPassword.
func TestPermissionsSurviveRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.AddText("perm test", pdf.TextStyle{Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})

	want := pdf.Permissions{
		AllowPrint:         true,
		AllowAccessibility: true,
		AllowAssembly:      true,
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "secret",
		Permissions:  &want,
	})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	reopened, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "secret")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	got, ok := reopened.Permissions()
	if !ok {
		t.Fatal("Permissions() returned !ok on reopened encrypted file")
	}
	if got != want {
		t.Errorf("Permissions roundtrip mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
}

// TestPermissionsOnUnencryptedDocument verifies the getter contract for
// the unencrypted case: zero value plus !ok.
func TestPermissionsOnUnencryptedDocument(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	got, ok := doc.Permissions()
	if ok {
		t.Errorf("Permissions() returned ok=true on unencrypted document; got %+v", got)
	}
	if got != (pdf.Permissions{}) {
		t.Errorf("Permissions() returned non-zero value on unencrypted document: %+v", got)
	}
}

// TestRemoveEncryption verifies the explicit decryption API: open with
// password, RemoveEncryption, Save → result is a plain PDF.
func TestRemoveEncryption(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.AddText("remove enc", pdf.TextStyle{Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})
	doc.SetPassword("secret", "")
	var encBuf bytes.Buffer
	if _, err := doc.WriteTo(&encBuf); err != nil {
		t.Fatalf("WriteTo encrypted: %v", err)
	}

	reopened, err := pdf.OpenStreamWithPassword(bytes.NewReader(encBuf.Bytes()), "secret")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	reopened.RemoveEncryption()

	var plainBuf bytes.Buffer
	if _, err := reopened.WriteTo(&plainBuf); err != nil {
		t.Fatalf("WriteTo plain: %v", err)
	}
	plain, err := pdf.OpenStream(bytes.NewReader(plainBuf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream on RemoveEncryption output should succeed; got: %v", err)
	}
	if _, ok := plain.Permissions(); ok {
		t.Error("Permissions().ok = true after RemoveEncryption + Save")
	}
}

// assertPagesEqual reports per-page mismatches between two extracted-text
// page slices, with concise diffs.
func assertPagesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("page count: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("page %d text mismatch:\n  got:  %q\n  want: %q",
				i+1, truncate(got[i], 200), truncate(want[i], 200))
		}
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// longestLine returns the longest line in s, used by TestEditInPlace as a
// minimal "original content survived" probe when full string-contains
// comparison is too strict due to layout reflow.
func longestLine(s string) string {
	lines := strings.Split(s, "\n")
	longest := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > len(longest) {
			longest = line
		}
	}
	return longest
}
