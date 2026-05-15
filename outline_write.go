package asposepdf

// outlineFlags returns the /F bit field per ISO 32000-1 §12.3.3 Table 153.
// bit 1 = italic, bit 2 = bold.
func outlineFlags(bold, italic bool) int {
	var f int
	if italic {
		f |= 1
	}
	if bold {
		f |= 2
	}
	return f
}

// visibleDescendantCount returns the magnitude used for the /Count entry
// per ISO 32000-1 §12.3.3:
//   Σ direct children + Σ (over expanded children) of count(child).
// The sign on the RESULT is applied by the caller — encodeOutlineItem
// negates if !node.IsExpanded() && node is not the root.
func visibleDescendantCount(node *OutlineItemCollection) int {
	total := len(node.children)
	for _, ch := range node.children {
		if ch.IsExpanded() {
			total += visibleDescendantCount(ch)
		}
	}
	return total
}

// encodeDestination produces the destination array per ISO 32000-1 §12.3.2.2.
// The page reference is a pdfDirectRef bypassing the writer's ID remap,
// so the array points at the page's actual emitted object number.
func encodeDestination(d Destination) pdfArray {
	if d == nil || d.Page() == nil {
		return nil
	}
	pageRef := pdfDirectRef{Num: d.Page().pageObj().Num}
	switch v := d.(type) {
	case *DestinationXYZ:
		return pdfArray{pageRef, pdfName("/XYZ"),
			optFloat(v.left, v.useLeft),
			optFloat(v.top, v.useTop),
			optFloat(v.zoom, v.useZoom),
		}
	case *DestinationFit:
		return pdfArray{pageRef, pdfName("/Fit")}
	case *DestinationFitH:
		return pdfArray{pageRef, pdfName("/FitH"), optFloat(v.top, v.useTop)}
	case *DestinationFitV:
		return pdfArray{pageRef, pdfName("/FitV"), optFloat(v.left, v.useLeft)}
	case *DestinationFitR:
		return pdfArray{pageRef, pdfName("/FitR"), v.left, v.bottom, v.right, v.top}
	case *DestinationFitB:
		return pdfArray{pageRef, pdfName("/FitB")}
	case *DestinationFitBH:
		return pdfArray{pageRef, pdfName("/FitBH"), optFloat(v.top, v.useTop)}
	case *DestinationFitBV:
		return pdfArray{pageRef, pdfName("/FitBV"), optFloat(v.left, v.useLeft)}
	}
	return nil
}

// optFloat returns v as float64 if use, else pdfNull{}.
func optFloat(v float64, use bool) pdfValue {
	if !use {
		return pdfNull{}
	}
	return v
}
