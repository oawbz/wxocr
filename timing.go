package ocr

import (
	"context"
	"time"
)

type timingObserverKey struct{}

// WithTimingObserver reports elapsed wall time for successive OCR stages.
// Repeated stage names (one per text region) should be summed by the caller.
// The callback runs synchronously; it must be fast and must not call the engine.
func WithTimingObserver(ctx context.Context, observer func(string, time.Duration)) context.Context {
	return context.WithValue(ctx, timingObserverKey{}, observer)
}

type stageTimer struct {
	observer func(string, time.Duration)
	previous time.Time
}

func newStageTimer(ctx context.Context) stageTimer {
	observer, _ := ctx.Value(timingObserverKey{}).(func(string, time.Duration))
	t := stageTimer{observer: observer}
	if observer != nil {
		t.previous = time.Now()
	}
	return t
}
func (t *stageTimer) mark(name string) {
	if t.observer == nil {
		return
	}
	now := time.Now()
	t.observer(name, now.Sub(t.previous))
	t.previous = time.Now()
}
