// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// A reference to a missing object is null, and stays so: output objects are numbered
// afresh, so the old number would belong to some other object in the saved file.
func TestSaveDanglingReferenceIsNull(t *testing.T) {
	for _, c := range []struct {
		name  string
		write func(d *Document, buf *bytes.Buffer) error
	}{
		{"classic", func(d *Document, buf *bytes.Buffer) error { _, err := d.WriteTo(buf); return err }},
		{"object streams", func(d *Document, buf *bytes.Buffer) error {
			d.compressObjects = true
			_, err := d.WriteTo(buf)
			return err
		}},
		{"linearized", func(d *Document, buf *bytes.Buffer) error { _, err := d.WriteToLinearized(buf); return err }},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := NewDocument(100, 100)
			p, _ := d.Page(1)
			// An early number: the objects after it take it over when renumbered.
			missing := d.addObject(pdfDict{"/Gone": true})
			for i := range 4 {
				p.pageDict()["/Keep"+string(rune('A'+i))] = pdfRef{Num: d.addObject(pdfDict{"/Filler": i})}
			}
			delete(d.objects, missing)
			p.pageDict()["/Dangling"] = pdfRef{Num: missing}

			var buf bytes.Buffer
			if err := c.write(d, &buf); err != nil {
				t.Fatal(err)
			}
			d2, err := OpenStream(&buf)
			if err != nil {
				t.Fatal(err)
			}
			p2, _ := d2.Page(1)
			if v := resolveRef(d2.objects, p2.pageDict()["/Dangling"]); v != nil {
				if _, isNull := v.(pdfNull); !isNull {
					t.Errorf("the dangling reference reads as %v", v)
				}
			}
		})
	}
}

// A file's cross-reference and object streams describe that file, not the next one.
func TestSaveDropsSourceFileStructure(t *testing.T) {
	d := NewDocument(100, 100)
	p, _ := d.Page(1)
	if err := p.AddText("structure", TextStyle{}, Rectangle{LLX: 10, LLY: 10, URX: 90, URY: 90}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Optimize(DefaultOptimizationOptions()); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	if _, err := d.WriteTo(&packed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(packed.Bytes(), []byte("/XRef")) {
		t.Fatal("fixture has no cross-reference stream")
	}
	d2, err := OpenStream(bytes.NewReader(packed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := d2.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"/XRef", "/ObjStm"} {
		if bytes.Contains(out.Bytes(), []byte(s)) {
			t.Errorf("%s survived the save", s)
		}
	}
	d3, err := OpenStream(&out)
	if err != nil {
		t.Fatal(err)
	}
	p3, _ := d3.Page(1)
	if text, _ := p3.ExtractText(); text != "structure" {
		t.Errorf("text %q", text)
	}
}
