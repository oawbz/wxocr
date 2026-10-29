package ocr

import (
	"context"
	"errors"
	"fmt"
)

// Warmup initializes execution caches for every detector aspect-ratio bucket,
// all recognition width buckets, and the fixed paragraph shape. Call before
// accepting requests; it does not change recognition results or retain images.
func (e *Engine) Warmup(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("engine is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	run := func(m *model, w, h int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, _, err := m.runAll(make([]float32, 3*w*h), int64(h), int64(w))
		return err
	}
	// Two passes warm both first-use execution and reusable allocator state.
	for pass := 0; pass < 2; pass++ {
		for side := 160; side <= 960; side += 160 {
			if err := run(e.detector, 960, side); err != nil {
				return fmt.Errorf("warm detector 960x%d: %w", side, err)
			}
			if side != 960 {
				if err := run(e.detector, side, 960); err != nil {
					return fmt.Errorf("warm detector %dx960: %w", side, err)
				}
			}
		}
		for _, w := range []int{64, 128, 224, 352, 512, 704, 960} {
			if err := run(e.recognizer, w, 32); err != nil {
				return fmt.Errorf("warm recognizer width %d: %w", w, err)
			}
		}
		if err := run(e.paragraph, 768, 768); err != nil {
			return fmt.Errorf("warm paragraph: %w", err)
		}
	}
	return ctx.Err()
}
