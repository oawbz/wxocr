package ocr

import (
	"errors"
	"math"
)

// pageOrientation follows the native approximate softmax and conservative
// direction thresholds. The result is the corner shift for recognition crops.
func pageOrientation(logits []float32) (int, float32, error) {
	if len(logits) != 4 {
		return 0, 0, errors.New("invalid orientation output shape")
	}
	maximum := logits[0]
	for _, v := range logits {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return 0, 0, errors.New("non-finite orientation output")
		}
		if v > maximum {
			maximum = v
		}
	}
	var probabilities [4]float32
	var sum float32
	for i, v := range logits {
		exponent := math.FMA(float64(v-maximum), 1.4426950409, 126.93490512) * (1 << 23)
		if exponent > 0 {
			probabilities[i] = math.Float32frombits(uint32(exponent))
		}
		sum += probabilities[i]
	}
	best := 0
	for i := range probabilities {
		probabilities[i] /= sum
		if probabilities[i] > probabilities[best] {
			best = i
		}
	}
	confidence := probabilities[best]
	if float64(confidence) <= 0.7 || (best == 2 && float64(probabilities[0]) >= 0.15) {
		return 0, confidence, nil
	}
	return best, confidence, nil
}

func (q quadrilateral) oriented(shift int) quadrilateral {
	return quadrilateral{q[shift%4], q[(shift+1)%4], q[(shift+2)%4], q[(shift+3)%4]}
}
