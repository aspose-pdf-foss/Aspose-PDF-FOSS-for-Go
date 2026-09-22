// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// toXRefStreamOnly rewrites a classic-xref PDF so that its only
// cross-reference section is an uncompressed /Type /XRef stream carrying the
// trailer entries — no `trailer` keyword anywhere, objects left top-level,
// as seen in real-world producer output.
func toXRefStreamOnly(t *testing.T, classic []byte) []byte {
	t.Helper()
	xi := bytes.LastIndex(classic, []byte("\nxref"))
	ti := bytes.LastIndex(classic, []byte("trailer"))
	si := bytes.LastIndex(classic, []byte("startxref"))
	if xi < 0 || ti < xi || si < ti {
		t.Fatal("input is not a classic-xref PDF")
	}
	body := classic[:xi+1]
	tr := string(classic[ti+len("trailer") : si])
	tr = tr[strings.Index(tr, "<<")+2 : strings.LastIndex(tr, ">>")]
	tr = regexp.MustCompile(`/Size\s+\d+`).ReplaceAllString(tr, "")

	offs := map[int]int{}
	maxNum := 0
	for _, m := range regexp.MustCompile(`(?m)^(\d+) 0 obj`).FindAllSubmatchIndex(body, -1) {
		n, _ := strconv.Atoi(string(body[m[2]:m[3]]))
		offs[n] = m[0]
		maxNum = max(maxNum, n)
	}
	xrefNum, xrefOff := maxNum+1, len(body)
	offs[xrefNum] = xrefOff
	var rows bytes.Buffer
	for n := 0; n <= xrefNum; n++ {
		if o, ok := offs[n]; ok {
			rows.WriteByte(1)
			_ = binary.Write(&rows, binary.BigEndian, uint32(o))
			rows.Write([]byte{0, 0})
		} else {
			rows.Write([]byte{0, 0, 0, 0, 0, 0xff, 0xff})
		}
	}

	var out bytes.Buffer
	out.Write(body)
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Length %d %s >>\nstream\n",
		xrefNum, xrefNum+1, rows.Len(), tr)
	out.Write(rows.Bytes())
	fmt.Fprintf(&out, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	if bytes.Contains(out.Bytes(), []byte("trailer")) {
		t.Fatal("rewritten PDF still contains a trailer keyword")
	}
	return out.Bytes()
}

var xrefStreamAlgorithms = []struct {
	name string
	alg  pdf.EncryptionAlgorithm
}{
	{"RC4-40", pdf.EncryptionAlgRC4_40},
	{"RC4-128", pdf.EncryptionAlgRC4_128},
	{"AES-128", pdf.EncryptionAlgAES128},
	{"AES-256", pdf.EncryptionAlgAES256},
}

func encryptedXRefStreamPDF(t *testing.T, alg pdf.EncryptionAlgorithm) []byte {
	t.Helper()
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("secret text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "user", OwnerPassword: "owner", Algorithm: alg})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return toXRefStreamOnly(t, buf.Bytes())
}

// TestXRefStreamWrongPasswordRejected opens an encrypted PDF whose only
// cross-reference section is an xref stream, under each Standard handler
// profile. A wrong or empty password must fail with ErrInvalidPassword — not
// fall back to xref reconstruction, which finds no `trailer` keyword, drops
// /Encrypt, and opens the ciphertext as a plain document — while the user
// and owner passwords both unlock the text.
func TestXRefStreamWrongPasswordRejected(t *testing.T) {
	for _, tc := range xrefStreamAlgorithms {
		t.Run(tc.name, func(t *testing.T) {
			data := encryptedXRefStreamPDF(t, tc.alg)
			for _, pw := range []string{"wrong", ""} {
				if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), pw); !errors.Is(err, pdf.ErrInvalidPassword) {
					t.Errorf("password %q: err = %v, want ErrInvalidPassword", pw, err)
				}
			}
			if _, err := pdf.OpenStream(bytes.NewReader(data)); !errors.Is(err, pdf.ErrEncrypted) {
				t.Errorf("OpenStream: err = %v, want ErrEncrypted", err)
			}
			for _, pw := range []string{"user", "owner"} {
				doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), pw)
				if err != nil {
					t.Fatalf("password %q: %v", pw, err)
				}
				text, err := doc.ExtractText()
				if err != nil || len(text) != 1 || !strings.Contains(text[0], "secret text") {
					t.Errorf("password %q: text = %q, %v", pw, text, err)
				}
			}
		})
	}
}

// TestXRefStreamReconstructedKeepsEncrypt opens the same PDFs with
// startxref pointing past the end of the file, forcing xref reconstruction.
// The rebuilt trailer must take /Encrypt and /ID from the /XRef stream
// dictionary, so a wrong password is still rejected and the right one still
// decrypts.
func TestXRefStreamReconstructedKeepsEncrypt(t *testing.T) {
	for _, tc := range xrefStreamAlgorithms {
		t.Run(tc.name, func(t *testing.T) {
			data := encryptedXRefStreamPDF(t, tc.alg)
			i := bytes.LastIndex(data, []byte("startxref\n")) + len("startxref\n")
			broken := append(append(append([]byte{}, data[:i]...), "999999999"...),
				data[bytes.IndexByte(data[i:], '\n')+i:]...)

			if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(broken), "wrong"); !errors.Is(err, pdf.ErrInvalidPassword) {
				t.Errorf("wrong password: err = %v, want ErrInvalidPassword", err)
			}
			doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(broken), "user")
			if err != nil {
				t.Fatalf("user password: %v", err)
			}
			if text, _ := doc.ExtractText(); len(text) != 1 || !strings.Contains(text[0], "secret text") {
				t.Errorf("text = %q", text)
			}
		})
	}
}

// TestReconstructedTrailerAdoptsEncryptDict opens the same PDFs truncated
// inside the /XRef stream, so no trailer entries survive anywhere. The
// scanned /Encrypt dictionary must still be adopted: the open fails rather
// than returning the ciphertext as a plain document.
func TestReconstructedTrailerAdoptsEncryptDict(t *testing.T) {
	for _, tc := range xrefStreamAlgorithms {
		t.Run(tc.name, func(t *testing.T) {
			data := encryptedXRefStreamPDF(t, tc.alg)
			stripped := data[:bytes.LastIndex(data, []byte("/Type /XRef"))]
			doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(stripped), "wrong")
			if err == nil {
				text, _ := doc.ExtractText()
				t.Fatalf("opened with a wrong password; text = %q", text)
			}
		})
	}
}
