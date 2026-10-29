package ocr

import "context"

type highResolutionKey struct{}

// WithHighResolution selects extra detection passes, region splitting and
// recognition retries for this request. The default is fast whole-image OCR.
// The option belongs to the context, so concurrent callers can use different
// modes without changing or reloading the shared models.
func WithHighResolution(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, highResolutionKey{}, enabled)
}

// HighResolutionEnabled reports whether enhanced recognition was requested.
func HighResolutionEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(highResolutionKey{}).(bool)
	return enabled
}
