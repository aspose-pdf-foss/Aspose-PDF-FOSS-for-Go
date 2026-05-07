package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestTextIconConstants(t *testing.T) {
	all := []pdf.TextIcon{
		pdf.TextIconUnknown,
		pdf.TextIconComment,
		pdf.TextIconKey,
		pdf.TextIconNote,
		pdf.TextIconHelp,
		pdf.TextIconNewParagraph,
		pdf.TextIconParagraph,
		pdf.TextIconInsert,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("TextIcon[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestFreeTextIntentConstants(t *testing.T) {
	if pdf.FreeTextIntentFreeText != 0 {
		t.Errorf("FreeTextIntentFreeText = %d, want 0", pdf.FreeTextIntentFreeText)
	}
	all := []pdf.FreeTextIntent{
		pdf.FreeTextIntentFreeText,
		pdf.FreeTextIntentCallout,
		pdf.FreeTextIntentTypewriter,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("FreeTextIntent[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestBorderEffectConstants(t *testing.T) {
	if pdf.BorderEffectNone != 0 {
		t.Errorf("BorderEffectNone = %d, want 0", pdf.BorderEffectNone)
	}
	if pdf.BorderEffectCloudy != 1 {
		t.Errorf("BorderEffectCloudy = %d, want 1", pdf.BorderEffectCloudy)
	}
}

func TestStampNameConstants(t *testing.T) {
	all := []pdf.StampName{
		pdf.StampNameUnknown,
		pdf.StampNameApproved,
		pdf.StampNameAsIs,
		pdf.StampNameConfidential,
		pdf.StampNameDepartmental,
		pdf.StampNameDraft,
		pdf.StampNameExperimental,
		pdf.StampNameExpired,
		pdf.StampNameFinal,
		pdf.StampNameForComment,
		pdf.StampNameForPublicRelease,
		pdf.StampNameNotApproved,
		pdf.StampNameNotForPublicRelease,
		pdf.StampNameSold,
		pdf.StampNameTopSecret,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("StampName[%d] = %d, want %d", i, int(v), i)
		}
	}
}
