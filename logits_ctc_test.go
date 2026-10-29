package ocr

import (
	"math"
	"math/rand"
	"testing"
)

func TestLogitsCompactMatchesFullReference(t *testing.T) {
	rng := rand.New(rand.NewSource(412))
	for _, steps := range []int{1, 10, 60} {
		for _, classes := range []int{3, 20, 13564} {
			charset := make([]string, classes-1)
			for i := range charset {
				charset[i] = string(rune(0x100 + i))
			}
			data := make([]float32, steps*classes)
			for i := range data {
				data[i] = float32(rng.NormFloat64() * 4)
			}
			// Include exact ties, blank emissions, and repeated labels.
			if steps > 1 {
				for c := 0; c < classes; c++ {
					data[c] = -100
				}
				data[0] = 20
				data[classes] = 20
				data[classes+1] = 20
			}
			reference := append([]float32(nil), data...)
			if err := recognitionProbabilities(reference, classes); err != nil {
				t.Fatal(err)
			}
			text, confidence, evidence, err := recognitionCTC(reference, steps, charset)
			if err != nil {
				t.Fatal(err)
			}
			got, gc, ge, err := recognitionLogitsCTC(data, steps, charset)
			if err != nil || got != text || math.Abs(float64(gc-confidence)) > 2e-6 || math.Abs(float64(ge-evidence)) > 2e-6 {
				t.Fatalf("mismatch %d/%d: %q %q %v %v %v", steps, classes, text, got, confidence, gc, err)
			}
		}
	}
	for _, v := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if _, _, _, err := recognitionLogitsCTC([]float32{v, 0}, 1, []string{"a"}); err == nil {
			t.Fatal("non-finite logits accepted")
		}
	}
}

func TestLogitsNearTieUsesRoundedProbabilityOrder(t *testing.T) {
	data := []float32{-100, 0, 1e-8}
	reference := append([]float32(nil), data...)
	if err := recognitionProbabilities(reference, 3); err != nil {
		t.Fatal(err)
	}
	want, _, _, _ := recognitionCTC(reference, 1, []string{"first", "second"})
	got, _, _, err := recognitionLogitsCTC(data, 1, []string{"first", "second"})
	if err != nil || got != want || want != "first" {
		t.Fatal("normalized probability tie changed", want, got, err)
	}
}
