package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFileAttachmentIconConstants(t *testing.T) {
	all := []pdf.FileAttachmentIcon{
		pdf.FileAttachmentIconUnknown,
		pdf.FileAttachmentIconGraph,
		pdf.FileAttachmentIconPaperclip,
		pdf.FileAttachmentIconPushPin,
		pdf.FileAttachmentIconTag,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("FileAttachmentIcon[%d] = %d, want %d", i, int(v), i)
		}
	}
}
