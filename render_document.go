// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
	"io"
)

// pageRange returns the 1-based page numbers to render: the given selection, or
// every page (in order) when the selection is empty. Out-of-range numbers are
// rejected so a bad index fails fast instead of rendering a wrong page.
func (d *Document) pageRange(pageNums []int) ([]int, error) {
	n := d.PageCount()
	if len(pageNums) == 0 {
		all := make([]int, n)
		for i := range all {
			all[i] = i + 1
		}
		return all, nil
	}
	for _, p := range pageNums {
		if p < 1 || p > n {
			return nil, fmt.Errorf("render: page %d out of range 1..%d", p, n)
		}
	}
	return append([]int(nil), pageNums...), nil
}

// RenderTIFF renders the document to a single multi-page TIFF written to w. With
// no pageNums every page is rendered in order; otherwise just the listed 1-based
// pages, in the given order. Pages are rendered one at a time, so peak memory is
// a single page image. Mirrors Aspose.PDF for .NET's TiffDevice whole-document
// output.
func (d *Document) RenderTIFF(w io.Writer, opts RenderOptions, pageNums ...int) error {
	pages, err := d.pageRange(pageNums)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return fmt.Errorf("render tiff: document has no pages")
	}
	return encodeTIFF(w, len(pages), opts.dpi(), true, func(i int) (image.Image, error) {
		return d.RenderImage(pages[i], opts)
	})
}
