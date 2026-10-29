package main

import (
	"context"
	"errors"
	"image"
	"sync"

	ocr "wxocr"
)

// Each worker owns independent ONNX sessions. Never share an Engine concurrently.
type enginePool struct {
	mu       sync.Mutex
	engines  []managedRecognizer
	idle     chan managedRecognizer
	closing  chan struct{}
	done     chan struct{}
	active   sync.WaitGroup
	closed   bool
	closeErr error
}

func newEnginePool(engines []managedRecognizer) *enginePool {
	p := &enginePool{engines: engines, idle: make(chan managedRecognizer, len(engines)), closing: make(chan struct{}), done: make(chan struct{})}
	for _, e := range engines {
		p.idle <- e
	}
	return p
}

func (p *enginePool) Recognize(ctx context.Context, src image.Image) (*ocr.Result, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("engine pool is closed")
	}
	p.active.Add(1)
	p.mu.Unlock()
	defer p.active.Done()
	var e managedRecognizer
	select {
	case e = <-p.idle:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closing:
		return nil, errors.New("engine pool is closed")
	}
	defer func() { p.idle <- e }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.Recognize(ctx, src)
}

func (p *enginePool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		<-p.done
		return p.closeErr
	}
	p.closed = true
	close(p.closing)
	p.mu.Unlock()
	p.active.Wait()
	for _, e := range p.engines {
		p.closeErr = errors.Join(p.closeErr, e.Close())
	}
	close(p.done)
	return p.closeErr
}
