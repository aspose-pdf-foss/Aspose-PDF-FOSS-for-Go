// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestNamedDestinations_CrossOutlineRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Chapter 1")
	oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
	doc.Outlines().Add(oic)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	nd, _ := doc2.Outlines().At(0).Destination().(*pdf.NamedDestination)
	if nd == nil || nd.Name() != "ch1" {
		t.Fatalf("outline named-dest lost; got %v", doc2.Outlines().At(0).Destination())
	}
	inner := nd.Resolve()
	if inner == nil || inner.DestinationType() != pdf.DestinationTypeFit {
		t.Errorf("Resolve = %v", inner)
	}
}

func TestNamedDestinations_CrossAES128(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("secret", pdf.NewDestinationXYZ(page, 50, 700, 1)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	if doc2.NamedDestinations().Get("secret") == nil {
		t.Error("named dest lost through AES-128 roundtrip")
	}
}

func TestNamedDestinations_CrossAES256(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("vault", pdf.NewDestinationFit(page)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	oic := pdf.NewOutlineItemCollection(doc)
	oic.SetTitle("Vault")
	oic.SetDestination(pdf.NewNamedDestination(doc, "vault"))
	doc.Outlines().Add(oic)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	if doc2.NamedDestinations().Get("vault") == nil {
		t.Error("named dest lost through AES-256 roundtrip")
	}
	if doc2.Outlines().At(0).Destination() == nil {
		t.Error("outline named-dest reference lost")
	}
}
