package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestEncryptionAlgorithmConstants(t *testing.T) {
	if int(pdf.EncryptionAlgAES128) != 0 {
		t.Errorf("EncryptionAlgAES128 = %d, want 0 (zero value default)", int(pdf.EncryptionAlgAES128))
	}
	if int(pdf.EncryptionAlgRC4_128) != 1 {
		t.Errorf("EncryptionAlgRC4_128 = %d, want 1", int(pdf.EncryptionAlgRC4_128))
	}
}

func TestEncryptionOptionsHasAlgorithmField(t *testing.T) {
	opts := pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	}
	if opts.Algorithm != pdf.EncryptionAlgAES128 {
		t.Errorf("Algorithm = %v", opts.Algorithm)
	}
}

func TestEncryptionAlgAES256Constant(t *testing.T) {
	if int(pdf.EncryptionAlgAES256) != 2 {
		t.Errorf("EncryptionAlgAES256 = %d, want 2 (after AES128=0, RC4_128=1)", int(pdf.EncryptionAlgAES256))
	}
}
