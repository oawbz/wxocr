package ocr

import (
	"context"
	"math"
	"math/rand"
	"testing"
)

func packCompact(prob []float32, steps, classes int) []float32 {
	greedy := make([]int, steps)
	for t := 0; t < steps; t++ {
		for c := 1; c < classes; c++ {
			if prob[t*classes+c] > prob[t*classes+greedy[t]] {
				greedy[t] = c
			}
		}
	}
	out := make([]float32, steps*(steps+2))
	for t := 0; t < steps; t++ {
		row := out[t*(steps+2) : (t+1)*(steps+2)]
		row[0] = float32(greedy[t])
		row[1] = prob[t*classes]
		for s, c := range greedy {
			row[2+s] = prob[t*classes+c]
		}
	}
	return out
}
func TestCompactCTCExact(t *testing.T) {
	rng := rand.New(rand.NewSource(46))
	charset := []string{"甲", "乙", "丙", "a", "b"}
	classes := len(charset) + 1
	for trial := 0; trial < 300; trial++ {
		steps := 1 + rng.Intn(40)
		prob := make([]float32, steps*classes)
		for step := 0; step < steps; step++ {
			var sum float32
			for c := 0; c < classes; c++ {
				v := float32(rng.Intn(5))
				if trial%5 == 0 {
					v = 1
				}
				if c == 0 && trial%7 == 0 {
					v = 10
				}
				prob[step*classes+c] = v
				sum += v
			}
			if sum == 0 {
				prob[step*classes] = 1
				sum = 1
			}
			for c := 0; c < classes; c++ {
				prob[step*classes+c] /= sum
			}
		}
		wt, wc, we, err := recognitionCTC(prob, steps, charset)
		if err != nil {
			t.Fatal(err)
		}
		gt, gc, ge, err := recognitionCompactCTC(packCompact(prob, steps, classes), steps, charset)
		if err != nil || gt != wt || gc != wc || ge != we {
			t.Fatalf("trial %d: got %q %v %v %v; want %q %v %v", trial, gt, gc, ge, err, wt, wc, we)
		}
	}
}
func TestCompactCTCRejectMalformed(t *testing.T) {
	for _, data := range [][]float32{nil, {0, 1}, {-1, 1, 1}, {1.5, 1, 1}, {2, 1, 1}, {0, float32(math.NaN()), 1}, {0, 1, float32(math.Inf(1))}, {0, 1, 1.1}} {
		if _, _, _, err := recognitionCompactCTC(data, 1, []string{"a"}); err == nil {
			t.Fatalf("accepted %v", data)
		}
	}
}
func TestWarmupClosedOrCanceled(t *testing.T) {
	if err := (&Engine{closed: true}).Warmup(context.Background()); err == nil {
		t.Fatal("closed engine warmed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Engine{}).Warmup(ctx); err != context.Canceled {
		t.Fatalf("canceled warmup: %v", err)
	}
}
