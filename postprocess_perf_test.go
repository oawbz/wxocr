package ocr

import (
	"image"
	"math/rand"
	"reflect"
	"testing"
)

func opening3Reference(mask []bool, w, h int) []bool {
	eroded := make([]bool, len(mask))
	out := make([]bool, len(mask))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := true
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && nx < w && ny >= 0 && ny < h && !mask[ny*w+nx] {
						value = false
					}
				}
			}
			eroded[y*w+x] = value
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && nx < w && ny >= 0 && ny < h && eroded[ny*w+nx] {
						value = true
					}
				}
			}
			out[y*w+x] = value
		}
	}
	return out
}

func TestOpening3Exact(t *testing.T) {
	rng := rand.New(rand.NewSource(19))
	for h := 1; h <= 21; h++ {
		for w := 1; w <= 23; w++ {
			for trial := 0; trial < 4; trial++ {
				a := make([]bool, w*h)
				for i := range a {
					a[i] = rng.Intn(4) != 0
				}
				if !reflect.DeepEqual(opening3(a, w, h), opening3Reference(a, w, h)) {
					t.Fatalf("%dx%d", w, h)
				}
			}
		}
	}
}
func TestPolygonScoreExact(t *testing.T) {
	rng := rand.New(rand.NewSource(29))
	const w, h = 43, 47
	prob := make([]float32, w*h)
	for i := range prob {
		prob[i] = rng.Float32()
	}
	polygons := [][]image.Point{{{0, 0}, {42, 0}, {42, 46}, {0, 46}}, {{5, 1}, {5, 20}, {5, 1}}, {{2, 2}, {19, 30}, {38, 4}, {5, 24}}}
	for i := 0; i < 300; i++ {
		p := make([]image.Point, 3+rng.Intn(12))
		for j := range p {
			p[j] = image.Pt(rng.Intn(w), rng.Intn(h))
		}
		polygons = append(polygons, p)
	}
	for i, p := range polygons {
		box := image.Rectangle{}
		for _, v := range p {
			box = box.Union(image.Rect(v.X, v.Y, v.X+1, v.Y+1))
		}
		var sum float64
		count := 0
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if polygonContains(p, x, y) {
					sum += float64(prob[y*w+x])
					count++
				}
			}
		}
		got, n := polygonProbability(prob, w, p, box)
		if got != sum || n != count {
			t.Fatalf("polygon %d sum %v/%v count %d/%d: %v", i, got, sum, n, count, p)
		}
	}
}
