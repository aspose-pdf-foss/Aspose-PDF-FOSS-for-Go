// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// Aspose .NET sample: register named destinations
//   doc.NamedDestinations.Add("ch1",
//       new XYZExplicitDestination(doc.Pages[1], 0, 800, 1));
//   doc.NamedDestinations.Add("appendix",
//       new FitExplicitDestination(doc.Pages[1]));
func TestAsposeParity_NamedDestinationsAdd(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	if err := nd.Add("ch1", pdf.NewDestinationXYZ(page, 0, 800, 1)); err != nil {
		t.Fatal(err)
	}
	if err := nd.Add("appendix", pdf.NewDestinationFit(page)); err != nil {
		t.Fatal(err)
	}
	if nd.Count() != 2 {
		t.Errorf("Count = %d, want 2", nd.Count())
	}
}

// Aspose .NET sample: outline pointing at named destination
//   OutlineItemCollection oic = new OutlineItemCollection(doc.Outlines);
//   oic.Title = "Chapter 1";
//   oic.Destination = new NamedDestination("ch1");
//   doc.Outlines.Add(oic);
func TestAsposeParity_OutlineWithNamedDest(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page)); err != nil {
		t.Fatal(err)
	}
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Chapter 1")
	oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
	if err := doc.Outlines().Add(oic); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample: indexer lookup
//   IAppointment dest = doc.NamedDestinations["ch1"];
func TestAsposeParity_IndexerLookup(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page)); err != nil {
		t.Fatal(err)
	}
	dest := doc.NamedDestinations().Get("ch1")
	if dest == nil {
		t.Error("Get('ch1') returned nil")
	}
}

// Aspose .NET sample: ContainsKey + Remove + Count
//   if (doc.NamedDestinations.ContainsKey("old")) { doc.NamedDestinations.Remove("old"); }
//   int n = doc.NamedDestinations.Count;
func TestAsposeParity_ContainsRemoveCount(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("old", pdf.NewDestinationFit(page)); err != nil {
		t.Fatal(err)
	}
	if !doc.NamedDestinations().Has("old") {
		t.Fatal("Has should report true")
	}
	doc.NamedDestinations().Remove("old")
	if doc.NamedDestinations().Count() != 0 {
		t.Error("Count after Remove != 0")
	}
}
