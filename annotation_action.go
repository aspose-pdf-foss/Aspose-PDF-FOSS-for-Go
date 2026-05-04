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

// parseAction reads a /A value (inline dict or indirect ref) and
// returns the matching concrete action type. Returns nil for
// unsupported subtypes (e.g. /Launch, /GoToR) or unresolvable values.
func parseAction(objects map[int]*pdfObject, v pdfValue) Action {
	d, ok := resolveRefToDict(objects, v)
	if !ok {
		return nil
	}
	s, _ := d["/S"].(pdfName)
	switch s {
	case "/URI":
		uri := decodeFormString(d["/URI"])
		return &GoToURIAction{uri: uri}
	}
	return nil
}
