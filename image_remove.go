// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strconv"
)

// serializeContentOps converts parsed operators back to content stream bytes.
func serializeContentOps(ops []contentOp) []byte {
	var buf bytes.Buffer
	for _, op := range ops {
		if op.Operator == "BI" {
			// Inline image: write BI, dict key/values, ID, data, EI.
			serializeInlineImage(&buf, op)
			continue
		}
		for i, operand := range op.Operands {
			if i > 0 {
				buf.WriteByte(' ')
			}
			serializeOperand(&buf, operand)
		}
		if len(op.Operands) > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(op.Operator)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func serializeOperand(buf *bytes.Buffer, v pdfValue) {
	switch val := v.(type) {
	case int:
		buf.WriteString(strconv.Itoa(val))
	case float64:
		buf.WriteString(formatFloat(val))
	case pdfName:
		buf.WriteString(string(val))
	case string:
		buf.WriteByte('(')
		buf.WriteString(escapeLiteral(val))
		buf.WriteByte(')')
	case pdfArray:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(' ')
			}
			serializeOperand(buf, item)
		}
		buf.WriteByte(']')
	case pdfDict:
		buf.WriteString("<<")
		for k, dv := range val {
			buf.WriteString(k)
			buf.WriteByte(' ')
			serializeOperand(buf, dv)
			buf.WriteByte(' ')
		}
		buf.WriteString(">>")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case pdfNull:
		buf.WriteString("null")
	default:
		fmt.Fprintf(buf, "%v", val)
	}
}

// Remove removes the image from the page.
// Deletes the XObject reference from page resources and the drawing
// operators from the content stream. The underlying XObject remains
// in the document's object store (safe for shared references).
// Images inside Form XObjects are not supported; only page-level images
// can be removed.
func (info *ImageInfo) Remove() error {
	if info.page == nil || info.stream == nil {
		return fmt.Errorf("image info: no image data")
	}
	if info.Name == "" {
		return fmt.Errorf("remove image: inline images cannot be removed")
	}

	// 1. Remove from page resources.
	resources := info.page.pageResources()
	if resources != nil {
		xobjVal := resolveRef(info.page.doc.objects, resources["/XObject"])
		if xobjDict, ok := xobjVal.(pdfDict); ok {
			delete(xobjDict, info.Name)
		}
	}

	// 2. Remove drawing operators from content stream.
	data, err := info.page.contentStreams()
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}

	filtered := removeImageOps(ops, info.Name)
	newData := serializeContentOps(filtered)

	// 3. Replace content stream.
	newStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    newData,
		Decoded: true,
	}
	newID := info.page.doc.nextID
	info.page.doc.nextID++
	info.page.doc.objects[newID] = &pdfObject{Num: newID, Value: newStream}

	pageDict := info.page.pageDict()
	pageDict["/Contents"] = pdfRef{Num: newID}

	return nil
}

// removeImageOps removes the q...Do...Q block containing a Do for the given image name.
// Note: the entire enclosing q/Q block is removed. If other operators share the same
// block, they are removed too. This matches typical PDF structure where each image
// draw is wrapped in its own q/Q pair.
func removeImageOps(ops []contentOp, name string) []contentOp {
	// Find all Do operators for this name.
	var doIndices []int
	for i, op := range ops {
		if op.Operator == "Do" && len(op.Operands) >= 1 {
			if operandName(op.Operands[0]) == name {
				doIndices = append(doIndices, i)
			}
		}
	}
	if len(doIndices) == 0 {
		return ops
	}

	// For each Do, find the enclosing q...Q block.
	type removeRange struct {
		start, end int
	}
	var ranges []removeRange

	for _, doIdx := range doIndices {
		// Walk backward to find the matching q.
		qIdx := -1
		depth := 0
		for i := doIdx - 1; i >= 0; i-- {
			if ops[i].Operator == "Q" {
				depth++
			} else if ops[i].Operator == "q" {
				if depth == 0 {
					qIdx = i
					break
				}
				depth--
			}
		}

		// Walk forward to find the matching Q.
		qEndIdx := -1
		depth = 0
		for i := doIdx + 1; i < len(ops); i++ {
			if ops[i].Operator == "q" {
				depth++
			} else if ops[i].Operator == "Q" {
				if depth == 0 {
					qEndIdx = i
					break
				}
				depth--
			}
		}

		if qIdx >= 0 && qEndIdx >= 0 {
			ranges = append(ranges, removeRange{qIdx, qEndIdx})
		} else {
			// No enclosing q/Q — remove just the cm before Do and the Do itself.
			start := doIdx
			if doIdx > 0 && ops[doIdx-1].Operator == "cm" {
				start = doIdx - 1
			}
			ranges = append(ranges, removeRange{start, doIdx})
		}
	}

	// Build result excluding the remove ranges.
	removed := make(map[int]bool)
	for _, r := range ranges {
		for i := r.start; i <= r.end; i++ {
			removed[i] = true
		}
	}

	var result []contentOp
	for i, op := range ops {
		if !removed[i] {
			result = append(result, op)
		}
	}
	return result
}

func serializeInlineImage(buf *bytes.Buffer, op contentOp) {
	if len(op.Operands) < 2 {
		return
	}
	buf.WriteString("BI\n")
	if dict, ok := op.Operands[0].(pdfDict); ok {
		for k, v := range dict {
			buf.WriteString(k)
			buf.WriteByte(' ')
			serializeOperand(buf, v)
			buf.WriteByte('\n')
		}
	}
	buf.WriteString("ID ")
	if data, ok := op.Operands[1].(string); ok {
		buf.WriteString(data)
	}
	buf.WriteString("\nEI\n")
}
