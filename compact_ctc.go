package ocr

import (
	"fmt"
	"math"
	"strings"
)

// Compact output [1,T,T+2] contains, for each timestep: greedy class ID,
// blank probability, then probabilities of the greedy class at EVERY timestep.
// This retains every probability needed by exact CTC sequence confidence, unlike
// top-k pruning. Repeated classes keep their original identity in the DP.
func recognitionCompactCTC(data []float32, steps int, charset []string) (string, float32, float32, error) {
	if steps < 1 || steps > 4096 || len(data) != steps*(steps+2) {
		return "", 0, 0, fmt.Errorf("invalid compact recognition shape")
	}
	stride := steps + 2
	greedy := make([]int, steps)
	columns := []int{-1} // column 1 holds blank; other columns reference selected timesteps.
	ids := map[int]int{0: 0}
	for t := 0; t < steps; t++ {
		v := data[t*stride]
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < 0 || v > float32(len(charset)) || float32(int(v)) != v {
			return "", 0, 0, fmt.Errorf("invalid compact class ID")
		}
		greedy[t] = int(v)
		if _, ok := ids[int(v)]; !ok {
			ids[int(v)] = len(columns)
			columns = append(columns, t)
		}
		for _, p := range data[t*stride+1 : (t+1)*stride] {
			if math.IsNaN(float64(p)) || math.IsInf(float64(p), 0) || p < 0 || p > 1 {
				return "", 0, 0, fmt.Errorf("invalid compact probability")
			}
		}
	}
	first, last := -1, -1
	previous := 0
	var text strings.Builder
	var labels []int
	var emissions float64
	for t, g := range greedy {
		if g != 0 {
			if first < 0 {
				first = t
			}
			last = t
		}
		if g != 0 && g != previous {
			text.WriteString(charset[g-1])
			labels = append(labels, ids[g])
			emissions += float64(data[t*stride+2+t])
		}
		previous = g
	}
	if first < 0 {
		return "", 0, 0, nil
	}
	classes := len(columns)
	prob := make([]float32, (last-first+1)*classes)
	for t := first; t <= last; t++ {
		row := prob[(t-first)*classes : (t-first+1)*classes]
		row[0] = data[t*stride+1]
		for c := 1; c < classes; c++ {
			row[c] = data[t*stride+2+columns[c]]
		}
	}
	return text.String(), ctcSequenceConfidence(prob, last-first+1, classes, labels), float32(emissions / float64(len(labels))), nil
}
