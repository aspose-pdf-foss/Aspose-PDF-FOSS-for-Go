// SPDX-License-Identifier: MIT

package asposepdf

// DestinationType identifies the destination flavor.
type DestinationType int

const (
	DestinationTypeXYZ DestinationType = iota
	DestinationTypeFit
	DestinationTypeFitH
	DestinationTypeFitV
	DestinationTypeFitR
	DestinationTypeFitB
	DestinationTypeFitBH
	DestinationTypeFitBV
	DestinationTypeNamed // named destination reference via collection lookup
)

// Destination is the common interface for all explicit destinations.
// Per ISO 32000-1 §12.3.2.2.
type Destination interface {
	DestinationType() DestinationType
	Page() *Page
}

// DestinationXYZ — [page /XYZ left top zoom]. Any of left/top/zoom may
// be left "unchanged" (encoded as /null in PDF) via the Has* flags.
type DestinationXYZ struct {
	page                     *Page
	left, top, zoom          float64
	useLeft, useTop, useZoom bool
}

func NewDestinationXYZ(page *Page, left, top, zoom float64) *DestinationXYZ {
	return &DestinationXYZ{page: page, left: left, top: top, zoom: zoom,
		useLeft: true, useTop: true, useZoom: true}
}

func NewDestinationXYZUnchanged(page *Page, left float64, useLeft bool,
	top float64, useTop bool, zoom float64, useZoom bool) *DestinationXYZ {
	return &DestinationXYZ{page: page, left: left, top: top, zoom: zoom,
		useLeft: useLeft, useTop: useTop, useZoom: useZoom}
}

func (d *DestinationXYZ) DestinationType() DestinationType { return DestinationTypeXYZ }
func (d *DestinationXYZ) Page() *Page                      { return d.page }
func (d *DestinationXYZ) Left() float64                    { return d.left }
func (d *DestinationXYZ) Top() float64                     { return d.top }
func (d *DestinationXYZ) Zoom() float64                    { return d.zoom }
func (d *DestinationXYZ) HasLeft() bool                    { return d.useLeft }
func (d *DestinationXYZ) HasTop() bool                     { return d.useTop }
func (d *DestinationXYZ) HasZoom() bool                    { return d.useZoom }

// DestinationFit — [page /Fit]
type DestinationFit struct{ page *Page }

func NewDestinationFit(page *Page) *DestinationFit         { return &DestinationFit{page: page} }
func (d *DestinationFit) DestinationType() DestinationType { return DestinationTypeFit }
func (d *DestinationFit) Page() *Page                      { return d.page }

// DestinationFitH — [page /FitH top]
type DestinationFitH struct {
	page   *Page
	top    float64
	useTop bool
}

func NewDestinationFitH(page *Page, top float64) *DestinationFitH {
	return &DestinationFitH{page: page, top: top, useTop: true}
}

func NewDestinationFitHUnchanged(page *Page) *DestinationFitH {
	return &DestinationFitH{page: page, useTop: false}
}

func (d *DestinationFitH) DestinationType() DestinationType { return DestinationTypeFitH }
func (d *DestinationFitH) Page() *Page                      { return d.page }
func (d *DestinationFitH) Top() float64                     { return d.top }
func (d *DestinationFitH) HasTop() bool                     { return d.useTop }

// DestinationFitV — [page /FitV left]
type DestinationFitV struct {
	page    *Page
	left    float64
	useLeft bool
}

func NewDestinationFitV(page *Page, left float64) *DestinationFitV {
	return &DestinationFitV{page: page, left: left, useLeft: true}
}

func NewDestinationFitVUnchanged(page *Page) *DestinationFitV {
	return &DestinationFitV{page: page, useLeft: false}
}

func (d *DestinationFitV) DestinationType() DestinationType { return DestinationTypeFitV }
func (d *DestinationFitV) Page() *Page                      { return d.page }
func (d *DestinationFitV) Left() float64                    { return d.left }
func (d *DestinationFitV) HasLeft() bool                    { return d.useLeft }

// DestinationFitR — [page /FitR left bottom right top]
type DestinationFitR struct {
	page                     *Page
	left, bottom, right, top float64
}

func NewDestinationFitR(page *Page, left, bottom, right, top float64) *DestinationFitR {
	return &DestinationFitR{page: page, left: left, bottom: bottom, right: right, top: top}
}

func (d *DestinationFitR) DestinationType() DestinationType { return DestinationTypeFitR }
func (d *DestinationFitR) Page() *Page                      { return d.page }
func (d *DestinationFitR) Left() float64                    { return d.left }
func (d *DestinationFitR) Bottom() float64                  { return d.bottom }
func (d *DestinationFitR) Right() float64                   { return d.right }
func (d *DestinationFitR) Top() float64                     { return d.top }

// DestinationFitB — [page /FitB]
type DestinationFitB struct{ page *Page }

func NewDestinationFitB(page *Page) *DestinationFitB        { return &DestinationFitB{page: page} }
func (d *DestinationFitB) DestinationType() DestinationType { return DestinationTypeFitB }
func (d *DestinationFitB) Page() *Page                      { return d.page }

// DestinationFitBH — [page /FitBH top]
type DestinationFitBH struct {
	page   *Page
	top    float64
	useTop bool
}

func NewDestinationFitBH(page *Page, top float64) *DestinationFitBH {
	return &DestinationFitBH{page: page, top: top, useTop: true}
}

func NewDestinationFitBHUnchanged(page *Page) *DestinationFitBH {
	return &DestinationFitBH{page: page, useTop: false}
}

func (d *DestinationFitBH) DestinationType() DestinationType { return DestinationTypeFitBH }
func (d *DestinationFitBH) Page() *Page                      { return d.page }
func (d *DestinationFitBH) Top() float64                     { return d.top }
func (d *DestinationFitBH) HasTop() bool                     { return d.useTop }

// DestinationFitBV — [page /FitBV left]
type DestinationFitBV struct {
	page    *Page
	left    float64
	useLeft bool
}

func NewDestinationFitBV(page *Page, left float64) *DestinationFitBV {
	return &DestinationFitBV{page: page, left: left, useLeft: true}
}

func NewDestinationFitBVUnchanged(page *Page) *DestinationFitBV {
	return &DestinationFitBV{page: page, useLeft: false}
}

func (d *DestinationFitBV) DestinationType() DestinationType { return DestinationTypeFitBV }
func (d *DestinationFitBV) Page() *Page                      { return d.page }
func (d *DestinationFitBV) Left() float64                    { return d.left }
func (d *DestinationFitBV) HasLeft() bool                    { return d.useLeft }
