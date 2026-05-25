// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

func TestHeuristicFont_Helvetica(t *testing.T) {
	tests := []struct {
		family string
		bold   bool
		italic bool
		want   Font
	}{
		{"Arial", false, false, FontHelvetica},
		{"Helvetica", false, false, FontHelvetica},
		{"sans-serif", false, false, FontHelvetica},
		{"Arial", true, false, FontHelveticaBold},
		{"Arial", false, true, FontHelveticaOblique},
		{"Arial", true, true, FontHelveticaBoldOblique},
	}
	for _, tt := range tests {
		got := heuristicFont(tt.family, tt.bold, tt.italic)
		if got.BaseFont() != tt.want.BaseFont() {
			t.Errorf("heuristicFont(%q, %v, %v) = %s, want %s",
				tt.family, tt.bold, tt.italic, got.BaseFont(), tt.want.BaseFont())
		}
	}
}

func TestHeuristicFont_Times(t *testing.T) {
	tests := []struct {
		family string
		bold   bool
		italic bool
		want   Font
	}{
		{"Times", false, false, FontTimesRoman},
		{"Times New Roman", false, false, FontTimesRoman},
		{"serif", false, false, FontTimesRoman},
		{"Georgia", false, false, FontTimesRoman},
		{"Times", true, false, FontTimesBold},
		{"Times", false, true, FontTimesItalic},
		{"Times", true, true, FontTimesBoldItalic},
	}
	for _, tt := range tests {
		got := heuristicFont(tt.family, tt.bold, tt.italic)
		if got.BaseFont() != tt.want.BaseFont() {
			t.Errorf("heuristicFont(%q, %v, %v) = %s, want %s",
				tt.family, tt.bold, tt.italic, got.BaseFont(), tt.want.BaseFont())
		}
	}
}

func TestHeuristicFont_Courier(t *testing.T) {
	tests := []struct {
		family string
		bold   bool
		italic bool
		want   Font
	}{
		{"Courier", false, false, FontCourier},
		{"Courier New", false, false, FontCourier},
		{"monospace", false, false, FontCourier},
		{"Courier", true, false, FontCourierBold},
		{"Courier", false, true, FontCourierOblique},
		{"Courier", true, true, FontCourierBoldOblique},
	}
	for _, tt := range tests {
		got := heuristicFont(tt.family, tt.bold, tt.italic)
		if got.BaseFont() != tt.want.BaseFont() {
			t.Errorf("heuristicFont(%q, %v, %v) = %s, want %s",
				tt.family, tt.bold, tt.italic, got.BaseFont(), tt.want.BaseFont())
		}
	}
}

func TestHeuristicFont_CommaList(t *testing.T) {
	got := heuristicFont("Times New Roman, Arial, sans-serif", false, false)
	if got.BaseFont() != FontTimesRoman.BaseFont() {
		t.Errorf("comma-list first match: got %s, want Times-Roman", got.BaseFont())
	}
}

func TestHeuristicFont_QuotedFamily(t *testing.T) {
	got := heuristicFont(`"Courier New"`, false, false)
	if got.BaseFont() != FontCourier.BaseFont() {
		t.Errorf("quoted family: got %s, want Courier", got.BaseFont())
	}
}

func TestHeuristicFont_UnknownFallsBackToHelvetica(t *testing.T) {
	got := heuristicFont("Wingdings", false, false)
	if got.BaseFont() != FontHelvetica.BaseFont() {
		t.Errorf("unknown family fallback: got %s, want Helvetica", got.BaseFont())
	}
}

func TestNormalizeSVGTextWhitespace(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello world", "hello world"},
		{"  hello   world  ", "hello world"},
		{"hello\nworld", "hello world"},
		{"hello\t\nworld", "hello world"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := normalizeSVGTextWhitespace(tt.in)
		if got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
