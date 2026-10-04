package ocr

import (
	"image"
	"math"
	"testing"
	"wxocr/internal/polygonclip"
)

func TestParagraphOffsetNativeSmallCorners(t *testing.T) {
	// Small asymmetric outline replayed through the original UnClip export.
	p := []image.Point{{317, 170}, {318, 169}, {320, 171}, {319, 172}, {318, 172}, {317, 171}}
	area, perimeter := paragraphMetrics(p)
	distance := float64(float32(float64(float32(area)) * 1.6 / float64(float32(perimeter))))
	got := polygonclip.Offset(p, distance)
	want := map[image.Point]bool{{321, 170}: true, {320, 171}: true, {319, 173}: true, {318, 173}: true, {316, 171}: true, {316, 170}: true, {317, 169}: true, {319, 168}: true}
	if len(got) < 4 {
		t.Fatalf("degenerate offset: %v", got)
	}
	// Half-integer arc samples may round to adjacent pixels across native libm
	// implementations; compare geometry with an explicit one-pixel tolerance.
	for expected := range want {
		distance := math.Inf(1)
		for _, v := range got {
			distance = math.Min(distance, math.Max(math.Abs(float64(v.X-expected.X)), math.Abs(float64(v.Y-expected.Y))))
		}
		if distance > 1 {
			t.Fatalf("native corner %v differs by %g pixels: %v", expected, distance, got)
		}
	}

}

func TestParagraphRegionsAreRectangles(t *testing.T) {
	const w, h = 48, 32
	probability := make([]float32, w*h)
	for y := 8; y < 24; y++ {
		for x := 8; x < 40; x++ {
			if x < 24 || y < 16 {
				probability[y*w+x] = .9
			}
		}
	}
	regions, err := paragraphRegions(probability, w, h, w, h, 1, 1)
	if err != nil || len(regions) != 1 {
		t.Fatalf("regions=%v error=%v", regions, err)
	}
	q := regions[0].polygon
	if len(q) != 4 {
		t.Fatalf("expected four rectangle corners, got %d", len(q))
	}
	a, b := point{q[1].x - q[0].x, q[1].y - q[0].y}, point{q[2].x - q[1].x, q[2].y - q[1].y}
	if math.Abs(a.x*b.x+a.y*b.y) > 1e-3 {
		t.Fatal("region edges are not perpendicular")
	}
}

func TestParagraphThresholdMatchesFloat32Comparison(t *testing.T) {
	const size = 16
	p := make([]float32, size*size)
	for i := range p {
		p[i] = float32(.3)
	}
	regions, err := paragraphRegions(p, size, size, size, size, 1, 1)
	if err != nil || len(regions) != 0 {
		t.Fatalf("native CMP_GT must exclude values equal to float32(.3): %v, %v", regions, err)
	}
	for i := range p {
		p[i] = math.Nextafter32(float32(.3), 1)
	}
	regions, err = paragraphRegions(p, size, size, size, size, 1, 1)
	if err != nil || len(regions) != 1 {
		t.Fatalf("values above threshold should form one region: %v, %v", regions, err)
	}
}
