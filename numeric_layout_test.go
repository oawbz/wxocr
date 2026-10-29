package ocr

import (
	"image"
	"testing"
)

func TestContinuationUsesGeometryWithoutSupplyingDigits(t *testing.T) {
	layout := continuationLayout{columns: []float64{80, 150, 220}, bottom: 200, width: 300, height: 400}
	q := quadrilateral{{60, 220}, {240, 220}, {240, 240}, {60, 240}}
	pieces := layout.split(q)
	if len(pieces) != 4 {
		t.Fatal("continuation not split at repeated columns", pieces)
	}
	q = quadrilateral{{60, 120}, {240, 120}, {240, 140}, {60, 140}}
	if len(layout.split(q)) != 0 {
		t.Fatal("table body treated as continuation")
	}
	layout.columns = nil
	if len(layout.split(q)) != 0 {
		t.Fatal("split without table evidence")
	}
}
func TestOrientedTileGridReducesShortAxisOverlap(t *testing.T) {
	b := image.Rect(7, 11, 2487, 3517)
	if len(detectionTiles(b, 1)) != 9 {
		t.Fatal("expected eight local crops for sideways page")
	}
	if len(detectionTiles(b, 0)) != 10 {
		t.Fatal("expected nine local crops for upright page")
	}
	for _, dir := range []int{0, 1, 2, 3} {
		tiles := detectionTiles(b, dir)
		for y := b.Min.Y; y < b.Max.Y; y += 17 {
			for x := b.Min.X; x < b.Max.X; x += 17 {
				covered := false
				for _, r := range tiles[1:] {
					if image.Pt(x, y).In(r) {
						covered = true
						break
					}
				}
				if !covered {
					t.Fatal("grid gap", x, y, dir)
				}
			}
		}
	}
}
func TestCropPreservesDecodedPixelLayout(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 9, 65, 89))
	for i := range src.Pix {
		src.Pix[i] = byte(i * 7)
	}
	rect := image.Rect(13, 17, 45, 67)
	crop := cropImage(src, rect)
	if _, ok := crop.(*image.NRGBA); !ok {
		t.Fatal("native resize disabled by wrapper")
	}
	a, _ := tensorRGB(crop, 64, 96)
	b, _ := tensorRGB(imageView{src, rect}, 64, 96)
	if len(a) != len(b) {
		t.Fatal("size mismatch")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("changed crop pixels", i, a[i], b[i])
		}
	}
}

func TestGlobalFallbackRequiresUnionCoverage(t *testing.T) {
	d := Diagnostic{Quad: [4][2]float64{{0, 0}, {100, 0}, {100, 20}, {0, 20}}}
	half := Diagnostic{Pass: 1, Reason: "accepted", Quad: [4][2]float64{{0, 0}, {50, 0}, {50, 20}, {0, 20}}}
	if coveredByAcceptedLocal(d, []Diagnostic{half, half, half}) {
		t.Fatal("overlapping duplicates hid uncovered text")
	}
	other := half
	other.Quad = [4][2]float64{{50, 0}, {100, 0}, {100, 20}, {50, 20}}
	if !coveredByAcceptedLocal(d, []Diagnostic{half, other}) {
		t.Fatal("fully covered coarse crop not skipped")
	}
	other.Reason = "candidate"
	if coveredByAcceptedLocal(d, []Diagnostic{half, other}) {
		t.Fatal("candidate removed fallback")
	}
}
func TestReviewDoesNotCompleteOrAlterDate(t *testing.T) {
	for _, s := range []string{"1972-01-1", "1973-01-(", "2025-03-0"} {
		reasons := continuationReviewReasons(s)
		found := false
		for _, r := range reasons {
			if r == "incomplete_or_ambiguous_date" {
				found = true
			}
		}
		if !found {
			t.Fatal("incomplete date not marked", s, reasons)
		}
	}
	reasons := continuationReviewReasons("2005-01")
	if len(reasons) != 1 {
		t.Fatal("year-month field incorrectly marked incomplete", reasons)
	}
}

func TestUprightRetryOnlyForNarrowSingleGlyphShapes(t *testing.T) {
	if !narrowUprightCrop(quadrilateral{{0, 0}, {18, 0}, {18, 40}, {0, 40}}) {
		t.Fatal("narrow glyph not eligible")
	}
	for _, q := range []quadrilateral{{{0, 0}, {100, 0}, {100, 40}, {0, 40}}, {{0, 0}, {5, 0}, {5, 100}, {0, 100}}} {
		if narrowUprightCrop(q) {
			t.Fatal("whole word or vertical string selected for retry")
		}
	}
}
