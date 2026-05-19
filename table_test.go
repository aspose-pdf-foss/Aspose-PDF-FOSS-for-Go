package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestTable_BorderSideBitmask(t *testing.T) {
	if pdf.BorderSideNone != 0 {
		t.Errorf("BorderSideNone = %d, want 0", pdf.BorderSideNone)
	}
	all := pdf.BorderSideTop | pdf.BorderSideRight | pdf.BorderSideBottom | pdf.BorderSideLeft
	if all != pdf.BorderSideAll {
		t.Errorf("composed All = %d, want BorderSideAll %d", all, pdf.BorderSideAll)
	}
}

func TestTable_BorderInfoZeroValue(t *testing.T) {
	var b pdf.BorderInfo
	if b.Sides != pdf.BorderSideNone || b.Width != 0 || b.Color != nil {
		t.Errorf("BorderInfo zero value = %+v, want zero/zero/nil", b)
	}
}

func TestTable_MarginInfoFields(t *testing.T) {
	m := pdf.MarginInfo{Top: 1, Right: 2, Bottom: 3, Left: 4}
	if m.Top != 1 || m.Right != 2 || m.Bottom != 3 || m.Left != 4 {
		t.Errorf("MarginInfo = %+v", m)
	}
}
