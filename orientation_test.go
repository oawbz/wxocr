package ocr

import (
	"image"
	"math"
	"testing"
)

func TestPageOrientation(t *testing.T) {
	for _, c := range []struct {
		name   string
		logits []float32
		want   int
	}{
		{"upright", []float32{8, 0, 0, 0}, 0}, {"clockwise", []float32{0, 8, 0, 0}, 1},
		{"upside down", []float32{0, 0, 8, 0}, 2}, {"counterclockwise", []float32{0, 0, 0, 8}, 3},
		{"uncertain", []float32{0, 0, 0, 0}, 0}, {"180 safety override", []float32{0, -10, 1.5, -10}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, confidence, err := pageOrientation(c.logits)
			if err != nil || got != c.want || confidence < 0 || confidence > 1 {
				t.Fatalf("got %d %v %v", got, confidence, err)
			}
		})
	}
	for _, values := range [][]float32{{1, 2, 3}, {0, 0, float32(math.NaN()), 0}, {0, float32(math.Inf(1)), 0, 0}} {
		if _, _, err := pageOrientation(values); err == nil {
			t.Fatal("accepted invalid output")
		}
	}
}

func TestCropTransformOrientation(t *testing.T) {
	q := quadrilateral{{20, 10}, {100, 10}, {100, 40}, {20, 40}}
	for shift := 0; shift < 4; shift++ {
		oriented := q.oriented(shift)
		m := cropTransform(oriented, 80, 32)
		for i, d := range (quadrilateral{{0, 0}, {80, 0}, {80, 32}, {0, 32}}) {
			z := m[6]*d.x + m[7]*d.y + m[8]
			x := (m[0]*d.x + m[1]*d.y + m[2]) / z
			y := (m[3]*d.x + m[4]*d.y + m[5]) / z
			if math.Abs(x-oriented[i].x) > 1e-4 || math.Abs(y-oriented[i].y) > 1e-4 {
				t.Fatalf("shift %d corner %d: %v %v", shift, i, x, y)
			}
		}
	}
}

func TestNativeVerticalRectangleRounding(t *testing.T) {
	// Native row-ordered hull/caliper origin leaves the date box fractionally
	// below 241 pixels in length. Truncating to 240 changes the recognition tensor.
	q, _ := minimumRectangle([]image.Point{{412, 36}, {437, 36}, {437, 93}, {433, 277}, {416, 277}, {412, 273}})
	q = q.oriented(1)
	width := int(math.Hypot(float64(float32(q[1].x)-float32(q[0].x)), float64(float32(q[1].y)-float32(q[0].y))))
	height := int(math.Hypot(float64(float32(q[3].x)-float32(q[0].x)), float64(float32(q[3].y)-float32(q[0].y))))
	if width != 240 || height != 25 {
		t.Fatalf("crop %dx%d, want 240x25", width, height)
	}
}
