// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestIsLinearized(t *testing.T) {
	src := pdf.NewDocument(200, 200)
	if src.IsLinearized() {
		t.Error("a document built in memory reports linearized")
	}
	var lin bytes.Buffer
	if _, err := src.WriteToLinearized(&lin); err != nil {
		t.Fatal(err)
	}
	d, err := pdf.OpenStream(bytes.NewReader(lin.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsLinearized() {
		t.Error("a linearized file is not reported as linearized")
	}
	if reopenBytes(t, d).IsLinearized() {
		t.Error("an ordinary save of it is still reported as linearized")
	}
	// Bytes appended after a linearized file (an incremental update) make /L
	// wrong, and the file is no longer linearized.
	grown := append(bytes.Clone(lin.Bytes()), "\n% appended\n"...)
	g, err := pdf.OpenStream(bytes.NewReader(grown))
	if err != nil {
		t.Fatal(err)
	}
	if g.IsLinearized() {
		t.Error("a file longer than its /L is reported as linearized")
	}
}
