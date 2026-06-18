// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Incremental signing (ISO 32000-1 §7.5.6). Instead of rewriting the file,
// a signature is appended as a new revision: the original bytes are kept
// verbatim, followed by the new/modified objects, a cross-reference section
// whose /Prev points at the previous one, and a fresh trailer. Because no
// earlier byte moves, any signature already in the file stays valid — which
// is what makes multiple signatures possible.

// buildFreshEncryptedSignedPDF signs a freshly-built (never-opened) encrypted
// document: it serializes the encrypted PDF without the signature, reopens it
// to obtain a source + preserved encryption state, then signs that
// incrementally. This keeps the output identical in shape to signing an
// existing encrypted file (which is the only interoperable form).
func buildFreshEncryptedSignedPDF(d *Document) ([]byte, error) {
	password := ""
	if d.encrypt != nil {
		password = d.encrypt.userPassword
	}
	saved := d.sign
	d.sign = nil
	encBytes, err := buildDocumentPDF(d)
	d.sign = saved
	if err != nil {
		return nil, err
	}
	reopened, err := OpenStreamWithPassword(bytes.NewReader(encBytes), password)
	if err != nil {
		return nil, fmt.Errorf("sign: reopening the encrypted document failed: %w", err)
	}
	cfg := *saved
	cfg.incremental = true
	reopened.sign = &cfg
	// Map the 1-based page to the reopened document (page objects are equivalent).
	return buildIncrementalSignedPDF(reopened)
}

