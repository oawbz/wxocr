package ocr

import (
	"image"
	"image/color"
	"testing"
)

func TestAdjacentFragmentsAreLineAligned(t *testing.T) {
	a := Line{Left: 10, Top: 20, Right: 40, Bottom: 40}
	b := Line{Left: 42, Top: 19, Right: 90, Bottom: 41}
	if !adjacentFragments(a, b) {
		t.Fatal("nearby fragments rejected")
	}
	row := b
	row.Top += 40
	row.Bottom += 40
	distant := b
	distant.Left += 60
	distant.Right += 60
	overlap := a
	overlap.Left += 2
	for _, v := range []Line{row, distant, overlap} {
		if adjacentFragments(a, v) {
			t.Fatal("independent line or duplicate joined")
		}
	}
}
func TestFragmentSeparatorBlocksNeighboringCells(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	for i := range im.Pix {
		im.Pix[i] = 255
	}
	a := Line{Left: 10, Top: 20, Right: 40, Bottom: 40}
	b := Line{Left: 44, Top: 20, Right: 90, Bottom: 40}
	if fragmentSeparator(im, a, b, 0) {
		t.Fatal("blank space treated as rule")
	}
	// A glyph stroke stops at the text height and must not become a cell divider.
	for y := 20; y < 40; y++ {
		im.SetNRGBA(42, y, color.NRGBA{A: 255})
	}
	if fragmentSeparator(im, a, b, 0) {
		t.Fatal("glyph treated as rule")
	}
	for y := 0; y < 80; y++ {
		im.SetNRGBA(42, y, color.NRGBA{A: 255})
	}
	if !fragmentSeparator(im, a, b, 0) {
		t.Fatal("cell border ignored")
	}
}
