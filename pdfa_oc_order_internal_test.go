// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// sharedOrderPDF builds a page whose layer configuration's /Order nests levels
// arrays, each referring fanout times to the next; the last names the layer.
func sharedOrderPDF(t *testing.T, fanout, levels int) []byte {
	t.Helper()
	first := 5
	objs := []string{
		fmt.Sprintf("1 0 obj << /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [4 0 R] /D << /Name (Default) /Order [%d 0 R %d 0 R] >> >> >> endobj", first, first),
		"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj",
		"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >> endobj",
		"4 0 obj << /Type /OCG /Name (Layer) >> endobj",
	}
	for i := 0; i < levels; i++ {
		n := first + i
		var body bytes.Buffer
		if i == levels-1 {
			body.WriteString("4 0 R")
		} else {
			for k := 0; k < fanout; k++ {
				fmt.Fprintf(&body, "%d 0 R ", n+1)
			}
		}
		objs = append(objs, fmt.Sprintf("%d 0 obj [%s] endobj", n, body.String()))
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = buf.Len()
		buf.WriteString(o + "\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xref)
	return buf.Bytes()
}

// TestOCOrderSharedArraysWalkedOnce pins GHSA-mp4r-cgm3-cq84: an /Order whose
// nested arrays are shared must be walked once per array, not once per path.
// Unfixed, fan-out 4 over 30 levels is 4^30 calls.
func TestOCOrderSharedArraysWalkedOnce(t *testing.T) {
	data := sharedOrderPDF(t, 4, 30)
	calls := map[string]func(d *Document) []string{
		"ValidatePDFA": func(d *Document) []string { return pdfaRules(d.ValidatePDFA(PDFA2B).Issues) },
		"ValidatePDFUA": func(d *Document) []string {
			var r []string
			for _, i := range d.ValidatePDFUA().Issues {
				r = append(r, i.Rule)
			}
			return r
		},
		"ConvertToPDFA": func(d *Document) []string {
			rep, err := d.ConvertToPDFA(PDFA2B)
			if err != nil {
				return []string{"error: " + err.Error()}
			}
			return pdfaRules(rep.Issues)
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			d, err := OpenStream(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan []string, 1)
			go func() { done <- call(d) }()
			select {
			case rules := <-done:
				for _, r := range rules {
					if r == "LAYER_CONFIG" || r == "UA_LAYER_CONFIG" {
						t.Errorf("layer reached through the shared arrays reported missing: %v", rules)
					}
				}
			case <-time.After(10 * time.Second):
				t.Fatal("did not finish: the shared /Order arrays are walked once per path")
			}
		})
	}
}

func pdfaRules(issues []PDFAIssue) []string {
	var r []string
	for _, i := range issues {
		r = append(r, i.Rule)
	}
	return r
}
