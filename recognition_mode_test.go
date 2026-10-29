package ocr

import (
	"context"
	"testing"
)

func TestRecognitionModeIsRequestScoped(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	enhanced := WithHighResolution(base, true)
	fast := WithHighResolution(enhanced, false)
	if HighResolutionEnabled(base) || !HighResolutionEnabled(enhanced) || HighResolutionEnabled(fast) {
		t.Fatal("mode leaked between request contexts")
	}
	cancel()
	if enhanced.Err() != context.Canceled || fast.Err() != context.Canceled {
		t.Fatal("mode discarded cancellation")
	}
}
