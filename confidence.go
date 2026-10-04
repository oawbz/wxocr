package ocr

import (
	"errors"
	"math"
)

// recognitionProbabilities applies stable Softmax to logits in place.
// Float64 exponentiation and accumulation avoid the native bit approximation.
func recognitionProbabilities(data []float32, classes int) error {
	if classes < 1 || len(data) == 0 || len(data)%classes != 0 {
		return errors.New("invalid recognition logits shape")
	}
	// Validate before mutating so malformed outputs never partly normalize.
	for _, value := range data {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("non-finite recognition logits")
		}
	}
	exponents := make([]float64, classes)
	for offset := 0; offset < len(data); offset += classes {
		row := data[offset : offset+classes]
		maximum := row[0]
		for _, value := range row {
			if value > maximum {
				maximum = value
			}
		}
		var sum float64
		for i, value := range row {
			exponents[i] = math.Exp(float64(value) - float64(maximum))
			sum += exponents[i]
		}
		for i := range row {
			row[i] = float32(exponents[i] / sum)
		}
	}
	return nil
}

// ctcSequenceConfidence sums CTC paths in the active text window yielding the decoded sequence, then
// takes a per-character geometric mean. It is model evidence, not a calibrated
// probability that the transcription is correct.
func ctcSequenceConfidence(prob []float32, steps, classes int, labels []int) float32 {
	if len(labels) == 0 {
		return 0
	}
	states := 2*len(labels) + 1
	previous, next := make([]float64, states), make([]float64, states)
	for i := range previous {
		previous[i] = math.Inf(-1)
	}
	previous[0] = math.Log(float64(prob[0]))
	previous[1] = math.Log(float64(prob[labels[0]]))
	label := func(s int) int {
		if s%2 == 0 {
			return 0
		}
		return labels[s/2]
	}
	add := func(a, b float64) float64 {
		if math.IsInf(a, -1) {
			return b
		}
		if math.IsInf(b, -1) {
			return a
		}
		if a < b {
			a, b = b, a
		}
		return a + math.Log1p(math.Exp(b-a))
	}
	for t := 1; t < steps; t++ {
		for s := 0; s < states; s++ {
			v := previous[s]
			if s > 0 {
				v = add(v, previous[s-1])
			}
			if s > 1 && s%2 != 0 && label(s) != label(s-2) {
				v = add(v, previous[s-2])
			}
			next[s] = v + math.Log(float64(prob[t*classes+label(s)]))
		}
		previous, next = next, previous
	}
	logProbability := add(previous[states-1], previous[states-2])
	return float32(math.Min(1, math.Exp(logProbability/float64(len(labels)))))
}
