package ocr

import (
	"math"
	"testing"
)

func TestRecognitionProbabilities(t *testing.T) {
	data := []float32{0, 2, -4, 100, 102, 96, -math.MaxFloat32, math.MaxFloat32, 0}
	if err := recognitionProbabilities(data, 3); err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(data); offset += 3 {
		row := data[offset : offset+3]
		sum := float64(row[0] + row[1] + row[2])
		if math.Abs(sum-1) > 1e-6 || row[1] <= row[0] || row[1] <= row[2] {
			t.Fatalf("invalid probabilities %v", row)
		}
	}
	for i := 0; i < 3; i++ {
		if data[i] != data[i+3] {
			t.Fatal("normalization changed after constant shift")
		}
	}
	text, confidence, err := decodeCTC(data[:6], 2, []string{"A", "B"})
	if err != nil || text != "A" || math.Abs(float64(confidence)-float64(data[1]*data[4]+data[0]*data[4]+data[1]*data[3])) > 1e-6 {
		t.Fatalf("CTC: %q %v %v", text, confidence, err)
	}
}

func TestRecognitionRejectsInvalidLogits(t *testing.T) {
	for _, data := range [][]float32{nil, {1, 2}, {float32(math.NaN()), 0, 1}, {0, float32(math.Inf(1)), 1}} {
		if err := recognitionProbabilities(data, 3); err == nil {
			t.Fatalf("accepted %v", data)
		}
	}
	if err := recognitionProbabilities([]float32{1}, 0); err == nil {
		t.Fatal("accepted zero classes")
	}
}

func TestCTCSequencePathsAndUncertainty(t *testing.T) {
	// AA collapses to A; blank-A and A-blank also contribute. A-blank-A
	// instead represents AA and must not leak into the score for A.
	for _, c := range []struct {
		prob     []float32
		steps    int
		wantText string
		want     float64
	}{
		{[]float32{.2, .8, .3, .7}, 2, "A", .94},
		{[]float32{0, 1, 1, 0, 0, 1}, 3, "AA", 1},
		{[]float32{1, 0, 1, 0}, 2, "", 0},
	} {
		text, score, err := decodeCTC(c.prob, c.steps, []string{"A"})
		if err != nil || text != c.wantText || math.Abs(float64(score)-c.want) > 1e-6 {
			t.Fatalf("%q %v %v", text, score, err)
		}
	}
	// A weak final character should not be hidden by many certain characters.
	strong := make([]float32, 10*11)
	weak := make([]float32, len(strong))
	for t := 0; t < 10; t++ {
		strong[t*11+t+1] = 1
		weak[t*11+t+1] = 1
	}
	weak[9*11+10] = .51
	weak[9*11] = .49
	_, good, _ := decodeCTC(strong, 10, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"})
	_, uncertain, _ := decodeCTC(weak, 10, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"})
	if good != 1 || uncertain >= .951 {
		t.Fatalf("uncertainty hidden: strong %v, weak %v", good, uncertain)
	}
}

func TestSoftmaxAnalyticAndExtremeLogits(t *testing.T) {
	logits := []float32{0, 1, 2}
	if err := recognitionProbabilities(logits, 3); err != nil {
		t.Fatal(err)
	}
	sum := 1 + math.Exp(1) + math.Exp(2)
	for i, v := range logits {
		if math.Abs(float64(v)-math.Exp(float64(i))/sum) > 1e-7 {
			t.Fatal(logits)
		}
	}
	logits = []float32{-math.MaxFloat32, math.MaxFloat32, 0}
	if err := recognitionProbabilities(logits, 3); err != nil || logits[1] != 1 {
		t.Fatal(logits, err)
	}
}

func TestCTCForwardMatchesEnumeratedPaths(t *testing.T) {
	prob := []float32{.1, .7, .2, .2, .6, .2, .7, .1, .2, .1, .7, .2}
	var total float64
	for path := 0; path < 81; path++ {
		n := path
		weight := 1.0
		var decoded []int
		previous := -1
		for step := 0; step < 4; step++ {
			label := n % 3
			n /= 3
			weight *= float64(prob[step*3+label])
			if label != 0 && label != previous {
				decoded = append(decoded, label)
			}
			previous = label
		}
		if len(decoded) == 2 && decoded[0] == 1 && decoded[1] == 1 {
			total += weight
		}
	}
	text, score, err := decodeCTC(prob, 4, []string{"A", "B"})
	if err != nil || text != "AA" || math.Abs(float64(score*score)-total) > 1e-6 {
		t.Fatalf("%q %v vs enumerated %v: %v", text, score, total, err)
	}
}

func TestCTCLongSequenceDoesNotUnderflow(t *testing.T) {
	const steps = 1200
	prob := make([]float32, steps*3)
	for step := 0; step < steps; step++ {
		prob[step*3] = .49
		prob[step*3+1+step%2] = .51
	}
	_, score, err := decodeCTC(prob, steps, []string{"A", "B"})
	if err != nil || math.Abs(float64(score)-.51) > 1e-6 {
		t.Fatalf("long sequence confidence %v: %v", score, err)
	}
	for _, bad := range [][]float32{{.8, .8}, {-.1, 1.1}} {
		if _, _, err := decodeCTC(bad, 1, []string{"A"}); err == nil {
			t.Fatal("invalid probability accepted")
		}
	}
}

func TestCTCBlankPaddingDoesNotLowerTextConfidence(t *testing.T) {
	compact := []float32{.1, .8, .1, .1, .1, .8}
	padded := make([]float32, 0, 306)
	for i := 0; i < 50; i++ {
		padded = append(padded, .9, .05, .05)
	}
	padded = append(padded, compact...)
	for i := 0; i < 50; i++ {
		padded = append(padded, .9, .05, .05)
	}
	text, score, err := decodeCTC(compact, 2, []string{"A", "B"})
	ptext, pscore, perr := decodeCTC(padded, 102, []string{"A", "B"})
	if err != nil || perr != nil || text != ptext || score != pscore {
		t.Fatalf("padding changed text/score: %q %v vs %q %v: %v %v", text, score, ptext, pscore, err, perr)
	}
}

func TestCandidateEvidenceIsSeparateFromSequenceScore(t *testing.T) {
	prob := []float32{.1, .8, .1}
	for i := 0; i < 20; i++ {
		prob = append(prob, .51, .25, .24)
	}
	prob = append(prob, .1, .1, .8)
	text, score, evidence, err := recognitionCTC(prob, 22, []string{"A", "B"})
	if err != nil || text != "AB" || evidence < .79 || score >= .4 {
		t.Fatalf("%q sequence %v evidence %v: %v", text, score, evidence, err)
	}
}

func TestSoftmaxScratchPreservesExactResult(t *testing.T) {
	values := make([]float32, 3*257)
	for i := range values {
		values[i] = float32(math.Sin(float64(i)*.31) * 60)
	}
	expected := append([]float32(nil), values...)
	for offset := 0; offset < len(expected); offset += 257 {
		row := expected[offset : offset+257]
		maximum := row[0]
		for _, v := range row {
			maximum = max(maximum, v)
		}
		var sum float64
		for _, v := range row {
			sum += math.Exp(float64(v) - float64(maximum))
		}
		for i, v := range row {
			row[i] = float32(math.Exp(float64(v)-float64(maximum)) / sum)
		}
	}
	if err := recognitionProbabilities(values, 257); err != nil {
		t.Fatal(err)
	}
	for i := range values {
		if values[i] != expected[i] {
			t.Fatal("scratch normalization changed a probability", i)
		}
	}
}