// buildIncrementalSignedPDF appends a signature revision to d.source and
// returns the complete signed bytes (with the PKCS#7 spliced in).
func buildIncrementalSignedPDF(d *Document) ([]byte, error) {
	if len(d.source) == 0 {
		return nil, fmt.Errorf("sign: incremental signing requires a document opened from an existing PDF")
	}
	if d.catalogNum == 0 {
		return nil, fmt.Errorf("sign: cannot determine catalog object number for incremental signing")
	}

	// Encryption: the original bytes stay verbatim (already encrypted); only
	// the appended signature objects are encrypted, with the same per-object
	// scheme. The signature /Contents and /ByteRange stay plaintext (pdfRaw).
	var encState *encryptState
	if d.preserved != nil {
		encState = d.preserved
	} else if d.encrypt != nil {
		var err error
		if encState, err = newEncryptState(d.encrypt); err != nil {
			return nil, fmt.Errorf("sign: %w", err)
		}
	}

	prevXref, err := lastStartxref(d.source)
	if err != nil {
		return nil, err
	}

	// New objects must start above every object number in the original file.
	// d.nextID only reflects objects kept in memory — the /Pages and /Catalog
	// nodes were dropped on open but still occupy numbers in the file — so
	// honor the original /Size captured from the trailer to avoid collisions.
	if d.nextID < d.origSize {
		d.nextID = d.origSize
	}

	when := d.sign.when
	if when.IsZero() {
		when = time.Now()
	}

	// modified holds new values for EXISTING objects (keyed by their original
	// number); newly created objects are added straight into d.objects with
	// numbers continuing from d.nextID and collected afterwards.
	modified := map[int]pdfValue{}
	baseNextID := d.nextID

	// --- Signature dictionary ---
	sigNum := d.nextID
	d.nextID++
	subFilter := pdfName("/adbe.pkcs7.detached")
	if d.sign.padES {
		subFilter = "/ETSI.CAdES.detached"
	}
	sigDict := pdfDict{
		"/Type":      pdfName("/Sig"),
		"/Filter":    pdfName("/Adobe.PPKLite"),
		"/SubFilter": subFilter,
		"/ByteRange": pdfRaw([]byte(byteRangePlaceholder)),
		"/Contents":  pdfRaw(contentsPlaceholder()),
		"/M":         pdfDateString(when),
	}
	if d.sign.name != "" {
		sigDict["/Name"] = d.sign.name
	}
	if d.sign.reason != "" {
		sigDict["/Reason"] = d.sign.reason
	}
	if d.sign.location != "" {
		sigDict["/Location"] = d.sign.location
	}
	if d.sign.contact != "" {
		sigDict["/ContactInfo"] = d.sign.contact
	}
	if d.sign.certify > NotCertified {
		sigDict["/Reference"] = pdfArray{pdfDict{
			"/Type":            pdfName("/SigRef"),
			"/TransformMethod": pdfName("/DocMDP"),
			"/TransformParams": pdfDict{
				"/Type": pdfName("/TransformParams"),
				"/P":    int(d.sign.certify),
				"/V":    pdfName("/1.2"),
			},
		}}
	}

	// --- Signature field / widget annotation ---
	pageIdx := 0
	if d.sign.page > 0 {
		pageIdx = d.sign.page - 1
	}
	page := d.pages[pageIdx]
	fieldNum := d.nextID
	d.nextID++
	fieldDict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Sig"),
		"/T":       d.uniqueSignatureFieldName(),
		"/V":       pdfRef{Num: sigNum},
		"/Rect":    pdfArray{0.0, 0.0, 0.0, 0.0},
		"/F":       132, // Print (4) + Locked (128)
		"/P":       pdfRef{Num: page.Num},
	}
	if d.sign.visible {
		r := d.sign.rect
		fieldDict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
		apID := d.nextID
		d.nextID++
		ap := generateSignatureAppearance(d.sign, d, when, r.URX-r.LLX, r.URY-r.LLY)
		d.objects[apID] = &pdfObject{Num: apID, Value: ap}
		fieldDict["/AP"] = pdfDict{"/N": pdfRef{Num: apID}}
	}
	d.objects[sigNum] = &pdfObject{Num: sigNum, Value: sigDict}
	d.objects[fieldNum] = &pdfObject{Num: fieldNum, Value: fieldDict}

	// --- /AcroForm: append the field, set /SigFlags. The catalog is only
	// emitted when its own bytes change (inline/new AcroForm or DocMDP). ---
	cat := deepCopyValue(pdfValue(d.catalog)).(pdfDict)
	catModified := false
	switch af := d.catalog["/AcroForm"].(type) {
	case pdfRef:
		afObj := d.objects[af.Num]
		afDict, ok := afObj.Value.(pdfDict)
		if !ok {
			return nil, fmt.Errorf("sign: /AcroForm is not a dictionary")
		}
		nd := deepCopyValue(pdfValue(afDict)).(pdfDict)
		appendFieldIncremental(d, nd, pdfRef{Num: fieldNum}, modified)
		nd["/SigFlags"] = 3
		modified[af.Num] = nd
	case pdfDict:
		nd := deepCopyValue(pdfValue(af)).(pdfDict)
		appendFieldIncremental(d, nd, pdfRef{Num: fieldNum}, modified)
		nd["/SigFlags"] = 3
		cat["/AcroForm"] = nd
		catModified = true
	default:
		cat["/AcroForm"] = pdfDict{"/Fields": pdfArray{pdfRef{Num: fieldNum}}, "/SigFlags": 3}
		catModified = true
	}
	if d.sign.certify > NotCertified {
		cat["/Perms"] = pdfDict{"/DocMDP": pdfRef{Num: sigNum}}
		catModified = true
	}
	if catModified {
		cat["/Type"] = pdfName("/Catalog")
		modified[d.catalogNum] = cat
	}

	// --- Page /Annots: add the widget. ---
	pageObj := d.objects[page.Num]
	if pageObj == nil {
		return nil, fmt.Errorf("sign: page object %d not found", page.Num)
	}
	pageDict := deepCopyValue(pageObj.Value).(pdfDict)
	if appendAnnotIncremental(d, pageDict, pdfRef{Num: fieldNum}, modified) {
		modified[page.Num] = pageDict
	}

	// --- Collect everything to emit (new objects + modified existing). ---
	type emitObj struct {
		num, gen int
		val      pdfValue
	}
	var emit []emitObj
	for n := baseNextID; n < d.nextID; n++ {
		if obj := d.objects[n]; obj != nil {
			emit = append(emit, emitObj{num: n, gen: 0, val: obj.Value})
		}
	}
	for num, val := range modified {
		gen := 0
		if obj := d.objects[num]; obj != nil {
			gen = obj.Gen
		}
		emit = append(emit, emitObj{num: num, gen: gen, val: val})
	}
	sort.Slice(emit, func(i, j int) bool { return emit[i].num < emit[j].num })

	// --- Serialize: original bytes, then the appended revision. ---
	var buf bytes.Buffer
	buf.Write(d.source)
	if d.source[len(d.source)-1] != '\n' {
		buf.WriteByte('\n')
	}

	identity := func(n int) int { return n }
	offsets := make(map[int]int64, len(emit))
	for _, e := range emit {
		offsets[e.num] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			num, gen := e.num, e.gen
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(num, gen, b) }
		}
		fmt.Fprintf(&buf, "%d %d obj\n", e.num, e.gen)
		if err := writeValue(&buf, e.val, identity, encFn); err != nil {
			return nil, err
		}
		buf.WriteString("\nendobj\n")
	}

	// --- Cross-reference section (classic table) + trailer. ---
	size := d.origSize
	for _, e := range emit {
		if e.num+1 > size {
			size = e.num + 1
		}
	}
	rows := make([]xrefRow, len(emit))
	for i, e := range emit {
		rows[i] = xrefRow{num: e.num, gen: e.gen, off: offsets[e.num]}
	}
	xrefOff := int64(buf.Len())
	writeIncrementalXref(&buf, rows)

	id0, id1 := d.incrementalID()
	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, " /Size %d", size)
	fmt.Fprintf(&buf, " /Root %d 0 R", d.catalogNum)
	fmt.Fprintf(&buf, " /Prev %d", prevXref)
	// Repeat /Encrypt so the newest trailer still marks the file encrypted; the
	// original /Encrypt object is untouched in the preserved source bytes.
	if encState != nil && d.encryptObjNum > 0 {
		fmt.Fprintf(&buf, " /Encrypt %d 0 R", d.encryptObjNum)
	}
	buf.WriteString(" /ID [")
	writeHexBytes(&buf, id0)
	writeHexBytes(&buf, id1)
	buf.WriteString("] >>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)

	return d.applySignature(buf.Bytes())
}

