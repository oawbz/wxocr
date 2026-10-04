package ocr

import (
	"image"
	"math"
)

// Native arcLength rounds each segment length to float32 before accumulation.
func paragraphMetrics(p []image.Point) (float64, float64) {
	area, _ := polygonMetrics(p)
	var length float64
	for i, a := range p {
		b := p[(i+1)%len(p)]
		dx, dy := float32(a.X-b.X), float32(a.Y-b.Y)
		length += float64(float32(math.Sqrt(float64(dx*dx + dy*dy))))
	}
	return area, length
}
