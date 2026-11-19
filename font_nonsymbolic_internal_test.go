// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strconv"
	"strings"
	"testing"
)

// A non-embedded simple font outside the Standard 14, with no /Encoding and
// no /ToUnicode, reads with StandardEncoding when its descriptor calls it
// nonsymbolic (pdf-go-k23c); one that says symbolic, or neither, stays
// unknown.
func TestNonsymbolicUnembeddedFontUsesStandardEncoding(t *testing.T) {
	for _, c := range []struct {
		name, subtype string
		flags         int
		want          string
	}{
		{"type1 nonsymbolic", "/Type1", 34, "Development ’quoted’"},
		{"truetype nonsymbolic", "/TrueType", 32, "Development ’quoted’"},
		{"symbolic", "/Type1", 4, ""},
		{"both bits", "/Type1", 36, ""},
		{"neither bit", "/Type1", 2, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := pageWith("", "", "<< /Font << /F2 5 0 R >> >>",
				"BT /F2 1 Tf 12 0 0 12 10 100 Tm (Development \\047quoted\\047) Tj ET")
			objs = append(objs,
				"<< /Type /Font /Subtype "+c.subtype+" /BaseFont /NewCenturySchlbk-Roman /FontDescriptor 6 0 R >>",
				"<< /Type /FontDescriptor /FontName /NewCenturySchlbk-Roman /Flags "+strconv.Itoa(c.flags)+
					" /FontBBox [-195 -250 1000 965] /ItalicAngle 0 /Ascent 737 /Descent -205 /CapHeight 722 /StemV 92 >>",
			)
			p := mustPage(t, openHardening(t, hardeningPDF(objs...)))
			txt, err := p.ExtractText()
			if err != nil {
				t.Fatal(err)
			}
			if c.want != "" {
				if txt != c.want {
					t.Errorf("text %q, want %q", txt, c.want)
				}
				ms, err := p.SearchText("Development")
				if err != nil || len(ms) != 1 {
					t.Errorf("search: %v %+v", err, ms)
				}
				return
			}
			if !strings.Contains(txt, "�") || strings.Contains(txt, "Development") {
				t.Errorf("symbolic font read as Latin: %q", txt)
			}
		})
	}
}
