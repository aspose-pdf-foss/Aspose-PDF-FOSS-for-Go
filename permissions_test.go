package asposepdf

import (
	"bytes"
	"testing"
)

// TestPermissionsBitPacking verifies Permissions.toPDFBits produces the
// Adobe-convention /P value per ISO 32000-1 Table 22: bits 1-2 shall be 0,
// bits 7-8 and 13-32 shall be 1 (reserved-set-high), and permission bits
// 3-6 and 9-12 reflect the eight boolean flags.
func TestPermissionsBitPacking(t *testing.T) {
	cases := []struct {
		name string
		p    Permissions
		want uint32
	}{
		{
			name: "all false — deny all",
			p:    Permissions{},
			want: 0xFFFFF0C0,
		},
		{
			name: "all true — allow all (matches Adobe constant -4)",
			p: Permissions{
				AllowPrint: true, AllowModify: true, AllowCopy: true, AllowAnnotations: true,
				AllowFormFill: true, AllowAccessibility: true, AllowAssembly: true, AllowPrintHighRes: true,
			},
			want: 0xFFFFFFFC,
		},
		{
			name: "print only",
			p:    Permissions{AllowPrint: true},
			want: 0xFFFFF0C4,
		},
		{
			name: "print + copy only",
			p:    Permissions{AllowPrint: true, AllowCopy: true},
			want: 0xFFFFF0D4,
		},
		{
			name: "accessibility only",
			p:    Permissions{AllowAccessibility: true},
			want: 0xFFFFF2C0,
		},
		{
			name: "print high res only (implies print in viewers per spec)",
			p:    Permissions{AllowPrintHighRes: true},
			want: 0xFFFFF8C0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uint32(tc.p.toPDFBits())
			if got != tc.want {
				t.Errorf("toPDFBits() = %#010x, want %#010x", got, tc.want)
			}
		})
	}
}

// TestEncryptWithCustomPermissionsMatchesPyPDF verifies key derivation
// against an external reference vector produced by pypdf with non-default
// permissions. Given the /O, /P, fileID pypdf chose, our computeEncKey +
// computeUserEntry must reproduce /U bit-for-bit.
//
// Source: pypdf 6.10.2 PdfWriter.encrypt(user_password="secret",
// algorithm="RC4-128", permissions_flag=UAP.PRINT|UAP.EXTRACT).
// /P = 0x14 = Print (bit 3) + Copy (bit 5), all reserved bits clear —
// pypdf does not set bits 7-8/13-32 high, but the derivation depends on
// the literal /P integer, so any /P value is a valid vector input.
func TestEncryptWithCustomPermissionsMatchesPyPDF(t *testing.T) {
	fileID := hexDecode(t, "e8b53c4162a2da07c50cc77f77f017af")
	oEntry := hexDecode(t, "0e522925a3e4e874c3cfacbef511a73ac4ec2bd865dcd3d4627614917abfd7e4")
	wantU := hexDecode(t, "9ba3c4e609b79518e43b00000bad92ec28bf4e5e4e758a4164004e56fffa0108")
	const password = "secret"
	var permP int32 = 0x14 // Print + Copy, reserved bits clear (pypdf convention)

	key := computeEncKey(password, oEntry, permP, fileID)
	gotU := computeUserEntry(key, fileID)

	if !bytes.Equal(gotU, wantU) {
		t.Errorf("computeUserEntry mismatch with /P=%#x\ngot:  %x\nwant: %x", permP, gotU, wantU)
	}
	if !verifyUserPassword(password, oEntry, wantU, fileID, permP) {
		t.Errorf("verifyUserPassword rejected correct password with custom /P")
	}
}

// TestSetPermissionsPropagatesToSavedFile covers the end-to-end public-API
// signal: SetPermissions affects what /P the saved file carries.
func TestSetPermissionsPropagatesToSavedFile(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetPassword("secret", "")
	doc.SetPermissions(Permissions{AllowPrint: true, AllowCopy: true})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	// Parse the saved bytes and pull /P from /Encrypt. Use low-level helpers
	// to avoid OpenStream (which currently rejects encrypted input).
	pIdx := bytes.Index(buf.Bytes(), []byte("/P "))
	if pIdx < 0 {
		t.Fatalf("/P not found in saved bytes")
	}
	// Read digits after "/P " — could be 1-10 digits, signed.
	rest := buf.Bytes()[pIdx+3:]
	end := 0
	for end < len(rest) && (rest[end] == '-' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	var pVal int
	_, err := parseIntBytes(rest[:end], &pVal)
	if err != nil {
		t.Fatalf("parse /P value %q: %v", rest[:end], err)
	}

	want := uint32(Permissions{AllowPrint: true, AllowCopy: true}.toPDFBits())
	if uint32(pVal) != want {
		t.Errorf("/P in saved file = %#010x, want %#010x", uint32(pVal), want)
	}
}

// TestSetPasswordWithoutSetPermissionsDefaultsAllowAll preserves backward
// compatibility: files produced without SetPermissions carry the Adobe
// all-allow constant 0xFFFFFFFC (= -4 signed).
func TestSetPasswordWithoutSetPermissionsDefaultsAllowAll(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.SetPassword("secret", "")

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/P 4294967292")) {
		t.Errorf("expected /P 4294967292 (all-allow default) in saved file")
	}
}

// parseIntBytes is a tiny shim used by the P-value test.
func parseIntBytes(b []byte, out *int) (int, error) {
	neg := false
	i := 0
	if len(b) > 0 && b[0] == '-' {
		neg = true
		i = 1
	}
	v := 0
	for ; i < len(b); i++ {
		v = v*10 + int(b[i]-'0')
	}
	if neg {
		v = -v
	}
	*out = v
	return len(b), nil
}
