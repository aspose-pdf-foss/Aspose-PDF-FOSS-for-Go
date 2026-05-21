package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// These tests are line-by-line translations of Aspose.PDF for .NET
// sample code. They don't add behavioral coverage beyond outline_test.go
// — they exist as executable proof that the Go API is shaped exactly
// like the .NET one. The comments above each test show the original
// .NET code so a .NET migrant can read both side-by-side.

// Aspose .NET sample: "Add bookmark"
//   OutlineItemCollection chapter = new OutlineItemCollection(doc.Outlines);
//   chapter.Title = "Chapter 1";
//   chapter.Bold = true;
//   chapter.Italic = false;
//   doc.Outlines.Add(chapter);
func TestAsposeParity_AddBookmark(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetTitle("Chapter 1")
	chapter.SetBold(true)
	chapter.SetItalic(false)
	if err := doc.Outlines().Add(chapter); err != nil {
		t.Fatal(err)
	}
	if doc.Outlines().Count() != 1 || doc.Outlines().At(0).Title() != "Chapter 1" {
		t.Error("Aspose parity: AddBookmark failed")
	}
}

// Aspose .NET sample: "XYZ destination"
//   chapter.Destination = new XYZExplicitDestination(doc.Pages[1], 0, 800, 1);
func TestAsposeParity_DestinationXYZ(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetTitle("Chapter 1")
	chapter.SetDestination(pdf.NewDestinationXYZ(page, 0, 800, 1))
	doc.Outlines().Add(chapter)
	dest := chapter.Destination()
	if dest.DestinationType() != pdf.DestinationTypeXYZ {
		t.Errorf("DestType = %v", dest.DestinationType())
	}
}

// Aspose .NET sample: nested children
//   OutlineItemCollection chapter = ...;
//   OutlineItemCollection section = new OutlineItemCollection(doc.Outlines);
//   section.Title = "Section 1.1";
//   chapter.Add(section);
//   doc.Outlines.Add(chapter);
func TestAsposeParity_NestedChildren(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetTitle("Chapter 1")
	section := pdf.NewOutlineItemCollection(doc)
	section.SetTitle("Section 1.1")
	chapter.Add(section)
	doc.Outlines().Add(chapter)
	if chapter.Count() != 1 || chapter.At(0).Title() != "Section 1.1" {
		t.Error("Aspose parity: NestedChildren failed")
	}
}

// Aspose .NET sample: Bold + Italic + Color
//   chapter.Bold = true; chapter.Italic = true;
//   chapter.Color = System.Drawing.Color.Red;
func TestAsposeParity_BoldItalicColor(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetBold(true)
	chapter.SetItalic(true)
	chapter.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	if !chapter.Bold() || !chapter.Italic() || chapter.Color() == nil {
		t.Error("Aspose parity: BoldItalicColor failed")
	}
}

// Aspose .NET sample: GoToAction
//   chapter.Action = new GoToAction(doc.Pages[2]);
func TestAsposeParity_GoToAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	chapter := pdf.NewOutlineItemCollection(doc)
	chapter.SetAction(pdf.NewGoToAction(2, 0))
	doc.Outlines().Add(chapter)
	if chapter.Action() == nil {
		t.Error("Aspose parity: GoToAction failed")
	}
}

// Aspose .NET sample: Insert + RemoveAt
//   doc.Outlines.Insert(0, item);
//   doc.Outlines.RemoveAt(0);
func TestAsposeParity_InsertRemoveAt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	item := pdf.NewOutlineItemCollection(doc)
	item.SetTitle("X")
	if err := doc.Outlines().Insert(0, item); err != nil {
		t.Fatal(err)
	}
	if doc.Outlines().Count() != 1 {
		t.Errorf("Count after Insert = %d", doc.Outlines().Count())
	}
	if err := doc.Outlines().RemoveAt(0); err != nil {
		t.Fatal(err)
	}
	if doc.Outlines().Count() != 0 {
		t.Error("Count after RemoveAt != 0")
	}
}