// xrefRow is one entry to write into the incremental xref table.
type xrefRow struct {
	num, gen int
	off      int64
}

// writeIncrementalXref writes a classic xref table covering rows (sorted by
// number), grouping consecutive numbers into subsections.
func writeIncrementalXref(buf *bytes.Buffer, rows []xrefRow) {
	buf.WriteString("xref\n")
	i := 0
	for i < len(rows) {
		j := i
		for j+1 < len(rows) && rows[j+1].num == rows[j].num+1 {
			j++
		}
		fmt.Fprintf(buf, "%d %d\n", rows[i].num, j-i+1)
		for k := i; k <= j; k++ {
			fmt.Fprintf(buf, "%010d %05d n \n", rows[k].off, rows[k].gen)
		}
		i = j + 1
	}
}

// appendFieldIncremental appends fieldRef to acro["/Fields"], handling the
// inline-array and indirect-array forms. For the indirect form the array
// object is recorded in modified; acro itself is a deep copy the caller owns.
func appendFieldIncremental(d *Document, acro pdfDict, fieldRef pdfRef, modified map[int]pdfValue) {
	switch fields := acro["/Fields"].(type) {
	case pdfArray:
		acro["/Fields"] = append(append(pdfArray{}, fields...), fieldRef)
	case pdfRef:
		if obj := d.objects[fields.Num]; obj != nil {
			if arr, ok := obj.Value.(pdfArray); ok {
				modified[fields.Num] = append(append(pdfArray{}, arr...), fieldRef)
				return
			}
		}
		acro["/Fields"] = pdfArray{fieldRef}
	default:
		acro["/Fields"] = pdfArray{fieldRef}
	}
}

// appendAnnotIncremental adds annotRef to page /Annots. Returns true when the
// page dict itself changed (inline/absent /Annots); for an indirect /Annots
// only the referenced array object is recorded in modified.
func appendAnnotIncremental(d *Document, page pdfDict, annotRef pdfRef, modified map[int]pdfValue) bool {
	switch ann := page["/Annots"].(type) {
	case pdfArray:
		page["/Annots"] = append(append(pdfArray{}, ann...), annotRef)
		return true
	case pdfRef:
		if obj := d.objects[ann.Num]; obj != nil {
			if arr, ok := obj.Value.(pdfArray); ok {
				modified[ann.Num] = append(append(pdfArray{}, arr...), annotRef)
				return false
			}
		}
		page["/Annots"] = pdfArray{annotRef}
		return true
	default:
		page["/Annots"] = pdfArray{annotRef}
		return true
	}
}

// uniqueSignatureFieldName returns a /T not already used by a signature
// field, so a second signature does not collide with "Signature1".
func (d *Document) uniqueSignatureFieldName() string {
	used := map[string]bool{}
	for _, sf := range d.collectSignatureFields() {
		used[sf.name] = true
	}
	for i := 1; ; i++ {
		name := "Signature" + strconv.Itoa(i)
		if !used[name] {
			return name
		}
	}
}

// incrementalID returns the two /ID elements for the appended trailer,
// reusing the original file identity when present and deriving one otherwise.
func (d *Document) incrementalID() ([]byte, []byte) {
	get := func(v pdfValue) []byte {
		switch s := v.(type) {
		case pdfHexString:
			return []byte(s)
		case string:
			return []byte(s)
		}
		return nil
	}
	if len(d.docID) == 2 {
		id0, id1 := get(d.docID[0]), get(d.docID[1])
		if id0 != nil && id1 != nil {
			return id0, id1
		}
	}
	sum := md5.Sum(d.source)
	return sum[:], sum[:]
}

// lastStartxref returns the byte offset referenced by the file's final
// startxref keyword — the previous cross-reference section for /Prev.
func lastStartxref(src []byte) (int64, error) {
	idx := bytes.LastIndex(src, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("sign: no startxref in source")
	}
	rest := src[idx+len("startxref"):]
	i := 0
	for i < len(rest) && (rest[i] == '\r' || rest[i] == '\n' || rest[i] == ' ' || rest[i] == '\t') {
		i++
	}
	j := i
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == i {
		return 0, fmt.Errorf("sign: malformed startxref")
	}
	n, err := strconv.ParseInt(string(rest[i:j]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sign: bad startxref offset: %w", err)
	}
	return n, nil
}
