package asposepdf

// pdfValue is any PDF value: dict, array, string, name, int, float, bool, null, ref, stream.
type pdfValue = any

// pdfNull represents PDF null.
type pdfNull struct{}

// pdfRef is an indirect reference "n g R".
type pdfRef struct {
	Num int
	Gen int
}

// pdfDirectRef is like pdfRef but writeValue outputs it without remapping.
// Use this when the object number is already in the output (new) space.
type pdfDirectRef struct {
	Num int
	Gen int
}

// pdfName is a PDF name object like /Name.
type pdfName string

// pdfHexString is a byte sequence that writeValue always emits as a PDF hex
// string (<…>), regardless of whether an encryption function is active.
// Used for /O and /U in the /Encrypt dictionary, where the value is raw
// binary (including embedded NULs) and literal-string encoding is a known
// interop trap.
type pdfHexString []byte

// pdfDict is a PDF dictionary.
type pdfDict map[string]pdfValue

// pdfArray is a PDF array.
type pdfArray []pdfValue

// pdfStream is a PDF stream object.
type pdfStream struct {
	Dict    pdfDict
	Data    []byte // decompressed when Decoded==true; raw otherwise
	Decoded bool   // true if Data has been successfully decompressed
}

// pdfObject is an indirect object "n g obj ... endobj".
type pdfObject struct {
	Num   int
	Gen   int
	Value pdfValue // dict, stream, array, etc.
}

// xrefEntry describes where to find a PDF object.
type xrefEntry struct {
	Offset       int64 // byte offset in file (type 1)
	Compressed   bool  // type 2: stored inside an object stream
	StreamObjNum int   // parent stream object number (type 2)
	StreamIndex  int   // index inside the stream (type 2)
	Free         bool  // type 0: free object
}
