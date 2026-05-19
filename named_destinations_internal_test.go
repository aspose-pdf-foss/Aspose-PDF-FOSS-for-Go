package asposepdf

import (
	"bytes"
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

func TestBuildNamedDestTree_Empty(t *testing.T) {
	doc := NewDocument(595, 842)
	treeRef, namesDictRef, objs := buildNamedDestTree(doc)
	if treeRef.Num != 0 || namesDictRef.Num != 0 || len(objs) != 0 {
		t.Errorf("empty doc: treeRef=%v namesDictRef=%v objCount=%d, want zeros", treeRef, namesDictRef, len(objs))
	}
}

func TestBuildNamedDestTree_FlatShape(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	nd.Add("alpha", NewDestinationFit(page))
	nd.Add("beta", NewDestinationFit(page))
	nd.Add("gamma", NewDestinationFit(page))
	treeRef, namesDictRef, objs := buildNamedDestTree(doc)
	if treeRef.Num == 0 || namesDictRef.Num == 0 {
		t.Fatal("refs should be non-zero")
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects (tree root + /Names dict), got %d", len(objs))
	}
	// Find tree root by /Names key.
	var treeRoot pdfDict
	for _, o := range objs {
		if d, ok := o.Value.(pdfDict); ok {
			if _, hasNames := d["/Names"]; hasNames {
				treeRoot = d
			}
		}
	}
	if treeRoot == nil {
		t.Fatal("no tree root found")
	}
	// /Names array: 3 names × 2 = 6 entries (name, dest, name, dest, ...).
	namesArr, _ := treeRoot["/Names"].(pdfArray)
	if len(namesArr) != 6 {
		t.Errorf("/Names len = %d, want 6", len(namesArr))
	}
	// Lex order check.
	if namesArr[0] != "alpha" || namesArr[2] != "beta" || namesArr[4] != "gamma" {
		t.Errorf("/Names not lex-sorted: %v %v %v", namesArr[0], namesArr[2], namesArr[4])
	}
	// /Limits.
	limits, _ := treeRoot["/Limits"].(pdfArray)
	if len(limits) != 2 || limits[0] != "alpha" || limits[1] != "gamma" {
		t.Errorf("/Limits wrong: %v", limits)
	}
}

func TestBuildNamedDestTree_SkipsNestedNamedDest(t *testing.T) {
	// Direct call simulating defensive write (Add already rejects this,
	// but the writer must defend too).
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	nd := doc.NamedDestinations()
	nd.Add("real", NewDestinationFit(page))
	// Bypass Add validation by writing directly into the map.
	nd.entries["loop"] = &NamedDestination{doc: doc, name: "real"}
	treeRef, _, _ := buildNamedDestTree(doc)
	if treeRef.Num == 0 {
		t.Fatal("should still emit (real entry survives)")
	}
	_ = bytes.Buffer{} // keep import used
}

func TestWalkNameTree_FlatLeaf(t *testing.T) {
	doc := NewDocument(595, 842)
	leaf := pdfDict{
		"/Names": pdfArray{
			"alpha", pdfArray{pdfRef{Num: 999}, pdfName("/Fit")},
			"beta", pdfArray{pdfRef{Num: 999}, pdfName("/Fit")},
		},
	}
	visited := map[string]bool{}
	walkNameTree(doc, leaf, func(name string, val pdfValue) {
		visited[name] = true
	})
	if !visited["alpha"] || !visited["beta"] {
		t.Errorf("visited = %v, want alpha + beta", visited)
	}
}

func TestWalkNameTree_KidsHierarchy(t *testing.T) {
	doc := NewDocument(595, 842)
	leafA := pdfDict{
		"/Names": pdfArray{"a", pdfArray{pdfRef{Num: 99}, pdfName("/Fit")}},
	}
	leafB := pdfDict{
		"/Names": pdfArray{"b", pdfArray{pdfRef{Num: 99}, pdfName("/Fit")}},
	}
	leafAID := doc.nextID
	doc.nextID++
	doc.objects[leafAID] = &pdfObject{Num: leafAID, Value: leafA}
	leafBID := doc.nextID
	doc.nextID++
	doc.objects[leafBID] = &pdfObject{Num: leafBID, Value: leafB}
	root := pdfDict{
		"/Kids": pdfArray{pdfRef{Num: leafAID}, pdfRef{Num: leafBID}},
	}
	visited := map[string]bool{}
	walkNameTree(doc, root, func(name string, val pdfValue) {
		visited[name] = true
	})
	if !visited["a"] || !visited["b"] {
		t.Errorf("visited = %v, want a + b", visited)
	}
}

func TestWalkNameTree_Cycle(t *testing.T) {
	doc := NewDocument(595, 842)
	// Build a cycle that ALSO has a reachable leaf, so we can distinguish
	// "cycle detected (visits leaf once)" from "infinite recursion (would
	// visit leaf many times before depth cap or test timeout)".
	leafID := doc.nextID
	doc.nextID++
	leaf := pdfDict{
		"/Names": pdfArray{"leafKey", pdfArray{pdfRef{Num: 99}, pdfName("/Fit")}},
	}
	doc.objects[leafID] = &pdfObject{Num: leafID, Value: leaf}

	rootID := doc.nextID
	doc.nextID++
	// Root's /Kids points to the leaf AND back to itself — the self-cycle
	// would re-visit the leaf forever without cycle protection.
	cycle := pdfDict{
		"/Kids": pdfArray{pdfRef{Num: leafID}, pdfRef{Num: rootID}},
	}
	doc.objects[rootID] = &pdfObject{Num: rootID, Value: cycle}

	visits := 0
	walkNameTree(doc, pdfRef{Num: rootID}, func(name string, val pdfValue) {
		visits++
	})
	if visits != 1 {
		t.Errorf("cycle protection failed: leaf visited %d times, want exactly 1", visits)
	}
}

func TestParseDestinationAny_Array(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")}
	d := parseDestinationAny(doc, arr)
	if d == nil {
		t.Fatal("nil")
	}
	if d.DestinationType() != DestinationTypeFit {
		t.Errorf("type = %v", d.DestinationType())
	}
}

func TestParseDestinationAny_DictWithD(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	dict := pdfDict{
		"/D": pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")},
	}
	d := parseDestinationAny(doc, dict)
	if d == nil || d.DestinationType() != DestinationTypeFit {
		t.Errorf("/D-wrapped parsing failed: %v", d)
	}
}
