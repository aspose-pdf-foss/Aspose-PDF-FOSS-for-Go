package asposepdf

import (
	"testing"
)

func TestDestinationTypeNamedConstant(t *testing.T) {
	if int(DestinationTypeNamed) != 8 {
		t.Errorf("DestinationTypeNamed = %d, want 8 (after FitBV=7)", int(DestinationTypeNamed))
	}
}

func TestNewNamedDestination_Basic(t *testing.T) {
	doc := NewDocument(595, 842)
	nd := NewNamedDestination(doc, "chapter1")
	if nd == nil {
		t.Fatal("NewNamedDestination returned nil")
	}
	if nd.DestinationType() != DestinationTypeNamed {
		t.Errorf("DestinationType = %v, want DestinationTypeNamed", nd.DestinationType())
	}
	if nd.Name() != "chapter1" {
		t.Errorf("Name() = %q, want \"chapter1\"", nd.Name())
	}
}

func TestNamedDestination_UnresolvedReturnsNil(t *testing.T) {
	doc := NewDocument(595, 842)
	nd := NewNamedDestination(doc, "no-such-name")
	if nd.Resolve() != nil {
		t.Error("Resolve() should be nil for unregistered name")
	}
	if nd.Page() != nil {
		t.Error("Page() should be nil for unregistered name")
	}
}
