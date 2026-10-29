package main

import (
	"context"
	"errors"
	"image"
	"sync/atomic"
	"testing"
	"time"
	ocr "wxocr"
)

type poolTestEngine struct {
	entered chan struct{}
	release chan struct{}
	closed  atomic.Int32
}

func (e *poolTestEngine) Recognize(ctx context.Context, _ image.Image) (*ocr.Result, error) {
	e.entered <- struct{}{}
	select {
	case <-e.release:
		return &ocr.Result{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (e *poolTestEngine) Close() error { e.closed.Add(1); return nil }
func TestEnginePoolParallelAndClose(t *testing.T) {
	entered, release := make(chan struct{}, 2), make(chan struct{})
	a, b := &poolTestEngine{entered: entered, release: release}, &poolTestEngine{entered: entered, release: release}
	p := newEnginePool([]managedRecognizer{a, b})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := p.Recognize(context.Background(), nil); results <- err }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not run in parallel")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Recognize(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting cancellation: %v", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case <-p.closing:
	case <-time.After(3 * time.Second):
		t.Fatal("close did not start")
	}
	if _, err := p.Recognize(context.Background(), nil); err == nil {
		t.Fatal("accepted after close")
	}
	if a.closed.Load() != 0 || b.closed.Load() != 0 {
		t.Fatal("closed active workers")
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if a.closed.Load() != 1 || b.closed.Load() != 1 {
		t.Fatal("workers not closed exactly once")
	}
}
func TestEngineWorkerConfig(t *testing.T) {
	c := defaultConfig()
	c.Token = "1234567890123456"
	c.EngineWorkers = 2
	if c.validate() == nil {
		t.Fatal("workers exceed request limit")
	}
	c.MaxConcurrentTasks = 2
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	c.ModelResident = false
	if c.validate() == nil {
		t.Fatal("nonresident pool accepted")
	}
}
