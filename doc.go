package asposepdf

// toIntBytes converts a byte slice of ASCII digits to int.
func toIntBytes(raw []byte) int {
	n := 0
	for _, b := range raw {
		if b >= '0' && b <= '9' {
			n = n*10 + int(b-'0')
		}
	}
	return n
}
