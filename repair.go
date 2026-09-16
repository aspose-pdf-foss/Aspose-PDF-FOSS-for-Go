// SPDX-License-Identifier: MIT

package asposepdf

// Damage reporting. A PDF whose cross-reference table is missing, truncated
// or simply wrong still opens here: Open falls back to scanning the file for
// object headers and rebuilding the table, and the parser tolerates a wrong
// stream /Length or a truncated Flate tail rather than failing the page.
// That recovery used to be silent, so a caller could neither log it nor gate
// on it (pdf-go-jn8g).
//
// There is deliberately no Repair() verb, unlike Aspose.PDF for .NET's
// Document.Repair: opening a damaged file already produces the recovered
// document, and saving it writes a clean one — the cross-reference table is
// rebuilt from the objects actually written, dangling references become null,
// and the trailer is regenerated. A method that only reported "done" would
// add a step without adding an effect.

// RepairReport describes what had to be reconstructed to open the document.
// The zero value means the file parsed as written.
//
// Scope: this reports structural recovery, decided while the file is being
// opened. Per-stream tolerance — a wrong /Length resolved by scanning for
// "endstream", a Flate stream with a bad checksum whose inflated bytes are
// kept — happens deep in the parser without a document in reach and is not
// counted here.
type RepairReport struct {
	// XRefReconstructed is set when the cross-reference table was rebuilt by
	// scanning the file for "N G obj" headers, because the one in the file
	// was missing, unparseable, or did not lead to a usable catalog.
	XRefReconstructed bool
	// ObjectsRecovered is how many objects that scan found. Zero unless
	// XRefReconstructed is set.
	ObjectsRecovered int
	// TrailerRecovered is set when the trailer had to be synthesised from a
	// /Type /Catalog object because the file's own trailer was missing or
	// carried no /Root.
	TrailerRecovered bool
}

// NeedsRepair reports whether the document was recovered rather than read as
// written. Saving it writes a clean file, so this is the moment to warn a
// user that the input was damaged.
func (d *Document) NeedsRepair() bool {
	return d.repair.XRefReconstructed
}

// RepairReport returns what had to be reconstructed to open the document.
// Mirrors the intent of Aspose.PDF for .NET's Document.IsRepairNeeded, which
// reports the same condition through an out parameter.
func (d *Document) RepairReport() RepairReport {
	return d.repair
}
