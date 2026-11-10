// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFileIsLinearized(t *testing.T) {
	file := func(dict string) []byte {
		return []byte("%PDF-1.6\n%\xe2\xe3\xcf\xd3\n7 0 obj\n" + dict + "\nendobj\n")
	}
	// /L is filled in with the real length, zero-padded so it does not change it.
	valid := file("<</Linearized 1/L 0000000000>>")
	i := bytes.Index(valid, []byte("0000000000"))
	copy(valid[i:], fmt.Sprintf("%010d", len(valid)))
	if !fileIsLinearized(valid) {
		t.Error("a valid parameter dictionary is not recognised")
	}
	for name, data := range map[string][]byte{
		"no /Linearized": file("<</L 40>>"),
		"wrong /L":       file("<</Linearized 1/L 5>>"),
		"/L not integer": file("<</Linearized 1/L (x)>>"),
		"not a dict":     file("[1 2 3]"),
		"no header":      []byte("7 0 obj <</Linearized 1/L 37>> endobj"),
	} {
		if fileIsLinearized(data) {
			t.Errorf("%s: reported linearized", name)
		}
	}
}
