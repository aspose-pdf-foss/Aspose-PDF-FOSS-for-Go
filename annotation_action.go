package asposepdf

// ActionType identifies the kind of action attached to an annotation
// (typically a LinkAnnotation's /A entry).
type ActionType int

const (
	ActionTypeUnknown ActionType = iota
	ActionTypeGoToURI
	ActionTypeGoTo
	ActionTypeNamed
	ActionTypeSubmitForm
	ActionTypeResetForm
	ActionTypeJavaScript
)

// Action is the common interface implemented by every concrete action
// type. Action values are inline within the parent annotation's /A
// dict — they are not separately addressable PDF objects.
type Action interface {
	ActionType() ActionType
	// encode returns the /A dict representation of this action.
	encode() pdfDict
}

// GoToURIAction opens a URI in the user's default handler (typically a
// web browser).
type GoToURIAction struct {
	uri string
}

func (a *GoToURIAction) ActionType() ActionType { return ActionTypeGoToURI }

// URI returns the destination URI.
func (a *GoToURIAction) URI() string { return a.uri }

// SetURI updates the destination URI.
func (a *GoToURIAction) SetURI(uri string) { a.uri = uri }

func (a *GoToURIAction) encode() pdfDict {
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/URI"),
		"/URI":  a.uri,
	}
}

// NewGoToURIAction builds a /URI action. Empty URI is permitted but
// usually not what callers want.
func NewGoToURIAction(uri string) *GoToURIAction { return &GoToURIAction{uri: uri} }

// GoToAction navigates to a page within the same document. PageNum is
// 1-based; Top is the y-coordinate of the destination view in default
// user space.
type GoToAction struct {
	pageNum int
	top     float64
	doc     *Document // optional — set when read from existing PDF for resolving page refs
}

func (a *GoToAction) ActionType() ActionType { return ActionTypeGoTo }

// PageNum returns the 1-based destination page number.
func (a *GoToAction) PageNum() int { return a.pageNum }

// Top returns the y-coordinate of the destination view.
func (a *GoToAction) Top() float64 { return a.top }

// SetPageNum updates the destination page number (1-based).
func (a *GoToAction) SetPageNum(n int) { a.pageNum = n }

// SetTop updates the y-coordinate of the destination view.
func (a *GoToAction) SetTop(t float64) { a.top = t }

func (a *GoToAction) encode() pdfDict {
	// /D = [pageRef /XYZ left top zoom]. Per ISO 32000-1 §12.3.2.2 the
	// first element should be an indirect ref to the destination page.
	// When doc is set (action read from a PDF or rebuilt from one) we
	// emit the pdfRef form. As a fallback for actions constructed via
	// NewGoToAction without a doc, we emit pageNum-1 as an int — most
	// viewers accept this, though it is technically deprecated.
	first := a.pageNum - 1
	if first < 0 {
		first = 0
	}
	dest := pdfArray{first, pdfName("/XYZ"), pdfNull{}, a.top, pdfNull{}}
	if a.doc != nil && a.pageNum >= 1 && a.pageNum <= len(a.doc.pages) {
		dest = pdfArray{
			pdfRef{Num: a.doc.pages[a.pageNum-1].Num},
			pdfName("/XYZ"),
			pdfNull{},
			a.top,
			pdfNull{},
		}
	}
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/GoTo"),
		"/D":    dest,
	}
}

// NewGoToAction builds a /GoTo action targeting the given 1-based page
// and a y-coordinate (top of view).
func NewGoToAction(pageNum int, top float64) *GoToAction {
	return &GoToAction{pageNum: pageNum, top: top}
}

// parseGoToAction reads /D — supports the [pageRef /XYZ left top zoom]
// explicit destination form. Named destinations (/D as name or string)
// return PageNum=0; callers can detect via PageNum() == 0.
func parseGoToAction(d pdfDict) *GoToAction {
	dest, ok := d["/D"].(pdfArray)
	if !ok || len(dest) < 1 {
		return &GoToAction{}
	}
	a := &GoToAction{}
	switch first := dest[0].(type) {
	case pdfRef:
		// pageNum stays 0; LinkAnnotation.Action() resolves it.
		_ = first
	case int:
		a.pageNum = first + 1
	case float64:
		a.pageNum = int(first) + 1
	}
	if len(dest) >= 4 {
		t, _ := toFloat(dest[3])
		a.top = t
	}
	return a
}

// NamedActionType identifies one of the standard viewer commands
// supported by /Named actions per ISO 32000-1 §12.6.4.11.
type NamedActionType int

const (
	NamedActionUnknown   NamedActionType = iota
	NamedActionFirstPage
	NamedActionLastPage
	NamedActionNextPage
	NamedActionPrevPage
	NamedActionPrint
)

// NamedAction triggers a built-in viewer command (FirstPage, Print, ...).
type NamedAction struct {
	name NamedActionType
}

func (a *NamedAction) ActionType() ActionType    { return ActionTypeNamed }
func (a *NamedAction) Name() NamedActionType     { return a.name }
func (a *NamedAction) SetName(n NamedActionType) { a.name = n }

// namedActionNames is the canonical NamedActionType ↔ /N name mapping.
// Both encode and parse paths key off this single map so adding a new
// command requires updating exactly one place.
var namedActionNames = map[NamedActionType]pdfName{
	NamedActionFirstPage: "/FirstPage",
	NamedActionLastPage:  "/LastPage",
	NamedActionNextPage:  "/NextPage",
	NamedActionPrevPage:  "/PrevPage",
	NamedActionPrint:     "/Print",
}

func namedActionToPDF(n NamedActionType) pdfName {
	return namedActionNames[n]
}

func pdfNameToNamedAction(s pdfName) NamedActionType {
	for n, name := range namedActionNames {
		if name == s {
			return n
		}
	}
	return NamedActionUnknown
}

func (a *NamedAction) encode() pdfDict {
	// Guard against NamedActionUnknown / non-standard names: emit a
	// safe default so the resulting PDF stays well-formed. Callers
	// should not normally encode an Unknown action.
	n := namedActionToPDF(a.name)
	if n == "" {
		n = "/FirstPage"
	}
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/Named"),
		"/N":    n,
	}
}

// NewNamedAction builds a /Named action.
func NewNamedAction(n NamedActionType) *NamedAction {
	return &NamedAction{name: n}
}

// parseAction returns the matching concrete action type for a resolved
// /A dict. Caller resolves indirect refs before calling. Returns nil
// for unsupported subtypes (e.g. /Launch, /GoToR).
func parseAction(d pdfDict) Action {
	s, _ := d["/S"].(pdfName)
	switch s {
	case "/URI":
		uri := decodeFormString(d["/URI"])
		return &GoToURIAction{uri: uri}
	case "/GoTo":
		return parseGoToAction(d)
	case "/Named":
		n, _ := d["/N"].(pdfName)
		return &NamedAction{name: pdfNameToNamedAction(n)}
	}
	return nil
}
