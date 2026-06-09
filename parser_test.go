// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestStreamEndIndex checks the structure-aware endstream scan used when /Length
// is indirect, missing, or wrong. Binary stream data can contain a spurious
// "endstream" byte sequence; the scan must prefer the one the object structure
// confirms (followed by "endobj") so the stream isn't truncated mid-data.
func TestStreamEndIndex(t *testing.T) {
	// Spurious "endstream" embedded in the data, then the real terminator.
	data := []byte("RAW\x00endstreamMORE-BINARY\r\nendstream\r\nendobj")
	got := streamEndIndex(data)
	want := len("RAW\x00endstreamMORE-BINARY\r\n")
	if got != want {
		t.Errorf("streamEndIndex = %d, want %d (the endstream before endobj)", got, want)
	}

	// No confirming "endobj": fall back to the first match (best-effort).
	d2 := []byte("DATA\r\nendstream\r\nstartxref")
	if got := streamEndIndex(d2); got != len("DATA\r\n") {
		t.Errorf("fallback streamEndIndex = %d, want %d", got, len("DATA\r\n"))
	}

	// No marker at all.
	if got := streamEndIndex([]byte("no marker here")); got != -1 {
		t.Errorf("streamEndIndex(no marker) = %d, want -1", got)
	}
}
